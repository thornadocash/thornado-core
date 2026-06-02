package thornado

import (
	"context"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

func processNetworkFeeAttestation(
	ctx cosmos.Context,
	mgr Manager,
	voter *keeper.ObservedNetworkFeeVoter,
	attester cosmos.AccAddress,
	active NodeAccounts,
	nf *common.NetworkFee,
	shouldPenalizeForDuplicate bool,
) error {
	observePenaltyPoints := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_SubmitPenaltyPoints)
	lackOfObservationPenalty := mgr.Keeper().GetConfigInt64(ctx, constants.Observation_MissPenaltyPoints)
	observeFlex := getConfigDurationBlocks(ctx, mgr.Keeper(), constants.Observation_DelayFlexibilityMinutes)

	penaltyCtx := ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_observe_network_fee"),
		telemetry.NewLabel("chain", string(nf.Chain)),
	}))

	if !voter.Sign(attester) {
		// Penalty for the network having to handle the extra message/s.
		if shouldPenalizeForDuplicate {
			mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		}
		ctx.Logger().Info("signer already signed network fee", "signer", attester.String(), "block height", nf.Height, "chain", nf.Chain.String())
		return nil
	}

	// doesn't have consensus yet
	if !voter.HasConsensus(active) {
		// Before consensus, penalty until consensus.
		mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
		return nil
	}

	if voter.BlockHeight > 0 {
		// After consensus, only decrement penalty points if within the Observation_DelayFlexibilityMinutes window.
		if (voter.BlockHeight + observeFlex) >= ctx.BlockHeight() {
			mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, lackOfObservationPenalty, attester)
		}
		// MsgNetworkFeeQuorum tx already processed
		return nil
	}

	voter.BlockHeight = ctx.BlockHeight()

	// This signer brings the voter to consensus; increment the signer's penalty points like the before-consensus signers,
	// then decrement all the signers' penalty points and increment the non-signers' penalty points.
	mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, observePenaltyPoints, attester)
	signers := voter.GetSigners()
	nonSigners := getNonSigners(active, signers)
	mgr.PenaltyManager().DecPenaltyPoints(penaltyCtx, observePenaltyPoints, signers...)
	mgr.PenaltyManager().IncPenaltyPoints(penaltyCtx, lackOfObservationPenalty, nonSigners...)

	ctx.Logger().Info("update network fee", "chain", nf.Chain.String(), "transaction-size", nf.TransactionSize, "fee-rate", nf.TransactionRate)
	if err := mgr.Keeper().SaveNetworkFee(ctx, nf.Chain, NetworkFee{
		Chain:              nf.Chain,
		TransactionSize:    nf.TransactionSize,
		TransactionFeeRate: nf.TransactionRate,
	}); err != nil {
		return ErrInternal(err, "fail to save network fee")
	}

	return nil
}
