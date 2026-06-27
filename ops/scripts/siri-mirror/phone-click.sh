#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <x_frac> <y_frac>   # fractions within phone screen area" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/config.env"

xf="$1"
yf="$2"

coords="$(WINDOW_X="${WINDOW_X}" WINDOW_Y="${WINDOW_Y}" PHONE_X="${PHONE_X}" PHONE_Y="${PHONE_Y}" PHONE_W="${PHONE_W}" PHONE_H="${PHONE_H}" xf="${xf}" yf="${yf}" python3 - <<'PY'
import os
wx = int(os.environ["WINDOW_X"])
wy = int(os.environ["WINDOW_Y"])
px = int(os.environ["PHONE_X"])
py = int(os.environ["PHONE_Y"])
pw = int(os.environ["PHONE_W"])
ph = int(os.environ["PHONE_H"])
xf = float(os.environ["xf"])
yf = float(os.environ["yf"])
x = wx + px + int(pw * xf)
y = wy + py + int(ph * yf)
print(f"{x},{y}")
PY
)"

osascript -e 'tell application "iPhone Mirroring" to activate' >/dev/null 2>&1 || true
sleep 0.15
/opt/homebrew/bin/cliclick "c:${coords}"
echo "clicked ${coords} (frac ${xf},${yf})"