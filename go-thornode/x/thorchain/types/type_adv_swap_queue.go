package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
)

// AdvSwapQueueIndexItem represents an item in the advanced swap queue index
type AdvSwapQueueIndexItem struct {
	TxID  common.TxID
	Index int
}
