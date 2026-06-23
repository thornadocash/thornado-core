package thornado

import (
	"context"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// processTxInAttestation processes a single attestation for an observed tx.
// This is used by both MsgObservedTxIn (single attester) and MsgObservedTxInQuorum (multiple attesters).
func processTxInAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter ObservedTxVoter,
	nas NodeAccounts,
	tx ObservedTx,
	signer cosmos.AccAddress,
	shouldPenalizeForDuplicate bool,
) (ObservedTxVoter, bool) {
	k := mgr.Keeper()
	penaltyManager := mgr.PenaltyManager()

	observePenaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, k, constants.Observation_DelayFlexibilityMinutes)

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_txin"),
		telemetry.NewLabel("chain", string(tx.Tx.Chain)),
	}))
	penaltyCtx = ctx.WithContext(context.WithValue(penaltyCtx.Context(), constants.CtxObservedTx, tx.Tx.ID.String()))

	ok := false
	if tx.BlockHeight > 0 {
		if err := k.SetLastObserveHeight(ctx, tx.Tx.Chain, signer, tx.BlockHeight); err != nil {
			ctx.Logger().Error("fail to save last observe height", "error", err, "signer", signer, "chain", tx.Tx.Chain)
		}
	}

	// As an observation requires processing by all nodes no matter what,
	// any observation should increment Observation_SubmitPenaltyPoints,
	// to be decremented only if contributing to or within Observation_DelayFlexibilityMinutes of consensus.
	penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, signer)

	if !voter.Add(tx, signer) {
		if !shouldPenalizeForDuplicate {
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signer)
		}
		// A duplicate message, so do nothing further.
		return voter, ok
	}
	if voter.HasFinalised(nas) {
		if voter.FinalisedHeight == 0 {
			ok = true
			voter.Height = ctx.BlockHeight() // Always record the consensus height of the finalised Tx
			if voter.UnfinalizedHeight == 0 {
				voter.UnfinalizedHeight = voter.Height // Preserve first consensus height
			}
			voter.FinalisedHeight = ctx.BlockHeight()
			voter.Tx = *voter.GetTx(nas)

			// This signer brings the voter to consensus;
			// decrement all the signers' penalty points and increment the non-signers' penalty points.
			signers := voter.GetConsensusSigners()
			nonSigners := getNonSigners(nas, signers)
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
			penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)
		} else if ctx.BlockHeight() <= (voter.FinalisedHeight+observeFlex) &&
			voter.Tx.IsFinal() == tx.IsFinal() &&
			voter.Tx.Tx.EqualsEx(tx.Tx) &&
			!voter.Tx.HasSigned(signer) {
			// Track already-decremented penalty points with the consensus Tx's Signers list.
			voter.Tx.Signers = append(voter.Tx.Signers, signer.String())
			// event the tx had been processed , given the signer just a bit late , so still take away their penalty points
			// but only when the tx signer are voting is the tx that already reached consensus
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints+lackOfObservationPenalty, signer)
		}
	}
	if !ok && voter.HasConsensus(nas) && !tx.IsFinal() && voter.FinalisedHeight == 0 {
		if voter.Height == 0 {
			ok = true
			voter.Height = ctx.BlockHeight()
			if voter.UnfinalizedHeight == 0 {
				voter.UnfinalizedHeight = voter.Height // Preserve first consensus height
			}
			// this is the tx that has consensus
			voter.Tx = *voter.GetTx(nas)

			// This signer brings the voter to consensus;
			// decrement all the signers' penalty points and increment the non-signers' penalty points.
			signers := voter.GetConsensusSigners()
			nonSigners := getNonSigners(nas, signers)
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
			penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)
		} else if ctx.BlockHeight() <= (voter.Height+observeFlex) &&
			voter.Tx.IsFinal() == tx.IsFinal() &&
			voter.Tx.Tx.EqualsEx(tx.Tx) &&
			!voter.Tx.HasSigned(signer) {
			// Track already-decremented penalty points with the consensus Tx's Signers list.
			voter.Tx.Signers = append(voter.Tx.Signers, signer.String())
			// event the tx had been processed , given the signer just a bit late , so still take away their penalty points
			// but only when the tx signer are voting is the tx that already reached consensus
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints+lackOfObservationPenalty, signer)
		}
	}

	k.SetObservedTxInVoter(ctx, voter)

	// Check to see if we have enough identical observations to process the transaction
	return voter, ok
}

// ensureVaultAndGetTxInVoter will make sure the vault exists, then get the ObservedTxInVoter from the store.
// if it doesn't exist, it will create a new one.
func ensureVaultAndGetTxInVoter(ctx cosmos.Context, vaultPubKey common.PubKey, txID common.TxID, k keeper.Keeper) (ObservedTxVoter, error) {
	// check we are sending to a valid vault
	if !k.VaultExists(ctx, vaultPubKey) {
		ctx.Logger().Info("Not valid Observed Pubkey", "observed pub key", vaultPubKey)
		return ObservedTxVoter{}, fmt.Errorf("vault not found for observed tx in pubkey: %s", vaultPubKey)
	}

	voter, err := k.GetObservedTxInVoter(ctx, txID)
	if err != nil {
		return ObservedTxVoter{}, fmt.Errorf("fail to get tx in voter: %w", err)
	}

	return voter, nil
}

