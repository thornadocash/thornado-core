#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-$$}"
RUN_ROOT="${RUN_ROOT:-/tmp/thornado-distributed-${RUN_ID}}"
INVENTORY="${INVENTORY:-$ROOT_DIR/ops/distributed-regtest.env}"
NODE="${NODE:-}"
export NO_CLEANUP_TRAP=1
export KEEP_ARTIFACTS="${KEEP_ARTIFACTS:-1}"
export KEEP_RUNNING="${KEEP_RUNNING:-1}"
export BTC_USE_LOCAL="${BTC_USE_LOCAL:-1}"
export BTC_EXTERNAL="${BTC_EXTERNAL:-0}"
export BTC_AUTO_MINE="${BTC_AUTO_MINE:-1}"
export BTC_AUTO_MINE_INTERVAL="${BTC_AUTO_MINE_INTERVAL:-20}"
export BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
export BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
export CONTROLLER_BTC_DATADIR="${CONTROLLER_BTC_DATADIR:-/var/lib/thornado-regtest-bitcoind}"
export API_BIND_HOST="${API_BIND_HOST:-0.0.0.0}"
export P2P_BIND_HOST="${P2P_BIND_HOST:-0.0.0.0}"
export API_BASE="${API_BASE:-2370}"
export GRPC_BASE="${GRPC_BASE:-13380}"
export RPC_BASE="${RPC_BASE:-33360}"
export P2P_BASE="${P2P_BASE:-33380}"
export EBIFROST_BASE="${EBIFROST_BASE:-58600}"
export FROST_P2P_BASE="${FROST_P2P_BASE:-9340}"
export FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
export METRICS_BASE="${METRICS_BASE:-14200}"
export THORNADO_BLOCK_TIME_SECONDS="${THORNADO_BLOCK_TIME_SECONDS:-6}"
export GENESIS_NODE_BOND_START_AMOUNT_SATS="${GENESIS_NODE_BOND_START_AMOUNT_SATS:-100000000}"
export GENESIS_CHURN_INTERVAL_MINUTES="${GENESIS_CHURN_INTERVAL_MINUTES:-10}"
export GENESIS_CHURN_RETRY_INTERVAL_MINUTES="${GENESIS_CHURN_RETRY_INTERVAL_MINUTES:-5}"
export GENESIS_HALT_CHURNING="${GENESIS_HALT_CHURNING:-2}"
export GENESIS_BTC_CONFIRMATIONS_MIN="${GENESIS_BTC_CONFIRMATIONS_MIN:-1}"
export GENESIS_BTC_CONF_MULTIPLIER_BASIS_POINTS="${GENESIS_BTC_CONF_MULTIPLIER_BASIS_POINTS:-10000}"

# shellcheck source=ops/scripts/real-4node-e2e.sh
source "$SCRIPT_DIR/real-4node-e2e.sh"

log_dist() {
  printf '[distributed] %s\n' "$*"
}

require_inventory() {
  [[ -f "$INVENTORY" ]] || {
    cat >&2 <<EOF
missing inventory: $INVENTORY
copy ops/distributed-regtest.env.example to $INVENTORY and fill host IPs
EOF
    exit 1
  }
  # shellcheck disable=SC1090
  source "$INVENTORY"
  PASS="${SIGNER_PASSWD:-$PASS}"
  CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
  BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
  BTC_P2P_PORT="${BTC_P2P_PORT:-24646}"
  API_BASE="${API_BASE:-2370}"
  GRPC_BASE="${GRPC_BASE:-13380}"
  RPC_BASE="${RPC_BASE:-33360}"
  P2P_BASE="${P2P_BASE:-33380}"
  EBIFROST_BASE="${EBIFROST_BASE:-58600}"
  FROST_P2P_BASE="${FROST_P2P_BASE:-9340}"
  FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
  METRICS_BASE="${METRICS_BASE:-14200}"
}

node_host() {
  local key="NODE${1}_HOST"
  printf '%s' "${!key:-}"
}

controller_host() {
  printf '%s' "${CONTROLLER_HOST:-$(node_host 1)}"
}

api_url() {
  local host
  host="$(node_host "$1")"
  if [[ -n "$host" ]]; then
    printf 'http://%s:%s\n' "$host" "$(api_port "$1")"
  else
    printf 'http://127.0.0.1:%s\n' "$(api_port "$1")"
  fi
}

