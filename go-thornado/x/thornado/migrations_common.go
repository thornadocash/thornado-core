package thornado

import (
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// Migrate4to5 migrates from version 4 to 5.
func (m Migrator) ClearObsoleteConfigs(ctx sdk.Context) error {
	// Loads the manager for this migration (we are in the x/upgrade's preblock)
	// Note, we do not require the manager loaded for this migration, but it is okay
	// to load it earlier and this is the pattern for migrations to follow.
	if err := m.mgr.LoadManagerIfNecessary(ctx); err != nil {
		return err
	}

	// Issue #2112, clearing obsolete Config keys.

	toClear := func(key string) bool {
		upperKey := strings.ToUpper(key)
		return (strings.Contains(upperKey, "BNB") && !strings.Contains(upperKey, "BSC")) || // Do not clear BSC-BNB keys.
			strings.Contains(upperKey, "TERRA") ||
			strings.Contains(upperKey, "YGG") ||
			strings.EqualFold(key, "MaxConfirmations") || // Only effective with -<Chain> .
			strings.EqualFold(key, "ConfMultiplierBasisPoints") || // Only effective with -<Chain> .
			strings.EqualFold(key, "SystemIncomeBurnRateBp") // Only Bps effective, not Bp.
	}

	iterNode := m.mgr.Keeper().GetNodeConfigIterator(ctx)
	defer iterNode.Close()
	for ; iterNode.Valid(); iterNode.Next() {
		key := trimKeyPrefix(iterNode.Key())

		if !toClear(key) {
			continue
		}

		// As with PurgeOperationalNodeConfigs,
		// not emitting individual EventSetNodeConfig events.
		m.mgr.Keeper().DeleteNodeConfigs(ctx, key)
	}

	iter := m.mgr.Keeper().GetConfigIterator(ctx)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		key := trimKeyPrefix(iter.Key())

		if !toClear(key) {
			continue
		}

		if err := m.mgr.Keeper().DeleteConfig(ctx, key); err != nil {
			ctx.Logger().Error("fail to delete config", "key", key, "error", err)
			continue
		}

		// As with Admin key deletion, emit a deletion event.
		configEvent := NewEventSetConfig(strings.ToUpper(key), "-1")
		if err := m.mgr.EventMgr().EmitEvent(ctx, configEvent); err != nil {
			ctx.Logger().Error("fail to emit set_config event", "error", err)
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

		if chain.Equals(common.BTCChain) {
			// GetGasUnits doesn't have a BTCChain entry,
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
	return nil
}

func (m Migrator) BurnReserveAndReduceMaxSupply(ctx sdk.Context) error {
	return nil
}
