#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

TAGS="${TAGS:-regtest mocknet}"
GO_BIN="${GO_BIN:-go}"
RUN_ROOT="${RUN_ROOT:-}"
LOCAL_RUN_ROOT="${LOCAL_RUN_ROOT:-${RUN_ROOT:-}}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/thornado}"
INVENTORY="${INVENTORY:-$ROOT_DIR/ops/distributed-regtest-nodeper.env}"
REMOTE_INVENTORY="${REMOTE_INVENTORY:-$REMOTE_ROOT/ops/distributed-regtest-nodeper.env}"
KNOWN_HOSTS="${KNOWN_HOSTS:-/tmp/thornado-hcloud-known-hosts}"
WORKER_NODES="${WORKER_NODES:-1 2 3 4}"
WORKER_HOSTS="${WORKER_HOSTS:-}"
COORDINATOR_HOST="${COORDINATOR_HOST:-}"
BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
API_BASE="${API_BASE:-2370}"
RPC_BASE="${RPC_BASE:-33360}"
FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"

ssh_base=(ssh -o BatchMode=yes -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new)
scp_base=(scp -q -o UserKnownHostsFile="$KNOWN_HOSTS" -o StrictHostKeyChecking=accept-new)

usage() {
  cat <<EOF
usage: ops/scripts/thornado-cluster.sh MODE ACTION

Canonical entrypoint for local and HCloud regtest clusters.
All Go builds use: -tags 'regtest mocknet'

local actions:
  local build                 build local thornado + bifrost
  local resume                resume an existing local real4/node5 run root
  local status                print local Thornado/Bifrost health
  local docker-up             start docker localnet wrapper
  local docker-down           stop docker localnet wrapper
  local docker-reset          remove docker localnet volumes/logs

cloud actions:
  cloud sync-ops             sync canonical ops scripts/docs to coordinator/workers
  cloud build                sync selected sources and build on coordinator
  cloud deploy                build on coordinator and install workers
  cloud deploy-restart        deploy and restart Bifrost workers
  cloud deploy-restart-all    deploy and restart Thornado then Bifrost
  cloud restart-bifrost       restart worker Bifrost only
  cloud restart-thornado      restart worker Thornado only
  cloud bootstrap             init a fresh node-per-server run root and start it
  cloud resume                restart processes against an existing run root
  cloud status                print HCloud Thornado/Bifrost health
  cloud test-flow3            run one parallel Flow 3 batch
  cloud test-remaining        run the edge/refund/fee/batch/fault harness
  cloud tail                  tail current worker logs

common env:
  RUN_ROOT                    required for cloud resume; generated for bootstrap if omitted
  INVENTORY                   local inventory file, default ops/distributed-regtest-nodeper.env
  REMOTE_ROOT                 default /root/thornado
  SOURCE_FILES                targeted source sync for deploy
  SKIP_SOURCE_SYNC            default inherited by hcloud-deploy-binaries.sh
  COUNT                       test-flow3 count, default 20
EOF
}

die() {
  echo "error: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

hash_files() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  else
    shasum -a 256 "$@"
  fi
}

load_inventory() {
  [[ -f "$INVENTORY" ]] || die "missing inventory: $INVENTORY"
  # shellcheck disable=SC1090
  source "$INVENTORY"
  COORDINATOR_HOST="${COORDINATOR_HOST:-${CONTROLLER_HOST:-}}"
  COORDINATOR_HOST="${COORDINATOR_HOST:?COORDINATOR_HOST missing from inventory}"
  BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
  BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
  API_BASE="${API_BASE:-2370}"
  RPC_BASE="${RPC_BASE:-33360}"
  FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
  if [[ -z "$WORKER_HOSTS" ]]; then
    local node key host hosts=()
    for node in $WORKER_NODES; do
      key="NODE${node}_HOST"
      host="${!key:-}"
      [[ -n "$host" ]] || die "${key} missing from inventory"
      hosts+=("$host")
    done
    WORKER_HOSTS="${hosts[*]}"
  fi
}

remote_quote() {
  printf '%q' "$1"
}

ssh_coord() {
  load_inventory
  "${ssh_base[@]}" "root@$COORDINATOR_HOST" "$@"
}

