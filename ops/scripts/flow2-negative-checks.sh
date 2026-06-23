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

log() { printf '[flow2-neg] %s\n' "$*"; }
die() { printf '[flow2-neg] ERROR: %s\n' "$*" >&2; exit 1; }

key_show_addr() {
  local home="$1" name="$2"
  printf '%s\n' "$PASS" | "$THORNADO" keys show "$name" --home "$home" --keyring-backend file -a
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
    --home "$home" --from "$from_addr" --keyring-backend file --keyring-dir "$home" \
    --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas 2500000 --fees 0stake \
    --broadcast-mode sync --yes --output json
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
  local out="$1" label="$2" want="${3:-}" txhash start res code raw_log safe
  safe="${label// /-}"
  printf '%s\n' "$out" >"$RUN_ROOT/meta/${safe}-checktx.json"
  code="$(jq -r '.code // 0' <<<"$out" 2>/dev/null || echo "not-json")"
  if [[ "$code" != "0" ]]; then
    raw_log="$(jq -r '.raw_log // .log // empty' <<<"$out" 2>/dev/null || true)"
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
  local home="$1" owner="$2" pow="$3" out deposit_pubkey owner_addr difficulty pow_token
  shift 3
  deposit_pubkey="$1"
  owner_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  difficulty="$(curl -fsS "http://127.0.0.1:1317/thornado/config" | jq -r '.int_64_values.Deposit_PowDifficultyCurrent // .int_64_values.Deposit_PowDifficultyMin // 20')"
  pow_token="$("$SHIELDER_HELPER" pow-token "$owner_addr" "$difficulty" "$pow")"
  out="$(thornado_tx "$home" "$owner" request-deposit "$pow_token" "$@")"
  assert_tx_success "$out" "flow2-neg request-deposit"
  wait_blocks 2
}

