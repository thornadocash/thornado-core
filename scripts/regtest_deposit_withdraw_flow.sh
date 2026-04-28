#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

NODE_URL="${NODE_URL:-http://127.0.0.1:3000}"
BIFROST_URL="${BIFROST_URL:-$NODE_URL}"
BITCOIN_CLI_COMMAND="${BITCOIN_CLI_COMMAND:-${BITCOIN_CLI:-bitcoin-cli}}"
BITCOIN_CLI_ARGS="${BITCOIN_CLI_ARGS:--regtest -rpcuser=user -rpcpassword=password -rpcwallet=thornado}"
BITCOIN_CUSTODY_CLI_ARGS="${BITCOIN_CUSTODY_CLI_ARGS:--regtest -rpcuser=user -rpcpassword=password -rpcwallet=thornado-custody}"
FAUCET_WALLET="${FAUCET_WALLET:-thornado}"
DEPOSIT_SATS="${DEPOSIT_SATS:-100000000}"
WITHDRAW_FEE_SATS="${WITHDRAW_FEE_SATS:-100000}"
CLIENT_SEED="${CLIENT_SEED:-regtest-flow-client-seed}"
USER_PUBKEY="${USER_PUBKEY:-regtest-flow-client-pubkey}"
POW_LABEL="${POW_LABEL:-regtest-flow}"

FLOW_DIR="$(mktemp -d "${TMPDIR:-/tmp}/thornado-regtest-flow.XXXXXX")"
cleanup() {
  rm -rf "$FLOW_DIR"
}
trap cleanup EXIT

read -r -a BITCOIN_CMD <<< "$BITCOIN_CLI_COMMAND"
read -r -a BTC_ARGS <<< "$BITCOIN_CLI_ARGS"
read -r -a BTC_CUSTODY_ARGS <<< "$BITCOIN_CUSTODY_CLI_ARGS"

btc() {
  "${BITCOIN_CMD[@]}" "${BTC_ARGS[@]}" "$@"
}

btc_custody() {
  "${BITCOIN_CMD[@]}" "${BTC_CUSTODY_ARGS[@]}" "$@"
}

ensure_wallet() {
  if ! btc getwalletinfo >/dev/null 2>&1; then
    local base_args=()
    for arg in "${BTC_ARGS[@]}"; do
      case "$arg" in
        -rpcwallet=*|-rpcwallet)
          ;;
        *)
          base_args+=("$arg")
          ;;
      esac
    done
    "${BITCOIN_CMD[@]}" "${base_args[@]}" createwallet "$FAUCET_WALLET" >/dev/null 2>&1 || true
    "${BITCOIN_CMD[@]}" "${base_args[@]}" loadwallet "$FAUCET_WALLET" >/dev/null 2>&1 || true
  fi
}

ensure_faucet_funds() {
  local blocks miner_address
  blocks="$(btc getblockcount)"
  if [[ "$blocks" -lt 101 ]]; then
    miner_address="$(btc getnewaddress "" bech32)"
    btc generatetoaddress $((101 - blocks)) "$miner_address" >/dev/null
  fi
}

import_watchonly_address() {
  local address="$1"
  local descriptor info
  info="$(btc_custody getdescriptorinfo "addr($address)" 2>/dev/null || true)"
  descriptor="$(printf '%s' "$info" | sed -n 's/.*"descriptor"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  if [[ -n "$descriptor" ]]; then
    btc_custody importdescriptors "[{\"desc\":\"$descriptor\",\"timestamp\":\"now\",\"active\":false}]" >/dev/null 2>&1 || true
  else
    btc_custody importaddress "$address" "" false >/dev/null 2>&1 || true
  fi
}

cat > "$FLOW_DIR/Cargo.toml" <<EOF
[package]
name = "thornado-regtest-flow"
version = "0.1.0"
edition = "2021"

