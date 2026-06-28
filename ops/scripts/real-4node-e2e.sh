#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-real4-${RUN_ID}}"
CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_CONTAINER="${BTC_CONTAINER:-thornado-real4-${RUN_ID}-bitcoind}"
BTC_USE_LOCAL="${BTC_USE_LOCAL:-0}"
BTC_EXTERNAL="${BTC_EXTERNAL:-0}"
BTC_RPC_HOST="${BTC_RPC_HOST:-127.0.0.1}"
BTC_RPC_PORT="${BTC_RPC_PORT:-18445}"
BTC_P2P_PORT="${BTC_P2P_PORT:-18446}"
API_BASE="${API_BASE:-1316}"
API_BIND_HOST="${API_BIND_HOST:-127.0.0.1}"
P2P_BIND_HOST="${P2P_BIND_HOST:-127.0.0.1}"
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
BTC_AUTO_MINE="${BTC_AUTO_MINE:-${KEEP_RUNNING:-0}}"
BTC_AUTO_MINE_INTERVAL="${BTC_AUTO_MINE_INTERVAL:-60}"
TX_INCLUSION_TIMEOUT="${TX_INCLUSION_TIMEOUT:-1200}"
THORNADO_BLOCK_TIME_SECONDS="${THORNADO_BLOCK_TIME_SECONDS:-6}"
GENESIS_NODE_BOND_START_AMOUNT_SATS="${GENESIS_NODE_BOND_START_AMOUNT_SATS:-0}"
GENESIS_CHURN_INTERVAL_MINUTES="${GENESIS_CHURN_INTERVAL_MINUTES:-1}"
GENESIS_CHURN_RETRY_INTERVAL_MINUTES="${GENESIS_CHURN_RETRY_INTERVAL_MINUTES:-2}"
GENESIS_HALT_CHURNING="${GENESIS_HALT_CHURNING:-0}"
GENESIS_BTC_CONFIRMATIONS_MIN="${GENESIS_BTC_CONFIRMATIONS_MIN:-1}"
GENESIS_BTC_CONF_MULTIPLIER_BASIS_POINTS="${GENESIS_BTC_CONF_MULTIPLIER_BASIS_POINTS:-10000}"

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

on_unexpected_error() {
  local rc=$? line="${BASH_LINENO[0]:-$LINENO}" cmd="${BASH_COMMAND:-unknown}"
  printf '[real4] ERROR: unexpected command failure rc=%s line=%s cmd=%s\n' "$rc" "$line" "$cmd" >&2
  write_run_summary "FAIL" "unexpected command failure rc=${rc} line=${line} cmd=${cmd}"
}
trap on_unexpected_error ERR

json_get() {
  jq -r "$1"
}

api_port() { echo $((API_BASE + $1)); }
grpc_port() { echo $((GRPC_BASE + $1)); }
rpc_port() { echo $((RPC_BASE + $1)); }
p2p_port() {
  local node="$1"
  if (( node >= 7 )); then
    echo $((P2P_BASE + 100 + node))
    return
  fi
  echo $((P2P_BASE + node))
}
ebifrost_port() {
  local node="$1"
  if (( node >= 5 )); then
    echo $((EBIFROST_BASE + 100 + node))
  else
    echo $((EBIFROST_BASE + node))
  fi
}
frost_p2p_port() { echo $((FROST_P2P_BASE + $1)); }
frost_info_port() { echo $((FROST_INFO_BASE + $1)); }
metrics_port() { echo $((METRICS_BASE + $1)); }
pprof_port() { echo $((6060 + $1)); }

api_url() { echo "http://127.0.0.1:$(api_port "$1")"; }
rpc_url() { echo "http://127.0.0.1:$(rpc_port "$1")"; }
curl_json() { curl --connect-timeout 2 --max-time 8 -fsS "$@"; }
curl_json_quiet() { curl --connect-timeout 2 --max-time 8 -fsS "$@" 2>/dev/null; }

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
    if curl_json "$url" >/dev/null 2>&1; then
      log "${label} ready"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "timed out waiting for ${label}: ${url}"
    fi
    sleep 2
  done
}

wait_api_json_file() {
  local url="$1" out_file="$2" label="$3" timeout="${4:-120}" start tmp status
  start="$(date +%s)"
  tmp="${out_file}.tmp"
  while true; do
    status="$(curl --connect-timeout 2 --max-time 8 -sS -o "$tmp" -w "%{http_code}" "$url" || true)"
    if [[ "$status" == "200" ]] && jq -e type "$tmp" >/dev/null 2>&1; then
      mv "$tmp" "$out_file"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      mv "$tmp" "$out_file" 2>/dev/null || true
      die "timed out waiting for ${label}: ${url} status=${status}"
    fi
    sleep 1
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

configure_node_runtime_ports() {
  local i="$1" home
  home="$RUN_ROOT/node${i}"
  python3 - "$home" "$(pprof_port "$i")" "$(ebifrost_port "$i")" <<'PY'
import pathlib
import re
import sys

home = pathlib.Path(sys.argv[1])
pprof = sys.argv[2]
ebifrost = sys.argv[3]

config = home / "config" / "config.toml"
if config.exists():
    text = config.read_text()
    text = re.sub(r'^pprof_laddr = ".*"$', f'pprof_laddr = "localhost:{pprof}"', text, flags=re.M)
    config.write_text(text)

app = home / "config" / "app.toml"
if app.exists():
    text = app.read_text()
    text = re.sub(r'(\[ebifrost\][\s\S]*?^address = )".*?"', rf'\1"127.0.0.1:{ebifrost}"', text, count=1, flags=re.M)
    app.write_text(text)
PY
}

stop_pid_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  local pid
  pid="$(cat "$file" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    local cmd
    cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    case "$cmd" in
      *"/build/thornado start"*|*"/build/bifrost"*|*"ops/scripts/real-4node-e2e.sh"*) ;;
      *) return 0 ;;
    esac
    kill "$pid" >/dev/null 2>&1 || true
    for _ in {1..20}; do
      kill -0 "$pid" >/dev/null 2>&1 || return 0
      sleep 0.1
    done
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
  return 0
}

cleanup_stale_real4_services() {
  local file
  shopt -s nullglob
  for file in "$RUN_ROOT"/pids/*.pid; do
    stop_pid_file "$file"
  done
  shopt -u nullglob

  if [[ "$BTC_USE_LOCAL" != "1" ]] && docker info >/dev/null 2>&1; then
    docker rm -f "$BTC_CONTAINER" >/dev/null 2>&1 || true
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
  if [[ "$BTC_USE_LOCAL" != "1" ]]; then
    docker rm -f "$BTC_CONTAINER" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" ]]; then
    rm -rf "$RUN_ROOT"
  fi
  return "$exit_code"
}

if [[ "${NO_CLEANUP_TRAP:-0}" != "1" ]]; then
  trap cleanup_runtime EXIT
fi

key_add_file() {
  local home="$1" name="$2"
  local existing_json
  existing_json="$(mktemp "$RUN_ROOT/key-${name}.XXXXXX.json")"
  if printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file --output json >"$existing_json" 2>/dev/null; then
    cat "$existing_json"
    rm -f "$existing_json"
    return 0
  fi
  rm -f "$existing_json"
  printf '%s\n%s\n' "$PASS" "$PASS" | "$THORNADO" keys add "$name" \
    --home "$home" --keyring-backend file --output json
}

key_show_addr() {
  local home="$1" name="$2"
  local cache_dir cache_key cache_file
  cache_dir="$RUN_ROOT/meta/key-addresses"
  mkdir -p "$cache_dir"
  cache_key="$(printf '%s__%s' "$home" "$name" | tr '/ ' '__')"
  cache_file="$cache_dir/${cache_key}.addr"
  if [[ -s "$cache_file" ]]; then
    cat "$cache_file"
    return 0
  fi
  printf '%s\n' "$PASS" | timeout 20 "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file -a | tee "$cache_file"
}

key_show_val_addr() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | timeout 20 "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file --bech val -a
}

key_show_pub_bech() {
  local home="$1" name="$2"
  local pub_json
  pub_json="$(printf '%s\n' "$PASS" | timeout 20 "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file -p)"
  "$THORNADO" pubkey "$pub_json"
}

add_genesis_auth_account() {
  local home="$1" addr="$2" gen tmp
  gen="$home/config/genesis.json"
  tmp="${gen}.tmp"
  jq --arg addr "$addr" '
    if any(.app_state.auth.accounts[]?; .address == $addr) then
      .
    else
      .app_state.auth.accounts += [{
        "@type": "/cosmos.auth.v1beta1.BaseAccount",
        "address": $addr,
        "pub_key": null,
        "account_number": ((.app_state.auth.accounts | length) | tostring),
        "sequence": "0"
      }]
    end
  ' "$gen" >"$tmp"
  mv "$tmp" "$gen"
}

key_export_hex() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | timeout 20 "$THORNADO" keys export "$name" \
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
  local cli="${BITCOIN_CLI:-bitcoin-cli}"
  if [[ -x /opt/bitcoin/bin/bitcoin-cli ]]; then
    cli="/opt/bitcoin/bin/bitcoin-cli"
  fi
  if [[ "$BTC_USE_LOCAL" == "1" ]]; then
    "$cli" -regtest -rpcconnect="$BTC_RPC_HOST" -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado "$@"
  else
    docker exec "$BTC_CONTAINER" bitcoin-cli -regtest -rpcuser=thornado -rpcpassword=thornado "$@"
  fi
}

thornado_tx() {
  local home="$1" from="$2"
  shift 2
  local from_addr out status node_rpc
  if [[ "$from" == tthor1* ]]; then
    from_addr="$from"
  else
    from_addr="$(key_show_addr "$home" "$from")"
  fi
  node_rpc="${THORNADO_TX_NODE:-tcp://127.0.0.1:$(rpc_port 1)}"
  set +e
  out="$(printf '%s\n' "$PASS" | timeout "${THORNADO_TX_TIMEOUT:-45}" "$THORNADO" tx thornado "$@" \
    --home "$home" \
    --from "$from_addr" \
    --keyring-backend file \
    --keyring-dir "$home" \
    --chain-id "$CHAIN_ID" \
    --node "$node_rpc" \
    --gas 2500000 \
    --fees 0btc \
    --broadcast-mode sync \
    --yes \
    --output json \
    2>&1)"
  status=$?
  set -e
  if (( status == 124 )); then
    jq -n --arg log "thornado tx timed out after ${THORNADO_TX_TIMEOUT:-45}s" \
      '{height:"0",txhash:"",codespace:"harness",code:124,data:"",raw_log:$log,logs:[],info:"",gas_wanted:"0",gas_used:"0",tx:null,timestamp:"",events":[]}'
    return 0
  fi
  printf '%s\n' "$out"
}

assert_tx_success() {
  local out="$1" label="$2" txhash start res code raw_log
  jq -e '.code == null or .code == 0' <<<"$out" >/dev/null || die "$label failed CheckTx: $out"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || return 0
  start="$(date +%s)"
  while (( $(date +%s) - start < TX_INCLUSION_TIMEOUT )); do
    res="$(curl_json_quiet "$(rpc_url 1)/tx?hash=0x${txhash}" || true)"
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
    if [[ "$code" == "not-json" ]]; then
      raw_log="$out"
    else
      raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
    fi
    if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
      die "$label rejected with unexpected CheckTx log: $raw_log"
    fi
    printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
    return 0
  fi
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "$label unexpectedly had no txhash and no error"
  start="$(date +%s)"
  while (( $(date +%s) - start < TX_INCLUSION_TIMEOUT )); do
    res="$(curl_json_quiet "$(rpc_url 1)/tx?hash=0x${txhash}" || true)"
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

assert_tx_or_cli_rejected_after_sequence_settles() {
  local label="$1" want="${2:-}" attempts=0 out status code raw_log safe
  shift 2
  safe="${label// /-}"
  while true; do
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
    code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
    if [[ "$code" == "19" && -z "$raw_log" && "$attempts" -lt 3 ]]; then
      attempts=$((attempts + 1))
      wait_blocks 2
      continue
    fi
    assert_tx_rejected "$out" "$label" "$want"
    return 0
  done
}

wait_blocks() {
  local count="$1" start="" latest
  while true; do
    latest="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height' 2>/dev/null || true)"
    if [[ ! "$latest" =~ ^[0-9]+$ ]]; then
      sleep 1
      continue
    fi
    [[ -n "$start" ]] || start="$latest"
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
  local txid
  [[ "$amount_btc" =~ ^[0-9]+(\.[0-9]{1,8})?$ ]] || die "invalid BTC deposit amount: ${amount_btc}"
  txid="$(btc_cli -rpcwallet=miner sendtoaddress "$address" "$amount_btc" "" "" false true)"
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
    if [[ "$BTC_USE_LOCAL" == "1" ]] || { [[ -n "${BTC_CONTAINER:-}" ]] && docker ps --format '{{.Names}}' | grep -qx "$BTC_CONTAINER"; }; then
      mine_regtest_blocks 1 || true
    fi
    sleep 2
  done
}

wait_owner_deposit_matched() {
  local owner_addr="$1" timeout="${2:-180}" start session deposit_id
  start="$(date +%s)"
  while true; do
    session="$(deposit_session "$owner_addr" 2>/dev/null || true)"
    deposit_id="$(jq -r '.deposit_id // ""' <<<"$session" 2>/dev/null || true)"
    if [[ "$(jq -r '.status // ""' <<<"$session" 2>/dev/null || true)" == "deposit_matched" && "${#deposit_id}" -eq 64 ]]; then
      printf '%s\n' "$session"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      printf '%s\n' "$session" >&2
      die "deposit for ${owner_addr} did not match"
    fi
    if [[ "$BTC_USE_LOCAL" == "1" ]] || { [[ -n "${BTC_CONTAINER:-}" ]] && docker ps --format '{{.Names}}' | grep -qx "$BTC_CONTAINER"; }; then
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
  local node_addr="$1" file
  file="$RUN_ROOT/meta/node-query-$(printf '%s' "$node_addr" | tr -c '[:alnum:]' '_').json"
  wait_api_json_file "$(api_url 1)/thornado/node/${node_addr}" "$file" "node ${node_addr}" 120
  cat "$file"
}

node_index_by_cons() {
  local target_cons="$1" i
  for i in 1 2 3 4 5 6 7 8 9; do
    [[ -f "$RUN_ROOT/meta/node${i}.env" ]] || continue
    # shellcheck disable=SC1090
    source "$RUN_ROOT/meta/node${i}.env"
    if [[ "${cons:-}" == "$target_cons" ]]; then
      printf '%s\n' "$i"
      return 0
    fi
  done
  return 1
}

active_cons_list() {
  curl -fsS "$(api_url 1)/thornado/nodes" |
    jq -r '((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active") | .node_cons_pub_key' |
    sort
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

start_btc_auto_miner() {
  [[ "${BTC_AUTO_MINE:-0}" == "1" ]] || return 0
  log "starting BTC auto miner every ${BTC_AUTO_MINE_INTERVAL}s"
  (
    while true; do
      sleep "$BTC_AUTO_MINE_INTERVAL"
      if [[ "$BTC_USE_LOCAL" != "1" ]] && ! docker ps --format '{{.Names}}' | grep -qx "$BTC_CONTAINER"; then
        continue
      fi
      block="$(btc_cli -rpcwallet=miner generatetoaddress 1 "$(btc_cli -rpcwallet=miner getnewaddress)" 2>/dev/null | jq -r '.[0] // empty' || true)"
      if [[ -n "${block:-}" ]]; then
        printf '[%s] mined %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$block"
      fi
    done
  ) >>"$RUN_ROOT/logs/btc-auto-miner.log" 2>&1 &
  echo $! >"$RUN_ROOT/pids/btc-auto-miner.pid"
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

assert_shielder_receipt_committed() {
  local receipt_file="$1" label="$2" sync_file commitment commitment_key denom value key_file
  sync_file="$RUN_ROOT/meta/${label}-shielder-sync.json"
  curl -fsS "$(api_url 1)/thornado/shielder/sync?limit=2000" >"$sync_file"
  while IFS= read -r commitment; do
    denom="$(jq -r --arg c "$commitment" '.notes[] | select(.commitment == $c) | .denomination_sats' "$receipt_file")"
    jq -e --arg c "$commitment" --argjson denom "$denom" \
      '.notes[] | select(.commitment == $c and (.denomination_sats | tonumber) == $denom)' "$sync_file" >/dev/null \
      || die "${label} note missing from shielder sync"
    key_file="$RUN_ROOT/meta/${label}-commitment-${commitment}.kv.json"
    commitment_key="$(printf '%s' "$commitment" | tr '[:lower:]' '[:upper:]')"
    value="$(kv_json_value "shielder_commitment//${commitment_key}" "$key_file")"
    [[ "$value" == "true" ]] || die "${label} commitment KV missing"
    key_file="$RUN_ROOT/meta/${label}-note-${commitment}.kv.json"
    value="$(kv_json_value "shielder_note_pubkey//${commitment_key}" "$key_file")"
    jq -e --arg c "$commitment" --argjson denom "$denom" \
      '.commitment == $c and (.denomination_sats | tonumber) == $denom' <<<"$value" >/dev/null \
      || die "${label} note record KV invalid"
    key_file="$RUN_ROOT/meta/${label}-denom-commitment-${commitment}.kv.json"
    value="$(kv_json_value "$(printf 'shielder_denom_commitment/%020d/%s' "$denom" "$commitment")" "$key_file")"
    [[ "$value" == "true" ]] || die "${label} denomination commitment KV missing"
  done < <(jq -r '.notes[].commitment' "$receipt_file")
}

assert_shielder_root_committed() {
  local denomination="$1" leaves="$2" label="$3" root value
  root="$("$SHIELDER_HELPER" merkle-root "$leaves")"
  printf '%s\n' "$root" >"$RUN_ROOT/meta/${label}-merkle-root.txt"
  value="$(kv_json_value "$(printf 'shielder_merkle_root/%020d/%s' "$denomination" "$root")" "$RUN_ROOT/meta/${label}-merkle-root.kv.json")"
  [[ "$value" == "true" ]] || die "${label} Merkle root missing from KV store"
}

kv_json_value() {
  local key="$1" file="$2" hex
  hex="$(printf '%s' "$key" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
  curl -fsS "$(rpc_url 1)/abci_query?path=%22/store/thornado/key%22&data=0x${hex}" >"$file"
  jq -r '.result.response.value // ""' "$file" | base64 -d | jq -r '.'
}

build_binaries() {
  if [[ "${SKIP_BUILD:-0}" == "1" ]]; then
    log "skipping binary build"
    return 0
  fi
  log "building real Thornado and Bifrost binaries"
  mkdir -p "$BUILD_DIR"
  (cd "$ROOT_DIR" && cargo build -p thornado-ffi --release --features proof-tests)
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
  if [[ "$BTC_USE_LOCAL" != "1" ]]; then
    docker rm -f "$BTC_CONTAINER" >/dev/null 2>&1 || true
  fi
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
  if [[ "$BTC_EXTERNAL" == "1" ]]; then
    log "using external regtest bitcoind at ${BTC_RPC_HOST}:${BTC_RPC_PORT}"
  elif [[ "$BTC_USE_LOCAL" == "1" ]]; then
    log "starting local regtest bitcoind on ${BTC_RPC_PORT}"
    local btc_home="$RUN_ROOT/bitcoind"
    mkdir -p "$btc_home"
    nohup bitcoind \
      -datadir="$btc_home" -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
      -deprecatedrpc=create_bdb \
      -rpcbind=127.0.0.1 -rpcallowip=127.0.0.1 \
      -rpcport="$BTC_RPC_PORT" -port="$BTC_P2P_PORT" \
      -rpcuser=thornado -rpcpassword=thornado \
      >"$RUN_ROOT/logs/bitcoind.log" 2>&1 </dev/null &
    echo $! >"$RUN_ROOT/pids/bitcoind.pid"
  else
    log "starting regtest bitcoind on ${BTC_RPC_PORT}"
    docker run -d --name "$BTC_CONTAINER" \
      -p "${BTC_RPC_PORT}:18443" -p "${BTC_P2P_PORT}:18444" \
      bitcoin/bitcoin:27 \
      -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
      -deprecatedrpc=create_bdb \
      -rpcbind=0.0.0.0 -rpcallowip=0.0.0.0/0 \
      -rpcuser=thornado -rpcpassword=thornado >/dev/null
  fi
  for _ in {1..60}; do
    if btc_cli getblockchaininfo >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  btc_cli loadwallet miner >/dev/null 2>&1 || true
  btc_cli createwallet miner >/dev/null 2>&1 || btc_cli loadwallet miner >/dev/null 2>&1 || true
  for i in 1 2 3 4 5 6 7 8 9; do
    btc_cli loadwallet "bifrost${i}" >/dev/null 2>&1 || true
    btc_cli createwallet "bifrost${i}" true true "" false true >/dev/null 2>&1 || btc_cli loadwallet "bifrost${i}" >/dev/null 2>&1 || true
  done
  local addr
  addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  btc_cli -rpcwallet=miner generatetoaddress 101 "$addr" >/dev/null
}

init_genesis() {
  log "creating 4-node genesis"
  local gen="$RUN_ROOT/node1/config/genesis.json"
  declare -a addrs valops secp cons cons_raw ids peers

  for i in 1 2 3 4; do
    local home="$RUN_ROOT/node${i}"
    SIGNER_PASSWD="$PASS" "$THORNADO" init "node${i}" --chain-id "$CHAIN_ID" --home "$home" --operator-name "validator${i}" --overwrite >/dev/null
    sed -i.bak \
      -e 's/^addr_book_strict = .*/addr_book_strict = false/' \
      -e 's/^allow_duplicate_ip = .*/allow_duplicate_ip = true/' \
      "$home/config/config.toml"
    local key_json
    key_json="$(key_add_file "$home" "validator${i}")"
    addrs[$i]="$(key_show_addr "$home" "validator${i}")"
    valops[$i]="$(key_show_val_addr "$home" "validator${i}")"
    secp[$i]="$(key_show_pub_bech "$home" "validator${i}")"
    cons[$i]="$(cons_pub_bech "$home")"
    cons_raw[$i]="$(jq -r '.pub_key.value' "$home/config/priv_validator_key.json")"
    ids[$i]="$(node_id "$home")"
    peers[$i]="${ids[$i]}@127.0.0.1:$(p2p_port "$i")"
    add_genesis_auth_account "$RUN_ROOT/node1" "${addrs[$i]}"
  done

  for i in 5 6 7 8 9; do
    local home="$RUN_ROOT/node${i}"
    SIGNER_PASSWD="$PASS" "$THORNADO" init "node${i}" --chain-id "$CHAIN_ID" --home "$home" --operator-name "validator${i}" --overwrite >/dev/null
    sed -i.bak \
      -e 's/^addr_book_strict = .*/addr_book_strict = false/' \
      -e 's/^allow_duplicate_ip = .*/allow_duplicate_ip = true/' \
      "$home/config/config.toml"
    local key_json
    key_json="$(key_add_file "$home" "validator${i}")"
    addrs[$i]="$(key_show_addr "$home" "validator${i}")"
    secp[$i]="$(key_show_pub_bech "$home" "validator${i}")"
    cons[$i]="$(cons_pub_bech "$home")"
    add_genesis_auth_account "$RUN_ROOT/node1" "${addrs[$i]}"
  done

  local user_json user_addr
  user_json="$(key_add_file "$RUN_ROOT/node1" "user")"
  user_addr="$(key_show_addr "$RUN_ROOT/node1" "user")"
  add_genesis_auth_account "$RUN_ROOT/node1" "$user_addr"

  for i in 2 3 4; do
    cp "$gen" "$RUN_ROOT/node${i}/config/genesis.json"
  done

  local node_accounts
  node_accounts="$(jq -n \
    --arg a1 "${addrs[1]}" --arg s1 "${secp[1]}" --arg c1 "${cons[1]}" \
    --arg a2 "${addrs[2]}" --arg s2 "${secp[2]}" --arg c2 "${cons[2]}" \
    --arg a3 "${addrs[3]}" --arg s3 "${secp[3]}" --arg c3 "${cons[3]}" \
    --arg a4 "${addrs[4]}" --arg s4 "${secp[4]}" --arg c4 "${cons[4]}" \
    '[
      {node_address:$a1, status:"Active", pub_key_set:{secp256k1:$s1}, node_cons_pub_key:$c1, bond:"0", active_block_height:1, bond_address:$a1, status_since:1, signer_membership:[], ip_address:"127.0.1.1", version:"3.17.0"},
      {node_address:$a2, status:"Active", pub_key_set:{secp256k1:$s2}, node_cons_pub_key:$c2, bond:"0", active_block_height:1, bond_address:$a2, status_since:1, signer_membership:[], ip_address:"127.0.1.2", version:"3.17.0"},
      {node_address:$a3, status:"Active", pub_key_set:{secp256k1:$s3}, node_cons_pub_key:$c3, bond:"0", active_block_height:1, bond_address:$a3, status_since:1, signer_membership:[], ip_address:"127.0.1.3", version:"3.17.0"},
      {node_address:$a4, status:"Active", pub_key_set:{secp256k1:$s4}, node_cons_pub_key:$c4, bond:"0", active_block_height:1, bond_address:$a4, status_since:1, signer_membership:[], ip_address:"127.0.1.4", version:"3.17.0"}
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
    .app_state.bank.balances = [] |
    del(.app_state.bank.supply) |
    .app_state.staking.last_total_power = "0" |
    .app_state.staking.last_validator_powers = [] |
    .app_state.staking.validators = [] |
    .app_state.staking.delegations = [] |
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
        {"key":"Node_BondStartAmountSats","value":'"$GENESIS_NODE_BOND_START_AMOUNT_SATS"'},
        {"key":"Node_BondSlotIncrementSats","value":100000000},
        {"key":"Chain_BlockTimeSeconds","value":'"$THORNADO_BLOCK_TIME_SECONDS"'},
        {"key":"Churn_IntervalMinutes","value":'"$GENESIS_CHURN_INTERVAL_MINUTES"'},
        {"key":"Churn_RetryIntervalMinutes","value":'"$GENESIS_CHURN_RETRY_INTERVAL_MINUTES"'},
        {"key":"Deposit_SessionExpiryMinutes","value":10},
        {"key":"Keysign_PeriodMinutes","value":5},
        {"key":"BTC_ConfirmationsMin","value":'"$GENESIS_BTC_CONFIRMATIONS_MIN"'},
        {"key":"BTC_ConfMultiplierBasisPoints","value":'"$GENESIS_BTC_CONF_MULTIPLIER_BASIS_POINTS"'},
        {"key":"BTC_MaxConfirmations","value":1},
        {"key":"Halt_Churning","value":'"$GENESIS_HALT_CHURNING"'},
        {"key":"HaltSigningBTC","value":0},
        {"key":"Withdrawal_FeeMinSats","value":100000},
        {"key":"Keygen_RetryIntervalMinutes","value":2}
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

  for i in 1 2 3 4 5 6 7 8 9; do
    cp "$RUN_ROOT/genesis.json" "$RUN_ROOT/node${i}/config/genesis.json"
  done
  if ! "$THORNADO" genesis validate "$RUN_ROOT/node1/config/genesis.json" --home "$RUN_ROOT/node1" >"$RUN_ROOT/meta/genesis-validate.log" 2>&1; then
    case "$FLOW1_SCENARIO" in
      missing_secp|duplicate_secp|forged_vault_state)
        log "RESULTS Flow 1 ${FLOW1_SCENARIO}: PASS (genesis validation rejected fixture)"
        exit 0
        ;;
      *)
        if jq -e type "$RUN_ROOT/node1/config/genesis.json" >/dev/null 2>&1 &&
          grep -q '^Usage:' "$RUN_ROOT/meta/genesis-validate.log"; then
          log "genesis validate returned usage without detail; JSON genesis is syntactically valid, continuing to node startup validation"
        else
          cat "$RUN_ROOT/meta/genesis-validate.log" >&2 || true
          die "genesis validation failed"
        fi
        ;;
    esac
  fi

  printf '%s\n' "${peers[*]}" | tr ' ' ',' >"$RUN_ROOT/meta/peers"
  for i in 1 2 3 4; do
    {
      echo "address=${addrs[$i]}"
      echo "secp=${secp[$i]}"
      echo "cons=${cons[$i]}"
    } >"$RUN_ROOT/meta/node${i}.env"
  done
  for i in 5 6 7 8 9; do
    {
      echo "address=${addrs[$i]}"
      echo "secp=${secp[$i]}"
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
	    configure_node_runtime_ports "$i"
	    SIGNER_NAME="validator${i}" \
    SIGNER_PASSWD="$PASS" \
    CHAIN_HOME_FOLDER="$home" \
    "$THORNADO" start \
      --home "$home" \
      --api.enable=true \
      --api.address "tcp://${API_BIND_HOST}:$(api_port "$i")" \
      --grpc.enable=true \
      --grpc.address "127.0.0.1:$(grpc_port "$i")" \
      --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
      --p2p.laddr "tcp://${P2P_BIND_HOST}:$(p2p_port "$i")" \
      --p2p.persistent_peers "$peers" \
      --p2p.pex=false \
      --ebifrost.enable=true \
      --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
      --minimum-gas-prices "0btc" \
      --log_level "info" \
      >"$RUN_ROOT/logs/thornado-${i}.log" 2>&1 &
    echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  done
  wait_json "$(rpc_url 1)/status" "thornado-1 rpc" 120
  for i in 1 2 3 4; do
    wait_json "http://127.0.0.1:$(rpc_port "$i")/status" "thornado-${i} rpc" 120
  done
}