// handleObservedTxInQuorum - will process the observed tx in quorum.
// used by both MsgObservedTxIn and MsgObservedTxInQuorum after processing
// attestation(s).
func handleObservedTxInQuorum(
	ctx cosmos.Context,
	mgr Manager,
	signer cosmos.AccAddress,
	activeNodeAccounts NodeAccounts,
	handler cosmos.Handler,
	tx common.ObservedTx,
	voter ObservedTxVoter,
	observers []cosmos.AccAddress,
	isQuorum bool,
) error {
	if !isQuorum {
		if voter.Height == ctx.BlockHeight() || voter.FinalisedHeight == ctx.BlockHeight() {
			// we've already process the transaction, but we should still
			// update the observing addresses
			mgr.ObMgr().AppendObserver(tx.Tx.Chain, observers)
		}
		return nil
	}

	// all logic after this is upon consensus

	if voter.Reverted {
		ctx.Logger().Info("tx had been reverted", "Tx", tx.String())
		return nil
	}

	k := mgr.Keeper()

	vault, err := k.GetVault(ctx, tx.ObservedPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault", "error", err)
		return nil
	}

	hasFinalised := voter.HasFinalised(activeNodeAccounts)

	if hasFinalised {
		if vault.IsBase() && !voter.UpdatedVault {
			if !tx.Tx.FromAddress.Equals(tx.Tx.ToAddress) {
				vault.AddFunds(tx.Tx.Coins)
				vault.InboundTxCount++
			}
			voter.UpdatedVault = true
		}
	}
	if creditBTCMigrationInboundDestination(ctx, mgr, &vault, &voter, tx) {
		ctx.Logger().Info("credited BTC migration destination vault",
			"vault", vault.PubKey.String(),
			"tx_id", tx.Tx.ID.String(),
			"coins", tx.Tx.Coins.String(),
		)
	}
	if tx.BlockHeight > 0 {
		if err = k.SetLastChainHeight(ctx, tx.Tx.Chain, tx.BlockHeight); err != nil {
			ctx.Logger().Error("fail to set last chain height", "error", err)
		}
	}

	// save the changes in Tx Voter to key value store
	k.SetObservedTxInVoter(ctx, voter)
	if err = k.SetVault(ctx, vault); err != nil {
		ctx.Logger().Error("fail to set vault", "error", err)
		return nil
	}

	if !vault.IsBase() {
		ctx.Logger().Info("Vault is not an Base vault, transaction ignored.")
		return nil
	}

	mgr.ObMgr().AppendObserver(tx.Tx.Chain, voter.Tx.GetSigners())

	if err := RecordDepositObservation(ctx, k, voter.Tx, hasFinalised); err != nil {
		ctx.Logger().Error("fail to record deposit observation", "error", err, "tx", voter.TxID)
	}

	if !hasFinalised {
		ctx.Logger().Info("transaction pending confirmation counting", "hash", voter.TxID)
		return nil
	}

	ctx.Logger().Debug("tx in finalized and has consensus",
		"id", tx.Tx.ID.String(),
		"chain", tx.Tx.Chain.String(),
		"height", tx.BlockHeight,
		"from", tx.Tx.FromAddress.String(),
		"to", tx.Tx.ToAddress.String(),
		"coins", tx.Tx.Coins.String(),
		"gas", common.Coins(tx.Tx.Gas).String(),
		"observed_vault_pubkey", tx.ObservedPubKey.String(),
	)

	if deposit, matchErr := MatchCoreDeposit(ctx, mgr, voter.Tx); matchErr == nil {
		voter.SetDone()
		k.SetObservedTxInVoter(ctx, voter)
		ctx.Logger().Info("deposit matched",
			"tx_id", deposit.DepositID.String(),
			"owner", deposit.Owner.String(),
			"amount_sats", deposit.AmountSats,
			"deposit_address", deposit.DepositAddress.String(),
		)
		return nil
	}

	if tx.Tx.Chain.Equals(common.BTCChain) {
		rootAddr, rootErr := common.DeriveBTCTaprootAddress(tx.ObservedPubKey, common.MainVaultPathIndex)
		currentVault, _, currentErr := currentBTCVaultAddress(ctx, k)
		if rootErr == nil && currentErr == nil && tx.Tx.ToAddress.Equals(rootAddr) && !tx.ObservedPubKey.Equals(currentVault.PubKey) {
			if err := queueVaultPathSweep(ctx, mgr, voter.Tx, tx.ObservedPubKey, common.MainVaultPathIndex); err != nil {
				ctx.Logger().Error("fail to queue old root vault sweep", "error", err, "tx", tx.String())
			} else {
				voter.SetDone()
				k.SetObservedTxInVoter(ctx, voter)
				return nil
			}
		}
	}

	if vault.Status == InactiveVault {
		ctx.Logger().Error("observed tx on inactive vault", "tx", tx.String())
		if tx.Tx.Chain.Equals(common.BTCChain) {
			if err := queueVaultPathSweep(ctx, mgr, voter.Tx, tx.ObservedPubKey, common.MainVaultPathIndex); err != nil {
				ctx.Logger().Error("fail to queue inactive vault sweep", "error", err, "tx", tx.String())
			} else {
				voter.SetDone()
				k.SetObservedTxInVoter(ctx, voter)
			}
		}
		return nil
	}

	ctx.Logger().Info("observed inbound did not match a registered deposit", "chain", tx.Tx.Chain, "id", tx.Tx.ID)

	voter.SetDone()
	k.SetObservedTxInVoter(ctx, voter)

	return nil
}

