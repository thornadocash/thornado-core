#!/usr/bin/env bash
set -euo pipefail

die() {
  echo "ERR: $*"
  exit 1
}

# check docs version
version=$(cat version)
if ! grep "^  version: ${version}" openapi/openapi.yaml; then
  die "docs version (openapi/openapi.yaml) does not match version file ${version}"
fi

# format golang
which gofumpt &>/dev/null || go install mvdan.cc/gofumpt@v0.5.0
FILTER=(-e '^docs/' -e '.pb.go$' -e '^openapi/gen' -e '_gen.go' -e '.pb.gw.go$' -e 'wire_gen.go$' -e '^api/')

if [ -n "$(git ls-files '*.go' | grep -v "${FILTER[@]}" | xargs gofumpt -l 2>/dev/null)" ]; then
  git ls-files '*.go' | grep -v "${FILTER[@]}" | xargs gofumpt -w 2>/dev/null
  die "Go formatting errors"
fi
go mod verify

./scripts/lint-handlers.bash
./scripts/lint-tokens.bash

go run tools/analyze/main.go -rand -map_iteration ./common/... ./constants/... ./x/...

go run tools/analyze/main.go -float_comparison ./...

go run tools/lint-whitelist-tokens/main.go

# ensure upgrades in app/upgrades only match the current version
for dir in app/upgrades/*/; do
  upgrade_version=$(basename "${dir}")
  if [ "${upgrade_version}" != "${version}" ] && [ "${upgrade_version}" != "standard" ]; then
    die "upgrade version ${upgrade_version} does not match current version ${version}"
  fi
done
