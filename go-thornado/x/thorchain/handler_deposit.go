package thorchain

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/blang/semver"
	tmtypes "github.com/cometbft/cometbft/types"
	se "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

// DepositHandler is to process native messages on THORChain
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
	if h.mgr.Keeper().IsChainHalted(ctx, common.THORChain) {
		return nil, fmt.Errorf("unable to use MsgDeposit while THORChain is halted")
	}

	asset := msg.Coins[0].Asset

	if asset.IsTradeAsset() || asset.IsSecuredAsset() {
		return nil, fmt.Errorf("trade and secured assets are not part of the Thornado custody fork")
	}

	coins, err := msg.Coins.Native()
	if err != nil {
		return nil, ErrInternal(err, "coins are native to THORChain")
	}

	// HasCoins always returns false if the address has no balances
	// (such as if having had just enough for the network fee),
	// so check first whether there is any non-zero Amount necessary.
	if !msg.Coins.IsEmpty() && !h.mgr.Keeper().HasCoins(ctx, msg.GetSigners()[0], coins) {
		return nil, se.ErrInsufficientFunds
	}

	txIDSource := ctx.TxBytes()
	if len(txIDSource) == 0 {
		txIDSource, _ = msg.Marshal()
		height := make([]byte, 8)
		binary.BigEndian.PutUint64(height, uint64(ctx.BlockHeight()))
		txIDSource = append(txIDSource, height...)
	}
	hash := tmtypes.Tx(txIDSource).Hash()
	txID, err := common.NewTxID(fmt.Sprintf("%X", hash))
	if idx > 0 {
		ctx.Logger().Info("auto-increment txid", "idx", idx, "hash", fmt.Sprintf("%X", hash))
		txID, err = common.NewTxID(fmt.Sprintf("%X-%d", hash, idx))
		if err != nil {
			return nil, fmt.Errorf("fail to get tx hash: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("fail to get tx hash: %w", err)
	}
	existingVoter, err := h.mgr.Keeper().GetObservedTxInVoter(ctx, txID)
	if err != nil {
		ctx.Logger().Error("GetObservedTxInVoter error", "idx", idx, "txID", txID.String(), "error", err)
		return nil, fmt.Errorf("fail to get existing voter: %w", err)
	}
	if len(existingVoter.Txs) > 0 {
		ctx.Logger().Info("txid collision detected, retrying with auto-incremented idx", "txID", txID.String(), "existingTxs", len(existingVoter.Txs), "idx", idx)
		maxRetries := uint16(h.mgr.Keeper().GetConfigInt64(ctx, constants.MaxDepositTxIDRetries))
		if idx >= maxRetries {
			return nil, fmt.Errorf("txid: %s already exist after max retries (%d)", txID.String(), maxRetries)
		}
		return h.handle(ctx, msg, idx+1)
	}
	from, err := common.NewAddress(msg.GetSigners()[0].String())
	if err != nil {
		return nil, fmt.Errorf("fail to get from address: %w", err)
	}

	handler := NewInternalHandler(h.mgr)

	// Add memo for memoless transactions (generate reference from amount)
	if len(msg.Coins) > 0 && strings.TrimSpace(msg.Memo) == "" {
		refAsset := msg.Coins[0].Asset
		var referenceID string
		referenceID, err = h.extractReferenceFromNativeAmount(ctx, msg.Coins[0].Amount.Uint64())
		if err != nil {
			return nil, fmt.Errorf("memoless native deposit failed: %w", err)
		}
		refMemo := NewReferenceReadMemo(referenceID)
		msg.Memo = refMemo.CreateMemo() // Sets "r:XXXXX"
		ctx.Logger().Info("generated reference memo for memoless native deposit", "reference", referenceID, "asset", refAsset)
	}

	memo, err := ParseMemoWithTHORNames(ctx, h.mgr.Keeper(), msg.Memo)
	if err != nil {
		return nil, ErrInternal(err, "invalid memo")
	}

	if memo.IsOutbound() || memo.IsInternal() {
		return nil, fmt.Errorf("cannot send inbound an outbound or internal transaction")
	}

	// Resolve reference memo before determining target module to ensure funds go to the correct module
	if len(msg.Coins) > 0 && memo.GetType() == TxReferenceReadMemo {
		asset := msg.Coins[0].Asset
		// Create a temporary tx for resolution context
		tempTx := common.NewTx(txID, from, common.NoAddress, msg.Coins, common.Gas{}, msg.Memo)
		// Use current block height as observation height for native deposits (instant finality)
		resolvedMemo := fetchMemoFromReference(ctx, h.mgr, asset, tempTx, ctx.BlockHeight())
		preMemo := msg.Memo
		msg.Memo = resolvedMemo
		ctx.Logger().Info("reference memo conversion for native deposit", "pre", preMemo, "post", msg.Memo, "asset", asset)

		// Re-parse the resolved memo
		memo, err = ParseMemoWithTHORNames(ctx, h.mgr.Keeper(), msg.Memo)
		if err != nil {
			return nil, ErrInternal(err, "invalid resolved memo")
		}
	}

	var targetModule string
	switch memo.GetType() {
	case TxBond, TxUnBond, TxLeave, TxOperatorRotate:
		targetModule = BondName
	// For TxTCYClaim, send to Reserve so retrievable if done accidentally
	case TxReserve, TxTHORName, TxTCYClaim, TxMaint, TxModifyLimitSwap:
		targetModule = ReserveName
	case TxTCYStake, TxTCYUnstake:
		targetModule = TCYStakeName
	default:
		targetModule = AsgardName
	}

	// Only permit coin types other than RUNE to be sent to network modules when explicitly allowed.
	// (When the Amount is zero, the Asset type is irrelevant.)
	// Coins having exactly one Coin is ensured by the validate function,
	// but IsEmpty covers a hypothetical no-Coin scenario too.
	if !msg.Coins.IsEmpty() && (!msg.Coins[0].Asset.IsRune() && !msg.Coins[0].Asset.IsTCY() && !msg.Coins[0].Asset.IsWhitelisted()) && targetModule != AsgardName {
		return nil, fmt.Errorf("(%s) memos are for the (%s) module, for which messages must only contain RUNE or TCY", memo.GetType().String(), targetModule)
	}

	coinsInMsg := msg.Coins
	if !coinsInMsg.IsEmpty() && !coinsInMsg[0].Asset.IsTradeAsset() && !coinsInMsg[0].Asset.IsSecuredAsset() {
		// send funds to target module
		err = h.mgr.Keeper().SendFromAccountToModule(ctx, msg.GetSigners()[0], targetModule, msg.Coins)
		if err != nil {
			return nil, err
		}
	}

	to, err := h.mgr.Keeper().GetModuleAddress(targetModule)
	if err != nil {
		return nil, fmt.Errorf("fail to get to address: %w", err)
	}

	tx := common.NewTx(txID, from, to, coinsInMsg, common.Gas{}, msg.Memo)
	tx.Chain = common.THORChain

	// construct msg from memo
	txIn := ObservedTx{Tx: tx}
	txInVoter := NewObservedTxVoter(txIn.Tx.ID, []common.ObservedTx{txIn})
	txInVoter.Height = ctx.BlockHeight() // While FinalisedHeight may be overwritten, Height records the consensus height
	txInVoter.FinalisedHeight = ctx.BlockHeight()
	txInVoter.Tx = txIn
	h.mgr.Keeper().SetObservedTxInVoter(ctx, txInVoter)

	m, txErr := processOneTxIn(ctx, h.mgr.Keeper(), txIn, msg.Signer)
	if txErr != nil {
		ctx.Logger().Error("fail to process native inbound tx", "error", txErr.Error(), "tx hash", tx.ID.String())
		return nil, txErr
	}

	mCtx := ctx
	result, err := handler(mCtx, m)
	if err != nil {
		return nil, err
	}

	// if an outbound is not expected, mark the voter as done
	if !memo.GetType().HasOutbound() {
		// retrieve the voter from store in case the handler caused a change
		voter, err := h.mgr.Keeper().GetObservedTxInVoter(ctx, txID)
		if err != nil {
			return nil, fmt.Errorf("fail to get voter")
		}
		voter.SetDone()
		h.mgr.Keeper().SetObservedTxInVoter(ctx, voter)
	}
	return result, nil
}

// extractReferenceFromNativeAmount extracts the reference ID from a native asset transaction amount.
// All native assets use common.THORChainDecimals (8 decimals), so no pool lookup is needed.
func (h DepositHandler) extractReferenceFromNativeAmount(ctx cosmos.Context, amount uint64) (string, error) {
	if amount == 0 {
		return "", fmt.Errorf("zero amount in transaction for reference generation")
	}

	baseEnd := h.mgr.Keeper().GetConfigInt64(ctx, constants.MemolessTxnRefCount)
	txnRefLength := len(fmt.Sprintf("%d", baseEnd))

	// Prevent overflow: uint64 max is ~1.8e19, so max safe length is 19 digits
	const maxRefLength = 19
	if txnRefLength > maxRefLength {
		return "", fmt.Errorf("reference length %d exceeds maximum %d to prevent overflow", txnRefLength, maxRefLength)
	}

	// Calculate modulus based on reference length (10^txnRefLength)
	var modulus uint64 = 1
	if baseEnd > 0 {
		for i := 0; i < txnRefLength; i++ {
			modulus *= 10
		}
	}

	// Native assets already use THORChain decimals (8), so no normalization needed
	// Extract reference from the amount directly
	refNum := amount % modulus

	// Zero references are not allowed as they would create ambiguous reference IDs.
	// This occurs when the amount is exactly divisible by the modulus
	// (e.g., 50000000 with modulus 100 → ref 0).
	// Users should adjust their amount slightly to avoid this edge case.
	if refNum == 0 {
		return "", fmt.Errorf("zero reference from amount is invalid: amount %d is divisible by modulus %d", amount, modulus)
	}

	return leadingZeros(txnRefLength, fmt.Sprintf("%d", refNum)), nil
}

// DepositAnteHandler called by the ante handler to gate mempool entry
// and also during deliver. Store changes will persist if this function
// succeeds, regardless of the success of the transaction.
func DepositAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgDeposit) (cosmos.Context, error) {
	return ctx, k.DeductNativeTxFeeFromAccount(ctx, msg.GetSigners()[0])
}
