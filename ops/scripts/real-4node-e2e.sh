#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-real4-${RUN_ID}}"
CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_CONTAINER="${BTC_CONTAINER:-thornado-real4-${RUN_ID}-bitcoind}"
BTC_RPC_PORT="${BTC_RPC_PORT:-18445}"
BTC_P2P_PORT="${BTC_P2P_PORT:-18446}"
API_BASE="${API_BASE:-1316}"
GRPC_BASE="${GRPC_BASE:-9090}"
RPC_BASE="${RPC_BASE:-26656}"
P2P_BASE="${P2P_BASE:-26650}"
EBIFROST_BASE="${EBIFROST_BASE:-50050}"
FROST_P2P_BASE="${FROST_P2P_BASE:-5040}"
FROST_INFO_BASE="${FROST_INFO_BASE:-6040}"
METRICS_BASE="${METRICS_BASE:-9000}"
FLOW1_SCENARIO="${FLOW1_SCENARIO:-happy}"
FLOW1_SKIP_BIFROST_NODES="${FLOW1_SKIP_BIFROST_NODES:-}"
KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-1}"

THORNADO="${BUILD_DIR}/thornado"
BIFROST="${BUILD_DIR}/bifrost"
THORNADO_UI="${BUILD_DIR}/thornado-ui"
SHIELDER_HELPER="${BUILD_DIR}/shielder-e2e-helper"

log() {
  printf '[real4] %s\n' "$*"
}

die() {
  printf '[real4] ERROR: %s\n' "$*" >&2
  write_run_summary "FAIL" "$*"
  exit 1
}

write_run_summary() {
  local status="$1" message="${2:-}"
  mkdir -p "$RUN_ROOT/meta" 2>/dev/null || true
  jq -n \
    --arg status "$status" \
    --arg message "$message" \
    --arg run_root "$RUN_ROOT" \
    --arg btc_container "$BTC_CONTAINER" \
    --arg btc_rpc_port "$BTC_RPC_PORT" \
    --arg btc_p2p_port "$BTC_P2P_PORT" \
    --arg flow_limit "${FLOW_LIMIT:-7}" \
    --arg flow1_scenario "$FLOW1_SCENARIO" \
    --arg completed_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{status:$status,message:$message,run_root:$run_root,btc_container:$btc_container,btc_rpc_port:$btc_rpc_port,btc_p2p_port:$btc_p2p_port,flow_limit:$flow_limit,flow1_scenario:$flow1_scenario,completed_at:$completed_at}' \
    >"$RUN_ROOT/meta/run-summary.json" 2>/dev/null || true
}

json_get() {
  jq -r "$1"
}

api_port() { echo $((API_BASE + $1)); }
grpc_port() { echo $((GRPC_BASE + $1)); }
rpc_port() { echo $((RPC_BASE + $1)); }
p2p_port() { echo $((P2P_BASE + $1)); }
ebifrost_port() { echo $((EBIFROST_BASE + $1)); }
frost_p2p_port() { echo $((FROST_P2P_BASE + $1)); }
frost_info_port() { echo $((FROST_INFO_BASE + $1)); }
metrics_port() { echo $((METRICS_BASE + $1)); }

api_url() { echo "http://127.0.0.1:$(api_port "$1")"; }
rpc_url() { echo "http://127.0.0.1:$(rpc_port "$1")"; }

wait_tcp() {
  local host="$1" port="$2" label="$3" timeout="${4:-120}" start
  start="$(date +%s)"
  while true; do
    if (echo >"/dev/tcp/${host}/${port}") >/dev/null 2>&1; then
      log "${label} reachable"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "timed out waiting for ${label}"
    fi
    sleep 1
  done
}

wait_json() {
  local url="$1" label="$2" timeout="${3:-120}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      log "${label} ready"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "timed out waiting for ${label}: ${url}"
    fi
    sleep 2
  done
}

port_busy() {
  local port="$1"
  python3 - "$port" <<'PY'
import socket
import sys

port = int(sys.argv[1])
sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
try:
    sock.bind(("0.0.0.0", port))
except OSError:
    raise SystemExit(0)
raise SystemExit(1)
PY
}

next_free_port() {
  local port="$1"
  while port_busy "$port"; do
    port=$((port + 1))
  done
  echo "$port"
}

stop_pid_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  local pid
  pid="$(cat "$file" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    for _ in {1..20}; do
      kill -0 "$pid" >/dev/null 2>&1 || return 0
      sleep 0.1
    done
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
}

