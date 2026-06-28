package thornado

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

func processSolvencyAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter *keeper.SolvencyVoter,
	attester cosmos.AccAddress,
	active NodeAccounts,
	s *common.Solvency,
	shouldPenalizeForDuplicate bool,
) error {
	k := mgr.Keeper()

	observePenaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, k, constants.Observation_DelayFlexibilityMinutes)

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_solvency"),
		telemetry.NewLabel("chain", string(s.Chain)),
	}))

	penaltyManager := mgr.PenaltyManager()

	if !voter.Sign(attester) {
		// Penalty for the network having to handle the extra message/s.
		if shouldPenalizeForDuplicate {
			penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		}
		ctx.Logger().Info("signer already signed MsgSolvency", "signer", attester.String(), "id", s.Id)
		return nil
	}

	if !voter.HasConsensus(active) {
		// Before consensus, penalty until consensus.
		penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		return nil
	}

	// from this point , solvency reach consensus
	if voter.ConsensusBlockHeight > 0 {
		// After consensus, only decrement penalty points if within the Observation_DelayFlexibilityMinutes window.
		if (voter.ConsensusBlockHeight + observeFlex) >= ctx.BlockHeight() {
			penaltyManager.DecPenaltyPoints(penaltyCtx, lackOfObservationPenalty, attester)
		}
		// solvency tx already processed
		return nil
	}
	vault, err := k.GetVault(ctx, voter.PubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault", "error", err)
		return fmt.Errorf("fail to get vault: %w", err)
	}

	voter.ConsensusBlockHeight = ctx.BlockHeight()

	// This signer brings the voter to consensus; increment the signer's penalty points like the before-consensus signers,
	// then decrement all the signers' penalty points and increment the non-signers' penalty points.
	penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
	signers := voter.GetSigners()
	nonSigners := getNonSigners(active, signers)
	penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
	penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)

	// Do checks for whether to act on this consensus or not.
	haltSolvencyCheckKey := constants.Halt_SolvencyCheck.String()
	stopSolvencyCheck := k.GetConfigInt64(ctx, constants.Halt_SolvencyCheck)
	haltChain := stopSolvencyCheck
	// If the chain was halted this block, leave it halted without overriding.
	// (For instance if halted because of a different vault which is insolvent.)
	// Also don't unhalt if the chain was manually halted for a future height
	// or indefinitely ('1').
	if shouldSkipSolvencyHaltAction(haltChain, ctx.BlockHeight()) {
		return nil
	}
	// If the solvency message is from a height which does not reflect inbounds
	// reflected in the supermajority-observation vault balances,
	// do not act on it.
	lastChainHeight, err := k.GetLastChainHeight(ctx, voter.Chain)
	if err != nil {
		ctx.Logger().Error("fail to get last chain height", "chain", voter.Chain, "error", err)
	}
	// According to the validate msg.Id check, the Height is consistent for all the voter's messages.
	if s.Height < lastChainHeight {
		ctx.Logger().Info("solvency message consensus for height before last chain height inbound supermajority observation", "chain", voter.Chain, "vault pubkey", voter.PubKey, "last chain height", lastChainHeight, "solvency message height", s.Height)
		return nil
	}

	isInsolvent := insolvencyCheck(ctx, mgr, vault, voter.Coins, voter.Chain)

	// If insolvent: halt the chain if unhalted, or refresh the halt height if already halted
	// from an earlier block. Refreshing the halt height prevents a different solvent vault
	// from unhalting the chain while this vault remains insolvent.
	if isInsolvent {
		if haltChain <= 0 {
			setSolvencyAndSigningHalt(ctx, k, mgr.EventMgr(), voter.Chain, ctx.BlockHeight())
			ctx.Logger().Info("chain is insolvent, halt until it is resolved", "chain", voter.Chain)
		} else if haltChain > 1 && haltChain < ctx.BlockHeight() {
			// Refresh halt height to current block so a solvent vault can't unhalt next block
			setSolvencyAndSigningHalt(ctx, k, mgr.EventMgr(), voter.Chain, ctx.BlockHeight())
		}
	}

	// If not insolvent and the chain is halted from an earlier block height, unhalt the chain.
	// A different insolvent vault on the same chain will have refreshed haltChain to the current
	// block, preventing this unhalt via the guard at line 106 (haltChain >= ctx.BlockHeight()).
	if !isInsolvent && haltChain > 1 {
		// if the chain was halted by previous solvency checker, auto unhalt it
		ctx.Logger().Info("auto un-halt", "chain", voter.Chain, "previous halt height", haltChain, "current block height", ctx.BlockHeight())
		k.SetConfig(ctx, haltSolvencyCheckKey, 0)
		configEvent := NewEventSetConfig(strings.ToUpper(haltSolvencyCheckKey), "0")
		if err := mgr.EventMgr().EmitEvent(ctx, configEvent); err != nil {
			ctx.Logger().Error("fail to emit set_config event", "error", err)
		}
		clearMatchingSolvencySigningHalt(ctx, k, mgr.EventMgr(), voter.Chain, haltChain)
	}

	return nil
}

