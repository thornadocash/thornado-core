package thornado

import (
	"strings"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// RepairRetiringVaultDebitSatsKey is a voted one-shot repair: the value is a
// BTC amount in sats to subtract from the single retiring base vault's book
// balance. It reconciles the book with the real wallet balance after an
// off-protocol outflow the observation pipeline cannot attribute — e.g. a
// double-paid refund, where the retry re-paid an already-landed refund so the
// wallet lost funds the book never debited (solvency then halts BTC signing
// forever, since no observation can ever settle the gap).
const RepairRetiringVaultDebitSatsKey = "REPAIR_RETIRINGVAULTDEBITSATS"

type vaultDebitRepairKeeper interface {
	GetConfig(ctx cosmos.Context, key string) (int64, error)
	SetConfig(ctx cosmos.Context, key string, value int64)
	GetBaseVaultsByStatus(ctx cosmos.Context, status VaultStatus) (Vaults, error)
	SetVault(ctx cosmos.Context, vault Vault) error
	InvariantRoutes() []common.InvariantRoute
}

func applyVotedRetiringVaultDebitRepair(ctx cosmos.Context, k vaultDebitRepairKeeper, eventMgr EventManager) {
	amount, err := k.GetConfig(ctx, RepairRetiringVaultDebitSatsKey)
	if err != nil || amount <= 0 {
		return
	}
	// Clear first: a failing repair must not re-fire every block.
	k.SetConfig(ctx, RepairRetiringVaultDebitSatsKey, 0)
	emitConfigEvent(ctx, eventMgr, RepairRetiringVaultDebitSatsKey, 0)

	retiring, err := k.GetBaseVaultsByStatus(ctx, RetiringVault)
	if err != nil {
		ctx.Logger().Error("vault debit repair: fail to get retiring vaults", "error", err)
		return
	}
	if len(retiring) != 1 {
		ctx.Logger().Error("vault debit repair needs exactly one retiring vault", "count", len(retiring))
		return
	}
	vault := retiring[0]
	before := vault.Coins.GetCoin(common.BTCAsset).Amount
	debit := cosmos.NewUint(uint64(amount))
	if before.LT(debit) {
		ctx.Logger().Error("vault debit repair exceeds vault book balance",
			"vault", vault.PubKey.String(), "book", before.String(), "debit", debit.String())
		return
	}
	vault.SubFunds(common.NewCoins(common.NewCoin(common.BTCAsset, debit)))
	if err := k.SetVault(ctx, vault); err != nil {
		ctx.Logger().Error("vault debit repair: fail to save vault", "error", err)
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
		vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, debit)))
		if restoreErr := k.SetVault(ctx, vault); restoreErr != nil {
			ctx.Logger().Error("vault debit repair: fail to restore vault after invariant break", "error", restoreErr)
			return
		}
		ctx.Logger().Error("vault debit repair reverted: would break vault backing invariant",
			"vault", vault.PubKey.String(), "debit", debit.String(), "messages", strings.Join(msg, "; "))
		return
	}

	ctx.Logger().Info("applied retiring vault debit repair",
		"vault", vault.PubKey.String(),
		"debit_sats", amount,
		"book_before", before.String(),
		"book_after", vault.Coins.GetCoin(common.BTCAsset).Amount.String(),
	)
}
