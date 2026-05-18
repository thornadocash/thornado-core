package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgConsolidate{}
	_ sdk.HasValidateBasic = &MsgConsolidate{}
	_ sdk.LegacyMsg        = &MsgConsolidate{}
)

// NewMsgConsolidate is a constructor function for MsgConsolidate
func NewMsgConsolidate(observedTx common.ObservedTx, signer cosmos.AccAddress) *MsgConsolidate {
	return &MsgConsolidate{
		ObservedTx: observedTx,
		Signer:     signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgConsolidate) ValidateBasic() error {
	if err := m.ObservedTx.Valid(); err != nil {
		return cosmos.ErrInvalidCoins(err.Error())
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgConsolidate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
