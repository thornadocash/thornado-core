#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

trap '"${OPS_DIR}/scripts/collect-logs.sh" "${OPS_DIR}/logs/smoke/$(date +%Y%m%d-%H%M%S)" || true' ERR

"${OPS_DIR}/scripts/localnet-up.sh"
"${OPS_DIR}/scripts/wait-for-health.sh"
"${OPS_DIR}/scripts/bootstrap-regtest.sh"
"${OPS_DIR}/scripts/bootstrap-thornado.sh"
"${OPS_DIR}/scripts/send-deposit.sh"
"${OPS_DIR}/scripts/run-withdrawal.sh"
