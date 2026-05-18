package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// MaintHandler is to handle maintenance toggle messages
// The maintenance flag allows node operators to temporarily exclude their nodes
// from churn cycles without affecting their bond or node status.
//
// When maintenance mode is enabled:
//   - Node will remain in the network but will be excluded from consideration during churn cycles
//   - Node status is not changed directly by maintenance mode, but NodePreflightCheck will
//     return NodeStandby for nodes in maintenance mode
//   - Maintenance mode can be enabled regardless of other node states (including RequestedToLeave)
//   - Only the node operator can toggle maintenance mode for their own node
type MaintHandler struct {
	mgr Manager
}

// NewMaintHandler create new instance of MaintHandler
func NewMaintHandler(mgr Manager) MaintHandler {
	return MaintHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute maintenance toggle logic
func (h MaintHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*types.MsgMaint)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive maintenance toggle request", "node", msg.Signer)
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg maintenance toggle failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg maintenance toggle", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h MaintHandler) validate(ctx cosmos.Context, msg types.MsgMaint) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	// Ensure that the signer is a valid node account
	if err := validateMaintAuth(ctx, h.mgr.Keeper(), msg.NodeAddress, msg.Signer); err != nil {
		return err
	}

	return nil
}

func validateMaintAuth(ctx cosmos.Context, k keeper.Keeper, acc, signer cosmos.AccAddress) error {
	nodeAccount, err := k.GetNodeAccount(ctx, acc)
	if err != nil {
		ctx.Logger().Error("fail to get node account", "error", err, "address", acc.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", acc))
	}
	if nodeAccount.IsEmpty() {
		ctx.Logger().Error("fail to get node account, empty")
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", acc))
	}
	bondAddr, err := nodeAccount.BondAddress.AccAddress()
	if err != nil {
		return err
	}
	if !bondAddr.Equals(signer) {
		ctx.Logger().Error("unauthorized account", "operator", bondAddr.String(), "signer", signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", acc))
	}
	return nil
}

func (h MaintHandler) handle(ctx cosmos.Context, msg types.MsgMaint) error {
	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.NodeAddress)
	if err != nil {
		ctx.Logger().Error("fail to get node account", "error", err, "address", msg.NodeAddress.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("unable to find account: %s", msg.NodeAddress))
	}

	// Toggle the maintenance flag
	newMaintStatus := !nodeAccount.Maintenance
	nodeAccount.Maintenance = newMaintStatus
	if err = h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		return fmt.Errorf("fail to save node account: %w", err)
	}

	// Enhanced logging for better visibility of maintenance changes
	if newMaintStatus {
		ctx.Logger().Info("Node entered maintenance mode",
			"node_address", msg.NodeAddress.String())
	} else {
		ctx.Logger().Info("Node exited maintenance mode",
			"node_address", msg.NodeAddress.String())
	}

	maintStatus := "off"
	if nodeAccount.Maintenance {
		maintStatus = "on"
	}
	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("toggle_maintenance",
			cosmos.NewAttribute("node_address", msg.NodeAddress.String()),
			cosmos.NewAttribute("maintenance", maintStatus)))

	return nil
}
