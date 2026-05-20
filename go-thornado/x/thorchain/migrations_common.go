package thorchain

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// Migrate4to5 migrates from version 4 to 5.
func (m Migrator) ClearObsoleteMimirs(ctx sdk.Context) error {
	// Loads the manager for this migration (we are in the x/upgrade's preblock)
	// Note, we do not require the manager loaded for this migration, but it is okay
	// to load it earlier and this is the pattern for migrations to follow.
	if err := m.mgr.LoadManagerIfNecessary(ctx); err != nil {
		return err
	}

	// Issue #2112, clearing obsolete Mimir keys.

	toClear := func(key string) bool {
		upperKey := strings.ToUpper(key)
		return (strings.Contains(upperKey, "BNB") && !strings.Contains(upperKey, "BSC")) || // Do not clear BSC-BNB keys.
			strings.Contains(upperKey, "TERRA") ||
			strings.Contains(upperKey, "YGG") ||
			strings.EqualFold(key, "MaxConfirmations") || // Only effective with -<Chain> .
			strings.EqualFold(key, "ConfMultiplierBasisPoints") || // Only effective with -<Chain> .
			strings.EqualFold(key, "SystemIncomeBurnRateBp") // Only Bps effective, not Bp.
	}

	iterNode := m.mgr.Keeper().GetNodeMimirIterator(ctx)
	defer iterNode.Close()
	for ; iterNode.Valid(); iterNode.Next() {
		key := trimKeyPrefix(iterNode.Key())

		if !toClear(key) {
			continue
		}

		// As with PurgeOperationalNodeMimirs,
		// not emitting individual EventSetNodeMimir events.
		m.mgr.Keeper().DeleteNodeMimirs(ctx, key)
	}

	iter := m.mgr.Keeper().GetMimirIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key := trimKeyPrefix(iter.Key())

		if !toClear(key) {
			continue
		}

		if err := m.mgr.Keeper().DeleteMimir(ctx, key); err != nil {
			ctx.Logger().Error("fail to delete mimir", "key", key, "error", err)
			continue
		}

		// As with Admin key deletion, emit a deletion event.
		mimirEvent := NewEventSetMimir(strings.ToUpper(key), "-1")
		if err := m.mgr.EventMgr().EmitEvent(ctx, mimirEvent); err != nil {
			ctx.Logger().Error("fail to emit set_mimir event", "error", err)
		}
	}

	return nil
}

// Migrate7to8 migrates from version 7 to 8.
func (m Migrator) Migrate7to8(ctx sdk.Context) error {
	// Loads the manager for this migration (we are in the x/upgrade's preblock)
	// Note, we do not require the manager loaded for this migration, but it is okay
	// to load it earlier and this is the pattern for migrations to follow.
	if err := m.mgr.LoadManagerIfNecessary(ctx); err != nil {
		return err
	}

	// Update all KVStore network fees from (Mainnet-only) legacy 1e8 TransactionFeeRate
	// to gas rate units TransactionFeeRate.
	for _, chain := range common.AllChains {
		_, gasRateUnitsPerOne := chain.GetGasUnits()

		if gasRateUnitsPerOne.Equal(cosmos.NewUint(common.One)) {
			// This is already in the right units.
			continue
		}

		if chain.IsTHORChain() {
			// GetGasUnits doesn't have a THORChain entry,
			// so in case of unintended effects skip this.
			continue
		}

		networkFee, err := m.mgr.Keeper().GetNetworkFee(ctx, chain)
		if err != nil {
			ctx.Logger().Error("Error getting NetworkFee for chain", "chain", chain.String(), "error", err)
			return err
		}
		ctx.Logger().Info("NetworkFee details", "chain", chain.String(), "transactionSize", networkFee.TransactionSize, "transactionFeeRate", networkFee.TransactionFeeRate)

		// Skip if TransactionSize is 0 to avoid validation error
		if networkFee.TransactionSize == 0 {
			ctx.Logger().Info("Skipping chain due to zero TransactionSize", "chain", chain.String())
			continue
		}

		networkFee.TransactionFeeRate = cosmos.NewUint(networkFee.TransactionFeeRate).Mul(gasRateUnitsPerOne).QuoUint64(common.One).Uint64()
		if err := m.mgr.Keeper().SaveNetworkFee(ctx, chain, networkFee); err != nil {
			return err
		}
	}

	return nil
}

