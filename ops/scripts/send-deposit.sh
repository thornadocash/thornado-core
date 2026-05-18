#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

if [[ -n "${VAULT_DEPOSIT_ADDRESS:-}" ]]; then
  amount="${DEPOSIT_AMOUNT_BTC:-1}"
  wallet="${CLIENT_WALLET:-client}"
  txid="$(bitcoin_cli -rpcwallet="${wallet}" sendtoaddress "${VAULT_DEPOSIT_ADDRESS}" "${amount}")"
  miner_wallet="${MINER_WALLET:-miner}"
  miner_addr="$(bitcoin_cli -rpcwallet="${miner_wallet}" getnewaddress miner bech32)"
  bitcoin_cli generatetoaddress 1 "${miner_addr}" >/dev/null
  echo "Sent deposit."
  echo "txid: ${txid}"
  echo "amount: ${amount}"
  echo "address: ${VAULT_DEPOSIT_ADDRESS}"
  exit 0
fi

run_hook_or_explain \
  SEND_DEPOSIT_CMD \
  "privacy deposit" \
  "derive/fetch vault deposit address, send regtest BTC with agreed note payload, and wait for Bifrost observation"
