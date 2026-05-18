package types_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

func TestRegisterCodec(t *testing.T) {
	cdc := codec.NewLegacyAmino()
	types.RegisterCodec(cdc)
	msg := types.NewMsgScheduleExecuteContract(validAddr, 100, []byte(`{}`))
	bz, err := cdc.MarshalJSON(msg)
	require.NoError(t, err)
	require.NotEmpty(t, bz)
}

func TestRegisterInterfaces(t *testing.T) {
	registry := cdctypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	any, err := cdctypes.NewAnyWithValue(types.NewMsgScheduleExecuteContract(validAddr, 100, []byte(`{}`)))
	require.NoError(t, err)
	require.NotNil(t, any)
}