rpc_url() {
  local host
  host="$(node_host "$1")"
  if [[ -n "$host" ]]; then
    printf 'http://%s:%s\n' "$host" "$(rpc_port "$1")"
  else
    printf 'http://127.0.0.1:%s\n' "$(rpc_port "$1")"
  fi
}

inventory_peers() {
  local peers=() i host id
  for i in 1 2 3 4; do
    host="$(node_host "$i")"
    [[ -n "$host" ]] || die "NODE${i}_HOST missing from inventory"
    id="$(node_id "$RUN_ROOT/node${i}")"
    peers+=("${id}@${host}:$(p2p_port "$i")")
  done
  (IFS=,; printf '%s\n' "${peers[*]}")
}

rewrite_peers_from_inventory() {
  require_inventory
  mkdir -p "$RUN_ROOT/meta"
  inventory_peers >"$RUN_ROOT/meta/peers"
}

btc_cli_controller() {
  local cli="${BITCOIN_CLI:-bitcoin-cli}"
  if [[ -x /opt/bitcoin/bin/bitcoin-cli ]]; then
    cli="/opt/bitcoin/bin/bitcoin-cli"
  fi
  "$cli" -regtest -rpcconnect="${BTC_RPC_HOST:-127.0.0.1}" -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado "$@"
}

ensure_controller_wallets() {
  local i
  btc_cli_controller loadwallet miner >/dev/null 2>&1 || true
  btc_cli_controller createwallet miner >/dev/null 2>&1 || btc_cli_controller loadwallet miner >/dev/null 2>&1 || true
  for i in 1 2 3 4 5 6 7 8 9; do
    btc_cli_controller loadwallet "bifrost${i}" >/dev/null 2>&1 || true
    btc_cli_controller createwallet "bifrost${i}" true true "" false true >/dev/null 2>&1 || btc_cli_controller loadwallet "bifrost${i}" >/dev/null 2>&1 || true
  done
}

ensure_worker_wallets() {
  local i
  bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado loadwallet miner >/dev/null 2>&1 || true
  bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado createwallet miner >/dev/null 2>&1 ||
    bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado loadwallet miner >/dev/null 2>&1 || true
  for i in 1 2 3 4 5 6 7 8 9; do
    bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado loadwallet "bifrost${i}" >/dev/null 2>&1 || true
    bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado createwallet "bifrost${i}" true true "" false true >/dev/null 2>&1 ||
      bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado loadwallet "bifrost${i}" >/dev/null 2>&1 || true
  done
}

start_controller_bitcoind_fixed() {
  local btc_home="$CONTROLLER_BTC_DATADIR" addr height missing
  mkdir -p "$btc_home" "$RUN_ROOT/logs" "$RUN_ROOT/pids"
  if btc_cli_controller getblockchaininfo >/dev/null 2>&1; then
    log_dist "controller bitcoind already reachable on rpc=${BTC_RPC_PORT}"
    ensure_controller_wallets
  else
  log_dist "starting controller regtest bitcoind rpc=${BTC_RPC_PORT} p2p=${BTC_P2P_PORT}"
  nohup bitcoind \
    -datadir="$btc_home" -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
    -deprecatedrpc=create_bdb \
    -rpcbind=0.0.0.0 -rpcallowip=0.0.0.0/0 \
    -rpcport="$BTC_RPC_PORT" -port="$BTC_P2P_PORT" \
    -rpcuser=thornado -rpcpassword=thornado \
    >"$RUN_ROOT/logs/bitcoind.log" 2>&1 </dev/null &
  echo "$!" >"$RUN_ROOT/pids/bitcoind.pid"
  for _ in {1..60}; do
    if btc_cli_controller getblockchaininfo >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  ensure_controller_wallets
  fi
  height="$(btc_cli_controller getblockcount 2>/dev/null || printf '0')"
  if (( height < 101 )); then
    missing=$((101 - height))
  addr="$(btc_cli_controller -rpcwallet=miner getnewaddress)"
    btc_cli_controller -rpcwallet=miner generatetoaddress "$missing" "$addr" >/dev/null
  fi
}

