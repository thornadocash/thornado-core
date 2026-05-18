#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

run_hook_or_explain \
  THORNODE_BOOTSTRAP_CMD \
  "Thornode validator bootstrap" \
  "initialize validators, create node accounts, bond nodes, and prepare churn"