ssh_worker() {
  local host="$1"
  shift
  "${ssh_base[@]}" "root@$host" "$@"
}

cloud_run_root() {
  if [[ -n "$RUN_ROOT" ]]; then
    printf '%s\n' "$RUN_ROOT"
  else
    printf '/tmp/thornado-nodeper-%s\n' "$(date -u +%Y%m%d%H%M%S)"
  fi
}

local_build() {
  require_cmd "$GO_BIN"
  mkdir -p "$ROOT_DIR/build"
  (
    cd "$ROOT_DIR/go-thornado"
    "$GO_BIN" build -tags "$TAGS" -o "$ROOT_DIR/build/thornado" ./cmd/thornado
    "$GO_BIN" build -tags "$TAGS" -o "$ROOT_DIR/build/bifrost" ./cmd/bifrost
  )
  hash_files "$ROOT_DIR/build/thornado" "$ROOT_DIR/build/bifrost"
}

local_resume() {
  if [[ -z "$LOCAL_RUN_ROOT" ]]; then
    "$SCRIPT_DIR/resume-real4-cluster.sh"
  else
    RUN_ROOT="$LOCAL_RUN_ROOT" "$SCRIPT_DIR/resume-real4-cluster.sh"
  fi
}

local_status() {
  local run_root="${LOCAL_RUN_ROOT:-$RUN_ROOT}" node rpc info ok
  [[ -n "$run_root" ]] || run_root="$(ls -td /tmp/thornado-node5-churn-* /tmp/thornado-real4-* 2>/dev/null | head -1 || true)"
  [[ -n "$run_root" ]] || die "no local run root found"
  echo "run_root=$run_root"
  for node in 1 2 3 4 5; do
    rpc=$((RPC_BASE + node))
    info=$((FROST_INFO_BASE + node))
    ok="down"
    if curl -fsS --max-time 3 "http://127.0.0.1:${rpc}/status" >/dev/null 2>&1; then
      ok="rpc"
    fi
    if curl -fsS --max-time 3 "http://127.0.0.1:${info}/ping" >/dev/null 2>&1; then
      ok="${ok}+bifrost"
    fi
    echo "node${node} ${ok} rpc=${rpc} bifrost=${info}"
  done
}

cloud_deploy_action() {
  local action="$1"
  load_inventory
  COORDINATOR_HOST="$COORDINATOR_HOST" WORKER_HOSTS="$WORKER_HOSTS" REMOTE_ROOT="$REMOTE_ROOT" \
    WORKER_NODES="$WORKER_NODES" REMOTE_INVENTORY="$REMOTE_INVENTORY" \
    RUN_ROOT="${RUN_ROOT:-}" KNOWN_HOSTS="$KNOWN_HOSTS" TAGS="$TAGS" \
    BTC_RPC_PORT="$BTC_RPC_PORT" BTC_P2P_PORT="$BTC_P2P_PORT" \
    "$SCRIPT_DIR/hcloud-deploy-binaries.sh" "$action"
}

cloud_sync_ops() {
  local files
  files="${SOURCE_FILES:-ops/scripts/thornado-cluster.sh ops/scripts/distributed-regtest-cluster.sh ops/scripts/real-4node-e2e.sh ops/scripts/hcloud-deploy-binaries.sh ops/scripts/hcloud-parallel-flow3.sh ops/scripts/hcloud-continue-parallel-flow3.sh ops/scripts/hcloud-remaining-tests.sh ops/scripts/hcloud-edge-cases.sh ops/scripts/hcloud-fee-swing-test.sh ops/scripts/hcloud-fee-reschedule-test.sh ops/scripts/hcloud-refund-script-test.sh docs/thornado-cluster-runbook.md docs/flow-checks/flow-2-bonded-standby-node.md}"
  SKIP_SOURCE_SYNC=0 INCLUDE_UNTRACKED=1 SOURCE_FILES="$files" cloud_deploy_action build
}

cloud_init_controller() {
  local run_root="$1"
  ssh_coord "cd $(remote_quote "$REMOTE_ROOT") && INVENTORY=$(remote_quote "$REMOTE_INVENTORY") WORKER_NODES=$(remote_quote "$WORKER_NODES") RUN_ROOT=$(remote_quote "$run_root") SKIP_BUILD=1 BTC_RPC_PORT=$(remote_quote "$BTC_RPC_PORT") BTC_P2P_PORT=$(remote_quote "$BTC_P2P_PORT") ops/scripts/distributed-regtest-cluster.sh init-controller"
}