assert_genesis_runtime_config() {
  local file="$RUN_ROOT/meta/genesis-runtime-config.json"
  wait_api_json_file "$(api_url 1)/thornado/config" "$file" "genesis runtime config" 120
  jq -e '
    (.HALT_CHURNING.value | tonumber) >= 2 and
    (.NODE_BONDSTARTAMOUNTSATS.value | tostring) == "100000000" and
    (.NODE_SETDESIRED.value | tostring) == "4" and
    (.BTC_CONFIRMATIONSMIN.value | tostring) == "1" and
    (.BTC_CONFMULTIPLIERBASISPOINTS.value | tostring) == "10000" and
    (.CHURN_INTERVALMINUTES.value | tostring) == "10" and
    (.CHURN_RETRYINTERVALMINUTES.value | tostring) == "5"
  ' "$file" >/dev/null || die "genesis runtime config validation failed"
}

start_worker_bitcoind() {
  local host controller
  require_inventory
  host="$(node_host "$NODE")"
  controller="$(controller_host)"
  [[ -n "$host" && -n "$controller" ]] || die "worker NODE and inventory hosts are required"
  mkdir -p "$RUN_ROOT/bitcoind" "$RUN_ROOT/logs" "$RUN_ROOT/pids"
  if bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado getblockchaininfo >/dev/null 2>&1; then
    log_dist "worker node${NODE} bitcoind already reachable on rpc=${BTC_RPC_PORT}"
    ensure_worker_wallets
    return 0
  fi
  nohup bitcoind \
    -datadir="$RUN_ROOT/bitcoind" -regtest=1 -server=1 -txindex=1 -fallbackfee=0.0001 \
    -deprecatedrpc=create_bdb \
    -rpcbind=0.0.0.0 -rpcallowip=0.0.0.0/0 \
    -rpcport="$BTC_RPC_PORT" -port="$BTC_P2P_PORT" \
    -rpcuser=thornado -rpcpassword=thornado \
    -connect="${controller}:${BTC_P2P_PORT}" \
    >"$RUN_ROOT/logs/bitcoind-node${NODE}.log" 2>&1 </dev/null &
  echo "$!" >"$RUN_ROOT/pids/bitcoind-node${NODE}.pid"
  for _ in {1..60}; do
    if bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport="$BTC_RPC_PORT" -rpcuser=thornado -rpcpassword=thornado getblockchaininfo >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  ensure_worker_wallets
}

start_worker_thornado() {
  local home="$RUN_ROOT/node${NODE}" peers host
  require_inventory
  host="$(node_host "$NODE")"
  if curl -fsS "http://127.0.0.1:$(rpc_port "$NODE")/status" >/dev/null 2>&1; then
    log_dist "worker thornado-${NODE} already reachable"
    return 0
  fi
  if [[ -f "$RUN_ROOT/meta/peers" ]]; then
    peers="$(cat "$RUN_ROOT/meta/peers")"
  else
    peers="$(inventory_peers)"
  fi
  configure_node_runtime_ports "$NODE"
  SIGNER_NAME="validator${NODE}" SIGNER_PASSWD="$PASS" CHAIN_HOME_FOLDER="$home" EXTERNAL_IP="$host" \
    "$THORNADO" start \
      --home "$home" \
      --api.enable=true --api.address "tcp://0.0.0.0:$(api_port "$NODE")" \
      --grpc.enable=true --grpc.address "0.0.0.0:$(grpc_port "$NODE")" \
      --rpc.laddr "tcp://0.0.0.0:$(rpc_port "$NODE")" \
      --p2p.laddr "tcp://0.0.0.0:$(p2p_port "$NODE")" \
      --p2p.persistent_peers "$peers" --p2p.pex=false \
      --ebifrost.enable=true --ebifrost.address "0.0.0.0:$(ebifrost_port "$NODE")" \
      --minimum-gas-prices "0btc" --log_level info \
      >"$RUN_ROOT/logs/thornado-${NODE}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${NODE}.pid"
}

frost_bootstrap_from_inventory() {
  local peers=() i host peer
  for i in 1 2 3 4 5 6 7 8 9; do
    host="$(node_host "$i")"
    [[ -n "$host" ]] || continue
    peer="$(curl --connect-timeout 2 --max-time 5 -fsS "http://${host}:$(frost_info_port "$i")/p2pid" 2>/dev/null || true)"
    [[ -n "$peer" ]] && peers+=("/ip4/${host}/tcp/$(frost_p2p_port "$i")/p2p/${peer}")
  done
  (IFS=,; printf '%s\n' "${peers[*]}")
}

