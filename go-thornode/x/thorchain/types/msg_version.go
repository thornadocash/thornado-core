package types

import (
	"fmt"

	"github.com/blang/semver"

	"google.golang.org/protobuf/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSetVersion{}
	_ sdk.HasValidateBasic = &MsgSetVersion{}
	_ sdk.LegacyMsg        = &MsgSetVersion{}
)

// NewMsgSetVersion is a constructor function for NewMsgSetVersion
func NewMsgSetVersion(version string, signer cosmos.AccAddress) *MsgSetVersion {
	return &MsgSetVersion{
		Version: version,
		Signer:  signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgSetVersion) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := semver.Make(m.Version); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	return nil
}

// GetVersion return the semantic version
func (m *MsgSetVersion) GetVersion() (semver.Version, error) {
	return semver.Make(m.Version)
}

// GetSigners defines whose signature is required
func (m *MsgSetVersion) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgSetVersionCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgSetVersion)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgSetVersion: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
