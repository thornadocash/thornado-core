#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260627104200}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
INVENTORY="${INVENTORY:-${ROOT_DIR}/ops/distributed-regtest-nodeper.env}"
COUNT="${COUNT:-20}"
DEPOSIT_AMOUNTS_CSV="${DEPOSIT_AMOUNTS_CSV:-}"
DUPLICATE_RECIPIENT="${DUPLICATE_RECIPIENT:-0}"
FLOW3_HALT_SIGNING_BEFORE_REDEEMS="${FLOW3_HALT_SIGNING_BEFORE_REDEEMS:-0}"
FLOW3_STOP_AFTER_REDEEMS="${FLOW3_STOP_AFTER_REDEEMS:-0}"

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
NODE_SPECS="${NODE_SPECS:-}"

node_spec_field() {
  local idx="$1" field="$2" entry spec host api rpc signer
  if [[ -n "$NODE_SPECS" ]]; then
    IFS=',' read -ra entries <<<"$NODE_SPECS"
    for entry in "${entries[@]}"; do
      if [[ "${entry%%=*}" == "$idx" ]]; then
        spec="${entry#*=}"
        IFS=':' read -r host api rpc signer <<<"$spec"
        case "$field" in
          host) printf '%s' "$host" ;;
          api) printf '%s' "$api" ;;
          rpc) printf '%s' "$rpc" ;;
          signer) printf '%s' "$signer" ;;
        esac
        return 0
      fi
    done
  fi
  return 1
}

node_host() {
  local key="NODE${1}_HOST"
  if node_spec_field "$1" host; then
    return 0
  fi
  printf '%s' "${!key:-}"
}

api_url() {
  local port
  if port="$(node_spec_field "$1" api)"; then
    printf 'http://%s:%s\n' "$(node_host "$1")" "$port"
    return 0
  fi
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((API_BASE + $1))"
}

rpc_url() {
  local port
  if port="$(node_spec_field "$1" rpc)"; then
    printf 'http://%s:%s\n' "$(node_host "$1")" "$port"
    return 0
  fi
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((RPC_BASE + $1))"
}

if [[ -z "${THORNADO_TX_NODE:-}" ]]; then
  if tx_rpc_port="$(node_spec_field 1 rpc)"; then
    export THORNADO_TX_NODE="tcp://$(node_host 1):${tx_rpc_port}"
  else
    export THORNADO_TX_NODE="tcp://$(node_host 1):$((RPC_BASE + 1))"
  fi
fi

deposit_amount_for_index() {
  local i="$1" amount
  if [[ -n "$DEPOSIT_AMOUNTS_CSV" ]]; then
    amount="$(awk -v idx="$i" 'BEGIN{FS=","} {gsub(/[[:space:]]/,""); print $idx}' <<<"$DEPOSIT_AMOUNTS_CSV")"
    if [[ -n "$amount" ]]; then
      printf '%s\n' "$amount"
      return 0
    fi
  fi
  printf '0.20000000\n'
}

thornado_tx_seq() {
	local seq="$1" account_number="$2" home="$3" from="$4"
	shift 4
	local attempt from_addr max_attempts out status node_rpc
	from_addr="$(key_show_addr "$home" "$from")"
	node_rpc="${THORNADO_TX_NODE}"
	max_attempts="${THORNADO_TX_ATTEMPTS:-5}"
	for attempt in $(seq 1 "$max_attempts"); do
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
			--offline \
			--account-number "$account_number" \
			--sequence "$seq" \
			--yes \
			--output json \
			2>&1)"
		status=$?
		set -e
		if (( status == 124 )); then
			out="thornado tx timed out after ${THORNADO_TX_TIMEOUT:-45}s"
		elif jq -e . <<<"$out" >/dev/null 2>&1; then
			printf '%s\n' "$out"
			return 0
		fi
		if (( attempt < max_attempts )) && grep -Eqi 'EOF|connection reset|context deadline|timeout' <<<"$out"; then
			sleep 1
			continue
		fi
		if [[ ! "$status" =~ ^[0-9]+$ ]] || (( status == 0 )); then
			status=125
		fi
		jq -nc --arg log "$out" --argjson code "$status" \
			'{height:"0",txhash:"",codespace:"harness",code:$code,data:"",raw_log:$log,logs:[],info:"",gas_wanted:"0",gas_used:"0",tx:null,timestamp:"",events":[]}'
		return 0
	done
}

