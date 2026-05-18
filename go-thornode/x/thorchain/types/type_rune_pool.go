package types

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func NewRUNEPool() RUNEPool {
	return RUNEPool{
		ReserveUnits:  cosmos.ZeroUint(),
		PoolUnits:     cosmos.ZeroUint(),
		RuneDeposited: cosmos.ZeroUint(),
		RuneWithdrawn: cosmos.ZeroUint(),
	}
}

func (rp RUNEPool) CurrentDeposit() cosmos.Int {
	deposited := cosmos.NewIntFromBigInt(rp.RuneDeposited.BigInt())
	withdrawn := cosmos.NewIntFromBigInt(rp.RuneWithdrawn.BigInt())
	return deposited.Sub(withdrawn)
}

func (rp RUNEPool) TotalUnits() cosmos.Uint {
	return rp.ReserveUnits.Add(rp.PoolUnits)
}
