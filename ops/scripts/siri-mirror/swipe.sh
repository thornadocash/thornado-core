#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 4 ]]; then
  echo "usage: $0 <x1> <y1> <x2> <y2>   # fractions within phone area" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${ROOT}/config.env"

coord() {
  local xf="$1"
  local yf="$2"
  WINDOW_X="${WINDOW_X}" WINDOW_Y="${WINDOW_Y}" PHONE_X="${PHONE_X}" PHONE_Y="${PHONE_Y}" PHONE_W="${PHONE_W}" PHONE_H="${PHONE_H}" xf="${xf}" yf="${yf}" python3 - <<'PY'
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
}

c1="$(coord "$1" "$2")"
c2="$(coord "$3" "$4")"

osascript -e 'tell application "iPhone Mirroring" to activate' >/dev/null 2>&1 || true
sleep 0.15
/opt/homebrew/bin/cliclick "dd:${c1}" "du:${c2}"
echo "swiped ${c1} -> ${c2}"