#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

SKIP_PREFLIGHT=0
if [[ "${1:-}" == "--skip-preflight" ]]; then
  SKIP_PREFLIGHT=1
  shift
fi

echo "Starting Thornado localnet with profiles: ${COMPOSE_PROFILES}"
if [[ "${COMPOSE_PROFILES}" == *mock* ]]; then
  echo "Using ops mock-service override for unfinished Go/Rust services."
fi

if [[ "${SKIP_PREFLIGHT}" != "1" ]]; then
  if ! missing_build_contexts; then
    echo
    echo "The split fork directories are not present yet."
    echo "Use --skip-preflight only if you intentionally want Docker Compose to fail at build time."
    exit 1
  fi
fi

mkdir -p "${OPS_DIR}/logs"
compose_with_profiles up -d "$@"
