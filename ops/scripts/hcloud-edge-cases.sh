#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260627104200}"
ROOT_DIR="${ROOT_DIR:-/root/thornado}"
INVENTORY="${INVENTORY:-${ROOT_DIR}/ops/distributed-regtest-nodeper.env}"
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

signer_url() {
  local port
  if port="$(node_spec_field "$1" signer)"; then
    printf 'http://%s:%s\n' "$(node_host "$1")" "$port"
    return 0
  fi
  printf 'http://%s:%s\n' "$(node_host "$1")" "$((FROST_INFO_BASE + $1))"
}

if [[ -z "${THORNADO_TX_NODE:-}" ]]; then
  if tx_rpc_port="$(node_spec_field 1 rpc)"; then
    export THORNADO_TX_NODE="tcp://$(node_host 1):${tx_rpc_port}"
  else
    export THORNADO_TX_NODE="tcp://$(node_host 1):$((RPC_BASE + 1))"
  fi
fi

mkdir -p "$RUN_ROOT/meta/edge-cases"
run_dir="$RUN_ROOT/meta/edge-cases/$(date -u +%Y%m%d%H%M%S)"
mkdir -p "$run_dir"
printf 'case,status,detail\n' >"$run_dir/results.csv"

json_btc_amount() {
  local amount="$1"
  jq -cn --arg amount "$amount" '$amount | tonumber'
}

op_return_hex() {
  printf '%s' "$1" | xxd -p -c 256
}

send_raw_outputs() {
  local label="$1" outputs="$2" dir="$3" raw funded signed txid
  printf '%s\n' "$outputs" >"$dir/${label}-outputs.json"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "[]" "$outputs")"
  printf '%s\n' "$raw" >"$dir/${label}-raw.hex"
  funded="$(btc_cli -rpcwallet=miner fundrawtransaction "$raw")"
  printf '%s\n' "$funded" >"$dir/${label}-funded.json"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$(jq -r '.hex' <<<"$funded")")"
  printf '%s\n' "$signed" >"$dir/${label}-signed.json"
  jq -e '.complete == true' <<<"$signed" >/dev/null || die "${label} raw BTC tx did not sign"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$signed")" 0)"
  [[ "$txid" =~ ^[0-9a-f]{64}$ ]] || die "${label} raw BTC tx returned invalid txid: ${txid}"
  printf '%s\n' "$txid" >"$dir/${label}-txid.txt"
  btc_cli getrawtransaction "$txid" true >"$dir/${label}-btc-tx-before-mine.json"
  echo "$txid"
}

select_miner_utxo() {
  local min_btc="$1"
  btc_cli -rpcwallet=miner listunspent 1 9999999 |
    jq -c --argjson min "$min_btc" '[.[] | select((.spendable // true) == true and (.amount | tonumber) > $min)] | sort_by(.amount) | .[0]'
}

send_single_input_outputs() {
  local label="$1" inputs="$2" outputs="$3" dir="$4" raw signed txid
  printf '%s\n' "$inputs" >"$dir/${label}-inputs.json"
  printf '%s\n' "$outputs" >"$dir/${label}-outputs.json"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$outputs")"
  printf '%s\n' "$raw" >"$dir/${label}-raw.hex"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$raw")"
  printf '%s\n' "$signed" >"$dir/${label}-signed.json"
  jq -e '.complete == true' <<<"$signed" >/dev/null || die "${label} raw BTC tx did not sign"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$signed")" 0)"
  [[ "$txid" =~ ^[0-9a-f]{64}$ ]] || die "${label} raw BTC tx returned invalid txid: ${txid}"
  printf '%s\n' "$txid" >"$dir/${label}-txid.txt"
  btc_cli getrawtransaction "$txid" true >"$dir/${label}-btc-tx-before-mine.json"
  echo "$txid"
}

