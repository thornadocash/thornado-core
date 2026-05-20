package thorchain

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	tcyclaimlist "github.com/thornadocash/go-thornado/common/tcyclaimlist"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// unsafeAddRefundOutbound - schedules a REFUND outbound to destAddr with coin amount
// - inHash: the tx hash being refunded
// - destAddr: the address to refund to
// - coin: the amount to refund
// - height: the block height to schedule the outbound
func unsafeAddRefundOutbound(ctx cosmos.Context, mgr *Mgrs, inHash, destAddr string, coin common.Coin, height int64) error {
	if coin.IsEmpty() || coin.IsNative() {
		return fmt.Errorf("coin must be an external asset")
	}

	// tx details
	dest, err := common.NewAddress(destAddr)
	if err != nil {
		return err
	}
	inTxId, err := common.NewTxID(inHash)
	if err != nil {
		return err
	}
	memo := fmt.Sprintf("REFUND:%s", inTxId.String())

	// choose an asgard with enough balance
	var asg Vault
	activeAsgards, err := mgr.Keeper().GetAsgardVaultsByStatus(ctx, types.VaultStatus_ActiveVault)
	if err != nil || len(activeAsgards) == 0 {
		return fmt.Errorf("fail to get active asgard vaults: %w", err)
	}
	for _, v := range activeAsgards {
		if v.GetCoin(coin.Asset).Amount.GTE(coin.Amount) {
			asg = v
			break
		}
	}
	if asg.IsEmpty() {
		return fmt.Errorf("no asgard with enough balance")
	}

	maxGasCoin, err := mgr.GasMgr().GetMaxGas(ctx, coin.Asset.GetChain())
	if err != nil {
		return fmt.Errorf("fail to get max gas: %w", err)
	}

	txOut := TxOutItem{
		Chain:            coin.Asset.GetChain(),
		InHash:           inTxId,
		ToAddress:        dest,
		VaultPubKey:      asg.PubKey,
		VaultPubKeyEddsa: asg.PubKeyEddsa,
		Coin:             coin,
		MaxGas:           common.Gas{maxGasCoin},
		Memo:             memo,
	}

	err = mgr.TxOutStore().UnSafeAddTxOutItem(ctx, mgr, txOut, height)
	if err != nil {
		return fmt.Errorf("fail to add outbound tx: %w", err)
	}

	return nil
}

// When an ObservedTxInVoter has dangling Actions items swallowed by the vaults, requeue
// them. This can happen when a TX has multiple outbounds scheduled and one of them is
// erroneously scheduled for the past.
// func requeueDanglingActions(ctx cosmos.Context, mgr *Mgrs, txIDs []common.TxID) {
// 	// Select the least secure ActiveVault Asgard for all outbounds.
// 	// Even if it fails (as in if the version changed upon the keygens-complete block of a churn),
// 	// updating the voter's FinalisedHeight allows another MaxOutboundAttempts for LackSigning vault selection.
// 	activeAsgards, err := mgr.Keeper().GetAsgardVaultsByStatus(ctx, types.VaultStatus_ActiveVault)
// 	if err != nil || len(activeAsgards) == 0 {
// 		ctx.Logger().Error("fail to get active asgard vaults", "error", err)
// 		return
// 	}
// 	if len(activeAsgards) > 1 {
// 		signingTransactionPeriod := mgr.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)
// 		activeAsgards = mgr.Keeper().SortBySecurity(ctx, activeAsgards, signingTransactionPeriod)
// 	}
// 	vaultPubKey := activeAsgards[0].PubKey

// 	for _, txID := range txIDs {
// 		voter, err := mgr.Keeper().GetObservedTxInVoter(ctx, txID)
// 		if err != nil {
// 			ctx.Logger().Error("fail to get observed tx voter", "error", err)
// 			continue
// 		}

// 		if len(voter.OutTxs) >= len(voter.Actions) {
// 			log := fmt.Sprintf("(%d) OutTxs present for (%s), despite expecting fewer than the (%d) Actions.", len(voter.OutTxs), txID.String(), len(voter.Actions))
// 			ctx.Logger().Debug(log)
// 			continue
// 		}

