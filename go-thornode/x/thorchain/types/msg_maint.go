package types

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgMaint{}
	_ sdk.HasValidateBasic = &MsgMaint{}
	_ sdk.LegacyMsg        = &MsgMaint{}
)

// NewMsgMaint create new MsgMaint message
func NewMsgMaint(nodeAddr, signer cosmos.AccAddress) *MsgMaint {
	return &MsgMaint{
		NodeAddress: nodeAddr,
		Signer:      signer,
	}
}

// Route should return the router key of the module
func (m *MsgMaint) Route() string { return RouterKey }

// Type should return the action
func (m *MsgMaint) Type() string { return "maint" }

// ValidateBasic runs stateless checks on the message
func (m *MsgMaint) ValidateBasic() error {
	if err := cosmos.VerifyAddressFormat(m.Signer); err != nil {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if err := cosmos.VerifyAddressFormat(m.NodeAddress); err != nil {
		return cosmos.ErrInvalidAddress(m.NodeAddress.String())
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (m *MsgMaint) GetSignBytes() []byte {
	return cosmos.MustSortJSON(ModuleCdc.MustMarshalJSON(m))
}

// GetSigners defines whose signature is required
func (m *MsgMaint) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgMaintCustomGetSigners(m proto.Message) ([][]byte, error) {
	msgMaint, ok := m.(*types.MsgMaint)
	if !ok {
		return nil, errors.New("can't cast as MsgMaint")
	}
	return [][]byte{msgMaint.Signer}, nil
}
