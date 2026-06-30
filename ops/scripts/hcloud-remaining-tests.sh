#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260628131009}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
INVENTORY="${INVENTORY:-${ROOT_DIR}/ops/distributed-regtest-nodeper.env}"
RUN_ID="${RUN_ID:-$(date -u +%Y%m%d%H%M%S)}"
SKIP_EDGE="${SKIP_EDGE:-0}"
SKIP_REFUND="${SKIP_REFUND:-0}"
SKIP_FEE="${SKIP_FEE:-0}"
SKIP_BATCH="${SKIP_BATCH:-0}"
SKIP_BATCH_MIXED="${SKIP_BATCH_MIXED:-0}"
SKIP_BATCH_PRESSURE="${SKIP_BATCH_PRESSURE:-0}"
SKIP_IDEMPOTENCY="${SKIP_IDEMPOTENCY:-0}"
SKIP_DISRUPTIVE="${SKIP_DISRUPTIVE:-0}"
SKIP_REORG_PHASE="${SKIP_REORG_PHASE:-0}"
RESUME_SUITE="${RESUME_SUITE:-0}"
RESUME_BATCH="${RESUME_BATCH:-0}"
RUN_DISRUPTIVE="${RUN_DISRUPTIVE:-1}"
RUN_REORG="${RUN_REORG:-1}"
SSH_OPTS="${SSH_OPTS:--o BatchMode=yes -o StrictHostKeyChecking=accept-new}"

export RUN_ROOT
export BTC_USE_LOCAL=1
export BTC_RPC_HOST="${BTC_RPC_HOST:-127.0.0.1}"
export BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
export BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
export CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
export SIGNER_PASSWD="${SIGNER_PASSWD:-passphrase123}"
export TX_INCLUSION_TIMEOUT="${TX_INCLUSION_TIMEOUT:-1200}"
export THORNADO_TX_TIMEOUT="${THORNADO_TX_TIMEOUT:-60}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/ops/scripts/real-4node-e2e.sh"
trap - EXIT ERR

if [[ -f "$INVENTORY" ]]; then
  # shellcheck disable=SC1090
  source "$INVENTORY"
fi

API_BASE="${API_BASE:-2370}"
RPC_BASE="${RPC_BASE:-33360}"
FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
suite_dir="$RUN_ROOT/meta/remaining-tests/$RUN_ID"
mkdir -p "$suite_dir"
if [[ "$RESUME_SUITE" != "1" || ! -s "$suite_dir/results.csv" ]]; then
  printf 'phase,status,started_at,ended_at,detail\n' >"$suite_dir/results.csv"
fi

node_host() {
  local key="NODE${1}_HOST"
  printf '%s' "${!key:-}"
}

api_url() {
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((API_BASE + $1))"
}

rpc_url() {
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((RPC_BASE + $1))"
}

signer_url() {
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((FROST_INFO_BASE + $1))"
}

export THORNADO_TX_NODE="${THORNADO_TX_NODE:-tcp://$(node_host 1):$((RPC_BASE + 1))}"

ssh_worker() {
  local node="$1"
  shift
  # shellcheck disable=SC2086
  ssh $SSH_OPTS "root@$(node_host "$node")" "$@"
}

remote_btc_cli() {
  local node="$1"
  shift
  ssh_worker "$node" "bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport=${BTC_RPC_PORT} -rpcuser=thornado -rpcpassword=thornado $*"
}

csv_escape() {
  printf '%s' "$1" | sed 's/"/""/g; s/^/"/; s/$/"/'
}

record_phase() {
  local phase="$1" status="$2" started="$3" ended="$4" detail="$5"
  printf '%s,%s,%s,%s,%s\n' \
    "$(csv_escape "$phase")" \
    "$(csv_escape "$status")" \
    "$(csv_escape "$started")" \
    "$(csv_escape "$ended")" \
    "$(csv_escape "$detail")" >>"$suite_dir/results.csv"
}