cleanup_stale_real4_services() {
  local file container
  shopt -s nullglob
  for file in /tmp/thornado-real4-*/pids/*.pid; do
    stop_pid_file "$file"
  done
  shopt -u nullglob

  if timeout 10 docker info >/dev/null 2>&1; then
    docker ps -a --format '{{.Names}}' \
      | awk '/^thornado-real4-.*-bitcoind$/ {print}' \
      | while read -r container; do
          timeout 15 docker rm -f "$container" >/dev/null 2>&1 || true
        done
  fi
}

cleanup_runtime() {
  local exit_code=$?
  if [[ "${KEEP_RUNNING:-0}" == "1" ]]; then
    if (( exit_code != 0 )); then
      log "KEEP_RUNNING=1; run failed but cluster remains live at ${RUN_ROOT}. Press Ctrl-C to stop this script."
      while true; do
        sleep 60
      done
    fi
    return "$exit_code"
  fi
  log "cleaning up runtime processes and bitcoind"
  for file in "$RUN_ROOT"/pids/*.pid; do
    stop_pid_file "$file"
  done
  timeout 15 docker rm -f "$BTC_CONTAINER" >/dev/null 2>&1 || true
  if [[ "$KEEP_ARTIFACTS" != "1" ]]; then
    rm -rf "$RUN_ROOT"
  fi
  return "$exit_code"
}

trap cleanup_runtime EXIT

pty_ed25519() {
  local home="$1" name="$2" mnemonic="$3"
  SIGNER_NAME="$name" SIGNER_PASSWD="$PASS" "$ROOT_DIR/ops/scripts/tty-expect.py" \
    "$THORNADO" ed25519 --home "$home" \
    --expect "Enter keyring passphrase" --send "$PASS" \
    --expect "Enter mnemonic" --send "$mnemonic" \
    --expect "Enter keyring passphrase" --send "$PASS" \
    --expect "Re-enter keyring passphrase" --send "$PASS"
}

pty_ed25519_mnemonic_first() {
  local home="$1" name="$2" mnemonic="$3"
  SIGNER_NAME="$name" SIGNER_PASSWD="$PASS" "$ROOT_DIR/ops/scripts/tty-expect.py" \
    "$THORNADO" ed25519 --home "$home" \
    --expect "Enter mnemonic" --send "$mnemonic" \
    --expect "Enter keyring passphrase" --send "$PASS" \
    --expect "Re-enter keyring passphrase" --send "$PASS"
}

key_add_file() {
  local home="$1" name="$2"
  printf '%s\n%s\n' "$PASS" "$PASS" | "$THORNADO" keys add "$name" \
    --home "$home" --keyring-backend file --output json
}

key_show_addr() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file -a
}

key_show_val_addr() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file --bech val -a
}

key_show_pub_bech() {
  local home="$1" name="$2"
  local pub_json
  pub_json="$(printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file -p)"
  "$THORNADO" pubkey "$pub_json"
}

key_export_hex() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | "$THORNADO" keys export "$name" \
    --home "$home" --keyring-backend file --unarmored-hex --unsafe --yes
}

cons_pub_bech() {
  local home="$1"
  local pub_json
  pub_json="$(jq -c '.pub_key | {"@type":"/cosmos.crypto.ed25519.PubKey","key":.value}' "$home/config/priv_validator_key.json")"
  "$THORNADO" pubkey --bech cons "$pub_json"
}

node_id() {
  "$THORNADO" comet show-node-id --home "$1"
}

btc_cli() {
  docker exec "$BTC_CONTAINER" bitcoin-cli -regtest -rpcuser=thornado -rpcpassword=thornado "$@"
}

thornado_tx() {
  local home="$1" from="$2"
  shift 2
  local from_addr
  if [[ "$from" == tthor1* ]]; then
    from_addr="$from"
  else
    from_addr="$(key_show_addr "$home" "$from")"
  fi
  printf '%s\n' "$PASS" | "$THORNADO" tx thornado "$@" \
    --home "$home" \
    --from "$from_addr" \
    --keyring-backend file \
    --keyring-dir "$home" \
    --chain-id "$CHAIN_ID" \
    --node tcp://127.0.0.1:$(rpc_port 1) \
    --gas 2500000 \
    --fees 0stake \
    --broadcast-mode sync \
    --yes \
    --output json
}

assert_tx_success() {
  local out="$1" label="$2" txhash start res code raw_log
  jq -e '.code == null or .code == 0' <<<"$out" >/dev/null || die "$label failed CheckTx: $out"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || return 0
  start="$(date +%s)"
  while (( $(date +%s) - start < 60 )); do
    res="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}" 2>/dev/null || true)"
    if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" == "0" ]]; then
        return 0
      fi
      printf '%s\n' "$out" >"$RUN_ROOT/meta/${label// /-}-checktx.json"
      printf '%s\n' "$res" >"$RUN_ROOT/meta/${label// /-}-delivertx.json"
      raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
      die "$label failed DeliverTx code=$code log=$raw_log"
    fi
    sleep 1
  done
  die "$label tx $txhash was not found"
}

assert_tx_rejected() {
  local out="$1" label="$2" want="${3:-}" txhash start res code raw_log safe
  safe="${label// /-}"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}-checktx.json"
  code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
  if [[ "$code" != "0" ]]; then
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
    if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
      die "$label rejected with unexpected CheckTx log: $raw_log"
    fi
    printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
    return 0
  fi
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "$label unexpectedly had no txhash and no error"
  start="$(date +%s)"
  while (( $(date +%s) - start < 60 )); do
    res="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}" 2>/dev/null || true)"
    if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
      printf '%s\n' "$res" >"$RUN_ROOT/meta/${safe}-delivertx.json"
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" != "0" ]]; then
        raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
        if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
          die "$label rejected with unexpected DeliverTx log: $raw_log"
        fi
        printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
        return 0
      fi
      die "$label unexpectedly succeeded"
    fi
    sleep 1
  done
  die "$label tx $txhash was not found"
}

assert_tx_or_cli_rejected() {
  local label="$1" want="${2:-}" out status safe raw_log
  shift 2
  safe="${label// /-}"
  set +e
  out="$("$@" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}-output.txt"
  if (( status != 0 )); then
    raw_log="$out"
    if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
      die "$label failed CLI with unexpected log: $raw_log"
    fi
    printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
    return 0
  fi
  assert_tx_rejected "$out" "$label" "$want"
}

wait_blocks() {
  local count="$1" start latest
  start="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  while true; do
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    (( latest >= start + count )) && return 0
    sleep 1
  done
}

request_deposit() {
  local home="$1" owner="$2" pow="$3" out owner_addr pow_token difficulty deposit_pubkey
  shift 3
  if (( $# == 0 )); then
    deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${pow}-deposit-pubkey")"
    set -- "$deposit_pubkey"
  else
    deposit_pubkey="$1"
  fi
  owner_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  difficulty="$(curl -fsS "$(api_url 1)/thornado/config" | jq -r '.int_64_values.Deposit_PowDifficultyCurrent // .int_64_values.Deposit_PowDifficultyMin // 20')"
  pow_token="$("$SHIELDER_HELPER" pow-token "$owner_addr" "$difficulty" "$pow")"
  out="$(thornado_tx "$home" "$owner" request-deposit "$pow_token" "$@")"
  assert_tx_success "$out" "request-deposit"
  wait_blocks 2
  echo "$out"
}

mine_to_registered_deposit() {
  local address="$1" amount_btc="$2"
  local utxo in_txid in_vout utxo_amount change_address change_amount inputs outputs raw signed txid
  utxo="$(btc_cli -rpcwallet=miner listunspent 1 9999999 | jq -c 'map(select(.spendable == true and .amount > 1))[0]')"
  [[ -n "$utxo" && "$utxo" != "null" ]] || die "miner wallet has no spendable UTXO"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  utxo_amount="$(jq -r '.amount' <<<"$utxo")"
  change_address="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  change_amount="$(awk -v u="$utxo_amount" -v a="$amount_btc" 'BEGIN {c = u - a - 0.00002000; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  outputs="$(jq -nc --arg address "$address" --argjson amount "$amount_btc" --arg change "$change_address" --argjson change_amount "$change_amount" '[{($address):$amount},{($change):$change_amount}]')"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$outputs")"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$raw" | jq -r '.hex')"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$signed")"
  mine_regtest_blocks 2
  echo "$txid"
}

wait_deposit_matched() {
  local deposit_id="$1" timeout="${2:-180}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -e '.status == "deposit_matched"' >/dev/null 2>&1; then
      curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" >&2 || true
      die "deposit ${deposit_id} did not match"
    fi
    if [[ -n "${BTC_CONTAINER:-}" ]] && docker ps --format '{{.Names}}' | grep -qx "$BTC_CONTAINER"; then
      mine_regtest_blocks 1 || true
    fi
    sleep 2
  done
}

wait_deposit_committed() {
  local deposit_id="$1" timeout="${2:-120}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -e '.status == "committed"' >/dev/null 2>&1; then
      curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" >&2 || true
      die "deposit ${deposit_id} did not commit"
    fi
    sleep 2
  done
}

deposit_session() {
  local owner_addr="$1"
  curl -fsS "$(api_url 1)/thornado/deposit/session/${owner_addr}"
}

node_query() {
  local node_addr="$1"
  curl -fsS "$(api_url 1)/thornado/node/address/${node_addr}"
}

wait_new_deposit_session() {
  local owner_addr="$1" previous_address="$2" previous_path="$3" timeout="${4:-60}" start session address path
  start="$(date +%s)"
  while true; do
    session="$(deposit_session "$owner_addr" 2>/dev/null || true)"
    if [[ -n "$session" ]]; then
      address="$(jq -r '.deposit_address // ""' <<<"$session")"
      path="$(jq -r '.deposit_path_index // ""' <<<"$session")"
      if [[ -n "$address" && "$address" != "null" ]] && { [[ "$address" != "$previous_address" ]] || [[ "$path" != "$previous_path" ]]; }; then
        printf '%s\n' "$session"
        return 0
      fi
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "fresh deposit session was not indexed for ${owner_addr}"
    fi
    sleep 1
  done
}

mine_regtest_blocks() {
  local count="${1:-1}"
  btc_cli -rpcwallet=miner generatetoaddress "$count" "$(btc_cli -rpcwallet=miner getnewaddress)" >/dev/null
}

wait_btc_balance_at_least() {
  local address="$1" min_btc="$2" timeout="${3:-240}" start
  start="$(date +%s)"
  while true; do
    local received
    received="$(btc_cli -rpcwallet=miner getreceivedbyaddress "$address" 0)"
    if awk "BEGIN {exit !($received >= $min_btc)}"; then
      echo "$received"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "bitcoin address ${address} received ${received}, wanted ${min_btc}"
    fi
    sleep 2
  done
}

record_shielder_leaf() {
  local denomination="$1" commitment="$2" file tmp
  file="$RUN_ROOT/meta/shielder-leaves-${denomination}.json"
  tmp="${file}.tmp"
  if [[ -f "$file" ]]; then
    jq --arg c "$commitment" '. + [$c] | unique | sort' "$file" >"$tmp"
  else
    jq -n --arg c "$commitment" '[$c]' >"$tmp"
  fi
  mv "$tmp" "$file"
}

record_shielder_notes() {
  local notes_json="$1"
  jq -c '.notes[] | {denomination_sats, commitment}' <<<"$notes_json" | while read -r note; do
    record_shielder_leaf "$(jq -r '.denomination_sats' <<<"$note")" "$(jq -r '.commitment' <<<"$note")"
  done
}

shielder_leaves() {
  local denomination="$1" file
  file="$RUN_ROOT/meta/shielder-leaves-${denomination}.json"
  [[ -f "$file" ]] || die "missing shielder leaves for denomination ${denomination}"
  jq -c 'sort' "$file"
}

kv_json_value() {
  local key="$1" file="$2" hex
  hex="$(printf '%s' "$key" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
  curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${hex}" >"$file"
  jq -r '.result.response.value // ""' "$file" | base64 -d | jq -r '.'
}

build_binaries() {
  log "building real Thornado and Bifrost binaries"
  mkdir -p "$BUILD_DIR"
  (cd "$ROOT_DIR" && cargo build -p thornado-ffi --release)
  "$ROOT_DIR/go-thornado/go-wrappers/frost/build-libgofrost.sh" >/dev/null
  (cd "$ROOT_DIR/go-thornado" && go build -tags 'regtest mocknet' -o "$THORNADO" ./cmd/thornado)
  (cd "$ROOT_DIR/go-thornado" && go build -tags 'regtest mocknet' -o "$BIFROST" ./cmd/bifrost)
  (cd "$ROOT_DIR/go-thornado" && go build -tags 'regtest mocknet' -o "$THORNADO_UI" ./cmd/thornado-ui)
  (cd "$ROOT_DIR/go-thornado" && go build -tags 'regtest mocknet' -o "$SHIELDER_HELPER" ./cmd/shielder-e2e-helper)
}

reset_all() {
  log "tearing down previous real4 state"
  cleanup_stale_real4_services
  for file in "$RUN_ROOT"/pids/*.pid; do
    stop_pid_file "$file"
  done
  sleep 1
  for file in "$RUN_ROOT"/pids/*.pid; do
    stop_pid_file "$file"
  done
  timeout 15 docker rm -f "$BTC_CONTAINER" >/dev/null 2>&1 || true
  rm -rf "$RUN_ROOT"
  mkdir -p "$RUN_ROOT"/{logs,pids,meta}
  write_run_summary "RUNNING" "initialized"
}

start_bitcoind() {
  BTC_RPC_PORT="$(next_free_port "$BTC_RPC_PORT")"
  BTC_P2P_PORT="$(next_free_port "$BTC_P2P_PORT")"
  if [[ "$BTC_P2P_PORT" == "$BTC_RPC_PORT" ]]; then
    BTC_P2P_PORT="$(next_free_port "$((BTC_P2P_PORT + 1))")"
  fi
  log "starting regtest bitcoind on ${BTC_RPC_PORT}"
  docker run -d --name "$BTC_CONTAINER" \
    -p "${BTC_RPC_PORT}:18443" -p "${BTC_P2P_PORT}:18444" \
    bitcoin/bitcoin:27 \
    -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
    -deprecatedrpc=create_bdb \
    -rpcbind=0.0.0.0 -rpcallowip=0.0.0.0/0 \
    -rpcuser=thornado -rpcpassword=thornado >/dev/null
  for _ in {1..60}; do
    if btc_cli getblockchaininfo >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  btc_cli createwallet miner >/dev/null 2>&1 || true
  for i in 1 2 3 4 6; do
    btc_cli createwallet "bifrost${i}" true true "" false true >/dev/null 2>&1 || true
  done
  local addr
  addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  btc_cli -rpcwallet=miner generatetoaddress 101 "$addr" >/dev/null
}

init_genesis() {
  log "creating 4-node genesis"
  local gen="$RUN_ROOT/node1/config/genesis.json"
  declare -a addrs valops secp ed cons cons_raw ids peers

  for i in 1 2 3 4; do
    local home="$RUN_ROOT/node${i}"
    "$THORNADO" init "node${i}" --chain-id "$CHAIN_ID" --home "$home" --overwrite >/dev/null
    sed -i.bak \
      -e 's/^addr_book_strict = .*/addr_book_strict = false/' \
      -e 's/^allow_duplicate_ip = .*/allow_duplicate_ip = true/' \
      "$home/config/config.toml"
    local key_json mnemonic
    key_json="$(key_add_file "$home" "validator${i}")"
    mnemonic="$(jq -r '.mnemonic' <<<"$key_json")"
    pty_ed25519 "$home" "validator${i}" "$mnemonic" >"$RUN_ROOT/logs/ed25519-${i}.log"
    addrs[$i]="$(key_show_addr "$home" "validator${i}")"
    valops[$i]="$(key_show_val_addr "$home" "validator${i}")"
    secp[$i]="$(key_show_pub_bech "$home" "validator${i}")"
    ed[$i]="$(tail -n1 "$RUN_ROOT/logs/ed25519-${i}.log" | tr -d '\r')"
    cons[$i]="$(cons_pub_bech "$home")"
    cons_raw[$i]="$(jq -r '.pub_key.value' "$home/config/priv_validator_key.json")"
    ids[$i]="$(node_id "$home")"
    peers[$i]="${ids[$i]}@127.0.0.1:$(p2p_port "$i")"
    "$THORNADO" genesis add-genesis-account "${addrs[$i]}" 100000000000000stake --home "$home" >/dev/null
    if [[ "$i" != "1" ]]; then
      "$THORNADO" genesis add-genesis-account "${addrs[$i]}" 100000000000000stake --home "$RUN_ROOT/node1" >/dev/null
    fi
  done

  for i in 5 6; do
    local home="$RUN_ROOT/node${i}"
    "$THORNADO" init "node${i}" --chain-id "$CHAIN_ID" --home "$home" --overwrite >/dev/null
    local key_json mnemonic
    key_json="$(key_add_file "$home" "validator${i}")"
    mnemonic="$(jq -r '.mnemonic' <<<"$key_json")"
    pty_ed25519_mnemonic_first "$home" "validator${i}" "$mnemonic" >"$RUN_ROOT/logs/ed25519-${i}.log"
    addrs[$i]="$(key_show_addr "$home" "validator${i}")"
    secp[$i]="$(key_show_pub_bech "$home" "validator${i}")"
    ed[$i]="$(tail -n1 "$RUN_ROOT/logs/ed25519-${i}.log" | tr -d '\r')"
    cons[$i]="$(cons_pub_bech "$home")"
    "$THORNADO" genesis add-genesis-account "${addrs[$i]}" 100000000000000stake --home "$RUN_ROOT/node1" >/dev/null
  done

  local user_json user_addr
  user_json="$(key_add_file "$RUN_ROOT/node1" "user")"
  user_addr="$(key_show_addr "$RUN_ROOT/node1" "user")"
  "$THORNADO" genesis add-genesis-account "$user_addr" 100000000000000stake --home "$RUN_ROOT/node1" >/dev/null

  for i in 2 3 4; do
    cp "$gen" "$RUN_ROOT/node${i}/config/genesis.json"
  done

  local node_accounts
  node_accounts="$(jq -n \
    --arg a1 "${addrs[1]}" --arg s1 "${secp[1]}" --arg e1 "${ed[1]}" --arg c1 "${cons[1]}" \
    --arg a2 "${addrs[2]}" --arg s2 "${secp[2]}" --arg e2 "${ed[2]}" --arg c2 "${cons[2]}" \
    --arg a3 "${addrs[3]}" --arg s3 "${secp[3]}" --arg e3 "${ed[3]}" --arg c3 "${cons[3]}" \
    --arg a4 "${addrs[4]}" --arg s4 "${secp[4]}" --arg e4 "${ed[4]}" --arg c4 "${cons[4]}" \
    '[
      {node_address:$a1, status:"Active", pub_key_set:{secp256k1:$s1, ed25519:$e1}, node_cons_pub_key:$c1, bond:"0", active_block_height:1, bond_address:$a1, status_since:1, signer_membership:[], version:"3.17.0"},
      {node_address:$a2, status:"Active", pub_key_set:{secp256k1:$s2, ed25519:$e2}, node_cons_pub_key:$c2, bond:"0", active_block_height:1, bond_address:$a2, status_since:1, signer_membership:[], version:"3.17.0"},
      {node_address:$a3, status:"Active", pub_key_set:{secp256k1:$s3, ed25519:$e3}, node_cons_pub_key:$c3, bond:"0", active_block_height:1, bond_address:$a3, status_since:1, signer_membership:[], version:"3.17.0"},
      {node_address:$a4, status:"Active", pub_key_set:{secp256k1:$s4, ed25519:$e4}, node_cons_pub_key:$c4, bond:"0", active_block_height:1, bond_address:$a4, status_since:1, signer_membership:[], version:"3.17.0"}
    ]')"

  case "$FLOW1_SCENARIO" in
    three_active)
      node_accounts="$(jq '.[3].status = "Standby"' <<<"$node_accounts")"
      ;;
    missing_secp)
      node_accounts="$(jq '.[3].pub_key_set.secp256k1 = ""' <<<"$node_accounts")"
      ;;
    duplicate_secp)
      node_accounts="$(jq '.[3].pub_key_set.secp256k1 = .[2].pub_key_set.secp256k1' <<<"$node_accounts")"
      ;;
  esac

  jq --argjson nodes "$node_accounts" \
    --arg c1raw "${cons_raw[1]}" --arg c2raw "${cons_raw[2]}" --arg c3raw "${cons_raw[3]}" --arg c4raw "${cons_raw[4]}" \
    --arg a1 "${addrs[1]}" --arg a2 "${addrs[2]}" --arg a3 "${addrs[3]}" --arg a4 "${addrs[4]}" \
    --arg v1 "${valops[1]}" --arg v2 "${valops[2]}" --arg v3 "${valops[3]}" --arg v4 "${valops[4]}" \
    '
    .validators = [
      {"address":"","pub_key":{"type":"tendermint/PubKeyEd25519","value":$c1raw},"power":"1","name":"node1"},
      {"address":"","pub_key":{"type":"tendermint/PubKeyEd25519","value":$c2raw},"power":"1","name":"node2"},
      {"address":"","pub_key":{"type":"tendermint/PubKeyEd25519","value":$c3raw},"power":"1","name":"node3"},
      {"address":"","pub_key":{"type":"tendermint/PubKeyEd25519","value":$c4raw},"power":"1","name":"node4"}
    ] |
    .app_state.genutil.gen_txs = [] |
    .app_state.bank.balances += [{"address":"tthor1fl48vsnmsdzcv85q5d2q4z5ajdha8yu3htpy4d","coins":[{"denom":"stake","amount":"4000000000000"}]}] |
    .app_state.bank.supply = [{"denom":"stake","amount":"704000000000000"}] |
    .app_state.staking.last_total_power = "4" |
    .app_state.staking.last_validator_powers = [
      {"address":$v1,"power":"1"},
      {"address":$v2,"power":"1"},
      {"address":$v3,"power":"1"},
      {"address":$v4,"power":"1"}
    ] |
    .app_state.staking.validators = [
      {"operator_address":$v1,"consensus_pubkey":{"@type":"/cosmos.crypto.ed25519.PubKey","key":$c1raw},"jailed":false,"status":"BOND_STATUS_BONDED","tokens":"1000000000000","delegator_shares":"1000000000000.000000000000000000","description":{"moniker":"node1","identity":"","website":"","security_contact":"","details":""},"unbonding_height":"0","unbonding_time":"1970-01-01T00:00:00Z","commission":{"commission_rates":{"rate":"0.100000000000000000","max_rate":"0.200000000000000000","max_change_rate":"0.010000000000000000"},"update_time":"1970-01-01T00:00:00Z"},"min_self_delegation":"1","unbonding_on_hold_ref_count":"0","unbonding_ids":[]},
      {"operator_address":$v2,"consensus_pubkey":{"@type":"/cosmos.crypto.ed25519.PubKey","key":$c2raw},"jailed":false,"status":"BOND_STATUS_BONDED","tokens":"1000000000000","delegator_shares":"1000000000000.000000000000000000","description":{"moniker":"node2","identity":"","website":"","security_contact":"","details":""},"unbonding_height":"0","unbonding_time":"1970-01-01T00:00:00Z","commission":{"commission_rates":{"rate":"0.100000000000000000","max_rate":"0.200000000000000000","max_change_rate":"0.010000000000000000"},"update_time":"1970-01-01T00:00:00Z"},"min_self_delegation":"1","unbonding_on_hold_ref_count":"0","unbonding_ids":[]},
      {"operator_address":$v3,"consensus_pubkey":{"@type":"/cosmos.crypto.ed25519.PubKey","key":$c3raw},"jailed":false,"status":"BOND_STATUS_BONDED","tokens":"1000000000000","delegator_shares":"1000000000000.000000000000000000","description":{"moniker":"node3","identity":"","website":"","security_contact":"","details":""},"unbonding_height":"0","unbonding_time":"1970-01-01T00:00:00Z","commission":{"commission_rates":{"rate":"0.100000000000000000","max_rate":"0.200000000000000000","max_change_rate":"0.010000000000000000"},"update_time":"1970-01-01T00:00:00Z"},"min_self_delegation":"1","unbonding_on_hold_ref_count":"0","unbonding_ids":[]},
      {"operator_address":$v4,"consensus_pubkey":{"@type":"/cosmos.crypto.ed25519.PubKey","key":$c4raw},"jailed":false,"status":"BOND_STATUS_BONDED","tokens":"1000000000000","delegator_shares":"1000000000000.000000000000000000","description":{"moniker":"node4","identity":"","website":"","security_contact":"","details":""},"unbonding_height":"0","unbonding_time":"1970-01-01T00:00:00Z","commission":{"commission_rates":{"rate":"0.100000000000000000","max_rate":"0.200000000000000000","max_change_rate":"0.010000000000000000"},"update_time":"1970-01-01T00:00:00Z"},"min_self_delegation":"1","unbonding_on_hold_ref_count":"0","unbonding_ids":[]}
    ] |
    .app_state.staking.delegations = [
      {"delegator_address":$a1,"validator_address":$v1,"shares":"1000000000000.000000000000000000"},
      {"delegator_address":$a2,"validator_address":$v2,"shares":"1000000000000.000000000000000000"},
      {"delegator_address":$a3,"validator_address":$v3,"shares":"1000000000000.000000000000000000"},
      {"delegator_address":$a4,"validator_address":$v4,"shares":"1000000000000.000000000000000000"}
    ] |
    .app_state.thornado = {
      observed_tx_in_voters: [],
      observed_tx_out_voters: [],
      tx_outs: [],
      node_accounts: $nodes,
      vaults: [],
      last_chain_heights: [{"chain":"BTC","height":101}],
      last_signed_height: 1,
      network: {},
      network_fees: [{"chain":"BTC","transaction_size":250,"transaction_fee_rate":1}],
      configs: [
        {"key":"NodePauseChainGlobal","value":0},
        {"key":"Node_SetDesired","value":4},
        {"key":"Vault_BaseMembersMin","value":4},
        {"key":"Node_BondStartAmountSats","value":0},
        {"key":"Node_BondSlotIncrementSats","value":100000000},
        {"key":"Churn_IntervalBlocks","value":20},
        {"key":"Churn_RetryIntervalBlocks","value":720},
        {"key":"Deposit_SessionExpiryMinutes","value":10},
        {"key":"Keysign_PeriodBlocks","value":300},
        {"key":"HaltSigningBTC","value":0},
        {"key":"Withdrawal_FeeMinSats","value":100000},
        {"key":"Keygen_RetryIntervalBlocks","value":5}
      ],
      nodeConfigs: [],
      config_defaults: []
    }
  ' "$RUN_ROOT/node1/config/genesis.json" >"$RUN_ROOT/genesis.json"

  if [[ "$FLOW1_SCENARIO" == "forged_vault_state" ]]; then
    jq \
      --arg s1 "${secp[1]}" --arg s2 "${secp[2]}" --arg s3 "${secp[3]}" --arg s4 "${secp[4]}" \
      '.app_state.thornado.vaults = [{
        block_height: 1,
        pub_key: "not-a-valid-thor-pubkey",
        coins: [],
        type: "BaseVault",
        status: "ActiveVault",
        status_since: 1,
        membership: [$s1, $s2, $s3, $s4],
        chains: ["BTC"],
        addresses: []
      }]' "$RUN_ROOT/genesis.json" >"$RUN_ROOT/genesis-forged-vault.json"
    mv "$RUN_ROOT/genesis-forged-vault.json" "$RUN_ROOT/genesis.json"
  fi

  for i in 1 2 3 4 5 6; do
    cp "$RUN_ROOT/genesis.json" "$RUN_ROOT/node${i}/config/genesis.json"
  done
  if ! "$THORNADO" genesis validate --home "$RUN_ROOT/node1" >"$RUN_ROOT/meta/genesis-validate.log" 2>&1; then
    case "$FLOW1_SCENARIO" in
      missing_secp|duplicate_secp|forged_vault_state)
        log "RESULTS Flow 1 ${FLOW1_SCENARIO}: PASS (genesis validation rejected fixture)"
        exit 0
        ;;
      *)
        cat "$RUN_ROOT/meta/genesis-validate.log" >&2 || true
        die "genesis validation failed"
        ;;
    esac
  fi

  printf '%s\n' "${peers[*]}" | tr ' ' ',' >"$RUN_ROOT/meta/peers"
  for i in 1 2 3 4; do
    {
      echo "address=${addrs[$i]}"
      echo "secp=${secp[$i]}"
      echo "ed=${ed[$i]}"
      echo "cons=${cons[$i]}"
    } >"$RUN_ROOT/meta/node${i}.env"
  done
  for i in 5 6; do
    {
      echo "address=${addrs[$i]}"
      echo "secp=${secp[$i]}"
      echo "ed=${ed[$i]}"
      echo "cons=${cons[$i]}"
    } >"$RUN_ROOT/meta/node${i}.env"
  done
  {
    echo "address=${user_addr}"
  } >"$RUN_ROOT/meta/user.env"
}

