//go:build !mocknet
// +build !mocknet

package basetokens

import (
	_ "embed"
)

//go:embed base_mainnet_latest.json
var BASETokenListRaw []byte
