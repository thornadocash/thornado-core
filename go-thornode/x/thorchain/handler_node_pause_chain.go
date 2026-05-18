package thorchain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

// NodePauseChainHandler is to handle node pause chain messages
type NodePauseChainHandler struct {
	mgr Manager
}

// NewNodePauseChainHandler create new instance of NodePauseChainHandler
func NewNodePauseChainHandler(mgr Manager) NodePauseChainHandler {
	return NodePauseChainHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute node pause chain logic
func (h NodePauseChainHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgNodePauseChain)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive node pause chain", "node", msg.Signer, "value", msg.Value)
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg node pause chain failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg set node pause chain", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

func (h NodePauseChainHandler) validate(ctx cosmos.Context, msg MsgNodePauseChain) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(fmt.Sprintf("%+v are not authorized", msg.GetSigners()))
	}

	return nil
}

func (h NodePauseChainHandler) handle(ctx cosmos.Context, msg MsgNodePauseChain) error {
	ctx.Logger().Info("handleMsgNodePauseChain request", "node", msg.Signer, "value", msg.Value)
	// get block height of last churn
	active, err := h.mgr.Keeper().GetAsgardVaultsByStatus(ctx, ActiveVault)
	if err != nil {
		return err
	}
	lastChurn := int64(-1)
	for _, vault := range active {
		if vault.StatusSince > lastChurn {
			lastChurn = vault.StatusSince
		}
	}

	// check that node hasn't used this handler since the last churn already
	nodeHeight := h.mgr.Keeper().GetNodePauseChain(ctx, msg.Signer)
	if nodeHeight > 0 && nodeHeight >= lastChurn {
		return fmt.Errorf("node has already chosen pause/resume since the last churn")
	}

	// get the current block height set by node pause chain global
	key := "NodePauseChainGlobal"
	pauseHeight, err := h.mgr.Keeper().GetMimir(ctx, key)
	if err != nil {
		return err
	}

	blocks := h.mgr.Keeper().GetConfigInt64(ctx, constants.NodePauseChainBlocks)

	if msg.Value > 0 { // node intends to pause chain
		if pauseHeight > ctx.BlockHeight() { // chain is paused
			pauseHeight += blocks
			h.mgr.Keeper().SetNodePauseChain(ctx, msg.Signer)
		} else { // chain isn't paused
			pauseHeight = ctx.BlockHeight() + blocks
			h.mgr.Keeper().SetNodePauseChain(ctx, msg.Signer)
		}
	} else { // node intends to resume chain
		if pauseHeight <= ctx.BlockHeight() {
			// chain isn't paused, so don't do anything
			return nil
		}
		h.mgr.Keeper().SetNodePauseChain(ctx, msg.Signer)
		pauseHeight -= blocks
	}

	h.mgr.Keeper().SetMimir(ctx, key, pauseHeight)
	mimirEvent := NewEventSetMimir(strings.ToUpper(key), strconv.FormatInt(pauseHeight, 10))
	if err = h.mgr.EventMgr().EmitEvent(ctx, mimirEvent); err != nil {
		ctx.Logger().Error("fail to emit set_mimir event", "error", err)
	}

	return nil
}

// NodePauseChainAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func NodePauseChainAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgNodePauseChain) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