// processTxOutAttestation processes a single attestation for an observed tx.
// This is used by both MsgObservedTxOut (single attester) and MsgObservedTxOutQuorum (multiple attesters).
func processTxOutAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter ObservedTxVoter,
	nas NodeAccounts,
	tx ObservedTx,
	signer cosmos.AccAddress,
	shouldPenalizeForDuplicate bool,
) (ObservedTxVoter, bool) {
	k := mgr.Keeper()
	penaltyManager := mgr.PenaltyManager()

	observePenaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, k, constants.Observation_DelayFlexibilityMinutes)
	ok := false

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_txout"),
		telemetry.NewLabel("chain", string(tx.Tx.Chain)),
	}))
	penaltyCtx = ctx.WithContext(context.WithValue(penaltyCtx.Context(), constants.CtxObservedTx, tx.Tx.ID.String()))

	if tx.BlockHeight > 0 {
		if err := k.SetLastObserveHeight(ctx, tx.Tx.Chain, signer, tx.BlockHeight); err != nil {
			ctx.Logger().Error("fail to save last observe height", "error", err, "signer", signer, "chain", tx.Tx.Chain)
		}
	}

	// As an observation requires processing by all nodes no matter what,
	// any observation should increment Observation_SubmitPenaltyPoints,
	// to be decremented only if contributing to or within Observation_DelayFlexibilityMinutes of consensus.
	penaltyManager.IncPenaltyPoints(penaltyCtx, observePenaltyPoints, signer)

	if !voter.Add(tx, signer) {
		if !shouldPenalizeForDuplicate {
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signer)
		}
		// A duplicate message, so do nothing further.
		return voter, ok
	}

	if voter.HasFinalised(nas) {
		if voter.FinalisedHeight == 0 {
			if voter.Height == 0 {
				ok = true
				// Record the consensus height at which outbound consensus actions are taken.
				voter.Height = ctx.BlockHeight()
				if voter.UnfinalizedHeight == 0 {
					voter.UnfinalizedHeight = voter.Height // Preserve first consensus height
				}
			}
			voter.FinalisedHeight = ctx.BlockHeight()
			voter.Tx = *voter.GetTx(nas)

			ctx.Logger().Debug("tx out finalized and has consensus",
				"id", tx.Tx.ID.String(),
				"chain", tx.Tx.Chain.String(),
				"height", tx.BlockHeight,
				"from", tx.Tx.FromAddress.String(),
				"to", tx.Tx.ToAddress.String(),
				"coins", tx.Tx.Coins.String(),
				"gas", common.Coins(tx.Tx.Gas).String(),
				"observed_vault_pubkey", tx.ObservedPubKey.String(),
			)

			// This signer brings the voter to consensus;
			// decrement all the signers' penalty points and increment the non-signers' penalty points.
			signers := voter.GetConsensusSigners()
			nonSigners := getNonSigners(nas, signers)
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
			penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)
		} else if ctx.BlockHeight() <= (voter.FinalisedHeight+observeFlex) &&
			voter.Tx.IsFinal() == tx.IsFinal() &&
			voter.Tx.Tx.EqualsEx(tx.Tx) &&
			!voter.Tx.HasSigned(signer) {
			// Track already-decremented penalty points with the consensus Tx's Signers list.
			voter.Tx.Signers = append(voter.Tx.Signers, signer.String())
			// event the tx had been processed , given the signer just a bit late , so we still take away their penalty points
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints+lackOfObservationPenalty, signer)
		}

		// Gas correction for re-org re-observations.
		// When a voter is finalized but a re-observation has the same tx except gas
		// (likely due to a reorg changing the effective gas price), check if enough
		// nodes agree on the corrected gas to update vault accounting.
		if voter.FinalisedHeight > 0 &&
			voter.Tx.IsFinal() == tx.IsFinal() &&
			!voter.Tx.Tx.Gas.Equals(tx.Tx.Gas) &&
			voter.Tx.Tx.EqualsExIgnoreGas(tx.Tx) {
			gasCorrectionSigners := countMatchingSigners(voter, tx, nas)
			if HasSuperMajority(gasCorrectionSigners, len(nas)) {
				correctOutboundGas(ctx, mgr, voter.Tx, tx)
				voter.Tx.Tx.Gas = tx.Tx.Gas
			}
		}
	}
	if !ok && voter.HasConsensus(nas) && !tx.IsFinal() && voter.FinalisedHeight == 0 {
		if voter.Height == 0 {
			ok = true
			// Record the consensus height at which outbound consensus actions are taken,
			// even if not yet Finalised.
			voter.Height = ctx.BlockHeight()
			if voter.UnfinalizedHeight == 0 {
				voter.UnfinalizedHeight = voter.Height // Preserve first consensus height
			}
			// this is the tx that has consensus
			voter.Tx = *voter.GetTx(nas)

			// This signer brings the voter to consensus;
			// decrement all the signers' penalty points and increment the non-signers' penalty points.
			signers := voter.GetConsensusSigners()
			nonSigners := getNonSigners(nas, signers)
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
			penaltyManager.IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)
		} else if ctx.BlockHeight() <= (voter.Height+observeFlex) &&
			voter.Tx.IsFinal() == tx.IsFinal() &&
			voter.Tx.Tx.EqualsEx(tx.Tx) &&
			!voter.Tx.HasSigned(signer) {
			// Track already-decremented penalty points with the consensus Tx's Signers list.
			voter.Tx.Signers = append(voter.Tx.Signers, signer.String())
			// event the tx had been processed , given the signer just a bit late , so still take away their penalty points
			// but only when the tx signer are voting is the tx that already reached consensus
			penaltyManager.DecPenaltyPoints(penaltyCtx, observePenaltyPoints+lackOfObservationPenalty, signer)
		}

		// Gas correction for re-org re-observations (non-finalized consensus path).
		if voter.Height > 0 &&
			!voter.Tx.Tx.Gas.Equals(tx.Tx.Gas) &&
			voter.Tx.Tx.EqualsExIgnoreGas(tx.Tx) {
			gasCorrectionSigners := countMatchingSigners(voter, tx, nas)
			if HasSuperMajority(gasCorrectionSigners, len(nas)) {
				correctOutboundGas(ctx, mgr, voter.Tx, tx)
				voter.Tx.Tx.Gas = tx.Tx.Gas
			}
		}
	}

	k.SetObservedTxOutVoter(ctx, voter)

	// Check to see if we have enough identical observations to process the transaction
	return voter, ok
}

// countMatchingSigners counts the number of unique active node signers that have observed
// the exact same tx (including gas) as the given observation.
func countMatchingSigners(voter ObservedTxVoter, tx ObservedTx, nas NodeAccounts) int {
	signers := make(map[string]struct{})
	for _, otherTx := range voter.Txs {
		if otherTx.IsFinal() != tx.IsFinal() {
			continue
		}
		if !otherTx.Tx.EqualsEx(tx.Tx) {
			continue
		}
		for _, s := range otherTx.GetSigners() {
			if !nas.IsNodeKeys(s) {
				continue
			}
			signers[s.String()] = struct{}{}
		}
	}
	return len(signers)
}

