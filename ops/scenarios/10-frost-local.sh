#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GO_DIR="${ROOT_DIR}/go-thornado"

cd "${GO_DIR}"

go test ./go-wrappers/frost/go-frost/sessions -count=1
go test ./bifrost/pkg/chainclients/utxo -run 'Frost|Bitcoin|Taproot|Signer' -count=1
go test ./bifrost/tss ./bifrost/p2p/messages ./bifrost/p2p/storage ./bifrost/signer ./x/thornado/types -count=1

echo "FROST local scenario passed"
