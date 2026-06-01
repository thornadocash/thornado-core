#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${ROOT_DIR}/scripts/deploy-localnet.sh" --skip-regtest "$@"
"${ROOT_DIR}/scripts/run-frost-dkg.sh"
"${ROOT_DIR}/scripts/run-frost-dkg.sh" --status

echo "FROST DKG scenario passed"
