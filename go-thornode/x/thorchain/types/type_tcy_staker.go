package types

import (
	"errors"

	"cosmossdk.io/math"
	"gitlab.com/thorchain/thornode/v3/common"
)

func NewTCYStaker(address common.Address, amount math.Uint) TCYStaker {
	return TCYStaker{
		Address: address,
		Amount:  amount,
	}
}

func (t *TCYStaker) Valid() error {
	if t.Address.IsEmpty() {
		return errors.New("address is empty")
	}
	if t.Amount.IsZero() {
		return errors.New("staking amount is zero")
	}
	return nil
}

func (t *TCYStaker) IsEmpty() bool {
	return t.Address.IsEmpty() && t.Amount.IsZero()
}
