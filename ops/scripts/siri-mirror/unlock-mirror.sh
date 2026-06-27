#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Tap the Mac-login password field on the mirroring lock screen.
"${ROOT}/phone-click.sh" 0.50 0.72
echo "focus password field — enter Mac login password or use Touch ID on Mac"