[dependencies]
anyhow = "1"
reqwest = { version = "0.12", features = ["blocking", "json"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
thornado-core = { path = "$REPO_ROOT/crates/thornado-core" }
EOF

mkdir -p "$FLOW_DIR/src"
cat > "$FLOW_DIR/src/main.rs" <<'EOF'
use anyhow::{bail, Context, Result};
use reqwest::blocking::Client;
use serde::Deserialize;
use serde_json::{json, Value};
use std::collections::BTreeSet;
use thornado_core::{
    derive_split_receipt, mine_deposit_pow, zk_withdrawal_from_receipt, DenominationTree,
};

#[derive(Debug, Deserialize)]
struct LeavesResponse {
    leaves: Vec<String>,
}

fn main() -> Result<()> {
    let node_url = std::env::var("NODE_URL")?;
    let bifrost_url = std::env::var("BIFROST_URL").unwrap_or_else(|_| node_url.clone());
    let deposit_sats = std::env::var("DEPOSIT_SATS")?.parse::<u64>()?;
    let withdraw_fee_sats = std::env::var("WITHDRAW_FEE_SATS")?.parse::<u64>()?;
    let client_seed = std::env::var("CLIENT_SEED")?;
    let user_pubkey = std::env::var("USER_PUBKEY")?;
    let pow_label = std::env::var("POW_LABEL")?;
    let recipient = std::env::var("WITHDRAW_RECIPIENT")?;
    let deposit_txid = std::env::var("DEPOSIT_TXID").ok();
    let existing_intent_id = std::env::var("INTENT_ID").ok();

    let client = Client::new();
    let intent_id = match existing_intent_id {
        Some(intent_id) => intent_id,
        None => {
            let pow_token = mine_deposit_pow(&pow_label);
            let requested = post(
                &client,
                &node_url,
                "/deposit/request",
                json!({ "pow_token": pow_token, "user_pubkey": user_pubkey }),
            )?;
            let created = find_event(&requested, "DepositIntentCreated")?;
            let intent_id = created["intent_id"]
                .as_str()
                .context("DepositIntentCreated.intent_id missing")?
                .to_string();
            let deposit_address = created["deposit_address"]
                .as_str()
                .context("DepositIntentCreated.deposit_address missing")?
                .to_string();

            println!("INTENT_ID={intent_id}");
            println!("DEPOSIT_ADDRESS={deposit_address}");
            intent_id
        }
    };

    if let Some(txid) = deposit_txid {
        post(
            &client,
            &node_url,
            "/deposit/observe",
            json!({
                "intent_id": intent_id,
                "txid": txid,
                "amount_sats": deposit_sats
            }),
        )?;
        post(
            &client,
            &node_url,
            "/deposit/confirm",
            json!({ "intent_id": intent_id }),
        )?;

        let receipt = derive_split_receipt(&intent_id, deposit_sats, &client_seed)?;
        post(
            &client,
            &node_url,
            "/split",
            json!({
                "deposit_id": intent_id,
                "note_commitments": receipt.commitments()
            }),
        )?;

        let note = receipt
            .notes
            .first()
            .context("split receipt did not produce notes")?;
        let leaves: LeavesResponse = client
            .get(format!("{node_url}/notes/leaves/{}", note.denomination_sats))
            .send()
            .context("GET /notes/leaves failed")?
            .error_for_status()
            .context("GET /notes/leaves returned error")?
            .json()
            .context("decode /notes/leaves failed")?;
        let mut tree = DenominationTree {
            leaves: Vec::new(),
            known_roots: BTreeSet::new(),
        };
        for leaf in leaves.leaves {
            tree.insert(leaf);
        }
        let (proof, public) = zk_withdrawal_from_receipt(
            note,
            &client_seed,
            &tree,
            recipient,
            withdraw_fee_sats,
        )?;
        let withdrawn = post(
            &client,
            &node_url,
            "/withdraw",
            json!({ "proof": proof, "public": public }),
        )?;
        println!("WITHDRAW_RESPONSE={}", serde_json::to_string(&withdrawn)?);
        if !has_event(&withdrawn, "WithdrawalAuthorized") {
            let tick = post(&client, &bifrost_url, "/bifrost/tick", json!({}))?;
            println!("BIFROST_TICK_RESPONSE={}", serde_json::to_string(&tick)?);
        }
        let bitcoin_tick = post(&client, &bifrost_url, "/bifrost/tick", json!({}))?;
        println!("BITCOIN_TICK_RESPONSE={}", serde_json::to_string(&bitcoin_tick)?);
        println!("FIRST_NOTE_DENOMINATION_SATS={}", note.denomination_sats);
    }

    Ok(())
}

fn post(client: &Client, node_url: &str, path: &str, body: Value) -> Result<Value> {
    let response = client
        .post(format!("{node_url}{path}"))
        .json(&body)
        .send()
        .with_context(|| format!("POST {path} failed"))?;
    let status = response.status();
    let value = response
        .json::<Value>()
        .with_context(|| format!("decode POST {path} response failed"))?;
    if !status.is_success() {
        bail!("POST {path} failed with {status}: {value}");
    }
    Ok(value)
}

fn find_event<'a>(response: &'a Value, variant: &str) -> Result<&'a Value> {
    response["events"]
        .as_array()
        .context("response.events missing")?
        .iter()
        .find_map(|event| event.get(variant))
        .with_context(|| format!("event {variant} not found in response"))
}

