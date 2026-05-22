package thornado

import (
	"context"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// CommonOutboundTxHandler is the place where those common logic can be shared
// between multiple different kind of outbound tx handler
// at the moment, handler_refund, and handler_outbound_tx are largely the same
// , only some small difference
type CommonOutboundTxHandler struct {
	mgr Manager
}

// NewCommonOutboundTxHandler create a new instance of the CommonOutboundTxHandler
func NewCommonOutboundTxHandler(mgr Manager) CommonOutboundTxHandler {
	return CommonOutboundTxHandler{
		mgr: mgr,
	}
}

func (h CommonOutboundTxHandler) slash(ctx cosmos.Context, tx ObservedTx) error {
	toSlash := make(common.Coins, len(tx.Tx.Coins))
	copy(toSlash, tx.Tx.Coins)
	toSlash = toSlash.Add(tx.Tx.Gas.ToCoins()...)

	ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{ // nolint
		telemetry.NewLabel("reason", "failed_outbound"),
		telemetry.NewLabel("chain", string(tx.Tx.Chain)),
	}))

	return h.mgr.Slasher().SlashVault(ctx, tx.ObservedPubKey, toSlash, h.mgr)
}

func (h CommonOutboundTxHandler) handle(ctx cosmos.Context, tx ObservedTx, inTxID common.TxID) (*cosmos.Result, error) {
	// Validate that the observed pubkey corresponds to an existing vault.
	// This prevents fake outbound transactions from arbitrary public keys.
	// Note: Vault existence is also checked upstream in ensureVaultAndGetTxOutVoter,
	// but we add this check here as defense-in-depth since this handler can be
	// called from multiple paths (refund, outbound).
	vault, err := h.mgr.Keeper().GetVault(ctx, tx.ObservedPubKey)
	if err != nil {
		ctx.Logger().Error("fail to get vault for outbound validation", "error", err, "pubkey", tx.ObservedPubKey)
		return nil, ErrInternal(err, "fail to get vault for observed pubkey")
	}
	// A completely empty vault (no pubkey) indicates it doesn't exist
	if vault.PubKey.IsEmpty() {
		ctx.Logger().Error("vault not found for observed outbound", "pubkey", tx.ObservedPubKey)
		return nil, ErrInternal(nil, fmt.Sprintf("vault not found for observed pubkey: %s", tx.ObservedPubKey))
	}

	// note: Outbound tx usually it is related to an inbound tx except migration
	// thus here try to get the ObservedTxInVoter,  and set the tx out hash accordingly
	voter, err := h.mgr.Keeper().GetObservedTxInVoter(ctx, inTxID)
	if err != nil {
		return nil, ErrInternal(err, "fail to get observed tx voter")
	}
	if voter.AddOutTx(tx.Tx) {
		if err = h.mgr.EventMgr().EmitEvent(ctx, NewEventOutbound(inTxID, tx.Tx)); err != nil {
			return nil, ErrInternal(err, "fail to emit outbound event")
		}
	}
	h.mgr.Keeper().SetObservedTxInVoter(ctx, voter)

	if tx.Tx.Chain.Equals(common.Thornado) {
		return &cosmos.Result{}, nil
	}

	shouldSlash := true
	signingTransPeriod := h.mgr.GetConstants().GetInt64Value(constants.SigningTransactionPeriod)
	// every Signing Transaction Period , Thornado will check whether a
	// TxOutItem had been sent by signer or not
	// if a txout item that is older than SigningTransactionPeriod, but has not
	// been sent out by signer , LackSigning will create a new TxOutItem
	// and mark the previous TxOutItem as complete.
	//
	// Check the blocks backwards
	// (assuming that most signed outbounds will have been scheduled more rather than less recently)
	// to the current height starting from inbound consensus height
	// or one signingTransPeriod ago, whichever is later.
	earliestHeight := ctx.BlockHeight() - signingTransPeriod
	if voter.Height > earliestHeight {
		earliestHeight = voter.Height
	}

	latestHeight := ctx.BlockHeight()

	for height := latestHeight; height >= earliestHeight; height-- {
		// update txOut record with our TxID that sent funds out of the pool
		var txOut *TxOut
		txOut, err = h.mgr.Keeper().GetTxOut(ctx, height)
		if err != nil {
			ctx.Logger().Error("unable to get txOut record", "error", err)
			return nil, cosmos.ErrUnknownRequest(err.Error())
		}

		// Save TxOut back with the TxID only when the TxOut on the block height is
		// not empty
		for i, txOutItem := range txOut.TxArray {
			// withdraw , refund etc, one inbound tx might result two outbound
			// txes, Thornado have to correlate outbound tx back to the
			// inbound, and also txitem , thus Thornado could record both
			// outbound tx hash correctly given every tx item will only have
			// one coin in it , Thornado could use that to identify which tx it
			// is

			// Use deterministic case-insensitive comparison for aggregator fields.
			// strings.EqualFold uses Unicode case folding which can have locale-specific
			// behavior. Using strings.ToLower on both sides ensures deterministic comparison
			// across all nodes in the consensus path.
			// nolint:staticcheck // SA6005: ToLower is intentionally used instead of EqualFold for determinism
			if txOutItem.InHash.Equals(inTxID) &&
				txOutItem.OutHash.IsEmpty() &&
				tx.Tx.Chain.Equals(txOutItem.Chain) &&
				tx.Tx.ToAddress.Equals(txOutItem.ToAddress) &&
				strings.ToLower(tx.Aggregator) == strings.ToLower(txOutItem.Aggregator) &&
				strings.ToLower(tx.AggregatorTarget) == strings.ToLower(txOutItem.AggregatorTargetAsset) &&
				(tx.ObservedPubKey.Equals(txOutItem.VaultPubKey) ||
					tx.ObservedPubKey.Equals(txOutItem.VaultPubKeyEddsa)) {

				matchCoin := tx.Tx.Coins.EqualsEx(common.Coins{txOutItem.Coin})
				// when outbound is gas asset
				if !matchCoin && txOutItem.Coin.Asset.Equals(txOutItem.Chain.GetGasAsset()) {
					asset := txOutItem.Chain.GetGasAsset()
					intendToSpend := txOutItem.Coin.Amount.Add(txOutItem.MaxGas.ToCoins().GetCoin(asset).Amount)
					actualSpend := tx.Tx.Coins.GetCoin(asset).Amount.Add(tx.Tx.Gas.ToCoins().GetCoin(asset).Amount)
					if intendToSpend.Equal(actualSpend) {
						matchCoin = true
						maxGasAmt := txOutItem.MaxGas.ToCoins().GetCoin(asset).Amount
						realGasAmt := tx.Tx.Gas.ToCoins().GetCoin(asset).Amount
						ctx.Logger().Info("override match coin", "intend to spend", intendToSpend, "actual spend", actualSpend, "max_gas", maxGasAmt, "actual gas", realGasAmt)
						if maxGasAmt.GT(realGasAmt) {
							// Don't reimburse gas difference if the outbound is from an InactiveVault.
							// Reuse the vault variable from initial validation to avoid repeated store reads.
							if vault.Status != InactiveVault {
								// the outbound spend less than MaxGas
								diffGas := maxGasAmt.Sub(realGasAmt)
								h.mgr.GasMgr().AddGasAsset(txOutItem.Coin.Asset, common.Gas{
									common.NewCoin(asset, diffGas),
								}, false)
							}
						} else if maxGasAmt.LT(realGasAmt) {
							// signer spend more than the maximum gas prescribed by Thornado , slash it
							ctx.Logger().Info("slash node", "max gas", maxGasAmt, "real gas spend", realGasAmt, "gap", common.SafeSub(realGasAmt, maxGasAmt).String())
							matchCoin = false
						}
					}
				}
				if txOutItem.Chain.IsEVM() {
					gasAsset := txOutItem.Chain.GetGasAsset()
					maxGasAmount := txOutItem.MaxGas.ToCoins().GetCoin(gasAsset).Amount
					gasAmount := tx.Tx.Gas.ToCoins().GetCoin(gasAsset).Amount
					maxGasCap, known := maxEVMGasForChain(ctx, h.mgr.Keeper(), txOutItem.Chain)
					if !known {
						ctx.Logger().Info(
							"using default max gas cap for unlisted EVM chain",
							"chain", txOutItem.Chain,
							"max_gas_cap", maxGasCap.String(),
						)
					}
					if gasAmount.GTE(maxGasCap) && maxGasAmount.LT(maxGasCap) {
						ctx.Logger().Info("EVM chain transaction spent more than max gas cap, should be slashed", "chain", txOutItem.Chain, "gas", gasAmount.String(), "max gas cap", maxGasCap)
						matchCoin = false
					}
				}

				if !matchCoin {
					continue
				}
				txOut.TxArray[i].OutHash = tx.Tx.ID
				shouldSlash = false
				if err = h.mgr.Keeper().SetTxOut(ctx, txOut); err != nil {
					ctx.Logger().Error("fail to save tx out", "error", err)
				}

				break

			}
		}
		// If the TxOutItem matching the observed outbound has been found,
		// do not check other blocks.
		if !shouldSlash {
			break
		}
	}

	slashed := false
	// Slash the vault if no matching TxOutItem was found, unless this is an
	// authorized operational transaction (fake gas tx or cancel tx).
	// - isOutboundFakeGasTx: Fake gas transactions used for EVM chain operations
	//   (amount=1, gas asset, self-referential OUT: memo)
	// - isCancelOrApprovalTx: Cancel transactions sent by bifrost to unstuck pending transactions
	//   on EVM chains (vault-to-vault, dust threshold amount, empty memo)
	if shouldSlash && !isOutboundFakeGasTx(tx) && !isCancelOrApprovalTx(tx) {
		ctx.Logger().Info("slash node account, no matched tx out item", "inbound txid", inTxID, "outbound tx", tx.Tx)

		// Send security alert for unmatched outbounds.
		// Note: Security event emission errors are logged but don't halt processing,
		// as the slashing penalty is the primary security mechanism.
		msg := fmt.Sprintf("missing tx out in=%s", inTxID)
		if err = h.mgr.EventMgr().EmitEvent(ctx, NewEventSecurity(tx.Tx, msg)); err != nil {
			ctx.Logger().Error("fail to emit security event", "error", err)
		}

		if err = h.slash(ctx, tx); err != nil {
			return nil, ErrInternal(err, "fail to slash account")
		}
		slashed = true
	}

	if err := h.mgr.Keeper().SetLastSignedHeight(ctx, voter.FinalisedHeight); err != nil {
		ctx.Logger().Info("fail to update last signed height", "error", err)
	}

	// the slash event is not exposed, but detected upstream in the handler
	var events []abcitypes.Event
	if slashed {
		events = append(events, abcitypes.Event{Type: "vault-slash"})
	}

	return &cosmos.Result{Events: events}, nil
}

// calcReclaim attempts to split spent clout between two reclaimable clouts as equally as possible.
func calcReclaim(reclaimable1, reclaimable2, spent cosmos.Uint) (reclaim1, reclaim2 cosmos.Uint) {
	// Ensure that the spent clout doesn't exceed the total reclaimable clout
	totalReclaimable := reclaimable1.Add(reclaimable2)
	if spent.GT(totalReclaimable) {
		return reclaimable1, reclaimable2
	}

	// Split the spent clout in half
	halfSpent := spent.Quo(sdkmath.NewUint(2))

	// If either clout is less than half the spent amount, allocate all to that clout
	if reclaimable1.LT(halfSpent) {
		return reclaimable1, spent.Sub(reclaimable1)
	} else if reclaimable2.LT(spent.Sub(halfSpent)) {
		return spent.Sub(reclaimable2), reclaimable2
	}

	// Otherwise, split the spent clout equally
	return halfSpent, spent.Sub(halfSpent)
}

func maxEVMGasForChain(ctx cosmos.Context, k keeper.Keeper, chain common.Chain) (cosmos.Uint, bool) {
	return cosmos.NewUint(constants.DefaultMaxEVMGas), false
}