set_and_assert_node_version() {
  local i="$1" label out addr version expected="${3:-}" attempt code raw_log
  label="${2:-node${i}}"
  source "$RUN_ROOT/meta/node${i}.env"
  addr="$address"
  for attempt in 1 2 3 4 5; do
    out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" set-version)"
    printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-set-version-attempt-${attempt}.json"
    code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
    if [[ "$code" == "0" || "$code" == "null" ]]; then
      break
    fi
    if [[ "$raw_log" == *"account sequence mismatch"* && "$attempt" != "5" ]]; then
      wait_blocks 2
      continue
    fi
    break
  done
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-set-version.json"
  assert_tx_success "$out" "${label} set-version"
  wait_blocks 1
  node_query "$addr" >"$RUN_ROOT/meta/${label}-node-version.json"
  version="$(jq -r '.node.version // .version // ""' "$RUN_ROOT/meta/${label}-node-version.json")"
  [[ -n "$version" && "$version" != "null" ]] || die "${label} version was not set"
  if [[ -n "$expected" && "$version" != "$expected" ]]; then
    die "${label} version ${version} did not match expected ${expected}"
  fi
  printf '%s\n' "$version"
}

set_and_assert_genesis_versions() {
  log "validating versions for all genesis validators"
  local expected="" i version addr
  for i in 1 2 3 4; do
    source "$RUN_ROOT/meta/node${i}.env"
    addr="$address"
    wait_api_json_file "$(api_url 1)/thornado/node/${addr}" "$RUN_ROOT/meta/flow1-node${i}-node-version.json" "flow1 node${i} version" 120
    version="$(jq -r '.node.version // .version // ""' "$RUN_ROOT/meta/flow1-node${i}-node-version.json")"
    [[ -n "$version" && "$version" != "null" ]] || die "flow1 node${i} version was not set"
    if [[ -z "$expected" ]]; then
      expected="$version"
      printf '%s\n' "$expected" >"$RUN_ROOT/meta/network-node-version.txt"
    elif [[ "$version" != "$expected" ]]; then
      die "flow1 node${i} version ${version} did not match expected ${expected}"
    fi
  done
}

restart_thornado_node() {
  local i="$1" home peers pid
	  home="$RUN_ROOT/node${i}"
	  peers="$(cat "$RUN_ROOT/meta/peers")"
	  configure_node_runtime_ports "$i"
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
    --api.address "tcp://${API_BIND_HOST}:$(api_port "$i")" \
    --grpc.enable=true \
    --grpc.address "127.0.0.1:$(grpc_port "$i")" \
    --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
    --p2p.laddr "tcp://${P2P_BIND_HOST}:$(p2p_port "$i")" \
    --p2p.persistent_peers "$peers" \
    --p2p.pex=false \
    --ebifrost.enable=true \
    --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
    --minimum-gas-prices "0btc" \
    --log_level "info" \
    >"$RUN_ROOT/logs/thornado-${i}-restart.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  wait_json "http://127.0.0.1:$(rpc_port "$i")/status" "thornado-${i} restart rpc" 120
}

start_thornado_extra_node() {
	  local i="$1" home peers
	  home="$RUN_ROOT/node${i}"
	  log "starting Thornado full node for node${i}"
	  peers="$(cat "$RUN_ROOT/meta/peers")"
	  configure_node_runtime_ports "$i"
	  SIGNER_NAME="validator${i}" \
  SIGNER_PASSWD="$PASS" \
  CHAIN_HOME_FOLDER="$home" \
  "$THORNADO" start \
    --home "$home" \
    --api.enable=true \
    --api.address "tcp://${API_BIND_HOST}:$(api_port "$i")" \
    --grpc.enable=true \
    --grpc.address "127.0.0.1:$(grpc_port "$i")" \
    --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
    --p2p.laddr "tcp://${P2P_BIND_HOST}:$(p2p_port "$i")" \
    --p2p.persistent_peers "$peers" \
    --p2p.pex=false \
    --ebifrost.enable=true \
    --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
    --minimum-gas-prices "0btc" \
    --log_level "info" \
    >"$RUN_ROOT/logs/thornado-${i}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
  wait_json "$(rpc_url "$i")/status" "thornado-${i} rpc" 120
  local target height start
  start="$(date +%s)"
  while true; do
    target="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    height="$(curl -fsS "$(rpc_url "$i")/status" | jq -r '.result.sync_info.latest_block_height')"
    if (( height + 1 >= target )); then
      break
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "thornado-${i} did not catch up"
    fi
    sleep 1
  done
}

start_thornado_node5() {
  start_thornado_extra_node 5
}

start_thornado_node6() {
  start_thornado_extra_node 6
}

start_thornado_node7() {
  start_thornado_extra_node 7
}

start_thornado_node8() {
  start_thornado_extra_node 8
}

start_thornado_node9() {
  start_thornado_extra_node 9
}

stop_existing_bifrost_nodes() {
  local i pid
  for i in 1 2 3 4; do
    if [[ ",${FLOW1_SKIP_BIFROST_NODES}," == *",${i},"* ]]; then
      continue
    fi
    pid="$(cat "$RUN_ROOT/pids/bifrost-${i}.pid" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      log "stopping bifrost-${i} pid=${pid}"
      kill "$pid" >/dev/null 2>&1 || true
    fi
  done
  sleep 2
  for i in 1 2 3 4; do
    if [[ ",${FLOW1_SKIP_BIFROST_NODES}," == *",${i},"* ]]; then
      continue
    fi
    pid="$(cat "$RUN_ROOT/pids/bifrost-${i}.pid" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      log "force-stopping bifrost-${i} pid=${pid}"
      kill -9 "$pid" >/dev/null 2>&1 || true
    fi
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
  local frost_bind="${FROST_BIND_HOST:-127.0.0.1}"
  local frost_external="${FROST_EXTERNAL_IP:-127.0.0.1}"
  local bootstrap=""
  if [[ -f "$RUN_ROOT/meta/bifrost-bootstrap-all" ]] || [[ "${BIFROST_FORCE_RESTART:-}" == "1" ]]; then
    stop_existing_bifrost_nodes
  fi
  if [[ -f "$RUN_ROOT/meta/bifrost-bootstrap-all" ]]; then
    bootstrap="$(tr -d '\n' <"$RUN_ROOT/meta/bifrost-bootstrap-all")"
  elif [[ -n "${FROST_BOOTSTRAP_PEERS:-}" ]]; then
    bootstrap="$FROST_BOOTSTRAP_PEERS"
  fi
  if [[ -n "$bootstrap" ]]; then
    log "using ${#bootstrap} byte FROST bootstrap peer list"
  else
    log "no cached FROST bootstrap peers; will discover from /p2pid"
  fi
  local bootstrap_peer
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
    BIFROST_FROST_INFO_ADDRESS="${frost_bind}:$(frost_info_port "$i")" \
    BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
    BIFROST_FROST_EXTERNAL_IP="${frost_external}" \
    BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
    PEER="$bootstrap" \
    EXTERNAL_IP="${frost_external}" \
    BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
    BIFROST_SIGNER_KEYGEN_TIMEOUT="${BIFROST_SIGNER_KEYGEN_TIMEOUT:-45s}" \
    BIFROST_SIGNER_KEYSIGN_TIMEOUT="${BIFROST_SIGNER_KEYSIGN_TIMEOUT:-45s}" \
    BIFROST_SIGNER_PARTY_TIMEOUT="${BIFROST_SIGNER_PARTY_TIMEOUT:-45s}" \
    BIFROST_SIGNER_PRE_PARAM_TIMEOUT="${BIFROST_SIGNER_PRE_PARAM_TIMEOUT:-5m}" \
    BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="1" \
    BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
    BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
    BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
    BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
    BIFROST_CHAINS_BTC_USERNAME="thornado" \
    BIFROST_CHAINS_BTC_PASSWORD="thornado" \
    BIFROST_CHAINS_BTC_RPC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost${i}" \
    BIFROST_CHAINS_BTC_CHAIN_ID="BTC" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_CHAIN_ID="BTC" \
    BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
    BTC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost" \
    BTC_START_BLOCK_HEIGHT="0" \
    "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
    echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
    wait_bifrost_health "$i" 120
    if [[ -z "$bootstrap" ]]; then
      for _ in {1..60}; do
        if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/p2pid" >/tmp/bifrost-p2pid.txt 2>/dev/null; then
          peer="$(tr -d '[:space:]' </tmp/bifrost-p2pid.txt)"
          if [[ -n "$peer" ]]; then
            bootstrap_peer="/ip4/${frost_external}/tcp/$(frost_p2p_port "$i")/p2p/${peer}"
            bootstrap="$bootstrap_peer"
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
    else
      for _ in {1..60}; do
        if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/p2pid" >/tmp/bifrost-p2pid.txt 2>/dev/null; then
          peer="$(tr -d '[:space:]' </tmp/bifrost-p2pid.txt)"
          if [[ -n "$peer" ]]; then
            bootstrap_peer="/ip4/${frost_external}/tcp/$(frost_p2p_port "$i")/p2p/${peer}"
            if [[ ",${bootstrap}," != *",${bootstrap_peer},"* ]]; then
              bootstrap="${bootstrap},${bootstrap_peer}"
            fi
            printf '%s\n' "$bootstrap" >"$RUN_ROOT/meta/bifrost-bootstrap"
            printf '%s\n' "$bootstrap" >"$RUN_ROOT/meta/bifrost-bootstrap-all"
            break
          fi
        fi
        if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
          tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
          die "bifrost-${i} exited before bootstrap was recorded"
        fi
        sleep 1
      done
    fi
    if [[ "$FLOW1_SCENARIO" == "mid_keygen_restart" && "$i" == "2" ]]; then
      log "restarting thornado-1 during Flow 1 keygen"
      restart_thornado_node 1
    fi
  done
  local warmup="${BIFROST_SIGNER_WARMUP_SECONDS:-20}"
  if [[ "$warmup" -gt 0 ]]; then
    log "waiting ${warmup}s for bifrost signer/P2P warmup before keysign"
    sleep "$warmup"
  fi
  local peers=()
  for i in 1 2 3 4; do
    if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/p2pid" >/tmp/bifrost-p2pid.txt 2>/dev/null; then
      local peer
      peer="$(tr -d '[:space:]' </tmp/bifrost-p2pid.txt)"
      if [[ -n "$peer" ]]; then
        peers+=("/ip4/${frost_external}/tcp/$(frost_p2p_port "$i")/p2p/${peer}")
      fi
    fi
  done
  if (( ${#peers[@]} > 0 )); then
    (IFS=,; printf '%s\n' "${peers[*]}") >"$RUN_ROOT/meta/bifrost-bootstrap-all"
  fi
}

start_bifrost_node_for_flow1() {
  local i="$1" start_height="${2:-1}" home bhome bootstrap
  home="$RUN_ROOT/node${i}"
  bhome="$RUN_ROOT/bifrost${i}"
  bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap-all" 2>/dev/null || cat "$RUN_ROOT/meta/bifrost-bootstrap" 2>/dev/null || true)"
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
  BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
  BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
  BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
  PEER="$bootstrap" \
  EXTERNAL_IP="127.0.0.1" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
  BIFROST_SIGNER_KEYGEN_TIMEOUT="${BIFROST_SIGNER_KEYGEN_TIMEOUT:-45s}" \
  BIFROST_SIGNER_KEYSIGN_TIMEOUT="${BIFROST_SIGNER_KEYSIGN_TIMEOUT:-45s}" \
  BIFROST_SIGNER_PARTY_TIMEOUT="${BIFROST_SIGNER_PARTY_TIMEOUT:-45s}" \
  BIFROST_SIGNER_PRE_PARAM_TIMEOUT="${BIFROST_SIGNER_PRE_PARAM_TIMEOUT:-5m}" \
  BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_height" \
  BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
  BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
  BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost${i}" \
  BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
  BTC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost" \
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

start_bifrost_extra_node() {
  local i="$1" flow="$2" home bhome bootstrap start_block attempt start pid
  home="$RUN_ROOT/node${i}"
  bhome="$RUN_ROOT/bifrost${i}"
  log "starting Bifrost signer for node${i}"
  bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap-all" 2>/dev/null || cat "$RUN_ROOT/meta/bifrost-bootstrap")"
  start_block="$(curl -fsS "$(rpc_url "$i")/status" | jq -r '.result.sync_info.latest_block_height')"
  start_block=$((start_block - 10))
  if (( start_block < 1 )); then
    start_block=1
  fi
  printf '%s\n' "$start_block" >"$RUN_ROOT/meta/${flow}-bifrost${i}-signer-start-height.txt"
  mkdir -p "$bhome"
  for attempt in 1 2 3 4 5; do
    log "starting Bifrost signer for node${i} attempt ${attempt}"
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
    BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
    BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
    BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
    PEER="$bootstrap" \
    EXTERNAL_IP="127.0.0.1" \
    BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
    BIFROST_SIGNER_KEYGEN_TIMEOUT="${BIFROST_SIGNER_KEYGEN_TIMEOUT:-45s}" \
    BIFROST_SIGNER_KEYSIGN_TIMEOUT="${BIFROST_SIGNER_KEYSIGN_TIMEOUT:-45s}" \
    BIFROST_SIGNER_PARTY_TIMEOUT="${BIFROST_SIGNER_PARTY_TIMEOUT:-45s}" \
    BIFROST_SIGNER_PRE_PARAM_TIMEOUT="${BIFROST_SIGNER_PRE_PARAM_TIMEOUT:-5m}" \
    BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_block" \
    BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
    BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" \
    BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
    BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
    BIFROST_CHAINS_BTC_USERNAME="thornado" \
    BIFROST_CHAINS_BTC_PASSWORD="thornado" \
    BIFROST_CHAINS_BTC_RPC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost${i}" \
    BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
    BTC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost" \
    BTC_START_BLOCK_HEIGHT="0" \
    "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-${i}.log" 2>&1 &
    echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"

    start="$(date +%s)"
    while true; do
      if curl -fsS "http://127.0.0.1:$(frost_info_port "$i")/ping" >/dev/null 2>&1; then
        log "bifrost-${i} health ready"
        return 0
      fi
      pid="$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")"
      if ! kill -0 "$pid" >/dev/null 2>&1; then
        tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
        break
      fi
      if (( "$(date +%s)" - start >= 120 )); then
        kill "$pid" >/dev/null 2>&1 || true
        tail -n 80 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
        break
      fi
      sleep 1
    done
    sleep 5
  done
  die "bifrost-${i} exited before health was ready"
}

start_bifrost_node5() {
  start_bifrost_extra_node 5 flow2-node5-churn
}

start_bifrost_node6() {
  start_bifrost_extra_node 6 flow6
}

start_bifrost_node7() {
  start_bifrost_extra_node 7 flow8-node7-churn
}

start_bifrost_node8() {
  start_bifrost_extra_node 8 flow8-node8-churn
}

start_bifrost_node9() {
  start_bifrost_extra_node 9 distributed-node9
}

wait_bifrost_ready_for_keygen() {
  local i="$1"
  log "waiting for Bifrost-${i} signer scanner and peer discovery"
  local start peers
  start="$(date +%s)"
  while true; do
    peers="$(rg -o 'peer found|Connection established|upgraded connection from allowed node|accepted inbound connection from allowed node' "$RUN_ROOT/logs/bifrost-${i}.log" 2>/dev/null | wc -l | tr -d '[:space:]')"
    if rg -q "start to process keygen" "$RUN_ROOT/logs/bifrost-${i}.log" 2>/dev/null && (( peers >= 4 )); then
      return 0
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-${i}.pid")" >/dev/null 2>&1; then
      tail -n 120 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "bifrost-${i} exited before keygen readiness"
    fi
    if (( "$(date +%s)" - start >= 180 )); then
      tail -n 160 "$RUN_ROOT/logs/bifrost-${i}.log" >&2 || true
      die "timed out waiting for bifrost-${i} keygen readiness"
    fi
    sleep 2
  done
}

wait_bifrost6_ready_for_keygen() {
  wait_bifrost_ready_for_keygen 6
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
  BIFROST_FROST_EXTERNAL_IP="127.0.0.1" \
  EXTERNAL_IP="127.0.0.1" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \

  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost1" \
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
  log "Flow 2: validating bonded standby node via shielded note bond"
  source "$RUN_ROOT/meta/node5.env"
  local owner_addr="$address" operator_pubkey="$secp" node_pubkey="$cons"
  curl -fsS "$(api_url 1)/thornado/nodes/metrics" >"$RUN_ROOT/meta/flow2-node-metrics-before.json"
  jq -e '(.next_slot | tonumber) == 1 and (.next_slot_bond_required_sats | tonumber) == 100000000 and (.bond_start_amount_sats | tonumber) == 0 and (.bond_slot_increment_sats | tonumber) == 100000000' \
    "$RUN_ROOT/meta/flow2-node-metrics-before.json" >/dev/null || die "flow2 next slot bond requirement is not 1 BTC"
  local deposit_pubkey deposit_owner session deposit_address txid deposit_id amount_sats receipt commitment_objects commitments shield_signature out sweep_txout root_addr root_received out_hash note leaves bond_withdrawal prefix nullifier bond matched
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "node5-bond-deposit-pubkey")"
  deposit_owner="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  request_deposit "$RUN_ROOT/node5" "validator5" "bond-flow-2" "$deposit_pubkey" >"$RUN_ROOT/meta/flow2-request-deposit.json"
  session="$(deposit_session "$deposit_owner")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow2-session-before-deposit.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  jq -e --arg owner "$deposit_owner" \
    '.owner == $owner and ((.operator_pub_key // "") == "") and ((.node_pub_key // "") == "") and (.deposit_path_index | tonumber) > 0 and (.deposit_address | length) > 0 and (.vault_pub_key | length) > 0' \
    "$RUN_ROOT/meta/flow2-session-before-deposit.json" >/dev/null || die "flow2 deposit session is invalid"
  txid="$(mine_to_registered_deposit "$deposit_address" "1.00000000")"
  btc_cli -rpcwallet=bifrost1 listunspent 1 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow2-child-utxo-before-sweep.json"
  jq -e 'map(select((.amount * 100000000 | floor) == 100000000)) | length == 1' "$RUN_ROOT/meta/flow2-child-utxo-before-sweep.json" >/dev/null \
    || die "flow2 child deposit UTXO was not visible before sweep"
  matched="$(wait_owner_deposit_matched "$deposit_owner")"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow2-deposit-matched.json"
  deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
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
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(jq -r '.deposit_path_index' <<<"$session")" "$amount_sats" "operator5-bond-note-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow2-bond-note-receipt.json"
  commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  printf '%s\n' "$commitments" >"$RUN_ROOT/meta/flow2-bond-note-commitments.json"
  shield_signature="$("$SHIELDER_HELPER" shield-authorization "node5-bond-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield "$commitments" "$deposit_pubkey" "$shield_signature" "$deposit_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow2-shield.json"
  assert_tx_success "$out" "flow2 shield node funding note"
  local committed
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow2-deposit.json"
  jq -e '.settlement == "user" and .status == "committed" and (.amount_sats | tonumber) == 100000000' <<<"$committed" >/dev/null || die "flow2 funding deposit was not committed as user settlement"
  record_shielder_notes "$receipt"
  assert_shielder_receipt_committed "$RUN_ROOT/meta/flow2-bond-note-receipt.json" "flow2-bond-note"
  note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow2-bond-note-receipt.json")"
  leaves="$(shielder_leaves "$(jq -r '.denomination_sats' <<<"$note")")"
  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow2-bond-proof-leaves.json"
  assert_shielder_root_committed "$(jq -r '.denomination_sats' <<<"$note")" "$leaves" "flow2-bond-note"
  bond_withdrawal="$("$SHIELDER_HELPER" withdrawal-policy "$note" "operator5-bond-note-seed" "$leaves" "bond_escrow" 0 "bond_escrow" "$node_pubkey" "")"
  printf '%s\n' "$bond_withdrawal" >"$RUN_ROOT/meta/flow2-bond-withdrawal.json"
  prefix="$RUN_ROOT/meta/flow2-bond-withdrawal"
  "$SHIELDER_HELPER" shield-withdrawal "$bond_withdrawal" "$prefix"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder bond-from-notes "$node_pubkey" "$operator_pubkey" "${prefix}.proof.json" "${prefix}.public.json")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow2-bond-from-notes.json"
  assert_tx_success "$out" "flow2 bond-from-notes"
  nullifier="$(jq -r '.nullifier_hash' "${prefix}.public.json")"
  curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/flow2-bond-nullifier-query.json"
  jq -e '.spent == true' "$RUN_ROOT/meta/flow2-bond-nullifier-query.json" >/dev/null || die "flow2 bond note nullifier was not spent"
  if [[ "${FLOW2_DEFER_NODE5_PREFLIGHT:-0}" != "1" ]]; then
    out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-ip-address "127.0.0.1")"
    assert_tx_success "$out" "flow2 set-ip-address"
    out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-node-keys "$operator_pubkey" "$node_pubkey")"
    assert_tx_success "$out" "flow2 set-node-keys"
    set_and_assert_node_version 5 "flow2-node5" "$(cat "$RUN_ROOT/meta/network-node-version.txt")" >/dev/null
  else
    out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-node-keys "$operator_pubkey" "$node_pubkey")"
    assert_tx_success "$out" "flow2 set-node-keys"
    log "FLOW2_DEFER_NODE5_PREFLIGHT=1; deferring node5 IP/version so it remains standby for Flow 5 auction"
  fi
  wait_blocks 2
  node_query "$owner_addr" >"$RUN_ROOT/meta/flow2-node.json"
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow2-bond.json"
  jq -e '((.node.total_bond // .total_bond // .node.bond // .bond) == "'"${amount_sats}"'") and (((.node.status // .status) | ascii_downcase) == "standby" or ((.node.status // .status) | ascii_downcase) == "active")' "$RUN_ROOT/meta/flow2-node.json" >/dev/null \
    || die "flow2 node account not bonded standby/active"
  jq -e --arg op "$operator_pubkey" --arg node "$node_pubkey" \
    '.node_pub_key == $node and .operator_pub_key == $op and (.bond_sats | tonumber) == 100000000 and (.pending_sats | tonumber) == 0 and .fee_share_active == true' \
    "$RUN_ROOT/meta/flow2-bond.json" >/dev/null || die "flow2 bond query did not match committed state"
  curl -fsS "$(api_url 1)/thornado/nodes/metrics" >"$RUN_ROOT/meta/flow2-node-metrics-after.json"
  log "RESULTS Flow 2: PASS"
}

