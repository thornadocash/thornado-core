#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-${1:-$(ls -td /tmp/thornado-node5-churn-* /tmp/thornado-real4-* 2>/dev/null | head -1)}}"
if [[ -z "${RUN_ROOT:-}" || ! -d "$RUN_ROOT" ]]; then
  echo "RUN_ROOT not found" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
BUILD_DIR="${BUILD_DIR:-$REPO_ROOT/build}"
THORNADO="${THORNADO:-$BUILD_DIR/thornado}"
BIFROST="${BIFROST:-$BUILD_DIR/bifrost}"

CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
BTC_START_BLOCK_HEIGHT="${BTC_START_BLOCK_HEIGHT:-0}"
API_BASE="${API_BASE:-2370}"
GRPC_BASE="${GRPC_BASE:-13380}"
RPC_BASE="${RPC_BASE:-33360}"
P2P_BASE="${P2P_BASE:-33380}"
EBIFROST_BASE="${EBIFROST_BASE:-58600}"
FROST_P2P_BASE="${FROST_P2P_BASE:-9340}"
FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
METRICS_BASE="${METRICS_BASE:-14200}"
LAUNCH_LABEL="${LAUNCH_LABEL:-com.thornado.real4.$(basename "$RUN_ROOT" | tr -c '[:alnum:]' '_')}"

api_port() { echo $((API_BASE + $1)); }
grpc_port() { echo $((GRPC_BASE + $1)); }
rpc_port() { echo $((RPC_BASE + $1)); }
p2p_port() { echo $((P2P_BASE + $1)); }
ebifrost_port() { echo $((EBIFROST_BASE + $1)); }
frost_p2p_port() { echo $((FROST_P2P_BASE + $1)); }
frost_info_port() { echo $((FROST_INFO_BASE + $1)); }
metrics_port() { echo $((METRICS_BASE + $1)); }

wait_url() {
  local url="$1" label="$2" timeout="${3:-120}" start
  start="$(date +%s)"
  while true; do
    if curl --connect-timeout 2 --max-time 5 -fsS "$url" >/dev/null 2>&1; then
      echo "[resume-real4] $label ready"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      echo "[resume-real4] timed out waiting for $label: $url" >&2
      return 1
    fi
    sleep 1
  done
}

restart_bifrost() {
  local i="$1" pid_file="$RUN_ROOT/pids/bifrost-${i}.pid" pid
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    echo "[resume-real4] restarting bifrost-$i for failed health"
    kill "$pid" >/dev/null 2>&1 || true
  fi
}

stop_run_children() {
  local pid_file pid cmd
  shopt -s nullglob
  for pid_file in "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid; do
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    [[ -z "$pid" ]] && continue
    cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    case "$pid_file:$cmd" in
      *"/pids/btc-miner.pid:"*|*"$BUILD_DIR/thornado start"*|*"$BUILD_DIR/bifrost"*) kill "$pid" >/dev/null 2>&1 || true ;;
    esac
  done
  sleep 2
  for pid_file in "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid; do
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    [[ -z "$pid" ]] && continue
    cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    case "$pid_file:$cmd" in
      *"/pids/btc-miner.pid:"*|*"$BUILD_DIR/thornado start"*|*"$BUILD_DIR/bifrost"*) kill -9 "$pid" >/dev/null 2>&1 || true ;;
    esac
  done
  shopt -u nullglob
}