// correctOutboundGas adjusts a vault's gas accounting when a reorg results in
// a different gas amount for the same outbound transaction. It computes the delta
// between the original consensus gas and the corrected gas, then adjusts the vault
// balance and gas manager accordingly.
func correctOutboundGas(ctx cosmos.Context, mgr Manager, consensusTx common.ObservedTx, correctedTx ObservedTx) {
	oldGas := consensusTx.Tx.Gas
	newGas := correctedTx.Tx.Gas

	vault, err := mgr.Keeper().GetVault(ctx, correctedTx.ObservedPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault for gas correction", "error", err)
		return
	}

	// Calculate gas delta per asset and adjust vault balance.
	var additionalGas common.Gas
	for _, newGasCoin := range newGas {
		oldAmount := oldGas.ToCoins().GetCoin(newGasCoin.Asset).Amount
		if newGasCoin.Amount.GT(oldAmount) {
			// More gas was actually spent - deduct additional from vault.
			delta := common.NewCoin(newGasCoin.Asset, newGasCoin.Amount.Sub(oldAmount))
			vault.SubFunds(common.NewCoins(delta))
			additionalGas = additionalGas.Add(delta)
		} else if oldAmount.GT(newGasCoin.Amount) {
			// Less gas was actually spent - credit back to vault.
			delta := common.NewCoin(newGasCoin.Asset, oldAmount.Sub(newGasCoin.Amount))
			vault.AddFunds(common.NewCoins(delta))
		}
	}

	if err := mgr.Keeper().SetVault(ctx, vault); err != nil {
		ctx.Logger().Error("fail to save vault after gas correction", "error", err)
		return
	}

	// If more gas was spent, add the delta to the gas manager for reserve reimbursement,
	// skipping reimbursement for inactive vaults.
	if !additionalGas.IsEmpty() {
		if vault.Status == InactiveVault {
			return
		}
		outAsset := common.EmptyAsset
		if len(correctedTx.Tx.Coins) != 0 {
			outAsset = correctedTx.Tx.Coins[0].Asset
		}
		mgr.GasMgr().AddGasAsset(outAsset, additionalGas, false)
	}

	ctx.Logger().Info("corrected outbound gas after reorg",
		"txid", correctedTx.Tx.ID,
		"old_gas", common.Coins(oldGas).String(),
		"new_gas", common.Coins(newGas).String(),
	)
}

// ensureVaultAndGetTxOutVoter will make sure the vault exists, then get the ObservedTxOutVoter from the store.
// if it doesn't exist, it will create a new one.
func ensureVaultAndGetTxOutVoter(ctx cosmos.Context, k keeper.Keeper, vaultPubKey common.PubKey, txID common.TxID, observers []cosmos.AccAddress, keysignMs int64) (ObservedTxVoter, error) {
	// check we are sending from a valid vault
	if !k.VaultExists(ctx, vaultPubKey) {
		ctx.Logger().Info("Not valid Observed Pubkey", "observed pub key", vaultPubKey)
		return ObservedTxVoter{}, fmt.Errorf("vault not found for observed tx out pubkey: %s", vaultPubKey)
	}

	if keysignMs > 0 {
		keysignMetric, err := k.GetFrostKeysignMetric(ctx, txID)
		if err != nil {
			ctx.Logger().Error("fail to get keysign metric", "error", err)
		} else {
			for _, o := range observers {
				keysignMetric.AddNodeFrostTime(o, keysignMs)
			}
			k.SetFrostKeysignMetric(ctx, keysignMetric)
		}
	}

	voter, err := k.GetObservedTxOutVoter(ctx, txID)
	if err != nil {
		return ObservedTxVoter{}, fmt.Errorf("fail to get tx out voter: %w", err)
	}

	return voter, nil
}

// handleObservedTxOutQuorum - will process the observed tx out quorum.
// used by both MsgObservedTxOut and MsgObservedTxOutQuorum after processing
// attestation(s).
func handleObservedTxOutQuorum(
	ctx cosmos.Context,
	mgr Manager,
	signer cosmos.AccAddress,
	activeNodeAccounts NodeAccounts,
	handler cosmos.Handler,
	tx common.ObservedTx,
	voter ObservedTxVoter,
	observers []cosmos.AccAddress,
	isQuorum bool,
) error {
	// check whether the tx has consensus
	if !isQuorum {
		if voter.Height == ctx.BlockHeight() || voter.FinalisedHeight == ctx.BlockHeight() {
			// we've already process the transaction, but we should still
			// update the observing addresses
			mgr.ObMgr().AppendObserver(tx.Tx.Chain, observers)
		}
		if voter.FinalisedHeight > 0 {
			_ = markObservedOutboundTxOut(ctx, mgr, txForOutboundReplayMatch(voter, tx))
		} else if tx.IsFinal() {
			_ = markObservedOutboundTxOut(ctx, mgr, tx)
		}
		return nil
	}

	k := mgr.Keeper()

	if isCancelOrApprovalTx(tx) {
		ctx.Logger().Info("skipping penalty for cancel tx", "txid", tx.Tx.ID)
		// Credit gas to gas manager and deduct from vault (legit operational spend, no penalty)
		// This also adds to fee_spent_rune for dynamic outbound fee calculation
		if err := addGasFees(ctx, mgr, tx); err != nil {
			ctx.Logger().Error("fail to add gas fees for cancel tx", "error", err)
		}
		return nil
	}

	vaultPenalty := false
	txOut := voter.GetTx(activeNodeAccounts) // get consensus tx, in case our for loop is incorrect
	if txOut == nil || txOut.IsEmpty() {
		ctx.Logger().Error("fail to get consensus tx from voter", "txid", tx.Tx.ID)
	} else {
		if tx.Tx.Chain.IsEmpty() {
			ctx.Logger().Error("fail to process txOut: chain is empty", "tx", tx.Tx.String())
		} else {
			mgr.ObMgr().AppendObserver(tx.Tx.Chain, txOut.GetSigners())

			if tx.KeysignMs > 0 {
				keysignMetric, kmErr := k.GetFrostKeysignMetric(ctx, tx.Tx.ID)
				if kmErr != nil {
					ctx.Logger().Error("fail to get frost keysign metric", "error", kmErr, "hash", tx.Tx.ID)
				} else {
					evt := NewEventFrostKeysignMetric(keysignMetric.TxID, keysignMetric.GetMedianTime())
					if emitErr := mgr.EventMgr().EmitEvent(ctx, evt); emitErr != nil {
						ctx.Logger().Error("fail to emit frost metric event", "error", emitErr)
					}
				}
			}
		}
	}

	matchedTxOut, newlyMatchedTxOut := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	migrationSourceSettled := false
	if matchedTxOut {
		migrationSourceSettled = settleBTCMigrationSourceVault(ctx, mgr, tx)
	}
	if shouldSkipAlreadyProcessedObservedOutbound(voter, matchedTxOut, newlyMatchedTxOut, tx) {
		ctx.Logger().Debug("observed outbound already matched to txout",
			"tx_id", tx.Tx.ID.String(),
			"chain", tx.Tx.Chain.String(),
			"pubkey", tx.ObservedPubKey.String(),
		)
		return nil
	}
	if !matchedTxOut && observedOutboundRequiresTxOutMatch(tx) {
		ctx.Logger().Info("halt BTC vault, observed outbound did not match an open txout",
			"tx_id", tx.Tx.ID.String(),
			"chain", tx.Tx.Chain.String(),
			"from", tx.Tx.FromAddress.String(),
			"to", tx.Tx.ToAddress.String(),
			"pubkey", tx.ObservedPubKey.String(),
		)
		if err := haltBTCVaultForIssue(ctx, mgr.Keeper(), mgr.EventMgr(), tx.Tx, "observed outbound without open txout"); err != nil {
			return fmt.Errorf("fail to halt BTC vault: %w", err)
		}
	}

	// Vault accounting - always runs once because outbound is irrevocable.
	// Gas and coin deductions must happen regardless of handler success.

	// only deduct gas fee via the manager if there was not a vault penalty that covered it
	if !vaultPenalty && !migrationSourceSettled {
		if err := addGasFees(ctx, mgr, tx); err != nil {
			ctx.Logger().Error("fail to add gas fee", "error", err)
		}
	}

	// If sending from one of our vaults, decrement coins
	vault, err := k.GetVault(ctx, tx.ObservedPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault", "error", err)
	} else if !migrationSourceSettled {
		// if the vault was penalized we skipped the gas manager above and deduct gas directly
		if vaultPenalty {
			vault.SubFunds(tx.Tx.Gas.ToCoins())
		}

		// Don't add to or subtract from vault balances when the sender and recipient are the same
		// (particularly avoid Consolidate SafeSub zeroing of vault balances).
		// Child deposit sweeps are internal to the same vault key, so only gas leaves the vault.
		if !tx.Tx.FromAddress.Equals(tx.Tx.ToAddress) && !isBTCChildSweepToRoot(tx, vault.PubKey) {
			// skip deducting funds for outbound fake gas
			if !isOutboundFakeGasTx(tx) {
				vault.SubFunds(tx.Tx.Coins)
			}
			vault.OutboundTxCount++
		}

		if !vault.HasFunds() && vault.Status == RetiringVault {
			// we have successfully removed all funds from a retiring vault,
			// mark it as inactive
			vault.UpdateStatus(InactiveVault, ctx.BlockHeight())
		}
		// if the vault is frozen, then unfreeze it. Since we saw that a
		// transaction was signed
		for _, coin := range tx.Tx.Coins {
			for i := range vault.Frozen {
				if strings.EqualFold(coin.Asset.GetChain().String(), vault.Frozen[i]) {
					vault.Frozen = append(vault.Frozen[:i], vault.Frozen[i+1:]...)
					break
				}
			}
		}
		if err := k.SetVault(ctx, vault); err != nil {
			ctx.Logger().Error("fail to save vault", "error", err)
		}
	}

	// Mark voter as done AFTER all vault operations to prevent
	// inconsistency if vault ops fail.
	voter.SetDone()
	k.SetObservedTxOutVoter(ctx, voter)

	ctx.Logger().Info("tx out processed", "chain", tx.Tx.Chain, "id", tx.Tx.ID, "finalized", tx.IsFinal())

	return nil
}

