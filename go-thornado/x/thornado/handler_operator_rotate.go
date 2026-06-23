package thornado

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
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
	if err := msg.ValidateBasic(); err != nil {
		return err
	}
	_, _, err := operatorRotateBonds(ctx, h.mgr.Keeper(), msg)
	return err
}

func (h OperatorRotateHandler) handle(ctx cosmos.Context, msg MsgOperatorRotate) error {
	bonds, newOperatorPubKey, err := operatorRotateBonds(ctx, h.mgr.Keeper(), msg)
	if err != nil {
		return err
	}
	newOperatorAddress, err := newOperatorPubKey.GetThorAddress()
	if err != nil {
		return err
	}
	newOperatorCommonAddress, err := common.NewAddress(newOperatorAddress.String())
	if err != nil {
		return err
	}
	for _, bond := range bonds {
		currentOperator, err := bond.OperatorPubKey.GetThorAddress()
		if err != nil {
			return err
		}
		if err := rotateOperatorBonder(ctx, h.mgr.Keeper(), &bond, currentOperator, newOperatorAddress); err != nil {
			return err
		}
		bond.OperatorPubKey = newOperatorPubKey
		bond.UpdatedHeight = ctx.BlockHeight()
		if err := h.mgr.Keeper().SetShielderNodeBond(ctx, bond); err != nil {
			return err
		}
		nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, bond.NodeAddress)
		if err != nil {
			return err
		}
		if !nodeAccount.IsEmpty() {
			nodeAccount.BondAddress = newOperatorCommonAddress
			if err := h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
				return ErrInternal(err, "fail to save node account")
			}
		}
		if err := h.rotateOpenAuctions(ctx, bond.NodePubKey, newOperatorAddress, newOperatorPubKey); err != nil {
			return err
		}
		if h.mgr.EventMgr() != nil {
			rotateEvent := NewEventOperatorRotate(currentOperator, bond.NodeAddress, newOperatorAddress)
			if err := h.mgr.EventMgr().EmitEvent(ctx, rotateEvent); err != nil {
				ctx.Logger().Error("fail to emit rotate event", "error", err)
			}
		}
	}
	return nil
}

func (h OperatorRotateHandler) rotate(ctx cosmos.Context, operator cosmos.AccAddress, nodeAcc NodeAccount) error {
	bond, err := resolveNodeBondForNodeAccount(ctx, h.mgr.Keeper(), nodeAcc)
	if err != nil {
		return err
	}
	currentOperator, err := bond.OperatorPubKey.GetThorAddress()
	if err != nil {
		return ErrInternal(err, "fail to get operator address")
	}

	if currentOperator.Equals(operator) {
		return fmt.Errorf("operator %s is already assigned to node %s", operator, nodeAcc.NodeAddress)
	}

	return fmt.Errorf("operator rotation is disabled; node operator is the registered operator pubkey")
}

func OperatorRotateAnteHandler(ctx cosmos.Context, _ semver.Version, k keeper.Keeper, msg types.MsgOperatorRotate) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	if _, _, err := operatorRotateBonds(ctx, k, msg); err != nil {
		return ctx, cosmos.ErrUnauthorized(err.Error())
	}
	return ctx.WithPriority(ActiveNodePriority), nil
}

func operatorRotateBonds(ctx cosmos.Context, k keeper.Keeper, msg types.MsgOperatorRotate) ([]types.ShielderNodeBond, common.PubKey, error) {
	newOperatorPubKey, err := common.NewPubKey(msg.OperatorPubKey)
	if err != nil {
		return nil, common.EmptyPubKey, err
	}
	newOperatorAddress, err := newOperatorPubKey.GetThorAddress()
	if err != nil {
		return nil, common.EmptyPubKey, err
	}
	if !newOperatorAddress.Equals(msg.OperatorAddress) {
		return nil, common.EmptyPubKey, fmt.Errorf("operator pubkey does not match operator address")
	}
	if newOperatorAddress.Equals(msg.Signer) {
		return nil, common.EmptyPubKey, fmt.Errorf("operator %s is already assigned", msg.OperatorAddress)
	}
	iter := k.GetShielderNodeBondIterator(ctx)
	if iter == nil {
		return nil, common.EmptyPubKey, fmt.Errorf("no node bonds found for operator %s", msg.Signer)
	}
	defer iter.Close()
	var bonds []types.ShielderNodeBond
	for ; iter.Valid(); iter.Next() {
		var bond types.ShielderNodeBond
		if err := json.Unmarshal(iter.Value(), &bond); err != nil {
			return nil, common.EmptyPubKey, fmt.Errorf("unmarshal node bond: %w", err)
		}
		if bond.Sold || bond.OperatorPubKey.IsEmpty() || bond.NodeAddress.Empty() {
			continue
		}
		operator, err := bond.OperatorPubKey.GetThorAddress()
		if err != nil || !operator.Equals(msg.Signer) {
			continue
		}
		newBonder, err := k.GetShielderNodeBonder(ctx, bond.NodePubKey, newOperatorAddress)
		if err != nil {
			return nil, common.EmptyPubKey, err
		}
		if shielderNodeBonderExists(bond, newBonder, newOperatorAddress) {
			return nil, common.EmptyPubKey, fmt.Errorf("operator %s is already a bonder for node %s", msg.OperatorAddress, bond.NodeAddress)
		}
		oldBonder, err := k.GetShielderNodeBonder(ctx, bond.NodePubKey, msg.Signer)
		if err != nil {
			return nil, common.EmptyPubKey, err
		}
		if !shielderNodeBonderExists(bond, oldBonder, msg.Signer) {
			return nil, common.EmptyPubKey, fmt.Errorf("no bonder matches current operator %s for node %s", msg.Signer, bond.NodeAddress)
		}
		bonds = append(bonds, bond)
	}
	if len(bonds) == 0 {
		return nil, common.EmptyPubKey, fmt.Errorf("no node bonds found for operator %s", msg.Signer)
	}
	return bonds, newOperatorPubKey, nil
}