start_thornado_nodes() {
  log "starting Thornado validators"
  local peers
  peers="$(cat "$RUN_ROOT/meta/peers")"
  for i in 1 2 3 4; do
    local home="$RUN_ROOT/node${i}"
    SIGNER_NAME="validator${i}" \
    SIGNER_PASSWD="$PASS" \
    CHAIN_HOME_FOLDER="$home" \
    "$THORNADO" start \
      --home "$home" \
      --api.enable=true \
      --api.address "tcp://127.0.0.1:$(api_port "$i")" \
      --grpc.enable=true \
      --grpc.address "127.0.0.1:$(grpc_port "$i")" \
      --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
      --p2p.laddr "tcp://127.0.0.1:$(p2p_port "$i")" \
      --p2p.persistent_peers "$peers" \
      --p2p.pex=false \
      --ebifrost.enable=true \
      --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
      --minimum-gas-prices "0stake" \
      --log_level "info" \
      >"$RUN_ROOT/logs/thornado-${i}.log" 2>&1 &
    echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  done
  wait_json "$(rpc_url 1)/status" "thornado-1 rpc" 120
  for i in 1 2 3 4; do
    wait_json "http://127.0.0.1:$(rpc_port "$i")/status" "thornado-${i} rpc" 120
  done
}

restart_thornado_node() {
  local i="$1" home peers pid
  home="$RUN_ROOT/node${i}"
  peers="$(cat "$RUN_ROOT/meta/peers")"
  pid="$(cat "$RUN_ROOT/pids/thornado-${i}.pid" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    sleep 2
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
  SIGNER_NAME="validator${i}" \
  SIGNER_PASSWD="$PASS" \
  CHAIN_HOME_FOLDER="$home" \
  "$THORNADO" start \
    --home "$home" \
    --api.enable=true \
    --api.address "tcp://127.0.0.1:$(api_port "$i")" \
    --grpc.enable=true \
    --grpc.address "127.0.0.1:$(grpc_port "$i")" \
    --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
    --p2p.laddr "tcp://127.0.0.1:$(p2p_port "$i")" \
    --p2p.persistent_peers "$peers" \
    --p2p.pex=false \
    --ebifrost.enable=true \
    --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
    --minimum-gas-prices "0stake" \
    --log_level "info" \
    >"$RUN_ROOT/logs/thornado-${i}-restart.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  wait_json "http://127.0.0.1:$(rpc_port "$i")/status" "thornado-${i} restart rpc" 120
}

start_thornado_node6() {
  log "starting Thornado full node for node6"
  local i=6 home="$RUN_ROOT/node6" peers
  peers="$(cat "$RUN_ROOT/meta/peers")"
  SIGNER_NAME="validator6" \
  SIGNER_PASSWD="$PASS" \
  CHAIN_HOME_FOLDER="$home" \
  "$THORNADO" start \
    --home "$home" \
    --api.enable=true \
    --api.address "tcp://127.0.0.1:$(api_port 6)" \
    --grpc.enable=true \
    --grpc.address "127.0.0.1:$(grpc_port 6)" \
    --rpc.laddr "tcp://127.0.0.1:$(rpc_port 6)" \
    --p2p.laddr "tcp://127.0.0.1:$(p2p_port 6)" \
    --p2p.persistent_peers "$peers" \
    --p2p.pex=false \
    --ebifrost.enable=true \
    --ebifrost.address "127.0.0.1:$(ebifrost_port 6)" \
    --minimum-gas-prices "0stake" \
    --log_level "info" \
    >"$RUN_ROOT/logs/thornado-${i}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  wait_json "$(rpc_url 6)/status" "thornado-6 rpc" 120
  local target height start
  start="$(date +%s)"
  while true; do
    target="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    height="$(curl -fsS "$(rpc_url 6)/status" | jq -r '.result.sync_info.latest_block_height')"
    if (( height + 1 >= target )); then
      break
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "thornado-6 did not catch up"
    fi
    sleep 1
  done
}

wait_bifrost_health() {
  local i="$1" timeout="${2:-120}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/ping" >/dev/null 2>&1; then
      log "bifrost-${i} health ready"
      return 0
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
      tail -n 120 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "bifrost-${i} exited before health was ready"
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      tail -n 120 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "timed out waiting for bifrost-${i} health"
    fi
    sleep 1
  done
}

start_bifrost_nodes() {
  log "starting Bifrost signers"
  local bootstrap=""
  for i in 1 2 3 4; do
    if [[ ",${FLOW1_SKIP_BIFROST_NODES}," == *",${i},"* ]]; then
      log "skipping bifrost-${i} for scenario"
      continue
    fi
    local home="$RUN_ROOT/node${i}"
    local bhome="$RUN_ROOT/bifrost${i}"
    mkdir -p "$bhome"
    SIGNER_NAME="validator${i}" \
    SIGNER_PASSWD="$PASS" \
    BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
    BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:$(api_port "$i")" \
    BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
    BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:$(ebifrost_port "$i")" \
    BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
    BIFROST_THORNADO_SIGNER_NAME="validator${i}" \
    THOR_BLOCK_TIME="100ms" \
    BLOCK_SCANNER_BACKOFF="100ms" \
    CHAIN_ID="$CHAIN_ID" \
    CHAIN_API="127.0.0.1:$(api_port "$i")" \
    CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
    BIFROST_METRICS_LISTEN_PORT="$(metrics_port "$i")" \
    BIFROST_FROST_P2P_PORT="$(frost_p2p_port "$i")" \
    BIFROST_FROST_INFO_ADDRESS="127.0.0.1:$(frost_info_port "$i")" \
    BIFROST_TSS_INFO_ADDRESS="127.0.0.1:$(frost_info_port "$i")" \
    BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
    BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
    BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
    PEER="$bootstrap" \
    EXTERNAL_IP="127.0.0.1" \
    BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
    BIFROST_SIGNER_KEYGEN_TIMEOUT="5s" \
    BIFROST_SIGNER_KEYSIGN_TIMEOUT="5s" \
    BIFROST_SIGNER_PARTY_TIMEOUT="5s" \
    BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5s" \
    BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="1" \
    BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
    BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
    BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
    BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
    BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
    BIFROST_CHAINS_BTC_USERNAME="thornado" \
    BIFROST_CHAINS_BTC_PASSWORD="thornado" \
    BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost${i}" \
    BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
    BTC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost" \
    BTC_START_BLOCK_HEIGHT="0" \
    "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
    echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
    sleep 2
    wait_bifrost_health "$i" 120
    if [[ -z "$bootstrap" ]]; then
      for _ in {1..60}; do
        if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/p2pid" >/tmp/bifrost-p2pid.txt 2>/dev/null; then
          local peer
          peer="$(tr -d '[:space:]' </tmp/bifrost-p2pid.txt)"
          if [[ -n "$peer" ]]; then
            bootstrap="/ip4/127.0.0.1/tcp/$(frost_p2p_port "$i")/p2p/${peer}"
            printf '%s\n' "$bootstrap" >"$RUN_ROOT/meta/bifrost-bootstrap"
            break
          fi
        fi
        if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
          tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
          die "bifrost-${i} exited before health was ready"
        fi
        sleep 1
      done
    fi
    if [[ "$FLOW1_SCENARIO" == "mid_keygen_restart" && "$i" == "2" ]]; then
      log "restarting thornado-1 during Flow 1 keygen"
      restart_thornado_node 1
    fi
  done
  for i in 1 2 3 4; do
    if [[ ",${FLOW1_SKIP_BIFROST_NODES}," == *",${i},"* ]]; then
      continue
    fi
    wait_bifrost_health "$i" 120
  done
  local peers=()
  for i in 1 2 3 4; do
    if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/p2pid" >/tmp/bifrost-p2pid.txt 2>/dev/null; then
      local peer
      peer="$(tr -d '[:space:]' </tmp/bifrost-p2pid.txt)"
      if [[ -n "$peer" ]]; then
        peers+=("/ip4/127.0.0.1/tcp/$(frost_p2p_port "$i")/p2p/${peer}")
      fi
    fi
  done
  if (( ${#peers[@]} > 0 )); then
    (IFS=,; printf '%s\n' "${peers[*]}") >"$RUN_ROOT/meta/bifrost-bootstrap-all"
  fi
}

start_bifrost_node_for_flow1() {
  local i="$1" start_height="${2:-1}" home="$RUN_ROOT/node${i}" bhome="$RUN_ROOT/bifrost${i}" bootstrap
  bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap" 2>/dev/null || true)"
  mkdir -p "$bhome"
  SIGNER_NAME="validator${i}" \
  SIGNER_PASSWD="$PASS" \
  BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
  BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:$(api_port "$i")" \
  BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
  BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:$(ebifrost_port "$i")" \
  BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
  BIFROST_THORNADO_SIGNER_NAME="validator${i}" \
  THOR_BLOCK_TIME="100ms" \
  BLOCK_SCANNER_BACKOFF="100ms" \
  CHAIN_ID="$CHAIN_ID" \
  CHAIN_API="127.0.0.1:$(api_port "$i")" \
  CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
  BIFROST_METRICS_LISTEN_PORT="$(metrics_port "$i")" \
  BIFROST_FROST_P2P_PORT="$(frost_p2p_port "$i")" \
  BIFROST_FROST_INFO_ADDRESS="127.0.0.1:$(frost_info_port "$i")" \
  BIFROST_TSS_INFO_ADDRESS="127.0.0.1:$(frost_info_port "$i")" \
  BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
  BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
  BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
  PEER="$bootstrap" \
  EXTERNAL_IP="127.0.0.1" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
  BIFROST_SIGNER_KEYGEN_TIMEOUT="5s" \
  BIFROST_SIGNER_KEYSIGN_TIMEOUT="5s" \
  BIFROST_SIGNER_PARTY_TIMEOUT="5s" \
  BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5s" \
  BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_height" \
  BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
  BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
  BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
  BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost${i}" \
  BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
  BTC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost" \
  BTC_START_BLOCK_HEIGHT="0" \
  "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
  local start
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/ping" >/dev/null 2>&1; then
      log "bifrost-${i} health ready"
      return 0
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
      tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "bifrost-${i} exited before health was ready"
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "timed out waiting for bifrost-${i} health"
    fi
    sleep 1
  done
}

start_bifrost_node6() {
  log "starting Bifrost signer for node6"
  local i=6 home="$RUN_ROOT/node6" bhome="$RUN_ROOT/bifrost6" bootstrap start_block
  bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap-all" 2>/dev/null || cat "$RUN_ROOT/meta/bifrost-bootstrap")"
  start_block="$(curl -fsS "$(rpc_url 6)/status" | jq -r '.result.sync_info.latest_block_height')"
  start_block=$((start_block - 10))
  if (( start_block < 1 )); then
    start_block=1
  fi
  printf '%s\n' "$start_block" >"$RUN_ROOT/meta/flow6-bifrost6-signer-start-height.txt"
  mkdir -p "$bhome"
  SIGNER_NAME="validator6" \
  SIGNER_PASSWD="$PASS" \
  BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
  BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:$(api_port 6)" \
  BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:$(rpc_port 6)" \
  BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:$(ebifrost_port 6)" \
  BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
  BIFROST_THORNADO_SIGNER_NAME="validator6" \
  THOR_BLOCK_TIME="100ms" \
  BLOCK_SCANNER_BACKOFF="100ms" \
  CHAIN_ID="$CHAIN_ID" \
  CHAIN_API="127.0.0.1:$(api_port 6)" \
  CHAIN_RPC="127.0.0.1:$(rpc_port 6)" \
  BIFROST_METRICS_LISTEN_PORT="$(metrics_port 6)" \
  BIFROST_FROST_P2P_PORT="$(frost_p2p_port 6)" \
  BIFROST_FROST_INFO_ADDRESS="127.0.0.1:$(frost_info_port 6)" \
  BIFROST_TSS_INFO_ADDRESS="127.0.0.1:$(frost_info_port 6)" \
  BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
  BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
  BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
  PEER="$bootstrap" \
  EXTERNAL_IP="127.0.0.1" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
  BIFROST_SIGNER_KEYGEN_TIMEOUT="5s" \
  BIFROST_SIGNER_KEYSIGN_TIMEOUT="5s" \
  BIFROST_SIGNER_PARTY_TIMEOUT="5s" \
  BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5s" \
  BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_block" \
  BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
  BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
  BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
  BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost6" \
  BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
  BTC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost" \
  BTC_START_BLOCK_HEIGHT="0" \
  "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
  local start
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:$(frost_info_port 6)/ping" >/dev/null 2>&1; then
      log "bifrost-6 health ready"
      break
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
      tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "bifrost-6 exited before health was ready"
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "timed out waiting for bifrost-6 health"
    fi
    sleep 1
  done
}

wait_bifrost6_ready_for_keygen() {
  log "waiting for Bifrost-6 signer scanner and peer discovery"
  local start peers
  start="$(date +%s)"
  while true; do
    peers="$(rg -o 'peer found|Connection established|upgraded connection from allowed node|accepted inbound connection from allowed node' "$RUN_ROOT/logs/bifrost-6.log" 2>/dev/null | wc -l | tr -d '[:space:]')"
    if rg -q "start to process keygen" "$RUN_ROOT/logs/bifrost-6.log" 2>/dev/null && (( peers >= 4 )); then
      return 0
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-6.pid")" >/dev/null 2>&1; then
      tail -n 120 "$RUN_ROOT/logs/bifrost-6.log" >&2 || true
      die "bifrost-6 exited before keygen readiness"
    fi
    if (( "$(date +%s)" - start >= 180 )); then
      tail -n 160 "$RUN_ROOT/logs/bifrost-6.log" >&2 || true
      die "timed out waiting for bifrost-6 keygen readiness"
    fi
    sleep 2
  done
}

validate_flow1() {
  log "Flow 1: validating genesis, auto keygen request, and FROST vault"
  local height latest found=0
  for _ in {1..120}; do
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    if (( latest > 1 )); then
      break
    fi
    sleep 1
  done

  local active
  active="$(curl -fsS $(api_url 1)/thornado/nodes | jq '[((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active")] | length')"
  [[ "$active" == "4" ]] || die "expected 4 active nodes, got ${active}"

  for _ in {1..180}; do
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    for ((height=1; height<=latest; height++)); do
      if curl -fsS "$(api_url 1)/thornado/keygen/${height}/$(source "$RUN_ROOT/meta/node1.env"; echo "$secp")" \
        | jq -e '.keygen_block.keygens | length > 0' >/dev/null 2>&1; then
        echo "$height" >"$RUN_ROOT/meta/keygen-height"
        found=1
        break
      fi
    done
    [[ "$found" == "1" ]] && break
    sleep 2
  done
  [[ "$found" == "1" ]] || die "no automatic keygen block observed"

  for _ in {1..180}; do
    if curl -fsS $(api_url 1)/thornado/vaults/base | jq -e 'if type == "array" then length > 0 else (.vaults | length > 0) end' >/dev/null 2>&1; then
      curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/base-vaults.json"
      log "RESULTS Flow 1: PASS"
      return 0
    fi
    sleep 2
  done
  die "FROST keygen did not create a base vault"
}

base_vault_count() {
  curl -fsS $(api_url 1)/thornado/vaults/base | jq 'if type == "array" then length else (.vaults | length) end'
}

wait_flow1_blocks() {
  local min_height="$1" latest
  for _ in {1..120}; do
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    if (( latest >= min_height )); then
      return 0
    fi
    sleep 1
  done
  die "timed out waiting for height ${min_height}"
}

validate_flow1_no_vault() {
  local label="$1" count
  log "Flow 1 ${label}: validating no base vault is created"
  wait_flow1_blocks 10
  count="$(base_vault_count)"
  [[ "$count" == "0" ]] || die "expected no base vault for ${label}, got ${count}"
  curl -fsS $(api_url 1)/thornado/nodes >"$RUN_ROOT/meta/${label}-nodes.json"
  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/${label}-base-vaults.json"
  log "RESULTS Flow 1 ${label}: PASS"
}

validate_flow1_offline_node() {
  local offline_i="${1:-4}" vault count localstate expected
  log "Flow 1 offline bifrost-${offline_i}: validating late signer recovery"
  wait_flow1_blocks 10
  count="$(base_vault_count)"
  [[ "$count" == "0" ]] || die "offline bifrost-${offline_i}: base vault was created before all signer shares existed"
  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/offline-bifrost-base-vault-before.json"

  start_bifrost_node_for_flow1 "$offline_i" 1
  for _ in {1..180}; do
    count="$(base_vault_count)"
    if [[ "$count" == "1" ]]; then
      break
    fi
    sleep 2
  done
  [[ "$count" == "1" ]] || die "offline bifrost-${offline_i}: base vault was not created after late signer joined"
  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/offline-bifrost-base-vault-after.json"
  vault="$(jq -r '.[0].pub_key' "$RUN_ROOT/meta/offline-bifrost-base-vault-after.json")"
  jq -e 'length == 1 and .[0].status == "ActiveVault" and (.[0].membership | length) == 4' "$RUN_ROOT/meta/offline-bifrost-base-vault-after.json" >/dev/null \
    || die "offline bifrost-${offline_i}: active vault did not retain full membership"

  for i in 1 2 3 "$offline_i"; do
    localstate="$RUN_ROOT/bifrost${i}/localstate-${vault}.json"
    [[ -f "$localstate" ]] || die "offline bifrost-${offline_i}: bifrost-${i} did not persist local FROST state"
    expected="$(sed -n 's/^secp=//p' "$RUN_ROOT/meta/node${i}.env")"
    jq -e --arg expected "$expected" '.signing_engine == "frost" and .local_party_key == $expected and (.participant_keys | length) == 4' "$localstate" >/dev/null \
      || die "offline bifrost-${offline_i}: bifrost-${i} local FROST state did not match signer"
  done
  log "RESULTS Flow 1 offline-bifrost-${offline_i}: PASS"
}

validate_flow1_forged_vault_state() {
  log "Flow 1 forged vault state: validating Bifrost refuses malformed vault pubkey"
  local i=1 home="$RUN_ROOT/node1" bhome="$RUN_ROOT/bifrost1"
  mkdir -p "$bhome"
  SIGNER_NAME="validator1" \
  SIGNER_PASSWD="$PASS" \
  BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
  BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:$(api_port 1)" \
  BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:$(rpc_port 1)" \
  BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:$(ebifrost_port 1)" \
  BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
  BIFROST_THORNADO_SIGNER_NAME="validator1" \
  CHAIN_ID="$CHAIN_ID" \
  CHAIN_API="127.0.0.1:$(api_port 1)" \
  CHAIN_RPC="127.0.0.1:$(rpc_port 1)" \
  BIFROST_METRICS_LISTEN_PORT="$(metrics_port 1)" \
  BIFROST_FROST_P2P_PORT="$(frost_p2p_port 1)" \
  BIFROST_FROST_INFO_ADDRESS="127.0.0.1:$(frost_info_port 1)" \
  BIFROST_TSS_INFO_ADDRESS="127.0.0.1:$(frost_info_port 1)" \
  BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
  EXTERNAL_IP="127.0.0.1" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
  BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost1" \
  BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
  "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
  for _ in {1..60}; do
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
      break
    fi
    if curl -fsS "http://127.0.0.1:$(frost_info_port 1)/ping" >/dev/null 2>&1; then
      die "forged vault state: Bifrost became healthy with malformed vault pubkey"
    fi
    sleep 1
  done
  rg 'not bech32|fail to register|failed to load chain|malformed|invalid' "$RUN_ROOT/logs/bifrost-${i}.log" >"$RUN_ROOT/meta/forged-vault-bifrost-rejection.txt" \
    || die "forged vault state: Bifrost rejection evidence not found"
  log "RESULTS Flow 1 forged-vault-state: PASS"
}

validate_flow2() {
  log "Flow 2: validating bonded standby node via POW deposit and protocol commitment"
  source "$RUN_ROOT/meta/node5.env"
  local owner_addr="$address" operator_pubkey="$secp" node_pubkey="$cons"
  curl -fsS "$(api_url 1)/thornado/node/metrics" >"$RUN_ROOT/meta/flow2-node-metrics-before.json"
  jq -e '(.next_slot | tonumber) == 1 and (.next_slot_bond_required_sats | tonumber) == 100000000 and (.bond_start_amount_sats | tonumber) == 0 and (.bond_slot_increment_sats | tonumber) == 100000000' \
    "$RUN_ROOT/meta/flow2-node-metrics-before.json" >/dev/null || die "flow2 next slot bond requirement is not 1 BTC"
  request_deposit "$RUN_ROOT/node5" "validator5" "bond-flow-2" --operator-pubkey "$operator_pubkey" --node-pubkey "$node_pubkey" >"$RUN_ROOT/meta/flow2-request-deposit.json"
  local session deposit_address txid deposit_id amount_sats receipt commitments out sweep_txout root_addr root_received out_hash
  session="$(deposit_session "$owner_addr")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow2-session-before-deposit.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  jq -e --arg owner "$owner_addr" --arg op "$operator_pubkey" --arg node "$node_pubkey" \
    '.owner == $owner and .operator_pub_key == $op and .node_pub_key == $node and (.deposit_path_index | tonumber) > 0 and (.deposit_address | length) > 0 and (.vault_pub_key | length) > 0' \
    "$RUN_ROOT/meta/flow2-session-before-deposit.json" >/dev/null || die "flow2 deposit session is invalid"
  txid="$(mine_to_registered_deposit "$deposit_address" "1.00000000")"
  btc_cli -rpcwallet=bifrost1 listunspent 1 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow2-child-utxo-before-sweep.json"
  jq -e 'map(select((.amount * 100000000 | floor) == 100000000)) | length == 1' "$RUN_ROOT/meta/flow2-child-utxo-before-sweep.json" >/dev/null \
    || die "flow2 child deposit UTXO was not visible before sweep"
  deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  wait_deposit_matched "$deposit_id" >/dev/null
  sweep_txout="$(wait_sweep_signed "$deposit_id" 1200)"
  printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow2-sweep-txout.json"
  root_addr="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .to_address' <<<"$sweep_txout" | head -n1)"
  root_received="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .coin.amount' <<<"$sweep_txout" | head -n1)"
  out_hash="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .out_hash' <<<"$sweep_txout" | head -n1)"
  wait_confirmed_btc_output "$out_hash" "$root_addr" "$root_received" "$RUN_ROOT/meta/flow2-btc-sweep-tx-confirmed.json" 180
  btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow2-child-utxo-after-sweep.json"
  jq -e 'length == 0' "$RUN_ROOT/meta/flow2-child-utxo-after-sweep.json" >/dev/null || die "flow2 child deposit UTXO remained spendable after sweep"
  amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats')"
  [[ "$amount_sats" == "100000000" ]] || die "flow2 observed bond amount was not 1 BTC"
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(jq -r '.deposit_path_index' <<<"$session")" "$amount_sats" "operator5-seed")"
  commitments="$("$SHIELDER_HELPER" protocol-commitments "$amount_sats")"
  printf '%s\n' "$commitments" >"$RUN_ROOT/meta/flow2-protocol-commitments.json"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder split "$deposit_id" "$commitments")"
  assert_tx_success "$out" "flow2 split"
  local committed
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow2-deposit.json"
  jq -e '.bond_confirmed == true and .settlement == "operator_bond" and (.amount_sats | tonumber) == 100000000 and (.commitment_count | tonumber) == 1' <<<"$committed" >/dev/null || die "flow2 bond was not confirmed"
  record_shielder_leaf "$amount_sats" "$("$SHIELDER_HELPER" protocol-bond-commitment \
    "$deposit_id" "$operator_pubkey" "$node_pubkey" "$(jq -r '.node_slot' <<<"$committed")" "$amount_sats" "$(jq -r '.vault_pub_key' <<<"$committed")")"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-ip-address "127.0.0.1")"
  assert_tx_success "$out" "flow2 set-ip-address"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-node-keys "$operator_pubkey" "$ed" "$node_pubkey")"
  assert_tx_success "$out" "flow2 set-node-keys"
  wait_blocks 2
  node_query "$owner_addr" >"$RUN_ROOT/meta/flow2-node.json"
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow2-bond.json"
  jq -e '((.node.total_bond // .total_bond // .node.bond // .bond) == "'"${amount_sats}"'") and (((.node.status // .status) | ascii_downcase) == "standby" or ((.node.status // .status) | ascii_downcase) == "active")' "$RUN_ROOT/meta/flow2-node.json" >/dev/null \
    || die "flow2 node account not bonded standby/active"
  jq -e --arg op "$operator_pubkey" --arg node "$node_pubkey" \
    '.node_pub_key == $node and .operator_pub_key == $op and (.bond_sats | tonumber) == 100000000 and (.pending_sats | tonumber) == 0 and .fee_share_active == true' \
    "$RUN_ROOT/meta/flow2-bond.json" >/dev/null || die "flow2 bond query did not match committed state"
  curl -fsS "$(api_url 1)/thornado/node/metrics" >"$RUN_ROOT/meta/flow2-node-metrics-after.json"
  log "RESULTS Flow 2: PASS"
}

