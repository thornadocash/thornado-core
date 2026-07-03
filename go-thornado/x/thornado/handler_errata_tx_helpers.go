package thornado

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func processErrataTxAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter *keeper.ErrataTxVoter,
	attester cosmos.AccAddress,
	active NodeAccounts,
	er *common.ErrataTx,
	shouldPenalizeForDuplicate bool,
) error {
	k := mgr.Keeper()
	eventMgr := mgr.EventMgr()

	observePenaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, k, constants.Observation_DelayFlexibilityMinutes)

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{ // nolint
		telemetry.NewLabel("reason", "failed_observe_errata"),
		telemetry.NewLabel("chain", string(er.Chain)),
	}))

	penaltyManager := mgr.PenaltyManager()

	if !voter.Sign(attester) {
		// Penalty for the network having to handle the extra message/s.
		if shouldPenalizeForDuplicate {
			penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		}
		ctx.Logger().Info("signer already signed MsgErrataTx", "signer", attester.String(), "txid", er.Id)
		if voter.BlockHeight > 0 || voter.HasConsensus(active) {
			return processErrataState(ctx, k, eventMgr, er)
		}
		return nil
	}

	// doesn't have consensus yet
	if !voter.HasConsensus(active) {
		// Before consensus, penalty until consensus.
		penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		ctx.Logger().Info("not having consensus yet, return")
		return nil
	}

	if voter.BlockHeight > 0 {
		// After consensus, only decrement penalty points if within the Observation_DelayFlexibilityMinutes window.
		if (voter.BlockHeight + observeFlex) >= ctx.BlockHeight() {
			penaltyManager.DecPenaltyPoints(penaltyCtx, lackOfObservationPenalty, attester)
		}
		return processErrataState(ctx, k, eventMgr, er)
	}

	voter.BlockHeight = ctx.BlockHeight()

	// This signer brings the voter to consensus; increment the signer's penalty points like the before-consensus signers,
	// then decrement all the signers' penalty points and increment the non-signers' penalty points.
	penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
	signers := voter.GetSigners()
	nonSigners := getNonSigners(active, signers)
	penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
	penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)

	return processErrataState(ctx, k, eventMgr, er)
}

