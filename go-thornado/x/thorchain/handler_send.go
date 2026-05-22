package thorchain

import (
	"context"
	"fmt"
	"strings"

	math "cosmossdk.io/math"
	"github.com/blang/semver"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

var _ types.MsgServer = (*BankSendHandler)(nil)

// BankSendHandler is a wrapper handler used to override msg routing for cosmos bank sends.
type BankSendHandler struct {
	h BaseHandler[sdk.Msg]
}

// NewBankSendHandler create a new instance of BankSendHandler
func NewBankSendHandler(h BaseHandler[sdk.Msg]) BankSendHandler {
	return BankSendHandler{h: h}
}

// Send is the entrypoint for bank MsgSend, passing through to the thornado handler.
func (h BankSendHandler) Send(goCtx context.Context, msg *bank.MsgSend) (*bank.MsgSendResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := h.h.Run(ctx, msg); err != nil {
		return nil, err
	}

	return &bank.MsgSendResponse{}, nil
}

// MultiSend not allowed by ante handler, but necessary to satisfy MsgServer interface for bank send bank.MsgMultiSend.
func (h BankSendHandler) MultiSend(ctx context.Context, msg *bank.MsgMultiSend) (*bank.MsgMultiSendResponse, error) {
	return &bank.MsgMultiSendResponse{}, nil
}

// UpdateParams defines a governance operation for updating the x/bank module parameters.
// The authority is defined in the keeper.
//
// Since: cosmos-sdk 0.47
func (h BankSendHandler) UpdateParams(context.Context, *bank.MsgUpdateParams) (*bank.MsgUpdateParamsResponse, error) {
	return &bank.MsgUpdateParamsResponse{}, nil
}

// SetSendEnabled is a governance operation for setting the SendEnabled flag
// on any number of Denoms. Only the entries to add or update should be
// included. Entries that already exist in the store, but that aren't
// included in this message, will be left unchanged.
//
// Since: cosmos-sdk 0.47
func (h BankSendHandler) SetSendEnabled(context.Context, *bank.MsgSetSendEnabled) (*bank.MsgSetSendEnabledResponse, error) {
	return &bank.MsgSetSendEnabledResponse{}, nil
}

// NewSendHandler create a new instance of SendHandler
func NewSendHandler(mgr Manager) BaseHandler[sdk.Msg] {
	return BaseHandler[sdk.Msg]{
		mgr:    mgr,
		logger: MsgSendLogger,
		validators: NewValidators[sdk.Msg]().
			Register("3.0.0", MsgSendValidate),
		handlers: NewHandlers[sdk.Msg]().
			Register("3.0.0", MsgSendHandle),
	}
}

func MsgSendLogger(ctx cosmos.Context, m sdk.Msg) {
	msg, err := getThorSend(m)
	if err != nil {
		return
	}

	ctx.Logger().Info("receive MsgSend", "from", msg.FromAddress, "to", msg.ToAddress, "coins", msg.Amount)
}

// getThorSend returns a thor MsgSend from either a thor MsgSend or a x/bank MsgSend
func getThorSend(msg sdk.Msg) (*MsgSend, error) {
	switch msg := msg.(type) {
	case *MsgSend:
		return msg, nil
	case *bank.MsgSend:
		fromAddress, err := cosmos.AccAddressFromBech32(msg.FromAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to parse from address: %s", msg.FromAddress)
		}

		toAddress, err := cosmos.AccAddressFromBech32(msg.ToAddress)
		if err != nil {
			return nil, fmt.Errorf("fail to parse to address: %s", msg.ToAddress)
		}

		return &MsgSend{
			FromAddress: fromAddress,
			ToAddress:   toAddress,
			Amount:      msg.Amount,
		}, nil
	default:
		return nil, fmt.Errorf("not a supported send message type: %T", msg)
	}
}

func getDeposit(ctx cosmos.Context, fromAddress sdk.AccAddress, sdkCoins sdk.Coins) (*MsgDeposit, error) {
	var memo string
	ctxTxMemo := ctx.Context().Value(ContextKeyTxMemo)
	if ctxTxMemo != nil {
		if m, ok := ctxTxMemo.(string); ok {
			memo = m
		}
	}
	coins := make(common.Coins, len(sdkCoins))
	for i, coin := range sdkCoins {
		asset, err := common.NewAsset(
			// Strip x/denom denom string prefix to force asset.NewAsset
			// to interpret custom denom strings as native assets
			strings.TrimPrefix(coin.Denom, "x/"),
		)
		if err != nil {
			return nil, err
		}

		coins[i] = common.Coin{
			Asset:  asset,
			Amount: math.NewUintFromBigInt(coin.Amount.BigInt()),
		}
	}
	return NewMsgDeposit(coins, memo, fromAddress), nil
}

// SendAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func SendAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, m sdk.Msg) (cosmos.Context, error) {
	msg, err := getThorSend(m)
	if err != nil {
		return ctx, err
	}
	return ctx, k.DeductNativeTxFeeFromAccount(ctx, msg.GetSigners()[0])
}

func MsgSendValidate(ctx cosmos.Context, mgr Manager, m sdk.Msg) error {
	msg, err := getThorSend(m)
	if err != nil {
		return err
	}

	if err := msg.ValidateBasic(); err != nil {
		return err
	}

	k := mgr.Keeper()
	isThorModAddr := msg.ToAddress.Equals(k.GetModuleAccAddress(ModuleName))

	// Allow MsgSend with memo-based MsgDeposit is allowed to the thor module address
	if isThorModAddr {
		msgDeposit, err := getDeposit(ctx, msg.GetSigners()[0], msg.Amount)
		if err != nil {
			return err
		}
		return NewDepositHandler(mgr).validate(ctx, *msgDeposit)
	}
	// disallow sends to other modules, they should only be interacted with via deposit messages.
	if IsModuleAccAddress(k, msg.ToAddress) {
		return fmt.Errorf("cannot use MsgSend for Module transactions, use MsgDeposit instead")
	}
	// Having been confirmed to be for MsgSend and not MsgDeposit, do the Cosmos-SDK Coins IsValid check.
	// (This implicitly includes IsAllPositive.)
	if !msg.Amount.IsValid() {
		return cosmos.ErrInvalidCoins("coins must be valid")
	}
	return nil
}

func MsgSendHandle(ctx cosmos.Context, mgr Manager, m sdk.Msg) (*cosmos.Result, error) {
	msg, err := getThorSend(m)
	if err != nil {
		return nil, err
	}

	k := mgr.Keeper()

	if k.IsChainHalted(ctx, common.Thornado) {
		return nil, fmt.Errorf("unable to use MsgSend while Thornado is halted")
	}

	// MsgSend to the thornado module address is treated as a MsgDeposit for client compatibility reasons.
	// In this case, the memo will be used like in any other MsgDeposit.
	if msg.ToAddress.Equals(k.GetModuleAccAddress(ModuleName)) {
		msgDeposit, err := getDeposit(ctx, msg.FromAddress, msg.Amount)
		if err != nil {
			return nil, err
		}

		return NewDepositHandler(mgr).handle(ctx, *msgDeposit, 0)
	} else if err := k.SendCoins(ctx, msg.FromAddress, msg.ToAddress, msg.Amount); err != nil {
		return nil, err
	}

	return &cosmos.Result{}, nil
}
