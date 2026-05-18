package ebifrost

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type InjectedTxDecorator struct{}

func NewInjectedTxDecorator() InjectedTxDecorator {
	return InjectedTxDecorator{}
}

func (itd InjectedTxDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	if _, ok := tx.(wInjectTx); !ok {
		for _, m := range tx.GetMsgs() {
			switch m.(type) {
			case *types.MsgObservedTxQuorum, *types.MsgNetworkFeeQuorum, *types.MsgSolvencyQuorum, *types.MsgErrataTxQuorum, *types.MsgPriceFeedQuorumBatch:
				// only allowed through an InjectTx, fail.
				return ctx, cosmos.ErrUnauthorized(fmt.Sprintf("msg only allowed via proposal inject tx: %T", m))
			default:
				// proceed
			}
		}

		return next(ctx, tx, simulate)
	}

	// only allow if we are in deliver tx (only way is via proposer injected tx)
	if ctx.IsCheckTx() || ctx.IsReCheckTx() || simulate {
		return ctx, cosmos.ErrUnauthorized("inject txs only allowed via proposal")
	}

	msgs := tx.GetMsgs()

	if len(msgs) != 1 {
		return ctx, cosmos.ErrUnauthorized("inject txs only allowed with 1 msg")
	}

	// make sure entire tx is only allowed msgs
	for _, m := range msgs {
		switch m.(type) {
		case *types.MsgObservedTxQuorum, *types.MsgNetworkFeeQuorum, *types.MsgSolvencyQuorum, *types.MsgErrataTxQuorum, *types.MsgPriceFeedQuorumBatch:
			// allowed

		default:
			return ctx, cosmos.ErrUnauthorized(fmt.Sprintf("invalid inject tx message type: %T", m))
		}
	}

	// skip rest of antes
	return ctx, nil
}
