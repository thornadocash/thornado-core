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
  printf '[flow2-neg] %s\n' "$*"
}

die() {
  printf '[flow2-neg] ERROR: %s\n' "$*" >&2
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
  local out="$1" label="$2" txhash start res code raw_log
  jq -e '.code == null or .code == 0' <<<"$out" >/dev/null || die "$label failed CheckTx: $out"
  txhash="$(jq -r '.txhash // empty' <<<"$out")"
  [[ -n "$txhash" ]] || return 0
  start="$(date +%s)"
  while (( $(date +%s) - start < 60 )); do
    res="$(curl -fsS "http://127.0.0.1:26657/tx?hash=0x${txhash}" 2>/dev/null || true)"
    if [[ -n "$res" ]] && jq -e '.result.tx_result' <<<"$res" >/dev/null 2>&1; then
      code="$(jq -r '.result.tx_result.code // 0' <<<"$res")"
      if [[ "$code" == "0" ]]; then
        return 0
      fi
      printf '%s\n' "$out" >"$RUN_ROOT/meta/${label// /-}-checktx.json"
      printf '%s\n' "$res" >"$RUN_ROOT/meta/${label// /-}-delivertx.json"
      raw_log="$(jq -r '.result.tx_result.log // .result.tx_result.info // empty' <<<"$res")"
      die "$label failed DeliverTx code=$code log=$raw_log"
    fi
    sleep 1
  done
  die "$label tx $txhash was not found"
}

assert_tx_rejected() {
  local out="$1" label="$2" want="${3:-}" txhash start res code raw_log safe
  safe="${label// /-}"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}-checktx.json"
  code="$(jq -r '.code // 0' <<<"$out")"
  if [[ "$code" != "0" ]]; then
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out")"
    if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
      die "$label rejected with unexpected CheckTx log: $raw_log"
    fi
    printf '%s\n' "$raw_log" >"$RUN_ROOT/meta/${safe}-rejected.log"
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
        if [[ -n "$want" && "$raw_log" != *"$want"* ]]; then
          die "$label rejected with unexpected DeliverTx log: $raw_log"
        fi
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
  shift 3
  out="$(thornado_tx "$home" "$owner" request-deposit "$pow" "$@")"
  assert_tx_success "$out" "flow2-neg request-deposit"
  wait_blocks 2
  echo "$out"
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

