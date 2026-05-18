package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ types.QueryServer = Keeper{}

// Schedule implements types.QueryServer.
func (k Keeper) Schedule(ctx context.Context, req *types.QueryScheduleRequest) (*types.QueryScheduleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	schedule, err := k.GetSchedule(sdkCtx, req.Height)
	if err != nil {
		return nil, err
	}
	return &types.QueryScheduleResponse{
		Schedule: schedule,
	}, nil
}

// Schedules implements types.QueryServer.
func (k Keeper) Schedules(ctx context.Context, req *types.QuerySchedulesRequest) (*types.QuerySchedulesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	var schedules []*types.Schedule
	var pageRes *query.PageResponse
	var err error

	if req.Sender != "" {
		_, err = sdk.AccAddressFromBech32(req.Sender)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid sender address: %s", req.Sender)
		}

		schedules, pageRes, err = k.listSchedulesBySender(sdkCtx, &req.Pagination, req.Sender)
	} else {
		schedules, pageRes, err = k.ListSchedules(sdkCtx, &req.Pagination)
	}

	if err != nil {
		return nil, err
	}

	return &types.QuerySchedulesResponse{
		Schedules:  schedules,
		Pagination: *pageRes,
	}, nil
}

func (k Keeper) ListSchedules(
	ctx sdk.Context,
	pageReq *query.PageRequest,
) ([]*types.Schedule, *query.PageResponse, error) {
	return query.CollectionPaginate(
		ctx, k.Store(), pageReq,
		func(key uint64, value types.Schedule) (*types.Schedule, error) {
			return &value, nil
		},
	)
}

func (k Keeper) listSchedulesBySender(
	ctx sdk.Context,
	pageReq *query.PageRequest,
	sender string,
) ([]*types.Schedule, *query.PageResponse, error) {
	return query.CollectionPaginate(
		ctx, k.SenderIndex(), pageReq,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (*types.Schedule, error) {
			schedule, err := k.Store().Get(ctx, key.K2())
			if err != nil {
				return nil, err
			}
			return &schedule, nil
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](sender),
	)
}
