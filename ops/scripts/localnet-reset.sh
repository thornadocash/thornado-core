#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

REMOVE_LOGS=0
if [[ "${1:-}" == "--logs" ]]; then
  REMOVE_LOGS=1
fi

compose_cmd down -v --remove-orphans

if [[ "${REMOVE_LOGS}" == "1" ]]; then
  rm -rf "${OPS_DIR}/logs"
fi

echo "Localnet reset complete."
