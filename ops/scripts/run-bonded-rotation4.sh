#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ROOT_DIR/.env"
  set +a
fi

export FLOW_MODE="${FLOW_MODE:-bonded_rotation4}"
export FLOW_LIMIT="${FLOW_LIMIT:-1}"

exec "$ROOT_DIR/ops/scripts/real-4node-e2e.sh" "$@"
