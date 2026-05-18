package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/blang/semver"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgProposeUpgrade{}
	_ sdk.HasValidateBasic = &MsgProposeUpgrade{}
	_ sdk.LegacyMsg        = &MsgProposeUpgrade{}

	_ sdk.Msg              = &MsgApproveUpgrade{}
	_ sdk.HasValidateBasic = &MsgApproveUpgrade{}
	_ sdk.LegacyMsg        = &MsgApproveUpgrade{}

	_ sdk.Msg              = &MsgRejectUpgrade{}
	_ sdk.HasValidateBasic = &MsgRejectUpgrade{}
	_ sdk.LegacyMsg        = &MsgRejectUpgrade{}
)

// NewMsgProposeUpgrade is a constructor function for NewMsgProposeUpgrade
func NewMsgProposeUpgrade(name string, height int64, info string, signer cosmos.AccAddress) *MsgProposeUpgrade {
	return &MsgProposeUpgrade{
		Name: name,
		Upgrade: Upgrade{
			Height: height,
			Info:   info,
		},
		Signer: signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgProposeUpgrade) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if len(m.Name) == 0 {
		return cosmos.ErrUnknownRequest("name cannot be empty")
	}
	if len(m.Name) > 100 {
		return cosmos.ErrUnknownRequest("name cannot be longer than 100 characters")
	}
	if _, err := semver.Parse(m.Name); err != nil {
		return cosmos.ErrUnknownRequest("name is not a valid semver")
	}
	if len(m.Upgrade.Info) > 2500 {
		return cosmos.ErrUnknownRequest("info cannot be longer than 2500 characters")
	}
	if m.Upgrade.Height == 0 {
		return cosmos.ErrUnknownRequest("height cannot be zero")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgProposeUpgrade) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgProposeUpgradeCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgProposeUpgrade)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgProposeUpgrade: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

// NewMsgApproveUpgrade is a constructor function for NewMsgApproveUpgrade
func NewMsgApproveUpgrade(name string, signer cosmos.AccAddress) *MsgApproveUpgrade {
	return &MsgApproveUpgrade{
		Name:   name,
		Signer: signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgApproveUpgrade) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Name == "" {
		return cosmos.ErrUnknownRequest("name cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgApproveUpgrade) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgApproveUpgradeCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgApproveUpgrade)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgApproveUpgrade: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

// NewMsgRejectUpgrade is a constructor function for NewMsgRejectUpgrade
func NewMsgRejectUpgrade(name string, signer cosmos.AccAddress) *MsgRejectUpgrade {
	return &MsgRejectUpgrade{
		Name:   name,
		Signer: signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgRejectUpgrade) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Name == "" {
		return cosmos.ErrUnknownRequest("name cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgRejectUpgrade) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgRejectUpgradeCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgRejectUpgrade)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgRejectUpgrade: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