func shouldSkipSolvencyHaltAction(haltHeight, blockHeight int64) bool {
	return haltHeight == 1 || haltHeight >= blockHeight
}

type solvencyHaltConfigKeeper interface {
	GetConfig(cosmos.Context, string) (int64, error)
	SetConfig(cosmos.Context, string, int64)
}

func setSolvencyAndSigningHalt(ctx cosmos.Context, k solvencyHaltConfigKeeper, eventMgr EventManager, chain common.Chain, height int64) {
	haltSolvencyCheckKey := constants.Halt_SolvencyCheck.String()
	haltSigningKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, chain)

	k.SetConfig(ctx, haltSolvencyCheckKey, height)
	k.SetConfig(ctx, haltSigningKey, height)

	if eventMgr == nil {
		return
	}
	if err := eventMgr.EmitEvent(ctx, NewEventSetConfig(strings.ToUpper(haltSolvencyCheckKey), strconv.FormatInt(height, 10))); err != nil {
		ctx.Logger().Error("fail to emit set_config event", "error", err)
	}
	if err := eventMgr.EmitEvent(ctx, NewEventSetConfig(strings.ToUpper(haltSigningKey), strconv.FormatInt(height, 10))); err != nil {
		ctx.Logger().Error("fail to emit set_config event", "error", err)
	}
}

func clearMatchingSolvencySigningHalt(ctx cosmos.Context, k solvencyHaltConfigKeeper, eventMgr EventManager, chain common.Chain, haltChain int64) {
	haltSigningKey := fmt.Sprintf(constants.ConfigTemplateHaltSigning, chain)
	if signingHalt, _ := k.GetConfig(ctx, haltSigningKey); signingHalt != haltChain {
		return
	}

	k.SetConfig(ctx, haltSigningKey, 0)
	if eventMgr == nil {
		return
	}
	if err := eventMgr.EmitEvent(ctx, NewEventSetConfig(strings.ToUpper(haltSigningKey), "0")); err != nil {
		ctx.Logger().Error("fail to emit set_config event", "error", err)
	}
}

// insolvencyCheck compare the coins in vault against the coins report by solvency message
// insolvent usually means vault has more coins than wallet
// return true means the vault is insolvent , the network should halt , otherwise false
func insolvencyCheck(ctx cosmos.Context, mgr Manager, vault Vault, coins common.Coins, chain common.Chain) bool {
	adjustVault, err := excludePendingOutboundFromVault(ctx, mgr, vault)
	if err != nil {
		ctx.Logger().Error("fail to exclude pending outbound from vault, assuming insolvent", "error", err)
		return true
	}
	// Build a map of original vault coin amounts so we can detect SafeSub clamping.
	// When pending outbounds exceed the vault balance, SafeSub clamps to zero and
	// the adjusted coin is empty. In that case, fall back to the original vault amount
	// so the solvency check can still detect theft.
	originalAmounts := make(map[string]cosmos.Uint)
	for _, c := range vault.Coins {
		if c.Asset.Chain.Equals(chain) && !c.IsEmpty() {
			originalAmounts[c.Asset.String()] = c.Amount
		}
	}
	// Use the coin in vault as baseline , wallet can have more coins than vault
	for _, c := range adjustVault.Coins {
		if !c.Asset.Chain.Equals(chain) {
			continue
		}
		// If adjusted amount is zero but original was non-zero, pending outbounds
		// exceeded the vault balance. Use the original amount as a fallback so that
		// insolvency (e.g. wallet drained to zero) is not silently bypassed.
		if c.IsEmpty() {
			origAmount, ok := originalAmounts[c.Asset.String()]
			if !ok || origAmount.IsZero() {
				continue
			}
			ctx.Logger().Info("pending outbounds exceed vault balance, using original for solvency check", "asset", c.Asset.String(), "original", origAmount.String())
			c.Amount = origAmount
		}
		walletCoin := coins.GetCoin(c.Asset)

		// Compute the gap between vault and wallet balances.
		// When walletCoin is absent (IsEmpty), the full vault amount is the gap,
		// which then goes through the same asset-value threshold check as partial gaps.
		gap := cosmos.ZeroUint()
		if walletCoin.IsEmpty() {
			gap = c.Amount
		} else if c.Amount.GT(walletCoin.Amount) {
			gap = c.Amount.Sub(walletCoin.Amount)
		}
		if gap.IsZero() {
			continue
		}

		if gasAllowance := recentAuthorizedOutboundGas(ctx, mgr, vault, chain, c.Asset); !gasAllowance.IsZero() {
			if gap.LTE(gasAllowance) {
				ctx.Logger().Info(
					"vault solvency gap covered by recent authorized outbound gas",
					"asset", c.Asset.String(),
					"vault amount", c.Amount.String(),
					"wallet amount", walletCoin.Amount.String(),
					"gap", gap.String(),
					"authorized outbound gas", gasAllowance.String(),
				)
				continue
			}
			gap = gap.Sub(gasAllowance)
		}

		gapValue := gap
		if gapValue.IsZero() && !gap.IsZero() {
			ctx.Logger().Info("cannot value gap for solvency check (vault has no accounted funds), treating as insolvent", "asset", c.Asset.String(), "gap", gap.String())
			return true
		}
		if !gapValue.IsZero() {
			ctx.Logger().Info("vault insolvent", "asset", c.Asset.String(), "vault amount", c.Amount.String(), "wallet amount", walletCoin.Amount.String(), "gap", gap.String(), "gap value", gapValue.String())
			return true
		}
	}
	return false
}

