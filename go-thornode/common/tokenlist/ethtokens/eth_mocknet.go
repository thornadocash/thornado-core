//go:build mocknet
// +build mocknet

package ethtokens

import (
	_ "embed"
)

//go:embed eth_mocknet_latest.json
var ETHTokenListRaw []byte
