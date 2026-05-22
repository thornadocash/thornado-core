package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSend{}
	_ sdk.HasValidateBasic = &MsgSend{}
	_ sdk.LegacyMsg        = &MsgSend{}
)

// NewMsgConfig is a constructor function for MsgConfig
func NewMsgConfig(key string, value int64, signer cosmos.AccAddress) *MsgConfig {
	return &MsgConfig{
		Key:    key,
		Value:  value,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgConfig) ValidateBasic() error {
	if m.Key == "" {
		return cosmos.ErrUnknownRequest("key cannot be empty")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgConfig) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgConfigCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgConfig)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgConfig: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
