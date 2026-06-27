#!/usr/bin/env bash
set -euo pipefail

export RUN_ROOT="${RUN_ROOT:-/var/lib/thornado/real5}"
export CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
export BTC_RPC_HOST="thornado-real5-bitcoind-1"
export BTC_RPC_PORT="18443"
export BTC_USE_LOCAL=1
export CHURN_INTERVAL_MINUTES="${CHURN_INTERVAL_MINUTES:-10}"
export CHURN_RETRY_INTERVAL_MINUTES="${CHURN_RETRY_INTERVAL_MINUTES:-1}"
export THORNADO_BLOCK_TIME_SECONDS="${THORNADO_BLOCK_TIME_SECONDS:-6}"
export CHURN_ROUNDS="${CHURN_ROUNDS:-10}"
export THORNADO_TX_TIMEOUT="${THORNADO_TX_TIMEOUT:-90}"
export TX_INCLUSION_TIMEOUT="${TX_INCLUSION_TIMEOUT:-1200}"

cd /workspace
if [[ -f /workspace/ops/scripts/real-4node-e2e-manual.sh ]]; then
  source /workspace/ops/scripts/real-4node-e2e-manual.sh
else
  source /workspace/ops/scripts/real-4node-e2e.sh
fi
trap - EXIT
trap 'echo "[manual10] ERROR line=$LINENO cmd=$BASH_COMMAND" >&2' ERR

btc_cli() {
  local wallet="" args=()
  while (($#)); do
    case "$1" in
      -rpcwallet=*) wallet="${1#-rpcwallet=}"; shift ;;
      *) args+=("$1"); shift ;;
    esac
  done
  local method="${args[0]}"
  unset 'args[0]'
  local params="[]" a
  for a in "${args[@]}"; do
    if jq -e . >/dev/null 2>&1 <<<"$a"; then
      params="$(jq -c --argjson v "$a" '. + [$v]' <<<"$params")"
    else
      params="$(jq -c --arg v "$a" '. + [$v]' <<<"$params")"
    fi
  done
  local url="http://${BTC_RPC_HOST}:${BTC_RPC_PORT}/"
  [[ -n "$wallet" ]] && url="http://${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/${wallet}"
  local body res err
  body="$(jq -nc --arg m "$method" --argjson p "$params" '{jsonrpc:"1.0",id:"manual10",method:$m,params:$p}')"
  res="$(curl --connect-timeout 5 --max-time 60 -sS --user thornado:thornado --data-binary "$body" -H 'content-type: text/plain;' "$url")"
  err="$(jq -r '.error.message // empty' <<<"$res")"
  if [[ -n "$err" ]]; then
    echo "$err" >&2
    return 1
  fi
  jq -r '.result | if type == "string" then . else tojson end' <<<"$res"
}

snapshot_state() {
  local label="$1" dir="$RUN_ROOT/meta/manual10"
  mkdir -p "$dir"
  : >"$dir/${label}-all-node-queries.jsonl"
  local env node
  for env in "$RUN_ROOT"/meta/node*.env; do
    node="${env##*/node}"
    node="${node%.env}"
    source "$env"
    node_query "$address" >>"$dir/${label}-all-node-queries.jsonl" || true
  done
  jq -s 'map(.node // .)' "$dir/${label}-all-node-queries.jsonl" >"$dir/${label}-nodes.json"
  curl_json_quiet "$(api_url 1)/thornado/vaults/base" >"$dir/${label}-vaults.json"
  curl_json_quiet "$(api_url 1)/thornado/txout/all" >"$dir/${label}-txouts.json" || printf '[]\n' >"$dir/${label}-txouts.json"

  local height btc active standby bonded active_vault vault_amt retired_nonzero pending
  height="$(curl_json_quiet "$(rpc_url 1)/status" | jq -r '.result.sync_info.latest_block_height')"
  btc="$(btc_cli getblockchaininfo | jq -r '.blocks')"
  active="$(jq '[.[]? | select((.status|ascii_downcase)=="active")] | length' "$dir/${label}-nodes.json")"
  standby="$(jq '[.[]? | select((.status|ascii_downcase)=="standby" or (.status|ascii_downcase)=="whitelisted")] | length' "$dir/${label}-nodes.json")"
  bonded="$(jq '[.[]? | select(((.bond // .total_bond // "0") | tonumber) > 0)] | length' "$dir/${label}-nodes.json")"
  active_vault="$(jq -r '[.[] | select(.status=="ActiveVault")][0].pub_key' "$dir/${label}-vaults.json")"
  vault_amt="$(jq '([.[] | select(.status=="ActiveVault")][0].coins // []) | map(select(.asset=="BTC.BTC") | .amount | tonumber) | add // 0' "$dir/${label}-vaults.json")"
  retired_nonzero="$(jq '[.[] | select(.status!="ActiveVault") | select(((.coins//[])|map(.amount|tonumber)|add//0) > 0)] | length' "$dir/${label}-vaults.json")"
  pending="$(jq '[.[] | (.pending_tx_block_heights // [])[]?] | length' "$dir/${label}-vaults.json")"
  jq -n \
    --arg lbl "$label" \
    --arg height "$height" \
    --arg btc "$btc" \
    --arg active "$active" \
    --arg standby "$standby" \
    --arg bonded "$bonded" \
    --arg active_vault "$active_vault" \
    --arg vault_amt "$vault_amt" \
    --arg retired_nonzero "$retired_nonzero" \
    --arg pending "$pending" \
    '{"label":$lbl,height:($height|tonumber),btc_blocks:($btc|tonumber),active:($active|tonumber),standby_or_whitelisted:($standby|tonumber),bonded_nodes:($bonded|tonumber),active_vault:$active_vault,active_vault_btc_sats:($vault_amt|tonumber),retired_nonzero:($retired_nonzero|tonumber),pending_vault_heights:($pending|tonumber)}' |
    tee "$dir/${label}-summary.json"
}