func creditBTCMigrationDestination(ctx cosmos.Context, mgr Manager, tx ObservedTx) {
	// BTC vault-to-vault migrations are observed twice: as an inbound to the
	// destination vault and as an outbound from the source vault. Destination
	// accounting is owned by the inbound path; outbound handling only settles
	// the source vault. Keeping this as a no-op preserves old test call sites
	// while preventing double credits.
}

func creditBTCMigrationInboundDestination(ctx cosmos.Context, mgr Manager, vault *Vault, voter *ObservedTxVoter, tx ObservedTx) bool {
	if !tx.Tx.Chain.Equals(common.BTCChain) ||
		!vault.IsBase() ||
		voter.UpdatedVault {
		return false
	}
	if !observedBTCMigrationInbound(ctx, mgr.Keeper(), tx) {
		return false
	}
	if !tx.Tx.FromAddress.Equals(tx.Tx.ToAddress) {
		vault.AddFunds(tx.Tx.Coins)
		vault.InboundTxCount++
	}
	voter.UpdatedVault = true
	return true
}

func observedBTCMigrationInbound(ctx cosmos.Context, k keeper.Keeper, tx ObservedTx) bool {
	signingPeriod := getConfigDurationBlocks(ctx, k, constants.Keysign_PeriodMinutes)
	earliestHeight := ctx.BlockHeight() - signingPeriod
	if earliestHeight < 1 {
		earliestHeight = 1
	}
	for height := ctx.BlockHeight(); height >= earliestHeight; height-- {
		txOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			continue
		}
		for _, item := range txOut.TxArray {
			if item.TxType == types.TxOutTypeMigrate &&
				!item.OutHash.IsEmpty() &&
				item.OutHash.Equals(tx.Tx.ID) &&
				item.ToAddress.Equals(tx.Tx.ToAddress) &&
				item.Coin.Asset.Equals(common.BTCAsset) {
				return true
			}
		}
	}
	return false
}

func settleBTCMigrationSourceVault(ctx cosmos.Context, mgr Manager, tx ObservedTx) bool {
	if !tx.Tx.Chain.Equals(common.BTCChain) || tx.Tx.ID.IsEmpty() {
		return false
	}
	if !tx.IsFinal() {
		return false
	}
	item, txOutHeight, ok := findMatchingBTCMigrationTxOut(ctx, mgr.Keeper(), tx)
	if !ok {
		return false
	}
	return settleBTCMigrationSourceVaultItem(ctx, mgr, txOutHeight, item)
}

func findMatchingBTCMigrationTxOut(ctx cosmos.Context, k keeper.Keeper, tx ObservedTx) (TxOutItem, int64, bool) {
	signingPeriod := getConfigDurationBlocks(ctx, k, constants.Keysign_PeriodMinutes)
	earliestHeight := ctx.BlockHeight() - signingPeriod
	if earliestHeight < 1 {
		earliestHeight = 1
	}
	for height := ctx.BlockHeight(); height >= earliestHeight; height-- {
		txOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			continue
		}
		for _, item := range txOut.TxArray {
			if item.TxType != types.TxOutTypeMigrate ||
				item.OutHash.IsEmpty() ||
				!item.OutHash.Equals(tx.Tx.ID) ||
				!observedOutboundMatchesSettledTxOut(tx, item) {
				continue
			}
			return item, txOut.Height, true
		}
	}
	return TxOutItem{}, 0, false
}

