#!/usr/bin/env bash
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

RUN_ROOT="${RUN_ROOT:?RUN_ROOT is required}"
REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
BUILD_DIR="${BUILD_DIR:-$REPO_ROOT/build}"
THORNADO="${THORNADO:-$BUILD_DIR/thornado}"
BIFROST="${BIFROST:-$BUILD_DIR/bifrost}"
BITCOIN_CLI="${BITCOIN_CLI:-$(command -v bitcoin-cli)}"

CHAIN_ID="${CHAIN_ID:-thornado-e2e}"
PASS="${SIGNER_PASSWD:-passphrase123}"
BTC_RPC_PORT="${BTC_RPC_PORT:-24645}"
BTC_RPC_HOST="${BTC_RPC_HOST:-127.0.0.1}"
BTC_START_BLOCK_HEIGHT="${BTC_START_BLOCK_HEIGHT:-0}"
API_BASE="${API_BASE:-2370}"
GRPC_BASE="${GRPC_BASE:-13380}"
RPC_BASE="${RPC_BASE:-33360}"
P2P_BASE="${P2P_BASE:-33380}"
EBIFROST_BASE="${EBIFROST_BASE:-58600}"
FROST_P2P_BASE="${FROST_P2P_BASE:-9340}"
FROST_INFO_BASE="${FROST_INFO_BASE:-10340}"
METRICS_BASE="${METRICS_BASE:-14200}"

api_port() { echo $((API_BASE + $1)); }
grpc_port() { echo $((GRPC_BASE + $1)); }
rpc_port() { echo $((RPC_BASE + $1)); }
p2p_port() { echo $((P2P_BASE + $1)); }
ebifrost_port() { echo $((EBIFROST_BASE + $1)); }
frost_p2p_port() { echo $((FROST_P2P_BASE + $1)); }
frost_info_port() { echo $((FROST_INFO_BASE + $1)); }
metrics_port() { echo $((METRICS_BASE + $1)); }

stop_all() {
  local pid_file pid cmd
  shopt -s nullglob
  for pid_file in "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid; do
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
      cmd="$(ps -p "$pid" -o command= 2>/dev/null || true)"
      case "$pid_file:$cmd" in
        *"/pids/btc-miner.pid:"*|*"/build/thornado start"*|*"/build/bifrost"*) kill "$pid" >/dev/null 2>&1 || true ;;
      esac
    fi
  done
  for pid_file in "$RUN_ROOT"/pids/thornado-*.pid "$RUN_ROOT"/pids/bifrost-*.pid "$RUN_ROOT"/pids/btc-miner.pid; do
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if [[ -n "$pid" ]]; then
      for _ in {1..20}; do
        kill -0 "$pid" >/dev/null 2>&1 || break
        sleep 0.25
      done
    fi
  done
  shopt -u nullglob
}

btc_miner_loop() {
  local addr
  printf '[run-real4] starting btc miner wallet=miner rpc=%s interval=20s at %s\n' "$BTC_RPC_PORT" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
  while true; do
    if addr="$("$BITCOIN_CLI" -regtest -rpcwallet=miner -rpcuser=thornado -rpcpassword=thornado -rpcport="$BTC_RPC_PORT" -datadir="$RUN_ROOT/bitcoind" getnewaddress 2>>"$RUN_ROOT/logs/btc-miner.log")"; then
      "$BITCOIN_CLI" -regtest -rpcwallet=miner -rpcuser=thornado -rpcpassword=thornado -rpcport="$BTC_RPC_PORT" -datadir="$RUN_ROOT/bitcoind" generatetoaddress 1 "$addr" >>"$RUN_ROOT/logs/btc-miner.log" 2>&1 || true
    fi
    sleep 20
  done
}

start_btc_miner() {
  btc_miner_loop &
  echo "$!" >"$RUN_ROOT/pids/btc-miner.pid"
}

start_thornado() {
  local i="$1" home="$RUN_ROOT/node${i}" peers
  peers="$(cat "$RUN_ROOT/meta/peers")"
  printf '[run-real4] starting thornado-%s rpc=%s api=%s at %s\n' "$i" "$(rpc_port "$i")" "$(api_port "$i")" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
  SIGNER_NAME="validator${i}" SIGNER_PASSWD="$PASS" CHAIN_HOME_FOLDER="$home" \
    "$THORNADO" start \
      --home "$home" \
      --api.enable=true --api.address "tcp://127.0.0.1:$(api_port "$i")" \
      --grpc.enable=true --grpc.address "127.0.0.1:$(grpc_port "$i")" \
      --rpc.laddr "tcp://127.0.0.1:$(rpc_port "$i")" \
      --p2p.laddr "tcp://127.0.0.1:$(p2p_port "$i")" \
      --p2p.persistent_peers "$peers" --p2p.pex=false \
      --ebifrost.enable=true --ebifrost.address "127.0.0.1:$(ebifrost_port "$i")" \
      --minimum-gas-prices "0stake" --log_level info \
      >>"$RUN_ROOT/logs/thornado-${i}-resume.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/thornado-${i}.pid"
}

