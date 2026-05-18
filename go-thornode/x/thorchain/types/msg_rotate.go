package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgOperatorRotate{}
	_ sdk.HasValidateBasic = &MsgOperatorRotate{}
	_ sdk.LegacyMsg        = &MsgOperatorRotate{}
)

// NewMsgOperatorRotate is a constructor function for MsgOperatorRotate
func NewMsgOperatorRotate(signer, operatorAddress cosmos.AccAddress, coin common.Coin) *MsgOperatorRotate {
	return &MsgOperatorRotate{
		Signer:          signer,
		OperatorAddress: operatorAddress,
		Coin:            coin,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgOperatorRotate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrUnknownRequest("signer cannot be empty")
	}
	if m.OperatorAddress.Empty() {
		return cosmos.ErrUnknownRequest("operator address cannot be empty")
	}
	if !m.Coin.Amount.IsZero() {
		return cosmos.ErrUnknownRequest("coin amount must be zero")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgOperatorRotate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
