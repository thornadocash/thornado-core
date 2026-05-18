package keeper

import (
	"fmt"

	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

type (
	Keeper struct {
		cdc          codec.Codec
		storeService storetypes.KVStoreService
		wasmKeeper   types.WasmKeeper

		authority string
	}
)

func NewKeeper(
	cdc codec.Codec,
	storeService storetypes.KVStoreService,
	wasmKeeper types.WasmKeeper,
	authority string,
) Keeper {
	return Keeper{
		cdc:          cdc,
		storeService: storeService,
		wasmKeeper:   wasmKeeper,
		authority:    authority,
	}
}

func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

func (k Keeper) ExecuteSchedule(ctx sdk.Context) error {
	schedule, err := k.GetSchedule(ctx, uint64(ctx.BlockHeight()))
	if err != nil {
		return err
	}
	for _, msg := range schedule.Msgs {
		// Already validated in msg server
		sender, _ := sdk.AccAddressFromBech32(msg.Sender)
		_, execErr := k.wasmKeeper.Execute(ctx,
			sender,
			authtypes.NewModuleAddress(types.ModuleName),
			msg.Msg,
			sdk.Coins{},
		)
		// Fragile contracts should not error the endblock. Log the error and continue.
		if execErr != nil {
			ctx.EventManager().EmitEvents(sdk.Events{
				sdk.NewEvent(
					types.EventExecuteErrorMsg,
					sdk.NewAttribute(types.AttributeSender, msg.Sender),
					sdk.NewAttribute(types.AttributeMsg, string(msg.Msg)),
					sdk.NewAttribute(types.AttributeError, execErr.Error()),
				),
			})
		}
	}
	err = k.RemoveSchedule(ctx, schedule.Height)
	return err
}
