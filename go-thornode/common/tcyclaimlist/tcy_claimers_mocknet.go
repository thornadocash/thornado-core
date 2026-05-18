//go:build mocknet
// +build mocknet

package tcyclaimlist

import (
	_ "embed"
)

//go:embed tcy_claimers_mocknet.json
var TCYClaimsListRaw []byte
