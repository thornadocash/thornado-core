package keeper

import (
	"errors"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func (k Keeper) Admins() collections.Map[string, sdk.AccAddress] {
	return collections.NewMap(
		collections.NewSchemaBuilder(k.storeService),
		types.DenomAdminPrefix, types.DenomAdminKey,
		codec.NewStringKeyCodec[string](),
		codec.KeyToValueCodec(sdk.AccAddressKey),
	)
}

// GetAdmin returns the authority metadata for a specific denom
func (k Keeper) GetAdmin(ctx sdk.Context, denom string) (sdk.AccAddress, error) {
	admin, err := k.Admins().Get(ctx, denom)
	if errors.Is(err, collections.ErrNotFound) {
		return nil, nil
	}
	return admin, err
}

// SetAdmin stores authority metadata for a specific denom
func (k Keeper) SetAdmin(ctx sdk.Context, denom string, addr *sdk.AccAddress) error {
	if addr == nil {
		return k.Admins().Remove(ctx, denom)
	}
	return k.Admins().Set(ctx, denom, *addr)
}
