package types_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func TestGetTokenDenom(t *testing.T) {
	// valid id
	denom, err := types.GetTokenDenom("bitcoin")
	require.NoError(t, err)
	require.Equal(t, "x/bitcoin", denom)

	// empty id produces invalid denom
	_, err = types.GetTokenDenom("")
	require.Error(t, err)

	// too long id
	longID := "adsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsf"
	_, err = types.GetTokenDenom(longID)
	require.Error(t, err)
}

func TestNewDenomMintCoinsRestriction(t *testing.T) {
	restriction := types.NewDenomMintCoinsRestriction()

	// valid denom coins
	err := restriction(nil, sdk.NewCoins(sdk.NewInt64Coin("x/bitcoin", 100)))
	require.NoError(t, err)

	// invalid denom coins
	err = restriction(nil, sdk.NewCoins(sdk.NewInt64Coin("uatom", 100)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not have permission to mint")
}

func TestDecomposeDenoms(t *testing.T) {
	for _, tc := range []struct {
		desc  string
		denom string
		valid bool
	}{
		{
			desc:  "empty is invalid",
			denom: "",
			valid: false,
		},
		{
			desc:  "normal",
			denom: "x/bitcoin",
			valid: true,
		},
		{
			desc:  "multiple slashes in id",
			denom: "x/bitcoin/1",
			valid: true,
		},
		{
			desc:  "no id",
			denom: "x/",
			valid: false,
		},
		{
			desc:  "incorrect prefix",
			denom: "ibc/bitcoin",
			valid: false,
		},
		{
			desc:  "id of only slashes",
			denom: "x/////",
			valid: true,
		},
		{
			desc:  "too long name",
			denom: "x/adsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsfadsf",
			valid: false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := types.DeconstructDenom(tc.denom)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
