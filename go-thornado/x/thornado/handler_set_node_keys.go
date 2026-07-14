package thornado

import (
	"fmt"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// SetNodeKeysHandler process MsgSetNodeKeys
// MsgSetNodeKeys is used by operators after the node account had been white list , to update the consensus pubkey and node account pubkey
type SetNodeKeysHandler struct {
	mgr Manager
}

// NewSetNodeKeysHandler create a new instance of SetNodeKeysHandler
func NewSetNodeKeysHandler(mgr Manager) SetNodeKeysHandler {
	return SetNodeKeysHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to process MsgSetNodeKeys
func (h SetNodeKeysHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgSetNodeKeys)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgSetNodeKeys failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgSetNodeKey", "error", err)
	}
	return result, err
}

func (h SetNodeKeysHandler) validate(ctx cosmos.Context, msg MsgSetNodeKeys) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	nodeAccount, err := validateNodeKeysAuth(ctx, h.mgr.Keeper(), msg.Signer)
	if err != nil {
		return err
	}
	// A node with keys assigned may re-submit the IDENTICAL pubkey set to
	// rotate only its consensus key (lost/regenerated validator key on a
	// standby node — Active/Disabled are already rejected above). A different
	// pubkey set is still refused: node identity is immutable once assigned.
	if !nodeAccount.PubKeySet.IsEmpty() && !nodeAccount.PubKeySet.Equals(msg.PubKeySetSet) {
		return fmt.Errorf("node %s already has pubkey set assigned", nodeAccount.NodeAddress)
	}
	if err := h.mgr.Keeper().EnsureNodeKeysUnique(ctx, nodeAccount.NodeAddress, msg.NodeConsPubKey, msg.PubKeySetSet); err != nil {
		return err
	}

	addr, err := msg.PubKeySetSet.Secp256k1.GetThorAddress()
	if err != nil {
		return err
	}
	if !nodeAccount.NodeAddress.Equals(addr) {
		return fmt.Errorf("node address must match secp256k1 pubkey")
	}

	return nil
}

func (h SetNodeKeysHandler) handle(ctx cosmos.Context, msg MsgSetNodeKeys) (*cosmos.Result, error) {
	ctx.Logger().Info("handleMsgSetNodeKeys request")
	nodeAccount, err := resolveNodeAccountBySigner(ctx, h.mgr.Keeper(), msg.Signer)
	if err != nil {
		ctx.Logger().Error("fail to get node account", "error", err, "address", msg.Signer.String())
		return nil, cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", msg.Signer))
	}

	nodeAccount.UpdateStatus(NodeStandby, ctx.BlockHeight())
	nodeAccount.PubKeySet = msg.PubKeySetSet
	nodeAccount.NodeConsPubKey = msg.NodeConsPubKey
	if err := h.mgr.Keeper().SetNodeAccount(ctx, nodeAccount); err != nil {
		return nil, fmt.Errorf("fail to save node account: %w", err)
	}

	ctx.EventManager().EmitEvent(
		cosmos.NewEvent("set_node_keys",
			cosmos.NewAttribute("node_address", nodeAccount.NodeAddress.String()),
			cosmos.NewAttribute("node_secp256k1_pubkey", msg.PubKeySetSet.Secp256k1.String()),
			cosmos.NewAttribute("node_ed25519_pubkey", msg.PubKeySetSet.Ed25519.String()),
			cosmos.NewAttribute("node_consensus_pub_key", msg.NodeConsPubKey)))

	return &cosmos.Result{}, nil
}

func validateNodeKeysAuth(ctx cosmos.Context, k keeper.Keeper, signer cosmos.AccAddress) (NodeAccount, error) {
	nodeAccount, err := resolveNodeAccountBySigner(ctx, k, signer)
	if err != nil {
		return NodeAccount{}, cosmos.ErrUnauthorized(fmt.Sprintf("fail to get node account(%s):%s", signer.String(), err)) // notAuthorized
	}

	// You should not able to update node address when the node is active
	// for example if they update observer address
	if nodeAccount.Status == NodeActive {
		return NodeAccount{}, fmt.Errorf("node %s is active, so it can't update itself", nodeAccount.NodeAddress)
	}
	if nodeAccount.Status == NodeDisabled {
		return NodeAccount{}, fmt.Errorf("node %s is disabled, so it can't update itself", nodeAccount.NodeAddress)
	}

	// The assigned-keys check lives in the handler's validate(), where the
	// message is available: an identical pubkey set is allowed through so a
	// standby node can rotate a lost consensus key.

	return nodeAccount, nil
}

// SetNodeKeysAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func SetNodeKeysAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgSetNodeKeys) (cosmos.Context, error) {
	if _, err := validateNodeKeysAuth(ctx, k, msg.Signer); err != nil {
		return ctx, err
	}

	return ctx, nil
}
