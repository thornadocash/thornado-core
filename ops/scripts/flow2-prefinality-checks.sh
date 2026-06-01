#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-real4}"
CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_CONTAINER="${BTC_CONTAINER:-thornado-real4-bitcoind}"

THORNADO="${BUILD_DIR}/thornado"
SHIELDER_HELPER="${BUILD_DIR}/shielder-e2e-helper"

log() {
  printf '[flow2-prefinality] %s\n' "$*"
}

die() {
  printf '[flow2-prefinality] ERROR: %s\n' "$*" >&2
  exit 1
}

key_show_addr() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" \
    --home "$home" --keyring-backend file -a
}

btc_cli() {
  docker exec "$BTC_CONTAINER" bitcoin-cli -regtest -rpcuser=thornado -rpcpassword=thornado "$@"
}

thornado_tx() {
  local home="$1" from="$2"
  shift 2
  local from_addr
  if [[ "$from" == tthor1* ]]; then
    from_addr="$from"
  else
    from_addr="$(key_show_addr "$home" "$from")"
  fi
  printf '%s\n' "$PASS" | "$THORNADO" tx thornado "$@" \
    --home "$home" \
    --from "$from_addr" \
    --keyring-backend file \
    --keyring-dir "$home" \
    --chain-id "$CHAIN_ID" \
    --node tcp://127.0.0.1:26657 \
    --gas 2500000 \
    --fees 0stake \
    --broadcast-mode sync \
    --yes \
    --output json
}

wait_blocks() {
  local count="$1" start latest
  start="$(curl -fsS http://127.0.0.1:26657/status | jq -r '.result.sync_info.latest_block_height')"
  while true; do
    latest="$(curl -fsS http://127.0.0.1:26657/status | jq -r '.result.sync_info.latest_block_height')"
    (( latest >= start + count )) && return 0
    sleep 1
  done
}

assert_tx_success() {
  local out="$1" label="$2" txhash start res code raw_log safe
  safe="${label// /-}"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}.json"
  jq -e '.code == null or .code == 0' <<<"$out" >/dev/null || die "$label failed CheckTx: $out"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || return 0
  start="$(date +%s)"
  while (( $(date +%s) - start < 60 )); do
    res="$(curl -fsS "http://127.0.0.1:26657/tx?hash=0x${txhash}" 2>/dev/null || true)"
    if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" == "0" ]]; then
        printf '%s\n' "$res" >"$RUN_ROOT/meta/${safe}-delivertx.json"
        return 0
      fi
      raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
      die "$label failed DeliverTx code=$code log=$raw_log"
    fi
    sleep 1
  done
  die "$label tx $txhash was not found"
}

assert_tx_rejected() {
  local out="$1" label="$2" txhash start res code raw_log safe
  safe="${label// /-}"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}-checktx.json"
  code="$(jq -r '.code // 0' <<<"$out")"
  if [[ "$code" != "0" ]]; then
    jq -r '.raw_log // .log // empty' <<<"$out" >"$RUN_ROOT/meta/${safe}-rejected.log"
    return 0
  fi
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || die "$label unexpectedly had no txhash and no error"
  start="$(date +%s)"
  while (( $(date +%s) - start < 60 )); do
    res="$(curl -fsS "http://127.0.0.1:26657/tx?hash=0x${txhash}" 2>/dev/null || true)"
    if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
      printf '%s\n' "$res" >"$RUN_ROOT/meta/${safe}-delivertx.json"
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" != "0" ]]; then
        raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
        printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
        return 0
      fi
      die "$label unexpectedly succeeded"
    fi
    sleep 1
  done
  die "$label tx $txhash was not found"
}

request_deposit() {
  local home="$1" owner="$2" pow="$3" out
  out="$(thornado_tx "$home" "$owner" request-deposit "$pow")"
  assert_tx_success "$out" "flow2 prefinality request-deposit"
  wait_blocks 2
}

deposit_session() {
  local owner_addr="$1"
  curl -fsS "http://127.0.0.1:1317/thornado/deposit/session/${owner_addr}"
}

mine_regtest_blocks() {
  local count="${1:-1}"
  btc_cli -rpcwallet=miner generatetoaddress "$count" "$(btc_cli -rpcwallet=miner getnewaddress)" >/dev/null
}

send_unconfirmed_deposit() {
  local address="$1" amount_btc="$2"
  local utxo in_txid in_vout utxo_amount change_address change_amount inputs outputs raw signed
  utxo="$(btc_cli -rpcwallet=miner listunspent 1 9999999 | jq -c 'map(select(.spendable == true and .amount > 1))[0]')"
  [[ -n "$utxo" && "$utxo" != "null" ]] || die "miner wallet has no spendable UTXO"
  in_txid="$(jq -r '.txid' <<<"$utxo")"
  in_vout="$(jq -r '.vout' <<<"$utxo")"
  utxo_amount="$(jq -r '.amount' <<<"$utxo")"
  change_address="$(btc_cli -rpcwallet=miner getrawchangeaddress)"
  change_amount="$(awk -v u="$utxo_amount" -v a="$amount_btc" 'BEGIN {c = u - a - 0.00002000; if (c <= 0) exit 1; printf "%.8f", c}')"
  inputs="$(jq -nc --arg txid "$in_txid" --argjson vout "$in_vout" '[{txid:$txid,vout:$vout}]')"
  outputs="$(jq -nc --arg address "$address" --argjson amount "$amount_btc" --arg change "$change_address" --argjson change_amount "$change_amount" '[{($address):$amount},{($change):$change_amount}]')"
  raw="$(btc_cli -rpcwallet=miner createrawtransaction "$inputs" "$outputs")"
  signed="$(btc_cli -rpcwallet=miner signrawtransactionwithwallet "$raw" | jq -r '.hex')"
  btc_cli -rpcwallet=miner sendrawtransaction "$signed"
}

