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
)

type vaultDebitRepairKeeper interface {
	GetConfig(ctx cosmos.Context, key string) (int64, error)
	SetConfig(ctx cosmos.Context, key string, value int64)
	DeleteNodeConfigs(ctx cosmos.Context, key string)
	GetBaseVaultsByStatus(ctx cosmos.Context, status VaultStatus) (Vaults, error)
	SetVault(ctx cosmos.Context, vault Vault) error
	InvariantRoutes() []common.InvariantRoute
}

func applyVotedRetiringVaultDebitRepair(ctx cosmos.Context, k vaultDebitRepairKeeper, eventMgr EventManager) {
	applyVotedRetiringVaultRepair(ctx, k, eventMgr, RepairRetiringVaultDebitSatsKey, false)
	applyVotedRetiringVaultRepair(ctx, k, eventMgr, RepairRetiringVaultCreditSatsKey, true)
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
