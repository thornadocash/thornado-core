package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
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

func (server msgServer) CreateDenom(goCtx context.Context, msg *types.MsgCreateDenom) (*types.MsgCreateDenomResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	denom, err := server.Keeper.CreateDenom(ctx, msg.Id, msg.Sender)
	if err != nil {
		return nil, err
	}
	_, found := server.bankKeeper.GetDenomMetaData(ctx, denom)
	if found {
		return nil, types.ErrDenomExists
	}

	metadata := msg.GetMetadata()
	metadata.Base = denom
	server.bankKeeper.SetDenomMetaData(ctx, metadata)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventCreateDenom,
			sdk.NewAttribute(types.AttributeCreator, msg.Sender),
			sdk.NewAttribute(types.AttributeNewTokenDenom, denom),
		),
	})

	return &types.MsgCreateDenomResponse{
		NewTokenDenom: denom,
	}, nil
}

func (server msgServer) MintTokens(goCtx context.Context, msg *types.MsgMintTokens) (*types.MsgMintTokensResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	admin, err := server.Keeper.GetAdmin(ctx, msg.Amount.GetDenom())
	if err != nil {
		return nil, err
	}

	if msg.Sender != admin.String() {
		return nil, types.ErrUnauthorized
	}

	err = server.Keeper.mintTo(ctx, msg.Amount, msg.Recipient)
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventMintTokens,
			sdk.NewAttribute(types.AttributeMintToAddress, msg.Sender),
			sdk.NewAttribute(types.AttributeAmount, msg.Amount.String()),
		),
	})

	return &types.MsgMintTokensResponse{}, nil
}

func (server msgServer) BurnTokens(goCtx context.Context, msg *types.MsgBurnTokens) (*types.MsgBurnTokensResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	admin, err := server.Keeper.GetAdmin(ctx, msg.Amount.GetDenom())
	if err != nil {
		return nil, err
	}

	if msg.Sender != admin.String() {
		return nil, types.ErrUnauthorized
	}

	err = server.Keeper.burnFrom(ctx, msg.Amount, msg.Sender)
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventBurnTokens,
			sdk.NewAttribute(types.AttributeBurnFromAddress, msg.Sender),
			sdk.NewAttribute(types.AttributeAmount, msg.Amount.String()),
		),
	})

	return &types.MsgBurnTokensResponse{}, nil
}

func (server msgServer) ChangeDenomAdmin(goCtx context.Context, msg *types.MsgChangeDenomAdmin) (*types.MsgChangeDenomAdminResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	admin, err := server.Keeper.GetAdmin(ctx, msg.Denom)
	if err != nil {
		return nil, err
	}

	if msg.Sender != admin.String() {
		return nil, types.ErrUnauthorized
	}

	if msg.NewAdmin == "" {
		err = server.Keeper.SetAdmin(ctx, msg.Denom, nil)
		if err != nil {
			return nil, err
		}
	} else {
		admin, err = sdk.AccAddressFromBech32(msg.NewAdmin)
		if err != nil {
			return nil, err
		}
		err = server.Keeper.SetAdmin(ctx, msg.Denom, &admin)
		if err != nil {
			return nil, err
		}
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventChangeDenomAdmin,
			sdk.NewAttribute(types.AttributeDenom, msg.GetDenom()),
			sdk.NewAttribute(types.AttributeNewAdmin, msg.NewAdmin),
		),
	})

	return &types.MsgChangeDenomAdminResponse{}, nil
}
