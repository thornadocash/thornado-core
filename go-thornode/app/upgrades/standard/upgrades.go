package standard

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"gitlab.com/thorchain/thornode/v3/app/upgrades"
	keeperv1 "gitlab.com/thorchain/thornode/v3/x/thorchain/keeper/v1"
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
		// This is a THORChain specific upgrade step that should be
		// done in every upgrade handler and before any thorchain module migrations.
		ctx := sdk.UnwrapSDKContext(goCtx)
		if err := keeperv1.UpdateActiveValidatorVersions(ctx, ak.ThorchainKeeper, plan.Name); err != nil {
			return nil, fmt.Errorf("failed to update active validator versions: %w", err)
		}

		// Perform SDK module migrations
		return mm.RunMigrations(goCtx, configurator, fromVM)
	}
}
