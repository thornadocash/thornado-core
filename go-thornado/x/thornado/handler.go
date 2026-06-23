package thornado

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

// MsgHandler is an interface expect all handler to implement
type MsgHandler interface {
	Run(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error)
}

// NewInternalHandler returns a handler for "thornado" internal type messages.
func NewInternalHandler(mgr Manager) cosmos.Handler {
	return func(ctx cosmos.Context, msg cosmos.Msg) (*cosmos.Result, error) {
		handlerMap := getInternalHandlerMapping(mgr)
		h, ok := handlerMap[sdk.MsgTypeURL(msg)]
		if !ok {
			errMsg := fmt.Sprintf("Unrecognized thornado Msg type: %v", sdk.MsgTypeURL(msg))
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
	m[sdk.MsgTypeURL(&MsgMaint{})] = NewMaintHandler(mgr)
	m[sdk.MsgTypeURL(&MsgLeave{})] = NewLeaveHandler(mgr)
	m[sdk.MsgTypeURL(&MsgMigrate{})] = NewMigrateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgNoOp{})] = NewNoOpHandler(mgr)
	m[sdk.MsgTypeURL(&MsgConsolidate{})] = NewConsolidateHandler(mgr)
	m[sdk.MsgTypeURL(&MsgOperatorRotate{})] = NewOperatorRotateHandler(mgr)
	return m
}

func processOneTxIn(ctx cosmos.Context, keeper keeper.Keeper, tx ObservedTx, signer cosmos.AccAddress) (cosmos.Msg, error) {
	return nil, errInvalidMessage
}
