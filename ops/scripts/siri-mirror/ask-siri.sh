#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 \"prompt text\"" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/config.env"

PROMPT="$1"

"${ROOT}/phone-click.sh" "${SIRI_FIELD_X}" "${SIRI_FIELD_Y}" >/dev/null
sleep 0.25

# Select any existing text, then type the prompt.
/opt/homebrew/bin/cliclick kd:cmd t:a ku:cmd
sleep 0.1
/opt/homebrew/bin/cliclick t:"${PROMPT}"
sleep 0.15
/opt/homebrew/bin/cliclick kp:return

echo "sent prompt (${#PROMPT} chars)"