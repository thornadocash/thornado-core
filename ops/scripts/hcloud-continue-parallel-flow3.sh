#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260627104200}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
RUN_DIR="${RUN_DIR:?RUN_DIR is required}"
COUNT="${COUNT:-20}"

export RUN_ROOT
export BTC_USE_LOCAL=1
export BTC_RPC_HOST="${BTC_RPC_HOST:-127.0.0.1}"
export BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
export BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
export CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
export SIGNER_PASSWD="${SIGNER_PASSWD:-passphrase123}"
export TX_INCLUSION_TIMEOUT="${TX_INCLUSION_TIMEOUT:-1200}"
export THORNADO_TX_TIMEOUT="${THORNADO_TX_TIMEOUT:-60}"
export THORNADO_TX_NODE="${THORNADO_TX_NODE:-tcp://5.223.55.174:33363}"

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
    *) echo "http://5.223.55.174:33363" ;;
  esac
}

thornado_tx_seq() {
  local seq="$1" account_number="$2" home="$3" from="$4"
  shift 4
  local from_addr out status
  from_addr="$(key_show_addr "$home" "$from")"
  set +e
  out="$(printf '%s\n' "$PASS" | timeout "${THORNADO_TX_TIMEOUT}" "$THORNADO" tx thornado "$@" \
    --home "$home" \
    --from "$from_addr" \
    --keyring-backend file \
    --keyring-dir "$home" \
    --chain-id "$CHAIN_ID" \
    --node "$THORNADO_TX_NODE" \
    --gas 2500000 \
    --fees 0btc \
    --broadcast-mode sync \
    --offline \
    --account-number "$account_number" \
    --sequence "$seq" \
    --yes \
    --output json \
    2>&1)"
  status=$?
  set -e
  if (( status == 124 )); then
    jq -n --arg log "thornado tx timed out after ${THORNADO_TX_TIMEOUT}s" \
      '{height:"0",txhash:"",codespace:"harness",code:124,data:"",raw_log:$log,logs:[],info:"",gas_wanted:"0",gas_used:"0",tx:null,timestamp:"",events":[]}'
    return 0
  fi
  printf '%s\n' "$out"
}

assert_checktx_success() {
  local out="$1" label="$2" txhash code res
  code="$(jq -r '.code // 0' <<<"$out")"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  if [[ "$code" != "0" ]]; then
    if [[ -n "$txhash" ]]; then
      res="$(curl_json_quiet "$(rpc_url 3)/tx?hash=0x${txhash}" || true)"
      if [[ -n "$res" ]] && jq -e '.result.tx_result.code == 0' <<<"$res" >/dev/null 2>&1; then
        printf '%s\n' "$txhash"
        return 0
      fi
    fi
    die "$label failed CheckTx: $out"
  fi
  [[ -n "$txhash" ]] || die "$label returned no txhash"
  printf '%s\n' "$txhash"
}

wait_txhashes_included() {
  local hashes_file="$1" label="$2" timeout="${3:-300}" start missing txhash res code raw_log
  start="$(date +%s)"
  while true; do
    missing=0
    while read -r txhash; do
      [[ -n "$txhash" ]] || continue
      res="$(curl_json_quiet "$(rpc_url 3)/tx?hash=0x${txhash}" || true)"
      if [[ -z "$res" ]] || ! jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
        missing=$((missing + 1))
        continue
      fi
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" != "0" ]]; then
        raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
        die "$label tx $txhash failed DeliverTx code=$code log=$raw_log"
      fi
    done <"$hashes_file"
    (( missing == 0 )) && return 0
    if (( $(date +%s) - start >= timeout )); then
      die "$label had ${missing} txs not included"
    fi
    sleep 1
  done
}