snapshot_debug() {
  local label="$1" dir i base url
  dir="$suite_dir/debug-$label"
  mkdir -p "$dir"
  curl -fsS --max-time 8 "$(api_url 1)/thornado/config" >"$dir/config.json" || true
  curl -fsS --max-time 8 "$(api_url 1)/thornado/vaults/base" >"$dir/base-vaults.json" || true
  curl -fsS --max-time 8 "$(api_url 1)/thornado/vaults/solvency" >"$dir/vault-solvency.json" || true
  curl -fsS --max-time 8 "$(api_url 1)/thornado/txout/all" >"$dir/txout-all.json" || true
  for i in 1 2 3 4; do
    base="$(signer_url "$i")"
    for url in \
      ping \
      debug/health/full \
      debug/signer/txouts \
      debug/signer/performance \
      debug/frost/sessions; do
      curl -fsS --max-time 8 "$base/$url" >"$dir/node${i}-${url//\//-}.json" || true
    done
  done
}

health_check() {
  local i api rpc signer
  for i in 1 2 3 4; do
    api="$(api_url "$i")"
    rpc="$(rpc_url "$i")"
    signer="$(signer_url "$i")"
    curl -fsS --max-time 5 "$api/thornado/config" >/dev/null
    curl -fsS --max-time 5 "$rpc/status" >/dev/null
    curl -fsS --max-time 5 "$signer/ping" >/dev/null
  done
}

assert_signer_queues_empty() {
  local i file start all_empty
  start="$(date +%s)"
  while true; do
    all_empty=1
    for i in 1 2 3 4; do
      file="$suite_dir/node${i}-final-txouts.json"
      curl -fsS --max-time 8 "$(signer_url "$i")/debug/signer/txouts" >"$file" || all_empty=0
      jq -e 'length == 0' "$file" >/dev/null || all_empty=0
    done
    if (( all_empty == 1 )); then
      return 0
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "signer queues were not empty"
    fi
    sleep 2
  done
}

assert_cluster_healthy() {
  health_check
  assert_signer_queues_empty
}

restart_thornado_node() {
  local node="$1"
  ssh_worker "$node" "bash -s" <<REMOTE
set -euo pipefail
remote_root="$ROOT_DIR"
run_root="$RUN_ROOT"
node="$node"
build_id="remaining-thornado-\$(date -u +%Y%m%d%H%M%S)"
cmd_file="\$run_root/meta/thornado-\$node.restart.cmd"
env_file="\$run_root/meta/thornado-\$node.restart.env"
log="\$run_root/logs/thornado-\$node.restart-\$build_id.log"
home="\$run_root/node\$node"
pid="\$(pgrep -f "\$remote_root/build/thornado start --home \$home" | head -n1 || true)"
if [[ -n "\$pid" ]]; then
  mkdir -p "\$run_root/logs" "\$run_root/meta"
  cp "/proc/\$pid/cmdline" "\$cmd_file"
  cp "/proc/\$pid/environ" "\$env_file"
  kill -TERM "\$pid" 2>/dev/null || true
  for _ in \$(seq 1 30); do
    kill -0 "\$pid" 2>/dev/null || break
    sleep 0.5
  done
  kill -KILL "\$pid" 2>/dev/null || true
fi
[[ -s "\$cmd_file" && -s "\$env_file" ]] || { echo "missing Thornado restart files for node \$node" >&2; exit 1; }
mapfile -d '' env_args <"\$env_file"
mapfile -d '' cmd_args <"\$cmd_file"
nohup env "\${env_args[@]}" "\${cmd_args[@]}" >>"\$log" 2>&1 &
echo "\$!" >"\$run_root/meta/thornado-\$node.wrapper.pid"
echo "\$log" >"\$run_root/meta/thornado-\$node.current-log"
rpc_port=$((RPC_BASE + node))
for _ in \$(seq 1 90); do
  if curl -fsS "http://127.0.0.1:\$rpc_port/status" >/dev/null; then
    echo "thornado-\$node restarted log=\$log"
    exit 0
  fi
  sleep 1
done
tail -n 120 "\$log" >&2 || true
exit 1
REMOTE
}

