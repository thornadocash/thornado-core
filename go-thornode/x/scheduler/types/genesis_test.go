package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

func TestDefaultGenesis(t *testing.T) {
	gs := types.DefaultGenesis()
	require.NotNil(t, gs)
	require.Empty(t, gs.Schedules)
}

func TestGenesisState_Validate(t *testing.T) {
	tests := []struct {
		name     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			name:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			name: "valid with schedule",
			genState: &types.GenesisState{
				Schedules: []types.Schedule{
					{
						Height: 100,
						Msgs: []types.MsgScheduleExecuteContract{
							{Sender: validAddr, After: 50, Msg: []byte(`{}`)},
						},
					},
				},
			},
			valid: true,
		},
		{
			name: "invalid height zero",
			genState: &types.GenesisState{
				Schedules: []types.Schedule{
					{
						Height: 0,
						Msgs:   []types.MsgScheduleExecuteContract{},
					},
				},
			},
			valid: false,
		},
		{
			name: "invalid message in schedule",
			genState: &types.GenesisState{
				Schedules: []types.Schedule{
					{
						Height: 100,
						Msgs: []types.MsgScheduleExecuteContract{
							{Sender: "invalid", After: 50, Msg: []byte(`{}`)},
						},
					},
				},
			},
			valid: false,
		},
		{
			name: "empty msgs is valid",
			genState: &types.GenesisState{
				Schedules: []types.Schedule{
					{
						Height: 100,
						Msgs:   []types.MsgScheduleExecuteContract{},
					},
				},
			},
			valid: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
