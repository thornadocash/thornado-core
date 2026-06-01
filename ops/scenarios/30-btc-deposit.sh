#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${ROOT_DIR}/scripts/deploy-localnet.sh" "$@"
"${ROOT_DIR}/scripts/bootstrap-thornado.sh"
"${ROOT_DIR}/scripts/send-deposit.sh"

echo "BTC deposit scenario passed"
