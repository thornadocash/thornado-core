package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type ModifyLimitSwaps []ModifyLimitSwap

func NewModifyLimitSwap(from common.Address, source, target common.Coin, mod cosmos.Uint) ModifyLimitSwap {
	return ModifyLimitSwap{
		From:                 from,
		Source:               source,
		Target:               target,
		ModifiedTargetAmount: mod,
	}
}
