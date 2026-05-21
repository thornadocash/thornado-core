package thorchain

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

func processErrataTxAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter *keeper.ErrataTxVoter,
	attester cosmos.AccAddress,
	active NodeAccounts,
	er *common.ErrataTx,
	shouldSlashForDuplicate bool,
) error {
	k := mgr.Keeper()
	eventMgr := mgr.EventMgr()

	observeSlashPoints := mgr.GetConstants().GetInt64Value(constants.ObserveSlashPoints)
	lackOfObservationPenalty := mgr.GetConstants().GetInt64Value(constants.LackOfObservationPenalty)
	observeFlex := k.GetConfigInt64(ctx, constants.ObservationDelayFlexibility)

	slashCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{ // nolint
		telemetry.NewLabel("reason", "failed_observe_errata"),
		telemetry.NewLabel("chain", string(er.Chain)),
	}))

	slasher := mgr.Slasher()

	if !voter.Sign(attester) {
		// Slash for the network having to handle the extra message/s.
		if shouldSlashForDuplicate {
			slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
		}
		ctx.Logger().Info("signer already signed MsgErrataTx", "signer", attester.String(), "txid", er.Id)
		return nil
	}

	// doesn't have consensus yet
	if !voter.HasConsensus(active) {
		// Before consensus, slash until consensus.
		slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
		ctx.Logger().Info("not having consensus yet, return")
		return nil
	}

	if voter.BlockHeight > 0 {
		// After consensus, only decrement slash points if within the ObservationDelayFlexibility period.
		if (voter.BlockHeight + observeFlex) >= ctx.BlockHeight() {
			slasher.DecSlashPoints(slashCtx, lackOfObservationPenalty, attester)
		}
		// errata tx already processed
		return nil
	}

	voter.BlockHeight = ctx.BlockHeight()

	// This signer brings the voter to consensus; increment the signer's slash points like the before-consensus signers,
	// then decrement all the signers' slash points and increment the non-signers' slash points.
	slasher.IncSlashPoints(slashCtx, observeSlashPoints, attester)
	signers := voter.GetSigners()
	nonSigners := getNonSigners(active, signers)
	slasher.DecSlashPoints(slashCtx, observeSlashPoints, signers...)
	slasher.IncSlashPoints(slashCtx, lackOfObservationPenalty, nonSigners...)

	observedVoter, err := k.GetObservedTxInVoter(ctx, er.Id)
	if err != nil {
		return err
	}

	if len(observedVoter.Txs) == 0 {
		return processErrataOutboundTx(ctx, k, eventMgr, er)
	}
	if observedVoter.Tx.IsEmpty() {
		ctx.Logger().Info("tx has not reach consensus yet, so nothing need to be done", "tx_id", er.Id)
		return nil
	}

	tx := observedVoter.Tx.Tx
	if !tx.Chain.Equals(er.Chain) {
		// does not match chain
		return nil
	}

	// set the observed Tx to reverted only after chain validation passes
	observedVoter.SetReverted()
	k.SetObservedTxInVoter(ctx, observedVoter)
	if observedVoter.UpdatedVault {
		vaultPubKey := observedVoter.Tx.ObservedPubKey
		if !vaultPubKey.IsEmpty() {
			// try to deduct the asset from asgard
			var vault Vault
			vault, err = k.GetVault(ctx, vaultPubKey)
			if err != nil {
				return fmt.Errorf("fail to get active asgard vaults: %w", err)
			}
			vault.SubFunds(tx.Coins)
			if err = k.SetVault(ctx, vault); err != nil {
				return fmt.Errorf("fail to save vault, err: %w", err)
			}
		}
	}

	if !observedVoter.Tx.IsFinal() {
		ctx.Logger().Info("tx is not finalised, so nothing need to be done", "tx_id", er.Id)
		return nil
	}

	return nil
}

// processErrataOutboundTx handles an errata for an outbound tx that was sent but later reorged out.
// It re-credits funds to the vault so they are not abandoned.
// The tx is marked as reverted rather than rescheduled.
func processErrataOutboundTx(ctx cosmos.Context, k keeper.Keeper, eventMgr EventManager, er *common.ErrataTx) error {
	txOutVoter, err := k.GetObservedTxOutVoter(ctx, er.Id)
	if err != nil {
		return fmt.Errorf("fail to get observed tx out voter for tx (%s) : %w", er.Id, err)
	}
	if len(txOutVoter.Txs) == 0 {
		return fmt.Errorf("cannot find tx: %s", er.Id)
	}
	if txOutVoter.Tx.IsEmpty() {
		return fmt.Errorf("tx out voter is not finalised")
	}
	tx := txOutVoter.Tx.Tx
	if !tx.Chain.Equals(er.Chain) || tx.Coins.IsEmpty() {
		return nil
	}
	vaultPubKey := txOutVoter.Tx.ObservedPubKey
	if !vaultPubKey.IsEmpty() {
		v, err := k.GetVault(ctx, vaultPubKey)
		if err != nil {
			return fmt.Errorf("fail to get vault with pubkey %s: %w", vaultPubKey, err)
		}
		// Credit funds back to the vault so they are not lost.
		// Note: We intentionally do NOT change InactiveVault back to RetiringVault
		// as this could cause side effects (blocking churns, affecting node unbonding).
		// Recovery from InactiveVaults with funds should be handled via migration.
		v.AddFunds(tx.Coins)
		if v.Status == InactiveVault {
			ctx.Logger().Info("Errata credited funds to inactive vault - recovery via migration needed", "vault pub key", v.PubKey)
		}

		if !v.IsEmpty() {
			if err := k.SetVault(ctx, v); err != nil {
				return fmt.Errorf("fail to save vault: %w", err)
			}
		}
	}

	// emit security event
	event := NewEventSecurity(tx, "outbound errata")
	if err := eventMgr.EmitEvent(ctx, event); err != nil {
		return ErrInternal(err, "fail to emit security event")
	}

	txOutVoter.SetReverted()
	k.SetObservedTxOutVoter(ctx, txOutVoter)
	return nil
}