func excludePendingOutboundFromVault(ctx cosmos.Context, mgr Manager, vault Vault) (Vault, error) {
	// Deep copy vault coins to avoid mutating the caller's slice backing array.
	// Element-by-element copy ensures the pass-by-value contract is honored,
	// even if future code modifies Amount in-place rather than via SafeSub.
	coinsCopy := make(common.Coins, len(vault.Coins))
	for i, c := range vault.Coins {
		coinsCopy[i] = common.NewCoin(c.Asset, c.Amount)
	}
	vault.Coins = coinsCopy

	// Unsigned txouts are only pending during the signing window. Signed BTC txouts stay
	// pending until the observed-out voter finishes vault accounting.
	signingPeriod := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)
	startHeight := ctx.BlockHeight() - signingPeriod
	if startHeight < 1 {
		startHeight = 1
	}
	seenSignedOutHashes := make(map[common.TxID]struct{})
	for i := int64(1); i < ctx.BlockHeight(); i++ {
		blockOut, err := mgr.Keeper().GetTxOut(ctx, i)
		if err != nil {
			ctx.Logger().Error("fail to get block tx out", "error", err)
			return vault, fmt.Errorf("fail to get block tx out, err: %w", err)
		}
		vault = deductVaultBlockPendingOutbound(ctx, mgr, vault, blockOut, i >= startHeight, seenSignedOutHashes)
	}
	return vault, nil
}

func deductVaultBlockPendingOutbound(
	ctx cosmos.Context,
	mgr Manager,
	vault Vault,
	block *TxOut,
	includeOpenTxOut bool,
	seenSignedOutHashes map[common.TxID]struct{},
) Vault {
	if block == nil {
		return vault
	}
	for _, txOutItem := range block.TxArray {
		if !txOutItem.VaultPubKey.Equals(vault.PubKey) {
			continue
		}
		if txOutItem.OutHash.IsEmpty() {
			if !includeOpenTxOut {
				continue
			}
			vault = deductVaultOutboundItem(vault, txOutItem.Coin, txOutMaxGasCoin(txOutItem))
			continue
		}
		if !signedOutboundAwaitingVaultAccounting(ctx, mgr, txOutItem) {
			continue
		}
		gasCoin := signedOutboundGasCoin(ctx, mgr, vault, txOutItem, seenSignedOutHashes)
		vault = deductVaultOutboundItem(vault, txOutItem.Coin, gasCoin)
	}
	return vault
}

func deductVaultOutboundItem(vault Vault, coin, gasCoin common.Coin) Vault {
	for i, vaultCoin := range vault.Coins {
		if !coin.IsEmpty() && vaultCoin.Asset.Equals(coin.Asset) {
			vault.Coins[i].Amount = common.SafeSub(vault.Coins[i].Amount, coin.Amount)
		}
		if !gasCoin.IsEmpty() && vaultCoin.Asset.Equals(gasCoin.Asset) {
			vault.Coins[i].Amount = common.SafeSub(vault.Coins[i].Amount, gasCoin.Amount)
		}
	}
	return vault
}

