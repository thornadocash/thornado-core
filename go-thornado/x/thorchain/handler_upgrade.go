package thorchain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
	keeperv1 "github.com/thornadocash/go-thornado/x/thorchain/keeper/v1"
	"github.com/thornadocash/go-thornado/x/thorchain/types"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/blang/semver"
)

// ProposeUpgradeHandler is to handle the ProposeUpgrade message
type ProposeUpgradeHandler struct {
	mgr Manager
}

// NewProposeUpgradeHandler create new instance of ProposeUpgradeHandler
func NewProposeUpgradeHandler(mgr Manager) ProposeUpgradeHandler {
	return ProposeUpgradeHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute upgrade proposal logic
func (h ProposeUpgradeHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgProposeUpgrade)
	if !ok {
		return nil, errInvalidMessage
	}

	u := msg.Upgrade

	ctx.Logger().Info(
		"Validator propose upgrade",
		"thor_address", msg.Signer.String(),
		"name", msg.Name,
		"height", u.Height,
		"info", u.Info,
	)

	if err := h.validate(ctx, msg); err != nil {
		ctx.Logger().Error("msg propose upgrade failed validation", "error", err)
		return nil, err
	}

	if err := h.handle(ctx, msg); err != nil {
		ctx.Logger().Error("failed to process msg propose upgrade", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h ProposeUpgradeHandler) validate(ctx cosmos.Context, msg *MsgProposeUpgrade) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if err := signedByActiveNodeAccount(ctx, h.mgr.Keeper(), msg.Signer); err != nil {
		return cosmos.ErrUnauthorized(err.Error())
	}

	if ctx.BlockHeight() >= msg.Upgrade.Height {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade height %d must be in the future, current: %d", msg.Upgrade.Height, ctx.BlockHeight()))
	}

	k := h.mgr.Keeper()
	u, err := k.GetProposedUpgrade(ctx, msg.Name)
	if err != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("failed to get proposed upgrade: %s", msg.Name))
	}

	if u != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade proposal already exists: %s", msg.Name))
	}

	iter := k.GetUpgradeProposalIterator(ctx)
	defer iter.Close()

	const maxProposalCount = 3
	count := 0
	for ; iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()
		var proposal types.UpgradeProposal
		if err := k.Cdc().Unmarshal(value, &proposal); err != nil {
			return cosmos.ErrUnknownRequest(fmt.Sprintf("failed to unmarshal upgrade proposal: %s", key))
		}

		if !proposal.Proposer.Equals(msg.Signer) {
			continue
		}

		count++

		if count == maxProposalCount {
			return cosmos.ErrUnknownRequest(fmt.Sprintf("exceeded maximum number of upgrade proposals: %d", maxProposalCount))
		}
	}

	return nil
}

func (h ProposeUpgradeHandler) handle(ctx cosmos.Context, msg *MsgProposeUpgrade) error {
	u := msg.Upgrade
	name := msg.Name
	k := h.mgr.Keeper()

	if err := k.ProposeUpgrade(ctx, name, types.UpgradeProposal{
		Height:   u.Height,
		Info:     u.Info,
		Proposer: msg.Signer,
	}); err != nil {
		return fmt.Errorf("failed to propose upgrade: %w", err)
	}

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("propose_upgrade",
			cosmos.NewAttribute("thor_address", msg.Signer.String()),
			cosmos.NewAttribute("name", name),
			cosmos.NewAttribute("height", strconv.FormatInt(u.Height, 10)),
			cosmos.NewAttribute("info", u.Info),
		),
	)

	k.ApproveUpgrade(ctx, msg.Signer, name)

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("approve_upgrade",
			cosmos.NewAttribute("thor_address", msg.Signer.String()),
			cosmos.NewAttribute("name", name),
		),
	)

	return scheduleUpgradeIfNecessary(ctx, k, name)
}

// ApproveUpgradeHandler is to handle the ApproveUpgrade message
type ApproveUpgradeHandler struct {
	mgr Manager
}

// NewApproveUpgradeHandler create new instance of ApproveUpgradeHandler
func NewApproveUpgradeHandler(mgr Manager) ApproveUpgradeHandler {
	return ApproveUpgradeHandler{
		mgr: mgr,
	}
}