frost_bootstrap_from_nodes_api() {
  local query_node nodes_file
  query_node="${1:-1}"
  nodes_file="$RUN_ROOT/meta/frost-bootstrap-nodes-${query_node}.json"
  local peers=() i host secp peer_id
  mkdir -p "$RUN_ROOT/meta"
  curl --connect-timeout 2 --max-time 5 -fsS "$(api_url "$query_node")/thornado/nodes" >"$nodes_file" 2>/dev/null || return 1
  for i in 1 2 3 4 5 6 7 8 9; do
    host="$(node_host "$i")"
    secp="$(sed -n 's/^secp=//p' "$RUN_ROOT/meta/node${i}.env" 2>/dev/null || true)"
    [[ -n "$host" && -n "$secp" ]] || continue
    peer_id="$(jq -r --arg secp "$secp" '.[]? | select(.pub_key_set.secp256k1 == $secp) | .peer_id // empty' "$nodes_file" | head -n1)"
    [[ -n "$peer_id" && "$peer_id" != "null" ]] || continue
    peers+=("/ip4/${host}/tcp/$(frost_p2p_port "$i")/p2p/${peer_id}")
  done
  [[ "${#peers[@]}" -gt 0 ]] || return 1
  (IFS=,; printf '%s\n' "${peers[*]}")
}

write_frost_bootstrap_all() {
  local bootstrap existing="$RUN_ROOT/meta/bifrost-bootstrap-all"
  bootstrap="$(frost_bootstrap_from_nodes_api 1 || frost_bootstrap_from_inventory)"
  if [[ -n "$bootstrap" ]]; then
    printf '%s\n' "$bootstrap" >"$existing"
    log_dist "refreshed FROST bootstrap peers"
    return 0
  fi
  if [[ -s "$existing" ]]; then
    log_dist "keeping cached FROST bootstrap peers from ${existing}"
    return 0
  fi
  die "no FROST bootstrap peers found (live discovery failed and no cached bootstrap file)"
}

start_worker_bifrost() {
  local home="$RUN_ROOT/node${NODE}" bhome="$RUN_ROOT/bifrost${NODE}" host bootstrap controller start_block
  require_inventory
  host="$(node_host "$NODE")"
  controller="$(controller_host)"
  bootstrap="${FROST_BOOTSTRAP_PEERS:-}"
  if [[ -z "$bootstrap" ]]; then
    bootstrap="$(frost_bootstrap_from_nodes_api "$NODE" || true)"
  fi
  if [[ -z "$bootstrap" ]]; then
    bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap-all" 2>/dev/null || true)"
  fi
  if [[ -z "$bootstrap" ]]; then
    bootstrap="$(frost_bootstrap_from_inventory)"
  fi
  if [[ -z "$bootstrap" && "$NODE" != "1" ]]; then
    die "no FROST bootstrap peers found"
  fi
  if curl -fsS "http://127.0.0.1:$(frost_info_port "$NODE")/ping" >/dev/null 2>&1; then
    log_dist "worker bifrost-${NODE} already reachable"
    return 0
  fi
  start_block="$(curl --connect-timeout 2 --max-time 5 -fsS "http://${host}:$(rpc_port "$NODE")/status" | jq -r '.result.sync_info.latest_block_height')"
  start_block=$((start_block - 10))
  (( start_block < 1 )) && start_block=1
  mkdir -p "$bhome"
  SIGNER_NAME="validator${NODE}" SIGNER_PASSWD="$PASS" \
    BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
    BIFROST_THORNADO_CHAIN_HOST="${host}:$(api_port "$NODE")" \
    BIFROST_THORNADO_CHAIN_RPC="${host}:$(rpc_port "$NODE")" \
    BIFROST_THORNADO_CHAIN_EBIFROST="${host}:$(ebifrost_port "$NODE")" \
    BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
    BIFROST_THORNADO_SIGNER_NAME="validator${NODE}" \
    CHAIN_ID="$CHAIN_ID" CHAIN_API="${host}:$(api_port "$NODE")" CHAIN_RPC="${host}:$(rpc_port "$NODE")" \
    THOR_BLOCK_TIME="100ms" BLOCK_SCANNER_BACKOFF="100ms" \
    BIFROST_METRICS_LISTEN_PORT="$(metrics_port "$NODE")" \
    BIFROST_FROST_P2P_PORT="$(frost_p2p_port "$NODE")" \
    BIFROST_FROST_INFO_ADDRESS="0.0.0.0:$(frost_info_port "$NODE")" \
    BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
    BIFROST_FROST_EXTERNAL_IP="$host" BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
    EXTERNAL_IP="$host" \
    BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
    BIFROST_SIGNER_KEYGEN_TIMEOUT="45s" BIFROST_SIGNER_KEYSIGN_TIMEOUT="45s" \
    BIFROST_SIGNER_PARTY_TIMEOUT="45s" BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5m" \
    BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_block" \
    BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
    BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_PREFETCH_BLOCKS="${BTC_BLOCK_SCANNER_PREFETCH_BLOCKS:-16}" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
    BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
    BIFROST_CHAINS_BTC_USERNAME="thornado" BIFROST_CHAINS_BTC_PASSWORD="thornado" \
    BIFROST_CHAINS_BTC_RPC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost${NODE}" \
    BIFROST_CHAINS_BTC_CHAIN_ID="BTC" BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_CHAIN_ID="BTC" BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="0" \
    BTC_HOST="127.0.0.1:${BTC_RPC_PORT}/wallet/bifrost${NODE}" BTC_START_BLOCK_HEIGHT="0" \
    "$BIFROST" --log-level info >"$RUN_ROOT/logs/bifrost-${NODE}.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-${NODE}.pid"
}

