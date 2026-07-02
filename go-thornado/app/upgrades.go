package app

import (
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"

	"github.com/thornadocash/go-thornado/app/upgrades"
	"github.com/thornadocash/go-thornado/app/upgrades/standard"
)

// Upgrades is a list of chain upgrades.
var Upgrades = []upgrades.Upgrade{
	// If releasing a consensus breaking upgrade (one performed with a an upgrade
	// proposal), do not add anything to this list. The current version in the `version`
	// file at the root of the repo will be the `app.Version()` used to register the
	// upgrade automatically in RegisterUpgradeHandlers (see below).

	// If releasing a non-consensus breaking upgrade (i.e. Bifrost-only patch release)
	// that requires no upgrade proposal, then add the current consensus version upgrade
	// here. Example: if the current network version is 3.12.0 and you are releasing a
	// non-consensus breaking patch release 3.12.1, then add the 3.12.0 upgrade here in
	// the following format:
	// standard.NewUpgrade("3.12.0"),

	// If the upgrade requires store modifications, create a new upgrade package at
	// app/upgrades/<semver> with the upgrade and reference it here in the format:
	// v3_12_0.NewUpgrade(),
	//
	// Example pattern: https://gitlab.com/thornado/thornado/-/merge_requests/3837
	standard.NewUpgrade("3.17.1"),
}

// RegisterUpgradeHandlers registers the chain upgrade handlers
func (app *ThornadoApp) RegisterUpgradeHandlers() {
	// setupLegacyKeyTables(&app.ParamsKeeper)
	if len(Upgrades) == 0 {
		// always have a unique upgrade registered for the current version to test in system tests
		Upgrades = append(Upgrades, standard.NewUpgrade(app.Version()))
	}

	keepers := upgrades.AppKeepers{
		ThornadoKeeper:        app.ThornadoKeeper,
		AccountKeeper:         &app.AccountKeeper,
		ParamsKeeper:          &app.ParamsKeeper,
		ConsensusParamsKeeper: &app.ConsensusParamsKeeper,
		Codec:                 app.appCodec,
		GetStoreKey:           app.GetKey,
	}
	// register all upgrade handlers
	for _, upgrade := range Upgrades {
		app.UpgradeKeeper.SetUpgradeHandler(
			upgrade.UpgradeName,
			upgrade.CreateUpgradeHandler(
				app.ModuleManager,
				app.configurator,
				&keepers,
			),
		)
	}

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(fmt.Sprintf("failed to read upgrade info from disk %s", err))
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	// register store loader for current upgrade
	for _, upgrade := range Upgrades {
		if upgradeInfo.Name == upgrade.UpgradeName {
			app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &upgrade.StoreUpgrades)) // nolint:gosec
			break
		}
	}
}