func (m Migrator) CommonMigrate8to9(ctx sdk.Context) error {
	// reduce the minimum L1 outbound fee to $0.25
	m.mgr.Keeper().SetMimir(ctx, constants.MinimumL1OutboundFeeUSD.String(), 25000000)

	// ------------------------------ Asset Gas Multiplier Adjustments ------------------------------
	//
	// We are going to adjust the target surplus and multipliers for all assets. The steps are:
	// 1. Adjust the target surplus to 10k RUNE
	// 2. Compute the new multiplier for each asset
	// 3. If the multiplier is less than or equal to 100%, do nothing
	// 4. If the multiplier is greater than 100%, increase the withheld amount so that the multiplier is equal to 100%

	// Adjust the target surplus to 10k RUNE
	m.mgr.Keeper().SetMimir(ctx, constants.TargetOutboundFeeSurplusRune.String(), 10000_00000000) // 10k x 10^8

	// Adjust minimum DOFM multiplier to 1%
	// https://gitlab.com/thorchain/thornode/-/issues/2239#note_2690577110
	m.mgr.Keeper().SetMimir(ctx, constants.MinOutboundFeeMultiplierBasisPoints.String(), 100)

	// Gather all the parameters for the multiplier calculation
	targetSurplus := cosmos.NewUint(uint64(m.mgr.Keeper().GetConfigInt64(ctx, constants.TargetOutboundFeeSurplusRune)))
	maxMultiplier := cosmos.NewUint(uint64(m.mgr.Keeper().GetConfigInt64(ctx, constants.MaxOutboundFeeMultiplierBasisPoints)))
	minMultiplier := cosmos.NewUint(uint64(m.mgr.Keeper().GetConfigInt64(ctx, constants.MinOutboundFeeMultiplierBasisPoints)))

	// Collect all the assets via pool iterator (copied from querier.go outbound fees endpoint)
	var assets []common.Asset
	iterator := m.mgr.Keeper().GetPoolIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var pool Pool
		if err := m.mgr.Keeper().Cdc().Unmarshal(iterator.Value(), &pool); err != nil {
			return fmt.Errorf("fail to unmarshal pool: %w", err)
		}

		if pool.Asset.IsNative() {
			// To avoid clutter do not by default display the outbound fees
			// of THORChain Assets other than RUNE.
			continue
		}
		if pool.BalanceAsset.IsZero() || pool.BalanceRune.IsZero() {
			// A Layer 1 Asset's pool must have both depths be non-zero
			// for any outbound fee withholding or gas reimbursement to take place.
			// (This can take place even if the PoolUnits are zero and all liquidity is synths.)
			continue
		}
		if pool.Status != types.PoolStatus_Available {
			// A Layer 1 Asset's pool must be available
			// for any outbound fee withholding or gas reimbursement to take place.
			continue
		}

		assets = append(assets, pool.Asset)
	}

	type feesAndMultiplier struct {
		withheld   cosmos.Uint
		spent      cosmos.Uint
		surplus    cosmos.Uint
		multiplier cosmos.Uint
	}

	getFeesAndMultiplier := func(ctx cosmos.Context, asset common.Asset) (feesAndMultiplier, error) {
		var err error
		var fem feesAndMultiplier
		fem.withheld, err = m.mgr.Keeper().GetOutboundFeeWithheldRune(ctx, asset)
		if err != nil {
			return fem, err
		}
		fem.spent, err = m.mgr.Keeper().GetOutboundFeeSpentRune(ctx, asset)
		if err != nil {
			return fem, err
		}
		fem.surplus = common.SafeSub(fem.withheld, fem.spent)
		fem.multiplier = m.mgr.GasMgr().CalcOutboundFeeMultiplier(ctx, targetSurplus,
			fem.spent, fem.withheld, maxMultiplier, minMultiplier)
		return fem, nil
	}

	processAsset := func(asset common.Asset, targetMultiplier cosmos.Uint) error {
		before, err := getFeesAndMultiplier(ctx, asset)
		if err != nil {
			ctx.Logger().Error("failed to get fees and multiplier before adjustment", "asset", asset, "error", err)
			return err
		}

		equilibriumSurplus := m.mgr.Keeper().GetSurplusForTargetMultiplier(ctx, targetMultiplier)
		if before.multiplier.LTE(targetMultiplier) {
			ctx.Logger().Info("multiplier is less than 100%; no adjustment needed",
				"asset", asset,
				"multiplier", before.multiplier,
				"surplus", before.surplus,
				"target_multiplier", targetMultiplier,
				"equilibrium_surplus", equilibriumSurplus,
				"withheld_adjustment", "none",
			)
			return nil
		}

		// Increase withheld so that the multiplier is equal to 100%
		withheldAdjustment := common.SafeSub(equilibriumSurplus, before.surplus)
		err = m.mgr.Keeper().AddToOutboundFeeWithheldRune(ctx, asset, withheldAdjustment)
		if err != nil {
			ctx.Logger().Error("failed to adjust withheld amount", "asset", asset, "adjustment", withheldAdjustment, "error", err)
			return err
		}

		after, err := getFeesAndMultiplier(ctx, asset)
		if err != nil {
			ctx.Logger().Error("failed to get fees and multiplier after adjustment", "asset", asset, "error", err)
			return err
		}

		ctx.Logger().Info("multiplier was greater than target; adjusted",
			"asset", asset,
			"before_withheld", before.withheld,
			"before_surplus", before.surplus,
			"before_multiplier", before.multiplier,
			"target_multiplier", targetMultiplier,
			"equilibrium_surplus", equilibriumSurplus,
			"adjustment", withheldAdjustment,
			"after_withheld", after.withheld,
			"after_surplus", after.surplus,
			"after_multiplier", after.multiplier,
		)

		return nil
	}

	for _, asset := range assets {
		err := processAsset(asset, cosmos.NewUint(10_000)) // target 100% multiplier
		if err != nil {
			return err
		}
	}

	return nil
}

