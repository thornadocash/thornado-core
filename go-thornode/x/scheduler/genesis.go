package scheduler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/keeper"
	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

// InitGenesis initializes the denom module's state from a provided genesis
// state.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, genState types.GenesisState) {
	for _, schedule := range genState.GetSchedules() {
		err := k.SetSchedule(ctx, schedule)
		if err != nil {
			panic(err)
		}
	}
}

// ExportGenesis returns the denom module's exported genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	schedules := []types.Schedule{}
	iterator, err := k.Store().Iterate(ctx, nil)
	if err != nil {
		panic(err)
	}
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		schedule, err := iterator.Value()
		if err != nil {
			panic(err)
		}

		schedules = append(schedules, schedule)
	}

	return &types.GenesisState{
		Schedules: schedules,
	}
}
