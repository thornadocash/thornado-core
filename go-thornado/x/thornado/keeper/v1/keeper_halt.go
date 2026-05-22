package keeperv1

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) IsTradingHalt(ctx cosmos.Context, msg cosmos.Msg) bool {
	return false
}

func (k KVStore) IsGlobalTradingHalted(ctx cosmos.Context) bool {
	return false
}

func (k KVStore) IsChainTradingHalted(ctx cosmos.Context, chain common.Chain) bool {
	return k.IsChainHalted(ctx, chain)
}

func (k KVStore) IsChainHalted(ctx cosmos.Context, chain common.Chain) bool {
	haltChain, err := k.GetMimir(ctx, "HaltChainGlobal")
	if err == nil && (haltChain > 0 && haltChain <= ctx.BlockHeight()) {
		ctx.Logger().Debug("global is halt")
		return true
	}

	pauseChain, err := k.GetMimir(ctx, "NodePauseChainGlobal")
	if err == nil && pauseChain >= ctx.BlockHeight() {
		ctx.Logger().Debug("node global is pause")
		return true
	}

	haltMimirKey := fmt.Sprintf("Halt%sChain", chain)
	haltChain, err = k.GetMimir(ctx, haltMimirKey)
	if err == nil && (haltChain > 0 && haltChain <= ctx.BlockHeight()) {
		ctx.Logger().Debug("chain is halt via admin or double-spend check", "chain", chain)
		return true
	}

	solvencyHaltMimirKey := fmt.Sprintf("SolvencyHalt%sChain", chain)
	haltChain, err = k.GetMimir(ctx, solvencyHaltMimirKey)
	if err == nil && (haltChain > 0 && haltChain <= ctx.BlockHeight()) {
		ctx.Logger().Debug("chain is halt via solvency check", "chain", chain)
		return true
	}
	return false
}

// TODO: This is key is named `Pause` yet behaves like a `Halt`
// (halt from a height rather than pause until a height).
