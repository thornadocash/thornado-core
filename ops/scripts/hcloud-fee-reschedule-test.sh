#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260628131009}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
INVENTORY="${INVENTORY:-${ROOT_DIR}/ops/distributed-regtest-nodeper.env}"
COUNT="${COUNT:-8}"
FEE_RATE_SAT_VB="${FEE_RATE_SAT_VB:-3000}"
FEE_SPIKE_MAX_BLOCKS="${FEE_SPIKE_MAX_BLOCKS:-6}"

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

mkdir -p "$RUN_ROOT/meta/fee-reschedule"
run_dir="${RESUME_RUN_DIR:-$RUN_ROOT/meta/fee-reschedule/$(date -u +%Y%m%d%H%M%S)}"
flow_dir="$run_dir/flow3-pending"
mkdir -p "$run_dir"

halt_signing_value() {
  curl -fsS "$(api_url 1)/thornado/config" | jq -r '.HALTSIGNINGBTC.value // empty'
}

set_halt_signing_direct() {
  local value="$1" i attempt out code raw_log start current
  current="$(halt_signing_value || true)"
  if [[ "$current" == "$value" ]]; then
    return 0
  fi

  for i in 1 2 3 4; do
    for attempt in 1 2 3; do
      out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" config HaltSigningBTC "$value")"
      printf '%s\n' "$out" >"$run_dir/config-HaltSigningBTC-${value}-node${i}-attempt-${attempt}.json"
      code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
      raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
      if [[ "$code" == "0" || "$code" == "null" ]]; then
        break
      fi
      if { [[ "$raw_log" == *"account sequence mismatch"* ]] || [[ "$raw_log" == *"timed out"* ]]; } && [[ "$attempt" != "3" ]]; then
        sleep 2
        continue
      fi
      die "config HaltSigningBTC=${value} vote failed on node${i}: ${raw_log:-$out}"
    done
  done

  start="$(date +%s)"
  while true; do
    current="$(halt_signing_value || true)"
    if [[ "$current" == "$value" ]]; then
      return 0
    fi
    if (( "$(date +%s)" - start >= 120 )); then
      die "config HaltSigningBTC did not apply value ${value}; current=${current:-empty}"
    fi
    sleep 2
  done
}

reset_signing() {
  set_halt_signing_direct 0 >/dev/null 2>&1 || true
}
trap reset_signing EXIT

network_fee_rate() {
  curl -fsS "$(api_url 1)/thornado/network_fee" | tee "$run_dir/network-fee-latest.json" | jq -r '.transaction_fee_rate | tonumber'
}

withdrawal_ids_json() {
  jq -Rsc 'split("\n") | map(select(length > 0))' "$flow_dir/withdrawal-ids.txt"
}

