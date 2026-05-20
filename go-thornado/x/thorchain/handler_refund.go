package thorchain

import (
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// RefundHandler a handle to process tx that had refund memo
// usually this type or tx is because Thorchain fail to process the tx, which result in a refund, signer honour the tx and refund customer accordingly
type RefundHandler struct {
	ch  CommonOutboundTxHandler
	mgr Manager
}

// NewRefundHandler create a new refund handler
func NewRefundHandler(mgr Manager) RefundHandler {
	return RefundHandler{
		ch:  NewCommonOutboundTxHandler(mgr),
		mgr: mgr,
	}
}

// Run is the main entry point to process refund outbound message
func (h RefundHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgRefundTx)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive MsgRefund", "tx ID", msg.InTxID.String())
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgRefund fail validation", "error", err)
		return nil, err
	}

	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgRefund", "error", err)
	}
	return result, err
}

func (h RefundHandler) validate(ctx cosmos.Context, msg MsgRefundTx) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	// Check for duplicate refund - verify this exact outbound tx hasn't already been processed
	voter, err := h.mgr.Keeper().GetObservedTxInVoter(ctx, msg.InTxID)
	if err != nil {
		return ErrInternal(err, "fail to get observed tx voter for duplicate check")
	}

	// Check if this specific outbound tx ID has already been recorded
	// This prevents the same refund from being processed multiple times
	for _, outTx := range voter.OutTxs {
		if outTx.ID.Equals(msg.Tx.Tx.ID) && !msg.Tx.Tx.ID.Equals(common.BlankTxID) {
			return cosmos.ErrUnknownRequest("refund transaction already processed")
		}
	}

	// Validate vault exists
	// Note: We allow refunds from any vault status (Active, Retiring, or Inactive)
	// because:
	// 1. Active/Retiring vaults: Normal refund processing
	// 2. Inactive vaults: Can legitimately send refunds for coins they received
	//    (e.g., from inbounds observed to their address before retirement)
	// The creation of refund outbounds from inactive vaults is controlled by
	// the slasher/manager code, not this handler. This handler only validates
	// observed refund transactions that were already sent.
	vault, err := h.mgr.Keeper().GetVault(ctx, msg.Tx.ObservedPubKey)
	if err != nil {
		return ErrInternal(err, "fail to get vault")
	}

	// Ensure vault exists and is valid
	if vault.IsEmpty() {
		return cosmos.ErrUnknownRequest("vault not found")
	}

	// Note: Vault fund sufficiency is validated later in the common outbound handler
	// and the TxOut matching logic. If a vault doesn't have sufficient funds, the
	// transaction will fail to match a TxOutItem and result in slashing.
	// We don't validate funds here to allow malicious/incorrect refunds to be
	// processed and slashed appropriately.

	return nil
}

func (h RefundHandler) handle(ctx cosmos.Context, msg MsgRefundTx) (*cosmos.Result, error) {
	return h.ch.handle(ctx, msg.Tx, msg.InTxID)
}