wait_observed_tx_final() {
  local hash="$1" label="$2" timeout="${3:-900}" start res
  start="$(date +%s)"
  while true; do
    res="$(curl_json_quiet "$(api_url 1)/thornado/tx/${hash}" || true)"
    if [[ -n "$res" ]] && jq -e '(.stages.inbound_observed.completed == true) and ((.stages.inbound_observed.final_count // 0) >= 3)' <<<"$res" >/dev/null 2>&1; then
      printf '%s\n' "$res" >"$run_dir/${label}-observed-final.json"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      printf '%s\n' "$res" >"$run_dir/${label}-observed-timeout.json"
      die "${label} observed tx did not finalize"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

request_edge_deposit() {
  local label="$1" dir="$2" deposit_pubkey owner session
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${label}-deposit-pubkey")"
  owner="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  printf '%s\n' "$deposit_pubkey" >"$dir/deposit-pubkey.txt"
  printf '%s\n' "$owner" >"$dir/owner-address.txt"
  request_deposit "$RUN_ROOT/node1" "user" "$label" "$deposit_pubkey" >"$dir/request-deposit.json"
  session="$(deposit_session "$owner")"
  printf '%s\n' "$session" >"$dir/session-before-btc.json"
  jq -e '.status == "address_issued" and (.deposit_address | length) > 0 and (.deposit_path_index | tonumber) > 0' "$dir/session-before-btc.json" >/dev/null \
    || die "${label} did not get a fresh deposit session"
}

find_vout_for_address_amount() {
  local tx_file="$1" address="$2" sats="$3"
  jq -r --arg address "$address" --argjson sats "$sats" '
    [.vout[] | select(.scriptPubKey.address == $address and (((.value * 100000000 + 0.5) | floor) == $sats))] |
    if length == 1 then .[0].n else empty end
  ' "$tx_file"
}

validate_matched_sweep() {
  local label="$1" dir="$2" txid="$3" owner="$4" addr="$5" path="$6" vout="$7" expected_sats="${8:-20000000}"
  local matched deposit_record deposit_id amount_sats sweep_txout out_hash
  matched="$(wait_owner_deposit_matched "$owner" 420)"
  printf '%s\n' "$matched" >"$dir/${label}-deposit-matched.json"
  deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
  deposit_record="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}")"
  printf '%s\n' "$deposit_record" >"$dir/${label}-deposit-record.json"
  amount_sats="$(jq -r '.amount_sats' <<<"$deposit_record")"
  jq -e \
    --arg addr "$addr" \
    --arg txid "$txid" \
    --argjson path "$path" \
    '.status == "deposit_matched" and
      .deposit_address == $addr and
      (.inbound_tx_id | ascii_downcase) == ($txid | ascii_downcase) and
      (.deposit_path_index | tonumber) == $path' <<<"$matched" >/dev/null \
    || die "${label} deposit was misclassified"
  jq -e \
    --arg addr "$addr" \
    --arg txid "$txid" \
    --argjson expected "$expected_sats" \
    '.status == "deposit_matched" and
      .deposit_address == $addr and
      (.inbound_tx_id | ascii_downcase) == ($txid | ascii_downcase) and
      (.amount_sats | tonumber) == $expected' <<<"$deposit_record" >/dev/null \
    || die "${label} deposit record had wrong amount"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 900)"
  printf '%s\n' "$sweep_txout" >"$dir/${label}-sweep-txout.json"
  jq -e --arg in_hash "$deposit_id" --argjson vout "$vout" --argjson expected "$expected_sats" '
    .txout.tx_array[] |
    select(.tx_type == "sweep" and .in_hash == $in_hash) |
    ((.source_inputs // []) | length) == 1 and
    ((.source_inputs[0].vout | tonumber) == $vout) and
    ((.source_inputs[0].amount_sats | tonumber) == $expected)
  ' <<<"$sweep_txout" >/dev/null || die "${label} sweep source input did not match deposit vout"
  out_hash="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .out_hash' <<<"$sweep_txout" | head -n1)"
  [[ -n "$out_hash" && "$out_hash" != "null" ]] || die "${label} sweep had no out_hash"
  wait_observed_tx_final "$out_hash" "${label}-sweep" 900
  wait_btc_utxo_spent "$txid" "$vout" "$dir/${label}-deposit-utxo-after-sweep.json" 300 \
    || die "${label} deposit UTXO remained spendable after sweep"
  printf '%s,%s,%s/%s/%s/vout%s\n' "$label" "pass" "$txid" "$deposit_id" "$amount_sats" "$vout" >>"$run_dir/results.csv"
}

case_direct_base_vault_refund() {
  local label="direct-base-vault-refund" dir vaults vault_pub vault_addr utxo in_txid in_vout in_amount fee non_change_total change inputs outputs txid upper raw vout change_addr refund_txout out_hash deposit_record
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "edge ${label}: sending a user deposit directly to the active base vault"
  curl -fsS "$(api_url 1)/thornado/vaults/base" >"$dir/base-vaults.json"
  vault_pub="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key // empty' "$dir/base-vaults.json")"
  [[ -n "$vault_pub" ]] || die "${label} could not find active base vault"
  vault_addr="$(jq -r --arg vault "$vault_pub" '.[] | select(.pub_key == $vault) | .addresses[]? | select(.chain == "BTC") | .address' "$dir/base-vaults.json" | head -n1)"
  if [[ -z "$vault_addr" || "$vault_addr" == "null" ]]; then
    vault_addr="$("$SHIELDER_HELPER" btc-address "$vault_pub" 0)"
  fi
  [[ -n "$vault_addr" ]] || die "${label} could not derive active base vault address"
  printf '%s\n' "$vault_pub" >"$dir/base-vault-pubkey.txt"
  printf '%s\n' "$vault_addr" >"$dir/base-vault-address.txt"
  utxo="$(select_miner_utxo 1)"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00002000"
  non_change_total="0.20000000"
  change="$(awk -v input="$in_amount" -v total="$non_change_total" -v fee="$fee" 'BEGIN {c = input - total - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  change_addr="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  outputs="$(jq -nc \
    --arg vault "$vault_addr" --arg change_addr "$change_addr" \
    --argjson amount "$(json_btc_amount 0.20000000)" \
    --argjson change "$(json_btc_amount "$change")" \
    '[{($vault):$amount},{($change_addr):$change}]')"
  txid="$(send_single_input_outputs "$label" "$inputs" "$outputs" "$dir")"
  upper="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/${label}-btc-tx-after-mine.json"
  vout="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$vault_addr" 20000000)"
  [[ "$vout" =~ ^[0-9]+$ ]] || die "${label} base-vault output vout was not unique"
  printf '%s\n' "$vout" >"$dir/base-vault-vout.txt"
  wait_observed_tx_final "$upper" "${label}-inbound" 900
  refund_txout="$(wait_txout_signed_by_in_hash "$upper" "refund" 900)"
  printf '%s\n' "$refund_txout" >"$dir/refund-txout.json"
  jq -e --arg in_hash "$upper" '
    .txout.tx_array[] |
    select(.tx_type == "refund" and .in_hash == $in_hash) |
    (.out_hash // "") != "" and
    ((.source_inputs // []) | length) == 1 and
    ((.coin.amount | tonumber) > 0) and
    ((.coin.amount | tonumber) < 20000000)
  ' <<<"$refund_txout" >/dev/null || die "${label} refund txout was not signed with source inputs"
  jq -e --arg in_hash "$upper" --argjson vout "$vout" '
    .txout.tx_array[] |
    select(.tx_type == "refund" and .in_hash == $in_hash) |
    ((.source_inputs[0].tx_id // .source_inputs[0].txid) | ascii_upcase) == $in_hash and
    ((.source_inputs[0].vout | tonumber) == $vout) and
    ((.source_inputs[0].amount_sats | tonumber) == 20000000)
  ' <<<"$refund_txout" >/dev/null || die "${label} refund did not use direct deposit source input"
  out_hash="$(jq -r --arg in_hash "$upper" '.txout.tx_array[] | select(.tx_type == "refund" and .in_hash == $in_hash) | .out_hash' <<<"$refund_txout" | head -n1)"
  wait_observed_tx_final "$out_hash" "${label}-refund" 900
  deposit_record="$(curl -fsS "$(api_url 1)/thornado/deposit/${upper}")"
  printf '%s\n' "$deposit_record" >"$dir/deposit-record.json"
  jq -e --arg txid "$upper" --arg addr "$vault_addr" --argjson vout "$vout" '
    .status == "return_complete" and
    .deposit_id == $txid and
    .deposit_address == $addr and
    ((.source_vout // 0) | tonumber) == $vout and
    (.amount_sats | tonumber) == 20000000
  ' <<<"$deposit_record" >/dev/null || die "${label} direct base-vault deposit record was not return_complete"
  printf '%s,pass,%s/%s/vout%s\n' "$label" "$txid" "$out_hash" "$vout" >>"$run_dir/results.csv"
}

case_exact_vout5_op_return_before() {
  local label="exact-vout5-op-return-before" seed dir session addr path utxo in_txid in_vout in_amount fee non_change_total change inputs outputs txid raw vout other1 other2 other3 other4 change_addr
  seed="edge-${label}-$(date +%s)"
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "edge ${label}: forcing vault output to vout 5"
  request_edge_deposit "$seed" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  path="$(jq -r '.deposit_path_index' <<<"$session")"
  utxo="$(select_miner_utxo 1)"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00002000"
  non_change_total="0.21000000"
  change="$(awk -v input="$in_amount" -v total="$non_change_total" -v fee="$fee" 'BEGIN {c = input - total - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  other1="$(btc_cli -rpcwallet=miner getnewaddress)"
  other2="$(btc_cli -rpcwallet=miner getnewaddress)"
  other3="$(btc_cli -rpcwallet=miner getnewaddress)"
  other4="$(btc_cli -rpcwallet=miner getnewaddress)"
  change_addr="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  outputs="$(jq -nc \
    --arg data "$(op_return_hex "$seed")" \
    --arg o1 "$other1" --arg o2 "$other2" --arg o3 "$other3" --arg o4 "$other4" \
    --arg deposit "$addr" --arg change_addr "$change_addr" \
    --argjson a1 "$(json_btc_amount 0.00100000)" \
    --argjson a2 "$(json_btc_amount 0.00200000)" \
    --argjson a3 "$(json_btc_amount 0.00300000)" \
    --argjson a4 "$(json_btc_amount 0.00400000)" \
    --argjson deposit_amount "$(json_btc_amount 0.20000000)" \
    --argjson change "$(json_btc_amount "$change")" \
    '[{data:$data},{($o1):$a1},{($o2):$a2},{($o3):$a3},{($o4):$a4},{($deposit):$deposit_amount},{($change_addr):$change}]')"
  txid="$(send_single_input_outputs "$label" "$inputs" "$outputs" "$dir")"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/${label}-btc-tx-after-mine.json"
  vout="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$addr" 20000000)"
  [[ "$vout" == "5" ]] || die "${label} expected vault output at vout 5, got ${vout:-empty}"
  validate_matched_sweep "$label" "$dir" "$txid" "$(cat "$dir/owner-address.txt")" "$addr" "$path" "$vout" 20000000
}

case_op_return_after_vault() {
  local label="op-return-after-vault" seed dir session addr path utxo in_txid in_vout in_amount fee non_change_total change inputs outputs txid raw vout other1 other2 change_addr
  seed="edge-${label}-$(date +%s)"
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "edge ${label}: placing OP_RETURN after the vault output"
  request_edge_deposit "$seed" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  path="$(jq -r '.deposit_path_index' <<<"$session")"
  utxo="$(select_miner_utxo 1)"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00002000"
  non_change_total="0.20300000"
  change="$(awk -v input="$in_amount" -v total="$non_change_total" -v fee="$fee" 'BEGIN {c = input - total - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  other1="$(btc_cli -rpcwallet=miner getnewaddress)"
  other2="$(btc_cli -rpcwallet=miner getnewaddress)"
  change_addr="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  outputs="$(jq -nc \
    --arg data "$(op_return_hex "$seed")" \
    --arg o1 "$other1" --arg o2 "$other2" --arg deposit "$addr" --arg change_addr "$change_addr" \
    --argjson a1 "$(json_btc_amount 0.00100000)" \
    --argjson deposit_amount "$(json_btc_amount 0.20000000)" \
    --argjson a2 "$(json_btc_amount 0.00200000)" \
    --argjson change "$(json_btc_amount "$change")" \
    '[{($o1):$a1},{($deposit):$deposit_amount},{data:$data},{($o2):$a2},{($change_addr):$change}]')"
  txid="$(send_single_input_outputs "$label" "$inputs" "$outputs" "$dir")"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/${label}-btc-tx-after-mine.json"
  vout="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$addr" 20000000)"
  [[ "$vout" == "1" ]] || die "${label} expected vault output at vout 1, got ${vout:-empty}"
  validate_matched_sweep "$label" "$dir" "$txid" "$(cat "$dir/owner-address.txt")" "$addr" "$path" "$vout" 20000000
}

case_multi_output_one_vault() {
  local label="edge-multi-output-one-vault-$(date +%s)" dir session addr path outputs txid raw vout matched deposit_record deposit_id amount_sats sweep_txout out_hash unrelated1 unrelated2
  dir="$run_dir/multi-output-one-vault"
  mkdir -p "$dir"
  log "edge multi-output-one-vault: requesting deposit"
  request_edge_deposit "$label" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  path="$(jq -r '.deposit_path_index' <<<"$session")"
  unrelated1="$(btc_cli -rpcwallet=miner getnewaddress)"
  unrelated2="$(btc_cli -rpcwallet=miner getnewaddress)"
  outputs="$(jq -nc \
    --arg data "$(op_return_hex "$label")" \
    --arg unrelated1 "$unrelated1" \
    --arg deposit "$addr" \
    --arg unrelated2 "$unrelated2" \
    --argjson amount1 "$(json_btc_amount 0.00100000)" \
    --argjson deposit_amount "$(json_btc_amount 0.20000000)" \
    --argjson amount2 "$(json_btc_amount 0.00200000)" \
    '[{data:$data},{($unrelated1):$amount1},{($deposit):$deposit_amount},{($unrelated2):$amount2}]')"
  txid="$(send_raw_outputs multi-output "$outputs" "$dir")"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/multi-output-btc-tx-after-mine.json"
  vout="$(find_vout_for_address_amount "$dir/multi-output-btc-tx-after-mine.json" "$addr" 20000000)"
  [[ "$vout" =~ ^[0-9]+$ ]] || die "multi-output deposit vout was not unique"
  printf '%s\n' "$vout" >"$dir/deposit-vout.txt"
  matched="$(wait_owner_deposit_matched "$(cat "$dir/owner-address.txt")" 420)"
  printf '%s\n' "$matched" >"$dir/deposit-matched.json"
  deposit_id="$(jq -r '.deposit_id' <<<"$matched")"
  deposit_record="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}")"
  printf '%s\n' "$deposit_record" >"$dir/deposit-record.json"
  amount_sats="$(jq -r '.amount_sats' <<<"$deposit_record")"
  jq -e \
    --arg addr "$addr" \
    --arg txid "$txid" \
    --argjson path "$path" \
    '.status == "deposit_matched" and
      .deposit_address == $addr and
      (.inbound_tx_id | ascii_downcase) == ($txid | ascii_downcase) and
      (.deposit_path_index | tonumber) == $path' <<<"$matched" >/dev/null \
    || die "multi-output deposit was misclassified"
  jq -e \
    --arg addr "$addr" \
    --arg txid "$txid" \
    '.status == "deposit_matched" and
      .deposit_address == $addr and
      (.inbound_tx_id | ascii_downcase) == ($txid | ascii_downcase) and
      (.amount_sats | tonumber) == 20000000' <<<"$deposit_record" >/dev/null \
    || die "multi-output deposit record had wrong amount"
  sweep_txout="$(wait_sweep_signed "$deposit_id" 900)"
  printf '%s\n' "$sweep_txout" >"$dir/sweep-txout.json"
  jq -e --arg in_hash "$deposit_id" --argjson vout "$vout" '
    .txout.tx_array[] |
    select(.tx_type == "sweep" and .in_hash == $in_hash) |
    ((.source_inputs // []) | length) == 1 and
    ((.source_inputs[0].vout | tonumber) == $vout) and
    ((.source_inputs[0].amount_sats | tonumber) == 20000000)
  ' <<<"$sweep_txout" >/dev/null || die "multi-output sweep source input did not match deposit vout"
  out_hash="$(jq -r --arg in_hash "$deposit_id" '.txout.tx_array[] | select(.tx_type == "sweep" and .in_hash == $in_hash) | .out_hash' <<<"$sweep_txout" | head -n1)"
  [[ -n "$out_hash" && "$out_hash" != "null" ]] || die "multi-output sweep had no out_hash"
  wait_observed_tx_final "$out_hash" "multi-output-sweep" 900
  wait_btc_utxo_spent "$txid" "$vout" "$dir/deposit-utxo-after-sweep.json" 300 \
    || die "multi-output deposit UTXO remained spendable after sweep"
  printf 'multi-output-one-vault,pass,%s/%s/%s\n' "$txid" "$deposit_id" "$amount_sats" >>"$run_dir/results.csv"
}

case_vault_output_plus_dust_change_duplicate_amount() {
  local label="vault-dust-change-duplicate-amount" seed dir session addr path utxo in_txid in_vout in_amount fee non_change_total change inputs outputs txid raw vout dust_addr duplicate_addr other_addr change_addr
  seed="edge-${label}-$(date +%s)"
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "edge ${label}: vault output with dust/change and duplicate-looking amount"
  request_edge_deposit "$seed" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  path="$(jq -r '.deposit_path_index' <<<"$session")"
  utxo="$(select_miner_utxo 1)"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00003000"
  non_change_total="0.40100546"
  change="$(awk -v input="$in_amount" -v total="$non_change_total" -v fee="$fee" 'BEGIN {c = input - total - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  dust_addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  duplicate_addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  other_addr="$(btc_cli -rpcwallet=miner getnewaddress)"
  change_addr="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  outputs="$(jq -nc \
    --arg dust_addr "$dust_addr" --arg duplicate "$duplicate_addr" --arg deposit "$addr" --arg other "$other_addr" --arg change_addr "$change_addr" \
    --argjson dust "$(json_btc_amount 0.00000546)" \
    --argjson duplicate_amount "$(json_btc_amount 0.20000000)" \
    --argjson deposit_amount "$(json_btc_amount 0.20000000)" \
    --argjson other_amount "$(json_btc_amount 0.00100000)" \
    --argjson change "$(json_btc_amount "$change")" \
    '[{($dust_addr):$dust},{($duplicate):$duplicate_amount},{($deposit):$deposit_amount},{($other):$other_amount},{($change_addr):$change}]')"
  txid="$(send_single_input_outputs "$label" "$inputs" "$outputs" "$dir")"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/${label}-btc-tx-after-mine.json"
  vout="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$addr" 20000000)"
  [[ "$vout" =~ ^[0-9]+$ ]] || die "${label} deposit output vout was not unique"
  validate_matched_sweep "$label" "$dir" "$txid" "$(cat "$dir/owner-address.txt")" "$addr" "$path" "$vout" 20000000
}

case_multiple_vault_outputs_one_tx() {
  local label="multiple-vault-outputs-one-tx" seed dir dir_a dir_b session_a session_b addr_a addr_b owner_a owner_b path_a path_b outputs txid raw vout_a vout_b start current_a current_b matched_a matched_b status_a status_b upper
  seed="edge-${label}-$(date +%s)"
  dir="$run_dir/$label"
  dir_a="$dir/deposit-a"
  dir_b="$dir/deposit-b"
  mkdir -p "$dir_a" "$dir_b"
  log "edge ${label}: two registered deposit outputs in one BTC tx"
  request_edge_deposit "${seed}-a" "$dir_a"
  request_edge_deposit "${seed}-b" "$dir_b"
  session_a="$(cat "$dir_a/session-before-btc.json")"
  session_b="$(cat "$dir_b/session-before-btc.json")"
  addr_a="$(jq -r '.deposit_address' <<<"$session_a")"
  addr_b="$(jq -r '.deposit_address' <<<"$session_b")"
  owner_a="$(cat "$dir_a/owner-address.txt")"
  owner_b="$(cat "$dir_b/owner-address.txt")"
  path_a="$(jq -r '.deposit_path_index' <<<"$session_a")"
  path_b="$(jq -r '.deposit_path_index' <<<"$session_b")"
  outputs="$(jq -nc \
    --arg a "$addr_a" --arg b "$addr_b" \
    --argjson amount_a "$(json_btc_amount 0.20000000)" \
    --argjson amount_b "$(json_btc_amount 0.30000000)" \
    '[{($a):$amount_a},{($b):$amount_b}]')"
  txid="$(send_raw_outputs "$label" "$outputs" "$dir")"
  upper="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  mine_regtest_blocks 2
  raw="$(btc_cli getrawtransaction "$txid" true)"
  printf '%s\n' "$raw" >"$dir/${label}-btc-tx-after-mine.json"
  vout_a="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$addr_a" 20000000)"
  vout_b="$(find_vout_for_address_amount "$dir/${label}-btc-tx-after-mine.json" "$addr_b" 30000000)"
  [[ "$vout_a" =~ ^[0-9]+$ ]] || die "${label} first deposit output vout was not unique"
  [[ "$vout_b" =~ ^[0-9]+$ ]] || die "${label} second deposit output vout was not unique"
  printf '%s\n' "$vout_a" >"$dir/vout-a.txt"
  printf '%s\n' "$vout_b" >"$dir/vout-b.txt"
  wait_observed_tx_final "$upper" "${label}-inbound" 900

  start="$(date +%s)"
  matched_a=0
  matched_b=0
  while true; do
    current_a="$(deposit_session "$owner_a" 2>/dev/null || true)"
    current_b="$(deposit_session "$owner_b" 2>/dev/null || true)"
    printf '%s\n' "$current_a" >"$dir_a/session-after-btc.json"
    printf '%s\n' "$current_b" >"$dir_b/session-after-btc.json"
    jq -e '.status == "deposit_matched"' <<<"$current_a" >/dev/null 2>&1 && matched_a=1 || matched_a=0
    jq -e '.status == "deposit_matched"' <<<"$current_b" >/dev/null 2>&1 && matched_b=1 || matched_b=0
    if (( matched_a == 1 && matched_b == 1 )); then
      break
    fi
    if (( "$(date +%s)" - start >= 180 )); then
      curl_json_quiet "$(api_url 1)/thornado/tx/${upper}" >"$dir/tx-stages-timeout.json" || true
      status_a="$(jq -r '.status // "missing"' <<<"$current_a" 2>/dev/null || printf 'missing')"
      status_b="$(jq -r '.status // "missing"' <<<"$current_b" 2>/dev/null || printf 'missing')"
      die "${label} did not match both deposit outputs in one BTC tx: a=${status_a} b=${status_b}"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done

  validate_matched_sweep "${label}-a" "$dir_a" "$txid" "$owner_a" "$addr_a" "$path_a" "$vout_a" 20000000
  validate_matched_sweep "${label}-b" "$dir_b" "$txid" "$owner_b" "$addr_b" "$path_b" "$vout_b" 30000000
  printf '%s,pass,%s/vout%s/vout%s\n' "$label" "$txid" "$vout_a" "$vout_b" >>"$run_dir/results.csv"
}

case_dust_deposit_ignored() {
  local label="edge-dust-$(date +%s)" dir session addr outputs txid owner start current deposit_id
  dir="$run_dir/dust-deposit"
  mkdir -p "$dir"
  log "edge dust-deposit: sending dust to a registered deposit address"
  request_edge_deposit "$label" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  owner="$(cat "$dir/owner-address.txt")"
  outputs="$(jq -nc \
    --arg data "$(op_return_hex "$label")" \
    --arg deposit "$addr" \
    --argjson dust "$(json_btc_amount 0.00000545)" \
    '[{data:$data},{($deposit):$dust}]')"
  txid="$(send_raw_outputs dust "$outputs" "$dir")"
  mine_regtest_blocks 2
  start="$(date +%s)"
  while true; do
    current="$(deposit_session "$owner" 2>/dev/null || true)"
    printf '%s\n' "$current" >"$dir/session-after-dust.json"
    deposit_id="$(jq -r '.deposit_id // ""' <<<"$current" 2>/dev/null || true)"
    if [[ -n "$deposit_id" && "$deposit_id" != "null" ]]; then
      die "dust deposit unexpectedly matched deposit_id=${deposit_id}"
    fi
    if (( "$(date +%s)" - start >= 75 )); then
      break
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
  if find_signed_sweep_txout "$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')" >"$dir/unexpected-dust-sweep.json"; then
    die "dust deposit queued a sweep"
  fi
  printf 'dust-deposit,pass,%s\n' "$txid" >>"$run_dir/results.csv"
}

case_coinbase_to_deposit_address() {
  local label="coinbase-to-deposit-address" seed dir session addr owner block_hash txid start current deposit_id
  seed="edge-${label}-$(date +%s)"
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "edge ${label}: mining coinbase directly to a registered deposit address"
  request_edge_deposit "$seed" "$dir"
  session="$(cat "$dir/session-before-btc.json")"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  owner="$(cat "$dir/owner-address.txt")"
  block_hash="$(btc_cli -rpcwallet=miner generatetoaddress 1 "$addr" | jq -r '.[0]')"
  printf '%s\n' "$block_hash" >"$dir/block-hash.txt"
  btc_cli getblock "$block_hash" 2 >"$dir/block.json"
  txid="$(jq -r '.tx[0].txid' "$dir/block.json")"
  printf '%s\n' "$txid" >"$dir/coinbase-txid.txt"
  start="$(date +%s)"
  while true; do
    current="$(deposit_session "$owner" 2>/dev/null || true)"
    printf '%s\n' "$current" >"$dir/session-after-coinbase.json"
    deposit_id="$(jq -r '.deposit_id // ""' <<<"$current" 2>/dev/null || true)"
    if [[ -n "$deposit_id" && "$deposit_id" != "null" ]]; then
      die "coinbase deposit unexpectedly matched deposit_id=${deposit_id}"
    fi
    if (( "$(date +%s)" - start >= 75 )); then
      break
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
  if find_signed_sweep_txout "$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')" >"$dir/unexpected-coinbase-sweep.json"; then
    die "coinbase deposit queued a sweep"
  fi
  printf '%s,pass,%s\n' "$label" "$txid" >>"$run_dir/results.csv"
}

run_edge_case() {
  case "$1" in
    multi-output-one-vault) case_multi_output_one_vault ;;
    vault-dust-change-duplicate-amount) case_vault_output_plus_dust_change_duplicate_amount ;;
    direct-base-vault-refund) case_direct_base_vault_refund ;;
    exact-vout5-op-return-before) case_exact_vout5_op_return_before ;;
    op-return-after-vault) case_op_return_after_vault ;;
    dust-deposit-ignored) case_dust_deposit_ignored ;;
    coinbase-to-deposit-address) case_coinbase_to_deposit_address ;;
    multiple-vault-outputs-one-tx) case_multiple_vault_outputs_one_tx ;;
    *) die "unknown edge case: $1" ;;
  esac
}

