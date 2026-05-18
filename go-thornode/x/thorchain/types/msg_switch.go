package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSwitch{}
	_ sdk.HasValidateBasic = &MsgSwitch{}
	_ sdk.LegacyMsg        = &MsgSwitch{}
)

// NewMsgSwitch is a constructor function for MsgSwitch
func NewMsgSwitch(asset common.Asset, amount cosmos.Uint, acc, signer cosmos.AccAddress, tx common.Tx) *MsgSwitch {
	return &MsgSwitch{
		Tx:      tx,
		Asset:   asset,
		Amount:  amount,
		Address: acc,
		Signer:  signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgSwitch) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset cannot be empty")
	}
	if m.Asset.IsNative() {
		return cosmos.ErrUnknownRequest("native assets cannot be switched")
	}
	if m.Amount.IsZero() {
		return cosmos.ErrUnknownRequest("amount cannot be zero")
	}
	if m.Address.Empty() {
		return cosmos.ErrInvalidAddress(m.Address.String())
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Tx.ID.IsEmpty() {
		return cosmos.ErrUnknownRequest("txID cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgSwitch) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
