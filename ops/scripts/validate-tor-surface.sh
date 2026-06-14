#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

require_text() {
  file="$1"
  text="$2"
  if ! grep -Fq "$text" "$ROOT_DIR/$file"; then
    echo "missing expected text in $file: $text" >&2
    exit 1
  fi
}

reject_text() {
  file="$1"
  text="$2"
  if grep -Fq "$text" "$ROOT_DIR/$file"; then
    echo "rejected text found in $file: $text" >&2
    exit 1
  fi
}

require_text go-thornado/cmd/thornado-ui/main.go 'flag.String("listen", "127.0.0.1:1316"'
require_text go-thornado/cmd/thornado-ui/main.go 'flag.String("node", "http://127.0.0.1:1317"'
require_text go-thornado/config/default.yaml 'address: tcp://127.0.0.1:1317'
require_text go-thornado/config/default.yaml 'address: 127.0.0.1:9090'
require_text go-thornado/config/config.go 'tcp://127.0.0.1:%d", rpcPort'
require_text ops/tor/torrc 'HiddenServicePort 80 127.0.0.1:1316'
require_text ops/tor/torrc 'HiddenServicePort 80 127.0.0.1:1317'
require_text ops/tor/torrc 'HiddenServicePort 80 127.0.0.1:26657'

reject_text go-thornado/ui/static/index.html '<script src="http://'
reject_text go-thornado/ui/static/index.html '<script src="https://'
reject_text go-thornado/ui/static/index.html 'location.replace("http://127.0.0.1:1316'

echo "Tor public surface validation passed"