validate_flow3() {
  log "Flow 3: validating user deposit, split, redeem, fee, txout, and BTC outbound"
  source "$RUN_ROOT/meta/user.env"
  local user_account_addr="$address" deposit_pubkey user_addr
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "user-flow-3-deposit-pubkey")"
  user_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  if [[ "${FLOW3_MAIN_ONLY:-0}" != "1" ]]; then
    assert_tx_or_cli_rejected "flow3 request amount arg" "accepts 1 arg" thornado_tx "$RUN_ROOT/node1" "user" request-deposit "user-flow-3-amount" "20000000"
  fi
  request_deposit "$RUN_ROOT/node1" "user" "user-flow-3" "$deposit_pubkey" >"$RUN_ROOT/meta/flow3-request-deposit.json"
  local session deposit_address txid deposit_id amount_sats path_index receipt commitment_objects commitments shield_signature out committed matched sweep_txout
  session="$(deposit_session "$user_addr")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow3-session-before-deposit.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  path_index="$(jq -r '.deposit_path_index' <<<"$session")"
  jq -e '.owner == "'"${user_addr}"'" and (.deposit_address | length) > 0 and (.vault_pub_key | length) > 0 and (.deposit_path_index | tonumber) > 0 and ((.amount_sats // "") == "" or (.amount_sats // "0") == "0")' \
    "$RUN_ROOT/meta/flow3-session-before-deposit.json" >/dev/null || die "flow3 deposit session unexpectedly contains amount or missing identity"
  txid="$(mine_to_registered_deposit "$deposit_address" "0.20000000")"
  btc_cli -rpcwallet=bifrost1 listunspent 1 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow3-child-utxo-before-sweep.json"
  jq -e 'map(select((.amount * 100000000 | floor) == 20000000)) | length == 1' "$RUN_ROOT/meta/flow3-child-utxo-before-sweep.json" >/dev/null \
    || die "flow3 child deposit UTXO was not visible before sweep"
  deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  matched="$(wait_deposit_matched "$deposit_id")"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow3-deposit-matched.json"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 420)"
  printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow3-sweep-txout.json"
  btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow3-child-utxo-after-sweep.json"
  jq -e 'length == 0' "$RUN_ROOT/meta/flow3-child-utxo-after-sweep.json" >/dev/null || die "flow3 child deposit UTXO remained spendable after sweep"
  amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats')"
  [[ "$amount_sats" == "20000000" ]] || die "flow3 observed deposit amount was not the actual BTC amount"
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$path_index" "$amount_sats" "user-flow-3-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow3-receipt.json"
  commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  printf '%s\n' "$commitment_objects" >"$RUN_ROOT/meta/flow3-commitment-objects.json"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  printf '%s\n' "$commitments" >"$RUN_ROOT/meta/flow3-commitments.json"
  shield_signature="$("$SHIELDER_HELPER" shield-authorization "user-flow-3-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
  printf '%s\n' "$shield_signature" >"$RUN_ROOT/meta/flow3-shield-signature.txt"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder shield "$commitments" "$deposit_pubkey" "$shield_signature" "$deposit_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-split.json"
  assert_tx_success "$out" "flow3 split"
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow3-deposit.json"
  jq -e '.status == "committed" and .settlement == "user"' <<<"$committed" >/dev/null || die "flow3 user split not committed"
  record_shielder_notes "$receipt"
  curl -fsS "$(api_url 1)/thornado/shielder/sync" >"$RUN_ROOT/meta/flow3-shielder-sync-after-split.json"
  jq -e '(.notes | length) >= 2 and ([.notes[] | select((.commitment // "") != "" and ((.denomination_sats // "0") | tonumber) > 0)] | length) >= 2' \
    "$RUN_ROOT/meta/flow3-shielder-sync-after-split.json" >/dev/null || die "flow3 shielder sync did not expose public note records"

  local root denom note leaves recipient fee withdrawal prefix withdrawal_id withdraw_query nullifier quote outbound_txout out_hash expected_payout recipient_received
  note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow3-receipt.json")"
  leaves="$(shielder_leaves "$(jq -r '.denomination_sats' <<<"$note")")"
  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow3-proof-leaves.json"
  recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
  printf '%s\n' "$recipient" >"$RUN_ROOT/meta/flow3-recipient-address.txt"
  curl -fsS "$(api_url 1)/thornado/fee/entitlements" >"$RUN_ROOT/meta/flow3-fee-entitlements-before.json"
  curl -fsS "$(api_url 1)/thornado/shielder/redeem/quote/$(jq -r '.denomination_sats' <<<"$note")" >"$RUN_ROOT/meta/flow3-redeem-quote.json"
  fee="$(jq -r '.fee_sats' "$RUN_ROOT/meta/flow3-redeem-quote.json")"
  (( fee > 0 )) || die "flow3 redeem quote returned zero fee"
  withdrawal="$("$SHIELDER_HELPER" withdrawal "$note" "user-flow-3-seed" "$leaves" "$recipient" "$fee")"
  printf '%s\n' "$withdrawal" >"$RUN_ROOT/meta/flow3-withdrawal.json"
  prefix="$RUN_ROOT/meta/flow3-withdrawal"
  "$SHIELDER_HELPER" shield-withdrawal "$withdrawal" "$prefix"
  root="$(jq -r '.merkle_root' "${prefix}.public.json")"
  denom="$(jq -r '.denomination_sats' "${prefix}.public.json")"
  jq -e --arg root "$root" --argjson denom "$denom" '.merkle_root == $root and (.denomination_sats | tonumber) == $denom' \
    "${prefix}.public.json" >/dev/null || die "flow3 withdrawal proof public inputs were not generated"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${prefix}.proof.json" "${prefix}.public.json")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-redeem.json"
  assert_tx_success "$out" "flow3 redeem"
  withdrawal_id="$(jq -r '.logs[0].events[]? | select(.type=="message") | .attributes[]? | select(.key=="withdrawal_id") | .value' <<<"$out" | tail -n1)"
  if [[ -z "$withdrawal_id" || "$withdrawal_id" == "null" ]]; then
    withdrawal_id="$(printf '%s' "$(jq -r '.[1].nullifier_hash + "|" + .[1].recipient' <<<"$withdrawal")" | shasum -a 256 | awk '{print toupper($1)}')"
  fi
  printf '%s\n' "$withdrawal_id" >"$RUN_ROOT/meta/flow3-withdrawal-id.txt"
  withdraw_query="$(curl -fsS "$(api_url 1)/thornado/shielder/redeem/${withdrawal_id}")"
  printf '%s\n' "$withdraw_query" >"$RUN_ROOT/meta/flow3-withdrawal-query.json"
  jq -e '.status == "keysign_queued" and ((.fee_sats | tonumber) == '"$fee"')' <<<"$withdraw_query" >/dev/null || die "flow3 withdrawal not queued with expected fee"
  nullifier="$(jq -r '.[1].nullifier_hash' <<<"$withdrawal")"
  curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/flow3-nullifier-query.json"
  jq -e '.spent == true and .withdrawal_id == "'"${withdrawal_id}"'"' "$RUN_ROOT/meta/flow3-nullifier-query.json" >/dev/null \
    || die "flow3 nullifier was not marked spent"
  outbound_txout="$(wait_txout_signed_by_in_hash "$withdrawal_id" "out" 1200)"
  printf '%s\n' "$outbound_txout" >"$RUN_ROOT/meta/flow3-withdrawal-txout.json"
  out_hash="$(jq -r --arg in_hash "$withdrawal_id" '.txout.tx_array[] | select(.in_hash == $in_hash) | .out_hash' <<<"$outbound_txout" | head -n1)"
  btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true >"$RUN_ROOT/meta/flow3-btc-outbound-tx.json"
  expected_payout="$(( $(jq -r '.denomination_sats' <<<"$note") - fee ))"
  jq -e --arg in_hash "$withdrawal_id" --arg recipient "$recipient" --argjson payout "$expected_payout" \
    '.txout.tx_array[] | select(.tx_type == "out" and .in_hash == $in_hash and .to_address == $recipient and (.coin.amount | tonumber) == $payout)' \
    <<<"$outbound_txout" >/dev/null || die "flow3 withdrawal txout did not match expected payout"
  jq -e --arg recipient "$recipient" --argjson payout "$expected_payout" \
    '([.vout[] | select(.scriptPubKey.address == $recipient)] | length) == 1 and ((([.vout[] | select(.scriptPubKey.address == $recipient)][0].value * 100000000) | floor) == $payout)' \
    "$RUN_ROOT/meta/flow3-btc-outbound-tx.json" >/dev/null || die "flow3 BTC outbound transaction did not have exactly one expected recipient output"
  recipient_received="$(wait_btc_balance_at_least "$recipient" "0.009" 300)"
  printf '%s\n' "$recipient_received" >"$RUN_ROOT/meta/flow3-recipient-received-btc.txt"
  btc_cli -rpcwallet=miner listunspent 0 9999999 "[\"${recipient}\"]" >"$RUN_ROOT/meta/flow3-recipient-utxos.json"
  jq -e --arg txid "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" --argjson payout "$expected_payout" \
    'length == 1 and .[0].txid == $txid and (((.[0].amount * 100000000) | floor) == $payout)' \
    "$RUN_ROOT/meta/flow3-recipient-utxos.json" >/dev/null || die "flow3 recipient had duplicate or unexpected BTC payout UTXOs"
  curl -fsS "$(api_url 1)/thornado/fee/entitlements" >"$RUN_ROOT/meta/flow3-fee-entitlements-after.json"
  jq -e --argjson fee "$fee" '([.entitlements[]? | (.claimable_sats | tonumber)] | add // 0) >= $fee' "$RUN_ROOT/meta/flow3-fee-entitlements-after.json" >/dev/null \
    || die "flow3 fee entitlement did not increase enough to explain withdrawal fee"

  if [[ "${FLOW3_MAIN_ONLY:-0}" == "1" ]]; then
    log "RESULTS Flow 3: PASS"
    return 0
  fi

  assert_tx_or_cli_rejected "flow3 fully split root" "deposit already fully split" thornado_tx "$RUN_ROOT/node1" "user" shielder split "$deposit_id" "$commitments"
  assert_tx_or_cli_rejected "flow3 duplicate redeem" "shielder nullifier already spent" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${prefix}.proof.json" "${prefix}.public.json"

  local second_note second_withdrawal bad_prefix bad_recipient low_fee fake_receipt fake_note fake_leaves fake_withdrawal neg_session neg_addr neg_txid neg_id neg_match malformed_commitments short_receipt short_commitments alt_receipt alt_commitments alt_committed
  second_note="$(jq -c '.notes[1]' "$RUN_ROOT/meta/flow3-receipt.json")"
  bad_prefix="$RUN_ROOT/meta/flow3-bad"
  second_withdrawal="$("$SHIELDER_HELPER" withdrawal "$second_note" "user-flow-3-seed" "$leaves" "$recipient" "$fee")"
  "$SHIELDER_HELPER" split-withdrawal "$second_withdrawal" "$bad_prefix"
  printf '{}' >"${bad_prefix}-invalid.proof.json"
  assert_tx_or_cli_rejected "flow3 invalid proof" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}-invalid.proof.json" "${bad_prefix}.public.json"
  bad_recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
  jq --arg recipient "$bad_recipient" '.recipient = $recipient' "${bad_prefix}.public.json" >"${bad_prefix}-wrong-recipient.public.json"
  assert_tx_or_cli_rejected "flow3 wrong recipient binding" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}.proof.json" "${bad_prefix}-wrong-recipient.public.json"
  jq '.denomination_sats = (.denomination_sats + 10000000)' "${bad_prefix}.public.json" >"${bad_prefix}-larger-amount.public.json"
  assert_tx_or_cli_rejected "flow3 amount larger than note" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}.proof.json" "${bad_prefix}-larger-amount.public.json"
  low_fee="$((fee - 1))"
  (( low_fee >= 0 )) || low_fee=0
  second_withdrawal="$("$SHIELDER_HELPER" withdrawal "$second_note" "user-flow-3-seed" "$leaves" "$recipient" "$low_fee")"
  "$SHIELDER_HELPER" split-withdrawal "$second_withdrawal" "${bad_prefix}-low-fee"
  assert_tx_or_cli_rejected "flow3 low fee redeem" "invalid withdrawal fee authorization" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}-low-fee.proof.json" "${bad_prefix}-low-fee.public.json"
  nullifier="$(jq -r '.nullifier_hash' "${bad_prefix}-low-fee.public.json")"
  curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/flow3-low-fee-nullifier-query.json"
  jq -e '.spent == false' "$RUN_ROOT/meta/flow3-low-fee-nullifier-query.json" >/dev/null || die "flow3 rejected low-fee redeem consumed nullifier"

  fake_receipt="$("$SHIELDER_HELPER" receipt-simple "$(jq -r '.denomination_sats' <<<"$note")" "flow3-unknown-root-seed")"
  fake_note="$(jq -c '.notes[0]' <<<"$fake_receipt")"
  fake_leaves="$(jq -c '[.notes[0].commitment]' <<<"$fake_receipt")"
  fake_withdrawal="$("$SHIELDER_HELPER" withdrawal "$fake_note" "flow3-unknown-root-seed" "$fake_leaves" "$recipient" "$fee")"
  "$SHIELDER_HELPER" split-withdrawal "$fake_withdrawal" "${bad_prefix}-unknown-root"
  assert_tx_or_cli_rejected "flow3 unknown root redeem" "unknown shielder merkle root" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}-unknown-root.proof.json" "${bad_prefix}-unknown-root.public.json"

  request_deposit "$RUN_ROOT/node1" "user" "user-flow-3-neg-split" >"$RUN_ROOT/meta/flow3-neg-request-deposit.json"
  neg_session="$(deposit_session "$user_addr")"
  printf '%s\n' "$neg_session" >"$RUN_ROOT/meta/flow3-neg-session.json"
  neg_addr="$(jq -r '.deposit_address' <<<"$neg_session")"
  neg_txid="$(mine_to_registered_deposit "$neg_addr" "0.20000000")"
  neg_id="$(printf '%s' "$neg_txid" | tr '[:lower:]' '[:upper:]')"
  neg_match="$(wait_deposit_matched "$neg_id")"
  printf '%s\n' "$neg_match" >"$RUN_ROOT/meta/flow3-neg-deposit-matched.json"
  malformed_commitments="$(jq -nc '["{\"denomination_sats\":10000000"]')"
  assert_tx_or_cli_rejected "flow3 malformed commitment json" "invalid shielder commitment" thornado_tx "$RUN_ROOT/node1" "user" shielder split "$neg_id" "$malformed_commitments"
  assert_tx_or_cli_rejected "flow3 wrong owner split" "deposit owner mismatch" thornado_tx "$RUN_ROOT/node5" "validator5" shielder split "$neg_id" "$commitments"
  short_receipt="$("$SHIELDER_HELPER" receipt-simple "10000000" "flow3-short-split-seed")"
  short_commitments="$("$SHIELDER_HELPER" commitment-objects "$short_receipt")"
  short_commitments="$(jq -c 'map(tostring)' <<<"$short_commitments")"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder split "$neg_id" "$short_commitments")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-partial-split-a.json"
  assert_tx_success "$out" "flow3 partial split A"
  curl -fsS "$(api_url 1)/thornado/deposit/${neg_id}" >"$RUN_ROOT/meta/flow3-neg-deposit-after-partial-a.json"
  jq -e '.status == "committed" and ((.commitment_count // "0" | tonumber) == 1)' "$RUN_ROOT/meta/flow3-neg-deposit-after-partial-a.json" >/dev/null \
    || die "flow3 partial split A did not store exactly one commitment"
  alt_receipt="$("$SHIELDER_HELPER" receipt-simple "10000000" "flow3-alt-owner-seed")"
  printf '%s\n' "$alt_receipt" >"$RUN_ROOT/meta/flow3-alt-owner-receipt.json"
  alt_commitments="$("$SHIELDER_HELPER" commitment-objects "$alt_receipt")"
  alt_commitments="$(jq -c 'map(tostring)' <<<"$alt_commitments")"
  printf '%s\n' "$alt_commitments" >"$RUN_ROOT/meta/flow3-alt-owner-commitments.json"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder split "$neg_id" "$alt_commitments")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-alt-owner-split.json"
  assert_tx_success "$out" "flow3 partial split B"
  alt_committed="$(wait_deposit_committed "$neg_id")"
  printf '%s\n' "$alt_committed" >"$RUN_ROOT/meta/flow3-alt-owner-deposit.json"
  jq -e '.settlement == "user" and (.commitment_count | tonumber) == 2' <<<"$alt_committed" >/dev/null || die "flow3 partial split B did not append the second commitment"

  cat >"$RUN_ROOT/meta/flow3-negative-results.md" <<EOF
