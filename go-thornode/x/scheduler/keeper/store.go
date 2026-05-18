package keeper

import (
	"fmt"

	"cosmossdk.io/collections"

	"cosmossdk.io/collections/codec"

	sdkcodec "github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

func (k Keeper) Store() collections.Map[uint64, types.Schedule] {
	return collections.NewMap(
		collections.NewSchemaBuilder(k.storeService),
		types.SchedulePrefix, types.ScheduleKey,
		codec.NewUint64Key[uint64](),
		sdkcodec.CollValue[types.Schedule](k.cdc),
	)
}

func (k Keeper) SenderIndex() collections.KeySet[collections.Pair[string, uint64]] {
	return collections.NewKeySet(
		collections.NewSchemaBuilder(k.storeService),
		types.SenderIndexPrefix, types.SenderIndexKey,
		collections.PairKeyCodec(collections.StringKey, codec.NewUint64Key[uint64]()),
	)
}

func (k Keeper) AddMsg(ctx sdk.Context, msg types.MsgScheduleExecuteContract) error {
	height := uint64(ctx.BlockHeight()) + msg.After + 1

	existing, err := k.GetSchedule(ctx, height)
	if err != nil {
		return err
	}
	existing.Msgs = append(existing.Msgs, msg)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventScheduleMsg,
			sdk.NewAttribute(types.AttributeSender, msg.Sender),
			sdk.NewAttribute(types.AttributeAfter, fmt.Sprint(msg.After)),
			sdk.NewAttribute(types.AttributeHeight, fmt.Sprint(height)),
			sdk.NewAttribute(types.AttributeMsg, string(msg.Msg)),
		),
	})

	// directly set schedule, to avoid unnecessary deletes and inserts
	err = k.Store().Set(ctx, height, *existing)
	if err != nil {
		return err
	}

	return k.SenderIndex().Set(ctx, collections.Join(msg.Sender, height))
}

func (k Keeper) GetSchedule(ctx sdk.Context, height uint64) (*types.Schedule, error) {
	has, err := k.Store().Has(ctx, height)
	if err != nil {
		return nil, err
	}
	if !has {
		return &types.Schedule{
			Height: height,
		}, nil
	}

	existing, err := k.Store().Get(ctx, height)
	return &existing, err
}

// SetSchedule sets or updates a schedule. On update, it deletes the sender
// index for all messages of the existing schedule and writes new entries
// for all messages of the new schedule.
func (k Keeper) SetSchedule(ctx sdk.Context, schedule types.Schedule) error {
	// check and remove old schedule and sender index
	has, err := k.Store().Has(ctx, schedule.Height)
	if err != nil {
		return err
	}

	if has {
		err = k.RemoveSchedule(ctx, schedule.Height)
		if err != nil {
			return err
		}
	}

	err = k.Store().Set(ctx, schedule.Height, schedule)
	if err != nil {
		return err
	}

	for _, msg := range schedule.Msgs {
		err = k.SenderIndex().Set(ctx, collections.Join(msg.Sender, schedule.Height))
		if err != nil {
			return err
		}
	}

	return nil
}

func (k Keeper) RemoveSchedule(ctx sdk.Context, height uint64) error {
	schedule, err := k.GetSchedule(ctx, height)
	if err != nil {
		return err
	}

	for _, msg := range schedule.Msgs {
		err = k.SenderIndex().Remove(ctx, collections.Join(msg.Sender, height))
		if err != nil {
			return err
		}
	}
	return k.Store().Remove(ctx, height)
}
