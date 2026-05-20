package types

import (
	"github.com/thornadocash/go-thornado/common"
)

// AdvSwapQueueIndexItem represents an item in the advanced swap queue index
type AdvSwapQueueIndexItem struct {
	TxID  common.TxID
	Index int
}