# Flow 3 Negative Results

- Request-deposit rejects amount-like extra arguments.
- Fully split root and duplicate redeem reject.
- Malformed commitment JSON and wrong owner split reject without mutating the deposit.
- Two partial splits against one deposit root are accepted and append commitments.
- Invalid proof, wrong recipient binding, larger public amount, low fee, and unknown root redeem attempts reject.
- Low-fee rejection does not consume the note nullifier.
- Owner-signed alternate commitments with the correct denominations are accepted for a user deposit.
EOF
  log "RESULTS Flow 3: PASS"
}

validate_flow4() {
  log "Flow 4: validating operator fee claim after split"
  source "$RUN_ROOT/meta/node5.env"
  local node_pubkey="$cons" owner_addr="$address" entitlement claim accrued fee_share receipt commitments note_pubkeys payload priv sig out deposit_id committed note_count
  local expected_fee pool_before pool_after pool_claimed_before pool_claimed_after bond_before bond_after txhash txres txheight
  entitlement="$(curl -fsS "$(api_url 1)/thornado/fee/entitlement/${node_pubkey}")"
  printf '%s\n' "$entitlement" >"$RUN_ROOT/meta/flow4-entitlement-before.json"
  curl -fsS "$(api_url 1)/thornado/fees" >"$RUN_ROOT/meta/flow4-fee-pool-before.json"
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow4-bond-before.json"
  claim="$(jq -r '.claimable_sats' <<<"$entitlement")"
  accrued="$(jq -r '.accrued_sats' <<<"$entitlement")"
  fee_share="$(jq -r '.fee_per_slot_share' <<<"$entitlement")"
  [[ "$claim" != "0" && "$claim" != "null" ]] || die "flow4 no claimable operator fees"
  expected_fee="$(jq -r '.fee_sats' "$RUN_ROOT/meta/flow3-redeem-quote.json")"
  [[ "$claim" == "$expected_fee" ]] || die "flow4 claimable fee ${claim} did not match flow3 fee ${expected_fee}"
  jq -e --arg node "$node_pubkey" --argjson claim "$claim" \
    '.node_pub_key == $node and .fee_share_active == true and (.claimable_sats | tonumber) == $claim and (.accrued_sats | tonumber) > (.fee_debt_sats | tonumber)' \
    "$RUN_ROOT/meta/flow4-entitlement-before.json" >/dev/null || die "flow4 entitlement before claim is invalid"
  receipt="$("$SHIELDER_HELPER" receipt-simple "$claim" "operator5-fee-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow4-receipt.json"
  jq -e --argjson claim "$claim" '([.notes[].denomination_sats] | add) == $claim and (.notes | length) > 0' \
    "$RUN_ROOT/meta/flow4-receipt.json" >/dev/null || die "flow4 receipt did not cover claim"
  commitments="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitments")"
  note_count="$(jq -r '.notes | length' <<<"$receipt")"
  note_pubkeys="$(
    for ((idx=0; idx<note_count; idx++)); do
      "$SHIELDER_HELPER" pubkey "operator5-fee-note-${idx}"
    done | jq -R -s -c 'split("\n")[:-1]'
  )"
  printf '%s\n' "$note_pubkeys" >"$RUN_ROOT/meta/flow4-fee-note-pubkeys.json"
  jq -e --argjson count "$note_count" 'length == $count and (unique | length) == $count' \
    "$RUN_ROOT/meta/flow4-fee-note-pubkeys.json" >/dev/null || die "flow4 fee note pubkeys invalid"
  payload="$("$SHIELDER_HELPER" fee-payload "$node_pubkey" "$owner_addr" "$accrued" "$fee_share" "$(jq -c '[.notes[] | {denomination_sats, commitment}]' <<<"$receipt")" "$note_pubkeys")"
  printf '%s\n' "$payload" >"$RUN_ROOT/meta/flow4-fee-payload.hex"
  priv="$(key_export_hex "$RUN_ROOT/node5" "validator5")"
  sig="$("$SHIELDER_HELPER" sign-hex "$payload" "$priv")"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder split-fees "$node_pubkey" "ff${sig:2}" "$commitments" "$note_pubkeys")"
  assert_tx_rejected "$out" "flow4 wrong operator signature" "invalid shielder fee operator signature"

  local bad_receipt bad_commitments bad_pubkeys bad_payload bad_sig
  bad_receipt="$("$SHIELDER_HELPER" receipt-simple "$((claim + 900000))" "operator5-fee-too-much-seed")"
  jq -e --argjson claim "$claim" '([.notes[].denomination_sats] | add) > $claim' <<<"$bad_receipt" >/dev/null ||
    die "flow4 oversized fee claim fixture was not actually oversized"
  bad_commitments="$("$SHIELDER_HELPER" commitment-objects "$bad_receipt")"
  bad_commitments="$(jq -c 'map(tostring)' <<<"$bad_commitments")"
  bad_pubkeys="$("$SHIELDER_HELPER" pubkey "operator5-fee-too-much-note" | jq -R -s -c 'split("\n")[:-1]')"
  bad_payload="$("$SHIELDER_HELPER" fee-payload "$node_pubkey" "$owner_addr" "$accrued" "$fee_share" "$(jq -c '[.notes[] | {denomination_sats, commitment}]' <<<"$bad_receipt")" "$bad_pubkeys")"
  bad_sig="$("$SHIELDER_HELPER" sign-hex "$bad_payload" "$priv")"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder split-fees "$node_pubkey" "$bad_sig" "$bad_commitments" "$bad_pubkeys")"
  assert_tx_rejected "$out" "flow4 oversized fee claim" "shielder commitment denominations exceed amount"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder split-fees "$node_pubkey" "$sig" "$commitments" "$note_pubkeys")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow4-split-fees.json"
  assert_tx_success "$out" "flow4 split-fees"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "flow4 split-fees txhash missing"
  txres="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}")"
  printf '%s\n' "$txres" >"$RUN_ROOT/meta/flow4-split-fees-delivertx.json"
  txheight="$(jq -r '.result.height' <<<"$txres")"
  [[ -n "$txheight" && "$txheight" != "null" ]] || die "flow4 split-fees deliver height missing"
  deposit_id="$(printf 'thornado:fee-split:v1|%s|%s|%s|%s' "$node_pubkey" "$owner_addr" "$accrued" "$txheight" | shasum -a 256 | awk '{print toupper($1)}')"
  printf '%s\n' "$deposit_id" >"$RUN_ROOT/meta/flow4-fee-deposit-id.txt"
  wait_blocks 2
  entitlement="$(curl -fsS "$(api_url 1)/thornado/fee/entitlement/${node_pubkey}")"
  printf '%s\n' "$entitlement" >"$RUN_ROOT/meta/flow4-entitlement-after.json"
  curl -fsS "$(api_url 1)/thornado/fees" >"$RUN_ROOT/meta/flow4-fee-pool-after.json"
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow4-bond-after.json"
  jq -e --argjson accrued "$accrued" '(.claimable_sats | tonumber) == 0 and (.fee_debt_sats | tonumber) == $accrued and (.pending_fee_deposit_id == "")' \
    "$RUN_ROOT/meta/flow4-entitlement-after.json" >/dev/null || die "flow4 fee entitlement not claimed"
  jq -e --argjson accrued "$accrued" --argjson bond "$(jq -r '.bond_sats' "$RUN_ROOT/meta/flow4-bond-before.json")" \
    '(.fee_debt_sats | tonumber) == $accrued and (.bond_sats | tonumber) == $bond and .fee_share_active == true' \
    "$RUN_ROOT/meta/flow4-bond-after.json" >/dev/null || die "flow4 bond debt/principal invalid after claim"
  pool_claimed_before="$(jq -r '.total_claimed_sats' "$RUN_ROOT/meta/flow4-fee-pool-before.json")"
  pool_claimed_after="$(jq -r '.total_claimed_sats' "$RUN_ROOT/meta/flow4-fee-pool-after.json")"
  [[ "$pool_claimed_after" == "$((pool_claimed_before + claim))" ]] || die "flow4 fee pool claimed total did not increase by claim"
  committed="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" 2>/dev/null || true)"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow4-fee-deposit.json"
  jq -e --arg owner "$owner_addr" --argjson claim "$claim" --argjson count "$note_count" \
    '.owner == $owner and .settlement == "operator_fee" and .status == "committed" and (.amount_sats | tonumber) == $claim and (.commitment_count | tonumber) == $count' \
    "$RUN_ROOT/meta/flow4-fee-deposit.json" >/dev/null || die "flow4 fee deposit record invalid"
  record_shielder_notes "$receipt"
  local root denom commitment commitment_key commitment_value denom_key denom_value pubkey pubkey_key pubkey_value fee_note leaves quote_status quote_body root_key root_value
  curl -fsS "$(api_url 1)/thornado/shielder/roots" >"$RUN_ROOT/meta/flow4-shielder-roots.json"
  while IFS= read -r commitment; do
    denom="$(jq -r --arg c "$commitment" '.notes[] | select(.commitment == $c) | .denomination_sats' "$RUN_ROOT/meta/flow4-receipt.json")"
    commitment_key="$(printf 'shielder_commitment//%s' "$(printf '%s' "$commitment" | tr '[:lower:]' '[:upper:]')" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
    curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${commitment_key}" >"$RUN_ROOT/meta/flow4-commitment-${commitment}.kv.json"
    commitment_value="$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow4-commitment-${commitment}.kv.json" | base64 -d | jq -r '.')"
    [[ "$commitment_value" == "$deposit_id" ]] || die "flow4 commitment KV did not point at fee deposit"
    denom_key="$(printf 'shielder_denom_commitment/%020d/%s' "$denom" "$commitment" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
    curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${denom_key}" >"$RUN_ROOT/meta/flow4-denom-commitment-${commitment}.kv.json"
    denom_value="$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow4-denom-commitment-${commitment}.kv.json" | base64 -d | jq -r '.')"
    [[ "$denom_value" == "$deposit_id" ]] || die "flow4 denomination commitment KV did not point at fee deposit"
  done < <(jq -r '.notes[].commitment' "$RUN_ROOT/meta/flow4-receipt.json")
  jq -r '.[]' "$RUN_ROOT/meta/flow4-fee-note-pubkeys.json" | while IFS= read -r pubkey; do
    pubkey_key="$(printf 'shielder_fee_note_pubkey//%s' "$(printf '%s' "$pubkey" | tr '[:lower:]' '[:upper:]')" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
    curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${pubkey_key}" >"$RUN_ROOT/meta/flow4-fee-note-pubkey-${pubkey}.kv.json"
    pubkey_value="$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow4-fee-note-pubkey-${pubkey}.kv.json" | base64 -d | jq -r '.')"
    [[ "$pubkey_value" == "$deposit_id" ]] || die "flow4 fee note pubkey KV did not point at fee deposit"
  done
  fee_note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow4-receipt.json")"
  denom="$(jq -r '.denomination_sats' <<<"$fee_note")"
  leaves="$(shielder_leaves "$denom")"
  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow4-proof-leaves.json"
  root="$("$SHIELDER_HELPER" merkle-root "$leaves")"
  jq -e --arg root "$root" --argjson denom "$denom" '.roots[] | select(.root == $root and (.denomination_sats | tonumber) == $denom)' \
    "$RUN_ROOT/meta/flow4-shielder-roots.json" >/dev/null || die "flow4 fee note root missing from roots API"
  root_key="$(printf 'shielder_merkle_root/%020d/%s' "$denom" "$root" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
  curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${root_key}" >"$RUN_ROOT/meta/flow4-merkle-root.kv.json"
  root_value="$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow4-merkle-root.kv.json" | base64 -d | jq -r '.')"
  [[ "$root_value" == "true" ]] || die "flow4 fee note root missing from KV store"
  quote_body="$RUN_ROOT/meta/flow4-fee-note-redeem-quote-error.json"
  quote_status="$(curl -sS -o "$quote_body" -w "%{http_code}" "$(api_url 1)/thornado/shielder/redeem/quote/${denom}" || true)"
  printf '%s\n' "$quote_status" >"$RUN_ROOT/meta/flow4-fee-note-redeem-quote-status.txt"
  [[ "$quote_status" != "200" ]] || die "flow4 100k fee note unexpectedly had a standalone redeem quote"
  grep -q "withdrawal fee exceeds amount" "$quote_body" || die "flow4 100k fee note quote rejected for unexpected reason"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder split-fees "$node_pubkey" "$sig" "$commitments" "$note_pubkeys")"
  assert_tx_rejected "$out" "flow4 duplicate fee claim" "no shielder fees claimable"
  log "RESULTS Flow 4: PASS"
}

