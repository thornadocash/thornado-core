//go:build stagenet
// +build stagenet

package tcyclaimlist

import (
	_ "embed"
)

//go:embed tcy_claimers_stagenet.json
var TCYClaimsListRaw []byte
