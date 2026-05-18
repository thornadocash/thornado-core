package thorchain

import (
	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

// NetworkFeeHandler a handler to process MsgNetworkFee messages
type NetworkFeeHandler struct {
	mgr Manager
}

// NewNetworkFeeHandler create a new instance of network fee handler
func NewNetworkFeeHandler(mgr Manager) NetworkFeeHandler {
	return NetworkFeeHandler{
		mgr: mgr,
	}
}

// Run is the main entry point for network fee logic
func (h NetworkFeeHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgNetworkFee)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgNetworkFee failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg)
	if err != nil {
		ctx.Logger().Error("fail to process MsgNetworkFee", "error", err)
	}
	return result, err
}

func (h NetworkFeeHandler) validate(ctx cosmos.Context, msg MsgNetworkFee) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	if !isSignedByActiveNodeAccounts(ctx, h.mgr.Keeper(), msg.GetSigners()) {
		return cosmos.ErrUnauthorized(errNotAuthorized.Error())
	}
	return nil
}

func (h NetworkFeeHandler) handle(ctx cosmos.Context, msg MsgNetworkFee) (*cosmos.Result, error) {
	active, err := h.mgr.Keeper().ListActiveValidators(ctx)
	if err != nil {
		err = wrapError(ctx, err, "fail to get list of active node accounts")
		return nil, err
	}

	voter, err := h.mgr.Keeper().GetObservedNetworkFeeVoter(ctx, msg.BlockHeight, msg.Chain, int64(msg.TransactionFeeRate), int64(msg.TransactionSize))
	if err != nil {
		return nil, err
	}

	defer func() {
		h.mgr.Keeper().SetObservedNetworkFeeVoter(ctx, voter)
	}()

	nf := &common.NetworkFee{
		Chain:           msg.Chain,
		Height:          msg.BlockHeight,
		TransactionSize: msg.TransactionSize,
		TransactionRate: msg.TransactionFeeRate,
	}

	if err := processNetworkFeeAttestation(ctx, h.mgr, &voter, msg.Signer, active, nf, true); err != nil {
		return nil, err
	}

	return &cosmos.Result{}, nil
}

// NetworkFeeAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func NetworkFeeAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgNetworkFee) (cosmos.Context, error) {
	return activeNodeAccountsSignerPriority(ctx, k, msg.GetSigners())
}