validate_flow5() {
  log "Flow 5: validating standby node slot auction and BTC bid split"
  source "$RUN_ROOT/meta/node5.env"
  local seller_addr="$address" seller_operator_pubkey="$secp" seller_node_pubkey="$cons"
  source "$RUN_ROOT/meta/node6.env"
  local bidder_addr="$address" bidder_operator_pubkey="$secp" bidder_node_pubkey="$cons"
  local height expiry out auction_id auction_key auction_kv bid_key bid_kv deposit_key deposit_kv session deposit_address txid deposit_id bid_id receipt commitments committed
  local seller_bond new_bond seller_node bidder_node seller_slot note_count sweep_txout root_addr root_received out_hash matched selected_auction selected_bid
  node_query "$seller_addr" >"$RUN_ROOT/meta/flow5-seller-node-before.json"
  curl -fsS "$(api_url 1)/thornado/bond/${seller_node_pubkey}" >"$RUN_ROOT/meta/flow5-seller-bond-before.json"
  jq -e '((.node.status // .status) | ascii_downcase) == "standby"' "$RUN_ROOT/meta/flow5-seller-node-before.json" >/dev/null \
    || die "flow5 seller is not standby before auction"
  jq -e --arg node "$seller_node_pubkey" --arg op "$seller_operator_pubkey" \
    '.node_pub_key == $node and .operator_pub_key == $op and (.bond_sats | tonumber) == 100000000 and .sold == false and .fee_share_active == true' \
    "$RUN_ROOT/meta/flow5-seller-bond-before.json" >/dev/null || die "flow5 seller bond is not eligible"
  seller_slot="$(jq -r '.slot' "$RUN_ROOT/meta/flow5-seller-bond-before.json")"

  height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  expiry=$((height + 300))
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-create "$bidder_node_pubkey" 100000000 "$expiry")"
  assert_tx_rejected "$out" "flow5 unbonded auction create" "node has no bonded slot"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$expiry")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-create.json"
  assert_tx_success "$out" "flow5 auction-create"
  wait_blocks 2
  auction_id="$(curl -fsS $(api_url 1)/thornado/node/auctions | jq -r --arg seller "$seller_node_pubkey" '.auctions | sort_by(.created_height) | reverse[] | select(.seller_node_pub_key == $seller and .status == "open") | .auction_id' | head -n1)"
  [[ -n "$auction_id" ]] || die "flow5 auction not found"
  printf '%s\n' "$auction_id" >"$RUN_ROOT/meta/flow5-auction-id.txt"
  curl -fsS "$(api_url 1)/thornado/node/auction/${auction_id}" >"$RUN_ROOT/meta/flow5-auction-open.json"
  jq -e --arg seller "$seller_addr" --arg op "$seller_operator_pubkey" --arg node "$seller_node_pubkey" --argjson slot "$seller_slot" --argjson expiry "$expiry" \
    '.seller == $seller and .seller_operator_pub_key == $op and .seller_node_pub_key == $node and (.slot | tonumber) == $slot and (.original_bond_sats | tonumber) == 100000000 and (.reserve_sats | tonumber) == 100000000 and (.expiry_height | tonumber) == $expiry and .status == "open"' \
    "$RUN_ROOT/meta/flow5-auction-open.json" >/dev/null || die "flow5 open auction query is invalid"
  auction_key="$(printf 'node_slot_auction//%s' "$(printf '%s' "$auction_id" | tr '[:lower:]' '[:upper:]')")"
  auction_kv="$(kv_json_value "$auction_key" "$RUN_ROOT/meta/flow5-auction-open.kv.json")"
  jq -e --arg auction "$auction_id" '.auction_id == $auction and .status == "open"' <<<"$auction_kv" >/dev/null || die "flow5 auction KV missing open auction"

  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-bid-pow "${auction_id}-fake" "auction-flow-5-fake" "$bidder_operator_pubkey" "$bidder_node_pubkey")"
  assert_tx_rejected "$out" "flow5 fake auction bid" "node slot auction is not open"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-bid-pow "$auction_id" "auction-flow-5" "$bidder_operator_pubkey" "$bidder_node_pubkey")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-bid-pow.json"
  assert_tx_success "$out" "flow5 auction-bid-pow"
  wait_blocks 2
  session="$(deposit_session "$bidder_addr")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow5-bid-session.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  jq -e --arg owner "$bidder_addr" --arg auction "$auction_id" --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" \
    '.owner == $owner and .auction_id == $auction and .operator_pub_key == $op and .node_pub_key == $node and (.deposit_path_index | tonumber) > 0 and (.deposit_address | length) > 0 and (.vault_pub_key | length) > 0' \
    "$RUN_ROOT/meta/flow5-bid-session.json" >/dev/null || die "flow5 bid deposit session is invalid"
  bid_id="$(curl -fsS "$(api_url 1)/thornado/node/auction/${auction_id}/bids" | jq -r '.bids[0].bid_id')"
  [[ -n "$bid_id" && "$bid_id" != "null" ]] || die "flow5 bid not found before deposit"
  printf '%s\n' "$bid_id" >"$RUN_ROOT/meta/flow5-bid-id.txt"
  curl -fsS "$(api_url 1)/thornado/node/bid/${bid_id}" >"$RUN_ROOT/meta/flow5-bid-before-deposit.json"
  jq -e --arg bid "$bid_id" --arg auction "$auction_id" --arg bidder "$bidder_addr" --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" \
    '.bid_id == $bid and .auction_id == $auction and .bidder == $bidder and .operator_pub_key == $op and .node_pub_key == $node and ((.deposit_id // "") == "") and ((.amount_sats // 0 | tonumber) == 0) and .selected == false and .settled == false' \
    "$RUN_ROOT/meta/flow5-bid-before-deposit.json" >/dev/null || die "flow5 bid state before deposit is invalid"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-select-bid "$auction_id" "$bid_id")"
  assert_tx_rejected "$out" "flow5 select before deposit" "node slot bid deposit not matched"

  txid="$(mine_to_registered_deposit "$deposit_address" "1.00000000")"
  btc_cli -rpcwallet=bifrost1 listunspent 1 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow5-child-utxo-before-sweep.json"
  jq -e 'map(select((.amount * 100000000 | floor) == 100000000)) | length == 1' "$RUN_ROOT/meta/flow5-child-utxo-before-sweep.json" >/dev/null \
    || die "flow5 child auction deposit UTXO was not visible before sweep"
  deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  matched="$(wait_deposit_matched "$deposit_id" 420)"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow5-deposit-matched.json"
  jq -e --arg auction "$auction_id" --arg node "$bidder_node_pubkey" \
    '.status == "deposit_matched" and .auction_id == $auction and .node_pub_key == $node and (.amount_sats | tonumber) == 100000000' \
    "$RUN_ROOT/meta/flow5-deposit-matched.json" >/dev/null || die "flow5 matched bid deposit is invalid"
  curl -fsS "$(api_url 1)/thornado/tx/${deposit_id}" >"$RUN_ROOT/meta/flow5-observed-tx.json"
  jq -e --arg id "$deposit_id" '.. | strings | ascii_upcase | select(. == $id)' "$RUN_ROOT/meta/flow5-observed-tx.json" >/dev/null \
    || die "flow5 observed tx query did not contain auction deposit id"
  curl -fsS "$(api_url 1)/thornado/node/bid/${bid_id}" >"$RUN_ROOT/meta/flow5-bid-after-deposit.json"
  jq -e --arg bid "$bid_id" --arg auction "$auction_id" --arg deposit "$deposit_id" \
    '.bid_id == $bid and .auction_id == $auction and .deposit_id == $deposit and (.amount_sats | tonumber) == 100000000 and .selected == false and .settled == false' \
    "$RUN_ROOT/meta/flow5-bid-after-deposit.json" >/dev/null || die "flow5 bid was not updated by deposit match"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 420)"
  printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow5-sweep-txout.json"
  root_addr="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .to_address' <<<"$sweep_txout" | head -n1)"
  root_received="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .coin.amount' <<<"$sweep_txout" | head -n1)"
  out_hash="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .out_hash' <<<"$sweep_txout" | head -n1)"
  wait_confirmed_btc_output "$out_hash" "$root_addr" "$root_received" "$RUN_ROOT/meta/flow5-btc-sweep-tx-confirmed.json" 180
  btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow5-child-utxo-after-sweep.json"
  jq -e 'length == 0' "$RUN_ROOT/meta/flow5-child-utxo-after-sweep.json" >/dev/null || die "flow5 child auction deposit UTXO remained spendable after sweep"

  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-select-bid "$auction_id" "$bid_id")"
  assert_tx_rejected "$out" "flow5 non-seller select bid" "node slot auction seller mismatch"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-select-bid "$auction_id" "$bid_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-select-bid.json"
  assert_tx_success "$out" "flow5 auction-select-bid"
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/node/auction/${auction_id}" >"$RUN_ROOT/meta/flow5-auction-selected.json"
  curl -fsS "$(api_url 1)/thornado/node/bid/${bid_id}" >"$RUN_ROOT/meta/flow5-bid-selected.json"
  jq -e --arg bid "$bid_id" '.status == "selected" and .selected_bid_id == $bid' "$RUN_ROOT/meta/flow5-auction-selected.json" >/dev/null \
    || die "flow5 auction did not enter selected state"
  jq -e '.selected == true and .settled == false' "$RUN_ROOT/meta/flow5-bid-selected.json" >/dev/null || die "flow5 bid was not selected"

  receipt="$("$SHIELDER_HELPER" receipt-simple 100000000 "operator5-sale-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow5-seller-receipt.json"
  jq -e '([.notes[].denomination_sats] | add) == 100000000 and (.notes | length) > 0' \
    "$RUN_ROOT/meta/flow5-seller-receipt.json" >/dev/null || die "flow5 seller receipt does not cover principal"
  commitments="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitments")"
  note_count="$(jq -r '.notes | length' <<<"$receipt")"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-split "$auction_id" "$bid_id" "$commitments")"
  assert_tx_rejected "$out" "flow5 non-seller auction split" "node slot auction seller mismatch"
  local bad_receipt bad_commitments
  bad_receipt="$("$SHIELDER_HELPER" receipt-simple 90000000 "operator5-sale-bad-seed")"
  bad_commitments="$("$SHIELDER_HELPER" commitment-objects "$bad_receipt")"
  bad_commitments="$(jq -c 'map(tostring)' <<<"$bad_commitments")"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-split "$auction_id" "$bid_id" "$bad_commitments")"
  assert_tx_rejected "$out" "flow5 bad seller payout split" "spendable remainder"
  curl -fsS "$(api_url 1)/thornado/node/bid/${bid_id}" >"$RUN_ROOT/meta/flow5-bid-after-rejected-splits.json"
  jq -e '.selected == true and .settled == false' "$RUN_ROOT/meta/flow5-bid-after-rejected-splits.json" >/dev/null \
    || die "flow5 rejected auction split mutated selected bid"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-split "$auction_id" "$bid_id" "$commitments")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-split.json"
  assert_tx_success "$out" "flow5 auction-split"
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow5-deposit.json"
  jq -e --arg owner "$seller_addr" --argjson count "$note_count" \
    '.owner == $owner and .settlement == "operator_sale" and .status == "committed" and .bond_confirmed == true and (.amount_sats | tonumber) == 100000000 and (.commitment_count | tonumber) == $count' \
    <<<"$committed" >/dev/null || die "flow5 auction deposit not settled correctly"
  deposit_key="$(printf 'deposit_record//%s' "$(printf '%s' "$deposit_id" | tr '[:lower:]' '[:upper:]')")"
  deposit_kv="$(kv_json_value "$deposit_key" "$RUN_ROOT/meta/flow5-deposit.kv.json")"
  jq -e --arg owner "$seller_addr" --arg auction "$auction_id" --argjson count "$note_count" \
    '.owner == $owner and .auction_id == $auction and .settlement == "operator_sale" and .status == "committed" and .bond_confirmed == true and (.amount_sats | tonumber) == 100000000 and (.seller_payout_sats | tonumber) == 100000000 and ((.protocol_bond_sats // 0) | tonumber) == 0 and (.commitments | length) == $count' \
    <<<"$deposit_kv" >/dev/null || die "flow5 auction deposit KV not settled correctly"
  curl -fsS "$(api_url 1)/thornado/node/auction/${auction_id}" >"$RUN_ROOT/meta/flow5-auction-settled.json"
  curl -fsS "$(api_url 1)/thornado/node/bid/${bid_id}" >"$RUN_ROOT/meta/flow5-bid-settled.json"
  jq -e --arg bid "$bid_id" '.status == "settled" and .selected_bid_id == $bid' "$RUN_ROOT/meta/flow5-auction-settled.json" >/dev/null \
    || die "flow5 auction did not settle"
  jq -e --arg deposit "$deposit_id" '.deposit_id == $deposit and .selected == true and .settled == true' "$RUN_ROOT/meta/flow5-bid-settled.json" >/dev/null \
    || die "flow5 bid did not settle"
  auction_kv="$(kv_json_value "$auction_key" "$RUN_ROOT/meta/flow5-auction-settled.kv.json")"
  jq -e --arg auction "$auction_id" --arg bid "$bid_id" '.auction_id == $auction and .selected_bid_id == $bid and .status == "settled"' <<<"$auction_kv" >/dev/null \
    || die "flow5 auction KV did not settle"
  bid_key="$(printf 'node_slot_bid//%s' "$(printf '%s' "$bid_id" | tr '[:lower:]' '[:upper:]')")"
  bid_kv="$(kv_json_value "$bid_key" "$RUN_ROOT/meta/flow5-bid-settled.kv.json")"
  jq -e --arg bid "$bid_id" --arg deposit "$deposit_id" '.bid_id == $bid and .deposit_id == $deposit and .selected == true and .settled == true' <<<"$bid_kv" >/dev/null \
    || die "flow5 bid KV did not settle"

  seller_bond="$(curl -fsS "$(api_url 1)/thornado/bond/${seller_node_pubkey}")"
  new_bond="$(curl -fsS "$(api_url 1)/thornado/bond/${bidder_node_pubkey}")"
  printf '%s\n' "$seller_bond" >"$RUN_ROOT/meta/flow5-seller-bond-after.json"
  printf '%s\n' "$new_bond" >"$RUN_ROOT/meta/flow5-buyer-bond-after.json"
  jq -e --arg auction "$auction_id" '.sold == true and .sold_auction_id == $auction and (.bond_sats | tonumber) == 0 and .fee_share_active == false' \
    "$RUN_ROOT/meta/flow5-seller-bond-after.json" >/dev/null || die "flow5 seller bond not sold"
  jq -e --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" --argjson slot "$seller_slot" \
    '.node_pub_key == $node and .operator_pub_key == $op and (.slot | tonumber) == $slot and (.bond_sats | tonumber) == 100000000 and .fee_share_active == true and .node_status == "Standby"' \
    "$RUN_ROOT/meta/flow5-buyer-bond-after.json" >/dev/null || die "flow5 new bond not standby with auction principal"
  node_query "$seller_addr" >"$RUN_ROOT/meta/flow5-seller-node-after.json"
  node_query "$bidder_addr" >"$RUN_ROOT/meta/flow5-buyer-node-after.json"
  jq -e '((.node.status // .status) | ascii_downcase) == "standby"' "$RUN_ROOT/meta/flow5-buyer-node-after.json" >/dev/null \
    || die "flow5 buyer node account is not standby"

  record_shielder_notes "$receipt"
  curl -fsS "$(api_url 1)/thornado/shielder/roots" >"$RUN_ROOT/meta/flow5-shielder-roots.json"
  local root denom commitment commitment_key commitment_value denom_key denom_value leaves root_key root_value
  while IFS= read -r commitment; do
    denom="$(jq -r --arg c "$commitment" '.notes[] | select(.commitment == $c) | .denomination_sats' "$RUN_ROOT/meta/flow5-seller-receipt.json")"
    commitment_key="$(printf 'shielder_commitment//%s' "$(printf '%s' "$commitment" | tr '[:lower:]' '[:upper:]')")"
    commitment_value="$(kv_json_value "$commitment_key" "$RUN_ROOT/meta/flow5-commitment-${commitment}.kv.json")"
    [[ "$commitment_value" == "$deposit_id" ]] || die "flow5 seller commitment KV did not point at sale deposit"
    denom_key="$(printf 'shielder_denom_commitment/%020d/%s' "$denom" "$commitment")"
    denom_value="$(kv_json_value "$denom_key" "$RUN_ROOT/meta/flow5-denom-commitment-${commitment}.kv.json")"
    [[ "$denom_value" == "$deposit_id" ]] || die "flow5 seller denom commitment KV did not point at sale deposit"
  done < <(jq -r '.notes[].commitment' "$RUN_ROOT/meta/flow5-seller-receipt.json")
  denom="$(jq -r '.notes[0].denomination_sats' "$RUN_ROOT/meta/flow5-seller-receipt.json")"
  leaves="$(shielder_leaves "$denom")"
  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow5-proof-leaves.json"
  root="$("$SHIELDER_HELPER" merkle-root "$leaves")"
  jq -e --arg root "$root" --argjson denom "$denom" '.roots[] | select(.root == $root and (.denomination_sats | tonumber) == $denom)' \
    "$RUN_ROOT/meta/flow5-shielder-roots.json" >/dev/null || die "flow5 seller note root missing from roots API"
  root_key="$(printf 'shielder_merkle_root/%020d/%s' "$denom" "$root")"
  root_value="$(kv_json_value "$root_key" "$RUN_ROOT/meta/flow5-merkle-root.kv.json")"
  [[ "$root_value" == "true" ]] || die "flow5 seller note root missing from KV store"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-split "$auction_id" "$bid_id" "$commitments")"
  assert_tx_rejected "$out" "flow5 duplicate auction split" "node slot auction bid not selected"
  height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$((height + 300))")"
  assert_tx_rejected "$out" "flow5 sold node second auction" "node has no bonded slot"
  log "RESULTS Flow 5: PASS"
}