cloud_export_and_copy_bundles() {
  local run_root="$1" node host tmp bundle
  tmp="${TMPDIR:-/tmp}/thornado-worker-bundles-$(date -u +%Y%m%d%H%M%S)"
  mkdir -p "$tmp"
  ssh_coord "cd $(remote_quote "$REMOTE_ROOT") && INVENTORY=$(remote_quote "$REMOTE_INVENTORY") WORKER_NODES=$(remote_quote "$WORKER_NODES") RUN_ROOT=$(remote_quote "$run_root") ops/scripts/distributed-regtest-cluster.sh export-worker-bundles"
  for node in $WORKER_NODES; do
    host_var="NODE${node}_HOST"
    host="${!host_var}"
    bundle="$run_root/meta/worker-node${node}.tgz"
    "${scp_base[@]}" "root@$COORDINATOR_HOST:$bundle" "$tmp/worker-node${node}.tgz"
    ssh_worker "$host" "mkdir -p $(remote_quote "$run_root")"
    "${scp_base[@]}" "$tmp/worker-node${node}.tgz" "root@$host:$run_root/worker-node${node}.tgz"
    ssh_worker "$host" "tar -xzf $(remote_quote "$run_root/worker-node${node}.tgz") -C $(remote_quote "$run_root")"
  done
}

cloud_start_miner() {
  local run_root="$1"
  ssh_coord "bash -s -- $(remote_quote "$run_root") $(remote_quote "$BTC_RPC_PORT")" <<'REMOTE'
set -euo pipefail
run_root="$1"
btc_rpc_port="$2"
mkdir -p "$run_root/logs" "$run_root/pids"
pid_file="$run_root/pids/btc-auto-miner.pid"
if [[ -s "$pid_file" ]]; then
  old_pid="$(cat "$pid_file")"
  kill "$old_pid" 2>/dev/null || true
fi
nohup bash -c '
set -euo pipefail
run_root="$1"
btc_rpc_port="$2"
while true; do
  addr="$(bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$btc_rpc_port" -rpcuser=thornado -rpcpassword=thornado -rpcwallet=miner getnewaddress 2>/dev/null || true)"
  if [[ -n "$addr" ]]; then
    bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$btc_rpc_port" -rpcuser=thornado -rpcpassword=thornado -rpcwallet=miner generatetoaddress 1 "$addr" >/dev/null 2>&1 || true
  fi
  sleep 20
done
' bash "$run_root" "$btc_rpc_port" >"$run_root/logs/btc-auto-miner.log" 2>&1 &
echo "$!" >"$pid_file"
echo "btc-auto-miner pid=$(cat "$pid_file")"
REMOTE
}

cloud_start_controller() {
  local run_root="$1"
  ssh_coord "cd $(remote_quote "$REMOTE_ROOT") && INVENTORY=$(remote_quote "$REMOTE_INVENTORY") RUN_ROOT=$(remote_quote "$run_root") BTC_RPC_PORT=$(remote_quote "$BTC_RPC_PORT") BTC_P2P_PORT=$(remote_quote "$BTC_P2P_PORT") ops/scripts/distributed-regtest-cluster.sh start-controller-bitcoind"
  cloud_start_miner "$run_root"
}

cloud_worker_action_parallel() {
  local run_root="$1" action="$2" node host failures=0 tmp pid
  tmp="${TMPDIR:-/tmp}/thornado-cloud-${action}-$(date -u +%Y%m%d%H%M%S)"
  mkdir -p "$tmp"
  for node in $WORKER_NODES; do
    host_var="NODE${node}_HOST"
    host="${!host_var}"
    ssh_worker "$host" "cd $(remote_quote "$REMOTE_ROOT") && INVENTORY=$(remote_quote "$REMOTE_INVENTORY") RUN_ROOT=$(remote_quote "$run_root") NODE=$(remote_quote "$node") BTC_RPC_PORT=$(remote_quote "$BTC_RPC_PORT") BTC_P2P_PORT=$(remote_quote "$BTC_P2P_PORT") ops/scripts/distributed-regtest-cluster.sh $(remote_quote "$action")" >"$tmp/node${node}.log" 2>&1 &
    echo "$!" >"$tmp/node${node}.pid"
  done
  for node in $WORKER_NODES; do
    pid="$(cat "$tmp/node${node}.pid")"
    if wait "$pid"; then
      echo "${action} node${node} ok"
    else
      echo "${action} node${node} failed" >&2
      cat "$tmp/node${node}.log" >&2
      failures=$((failures + 1))
    fi
  done
  (( failures == 0 ))
}