deposit_pow_token() {
  local deposit_pubkey="$1" pow="$2" owner_addr difficulty
  owner_addr="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  difficulty="$(curl -fsS "http://127.0.0.1:1317/thornado/config" | jq -r '.int_64_values.Deposit_PowDifficultyCurrent // .int_64_values.Deposit_PowDifficultyMin // 20')"
  "$SHIELDER_HELPER" pow-token "$owner_addr" "$difficulty" "$pow"
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
    (( "$(date +%s)" - start < timeout )) || die "deposit ${deposit_id} did not match"
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

fund_note() {
  local home="$1" signer="$2" seed="$3" sats="$4" btc="$5" out_prefix="$6"
  local deposit_pubkey owner session addr txid deposit_id amount_sats receipt objects commitments sig out
  deposit_pubkey="$("$SHIELDER_HELPER" pubkey "${seed}-deposit-pubkey")"
  owner="$("$SHIELDER_HELPER" owner-address "$deposit_pubkey")"
  request_deposit "$home" "$signer" "${seed}-pow" "$deposit_pubkey" >/dev/null
  session="$(deposit_session "$owner")"
  printf '%s\n' "$session" >"$RUN_ROOT/meta/${out_prefix}-session.json"
  addr="$(jq -r '.deposit_address' <<<"$session")"
  txid="$(mine_to_registered_deposit "$addr" "$btc")"
  deposit_id="$(printf '%s' "$txid" | tr '[:lower:]' '[:upper:]')"
  wait_deposit_matched "$deposit_id" >"$RUN_ROOT/meta/${out_prefix}-matched.json"
  amount_sats="$(curl -fsS "http://127.0.0.1:1317/thornado/deposit/${deposit_id}" | jq -r '.amount_sats')"
  [[ "$amount_sats" == "$sats" ]] || die "${out_prefix} amount ${amount_sats}, want ${sats}"
  receipt="$("$SHIELDER_HELPER" receipt "$deposit_id" "$(jq -r '.deposit_path_index' <<<"$session")" "$amount_sats" "${seed}-note-seed")"
  printf '%s\n' "$receipt" >"$RUN_ROOT/meta/${out_prefix}-receipt.json"
  objects="$("$SHIELDER_HELPER" commitment-objects "$receipt")"
  commitments="$(jq -c 'map(tostring)' <<<"$objects")"
  sig="$("$SHIELDER_HELPER" shield-authorization "${seed}-deposit-pubkey" "$deposit_id" "$amount_sats" "$objects" | jq -r '.signature')"
  out="$(thornado_tx "$home" "$signer" shielder shield "$commitments" "$deposit_pubkey" "$sig" "$deposit_id")"
  assert_tx_success "$out" "${out_prefix} shield"
  wait_deposit_committed "$deposit_id" >"$RUN_ROOT/meta/${out_prefix}-committed.json"
}

bond_note() {
  local home="$1" signer="$2" receipt_file="$3" seed="$4" node_pubkey="$5" operator_pubkey="$6" prefix="$7"
  local note leaves withdrawal out
  note="$(jq -c '.notes[0]' "$receipt_file")"
  leaves="$(jq -nc --arg c "$(jq -r '.notes[0].commitment' "$receipt_file")" '[$c]')"
  withdrawal="$("$SHIELDER_HELPER" withdrawal-policy "$note" "${seed}-note-seed" "$leaves" "bond_escrow" 0 "bond_escrow" "$node_pubkey" "")"
  printf '%s\n' "$withdrawal" >"$RUN_ROOT/meta/${prefix}-withdrawal.json"
  "$SHIELDER_HELPER" shield-withdrawal "$withdrawal" "$RUN_ROOT/meta/${prefix}"
  out="$(thornado_tx "$home" "$signer" shielder bond-from-notes "$node_pubkey" "$operator_pubkey" "$RUN_ROOT/meta/${prefix}.proof.json" "$RUN_ROOT/meta/${prefix}.public.json")"
  printf '%s\n' "$out"
}

main() {
  [[ -d "$RUN_ROOT/meta" ]] || die "missing run meta dir: $RUN_ROOT/meta"
  curl -fsS http://127.0.0.1:26657/status >/dev/null || die "thornado rpc is not live"

  source "$RUN_ROOT/meta/node5.env"
  local node5_cons="$cons"
  source "$RUN_ROOT/meta/node1.env"
  local node1_addr="$address" node1_secp="$secp"
  source "$RUN_ROOT/meta/node6.env"
  local node6_addr="$address" node6_secp="$secp" node6_cons="$cons"

  log "checking POW replay rejection"
  local out
  local replay_pubkey replay_pow
  replay_pubkey="$("$SHIELDER_HELPER" pubkey "node5-bond-deposit-pubkey")"
  replay_pow="$(deposit_pow_token "$replay_pubkey" "bond-flow-2")"
  out="$(thornado_tx "$RUN_ROOT/node5" validator5 request-deposit "$replay_pow" "$replay_pubkey")"
  assert_tx_rejected "$out" "flow2 pow replay" "deposit pow token already used"

  log "checking set-node-keys before bond rejection"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node6_cons")"
  assert_tx_rejected "$out" "flow2 node6 keys before bond"

  log "checking below-minimum BTC deposit does not match"
  request_deposit "$RUN_ROOT/node1" user flow2-dust "$("$SHIELDER_HELPER" pubkey "flow2-dust")" >/dev/null
  local dust_owner dust_session dust_addr dust_txid
  dust_owner="$("$SHIELDER_HELPER" owner-address "$("$SHIELDER_HELPER" pubkey "flow2-dust")")"
  dust_session="$(deposit_session "$dust_owner")"
  printf '%s\n' "$dust_session" >"$RUN_ROOT/meta/flow2-dust-session.json"
  dust_addr="$(jq -r '.deposit_address' <<<"$dust_session")"
  dust_txid="$(mine_to_registered_deposit "$dust_addr" "0.00000545")"
  printf '%s\n' "$dust_txid" >"$RUN_ROOT/meta/flow2-dust-txid.txt"
  wait_blocks 8
  mine_regtest_blocks 8
  deposit_session "$dust_owner" >"$RUN_ROOT/meta/flow2-dust-session-after.json"
  jq -e '.status == "address_issued" and (.deposit_id // "") == ""' "$RUN_ROOT/meta/flow2-dust-session-after.json" >/dev/null || die "dust deposit unexpectedly matched"

  log "checking first node bonder must be the operator"
  fund_note "$RUN_ROOT/node1" validator1 flow2-node6-first-bonder-wrong 100000000 "1.00000000" flow2-node6-first-bonder-wrong
  out="$(bond_note "$RUN_ROOT/node1" validator1 "$RUN_ROOT/meta/flow2-node6-first-bonder-wrong-receipt.json" flow2-node6-first-bonder-wrong "$node6_cons" "$node6_secp" flow2-node6-first-bonder-wrong)"
  assert_tx_rejected "$out" "flow2 first bonder must be operator" "first node bonder must be the operator"

  log "checking underbond note accumulates pending bond"
  fund_note "$RUN_ROOT/node6" validator6 flow2-node6-underbond 100000000 "1.00000000" flow2-node6-underbond
  out="$(bond_note "$RUN_ROOT/node6" validator6 "$RUN_ROOT/meta/flow2-node6-underbond-receipt.json" flow2-node6-underbond "$node6_cons" "$node6_secp" flow2-node6-underbond)"
  assert_tx_success "$out" "flow2 node6 underbond note"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow2-node6-after-underbond-bond.json"
  jq -e '.pending_sats == 100000000 and .bond_sats == 0 and .fee_share_active == false' "$RUN_ROOT/meta/flow2-node6-after-underbond-bond.json" >/dev/null || die "node6 underbond did not stay pending"

  log "checking wrong node key, wrong policy, and activation top-up"
  fund_note "$RUN_ROOT/node6" validator6 flow2-node6-goodbond 100000000 "1.00000000" flow2-node6-goodbond
  local note leaves bad
  note="$(jq -c '.notes[0]' "$RUN_ROOT/meta/flow2-node6-goodbond-receipt.json")"
  leaves="$(jq -nc --arg c "$(jq -r '.notes[0].commitment' "$RUN_ROOT/meta/flow2-node6-goodbond-receipt.json")" '[$c]')"
  bad="$("$SHIELDER_HELPER" withdrawal-policy "$note" "flow2-node6-goodbond-note-seed" "$leaves" "bond_escrow" 0 "bond_escrow" "$node5_cons" "")"
  printf '%s\n' "$bad" >"$RUN_ROOT/meta/flow2-node6-wrong-node-withdrawal.json"
  "$SHIELDER_HELPER" shield-withdrawal "$bad" "$RUN_ROOT/meta/flow2-node6-wrong-node"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 shielder bond-from-notes "$node6_cons" "$node6_secp" "$RUN_ROOT/meta/flow2-node6-wrong-node.proof.json" "$RUN_ROOT/meta/flow2-node6-wrong-node.public.json")"
  assert_tx_rejected "$out" "flow2 wrong node bond note" "bond node pubkey mismatch"
  bad="$("$SHIELDER_HELPER" withdrawal-policy "$note" "flow2-node6-goodbond-note-seed" "$leaves" "bond_escrow" 0 "user_btc" "$node6_cons" "")"
  printf '%s\n' "$bad" >"$RUN_ROOT/meta/flow2-node6-wrong-policy-withdrawal.json"
  "$SHIELDER_HELPER" shield-withdrawal "$bad" "$RUN_ROOT/meta/flow2-node6-wrong-policy"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 shielder bond-from-notes "$node6_cons" "$node6_secp" "$RUN_ROOT/meta/flow2-node6-wrong-policy.proof.json" "$RUN_ROOT/meta/flow2-node6-wrong-policy.public.json")"
  assert_tx_rejected "$out" "flow2 wrong policy bond note" "bond notes require bond_escrow"
  out="$(bond_note "$RUN_ROOT/node6" validator6 "$RUN_ROOT/meta/flow2-node6-goodbond-receipt.json" flow2-node6-goodbond "$node6_cons" "$node6_secp" flow2-node6-goodbond)"
  assert_tx_success "$out" "flow2 node6 goodbond note"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 shielder bond-from-notes "$node6_cons" "$node6_secp" "$RUN_ROOT/meta/flow2-node6-goodbond.proof.json" "$RUN_ROOT/meta/flow2-node6-goodbond.public.json")"
  assert_tx_rejected "$out" "flow2 duplicate bond note" "shielder nullifier already spent"
  curl -fsS "http://127.0.0.1:1317/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow2-node6-after-activation-bond.json"
  jq -e '.pending_sats == 0 and .bond_sats == 200000000 and .fee_share_active == true' "$RUN_ROOT/meta/flow2-node6-after-activation-bond.json" >/dev/null || die "node6 activation bond accounting incorrect"

  log "checking multiple bonders cannot replace the registered operator"
  fund_note "$RUN_ROOT/node1" validator1 flow2-node6-wrong-operator 100000000 "1.00000000" flow2-node6-wrong-operator
  out="$(bond_note "$RUN_ROOT/node1" validator1 "$RUN_ROOT/meta/flow2-node6-wrong-operator-receipt.json" flow2-node6-wrong-operator "$node6_cons" "$node1_secp" flow2-node6-wrong-operator)"
  assert_tx_rejected "$out" "flow2 existing bond wrong operator" "bond operator mismatch"
  fund_note "$RUN_ROOT/node1" validator1 flow2-node6-topup 100000000 "1.00000000" flow2-node6-topup
  out="$(bond_note "$RUN_ROOT/node1" validator1 "$RUN_ROOT/meta/flow2-node6-topup-receipt.json" flow2-node6-topup "$node6_cons" "$node6_secp" flow2-node6-topup)"
  assert_tx_success "$out" "flow2 node6 different-bonder topup"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow2-node6-after-topup-bond.json"
  jq -e --arg op "$node6_secp" '.operator_pub_key == $op and .bond_sats == 300000000' "$RUN_ROOT/meta/flow2-node6-after-topup-bond.json" >/dev/null || die "node6 topup changed operator or wrong bond amount"
  curl -fsS "http://127.0.0.1:1317/thornado/node/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-topup-node.json"
  jq -e --arg op "$node6_addr" '.node_operator_address == $op and .node_address == $op' "$RUN_ROOT/meta/flow2-node6-after-topup-node.json" >/dev/null || die "node6 topup overwrote registered operator/node address"

  log "checking duplicate consensus key rejection"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node5_cons")"
  assert_tx_rejected "$out" "flow2 duplicate consensus key"
  curl -fsS "http://127.0.0.1:1317/thornado/node/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-duplicate-consensus.json"

  log "checking valid node6 keys still work after duplicate-key rejection"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-ip-address 127.0.0.1)"
  assert_tx_success "$out" "flow2 node6 set-ip-address"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 set-node-keys "$node6_secp" "$node6_cons")"
  assert_tx_success "$out" "flow2 node6 set-node-keys"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/node/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-final-node.json"

  log "checking operator rotation moves node control without moving node address"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 node rotate-operator "$node1_secp")"
  assert_tx_success "$out" "flow2 node6 rotate operator"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/bond/${node6_cons}" >"$RUN_ROOT/meta/flow2-node6-after-rotate-bond.json"
  jq -e --arg op "$node1_secp" --arg node "$node6_addr" '.operator_pub_key == $op and .node_address == $node and .bond_sats == 300000000' "$RUN_ROOT/meta/flow2-node6-after-rotate-bond.json" >/dev/null || die "node6 rotate did not update bond operator correctly"
  curl -fsS "http://127.0.0.1:1317/thornado/node/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-rotate-node.json"
  jq -e --arg op "$node1_addr" --arg node "$node6_addr" '.node_operator_address == $op and .node_address == $node' "$RUN_ROOT/meta/flow2-node6-after-rotate-node.json" >/dev/null || die "node6 rotate did not preserve node address with new operator"
  out="$(thornado_tx "$RUN_ROOT/node6" validator6 node maint "$node6_addr")"
  assert_tx_rejected "$out" "flow2 old operator maint after rotate" "not authorized"
  out="$(thornado_tx "$RUN_ROOT/node1" validator1 node maint "$node6_addr")"
  assert_tx_success "$out" "flow2 new operator maint after rotate"
  wait_blocks 2
  curl -fsS "http://127.0.0.1:1317/thornado/node/${node6_addr}" >"$RUN_ROOT/meta/flow2-node6-after-rotate-maint.json"
  jq -e '.maintenance == true' "$RUN_ROOT/meta/flow2-node6-after-rotate-maint.json" >/dev/null || die "new operator did not toggle node6 maintenance"

  curl -fsS "http://127.0.0.1:1317/thornado/nodes/metrics" >"$RUN_ROOT/meta/flow2-negative-node-metrics-after.json"
  curl -fsS "http://127.0.0.1:1317/thornado/shielder/sync?limit=2000" >"$RUN_ROOT/meta/flow2-negative-shielder-sync.json"

  cat >"$RUN_ROOT/meta/flow2-negative-results.md" <<EOF
# Flow 2 Negative Results

- POW replay rejected.
- Node6 set-node-keys before note bond rejected.
- 545-sat deposit stayed address_issued and did not match.
- Non-operator first bond attempt rejected.
- Node6 1 BTC note bond stayed pending below the 2 BTC requirement.
- Wrong node key, wrong policy, wrong existing operator, and duplicate nullifier bond attempts rejected.
- Node6 second 1 BTC note activated the node bond through MsgBondFromNotes.
- A different bonder topped up Node6 without replacing the registered operator.
- Duplicate consensus key rejected.
- Node6 valid key setup succeeded after rejection.
- Operator rotation moved Node6 control to the new operator, preserved the node address, rejected old-operator maintenance, and accepted new-operator maintenance.
EOF
  log "RESULTS Flow 2 negative checks: PASS"
}

main "$@"