main() {
  [[ -d "$RUN_ROOT/meta" ]] || die "missing run meta dir: $RUN_ROOT/meta"
  curl -fsS http://127.0.0.1:26657/status >/dev/null || die "thornado rpc is not live"

  # shellcheck source=/dev/null
  source "$RUN_ROOT/meta/node5.env"
  local node5_addr="$address" node5_secp="$secp" node5_cons="$cons"
  local flow2_deposit_id
  flow2_deposit_id="$(jq -r '.deposit_id' "$RUN_ROOT/meta/flow2-deposit.json")"

  log "checking POW replay rejection"
  local out
  out="$(thornado_tx "$RUN_ROOT/node5" validator5 request-deposit bond-flow-2 --operator-pubkey "$node5_secp" --node-pubkey "$node5_cons")"
  assert_tx_rejected "$out" "flow2 pow replay" "deposit pow token already used"

  log "checking wrong-owner split rejection"
  out="$(thornado_tx "$RUN_ROOT/node1" validator1 shielder split "$flow2_deposit_id" "$("$SHIELDER_HELPER" protocol-commitments 100000000)")"
  assert_tx_rejected "$out" "flow2 wrong owner split" "deposit owner mismatch"

  log "checking wrong deposit id rejection"
  out="$(thornado_tx "$RUN_ROOT/node5" validator5 shielder split AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA "$("$SHIELDER_HELPER" protocol-commitments 100000000)")"
  assert_tx_rejected "$out" "flow2 wrong deposit id" "deposit not found"

  log "checking duplicate split rejection"
  out="$(thornado_tx "$RUN_ROOT/node5" validator5 shielder split "$flow2_deposit_id" "$("$SHIELDER_HELPER" protocol-commitments 100000000)")"
  assert_tx_rejected "$out" "flow2 duplicate split" "deposit already split"

  log "checking set-node-keys before bond rejection"
  # shellcheck source=/dev/null
  source "$RUN_ROOT/meta/node6.env"
  local node6_addr="$address" node6_secp="$secp" node6_ed="$ed" node6_cons="$cons"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node6_ed" "$node6_cons")"
  assert_tx_rejected "$out" "flow2 node6 keys before bond"

  log "checking below-minimum BTC deposit does not match"
  request_deposit "$RUN_ROOT/node1" user flow2-dust >/dev/null
  local user_addr dust_session dust_addr dust_txid
  user_addr="$(key_show_addr "$RUN_ROOT/node1" user)"
  dust_session="$(deposit_session "$user_addr")"
  printf '%s\n' "$dust_session" >"$RUN_ROOT/meta/flow2-dust-session.json"
  dust_addr="$(jq -r '.deposit_address' <<<"$dust_session")"
  dust_txid="$(mine_to_registered_deposit "$dust_addr" "0.00000545")"
  printf '%s\n' "$dust_txid" >"$RUN_ROOT/meta/flow2-dust-txid.txt"
  wait_blocks 8
  mine_regtest_blocks 8
  deposit_session "$user_addr" >"$RUN_ROOT/meta/flow2-dust-session-after.json"
  jq -e '.status == "address_issued" and (.deposit_id // "") == ""' "$RUN_ROOT/meta/flow2-dust-session-after.json" >/dev/null \
    || die "dust deposit unexpectedly matched"

  log "checking below-required bond split rejection"
  request_deposit "$RUN_ROOT/node6" validator6 flow2-node6-underbond --operator-pubkey "$node6_secp" --node-pubkey "$node6_cons" >/dev/null
  local under_session under_addr under_txid under_id under_commitments
  under_session="$(deposit_session "$node6_addr")"
  printf '%s\n' "$under_session" >"$RUN_ROOT/meta/flow2-node6-underbond-session.json"
  under_addr="$(jq -r '.deposit_address' <<<"$under_session")"
  under_txid="$(mine_to_registered_deposit "$under_addr" "1.00000000")"
  under_id="$(printf '%s' "$under_txid" | tr '[:lower:]' '[:upper:]')"
  wait_deposit_matched "$under_id" >"$RUN_ROOT/meta/flow2-node6-underbond-deposit.json"
  under_commitments="$("$SHIELDER_HELPER" protocol-commitments 100000000)"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 shielder split "$under_id" "$under_commitments")"
  assert_tx_rejected "$out" "flow2 node6 underbond split" "node bond below required amount"
  curl -fsS "http://127.0.0.1:1317/thornado/node/address/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-underbond-node-query.json" 2>/dev/null || true
  curl -fsS "http://127.0.0.1:1317/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow2-node6-after-underbond-bond-query.json" 2>/dev/null || true

  log "checking sufficient node6 bond with arbitrary supplied commitment is made deterministic"
  request_deposit "$RUN_ROOT/node6" validator6 flow2-node6-goodbond --operator-pubkey "$node6_secp" --node-pubkey "$node6_cons" >/dev/null
  local good_session good_addr good_txid good_id good_commitments good_deposit expected_commitment commitment_key attacker_key kv_value
  good_session="$(deposit_session "$node6_addr")"
  printf '%s\n' "$good_session" >"$RUN_ROOT/meta/flow2-node6-goodbond-session.json"
  good_addr="$(jq -r '.deposit_address' <<<"$good_session")"
  good_txid="$(mine_to_registered_deposit "$good_addr" "2.00000000")"
  good_id="$(printf '%s' "$good_txid" | tr '[:lower:]' '[:upper:]')"
  wait_deposit_matched "$good_id" >"$RUN_ROOT/meta/flow2-node6-goodbond-matched.json"
  good_commitments="$(jq -nc '[({denomination_sats:200000000, commitment:"ATTACKER_SUPPLIED"} | tostring)]')"
  printf '%s\n' "$good_commitments" >"$RUN_ROOT/meta/flow2-node6-goodbond-supplied-commitments.json"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 shielder split "$good_id" "$good_commitments")"
  assert_tx_success "$out" "flow2 node6 goodbond split"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/deposit/${good_id}" >"$RUN_ROOT/meta/flow2-node6-goodbond-deposit.json"
  good_deposit="$RUN_ROOT/meta/flow2-node6-goodbond-deposit.json"
  expected_commitment="$("$SHIELDER_HELPER" protocol-bond-commitment \
    "$good_id" "$node6_secp" "$node6_cons" "$(jq -r '.node_slot' "$good_deposit")" "200000000" "$(jq -r '.vault_pub_key' "$good_deposit")")"
  printf '%s\n' "$expected_commitment" >"$RUN_ROOT/meta/flow2-node6-goodbond-expected-commitment.txt"
  commitment_key="$(printf 'shielder_commitment//%s' "$expected_commitment" | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
  curl -fsS "http://127.0.0.1:26657/abci_query?path=%22/store/thornado/key%22&data=0x${commitment_key}" >"$RUN_ROOT/meta/flow2-node6-goodbond-commitment-kv.json"
  kv_value="$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow2-node6-goodbond-commitment-kv.json" | base64 -d | jq -r '.')"
  [[ "$kv_value" == "$good_id" ]] || die "node6 deterministic commitment not found in KV"
  attacker_key="$(printf 'shielder_commitment//ATTACKER_SUPPLIED' | xxd -p -c 256 | tr '[:lower:]' '[:upper:]')"
  curl -fsS "http://127.0.0.1:26657/abci_query?path=%22/store/thornado/key%22&data=0x${attacker_key}" >"$RUN_ROOT/meta/flow2-node6-attacker-commitment-kv.json"
  [[ -z "$(jq -r '.result.response.value // ""' "$RUN_ROOT/meta/flow2-node6-attacker-commitment-kv.json")" ]] \
    || die "attacker-supplied commitment was stored"

  log "checking duplicate consensus key rejection"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node6_ed" "$node5_cons")"
  assert_tx_rejected "$out" "flow2 duplicate consensus key"
  curl -fsS "http://127.0.0.1:1317/thornado/node/address/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-duplicate-consensus.json"

  log "checking valid node6 keys still work after duplicate-key rejection"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-ip-address 127.0.0.1)"
  assert_tx_success "$out" "flow2 node6 set-ip-address"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node6_ed" "$node6_cons")"
  assert_tx_success "$out" "flow2 node6 set-node-keys"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/node/address/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-final-node.json"
  curl -fsS "http://127.0.0.1:1317/thornado/node/metrics" >"$RUN_ROOT/meta/flow2-negative-node-metrics-after.json"
  curl -fsS "http://127.0.0.1:1317/thornado/shielder/roots" >"$RUN_ROOT/meta/flow2-negative-shielder-roots.json"

  cat >"$RUN_ROOT/meta/flow2-negative-results.md" <<EOF
# Flow 2 Negative Results

- POW replay rejected.
- Wrong-owner split rejected.
- Wrong deposit id rejected.
- Duplicate split rejected.
- Node6 set-node-keys before bond rejected.
- 545-sat deposit stayed address_issued and did not match.
- Node6 1 BTC underbond split rejected against 2 BTC requirement.
- Node6 2 BTC bond split accepted and stored deterministic protocol commitment, ignoring supplied text.
- Duplicate consensus key rejected.
- Node6 valid key setup succeeded after rejection.
EOF
  log "RESULTS Flow 2 negative checks: PASS"
}

main "$@"