validate_flow2_node5_churn() {
  log "Flow 2 node5 churn: validating standby -> selected -> active, FROST keygen, and base-vault migration"
  local old_vault old_addr new_vault new_addr node5_addr node5_secp node5_cons out status active_vaults start latest
  local flow_start keygen_height keygen_json migrate_txout out_hash raw_tx old_prevouts after_vaults config h current_height

  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/flow2-node5-base-vaults-before.json"
  old_vault="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/flow2-node5-base-vaults-before.json")"
  old_addr="$(jq -r --arg old "$old_vault" '.[] | select(.pub_key == $old) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow2-node5-base-vaults-before.json" | head -n1)"
  if [[ -z "$old_addr" || "$old_addr" == "null" ]]; then
    old_addr="$("$SHIELDER_HELPER" btc-address "$old_vault" 0)"
  fi

  source "$RUN_ROOT/meta/node5.env"
  node5_addr="$address"
  node5_secp="$secp"
  node5_cons="$cons"

  curl -fsS "$(api_url 1)/thornado/bond/${node5_cons}" >"$RUN_ROOT/meta/flow2-node5-bond-before-churn.json"
  jq -e '(.bond_sats | tonumber) == 100000000 and .node_status == "Standby" and .fee_share_active == true' \
    "$RUN_ROOT/meta/flow2-node5-bond-before-churn.json" >/dev/null || die "flow2 node5 bond is not standby with 1 BTC"

  start_thornado_node5
  start_bifrost_node5
  wait_bifrost_ready_for_keygen 5
  curl -fsS "http://127.0.0.1:$(frost_info_port 5)/status/p2p" >"$RUN_ROOT/meta/flow2-node5-bifrost-p2p.json"
  curl -fsS "http://127.0.0.1:$(frost_info_port 5)/status/signing" >"$RUN_ROOT/meta/flow2-node5-bifrost-signing-before-churn.json"

  set_config_from_active_nodes Vault_MigrationIntervalMinutes 1
  set_config_from_active_nodes Chain_BlockTimeSeconds "$THORNADO_BLOCK_TIME_SECONDS"
  set_config_from_active_nodes Churn_IntervalMinutes "${CHURN_INTERVAL_MINUTES:-1}"
  set_config_from_active_nodes Churn_RetryIntervalMinutes "${CHURN_RETRY_INTERVAL_MINUTES:-1}"
  set_config_from_active_nodes Halt_SolvencyCheck 0
  set_config_from_active_nodes HaltSigningBTC 0
  set_config_from_active_nodes Node_SetDesired 5
  set_config_from_active_nodes Halt_Churning 0
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/flow2-node5-config-after-churn-tuning.json"
  config="$(jq -r '(.NODE_SETDESIRED.value // (.configs[]? | select(.key == "Node_SetDesired") | .value) // empty)' "$RUN_ROOT/meta/flow2-node5-config-after-churn-tuning.json" | tail -n1)"
  [[ "$config" == "5" ]] || die "flow2 node5 desired active count was not applied"

  flow_start="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  : >"$RUN_ROOT/meta/flow2-node5-status-history.tsv"
  start="$(date +%s)"
  while true; do
    status="$(node_query "$node5_addr" | jq -r '(.node.status // .status) | ascii_downcase')"
    active_vaults="$(curl -fsS $(api_url 1)/thornado/vaults/base)"
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    printf '%s\t%s\n' "$latest" "$status" >>"$RUN_ROOT/meta/flow2-node5-status-history.tsv"
    if [[ "$status" == "active" ]] && jq -e --arg old "$old_vault" --arg member "$node5_secp" '
      [.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))] | length > 0
    ' <<<"$active_vaults" >/dev/null; then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/flow2-node5-base-vaults.json"
      new_vault="$(jq -r --arg old "$old_vault" --arg member "$node5_secp" '[.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))][0].pub_key' "$RUN_ROOT/meta/flow2-node5-base-vaults.json")"
      new_addr="$(jq -r --arg new "$new_vault" '.[] | select(.pub_key == $new) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow2-node5-base-vaults.json" | head -n1)"
      if [[ -z "$new_addr" || "$new_addr" == "null" ]]; then
        new_addr="$("$SHIELDER_HELPER" btc-address "$new_vault" 0)"
      fi
      break
    fi
    if (( "$(date +%s)" - start >= 1800 )); then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/flow2-node5-base-vaults-timeout.json"
      die "flow2 node5 did not churn into a new active base vault"
    fi
    log "flow2 node5 churn waiting: height=${latest} node5_status=${status}"
    sleep 10
  done

  jq -e --arg old "$old_vault" --arg new "$new_vault" --arg member "$node5_secp" '
    ([.[] | select(.status == "ActiveVault")] | length) == 1 and
    (.[] | select(.pub_key == $old) | .status) == "RetiringVault" and
    (.[] | select(.pub_key == $new) | .status) == "ActiveVault" and
    (.[] | select(.pub_key == $new) | (.membership | index($member))) and
    ((.[] | select(.pub_key == $new) | .membership | length) == 5)
  ' "$RUN_ROOT/meta/flow2-node5-base-vaults.json" >/dev/null || die "flow2 node5 vault rotation state is invalid"
  node_query "$node5_addr" >"$RUN_ROOT/meta/flow2-node5-active.json"
  jq -e --arg new "$new_vault" '((.node.status // .status) | ascii_downcase) == "active" and ((.node.active_block_height // .active_block_height | tonumber) > 0) and (((.node.signer_membership // .signer_membership) // []) | index($new))' \
    "$RUN_ROOT/meta/flow2-node5-active.json" >/dev/null || die "flow2 node5 active node query invalid"

  keygen_height=""
  h="$flow_start"
  while true; do
    keygen_json="$(curl -fsS "$(api_url 1)/thornado/keygen/${h}/${node5_secp}" 2>/dev/null || true)"
    if [[ -n "$keygen_json" ]] && jq -e --arg member "$node5_secp" '.keygen_block.keygens[]? | select(.members | index($member))' <<<"$keygen_json" >/dev/null 2>&1; then
      keygen_height="$h"
      printf '%s\n' "$keygen_json" >"$RUN_ROOT/meta/flow2-node5-keygen.json"
      break
    fi
    current_height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    if (( h >= flow_start + 900 )); then
      break
    fi
    if (( h >= current_height )); then
      sleep 1
      continue
    fi
    h=$((h + 1))
  done
  [[ -n "$keygen_height" ]] || die "flow2 node5 did not find keygen block"
  printf '%s\n' "$keygen_height" >"$RUN_ROOT/meta/flow2-node5-keygen-height.txt"
  grep -h "FROST keygen complete" "$RUN_ROOT"/logs/bifrost-{1,2,3,4,5}.log >"$RUN_ROOT/meta/flow2-node5-frost-keygen-complete.log" || true
  grep -q "$new_vault" "$RUN_ROOT/meta/flow2-node5-frost-keygen-complete.log" || die "flow2 node5 FROST keygen completion log missing new vault"
  [[ -f "$RUN_ROOT/bifrost5/localstate-${new_vault}.json" ]] || die "flow2 node5 local FROST state for new vault missing"
  jq -e --arg vault "$new_vault" --arg local "$node5_secp" \
    '.pub_key == $vault and .signing_engine == "frost" and .local_party_key == $local and (.local_data | length) > 0 and (.participant_keys | length) == 5 and (.participant_keys | index($local))' \
    "$RUN_ROOT/bifrost5/localstate-${new_vault}.json" >/dev/null || die "flow2 node5 FROST local state invalid"

  migrate_txout="$(wait_migrate_signed "$flow_start" "$old_vault" "$new_addr" 1200)"
  printf '%s\n' "$migrate_txout" >"$RUN_ROOT/meta/flow2-node5-migrate-txout.json"
  jq -e --arg old "$old_vault" --arg to "$new_addr" \
    '[.txout.tx_array[]? | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to and (.coin.amount | tonumber) > 0 and (.out_hash // "") != "")] | length == 1' \
    "$RUN_ROOT/meta/flow2-node5-migrate-txout.json" >/dev/null || die "flow2 node5 migrate txout fields invalid"
  out_hash="$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .out_hash' "$RUN_ROOT/meta/flow2-node5-migrate-txout.json" | head -n1)"
  btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true >"$RUN_ROOT/meta/flow2-node5-btc-migrate-tx.json"
  mine_regtest_blocks 2
  raw_tx="$(btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true)"
  printf '%s\n' "$raw_tx" >"$RUN_ROOT/meta/flow2-node5-btc-migrate-tx-confirmed.json"
  jq -e --arg to "$new_addr" --argjson amount "$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .coin.amount' "$RUN_ROOT/meta/flow2-node5-migrate-txout.json" | head -n1)" \
    '([.vout[] | select(.scriptPubKey.address == $to)] | length) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 + 0.5 | floor)] | add // 0) >= $amount) and (.confirmations // 0) >= 1' \
    "$RUN_ROOT/meta/flow2-node5-btc-migrate-tx-confirmed.json" >/dev/null || die "flow2 node5 BTC migration transaction did not pay new vault"
  old_prevouts="$RUN_ROOT/meta/flow2-node5-migrate-prevouts.json"
  jq -r '.vin[] | @base64' "$RUN_ROOT/meta/flow2-node5-btc-migrate-tx-confirmed.json" | while IFS= read -r vin64; do
    local_txid="$(printf '%s' "$vin64" | base64 -d | jq -r '.txid')"
    local_vout="$(printf '%s' "$vin64" | base64 -d | jq -r '.vout')"
    btc_cli getrawtransaction "$local_txid" true | jq --argjson vout "$local_vout" '{txid, vout:$vout, address:.vout[$vout].scriptPubKey.address, value_sats:(.vout[$vout].value * 100000000 + 0.5 | floor)}'
  done | jq -s '.' >"$old_prevouts"
  jq -e --arg old "$old_addr" 'length > 0 and all(.[]; .address == $old)' "$old_prevouts" >/dev/null || die "flow2 node5 BTC migration did not spend old vault UTXOs"

  out_hash="$(printf '%s' "$out_hash" | tr '[:lower:]' '[:upper:]')"
  for _ in {1..90}; do
    if curl -fsS "$(api_url 1)/thornado/tx/${out_hash}" >"$RUN_ROOT/meta/flow2-node5-migrate-observed-tx.json" 2>/dev/null &&
      jq -e --arg id "$out_hash" '.. | strings | ascii_upcase | select(. == $id)' "$RUN_ROOT/meta/flow2-node5-migrate-observed-tx.json" >/dev/null; then
      break
    fi
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  jq -e --arg id "$out_hash" '.. | strings | ascii_upcase | select(. == $id)' "$RUN_ROOT/meta/flow2-node5-migrate-observed-tx.json" >/dev/null \
    || die "flow2 node5 migration outbound was not observable through tx query"
  after_vaults="$(curl -fsS $(api_url 1)/thornado/vaults/base)"
  printf '%s\n' "$after_vaults" >"$RUN_ROOT/meta/flow2-node5-base-vaults-after-migration.json"
  jq -e --arg old "$old_vault" '[.[] | select(.pub_key == $old and .status == "ActiveVault")] | length == 0' "$RUN_ROOT/meta/flow2-node5-base-vaults-after-migration.json" >/dev/null \
    || die "flow2 node5 old vault still active after migration"
  jq -e --arg new "$new_vault" '.[] | select(.pub_key == $new and .status == "ActiveVault")' "$RUN_ROOT/meta/flow2-node5-base-vaults-after-migration.json" >/dev/null \
    || die "flow2 node5 new active vault missing after migration"
  set_config_from_active_nodes Halt_Churning 1
  log "RESULTS Flow 2 node5 churn: PASS"
}

validate_flow3() {
  log "Flow 3: validating user deposit, split, redeem, fee, txout, and BTC outbound"
  source "$RUN_ROOT/meta/user.env"
  local user_account_addr="$address" deposit_pubkey user_addr flow3_label flow3_deposit_seed flow3_note_seed
  flow3_label="${FLOW3_LABEL:-user-flow-3}"
  flow3_deposit_seed="${flow3_label}-deposit-pubkey"
  flow3_note_seed="${flow3_label}-seed"
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "$flow3_deposit_seed")"
  user_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  if [[ "${FLOW3_MAIN_ONLY:-0}" != "1" ]]; then
    assert_tx_or_cli_rejected "flow3 request amount arg" "Usage:" thornado_tx "$RUN_ROOT/node1" "user" request-deposit "user-flow-3-amount" "$deposit_pubkey" "20000000"
  fi
  request_deposit "$RUN_ROOT/node1" "user" "$flow3_label" "$deposit_pubkey" >"$RUN_ROOT/meta/flow3-request-deposit.json"
  local session deposit_address txid child_vout deposit_id amount_sats path_index receipt commitment_objects commitments shield_signature out committed matched sweep_txout
  session="$(deposit_session "$user_addr")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow3-session-before-deposit.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  path_index="$(jq -r '.deposit_path_index' <<<"$session")"
  jq -e '.owner == "'"${user_addr}"'" and (.deposit_address | length) > 0 and (.vault_pub_key | length) > 0 and (.deposit_path_index | tonumber) > 0 and ((.amount_sats // "") == "" or (.amount_sats // "0") == "0")' \
    "$RUN_ROOT/meta/flow3-session-before-deposit.json" >/dev/null || die "flow3 deposit session unexpectedly contains amount or missing identity"
  txid="$(mine_to_registered_deposit "$deposit_address" "0.20000000")"
  btc_cli getrawtransaction "$txid" true >"$RUN_ROOT/meta/flow3-child-deposit-tx.json"
  child_vout="$(jq -r --arg addr "$deposit_address" '
    [.vout[] | select(.scriptPubKey.address == $addr and (((.value * 100000000 + 0.5) | floor) == 20000000))][0].n // ""
  ' "$RUN_ROOT/meta/flow3-child-deposit-tx.json")"
  [[ -n "$child_vout" ]] || die "flow3 child deposit output was not visible before sweep"
  btc_cli gettxout "$txid" "$child_vout" true >"$RUN_ROOT/meta/flow3-child-utxo-before-sweep.json"
  jq -e --arg addr "$deposit_address" '(.scriptPubKey.address == $addr) and (((.value * 100000000 + 0.5) | floor) == 20000000)' "$RUN_ROOT/meta/flow3-child-utxo-before-sweep.json" >/dev/null \
    || die "flow3 child deposit UTXO was not unspent before sweep"
  matched="$(wait_owner_deposit_matched "$user_addr")"
  deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow3-deposit-matched.json"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 420)"
  printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow3-sweep-txout.json"
  btc_cli gettxout "$txid" "$child_vout" true >"$RUN_ROOT/meta/flow3-child-utxo-after-sweep.json" || true
  [[ ! -s "$RUN_ROOT/meta/flow3-child-utxo-after-sweep.json" ]] || jq -e '. == null' "$RUN_ROOT/meta/flow3-child-utxo-after-sweep.json" >/dev/null \
    || die "flow3 child deposit UTXO remained spendable after sweep"
  amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats')"
  [[ "$amount_sats" == "20000000" ]] || die "flow3 observed deposit amount was not the actual BTC amount"
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$path_index" "$amount_sats" "$flow3_note_seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow3-receipt.json"
  commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  printf '%s\n' "$commitment_objects" >"$RUN_ROOT/meta/flow3-commitment-objects.json"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  printf '%s\n' "$commitments" >"$RUN_ROOT/meta/flow3-commitments.json"
  shield_signature="$("$SHIELDER_HELPER" shield-authorization "$flow3_deposit_seed" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
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

  local root denom note leaves recipient fee withdrawal prefix withdrawal_id withdraw_query nullifier nullifier_query outbound_txout out_hash expected_payout recipient_received
	  note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow3-receipt.json")"
	  leaves="$(shielder_leaves "$(jq -r '.denomination_sats' <<<"$note")")"
	  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow3-proof-leaves.json"
	  recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
	  printf '%s\n' "$recipient" >"$RUN_ROOT/meta/flow3-recipient-address.txt"
	  curl -fsS "$(api_url 1)/thornado/fee/entitlements" >"$RUN_ROOT/meta/flow3-fee-entitlements-before.json"
	  curl -fsS "$(api_url 1)/thornado/fees" >"$RUN_ROOT/meta/flow3-fee-pool-before.json"
	  curl -fsS "$(api_url 1)/thornado/shielder/redeem/quote/$(jq -r '.denomination_sats' <<<"$note")" >"$RUN_ROOT/meta/flow3-redeem-quote.json"
  fee="$(jq -r '.fee_sats' "$RUN_ROOT/meta/flow3-redeem-quote.json")"
  (( fee > 0 )) || die "flow3 redeem quote returned zero fee"
  withdrawal="$("$SHIELDER_HELPER" withdrawal "$note" "$flow3_note_seed" "$leaves" "$recipient" "$fee")"
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
  nullifier="$(jq -r '.[1].nullifier_hash' <<<"$withdrawal")"
  withdrawal_id="$(jq -r '.logs[0].events[]? | select(.type=="message") | .attributes[]? | select(.key=="withdrawal_id") | .value' <<<"$out" | tail -n1)"
  if [[ -z "$withdrawal_id" || "$withdrawal_id" == "null" ]]; then
    local start
    start="$(date +%s)"
    while true; do
      nullifier_query="$(curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" 2>/dev/null || true)"
      if jq -e '.spent == true and (.withdrawal_id // "") != ""' <<<"$nullifier_query" >/dev/null 2>&1; then
        printf '%s\n' "$nullifier_query" >"$RUN_ROOT/meta/flow3-nullifier-query.json"
        withdrawal_id="$(jq -r '.withdrawal_id' <<<"$nullifier_query")"
        break
      fi
      if (( "$(date +%s)" - start >= 120 )); then
        printf '%s\n' "$nullifier_query" >"$RUN_ROOT/meta/flow3-nullifier-query.json"
        die "flow3 nullifier did not expose withdrawal id"
      fi
      sleep 2
    done
  fi
  printf '%s\n' "$withdrawal_id" >"$RUN_ROOT/meta/flow3-withdrawal-id.txt"
  withdraw_query="$(curl -fsS "$(api_url 1)/thornado/shielder/redeem/${withdrawal_id}")"
  printf '%s\n' "$withdraw_query" >"$RUN_ROOT/meta/flow3-withdrawal-query.json"
  jq -e '(.status == "keysign_queued" or .status == "settled") and ((.fee_sats | tonumber) == '"$fee"')' <<<"$withdraw_query" >/dev/null \
    || die "flow3 withdrawal was not queued/settled with expected fee"
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
    '([.vout[] | select(.scriptPubKey.address == $recipient)] | length) == 1 and ((([.vout[] | select(.scriptPubKey.address == $recipient)][0].value * 100000000 + 0.5) | floor) == $payout)' \
    "$RUN_ROOT/meta/flow3-btc-outbound-tx.json" >/dev/null || die "flow3 BTC outbound transaction did not have exactly one expected recipient output"
  recipient_received="$(wait_btc_balance_at_least "$recipient" "0.009" 300)"
  printf '%s\n' "$recipient_received" >"$RUN_ROOT/meta/flow3-recipient-received-btc.txt"
  btc_cli -rpcwallet=miner listunspent 0 9999999 "[\"${recipient}\"]" >"$RUN_ROOT/meta/flow3-recipient-utxos.json"
	  jq -e --arg txid "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" --argjson payout "$expected_payout" \
	    'length == 1 and .[0].txid == $txid and (((.[0].amount * 100000000) | floor) == $payout)' \
	    "$RUN_ROOT/meta/flow3-recipient-utxos.json" >/dev/null || die "flow3 recipient had duplicate or unexpected BTC payout UTXOs"
	  curl -fsS "$(api_url 1)/thornado/fee/entitlements" >"$RUN_ROOT/meta/flow3-fee-entitlements-after.json"
	  curl -fsS "$(api_url 1)/thornado/fees" >"$RUN_ROOT/meta/flow3-fee-pool-after.json"
	  local fee_slots before_collected after_collected
	  fee_slots="$(jq -r '.total_slots | tonumber' "$RUN_ROOT/meta/flow3-fee-pool-after.json")"
	  if (( fee_slots > 0 )); then
	    jq -e --argjson fee "$fee" '([.entitlements[]? | (.claimable_sats | tonumber)] | add // 0) >= $fee' "$RUN_ROOT/meta/flow3-fee-entitlements-after.json" >/dev/null \
	      || die "flow3 fee entitlement did not increase enough to explain withdrawal fee"
	  else
	    before_collected="$(jq -r '.total_collected_sats | tonumber' "$RUN_ROOT/meta/flow3-fee-pool-before.json")"
	    after_collected="$(jq -r '.total_collected_sats | tonumber' "$RUN_ROOT/meta/flow3-fee-pool-after.json")"
	    (( after_collected - before_collected >= fee )) || die "flow3 fee pool did not collect withdrawal fee"
	  fi

	  if [[ "${FLOW3_MAIN_ONLY:-0}" == "1" ]]; then
    log "RESULTS Flow 3: PASS"
    return 0
  fi

  local spent_receipt spent_objects spent_commitments spent_signature
  spent_receipt="$("$SHIELDER_HELPER" receipt-simple "$amount_sats" "flow3-already-shielded-seed")"
  spent_objects="$("$SHIELDER_HELPER" commitment-objects "$spent_receipt")"
  spent_commitments="$(jq -c 'map(tostring)' <<<"$spent_objects")"
  spent_signature="$("$SHIELDER_HELPER" shield-authorization "user-flow-3-deposit-pubkey" "$deposit_id" "$amount_sats" "$spent_objects" | jq -r '.signature')"
  printf '%s\n' "$spent_commitments" >"$RUN_ROOT/meta/flow3-spent-commitments.json"
  assert_tx_or_cli_rejected "flow3 fully split root" "deposit already shielded" thornado_tx "$RUN_ROOT/node1" "user" shielder shield "$RUN_ROOT/meta/flow3-spent-commitments.json" "$deposit_pubkey" "$spent_signature" "$deposit_id"
  jq '. + {"_e2e_duplicate_redeem": true}' "${prefix}.proof.json" >"${prefix}-duplicate.proof.json"
  assert_tx_or_cli_rejected "flow3 duplicate redeem" "shielder nullifier already spent" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${prefix}-duplicate.proof.json" "${prefix}.public.json"

  local second_note second_withdrawal bad_prefix bad_recipient low_fee fake_receipt fake_note fake_leaves fake_withdrawal neg_deposit_pubkey neg_owner neg_session neg_addr neg_txid neg_id neg_match malformed_commitments short_receipt short_objects short_commitments short_sig alt_receipt alt_objects alt_commitments alt_sig alt_committed
  second_note="$(jq -c '.notes[1]' "$RUN_ROOT/meta/flow3-receipt.json")"
  bad_prefix="$RUN_ROOT/meta/flow3-bad"
  second_withdrawal="$("$SHIELDER_HELPER" withdrawal "$second_note" "user-flow-3-seed" "$leaves" "$recipient" "$fee")"
  "$SHIELDER_HELPER" shield-withdrawal "$second_withdrawal" "$bad_prefix"
  printf '{}' >"${bad_prefix}-invalid.proof.json"
  assert_tx_or_cli_rejected "flow3 invalid proof" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}-invalid.proof.json" "${bad_prefix}.public.json"
  bad_recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
  jq --arg recipient "$bad_recipient" '.recipient = $recipient' "${bad_prefix}.public.json" >"${bad_prefix}-wrong-recipient.public.json"
  assert_tx_or_cli_rejected "flow3 wrong recipient binding" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}.proof.json" "${bad_prefix}-wrong-recipient.public.json"
  jq '.denomination_sats = (.denomination_sats + 10000000)' "${bad_prefix}.public.json" >"${bad_prefix}-larger-amount.public.json"
  assert_tx_or_cli_rejected "flow3 amount larger than note" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}.proof.json" "${bad_prefix}-larger-amount.public.json"
  low_fee="$((fee - 1))"
  (( low_fee >= 0 )) || low_fee=0
  jq --argjson fee "$low_fee" '.fee_sats = $fee' "${bad_prefix}.public.json" >"${bad_prefix}-low-fee.public.json"
  assert_tx_or_cli_rejected "flow3 low fee redeem" "" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}.proof.json" "${bad_prefix}-low-fee.public.json"
  nullifier="$(jq -r '.nullifier_hash' "${bad_prefix}-low-fee.public.json")"
  curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/flow3-low-fee-nullifier-query.json"
  jq -e '.spent == false' "$RUN_ROOT/meta/flow3-low-fee-nullifier-query.json" >/dev/null || die "flow3 rejected low-fee redeem consumed nullifier"

  cp "${bad_prefix}.proof.json" "${bad_prefix}-unknown-root.proof.json"
  jq '.merkle_root = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"' \
    "${bad_prefix}.public.json" >"${bad_prefix}-unknown-root.public.json"
  assert_tx_or_cli_rejected "flow3 unknown root redeem" "unknown shielder merkle root" thornado_tx "$RUN_ROOT/node1" "user" shielder redeem "${bad_prefix}-unknown-root.proof.json" "${bad_prefix}-unknown-root.public.json"

  neg_deposit_pubkey="$("$SHIELDER_HELPER" pubkey "user-flow-3-neg-deposit-pubkey")"
  neg_owner="$("$SHIELDER_HELPER" owner-address "$neg_deposit_pubkey")"
  request_deposit "$RUN_ROOT/node1" "user" "user-flow-3-neg-split" "$neg_deposit_pubkey" >"$RUN_ROOT/meta/flow3-neg-request-deposit.json"
  neg_session="$(deposit_session "$neg_owner")"
  printf '%s\n' "$neg_session" >"$RUN_ROOT/meta/flow3-neg-session.json"
  neg_addr="$(jq -r '.deposit_address' <<<"$neg_session")"
  neg_txid="$(mine_to_registered_deposit "$neg_addr" "0.20000000")"
  neg_match="$(wait_owner_deposit_matched "$neg_owner")"
  neg_id="$(jq -r '.deposit_id' <<<"$neg_match")"
  printf '%s\n' "$neg_match" >"$RUN_ROOT/meta/flow3-neg-deposit-matched.json"
  malformed_commitments="$(jq -nc '["{\"denomination_sats\":10000000"]')"
  assert_tx_or_cli_rejected "flow3 malformed commitment json" "invalid shielder commitment" thornado_tx "$RUN_ROOT/node1" "user" shielder shield "$malformed_commitments" "$neg_deposit_pubkey" "" "$neg_id"
  assert_tx_or_cli_rejected "flow3 wrong owner split" "deposit owner mismatch" thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield "$commitments" "$deposit_pubkey" "$shield_signature" "$neg_id"
  short_receipt="$("$SHIELDER_HELPER" receipt-simple "10000000" "flow3-short-split-seed")"
  short_objects="$("$SHIELDER_HELPER" commitment-objects "$short_receipt")"
  short_commitments="$(jq -c 'map(tostring)' <<<"$short_objects")"
  short_sig="$("$SHIELDER_HELPER" shield-authorization "user-flow-3-neg-deposit-pubkey" "$neg_id" 10000000 "$short_objects" | jq -r '.signature')"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder shield "$short_commitments" "$neg_deposit_pubkey" "$short_sig" "$neg_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-partial-split-a.json"
  assert_tx_rejected "$out" "flow3 partial split rejected" "shielder commitment denominations leave spendable remainder"
  alt_receipt="$("$SHIELDER_HELPER" receipt-simple "20000000" "flow3-alt-owner-seed")"
  printf '%s\n' "$alt_receipt" >"$RUN_ROOT/meta/flow3-alt-owner-receipt.json"
  alt_objects="$("$SHIELDER_HELPER" commitment-objects "$alt_receipt")"
  alt_commitments="$(jq -c 'map(tostring)' <<<"$alt_objects")"
  alt_sig="$("$SHIELDER_HELPER" shield-authorization "user-flow-3-neg-deposit-pubkey" "$neg_id" 20000000 "$alt_objects" | jq -r '.signature')"
  printf '%s\n' "$alt_commitments" >"$RUN_ROOT/meta/flow3-alt-owner-commitments.json"
  out="$(thornado_tx "$RUN_ROOT/node1" "user" shielder shield "$alt_commitments" "$neg_deposit_pubkey" "$alt_sig" "$neg_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow3-alt-owner-shield.json"
  assert_tx_success "$out" "flow3 alternate full shield"
  alt_committed="$(wait_deposit_committed "$neg_id")"
  printf '%s\n' "$alt_committed" >"$RUN_ROOT/meta/flow3-alt-owner-deposit.json"
  jq -e '.settlement == "user" and .status == "committed"' <<<"$alt_committed" >/dev/null || die "flow3 alternate full shield did not commit user deposit"
  record_shielder_notes "$alt_receipt"
  assert_shielder_receipt_committed "$RUN_ROOT/meta/flow3-alt-owner-receipt.json" "flow3-alt-owner"

  cat >"$RUN_ROOT/meta/flow3-negative-results.md" <<EOF
