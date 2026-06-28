#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260627104200}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
ITERATIONS="${ITERATIONS:-20}"
WAIT_OBSERVED_OUT_FINAL_EACH="${WAIT_OBSERVED_OUT_FINAL_EACH:-0}"
WAIT_OBSERVED_OUT_FINAL_END="${WAIT_OBSERVED_OUT_FINAL_END:-1}"

export RUN_ROOT
export BTC_USE_LOCAL=1
export BTC_RPC_HOST="${BTC_RPC_HOST:-127.0.0.1}"
export BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
export BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
export CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
export SIGNER_PASSWD="${SIGNER_PASSWD:-passphrase123}"
export TX_INCLUSION_TIMEOUT="${TX_INCLUSION_TIMEOUT:-1200}"
export THORNADO_TX_TIMEOUT="${THORNADO_TX_TIMEOUT:-60}"
export THORNADO_TX_NODE="${THORNADO_TX_NODE:-tcp://5.223.51.101:33361}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/ops/scripts/real-4node-e2e.sh"
trap - EXIT ERR

api_url() {
  case "$1" in
    1) echo "http://5.223.51.101:2371" ;;
    2) echo "http://5.223.55.114:2372" ;;
    3) echo "http://5.223.55.174:2373" ;;
    4) echo "http://5.223.92.204:2374" ;;
    *) echo "http://5.223.51.101:2371" ;;
  esac
}

rpc_url() {
  case "$1" in
    1) echo "http://5.223.51.101:33361" ;;
    2) echo "http://5.223.55.114:33362" ;;
    3) echo "http://5.223.55.174:33363" ;;
    4) echo "http://5.223.92.204:33364" ;;
    *) echo "http://5.223.51.101:33361" ;;
  esac
}

wait_observed_out_final() {
  local txid="$1" timeout="${2:-240}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "$(api_url 1)/thornado/tx/${txid}" >"$RUN_ROOT/meta/repeated-last-observed-out.json" 2>/dev/null &&
      jq -e '(.stages.inbound_observed.completed == true) and ((.stages.inbound_observed.final_count // 0) >= 3)' "$RUN_ROOT/meta/repeated-last-observed-out.json" >/dev/null; then
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      return 1
    fi
    mine_regtest_blocks 1 || true
    sleep 2
  done
}

snapshot_bifrost_debug() {
  local label="$1"
  local dir="$RUN_ROOT/meta/${label}.bifrost"
  mkdir -p "$dir"
  local spec h p name
  for spec in \
    node1,5.223.51.101,10341 \
    node2,5.223.55.114,10342 \
    node3,5.223.55.174,10343 \
    node4,5.223.92.204,10344; do
    IFS=, read -r name h p <<<"$spec"
    curl -fsS --max-time 8 "http://${h}:${p}/debug/health/full" >"${dir}/${name}-health.json" || true
    curl -fsS --max-time 8 "http://${h}:${p}/debug/attestations/performance" >"${dir}/${name}-attestations.json" || true
  done
}

mkdir -p "$RUN_ROOT/meta"
if [[ -n "${WITHDRAWAL_BATCH_WINDOW_MINUTES:-}" ]]; then
  set_config_from_active_nodes Withdrawal_BatchWindowMinutes "$WITHDRAWAL_BATCH_WINDOW_MINUTES"
fi
snapshot_bifrost_debug "repeated-before"

success=0
printf 'iteration,deposit_id,withdrawal_id,out_hash\n' >"$RUN_ROOT/meta/repeated-flow3-results.csv"
for i in $(seq 1 "$ITERATIONS"); do
  export FLOW3_MAIN_ONLY=1
  export FLOW3_LABEL="repeated-flow3-${i}-$(date +%s)"
  validate_flow3
  iter_dir="$RUN_ROOT/meta/repeated-flow3-${i}"
  rm -rf "$iter_dir"
  mkdir -p "$iter_dir"
  cp "$RUN_ROOT"/meta/flow3-* "$iter_dir"/ 2>/dev/null || true
  deposit_id="$(jq -r '.deposit_id // empty' "$RUN_ROOT/meta/flow3-deposit.json")"
  withdrawal_id="$(cat "$RUN_ROOT/meta/flow3-withdrawal-id.txt")"
  out_hash="$(jq -r --arg in_hash "$withdrawal_id" '.txout.tx_array[] | select(.in_hash == $in_hash) | .out_hash' "$RUN_ROOT/meta/flow3-withdrawal-txout.json" | head -n1)"
  if [[ "$WAIT_OBSERVED_OUT_FINAL_EACH" == "1" ]]; then
    wait_observed_out_final "$out_hash" 300
    cp "$RUN_ROOT/meta/repeated-last-observed-out.json" "$iter_dir/observed-out.json" 2>/dev/null || true
  fi
  printf '%s,%s,%s,%s\n' "$i" "$deposit_id" "$withdrawal_id" "$out_hash" >>"$RUN_ROOT/meta/repeated-flow3-results.csv"
  success=$((success + 1))
done

if [[ "$WAIT_OBSERVED_OUT_FINAL_END" == "1" ]]; then
  while IFS=, read -r i _deposit_id _withdrawal_id out_hash; do
    [[ "$i" == "iteration" ]] && continue
    [[ -n "$out_hash" ]] || die "iteration ${i} did not record an outbound hash"
    wait_observed_out_final "$out_hash" 300
    iter_dir="$RUN_ROOT/meta/repeated-flow3-${i}"
    cp "$RUN_ROOT/meta/repeated-last-observed-out.json" "$iter_dir/observed-out.json" 2>/dev/null || true
  done <"$RUN_ROOT/meta/repeated-flow3-results.csv"
fi

snapshot_bifrost_debug "repeated-after"
jq -n --argjson requested "$ITERATIONS" --argjson success "$success" \
  '{requested:$requested, success:$success, failed:($requested - $success)}' \
  >"$RUN_ROOT/meta/repeated-flow3-summary.json"
cat "$RUN_ROOT/meta/repeated-flow3-summary.json"
