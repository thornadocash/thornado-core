package types

import (
	"fmt"
	"net"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSetIPAddress{}
	_ sdk.HasValidateBasic = &MsgSetIPAddress{}
	_ sdk.LegacyMsg        = &MsgSetIPAddress{}
)

// NewMsgSetIPAddress is a constructor function for NewMsgSetIPAddress
func NewMsgSetIPAddress(ip string, signer cosmos.AccAddress) *MsgSetIPAddress {
	return &MsgSetIPAddress{
		IPAddress: ip,
		Signer:    signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgSetIPAddress) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if net.ParseIP(m.IPAddress) == nil {
		return cosmos.ErrUnknownRequest("invalid IP address")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgSetIPAddress) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgSetIPAddressCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgSetIPAddress)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgSetIPAddress: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
