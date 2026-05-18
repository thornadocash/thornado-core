package denom

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/denom/keeper"
	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

// InitGenesis initializes the denom module's state from a provided genesis
// state.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, genState types.GenesisState) {
	for _, genDenom := range genState.GetAdmins() {
		id, err := types.DeconstructDenom(genDenom.GetDenom())
		if err != nil {
			panic(err)
		}
		_, err = k.CreateDenom(ctx, id, genDenom.Admin)
		if err != nil {
			panic(err)
		}
		addr := sdk.MustAccAddressFromBech32(genDenom.Admin)
		err = k.SetAdmin(ctx, genDenom.GetDenom(), &addr)
		if err != nil {
			panic(err)
		}
	}
}

// ExportGenesis returns the denom module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	genDenoms := []types.GenesisDenom{}
	iterator, err := k.Admins().Iterate(ctx, nil)
	if err != nil {
		panic(err)
	}
	var denom string
	var admin sdk.AccAddress
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		denom, err = iterator.Key()
		if err != nil {
			panic(err)
		}
		admin, err = iterator.Value()
		if err != nil {
			panic(err)
		}

		genDenoms = append(genDenoms, types.GenesisDenom{
			Denom: denom,
			Admin: admin.String(),
		})
	}

	return &types.GenesisState{
		Admins: genDenoms,
	}
}