func settleBTCMigrationSourceVaultItem(ctx cosmos.Context, mgr Manager, txOutHeight int64, item TxOutItem) bool {
	if item.TxType != types.TxOutTypeMigrate || item.VaultPubKey.IsEmpty() {
		return false
	}
	if len(item.SourceInputs) == 0 {
		return false
	}
	spent := cosmos.ZeroUint()
	for _, source := range item.SourceInputs {
		spent = spent.Add(cosmos.NewUint(uint64(source.AmountSats)))
	}
	if spent.IsZero() {
		return false
	}
	vault, err := mgr.Keeper().GetVault(ctx, item.VaultPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get BTC migration source vault", "error", err, "vault", item.VaultPubKey.String(), "tx_id", item.OutHash.String())
		return false
	}
	if !vaultHasPendingTxHeight(vault, txOutHeight) {
		return false
	}
	vault.SubFunds(common.NewCoins(common.NewCoin(common.BTCAsset, spent)))
	vault.RemovePendingTxBlockHeights(txOutHeight)
	if !vault.HasFunds() && vault.Status == RetiringVault {
		vault.UpdateStatus(InactiveVault, ctx.BlockHeight())
	}
	if err := mgr.Keeper().SetVault(ctx, vault); err != nil {
		ctx.Logger().Error("fail to settle BTC migration source vault", "error", err, "vault", item.VaultPubKey.String(), "tx_id", item.OutHash.String())
		return false
	}
	ctx.Logger().Info("settled BTC migration source vault",
		"vault", item.VaultPubKey.String(),
		"tx_id", item.OutHash.String(),
		"spent", spent.String(),
		"txout_height", txOutHeight,
	)
	return true
}

func vaultHasPendingTxHeight(vault Vault, height int64) bool {
	for _, pendingHeight := range vault.PendingTxBlockHeights {
		if pendingHeight == height {
			return true
		}
	}
	return false
}

func observedTxMatchesTxOutType(ctx cosmos.Context, k keeper.Keeper, tx ObservedTx, txType string) bool {
	signingPeriod := getConfigDurationBlocks(ctx, k, constants.Keysign_PeriodMinutes)
	earliestHeight := ctx.BlockHeight() - signingPeriod
	if earliestHeight < 1 {
		earliestHeight = 1
	}
	for height := ctx.BlockHeight(); height >= earliestHeight; height-- {
		txOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			continue
		}
		for _, item := range txOut.TxArray {
			if item.GetTxType() != txType {
				continue
			}
			if item.OutHash.IsEmpty() && observedOutboundMatchesTxOut(tx, item) {
				return true
			}
			if item.OutHash.Equals(tx.Tx.ID) && observedOutboundMatchesSettledTxOut(tx, item) {
				return true
			}
		}
	}
	return false
}

func shouldSkipAlreadyProcessedObservedOutbound(voter ObservedTxVoter, matchedTxOut, newlyMatchedTxOut bool, tx ObservedTx) bool {
	if !matchedTxOut || newlyMatchedTxOut || !observedOutboundRequiresTxOutMatch(tx) {
		return false
	}
	if voter.Tx.Status == common.Status_done {
		return true
	}
	for _, observed := range voter.Txs {
		if observed.Status == common.Status_done {
			return true
		}
	}
	return false
}

func txForOutboundReplayMatch(voter ObservedTxVoter, tx ObservedTx) ObservedTx {
	if tx.IsFinal() &&
		len(tx.Tx.SourceInputs) > 0 &&
		voter.Tx.IsFinal() == tx.IsFinal() &&
		voter.Tx.ObservedPubKey.Equals(tx.ObservedPubKey) &&
		voter.Tx.Tx.EqualsEx(tx.Tx) {
		return tx
	}
	return voter.Tx
}

func markObservedOutboundTxOut(ctx cosmos.Context, mgr Manager, tx ObservedTx) bool {
	matched, _ := markObservedOutboundTxOutStatus(ctx, mgr, tx)
	return matched
}

func markObservedOutboundTxOutStatus(ctx cosmos.Context, mgr Manager, tx ObservedTx) (bool, bool) {
	signingPeriod := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Keysign_PeriodMinutes)
	earliestHeight := ctx.BlockHeight() - signingPeriod
	if earliestHeight < 1 {
		earliestHeight = 1
	}
	for height := ctx.BlockHeight(); height >= earliestHeight; height-- {
		txOut, err := mgr.Keeper().GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Debug("unable to get txOut record", "error", err, "height", height)
			continue
		}
		if observedOutboundAlreadyMatchedTxOut(txOut, tx) {
			return true, false
		}
		if markObservedOutboundTxOutBatch(ctx, mgr, txOut, tx) {
			return true, true
		}
		for i, item := range txOut.TxArray {
			if !observedOutboundMatchesTxOut(tx, item) {
				ctx.Logger().Info("observed outbound did not match txout item",
					"height", height,
					"tx_id", tx.Tx.ID.String(),
					"tx_chain", tx.Tx.Chain.String(),
					"tx_to", tx.Tx.ToAddress.String(),
					"tx_from", tx.Tx.FromAddress.String(),
					"tx_pubkey", tx.ObservedPubKey.String(),
					"tx_coins", tx.Tx.Coins.String(),
					"tx_gas", common.Coins(tx.Tx.Gas).String(),
					"tx_source_inputs", tx.Tx.SourceInputs,
					"item_chain", item.Chain.String(),
					"item_to", item.ToAddress.String(),
					"item_pubkey", item.VaultPubKey.String(),
					"item_coin", item.Coin.String(),
					"item_max_gas", common.Coins(item.MaxGas).String(),
					"item_out_hash", item.OutHash.String(),
					"item_tx_type", item.TxType,
					"item_vault_path_index", item.VaultPathIndex,
					"item_source_inputs", item.SourceInputs,
				)
				continue
			}
			ctx.Logger().Info("matched observed outbound to txout item", "height", height, "tx_id", tx.Tx.ID.String(), "in_hash", item.InHash.String())
			txOut.TxArray[i].OutHash = tx.Tx.ID
			txOut.TxArray[i].OutVout = 0
			if tx.Tx.Chain.Equals(common.BTCChain) && types.IsInternalTxOutType(item.TxType) && item.Coin.Asset.Equals(common.BTCAsset) {
				observedCoin := tx.Tx.Coins.GetCoin(common.BTCAsset)
				if !observedCoin.IsEmpty() && !observedCoin.Amount.IsZero() {
					txOut.TxArray[i].Coin = observedCoin
				}
			}
			settledItem := txOut.TxArray[i]
			if err := mgr.Keeper().SetTxOut(ctx, txOut); err != nil {
				ctx.Logger().Error("fail to save tx out", "error", err)
			}
			markMatchedTxOutItemSettled(ctx, mgr, settledItem, tx)
			return true, true
		}
	}
	return false, false
}

