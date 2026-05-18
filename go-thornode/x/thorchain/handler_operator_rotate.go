package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

// OperatorRotateHandler is the handler to process MsgOperatorRotate.
type OperatorRotateHandler struct {
	mgr Manager
}

// NewOperatorRotateHandler creates a new instance of OperatorRotateHandler.
func NewOperatorRotateHandler(mgr Manager) OperatorRotateHandler {
	return OperatorRotateHandler{
		mgr: mgr,
	}
}

// Run is the main entry point for OperatorRotateHandler.
func (h OperatorRotateHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgOperatorRotate)
	if !ok {
		return nil, errInvalidMessage
	}

	err := h.validate(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("MsgOperatorRotate failed validation", "error", err)
		return nil, err
	}

	err = h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgOperatorRotate", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, err
}

func (h OperatorRotateHandler) validate(ctx cosmos.Context, msg MsgOperatorRotate) error {
	return msg.ValidateBasic()
}

func (h OperatorRotateHandler) handle(ctx cosmos.Context, msg MsgOperatorRotate) error {
	// check mimir if rotate is halted
	haltOperatorRotate := h.mgr.Keeper().GetConfigInt64(ctx, constants.HaltOperatorRotate)
	if haltOperatorRotate > 0 {
		return fmt.Errorf("rotate is halted")
	}

	// rotate is only allowed in the first half of churn
	lastChurnHeight := getLastChurnHeight(ctx, h.mgr.Keeper())
	churnInterval := h.mgr.Keeper().GetConfigInt64(ctx, constants.ChurnInterval)
	halfChurn := churnInterval / 2
	rotateCutoffHeight := lastChurnHeight + halfChurn
	if ctx.BlockHeight() > rotateCutoffHeight {
		return fmt.Errorf("rotate is only allowed in the first half of churn")
	}

	// find nodes operated by the signer
	iter := h.mgr.Keeper().GetNodeAccountIterator(ctx)
	defer iter.Close()
	rotateNodes := NodeAccounts{}
	for ; iter.Valid(); iter.Next() {
		var na NodeAccount
		if err := h.mgr.Keeper().Cdc().Unmarshal(iter.Value(), &na); err != nil {
			return fmt.Errorf("fail to unmarshal node account, %w", err)
		}

		// skip empty node accounts
		if na.IsEmpty() {
			continue
		}

		// rotation not allowed if any of the new operator nodes are active
		if na.Status == NodeActive && na.BondAddress.Equals(common.Address(msg.OperatorAddress.String())) {
			return fmt.Errorf("cannot rotate to operator with active node: %s", na.NodeAddress)
		}

		// filter by bond address
		if !na.BondAddress.Equals(common.Address(msg.Signer.String())) {
			continue
		}

		// rotation not allowed if any of the old operator nodes are active
		if na.Status == NodeActive {
			return fmt.Errorf("cannot rotate from operator with active node: %s", na.NodeAddress)
		}

		rotateNodes = append(rotateNodes, na)
	}

	if len(rotateNodes) == 0 {
		return fmt.Errorf("no nodes found for operator %s", msg.Signer)
	}

	// rotate each node
	for _, node := range rotateNodes {
		if err := h.rotate(ctx, msg.OperatorAddress, node); err != nil {
			return err
		}
	}

	return nil
}

func (h OperatorRotateHandler) rotate(ctx cosmos.Context, operator cosmos.AccAddress, nodeAcc NodeAccount) error {
	currentOperator, err := nodeAcc.BondAddress.AccAddress()
	if err != nil {
		return ErrInternal(err, "fail to get bond address")
	}

	// get current bond provider records
	bp, err := h.mgr.Keeper().GetBondProviders(ctx, nodeAcc.NodeAddress)
	if err != nil {
		return ErrInternal(err, fmt.Sprintf("fail to get bond providers(%s)", nodeAcc.NodeAddress))
	}

	// check for duplicate bond provider before making changes
	if bp.Has(operator) {
		return fmt.Errorf("operator %s is already a bond provider for node %s", operator, nodeAcc.NodeAddress)
	}

	// call passiveBackfill before mutating BondAddress so it reads the correct operator
	err = passiveBackfill(ctx, h.mgr, nodeAcc, &bp)
	if err != nil {
		return err
	}

	// rotate the operator address after passiveBackfill
	nodeAcc.BondAddress = common.Address(operator.String())

	// update the corresponding bond provider record
	updated := false
	for i, provider := range bp.Providers {
		if provider.BondAddress.Equals(currentOperator) {
			bp.Providers[i].BondAddress = operator
			updated = true
		}
	}
	if !updated && len(bp.Providers) > 0 {
		return fmt.Errorf("no bond provider matches current operator %s for node %s", currentOperator, nodeAcc.NodeAddress)
	}

	// store updated bond provider records
	err = h.mgr.Keeper().SetBondProviders(ctx, bp)
	if err != nil {
		return ErrInternal(err, "fail to save bond providers")
	}

	// store updated node account
	err = h.mgr.Keeper().SetNodeAccount(ctx, nodeAcc)
	if err != nil {
		return ErrInternal(err, "fail to save node account")
	}

	rotateEvent := NewEventOperatorRotate(currentOperator, nodeAcc.NodeAddress, operator)
	if err := h.mgr.EventMgr().EmitEvent(ctx, rotateEvent); err != nil {
		ctx.Logger().Error("fail to emit rotate event", "error", err)
	}

	return nil
}