// BurnReserveAndReduceMaxSupply implements ADR-023: burns reserve down to 9.3M RUNE
// and sets MaxRuneSupply to the post-burn total supply (derived from live state).
func (m Migrator) BurnReserveAndReduceMaxSupply(ctx sdk.Context) error {
	const reserveRetain uint64 = 9_300_000_00000000 // 9.3M RUNE to keep in reserve

	reserveBalance := m.mgr.Keeper().GetRuneBalanceOfModule(ctx, ReserveName)
	retainAmount := cosmos.NewUint(reserveRetain)

	if reserveBalance.GT(retainAmount) {
		burnAmount := common.SafeSub(reserveBalance, retainAmount)
		burnCoin := common.NewCoin(common.RuneNative, burnAmount)

		if err := m.mgr.Keeper().SendFromModuleToModule(ctx, ReserveName, ModuleName, common.NewCoins(burnCoin)); err != nil {
			return fmt.Errorf("fail to transfer reserve RUNE for burn: %w", err)
		}
		if err := m.mgr.Keeper().BurnFromModule(ctx, ModuleName, burnCoin); err != nil {
			return fmt.Errorf("fail to burn reserve RUNE: %w", err)
		}

		burnEvt := NewEventMintBurn(BurnSupplyType, burnCoin.Asset.Native(), burnCoin.Amount, "adr023_reserve_burn")
		if err := m.mgr.EventMgr().EmitEvent(ctx, burnEvt); err != nil {
			ctx.Logger().Error("fail to emit reserve burn event", "error", err)
		}

		ctx.Logger().Info("ADR-023: burned reserve RUNE",
			"burn_amount", burnAmount.String(),
			"remaining_reserve", retainAmount.String())
	} else {
		ctx.Logger().Info("ADR-023: reserve balance already at or below target, skipping burn",
			"reserve_balance", reserveBalance.String(),
			"retain_target", retainAmount.String())
	}

	// Derive MaxRuneSupply from post-burn total supply to avoid state drift.
	postBurnSupply := m.mgr.Keeper().GetTotalSupply(ctx, common.RuneAsset())
	newMaxSupply := int64(postBurnSupply.Uint64())

	m.mgr.Keeper().SetMimir(ctx, constants.MaxRuneSupply.String(), newMaxSupply)
	ctx.Logger().Info("ADR-023: set MaxRuneSupply to post-burn total supply", "new_max_supply", newMaxSupply)

	return nil
}
