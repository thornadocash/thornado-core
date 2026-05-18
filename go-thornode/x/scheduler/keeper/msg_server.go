package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (server msgServer) ScheduleExecuteContract(goCtx context.Context, msg *types.MsgScheduleExecuteContract) (*types.MsgScheduleExecuteContractResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	err := server.Keeper.AddMsg(ctx, *msg)
	if err != nil {
		return nil, err
	}
	return &types.MsgScheduleExecuteContractResponse{}, nil
}
