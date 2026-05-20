#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

TIMEOUT="${LOCALNET_HEALTH_TIMEOUT_SECONDS:-180}"

wait_for_container_healthy bitcoind-regtest "${TIMEOUT}"
bitcoin_cli getblockchaininfo >/dev/null

if [[ -n "$(service_container_id thornode-1)" ]]; then
  wait_for_tcp 127.0.0.1 26657 thornode-1-cometbft "${TIMEOUT}"
fi

if [[ -n "$(service_container_id bifrost-1)" ]]; then
  wait_for_tcp 127.0.0.1 6040 bifrost-1 "${TIMEOUT}"
fi

echo "Localnet health checks passed."
