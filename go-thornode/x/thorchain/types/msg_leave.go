package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgLeave{}
	_ sdk.HasValidateBasic = &MsgLeave{}
	_ sdk.LegacyMsg        = &MsgLeave{}
)

// NewMsgLeave create a new instance of MsgLeave
func NewMsgLeave(tx common.Tx, addr, signer cosmos.AccAddress) *MsgLeave {
	return &MsgLeave{
		Tx:          tx,
		NodeAddress: addr,
		Signer:      signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgLeave) ValidateBasic() error {
	if m.Tx.FromAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("from address cannot be empty")
	}
	// here we can't call m.Tx.Valid , because we allow user to send leave request without any coins in it
	// m.Tx.Valid will reject this kind request , which result leave to fail
	if m.Tx.ID.IsEmpty() {
		return cosmos.ErrUnknownRequest("tx id cannot be empty")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer cannot be empty ")
	}
	if m.NodeAddress.Empty() {
		return cosmos.ErrInvalidAddress("node address cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgLeave) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
