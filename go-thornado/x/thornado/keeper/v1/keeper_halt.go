package keeperv1

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

func (k KVStore) IsChainHalted(ctx cosmos.Context, chain common.Chain) bool {
	haltChain := k.GetConfigInt64(ctx, constants.Halt_ChainGlobal)
	if haltChain > 0 && haltChain <= ctx.BlockHeight() {
		ctx.Logger().Debug("global is halt")
		return true
	}

	pauseChain, err := k.GetConfig(ctx, constants.ConfigKeyNodePauseChainGlobal)
	if err == nil && pauseChain >= ctx.BlockHeight() {
		ctx.Logger().Debug("node global is pause")
		return true
	}

	haltChain = k.GetConfigInt64(ctx, constants.Halt_SolvencyCheck)
	if haltChain > 0 && haltChain <= ctx.BlockHeight() {
		ctx.Logger().Debug("chain is halt via solvency check", "chain", chain)
		return true
	}
	return false
}

// TODO: This is key is named `Pause` yet behaves like a `Halt`
// (halt from a height rather than pause until a height).
