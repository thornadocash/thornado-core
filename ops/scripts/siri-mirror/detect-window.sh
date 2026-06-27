#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${ROOT}/config.env"

read_window() {
  osascript <<'APPLESCRIPT'
tell application "iPhone Mirroring" to activate
delay 0.3
tell application "System Events"
  tell process "iPhone Mirroring"
    set frontmost to true
    set winCount to count of windows
    if winCount is 0 then
      return "ERROR:no_windows"
    end if
    set w to window 1
    set p to position of w
    set s to size of w
    return (item 1 of p as text) & "," & (item 2 of p as text) & "," & (item 1 of s as text) & "," & (item 2 of s as text)
  end tell
end tell
APPLESCRIPT
}

out="$(read_window)"
if [[ "${out}" == ERROR:* ]]; then
  echo "${out}" >&2
  exit 1
fi

IFS=',' read -r wx wy ww wh <<<"${out}"

# Narrow phone-shaped window: mirrored display fills client area.
px=0
py=28
pw=${ww}
ph=$((wh - 28))

mkdir -p "$(dirname "${CONFIG}")"
cat > "${CONFIG}" <<EOF
# Auto-detected $(date -u +%Y-%m-%dT%H:%M:%SZ)
WINDOW_X=${wx}
WINDOW_Y=${wy}
WINDOW_W=${ww}
WINDOW_H=${wh}
PHONE_X=${px}
PHONE_Y=${py}
PHONE_W=${pw}
PHONE_H=${ph}
SIRI_FIELD_X=0.50
SIRI_FIELD_Y=0.12
RUN_ROOT=/tmp/siri-mirror-bot
CAPTURE_PATH=\${RUN_ROOT}/capture.png
EOF

echo "Wrote ${CONFIG}"
echo "WINDOW=${wx},${wy} ${ww}x${wh}"
echo "PHONE(inset)=${px},${py} ${pw}x${ph}"