//go:build mocknet
// +build mocknet

package utxo

import (
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func GetConfMulBasisPoint(chain string, bridge thornadoclient.ThornadoBridge) (cosmos.Uint, error) {
	return cosmos.NewUint(1), nil
}

func MaxConfAdjustment(confirm uint64, chain string, bridge thornadoclient.ThornadoBridge) (uint64, error) {
	return 1, nil
}