node_status() {
  local node="$1"
  source "$RUN_ROOT/meta/node${node}.env"
  node_query "$address" | jq -r '(.node.status // .status) | ascii_downcase'
}

is_bonded() {
  local node="$1"
  source "$RUN_ROOT/meta/node${node}.env"
  curl -fsS "$(api_url 1)/thornado/bond/${cons}" 2>/dev/null |
    jq -e '(.bond_sats | tonumber) > 0 and .fee_share_active == true' >/dev/null
}

mkdir -p "$RUN_ROOT/meta/manual10"
exec > >(tee -a "$RUN_ROOT/logs/manual-10-churn.log") 2>&1

echo "[manual10] start $(date -u +%FT%TZ)"
set_config_from_active_nodes Halt_Churning 1
set_config_from_active_nodes HaltSigningBTC 0
set_config_from_active_nodes Halt_SolvencyCheck 0
set_config_from_active_nodes Node_SetDesired 4
set_config_from_active_nodes Vault_MigrationIntervalMinutes 1
set_config_from_active_nodes Chain_BlockTimeSeconds "$THORNADO_BLOCK_TIME_SECONDS"
set_config_from_active_nodes Churn_IntervalMinutes "$CHURN_INTERVAL_MINUTES"
set_config_from_active_nodes Churn_RetryIntervalMinutes "$CHURN_RETRY_INTERVAL_MINUTES"
snapshot_state initial

candidate=2
if ! is_bonded "$candidate"; then
  bond_label="manual10-node${candidate}-bond-$(date +%s)"
  required="$(required_bond_sats_for_node "$candidate" "$bond_label")"
  echo "[manual10] bonding initial standby node${candidate} required_sats=${required}"
  bond_extra_node_from_notes "$candidate" "$required" "$bond_label"
  snapshot_state "after-initial-bond-node${candidate}"
fi

for round in $(seq 1 "$CHURN_ROUNDS"); do
  echo "[manual10] round=${round} candidate=node${candidate} status=$(node_status "$candidate")"
  wait_all_signed_txouts_finalized "manual10-round${round}-pre" 1200
  churn_extra_node_with_migration "$candidate" "manual10-round${round}-node${candidate}" 4
  assert_live_nodes_app_hash_converged "manual10 round${round}"
  assert_no_stale_migrate_signer_retry "manual10 round${round}"
  removed_cons="$(head -n1 "$RUN_ROOT/meta/manual10-round${round}-node${candidate}-removed-cons.txt")"
  removed_node="$(node_index_by_cons "$removed_cons")"
  echo "[manual10] round=${round} removed=node${removed_node} cons=${removed_cons}"
  snapshot_state "round${round}-after-node${candidate}-in-node${removed_node}-out"
  candidate="$removed_node"
  sleep 5
done

snapshot_state final
write_run_summary PASS "manual ${CHURN_ROUNDS} churns completed"
echo "[manual10] PASS rounds=${CHURN_ROUNDS} $(date -u +%FT%TZ)"