cloud_wait_worker_thornado() {
  local node host_var host rpc start
  start="$(date +%s)"
  while true; do
    local ready=0
    for node in $WORKER_NODES; do
      host_var="NODE${node}_HOST"
      host="${!host_var}"
      rpc=$((RPC_BASE + node))
      if curl -fsS --max-time 4 "http://${host}:${rpc}/status" >/dev/null 2>&1; then
        ready=$((ready + 1))
      fi
    done
    if [[ "$ready" == "$(wc -w <<<"$WORKER_NODES" | tr -d ' ')" ]]; then
      echo "all worker Thornado RPC endpoints are reachable"
      return 0
    fi
    if (( "$(date +%s)" - start >= 180 )); then
      die "timed out waiting for worker Thornado RPC endpoints"
    fi
    sleep 2
  done
}

cloud_start_workers() {
  local run_root="$1"
  cloud_worker_action_parallel "$run_root" start-worker-bitcoind
  cloud_worker_action_parallel "$run_root" start-worker-thornado
  cloud_wait_worker_thornado
  cloud_worker_action_parallel "$run_root" start-worker-bifrost
}

cloud_bootstrap() {
  local run_root
  load_inventory
  run_root="$(cloud_run_root)"
  echo "cloud bootstrap run_root=$run_root"
  cloud_init_controller "$run_root"
  cloud_export_and_copy_bundles "$run_root"
  cloud_start_controller "$run_root"
  cloud_start_workers "$run_root"
  RUN_ROOT="$run_root" cloud_status
}

cloud_resume() {
  local run_root
  load_inventory
  [[ -n "$RUN_ROOT" ]] || die "RUN_ROOT is required for cloud resume"
  run_root="$RUN_ROOT"
  cloud_start_controller "$run_root"
  cloud_start_workers "$run_root"
  cloud_status
}

cloud_status() {
  load_inventory
  local node host_var host api rpc info api_state rpc_height signer_state queue_count
  echo "coordinator=$COORDINATOR_HOST run_root=${RUN_ROOT:-unknown}"
  for node in $WORKER_NODES; do
    host_var="NODE${node}_HOST"
    host="${!host_var}"
    api="http://${host}:$((API_BASE + node))"
    rpc="http://${host}:$((RPC_BASE + node))"
    info="http://${host}:$((FROST_INFO_BASE + node))"
    api_state="down"
    rpc_height="-"
    signer_state="down"
    queue_count="-"
    if curl -fsS --max-time 4 "$api/thornado/config" >/dev/null 2>&1; then
      api_state="up"
    fi
    rpc_height="$(curl -fsS --max-time 4 "$rpc/status" 2>/dev/null | jq -r '.result.sync_info.latest_block_height // "-"' 2>/dev/null || printf '-')"
    if curl -fsS --max-time 4 "$info/ping" >/dev/null 2>&1; then
      signer_state="up"
      queue_count="$(curl -fsS --max-time 4 "$info/debug/signer/txouts" 2>/dev/null | jq -r 'length' 2>/dev/null || printf '?')"
    fi
    echo "node${node} host=${host} api=${api_state} height=${rpc_height} bifrost=${signer_state} signer_queue=${queue_count}"
  done
}

cloud_test_flow3() {
  load_inventory
  [[ -n "$RUN_ROOT" ]] || die "RUN_ROOT is required"
  ssh_coord "cd $(remote_quote "$REMOTE_ROOT") && RUN_ROOT=$(remote_quote "$RUN_ROOT") ROOT_DIR=$(remote_quote "$REMOTE_ROOT") INVENTORY=$(remote_quote "$REMOTE_INVENTORY") COUNT=$(remote_quote "${COUNT:-20}") TX_INCLUSION_TIMEOUT=$(remote_quote "${TX_INCLUSION_TIMEOUT:-1200}") THORNADO_TX_TIMEOUT=$(remote_quote "${THORNADO_TX_TIMEOUT:-60}") ops/scripts/hcloud-parallel-flow3.sh"
}

