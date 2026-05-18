package types

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

var (
	_ sdk.Msg              = &MsgTCYUnstake{}
	_ sdk.HasValidateBasic = &MsgTCYUnstake{}
	_ sdk.LegacyMsg        = &MsgTCYUnstake{}
)

// NewMsgTCYUnstake create new MsgTCYUnstake message
func NewMsgTCYUnstake(tx common.Tx, basisPoints math.Uint, signer sdk.AccAddress) *MsgTCYUnstake {
	return &MsgTCYUnstake{
		Signer:      signer,
		Tx:          tx,
		BasisPoints: basisPoints,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgTCYUnstake) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer must not be empty")
	}
	if m.BasisPoints.IsZero() || m.BasisPoints.GT(cosmos.NewUint(constants.MaxBasisPts)) {
		return cosmos.ErrUnknownRequest("invalid basis points")
	}
	if !m.Tx.FromAddress.IsChain(common.THORChain) {
		return cosmos.ErrInvalidAddress("address should be rune address")
	}
	if !m.Tx.Coins.IsEmpty() {
		return cosmos.ErrInvalidCoins("coins must be empty (zero amount)")
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTCYUnstake) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
