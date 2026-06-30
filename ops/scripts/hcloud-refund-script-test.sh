#!/usr/bin/env bash
set -euo pipefail

RUN_ROOT="${RUN_ROOT:-/tmp/thornado-nodeper-20260628131009}"
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

mkdir -p "$RUN_ROOT/meta/refund-scripts"
run_dir="$RUN_ROOT/meta/refund-scripts/$(date -u +%Y%m%d%H%M%S)"
mkdir -p "$run_dir"
printf 'case,status,detail\n' >"$run_dir/results.csv"

json_btc_amount() {
  local amount="$1"
  jq -cn --arg amount "$amount" '$amount | tonumber'
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

find_vout_for_address_amount() {
  local tx_file="$1" address="$2" sats="$3"
  jq -r --arg address "$address" --argjson sats "$sats" '
    [.vout[] | select(.scriptPubKey.address == $address and (((.value * 100000000 + 0.5) | floor) == $sats))] |
    if length == 1 then .[0].n else empty end
  ' "$tx_file"
}

fund_source_utxo() {
  local label="$1" address_type="$2" dir="$3" source txid utxo
  source="$(btc_cli -rpcwallet=miner getnewaddress "${label}-source" "$address_type")"
  printf '%s\n' "$source" >"$dir/source-address.txt"
  txid="$(btc_cli -rpcwallet=miner sendtoaddress "$source" 0.25000000 "" "" false true)"
  printf '%s\n' "$txid" >"$dir/source-fund-txid.txt"
  mine_regtest_blocks 1
  utxo="$(btc_cli -rpcwallet=miner listunspent 1 9999999 "[\"$source\"]" | jq -c '[.[] | select((.amount | tonumber) >= 0.25)] | sort_by(.confirmations) | .[-1]')"
  [[ -n "$utxo" && "$utxo" != "null" ]] || die "${label} source UTXO was not available"
  printf '%s\n' "$utxo" >"$dir/source-utxo.json"
}

send_direct_vault_deposit() {
  local label="$1" dir="$2" vault_addr="$3" utxo in_txid in_vout in_amount fee deposit_amount change inputs outputs raw signed txid
  utxo="$(cat "$dir/source-utxo.json")"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  in_amount="$(jq -r '.amount' <<<"$utxo")"
  fee="0.00002000"
  deposit_amount="0.20000000"
  change="$(awk -v input="$in_amount" -v amount="$deposit_amount" -v fee="$fee" 'BEGIN {c = input - amount - fee; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  outputs="$(jq -nc \
    --arg vault "$vault_addr" \
    --arg change_addr "$(btc_cli -rpcwallet=miner getrawchangeaddress)" \
    --argjson amount "$(json_btc_amount "$deposit_amount")" \
    --argjson change "$(json_btc_amount "$change")" \
    '[{($vault):$amount},{($change_addr):$change}]')"
  printf '%s\n' "$inputs" >"$dir/direct-inputs.json"
  printf '%s\n' "$outputs" >"$dir/direct-outputs.json"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$outputs")"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$raw")"
  printf '%s\n' "$signed" >"$dir/direct-signed.json"
  jq -e '.complete == true' <<<"$signed" >/dev/null || die "${label} direct deposit did not sign"
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$(jq -r '.hex' <<<"$signed")" 0)"
  printf '%s\n' "$txid" >"$dir/direct-txid.txt"
  mine_regtest_blocks 2
  btc_cli getrawtransaction "$txid" true >"$dir/direct-btc-tx.json"
}

assert_refund_case() {
  local label="$1" dir="$2" vault_addr="$3" source upper txid vout out_hash deposit_record
  source="$(cat "$dir/source-address.txt")"
  txid="$(cat "$dir/direct-txid.txt")"
  upper="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  vout="$(find_vout_for_address_amount "$dir/direct-btc-tx.json" "$vault_addr" 20000000)"
  [[ "$vout" =~ ^[0-9]+$ ]] || die "${label} direct vault output vout was not unique"
  printf '%s\n' "$vout" >"$dir/direct-vout.txt"
  wait_observed_tx_final "$upper" "${label}-inbound" 900
  deposit_record="$(wait_deposit_return_complete "$upper" "$dir/deposit-record.json" 900)"
  jq -e --arg txid "$upper" --arg addr "$vault_addr" --argjson vout "$vout" '
    .status == "return_complete" and
    .deposit_id == $txid and
    .deposit_address == $addr and
    ((.source_vout // 0) | tonumber) == $vout and
    ((.amount_sats // 0) | tonumber) == 20000000
  ' <<<"$deposit_record" >/dev/null || die "${label} deposit record was not return_complete"
  out_hash="$(wait_wallet_refund_to_source "$source" "$dir" 900)"
  wait_observed_tx_final "$out_hash" "${label}-refund" 900
  printf '%s,pass,%s/%s/%s\n' "$label" "$upper" "$out_hash" "$source" >>"$run_dir/results.csv"
}

wait_deposit_return_complete() {
  local deposit_id="$1" out_file="$2" timeout="${3:-900}" start record
  start="$(date +%s)"
  while true; do
    record="$(curl -fsS "$(api_url 1)/thornado/deposit/${deposit_id}" || true)"
    if [[ -n "$record" ]] && jq -e '.status == "return_complete"' <<<"$record" >/dev/null 2>&1; then
      printf '%s\n' "$record" >"$out_file"
      printf '%s\n' "$record"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      printf '%s\n' "$record" >"$out_file"
      die "deposit ${deposit_id} did not reach return_complete"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
}

wait_wallet_refund_to_source() {
  local source="$1" dir="$2" timeout="${3:-900}" start txid raw
  start="$(date +%s)"
  while true; do
    btc_cli -rpcwallet=miner listtransactions "*" 10000 0 true >"$dir/wallet-transactions.json"
    txid="$(jq -r --arg source "$source" '
      [.[] | select(.category == "receive" and .address == $source and ((.amount * 100000000 + 0.5) | floor) < 20000000 and ((.amount * 100000000 + 0.5) | floor) > 19000000)] |
      sort_by(.time) | last | .txid // empty
    ' "$dir/wallet-transactions.json")"
    if [[ -n "$txid" ]]; then
      btc_cli getrawtransaction "$txid" true >"$dir/refund-btc-tx.json"
      raw="$(cat "$dir/refund-btc-tx.json")"
      jq -e --arg source "$source" '
        any(.vout[]; .scriptPubKey.address == $source and (((.value * 100000000 + 0.5) | floor) < 20000000) and (((.value * 100000000 + 0.5) | floor) > 19000000))
      ' <<<"$raw" >/dev/null || die "refund tx ${txid} did not pay source address"
      printf '%s\n' "$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')" >"$dir/refund-txid.txt"
      cat "$dir/refund-txid.txt"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      die "refund to source address ${source} was not found in wallet"
    fi
    mine_regtest_blocks 1 || true
    sleep 1
  done
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

curl -fsS "$(api_url 1)/thornado/vaults/base" >"$run_dir/base-vaults.json"
vault_pub="$(jq -r '[.[] | select(.status == "ActiveVault")][0].pub_key // empty' "$run_dir/base-vaults.json")"
[[ -n "$vault_pub" ]] || die "could not find active base vault"
vault_addr="$(jq -r --arg vault "$vault_pub" '.[] | select(.pub_key == $vault) | .addresses[]? | select(.chain == "BTC") | .address' "$run_dir/base-vaults.json" | head -n1)"
if [[ -z "$vault_addr" || "$vault_addr" == "null" ]]; then
  vault_addr="$("$SHIELDER_HELPER" btc-address "$vault_pub" 0)"
fi
printf '%s\n' "$vault_addr" >"$run_dir/base-vault-address.txt"

for spec in "p2wpkh bech32" "p2tr bech32m" "p2sh-segwit p2sh-segwit"; do
  read -r label address_type <<<"$spec"
  dir="$run_dir/$label"
  mkdir -p "$dir"
  log "refund-script ${label}: funding source UTXO"
  fund_source_utxo "$label" "$address_type" "$dir"
  log "refund-script ${label}: sending direct base-vault deposit"
  send_direct_vault_deposit "$label" "$dir" "$vault_addr"
  assert_refund_case "$label" "$dir" "$vault_addr"
done

assert_signer_queues_empty
jq -n --arg run_dir "$run_dir" --arg status pass '{run_dir:$run_dir,status:$status}' >"$run_dir/summary.json"
cat "$run_dir/summary.json"
