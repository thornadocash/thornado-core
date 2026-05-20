//go:build chainnet
// +build chainnet

package tcyclaimlist

import (
	_ "embed"
)

//go:embed tcy_claimers_chainnet.json
var TCYClaimsListRaw []byte