func observedOutboundRequiresTxOutMatch(tx ObservedTx) bool {
	if !tx.Tx.Chain.Equals(common.BTCChain) {
		return false
	}
	if tx.ObservedPubKey.IsEmpty() || isOutboundFakeGasTx(tx) || isCancelOrApprovalTx(tx) {
		return false
	}
	return true
}

func markObservedOutboundTxOutBatch(ctx cosmos.Context, mgr Manager, txOut *TxOut, tx ObservedTx) bool {
	if txOut == nil {
		return false
	}
	if !tx.Tx.Chain.Equals(common.BTCChain) || len(txOut.TxArray) < 2 {
		return false
	}
	rootAddr, err := common.DeriveBTCTaprootAddress(tx.ObservedPubKey, common.MainVaultPathIndex)
	if err != nil || !tx.Tx.FromAddress.Equals(rootAddr) {
		return false
	}
	observedAmount := tx.Tx.Coins.GetCoin(common.BTCAsset).Amount
	total := cosmos.ZeroUint()
	matched := 0
	for _, item := range txOut.TxArray {
		if !item.OutHash.IsEmpty() ||
			!item.Chain.Equals(common.BTCChain) ||
			!item.VaultPubKey.Equals(tx.ObservedPubKey) ||
			item.VaultPathIndex != common.MainVaultPathIndex ||
			!types.IsBatchableTxOutType(item.TxType) ||
			!item.Coin.Asset.Equals(common.BTCAsset) {
			return false
		}
		total = total.Add(item.Coin.Amount)
		matched++
	}
	if matched < 2 || !total.Equal(observedAmount) {
		return false
	}
	for i := range txOut.TxArray {
		txOut.TxArray[i].OutHash = tx.Tx.ID
		txOut.TxArray[i].OutVout = uint32(i)
	}
	if err := mgr.Keeper().SetTxOut(ctx, txOut); err != nil {
		ctx.Logger().Error("fail to save batched tx out", "error", err)
		return false
	}
	for _, item := range txOut.TxArray {
		markMatchedTxOutItemSettled(ctx, mgr, item, tx)
	}
	ctx.Logger().Info("matched observed outbound to txout batch",
		"height", txOut.Height,
		"tx_id", tx.Tx.ID.String(),
		"items", matched,
	)
	return true
}

func observedOutboundAlreadyMatchedTxOut(txOut *TxOut, tx ObservedTx) bool {
	if observedOutboundAlreadyMatchedTxOutBatch(txOut, tx) {
		return true
	}
	if txOut == nil || tx.Tx.ID.IsEmpty() {
		return false
	}
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() || !item.OutHash.Equals(tx.Tx.ID) {
			continue
		}
		if observedOutboundMatchesSettledTxOut(tx, item) {
			return true
		}
		candidate := item
		candidate.OutHash = common.TxID("")
		if observedOutboundMatchesTxOut(tx, candidate) {
			return true
		}
	}
	return false
}

func observedOutboundMatchesSettledTxOut(tx ObservedTx, item TxOutItem) bool {
	if item.OutHash.IsEmpty() ||
		!item.OutHash.Equals(tx.Tx.ID) ||
		!tx.Tx.Chain.Equals(item.Chain) ||
		!tx.Tx.ToAddress.Equals(item.ToAddress) ||
		!tx.ObservedPubKey.Equals(item.VaultPubKey) {
		return false
	}
	if tx.Tx.Chain.Equals(common.BTCChain) && types.IsInternalTxOutType(item.TxType) {
		if len(item.SourceInputs) > 0 && !observedTxSpentTxOutInputs(tx.Tx.SourceInputs, item.SourceInputs) {
			return false
		}
		sourceAddr, err := common.DeriveBTCTaprootAddress(item.VaultPubKey, item.VaultPathIndex)
		if err != nil || !tx.Tx.FromAddress.Equals(sourceAddr) {
			return false
		}
		return tx.Tx.Coins.GetCoin(common.BTCAsset).Amount.Equal(item.Coin.Amount)
	}
	return observedOutboundMatchesTxOut(tx, TxOutItem{
		Chain:          item.Chain,
		ToAddress:      item.ToAddress,
		VaultPubKey:    item.VaultPubKey,
		Coin:           item.Coin,
		MaxGas:         item.MaxGas,
		GasRate:        item.GasRate,
		InHash:         item.InHash,
		ModuleName:     item.ModuleName,
		TxType:         item.TxType,
		VaultPathIndex: item.VaultPathIndex,
		SourceInputs:   item.SourceInputs,
	})
}

func observedOutboundAlreadyMatchedTxOutBatch(txOut *TxOut, tx ObservedTx) bool {
	if txOut == nil || tx.Tx.ID.IsEmpty() || !tx.Tx.Chain.Equals(common.BTCChain) || len(txOut.TxArray) < 2 {
		return false
	}
	rootAddr, err := common.DeriveBTCTaprootAddress(tx.ObservedPubKey, common.MainVaultPathIndex)
	if err != nil || !tx.Tx.FromAddress.Equals(rootAddr) {
		return false
	}
	total := cosmos.ZeroUint()
	matched := 0
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() ||
			!item.OutHash.Equals(tx.Tx.ID) ||
			!item.Chain.Equals(common.BTCChain) ||
			!item.VaultPubKey.Equals(tx.ObservedPubKey) ||
			item.VaultPathIndex != common.MainVaultPathIndex ||
			!types.IsBatchableTxOutType(item.TxType) ||
			!item.Coin.Asset.Equals(common.BTCAsset) {
			return false
		}
		total = total.Add(item.Coin.Amount)
		matched++
	}
	return matched >= 2 && total.Equal(tx.Tx.Coins.GetCoin(common.BTCAsset).Amount)
}

