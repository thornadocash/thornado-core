#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

if [[ "${1:-}" == "--status" ]]; then
  run_hook_or_explain \
    FROST_DKG_STATUS_CMD \
    "FROST DKG status query" \
    "query DKG session status from signer sidecars and print vault pubkey when available"
  exit 0
fi

run_hook_or_explain \
  FROST_DKG_CMD \
  "FROST DKG" \
  "start DKG across signer sidecars, wait for completion, and persist/report vault pubkey"
