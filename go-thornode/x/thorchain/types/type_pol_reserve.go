package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// NewPOLReserveDeposit creates a new POLReserveDeposit for the given asset
func NewPOLReserveDeposit(asset common.Asset) POLReserveDeposit {
	return POLReserveDeposit{
		Asset:         asset,
		RuneDeposited: cosmos.ZeroUint(),
	}
}
