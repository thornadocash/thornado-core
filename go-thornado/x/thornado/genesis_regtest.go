//go:build regtest
// +build regtest

package thornado

import (
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
)

func InitGenesis(ctx cosmos.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	nodes := initGenesis(ctx, keeper, data)
	if len(nodes) == 0 {
		return nodes
	}
	return nodes[:1]
}
