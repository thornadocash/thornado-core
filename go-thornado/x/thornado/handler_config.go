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

var configValidKey = regexp.MustCompile(constants.ConfigKeyRegex).MatchString

// ConfigHandler is to handle config messages
type ConfigHandler struct {
	mgr Manager
}

// NewConfigHandler create new instance of ConfigHandler
func NewConfigHandler(mgr Manager) ConfigHandler {
	return ConfigHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute config logic
func (h ConfigHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgConfig)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg config failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg set config", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h ConfigHandler) validate(ctx cosmos.Context, msg MsgConfig) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !configValidKey(msg.Key) || len(msg.Key) > constants.MaxConfigLength {
		return cosmos.ErrUnknownRequest("invalid config key")
	}
	if _, err := validateConfigAuth(ctx, h.mgr.Keeper(), msg); err != nil {
		return err
	}

	return nil
}

func (h ConfigHandler) handle(ctx cosmos.Context, msg MsgConfig) error {
	ctx.Logger().Info("handleMsgConfig request", "node", msg.Signer, "key", msg.Key, "value", msg.Value)

	// Get the current Config key value if it exists.
	currentConfigValue, _ := h.mgr.Keeper().GetConfig(ctx, msg.Key)
	// Here, an error is assumed to mean the Config key is currently unset.

	// Cost and emitting of SetNodeConfig, even if a duplicate
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
	// move set config cost from bond module to reserve
	coin := common.NewCoin(common.RuneNative, cost)
	if !cost.IsZero() {
		if err = h.mgr.Keeper().SendFromModuleToModule(ctx, BondName, ReserveName, common.NewCoins(coin)); err != nil {
			ctx.Logger().Error("fail to transfer funds from bond to reserve", "error", err)
			return err
		}
	}
	if err = h.mgr.Keeper().SetNodeConfig(ctx, msg.Key, msg.Value, msg.Signer); err != nil {
		ctx.Logger().Error("fail to save node config", "error", err)
		return err
	}
	nodeConfigEvent := NewEventSetNodeConfig(strings.ToUpper(msg.Key), strconv.FormatInt(msg.Value, 10), msg.Signer.String())
	if err = h.mgr.EventMgr().EmitEvent(ctx, nodeConfigEvent); err != nil {
		ctx.Logger().Error("fail to emit set_node_config event", "error", err)
		return err
	}
	bondEvent := NewEventBond(cost, BondCost, common.Tx{}, &nodeAccount, nil)
	if err = h.mgr.EventMgr().EmitEvent(ctx, bondEvent); err != nil {
		ctx.Logger().Error("fail to emit bond event", "error", err)
		return err
	}

	// If the Config key is already the submitted value, don't do anything further.
	if msg.Value == currentConfigValue {
		return nil
	}

	nodeConfigs, err := h.mgr.Keeper().GetNodeConfigs(ctx, msg.Key)
	if err != nil {
		ctx.Logger().Error("fail to get node configs", "error", err)
		return err
	}
	activeNodes, err := h.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		ctx.Logger().Error("fail to list active nodes", "error", err)
		return err
	}

	var effectiveValue int64
	if h.mgr.Keeper().IsOperationalConfig(msg.Key) {
		// A value of -1 indicates either a tie or that no values satisfy the required minimum votes.
		operationalVotesMin := h.mgr.Keeper().GetConfigInt64(ctx, constants.Config_OperationalVotesMin)
		effectiveValue = nodeConfigs.ValueOfOperational(msg.Key, operationalVotesMin, activeNodes.GetNodeAddresses())
	} else {
		// Economic Config, so require supermajority to set.
		var currentlyHasSuperMajority bool
		effectiveValue, currentlyHasSuperMajority = nodeConfigs.HasSuperMajority(msg.Key, activeNodes.GetNodeAddresses())
		if !currentlyHasSuperMajority {
			effectiveValue = -1
		}
	}
	// If the effective value is negative (used to signal no effective value), change nothing.
	if effectiveValue < 0 {
		return nil
	}
	// If the current Config value is already the effective value, change nothing.
	if currentConfigValue == effectiveValue {
		return nil
	}
	// If the MsgConfig value doesn't match the effective value, change nothing.
	if msg.Value != effectiveValue {
		return nil
	}
	// Reaching this point indicates a new config value is to be set.
	h.mgr.Keeper().SetConfig(ctx, msg.Key, effectiveValue)
	configEvent := NewEventSetConfig(strings.ToUpper(msg.Key), strconv.FormatInt(effectiveValue, 10))
	if err = h.mgr.EventMgr().EmitEvent(ctx, configEvent); err != nil {
		return fmt.Errorf("fail to emit set_config event: %w", err)
	}

	return nil
}

func validateConfigAuth(ctx cosmos.Context, k keeper.Keeper, msg MsgConfig) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}

// ConfigAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func ConfigAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgConfig) (cosmos.Context, error) {
	return validateConfigAuth(ctx, k, msg)
}