assert_checktx_success() {
  local out="$1" label="$2" txhash code res raw_log start
  code="$(jq -r '.code // 0' <<<"$out")"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  if [[ "$code" != "0" ]]; then
    if [[ -n "$txhash" ]]; then
      start="$(date +%s)"
      while (( $(date +%s) - start < 30 )); do
        res="$(curl_json_quiet "$(rpc_url 1)/tx?hash=0x${txhash}" || true)"
        if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
          code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
          if [[ "$code" == "0" ]]; then
            printf '%s\n' "$txhash"
            return 0
          fi
          raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
          die "$label tx $txhash failed DeliverTx code=$code log=$raw_log"
        fi
        sleep 1
      done
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
      res="$(curl_json_quiet "$(rpc_url 1)/tx?hash=0x${txhash}" || true)"
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
    if (( "$(date +%s)" - start >= timeout )); then
      die "$label had ${missing} txs not included"
    fi
    sleep 1
  done
}

wait_all_txouts_signed() {
  local tx_type="$1" timeout="${2:-1200}" start ids_file="$3" out_dir="$4" missing found id
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

wait_all_observed_out_final() {
  local hashes_file="$1" timeout="${2:-600}" start missing hash
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
    if (( "$(date +%s)" - start >= timeout )); then
      die "${missing} observed outbounds did not finalize"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

mkdir -p "$RUN_ROOT/meta/parallel-flow3"
run_dir="${RUN_DIR:-${RESUME_RUN_DIR:-$RUN_ROOT/meta/parallel-flow3/$(date -u +%Y%m%d%H%M%S)}}"
mkdir -p "$run_dir"

source "$RUN_ROOT/meta/user.env"
user_addr="$address"
if [[ -n "${RESUME_RUN_DIR:-}" ]]; then
  source "$run_dir/summary.env"
  COUNT="${count:-$COUNT}"
  log "parallel Flow 3: resuming ${COUNT} deposits from $run_dir"
else
  account_json="$(curl -fsS "$(api_url 1)/cosmos/auth/v1beta1/accounts/${user_addr}")"
  account_number="$(jq -r '.account.account_number // .account.base_account.account_number // "0"' <<<"$account_json")"
  base_sequence="$(jq -r '.account.sequence // .account.base_account.sequence // "0"' <<<"$account_json")"
  difficulty="$(curl -fsS "$(api_url 1)/thornado/config" | jq -r '.int_64_values.Deposit_PowDifficultyCurrent // .int_64_values.Deposit_PowDifficultyMin // 20')"

  printf 'account_number=%s\nbase_sequence=%s\ncount=%s\n' "$account_number" "$base_sequence" "$COUNT" >"$run_dir/summary.env"
  printf 'index,deposit_id,withdrawal_id,out_hash\n' >"$run_dir/results.csv"

  log "parallel Flow 3: preparing ${COUNT} deposits"
  for i in $(seq 1 "$COUNT"); do
    d="$run_dir/$i"
    mkdir -p "$d"
    label="parallel-flow3-${i}-$(date +%s)"
    deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${label}-deposit-pubkey")"
    owner_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
    pow_token="$("$SHIELDER_HELPER" pow-token "$owner_addr" "$difficulty" "$label")"
    printf '%s\n' "$deposit_pubkey" >"$d/deposit-pubkey.txt"
    printf '%s\n' "$owner_addr" >"$d/owner-address.txt"
    printf '%s\n' "$label" >"$d/label.txt"
    printf '%s\n' "$pow_token" >"$d/pow-token.txt"
  done

  log "parallel Flow 3: broadcasting request-deposit txs"
  >"$run_dir/request-txhashes.txt"
  for i in $(seq 1 "$COUNT"); do
    d="$run_dir/$i"
    seq=$((base_sequence + i - 1))
    out="$(thornado_tx_seq "$seq" "$account_number" "$RUN_ROOT/node1" "user" request-deposit "$(cat "$d/pow-token.txt")" "$(cat "$d/deposit-pubkey.txt")")"
    printf '%s\n' "$out" >"$d/request-deposit.json"
    assert_checktx_success "$out" "parallel request-deposit ${i}" >>"$run_dir/request-txhashes.txt"
  done
  wait_txhashes_included "$run_dir/request-txhashes.txt" "parallel request-deposit" 300

  log "parallel Flow 3: funding deposit addresses"
  for i in $(seq 1 "$COUNT"); do
    d="$run_dir/$i"
    session="$(deposit_session "$(cat "$d/owner-address.txt")")"
    printf '%s\n' "$session" >"$d/session-before-deposit.json"
    address="$(jq -r '.deposit_address' <<<"$session")"
    path_index="$(jq -r '.deposit_path_index' <<<"$session")"
    deposit_amount="$(deposit_amount_for_index "$i")"
    printf '%s\n' "$deposit_amount" >"$d/deposit-amount-btc.txt"
    txid="$(btc_cli -rpcwallet=miner sendtoaddress "$address" "$deposit_amount" "" "" false true)"
    printf '%s\n' "$txid" >"$d/btc-deposit-txid.txt"
    printf '%s\n' "$path_index" >"$d/path-index.txt"
  done
  mine_regtest_blocks 2

  >"$run_dir/deposit-ids.txt"
  for i in $(seq 1 "$COUNT"); do
    d="$run_dir/$i"
    matched="$(wait_owner_deposit_matched "$(cat "$d/owner-address.txt")" 420)"
    printf '%s\n' "$matched" >"$d/deposit-matched.json"
    deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
    amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats // empty')"
    [[ "$amount_sats" =~ ^[0-9]+$ ]] || die "parallel deposit ${i} amount_sats was not available"
    printf '%s\n' "$deposit_id" >"$d/deposit-id.txt"
    printf '%s\n' "$amount_sats" >"$d/amount-sats.txt"
    printf '%s\n' "$deposit_id" >>"$run_dir/deposit-ids.txt"
  done

  log "parallel Flow 3: waiting for sweeps"
  wait_all_txouts_signed "sweep" 1200 "$run_dir/deposit-ids.txt" "$run_dir"
fi

log "parallel Flow 3: broadcasting shield txs"
>"$run_dir/shield-txhashes.txt"
for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  label="$(cat "$d/label.txt")"
  deposit_id="$(cat "$d/deposit-id.txt")"
  amount_sats="$(cat "$d/amount-sats.txt")"
  if [[ ! "$amount_sats" =~ ^[0-9]+$ ]]; then
    amount_sats="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" | jq -r '.amount_sats // empty')"
    [[ "$amount_sats" =~ ^[0-9]+$ ]] || die "parallel deposit ${i} amount-sats was not available"
    printf '%s\n' "$amount_sats" >"$d/amount-sats.txt"
  fi
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(cat "$d/path-index.txt")" "$amount_sats" "${label}-seed")"
  printf '%s\n' "$receipt" >"$d/receipt.json"
  commitment_objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  printf '%s\n' "$commitment_objects" >"$d/commitment-objects.json"
  commitments="$(jq -c 'map(tostring)' <<<"$commitment_objects")"
  printf '%s\n' "$commitments" >"$d/commitments.json"
  shield_signature="$("$SHIELDER_HELPER" shield-authorization "${label}-deposit-pubkey" "$deposit_id" "$amount_sats" "$commitment_objects" | jq -r '.signature')"
  seq=$((base_sequence + COUNT + i - 1))
  out="$(thornado_tx_seq "$seq" "$account_number" "$RUN_ROOT/node1" "user" shielder shield "$commitments" "$(cat "$d/deposit-pubkey.txt")" "$shield_signature" "$deposit_id")"
  printf '%s\n' "$out" >"$d/shield.json"
  assert_checktx_success "$out" "parallel shield ${i}" >>"$run_dir/shield-txhashes.txt"
done
wait_txhashes_included "$run_dir/shield-txhashes.txt" "parallel shield" 300

for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  deposit_id="$(cat "$d/deposit-id.txt")"
  committed="$(wait_deposit_committed "$deposit_id" 240)"
  printf '%s\n' "$committed" >"$d/deposit-committed.json"
done

curl -fsS "$(api_url 1)/thornado/shielder/sync?limit=50000" >"$run_dir/shielder-sync-after-shields.json"

log "parallel Flow 3: broadcasting redeem txs"
>"$run_dir/withdrawal-ids.txt"
>"$run_dir/redeem-txhashes.txt"
if [[ "$DUPLICATE_RECIPIENT" == "1" ]]; then
  btc_cli -rpcwallet=miner getnewaddress >"$run_dir/shared-recipient-address.txt"
fi
for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  label="$(cat "$d/label.txt")"
  note="$(jq -c '.notes[0]' "$d/receipt.json")"
  denom="$(jq -r '.denomination_sats' <<<"$note")"
  leaves="$(jq -c --argjson denom "$denom" '[.notes[] | select((.denomination_sats | tonumber) == $denom) | .commitment] | sort' "$run_dir/shielder-sync-after-shields.json")"
  printf '%s\n' "$leaves" >"$d/proof-leaves.json"
  assert_shielder_root_committed "$denom" "$leaves" "parallel-flow3-${i}"
  if [[ "$DUPLICATE_RECIPIENT" == "1" ]]; then
    recipient="$(cat "$run_dir/shared-recipient-address.txt")"
  else
    recipient="$(btc_cli -rpcwallet=miner getnewaddress)"
  fi
  printf '%s\n' "$recipient" >"$d/recipient-address.txt"
  curl -fsS "$(api_url 1)/thornado/shielder/redeem/quote/${denom}" >"$d/redeem-quote.json"
  fee="$(jq -r '.fee_sats' "$d/redeem-quote.json")"
  withdrawal="$("$SHIELDER_HELPER" withdrawal "$note" "${label}-seed" "$leaves" "$recipient" "$fee")"
  printf '%s\n' "$withdrawal" >"$d/withdrawal.json"
  prefix="$d/withdrawal"
  "$SHIELDER_HELPER" shield-withdrawal "$withdrawal" "$prefix"
done

log "parallel Flow 3: submitting redeem txs concurrently"
if [[ "$FLOW3_HALT_SIGNING_BEFORE_REDEEMS" == "1" ]]; then
  set_config_from_active_nodes HaltSigningBTC 1
fi
pids=()
for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  prefix="$d/withdrawal"
  seq=$((base_sequence + COUNT + COUNT + i - 1))
  (
    out="$(thornado_tx_seq "$seq" "$account_number" "$RUN_ROOT/node1" "user" shielder redeem "${prefix}.proof.json" "${prefix}.public.json")"
    printf '%s\n' "$out" >"$d/redeem.json"
    assert_checktx_success "$out" "parallel redeem ${i}" >"$d/redeem-txhash.txt"
  ) >"$d/redeem-submit.log" 2>&1 &
  pids+=("$!")
done
failed=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    failed=1
  fi
done
for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  cat "$d/redeem-submit.log"
  [[ "$failed" == "0" ]] || continue
  cat "$d/redeem-txhash.txt" >>"$run_dir/redeem-txhashes.txt"
done
[[ "$failed" == "0" ]] || die "one or more parallel redeem tx submissions failed"
wait_txhashes_included "$run_dir/redeem-txhashes.txt" "parallel redeem" 300

for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  out="$(cat "$d/redeem.json")"
  withdrawal_id="$(jq -r '.logs[0].events[]? | select(.type=="message") | .attributes[]? | select(.key=="withdrawal_id") | .value' <<<"$out" | tail -n1)"
  if [[ -z "$withdrawal_id" || "$withdrawal_id" == "null" ]]; then
    nullifier="$(jq -r '.[1].nullifier_hash' "$d/withdrawal.json")"
    nullifier_query="$(curl -fsS "$(api_url 1)/thornado/shielder/nullifier/${nullifier}")"
    withdrawal_id="$(jq -r '.withdrawal_id // empty' <<<"$nullifier_query")"
  fi
  [[ -n "$withdrawal_id" && "$withdrawal_id" != "null" ]] || die "parallel redeem ${i} did not expose withdrawal id"
  printf '%s\n' "$withdrawal_id" >"$d/withdrawal-id.txt"
  printf '%s\n' "$withdrawal_id" >>"$run_dir/withdrawal-ids.txt"
done

if [[ "$FLOW3_STOP_AFTER_REDEEMS" == "1" ]]; then
  jq -n --argjson count "$COUNT" --arg run_dir "$run_dir" \
    '{requested:$count, queued_redeems:$count, run_dir:$run_dir, stopped_after_redeems:true}' >"$run_dir/partial-summary.json"
  cat "$run_dir/partial-summary.json"
  exit 0
fi

log "parallel Flow 3: waiting for outbounds"
wait_all_txouts_signed "out" 1200 "$run_dir/withdrawal-ids.txt" "$run_dir"

>"$run_dir/out-hashes.txt"
for i in $(seq 1 "$COUNT"); do
  d="$run_dir/$i"
  withdrawal_id="$(cat "$d/withdrawal-id.txt")"
  out_hash="$(jq -r --arg in_hash "$withdrawal_id" '.txout.tx_array[] | select(.in_hash == $in_hash) | .out_hash' "$run_dir/${withdrawal_id}-out-txout.json" | head -n1)"
  printf '%s\n' "$out_hash" >"$d/out-hash.txt"
  printf '%s\n' "$out_hash" >>"$run_dir/out-hashes.txt"
  printf '%s,%s,%s,%s\n' "$i" "$(cat "$d/deposit-id.txt")" "$withdrawal_id" "$out_hash" >>"$run_dir/results.csv"
done

log "parallel Flow 3: waiting for observed outbounds"
wait_all_observed_out_final "$run_dir/out-hashes.txt" 900

jq -n --argjson count "$COUNT" --arg run_dir "$run_dir" \
  --arg duplicate_recipient "$DUPLICATE_RECIPIENT" \
  --arg deposit_amounts_csv "$DEPOSIT_AMOUNTS_CSV" \
  '{requested:$count, success:$count, failed:0, run_dir:$run_dir, duplicate_recipient:($duplicate_recipient == "1"), deposit_amounts_csv:$deposit_amounts_csv}' >"$run_dir/summary.json"
cat "$run_dir/summary.json"
