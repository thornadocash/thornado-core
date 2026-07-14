package thornado

import (
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// Voted one-shot vault accounting repairs. The value is a BTC amount in sats
// applied to the single retiring base vault's book balance, then the config
// AND the node votes reset so a straggler vote cannot re-fire the repair.
//
// DEBIT reconciles the book downward after an off-protocol outflow the
// observation pipeline cannot attribute — e.g. a double-paid refund, where
// the retry re-paid an already-landed refund so the wallet lost funds the
// book never debited (solvency then halts BTC signing forever, since no
// observation can ever settle the gap). CREDIT is the inverse, for undoing
// an over-debit.
const (
	RepairRetiringVaultDebitSatsKey  = "REPAIR_RETIRINGVAULTDEBITSATS"
	RepairRetiringVaultCreditSatsKey = "REPAIR_RETIRINGVAULTCREDITSATS"
	// Voted one-shot: the value is a txout block height. Open BTC items at
	// that height get their prescribed SourceInputs/MaxGas/GasRate cleared so
	// the end-block exact refresh re-selects fresh unspent inputs. Recovers
	// items whose prescribed inputs were consumed by ANOTHER settled outbound
	// (double-prescription), which the refresh logic never retouches.
	RepairReselectTxOutHeightKey = "REPAIR_RESELECTTXOUTHEIGHT"
	// Voted one-shot: the value is a txout block height. Every BTC item at
	// that height is REOPENED — OutHash/OutVout cleared along with the input
	// prescription — so the refresh re-selects inputs and the signers sign it
	// again. Recovers a block falsely settled by an observed tx that never
	// paid its recipients (duplicate input unions made two blocks
	// indistinguishable to the batch matcher). Operators must verify the
	// block's recipients were NOT paid before voting this.
	RepairReopenTxOutHeightKey = "REPAIR_REOPENTXOUTHEIGHT"
)

type vaultDebitRepairKeeper interface {
	GetConfig(ctx cosmos.Context, key string) (int64, error)
	SetConfig(ctx cosmos.Context, key string, value int64)
	DeleteNodeConfigs(ctx cosmos.Context, key string)
	GetBaseVaultsByStatus(ctx cosmos.Context, status VaultStatus) (Vaults, error)
	SetVault(ctx cosmos.Context, vault Vault) error
	InvariantRoutes() []common.InvariantRoute
}

func applyVotedRetiringVaultDebitRepair(ctx cosmos.Context, mgr Manager) {
	k := mgr.Keeper()
	applyVotedRetiringVaultRepair(ctx, k, mgr.EventMgr(), RepairRetiringVaultDebitSatsKey, false)
	applyVotedRetiringVaultRepair(ctx, k, mgr.EventMgr(), RepairRetiringVaultCreditSatsKey, true)
	applyVotedTxOutReselect(ctx, mgr)
	applyVotedTxOutReopen(ctx, mgr)
}

func applyVotedTxOutReopen(ctx cosmos.Context, mgr Manager) {
	k := mgr.Keeper()
	height, err := k.GetConfig(ctx, RepairReopenTxOutHeightKey)
	if err != nil || height <= 0 {
		return
	}
	k.SetConfig(ctx, RepairReopenTxOutHeightKey, 0)
	k.DeleteNodeConfigs(ctx, RepairReopenTxOutHeightKey)
	emitConfigEvent(ctx, mgr.EventMgr(), RepairReopenTxOutHeightKey, 0)

	txOut, err := k.GetTxOut(ctx, height)
	if err != nil || txOut == nil || txOut.IsEmpty() {
		ctx.Logger().Error("txout reopen repair: no txout block", "height", height, "error", err)
		return
	}
	reopened := 0
	pending := 0
	for i, item := range txOut.TxArray {
		if !item.Chain.Equals(common.BTCChain) {
			continue
		}
		if item.OutHash.IsEmpty() {
			pending++
			continue
		}
		txOut.TxArray[i].OutHash = common.TxID("")
		txOut.TxArray[i].OutVout = 0
		txOut.TxArray[i].SourceInputs = nil
		txOut.TxArray[i].MaxGas = common.Gas{}
		txOut.TxArray[i].GasRate = 0
		reopened++
	}
	if reopened == 0 && pending == 0 {
		ctx.Logger().Info("txout reopen repair: nothing to reopen", "height", height)
		return
	}
	// A settled block is stamped complete; updateBatchStates never demotes it,
	// and queryKeysign hides pending items behind that status, so signers skip
	// the block forever. Reset the batch state machine along with the items.
	txOut.Status = TxOutStatusPendingBatch
	txOut.SigningLeader = ""
	txOut.SigningAttempt = 0
	txOut.RetryUntilHeight = 0
	if err := refreshBTCExactTxOutBlock(ctx, k, txOut); err != nil {
		ctx.Logger().Error("txout reopen repair: refresh failed; will retry at end-block", "height", height, "error", err)
	}
	if err := k.SetTxOut(ctx, txOut); err != nil {
		ctx.Logger().Error("txout reopen repair: fail to save txout", "height", height, "error", err)
		return
	}
	ctx.Logger().Info("applied txout reopen repair", "height", height, "items_reopened", reopened)
}

func applyVotedTxOutReselect(ctx cosmos.Context, mgr Manager) {
	k := mgr.Keeper()
	height, err := k.GetConfig(ctx, RepairReselectTxOutHeightKey)
	if err != nil || height <= 0 {
		return
	}
	k.SetConfig(ctx, RepairReselectTxOutHeightKey, 0)
	k.DeleteNodeConfigs(ctx, RepairReselectTxOutHeightKey)
	emitConfigEvent(ctx, mgr.EventMgr(), RepairReselectTxOutHeightKey, 0)

	txOut, err := k.GetTxOut(ctx, height)
	if err != nil || txOut == nil || txOut.IsEmpty() {
		ctx.Logger().Error("txout reselect repair: no txout block", "height", height, "error", err)
		return
	}
	cleared := 0
	for i, item := range txOut.TxArray {
		if !item.OutHash.IsEmpty() || !item.Chain.Equals(common.BTCChain) || len(item.SourceInputs) == 0 {
			continue
		}
		txOut.TxArray[i].SourceInputs = nil
		txOut.TxArray[i].MaxGas = common.Gas{}
		txOut.TxArray[i].GasRate = 0
		cleared++
	}
	if cleared == 0 {
		ctx.Logger().Info("txout reselect repair: nothing to clear", "height", height)
		return
	}
	if err := refreshBTCExactTxOutBlock(ctx, k, txOut); err != nil {
		ctx.Logger().Error("txout reselect repair: refresh failed; will retry at end-block", "height", height, "error", err)
	}
	if err := k.SetTxOut(ctx, txOut); err != nil {
		ctx.Logger().Error("txout reselect repair: fail to save txout", "height", height, "error", err)
		return
	}
	ctx.Logger().Info("applied txout reselect repair", "height", height, "items_cleared", cleared)
}

func applyVotedRetiringVaultRepair(ctx cosmos.Context, k vaultDebitRepairKeeper, eventMgr EventManager, key string, credit bool) {
	amount, err := k.GetConfig(ctx, key)
	if err != nil || amount <= 0 {
		return
	}
	// Consume the trigger before acting: clear the effective value AND the
	// node votes, so neither a failing repair nor a straggler vote arriving
	// after this block can fire the repair a second time.
	k.SetConfig(ctx, key, 0)
	k.DeleteNodeConfigs(ctx, key)
	emitConfigEvent(ctx, eventMgr, key, 0)

	retiring, err := k.GetBaseVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		ctx.Logger().Error("vault repair: fail to get retiring vaults", "error", err)
		return
	}
	if len(retiring) != 1 {
		ctx.Logger().Error("vault repair needs exactly one retiring vault", "count", len(retiring))
		return
	}
	vault := retiring[0]
	before := vault.Coins.GetCoin(common.BTCAsset).Amount
	delta := cosmos.NewUint(uint64(amount))
	if credit {
		vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, delta)))
	} else {
		if before.LT(delta) {
			ctx.Logger().Error("vault debit repair exceeds vault book balance",
				"vault", vault.PubKey.String(), "book", before.String(), "debit", delta.String())
			return
		}
		vault.SubFunds(common.NewCoins(common.NewCoin(common.BTCAsset, delta)))
	}
	if err := k.SetVault(ctx, vault); err != nil {
		ctx.Logger().Error("vault repair: fail to save vault", "error", err)
		return
	}

	for _, route := range k.InvariantRoutes() {
		if !strings.EqualFold(route.Route, "vault_backing") {
			continue
		}
		msg, broken := route.Invariant(ctx)
		if !broken {
			continue
		}
		if credit {
			vault.SubFunds(common.NewCoins(common.NewCoin(common.BTCAsset, delta)))
		} else {
			vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, delta)))
		}
		if restoreErr := k.SetVault(ctx, vault); restoreErr != nil {
			ctx.Logger().Error("vault repair: fail to restore vault after invariant break", "error", restoreErr)
			return
		}
		ctx.Logger().Error("vault repair reverted: would break vault backing invariant",
			"vault", vault.PubKey.String(), "key", key, "amount", delta.String(), "messages", strings.Join(msg, "; "))
		return
	}

	ctx.Logger().Info("applied retiring vault accounting repair",
		"vault", vault.PubKey.String(),
		"key", key,
		"credit", credit,
		"amount_sats", amount,
		"book_before", before.String(),
		"book_after", vault.Coins.GetCoin(common.BTCAsset).Amount.String(),
	)
}