fn has_event(response: &Value, variant: &str) -> bool {
    response["events"]
        .as_array()
        .map(|events| events.iter().any(|event| event.get(variant).is_some()))
        .unwrap_or(false)
}
EOF

ensure_wallet
ensure_faucet_funds

WITHDRAW_RECIPIENT="$(btc getnewaddress "" bech32)"

REQUEST_OUTPUT="$(
  NODE_URL="$NODE_URL" \
  BIFROST_URL="$BIFROST_URL" \
  DEPOSIT_SATS="$DEPOSIT_SATS" \
  WITHDRAW_FEE_SATS="$WITHDRAW_FEE_SATS" \
  CLIENT_SEED="$CLIENT_SEED" \
  USER_PUBKEY="$USER_PUBKEY" \
  POW_LABEL="$POW_LABEL-$(date +%s)-$$" \
  WITHDRAW_RECIPIENT="$WITHDRAW_RECIPIENT" \
  cargo run --quiet --manifest-path "$FLOW_DIR/Cargo.toml"
)"

echo "$REQUEST_OUTPUT"
DEPOSIT_ADDRESS="$(printf '%s\n' "$REQUEST_OUTPUT" | sed -n 's/^DEPOSIT_ADDRESS=//p')"
INTENT_ID="$(printf '%s\n' "$REQUEST_OUTPUT" | sed -n 's/^INTENT_ID=//p')"

if [[ -z "$DEPOSIT_ADDRESS" || -z "$INTENT_ID" ]]; then
  echo "failed to parse deposit request output" >&2
  exit 1
fi

DEPOSIT_BTC="$(awk -v sats="$DEPOSIT_SATS" 'BEGIN { printf "%.8f", sats / 100000000 }')"
import_watchonly_address "$DEPOSIT_ADDRESS"
DEPOSIT_TXID="$(btc sendtoaddress "$DEPOSIT_ADDRESS" "$DEPOSIT_BTC")"
MINER_ADDRESS="$(btc getnewaddress "" bech32)"
btc generatetoaddress 1 "$MINER_ADDRESS" >/dev/null

echo "DEPOSIT_TXID=$DEPOSIT_TXID"
echo "WITHDRAW_RECIPIENT=$WITHDRAW_RECIPIENT"

NODE_URL="$NODE_URL" \
BIFROST_URL="$BIFROST_URL" \
DEPOSIT_SATS="$DEPOSIT_SATS" \
WITHDRAW_FEE_SATS="$WITHDRAW_FEE_SATS" \
CLIENT_SEED="$CLIENT_SEED" \
USER_PUBKEY="$USER_PUBKEY" \
POW_LABEL="$POW_LABEL-continue-$(date +%s)-$$" \
WITHDRAW_RECIPIENT="$WITHDRAW_RECIPIENT" \
INTENT_ID="$INTENT_ID" \
DEPOSIT_TXID="$DEPOSIT_TXID" \
cargo run --quiet --manifest-path "$FLOW_DIR/Cargo.toml" >/tmp/thornado-regtest-flow-final.$$.log

cat /tmp/thornado-regtest-flow-final.$$.log
rm -f /tmp/thornado-regtest-flow-final.$$.log