restart_bifrost_node() {
  local node="$1"
  ssh_worker "$node" "bash -s" <<REMOTE
set -euo pipefail
remote_root="$ROOT_DIR"
run_root="$RUN_ROOT"
node="$node"
build_id="remaining-bifrost-\$(date -u +%Y%m%d%H%M%S)"
env_file="\$run_root/meta/bifrost-\$node.restart.env"
log="\$run_root/logs/bifrost-\$node.restart-\$build_id.log"
pid="\$(pgrep -f "\$remote_root/build/bifrost --log-level" | head -n1 || true)"
if [[ -n "\$pid" && ! -s "\$env_file" ]]; then
  cp "/proc/\$pid/environ" "\$env_file"
fi
if [[ -n "\$pid" ]]; then
  kill -TERM "\$pid" 2>/dev/null || true
  for _ in \$(seq 1 30); do
    kill -0 "\$pid" 2>/dev/null || break
    sleep 0.5
  done
  kill -KILL "\$pid" 2>/dev/null || true
fi
[[ -s "\$env_file" ]] || { echo "missing Bifrost restart env for node \$node" >&2; exit 1; }
mkdir -p "\$run_root/logs" "\$run_root/meta"
nohup xargs -0 -a "\$env_file" sh -c 'exec env "$@" /root/thornado/build/bifrost --log-level debug' sh >>"\$log" 2>&1 &
echo "\$!" >"\$run_root/meta/bifrost-\$node.wrapper.pid"
echo "\$log" >"\$run_root/meta/bifrost-\$node.current-log"
port=$((FROST_INFO_BASE + node))
for _ in \$(seq 1 90); do
  if curl -fsS "http://127.0.0.1:\$port/ping" >/dev/null; then
    echo "bifrost-\$node restarted log=\$log"
    exit 0
  fi
  sleep 1
done
tail -n 120 "\$log" >&2 || true
exit 1
REMOTE
}

restart_bitcoind_node() {
  local node="$1"
  ssh_worker "$node" "bash -s" <<REMOTE
set -euo pipefail
run_root="$RUN_ROOT"
node="$node"
controller="${COORDINATOR_HOST:-5.223.51.101}"
pid="\$(pgrep -f "bitcoind -datadir=\$run_root/bitcoind" | head -n1 || true)"
if [[ -n "\$pid" ]]; then
  kill -TERM "\$pid" 2>/dev/null || true
  for _ in \$(seq 1 30); do
    kill -0 "\$pid" 2>/dev/null || break
    sleep 0.5
  done
  kill -KILL "\$pid" 2>/dev/null || true
fi
mkdir -p "\$run_root/bitcoind" "\$run_root/logs" "\$run_root/pids"
nohup bitcoind \
  -datadir="\$run_root/bitcoind" -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
  -deprecatedrpc=create_bdb -rpcbind=0.0.0.0 -rpcallowip=0.0.0.0/0 \
  -rpcport="$BTC_RPC_PORT" -port="$BTC_P2P_PORT" \
  -rpcuser=thornado -rpcpassword=thornado \
  -connect="\$controller:$BTC_P2P_PORT" \
  >"\$run_root/logs/bitcoind-node\${node}.restart.log" 2>&1 &
echo "\$!" >"\$run_root/pids/bitcoind-node\${node}.pid"
for _ in \$(seq 1 90); do
  if bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado getblockchaininfo >/dev/null 2>&1; then
    bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado loadwallet "bifrost\${node}" >/dev/null 2>&1 || true
    echo "bitcoind-\$node restarted"
    exit 0
  fi
  sleep 1
done
tail -n 120 "\$run_root/logs/bitcoind-node\${node}.restart.log" >&2 || true
exit 1
REMOTE
}

partition_frost_node() {
  local node="$1" seconds="${2:-20}"
  ssh_worker "$node" "bash -s" <<REMOTE
set -euo pipefail
port=$((FROST_P2P_BASE + node))
cleanup() {
  iptables -D INPUT -p tcp --dport "\$port" -j DROP 2>/dev/null || true
  iptables -D OUTPUT -p tcp --sport "\$port" -j DROP 2>/dev/null || true
  iptables -D OUTPUT -p tcp --dport "\$port" -j DROP 2>/dev/null || true
}
trap cleanup EXIT
cleanup
iptables -I INPUT 1 -p tcp --dport "\$port" -j DROP
iptables -I OUTPUT 1 -p tcp --sport "\$port" -j DROP
iptables -I OUTPUT 1 -p tcp --dport "\$port" -j DROP
sleep "$seconds"
cleanup
REMOTE
}

