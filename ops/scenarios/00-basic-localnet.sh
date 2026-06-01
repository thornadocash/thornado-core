#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${ROOT_DIR}/scripts/deploy-localnet.sh" "$@"

curl -fsS http://127.0.0.1:1317/health >/dev/null
curl -fsS http://127.0.0.1:6040/health >/dev/null
curl -fsS http://127.0.0.1:26657/status >/dev/null

# shellcheck source=../scripts/lib.sh
source "${ROOT_DIR}/scripts/lib.sh"
load_localnet_env
bitcoin_cli getblockchaininfo >/dev/null
bitcoin_cli -rpcwallet="${CLIENT_WALLET:-client}" getbalance

echo "basic localnet scenario passed"