func txOutMaxGasCoin(txOutItem TxOutItem) common.Coin {
	if txOutItem.MaxGas.IsEmpty() {
		return common.Coin{}
	}
	return txOutItem.MaxGas.ToCoins().GetCoin(txOutItem.Chain.GetGasAsset())
}

func signedOutboundAwaitingVaultAccounting(ctx cosmos.Context, mgr Manager, txOutItem TxOutItem) bool {
	if !txOutItem.Chain.Equals(common.BTCChain) {
		return false
	}
	voter, err := mgr.Keeper().GetObservedTxOutVoter(ctx, txOutItem.OutHash)
	if err != nil {
		return true
	}
	return !observedOutboundVoterDone(voter)
}

func observedOutboundVoterDone(voter ObservedTxVoter) bool {
	if voter.Tx.Status == common.Status_done {
		return true
	}
	for _, tx := range voter.Txs {
		if tx.Status == common.Status_done {
			return true
		}
	}
	return false
}

func signedOutboundGasCoin(
	ctx cosmos.Context,
	mgr Manager,
	vault Vault,
	txOutItem TxOutItem,
	seenSignedOutHashes map[common.TxID]struct{},
) common.Coin {
	asset := txOutItem.Chain.GetGasAsset()
	if _, ok := seenSignedOutHashes[txOutItem.OutHash]; ok {
		return common.Coin{}
	}
	seenSignedOutHashes[txOutItem.OutHash] = struct{}{}

	voter, err := mgr.Keeper().GetObservedTxOutVoter(ctx, txOutItem.OutHash)
	if err == nil {
		if gas, ok := observedOutboundGas(voter, vault, txOutItem.Chain, asset); ok && !gas.IsZero() {
			return common.NewCoin(asset, gas)
		}
	}
	return txOutMaxGasCoin(txOutItem)
}

func recentAuthorizedOutboundGas(ctx cosmos.Context, mgr Manager, vault Vault, chain common.Chain, asset common.Asset) cosmos.Uint {
	if !asset.Equals(chain.GetGasAsset()) {
		return cosmos.ZeroUint()
	}

	signingPeriod := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)
	startHeight := ctx.BlockHeight() - signingPeriod
	if startHeight < 1 {
		startHeight = 1
	}

	seenOutHashes := make(map[common.TxID]struct{})
	total := cosmos.ZeroUint()
	for height := startHeight; height < ctx.BlockHeight(); height++ {
		blockOut, err := mgr.Keeper().GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("fail to get block tx out while checking outbound gas allowance", "error", err)
			continue
		}
		for _, item := range blockOut.TxArray {
			if !item.VaultPubKey.Equals(vault.PubKey) || !item.Chain.Equals(chain) || item.OutHash.IsEmpty() {
				continue
			}
			if _, ok := seenOutHashes[item.OutHash]; ok {
				continue
			}
			seenOutHashes[item.OutHash] = struct{}{}
			voter, err := mgr.Keeper().GetObservedTxOutVoter(ctx, item.OutHash)
			if err != nil {
				continue
			}
			if gas, ok := observedOutboundGas(voter, vault, chain, asset); ok {
				total = total.Add(gas)
			}
		}
	}
	return total
}

func observedOutboundGas(voter ObservedTxVoter, vault Vault, chain common.Chain, asset common.Asset) (cosmos.Uint, bool) {
	if gas, ok := observedTxGas(voter.Tx, voter.TxID, vault, chain, asset); ok {
		return gas, true
	}
	for _, tx := range voter.Txs {
		if gas, ok := observedTxGas(tx, voter.TxID, vault, chain, asset); ok {
			return gas, true
		}
	}
	return cosmos.ZeroUint(), false
}

func observedTxGas(tx common.ObservedTx, txID common.TxID, vault Vault, chain common.Chain, asset common.Asset) (cosmos.Uint, bool) {
	if tx.IsEmpty() ||
		!tx.Tx.ID.Equals(txID) ||
		!tx.Tx.Chain.Equals(chain) ||
		!tx.ObservedPubKey.Equals(vault.PubKey) {
		return cosmos.ZeroUint(), false
	}
	return tx.Tx.Gas.ToCoins().GetCoin(asset).Amount, true
}
