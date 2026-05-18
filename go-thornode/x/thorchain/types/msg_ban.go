package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgBan{}
	_ sdk.HasValidateBasic = &MsgBan{}
	_ sdk.LegacyMsg        = &MsgBan{}
)

// NewMsgBan is a constructor function for NewMsgBan
func NewMsgBan(addr, signer cosmos.AccAddress) *MsgBan {
	return &MsgBan{
		NodeAddress: addr,
		Signer:      signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgBan) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.NodeAddress.Empty() {
		return cosmos.ErrInvalidAddress(m.NodeAddress.String())
	}
	return nil
}

// GetSigners return all the signer who signed this message
// Implements LegacyMsg.
func (m *MsgBan) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgBanCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgBan)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgBan: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
