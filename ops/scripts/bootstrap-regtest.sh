#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

MINER_WALLET="${MINER_WALLET:-miner}"
CLIENT_WALLET="${CLIENT_WALLET:-client}"
BOOTSTRAP_BLOCKS="${BOOTSTRAP_BLOCKS:-101}"
CLIENT_FUNDS_BTC="${CLIENT_FUNDS_BTC:-25}"

wallet_exists() {
  local wallet="$1"
  bitcoin_cli listwalletdir | grep -q "\"name\": \"${wallet}\""
}

wallet_loaded() {
  local wallet="$1"
  bitcoin_cli listwallets | grep -q "\"${wallet}\""
}

ensure_wallet() {
  local wallet="$1"
  if ! wallet_exists "${wallet}"; then
    bitcoin_cli createwallet "${wallet}" >/dev/null
  elif ! wallet_loaded "${wallet}"; then
    bitcoin_cli loadwallet "${wallet}" >/dev/null
  fi
}

ensure_wallet "${MINER_WALLET}"
ensure_wallet "${CLIENT_WALLET}"

miner_addr="$(bitcoin_cli -rpcwallet="${MINER_WALLET}" getnewaddress miner bech32)"
bitcoin_cli generatetoaddress "${BOOTSTRAP_BLOCKS}" "${miner_addr}" >/dev/null

client_addr="$(bitcoin_cli -rpcwallet="${CLIENT_WALLET}" getnewaddress client bech32)"
bitcoin_cli -rpcwallet="${MINER_WALLET}" sendtoaddress "${client_addr}" "${CLIENT_FUNDS_BTC}" >/dev/null
bitcoin_cli generatetoaddress 1 "${miner_addr}" >/dev/null

echo "Regtest bootstrapped."
echo "Miner wallet: ${MINER_WALLET}"
echo "Client wallet: ${CLIENT_WALLET}"
echo "Client address: ${client_addr}"
echo "Client balance: $(bitcoin_cli -rpcwallet="${CLIENT_WALLET}" getbalance)"
