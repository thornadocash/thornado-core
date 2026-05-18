//go:build !mocknet
// +build !mocknet

package poltokens

import (
	_ "embed"
)

//go:embed pol_mainnet_latest.json
var POLTokenListRaw []byte
