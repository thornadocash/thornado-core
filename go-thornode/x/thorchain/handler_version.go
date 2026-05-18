package thorchain

import (
	"fmt"

	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

// VersionHandler is to handle Version message
type VersionHandler struct {
	mgr Manager
}

// NewVersionHandler create new instance of VersionHandler
func NewVersionHandler(mgr Manager) VersionHandler {
	return VersionHandler{
		mgr: mgr,
	}
}

// Run it the main entry point to execute Version logic
func (h VersionHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgSetVersion)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive version number", "version", msg.Version)
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg set version failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg set version", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h VersionHandler) validate(ctx cosmos.Context, msg MsgSetVersion) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	v, err := semver.Parse(msg.Version)
	if err != nil {
		ctx.Logger().Info("invalid version", "version", msg.Version)
		return cosmos.ErrUnknownRequest(fmt.Sprintf("%s is invalid", msg.Version))
	}
	if len(v.Build) > 0 || len(v.Pre) > 0 {
		return cosmos.ErrUnknownRequest("THORChain doesn't use Pre/Build version")
	}
	if err := validateVersionAuth(ctx, h.mgr.Keeper(), msg.Signer); err != nil {
		return cosmos.ErrUnauthorized(err.Error())
	}
	return nil
}

func (h VersionHandler) handle(ctx cosmos.Context, msg MsgSetVersion) error {
	ctx.Logger().Info("handleMsgSetVersion request", "Version:", msg.Version)

	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.Signer)
	if err != nil {
		return cosmos.ErrUnauthorized(fmt.Errorf("unable to find account(%s):%w", msg.Signer, err).Error())
	}

	version, err := msg.GetVersion()
	if err != nil {
		return fmt.Errorf("fail to parse version: %w", err)
	}

	if nodeAccount.GetVersion().LT(version) {
		nodeAccount.Version = version.String()
	}

	if err := h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		return fmt.Errorf("fail to save node account: %w", err)
	}

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("set_version",
			cosmos.NewAttribute("thor_address", msg.Signer.String()),
			cosmos.NewAttribute("version", msg.Version)))

	if nodeAccount.Status == NodeActive {
		// This could affect the MinJoinVersion, so update it.
		h.mgr.Keeper().SetMinJoinLast(ctx)
	}

	return nil
}

func validateVersionAuth(ctx cosmos.Context, k keeper.Keeper, signer cosmos.AccAddress) error {
	nodeAccount, err := k.GetNodeAccount(ctx, signer)
	if err != nil {
		ctx.Logger().Error("fail to get node account", "error", err, "address", signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", signer))
	}
	if nodeAccount.IsEmpty() {
		ctx.Logger().Error("unauthorized account", "address", signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", signer))
	}
	if nodeAccount.Type != NodeTypeValidator {
		ctx.Logger().Error("unauthorized account, node account must be a validator", "address", signer.String(), "type", nodeAccount.Type)
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", signer))
	}
	return nil
}

// VersionAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func VersionAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgSetVersion) (cosmos.Context, error) {
	if err := validateVersionAuth(ctx, k, msg.Signer); err != nil {
		return ctx, err
	}

	return ctx, k.DeductNativeTxFeeFromBond(ctx, msg.Signer)
}
