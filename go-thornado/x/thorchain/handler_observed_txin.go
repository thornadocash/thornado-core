package thorchain

import (
	"fmt"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

// ObservedTxInHandler to handle MsgObservedTxIn
type ObservedTxInHandler struct {
	mgr Manager
}

// NewObservedTxInHandler create a new instance of ObservedTxInHandler
func NewObservedTxInHandler(mgr Manager) ObservedTxInHandler {
	return ObservedTxInHandler{
		mgr: mgr,
	}
}

// Run is the main entry point of ObservedTxInHandler
func (h ObservedTxInHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgObservedTxIn)
	if !ok {
		return nil, errInvalidMessage
	}
	err := h.validate(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("MsgObservedTxIn failed validation", "error", err)
		return nil, err
	}

	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to handle MsgObservedTxIn message", "error", err)
	}
	return result, err
}

func (h ObservedTxInHandler) validate(ctx cosmos.Context, msg MsgObservedTxIn) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(fmt.Sprintf("%+v are not authorized", msg.GetSigners()))
	}

	return nil
}

func (h ObservedTxInHandler) handle(ctx cosmos.Context, msg MsgObservedTxIn) (*cosmos.Result, error) {
	activeNodeAccounts, err := h.mgr.Keeper().ListActiveValidators(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get list of active node accounts")
	}
	handler := NewInternalHandler(h.mgr)
	for _, tx := range msg.Txs {
		voter, err := ensureVaultAndGetTxInVoter(ctx, tx.ObservedPubKey, tx.Tx.ID, h.mgr.Keeper())
		if err != nil {
			ctx.Logger().Error("fail to ensure vault and get tx in voter", "error", err)
			continue
		}

		voter, isQuorum := processTxInAttestation(ctx, h.mgr, voter, activeNodeAccounts, tx, msg.Signer, true)
		if err := handleObservedTxInQuorum(ctx, h.mgr, msg.Signer, activeNodeAccounts, handler, tx, voter, voter.Tx.GetSigners(), isQuorum); err != nil {
			ctx.Logger().Error("fail to handle observed tx in quorum", "error", err, "tx", tx.Tx.ID)
		}
	}
	return &cosmos.Result{}, nil
}

// ObservedTxInAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func ObservedTxInAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgObservedTxIn) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
