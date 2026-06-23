#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-node5-churn-20260619191627}"
LABEL="${LAUNCH_LABEL:-com.thornado.real4.node5_direct_runner}"

BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
BTC_START_BLOCK_HEIGHT="${BTC_START_BLOCK_HEIGHT:-361}"
API_BASE="${API_BASE:-2370}"
GRPC_BASE="${GRPC_BASE:-13380}"
RPC_BASE="${RPC_BASE:-33360}"
P2P_BASE="${P2P_BASE:-33380}"
EBIFROST_BASE="${EBIFROST_BASE:-58600}"
FROST_P2P_BASE="${FROST_P2P_BASE:-9340}"
FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
METRICS_BASE="${METRICS_BASE:-14200}"

mkdir -p "$RUN_ROOT"/{logs,pids}

launchctl remove "$LABEL" >/dev/null 2>&1 || true
launchctl remove "com.thornado.real4.$(basename "$RUN_ROOT" | tr -c '[:alnum:]' '_')" >/dev/null 2>&1 || true

for pid_file in "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid; do
  [[ -e "$pid_file" ]] || continue
  pid="$(cat "$pid_file" 2>/dev/null || true)"
  [[ -n "$pid" ]] && kill "$pid" >/dev/null 2>&1 || true
done
sleep 2

stamp="$(date +%Y%m%d%H%M%S)"
for log in resume-runner.log direct-launchd.out direct-launchd.err btc-miner.log; do
  [[ -s "$RUN_ROOT/logs/$log" ]] && mv "$RUN_ROOT/logs/$log" "$RUN_ROOT/logs/$log.$stamp"
done

rm -f "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid "$RUN_ROOT"/pids/runner.pid

launchctl submit -l "$LABEL" \
  -o "$RUN_ROOT/logs/direct-launchd.out" \
  -e "$RUN_ROOT/logs/direct-launchd.err" \
  -- /usr/bin/env \
    RUN_ROOT="$RUN_ROOT" \
    REPO_ROOT="$REPO_ROOT" \
    BUILD_DIR="$REPO_ROOT/build" \
    THORNADO="$REPO_ROOT/build/thornado" \
    BIFROST="$REPO_ROOT/build/bifrost" \
    CHAIN_ID="thornado-e2e" \
    SIGNER_PASSWD="passphrase123" \
    BTC_RPC_PORT="$BTC_RPC_PORT" \
    BTC_START_BLOCK_HEIGHT="$BTC_START_BLOCK_HEIGHT" \
    API_BASE="$API_BASE" \
    GRPC_BASE="$GRPC_BASE" \
    RPC_BASE="$RPC_BASE" \
    P2P_BASE="$P2P_BASE" \
    EBIFROST_BASE="$EBIFROST_BASE" \
    FROST_P2P_BASE="$FROST_P2P_BASE" \
    FROST_INFO_BASE="$FROST_INFO_BASE" \
    METRICS_BASE="$METRICS_BASE" \
    "$SCRIPT_DIR/run-real4-processes.sh"

sleep 2
runner_pid="$(launchctl list "$LABEL" 2>/dev/null | awk -F'= ' '/"PID"/ {gsub(/;/, "", $2); print $2}')"
if [[ -z "$runner_pid" ]]; then
  echo "failed to launch $LABEL" >&2
  launchctl list "$LABEL" 2>/dev/null >&2 || true
  exit 1
fi
echo "$runner_pid" >"$RUN_ROOT/pids/runner.pid"
echo "runner=$runner_pid label=$LABEL run_root=$RUN_ROOT"
