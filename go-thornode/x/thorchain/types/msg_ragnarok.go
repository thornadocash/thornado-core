package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgRagnarok{}
	_ sdk.HasValidateBasic = &MsgRagnarok{}
	_ sdk.LegacyMsg        = &MsgRagnarok{}
)

// NewMsgRagnarok is a constructor function for MsgRagnarok
func NewMsgRagnarok(tx common.ObservedTx, blockHeight int64, signer cosmos.AccAddress) *MsgRagnarok {
	return &MsgRagnarok{
		Tx:          tx,
		BlockHeight: blockHeight,
		Signer:      signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgRagnarok) ValidateBasic() error {
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
func (m *MsgRagnarok) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
