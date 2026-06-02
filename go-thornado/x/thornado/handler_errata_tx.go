package thornado

import (
	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// ErrataTxHandler is to handle ErrataTx message
type ErrataTxHandler struct {
	mgr Manager
}

// NewErrataTxHandler create new instance of ErrataTxHandler
func NewErrataTxHandler(mgr Manager) ErrataTxHandler {
	return ErrataTxHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute ErrataTx logic
func (h ErrataTxHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgErrataTx)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg errata tx failed validation", "error", err)
		return nil, err
	}
	return h.handle(ctx, *msg)
}

func (h ErrataTxHandler) validate(ctx cosmos.Context, msg MsgErrataTx) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(errNotAuthorized.Error())
	}

	return nil
}

func (h ErrataTxHandler) handle(ctx cosmos.Context, msg MsgErrataTx) (*cosmos.Result, error) {
	ctx.Logger().Info("handleMsgErrataTx request", "txid", msg.TxID.String())
	active, err := h.mgr.Keeper().ListActiveNodes(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, "fail to get list of active node accounts")
	}

	voter, err := h.mgr.Keeper().GetErrataTxVoter(ctx, msg.TxID, msg.Chain)
	if err != nil {
		return nil, err
	}

	defer func() {
		h.mgr.Keeper().SetErrataTxVoter(ctx, voter)
	}()

	if err := processErrataTxAttestation(ctx, h.mgr, &voter, msg.Signer, active, &common.ErrataTx{Id: msg.TxID, Chain: msg.Chain}, true); err != nil {
		ctx.Logger().Error("fail to process errata tx attestation", "error", err)
		return nil, err
	}

	return &cosmos.Result{}, nil
}

// ErrataTxAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func ErrataTxAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgErrataTx) (cosmos.Context, error) {
	if err := msg.ValidateBasic(); err != nil {
		return ctx, err
	}
	newCtx, err := activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
	if err != nil {
		return ctx, err
	}
	voter, err := k.GetErrataTxVoter(ctx, msg.TxID, msg.Chain)
	if err != nil {
		return ctx, err
	}
	if err := reserveErrataTxAttestations(ctx, k, voter, msg.GetSigners()); err != nil {
		return ctx, err
	}
	return newCtx, nil
}
