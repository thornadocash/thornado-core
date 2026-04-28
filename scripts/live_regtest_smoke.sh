#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BITCOIND="${BITCOIND:-bitcoind}"
BITCOIN_CLI="${BITCOIN_CLI:-bitcoin-cli}"

command -v "$BITCOIND" >/dev/null
command -v "$BITCOIN_CLI" >/dev/null

DATA_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thornado-regtest.XXXXXX")"
SMOKE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thornado-rpc-smoke.XXXXXX")"
RPC_PORT="$((18443 + ($$ % 10000)))"
RPC_USER="thornado"
RPC_PASSWORD="thornado-pass"

cleanup() {
  "$BITCOIN_CLI" -regtest -datadir="$DATA_DIR" -rpcport="$RPC_PORT" \
    -rpcuser="$RPC_USER" -rpcpassword="$RPC_PASSWORD" stop >/dev/null 2>&1 || true
  rm -rf "$DATA_DIR" "$SMOKE_DIR"
}
trap cleanup EXIT

"$BITCOIND" -regtest -datadir="$DATA_DIR" -server -txindex=1 \
  -fallbackfee=0.0001 -rpcport="$RPC_PORT" \
  -rpcuser="$RPC_USER" -rpcpassword="$RPC_PASSWORD" -daemonwait >/dev/null

CLI=("$BITCOIN_CLI" -regtest -datadir="$DATA_DIR" -rpcport="$RPC_PORT" \
  -rpcuser="$RPC_USER" -rpcpassword="$RPC_PASSWORD")

"${CLI[@]}" createwallet vault >/dev/null
WALLET_CLI=("${CLI[@]}" -rpcwallet=vault)
MINER_ADDRESS="$("${WALLET_CLI[@]}" getnewaddress "" bech32)"
"${WALLET_CLI[@]}" generatetoaddress 101 "$MINER_ADDRESS" >/dev/null

RECIPIENT="$("${WALLET_CLI[@]}" getnewaddress "" bech32)"
CHANGE_ADDRESS="$("${WALLET_CLI[@]}" getnewaddress "" bech32)"
CHANGE_SCRIPT="$("${WALLET_CLI[@]}" getaddressinfo "$CHANGE_ADDRESS" \
  | sed -n 's/.*"scriptPubKey": "\([^"]*\)".*/\1/p')"

cat > "$SMOKE_DIR/Cargo.toml" <<EOF
[package]
name = "thornado-live-regtest-smoke"
version = "0.1.0"
edition = "2021"

[dependencies]
serde_json = "1"
thornado-bitcoin = { path = "$REPO_ROOT/crates/thornado-bitcoin" }
EOF
mkdir -p "$SMOKE_DIR/src"
cat > "$SMOKE_DIR/src/main.rs" <<'EOF'
use std::process::Command;
use thornado_bitcoin::{
    BitcoinBackend, BitcoinRpcConfig, BitcoinWithdrawalRequest, RpcBitcoinBackend,
    DEFAULT_FEE_RATE_SATS_PER_VB,
};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let rpc_url = std::env::var("BITCOIN_RPC_URL")?;
    let rpc_user = std::env::var("BITCOIN_RPC_USER")?;
    let rpc_password = std::env::var("BITCOIN_RPC_PASSWORD")?;
    let recipient = std::env::var("BITCOIN_RECIPIENT")?;
    let change_script_pubkey_hex = std::env::var("BITCOIN_CHANGE_SCRIPT")?;
    let bitcoin_cli = std::env::var("BITCOIN_CLI")?;
    let bitcoin_datadir = std::env::var("BITCOIN_DATADIR")?;
    let bitcoin_rpc_port = std::env::var("BITCOIN_RPC_PORT")?;

    let mut backend = RpcBitcoinBackend::new(BitcoinRpcConfig {
        url: rpc_url,
        user: rpc_user.clone(),
        password: rpc_password.clone(),
    })?;
    let utxos = backend.list_utxos();
    assert!(!utxos.is_empty(), "regtest wallet has no spendable UTXOs");

    let built = backend.build_withdrawal(BitcoinWithdrawalRequest {
        withdrawal_id: "live-wd-1".to_string(),
        recipient,
        amount_sats: 100_000_000,
        fee_rate_sats_per_vb: DEFAULT_FEE_RATE_SATS_PER_VB,
        change_script_pubkey_hex: Some(change_script_pubkey_hex),
        max_fee_rate_sats_per_vb: Some(200),
        min_relay_fee_sats: Some(1),
        max_inputs: Some(10),
        min_confirmations: Some(1),
        max_mempool_chain_length: Some(25),
    })?;
    let checkpoint =
        backend.validate_signing_checkpoint("live-wd-1", built.unsigned_tx_hex.clone())?;
    assert!(checkpoint.valid, "signing checkpoint failed");

    let signed = Command::new(bitcoin_cli)
        .arg("-regtest")
        .arg(format!("-datadir={bitcoin_datadir}"))
        .arg("-rpcwallet=vault")
        .arg(format!("-rpcport={bitcoin_rpc_port}"))
        .arg(format!("-rpcuser={rpc_user}"))
        .arg(format!("-rpcpassword={rpc_password}"))
        .arg("signrawtransactionwithwallet")
        .arg(&built.unsigned_tx_hex)
        .output()?;
    if !signed.status.success() {
        return Err(String::from_utf8_lossy(&signed.stderr).to_string().into());
    }
    let signed: serde_json::Value = serde_json::from_slice(&signed.stdout)?;
    assert_eq!(signed["complete"], true, "wallet did not fully sign transaction");
    let signed_tx_hex = signed["hex"]
        .as_str()
        .ok_or("missing signed transaction hex")?
        .to_string();

    let record = backend.broadcast_withdrawal("live-wd-1", signed_tx_hex)?;
    println!("{}", record.broadcast_txid.ok_or("missing broadcast txid")?);
    Ok(())
}
EOF

TXID="$(BITCOIN_RPC_URL="http://127.0.0.1:$RPC_PORT" \
  BITCOIN_RPC_USER="$RPC_USER" \
  BITCOIN_RPC_PASSWORD="$RPC_PASSWORD" \
  BITCOIN_RECIPIENT="$RECIPIENT" \
  BITCOIN_CHANGE_SCRIPT="$CHANGE_SCRIPT" \
  BITCOIN_CLI="$BITCOIN_CLI" \
  BITCOIN_DATADIR="$DATA_DIR" \
  BITCOIN_RPC_PORT="$RPC_PORT" \
  cargo run --quiet --manifest-path "$SMOKE_DIR/Cargo.toml")"

echo "live regtest withdrawal broadcast: $TXID"
