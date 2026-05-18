package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func TestGenesisState_Validate(t *testing.T) {
	for _, tc := range []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc: "valid genesis state",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "cosmos1t7egva48prqmzl59x5ngv4zx0dtrwewcdqdjr8",
					},
				},
			},
			valid: true,
		},
		{
			desc: "different admin from creator",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "cosmos1ft6e5esdtdegnvcr3djd3ftk4kwpcr6jta8eyh",
					},
				},
			},
			valid: true,
		},
		{
			desc: "empty admin",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "",
					},
				},
			},
			valid: true,
		},
		{
			desc: "no admin",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
					},
				},
			},
			valid: true,
		},
		{
			desc: "invalid denom - wrong prefix",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "ibc/bitcoin",
						Admin: "",
					},
				},
			},
			valid: false,
		},
		{
			desc: "invalid admin",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "moose",
					},
				},
			},
			valid: false,
		},
		{
			desc: "multiple denoms",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "",
					},
					{
						Denom: "x/litecoin",
						Admin: "",
					},
				},
			},
			valid: true,
		},
		{
			desc: "duplicate denoms",
			genState: &types.GenesisState{
				Admins: []types.GenesisDenom{
					{
						Denom: "x/bitcoin",
						Admin: "",
					},
					{
						Denom: "x/bitcoin",
						Admin: "",
					},
				},
			},
			valid: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
