package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgReserveContributor{}
	_ sdk.HasValidateBasic = &MsgReserveContributor{}
	_ sdk.LegacyMsg        = &MsgReserveContributor{}
)

// NewMsgReserveContributor is a constructor function for MsgReserveContributor
func NewMsgReserveContributor(tx common.Tx, contrib ReserveContributor, signer cosmos.AccAddress) *MsgReserveContributor {
	return &MsgReserveContributor{
		Tx:          tx,
		Contributor: contrib,
		Signer:      signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgReserveContributor) ValidateBasic() error {
	if err := m.Tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if err := m.Contributor.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgReserveContributor) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
