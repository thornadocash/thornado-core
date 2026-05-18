package types_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

const validAddr = "cosmos1t7egva48prqmzl59x5ngv4zx0dtrwewcdqdjr8"

func TestNewMsgScheduleExecuteContract(t *testing.T) {
	msg := types.NewMsgScheduleExecuteContract(validAddr, 100, []byte(`{"execute":{}}`))
	require.Equal(t, validAddr, msg.Sender)
	require.Equal(t, uint64(100), msg.After)
	require.Equal(t, []byte(`{"execute":{}}`), msg.Msg)
}

func TestMsgScheduleExecuteContract_ValidateBasic(t *testing.T) {
	tests := []struct {
		name  string
		msg   *types.MsgScheduleExecuteContract
		valid bool
	}{
		{
			name:  "valid",
			msg:   types.NewMsgScheduleExecuteContract(validAddr, 100, []byte(`{}`)),
			valid: true,
		},
		{
			name:  "invalid sender",
			msg:   types.NewMsgScheduleExecuteContract("invalid", 100, []byte(`{}`)),
			valid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.ValidateBasic()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestMsgScheduleExecuteContract_GetSigners(t *testing.T) {
	msg := types.NewMsgScheduleExecuteContract(validAddr, 100, []byte(`{}`))
	signers := msg.GetSigners()
	require.Len(t, signers, 1)
	addr, _ := sdk.AccAddressFromBech32(validAddr)
	require.Equal(t, addr, signers[0])
}
