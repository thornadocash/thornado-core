#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 \"prompt text\"" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROMPT="$1"

"${ROOT}/detect-window.sh" >/dev/null
"${ROOT}/open-siri-search.sh" >/dev/null
"${ROOT}/ask-siri.sh" "${PROMPT}"
sleep 5
"${ROOT}/capture.sh"