cloud_test_remaining() {
  load_inventory
  [[ -n "$RUN_ROOT" ]] || die "RUN_ROOT is required"
  ssh_coord "cd $(remote_quote "$REMOTE_ROOT") && RUN_ROOT=$(remote_quote "$RUN_ROOT") ROOT_DIR=$(remote_quote "$REMOTE_ROOT") INVENTORY=$(remote_quote "$REMOTE_INVENTORY") TX_INCLUSION_TIMEOUT=$(remote_quote "${TX_INCLUSION_TIMEOUT:-1200}") THORNADO_TX_TIMEOUT=$(remote_quote "${THORNADO_TX_TIMEOUT:-60}") ops/scripts/hcloud-remaining-tests.sh"
}

cloud_tail() {
  load_inventory
  [[ -n "$RUN_ROOT" ]] || die "RUN_ROOT is required"
  local node host_var host lines="${LINES:-80}"
  for node in $WORKER_NODES; do
    host_var="NODE${node}_HOST"
    host="${!host_var}"
    echo "== node${node} ${host} thornado =="
    ssh_worker "$host" "run_root=$(remote_quote "$RUN_ROOT") node=$(remote_quote "$node") lines=$(remote_quote "$lines"); log=\"\$run_root/logs/thornado-\$node.log\"; [[ -s \"\$run_root/meta/thornado-\$node.current-log\" ]] && log=\"\$(cat \"\$run_root/meta/thornado-\$node.current-log\")\"; if [[ -s \"\$log\" ]]; then tail -n \"\$lines\" \"\$log\"; else ls -t \"\$run_root/logs/thornado-\$node\"* 2>/dev/null | head -n1 | xargs -r tail -n \"\$lines\"; fi"
    echo "== node${node} ${host} bifrost =="
    ssh_worker "$host" "run_root=$(remote_quote "$RUN_ROOT") node=$(remote_quote "$node") lines=$(remote_quote "$lines"); log=\"\$run_root/logs/bifrost-\$node.log\"; [[ -s \"\$run_root/meta/bifrost-\$node.current-log\" ]] && log=\"\$(cat \"\$run_root/meta/bifrost-\$node.current-log\")\"; if [[ -s \"\$log\" ]]; then tail -n \"\$lines\" \"\$log\"; else ls -t \"\$run_root/logs/bifrost-\$node\"* 2>/dev/null | head -n1 | xargs -r tail -n \"\$lines\"; fi"
  done
}

mode="${1:-}"
action="${2:-}"

case "$mode:$action" in
  help:|--help:|-h:) usage ;;
  local:build) local_build ;;
  local:resume) local_resume ;;
  local:status) local_status ;;
  local:docker-up) "$SCRIPT_DIR/localnet-up.sh" "${@:3}" ;;
  local:docker-down) "$SCRIPT_DIR/localnet-down.sh" "${@:3}" ;;
  local:docker-reset) "$SCRIPT_DIR/localnet-reset.sh" --logs ;;
  cloud:sync-ops) cloud_sync_ops ;;
  cloud:build) cloud_deploy_action build ;;
  cloud:deploy) cloud_deploy_action deploy ;;
  cloud:deploy-restart) cloud_deploy_action deploy-restart ;;
  cloud:deploy-restart-all) cloud_deploy_action deploy-restart-all ;;
  cloud:restart-bifrost) cloud_deploy_action restart-bifrost ;;
  cloud:restart-thornado) cloud_deploy_action restart-thornado ;;
  cloud:bootstrap) cloud_bootstrap ;;
  cloud:resume) cloud_resume ;;
  cloud:status) cloud_status ;;
  cloud:test-flow3) cloud_test_flow3 ;;
  cloud:test-remaining) cloud_test_remaining ;;
  cloud:tail) cloud_tail ;;
  :|*:help|*:-h|*:--help) usage ;;
  *)
    usage >&2
    exit 2
    ;;
esac