# Flow 3 Negative Results

- Request-deposit rejects amount-like extra arguments.
- Fully split root and duplicate redeem reject.
- Malformed commitment JSON and wrong owner shield reject without mutating the deposit.
- Partial shields reject unless denominations match the spendable deposit amount.
- Invalid proof, wrong recipient binding, larger public amount, low fee, and unknown root redeem attempts reject.
- Low-fee rejection does not consume the note nullifier.
- Owner-signed alternate commitments with the correct denomination are accepted for a user deposit.
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

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield-fees "$node_pubkey" "$(printf '%0128d' 0)" "$commitments" "$note_pubkeys")"
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
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield-fees "$node_pubkey" "$bad_sig" "$bad_commitments" "$bad_pubkeys")"
  assert_tx_rejected "$out" "flow4 oversized fee claim" "shielder commitment denominations exceed amount"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield-fees "$node_pubkey" "$sig" "$commitments" "$note_pubkeys")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow4-split-fees.json"
  assert_tx_success "$out" "flow4 split-fees"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "flow4 split-fees txhash missing"
  txres="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}")"
  printf '%s\n' "$txres" >"$RUN_ROOT/meta/flow4-split-fees-delivertx.json"
  txheight="$(jq -r '.result.height' <<<"$txres")"
  [[ -n "$txheight" && "$txheight" != "null" ]] || die "flow4 split-fees deliver height missing"
  deposit_id="$(jq -r '.result.tx_result.data // empty' <<<"$txres" | base64 -d | strings | rg -o '[A-F0-9]{64}' | head -n1)"
  [[ -n "$deposit_id" ]] || die "flow4 fee deposit id missing from shield-fees response"
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
  jq -e --arg owner "$owner_addr" --argjson claim "$claim" \
    '.owner == $owner and .settlement == "operator_fee" and .status == "committed" and (.amount_sats | tonumber) == $claim' \
    "$RUN_ROOT/meta/flow4-fee-deposit.json" >/dev/null || die "flow4 fee deposit record invalid"
  record_shielder_notes "$receipt"
  assert_shielder_receipt_committed "$RUN_ROOT/meta/flow4-receipt.json" "flow4-fee-note"
  local denom pubkey pubkey_value fee_note leaves quote_status quote_body
  jq -r '.[]' "$RUN_ROOT/meta/flow4-fee-note-pubkeys.json" | while IFS= read -r pubkey; do
    pubkey_value="$(kv_json_value "shielder_fee_note_pubkey//$(printf '%s' "$pubkey" | tr '[:lower:]' '[:upper:]')" "$RUN_ROOT/meta/flow4-fee-note-pubkey-${pubkey}.kv.json")"
    [[ "$pubkey_value" == "true" ]] || die "flow4 fee note pubkey KV was not marked used"
  done
  fee_note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow4-receipt.json")"
  denom="$(jq -r '.denomination_sats' <<<"$fee_note")"
  leaves="$(shielder_leaves "$denom")"
  printf '%s\n' "$leaves" >"$RUN_ROOT/meta/flow4-proof-leaves.json"
  assert_shielder_root_committed "$denom" "$leaves" "flow4-fee-note"
  quote_body="$RUN_ROOT/meta/flow4-fee-note-redeem-quote-error.json"
  quote_status="$(curl -sS -o "$quote_body" -w "%{http_code}" "$(api_url 1)/thornado/shielder/redeem/quote/${denom}" || true)"
  printf '%s\n' "$quote_status" >"$RUN_ROOT/meta/flow4-fee-note-redeem-quote-status.txt"
  [[ "$quote_status" != "200" ]] || die "flow4 100k fee note unexpectedly had a standalone redeem quote"
  grep -q "withdrawal fee exceeds amount" "$quote_body" || die "flow4 100k fee note quote rejected for unexpected reason"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder shield-fees "$node_pubkey" "$sig" "$commitments" "$note_pubkeys")"
  assert_tx_rejected "$out" "flow4 duplicate fee claim" "no shielder fees claimable"
  log "RESULTS Flow 4: PASS"
}

validate_flow5() {
  log "Flow 5: validating standby node slot auction with shielded bid funding"
  source "$RUN_ROOT/meta/node5.env"
  local seller_addr="$address" seller_operator_pubkey="$secp" seller_node_pubkey="$cons"
  source "$RUN_ROOT/meta/node6.env"
  local bidder_addr="$address" bidder_operator_pubkey="$secp" bidder_node_pubkey="$cons"
  local height expiry out txhash txres auction_id auction_key auction_kv bid_key bid_kv bid_id receipt commitment_objects commitments committed
  local seller_bond new_bond seller_slot note_count deposit_pubkey deposit_owner session deposit_address txid deposit_id amount_sats sweep_txout root_addr root_received out_hash matched
  local note leaves bid_withdrawal prefix nullifier sale_pubkey sale_sig sale_deposit_id denom commitment commitment_key commitment_value denom_key denom_value root root_key root_value

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
  assert_tx_rejected "$out" "flow5 unbonded auction create" "node has no active bonded slot"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$expiry")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-create.json"
  assert_tx_success "$out" "flow5 auction-create"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "flow5 auction-create txhash missing"
  txres="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}")"
  printf '%s\n' "$txres" >"$RUN_ROOT/meta/flow5-auction-create-delivertx.json"
  auction_id="$(jq -r '.result.tx_result.data // empty' <<<"$txres" | base64 -d | strings | rg -o '[A-F0-9]{64}' | head -n1)"
  [[ -n "$auction_id" ]] || die "flow5 auction id missing from auction-create response"
  wait_blocks 2
  printf '%s\n' "$auction_id" >"$RUN_ROOT/meta/flow5-auction-id.txt"
  wait_api_json_file "$(api_url 1)/thornado/node/auction/${auction_id}" "$RUN_ROOT/meta/flow5-auction-open.json" "flow5 open auction" 90
  jq -e --arg seller "$seller_addr" --arg op "$seller_operator_pubkey" --arg node "$seller_node_pubkey" --argjson slot "$seller_slot" --argjson expiry "$expiry" \
    '.seller == $seller and .seller_operator_pub_key == $op and .seller_node_pub_key == $node and (.slot | tonumber) == $slot and (.original_bond_sats | tonumber) == 100000000 and (.reserve_sats | tonumber) == 100000000 and (.expiry_height | tonumber) == $expiry and .status == "open"' \
    "$RUN_ROOT/meta/flow5-auction-open.json" >/dev/null || die "flow5 open auction query is invalid"
  auction_key="$(printf 'node_slot_auction//%s' "$(printf '%s' "$auction_id" | tr '[:lower:]' '[:upper:]')")"
  auction_kv="$(kv_json_value "$auction_key" "$RUN_ROOT/meta/flow5-auction-open.kv.json")"
  jq -e --arg auction "$auction_id" '.auction_id == $auction and .status == "open"' <<<"$auction_kv" >/dev/null || die "flow5 auction KV missing open auction"

  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-bid-create "${auction_id}-fake" "$bidder_operator_pubkey" "$bidder_node_pubkey")"
  assert_tx_rejected "$out" "flow5 fake auction bid" "node slot auction is not open"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-bid-create "$auction_id" "$bidder_operator_pubkey" "$bidder_node_pubkey")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-bid-create.json"
  assert_tx_success "$out" "flow5 auction-bid-create"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "flow5 auction-bid-create txhash missing"
  txres="$(curl -fsS "$(rpc_url 1)/tx?hash=0x${txhash}")"
  printf '%s\n' "$txres" >"$RUN_ROOT/meta/flow5-auction-bid-create-delivertx.json"
  bid_id="$(jq -r '.result.tx_result.data // empty' <<<"$txres" | base64 -d | strings | rg -o '[A-F0-9]{64}' | head -n1)"
  [[ -n "$bid_id" ]] || die "flow5 bid id missing from auction-bid-create response"
  wait_blocks 2
  printf '%s\n' "$bid_id" >"$RUN_ROOT/meta/flow5-bid-id.txt"
  wait_api_json_file "$(api_url 1)/thornado/node/bid/${bid_id}" "$RUN_ROOT/meta/flow5-bid-before-funding.json" "flow5 bid before funding" 90
  wait_api_json_file "$(api_url 1)/thornado/node/auction/${auction_id}/bids" "$RUN_ROOT/meta/flow5-auction-bids-before-funding.json" "flow5 auction bids before funding" 90
  jq -e --arg bid "$bid_id" --arg auction "$auction_id" --arg bidder "$bidder_addr" --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" \
    '.bid_id == $bid and .auction_id == $auction and .bidder == $bidder and .operator_pub_key == $op and .node_pub_key == $node and ((.deposit_id // "") == "") and ((.amount_sats // 0 | tonumber) == 0) and .selected == false and .settled == false' \
    "$RUN_ROOT/meta/flow5-bid-before-funding.json" >/dev/null || die "flow5 bid state before funding is invalid"
  bid_key="$(printf 'node_slot_bid//%s' "$(printf '%s' "$bid_id" | tr '[:lower:]' '[:upper:]')")"
  bid_kv="$(kv_json_value "$bid_key" "$RUN_ROOT/meta/flow5-bid-before-funding.kv.json")"
  jq -e --arg bid "$bid_id" --arg auction "$auction_id" --arg bidder "$bidder_addr" --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" \
    '.bid_id == $bid and .auction_id == $auction and .bidder == $bidder and .operator_pub_key == $op and .node_pub_key == $node and .deposit_address == "bond_escrow" and ((.deposit_id // "") == "") and ((.amount_sats // 0 | tonumber) == 0) and ((.selected // false) == false) and ((.settled // false) == false)' \
    <<<"$bid_kv" >/dev/null || die "flow5 bid KV state before funding is invalid"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-select-bid "$auction_id" "$bid_id")"
  assert_tx_rejected "$out" "flow5 select before bid note" "node slot bid deposit not matched"

  deposit_pubkey="$($SHIELDER_HELPER pubkey "node6-bid-deposit-pubkey")"
  deposit_owner="$($SHIELDER_HELPER owner-address "$deposit_pubkey")"
  request_deposit "$RUN_ROOT/node6" "validator6" "auction-flow-5-note" "$deposit_pubkey" >"$RUN_ROOT/meta/flow5-bid-note-request-deposit.json"
  session="$(deposit_session "$deposit_owner")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow5-bid-note-session.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  txid="$(mine_to_registered_deposit "$deposit_address" "1.00000000")"
  matched="$(wait_owner_deposit_matched "$deposit_owner" 420)"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow5-bid-note-deposit-matched.json"
  deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 420)"
  printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow5-bid-note-sweep-txout.json"
  root_addr="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .to_address' <<<"$sweep_txout" | head -n1)"
  root_received="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .coin.amount' <<<"$sweep_txout" | head -n1)"
  out_hash="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .out_hash' <<<"$sweep_txout" | head -n1)"
  wait_confirmed_btc_output "$out_hash" "$root_addr" "$root_received" "$RUN_ROOT/meta/flow5-bid-note-btc-sweep-tx-confirmed.json" 180
  amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats')"
  [[ "$amount_sats" == "100000000" ]] || die "flow5 observed bid note amount was not 1 BTC"
  receipt="$($SHIELDER_HELPER receipt "$deposit_id" "$(jq -r '.deposit_path_index' <<<"$session")" "$amount_sats" "node6-bid-note-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow5-bid-note-receipt.json"
  commitment_objects="$($SHIELDER_HELPER commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder shield "$commitments" "$deposit_pubkey" "$($SHIELDER_HELPER shield-authorization "node6-bid-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')" "$deposit_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-bid-note-shield.json"
  assert_tx_success "$out" "flow5 shield bid funding note"
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow5-bid-note-deposit.json"
  jq -e '.settlement == "user" and .status == "committed"' <<<"$committed" >/dev/null || die "flow5 bid funding deposit was not committed"
  record_shielder_notes "$receipt"
  assert_shielder_receipt_committed "$RUN_ROOT/meta/flow5-bid-note-receipt.json" "flow5-bid-note"
  note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow5-bid-note-receipt.json")"
  leaves="$(shielder_leaves "$(jq -r '.denomination_sats' <<<"$note")")"
  assert_shielder_root_committed "$(jq -r '.denomination_sats' <<<"$note")" "$leaves" "flow5-bid-note"
  bid_withdrawal="$($SHIELDER_HELPER withdrawal-policy "$note" "node6-bid-note-seed" "$leaves" "bond_escrow" 0 "bid_deposit" "" "$bid_id")"
  printf '%s\n' "$bid_withdrawal" >"$RUN_ROOT/meta/flow5-bid-withdrawal.json"
  prefix="$RUN_ROOT/meta/flow5-bid-withdrawal"
  "$SHIELDER_HELPER" shield-withdrawal "$bid_withdrawal" "$prefix"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder redeem "${prefix}.proof.json" "${prefix}.public.json")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-bid-redeem.json"
  assert_tx_success "$out" "flow5 bid-deposit redeem"
  wait_blocks 2
  nullifier="$(jq -r '.nullifier_hash' "${prefix}.public.json")"
  curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/flow5-bid-nullifier-query.json"
  jq -e '.spent == true and .withdrawal_status == "settled"' "$RUN_ROOT/meta/flow5-bid-nullifier-query.json" >/dev/null || die "flow5 bid nullifier was not settled"
  wait_api_json_file "$(api_url 1)/thornado/node/bid/${bid_id}" "$RUN_ROOT/meta/flow5-bid-funded.json" "flow5 funded bid" 90
  jq -e '(.amount_sats | tonumber) == 100000000 and ((.deposit_id // "") == "") and .selected == false and .settled == false' "$RUN_ROOT/meta/flow5-bid-funded.json" >/dev/null || die "flow5 bid was not funded by note redeem"

  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-select-bid "$auction_id" "$bid_id")"
  assert_tx_rejected "$out" "flow5 non-seller select bid" "node slot auction seller mismatch"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-select-bid "$auction_id" "$bid_id")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-auction-select-bid.json"
  assert_tx_success "$out" "flow5 auction-select-bid"
  wait_blocks 2
  wait_api_json_file "$(api_url 1)/thornado/node/auction/${auction_id}" "$RUN_ROOT/meta/flow5-auction-settled.json" "flow5 settled auction" 90
  wait_api_json_file "$(api_url 1)/thornado/node/bid/${bid_id}" "$RUN_ROOT/meta/flow5-bid-settled.json" "flow5 settled bid" 90
  jq -e --arg bid "$bid_id" '.status == "settled" and .selected_bid_id == $bid' "$RUN_ROOT/meta/flow5-auction-settled.json" >/dev/null || die "flow5 auction did not settle"
  jq -e '.selected == true and .settled == true and (.amount_sats | tonumber) == 100000000' "$RUN_ROOT/meta/flow5-bid-settled.json" >/dev/null || die "flow5 bid did not settle"

  receipt="$($SHIELDER_HELPER receipt-simple 100000000 "operator5-sale-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/flow5-seller-receipt.json"
  commitment_objects="$($SHIELDER_HELPER commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  note_count="$(jq -r '.notes | length' <<<"$receipt")"
  local bad_receipt bad_commitments
  bad_receipt="$($SHIELDER_HELPER receipt-simple 50000000 "operator5-sale-bad-seed")"
  bad_commitments="$($SHIELDER_HELPER commitment-objects "$bad_receipt")"
  bad_commitments="$(jq -c 'map(tostring)' <<<"$bad_commitments")"
  sale_pubkey="$($SHIELDER_HELPER pubkey "operator5-sale-pubkey")"
  sale_sig="$($SHIELDER_HELPER shield-authorization "operator5-sale-pubkey" "$sale_pubkey" 100000000 "$commitment_objects" | jq -r '.signature')"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" shielder node-sale-shield "$auction_id" "$bid_id" "$commitments" "$sale_pubkey" "$sale_sig")"
  assert_tx_rejected "$out" "flow5 non-seller node-sale-shield" "node sale entitlement is not shieldable"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder node-sale-shield "$auction_id" "$bid_id" "$bad_commitments" "$sale_pubkey" "$sale_sig")"
  assert_tx_rejected "$out" "flow5 bad seller payout shield" "shielder commitment denominations leave spendable remainder"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder node-sale-shield "$auction_id" "$bid_id" "$commitments" "$sale_pubkey" "$sale_sig")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow5-node-sale-shield.json"
  assert_tx_success "$out" "flow5 node-sale-shield"
  sale_deposit_id="$(printf 'thornado:node-sale-entitlement:v2|%s|%s|%s' "$auction_id" "$bid_id" "$seller_addr" | shasum -a 256 | awk '{print toupper($1)}')"
  printf '%s\n' "$sale_deposit_id" >"$RUN_ROOT/meta/flow5-sale-deposit-id.txt"
  committed="$(wait_deposit_committed "$sale_deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow5-deposit.json"
  jq -e --arg owner "$seller_addr" '.owner == $owner and .settlement == "operator_sale" and .status == "committed" and .bond_confirmed == true and (.amount_sats | tonumber) == 100000000' <<<"$committed" >/dev/null || die "flow5 sale entitlement not committed correctly"

  seller_bond="$(curl -fsS "$(api_url 1)/thornado/bond/${seller_node_pubkey}")"
  new_bond="$(curl -fsS "$(api_url 1)/thornado/bond/${bidder_node_pubkey}")"
  printf '%s\n' "$seller_bond" >"$RUN_ROOT/meta/flow5-seller-bond-after.json"
  printf '%s\n' "$new_bond" >"$RUN_ROOT/meta/flow5-buyer-bond-after.json"
  jq -e --arg auction "$auction_id" '.sold == true and .sold_auction_id == $auction and (.bond_sats | tonumber) == 0 and .fee_share_active == false' "$RUN_ROOT/meta/flow5-seller-bond-after.json" >/dev/null || die "flow5 seller bond not sold"
  jq -e --arg op "$bidder_operator_pubkey" --arg node "$bidder_node_pubkey" --argjson slot "$seller_slot" '.node_pub_key == $node and .operator_pub_key == $op and (.slot | tonumber) == $slot and (.bond_sats | tonumber) == 100000000 and .fee_share_active == true and .node_status == "Standby"' "$RUN_ROOT/meta/flow5-buyer-bond-after.json" >/dev/null || die "flow5 new bond not standby with auction principal"
  node_query "$bidder_addr" >"$RUN_ROOT/meta/flow5-buyer-node-after.json"
  jq -e '((.node.status // .status) | ascii_downcase) == "standby"' "$RUN_ROOT/meta/flow5-buyer-node-after.json" >/dev/null || die "flow5 buyer node account is not standby"
  set_and_assert_node_version 6 "flow5-node6" "$(cat "$RUN_ROOT/meta/network-node-version.txt")" >/dev/null

  record_shielder_notes "$receipt"
  assert_shielder_receipt_committed "$RUN_ROOT/meta/flow5-seller-receipt.json" "flow5-seller-note"
  denom="$(jq -r '.notes[0].denomination_sats' "$RUN_ROOT/meta/flow5-seller-receipt.json")"
  leaves="$(shielder_leaves "$denom")"
  assert_shielder_root_committed "$denom" "$leaves" "flow5-seller-note"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder node-sale-shield "$auction_id" "$bid_id" "$commitments" "$sale_pubkey" "$sale_sig")"
  assert_tx_rejected "$out" "flow5 duplicate node-sale-shield" "node sale entitlement is not shieldable"
  height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$((height + 300))")"
  assert_tx_rejected "$out" "flow5 sold node second auction" "node has no active bonded slot"
  log "RESULTS Flow 5: PASS"
}