validate_flow6() {
  log "Flow 6: validating node6 churn-in and base-vault migration"
  local old_vault old_addr new_vault new_addr node5_secp node6_addr node6_secp node6_ed node6_cons out status active_vaults start latest
  local flow6_start keygen_height keygen_json migrate_txout out_hash raw_tx old_prevouts after_vaults post_session config h current_height
  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/flow6-base-vaults-before.json"
  old_vault="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/flow6-base-vaults-before.json")"
  old_addr="$(jq -r --arg old "$old_vault" '.[] | select(.pub_key == $old) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow6-base-vaults-before.json" | head -n1)"
  if [[ -z "$old_addr" || "$old_addr" == "null" ]]; then
    old_addr="$("$SHIELDER_HELPER" btc-address "$old_vault" 0)"
  fi
  source "$RUN_ROOT/meta/node5.env"
  node5_secp="$secp"
  source "$RUN_ROOT/meta/node6.env"
  node6_addr="$address"
  node6_secp="$secp"
  node6_ed="$ed"
  node6_cons="$cons"

  set_config_from_active_nodes Node_SetDesired 5
  set_config_from_active_nodes Churn_RetryIntervalBlocks 20
  set_config_from_active_nodes Vault_MigrationIntervalBlocks 20
  set_config_from_active_nodes Vault_MigrationRounds 1
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/flow6-config-after-migration-tuning.json"
  config="$(jq -r '(.NODE_SETDESIRED.value // (.configs[]? | select(.key == "Node_SetDesired") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "5" ]] || die "flow6 node desired config was not applied"
  config="$(jq -r '(.VAULT_MIGRATIONINTERVALBLOCKS.value // (.configs[]? | select(.key == "Vault_MigrationIntervalBlocks") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "20" ]] || die "flow6 migration interval config was not applied"
  config="$(jq -r '(.CHURN_RETRYINTERVALBLOCKS.value // (.configs[]? | select(.key == "Churn_RetryIntervalBlocks") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "20" ]] || die "flow6 churn retry config was not applied"

  curl -fsS "$(api_url 1)/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow6-node6-bond-before.json"
  jq -e '(.bond_sats | tonumber) == 100000000 and .node_status == "Standby"' "$RUN_ROOT/meta/flow6-node6-bond-before.json" >/dev/null \
    || die "flow6 node6 does not have standby auction bond"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" set-ip-address "127.0.0.1")"
  assert_tx_success "$out" "flow6 node6 set-ip-address"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" set-node-keys "$node6_secp" "$node6_ed" "$node6_cons")"
  assert_tx_success "$out" "flow6 node6 set-node-keys"
  wait_blocks 2
  node_query "$node6_addr" >"$RUN_ROOT/meta/flow6-node6-after-keys.json"
  jq -e --arg secp "$node6_secp" --arg ed "$node6_ed" --arg cons "$node6_cons" \
    '((.node.status // .status) | ascii_downcase) == "standby" and ((.node.pub_key_set.secp256k1 // .pub_key_set.secp256k1) == $secp) and ((.node.pub_key_set.ed25519 // .pub_key_set.ed25519) == $ed) and ((.node.node_cons_pub_key // .node_cons_pub_key) == $cons) and ((.node.ip_address // .ip_address) == "127.0.0.1")' \
    "$RUN_ROOT/meta/flow6-node6-after-keys.json" >/dev/null || die "flow6 node6 keys/ip did not register"
  log "waiting for Bifrost node-gater allowlists to include node6"
  sleep 70
  grep -ihE "allow|bond|node6|standby" "$RUN_ROOT"/logs/bifrost-{1,2,3,4}.log >"$RUN_ROOT/meta/flow6-node-gater-log-sample.txt" || true
  start_thornado_node6
  start_bifrost_node6
  wait_bifrost6_ready_for_keygen
  curl -fsS "http://127.0.0.1:$(frost_info_port 6)/status/p2p" >"$RUN_ROOT/meta/flow6-bifrost6-p2p.json"
  curl -fsS "http://127.0.0.1:$(frost_info_port 6)/status/scanner" >"$RUN_ROOT/meta/flow6-bifrost6-scanner.json"
  curl -fsS "http://127.0.0.1:$(frost_info_port 6)/status/signing" >"$RUN_ROOT/meta/flow6-bifrost6-signing-before-churn.json"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" set-version)"
  assert_tx_success "$out" "flow6 node6 set-version"
  wait_blocks 2
  node_query "$node6_addr" >"$RUN_ROOT/meta/flow6-node6-pre-churn.json"
  jq -e '(((.node.status // .status) | ascii_downcase) as $status | ($status == "standby" or $status == "selected")) and ((.node.version // .version) | length) > 0' \
    "$RUN_ROOT/meta/flow6-node6-pre-churn.json" >/dev/null || die "flow6 node6 pre-churn status/version invalid"

  flow6_start="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  : >"$RUN_ROOT/meta/flow6-node6-status-history.tsv"
  start="$(date +%s)"
  while true; do
    status="$(node_query "$node6_addr" | jq -r '(.node.status // .status) | ascii_downcase')"
    active_vaults="$(curl -fsS $(api_url 1)/thornado/vaults/base)"
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    printf '%s\t%s\n' "$latest" "$status" >>"$RUN_ROOT/meta/flow6-node6-status-history.tsv"
    if [[ "$status" == "active" ]] && jq -e --arg old "$old_vault" --arg member "$node6_secp" '
      [.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))] | length > 0
    ' <<<"$active_vaults" >/dev/null; then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/flow6-base-vaults.json"
      new_vault="$(jq -r --arg old "$old_vault" --arg member "$node6_secp" '[.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))][0].pub_key' "$RUN_ROOT/meta/flow6-base-vaults.json")"
      new_addr="$(jq -r --arg new "$new_vault" '.[] | select(.pub_key == $new) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow6-base-vaults.json" | head -n1)"
      if [[ -z "$new_addr" || "$new_addr" == "null" ]]; then
        new_addr="$("$SHIELDER_HELPER" btc-address "$new_vault" 0)"
      fi
      break
    fi
    if (( "$(date +%s)" - start >= 720 )); then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/flow6-base-vaults-timeout.json"
      die "flow6 node6 did not churn into a new active base vault"
    fi
    log "flow6 waiting: height=${latest} node6_status=${status}"
    sleep 10
  done
  jq -e --arg old "$old_vault" --arg new "$new_vault" --arg member "$node6_secp" --arg seller "$node5_secp" '
    ([.[] | select(.status == "ActiveVault")] | length) == 1 and
    (.[] | select(.pub_key == $old) | .status) == "RetiringVault" and
    (.[] | select(.pub_key == $new) | .status) == "ActiveVault" and
    (.[] | select(.pub_key == $new) | (.membership | index($member))) and
    ((.[] | select(.pub_key == $new) | .membership | index($seller)) == null)
  ' "$RUN_ROOT/meta/flow6-base-vaults.json" >/dev/null || die "flow6 vault rotation state is invalid"
  node_query "$node6_addr" >"$RUN_ROOT/meta/flow6-node6-active.json"
  jq -e --arg new "$new_vault" '((.node.status // .status) | ascii_downcase) == "active" and ((.node.active_block_height // .active_block_height | tonumber) > 0) and (((.node.signer_membership // .signer_membership) // []) | index($new))' \
    "$RUN_ROOT/meta/flow6-node6-active.json" >/dev/null || die "flow6 node6 active node query invalid"

  keygen_height=""
  h="$flow6_start"
  while true; do
    keygen_json="$(curl -fsS "$(api_url 1)/thornado/keygen/${h}/${node6_secp}" 2>/dev/null || true)"
    if [[ -n "$keygen_json" ]] && jq -e --arg member "$node6_secp" --arg seller "$node5_secp" '.keygen_block.keygens[]? | select((.members | index($member)) and ((.members | index($seller)) == null))' <<<"$keygen_json" >/dev/null 2>&1; then
      keygen_height="$h"
      printf '%s\n' "$keygen_json" >"$RUN_ROOT/meta/flow6-keygen-node6.json"
      break
    fi
    current_height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    if (( h >= flow6_start + 900 )); then
      break
    fi
    if (( h >= current_height )); then
      sleep 1
      continue
    fi
    h=$((h + 1))
  done
  [[ -n "$keygen_height" ]] || die "flow6 did not find keygen block for node6"
  printf '%s\n' "$keygen_height" >"$RUN_ROOT/meta/flow6-keygen-height.txt"
  grep -h "FROST keygen complete" "$RUN_ROOT"/logs/bifrost-{1,2,3,4,6}.log >"$RUN_ROOT/meta/flow6-frost-keygen-complete.log" || true
  grep -q "$new_vault" "$RUN_ROOT/meta/flow6-frost-keygen-complete.log" || die "flow6 FROST keygen completion log missing new vault"
  [[ -f "$RUN_ROOT/bifrost6/localstate-${new_vault}.json" ]] || die "flow6 bifrost6 local FROST state for new vault missing"
  jq -e --arg vault "$new_vault" --arg local "$node6_secp" --arg seller "$node5_secp" \
    '.pub_key == $vault and .signing_engine == "frost" and .local_party_key == $local and (.local_data | length) > 0 and (.participant_keys | length) == 5 and (.participant_keys | index($local)) and ((.participant_keys | index($seller)) == null)' \
    "$RUN_ROOT/bifrost6/localstate-${new_vault}.json" >/dev/null || die "flow6 bifrost6 FROST local state invalid"

  post_session="$(
    source "$RUN_ROOT/meta/user.env"
    request_deposit "$RUN_ROOT/node1" "user" "user-flow-6-post-churn" >/dev/null
    deposit_session "$address"
  )"
  printf '%s\n' "$post_session" >"$RUN_ROOT/meta/flow6-post-churn-deposit-session.json"
  jq -e --arg new "$new_vault" '.vault_pub_key == $new and (.deposit_address | length) > 0' "$RUN_ROOT/meta/flow6-post-churn-deposit-session.json" >/dev/null \
    || die "flow6 post-churn deposit session did not use new active vault"

  migrate_txout="$(wait_migrate_signed "$flow6_start" "$old_vault" "$new_addr" 1200)"
  printf '%s\n' "$migrate_txout" >"$RUN_ROOT/meta/flow6-migrate-txout.json"
  jq -e --arg old "$old_vault" --arg to "$new_addr" \
    '[.txout.tx_array[]? | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to and (.coin.amount | tonumber) > 0 and (.out_hash // "") != "")] | length == 1' \
    "$RUN_ROOT/meta/flow6-migrate-txout.json" >/dev/null || die "flow6 migrate txout fields invalid"
  out_hash="$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .out_hash' "$RUN_ROOT/meta/flow6-migrate-txout.json" | head -n1)"
  btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true >"$RUN_ROOT/meta/flow6-btc-migrate-tx.json"
  mine_regtest_blocks 2
  raw_tx="$(btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true)"
  printf '%s\n' "$raw_tx" >"$RUN_ROOT/meta/flow6-btc-migrate-tx-confirmed.json"
  jq -e --arg to "$new_addr" --argjson amount "$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .coin.amount' "$RUN_ROOT/meta/flow6-migrate-txout.json" | head -n1)" \
    '([.vout[] | select(.scriptPubKey.address == $to)] | length) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 | floor)] | add // 0) >= $amount) and (.confirmations // 0) >= 1' \
    "$RUN_ROOT/meta/flow6-btc-migrate-tx-confirmed.json" >/dev/null || die "flow6 BTC migration transaction did not pay new vault"
  old_prevouts="$RUN_ROOT/meta/flow6-migrate-prevouts.json"
  jq -r '.vin[] | @base64' "$RUN_ROOT/meta/flow6-btc-migrate-tx-confirmed.json" | while IFS= read -r vin64; do
    local_txid="$(printf '%s' "$vin64" | base64 -d | jq -r '.txid')"
    local_vout="$(printf '%s' "$vin64" | base64 -d | jq -r '.vout')"
    btc_cli getrawtransaction "$local_txid" true | jq --argjson vout "$local_vout" '{txid, vout:$vout, address:.vout[$vout].scriptPubKey.address, value_sats:((.vout[$vout].value * 100000000) | floor)}'
  done | jq -s '.' >"$old_prevouts"
  jq -e --arg old "$old_addr" 'length > 0 and all(.[]; .address == $old)' "$old_prevouts" >/dev/null || die "flow6 BTC migration did not spend old vault UTXOs"

  out_hash="$(printf '%s' "$out_hash" | tr '[:lower:]' '[:upper:]')"
  for _ in {1..90}; do
    if curl -fsS "$(api_url 1)/thornado/tx/${out_hash}" >"$RUN_ROOT/meta/flow6-migrate-observed-tx.json" 2>/dev/null &&
      jq -e --arg id "$out_hash" '.. | strings | ascii_upcase | select(. == $id)' "$RUN_ROOT/meta/flow6-migrate-observed-tx.json" >/dev/null; then
      break
    fi
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  jq -e --arg id "$out_hash" '.. | strings | ascii_upcase | select(. == $id)' "$RUN_ROOT/meta/flow6-migrate-observed-tx.json" >/dev/null \
    || die "flow6 migration outbound was not observable through tx query"
  after_vaults="$(curl -fsS $(api_url 1)/thornado/vaults/base)"
  printf '%s\n' "$after_vaults" >"$RUN_ROOT/meta/flow6-base-vaults-after-migration.json"
  curl -fsS "$(api_url 1)/thornado/vault/${old_vault}" >"$RUN_ROOT/meta/flow6-old-vault-after-migration.json"
  jq -e --arg old "$old_vault" '.pub_key == $old and (.status == "RetiringVault" or .status == "InactiveVault")' "$RUN_ROOT/meta/flow6-old-vault-after-migration.json" >/dev/null \
    || die "flow6 old vault status invalid after migration"
  jq -e --arg old "$old_vault" '[.[] | select(.pub_key == $old and .status == "ActiveVault")] | length == 0' "$RUN_ROOT/meta/flow6-base-vaults-after-migration.json" >/dev/null \
    || die "flow6 old vault still active after migration"
  jq -e --arg new "$new_vault" '.[] | select(.pub_key == $new and .status == "ActiveVault")' "$RUN_ROOT/meta/flow6-base-vaults-after-migration.json" >/dev/null \
    || die "flow6 new active vault missing after migration"
  log "RESULTS Flow 6: PASS"
}

