#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-real4}"
CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_CONTAINER="${BTC_CONTAINER:-thornado-real4-bitcoind}"
BTC_RPC_PORT="${BTC_RPC_PORT:-18445}"

THORNADO="${BUILD_DIR}/thornado"
BIFROST="${BUILD_DIR}/bifrost"

log() {
  printf '[flow2-supp] %s\n' "$*"
}

die() {
  printf '[flow2-supp] ERROR: %s\n' "$*" >&2
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

request_deposit() {
  local home="$1" owner="$2" pow="$3" out
  shift 3
  out="$(thornado_tx "$home" "$owner" request-deposit "$pow" "$@")"
  assert_tx_success "$out" "flow2-supp request-deposit ${pow}"
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

mine_to_registered_deposit() {
  local address="$1" amount_btc="$2"
  local utxo in_txid in_vout utxo_amount change_address change_amount inputs outputs raw signed txid
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
  txid="$(btc_cli -rpcwallet=miner sendrawtransaction "$signed")"
  mine_regtest_blocks 2
  echo "$txid"
}

node_status() {
  local addr="$1"
  curl -fsS "http://127.0.0.1:1317/thornado/node/address/${addr}" | jq -r '(.node.status // .status) | ascii_downcase'
}

set_config_from_active_nodes() {
  local key="$1" value="$2" i status out voted=0
  for i in 1 2 3 4; do
    # shellcheck source=/dev/null
    source "$RUN_ROOT/meta/node${i}.env"
    status="$(node_status "$address")"
    [[ "$status" == "active" ]] || continue
    out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" config "$key" "$value")"
    assert_tx_success "$out" "flow2-supp set ${key} node${i}"
    voted=$((voted + 1))
  done
  (( voted >= 1 )) || die "no active node voted for ${key}"
  wait_blocks 2
}

config_value() {
  local key="$1"
  curl -fsS "http://127.0.0.1:1317/thornado/config" | jq -r --arg key "$key" '.[$key].value'
}

restart_bifrost1_and_verify() {
  log "checking Bifrost scanner DB restart persistence"
  local before_hash after_hash pid bhome home bootstrap start_block start
  before_hash="$(jq -r '.txout.tx_array[] | select(.tx_type == "sweep") | .out_hash' "$RUN_ROOT/meta/flow2-sweep-txout.json" | head -n1)"
  curl -fsS "http://127.0.0.1:1317/thornado/deposit/$(jq -r '.deposit_id' "$RUN_ROOT/meta/flow2-deposit.json")" >"$RUN_ROOT/meta/flow2-restart-deposit-before.json"
  pid="$(cat "$RUN_ROOT/pids/bifrost-1.pid")"
  kill "$pid" >/dev/null 2>&1 || true
  start="$(date +%s)"
  while kill -0 "$pid" >/dev/null 2>&1; do
    if (( "$(date +%s)" - start >= 30 )); then
      kill -9 "$pid" >/dev/null 2>&1 || true
      break
    fi
    sleep 1
  done
  while kill -0 "$pid" >/dev/null 2>&1; do
    [[ "$(ps -p "$pid" -o stat= 2>/dev/null || true)" == *Z* ]] && break
    sleep 1
  done
  bhome="$RUN_ROOT/bifrost1"
  home="$RUN_ROOT/node1"
  bootstrap=""
  start_block="$(curl -fsS http://127.0.0.1:26657/status | jq -r '.result.sync_info.latest_block_height')"
  SIGNER_NAME="validator1" \
  SIGNER_PASSWD="$PASS" \
  BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
  BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:1317" \
  BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:26657" \
  BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:50051" \
  BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
  BIFROST_THORNADO_SIGNER_NAME="validator1" \
  THOR_BLOCK_TIME="100ms" \
  BLOCK_SCANNER_BACKOFF="100ms" \
  CHAIN_ID="$CHAIN_ID" \
  CHAIN_API="127.0.0.1:1317" \
  CHAIN_RPC="127.0.0.1:26657" \
  BIFROST_METRICS_LISTEN_PORT="9001" \
  BIFROST_TSS_P2P_PORT="5041" \
  BIFROST_TSS_INFO_ADDRESS="127.0.0.1:6041" \
  BIFROST_TSS_BOOTSTRAP_PEERS="$bootstrap" \
  BIFROST_TSS_EXTERNAL_IP="127.0.0.1" \
  BIFROST_TSS_ALLOW_ZERO_BOND_NODES="true" \
  BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
  BIFROST_SIGNER_KEYGEN_TIMEOUT="5s" \
  BIFROST_SIGNER_KEYSIGN_TIMEOUT="5s" \
  BIFROST_SIGNER_PARTY_TIMEOUT="5s" \
  BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5s" \
  BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_block" \
  BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
  BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="10" \
  BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
  BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
  BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
  BIFROST_CHAINS_BTC_USERNAME="thornado" \
  BIFROST_CHAINS_BTC_PASSWORD="thornado" \
  BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost1" \
  BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
  BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
  BTC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost" \
  BTC_START_BLOCK_HEIGHT="0" \
  "$BIFROST" --log-level debug >"$RUN_ROOT/logs/bifrost-1-restart.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-1.pid"
  start="$(date +%s)"
  while true; do
    if curl -fsS "http://127.0.0.1:6041/ping" >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$(cat "$RUN_ROOT/pids/bifrost-1.pid")" >/dev/null 2>&1; then
      tail -n 80 "$RUN_ROOT/logs/bifrost-1-restart.log" >&2 || true
      die "bifrost-1 exited during restart"
    fi
    (( "$(date +%s)" - start < 120 )) || die "bifrost-1 health did not recover"
    sleep 1
  done
  mine_regtest_blocks 2
  wait_blocks 2
  after_hash="$(jq -r '.txout.tx_array[] | select(.tx_type == "sweep") | .out_hash' "$RUN_ROOT/meta/flow2-sweep-txout.json" | head -n1)"
  [[ "$before_hash" == "$after_hash" && -n "$after_hash" ]] || die "flow2 sweep out_hash changed across restart"
  curl -fsS "http://127.0.0.1:1317/thornado/deposit/$(jq -r '.deposit_id' "$RUN_ROOT/meta/flow2-deposit.json")" >"$RUN_ROOT/meta/flow2-restart-deposit-after.json"
  jq -e '.status == "committed"' "$RUN_ROOT/meta/flow2-restart-deposit-after.json" >/dev/null || die "flow2 deposit changed after restart"
}

find_any_sweep_txout() {
  local in_hash="$1" now from h txout
  now="$(curl -fsS http://127.0.0.1:26657/status | jq -r '.result.sync_info.latest_block_height')"
  from=$((now - 180))
  (( from < 1 )) && from=1
  for ((h=from; h<=now; h++)); do
    txout="$(curl -fsS "http://127.0.0.1:1317/thornado/keysign/${h}" 2>/dev/null | jq -c '{height:'"$h"', txout:.keysign}' 2>/dev/null || true)"
    if [[ -n "$txout" ]] && jq -e --arg in_hash "$in_hash" '.txout.tx_array[]? | select(.tx_type == "sweep" and .in_hash == $in_hash)' <<<"$txout" >/dev/null 2>&1; then
      printf '%s\n' "$txout"
      return 0
    fi
  done
  return 1
}

validate_expired_session() {
  log "checking expired bond session"
  set_config_from_active_nodes Deposit_SessionExpiryBlocks 1
  [[ "$(config_value DEPOSIT_SESSIONEXPIRYBLOCKS)" == "1" ]] || die "Deposit_SessionExpiryBlocks did not become 1"
  # shellcheck source=/dev/null
  source "$RUN_ROOT/meta/node6.env"
  local node6_addr="$address" node6_secp="$secp" node6_cons="$cons"
  request_deposit "$RUN_ROOT/node6" validator6 flow2-expired-bond --operator-pubkey "$node6_secp" --node-pubkey "$node6_cons"
  local session deposit_address txid deposit_id start res
  session="$(deposit_session "$node6_addr")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/flow2-expired-session-before.json"
  deposit_address="$(jq -r '.deposit_address' <<<"$session")"
  wait_blocks 3
  txid="$(mine_to_registered_deposit "$deposit_address" "2.00000000")"
  deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  printf '%s\n' "$deposit_id" >"$RUN_ROOT/meta/flow2-expired-deposit-id.txt"
  start="$(date +%s)"
  while (( "$(date +%s)" - start < 75 )); do
    res="$(curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" 2>/dev/null || true)"
    if [[ -n "$res" ]]; then
      printf '%s\n' "$res" >"$RUN_ROOT/meta/flow2-expired-deposit-query.json"
      if jq -e '.status == "deposit_matched" or .status == "committed"' <<<"$res" >/dev/null 2>&1; then
        die "expired deposit unexpectedly matched"
      fi
    fi
    mine_regtest_blocks 1
    wait_blocks 1
    sleep 2
  done
  deposit_session "$node6_addr" >"$RUN_ROOT/meta/flow2-expired-session-after.json"
  jq -e '.status == "address_issued" and ((.deposit_id // "") == "")' "$RUN_ROOT/meta/flow2-expired-session-after.json" >/dev/null \
    || die "expired session changed to matched"
  if find_any_sweep_txout "$deposit_id" >"$RUN_ROOT/meta/flow2-expired-sweep-search.json"; then
    die "expired deposit queued a sweep txout"
  fi
  printf '{"found":false}\n' >"$RUN_ROOT/meta/flow2-expired-sweep-search.json"
  set_config_from_active_nodes Deposit_SessionExpiryBlocks 0
  [[ "$(config_value DEPOSIT_SESSIONEXPIRYBLOCKS)" == "0" ]] || die "Deposit_SessionExpiryBlocks did not reset to 0"
}

validate_stale_raw_observation() {
  log "checking stale sweep observation cannot mutate completed txout"
  local out_hash before tx from to amount gas pk height fake_id raw out tx_json i after
  out_hash="$(jq -r '.txout.tx_array[] | select(.tx_type == "sweep") | .out_hash' "$RUN_ROOT/meta/flow2-sweep-txout.json" | head -n1)"
  before="$out_hash"
  tx="$(curl -fsS "http://127.0.0.1:1317/thornado/tx/${out_hash}")"
  from="$(jq -r '.observed_tx.tx.from_address' <<<"$tx")"
  to="$(jq -r '.observed_tx.tx.to_address' <<<"$tx")"
  amount="$(jq -r '.observed_tx.tx.coins[] | select(.asset == "BTC.BTC") | .amount' <<<"$tx" | head -n1)"
  gas="$(jq -r '.observed_tx.tx.gas[] | select(.asset == "BTC.BTC") | .amount' <<<"$tx" | head -n1)"
  pk="$(jq -r '.observed_tx.observed_pub_key' <<<"$tx")"
  height="$(jq -r '.observed_tx.external_observed_height' <<<"$tx")"
  fake_id="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
  raw="$(jq -nc \
    --arg id "$fake_id" \
    --arg from "$from" \
    --arg to "$to" \
    --arg amount "$amount" \
    --arg gas "$gas" \
    --arg pk "$pk" \
    --argjson height "$height" \
    '[{tx:{id:$id,chain:"BTC",from_address:$from,to_address:$to,coins:[{asset:"BTC.BTC",amount:$amount}],gas:[{asset:"BTC.BTC",amount:$gas}],memo:""},block_height:$height,observed_pub_key:$pk,finalise_height:$height}]')"
  printf '%s\n' "$raw" >"$RUN_ROOT/meta/flow2-stale-raw-observation.json"
  for i in 1 2 3 4; do
    out="$(thornado_tx "$RUN_ROOT/node${i}" "validator${i}" observe-tx-outs --raw-observations "$raw")"
    printf '%s\n' "$out" >"$RUN_ROOT/meta/flow2-stale-observe-node${i}.out"
    tx_json="$(printf '%s\n' "$out" | tail -n 1)"
    assert_tx_success "$tx_json" "flow2-supp stale observe node${i}"
  done
  wait_blocks 2
  after="$(jq -r '.txout.tx_array[] | select(.tx_type == "sweep") | .out_hash' "$RUN_ROOT/meta/flow2-sweep-txout.json" | head -n1)"
  [[ "$before" == "$after" && "$after" != "$fake_id" ]] || die "stale observation mutated completed txout"
  curl -fsS "http://127.0.0.1:1317/thornado/tx/${fake_id}" >"$RUN_ROOT/meta/flow2-stale-fake-tx-query.json" 2>/dev/null || true
}

main() {
  [[ -d "$RUN_ROOT/meta" ]] || die "missing run meta dir: $RUN_ROOT/meta"
  curl -fsS http://127.0.0.1:26657/status >/dev/null || die "thornado rpc is not live"
  restart_bifrost1_and_verify
  validate_stale_raw_observation
  validate_expired_session
  cat >"$RUN_ROOT/meta/flow2-supplemental-results.md" <<EOF
# Flow 2 Supplemental Results

- Bifrost scanner DB restart preserved Flow 2 committed deposit and sweep out_hash.
- Stale/manual sweep observation did not mutate completed txout.
- Expired bond session did not become a matched/committed deposit and did not queue a sweep.
- Local regtest deployment uses Thornado KV and Bifrost local LevelDB only; no external DB/indexer is configured.
EOF
  log "RESULTS Flow 2 supplemental checks: PASS"
}

main "$@"