select_batch_txout() {
  local out_file="$1" ids
  ids="$(withdrawal_ids_json)"
  curl -fsS "$(api_url 1)/thornado/txout/out" >"$out_file.raw"
  jq -e --argjson ids "$ids" '
    [.txouts[]? |
      select((.tx_array // []) as $items |
        all($ids[]; . as $id | any($items[]?; .in_hash == $id))
      )
    ] | sort_by(.height) | .[0]
  ' "$out_file.raw" >"$out_file"
}

txout_min_rate() {
  local file="$1" ids
  ids="$(withdrawal_ids_json)"
  jq -r --argjson ids "$ids" '
    [.tx_array[]? |
      . as $item |
      select(any($ids[]; . == $item.in_hash)) |
      ($item.gas_rate // 0 | tonumber)
    ] | min // 0
  ' "$file"
}

assert_batch_metadata() {
  local file="$1" expected="$2" ids
  ids="$(withdrawal_ids_json)"
  jq -e --argjson ids "$ids" --argjson expected "$expected" '
    [.tx_array[]? | . as $item | select(any($ids[]; . == $item.in_hash))] as $items |
    ($items | length) == $expected and
    all($items[];
      (.out_hash // "") == "" and
      ((.source_inputs // []) | length) > 0 and
      ((.max_gas // []) | length) > 0 and
      ((.gas_rate // 0) | tonumber) > 0
    )
  ' "$file" >/dev/null
}

wait_pending_batch_metadata() {
  local file="$1" timeout="${2:-180}" start
  start="$(date +%s)"
  while true; do
    if select_batch_txout "$file" >/dev/null 2>&1 && assert_batch_metadata "$file" "$COUNT"; then
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      return 1
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
  printf '%s\n' "$funded" >"$run_dir/high-fee-funded-$(date +%s%N).json"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$(jq -r '.hex' <<<"$funded")")"
  jq -e '.complete == true' <<<"$signed" >/dev/null || die "high-fee tx did not sign"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$signed")" 0)"
  printf '%s\n' "$txid" >>"$run_dir/high-fee-txids.txt"
  block_hash="$(btc_cli -rpcwallet=miner generatetoaddress 1 "$(btc_cli -rpcwallet=miner getnewaddress)" | jq -r '.[0]')"
  printf '%s\n' "$block_hash" >>"$run_dir/high-fee-blocks.txt"
  block_height="$(btc_cli getblock "$block_hash" | jq -r '.height')"
  stats="$(btc_cli getblockstats "$block_height" '["avgfeerate","totalfee","txs"]')"
  printf '%s\n' "$stats" >>"$run_dir/high-fee-block-stats.jsonl"
  jq -e '(.avgfeerate | tonumber) > 0 and (.txs | tonumber) > 1' <<<"$stats" >/dev/null || die "high-fee block did not contain fee-paying tx"
}

wait_network_fee_above() {
  local old_rate="$1" timeout="${2:-180}" start rate
  start="$(date +%s)"
  while true; do
    rate="$(network_fee_rate || printf '0')"
    if (( rate > old_rate )); then
      printf '%s\n' "$rate"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      return 1
    fi
    sleep 2
  done
}

wait_txout_rate_above() {
  local old_rate="$1" timeout="${2:-90}" start file rate
  start="$(date +%s)"
  while true; do
    file="$run_dir/pending-after-spike.json"
    if select_batch_txout "$file" >/dev/null 2>&1; then
      assert_batch_metadata "$file" "$COUNT"
      rate="$(txout_min_rate "$file")"
      if (( rate > old_rate )); then
        printf '%s\n' "$rate"
        return 0
      fi
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      return 1
    fi
    sleep 2
  done
}

wait_all_txouts_signed_local() {
  local tx_type="$1" timeout="${2:-1200}" ids_file="$3" out_dir="$4" start missing found id
  start="$(date +%s)"
  while true; do
    missing=0
    while read -r id; do
      [[ -n "$id" ]] || continue
      if [[ "$tx_type" == "sweep" ]]; then
        found="$(find_signed_sweep_txout "$id" || true)"
      else
        found="$(find_signed_txout_by_in_hash "$id" "$tx_type" || true)"
      fi
      if [[ -z "$found" ]]; then
        missing=$((missing + 1))
      else
        printf '%s\n' "$found" >"${out_dir}/${id}-${tx_type}-txout.json"
      fi
    done <"$ids_file"
    (( missing == 0 )) && return 0
    if (( "$(date +%s)" - start >= timeout )); then
      die "${missing} ${tx_type} txouts were not signed"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

wait_all_observed_out_final_local() {
  local hashes_file="$1" timeout="${2:-900}" start missing hash
  start="$(date +%s)"
  while true; do
    missing=0
    while read -r hash; do
      [[ -n "$hash" ]] || continue
      if ! curl -fsS "$(api_url 1)/thornado/tx/${hash}" >/tmp/thornado-fee-reschedule-observed.json 2>/dev/null ||
        ! jq -e '(.stages.inbound_observed.completed == true) and ((.stages.inbound_observed.final_count // 0) >= 3)' /tmp/thornado-fee-reschedule-observed.json >/dev/null; then
        missing=$((missing + 1))
      fi
    done <"$hashes_file"
    (( missing == 0 )) && return 0
    if (( "$(date +%s)" - start >= timeout )); then
      die "${missing} observed outbounds did not finalize"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

assert_signer_queues_empty_local() {
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
    if (( "$(date +%s)" - start >= 180 )); then
      die "signer queues were not empty"
    fi
    sleep 2
  done
}

set_halt_signing_direct 0
old_network_rate="$(network_fee_rate)"
printf '%s\n' "$old_network_rate" >"$run_dir/network-fee-before.txt"

RUN_DIR="$flow_dir" COUNT="$COUNT" FLOW3_HALT_SIGNING_BEFORE_REDEEMS=1 FLOW3_STOP_AFTER_REDEEMS=1 "$ROOT_DIR/ops/scripts/hcloud-parallel-flow3.sh"

wait_pending_batch_metadata "$run_dir/pending-before-spike.json" 180 || die "pending fee-reschedule txout missing source/gas metadata"
before_txout_rate="$(txout_min_rate "$run_dir/pending-before-spike.json")"
printf '%s\n' "$before_txout_rate" >"$run_dir/txout-rate-before.txt"
(( before_txout_rate > 0 )) || die "pending txout gas rate was empty before fee spike"

new_network_rate=0
after_txout_rate=0
for _ in $(seq 1 "$FEE_SPIKE_MAX_BLOCKS"); do
  make_high_fee_block
  if new_network_rate="$(wait_network_fee_above "$old_network_rate" 120)"; then
    printf '%s\n' "$new_network_rate" >"$run_dir/network-fee-after.txt"
    if after_txout_rate="$(wait_txout_rate_above "$before_txout_rate" 90)"; then
      printf '%s\n' "$after_txout_rate" >"$run_dir/txout-rate-after.txt"
      break
    fi
  fi
done

(( new_network_rate > old_network_rate )) || die "network fee did not swing upward: ${old_network_rate} -> ${new_network_rate}"
(( after_txout_rate > before_txout_rate )) || die "pending txout gas rate did not reschedule upward: ${before_txout_rate} -> ${after_txout_rate}"

set_halt_signing_direct 0
wait_all_txouts_signed_local out 1200 "$flow_dir/withdrawal-ids.txt" "$flow_dir"

>"$flow_dir/out-hashes.txt"
printf 'index,deposit_id,withdrawal_id,out_hash\n' >"$flow_dir/results.csv"
for i in $(seq 1 "$COUNT"); do
  d="$flow_dir/$i"
  withdrawal_id="$(cat "$d/withdrawal-id.txt")"
  out_hash="$(jq -r --arg in_hash "$withdrawal_id" '.txout.tx_array[] | select(.in_hash == $in_hash) | .out_hash' "$flow_dir/${withdrawal_id}-out-txout.json" | head -n1)"
  [[ -n "$out_hash" && "$out_hash" != "null" ]] || die "withdrawal ${i} did not produce out hash"
  printf '%s\n' "$out_hash" >"$d/out-hash.txt"
  printf '%s\n' "$out_hash" >>"$flow_dir/out-hashes.txt"
  printf '%s,%s,%s,%s\n' "$i" "$(cat "$d/deposit-id.txt")" "$withdrawal_id" "$out_hash" >>"$flow_dir/results.csv"
done

wait_all_observed_out_final_local "$flow_dir/out-hashes.txt" 900
assert_signer_queues_empty_local

jq -n \
  --arg run_dir "$run_dir" \
  --arg flow_dir "$flow_dir" \
  --argjson count "$COUNT" \
  --argjson old_network_rate "$old_network_rate" \
  --argjson new_network_rate "$new_network_rate" \
  --argjson before_txout_rate "$before_txout_rate" \
  --argjson after_txout_rate "$after_txout_rate" \
  '{run_dir:$run_dir,flow_dir:$flow_dir,status:"pass",count:$count,old_network_rate:$old_network_rate,new_network_rate:$new_network_rate,before_txout_rate:$before_txout_rate,after_txout_rate:$after_txout_rate,rescheduled:true}' >"$run_dir/summary.json"
cat "$run_dir/summary.json"