run_phase() {
  local phase="$1" started ended detail rc
  shift
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  log "remaining ${phase}: start"
  snapshot_debug "${phase}-before"
  set +e
  "$@" >"$suite_dir/${phase}.log" 2>&1
  rc=$?
  set -e
  ended="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  snapshot_debug "${phase}-after"
  if (( rc != 0 )); then
    detail="rc=${rc} log=$suite_dir/${phase}.log"
    record_phase "$phase" "fail" "$started" "$ended" "$detail"
    tail -n 120 "$suite_dir/${phase}.log" >&2 || true
    die "remaining ${phase} failed: ${detail}"
  fi
  detail="log=$suite_dir/${phase}.log"
  record_phase "$phase" "pass" "$started" "$ended" "$detail"
  log "remaining ${phase}: pass"
}

validate_parallel_batch() {
  local dir="$1" expected="$2" label="$3" unique_out first_txout tx_array_count out_hash
  jq -e --argjson expected "$expected" '.requested == $expected and .success == $expected and .failed == 0' "$dir/summary.json" >/dev/null
  [[ "$(tail -n +2 "$dir/results.csv" | wc -l | tr -d ' ')" == "$expected" ]] || die "${label} results row count mismatch"
  unique_out="$(tail -n +2 "$dir/results.csv" | awk -F, '{print $4}' | sort -u | wc -l | tr -d ' ')"
  [[ "$unique_out" == "1" ]] || die "${label} expected one outbound batch, got ${unique_out}"
  out_hash="$(tail -n +2 "$dir/results.csv" | awk -F, 'NR==1{print $4}')"
  first_txout="$(find "$dir" -maxdepth 1 -name '*-out-txout.json' -print | head -n1)"
  [[ -n "$first_txout" ]] || die "${label} missing txout artifact"
  tx_array_count="$(jq --arg out_hash "$out_hash" '[.txout.tx_array[] | select(.out_hash == $out_hash)] | length' "$first_txout")"
  [[ "$tx_array_count" == "$expected" ]] || die "${label} tx_array count ${tx_array_count} != ${expected}"
  jq -e --arg out_hash "$out_hash" --argjson expected "$expected" '
    [.txout.tx_array[] | select(.out_hash == $out_hash)] as $items |
    ($items | length) == $expected and
    all($items[]; ((.source_inputs // []) | length) > 0 and ((.max_gas // []) | length) > 0 and ((.gas_rate // 0) > 0))
  ' "$first_txout" >/dev/null || die "${label} missing source inputs/max gas/gas rate"
}

run_flow_with_outbound_fault() {
  local label="$1"
  shift
  local dir="$suite_dir/$label" pid start
  mkdir -p "$dir"
  RUN_DIR="$dir" COUNT=4 "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh" >"$dir/run.log" 2>&1 &
  pid=$!
  start="$(date +%s)"
  while ! grep -q "waiting for outbounds" "$dir/run.log" 2>/dev/null; do
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid"
      return $?
    fi
    if (( "$(date +%s)" - start >= 900 )); then
      kill "$pid" 2>/dev/null || true
      die "${label} did not reach outbound wait"
    fi
    sleep 1
  done
  "$@"
  wait "$pid"
  validate_parallel_batch "$dir" 4 "$label"
  assert_cluster_healthy
}

run_thornado_restart_fault() {
  run_flow_with_outbound_fault "fault-thornado4-restart" restart_thornado_node 4
}

run_bitcoind_restart_fault() {
  run_flow_with_outbound_fault "fault-bitcoind4-restart" restart_bitcoind_node 4
}

run_frost_partition_fault() {
  run_flow_with_outbound_fault "fault-frost4-partition" partition_frost_node 4 20
}

run_round1_drop_fault() {
  local dir="$suite_dir/fault-round1-drop-node4" pid start triggered=0 baseline_id current_id
  mkdir -p "$dir"
  baseline_id="$(curl -fsS --max-time 2 "$(signer_url 4)/debug/frost/sessions" 2>/dev/null | jq -r '[.[] | select(.type == "keysign")][-1].id // ""' || true)"
  printf '%s\n' "$baseline_id" >"$dir/baseline-session-id.txt"
  RUN_DIR="$dir" COUNT=4 "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh" >"$dir/run.log" 2>&1 &
  pid=$!
  start="$(date +%s)"
  while kill -0 "$pid" 2>/dev/null; do
    if curl -fsS --max-time 2 "$(signer_url 4)/debug/frost/sessions" >"$dir/node4-sessions-latest.json" 2>/dev/null &&
      current_id="$(jq -r '[.[] | select(.type == "keysign")][-1].id // ""' "$dir/node4-sessions-latest.json")" &&
      [[ -n "$current_id" && "$current_id" != "$baseline_id" ]] &&
      jq -e '
        [.[] | select(.type == "keysign")][-1] |
        ((.broadcasts.sign_round1 // 0) > 0 or any((.phases // [])[]; .event == "broadcast" and .kind == "sign_round1"))
      ' "$dir/node4-sessions-latest.json" >/dev/null 2>&1; then
      triggered=1
      restart_bifrost_node 4
      break
    fi
    if (( "$(date +%s)" - start >= 900 )); then
      kill "$pid" 2>/dev/null || true
      die "round1-drop did not observe sign_round1"
    fi
    sleep 0.1
  done
  wait "$pid"
  [[ "$triggered" == "1" ]] || die "round1-drop flow exited before trigger"
  validate_parallel_batch "$dir" 4 "fault-round1-drop-node4"
  assert_cluster_healthy
}

run_batch_mixed_duplicate() {
  local dir="$suite_dir/batch-mixed-duplicate-24"
  local amounts="0.20000000,0.21000000,0.22000000,0.23000000,0.20000000,0.21000000,0.22000000,0.23000000,0.20000000,0.21000000,0.22000000,0.23000000,0.20000000,0.21000000,0.22000000,0.23000000,0.20000000,0.21000000,0.22000000,0.23000000,0.20000000,0.21000000,0.22000000,0.23000000"
  mkdir -p "$dir"
  if [[ "$RESUME_BATCH" == "1" && -s "$dir/shield-txhashes.txt" && ! -s "$dir/summary.json" ]]; then
    RUN_DIR="$dir" COUNT=24 DUPLICATE_RECIPIENT=1 \
      "$ROOT_DIR/ops/scripts/hcloud-continue-parallel-flow3.sh"
  else
    RUN_DIR="$dir" COUNT=24 DEPOSIT_AMOUNTS_CSV="$amounts" DUPLICATE_RECIPIENT=1 \
      "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh"
  fi
  validate_parallel_batch "$dir" 24 "batch-mixed-duplicate-24"
}

run_batch_pressure() {
  local dir="$suite_dir/batch-pressure-32"
  mkdir -p "$dir"
  if [[ "$RESUME_BATCH" == "1" && -s "$dir/shield-txhashes.txt" && ! -s "$dir/summary.json" ]]; then
    RUN_DIR="$dir" COUNT=32 "$ROOT_DIR/ops/scripts/hcloud-continue-parallel-flow3.sh"
  else
    RUN_DIR="$dir" COUNT=32 "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh"
  fi
  validate_parallel_batch "$dir" 32 "batch-pressure-32"
}

run_refund_scripts() {
  bash "$ROOT_DIR/ops/scripts/hcloud-refund-script-test.sh"
  assert_cluster_healthy
}

run_fee_swing() {
  COUNT=8 FEE_RATE_SAT_VB="${FEE_RATE_SAT_VB:-3000}" bash "$ROOT_DIR/ops/scripts/hcloud-fee-reschedule-test.sh"
  assert_cluster_healthy
}

raw_observation_from_tx_query() {
  local tx_file="$1"
  jq -c '
    (.observed_tx // .tx // empty) as $obs |
    [{
      tx: $obs.tx,
      block_height: ($obs.external_observed_height // $obs.block_height // $obs.finalise_height // 0),
      observed_pub_key: $obs.observed_pub_key,
      finalise_height: ($obs.finalise_height // $obs.external_observed_height // $obs.block_height // 0)
    }]
  ' "$tx_file"
}

repeat_observe() {
  local direction="$1" txid="$2" label="$3" query raw i out tx_json
  query="$suite_dir/${label}-tx-query.json"
  curl -fsS "$(api_url 1)/thornado/tx/${txid}" >"$query"
  raw="$(raw_observation_from_tx_query "$query")"
  printf '%s\n' "$raw" >"$suite_dir/${label}-raw-observation.json"
  for i in 1 2 3 4; do
    if [[ "$direction" == "out" ]]; then
      out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" observe-tx-outs --raw-observations "$raw")"
    else
      out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" observe-tx-ins --raw-observations "$raw")"
    fi
    printf '%s\n' "$out" >"$suite_dir/${label}-node${i}.json"
    tx_json="$(printf '%s\n' "$out" | tail -n1)"
    assert_tx_success "$tx_json" "${label} node${i}"
  done
  wait_blocks 2
  curl -fsS "$(api_url 1)/thornado/tx/${txid}" >"$suite_dir/${label}-tx-query-after.json"
}

run_idempotency_reobserve() {
  local batch_dir="$suite_dir/batch-mixed-duplicate-24" out_hash deposit_id
  out_hash="$(tail -n +2 "$batch_dir/results.csv" | awk -F, 'NR==1{print $4}')"
  deposit_id="$(tail -n +2 "$batch_dir/results.csv" | awk -F, 'NR==1{print $2}')"
  [[ -n "$out_hash" && -n "$deposit_id" ]] || die "idempotency missing batch artifacts"
  repeat_observe in "$deposit_id" "repeat-observe-inbound"
  repeat_observe out "$out_hash" "repeat-observe-outbound"
  assert_signer_queues_empty
}

select_miner_utxo() {
  local min_btc="$1"
  btc_cli -rpcwallet=miner listunspent 1 9999999 |
    jq -c --argjson min "$min_btc" '[.[] | select((.spendable // true) == true and (.amount | tonumber) > $min)] | sort_by(.amount) | .[0]'
}

json_btc_amount() {
  local amount="$1"
  jq -cn --arg amount "$amount" '$amount | tonumber'
}

generate_empty_block() {
  btc_cli -rpcwallet=miner generateblock "$(btc_cli -rpcwallet=miner getnewaddress)" "[]" | jq -r '.hash'
}

stop_auto_miner() {
  local pid
  pid="$(cat "$RUN_ROOT/pids/btc-auto-miner.pid" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    for _ in $(seq 1 20); do
      kill -0 "$pid" 2>/dev/null || break
      sleep 0.5
    done
    kill -KILL "$pid" 2>/dev/null || true
  fi
}

start_auto_miner() {
  local pid
  pid="$(cat "$RUN_ROOT/pids/btc-auto-miner.pid" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  mkdir -p "$RUN_ROOT/pids" "$RUN_ROOT/logs"
  nohup bash -c 'while true; do sleep 20; addr=$("bitcoin-cli" -regtest -rpcconnect=127.0.0.1 -rpcport="'"$BTC_RPC_PORT"'" -rpcuser=thornado -rpcpassword=thornado -rpcwallet=miner getnewaddress 2>/dev/null) || continue; block=$("bitcoin-cli" -regtest -rpcconnect=127.0.0.1 -rpcport="'"$BTC_RPC_PORT"'" -rpcuser=thornado -rpcpassword=thornado -rpcwallet=miner generatetoaddress 1 "$addr" 2>/dev/null | jq -r ".[0] // empty" || true); [ -n "$block" ] && printf "[%s] mined %s\n" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$block"; done' \
    >>"$RUN_ROOT/logs/btc-auto-miner.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/btc-auto-miner.pid"
}

wait_worker_btc_catchup() {
  local target="$1" start node height
  start="$(date +%s)"
  while true; do
    local ok=1
    for node in 1 2 3 4; do
      height="$(ssh_worker "$node" "bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport=$BTC_RPC_PORT -rpcuser=thornado -rpcpassword=thornado getblockchaininfo" | jq -r '.blocks // 0' || printf '0')"
      if (( height < target )); then
        ok=0
      fi
    done
    (( ok == 1 )) && return 0
    if (( "$(date +%s)" - start >= 180 )); then
      die "worker bitcoinds did not catch up to BTC height ${target}"
    fi
    sleep 2
  done
}

wait_owner_deposit_matched_no_mine() {
  local owner="$1" timeout="${2:-120}" start session deposit_id
  start="$(date +%s)"
  while true; do
    session="$(deposit_session "$owner" 2>/dev/null || true)"
    if [[ -n "$session" ]] && jq -e '.status == "deposit_matched" and (.deposit_id // "") != ""' <<<"$session" >/dev/null 2>&1; then
      printf '%s\n' "$session"
      return 0
    fi
    deposit_id="$(jq -r '.deposit_id // ""' <<<"$session" 2>/dev/null || true)"
    if [[ -n "$deposit_id" && "$deposit_id" != "null" ]]; then
      printf '%s\n' "$session"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      printf '%s\n' "$session" >&2
      die "deposit did not match before reorg"
    fi
    sleep 1
  done
}

wait_deposit_errata_or_unmatched() {
  local deposit_id="$1" owner="$2" out_file="$3" timeout="${4:-300}" start record session status
  start="$(date +%s)"
  while true; do
    record="$(curl_json_quiet "$(api_url 1)/thornado/deposit/${deposit_id}" || true)"
    session="$(deposit_session "$owner" 2>/dev/null || true)"
    status="$(jq -r '.status // ""' <<<"$record" 2>/dev/null || true)"
    printf '%s\n' "$record" >"$out_file"
    printf '%s\n' "$session" >"${out_file%.json}-session.json"
    if [[ "$status" == "errata" ]] ||
      jq -e --arg id "$deposit_id" '(.deposit_id // "") != $id or (.status // "") == "address_issued"' <<<"$session" >/dev/null 2>&1; then
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "deposit ${deposit_id} did not errata/unmatch after BTC reorg; status=${status}"
    fi
    sleep 2
  done
}

run_deposit_reorg_errata() {
  local dir="$suite_dir/reorg-deposit-observed-out" label owner deposit_pubkey session addr utxo in_txid in_vout in_amount fee deposit_amount change_addr change conflict_addr refund_amount inputs deposit_outputs conflict_outputs deposit_raw conflict_raw deposit_signed conflict_signed deposit_txid conflict_hex block_hash replacement_hash target_height matched deposit_id
  [[ "$RUN_REORG" == "1" ]] || { printf '{"skipped":true,"reason":"RUN_REORG is not 1"}\n' >"$suite_dir/reorg-skipped.json"; return 0; }
  dir="$suite_dir/reorg-deposit-observed-out"
  mkdir -p "$dir"
  stop_auto_miner
  trap 'start_auto_miner' EXIT

  label="remaining-reorg-$(date +%s)"
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${label}-deposit-pubkey")"
  owner="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  printf '%s\n' "$deposit_pubkey" >"$dir/deposit-pubkey.txt"
  printf '%s\n' "$owner" >"$dir/owner-address.txt"
  request_deposit "$RUN_ROOT/node1" "user" "$label" "$deposit_pubkey" >"$dir/request-deposit.json"
  session="$(deposit_session "$owner")"
  printf '%s\n' "$session" >"$dir/session-before-btc.json"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  [[ -n "$addr" && "$addr" != "null" ]] || die "reorg test did not get deposit address"

  utxo="$(select_miner_utxo 1)"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00002000"
  deposit_amount="0.20000000"
  change="$(awk -v input="$in_amount" -v amount="$deposit_amount" -v fee="$fee" 'BEGIN {c = input - amount - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  refund_amount="$(awk -v input="$in_amount" -v fee="$fee" 'BEGIN {c = input - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  change_addr="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  conflict_addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  deposit_outputs="$(jq -nc --arg deposit "$addr" --arg change_addr "$change_addr" --argjson amount "$(json_btc_amount "$deposit_amount")" --argjson change "$(json_btc_amount "$change")" '[{($deposit):$amount},{($change_addr):$change}]')"
  conflict_outputs="$(jq -nc --arg addr "$conflict_addr" --argjson amount "$(json_btc_amount "$refund_amount")" '[{($addr):$amount}]')"
  printf '%s\n' "$inputs" >"$dir/inputs.json"
  printf '%s\n' "$deposit_outputs" >"$dir/deposit-outputs.json"
  printf '%s\n' "$conflict_outputs" >"$dir/conflict-outputs.json"
  deposit_raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$deposit_outputs")"
  conflict_raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$conflict_outputs")"
  deposit_signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$deposit_raw")"
  conflict_signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$conflict_raw")"
  jq -e '.complete == true' <<<"$deposit_signed" >/dev/null || die "reorg deposit tx did not sign"
  jq -e '.complete == true' <<<"$conflict_signed" >/dev/null || die "reorg conflict tx did not sign"
  printf '%s\n' "$deposit_signed" >"$dir/deposit-signed.json"
  printf '%s\n' "$conflict_signed" >"$dir/conflict-signed.json"
  conflict_hex="$(jq -r '.hex' <<<"$conflict_signed")"
  deposit_txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$deposit_signed")" 0)"
  deposit_id="$(printf '%s' "$deposit_txid" | tr '[:lower:]' '[:upper:]')"
  printf '%s\n' "$deposit_txid" >"$dir/deposit-txid.txt"
  printf '%s\n' "$deposit_id" >"$dir/deposit-id.txt"

  block_hash="$(btc_cli -rpcwallet=miner generateblock "$(btc_cli -rpcwallet=miner getnewaddress)" "$(jq -nc --arg tx "$deposit_txid" '[$tx]')" | jq -r '.hash')"
  printf '%s\n' "$block_hash" >"$dir/deposit-block-hash.txt"
  target_height="$(btc_cli getblockchaininfo | jq -r '.blocks')"
  wait_worker_btc_catchup "$target_height"

  matched="$(wait_owner_deposit_matched_no_mine "$owner" 180)"
  printf '%s\n' "$matched" >"$dir/deposit-matched-before-reorg.json"
  jq -e --arg id "$deposit_id" '.deposit_id == $id' <<<"$matched" >/dev/null || die "reorg matched the wrong deposit"

  btc_cli invalidateblock "$block_hash"
  replacement_hash="$(btc_cli -rpcwallet=miner generateblock "$(btc_cli -rpcwallet=miner getnewaddress)" "$(jq -nc --arg tx "$conflict_hex" '[$tx]')" | jq -r '.hash')"
  printf '%s\n' "$replacement_hash" >"$dir/conflict-block-hash.txt"
  for _ in $(seq 1 8); do
    generate_empty_block >>"$dir/replacement-blocks.txt"
  done
  target_height="$(btc_cli getblockchaininfo | jq -r '.blocks')"
  wait_worker_btc_catchup "$target_height"
  wait_deposit_errata_or_unmatched "$deposit_id" "$owner" "$dir/deposit-after-reorg.json" 420
  assert_cluster_healthy
  start_auto_miner
  trap - EXIT
}

run_disruptive_faults() {
  if [[ "$RUN_DISRUPTIVE" != "1" ]]; then
    printf '{"skipped":true,"reason":"RUN_DISRUPTIVE is not 1"}\n' >"$suite_dir/disruptive-skipped.json"
    return 0
  fi
  run_thornado_restart_fault
  run_bitcoind_restart_fault
  run_frost_partition_fault
  run_round1_drop_fault
}

main() {
  health_check
  snapshot_debug preflight
  if [[ "$SKIP_EDGE" != "1" ]]; then
    run_phase edge-tx-shapes "$ROOT_DIR/ops/scripts/hcloud-edge-cases.sh"
  fi
  if [[ "$SKIP_REFUND" != "1" ]]; then
    run_phase refund-scripts run_refund_scripts
  fi
  if [[ "$SKIP_FEE" != "1" ]]; then
    run_phase fee-swing run_fee_swing
  fi
  if [[ "$SKIP_BATCH" != "1" ]]; then
    if [[ "$SKIP_BATCH_MIXED" != "1" ]]; then
      run_phase batch-mixed-duplicate-24 run_batch_mixed_duplicate
    fi
    if [[ "$SKIP_BATCH_PRESSURE" != "1" ]]; then
      run_phase batch-pressure-32 run_batch_pressure
    fi
  fi
  if [[ "$SKIP_IDEMPOTENCY" != "1" ]]; then
    run_phase idempotency-reobserve run_idempotency_reobserve
  fi
  if [[ "$SKIP_DISRUPTIVE" != "1" ]]; then
    run_phase disruptive-faults run_disruptive_faults
  fi
  if [[ "$SKIP_REORG_PHASE" != "1" ]]; then
    run_phase reorg-deposit-errata run_deposit_reorg_errata
  fi
  assert_signer_queues_empty
  snapshot_debug final
  jq -n --arg run_dir "$suite_dir" --arg status pass '{run_dir:$run_dir,status:$status}' >"$suite_dir/summary.json"
  cat "$suite_dir/summary.json"
}

main "$@"