func processErrataState(ctx cosmos.Context, k keeper.Keeper, eventMgr EventManager, er *common.ErrataTx) error {
	depositErrataProcessed, err := processErrataDepositRecord(ctx, k, er)
	if err != nil {
		return err
	}
	observedVoter, err := k.GetObservedTxInVoter(ctx, er.Id)
	if err != nil {
		return err
	}

	if len(observedVoter.Txs) == 0 {
		processed, err := processErrataOutboundTx(ctx, k, eventMgr, er)
		if err != nil {
			return err
		}
		if !processed && !depositErrataProcessed {
			ctx.Logger().Info("errata tx not found in thornado state; treating as already cleared", "tx_id", er.Id, "chain", er.Chain)
		}
		return nil
	}
	if observedVoter.Tx.IsEmpty() {
		observedVoter.SetReverted()
		k.SetObservedTxInVoter(ctx, observedVoter)
		ctx.Logger().Info("marked non-consensus observed tx errata", "tx_id", er.Id, "chain", er.Chain)
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
			// try to deduct the asset from base
			var vault Vault
			vault, err = k.GetVault(ctx, vaultPubKey)
			if err != nil {
				return fmt.Errorf("fail to get active base vaults: %w", err)
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

func processErrataDepositRecord(ctx cosmos.Context, k keeper.Keeper, er *common.ErrataTx) (bool, error) {
	if er == nil || !er.Chain.Equals(common.BTCChain) || er.Id.IsEmpty() {
		return false, nil
	}
	deposit, err := k.GetDepositRecord(ctx, er.Id)
	if err != nil {
		return false, err
	}
	if deposit.DepositID.IsEmpty() {
		return false, nil
	}
	deposit.Status = types.DepositStatusErrata
	deposit.BTCConfirmations = 0
	deposit.BTCObservedHeight = 0
	if err := k.SetDepositRecord(ctx, deposit); err != nil {
		return false, err
	}
	if err := purgeTxOutItemsForDeposit(ctx, k, deposit.DepositID, types.TxOutTypeSweep, types.TxOutTypeRefund); err != nil {
		return false, err
	}
	if session, err := k.GetDepositSession(ctx, deposit.Owner); err == nil && session.InboundTxID.Equals(er.Id) {
		session.InboundTxID = ""
		session.DepositID = ""
		session.BTCConfirmations = 0
		session.BTCObservedHeight = 0
		if session.Status == types.DepositStatusDepositObserved || session.Status == types.DepositStatusDepositMatched {
			session.Status = types.DepositStatusAddressIssued
		}
		if err := k.SetDepositSession(ctx, session); err != nil {
			return false, err
		}
	}
	ctx.Logger().Info("marked deposit errata", "deposit_id", er.Id, "chain", er.Chain)
	return true, nil
}

// processErrataOutboundTx handles an errata for an outbound tx that was sent but later reorged out.
// It re-credits funds to the vault so they are not abandoned.
// The tx is marked as reverted rather than rescheduled.
func processErrataOutboundTx(ctx cosmos.Context, k keeper.Keeper, eventMgr EventManager, er *common.ErrataTx) (bool, error) {
	txOutVoter, err := k.GetObservedTxOutVoter(ctx, er.Id)
	if err != nil {
		return false, fmt.Errorf("fail to get observed tx out voter for tx (%s) : %w", er.Id, err)
	}
	if len(txOutVoter.Txs) == 0 {
		return false, nil
	}
	if txOutVoter.Reverted {
		return true, nil
	}
	if txOutVoter.Tx.IsEmpty() {
		return true, fmt.Errorf("tx out voter is not finalised")
	}
	tx := txOutVoter.Tx.Tx
	if !tx.Chain.Equals(er.Chain) || tx.Coins.IsEmpty() {
		return true, nil
	}
	vaultPubKey := txOutVoter.Tx.ObservedPubKey
	if !vaultPubKey.IsEmpty() {
		v, err := k.GetVault(ctx, vaultPubKey)
		if err != nil {
			return true, fmt.Errorf("fail to get vault with pubkey %s: %w", vaultPubKey, err)
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
				return true, fmt.Errorf("fail to save vault: %w", err)
			}
		}
	}

	// emit security event
	event := NewEventSecurity(tx, "outbound errata")
	if err := eventMgr.EmitEvent(ctx, event); err != nil {
		return true, ErrInternal(err, "fail to emit security event")
	}

	// If this reorged-out outbound was a shielder redeem payout, re-queue it so the
	// user is paid rather than having a spent note stranded. The nullifier stays
	// spent (the note was legitimately consumed); we only re-issue the BTC send.
	// The recredited vault funds above restore the balance the new outbound spends.
	redeem, found, rerr := k.GetShielderRedeemByOutHash(ctx, er.Id.String())
	if rerr != nil {
		ctx.Logger().Error("fail to look up shielder redeem for errata", "error", rerr, "tx_id", er.Id.String())
	} else if found && redeem.Status == types.ShielderRedeemStatusSettled {
		k.DeleteShielderRedeemOutHash(ctx, er.Id.String())
		redeem.OutHash = ""
		redeem.Status = types.ShielderRedeemStatusAuthorized
		redeem.RequestedHeight = ctx.BlockHeight()
		// Retarget the re-issued outbound at the current active BTC vault, since the
		// original vault may now be retiring/inactive.
		if vault, _, verr := currentBTCVaultAddress(ctx, k); verr == nil {
			redeem.VaultPubKey = vault.PubKey
		}
		// Re-queue without re-booking the withdrawal fee (already collected on the
		// original queue and not refunded on errata), avoiding a double count.
		if _, qerr := ReQueueAuthorizedWithdrawalTxOut(ctx, k, redeem); qerr != nil {
			ctx.Logger().Error("fail to re-queue shielder redeem after errata", "error", qerr, "withdrawal_id", redeem.WithdrawalID)
		} else {
			ctx.Logger().Info("re-queued shielder redeem after outbound errata", "withdrawal_id", redeem.WithdrawalID, "tx_id", er.Id.String())
		}
	}

	txOutVoter.SetReverted()
	k.SetObservedTxOutVoter(ctx, txOutVoter)
	return true, nil
}
