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
	shouldSlashForDuplicate bool,
) error {
	k := mgr.Keeper()

	observeSlashPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, k, constants.Observation_DelayFlexibilityMinutes)

	slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_solvency"),
		telemetry.NewLabel("chain", string(s.Chain)),
	}))

	slasher := mgr.Slasher()

	if !voter.Sign(attester) {
		// Slash for the network having to handle the extra message/s.
		if shouldSlashForDuplicate {
			slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
		}
		ctx.Logger().Info("signer already signed MsgSolvency", "signer", attester.String(), "id", s.Id)
		return nil
	}

	if !voter.HasConsensus(active) {
		// Before consensus, slash until consensus.
		slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
		return nil
	}

	// from this point , solvency reach consensus
	if voter.ConsensusBlockHeight > 0 {
		// After consensus, only decrement slash points if within the Observation_DelayFlexibilityMinutes window.
		if (voter.ConsensusBlockHeight + observeFlex) >= ctx.BlockHeight() {
			slasher.DecSlashPoints(slashCtx, lackOfObservationPenalty, attester)
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

	// This signer brings the voter to consensus; increment the signer's slash points like the before-consensus signers,
	// then decrement all the signers' slash points and increment the non-signers' slash points.
	slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
	signers := voter.GetSigners()
	nonSigners := getNonSigners(active, signers)
	slasher.DecSlashPoints(slashCtx, observeSlashPoints, signers...)
	slasher.IncSlashPoints(slashCtx, lackOfObservationPenalty, nonSigners...)

	// Do checks for whether to act on this consensus or not.
	haltSolvencyCheckKey := constants.Halt_SolvencyCheck.String()
	stopSolvencyCheck := k.GetConfigInt64(ctx, constants.Halt_SolvencyCheck)
	if stopSolvencyCheck > 0 && stopSolvencyCheck < ctx.BlockHeight() {
		return nil
	}
	haltChain := stopSolvencyCheck
	// If the chain was halted this block, leave it halted without overriding.
	// (For instance if halted because of a different vault which is insolvent.)
	// Also don't unhalt if the chain was manually halted for a future height
	// or indefinitely ('1').
	if haltChain >= ctx.BlockHeight() || haltChain == 1 {
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
			k.SetConfig(ctx, haltSolvencyCheckKey, ctx.BlockHeight())
			configEvent := NewEventSetConfig(strings.ToUpper(haltSolvencyCheckKey), strconv.FormatInt(ctx.BlockHeight(), 10))
			if err := mgr.EventMgr().EmitEvent(ctx, configEvent); err != nil {
				ctx.Logger().Error("fail to emit set_config event", "error", err)
			}
			ctx.Logger().Info("chain is insolvent, halt until it is resolved", "chain", voter.Chain)
		} else if haltChain > 1 && haltChain < ctx.BlockHeight() {
			// Refresh halt height to current block so a solvent vault can't unhalt next block
			k.SetConfig(ctx, haltSolvencyCheckKey, ctx.BlockHeight())
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
	}

	return nil
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
		// which then goes through the same RUNE-value threshold check as partial gaps.
		gap := cosmos.ZeroUint()
		if walletCoin.IsEmpty() {
			gap = c.Amount
		} else if c.Amount.GT(walletCoin.Amount) {
			gap = c.Amount.Sub(walletCoin.Amount)
		}
		if gap.IsZero() {
			continue
		}

		gapInRune := gap
		if gapInRune.IsZero() && !gap.IsZero() {
			ctx.Logger().Info("cannot value gap for solvency check (pool has no liquidity), treating as insolvent", "asset", c.Asset.String(), "gap", gap.String())
			return true
		}
		if !gapInRune.IsZero() {
			ctx.Logger().Info("vault insolvent", "asset", c.Asset.String(), "vault amount", c.Amount.String(), "wallet amount", walletCoin.Amount.String(), "gap", gap.String(), "gap in rune", gapInRune.String())
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

	// go back Keysign_PeriodMinutes window to see whether there are outstanding tx, the vault need to send out
	// if there is , deduct it from their balance
	signingPeriod := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)
	startHeight := ctx.BlockHeight() - signingPeriod
	if startHeight < 1 {
		startHeight = 1
	}
	for i := startHeight; i < ctx.BlockHeight(); i++ {
		blockOut, err := mgr.Keeper().GetTxOut(ctx, i)
		if err != nil {
			ctx.Logger().Error("fail to get block tx out", "error", err)
			return vault, fmt.Errorf("fail to get block tx out, err: %w", err)
		}
		vault = deductVaultBlockPendingOutbound(vault, blockOut)
	}
	return vault, nil
}

func deductVaultBlockPendingOutbound(vault Vault, block *TxOut) Vault {
	for _, txOutItem := range block.TxArray {
		if !txOutItem.VaultPubKey.Equals(vault.PubKey) {
			continue
		}
		// only still outstanding txout will be considered
		if !txOutItem.OutHash.IsEmpty() {
			continue
		}
		// deduct the gas asset from the vault as well
		var gasCoin common.Coin
		if !txOutItem.MaxGas.IsEmpty() {
			gasCoin = txOutItem.MaxGas.ToCoins().GetCoin(txOutItem.Chain.GetGasAsset())
		}
		for i, vaultCoin := range vault.Coins {
			if vaultCoin.Asset.Equals(txOutItem.Coin.Asset) {
				vault.Coins[i].Amount = common.SafeSub(vault.Coins[i].Amount, txOutItem.Coin.Amount)
			}
			if vaultCoin.Asset.Equals(gasCoin.Asset) {
				vault.Coins[i].Amount = common.SafeSub(vault.Coins[i].Amount, gasCoin.Amount)
			}
		}
	}
	return vault
}
