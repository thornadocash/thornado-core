#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

run_hook_or_explain \
  RUN_WITHDRAWAL_CMD \
  "privacy withdrawal" \
  "submit withdrawal proof, wait for outbound queue/sign/broadcast, and verify recipient Bitcoin balance"
