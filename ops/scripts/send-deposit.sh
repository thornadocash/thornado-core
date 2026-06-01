#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

run_hook_or_explain \
  SEND_DEPOSIT_CMD \
  "privacy deposit" \
  "request a Shielder deposit address, send regtest BTC to that address, and wait for Bifrost observation"
