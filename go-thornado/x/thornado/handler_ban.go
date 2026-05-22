package thornado

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// BanHandler is to handle Ban message
type BanHandler struct {
	mgr Manager
}

// NewBanHandler create new instance of BanHandler
func NewBanHandler(mgr Manager) BanHandler {
	return BanHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute Ban logic
func (h BanHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgBan)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg ban failed validation", "error", err)
		return nil, err
	}
	return h.handle(ctx, *msg)
}

func (h BanHandler) validate(ctx cosmos.Context, msg MsgBan) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(errNotAuthorized.Error())
	}

	return nil
}

func (h BanHandler) handle(ctx cosmos.Context, msg MsgBan) (*cosmos.Result, error) {
	ctx.Logger().Info("handleMsgBan request", "node address", msg.NodeAddress.String())
	toBan, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.NodeAddress)
	if err != nil {
		err = wrapError(ctx, err, "fail to get to ban node account")
		return nil, err
	}
	if err = toBan.Valid(); err != nil {
		return nil, err
	}
	if toBan.ForcedToLeave {
		// already ban, no need to ban again
		return &cosmos.Result{}, nil
	}

	switch toBan.Status {
	case NodeActive, NodeStandby:
		// we can ban an active or standby node
	default:
		return nil, errorsmod.Wrap(errInternal, "cannot ban a node account that is not currently active or standby")
	}

	banner, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.Signer)
	if err != nil {
		err = wrapError(ctx, err, "fail to get banner node account")
		return nil, err
	}
	if err = banner.Valid(); err != nil {
		return nil, err
	}

	active, err := h.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		err = wrapError(ctx, err, "fail to get list of active node accounts")
		return nil, err
	}

	voter, err := h.mgr.Keeper().GetBanVoter(ctx, msg.NodeAddress)
	if err != nil {
		return nil, err
	}

	voter.Sign(msg.Signer)
	h.mgr.Keeper().SetBanVoter(ctx, voter)

	// doesn't have consensus yet
	if !voter.HasConsensus(active) {
		ctx.Logger().Info("not having consensus yet, return")
		return &cosmos.Result{}, nil
	}

	if voter.BlockHeight > 0 {
		// ban already processed
		return &cosmos.Result{}, nil
	}

	voter.BlockHeight = ctx.BlockHeight()
	h.mgr.Keeper().SetBanVoter(ctx, voter)

	toBan.ForcedToLeave = true
	toBan.LeaveScore = 1 // Set Leave Score to 1, which means the nodes is bad

	if err := h.mgr.Keeper().SetNodeAccount(ctx, toBan); err != nil {
		err = fmt.Errorf("fail to save node account: %w", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

// BanAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func BanAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgBan) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
