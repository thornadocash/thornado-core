package thornado

import "github.com/thornadocash/go-thornado/common/cosmos"

// ConsolidateHandler handles transactions the network sends to itself, to consolidate UTXOs
type ConsolidateHandler struct {
	mgr Manager
}

// NewConsolidateHandler create a new instance of the ConsolidateHandler
func NewConsolidateHandler(mgr Manager) ConsolidateHandler {
	return ConsolidateHandler{
		mgr: mgr,
	}
}

func (h ConsolidateHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgConsolidate)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgConsolidate failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("failed to process MsgConsolidate", "error", err)
		return nil, err
	}
	return result, nil
}

func (h ConsolidateHandler) validate(ctx cosmos.Context, msg MsgConsolidate) error {
	return msg.ValidateBasic()
}

func (h ConsolidateHandler) handle(ctx cosmos.Context, msg MsgConsolidate) (*cosmos.Result, error) {
	shouldHalt := false

	// ensure transaction is sending to/from same address
	if !msg.ObservedTx.Tx.FromAddress.Equals(msg.ObservedTx.Tx.ToAddress) {
		shouldHalt = true
	}

	vault, err := h.mgr.Keeper().GetVault(ctx, msg.ObservedTx.ObservedPubKey)
	if err != nil {
		ctx.Logger().Error("unable to get vault for consolidation", "error", err)
	} else { // nolint
		if !vault.IsBase() {
			shouldHalt = true
		}
	}

	if shouldHalt {
		ctx.Logger().Info("halt BTC vault, invalid consolidation", "tx", msg.ObservedTx.Tx)
		if err := haltBTCVaultForIssue(ctx, h.mgr.Keeper(), h.mgr.EventMgr(), msg.ObservedTx.Tx, "invalid consolidation"); err != nil {
			return nil, ErrInternal(err, "fail to halt BTC vault")
		}
	}

	return &cosmos.Result{}, nil
}
