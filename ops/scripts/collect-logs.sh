#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=lib.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib.sh"

load_localnet_env

OUT_DIR="${1:-${OPS_DIR}/logs/collected}"

mkdir -p "${OUT_DIR}"

compose_cmd ps > "${OUT_DIR}/compose-ps.txt" || true
compose_cmd logs --no-color > "${OUT_DIR}/compose.log" || true
docker volume ls --format '{{.Name}}' | grep '^thornado-localnet' > "${OUT_DIR}/docker-volumes.txt" || true

echo "Collected localnet logs into ${OUT_DIR}"
