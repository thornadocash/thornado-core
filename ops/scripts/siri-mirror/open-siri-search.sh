#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Best-effort: tap near top-center (pull-down search / Siri entry on many iOS builds).
"${ROOT}/phone-click.sh" 0.50 0.08
sleep 0.4
"${ROOT}/phone-click.sh" 0.50 0.20
sleep 0.3

echo "tapped siri/search entry points"