package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgTCYClaim{}
	_ sdk.HasValidateBasic = &MsgTCYClaim{}
	_ sdk.LegacyMsg        = &MsgTCYClaim{}
)

// NewMsgTCYClaim create new MsgTCYClaim message
func NewMsgTCYClaim(address, l1Address common.Address, signer sdk.AccAddress) *MsgTCYClaim {
	return &MsgTCYClaim{
		RuneAddress: address,
		L1Address:   l1Address,
		Signer:      signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgTCYClaim) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.RuneAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("rune addresses cannot be empty")
	}
	if m.L1Address.IsEmpty() {
		return cosmos.ErrInvalidAddress("l1 addresses cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTCYClaim) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
