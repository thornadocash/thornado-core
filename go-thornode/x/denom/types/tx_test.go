package types_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func validMetadata() banktypes.Metadata {
	return banktypes.Metadata{
		Name:        "Bitcoin",
		Symbol:      "BTC",
		Description: "test token",
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: "x/bitcoin", Exponent: 0},
		},
		Base:    "x/bitcoin",
		Display: "x/bitcoin",
	}
}

const (
	validAddr  = "cosmos1t7egva48prqmzl59x5ngv4zx0dtrwewcdqdjr8"
	validAddr2 = "cosmos1ft6e5esdtdegnvcr3djd3ftk4kwpcr6jta8eyh"
)

func TestNewMsgCreateDenom(t *testing.T) {
	msg := types.NewMsgCreateDenom(validAddr, "bitcoin")
	require.Equal(t, validAddr, msg.Sender)
	require.Equal(t, "bitcoin", msg.Id)
}

func TestMsgCreateDenom_ValidateBasic(t *testing.T) {
	validMsg := types.NewMsgCreateDenom(validAddr, "bitcoin")
	validMsg.Metadata = validMetadata()

	invalidSenderMsg := types.NewMsgCreateDenom("invalid", "bitcoin")
	invalidSenderMsg.Metadata = validMetadata()

	emptyIDMsg := types.NewMsgCreateDenom(validAddr, "")
	emptyIDMsg.Metadata = validMetadata()

	invalidMetaMsg := types.NewMsgCreateDenom(validAddr, "bitcoin")
	// leave Metadata as zero value (invalid)

	tests := []struct {
		name  string
		msg   *types.MsgCreateDenom
		valid bool
	}{
		{
			name:  "valid",
			msg:   validMsg,
			valid: true,
		},
		{
			name:  "invalid sender",
			msg:   invalidSenderMsg,
			valid: false,
		},
		{
			name:  "invalid denom id - empty",
			msg:   emptyIDMsg,
			valid: false,
		},
		{
			name:  "invalid metadata",
			msg:   invalidMetaMsg,
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

func TestMsgCreateDenom_GetSigners(t *testing.T) {
	msg := types.NewMsgCreateDenom(validAddr, "bitcoin")
	signers := msg.GetSigners()
	require.Len(t, signers, 1)
	addr, _ := sdk.AccAddressFromBech32(validAddr)
	require.Equal(t, addr, signers[0])
}

func TestNewMsgMintTokens(t *testing.T) {
	coin := sdk.NewInt64Coin("x/bitcoin", 100)
	msg := types.NewMsgMintTokens(validAddr, coin, validAddr2)
	require.Equal(t, validAddr, msg.Sender)
	require.Equal(t, coin, msg.Amount)
	require.Equal(t, validAddr2, msg.Recipient)
}

func TestMsgMintTokens_ValidateBasic(t *testing.T) {
	validCoin := sdk.NewInt64Coin("x/bitcoin", 100)
	tests := []struct {
		name  string
		msg   *types.MsgMintTokens
		valid bool
	}{
		{
			name:  "valid",
			msg:   types.NewMsgMintTokens(validAddr, validCoin, validAddr2),
			valid: true,
		},
		{
			name:  "invalid sender",
			msg:   types.NewMsgMintTokens("invalid", validCoin, validAddr2),
			valid: false,
		},
		{
			name:  "invalid recipient",
			msg:   types.NewMsgMintTokens(validAddr, validCoin, "invalid"),
			valid: false,
		},
		{
			name: "invalid amount",
			msg: &types.MsgMintTokens{
				Sender:    validAddr,
				Amount:    sdk.Coin{Denom: "x/bitcoin", Amount: math.NewInt(-1)},
				Recipient: validAddr2,
			},
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

func TestMsgMintTokens_GetSigners(t *testing.T) {
	coin := sdk.NewInt64Coin("x/bitcoin", 100)
	msg := types.NewMsgMintTokens(validAddr, coin, validAddr2)
	signers := msg.GetSigners()
	require.Len(t, signers, 1)
	addr, _ := sdk.AccAddressFromBech32(validAddr)
	require.Equal(t, addr, signers[0])
}

func TestNewMsgBurnTokens(t *testing.T) {
	coin := sdk.NewInt64Coin("x/bitcoin", 50)
	msg := types.NewMsgBurnTokens(validAddr, coin)
	require.Equal(t, validAddr, msg.Sender)
	require.Equal(t, coin, msg.Amount)
}

func TestMsgBurnTokens_ValidateBasic(t *testing.T) {
	validCoin := sdk.NewInt64Coin("x/bitcoin", 50)
	tests := []struct {
		name  string
		msg   *types.MsgBurnTokens
		valid bool
	}{
		{
			name:  "valid",
			msg:   types.NewMsgBurnTokens(validAddr, validCoin),
			valid: true,
		},
		{
			name:  "invalid sender",
			msg:   types.NewMsgBurnTokens("invalid", validCoin),
			valid: false,
		},
		{
			name: "invalid amount",
			msg: &types.MsgBurnTokens{
				Sender: validAddr,
				Amount: sdk.Coin{Denom: "x/bitcoin", Amount: math.NewInt(-1)},
			},
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

func TestMsgBurnTokens_GetSigners(t *testing.T) {
	coin := sdk.NewInt64Coin("x/bitcoin", 50)
	msg := types.NewMsgBurnTokens(validAddr, coin)
	signers := msg.GetSigners()
	require.Len(t, signers, 1)
	addr, _ := sdk.AccAddressFromBech32(validAddr)
	require.Equal(t, addr, signers[0])
}

func TestNewMsgChangeDenomAdmin(t *testing.T) {
	msg := types.NewMsgChangeDenomAdmin(validAddr, "x/bitcoin", validAddr2)
	require.Equal(t, validAddr, msg.Sender)
	require.Equal(t, "x/bitcoin", msg.Denom)
	require.Equal(t, validAddr2, msg.NewAdmin)
}

func TestMsgChangeDenomAdmin_ValidateBasic(t *testing.T) {
	tests := []struct {
		name  string
		msg   *types.MsgChangeDenomAdmin
		valid bool
	}{
		{
			name:  "valid",
			msg:   types.NewMsgChangeDenomAdmin(validAddr, "x/bitcoin", validAddr2),
			valid: true,
		},
		{
			name:  "invalid sender",
			msg:   types.NewMsgChangeDenomAdmin("invalid", "x/bitcoin", validAddr2),
			valid: false,
		},
		{
			name:  "invalid new admin",
			msg:   types.NewMsgChangeDenomAdmin(validAddr, "x/bitcoin", "invalid"),
			valid: false,
		},
		{
			name:  "invalid denom",
			msg:   types.NewMsgChangeDenomAdmin(validAddr, "notvalid", validAddr2),
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

func TestMsgChangeDenomAdmin_GetSigners(t *testing.T) {
	msg := types.NewMsgChangeDenomAdmin(validAddr, "x/bitcoin", validAddr2)
	signers := msg.GetSigners()
	require.Len(t, signers, 1)
	addr, _ := sdk.AccAddressFromBech32(validAddr)
	require.Equal(t, addr, signers[0])
}
