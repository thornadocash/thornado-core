package thornado

import (
	"fmt"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type LeaveHandler struct {
	mgr Manager
}

func NewLeaveHandler(mgr Manager) LeaveHandler {
	return LeaveHandler{
		mgr: mgr,
	}
}

func (h LeaveHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*types.MsgLeave)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive node leave request", "node_address", msg.NodeAddress.String(), "signer", msg.Signer.String())
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg leave failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg leave", "error", err)
		return nil, err
	}
	return &cosmos.Result{}, nil
}

func (h LeaveHandler) validate(ctx cosmos.Context, msg types.MsgLeave) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}
	return validateNodeOperatorAuth(ctx, h.mgr.Keeper(), msg.NodeAddress, msg.Signer)
}

func LeaveAnteHandler(ctx cosmos.Context, _ semver.Version, k keeper.Keeper, msg types.MsgLeave) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	if err := validateNodeOperatorAuth(ctx, k, msg.NodeAddress, msg.Signer); err != nil {
		return ctx, err
	}
	return ctx.WithPriority(ActiveNodePriority), nil
}

func validateNodeOperatorAuth(ctx cosmos.Context, k keeper.Keeper, nodeAddr, signer cosmos.AccAddress) error {
	if _, err := resolveNodeAccountByAddressAndSigner(ctx, k, nodeAddr, signer); err != nil {
		ctx.Logger().Error("fail to authorize node operator", "error", err, "node_address", nodeAddr.String(), "signer", signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", signer))
	}
	return nil
}

func (h LeaveHandler) handle(ctx cosmos.Context, msg types.MsgLeave) error {
	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.NodeAddress)
	if err != nil {
		return cosmos.ErrUnauthorized(fmt.Sprintf("unable to find account: %s", msg.NodeAddress))
	}
	if nodeAccount.IsEmpty() {
		return cosmos.ErrUnauthorized(fmt.Sprintf("unable to find account: %s", msg.NodeAddress))
	}

	nodeAccount.RequestedToLeave = true
	if nodeAccount.LeaveScore == 0 {
		nodeAccount.LeaveScore = uint64(ctx.BlockHeight())
	}
	if err := h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		return fmt.Errorf("fail to save node account: %w", err)
	}

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("request_leave",
			cosmos.NewAttribute("node_address", msg.NodeAddress.String()),
			cosmos.NewAttribute("signer", msg.Signer.String())))
	return nil
}