export_worker_bundle() {
  local node="$1" out="$RUN_ROOT/meta/worker-node${node}.tgz"
  tar -C "$RUN_ROOT" -czf "$out" "node${node}" meta/peers meta/node{1,2,3,4,5,6,7,8,9}.env
  printf '%s\n' "$out"
}

bond_worker_nodes() {
  local node required label node_pubkey bond_file bonded
  require_inventory
  for node in ${WORKER_NODES:-5 6 7 8 9}; do
    label="distributed-node${node}"
    # Persistent reruns may see completed bonds from an earlier attempt.
    # Skip the note-spend path once the chain already has the final bond.
    source "$RUN_ROOT/meta/node${node}.env"
    node_pubkey="$cons"
    bond_file="$RUN_ROOT/meta/${label}-bond.json"
    bonded="false"
    if curl_json_quiet "$(api_url 1)/thornado/bond/${node_pubkey}" >"$bond_file"; then
      bonded="$(jq -r '((.bond_sats // "0") | tonumber) > 0 and ((.pending_sats // "0") | tonumber) == 0 and (.fee_share_active == true)' "$bond_file")"
    fi
    if [[ "$bonded" == "true" ]]; then
      log_dist "${label}: bond already complete, skipping note spend"
      NODE_IP_ADDRESS="$(node_host "$node")" register_extra_node "$node" "$label"
      continue
    fi
    required="$(required_bond_sats_for_node "$node" "$label")"
    bond_extra_node_from_notes "$node" "$required" "${label}-bond"
    NODE_IP_ADDRESS="$(node_host "$node")" register_extra_node "$node" "$label"
  done
  assert_genesis_runtime_config
}

status_summary() {
  mkdir -p "$RUN_ROOT/meta"
  curl_json_quiet "$(api_url 1)/thornado/nodes" >"$RUN_ROOT/meta/distributed-nodes.json" || true
  curl_json_quiet "$(api_url 1)/thornado/vaults/base" >"$RUN_ROOT/meta/distributed-vaults-base.json" || true
  curl_json_quiet "$(api_url 1)/thornado/config" >"$RUN_ROOT/meta/distributed-config.json" || true
  jq -n \
    --slurpfile nodes "$RUN_ROOT/meta/distributed-nodes.json" \
    --slurpfile vaults "$RUN_ROOT/meta/distributed-vaults-base.json" \
    --slurpfile config "$RUN_ROOT/meta/distributed-config.json" \
    '{nodes:$nodes[0], vaults:$vaults[0], config:$config[0]}'
}

usage() {
  cat <<EOF
usage: ops/scripts/distributed-regtest-cluster.sh ACTION

controller actions (run on controller host):
  build
  init-controller
  start-controller-genesis
  resume-controller
  validate-genesis-config
  export-worker-bundles
  bond-workers
  bond-worker-topup
  validate-flow3
  status

worker action (run on worker host):
  NODE=1 ops/scripts/distributed-regtest-cluster.sh start-worker-bitcoind
  NODE=1 ops/scripts/distributed-regtest-cluster.sh start-worker-thornado
  NODE=1 ops/scripts/distributed-regtest-cluster.sh start-worker

FROST keygen and keysign run over libp2p between vault members. Ensure worker
Bifrosts can reach genesis FROST bootstrap peers before churn involving workers.

env:
  RUN_ROOT=$RUN_ROOT
  INVENTORY=$INVENTORY
EOF
}

