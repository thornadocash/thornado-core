package types

import (
	"errors"

	"cosmossdk.io/math"
	"gitlab.com/thorchain/thornode/v3/common"
)

func NewTCYClaimer(l1Address common.Address, asset common.Asset, amount math.Uint) TCYClaimer {
	return TCYClaimer{
		L1Address: l1Address,
		Asset:     asset,
		Amount:    amount,
	}
}

func (t *TCYClaimer) Valid() error {
	if t.L1Address.IsEmpty() {
		return errors.New("L1 address is empty")
	}
	if t.Amount.IsZero() {
		return errors.New("claim amount is zero")
	}
	if t.Asset.IsEmpty() {
		return errors.New("asset is empty")
	}
	return nil
}

func (t *TCYClaimer) IsEmpty() bool {
	return t.L1Address.IsEmpty() && t.Amount.IsZero() && t.Asset.IsEmpty()
}
