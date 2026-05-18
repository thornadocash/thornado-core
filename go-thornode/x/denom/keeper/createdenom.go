package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func (k Keeper) CreateDenom(ctx sdk.Context, denomId, adminAddr string) (newTokenDenom string, err error) {
	denom, err := types.GetTokenDenom(denomId)
	if err != nil {
		return "", err
	}

	admin, err := sdk.AccAddressFromBech32(adminAddr)
	if err != nil {
		return "", err
	}

	err = k.SetAdmin(ctx, denom, &admin)
	if err != nil {
		return "", err
	}

	return denom, nil
}
