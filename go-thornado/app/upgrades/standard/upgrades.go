package standard

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/thornadocash/go-thornado/app/upgrades"
	thornado "github.com/thornadocash/go-thornado/x/thornado"
	keeperv1 "github.com/thornadocash/go-thornado/x/thornado/keeper/v1"
)

// NewUpgrade constructor
func NewUpgrade(semver string) upgrades.Upgrade {
	// NOTE: DO NOT modify store upgrades here. Create a new package at
	// app/upgrades/<semver> and use that for the upgrade in app/upgrades.go.
	return upgrades.Upgrade{
		UpgradeName:          semver,
		CreateUpgradeHandler: CreateUpgradeHandler,
		StoreUpgrades: storetypes.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
	}
}

func CreateUpgradeHandler(
	mm upgrades.ModuleManager,
	configurator module.Configurator,
	ak *upgrades.AppKeepers,
) upgradetypes.UpgradeHandler {
	return func(goCtx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Active validator versions need to be updated since consensus
		// on the new version is required to resume the chain.
		// This is a Thornado specific upgrade step that should be
		// done in every upgrade handler and before any thornado module migrations.
		ctx := sdk.UnwrapSDKContext(goCtx)
		if err := keeperv1.UpdateActiveNodeVersions(ctx, ak.ThornadoKeeper, plan.Name); err != nil {
			return nil, fmt.Errorf("failed to update active validator versions: %w", err)
		}
		if err := thornado.RepairMixedBTCPendingBatches(ctx, ak.ThornadoKeeper); err != nil {
			return nil, fmt.Errorf("failed to repair mixed bitcoin pending batches: %w", err)
		}

		// Testnet reset: the shielder Merkle tree switched from sorted-order to
		// insertion-order (incremental) root computation, which changes every root.
		// Purge the commitment pool (commitments, notes, leaves, tree state, roots,
		// nullifiers) so the new tree starts clean. Outstanding notes are invalidated
		// by design; this is acceptable on testnet.
		ak.ThornadoKeeper.PurgeShielderPoolState(ctx)
		ctx.Logger().Info("purged shielder pool state for incremental merkle tree reset")

		// Perform SDK module migrations
		return mm.RunMigrations(goCtx, configurator, fromVM)
	}
}