if [[ -n "${EDGE_CASES:-}" ]]; then
  for edge_case in $EDGE_CASES; do
    run_edge_case "$edge_case"
  done
else
  run_edge_case multi-output-one-vault
  run_edge_case vault-dust-change-duplicate-amount
  run_edge_case direct-base-vault-refund
  run_edge_case exact-vout5-op-return-before
  run_edge_case op-return-after-vault
  run_edge_case dust-deposit-ignored
  run_edge_case coinbase-to-deposit-address
  run_edge_case multiple-vault-outputs-one-tx
fi

signer_nodes="${SIGNER_NODES:-${WORKER_NODES:-5 6 7 8 9}}"
start="$(date +%s)"
while true; do
  all_empty=1
  for i in $signer_nodes; do
    curl -fsS --max-time 5 "$(signer_url "$i")/debug/signer/txouts" >"$run_dir/signer-node${i}-txouts.json" || all_empty=0
    jq -e 'length == 0' "$run_dir/signer-node${i}-txouts.json" >/dev/null || all_empty=0
  done
  if (( all_empty == 1 )); then
    break
  fi
  if (( "$(date +%s)" - start >= 120 )); then
    die "signer queues were not empty after edge run"
  fi
  sleep 2
done

jq -n \
  --arg run_dir "$run_dir" \
  '{run_dir:$run_dir,status:"pass"}' >"$run_dir/summary.json"
cat "$run_dir/summary.json"