func rotateOperatorBonder(ctx cosmos.Context, k keeper.Keeper, bond *types.ShielderNodeBond, currentOperator, newOperator cosmos.AccAddress) error {
	oldBonder, err := k.GetShielderNodeBonder(ctx, bond.NodePubKey, currentOperator)
	if err != nil {
		return err
	}
	if !shielderNodeBonderExists(*bond, oldBonder, currentOperator) {
		return fmt.Errorf("no bonder matches current operator %s for node %s", currentOperator, bond.NodeAddress)
	}
	newBonder, err := k.GetShielderNodeBonder(ctx, bond.NodePubKey, newOperator)
	if err != nil {
		return err
	}
	if shielderNodeBonderExists(*bond, newBonder, newOperator) {
		return fmt.Errorf("operator %s is already a bonder for node %s", newOperator, bond.NodeAddress)
	}
	replaced := false
	for i, bonder := range bond.Bonders {
		if strings.TrimSpace(bonder) == currentOperator.String() {
			bond.Bonders[i] = newOperator.String()
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("no bonder index matches current operator %s for node %s", currentOperator, bond.NodeAddress)
	}
	if err := k.DeleteShielderNodeBonder(ctx, bond.NodePubKey, currentOperator); err != nil {
		return err
	}
	oldBonder.Bonder = newOperator
	oldBonder.UpdatedHeight = ctx.BlockHeight()
	return k.SetShielderNodeBonder(ctx, oldBonder)
}

func shielderNodeBonderExists(bond types.ShielderNodeBond, bonder types.ShielderNodeBonder, address cosmos.AccAddress) bool {
	if address.Empty() {
		return false
	}
	for _, existing := range bond.Bonders {
		if strings.TrimSpace(existing) == address.String() {
			return true
		}
	}
	return bonder.Bonder.Equals(address) && shielderNodeBonderHasState(bonder)
}

func shielderNodeBonderHasState(bonder types.ShielderNodeBonder) bool {
	return bonder.PendingSats != 0 ||
		bonder.PrincipalSats != 0 ||
		bonder.FeeDebtShare != 0 ||
		!bonder.PendingFeeDepositID.IsEmpty() ||
		!bonder.SaleEntitlementID.IsEmpty() ||
		bonder.SalePayoutSats != 0 ||
		bonder.CreatedHeight != 0 ||
		bonder.UpdatedHeight != 0
}

func (h OperatorRotateHandler) rotateOpenAuctions(ctx cosmos.Context, nodePubKey string, newOperatorAddress cosmos.AccAddress, newOperatorPubKey common.PubKey) error {
	iter := h.mgr.Keeper().GetNodeSlotAuctionIterator(ctx)
	if iter == nil {
		return nil
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var auction types.NodeSlotAuction
		if err := json.Unmarshal(iter.Value(), &auction); err != nil {
			return err
		}
		if auction.SellerNodePubKey != nodePubKey || auction.Status != types.NodeSlotAuctionOpen {
			continue
		}
		auction.Seller = newOperatorAddress
		auction.SellerOperatorPubKey = newOperatorPubKey
		auction.UpdatedHeight = ctx.BlockHeight()
		if err := h.mgr.Keeper().SetNodeSlotAuction(ctx, auction); err != nil {
			return err
		}
	}
	return nil
}
