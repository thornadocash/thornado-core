//go:build mocknet
// +build mocknet

package evm

import (
	"math/big"

	"gitlab.com/thorchain/thornode/v3/common"
)

// GetHeight returns the current block height.
func (e *EVMScanner) GetHeight() (int64, error) {
	height, err := e.ethRpc.GetBlockHeight()
	if err != nil {
		return -1, err
	}
	return height, nil
}

// ApprovalTxRateDustWei returns the amount of dust wei to be added to the approval tx
// fee rate to avoid a hash collision on approval transactions of different chains in
// mocknet. This function only returns unique amounts in mocknet, as mainnet chains have
// different chain IDs that are included in the transaction hash to prevent collisions.
func (e *EVMScanner) ApprovalTxRateDustWei() *big.Int {
	switch e.cfg.ChainID {
	case common.AVAXChain:
		return big.NewInt(1)
	case common.BASEChain:
		return big.NewInt(2)
	case common.BSCChain:
		return big.NewInt(3)
	case common.POLChain:
		return big.NewInt(4)
	default:
		e.logger.Fatal().Msg("unsupported chain id for approval tx dust wei")
		return nil
	}
}