wait_deposit_matched() {
  local deposit_id="$1" timeout="${2:-180}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" | jq -e '.status == "deposit_matched"' >/dev/null 2>&1; then
      curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}"
      return 0
    fi
    if (( "$(date +%s)" - start >= timeout )); then
      curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" >&2 || true
      die "deposit ${deposit_id} did not match"
    fi
    mine_regtest_blocks 1 || true
    sleep 2
  done
}

wait_deposit_committed() {
  local deposit_id="$1" timeout="${2:-120}" start
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" | jq -e '.status == "committed"' >/dev/null 2>&1; then
      curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}"
      return 0
    fi
    (( "$(date +%s)" - start < timeout )) || die "deposit ${deposit_id} did not commit"
    sleep 1
  done
}

main() {
  [[ -d "$RUN_ROOT/meta" ]] || die "missing run meta dir: $RUN_ROOT/meta"
  curl -fsS http://127.0.0.1:26657/status >/dev/null || die "thornado rpc is not live"

  source "$RUN_ROOT/meta/user.env"
  local user_addr="$address"
  local session deposit_address path_index txid deposit_id amount_sats receipt commitments out pre_query matched committed mempool_count
  amount_sats=20000000

  if [[ "${RESUME_PREFINALITY:-0}" == "1" ]]; then
    log "resuming existing unconfirmed BTC deposit"
    session="$(cat "$RUN_ROOT/meta/flow2-prefinality-session-before.json")"
    deposit_address="$(jq -r '.deposit_address' <<<"$session")"
    path_index="$(jq -r '.deposit_path_index' <<<"$session")"
    txid="$(cat "$RUN_ROOT/meta/flow2-prefinality-mempool-txid.txt")"
    deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  else
    log "requesting user deposit address"
    request_deposit "$RUN_ROOT/node1" user "flow2-prefinality-$(date +%s)"
    session="$(deposit_session "$user_addr")"
    printf '%s\n' "$session" >"$RUN_ROOT/meta/flow2-prefinality-session-before.json"
    deposit_address="$(jq -r '.deposit_address' <<<"$session")"
    path_index="$(jq -r '.deposit_path_index' <<<"$session")"

    log "broadcasting BTC deposit without mining"
    txid="$(send_unconfirmed_deposit "$deposit_address" "0.20000000")"
    deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
    printf '%s\n' "$txid" >"$RUN_ROOT/meta/flow2-prefinality-mempool-txid.txt"
    mempool_count="$(btc_cli getrawmempool | jq --arg txid "$txid" '[.[] | select(. == $txid)] | length')"
    [[ "$mempool_count" == "1" ]] || die "unconfirmed deposit was not in BTC mempool"
    wait_blocks 5

    pre_query="$(curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" 2>/dev/null || true)"
    if [[ -n "$pre_query" ]]; then
      printf '%s\n' "$pre_query" >"$RUN_ROOT/meta/flow2-prefinality-deposit-before-mining.json"
      if jq -e '.status == "deposit_matched" or .status == "committed"' <<<"$pre_query" >/dev/null 2>&1; then
        die "unconfirmed BTC deposit was matched before mining"
      fi
    else
      printf '{"found":false}\n' >"$RUN_ROOT/meta/flow2-prefinality-deposit-before-mining.json"
    fi
    deposit_session "$user_addr" >"$RUN_ROOT/meta/flow2-prefinality-session-before-mining.json"
  fi

  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$path_index" "$amount_sats" "flow2-prefinality-seed")"
  commitments="$("$SHIELDER_HELPER" commitments "$receipt")"
  out="$(thornado_tx "$RUN_ROOT/node1" user shielder split "$deposit_id" "$commitments")"
  assert_tx_rejected "$out" "flow2 prefinality split before mining"

  log "mining BTC deposit and checking it becomes matchable"
  mine_regtest_blocks 2
  matched="$(wait_deposit_matched "$deposit_id" 240)"
  printf '%s\n' "$matched" >"$RUN_ROOT/meta/flow2-prefinality-deposit-after-mining.json"

  out="$(thornado_tx "$RUN_ROOT/node1" user shielder split "$deposit_id" "$commitments")"
  assert_tx_success "$out" "flow2 prefinality split after mining"
  committed="$(wait_deposit_committed "$deposit_id")"
  printf '%s\n' "$committed" >"$RUN_ROOT/meta/flow2-prefinality-deposit-committed.json"
  jq -e '.status == "committed" and .settlement == "user" and ((.commitment_count | tonumber) > 0)' <<<"$committed" >/dev/null \
    || die "post-finality split did not commit user deposit"

  cat >"$RUN_ROOT/meta/flow2-prefinality-results.md" <<RESULTS
# Flow 2 Pre-Finality Results

- Unconfirmed BTC deposit stayed unmatched before mining.
- Split before BTC block inclusion was rejected.
- BTC deposit matched after mining.
- Split after BTC block inclusion committed successfully.
RESULTS
  log "RESULTS Flow 2 pre-finality checks: PASS"
}

main "$@"
