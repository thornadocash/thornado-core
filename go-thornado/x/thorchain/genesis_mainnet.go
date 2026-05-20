//go:build !regtest
// +build !regtest

package thorchain

import (
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
)

func InitGenesis(ctx cosmos.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	return initGenesis(ctx, keeper, data)
}
