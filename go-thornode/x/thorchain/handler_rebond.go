package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/constants"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type ReBondHandler struct {
	mgr Manager
}

func NewReBondHandler(mgr Manager) *ReBondHandler {
	return &ReBondHandler{mgr: mgr}
}

func (h ReBondHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgReBond)
	if !ok {
		return nil, errInvalidMessage
	}

	ctx.Logger().Info("receive MsgRebond",
		"node_address", msg.NodeAddress,
		"request_hash", msg.TxIn.ID,
		"new_address", msg.NewBondProviderAddress,
		"amount", msg.Amount,
	)

	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg rebond fail validation", "error", err)
		return nil, err
	}

	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg unbond", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h ReBondHandler) validate(ctx cosmos.Context, msg MsgReBond) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !msg.TxIn.Coins.IsEmpty() {
		return cosmos.ErrUnknownRequest("rebond message cannot have a non-zero coin amount")
	}

	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.NodeAddress)
	if err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to get node account(%s)", msg.NodeAddress))
	}

	if nodeAccount.Status == NodeUnknown {
		return ErrInternal(nil, "node account status is unknown")
	}

	nodeAccountAddress, err := nodeAccount.BondAddress.AccAddress()
	if err != nil {
		return ErrInternal(err, "fail to get node account address")
	}

	if nodeAccountAddress.Equals(msg.Signer) {
		return cosmos.ErrUnknownRequest("node account is not allowed to rebond")
	}

	bondProviders, err := h.mgr.Keeper().GetBondProviders(ctx, msg.NodeAddress)
	if err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to get bond providers(%s)", msg.NodeAddress))
	}

	if !bondProviders.Has(msg.Signer) {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("%s is not bonded with %s", msg.Signer, msg.NodeAddress))
	}

	if !bondProviders.Has(msg.NewBondProviderAddress) {
		return cosmos.ErrUnknownRequest("new bond address is not whitelisted for node account")
	}

	return nil
}

func (h ReBondHandler) handle(ctx cosmos.Context, msg MsgReBond) error {
	value := h.mgr.Keeper().GetConfigInt64(ctx, constants.HaltRebond)
	if value > 0 {
		return fmt.Errorf("rebond has been disabled by mimir")
	}

	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.NodeAddress)
	if err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to get node account(%s)", msg.NodeAddress))
	}

	bondProviders, err := h.mgr.Keeper().GetBondProviders(ctx, msg.NodeAddress)
	if err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to get bond providers(%s)", msg.NodeAddress))
	}

	bond := nodeAccount.Bond

	err = passiveBackfill(ctx, h.mgr, nodeAccount, &bondProviders)
	if err != nil {
		return err
	}

	bondProviders.Adjust(nodeAccount.Bond)
	oldProvider := bondProviders.Get(msg.Signer)
	newProvider := bondProviders.Get(msg.NewBondProviderAddress)

	if oldProvider.IsEmpty() {
		return cosmos.ErrUnknownRequest("old bond provider not found")
	}

	if oldProvider.Bond.IsZero() {
		return cosmos.ErrUnknownRequest("bond is zero")
	}

	if newProvider.IsEmpty() {
		return cosmos.ErrUnknownRequest("new bond provider not found")
	}

	amount := msg.Amount
	if amount.IsZero() || amount.GT(oldProvider.Bond) {
		amount = oldProvider.Bond
	}

	bondProviders.Unbond(amount, oldProvider.BondAddress)
	bondProviders.Bond(amount, newProvider.BondAddress)

	// if old provider bond is zero, remove it
	if amount.Equal(oldProvider.Bond) {
		bondProviders.Remove(oldProvider.BondAddress)
	}

	if !bond.Equal(nodeAccount.Bond) {
		return fmt.Errorf("node bond changed")
	}

	if err = h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to save node account(%s)", nodeAccount.String()))
	}

	if err = h.mgr.Keeper().SetBondProviders(ctx, bondProviders); err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to save bond providers(%s)", bondProviders.NodeAddress.String()))
	}

	rebondEvent := NewEventReBond(
		amount,
		msg.TxIn,
		&nodeAccount,
		oldProvider.BondAddress,
		newProvider.BondAddress,
	)

	if err = h.mgr.EventMgr().EmitEvent(ctx, rebondEvent); err != nil {
		ctx.Logger().Error("fail to emit rebond event", "error", err)
	}

	return nil
}
