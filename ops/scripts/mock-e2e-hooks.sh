#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

STATE_DIR="${OPS_DIR}/logs/mock-state"
STATE_FILE="${STATE_DIR}/state.env"
VAULT_WALLET="${VAULT_WALLET:-vault}"
DEPOSIT_AMOUNT_BTC="${DEPOSIT_AMOUNT_BTC:-1}"
WITHDRAWAL_FEE_BTC="${WITHDRAWAL_FEE_BTC:-0.0001}"

mkdir -p "${STATE_DIR}"

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

mine_one() {
  local miner_wallet="${MINER_WALLET:-miner}"
  ensure_wallet "${miner_wallet}"
  local miner_addr
  miner_addr="$(bitcoin_cli -rpcwallet="${miner_wallet}" getnewaddress miner bech32)"
  bitcoin_cli generatetoaddress 1 "${miner_addr}" >/dev/null
}

write_state_value() {
  local key="$1"
  local value="$2"
  touch "${STATE_FILE}"
  if grep -q "^${key}=" "${STATE_FILE}"; then
    python3 - "${STATE_FILE}" "${key}" "${value}" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
key = sys.argv[2]
value = sys.argv[3]
lines = path.read_text().splitlines()
with path.open("w") as f:
    for line in lines:
        if line.startswith(key + "="):
            f.write(f"{key}={value}\n")
        else:
            f.write(line + "\n")
PY
  else
    printf '%s=%s\n' "${key}" "${value}" >> "${STATE_FILE}"
  fi
}

load_state() {
  if [[ -f "${STATE_FILE}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${STATE_FILE}"
    set +a
  fi
}

ensure_request_state() {
 load_state
 ensure_wallet "${VAULT_WALLET}"
  if [[ -n "${REQUEST_DEPOSIT_ADDRESS:-}" ]]; then
    local address_mine
    address_mine="$(bitcoin_cli -rpcwallet="${VAULT_WALLET}" getaddressinfo "${REQUEST_DEPOSIT_ADDRESS}" | jq -r '.ismine')"
    if [[ "${address_mine}" != "true" ]]; then
      REQUEST_DEPOSIT_ADDRESS=""
    fi
  fi
  if [[ -z "${REQUEST_DEPOSIT_ADDRESS:-}" ]]; then
    REQUEST_DEPOSIT_ADDRESS="$(bitcoin_cli -rpcwallet="${VAULT_WALLET}" getnewaddress request bech32m)"
    write_state_value REQUEST_DEPOSIT_ADDRESS "${REQUEST_DEPOSIT_ADDRESS}"
  fi
  if [[ -z "${MOCK_VAULT_PUBKEY:-}" ]]; then
    MOCK_VAULT_PUBKEY="mock-frost-vault-$(date +%s)"
    write_state_value MOCK_VAULT_PUBKEY "${MOCK_VAULT_PUBKEY}"
  fi
}

bootstrap_thornado() {
  curl -fsS http://127.0.0.1:1317/health >/dev/null
  curl -fsS http://127.0.0.1:26657/status >/dev/null
  curl -fsS http://127.0.0.1:6040/health >/dev/null
  bitcoin_cli getblockchaininfo >/dev/null
  ensure_request_state
  write_state_value BOOTSTRAPPED_HEIGHT "$(bitcoin_cli getblockcount)"
  echo "mock Thornado bootstrap complete"
  echo "vault_pubkey=${MOCK_VAULT_PUBKEY}"
  echo "request_deposit_address=${REQUEST_DEPOSIT_ADDRESS}"
}

frost_dkg() {
  ensure_request_state
  write_state_value FROST_DKG_STATUS COMPLETE
  write_state_value FROST_DKG_SESSION_ID "mock-dkg-$(date +%s)"
  echo "mock FROST DKG complete"
  echo "status=COMPLETE"
  echo "vault_pubkey=${MOCK_VAULT_PUBKEY}"
  echo "bitcoin_address=${REQUEST_DEPOSIT_ADDRESS}"
}

frost_status() {
  load_state
  if [[ "${FROST_DKG_STATUS:-}" != "COMPLETE" ]]; then
    echo "mock FROST DKG not complete" >&2
    exit 1
  fi
  echo "status=${FROST_DKG_STATUS}"
  echo "session_id=${FROST_DKG_SESSION_ID:-unknown}"
  echo "vault_pubkey=${MOCK_VAULT_PUBKEY}"
  echo "bitcoin_address=${REQUEST_DEPOSIT_ADDRESS}"
}

send_deposit() {
  ensure_request_state
  ensure_wallet "${CLIENT_WALLET:-client}"
  local txid
  txid="$(bitcoin_cli -rpcwallet="${CLIENT_WALLET:-client}" sendtoaddress "${REQUEST_DEPOSIT_ADDRESS}" "${DEPOSIT_AMOUNT_BTC}")"
  mine_one
  write_state_value DEPOSIT_TXID "${txid}"
  write_state_value DEPOSIT_AMOUNT_BTC "${DEPOSIT_AMOUNT_BTC}"
  write_state_value DEPOSIT_STATUS committed
  echo "mock deposit complete"
  echo "txid=${txid}"
  echo "amount=${DEPOSIT_AMOUNT_BTC}"
  echo "address=${REQUEST_DEPOSIT_ADDRESS}"
}

run_withdrawal() {
  load_state
  if [[ -z "${DEPOSIT_TXID:-}" ]]; then
    send_deposit >/dev/null
    load_state
  fi
  ensure_wallet "${VAULT_WALLET}"
  ensure_wallet "${CLIENT_WALLET:-client}"
  local recipient amount txid
  recipient="$(bitcoin_cli -rpcwallet="${CLIENT_WALLET:-client}" getnewaddress recipient bech32)"
  amount="$(python3 - "${DEPOSIT_AMOUNT_BTC}" "${WITHDRAWAL_FEE_BTC}" <<'PY'
from decimal import Decimal
import sys
print(format(Decimal(sys.argv[1]) - Decimal(sys.argv[2]), "f"))
PY
)"
  txid="$(bitcoin_cli -rpcwallet="${VAULT_WALLET}" sendtoaddress "${recipient}" "${amount}")"
  mine_one
  write_state_value WITHDRAWAL_TXID "${txid}"
  write_state_value WITHDRAWAL_RECIPIENT "${recipient}"
  write_state_value WITHDRAWAL_AMOUNT_BTC "${amount}"
  write_state_value WITHDRAWAL_STATUS confirmed
  echo "mock withdrawal complete"
  echo "txid=${txid}"
  echo "recipient=${recipient}"
  echo "amount=${amount}"
}

case "${1:-}" in
  bootstrap-thornado)
    bootstrap_thornado
    ;;
  frost-dkg)
    frost_dkg
    ;;
  frost-status)
    frost_status
    ;;
  send-deposit)
    send_deposit
    ;;
  run-withdrawal)
    run_withdrawal
    ;;
  *)
    echo "usage: $0 {bootstrap-thornado|frost-dkg|frost-status|send-deposit|run-withdrawal}" >&2
    exit 1
    ;;
esac
