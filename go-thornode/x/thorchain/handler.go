package thorchain

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// MsgHandler is an interface expect all handler to implement
type MsgHandler interface {
	Run(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error)
}

// NewInternalHandler returns a handler for "thorchain" internal type messages.
func NewInternalHandler(mgr Manager) cosmos.Handler {
	return func(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error) {
		handlerMap := getInternalHandlerMapping(mgr)
		h, ok := handlerMap[sdk.MsgTypeURL(msg)]
		if !ok {
			errMsg := fmt.Sprintf("Unrecognized thorchain Msg type: %v", sdk.MsgTypeURL(msg))
			return nil, cosmos.ErrUnknownRequest(errMsg)
		}

		// CacheContext() returns a context which caches all changes and only forwards
		// to the underlying context when commit() is called. Call commit() only when
		// the handler succeeds, otherwise return error and the changes will be discarded.
		// On commit, cached events also have to be explicitly emitted.
		cacheCtx, commit := ctx.CacheContext()
		res, err := h.Run(cacheCtx, msg)
		if err == nil {
			// Success, commit the cached changes and events
			commit()
		}

		return res, err
	}
}

func getInternalHandlerMapping(mgr Manager) map[string]MsgHandler {
	m := make(map[string]MsgHandler)
	m[sdk.MsgTypeURL(&MsgOutboundTx{})] = NewOutboundTxHandler(mgr)
	m[sdk.MsgTypeURL(&MsgBond{})] = NewBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgUnBond{})] = NewUnBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgReBond{})] = NewReBondHandler(mgr)
	m[sdk.MsgTypeURL(&MsgLeave{})] = NewLeaveHandler(mgr)
	m[sdk.MsgTypeURL(&MsgMaint{})] = NewMaintHandler(mgr)
	m[sdk.MsgTypeURL(&MsgRefundTx{})] = NewRefundHandler(mgr)
	m[sdk.MsgTypeURL(&MsgMigrate{})] = NewMigrateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgNoOp{})] = NewNoOpHandler(mgr)
	m[sdk.MsgTypeURL(&MsgConsolidate{})] = NewConsolidateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgOperatorRotate{})] = NewOperatorRotateHandler(mgr)
	return m
}

func getMsgReferenceMemo(memo ReferenceWriteMemo, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgReferenceMemo(memo.GetAsset(), memo.GetMemo(), signer), nil
}

func getMsgRefundFromMemo(memo RefundMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgRefundTx(tx, memo.GetTxID(), signer), nil
}

func getMsgOutboundFromMemo(memo OutboundMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgOutboundTx(tx, memo.GetTxID(), signer), nil
}

func getMsgMigrateFromMemo(memo MigrateMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgMigrate(tx, memo.GetBlockHeight(), signer), nil
}

func getMsgRagnarokFromMemo(memo RagnarokMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgRagnarok(tx, memo.GetBlockHeight(), signer), nil
}

func getMsgLeaveFromMemo(memo LeaveMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgLeave(tx.Tx, memo.GetAccAddress(), signer), nil
}

func getMsgBondFromMemo(memo BondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	coin := tx.Tx.Coins.GetCoin(common.RuneAsset())
	return NewMsgBond(tx.Tx, memo.GetAccAddress(), coin.Amount, tx.Tx.FromAddress, memo.BondProviderAddress, signer, memo.NodeOperatorFee), nil
}

func getMsgUnbondFromMemo(memo UnbondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgUnBond(tx.Tx, memo.GetAccAddress(), memo.GetAmount(), tx.Tx.FromAddress, memo.BondProviderAddress, signer), nil
}

func getMsgRebondFromMemo(memo RebondMemo, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return NewMsgReBond(tx.Tx, memo.GetNodeAddress(), memo.GetNewProviderAddress(), memo.GetAmount(), signer), nil
}

func getMsgMaintFromMemo(memo MaintMemo, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return types.NewMsgMaint(memo.GetAccAddress(), signer), nil
}

func processOneTxIn(ctx cosmos.Context, keeper keeper.Keeper, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	if len(tx.Tx.Coins) != 1 {
		return nil, cosmos.ErrInvalidCoins("only send 1 coins per message")
	}

	memo, err := ParseMemoWithTHORNames(ctx, keeper, tx.Tx.Memo)
	if err != nil {
		ctx.Logger().Error("fail to parse memo", "error", err)
		return nil, err
	}

	// THORNode should not have one tx across chain, if it is cross chain it should be separate tx
	var newMsg cosmos.Msg
	switch m := memo.(type) {
	case RefundMemo:
		newMsg, err = getMsgRefundFromMemo(m, tx, signer)
	case OutboundMemo:
		newMsg, err = getMsgOutboundFromMemo(m, tx, signer)
	case MigrateMemo:
		newMsg, err = getMsgMigrateFromMemo(m, tx, signer)
	case BondMemo:
		newMsg, err = getMsgBondFromMemo(m, tx, signer)
	case UnbondMemo:
		newMsg, err = getMsgUnbondFromMemo(m, tx, signer)
	case RebondMemo:
		newMsg, err = getMsgRebondFromMemo(m, tx, signer)
	case LeaveMemo:
		newMsg, err = getMsgLeaveFromMemo(m, tx, signer)
	case NoOpMemo:
		newMsg = NewMsgNoOp(tx, signer, m.Action)
	case ConsolidateMemo:
		newMsg = NewMsgConsolidate(tx, signer)
	case MaintMemo:
		newMsg, err = getMsgMaintFromMemo(m, signer)
	case OperatorRotateMemo:
		newMsg = NewMsgOperatorRotate(signer, m.OperatorAddress, tx.Tx.Coins[0])
	default:
		return nil, errInvalidMemo
	}

	if err != nil {
		return newMsg, err
	}

	newMsgV, ok := newMsg.(sdk.HasValidateBasic)
	if !ok {
		return newMsg, fmt.Errorf("msg does not implement sdk.HasValidateBasic: %T", newMsg)
	}

	return newMsg, newMsgV.ValidateBasic()
}
