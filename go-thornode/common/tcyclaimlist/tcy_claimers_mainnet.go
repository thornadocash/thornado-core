//go:build !stagenet && !chainnet && !mocknet
// +build !stagenet,!chainnet,!mocknet

package tcyclaimlist

import (
	_ "embed"
)

//go:embed tcy_claimers_mainnet.json
var TCYClaimsListRaw []byte