// Run it the main entry point to execute Version logic
func (h ApproveUpgradeHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgApproveUpgrade)
	if !ok {
		return nil, errInvalidMessage
	}

	ctx.Logger().Info(
		"Validator approving upgrade",
		"thor_address", msg.Signer.String(),
		"name", msg.Name,
	)

	if err := h.validate(ctx, msg); err != nil {
		ctx.Logger().Error("msg approve upgrade failed validation", "error", err)
		return nil, err
	}

	if err := h.handle(ctx, msg); err != nil {
		ctx.Logger().Error("failed to process msg approve upgrade", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h ApproveUpgradeHandler) validate(ctx cosmos.Context, msg *MsgApproveUpgrade) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	k := h.mgr.Keeper()

	if err := signedByActiveNodeAccount(ctx, k, msg.Signer); err != nil {
		return cosmos.ErrUnauthorized(err.Error())
	}

	u, err := k.GetProposedUpgrade(ctx, msg.Name)
	if err != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("failed to get proposed upgrade: %s", msg.Name))
	}

	if u == nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade proposal does not exist: %s", msg.Name))
	}

	// Don't care about error here. If it doesn't exist, it's not approved.
	v, _ := k.GetUpgradeVote(ctx, msg.Signer, msg.Name)
	if v {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade already approved: %s", msg.Name))
	}

	return nil
}

func (h ApproveUpgradeHandler) handle(ctx cosmos.Context, msg *MsgApproveUpgrade) error {
	k := h.mgr.Keeper()
	name := msg.Name

	k.ApproveUpgrade(ctx, msg.Signer, name)

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("approve_upgrade",
			cosmos.NewAttribute("thor_address", msg.Signer.String()),
			cosmos.NewAttribute("name", name),
		),
	)

	return scheduleUpgradeIfNecessary(ctx, k, name)
}

// RejectUpgradeHandler is to handle the RejectUpgrade message
type RejectUpgradeHandler struct {
	mgr Manager
}

// NewRejectUpgradeHandler create new instance of RejectUpgradeHandler
func NewRejectUpgradeHandler(mgr Manager) RejectUpgradeHandler {
	return RejectUpgradeHandler{
		mgr: mgr,
	}
}

// Run it the main entry point to execute Version logic
func (h RejectUpgradeHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgRejectUpgrade)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info(
		"Validator rejecting upgrade",
		"thor_address", msg.Signer.String(),
		"name", msg.Name,
	)
	if err := h.validate(ctx, msg); err != nil {
		ctx.Logger().Error("msg reject upgrade failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, msg); err != nil {
		ctx.Logger().Error("failed to process msg reject upgrade", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h RejectUpgradeHandler) validate(ctx cosmos.Context, msg *MsgRejectUpgrade) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	k := h.mgr.Keeper()

	if err := signedByActiveNodeAccount(ctx, h.mgr.Keeper(), msg.Signer); err != nil {
		return cosmos.ErrUnauthorized(err.Error())
	}

	u, err := k.GetProposedUpgrade(ctx, msg.Name)
	if err != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("failed to get proposed upgrade: %s", msg.Name))
	}

	if u == nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade proposal does not exist: %s", msg.Name))
	}

	v, err := k.GetUpgradeVote(ctx, msg.Signer, msg.Name)
	if err == nil && !v {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("upgrade already rejected: %s", msg.Name))
	}

	return nil
}

func (h RejectUpgradeHandler) handle(ctx cosmos.Context, msg *MsgRejectUpgrade) error {
	k := h.mgr.Keeper()
	name := msg.Name

	k.RejectUpgrade(ctx, msg.Signer, name)

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("reject_upgrade",
			cosmos.NewAttribute("thor_address", msg.Signer.String()),
			cosmos.NewAttribute("name", name),
		),
	)

	return clearUpgradeIfNecessary(ctx, k, name)
}

