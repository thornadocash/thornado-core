package thorchain

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
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

	active, err := h.mgr.Keeper().ListActiveValidators(ctx)
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

	// slash the bond of the banned node
	slash := h.mgr.Keeper().GetConfigInt64(ctx, constants.BondSlashBan)
	if slash > 0 {
		// compute slash and decrement banned node bond
		slashAmt := cosmos.NewUint(uint64(slash))
		if slashAmt.GT(toBan.Bond) {
			slashAmt = toBan.Bond
		}
		toBan.Bond = common.SafeSub(toBan.Bond, slashAmt)

		// transfer the slash amount from bond to reserve
		coin := common.NewCoin(common.RuneNative, slashAmt)
		if err = h.mgr.Keeper().SendFromModuleToModule(ctx, BondName, ReserveName, common.NewCoins(coin)); err != nil {
			ctx.Logger().Error("fail to transfer funds from bond to reserve", "error", err)
			return nil, err
		}

		// emit bond slash event
		bondEvent := NewEventBond(slashAmt, BondCost, common.Tx{}, &toBan, nil)
		if err = h.mgr.EventMgr().EmitEvent(ctx, bondEvent); err != nil {
			return nil, fmt.Errorf("fail to emit bond event: %w", err)
		}
	}

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
