//go:build mocknet
// +build mocknet

package btc

import (
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

func GetConfMulBasisPoint(chain string, bridge thornadoclient.ThornadoBridge) (cosmos.Uint, error) {
	return cosmos.NewUint(constants.MaxBasisPts), nil
}

func MaxConfAdjustment(confirm uint64, chain string, bridge thornadoclient.ThornadoBridge) (uint64, error) {
	return 1, nil
}
