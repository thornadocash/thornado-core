//go:build !mocknet
// +build !mocknet

package trontokens

import (
	_ "embed"
)

//go:embed tron_mainnet_latest.json
var TRONTokenListRaw []byte
