package types_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func TestRegisterCodec(t *testing.T) {
	cdc := codec.NewLegacyAmino()
	types.RegisterCodec(cdc)
	// Verify registered types can be marshaled
	msg := types.NewMsgCreateDenom(validAddr, "bitcoin")
	bz, err := cdc.MarshalJSON(msg)
	require.NoError(t, err)
	require.NotEmpty(t, bz)
}

func TestRegisterInterfaces(t *testing.T) {
	registry := cdctypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	// Verify registered implementations resolve
	any, err := cdctypes.NewAnyWithValue(types.NewMsgCreateDenom(validAddr, "bitcoin"))
	require.NoError(t, err)
	require.NotNil(t, any)
}
