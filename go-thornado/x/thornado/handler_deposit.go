package thornado

import (
	"fmt"

	"github.com/blang/semver"
	se "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// DepositHandler is to process native messages on Thornado
type DepositHandler struct {
	mgr Manager
}

// NewDepositHandler create a new instance of DepositHandler
func NewDepositHandler(mgr Manager) DepositHandler {
	return DepositHandler{
		mgr: mgr,
	}
}

// Run is the main entry of DepositHandler
func (h DepositHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgDeposit)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("MsgDeposit failed validation", "error", err)
		return nil, err
	}
	result, err := h.handle(ctx, *msg, 0)
	if err != nil {
		ctx.Logger().Error("fail to process MsgDeposit", "error", err)
		return nil, err
	}
	return result, nil
}

func (h DepositHandler) validate(ctx cosmos.Context, msg MsgDeposit) error {
	// ValidateBasic is also executed in message service router's handler and isn't versioned there
	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	return nil
}

func (h DepositHandler) handle(ctx cosmos.Context, msg MsgDeposit, idx uint16) (*cosmos.Result, error) {
	if h.mgr.Keeper().IsChainHalted(ctx, common.Thornado) {
		return nil, fmt.Errorf("unable to use MsgDeposit while Thornado is halted")
	}

	coins, err := msg.Coins.Native()
	if err != nil {
		return nil, ErrInternal(err, "coins are native to Thornado")
	}

	// HasCoins always returns false if the address has no balances
	// (such as if having had just enough for the network fee),
	// so check first whether there is any non-zero Amount necessary.
	if !msg.Coins.IsEmpty() && !h.mgr.Keeper().HasCoins(ctx, msg.GetSigners()[0], coins) {
		return nil, se.ErrInsufficientFunds
	}

	return nil, fmt.Errorf("MsgDeposit memo routing is disabled in Thornado; use explicit Shielder or node-management messages")
}

// DepositAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func DepositAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgDeposit) (cosmos.Context, error) {
	return ctx, k.DeductNativeTxFeeFromAccount(ctx, msg.GetSigners()[0])
}