action="${1:-}"
case "$action" in
  build)
    build_binaries
    ;;
  start-controller-bitcoind)
    require_inventory
    mkdir -p "$RUN_ROOT"/{logs,pids,meta}
    start_controller_bitcoind_fixed
    ;;
  init-controller)
    require_inventory
    reset_all
    build_binaries
    if [[ "${BTC_EXTERNAL:-0}" != "1" ]]; then
      start_controller_bitcoind_fixed
    else
      ensure_controller_wallets
    fi
    init_genesis
    rewrite_peers_from_inventory
    ;;
  start-controller-genesis)
    rewrite_peers_from_inventory
    start_thornado_nodes
    start_btc_auto_miner
    start_bifrost_nodes
    validate_flow1
    log_dist "genesis ready; FROST vault keygen uses libp2p across all participating bifrosts"
    ;;
  resume-controller)
    require_inventory
    export PASS
    export FROST_BIND_HOST="${FROST_BIND_HOST:-0.0.0.0}"
    export FROST_EXTERNAL_IP="${FROST_EXTERNAL_IP:-$(controller_host)}"
    mkdir -p "$RUN_ROOT"/{logs,pids,meta}
    write_frost_bootstrap_all
    start_controller_bitcoind_fixed
    rewrite_peers_from_inventory
    start_thornado_nodes
    start_btc_auto_miner
    start_bifrost_nodes
    log_dist "controller resumed at ${RUN_ROOT}; FROST external=${FROST_EXTERNAL_IP}"
    ;;
  validate-genesis-config|post-genesis-config)
    assert_genesis_runtime_config
    ;;
  export-worker-bundles)
    rewrite_peers_from_inventory
    for node in ${WORKER_NODES:-5 6 7 8 9}; do
      export_worker_bundle "$node"
    done
    ;;
  start-worker)
    [[ "$NODE" =~ ^[1-9]$ ]] || die "set NODE to 1..9"
    start_worker_bitcoind
    start_worker_thornado
    wait_json "http://127.0.0.1:$(rpc_port "$NODE")/status" "worker thornado-${NODE}" 180
    start_worker_bifrost
    wait_bifrost_health "$NODE" 180
    ;;
  start-worker-bitcoind)
    [[ "$NODE" =~ ^[1-9]$ ]] || die "set NODE to 1..9"
    start_worker_bitcoind
    ;;
  start-worker-thornado)
    [[ "$NODE" =~ ^[1-9]$ ]] || die "set NODE to 1..9"
    start_worker_bitcoind
    start_worker_thornado
    wait_json "http://127.0.0.1:$(rpc_port "$NODE")/status" "worker thornado-${NODE}" 180
    ;;
  start-worker-bifrost)
    [[ "$NODE" =~ ^[1-9]$ ]] || die "set NODE to 1..9"
    start_worker_bifrost
    wait_bifrost_health "$NODE" 180
    ;;
  bond-workers)
    bond_worker_nodes
    ;;
  bond-worker-topup)
    require_inventory
    [[ "$NODE" =~ ^[1-9]$ ]] || die "set NODE to 1..9"
    [[ "${AMOUNT_SATS:-}" =~ ^[0-9]+$ ]] || die "set AMOUNT_SATS"
    bond_extra_node_from_notes "$NODE" "$AMOUNT_SATS" "${LABEL:-distributed-node${NODE}-topup}"
    NODE_IP_ADDRESS="$(node_host "$NODE")" register_extra_node "$NODE" "${LABEL:-distributed-node${NODE}-topup}"
    ;;
  validate-flow3)
    require_inventory
    export BTC_USE_LOCAL=1
    export BTC_RPC_HOST=127.0.0.1
    export BTC_RPC_PORT="$BTC_RPC_PORT"
    export THORNADO_TX_NODE="tcp://$(node_host 1):$(rpc_port 1)"
    export FLOW3_MAIN_ONLY="${FLOW3_MAIN_ONLY:-1}"
    validate_flow3
    ;;
  status)
    status_summary
    ;;
  ""|-h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
