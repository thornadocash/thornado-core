package thornado

import (
	math "math"

	"github.com/blang/semver"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256r1"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const ActiveNodePriority = int64(math.MaxInt64)

type AnteDecorator struct {
	keeper keeper.Keeper
}

func NewAnteDecorator(keeper keeper.Keeper) AnteDecorator {
	return AnteDecorator{
		keeper: keeper,
	}
}

func (ad AnteDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	if err = ad.rejectMultipleDepositMsgs(tx.GetMsgs()); err != nil {
		return ctx, err
	}

	// TODO remove on hard fork, when all signers will be allowed (v47+)
	if err = ad.rejectInvalidSigners(tx); err != nil {
		return ctx, err
	}

	// run the message-specific ante for each msg, all must succeed
	version, _ := ad.keeper.GetVersionWithCtx(ctx)
	for _, msg := range tx.GetMsgs() {
		newCtx, err = ad.anteHandleMessage(ctx, version, msg)
		if err != nil {
			return ctx, err
		}
	}

	return next(newCtx, tx, simulate)
}

// rejectInvalidSigners reject txs if they are signed with secp256r1 keys
func (ad AnteDecorator) rejectInvalidSigners(tx sdk.Tx) error {
	sigTx, okTx := tx.(authsigning.SigVerifiableTx)
	if !okTx {
		return cosmos.ErrUnknownRequest("invalid transaction type")
	}
	sigs, err := sigTx.GetSignaturesV2()
	if err != nil {
		return err
	}
	for _, sig := range sigs {
		pubkey := sig.PubKey
		switch pubkey := pubkey.(type) {
		case *secp256r1.PubKey:
			return cosmos.ErrUnknownRequest("secp256r1 keys not allowed")
		case multisig.PubKey:
			for _, pk := range pubkey.GetPubKeys() {
				if _, okPk := pk.(*secp256r1.PubKey); okPk {
					return cosmos.ErrUnknownRequest("secp256r1 keys not allowed")
				}
			}
		}
	}
	return nil
}

// rejectMultipleDepositMsgs only one deposit msg allowed per tx
func (ad AnteDecorator) rejectMultipleDepositMsgs(msgs []cosmos.Msg) error {
	hasDeposit := false
	for _, msg := range msgs {
		if ad.isDeposit(msg) {
			if hasDeposit {
				return cosmos.ErrUnknownRequest("only one deposit msg per tx")
			}
			hasDeposit = true
		}
	}
	return nil
}

// isDeposit returns true if the msg is a deposit
func (ad AnteDecorator) isDeposit(msg cosmos.Msg) bool {
	switch m := msg.(type) {
	case *types.MsgDeposit:
		return true
	case *types.MsgSend:
		return m.ToAddress.Equals(ad.keeper.GetModuleAccAddress(ModuleName))
	case *banktypes.MsgSend:
		return m.ToAddress == ad.keeper.GetModuleAccAddress(ModuleName).String()
	default:
		return false
	}
}

// anteHandleMessage calls the msg-specific ante handling for a given msg
func (ad AnteDecorator) anteHandleMessage(ctx sdk.Context, version semver.Version, msg cosmos.Msg) (sdk.Context, error) {
	// ideally each handler would impl an ante func and we could instantiate
	// handlers and call ante, but handlers require mgr which is unavailable
	switch m := msg.(type) {

	// consensus handlers
	case *types.MsgBan:
		return BanAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgErrataTx:
		return ErrataTxAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgNetworkFee:
		return NetworkFeeAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgObservedTxIn:
		return ObservedTxInAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgObservedTxOut:
		return ObservedTxOutAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSolvency:
		return SolvencyAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgTssPool:
		return TssAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgTssKeysignFail:
		return TssKeysignFailAnteHandler(ctx, version, ad.keeper, *m)

	// cli handlers (non-consensus)
	case *types.MsgSetIPAddress:
		return IPAddressAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgConfig:
		return ConfigAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgNodePauseChain:
		return NodePauseChainAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSetNodeKeys:
		return SetNodeKeysAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSetVersion:
		return VersionAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgProposeUpgrade, *types.MsgApproveUpgrade, *types.MsgRejectUpgrade:
		legacyMsg, ok := msg.(sdk.LegacyMsg)
		if !ok {
			return ctx, cosmos.ErrUnknownRequest("invalid message type")
		}
		return ActiveNodeAnteHandler(ctx, version, ad.keeper, legacyMsg.GetSigners()[0])

	// native handlers (non-consensus)
	case *types.MsgDeposit:
		return DepositAnteHandler(ctx, version, ad.keeper, *m)
	case *types.MsgSend, *banktypes.MsgSend:
		_, ok := m.(*banktypes.MsgSend)
		if ok {
			return ctx, cosmos.ErrUnknownRequest("bank sends are disabled")
		}
		return SendAnteHandler(ctx, version, ad.keeper, m)
	case *types.MsgDepositRequestPow,
		*types.MsgShielderSplit,
		*types.MsgShielderRedeem,
		*types.MsgShielderSplitFees,
		*types.MsgNodeSlotAuctionCreate,
		*types.MsgNodeSlotAuctionBidPow,
		*types.MsgNodeSlotAuctionSelectBid,
		*types.MsgNodeSlotAuctionSplit:
		return ctx, nil
	case *authz.MsgGrant,
		*authz.MsgRevoke,
		*authz.MsgExec:
		return ctx, nil
	default:
		return ctx, cosmos.ErrUnknownRequest("invalid message type")
	}
}

// InfiniteGasDecorator uses an infinite gas meter to prevent out-of-gas panics
// and allow non-versioned changes to be made without breaking consensus,
// as long as the resulting state is consistent.
type InfiniteGasDecorator struct {
	keeper keeper.Keeper
}

func NewGasDecorator(keeper keeper.Keeper) InfiniteGasDecorator {
	return InfiniteGasDecorator{
		keeper: keeper,
	}
}

func (d InfiniteGasDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	return next(ctx, tx, simulate)
}