find_signed_migrate_txout() {
  local from_height="$1" old_vault="${2:-}" to_address="${3:-}" now h txout
  now="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  (( from_height < 1 )) && from_height=1
  for ((h=from_height; h<=now; h++)); do
    txout="$(curl -fsS "$(api_url 1)/thornado/keysign/${h}" 2>/dev/null | jq -c '{height:'"$h"', txout:.keysign}' 2>/dev/null || true)"
    if [[ -n "$txout" ]] && jq -e --arg old "$old_vault" --arg to "$to_address" '
      .txout.tx_array[]? |
      select(.tx_type == "migrate" and (.out_hash // "") != "" and (($old == "") or (.vault_pub_key == $old)) and (($to == "") or (.to_address == $to)))
    ' <<<"$txout" >/dev/null 2>&1; then
      printf '%s\n' "$txout"
      return 0
    fi
  done
  return 1
}

wait_migrate_signed() {
  local from_height="$1" old_vault="${2:-}" to_address="${3:-}" timeout="${4:-720}" start found
  start="$(date +%s)"
  while true; do
    mine_regtest_blocks 1
    wait_blocks 1
    if found="$(find_signed_migrate_txout "$from_height" "$old_vault" "$to_address")"; then
      printf '%s\n' "$found"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "migrate txout from ${old_vault:-any} to ${to_address:-any} was not signed"
    fi
    sleep 2
  done
}

find_consolidate_txout() {
  local now from h txout to_address amount_sats received_sats
  now="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  from=$((now - 80))
  (( from < 1 )) && from=1
  for ((h=from; h<=now; h++)); do
    txout="$(curl -fsS "$(api_url 1)/thornado/keysign/${h}" 2>/dev/null | jq -c '{txout:.keysign}' 2>/dev/null || true)"
    if [[ -n "$txout" ]] && jq -e '.txout.tx_array[]? | select(.tx_type == "consolidate" and (.out_hash // "") != "")' <<<"$txout" >/dev/null 2>&1; then
      printf '%s\n' "$txout"
      return 0
    fi
    if [[ -n "$txout" ]] && jq -e '.txout.tx_array[]? | select(.tx_type == "consolidate")' <<<"$txout" >/dev/null 2>&1; then
      to_address="$(jq -r '.txout.tx_array[]? | select(.tx_type == "consolidate") | .to_address' <<<"$txout" | head -n1)"
      amount_sats="$(jq -r '.txout.tx_array[]? | select(.tx_type == "consolidate") | .coin.amount' <<<"$txout" | head -n1)"
      received_sats="$(btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${to_address}\"]" | jq '[.[].amount * 100000000] | add // 0 | floor')"
      if (( received_sats >= amount_sats )); then
        printf '%s\n' "$txout"
        return 0
      fi
    fi
  done
  return 1
}

find_signed_sweep_txout() {
  local in_hash="$1" now from h txout
  txout="$(curl -fsS "$(api_url 1)/thornado/txout/all" 2>/dev/null || true)"
  if [[ -n "$txout" ]] && jq -e --arg in_hash "$in_hash" '.txouts[]? | select(.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
    jq -c --arg in_hash "$in_hash" '{txout:(.txouts[] | select(.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
    return 0
  fi
  now="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  from=$((now - 120))
  (( from < 1 )) && from=1
  for ((h=from; h<=now; h++)); do
    txout="$(curl -fsS "$(api_url 1)/thornado/keysign/${h}" 2>/dev/null | jq -c '{txout:.keysign}' 2>/dev/null || true)"
    if [[ -n "$txout" ]] && jq -e --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and (.out_hash // "") != "")' <<<"$txout" >/dev/null 2>&1; then
      printf '%s\n' "$txout"
      return 0
    fi
  done
  return 1
}

find_signed_txout_by_in_hash() {
  local in_hash="$1" tx_type="${2:-}" now from h txout
  txout="$(curl -fsS "$(api_url 1)/thornado/txout/all" 2>/dev/null || true)"
  if [[ -n "$txout" ]]; then
    if [[ -n "$tx_type" ]]; then
      if jq -e --arg in_hash "$in_hash" --arg tx_type "$tx_type" '.txouts[]? | select(.tx_array[]? | select(.tx_type == $tx_type and .in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
        jq -c --arg in_hash "$in_hash" --arg tx_type "$tx_type" '{txout:(.txouts[] | select(.tx_array[]? | select(.tx_type == $tx_type and .in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
        return 0
      fi
    elif jq -e --arg in_hash "$in_hash" '.txouts[]? | select(.tx_array[]? | select(.in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
      jq -c --arg in_hash "$in_hash" '{txout:(.txouts[] | select(.tx_array[]? | select(.in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
      return 0
    fi
  fi
  now="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  from=$((now - 160))
  (( from < 1 )) && from=1
  for ((h=from; h<=now; h++)); do
    txout="$(curl -fsS "$(api_url 1)/thornado/keysign/${h}" 2>/dev/null | jq -c '{height:'"$h"', txout:.keysign}' 2>/dev/null || true)"
    if [[ -z "$txout" ]]; then
      continue
    fi
    if [[ -n "$tx_type" ]]; then
      if jq -e --arg in_hash "$in_hash" --arg tx_type "$tx_type" '.txout.tx_array[]? | select(.tx_type == $tx_type and .in_hash == $in_hash and (.out_hash // "") != "")' <<<"$txout" >/dev/null 2>&1; then
        printf '%s\n' "$txout"
        return 0
      fi
    elif jq -e --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.in_hash == $in_hash and (.out_hash // "") != "")' <<<"$txout" >/dev/null 2>&1; then
      printf '%s\n' "$txout"
      return 0
    fi
  done
  return 1
}

wait_txout_signed_by_in_hash() {
  local in_hash="$1" tx_type="${2:-}" timeout="${3:-240}" start found
  start="$(date +%s)"
  while true; do
    mine_regtest_blocks 1
    wait_blocks 1
    if found="$(find_signed_txout_by_in_hash "$in_hash" "$tx_type")"; then
      printf '%s\n' "$found"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "txout ${tx_type:-any} for ${in_hash} was not signed"
    fi
    sleep 2
  done
}

sweep_reached_root_vault() {
  local in_hash="$1" now from h txout to_address amount_sats received_sats
  now="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  from=$((now - 120))
  (( from < 1 )) && from=1
  for ((h=from; h<=now; h++)); do
    txout="$(curl -fsS "$(api_url 1)/thornado/keysign/${h}" 2>/dev/null | jq -c '{txout:.keysign}' 2>/dev/null || true)"
    if [[ -z "$txout" ]]; then
      continue
    fi
    to_address="$(jq -r --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash) | .to_address' <<<"$txout" | head -n1)"
    amount_sats="$(jq -r --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash) | .coin.amount' <<<"$txout" | head -n1)"
    if [[ -z "$to_address" || "$to_address" == "null" || -z "$amount_sats" || "$amount_sats" == "null" ]]; then
      continue
    fi
    received_sats="$(btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${to_address}\"]" | jq '[.[].amount * 100000000] | add // 0 | floor')"
    if (( received_sats >= amount_sats )); then
      return 0
    fi
  done
  return 1
}

wait_sweep_signed() {
  local in_hash="$1" timeout="${2:-240}" start found
  start="$(date +%s)"
  while true; do
    mine_regtest_blocks 1
    wait_blocks 1
    if found="$(find_signed_sweep_txout "$in_hash")"; then
      printf '%s\n' "$found"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "sweep txout for ${in_hash} was not signed"
    fi
    sleep 2
  done
}

wait_confirmed_btc_output() {
  local out_hash="$1" to_address="$2" amount_sats="$3" artifact="$4" timeout="${5:-180}"
  local txid start tx hex raw
  txid="$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')"
  start="$(date +%s)"
  while true; do
    if tx="$(btc_cli -rpcwallet=bifrost1 gettransaction "$txid" true 2>/dev/null)" &&
      hex="$(jq -r '.hex // empty' <<<"$tx")" &&
      [[ -n "$hex" ]]; then
      raw="$(btc_cli decoderawtransaction "$hex" | jq --argjson conf "$(jq -r '.confirmations // 0' <<<"$tx")" '. + {confirmations:$conf}')"
      printf '%s\n' "$raw" >"$artifact"
      if jq -e --arg to "$to_address" --argjson amount "$amount_sats" \
        '(.confirmations // 0) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 | floor)] | add // 0) >= $amount)' \
        "$artifact" >/dev/null; then
        return 0
      fi
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "BTC tx ${out_hash} did not confirm output to ${to_address}"
    fi
    mine_regtest_blocks 1
    sleep 1
  done
}

set_config_from_active_nodes() {
  local key="$1" value="$2" i status out voted=0
  for i in 1 2 3 4 5 6; do
    source "$RUN_ROOT/meta/node${i}.env"
    status="$(node_query "$address" | jq -r '(.node.status // .status) | ascii_downcase')"
    if [[ "$status" != "active" ]]; then
      continue
    fi
    out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" config "$key" "$value")"
    assert_tx_success "$out" "flow7 set ${key} node${i}"
    voted=$((voted + 1))
  done
  (( voted >= 3 )) || die "flow7 insufficient active nodes voted for ${key}"
  wait_blocks 2
}

validate_flow7() {
  log "Flow 7: validating many deposits and BTC consolidation"
  local i previous_session previous_address previous_path session deposit_address txid deposit_id found start
  set_config_from_active_nodes UTXO_MaxSpendCount 3
  source "$RUN_ROOT/meta/user.env"
  for i in 1 2 3; do
    previous_session="$(deposit_session "$address" 2>/dev/null || true)"
    previous_address="$(jq -r '.deposit_address // ""' <<<"$previous_session" 2>/dev/null || true)"
    previous_path="$(jq -r '.deposit_path_index // ""' <<<"$previous_session" 2>/dev/null || true)"
    request_deposit "$RUN_ROOT/node1" "user" "user-flow-7-${i}" >/dev/null
    session="$(wait_new_deposit_session "$address" "$previous_address" "$previous_path")"
    deposit_address="$(jq -r '.deposit_address' <<<"$session")"
    txid="$(mine_to_registered_deposit "$deposit_address" "0.01000000")"
    deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
    wait_deposit_matched "$deposit_id" 420 >/dev/null
    wait_sweep_signed "$deposit_id" 420 >/dev/null
  done
  start="$(date +%s)"
  while true; do
    mine_regtest_blocks 1
    wait_blocks 1
    if found="$(find_consolidate_txout)"; then
      printf '%s\n' "$found" >"$RUN_ROOT/meta/flow7-consolidate-txout.json"
      break
    fi
    if (( "$(date +%s)" - start >= 360 )); then
      die "flow7 consolidate txout was not signed"
    fi
    sleep 10
  done
  log "RESULTS Flow 7: PASS"
}

keep_running_if_requested() {
  if [[ "${KEEP_RUNNING:-0}" != "1" ]]; then
    return 0
  fi
  log "KEEP_RUNNING=1; cluster remains live at ${RUN_ROOT}. Press Ctrl-C to stop this script."
  while true; do
    sleep 60
  done
}

main() {
  local flow_limit="${FLOW_LIMIT:-7}"
  build_binaries
  reset_all
  start_bitcoind
  init_genesis
  start_thornado_nodes
  case "$FLOW1_SCENARIO" in
    three_active|missing_secp|duplicate_secp)
      FLOW1_SKIP_BIFROST_NODES="4"
      start_bifrost_nodes
      validate_flow1_no_vault "$FLOW1_SCENARIO"
      keep_running_if_requested
      return 0
      ;;
    offline_node4)
      FLOW1_SKIP_BIFROST_NODES="4"
      start_bifrost_nodes
      validate_flow1_offline_node 4
      keep_running_if_requested
      return 0
      ;;
    forged_vault_state)
      validate_flow1_forged_vault_state
      keep_running_if_requested
      return 0
      ;;
  esac
  start_bifrost_nodes
  validate_flow1
  if (( flow_limit <= 1 )); then
    keep_running_if_requested
    return 0
  fi
  if [[ "${SKIP_FLOW2:-0}" == "1" ]]; then
    log "SKIP_FLOW2=1; skipping bonded standby node flow"
  else
    validate_flow2
  fi
  if (( flow_limit <= 2 )); then
    keep_running_if_requested
    return 0
  fi
  validate_flow3
  if (( flow_limit <= 3 )); then
    keep_running_if_requested
    return 0
  fi
  validate_flow4
  if (( flow_limit <= 4 )); then
    keep_running_if_requested
    return 0
  fi
  validate_flow5
  if (( flow_limit <= 5 )); then
    keep_running_if_requested
    return 0
  fi
  validate_flow6
  if (( flow_limit <= 6 )); then
    keep_running_if_requested
    return 0
  fi
  validate_flow7
  log "All 7 flows passed at ${RUN_ROOT}"
  write_run_summary "PASS" "all requested flows passed"
  keep_running_if_requested
}

main "$@"
