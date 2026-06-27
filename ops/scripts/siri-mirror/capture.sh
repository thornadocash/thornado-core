#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/config.env"

mkdir -p "${RUN_ROOT}"

osascript -e 'tell application "iPhone Mirroring" to activate' >/dev/null 2>&1 || true
sleep 0.2

screencapture -x -R"${WINDOW_X},${WINDOW_Y},${WINDOW_W},${WINDOW_H}" "${CAPTURE_PATH}"
echo "${CAPTURE_PATH}"