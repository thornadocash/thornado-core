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
		for _, na := range data.NodeAccounts {
			if na.Status != NodeActive || na.NodeConsPubKey == "" {
				continue
			}
			pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, na.NodeConsPubKey)
			if err != nil {
				ctx.Logger().Error("fail to parse genesis consensus public key", "node", na.NodeAddress.String(), "key", na.NodeConsPubKey, "error", err)
				continue
			}
			nodes = append(nodes, abci.Ed25519ValidatorUpdate(pk.Bytes(), 100))
		}
	}
	return nodes
}