// 		var indices []int
// 		for i := range voter.Actions {
// 			if isActionsItemDangling(voter, i) {
// 				indices = append(indices, i)
// 			}
// 		}
// 		if len(indices) == 0 {
// 			log := fmt.Sprintf("No dangling Actions item found for (%s).", txID.String())
// 			ctx.Logger().Debug(log)
// 			continue
// 		}

// 		if len(voter.Actions)-len(voter.OutTxs) != len(indices) {
// 			log := fmt.Sprintf("(%d) Actions and (%d) OutTxs present for (%s), yet there appeared to be (%d) dangling Actions.", len(voter.Actions), len(voter.OutTxs), txID.String(), len(indices))
// 			ctx.Logger().Debug(log)
// 			continue
// 		}

// 		height := ctx.BlockHeight()

// 		// Update the voter's FinalisedHeight to give another MaxOutboundAttempts.
// 		voter.FinalisedHeight = height

// 		for _, index := range indices {
// 			// Use a pointer to update the voter as well.
// 			actionItem := &voter.Actions[index]

// 			// Update the vault pubkey.
// 			actionItem.VaultPubKey = vaultPubKey

// 			// Update the Actions item's MaxGas and GasRate.
// 			// Note that nothing in this function should require a GasManager BeginBlock.
// 			gasCoin, err := mgr.GasMgr().GetMaxGas(ctx, actionItem.Chain)
// 			if err != nil {
// 				ctx.Logger().Error("fail to get max gas", "chain", actionItem.Chain, "error", err)
// 				continue
// 			}
// 			actionItem.MaxGas = common.Gas{gasCoin}
// 			actionItem.GasRate = int64(mgr.GasMgr().GetGasRate(ctx, actionItem.Chain).Uint64())

// 			// UnSafeAddTxOutItem is used to queue the txout item directly, without for instance deducting another fee.
// 			err = mgr.TxOutStore().UnSafeAddTxOutItem(ctx, mgr, *actionItem, height)
// 			if err != nil {
// 				ctx.Logger().Error("fail to add outbound tx", "error", err)
// 				continue
// 			}
// 		}

// 		// Having requeued all dangling Actions items, set the updated voter.
// 		mgr.Keeper().SetObservedTxInVoter(ctx, voter)
// 	}
// }

func setTCYClaims(ctx cosmos.Context, mgr *Mgrs) error {
	claimers, err := getTCYClaimsFromData()
	if err != nil {
		return err
	}

	total := len(claimers)
	for i, claimer := range claimers {
		// Log every 100 claims
		if i%100 == 0 {
			ctx.Logger().Info("setting tcy claimers", "total", total, "number", i)
		}

		if err = mgr.Keeper().UpdateTCYClaimer(ctx, claimer.L1Address, claimer.Asset, claimer.Amount); err != nil {
			ctx.Logger().Error("failed to set tcy claimer", "address", claimer.L1Address.String(), "error", err)
		}
	}

	return nil
}

func getTCYClaimsFromData() ([]types.TCYClaimer, error) {
	var claimers []types.TCYClaimer
	for _, tcyClaimJSON := range tcyclaimlist.GetTCYClaimsList() {
		var asset common.Asset
		asset, err := common.NewAsset(tcyClaimJSON.Asset)
		if err != nil {
			return claimers, err
		}

		claimer := types.TCYClaimer{
			Asset:     asset,
			L1Address: common.Address(tcyClaimJSON.Address),
			Amount:    cosmos.NewUint(tcyClaimJSON.TCYClaim),
		}
		claimers = append(claimers, claimer)
	}

	return claimers, nil
}

// makeFakeTxInObservation accepts an array of unobserved inbounds, queries for active
// node accounts, and makes a fake observation for each validator and unobserved TxIn.
// trunk-ignore(golangci-lint/unused)
func makeFakeTxInObservation(ctx cosmos.Context, mgr Manager, txs ObservedTxs) error {
	activeNodes, err := mgr.Keeper().ListActiveValidators(ctx)
	if err != nil {
		ctx.Logger().Error("Failed to get active nodes", "err", err)
		return err
	}

	handler := NewObservedTxInHandler(mgr)

	for _, na := range activeNodes {
		txInMsg := NewMsgObservedTxIn(txs, na.NodeAddress)
		_, err := handler.handle(ctx, *txInMsg)
		if err != nil {
			ctx.Logger().Error("failed ObservedTxIn handler", "error", err)
			continue
		}
	}

	return nil
}
