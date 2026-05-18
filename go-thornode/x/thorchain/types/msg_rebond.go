package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func NewMsgReBond(
	txin common.Tx,
	nodeAddress cosmos.AccAddress,
	newProvider cosmos.AccAddress,
	amount cosmos.Uint,
	signer cosmos.AccAddress,
) *MsgReBond {
	return &MsgReBond{
		TxIn:                   txin,
		NodeAddress:            nodeAddress,
		NewBondProviderAddress: newProvider,
		Amount:                 amount,
		Signer:                 signer,
	}
}

func (m *MsgReBond) ValidateBasic() error {
	if m.NodeAddress.Empty() {
		return cosmos.ErrInvalidAddress("node address cannot be empty")
	}
	if m.NewBondProviderAddress.Empty() {
		return cosmos.ErrInvalidAddress("new bond address cannot be empty")
	}
	// here we can't call m.TxIn.Valid , because we allow user to send rebond request without any coins in it
	// m.TxIn.Valid will reject this kind request , which result rebond to fail
	if m.TxIn.ID.IsEmpty() {
		return cosmos.ErrUnknownRequest("tx id cannot be empty")
	}
	if m.TxIn.FromAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("tx from address cannot be empty")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("empty signer address")
	}
	return nil
}

func (m *MsgReBond) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