start_bifrost() {
  local i="$1" home="$RUN_ROOT/node${i}" bhome="$RUN_ROOT/bifrost${i}" bootstrap start_block
  bootstrap="$(cat "$RUN_ROOT/meta/bifrost-bootstrap-all" 2>/dev/null || cat "$RUN_ROOT/meta/bifrost-bootstrap")"
  start_block=1
  if [[ "$i" == "5" ]]; then
    start_block="$(cat "$RUN_ROOT/meta/flow2-node5-churn-bifrost5-signer-start-height.txt" 2>/dev/null || echo 1)"
  fi
  mkdir -p "$bhome"
  printf '[run-real4] starting bifrost-%s info=%s p2p=%s at %s\n' "$i" "$(frost_info_port "$i")" "$(frost_p2p_port "$i")" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
  SIGNER_NAME="validator${i}" SIGNER_PASSWD="$PASS" \
    BIFROST_THORNADO_CHAIN_ID="$CHAIN_ID" \
    BIFROST_THORNADO_CHAIN_HOST="127.0.0.1:$(api_port "$i")" \
    BIFROST_THORNADO_CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
    BIFROST_THORNADO_CHAIN_EBIFROST="127.0.0.1:$(ebifrost_port "$i")" \
    BIFROST_THORNADO_CHAIN_HOME_FOLDER="$home" \
    BIFROST_THORNADO_SIGNER_NAME="validator${i}" \
    CHAIN_ID="$CHAIN_ID" \
    THOR_BLOCK_TIME="100ms" BLOCK_SCANNER_BACKOFF="100ms" \
    CHAIN_API="127.0.0.1:$(api_port "$i")" CHAIN_RPC="127.0.0.1:$(rpc_port "$i")" \
    BIFROST_METRICS_LISTEN_PORT="$(metrics_port "$i")" \
    BIFROST_FROST_P2P_PORT="$(frost_p2p_port "$i")" \
    BIFROST_FROST_INFO_ADDRESS="127.0.0.1:$(frost_info_port "$i")" \
    BIFROST_FROST_BOOTSTRAP_PEERS="$bootstrap" \
    BIFROST_FROST_EXTERNAL_IP="127.0.0.1" BIFROST_FROST_ALLOW_ZERO_BOND_NODES="true" \
    PEER="$bootstrap" EXTERNAL_IP="127.0.0.1" \
    BIFROST_SIGNER_SIGNER_DB_PATH="$bhome/signer_db" \
    BIFROST_SIGNER_KEYGEN_TIMEOUT="5s" BIFROST_SIGNER_KEYSIGN_TIMEOUT="5s" \
    BIFROST_SIGNER_PARTY_TIMEOUT="5s" BIFROST_SIGNER_PRE_PARAM_TIMEOUT="5s" \
    BIFROST_SIGNER_BLOCK_SCANNER_START_BLOCK_HEIGHT="$start_block" \
    BIFROST_SIGNER_BLOCK_SCANNER_BLOCK_HEIGHT_DISCOVER_BACK_OFF="100ms" \
    BIFROST_SIGNER_BLOCK_SCANNER_PREFETCH_BLOCKS="1" BIFROST_SIGNER_BACKUP_KEYSHARES="false" \
    BIFROST_FROST_SHARED_DEALER_DIR="$RUN_ROOT/frost-dealer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_DB_PATH="$bhome/btc_observer" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_MAX_HEALTHY_LAG="24h" \
    BIFROST_CHAINS_BTC_SCANNER_LEVELDB_DB_PATH="$bhome/btc_scanner" \
    BIFROST_CHAINS_BTC_USERNAME="thornado" BIFROST_CHAINS_BTC_PASSWORD="thornado" \
    BIFROST_CHAINS_BTC_RPC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost${i}" \
    BIFROST_CHAINS_BTC_CHAIN_ID="BTC" BIFROST_CHAINS_BTC_CHAIN_NETWORK="regtest" \
    BIFROST_CHAINS_BTC_BLOCK_SCANNER_CHAIN_ID="BTC" BIFROST_CHAINS_BTC_BLOCK_SCANNER_START_BLOCK_HEIGHT="$BTC_START_BLOCK_HEIGHT" \
    BTC_HOST="${BTC_RPC_HOST}:${BTC_RPC_PORT}/wallet/bifrost" BTC_START_BLOCK_HEIGHT="$BTC_START_BLOCK_HEIGHT" \
    "$BIFROST" --log-level debug >>"$RUN_ROOT/logs/bifrost-${i}-resume.log" 2>&1 &
  echo "$!" >"$RUN_ROOT/pids/bifrost-${i}.pid"
}

on_exit() {
  local status="$?"
  printf '[run-real4] exiting status=%s line=%s at %s\n' "$status" "${BASH_LINENO[0]:-unknown}" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
  stop_all
}

trap 'stop_all; exit 0' TERM INT HUP
trap on_exit EXIT
mkdir -p "$RUN_ROOT"/{logs,pids}
stop_all

for i in 1 2 3 4 5; do
  start_thornado "$i"
done
sleep 8
for i in 1 2 3 4 5; do
  start_bifrost "$i"
  sleep 2
done
start_btc_miner

while true; do
  for i in 1 2 3 4 5; do
    pid="$(cat "$RUN_ROOT/pids/thornado-${i}.pid" 2>/dev/null || true)"
    if [[ -z "$pid" ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
      printf '[run-real4] restarting thornado-%s at %s\n' "$i" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
      start_thornado "$i"
    fi
  done
  for i in 1 2 3 4 5; do
    pid="$(cat "$RUN_ROOT/pids/bifrost-${i}.pid" 2>/dev/null || true)"
    if [[ -z "$pid" ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
      printf '[run-real4] restarting bifrost-%s at %s\n' "$i" "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
      start_bifrost "$i"
    fi
  done
  pid="$(cat "$RUN_ROOT/pids/btc-miner.pid" 2>/dev/null || true)"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" >/dev/null 2>&1; then
    printf '[run-real4] restarting btc miner at %s\n' "$(date -Iseconds)" >>"$RUN_ROOT/logs/resume-runner.log"
    start_btc_miner
  fi
  sleep 5
done
