package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgMigrate{}
	_ sdk.HasValidateBasic = &MsgMigrate{}
	_ sdk.LegacyMsg        = &MsgMigrate{}
)

// NewMsgMigrate is a constructor function for MsgMigrate
func NewMsgMigrate(tx common.ObservedTx, blockHeight int64, signer cosmos.AccAddress) *MsgMigrate {
	return &MsgMigrate{
		Tx:          tx,
		BlockHeight: blockHeight,
		Signer:      signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgMigrate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.BlockHeight <= 0 {
		return cosmos.ErrUnknownRequest("invalid block height")
	}
	if err := m.Tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgMigrate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
