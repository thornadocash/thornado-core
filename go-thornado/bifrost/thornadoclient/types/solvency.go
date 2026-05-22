package types

import (
	"github.com/thornadocash/go-thornado/common"
)

// Solvency structure is to hold all the information necessary to report solvency to Thornado
type Solvency struct {
	Height int64
	Chain  common.Chain
	PubKey common.PubKey
	Coins  common.Coins
}