wait_all_txouts_signed_local() {
  local tx_type="$1" timeout="$2" ids_file="$3" out_dir="$4" start missing found id
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
    if (( $(date +%s) - start >= timeout )); then
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
      if ! curl -fsS "$(api_url 1)/thornado/tx/${hash}" >/tmp/thornado-parallel-observed.json 2>/dev/null ||
        ! jq -e '(.stages.inbound_observed.completed == true) and ((.stages.inbound_observed.final_count // 0) >= 3)' /tmp/thornado-parallel-observed.json >/dev/null; then
        missing=$((missing + 1))
      fi
    done <"$hashes_file"
    (( missing == 0 )) && return 0
    if (( $(date +%s) - start >= timeout )); then
      die "${missing} observed outbounds did not finalize"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

source "$RUN_DIR/summary.env"
COUNT="${count:-$COUNT}"
echo "continue start $(date -u +%FT%TZ) run_dir=$RUN_DIR count=$COUNT tx_node=$THORNADO_TX_NODE"

wait_txhashes_included "$RUN_DIR/request-txhashes.txt" "parallel request preexisting" 300
wait_txhashes_included "$RUN_DIR/shield-txhashes.txt" "parallel shield preexisting" 300

start_i=$(( $(wc -l <"$RUN_DIR/shield-txhashes.txt") + 1 ))
echo "resuming shields from $start_i"
for i in $(seq "$start_i" "$COUNT"); do
  d="$RUN_DIR/$i"
  label="$(cat "$d/label.txt")"
  deposit_id="$(cat "$d/deposit-id.txt")"
  amount_sats="$(cat "$d/amount-sats.txt")"
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(cat "$d/path-index.txt")" "$amount_sats" "${label}-seed")"
  printf '%s\n' "$receipt" >"$d/receipt.json"
  commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  printf '%s\n' "$commitment_objects" >"$d/commitment-objects.json"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  printf '%s\n' "$commitments" >"$d/commitments.json"
  shield_signature="$("$SHIELDER_HELPER" shield-authorization "${label}-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
  seq=$((base_sequence + COUNT + i - 1))
  out="$(thornado_tx_seq "$seq" "$account_number" "$RUN_ROOT/node1" user shielder shield "$commitments" "$(cat "$d/deposit-pubkey.txt")" "$shield_signature" "$deposit_id")"
  printf '%s\n' "$out" >"$d/shield.json"
  assert_checktx_success "$out" "parallel shield ${i}" >>"$RUN_DIR/shield-txhashes.txt"
  echo "shield $i sent"
done

wait_txhashes_included "$RUN_DIR/shield-txhashes.txt" "parallel shield" 300

for i in $(seq 1 "$COUNT"); do
  d="$RUN_DIR/$i"
  deposit_id="$(cat "$d/deposit-id.txt")"
  committed="$(wait_deposit_committed "$deposit_id" 240)"
  printf '%s\n' "$committed" >"$d/deposit-committed.json"
done

curl -fsS "$(api_url 1)/thornado/shielder/sync?limit=50000" >"$RUN_DIR/shielder-sync-after-shields.json"

echo "broadcasting redeem txs"
>"$RUN_DIR/withdrawal-ids.txt"
>"$RUN_DIR/redeem-txhashes.txt"
for i in $(seq 1 "$COUNT"); do
  d="$RUN_DIR/$i"
  label="$(cat "$d/label.txt")"
  note="$(jq -c '.notes[0]' "$d/receipt.json")"
  denom="$(jq -r '.denomination_sats' <<<"$note")"
  leaves="$(jq -c --argjson denom "$denom" '[.notes[] | select((.denomination_sats | tonumber) == $denom) | .commitment] | sort' "$RUN_DIR/shielder-sync-after-shields.json")"
  printf '%s\n' "$leaves" >"$d/proof-leaves.json"
  assert_shielder_root_committed "$denom" "$leaves" "parallel-flow3-${i}"
  recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
  printf '%s\n' "$recipient" >"$d/recipient-address.txt"
  curl -fsS "$(api_url 1)/thornado/shielder/redeem/quote/${denom}" >"$d/redeem-quote.json"
  fee="$(jq -r '.fee_sats' "$d/redeem-quote.json")"
  withdrawal="$("$SHIELDER_HELPER" withdrawal "$note" "${label}-seed" "$leaves" "$recipient" "$fee")"
  printf '%s\n' "$withdrawal" >"$d/withdrawal.json"
  prefix="$d/withdrawal"
  "$SHIELDER_HELPER" shield-withdrawal "$withdrawal" "$prefix"
  seq=$((base_sequence + COUNT + COUNT + i - 1))
  out="$(thornado_tx_seq "$seq" "$account_number" "$RUN_ROOT/node1" user shielder redeem "${prefix}.proof.json" "${prefix}.public.json")"
  printf '%s\n' "$out" >"$d/redeem.json"
  assert_checktx_success "$out" "parallel redeem ${i}" >>"$RUN_DIR/redeem-txhashes.txt"
  echo "redeem $i sent"
done

wait_txhashes_included "$RUN_DIR/redeem-txhashes.txt" "parallel redeem" 300

for i in $(seq 1 "$COUNT"); do
  d="$RUN_DIR/$i"
  out="$(cat "$d/redeem.json")"
  withdrawal_id="$(jq -r '.logs[0].events[]? | select(.type=="message") | .attributes[]? | select(.key=="withdrawal_id") | .value' <<<"$out" | tail -n1)"
  if [[ -z "$withdrawal_id" || "$withdrawal_id" == "null" ]]; then
    nullifier="$(jq -r '.[1].nullifier_hash' "$d/withdrawal.json")"
    nullifier_query="$(curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}")"
    withdrawal_id="$(jq -r '.withdrawal_id // empty' <<<"$nullifier_query")"
  fi
  [[ -n "$withdrawal_id" && "$withdrawal_id" != "null" ]] || die "parallel redeem ${i} did not expose withdrawal id"
  printf '%s\n' "$withdrawal_id" >"$d/withdrawal-id.txt"
  printf '%s\n' "$withdrawal_id" >>"$RUN_DIR/withdrawal-ids.txt"
done

echo "waiting for outbounds"
wait_all_txouts_signed_local out 1200 "$RUN_DIR/withdrawal-ids.txt" "$RUN_DIR"

>"$RUN_DIR/out-hashes.txt"
printf 'index,deposit_id,withdrawal_id,out_hash\n' >"$RUN_DIR/results.csv"
for i in $(seq 1 "$COUNT"); do
  d="$RUN_DIR/$i"
  withdrawal_id="$(cat "$d/withdrawal-id.txt")"
  out_hash="$(jq -r --arg in_hash "$withdrawal_id" '.txout.tx_array[] | select(.in_hash == $in_hash) | .out_hash' "$RUN_DIR/${withdrawal_id}-out-txout.json" | head -n1)"
  printf '%s\n' "$out_hash" >"$d/out-hash.txt"
  printf '%s\n' "$out_hash" >>"$RUN_DIR/out-hashes.txt"
  printf '%s,%s,%s,%s\n' "$i" "$(cat "$d/deposit-id.txt")" "$withdrawal_id" "$out_hash" >>"$RUN_DIR/results.csv"
done

echo "waiting observed outbounds"
wait_all_observed_out_final_local "$RUN_DIR/out-hashes.txt" 900

jq -n --argjson count "$COUNT" --arg run_dir "$RUN_DIR" \
  '{requested:$count, success:$count, failed:0, run_dir:$run_dir}' >"$RUN_DIR/summary.json"
cat "$RUN_DIR/summary.json"
echo "continue done $(date -u +%FT%TZ)"
