package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgStoreMigrate{}
	_ sdk.HasValidateBasic = &MsgStoreMigrate{}
	_ sdk.LegacyMsg        = &MsgStoreMigrate{}
)

// NewMsgStoreMigrate is a constructor function for MsgStoreMigrate
func NewMsgStoreMigrate(key, value string, signer cosmos.AccAddress) *MsgStoreMigrate {
	return &MsgStoreMigrate{
		Key:    key,
		Value:  value,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
func (m *MsgStoreMigrate) ValidateBasic() error {
	if m.Key == "" {
		return cosmos.ErrUnknownRequest("key cannot be empty")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgStoreMigrate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgStoreMigrateCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgStoreMigrate)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgStoreMigrate: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
