package types

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
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
