#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260628131009}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
INVENTORY="${INVENTORY:-${ROOT_DIR}/ops/distributed-regtest-nodeper.env}"
COUNT="${COUNT:-6}"
FEE_RATE_SAT_VB="${FEE_RATE_SAT_VB:-1000}"

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

node_host() {
  local key="NODE${1}_HOST"
  printf '%s' "${!key:-}"
}

api_url() {
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((API_BASE + $1))"
}

signer_url() {
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((FROST_INFO_BASE + $1))"
}

export THORNADO_TX_NODE="${THORNADO_TX_NODE:-tcp://$(node_host 1):$((RPC_BASE + 1))}"

mkdir -p "$RUN_ROOT/meta/fee-swing"
run_dir="${RESUME_RUN_DIR:-$RUN_ROOT/meta/fee-swing/$(date -u +%Y%m%d%H%M%S)}"
mkdir -p "$run_dir"

network_fee_rate() {
  curl -fsS "$(api_url 1)/thornado/network_fee" | tee "$run_dir/network-fee-latest.json" | jq -r '.transaction_fee_rate | tonumber'
}

wait_network_fee_above() {
  local old_rate="$1" timeout="${2:-240}" start rate
  start="$(date +%s)"
  while true; do
    rate="$(network_fee_rate || printf '0')"
    if (( rate > old_rate )); then
      printf '%s\n' "$rate"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "network fee did not increase above ${old_rate}; latest=${rate}"
    fi
    sleep 2
  done
}

make_high_fee_block() {
  local addr raw outputs funded signed txid block_hash block_height stats
  addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  outputs="$(jq -nc --arg addr "$addr" '{($addr):0.01000000}')"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "[]" "$outputs")"
  funded="$(btc_cli -rpcwallet=miner fundrawtransaction "$raw" "$(jq -nc --argjson fee_rate "$FEE_RATE_SAT_VB" '{fee_rate:$fee_rate}')")"
  printf '%s\n' "$funded" >"$run_dir/high-fee-funded.json"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$(jq -r '.hex' <<<"$funded")")"
  printf '%s\n' "$signed" >"$run_dir/high-fee-signed.json"
  jq -e '.complete == true' <<<"$signed" >/dev/null || die "high-fee tx did not sign"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$signed")" 0)"
  printf '%s\n' "$txid" >"$run_dir/high-fee-txid.txt"
  block_hash="$(btc_cli -rpcwallet=miner generatetoaddress 1 "$(btc_cli -rpcwallet=miner getnewaddress)" | jq -r '.[0]')"
  printf '%s\n' "$block_hash" >"$run_dir/high-fee-block-hash.txt"
  block_height="$(btc_cli getblock "$block_hash" | jq -r '.height')"
  stats="$(btc_cli getblockstats "$block_height" '["avgfeerate","totalfee","txs"]')"
  printf '%s\n' "$stats" >"$run_dir/high-fee-block-stats.json"
  jq -e '(.avgfeerate | tonumber) > 0 and (.txs | tonumber) > 1' <<<"$stats" >/dev/null || die "high-fee block did not contain fee-paying tx"
}

assert_batch_fee_rate() {
  local batch_dir="$1" expected="$2" first_txout out_hash tx_count
  jq -e --argjson expected "$expected" '.requested == $expected and .success == $expected and .failed == 0' "$batch_dir/summary.json" >/dev/null
  out_hash="$(tail -n +2 "$batch_dir/results.csv" | awk -F, 'NR==1{print $4}')"
  tx_count="$(tail -n +2 "$batch_dir/results.csv" | awk -F, -v out="$out_hash" '$4 == out {n++} END {print n+0}')"
  [[ "$tx_count" == "$expected" ]] || die "fee-swing batch was not fully batched: ${tx_count}/${expected}"
  first_txout="$(find "$batch_dir" -maxdepth 1 -name '*-out-txout.json' -print | head -n1)"
  [[ -n "$first_txout" ]] || die "fee-swing missing txout artifact"
  jq -e --arg out_hash "$out_hash" --argjson expected "$expected" '
    [.txout.tx_array[] | select(.out_hash == $out_hash)] as $items |
    ($items | length) == $expected and
    all($items[];
      ((.source_inputs // []) | length) > 0 and
      ((.max_gas // []) | length) > 0 and
      ((.gas_rate // 0) | tonumber) > 0
    )
  ' "$first_txout" >/dev/null || die "fee-swing batch txouts did not preserve high fee metadata"
}

assert_signer_queues_empty() {
  local i file start all_empty
  start="$(date +%s)"
  while true; do
    all_empty=1
    for i in 1 2 3 4; do
      file="$run_dir/node${i}-final-txouts.json"
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

batch_dir="$run_dir/batch-after-fee-spike"
if [[ -n "${RESUME_RUN_DIR:-}" ]]; then
  old_rate="$(cat "$run_dir/network-fee-before.txt")"
  new_rate="$(cat "$run_dir/network-fee-after.txt")"
else
  old_rate="$(network_fee_rate)"
  printf '%s\n' "$old_rate" >"$run_dir/network-fee-before.txt"
  make_high_fee_block
  new_rate="$(wait_network_fee_above "$old_rate" 300)"
  printf '%s\n' "$new_rate" >"$run_dir/network-fee-after.txt"
  RUN_DIR="$batch_dir" COUNT="$COUNT" "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh"
fi
(( new_rate > old_rate )) || die "network fee did not swing upward: ${old_rate} -> ${new_rate}"
assert_batch_fee_rate "$batch_dir" "$COUNT"
assert_signer_queues_empty

txout_rates="$(find "$batch_dir" -maxdepth 1 -name '*-out-txout.json' -print -quit | xargs jq -r '[.txout.tx_array[].gas_rate] | unique | join(",")')"
jq -n \
  --arg run_dir "$run_dir" \
  --arg txout_rates "$txout_rates" \
  --argjson old_rate "$old_rate" \
  --argjson new_rate "$new_rate" \
  --argjson count "$COUNT" \
  '{run_dir:$run_dir,status:"pass",old_rate:$old_rate,new_rate:$new_rate,txout_rates:$txout_rates,count:$count}' >"$run_dir/summary.json"
cat "$run_dir/summary.json"