launch_runner() {
  local runner="$1" runner_pid
  if command -v launchctl >/dev/null 2>&1 && [[ "${DISABLE_LAUNCHCTL:-}" != "1" ]]; then
    launchctl remove "$LAUNCH_LABEL" >/dev/null 2>&1 || true
    launchctl submit -l "$LAUNCH_LABEL" \
      -o "$RUN_ROOT/logs/launchd-runner.out" \
      -e "$RUN_ROOT/logs/launchd-runner.err" \
      -- /usr/bin/env RUN_ROOT="$RUN_ROOT" REPO_ROOT="$REPO_ROOT" BUILD_DIR="$BUILD_DIR" THORNADO="$THORNADO" BIFROST="$BIFROST" \
        CHAIN_ID="$CHAIN_ID" SIGNER_PASSWD="$PASS" BTC_RPC_PORT="$BTC_RPC_PORT" BTC_START_BLOCK_HEIGHT="$BTC_START_BLOCK_HEIGHT" API_BASE="$API_BASE" GRPC_BASE="$GRPC_BASE" \
        RPC_BASE="$RPC_BASE" P2P_BASE="$P2P_BASE" EBIFROST_BASE="$EBIFROST_BASE" FROST_P2P_BASE="$FROST_P2P_BASE" \
        FROST_INFO_BASE="$FROST_INFO_BASE" METRICS_BASE="$METRICS_BASE" "$runner"
    sleep 2
    runner_pid="$(launchctl list "$LAUNCH_LABEL" 2>/dev/null | awk -F'= ' '/"PID"/ {gsub(/;/, "", $2); print $2}')"
    if [[ -z "$runner_pid" ]]; then
      echo "[resume-real4] launchd runner failed: $LAUNCH_LABEL" >&2
      launchctl list "$LAUNCH_LABEL" 2>/dev/null >&2 || true
      exit 1
    fi
    echo "$runner_pid" >"$RUN_ROOT/pids/runner.pid"
    echo "[resume-real4] launchd runner $LAUNCH_LABEL pid=$runner_pid"
    return 0
  fi

  nohup env RUN_ROOT="$RUN_ROOT" REPO_ROOT="$REPO_ROOT" BUILD_DIR="$BUILD_DIR" THORNADO="$THORNADO" BIFROST="$BIFROST" \
    CHAIN_ID="$CHAIN_ID" SIGNER_PASSWD="$PASS" BTC_RPC_PORT="$BTC_RPC_PORT" BTC_START_BLOCK_HEIGHT="$BTC_START_BLOCK_HEIGHT" API_BASE="$API_BASE" GRPC_BASE="$GRPC_BASE" \
    RPC_BASE="$RPC_BASE" P2P_BASE="$P2P_BASE" EBIFROST_BASE="$EBIFROST_BASE" FROST_P2P_BASE="$FROST_P2P_BASE" \
    FROST_INFO_BASE="$FROST_INFO_BASE" METRICS_BASE="$METRICS_BASE" "$runner" \
    >>"$RUN_ROOT/logs/resume-runner.log" 2>&1 < /dev/null &
  runner_pid="$!"
  echo "$runner_pid" >"$RUN_ROOT/pids/runner.pid"
}

wait_bifrost_health() {
  local i="$1" attempts=0
  while true; do
    if wait_url "http://127.0.0.1:$(frost_info_port "$i")/ping" "bifrost-$i health" 60; then
      return 0
    fi
    attempts=$((attempts + 1))
    if (( attempts >= 3 )); then
      return 1
    fi
    restart_bifrost "$i"
    sleep 10
  done
}

main() {
  local runner runner_pid
  mkdir -p "$RUN_ROOT"/{logs,pids}
  runner="$SCRIPT_DIR/run-real4-processes.sh"
  stop_run_children
  if [[ -s "$RUN_ROOT/pids/runner.pid" ]]; then
    runner_pid="$(cat "$RUN_ROOT/pids/runner.pid")"
    if kill -0 "$runner_pid" >/dev/null 2>&1; then
      kill "$runner_pid" >/dev/null 2>&1 || true
      sleep 2
    fi
  fi
  launch_runner "$runner"
  runner_pid="$(cat "$RUN_ROOT/pids/runner.pid")"
  sleep 2
  if ! kill -0 "$runner_pid" >/dev/null 2>&1; then
    echo "[resume-real4] runner exited immediately: $runner_pid" >&2
    tail -n 120 "$RUN_ROOT/logs/resume-runner.log" 2>/dev/null >&2 || true
    exit 1
  fi

  for i in 1 2 3 4 5; do
    wait_url "http://127.0.0.1:$(rpc_port "$i")/status" "thornado-$i rpc" 120
  done

  for i in 1 2 3 4 5; do
    wait_bifrost_health "$i"
  done

  echo "[resume-real4] resumed $RUN_ROOT"
}

main "$@"
