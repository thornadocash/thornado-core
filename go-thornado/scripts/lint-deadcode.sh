#!/bin/bash
# Dead code inspector. Meant to be run manually by devs,
# too many false positives to use in CI.
set -euo pipefail

which deadcode &>/dev/null || go install golang.org/x/tools/cmd/deadcode@latest

deadcode -test ./...