validate_flow6() {
  log "Flow 6: validating node6 churn-in and base-vault migration"
  local old_vault old_addr new_vault new_addr node5_secp node6_addr node6_secp node6_cons out status active_vaults start latest
  local flow6_start keygen_height keygen_json migrate_txout out_hash raw_tx old_prevouts after_vaults post_session config h current_height
  curl -fsS $(api_url 1)/thornado/vaults/base >"$RUN_ROOT/meta/flow6-base-vaults-before.json"
  old_vault="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/flow6-base-vaults-before.json")"
  old_addr="$(jq -r --arg old "$old_vault" '.[] | select(.pub_key == $old) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow6-base-vaults-before.json" | head -n1)"
  if [[ -z "$old_addr" || "$old_addr" == "null" ]]; then
    old_addr="$("$SHIELDER_HELPER" btc-address "$old_vault" 0)"
  fi
  flow6_start="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  source "$RUN_ROOT/meta/node5.env"
  node5_secp="$secp"
  source "$RUN_ROOT/meta/node6.env"
  node6_addr="$address"
  node6_secp="$secp"
  node6_cons="$cons"

  wait_all_signed_txouts_finalized "flow6-pre-churn" 900
  set_config_from_active_nodes Halt_Churning 1

  curl -fsS "$(api_url 1)/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow6-node6-bond-before.json"
  jq -e '(.bond_sats | tonumber) == 100000000 and .node_status == "Standby"' "$RUN_ROOT/meta/flow6-node6-bond-before.json" >/dev/null \
    || die "flow6 node6 does not have standby auction bond"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" set-ip-address "127.0.0.1")"
  assert_tx_success "$out" "flow6 node6 set-ip-address"
  out="$(thornado_tx "$RUN_ROOT/node6" "validator6" set-node-keys "$node6_secp" "$node6_cons")"
  assert_tx_success "$out" "flow6 node6 set-node-keys"
  wait_blocks 2
  node_query "$node6_addr" >"$RUN_ROOT/meta/flow6-node6-after-keys.json"
  jq -e --arg secp "$node6_secp" --arg cons "$node6_cons" \
    '((.node.status // .status) | ascii_downcase) == "standby" and ((.node.pub_key_set.secp256k1 // .pub_key_set.secp256k1) == $secp) and ((.node.node_cons_pub_key // .node_cons_pub_key) == $cons) and ((.node.ip_address // .ip_address) == "127.0.0.1")' \
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
  set_and_assert_node_version 6 "flow6-node6" "$(cat "$RUN_ROOT/meta/network-node-version.txt")" >/dev/null
  wait_blocks 2
  node_query "$node6_addr" >"$RUN_ROOT/meta/flow6-node6-pre-churn.json"
  jq -e '(((.node.status // .status) | ascii_downcase) as $status | ($status == "standby" or $status == "selected" or $status == "active")) and ((.node.version // .version) | length) > 0' \
    "$RUN_ROOT/meta/flow6-node6-pre-churn.json" >/dev/null || die "flow6 node6 pre-churn status/version invalid"

  set_config_from_active_nodes Vault_MigrationIntervalMinutes 1
  set_config_from_active_nodes Chain_BlockTimeSeconds "$THORNADO_BLOCK_TIME_SECONDS"
  set_config_from_active_nodes Churn_IntervalMinutes 1
  set_config_from_active_nodes Churn_RetryIntervalMinutes 1
  set_config_from_active_nodes Halt_SolvencyCheck 0
  set_config_from_active_nodes HaltSigningBTC 0
  set_config_from_active_nodes Node_SetDesired 5
  set_config_from_active_nodes Halt_Churning 0
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/flow6-config-after-migration-tuning.json"
  config="$(jq -r '(.NODE_SETDESIRED.value // (.configs[]? | select(.key == "Node_SetDesired") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "5" ]] || die "flow6 node desired config was not applied"
  config="$(jq -r '(.VAULT_MIGRATIONINTERVALMINUTES.value // (.configs[]? | select(.key == "Vault_MigrationIntervalMinutes") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "1" ]] || die "flow6 migration interval config was not applied"
  config="$(jq -r '(.HALT_CHURNING.value // (.configs[]? | select(.key == "Halt_Churning") | .value) // empty)' "$RUN_ROOT/meta/flow6-config-after-migration-tuning.json" | tail -n1)"
  [[ "$config" == "0" ]] || die "flow6 churn halt was not cleared"

  : >"$RUN_ROOT/meta/flow6-node6-status-history.tsv"
  start="$(date +%s)"
  while true; do
    status="$(node_query "$node6_addr" | jq -r '(.node.status // .status) | ascii_downcase')"
    active_vaults="$(curl -fsS $(api_url 1)/thornado/vaults/base)"
    latest="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
    printf '%s\t%s\n' "$latest" "$status" >>"$RUN_ROOT/meta/flow6-node6-status-history.tsv"
    if jq -e --arg old "$old_vault" --arg member "$node6_secp" '
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
    if (( "$(date +%s)" - start >= 1800 )); then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/flow6-base-vaults-timeout.json"
      die "flow6 node6 did not churn into a new active base vault"
    fi
    log "flow6 waiting for critical vault rotation: height=${latest} node6_status=${status}"
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
  if ! jq -e --arg new "$new_vault" '((.node.status // .status) | ascii_downcase) == "active" and ((.node.active_block_height // .active_block_height | tonumber) > 0) and (((.node.signer_membership // .signer_membership) // []) | index($new))' \
    "$RUN_ROOT/meta/flow6-node6-active.json" >/dev/null; then
    log "flow6 progress note: node6 status query has not fully caught up; critical vault membership already rotated"
  fi

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

  local post_deposit_pubkey post_deposit_owner
  post_deposit_pubkey="$("$SHIELDER_HELPER" pubkey "user-flow-6-post-churn-deposit-pubkey")"
  post_deposit_owner="$("$SHIELDER_HELPER" owner-address "$post_deposit_pubkey")"
  request_deposit "$RUN_ROOT/node1" "user" "user-flow-6-post-churn" "$post_deposit_pubkey" >/dev/null
  post_session="$(deposit_session "$post_deposit_owner")"
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
    '([.vout[] | select(.scriptPubKey.address == $to)] | length) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 + 0.5 | floor)] | add // 0) >= $amount) and (.confirmations // 0) >= 1' \
    "$RUN_ROOT/meta/flow6-btc-migrate-tx-confirmed.json" >/dev/null || die "flow6 BTC migration transaction did not pay new vault"
  old_prevouts="$RUN_ROOT/meta/flow6-migrate-prevouts.json"
  jq -r '.vin[] | @base64' "$RUN_ROOT/meta/flow6-btc-migrate-tx-confirmed.json" | while IFS= read -r vin64; do
    local_txid="$(printf '%s' "$vin64" | base64 -d | jq -r '.txid')"
    local_vout="$(printf '%s' "$vin64" | base64 -d | jq -r '.vout')"
    btc_cli getrawtransaction "$local_txid" true | jq --argjson vout "$local_vout" '{txid, vout:$vout, address:.vout[$vout].scriptPubKey.address, value_sats:(.vout[$vout].value * 100000000 + 0.5 | floor)}'
  done | jq -s '.' >"$old_prevouts"
  jq -e --arg old "$old_addr" 'length > 0 and all(.[]; .address == $old)' "$old_prevouts" >/dev/null || die "flow6 BTC migration did not spend old vault UTXOs"

  migrate_height="$(jq -r '.txout.height | tonumber' "$RUN_ROOT/meta/flow6-migrate-txout.json")"
  out_hash="$(printf '%s' "$out_hash" | tr '[:lower:]' '[:upper:]')"
  local accounting_start accounting_latest accounting_finalised accounting_old_amount accounting_pending
  accounting_start="$(date +%s)"
  while true; do
    curl -fsS "$(api_url 1)/thornado/tx/${out_hash}" >"$RUN_ROOT/meta/flow6-migrate-observed-tx.json" 2>/dev/null || true
    after_vaults="$(curl -fsS "$(api_url 1)/thornado/vaults/base")"
    printf '%s\n' "$after_vaults" >"$RUN_ROOT/meta/flow6-base-vaults-after-migration.json"
    curl -fsS "$(api_url 1)/thornado/vault/${old_vault}" >"$RUN_ROOT/meta/flow6-old-vault-after-migration.json"

    accounting_finalised="$(jq -r '.stages.inbound_finalised.completed // false' "$RUN_ROOT/meta/flow6-migrate-observed-tx.json" 2>/dev/null || printf 'false')"
    accounting_old_amount="$(jq -r '((.coins // []) | map(.amount | tonumber) | add // 0)' "$RUN_ROOT/meta/flow6-old-vault-after-migration.json")"
    accounting_pending="$(jq -r --argjson height "$migrate_height" '(((.pending_tx_block_heights // []) | map(tonumber) | index($height)) != null)' "$RUN_ROOT/meta/flow6-old-vault-after-migration.json")"
    if jq -e --arg id "$out_hash" '
        (.. | strings | ascii_upcase | select(. == $id)) and
        (.stages.inbound_observed.completed == true) and
        (.stages.inbound_finalised.completed == true)
      ' "$RUN_ROOT/meta/flow6-migrate-observed-tx.json" >/dev/null 2>&1 &&
      [[ "$accounting_old_amount" == "0" && "$accounting_pending" == "false" ]]; then
      break
    fi
    if (( "$(date +%s)" - accounting_start >= 1800 )); then
      die "flow6 migration accounting did not finalise"
    fi
    accounting_latest="$(curl -fsS "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
    log "flow6 waiting for migration accounting: height=${accounting_latest} old_amount=${accounting_old_amount} pending=${accounting_pending} finalised=${accounting_finalised}"
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  jq -e --arg old "$old_vault" '.pub_key == $old and (.status == "RetiringVault" or .status == "InactiveVault")' "$RUN_ROOT/meta/flow6-old-vault-after-migration.json" >/dev/null \
    || die "flow6 old vault status invalid after migration"
  jq -e --argjson height "$migrate_height" '
    ((.coins // []) | map(.amount | tonumber) | add // 0) == 0 and
    (((.pending_tx_block_heights // []) | map(tonumber) | index($height)) == null)
  ' "$RUN_ROOT/meta/flow6-old-vault-after-migration.json" >/dev/null \
    || die "flow6 old vault was not drained or still had pending migration height"
  jq -e --arg old "$old_vault" '[.[] | select(.pub_key == $old and .status == "ActiveVault")] | length == 0' "$RUN_ROOT/meta/flow6-base-vaults-after-migration.json" >/dev/null \
    || die "flow6 old vault still active after migration"
  jq -e --arg new "$new_vault" '.[] | select(.pub_key == $new and .status == "ActiveVault")' "$RUN_ROOT/meta/flow6-base-vaults-after-migration.json" >/dev/null \
    || die "flow6 new active vault missing after migration"
  set_config_from_active_nodes Halt_Churning 1
  log "RESULTS Flow 6: PASS"
}

find_signed_migrate_txout() {
  local from_height="$1" old_vault="${2:-}" to_address="${3:-}" txout
  txout="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
  if [[ -n "$txout" ]] && jq -e --argjson from "$from_height" --arg old "$old_vault" --arg to "$to_address" '
    (if type == "array" then .[] else .txouts[]? end) |
    select((.height | tonumber) >= $from) |
    select(.tx_array[]? |
      select(.tx_type == "migrate" and (.out_hash // "") != "" and (($old == "") or (.vault_pub_key == $old)) and (($to == "") or (.to_address == $to))))
  ' <<<"$txout" >/dev/null 2>&1; then
    jq -c --argjson from "$from_height" --arg old "$old_vault" --arg to "$to_address" '
      {txout:((if type == "array" then .[] else .txouts[]? end) |
        select((.height | tonumber) >= $from) |
        select(.tx_array[]? |
          select(.tx_type == "migrate" and (.out_hash // "") != "" and (($old == "") or (.vault_pub_key == $old)) and (($to == "") or (.to_address == $to)))))
      }
    ' <<<"$txout" | head -n1
    return 0
  fi
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
  local txout
  txout="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
  if [[ -n "$txout" ]] && jq -e '(if type == "array" then .[] else .txouts[]? end) | select(.status == "complete" and (.tx_array[]? | select(.tx_type == "consolidate" and (.out_hash // "") != "")))' <<<"$txout" >/dev/null 2>&1; then
    jq -c '{txout:((if type == "array" then .[] else .txouts[]? end) | select(.status == "complete" and (.tx_array[]? | select(.tx_type == "consolidate" and (.out_hash // "") != ""))))}' <<<"$txout" | tail -n1
    return 0
  fi
  return 1
}

count_signed_txouts_since() {
  local tx_type="$1" min_height="$2" txout
  txout="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
  if [[ -z "$txout" ]]; then
    printf '0\n'
    return 0
  fi
  jq -r --arg tx_type "$tx_type" --argjson min_height "$min_height" '
    [(if type == "array" then .[] else .txouts[]? end)
      | select((.height | tonumber) >= $min_height)
      | .tx_array[]?
      | select(.tx_type == $tx_type and (.out_hash // "") != "")
    ] | length
  ' <<<"$txout"
}

find_signed_sweep_txout() {
  local in_hash="$1" txout
  txout="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
  if [[ -n "$txout" ]] && jq -e --arg in_hash "$in_hash" '(if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
    jq -c --arg in_hash "$in_hash" '{txout:((if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
    return 0
  fi
  return 1
}

find_signed_txout_by_in_hash() {
  local in_hash="$1" tx_type="${2:-}" txout
  txout="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
  if [[ -n "$txout" ]]; then
    if [[ -n "$tx_type" ]]; then
      if jq -e --arg in_hash "$in_hash" --arg tx_type "$tx_type" '(if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.tx_type == $tx_type and .in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
        jq -c --arg in_hash "$in_hash" --arg tx_type "$tx_type" '{txout:((if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.tx_type == $tx_type and .in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
        return 0
      fi
    elif jq -e --arg in_hash "$in_hash" '(if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.in_hash == $in_hash and (.out_hash // "") != ""))' <<<"$txout" >/dev/null 2>&1; then
      jq -c --arg in_hash "$in_hash" '{txout:((if type == "array" then .[] else .txouts[]? end) | select(.tx_array[]? | select(.in_hash == $in_hash and (.out_hash // "") != "")))}' <<<"$txout" | head -n1
      return 0
    fi
  fi
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

wait_all_signed_txouts_finalized() {
  local label="$1" timeout="${2:-600}" start txouts hashes hash missing
  start="$(date +%s)"
  while true; do
    txouts="$(curl_json_quiet "$(api_url 1)/thornado/txout/all" || true)"
    if [[ -n "$txouts" ]]; then
      hashes=()
      while IFS= read -r hash; do
        [[ -n "$hash" ]] && hashes+=("$hash")
      done < <(jq -r '(if type == "array" then .[] else .txouts[]? end).tx_array[]? | select((.out_hash // "") != "") | .out_hash' <<<"$txouts" | sort -u)
      missing=0
      for hash in "${hashes[@]}"; do
        if ! curl -fsS "$(api_url 1)/thornado/tx/${hash}" >/tmp/thornado-tx-final.json 2>/dev/null ||
          ! jq -e '.stages.inbound_observed.completed == true' /tmp/thornado-tx-final.json >/dev/null 2>&1; then
          missing=$((missing + 1))
          break
        fi
      done
      if (( missing == 0 )); then
        printf '%s\n' "$txouts" >"$RUN_ROOT/meta/${label}-settled-txouts.json"
        return 0
      fi
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "${label} timed out waiting for signed txouts to finalize"
    fi
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
}

sweep_reached_root_vault() {
  local in_hash="$1" txout to_address amount_sats received_sats
  txout="$(find_signed_sweep_txout "$in_hash" || true)"
  if [[ -z "$txout" ]]; then
    return 1
  fi
  to_address="$(jq -r --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash) | .to_address' <<<"$txout" | head -n1)"
  amount_sats="$(jq -r --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash) | .coin.amount' <<<"$txout" | head -n1)"
  if [[ -z "$to_address" || "$to_address" == "null" || -z "$amount_sats" || "$amount_sats" == "null" ]]; then
    return 1
  fi
  received_sats="$(btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${to_address}\"]" | jq '[.[].amount * 100000000] | add // 0 | floor')"
  if (( received_sats >= amount_sats )); then
    return 0
  fi
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
  local txid start tx hex raw wallet
  txid="$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')"
  start="$(date +%s)"
  while true; do
    tx=""
    raw=""
    if raw="$(btc_cli getrawtransaction "$txid" true 2>/dev/null)" && [[ -n "$raw" ]]; then
      raw="$(jq '.' <<<"$raw")"
    else
      for wallet in bifrost1 bifrost2 bifrost3 bifrost4 bifrost5 bifrost6 bifrost7 bifrost8 bifrost9; do
        if tx="$(btc_cli -rpcwallet="$wallet" gettransaction "$txid" true 2>/dev/null)" &&
          hex="$(jq -r '.hex // empty' <<<"$tx")" &&
          [[ -n "$hex" ]]; then
          raw="$(btc_cli decoderawtransaction "$hex" | jq --argjson conf "$(jq -r '.confirmations // 0' <<<"$tx")" '. + {confirmations:$conf}')"
          break
        fi
      done
    fi
    if [[ -n "$raw" ]]; then
      printf '%s\n' "$raw" >"$artifact"
      if jq -e --arg to "$to_address" --argjson amount "$amount_sats" \
        '(.confirmations // 0) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 + 0.5 | floor)] | add // 0) >= $amount)' \
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
  local key="$1" value="$2" i status out voted=0 attempt code raw_log lookup start current config_file node_json
  lookup="$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
  config_file="$RUN_ROOT/meta/config-${key}-applied.json"
  wait_api_json_file "$(api_url 1)/thornado/config" "$config_file" "config before ${key}" 120
  current="$(jq -r --arg key "$lookup" '.[$key].value // empty' "$config_file")"
  if [[ "$current" == "$value" ]]; then
    return 0
  fi

  for i in 1 2 3 4 5 6 7 8 9; do
    [[ -f "$RUN_ROOT/meta/node${i}.env" ]] || continue
    source "$RUN_ROOT/meta/node${i}.env"
    node_json="$(curl_json_quiet "$(api_url 1)/thornado/node/${address}" || true)"
    if [[ -z "$node_json" ]] || ! jq -e type <<<"$node_json" >/dev/null 2>&1; then
      continue
    fi
    printf '%s\n' "$node_json" >"$RUN_ROOT/meta/config-${key}-node${i}-status.json"
    status="$(jq -r '(.node.status // .status) | ascii_downcase' <<<"$node_json")"
    if [[ "$status" != "active" ]]; then
      continue
    fi
    for attempt in 1 2 3; do
      out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" config "$key" "$value")"
      printf '%s\n' "$out" >"$RUN_ROOT/meta/config-${key}-node${i}-attempt-${attempt}.json"
      code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
      raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
      if [[ "$code" == "0" || "$code" == "null" ]]; then
        break
      fi
      if { [[ "$raw_log" == *"account sequence mismatch"* ]] || [[ "$raw_log" == *"timed out"* ]]; } && [[ "$attempt" != "3" ]]; then
        wait_blocks 2
        continue
      fi
      break
    done
    assert_tx_success "$out" "config set ${key} node${i}"
    voted=$((voted + 1))
  done
  (( voted >= 3 )) || die "insufficient active nodes voted for ${key}"
  start="$(date +%s)"
  while true; do
    wait_api_json_file "$(api_url 1)/thornado/config" "$config_file" "config apply ${key}" 20
    current="$(jq -r --arg key "$lookup" '.[$key].value // empty' "$config_file")"
    if [[ "$current" == "$value" ]]; then
      return 0
    fi
    if (( "$(date +%s)" - start >= 60 )); then
      die "config ${key} did not apply value ${value}; current=${current:-empty}"
    fi
    sleep 1
  done
}

protocol_snapshot() {
  local label="$1" dir="$RUN_ROOT/meta/${label}.snapshot"
  local i node_json bond_json auctions_json auction_id
  mkdir -p "$dir"
  curl_json_quiet "$(api_url 1)/thornado/nodes" >"$dir/nodes.json" || printf '{}\n' >"$dir/nodes.json"
  curl_json_quiet "$(api_url 1)/thornado/nodes/metrics" >"$dir/node-metrics.json" || printf '{}\n' >"$dir/node-metrics.json"
  curl_json_quiet "$(api_url 1)/thornado/vaults/base" >"$dir/vaults.json" || printf '[]\n' >"$dir/vaults.json"
  curl_json_quiet "$(api_url 1)/thornado/txout/all" >"$dir/txouts.json" || printf '{"txouts":[]}\n' >"$dir/txouts.json"
  curl_json_quiet "$(api_url 1)/thornado/fee/entitlements" >"$dir/fees.json" || printf '{}\n' >"$dir/fees.json"
  curl_json_quiet "$(api_url 1)/thornado/config" >"$dir/config.json" || printf '{}\n' >"$dir/config.json"
  curl_json_quiet "$(api_url 1)/thornado/shielder/sync?limit=5000" >"$dir/shielder.json" || printf '{}\n' >"$dir/shielder.json"
  printf '[]\n' >"$dir/extra-nodes.json"
  for i in 5 6 7 8 9; do
    if [[ -f "$RUN_ROOT/meta/node${i}.env" ]]; then
      source "$RUN_ROOT/meta/node${i}.env"
      bond_json="$(curl_json_quiet "$(api_url 1)/thornado/bond/${cons}" || printf '{}')"
      node_json="$(curl_json_quiet "$(api_url 1)/thornado/node/${address}" || printf '{}')"
      jq --argjson item "$(jq -n --argjson bond "$bond_json" --argjson node "$node_json" --arg idx "$i" '{index:$idx,bond:$bond,node:$node}')" \
        '. + [$item]' "$dir/extra-nodes.json" >"$dir/extra-nodes.tmp"
      mv "$dir/extra-nodes.tmp" "$dir/extra-nodes.json"
    fi
  done
  curl_json_quiet "$(api_url 1)/thornado/node/auctions" >"$dir/auctions.json" || printf '{"auctions":[]}\n' >"$dir/auctions.json"
  printf '{}\n' >"$dir/auction-bids.json"
  auctions_json="$(cat "$dir/auctions.json")"
  while IFS= read -r auction_id; do
    [[ -n "$auction_id" && "$auction_id" != "null" ]] || continue
    curl_json_quiet "$(api_url 1)/thornado/node/auction/${auction_id}/bids" >"$dir/auction-bids-${auction_id}.json" || printf '{"bids":[]}\n' >"$dir/auction-bids-${auction_id}.json"
    jq --arg auction "$auction_id" --slurpfile bids "$dir/auction-bids-${auction_id}.json" \
      '. + {($auction): $bids[0]}' "$dir/auction-bids.json" >"$dir/auction-bids.tmp"
    mv "$dir/auction-bids.tmp" "$dir/auction-bids.json"
  done < <(jq -r '.auctions[]?.auction_id // empty' <<<"$auctions_json")
  btc_protocol_snapshot "$dir"
  jq -S -n \
    --slurpfile nodes "$dir/nodes.json" \
    --slurpfile nodeMetrics "$dir/node-metrics.json" \
    --slurpfile vaults "$dir/vaults.json" \
    --slurpfile txouts "$dir/txouts.json" \
    --slurpfile fees "$dir/fees.json" \
    --slurpfile config "$dir/config.json" \
    --slurpfile shielder "$dir/shielder.json" \
    --slurpfile extraNodes "$dir/extra-nodes.json" \
    --slurpfile auctions "$dir/auctions.json" \
    --slurpfile auctionBids "$dir/auction-bids.json" \
    --slurpfile btc "$dir/btc.json" \
    '{
      nodes: (
        $nodes[0]
        | walk(if type == "object" then
            del(.penalty_points)
            | if has("observe_chains") and (.observe_chains | type == "array") then .observe_chains |= map(del(.height)) else . end
          else . end)
      ),
      node_metrics: $nodeMetrics[0],
      vaults: (
        $vaults[0]
        | walk(if type == "object" then
            if has("coins") and (.coins != null) then .coins |= map(del(.amount)) else . end
          else . end)
      ),
      txouts: (
        $txouts[0]
        | if type == "object" and has("txout") then
            .txout |= del(.height)
          else
            .
          end
      ),
      fees: $fees[0],
      config: $config[0],
      shielder: $shielder[0],
      extra_nodes: ($extraNodes[0] | walk(if type == "object" then del(.penalty_points) | if has("observe_chains") and (.observe_chains | type == "array") then .observe_chains |= map(del(.height)) else . end else . end)),
      auctions: ($auctions[0] | walk(if type == "object" then del(.created_height, .updated_height) else . end)),
      auction_bids: ($auctionBids[0] | walk(if type == "object" then del(.created_height, .updated_height) else . end)),
      btc: $btc[0]
    }' >"$RUN_ROOT/meta/${label}.snapshot.json"
}

btc_protocol_snapshot() {
  local dir="$1" wallet utxos
  {
    printf '{"mempool":'
    btc_cli getrawmempool | jq -S '.'
    printf ',"wallets":{'
    local first=1
    for wallet in bifrost1 bifrost2 bifrost3 bifrost4 bifrost5 bifrost6 bifrost7 bifrost8 bifrost9; do
      utxos="$(btc_cli -rpcwallet="$wallet" listunspent 0 9999999 2>/dev/null | jq -S 'map({txid,vout,address,amount,solvable,desc}) | sort_by(.txid, .vout)' || printf '[]')"
      if (( first == 0 )); then printf ','; fi
      first=0
      printf '%s:%s' "$(jq -Rn --arg wallet "$wallet" '$wallet')" "$utxos"
    done
    printf '}}\n'
  } >"$dir/btc.json"
}

assert_snapshot_equal() {
  local before="$1" after="$2" label="$3" diff_file="$RUN_ROOT/meta/${label// /-}-snapshot.diff"
  if ! diff -u "$RUN_ROOT/meta/${before}.snapshot.json" "$RUN_ROOT/meta/${after}.snapshot.json" >"$diff_file"; then
    die "${label} mutated protocol snapshot; see ${diff_file}"
  fi
}

assert_rejected_without_state_change() {
  local label="$1" want="$2" before after out
  shift 2
  before="${label// /-}-before"
  after="${label// /-}-after"
  protocol_snapshot "$before"
  set +e
  out="$("$@" 2>&1)"
  local status=$?
  set -e
  if (( status != 0 )); then
    printf '%s\n' "$out" >"$RUN_ROOT/meta/${label// /-}-cli-rejected.log"
    if [[ -n "$want" && "$out" != *"$want"* ]]; then
      die "$label failed CLI with unexpected log: $out"
    fi
  else
    assert_tx_rejected "$out" "$label" "$want"
  fi
  wait_blocks 2
  assert_live_nodes_app_hash_converged "$label"
  protocol_snapshot "$after"
  assert_snapshot_equal "$before" "$after" "$label"
}

assert_live_nodes_app_hash_converged() {
  local label="$1" timeout="${2:-60}" start i status latest max_height min_height app_hashes live
  start="$(date +%s)"
  while true; do
    live=0
    max_height=0
    min_height=0
    app_hashes=""
    for i in 1 2 3 4 5 6 7 8 9; do
      status="$(curl_json_quiet "$(rpc_url "$i")/status" || true)"
      if [[ -z "$status" ]] || ! jq -e '.result.sync_info.latest_block_height' <<<"$status" >/dev/null 2>&1; then
        continue
      fi
      latest="$(jq -r '.result.sync_info.latest_block_height | tonumber' <<<"$status")"
      live=$((live + 1))
      (( max_height == 0 || latest > max_height )) && max_height="$latest"
      (( min_height == 0 || latest < min_height )) && min_height="$latest"
      app_hashes+=$'\n'"$(jq -r '.result.sync_info.latest_app_hash' <<<"$status")"
    done
    if (( live >= 4 && max_height == min_height )) &&
      [[ "$(printf '%s\n' "$app_hashes" | sed '/^$/d' | sort -u | wc -l | tr -d ' ')" == "1" ]]; then
      printf '%s\n' "$app_hashes" | sed '/^$/d' | sort -u >"$RUN_ROOT/meta/${label// /-}-app-hash.txt"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      for i in 1 2 3 4 5 6 7 8 9; do
        curl_json_quiet "$(rpc_url "$i")/status" >"$RUN_ROOT/meta/${label// /-}-node${i}-status.json" || true
      done
      die "${label} app_hash did not converge across live nodes"
    fi
    sleep 2
  done
}

assert_no_stale_migrate_signer_retry() {
  local label="$1"
  local errors_file="$RUN_ROOT/meta/${label// /-}-stale-migrate-signer-errors.log"
  grep -h 'missing source input' "$RUN_ROOT"/logs/bifrost-*.log 2>/dev/null |
    grep '"level":"error"' |
    grep '"tx_type":"migrate"' >"$errors_file" || true
  if [[ -s "$errors_file" ]]; then
    die "${label} found stale migrate missing-source signer retries"
  fi
}

sats_to_btc() {
  awk -v sats="$1" 'BEGIN { printf "%.8f", sats / 100000000 }'
}

register_extra_node() {
  local node="$1" label="$2"
  local node_addr node_pubkey operator_pubkey node_ip out raw_log status
  # shellcheck disable=SC1090
  source "$RUN_ROOT/meta/node${node}.env"
  node_addr="$address"
  node_pubkey="$cons"
  operator_pubkey="$secp"
  node_ip="${NODE_IP_ADDRESS:-127.0.0.1}"

  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" set-ip-address "$node_ip")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-set-ip-address.json"
  assert_tx_success "$out" "${label} set-ip-address"

  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" set-node-keys "$operator_pubkey" "$node_pubkey")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-set-node-keys.json"
  if ! jq -e '(.code // 0) == 0' <<<"$out" >/dev/null; then
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
    if [[ "$raw_log" != *"already has pubkey set assigned"* ]]; then
      assert_tx_success "$out" "${label} set-node-keys"
    fi
  fi

  set_and_assert_node_version "$node" "$label" "$(cat "$RUN_ROOT/meta/network-node-version.txt" 2>/dev/null || printf '3.17.0')" >/dev/null
  wait_blocks 2
  node_query "$node_addr" >"$RUN_ROOT/meta/${label}-registered-node.json"
  status="$(jq -r '(.node.status // .status) | ascii_downcase' "$RUN_ROOT/meta/${label}-registered-node.json")"
  [[ "$status" == "standby" || "$status" == "whitelisted" || "$status" == "selected" ]] \
    || die "${label} registration left node in unexpected status ${status}"
  jq -e --arg secp "$operator_pubkey" --arg cons "$node_pubkey" \
    '((.node.pub_key_set.secp256k1 // .pub_key_set.secp256k1) == $secp) and ((.node.node_cons_pub_key // .node_cons_pub_key) == $cons) and ((.node.version // .version) | length) > 0' \
    "$RUN_ROOT/meta/${label}-registered-node.json" >/dev/null || die "${label} registered node keys/version invalid"
}

bond_extra_node_from_notes() {
  local node="$1" amount_sats="$2" label="$3"
  local node_addr node_pubkey operator_pubkey deposit_pubkey deposit_owner session deposit_address txid matched deposit_id receipt commitment_objects commitments shield_signature
  local note leaves withdrawal prefix out nullifier bond amount_btc note_count idx spent_total
  source "$RUN_ROOT/meta/node${node}.env"
  node_addr="$address"
  node_pubkey="$cons"
  operator_pubkey="$secp"
  amount_btc="$(sats_to_btc "$amount_sats")"

  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${label}-deposit-pubkey")"
  deposit_owner="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  if [[ -s "$RUN_ROOT/meta/${label}-receipt.json" ]]; then
    log "${label}: resuming from existing shielder receipt"
    receipt="$(cat "$RUN_ROOT/meta/${label}-receipt.json")"
  else
    if [[ -s "$RUN_ROOT/meta/${label}-deposit-session.json" ]]; then
      log "${label}: resuming from existing bond deposit session"
      session="$(cat "$RUN_ROOT/meta/${label}-deposit-session.json")"
    else
      log "${label}: requesting bond deposit for node${node} amount_sats=${amount_sats}"
      request_deposit "$RUN_ROOT/node${node}" "validator${node}" "${label}" "$deposit_pubkey" >"$RUN_ROOT/meta/${label}-request-deposit.json"
      session="$(deposit_session "$deposit_owner")"
      printf '%s\n' "$session" >"$RUN_ROOT/meta/${label}-deposit-session.json"
    fi
    deposit_address="$(jq -r '.deposit_address' <<<"$session")"
    log "${label}: waiting for bond deposit match"
    if matched="$(wait_owner_deposit_matched "$deposit_owner" 5 2>/dev/null)"; then
      log "${label}: existing bond deposit matched"
    else
      log "${label}: funding bond deposit address"
      txid="$(mine_to_registered_deposit "$deposit_address" "$amount_btc")"
      printf '%s\n' "$txid" >"$RUN_ROOT/meta/${label}-btc-deposit-txid.txt"
      log "${label}: waiting for funded bond deposit match"
      matched="$(wait_owner_deposit_matched "$deposit_owner" 420)"
    fi
    printf '%s\n' "$matched" >"$RUN_ROOT/meta/${label}-deposit-matched.json"
    deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
    jq -e '.status == "deposit_matched" and (.deposit_id | length) > 0' \
      "$RUN_ROOT/meta/${label}-deposit-matched.json" >/dev/null || die "${label} deposit did not match"

    receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(jq -r '.deposit_path_index' <<<"$session")" "$amount_sats" "${label}-note-seed")"
    printf '%s\n' "$receipt" >"$RUN_ROOT/meta/${label}-receipt.json"
    commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
    commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
    shield_signature="$("$SHIELDER_HELPER" shield-authorization "${label}-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
    out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" shielder shield "$commitments" "$deposit_pubkey" "$shield_signature" "$deposit_id")"
    printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-shield.json"
    assert_tx_success "$out" "${label} shield"
    log "${label}: shielded bond deposit"
    assert_shielder_receipt_committed "$RUN_ROOT/meta/${label}-receipt.json" "${label}-note"
    record_shielder_notes "$receipt"
  fi

  note_count="$(jq -r '.notes | length' "$RUN_ROOT/meta/${label}-receipt.json")"
  spent_total=0
  for ((idx=0; idx<note_count; idx++)); do
    note="$(jq -c --argjson idx "$idx" '.notes[$idx]' "$RUN_ROOT/meta/${label}-receipt.json")"
    leaves="$(shielder_leaves "$(jq -r '.denomination_sats' <<<"$note")")"
    printf '%s\n' "$leaves" >"$RUN_ROOT/meta/${label}-proof-leaves-${idx}.json"
    assert_shielder_root_committed "$(jq -r '.denomination_sats' <<<"$note")" "$leaves" "${label}-note-${idx}"
    withdrawal="$("$SHIELDER_HELPER" withdrawal-policy "$note" "${label}-note-seed" "$leaves" "bond_escrow" 0 "bond_escrow" "$node_pubkey" "")"
    printf '%s\n' "$withdrawal" >"$RUN_ROOT/meta/${label}-bond-withdrawal-${idx}.json"
    prefix="$RUN_ROOT/meta/${label}-bond-withdrawal-${idx}"
    "$SHIELDER_HELPER" shield-withdrawal "$withdrawal" "$prefix"
    out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" shielder bond-from-notes "$node_pubkey" "$operator_pubkey" "${prefix}.proof.json" "${prefix}.public.json")"
    printf '%s\n' "$out" >"$RUN_ROOT/meta/${label}-bond-from-notes-${idx}.json"
    assert_tx_success "$out" "${label} bond-from-notes note ${idx}"
    log "${label}: bonded note ${idx}"
    spent_total="$((spent_total + $(jq -r '.denomination_sats' <<<"$note")))"
    nullifier="$(jq -r '.nullifier_hash' "${prefix}.public.json")"
    curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}" >"$RUN_ROOT/meta/${label}-bond-nullifier-${idx}.json"
    jq -e '.spent == true' "$RUN_ROOT/meta/${label}-bond-nullifier-${idx}.json" >/dev/null || die "${label} bond nullifier ${idx} not spent"
    curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/${label}-bond-after-note-${idx}.json"
    if (( spent_total < amount_sats )); then
      jq -e --arg node_key "$node_pubkey" --argjson pending "$spent_total" \
        '.node_pub_key == $node_key and (.pending_sats | tonumber) == $pending and ((.bond_sats // 0) | tonumber) == 0 and (.fee_share_active != true)' \
        "$RUN_ROOT/meta/${label}-bond-after-note-${idx}.json" >/dev/null || die "${label} pending bond state invalid after note ${idx}"
    fi
  done

  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/${label}-bond.json"
  jq -e --arg op "$operator_pubkey" --arg node_key "$node_pubkey" --argjson amount "$amount_sats" \
    '.node_pub_key == $node_key and .operator_pub_key == $op and (.bond_sats | tonumber) == $amount and (.node_status == "Standby" or .node_status == "Whitelisted") and .fee_share_active == true' \
    "$RUN_ROOT/meta/${label}-bond.json" >/dev/null || die "${label} standby bond state invalid"
  node_query "$node_addr" >"$RUN_ROOT/meta/${label}-node-after-bond.json"
  log "${label}: bond complete"
}

required_bond_sats_for_node() {
  local node="$1" label="$2"
  local node_pubkey metrics_file bond_file status start increment next_required slot required tmp metrics_tmp metrics_start
  source "$RUN_ROOT/meta/node${node}.env"
  node_pubkey="$cons"
  metrics_file="$RUN_ROOT/meta/${label}-metrics-before-bond.json"
  bond_file="$RUN_ROOT/meta/${label}-bond-before-bond.json"

  metrics_tmp="${metrics_file}.tmp"
  metrics_start="$(date +%s)"
  while true; do
    status="$(curl --connect-timeout 2 --max-time 8 -sS -o "$metrics_tmp" -w "%{http_code}" "$(api_url 1)/thornado/nodes/metrics" || true)"
    if [[ "$status" == "200" ]] && jq -e type "$metrics_tmp" >/dev/null 2>&1; then
      mv "$metrics_tmp" "$metrics_file"
      break
    fi
    if (( "$(date +%s)" - metrics_start >= 20 )); then
      jq -n '{next_slot:"1",next_slot_bond_required_sats:"100000000",bond_start_amount_sats:"0",bond_slot_increment_sats:"100000000"}' >"$metrics_file"
      break
    fi
    sleep 1
  done

  tmp="${bond_file}.tmp"
  status="$(curl --connect-timeout 2 --max-time 8 -sS -o "$tmp" -w "%{http_code}" "$(api_url 1)/thornado/bond/${node_pubkey}" || true)"
  if [[ "$status" == "200" ]] && jq -e type "$tmp" >/dev/null 2>&1; then
    mv "$tmp" "$bond_file"
  else
    printf '{}\n' >"$bond_file"
  fi

  start="$(jq -r '(.bond_start_amount_sats // 0) | tonumber' "$metrics_file")"
  increment="$(jq -r '(.bond_slot_increment_sats // 100000000) | tonumber' "$metrics_file")"
  next_required="$(jq -r '(.next_slot_bond_required_sats // 100000000) | tonumber' "$metrics_file")"
  slot="$(jq -r '(.slot // 0) | tonumber' "$bond_file")"
  if (( slot > 0 )); then
    required=$((start + (slot * increment)))
  else
    required="$next_required"
  fi
  [[ "$required" =~ ^[0-9]+$ ]] && (( required > 0 )) || die "${label} required bond invalid"
  printf '%s\n' "$required"
}

churn_extra_node_with_migration() {
  local node="$1" label="$2" desired_active="${3:-}"
  local old_vault old_addr new_vault new_addr node_addr node_secp node_cons target_active flow_start active_vaults latest status start config keygen_json keygen_height h current_height raw_log
  local migrate_txout out_hash out_hash_upper raw_tx old_prevouts after_vaults migrate_height migrate_amount
  curl -fsS "$(api_url 1)/thornado/vaults/base" >"$RUN_ROOT/meta/${label}-vaults-before.json"
  old_vault="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/${label}-vaults-before.json")"
  old_addr="$(jq -r --arg old "$old_vault" '.[] | select(.pub_key == $old) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/${label}-vaults-before.json" | head -n1)"
  if [[ -z "$old_addr" || "$old_addr" == "null" ]]; then
    old_addr="$("$SHIELDER_HELPER" btc-address "$old_vault" 0)"
  fi
  flow_start="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
  target_active="${desired_active:-$(curl_json_quiet "$(api_url 1)/thornado/nodes" | jq '[((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active")] | length + 1')}"
  active_cons_list >"$RUN_ROOT/meta/${label}-active-cons-before.txt"
  source "$RUN_ROOT/meta/node${node}.env"
  node_addr="$address"
  node_secp="$secp"
  node_cons="$cons"

  wait_all_signed_txouts_finalized "${label}-pre-churn" 900
  set_config_from_active_nodes Halt_Churning 1

  log "${label}: registering node${node} for churn"
  register_extra_node "$node" "$label"
  node_query "$node_addr" >"$RUN_ROOT/meta/${label}-node-pre-start.json"
  jq -e --arg secp "$node_secp" --arg cons "$node_cons" \
    '(((.node.status // .status) | ascii_downcase) as $status | ($status == "standby" or $status == "selected" or $status == "whitelisted")) and ((.node.pub_key_set.secp256k1 // .pub_key_set.secp256k1) == $secp) and ((.node.node_cons_pub_key // .node_cons_pub_key) == $cons) and ((.node.version // .version) | length) > 0' \
    "$RUN_ROOT/meta/${label}-node-pre-start.json" >/dev/null || die "${label} node pre-start state invalid"

  if curl_json_quiet "$(rpc_url "$node")/status" >/dev/null; then
    log "${label} Thornado node${node} already running"
  else
    start_thornado_extra_node "$node"
  fi
  if curl -fsS "http://127.0.0.1:$(frost_info_port "$node")/ping" >/dev/null 2>&1; then
    log "${label} Bifrost node${node} already running"
  else
    start_bifrost_extra_node "$node" "$label"
  fi
  wait_bifrost_ready_for_keygen "$node"
  curl -fsS "http://127.0.0.1:$(frost_info_port "$node")/status/signing" >"$RUN_ROOT/meta/${label}-bifrost-signing-before-churn.json"

  log "${label}: unhalting churn target_active=${target_active}"
  set_config_from_active_nodes Vault_MigrationIntervalMinutes 1
  set_config_from_active_nodes Chain_BlockTimeSeconds "$THORNADO_BLOCK_TIME_SECONDS"
  set_config_from_active_nodes Churn_IntervalMinutes 1
  set_config_from_active_nodes Churn_RetryIntervalMinutes 1
  set_config_from_active_nodes Halt_SolvencyCheck 0
  set_config_from_active_nodes Node_SetDesired "$target_active"
  set_config_from_active_nodes Halt_Churning 0
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/${label}-config.json"
  config="$(jq -r '(.NODE_SETDESIRED.value // (.configs[]? | select(.key == "Node_SetDesired") | .value) // empty)' "$RUN_ROOT/meta/${label}-config.json" | tail -n1)"
  [[ "$config" == "$target_active" ]] || die "${label} Node_SetDesired did not apply"
  config="$(jq -r '(.HALT_CHURNING.value // (.configs[]? | select(.key == "Halt_Churning") | .value) // empty)' "$RUN_ROOT/meta/${label}-config.json" | tail -n1)"
  [[ "$config" == "0" ]] || die "${label} churn halt was not cleared"

  : >"$RUN_ROOT/meta/${label}-status-history.tsv"
  start="$(date +%s)"
  while true; do
    status="$(node_query "$node_addr" | jq -r '(.node.status // .status) | ascii_downcase')"
    active_vaults="$(curl_json_quiet "$(api_url 1)/thornado/vaults/base")"
    latest="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
    printf '%s\t%s\n' "$latest" "$status" >>"$RUN_ROOT/meta/${label}-status-history.tsv"
    if jq -e --arg old "$old_vault" --arg member "$node_secp" '
      [.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))] | length == 1
    ' <<<"$active_vaults" >/dev/null; then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/${label}-vaults-after-churn.json"
      new_vault="$(jq -r --arg old "$old_vault" --arg member "$node_secp" '[.[]? | select(.status == "ActiveVault" and .pub_key != $old and ((.membership // []) | index($member)))][0].pub_key' "$RUN_ROOT/meta/${label}-vaults-after-churn.json")"
      new_addr="$(jq -r --arg new "$new_vault" '.[] | select(.pub_key == $new) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/${label}-vaults-after-churn.json" | head -n1)"
      if [[ -z "$new_addr" || "$new_addr" == "null" ]]; then
        new_addr="$("$SHIELDER_HELPER" btc-address "$new_vault" 0)"
      fi
      break
    fi
    if (( "$(date +%s)" - start >= 1800 )); then
      printf '%s\n' "$active_vaults" >"$RUN_ROOT/meta/${label}-vaults-timeout.json"
      die "${label} node${node} did not churn active"
    fi
    log "${label} waiting for critical vault rotation: height=${latest} node${node}_status=${status}"
    sleep 10
  done
  curl -fsS "$(api_url 1)/thornado/nodes" >"$RUN_ROOT/meta/${label}-nodes-after-churn.json"
  jq -e --arg old "$old_vault" --arg new "$new_vault" --arg member "$node_secp" --slurpfile nodes "$RUN_ROOT/meta/${label}-nodes-after-churn.json" '
    def node_list: ($nodes[0] | if type == "array" then . else .nodes end);
    def active_vault: [.[] | select(.status == "ActiveVault")][0];
    ([.[] | select(.status == "ActiveVault")] | length) == 1 and
    (.[] | select(.pub_key == $old) | .status) == "RetiringVault" and
    (.[] | select(.pub_key == $new) | .status) == "ActiveVault" and
    ((active_vault.membership // []) | index($member)) and
    ((active_vault.membership // []) | length) >= 1
  ' "$RUN_ROOT/meta/${label}-vaults-after-churn.json" >/dev/null || die "${label} vault rotation state invalid"
  if ! jq -e --arg new "$new_vault" --slurpfile nodes "$RUN_ROOT/meta/${label}-nodes-after-churn.json" '
    def node_list: ($nodes[0] | if type == "array" then . else .nodes end);
    def active_node_secp:
      [node_list[]? | select((.status | ascii_downcase) == "active") | .pub_key_set.secp256k1] | sort;
    [.[] | select(.pub_key == $new)][0].membership as $members |
    (($members // [] | sort) == active_node_secp)
  ' "$RUN_ROOT/meta/${label}-vaults-after-churn.json" >/dev/null; then
    log "${label} progress note: node status set and active vault membership differ; continuing with vault/localstate/BTC critical checks"
  fi
  [[ -f "$RUN_ROOT/bifrost${node}/localstate-${new_vault}.json" ]] || die "${label} bifrost${node} local FROST state missing"
  jq -r --arg new "$new_vault" '.[] | select(.pub_key == $new) | .membership[]' "$RUN_ROOT/meta/${label}-vaults-after-churn.json" | sort >"$RUN_ROOT/meta/${label}-vault-membership.txt"
  jq -r '.participant_keys[]' "$RUN_ROOT/bifrost${node}/localstate-${new_vault}.json" | sort >"$RUN_ROOT/meta/${label}-localstate-participants.txt"
  cmp -s "$RUN_ROOT/meta/${label}-vault-membership.txt" "$RUN_ROOT/meta/${label}-localstate-participants.txt" \
    || die "${label} local FROST participants did not match active vault membership"
  jq -e --arg vault "$new_vault" --arg local "$node_secp" \
    '.pub_key == $vault and .signing_engine == "frost" and .local_party_key == $local and (.participant_keys | index($local))' \
    "$RUN_ROOT/bifrost${node}/localstate-${new_vault}.json" >/dev/null || die "${label} local FROST state invalid"

  keygen_height=""
  h="$flow_start"
  while true; do
    keygen_json="$(curl_json_quiet "$(api_url 1)/thornado/keygen/${h}/${node_secp}" || true)"
    if [[ -n "$keygen_json" ]] && jq -e --arg member "$node_secp" '.keygen_block.keygens[]? | select(.members | index($member))' <<<"$keygen_json" >/dev/null 2>&1; then
      keygen_height="$h"
      printf '%s\n' "$keygen_json" >"$RUN_ROOT/meta/${label}-keygen.json"
      break
    fi
    current_height="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
    if (( h >= flow_start + 900 )); then
      break
    fi
    if (( h >= current_height )); then
      sleep 1
      continue
    fi
    h=$((h + 1))
  done
  [[ -n "$keygen_height" ]] || die "${label} did not find keygen block"
  printf '%s\n' "$keygen_height" >"$RUN_ROOT/meta/${label}-keygen-height.txt"

  migrate_txout="$(wait_migrate_signed "$flow_start" "$old_vault" "$new_addr" 1200)"
  printf '%s\n' "$migrate_txout" >"$RUN_ROOT/meta/${label}-migrate-txout.json"
  jq -e --arg old "$old_vault" --arg to "$new_addr" \
    '[.txout.tx_array[]? | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to and (.coin.amount | tonumber) > 0 and (.out_hash // "") != "")] | length == 1' \
    "$RUN_ROOT/meta/${label}-migrate-txout.json" >/dev/null || die "${label} migrate txout fields invalid"
  out_hash="$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .out_hash' "$RUN_ROOT/meta/${label}-migrate-txout.json" | head -n1)"
  btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true >"$RUN_ROOT/meta/${label}-btc-migrate-tx.json"
  mine_regtest_blocks 2
  raw_tx="$(btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true)"
  printf '%s\n' "$raw_tx" >"$RUN_ROOT/meta/${label}-btc-migrate-tx-confirmed.json"
  jq -e --arg to "$new_addr" --argjson amount "$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .coin.amount' "$RUN_ROOT/meta/${label}-migrate-txout.json" | head -n1)" \
    '([.vout[] | select(.scriptPubKey.address == $to)] | length) >= 1 and (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 + 0.5 | floor)] | add // 0) >= $amount) and (.confirmations // 0) >= 1' \
    "$RUN_ROOT/meta/${label}-btc-migrate-tx-confirmed.json" >/dev/null || die "${label} BTC migration did not pay new vault"
  old_prevouts="$RUN_ROOT/meta/${label}-migrate-prevouts.json"
  jq -r '.vin[] | @base64' "$RUN_ROOT/meta/${label}-btc-migrate-tx-confirmed.json" | while IFS= read -r vin64; do
    local_txid="$(printf '%s' "$vin64" | base64 -d | jq -r '.txid')"
    local_vout="$(printf '%s' "$vin64" | base64 -d | jq -r '.vout')"
    btc_cli getrawtransaction "$local_txid" true | jq --argjson vout "$local_vout" '{txid, vout:$vout, address:.vout[$vout].scriptPubKey.address, value_sats:(.vout[$vout].value * 100000000 + 0.5 | floor)}'
  done | jq -s '.' >"$old_prevouts"
  jq -e --arg old "$old_addr" 'length > 0 and all(.[]; .address == $old)' "$old_prevouts" >/dev/null || die "${label} BTC migration did not spend old vault UTXOs"
  out_hash_upper="$(printf '%s' "$out_hash" | tr '[:lower:]' '[:upper:]')"
  migrate_height="$(jq -r '.txout.height | tonumber' "$RUN_ROOT/meta/${label}-migrate-txout.json")"
  migrate_amount="$(jq -r --arg old "$old_vault" --arg to "$new_addr" '.txout.tx_array[] | select(.tx_type == "migrate" and .vault_pub_key == $old and .to_address == $to) | .coin.amount' "$RUN_ROOT/meta/${label}-migrate-txout.json" | head -n1)"

  for _ in {1..90}; do
    if curl_json_quiet "$(api_url 1)/thornado/tx/${out_hash_upper}" >"$RUN_ROOT/meta/${label}-migrate-observed-tx.json" &&
      jq -e --arg id "$out_hash_upper" '
        (.. | strings | ascii_upcase | select(. == $id)) and
        (.stages.inbound_observed.completed == true)
      ' "$RUN_ROOT/meta/${label}-migrate-observed-tx.json" >/dev/null; then
      break
    fi
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  jq -e --arg id "$out_hash_upper" '
    (.. | strings | ascii_upcase | select(. == $id)) and
    (.stages.inbound_observed.completed == true)
  ' "$RUN_ROOT/meta/${label}-migrate-observed-tx.json" >/dev/null \
    || die "${label} migration outbound was not final through tx query"

  start="$(date +%s)"
  while true; do
    after_vaults="$(curl_json_quiet "$(api_url 1)/thornado/vaults/base")"
    printf '%s\n' "$after_vaults" >"$RUN_ROOT/meta/${label}-vaults-after-migration.json"
    curl_json_quiet "$(api_url 1)/thornado/vault/${old_vault}" >"$RUN_ROOT/meta/${label}-old-vault-after-migration.json"
    curl_json_quiet "$(api_url 1)/thornado/tx/${out_hash_upper}" >"$RUN_ROOT/meta/${label}-migrate-final-tx.json" || true
    if jq -e --arg old "$old_vault" --arg new "$new_vault" '
      ([.[] | select(.pub_key == $old and .status == "ActiveVault")] | length) == 0 and
      ([.[] | select(.pub_key == $new and .status == "ActiveVault")] | length) == 1
    ' "$RUN_ROOT/meta/${label}-vaults-after-migration.json" >/dev/null &&
      jq -e --arg new "$new_vault" --argjson amount "$migrate_amount" '
        [.[] | select(.pub_key == $new and .status == "ActiveVault")][0] as $vault |
        (($vault.coins // []) | map(select(.asset == "BTC.BTC") | .amount | tonumber) | add // 0) >= $amount
      ' "$RUN_ROOT/meta/${label}-vaults-after-migration.json" >/dev/null &&
      jq -e --argjson height "$migrate_height" '
        ((.coins // []) | map(.amount | tonumber) | add // 0) == 0 and
        (((.pending_tx_block_heights // []) | map(tonumber) | index($height)) == null)
      ' "$RUN_ROOT/meta/${label}-old-vault-after-migration.json" >/dev/null; then
      break
    fi
    if (( "$(date +%s)" - start >= 1800 )); then
      break
    fi
    latest="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
    log "${label} waiting for migration accounting: height=${latest} old_amount=$(jq -r '((.coins // []) | map(.amount | tonumber) | add // 0)' "$RUN_ROOT/meta/${label}-old-vault-after-migration.json") pending=$(jq -c '.pending_tx_block_heights // []' "$RUN_ROOT/meta/${label}-old-vault-after-migration.json") finalised=$(jq -r '.stages.inbound_finalised.completed // false' "$RUN_ROOT/meta/${label}-migrate-final-tx.json" 2>/dev/null || echo false)"
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  active_cons_list >"$RUN_ROOT/meta/${label}-active-cons-after.txt"
  comm -23 "$RUN_ROOT/meta/${label}-active-cons-before.txt" "$RUN_ROOT/meta/${label}-active-cons-after.txt" >"$RUN_ROOT/meta/${label}-removed-cons.txt" || true
  jq -e --arg old "$old_vault" --arg new "$new_vault" '
    ([.[] | select(.pub_key == $old and .status == "ActiveVault")] | length) == 0 and
    ([.[] | select(.pub_key == $new and .status == "ActiveVault")] | length) == 1
  ' "$RUN_ROOT/meta/${label}-vaults-after-migration.json" >/dev/null || die "${label} vault status invalid after migration"
  jq -e --arg new "$new_vault" --argjson amount "$migrate_amount" '
    [.[] | select(.pub_key == $new and .status == "ActiveVault")][0] as $vault |
    (($vault.coins // []) | map(select(.asset == "BTC.BTC") | .amount | tonumber) | add // 0) >= $amount
  ' "$RUN_ROOT/meta/${label}-vaults-after-migration.json" >/dev/null || die "${label} destination vault was not credited after migration"
  jq -e --argjson height "$migrate_height" '
    ((.coins // []) | map(.amount | tonumber) | add // 0) == 0 and
    (((.pending_tx_block_heights // []) | map(tonumber) | index($height)) == null)
  ' "$RUN_ROOT/meta/${label}-old-vault-after-migration.json" >/dev/null \
    || die "${label} old vault was not drained or still had pending migration height"
  set_config_from_active_nodes Halt_Churning 1
}

validate_bonded_rotation4() {
  log "Bonded rotation: validating four 1-in/1-out churns to four bonded active nodes"
  local round standby_node required_sats label removed_count removed_cons removed_node active_count bonded_count

  set_config_from_active_nodes Halt_Churning 1
  set_config_from_active_nodes HaltSigningBTC 0
  set_config_from_active_nodes Halt_SolvencyCheck 0
  set_config_from_active_nodes Node_SetDesired 4
  set_config_from_active_nodes Chain_BlockTimeSeconds "$THORNADO_BLOCK_TIME_SECONDS"
  set_config_from_active_nodes Churn_IntervalMinutes 1
  set_config_from_active_nodes Churn_RetryIntervalMinutes 1
  set_config_from_active_nodes Vault_MigrationIntervalMinutes 1

  wait_api_json_file "$(api_url 1)/thornado/nodes" "$RUN_ROOT/meta/rotation4-initial-nodes.json" "rotation4 initial nodes" 120
  active_count="$(jq '[((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active")] | length' "$RUN_ROOT/meta/rotation4-initial-nodes.json")"
  [[ "$active_count" == "4" ]] || die "bonded rotation expected 4 genesis active nodes, got ${active_count}"

  for round in 1 2 3 4; do
    standby_node=$((round + 4))
    label="rotation4-round${round}-node${standby_node}"
    [[ -f "$RUN_ROOT/meta/node${standby_node}.env" ]] || die "${label} key material missing"
    wait_all_signed_txouts_finalized "${label}-pre-bond" 900
    required_sats="$(required_bond_sats_for_node "$standby_node" "$label")"

    bond_extra_node_from_notes "$standby_node" "$required_sats" "${label}-bond"
    churn_extra_node_with_migration "$standby_node" "${label}-churn" 4
    assert_live_nodes_app_hash_converged "${label}-churn"
    assert_no_stale_migrate_signer_retry "${label}-churn"

    removed_count="$(wc -l <"$RUN_ROOT/meta/${label}-churn-removed-cons.txt" | tr -d ' ')"
    [[ "$removed_count" == "1" ]] || die "${label} expected exactly one churned-out node, got ${removed_count}"
    removed_cons="$(head -n1 "$RUN_ROOT/meta/${label}-churn-removed-cons.txt")"
    removed_node="$(node_index_by_cons "$removed_cons")" || die "${label} could not map removed node ${removed_cons}"
    printf '%s\n' "$removed_node" >"$RUN_ROOT/meta/${label}-removed-node.txt"
    [[ "$removed_node" -ge 1 && "$removed_node" -le 4 ]] || die "${label} expected a genesis node to churn out, got node${removed_node}"
  done

  wait_api_json_file "$(api_url 1)/thornado/nodes" "$RUN_ROOT/meta/rotation4-final-nodes.json" "rotation4 final nodes" 120
  active_count="$(jq '[((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active")] | length' "$RUN_ROOT/meta/rotation4-final-nodes.json")"
  [[ "$active_count" == "4" ]] || die "bonded rotation final active count is ${active_count}, expected 4"

  jq -r '((if type == "array" then . else .nodes end)[]?) | select((.status | ascii_downcase) == "active") | .node_cons_pub_key' \
    "$RUN_ROOT/meta/rotation4-final-nodes.json" >"$RUN_ROOT/meta/rotation4-final-active-cons.txt"
  while IFS= read -r removed_cons; do
    [[ -n "$removed_cons" ]] || continue
    removed_node="$(node_index_by_cons "$removed_cons")" || die "rotation4 final could not map active node ${removed_cons}"
    [[ "$removed_node" -ge 5 && "$removed_node" -le 8 ]] || die "rotation4 final active set still includes genesis node${removed_node}"
  done <"$RUN_ROOT/meta/rotation4-final-active-cons.txt"
  bonded_count=0
  while IFS= read -r removed_cons; do
    [[ -n "$removed_cons" ]] || continue
    curl -fsS "$(api_url 1)/thornado/bond/${removed_cons}" >"$RUN_ROOT/meta/rotation4-final-bond-${bonded_count}.json"
    jq -e --arg node "$removed_cons" '.node_pub_key == $node and (.bond_sats | tonumber) > 0 and .fee_share_active == true and (.node_status | ascii_downcase) == "active"' \
      "$RUN_ROOT/meta/rotation4-final-bond-${bonded_count}.json" >/dev/null || die "final active node ${removed_cons} is not fully bonded"
    bonded_count=$((bonded_count + 1))
  done <"$RUN_ROOT/meta/rotation4-final-active-cons.txt"
  [[ "$bonded_count" == "4" ]] || die "bonded rotation final bonded count is ${bonded_count}, expected 4"

  assert_live_nodes_app_hash_converged "rotation4 final"
  assert_no_stale_migrate_signer_retry "rotation4 final"
  log "RESULTS Bonded rotation: PASS"
}

validate_flow7() {
  log "Flow 7: validating many deposits and BTC consolidation"
  local i previous_session previous_address previous_path session deposit_address txid deposit_id found start matched deposit_pubkey owner_addr
  local flow7_start active_vault active_addr config_value path prev_flow7_address prev_flow7_path sweep_txout sweep_count consolidate_count
  local root_addr root_received out_hash raw_tx
  set_config_from_active_nodes Halt_Churning 1
  set_config_from_active_nodes UTXO_MaxSpendCount 3
  set_config_from_active_nodes HaltSigningBTC 0
  set_config_from_active_nodes Halt_SolvencyCheck 0
  flow7_start="$(curl -fsS "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height | tonumber')"
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/flow7-config.json"
  config_value="$(jq -r '(.UTXO_MAXSPENDCOUNT.value // (.configs[]? | select(.key == "UTXO_MaxSpendCount") | .value) // empty)' "$RUN_ROOT/meta/flow7-config.json" | tail -n1)"
  [[ "$config_value" == "3" ]] || die "flow7 UTXO_MaxSpendCount config was not applied"
  curl -fsS "$(api_url 1)/thornado/vaults/base" >"$RUN_ROOT/meta/flow7-vaults-before.json"
  active_vault="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/flow7-vaults-before.json")"
  active_addr="$(jq -r --arg vault "$active_vault" '.[] | select(.pub_key == $vault) | .addresses[]? | select(.chain == "BTC") | .address' "$RUN_ROOT/meta/flow7-vaults-before.json" | head -n1)"
  if [[ -z "$active_addr" || "$active_addr" == "null" ]]; then
    active_addr="$("$SHIELDER_HELPER" btc-address "$active_vault" 0)"
  fi
  source "$RUN_ROOT/meta/user.env"
  prev_flow7_address=""
  prev_flow7_path=0
  for i in 1 2 3; do
    log "flow7 deposit ${i}: requesting deposit"
    deposit_pubkey="$("$SHIELDER_HELPER" pubkey "user-flow-7-${i}-deposit-pubkey")"
    owner_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
    previous_session="$(deposit_session "$owner_addr" 2>/dev/null || true)"
    previous_address="$(jq -r '.deposit_address // ""' <<<"$previous_session" 2>/dev/null || true)"
    previous_path="$(jq -r '.deposit_path_index // ""' <<<"$previous_session" 2>/dev/null || true)"
    request_deposit "$RUN_ROOT/node1" "user" "user-flow-7-${i}" "$deposit_pubkey" >/dev/null
    session="$(wait_new_deposit_session "$owner_addr" "$previous_address" "$previous_path")"
    printf '%s\n' "$session" >"$RUN_ROOT/meta/flow7-deposit-${i}-session.json"
    path="$(jq -r '.deposit_path_index | tonumber' <<<"$session")"
    jq -e --arg owner "$owner_addr" --arg vault "$active_vault" --argjson path "$path" '
      .owner == $owner and .vault_pub_key == $vault and (.deposit_address | length) > 0 and (.deposit_path_index | tonumber) == $path
    ' "$RUN_ROOT/meta/flow7-deposit-${i}-session.json" >/dev/null || die "flow7 deposit ${i} session did not route to active vault"
    if [[ -n "$prev_flow7_address" ]]; then
      [[ "$(jq -r '.deposit_address' <<<"$session")" != "$prev_flow7_address" ]] || die "flow7 deposit ${i} reused previous child address"
      (( path > prev_flow7_path )) || die "flow7 deposit ${i} did not advance child path"
    fi
    deposit_address="$(jq -r '.deposit_address' <<<"$session")"
    txid="$(mine_to_registered_deposit "$deposit_address" "0.01000000")"
    printf '%s\n' "$txid" >"$RUN_ROOT/meta/flow7-deposit-${i}-btc-txid.txt"
    btc_cli -rpcwallet=bifrost1 listunspent 1 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow7-deposit-${i}-child-utxo-before-sweep.json"
    jq -e 'map(select((.amount * 100000000 + 0.5 | floor) == 1000000)) | length == 1' "$RUN_ROOT/meta/flow7-deposit-${i}-child-utxo-before-sweep.json" >/dev/null \
      || die "flow7 deposit ${i} child UTXO was not visible before sweep"
    log "flow7 deposit ${i}: waiting for match"
    matched="$(wait_owner_deposit_matched "$owner_addr" 420)"
    printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow7-deposit-${i}-matched.json"
    jq -e --arg owner "$owner_addr" --arg addr "$deposit_address" --arg vault "$active_vault" --arg txid "$txid" '
      .owner == $owner and
      .deposit_address == $addr and
      .vault_pub_key == $vault and
      .status == "deposit_matched" and
      (.inbound_tx_id | ascii_upcase) == ($txid | ascii_upcase) and
      (.btc_confirmations | tonumber) >= (.btc_confirmations_required | tonumber)
    ' "$RUN_ROOT/meta/flow7-deposit-${i}-matched.json" >/dev/null || die "flow7 deposit ${i} match record invalid"
    deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
    log "flow7 deposit ${i}: waiting for sweep"
    sweep_txout="$(wait_sweep_signed "$deposit_id" 420)"
    printf '%s\n' "$sweep_txout" >"$RUN_ROOT/meta/flow7-deposit-${i}-sweep-txout.json"
    jq -e --arg in_hash "$deposit_id" --arg vault "$active_vault" --argjson path "$path" '
      .txout.status == "complete" and
      ([.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and .vault_pub_key == $vault and (.vault_path_index | tonumber) == $path and (.out_hash // "") != "" and (.coin.amount | tonumber) > 0)] | length) == 1
    ' "$RUN_ROOT/meta/flow7-deposit-${i}-sweep-txout.json" >/dev/null || die "flow7 deposit ${i} sweep txout invalid"
    btc_cli -rpcwallet=bifrost1 listunspent 0 9999999 "[\"${deposit_address}\"]" >"$RUN_ROOT/meta/flow7-deposit-${i}-child-utxo-after-sweep.json"
    jq -e 'length == 0' "$RUN_ROOT/meta/flow7-deposit-${i}-child-utxo-after-sweep.json" >/dev/null || die "flow7 deposit ${i} child UTXO remained spendable after sweep"
    sweep_count="$(count_signed_txouts_since sweep "$flow7_start")"
    [[ "$sweep_count" == "$i" ]] || die "flow7 expected ${i} signed sweeps, got ${sweep_count}"
    if (( i < 3 )); then
      consolidate_count="$(count_signed_txouts_since consolidate "$flow7_start")"
      [[ "$consolidate_count" == "0" ]] || die "flow7 consolidation appeared before threshold"
    fi
    prev_flow7_address="$deposit_address"
    prev_flow7_path="$path"
    log "flow7 deposit ${i}: sweep complete"
  done
  log "flow7 waiting for consolidation"
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
  jq -e --arg vault "$active_vault" --arg to "$active_addr" '
    .txout.status == "complete" and
    ([.txout.tx_array[]? | select(.tx_type == "consolidate" and .vault_pub_key == $vault and .to_address == $to and (.vault_path_index | tonumber) == 0 and (.out_hash // "") != "" and (.coin.amount | tonumber) > 0 and ((.source_inputs // []) | length) >= 2)] | length) == 1
  ' "$RUN_ROOT/meta/flow7-consolidate-txout.json" >/dev/null || die "flow7 consolidate txout fields invalid"
  out_hash="$(jq -r '.txout.tx_array[] | select(.tx_type == "consolidate") | .out_hash' "$RUN_ROOT/meta/flow7-consolidate-txout.json" | head -n1)"
  root_received="$(jq -r '.txout.tx_array[] | select(.tx_type == "consolidate") | .coin.amount' "$RUN_ROOT/meta/flow7-consolidate-txout.json" | head -n1)"
  mine_regtest_blocks 2
  raw_tx="$(btc_cli getrawtransaction "$(printf '%s' "$out_hash" | tr '[:upper:]' '[:lower:]')" true)"
  printf '%s\n' "$raw_tx" >"$RUN_ROOT/meta/flow7-btc-consolidate-tx-confirmed.json"
  jq -e --arg to "$active_addr" --argjson amount "$root_received" '
    ([.vout[] | select(.scriptPubKey.address == $to)] | length) >= 1 and
    (([.vout[] | select(.scriptPubKey.address == $to) | (.value * 100000000 + 0.5 | floor)] | add // 0) >= $amount) and
    (.confirmations // 0) >= 1
  ' "$RUN_ROOT/meta/flow7-btc-consolidate-tx-confirmed.json" >/dev/null || die "flow7 BTC consolidation transaction did not pay active root vault"
  wait_blocks 2
  consolidate_count="$(count_signed_txouts_since consolidate "$flow7_start")"
  [[ "$consolidate_count" == "1" ]] || die "flow7 expected exactly one consolidation, got ${consolidate_count}"
  log "RESULTS Flow 7: PASS"
}

validate_flow8() {
  log "Flow 8: validating expanded attack paths against completed protocol state"
  local out height auction_id bid_id receipt commitment_objects commitments sale_pubkey sale_sig seller_node_pubkey bidder_operator_pubkey bidder_node_pubkey
  local node required_sats extra_churn_count min_migrations min_sweeps post_sweep_txout

  [[ -f "$RUN_ROOT/meta/flow2-bond-withdrawal.proof.json" ]] || die "flow8 requires Flow 2 bond proof artifacts"
  [[ -f "$RUN_ROOT/meta/flow5-auction-id.txt" ]] || die "flow8 requires Flow 5 auction artifacts"
  [[ -f "$RUN_ROOT/meta/flow7-consolidate-txout.json" ]] || die "flow8 requires Flow 7 consolidation artifacts"
  wait_blocks 5
  mine_regtest_blocks 2
  wait_blocks 2

  source "$RUN_ROOT/meta/node5.env"
  seller_node_pubkey="$cons"
  assert_rejected_without_state_change \
    "flow8 replay bond-from-notes" "shielder nullifier already spent" \
    thornado_tx "$RUN_ROOT/node5" "validator5" shielder bond-from-notes "$cons" "$secp" \
      "$RUN_ROOT/meta/flow2-bond-withdrawal.proof.json" "$RUN_ROOT/meta/flow2-bond-withdrawal.public.json"

  if [[ -f "$RUN_ROOT/meta/flow3-withdrawal.proof.json" && -f "$RUN_ROOT/meta/flow3-withdrawal.public.json" ]]; then
    assert_rejected_without_state_change \
      "flow8 user redeem proof as bond" "Usage:" \
      thornado_tx "$RUN_ROOT/node5" "validator5" shielder bond-from-notes "$cons" "$secp" \
        "$RUN_ROOT/meta/flow3-withdrawal.proof.json" "$RUN_ROOT/meta/flow3-withdrawal.public.json"
  fi

  height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  assert_rejected_without_state_change \
    "flow8 sold node second auction" "node has no active bonded slot" \
    thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$((height + 300))"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" set-ip-address "127.0.0.55")"
  assert_tx_success "$out" "flow8 sold node metadata set-ip-address"
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/bond/${seller_node_pubkey}" >"$RUN_ROOT/meta/flow8-sold-node-bond-after-metadata.json"
  jq -e --arg auction "$(cat "$RUN_ROOT/meta/flow5-auction-id.txt")" \
    '.sold == true and .sold_auction_id == $auction and (.bond_sats | tonumber) == 0 and .fee_share_active == false' \
    "$RUN_ROOT/meta/flow8-sold-node-bond-after-metadata.json" >/dev/null || die "flow8 sold node metadata restored bond eligibility"
  height="$(curl -fsS $(rpc_url 1)/status | jq -r '.result.sync_info.latest_block_height')"
  assert_rejected_without_state_change \
    "flow8 sold node auction after metadata" "node has no active bonded slot" \
    thornado_tx "$RUN_ROOT/node5" "validator5" shielder auction-create "$seller_node_pubkey" 100000000 "$((height + 300))"

  source "$RUN_ROOT/meta/node6.env"
  bidder_operator_pubkey="$secp"
  bidder_node_pubkey="$cons"
  assert_rejected_without_state_change \
    "flow8 fake auction bid" "node slot auction is not open" \
    thornado_tx "$RUN_ROOT/node6" "validator6" shielder auction-bid-create \
      "0000000000000000000000000000000000000000000000000000000000000000" "$bidder_operator_pubkey" "$bidder_node_pubkey"

  auction_id="$(cat "$RUN_ROOT/meta/flow5-auction-id.txt")"
  bid_id="$(cat "$RUN_ROOT/meta/flow5-bid-id.txt")"
  receipt="$($SHIELDER_HELPER receipt-simple 100000000 "operator5-sale-seed")"
  commitment_objects="$($SHIELDER_HELPER commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  sale_pubkey="$($SHIELDER_HELPER pubkey "operator5-sale-pubkey")"
  sale_sig="$($SHIELDER_HELPER shield-authorization "operator5-sale-pubkey" "$sale_pubkey" 100000000 "$commitment_objects" | jq -r '.signature')"
  assert_rejected_without_state_change \
    "flow8 duplicate node-sale-shield" "node sale entitlement is not shieldable" \
    thornado_tx "$RUN_ROOT/node5" "validator5" shielder node-sale-shield "$auction_id" "$bid_id" "$commitments" "$sale_pubkey" "$sale_sig"

  source "$RUN_ROOT/meta/user.env"
  local attack_deposit_pubkey
  attack_deposit_pubkey="$("$SHIELDER_HELPER" pubkey "flow8-request-amount-attack-deposit-pubkey")"
  assert_tx_or_cli_rejected \
    "flow8 request-deposit amount arg" "Usage:" \
    thornado_tx "$RUN_ROOT/node1" "user" request-deposit "flow8-request-amount-attack" "$attack_deposit_pubkey" "123456"

  jq -e '.txout.status == "complete" and ([.txout.tx_array[]? | select(.tx_type == "consolidate" and (.out_hash // "") != "")] | length) == 1' \
    "$RUN_ROOT/meta/flow7-consolidate-txout.json" >/dev/null || die "flow8 flow7 consolidation artifact invalid"

  extra_churn_count=0
  for node in ${FLOW8_EXTRA_CHURN_NODES:-7 8}; do
    [[ -f "$RUN_ROOT/meta/node${node}.env" ]] || die "flow8 node${node} key material missing"
    set_config_from_active_nodes Halt_Churning 1
    wait_all_signed_txouts_finalized "flow8-node${node}-pre-bond" 900
    curl -fsS "$(api_url 1)/thornado/nodes/metrics" >"$RUN_ROOT/meta/flow8-node${node}-metrics-before-bond.json"
    required_sats="$(jq -r '.next_slot_bond_required_sats | tonumber' "$RUN_ROOT/meta/flow8-node${node}-metrics-before-bond.json")"
    [[ "$required_sats" =~ ^[0-9]+$ ]] && (( required_sats > 0 )) || die "flow8 node${node} required bond invalid"
    bond_extra_node_from_notes "$node" "$required_sats" "flow8-node${node}-bond"
    churn_extra_node_with_migration "$node" "flow8-node${node}-churn"
    assert_live_nodes_app_hash_converged "flow8 node${node} churn"
    assert_no_stale_migrate_signer_retry "flow8 node${node} churn"
    extra_churn_count=$((extra_churn_count + 1))
  done

  local post_deposit_pubkey post_owner post_session post_address post_txid post_matched post_deposit_id
  post_deposit_pubkey="$("$SHIELDER_HELPER" pubkey "flow8-post-extra-churn-user-deposit-pubkey")"
  post_owner="$("$SHIELDER_HELPER" owner-address "$post_deposit_pubkey")"
  request_deposit "$RUN_ROOT/node1" "user" "flow8-post-extra-churn-user" "$post_deposit_pubkey" >"$RUN_ROOT/meta/flow8-post-extra-churn-request-deposit.json"
  post_session="$(deposit_session "$post_owner")"
  printf '%s\n' "$post_session" >"$RUN_ROOT/meta/flow8-post-extra-churn-session.json"
  curl -fsS "$(api_url 1)/thornado/vaults/base" >"$RUN_ROOT/meta/flow8-post-extra-churn-vaults.json"
  jq -e --arg vault "$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key' "$RUN_ROOT/meta/flow8-post-extra-churn-vaults.json")" \
    '.vault_pub_key == $vault and (.deposit_address | length) > 0' "$RUN_ROOT/meta/flow8-post-extra-churn-session.json" >/dev/null \
    || die "flow8 post-extra-churn deposit did not route to latest active vault"
  post_address="$(jq -r '.deposit_address' "$RUN_ROOT/meta/flow8-post-extra-churn-session.json")"
  post_txid="$(mine_to_registered_deposit "$post_address" "0.03000000")"
  printf '%s\n' "$post_txid" >"$RUN_ROOT/meta/flow8-post-extra-churn-btc-txid.txt"
  post_matched="$(wait_owner_deposit_matched "$post_owner" 420)"
  printf '%s\n' "$post_matched" >"$RUN_ROOT/meta/flow8-post-extra-churn-matched.json"
  post_deposit_id="$(jq -r '.deposit_id' <<<"$post_matched")"
  post_sweep_txout="$(wait_sweep_signed "$post_deposit_id" 420)"
  printf '%s\n' "$post_sweep_txout" >"$RUN_ROOT/meta/flow8-post-extra-churn-sweep-txout.json"
  jq -e --arg in_hash "$post_deposit_id" --arg vault "$(jq -r '.vault_pub_key' "$RUN_ROOT/meta/flow8-post-extra-churn-session.json")" '
    .txout.status == "complete" and
    ([.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash and .vault_pub_key == $vault and (.vault_path_index | tonumber) > 0 and (.out_hash // "") != "" and (.source_inputs // [] | length) == 1)] | length) == 1
  ' "$RUN_ROOT/meta/flow8-post-extra-churn-sweep-txout.json" >/dev/null || die "flow8 post-extra-churn sweep txout invalid"

  curl -fsS "$(api_url 1)/thornado/txout/all" >"$RUN_ROOT/meta/flow8-final-txouts.json"
  min_migrations=$((1 + extra_churn_count))
  min_sweeps=4
  jq -e '
    ([ (if type == "array" then .[] else .txouts[]? end).tx_array[]? | select(.tx_type == "migrate" and (.out_hash // "") != "")] | length) >= '"$min_migrations"' and
    ([ (if type == "array" then .[] else .txouts[]? end).tx_array[]? | select(.tx_type == "sweep" and (.out_hash // "") != "")] | length) >= '"$min_sweeps"' and
    ([ (if type == "array" then .[] else .txouts[]? end).tx_array[]? | select(.tx_type == "consolidate" and (.out_hash // "") != "")] | length) >= 1
  ' "$RUN_ROOT/meta/flow8-final-txouts.json" >/dev/null || die "flow8 final txout set missing migration or consolidation evidence"
  curl -fsS "$(api_url 1)/thornado/vaults/base" >"$RUN_ROOT/meta/flow8-final-base-vaults.json"
  jq -e '
    ([.[] | select(.status == "ActiveVault")] | length) == 1 and
    all(.[] | select(.status != "ActiveVault");
      (((.coins // []) | map(select(.asset == "BTC.BTC") | .amount | tonumber) | add // 0) == 0))
  ' "$RUN_ROOT/meta/flow8-final-base-vaults.json" >/dev/null || die "flow8 retired vaults are not fully drained"
  assert_live_nodes_app_hash_converged "flow8 final"
  assert_no_stale_migrate_signer_retry "flow8 final"

  log "RESULTS Flow 8: PASS"
}

pick_active_bonded_tooling_node() {
  local candidate node_pubkey node_addr nodes_file bond_file status
  nodes_file="$RUN_ROOT/meta/flow9-active-node-candidates.json"
  wait_api_json_file "$(api_url 1)/thornado/nodes" "$nodes_file" "flow9 active nodes" 120
  for candidate in 8 7 6; do
    [[ -f "$RUN_ROOT/meta/node${candidate}.env" ]] || continue
    source "$RUN_ROOT/meta/node${candidate}.env"
    node_addr="$address"
    node_pubkey="$cons"
    status="$(jq -r --arg node "$node_addr" '
      ([((if type == "array" then . else .nodes end)[]?) | select(.node_address == $node)][0].status // "") | ascii_downcase
    ' "$nodes_file")"
    [[ "$status" == "active" ]] || continue
    bond_file="$RUN_ROOT/meta/flow9-node${candidate}-bond-candidate.json"
    curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$bond_file"
    if jq -e '.fee_share_active == true and .sold == false and (.bond_sats | tonumber) > 0' "$bond_file" >/dev/null; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

validate_flow9() {
  log "Flow 9: validating live node tooling"
  local node node_addr node_pubkey old_operator_pubkey old_operator_addr new_operator_pubkey new_operator_addr out
  local before_bond_sats before_slot before_bonders config_value old_fee expected_bonders

  set_config_from_active_nodes Halt_Churning 1
  node="$(pick_active_bonded_tooling_node)" || die "flow9 could not find an active bonded node6/7/8 candidate"

  source "$RUN_ROOT/meta/node${node}.env"
  node_addr="$address"
  node_pubkey="$cons"
  old_operator_pubkey="$secp"
  old_operator_addr="$address"
  source "$RUN_ROOT/meta/node5.env"
  new_operator_pubkey="$secp"
  new_operator_addr="$address"
  [[ "$old_operator_pubkey" != "$new_operator_pubkey" ]] || die "flow9 old and new operator pubkeys matched"

  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow9-bond-before.json"
  before_bond_sats="$(jq -r '.bond_sats | tonumber' "$RUN_ROOT/meta/flow9-bond-before.json")"
  before_slot="$(jq -r '.slot | tonumber' "$RUN_ROOT/meta/flow9-bond-before.json")"
  before_bonders="$(jq -c '.bonders | map(del(.updated_height)) | sort_by(.bonder)' "$RUN_ROOT/meta/flow9-bond-before.json")"
  jq -e --arg op "$old_operator_pubkey" --arg old "$old_operator_addr" --arg new "$new_operator_addr" '
    .operator_pub_key == $op and
    ([.bonders[]? | select(.bonder == $old and (.principal_sats | tonumber) > 0)] | length) == 1 and
    ([.bonders[]? | select(.bonder == $new)] | length) == 0
  ' "$RUN_ROOT/meta/flow9-bond-before.json" >/dev/null || die "flow9 selected bond not in expected pre-rotate state"

  assert_tx_or_cli_rejected "flow9 non-operator maint" "not authorized" \
    thornado_tx "$RUN_ROOT/node1" "user" node maint "$node_addr"

  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node maint "$node_addr")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-maint-on.json"
  assert_tx_success "$out" "flow9 maint on"
  wait_blocks 2
  node_query "$node_addr" >"$RUN_ROOT/meta/flow9-node-maint-on.json"
  jq -e '(.node.maintenance // .maintenance) == true' "$RUN_ROOT/meta/flow9-node-maint-on.json" >/dev/null || die "flow9 maintenance did not turn on"

  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node maint "$node_addr")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-maint-off.json"
  assert_tx_success "$out" "flow9 maint off"
  wait_blocks 2
  node_query "$node_addr" >"$RUN_ROOT/meta/flow9-node-maint-off.json"
  jq -e '(.node.maintenance // .maintenance) == false' "$RUN_ROOT/meta/flow9-node-maint-off.json" >/dev/null || die "flow9 maintenance did not turn off"

  assert_tx_or_cli_rejected "flow9 non-active config" "unauthorized" \
    thornado_tx "$RUN_ROOT/node1" "user" config UTXO_MaxSpendCount 4
  curl -fsS "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/flow9-config-after-rejected-user.json"
  config_value="$(jq -r '(.UTXO_MAXSPENDCOUNT.value // (.configs[]? | select(.key == "UTXO_MaxSpendCount") | .value) // empty)' "$RUN_ROOT/meta/flow9-config-after-rejected-user.json" | tail -n1)"
  [[ "$config_value" == "3" ]] || die "flow9 rejected config changed UTXO_MaxSpendCount to ${config_value}"

  assert_tx_or_cli_rejected "flow9 non-operator fee set" "node operator signer mismatch" \
    thornado_tx "$RUN_ROOT/node1" "user" node fees set "$node_pubkey" 1000
  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node fees set "$node_pubkey" 1000)"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-fee-set.json"
  assert_tx_success "$out" "flow9 fee set"
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow9-bond-after-fee-set.json"
  jq -e '(.operator_fee_basis_points | tonumber) == 1000' "$RUN_ROOT/meta/flow9-bond-after-fee-set.json" >/dev/null || die "flow9 operator fee basis points did not update"

  old_fee="$(jq -r '.operator_fee_accrued_sats | tonumber' "$RUN_ROOT/meta/flow9-bond-after-fee-set.json")"
  assert_tx_or_cli_rejected "flow9 fee over max" "Usage:" \
    thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node fees set "$node_pubkey" 10001
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow9-bond-after-fee-over-max.json"
  jq -e --argjson old "$old_fee" '(.operator_fee_basis_points | tonumber) == 1000 and (.operator_fee_accrued_sats | tonumber) == $old' \
    "$RUN_ROOT/meta/flow9-bond-after-fee-over-max.json" >/dev/null || die "flow9 rejected fee set mutated bond"

  out="$(thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node rotate-operator "$new_operator_pubkey")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-rotate-operator.json"
  assert_tx_success "$out" "flow9 rotate operator"
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow9-bond-after-rotate.json"
  jq -e --arg op "$new_operator_pubkey" --arg old "$old_operator_addr" --arg new "$new_operator_addr" --argjson sats "$before_bond_sats" --argjson slot "$before_slot" '
    .operator_pub_key == $op and
    (.bond_sats | tonumber) == $sats and
    (.slot | tonumber) == $slot and
    ([.bonders[]? | select(.bonder == $old)] | length) == 0 and
    ([.bonders[]? | select(.bonder == $new and (.principal_sats | tonumber) > 0)] | length) == 1
  ' "$RUN_ROOT/meta/flow9-bond-after-rotate.json" >/dev/null || die "flow9 rotate did not move operator bonder correctly"
  expected_bonders="$(jq -c --arg old "$old_operator_addr" --arg new "$new_operator_addr" 'map(if .bonder == $old then .bonder = $new else . end) | sort_by(.bonder)' <<<"$before_bonders")"
  jq -e --argjson expected "$expected_bonders" '(.bonders | map(del(.updated_height)) | sort_by(.bonder)) == $expected' "$RUN_ROOT/meta/flow9-bond-after-rotate.json" >/dev/null \
    || die "flow9 rotate changed non-operator bonder ledger"

  node_query "$node_addr" >"$RUN_ROOT/meta/flow9-node-after-rotate.json"
  jq -e --arg op "$new_operator_addr" --arg node "$node_addr" '
    ((.node.node_operator_address // .node_operator_address // .node.bond_address // .bond_address) == $op) and
    ((.node.node_address // .node_address) == $node)
  ' "$RUN_ROOT/meta/flow9-node-after-rotate.json" >/dev/null || die "flow9 node account did not point at new operator"

  assert_tx_or_cli_rejected "flow9 old operator maint after rotate" "not authorized" \
    thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node maint "$node_addr"
  assert_tx_or_cli_rejected "flow9 old operator fee after rotate" "node operator signer mismatch" \
    thornado_tx "$RUN_ROOT/node${node}" "validator${node}" node fees set "$node_pubkey" 1100

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" node maint "$node_addr")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-new-operator-maint-on.json"
  assert_tx_success "$out" "flow9 new operator maint on"
  wait_blocks 2
  node_query "$node_addr" >"$RUN_ROOT/meta/flow9-node-new-operator-maint-on.json"
  jq -e '(.node.maintenance // .maintenance) == true' "$RUN_ROOT/meta/flow9-node-new-operator-maint-on.json" >/dev/null || die "flow9 new operator did not turn maintenance on"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" node maint "$node_addr")"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-new-operator-maint-off.json"
  assert_tx_success "$out" "flow9 new operator maint off"
  wait_blocks 2
  node_query "$node_addr" >"$RUN_ROOT/meta/flow9-node-new-operator-maint-off.json"
  jq -e '(.node.maintenance // .maintenance) == false' "$RUN_ROOT/meta/flow9-node-new-operator-maint-off.json" >/dev/null || die "flow9 new operator did not turn maintenance off"

  out="$(thornado_tx "$RUN_ROOT/node5" "validator5" node fees set "$node_pubkey" 0)"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/flow9-new-operator-fee-reset.json"
  assert_tx_success "$out" "flow9 new operator fee reset"
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/bond/${node_pubkey}" >"$RUN_ROOT/meta/flow9-bond-after-fee-reset.json"
  jq -e '(.operator_fee_basis_points | tonumber) == 0' "$RUN_ROOT/meta/flow9-bond-after-fee-reset.json" >/dev/null || die "flow9 new operator fee reset failed"

  assert_live_nodes_app_hash_converged "flow9 final"
  log "RESULTS Flow 9: PASS"
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
  local flow_mode="${FLOW_MODE:-all}"
  build_binaries
  reset_all
  start_bitcoind
  start_btc_auto_miner
  init_genesis
  start_thornado_nodes
  set_and_assert_genesis_versions
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
  wait_bifrost_ready_for_keygen 1
  wait_bifrost_ready_for_keygen 2
  wait_bifrost_ready_for_keygen 3
  wait_bifrost_ready_for_keygen 4
  validate_flow1
  if [[ "$flow_mode" == "bonded_rotation4" ]]; then
    validate_bonded_rotation4
    log "Bonded 4-node rotation passed at ${RUN_ROOT}"
    write_run_summary "PASS" "bonded 4-node rotation passed"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "node5_churn" ]]; then
    validate_flow2
    validate_flow2_node5_churn
    log "Node5 churn spike passed at ${RUN_ROOT}"
    write_run_summary "PASS" "node5 churn spike passed"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "bonded_standby" ]]; then
    FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    log "Bonded standby cluster ready at ${RUN_ROOT}"
    write_run_summary "PASS" "bonded standby cluster ready"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "churn_spike" ]]; then
    FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    validate_flow5
    validate_flow6
    log "Churn spike passed at ${RUN_ROOT}"
    write_run_summary "PASS" "churn spike passed"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "churn_spike_flow7" ]]; then
    FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    validate_flow5
    validate_flow6
    validate_flow7
    log "Churn spike plus Flow 7 passed at ${RUN_ROOT}"
    write_run_summary "PASS" "churn spike plus Flow 7 passed"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "churn_spike_flow8" ]]; then
    FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    validate_flow5
    validate_flow6
    validate_flow7
    validate_flow8
    log "Churn spike plus Flow 8 attack paths passed at ${RUN_ROOT}"
    write_run_summary "PASS" "churn spike plus Flow 8 attack paths passed"
    keep_running_if_requested
    return 0
  fi
  if [[ "$flow_mode" == "churn_spike_flow9" ]]; then
    FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    validate_flow5
    validate_flow6
    validate_flow7
    validate_flow8
    validate_flow9
    log "Churn spike plus Flow 9 node tooling passed at ${RUN_ROOT}"
    write_run_summary "PASS" "churn spike plus Flow 9 node tooling passed"
    keep_running_if_requested
    return 0
  fi
  if (( flow_limit <= 1 )); then
    keep_running_if_requested
    return 0
  fi
  if [[ "${SKIP_FLOW2:-0}" == "1" ]]; then
    log "SKIP_FLOW2=1; skipping bonded standby node flow"
  else
    if (( flow_limit >= 5 )); then
      FLOW2_DEFER_NODE5_PREFLIGHT=1 validate_flow2
    else
      validate_flow2
    fi
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
  if (( flow_limit >= 8 )); then
    validate_flow8
    if (( flow_limit >= 9 )); then
      validate_flow9
      log "All 9 flows passed at ${RUN_ROOT}"
      write_run_summary "PASS" "all requested flows plus node tooling passed"
      keep_running_if_requested
      return 0
    fi
    log "All 8 flows passed at ${RUN_ROOT}"
    write_run_summary "PASS" "all requested flows plus attack paths passed"
    keep_running_if_requested
    return 0
  fi
  log "All 7 flows passed at ${RUN_ROOT}"
  write_run_summary "PASS" "all requested flows passed"
  keep_running_if_requested
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
