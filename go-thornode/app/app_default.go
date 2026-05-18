//go:build !regtest
// +build !regtest

package app

import (
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/config"
)

// BeginBlocker application updates every begin block
func (app *THORChainApp) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	haltHeight := config.GetThornode().Cosmos.HaltHeight
	if haltHeight > 0 && ctx.BlockHeight() >= haltHeight {
		ctx.Logger().Info("halt height reached", "height", ctx.BlockHeight(), "halt height", haltHeight)
		os.Exit(0)
	}
	return app.ModuleManager.BeginBlock(ctx)
}

// EndBlocker application updates every end block
func (app *THORChainApp) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}
