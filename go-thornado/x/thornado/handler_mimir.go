package thornado

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

var mimirValidKey = regexp.MustCompile(constants.MimirKeyRegex).MatchString

// MimirHandler is to handle mimir messages
type MimirHandler struct {
	mgr Manager
}

// NewMimirHandler create new instance of MimirHandler
func NewMimirHandler(mgr Manager) MimirHandler {
	return MimirHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute mimir logic
func (h MimirHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgMimir)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg mimir failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg set mimir", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h MimirHandler) validate(ctx cosmos.Context, msg MsgMimir) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !mimirValidKey(msg.Key) || len(msg.Key) > constants.MaxMimirLength {
		return cosmos.ErrUnknownRequest("invalid mimir key")
	}
	if _, err := validateMimirAuth(ctx, h.mgr.Keeper(), msg); err != nil {
		return err
	}

	return nil
}

func (h MimirHandler) handle(ctx cosmos.Context, msg MsgMimir) error {
	ctx.Logger().Info("handleMsgMimir request", "node", msg.Signer, "key", msg.Key, "value", msg.Value)

	// Get the current Mimir key value if it exists.
	currentMimirValue, _ := h.mgr.Keeper().GetMimir(ctx, msg.Key)
	// Here, an error is assumed to mean the Mimir key is currently unset.

	// Cost and emitting of SetNodeMimir, even if a duplicate
	// (for instance if needed to confirm a new supermajority after a node number decrease).
	nodeAccount, err := h.mgr.Keeper().GetNodeAccount(ctx, msg.Signer)
	if err != nil {
		ctx.Logger().Error("fail to get node account", "error", err, "address", msg.Signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", msg.Signer))
	}
	cost := h.mgr.Keeper().GetNativeTxFee(ctx)
	if cost.GT(nodeAccount.Bond) {
		cost = nodeAccount.Bond
	}
	nodeAccount.Bond = common.SafeSub(nodeAccount.Bond, cost)
	if err = h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		ctx.Logger().Error("fail to save node account", "error", err)
		return fmt.Errorf("fail to save node account: %w", err)
	}
	// move set mimir cost from bond module to reserve
	coin := common.NewCoin(common.RuneNative, cost)
	if !cost.IsZero() {
		if err = h.mgr.Keeper().SendFromModuleToModule(ctx, BondName, ReserveName, common.NewCoins(coin)); err != nil {
			ctx.Logger().Error("fail to transfer funds from bond to reserve", "error", err)
			return err
		}
	}
	if err = h.mgr.Keeper().SetNodeMimir(ctx, msg.Key, msg.Value, msg.Signer); err != nil {
		ctx.Logger().Error("fail to save node mimir", "error", err)
		return err
	}
	nodeMimirEvent := NewEventSetNodeMimir(strings.ToUpper(msg.Key), strconv.FormatInt(msg.Value, 10), msg.Signer.String())
	if err = h.mgr.EventMgr().EmitEvent(ctx, nodeMimirEvent); err != nil {
		ctx.Logger().Error("fail to emit set_node_mimir event", "error", err)
		return err
	}
	bondEvent := NewEventBond(cost, BondCost, common.Tx{}, &nodeAccount, nil)
	if err = h.mgr.EventMgr().EmitEvent(ctx, bondEvent); err != nil {
		ctx.Logger().Error("fail to emit bond event", "error", err)
		return err
	}

	// If the Mimir key is already the submitted value, don't do anything further.
	if msg.Value == currentMimirValue {
		return nil
	}

	nodeMimirs, err := h.mgr.Keeper().GetNodeMimirs(ctx, msg.Key)
	if err != nil {
		ctx.Logger().Error("fail to get node mimirs", "error", err)
		return err
	}
	activeNodes, err := h.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to list active nodes", "error", err)
		return err
	}

	var effectiveValue int64
	if h.mgr.Keeper().IsOperationalMimir(msg.Key) {
		// A value of -1 indicates either a tie or that no values satisfy the required minimum votes.
		operationalVotesMin := h.mgr.Keeper().GetConfigInt64(ctx, constants.OperationalVotesMin)
		effectiveValue = nodeMimirs.ValueOfOperational(msg.Key, operationalVotesMin, activeNodes.GetNodeAddresses())
	} else {
		// Economic Mimir, so require supermajority to set.
		var currentlyHasSuperMajority bool
		effectiveValue, currentlyHasSuperMajority = nodeMimirs.HasSuperMajority(msg.Key, activeNodes.GetNodeAddresses())
		if !currentlyHasSuperMajority {
			effectiveValue = -1
		}
	}
	// If the effective value is negative (used to signal no effective value), change nothing.
	if effectiveValue < 0 {
		return nil
	}
	// If the current Mimir value is already the effective value, change nothing.
	if currentMimirValue == effectiveValue {
		return nil
	}
	// If the MsgMimir value doesn't match the effective value, change nothing.
	if msg.Value != effectiveValue {
		return nil
	}
	// Reaching this point indicates a new mimir value is to be set.
	h.mgr.Keeper().SetMimir(ctx, msg.Key, effectiveValue)
	mimirEvent := NewEventSetMimir(strings.ToUpper(msg.Key), strconv.FormatInt(effectiveValue, 10))
	if err = h.mgr.EventMgr().EmitEvent(ctx, mimirEvent); err != nil {
		return fmt.Errorf("fail to emit set_mimir event: %w", err)
	}

	return nil
}

func validateMimirAuth(ctx cosmos.Context, k keeper.Keeper, msg MsgMimir) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}

// MimirAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func MimirAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgMimir) (cosmos.Context, error) {
	return validateMimirAuth(ctx, k, msg)
}
