package thorchain

import (
	"fmt"

	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

// ObservedTxOutHandler process MsgObservedTxOut messages
type ObservedTxOutHandler struct {
	mgr Manager
}

// NewObservedTxOutHandler create a new instance of ObservedTxOutHandler
func NewObservedTxOutHandler(mgr Manager) ObservedTxOutHandler {
	return ObservedTxOutHandler{
		mgr: mgr,
	}
}

// Run is the main entry point for ObservedTxOutHandler
func (h ObservedTxOutHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgObservedTxOut)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgObserveTxOut failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to handle MsgObserveTxOut", "error", err)
	}
	return result, err
}

func (h ObservedTxOutHandler) validate(ctx cosmos.Context, msg MsgObservedTxOut) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(fmt.Sprintf("%+v are not authorized", msg.GetSigners()))
	}

	return nil
}

// Handle a message to observe outbound tx
func (h ObservedTxOutHandler) handle(ctx cosmos.Context, msg MsgObservedTxOut) (*cosmos.Result, error) {
	k := h.mgr.Keeper()
	activeNodeAccounts, err := k.ListActiveValidators(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get list of active node accounts")
	}

	handler := NewInternalHandler(h.mgr)

	for _, tx := range msg.Txs {
		voter, err := ensureVaultAndGetTxOutVoter(ctx, k, tx.ObservedPubKey, tx.Tx.ID, msg.GetSigners(), tx.KeysignMs)
		if err != nil {
			ctx.Logger().Error("fail to ensure vault and get tx out voter", "error", err)
			continue
		}

		// check whether the tx has consensus
		voter, isQuorum := processTxOutAttestation(ctx, h.mgr, voter, activeNodeAccounts, tx, msg.Signer, true)

		if err := handleObservedTxOutQuorum(ctx, h.mgr, msg.Signer, activeNodeAccounts, handler, tx, voter, msg.GetSigners(), isQuorum); err != nil {
			ctx.Logger().Error("fail to handle observed tx out quorum", "error", err)
		}
	}
	return &cosmos.Result{}, nil
}

// ObservedTxOutAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func ObservedTxOutAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgObservedTxOut) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
