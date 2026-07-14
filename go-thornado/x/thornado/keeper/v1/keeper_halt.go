package keeperv1

import (
	"strings"

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

// SetUnmatchedOutboundHeight records the block height at which a FINAL
// observed outbound first failed to match any open txout. Halting is deferred
// until a later processing still finds no match: transient one-block ordering
// between observation finality and batch refresh state was flapping
// halt/auto-unhalt on every unified sweep settlement, while a genuinely
// unauthorized spend never matches and still halts one grace period later
// (the solvency check independently halts on the drained wallet as well).
func (k KVStore) SetUnmatchedOutboundHeight(ctx cosmos.Context, txID common.TxID, height int64) {
	k.setInt64(ctx, k.GetKey(prefixUnmatchedOutbound, strings.ToLower(txID.String())), height)
}

// GetUnmatchedOutboundHeight returns the recorded first-unmatched height for
// the tx, or zero when the tx has never failed to match.
func (k KVStore) GetUnmatchedOutboundHeight(ctx cosmos.Context, txID common.TxID) int64 {
	var record int64
	_, _ = k.getInt64(ctx, k.GetKey(prefixUnmatchedOutbound, strings.ToLower(txID.String())), &record)
	return record
}

// DeleteUnmatchedOutbound clears the first-unmatched record once the tx
// matches a txout.
func (k KVStore) DeleteUnmatchedOutbound(ctx cosmos.Context, txID common.TxID) {
	k.del(ctx, k.GetKey(prefixUnmatchedOutbound, strings.ToLower(txID.String())))
}