func markMatchedTxOutItemSettled(ctx cosmos.Context, mgr Manager, item TxOutItem, tx ObservedTx) {
	if item.TxType == types.TxOutTypeSweep && !item.InHash.IsEmpty() {
		markDepositSweepComplete(ctx, mgr, item, tx)
	}
	if item.TxType == types.TxOutTypeRefund && !item.InHash.IsEmpty() {
		deposit, err := mgr.Keeper().GetDepositRecord(ctx, item.InHash)
		if err != nil {
			ctx.Logger().Error("fail to get refunded deposit record", "error", err, "deposit_id", item.InHash.String())
		} else if deposit.Status == types.DepositStatusReturnQueued {
			deposit.Status = types.DepositStatusReturnComplete
			if err := mgr.Keeper().SetDepositRecord(ctx, deposit); err != nil {
				ctx.Logger().Error("fail to mark deposit return complete", "error", err, "deposit_id", item.InHash.String())
			}
		}
	}
	if item.TxType == types.TxOutTypeOut && !item.InHash.IsEmpty() {
		redeem, err := mgr.Keeper().GetShielderRedeem(ctx, item.InHash.String())
		if err == nil && redeem.Status == types.DepositStatusKeysignQueued {
			redeem.Status = types.ShielderRedeemStatusSettled
			if err := mgr.Keeper().SetShielderRedeem(ctx, redeem); err != nil {
				ctx.Logger().Error("fail to mark shielder redeem settled", "error", err, "withdrawal_id", item.InHash.String())
			}
		} else if err != nil {
			ctx.Logger().Debug("matched outbound is not a shielder redeem", "error", err, "in_hash", item.InHash.String())
		}
	}
}

func observedTxSpentTxOutInputs(observed []common.TxInput, expected []types.TxOutInput) bool {
	if len(expected) == 0 {
		return true
	}
	if len(observed) == 0 {
		return false
	}
	matched := make([]bool, len(observed))
	for _, want := range expected {
		found := false
		for i, got := range observed {
			if matched[i] {
				continue
			}
			if want.TxId.Equals(got.TxID) && want.Vout == got.Vout && (want.AmountSats == 0 || got.AmountSats == 0 || want.AmountSats == got.AmountSats) {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func observedOutboundMatchesTxOut(tx ObservedTx, item TxOutItem) bool {
	if !item.OutHash.IsEmpty() ||
		!tx.Tx.Chain.Equals(item.Chain) ||
		!tx.Tx.ToAddress.Equals(item.ToAddress) {
		return false
	}
	btcInternal := tx.Tx.Chain.Equals(common.BTCChain) &&
		(item.TxType == types.TxOutTypeSweep || item.TxType == types.TxOutTypeMigrate || item.VaultPathIndex != 0)
	if !tx.ObservedPubKey.Equals(item.VaultPubKey) {
		if !btcInternal {
			return false
		}
		sourceAddr, err := common.DeriveBTCTaprootAddress(item.VaultPubKey, item.VaultPathIndex)
		if err != nil || !tx.Tx.FromAddress.Equals(sourceAddr) {
			return false
		}
	}
	if btcInternal {
		if len(item.SourceInputs) > 0 && !observedTxSpentTxOutInputs(tx.Tx.SourceInputs, item.SourceInputs) {
			return false
		}
		sourceAddr, err := common.DeriveBTCTaprootAddress(item.VaultPubKey, item.VaultPathIndex)
		if err != nil || !tx.Tx.FromAddress.Equals(sourceAddr) {
			return false
		}
		if item.Coin.Asset.Equals(common.BTCAsset) {
			maxGas := item.MaxGas.ToCoins().GetCoin(common.BTCAsset).Amount
			observedAmount := tx.Tx.Coins.GetCoin(common.BTCAsset).Amount
			observedGas := tx.Tx.Gas.ToCoins().GetCoin(common.BTCAsset).Amount
			intended := item.Coin.Amount.Add(maxGas)
			sourceTotal := cosmos.ZeroUint()
			for _, input := range item.SourceInputs {
				sourceTotal = sourceTotal.Add(cosmos.NewUint(input.AmountSats))
			}
			if sourceTotal.GT(intended) {
				intended = sourceTotal
			}
			actual := observedAmount.Add(observedGas)
			return actual.Equal(intended) &&
				observedGas.LTE(maxGas) &&
				observedAmount.GTE(item.Coin.Amount) &&
				observedAmount.LTE(intended)
		}
	}
	if tx.Tx.Coins.EqualsEx(common.Coins{item.Coin}) {
		return true
	}
	if !item.Coin.Asset.Equals(item.Chain.GetGasAsset()) {
		return false
	}
	asset := item.Chain.GetGasAsset()
	intended := item.Coin.Amount.Add(item.MaxGas.ToCoins().GetCoin(asset).Amount)
	actual := tx.Tx.Coins.GetCoin(asset).Amount.Add(tx.Tx.Gas.ToCoins().GetCoin(asset).Amount)
	if actual.Equal(intended) {
		return true
	}
	observedAmount := tx.Tx.Coins.GetCoin(asset).Amount
	if tx.Tx.Chain.Equals(common.BTCChain) &&
		observedAmount.GTE(item.Coin.Amount) &&
		observedAmount.LTE(intended) {
		return true
	}
	return false
}

func isBTCChildSweepToRoot(tx ObservedTx, pubkey common.PubKey) bool {
	if !tx.Tx.Chain.Equals(common.BTCChain) || pubkey.IsEmpty() {
		return false
	}
	rootAddr, err := common.DeriveBTCTaprootAddress(pubkey, common.MainVaultPathIndex)
	if err != nil || !tx.Tx.ToAddress.Equals(rootAddr) {
		return false
	}
	for _, pathType := range []common.VaultDepositPathType{common.VaultDepositPathUser, common.VaultDepositPathNode} {
		pathIndexes, err := common.VaultDepositLookaheadPathIndexes(pathType)
		if err != nil {
			return false
		}
		for _, pathIndex := range pathIndexes {
			childAddr, err := common.DeriveBTCTaprootAddress(pubkey, pathIndex)
			if err != nil {
				return false
			}
			if tx.Tx.FromAddress.Equals(childAddr) {
				return true
			}
		}
	}
	return false
}
