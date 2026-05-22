//go:build !regtest
// +build !regtest

package app

import (
	"os"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/thornadocash/go-thornado/config"
)

// BeginBlocker application updates every begin block
func (app *ThornadoApp) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	haltHeight := config.GetThornado().Cosmos.HaltHeight
	if haltHeight > 0 && ctx.BlockHeight() >= haltHeight {
		ctx.Logger().Info("halt height reached", "height", ctx.BlockHeight(), "halt height", haltHeight)
		os.Exit(0)
	}
	return app.ModuleManager.BeginBlock(ctx)
}

// EndBlocker application updates every end block
func (app *ThornadoApp) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}