func scheduleUpgradeIfNecessary(ctx cosmos.Context, k keeper.Keeper, name string) error {
	upgradePlan, upgradePlanErr := k.GetUpgradePlan(ctx)
	if upgradePlanErr == nil && upgradePlan.Name == name {
		// already scheduled
		return nil
	}
	// Only ErrNoUpgradePlanFound is expected; any other error is a real store/read failure.
	if upgradePlanErr != nil && !errors.Is(upgradePlanErr, upgradetypes.ErrNoUpgradePlanFound) {
		return fmt.Errorf("failed to get upgrade plan: %w", upgradePlanErr)
	}

	u, err := k.GetProposedUpgrade(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get proposed upgrade: %w", err)
	}

	if u == nil {
		return fmt.Errorf("upgrade proposal not found: %s", name)
	}

	uq, err := keeperv1.UpgradeApprovedByMajority(ctx, k, name)
	if err != nil {
		return fmt.Errorf("failed to determine if upgrade is approved by majority threshold of validators: %w", err)
	}

	if uq.Approved {
		if upgradePlanErr == nil {
			// A different upgrade is already scheduled; allow the approval vote to
			// persist without scheduling. The upgrade can be scheduled later when
			// the blocking upgrade completes or is cleared.
			ctx.Logger().Info("upgrade approved by majority but a different upgrade is already scheduled", "name", name, "scheduled", upgradePlan.Name)
			return nil
		}

		// upgrade approval is now over the majority threshold
		return k.ScheduleUpgrade(ctx, upgradetypes.Plan{
			Name:   name,
			Height: u.Height,
			Info:   u.Info,
		})
	}

	return nil
}

func clearUpgradeIfNecessary(ctx cosmos.Context, k keeper.Keeper, name string) error {
	upgradePlan, err := k.GetUpgradePlan(ctx)
	if errors.Is(err, upgradetypes.ErrNoUpgradePlanFound) || (err == nil && upgradePlan.Name != name) {
		// upgrade by this name not scheduled
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get upgrade plan: %w", err)
	}

	uq, err := keeperv1.UpgradeApprovedByMajority(ctx, k, name)
	if err != nil {
		return fmt.Errorf("failed to determine if upgrade is approved by majority threshold of validators: %w", err)
	}

	if !uq.Approved {
		// upgrade approval dropped below the majority threshold. upgrade plan was on chain, so cancel it.
		if err := k.ClearUpgradePlan(ctx); err != nil {
			return fmt.Errorf("failed to clear upgrade plan: %w", err)
		}

		// Now that the plan is cleared, check if another proposal already has
		// majority approval and schedule it.
		return scheduleAnyApprovedUpgrade(ctx, k)
	}

	return nil
}

// scheduleAnyApprovedUpgrade iterates all proposals and schedules the first one
// that has majority approval and no plan already on chain. This covers the case
// where a proposal reached quorum while a different plan was active.
func scheduleAnyApprovedUpgrade(ctx cosmos.Context, k keeper.Keeper) error {
	iter := k.GetUpgradeProposalIterator(ctx)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		key, value := iter.Key(), iter.Value()

		nameSplit := strings.Split(string(key), "/")
		name := nameSplit[len(nameSplit)-1]

		var proposal types.UpgradeProposal
		if err := k.Cdc().Unmarshal(value, &proposal); err != nil {
			return fmt.Errorf("failed to unmarshal proposed upgrade: %w", err)
		}

		uq, err := keeperv1.UpgradeApprovedByMajority(ctx, k, name)
		if err != nil {
			return fmt.Errorf("failed to check upgrade quorum for %s: %w", name, err)
		}

		if !uq.Approved {
			continue
		}

		ctx.Logger().Info("scheduling previously approved upgrade after blocking plan cleared", "name", name)
		return k.ScheduleUpgrade(ctx, upgradetypes.Plan{
			Name:   name,
			Height: proposal.Height,
			Info:   proposal.Info,
		})
	}

	return nil
}

// ActiveValidatorAnteHandler called by the ante handler to gate mempool entry and
// also during deliver to only active validator nodes. Store changes will persist
// if this function succeeds, regardless of the success of the transaction.
func ActiveValidatorAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, signer cosmos.AccAddress) (cosmos.Context, error) {
	if err := signedByActiveNodeAccount(ctx, k, signer); err != nil {
		return ctx, err
	}

	return ctx, k.DeductNativeTxFeeFromBond(ctx, signer)
}
