//go:build !regtest
// +build !regtest

package thorchain

import (
	abci "github.com/cometbft/cometbft/abci/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

func InitGenesis(ctx cosmos.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	return initGenesis(ctx, keeper, data)
}
