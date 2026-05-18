package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgNoOp{}
	_ sdk.HasValidateBasic = &MsgNoOp{}
	_ sdk.LegacyMsg        = &MsgNoOp{}
)

// NewMsgNoOp is a constructor function for MsgNoOp
func NewMsgNoOp(observedTx common.ObservedTx, signer cosmos.AccAddress, action string) *MsgNoOp {
	return &MsgNoOp{
		ObservedTx: observedTx,
		Signer:     signer,
		Action:     action,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgNoOp) ValidateBasic() error {
	if err := m.ObservedTx.Valid(); err != nil {
		return cosmos.ErrInvalidCoins(err.Error())
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgNoOp) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
