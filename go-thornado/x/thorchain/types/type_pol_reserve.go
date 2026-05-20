package types

import (
	"github.com/thornadocash/go-thornado/common"
	cosmos "github.com/thornadocash/go-thornado/common/cosmos"
)

// NewPOLReserveDeposit creates a new POLReserveDeposit for the given asset
func NewPOLReserveDeposit(asset common.Asset) POLReserveDeposit {
	return POLReserveDeposit{
		Asset:         asset,
		RuneDeposited: cosmos.ZeroUint(),
	}
}
