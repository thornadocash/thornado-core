package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgRefundTx{}
	_ sdk.HasValidateBasic = &MsgRefundTx{}
	_ sdk.LegacyMsg        = &MsgRefundTx{}
)

// NewMsgRefundTx is a constructor function for MsgOutboundTx
func NewMsgRefundTx(tx common.ObservedTx, txID common.TxID, signer cosmos.AccAddress) *MsgRefundTx {
	return &MsgRefundTx{
		Tx:     tx,
		InTxID: txID,
		Signer: signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgRefundTx) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.InTxID.IsEmpty() {
		return cosmos.ErrUnknownRequest("In Tx ID cannot be empty")
	}
	if err := m.Tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgRefundTx) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
