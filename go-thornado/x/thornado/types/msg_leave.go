package types

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgLeave{}
	_ sdk.HasValidateBasic = &MsgLeave{}
	_ sdk.LegacyMsg        = &MsgLeave{}
)

func NewMsgLeave(nodeAddr, signer cosmos.AccAddress) *MsgLeave {
	return &MsgLeave{
		NodeAddress: nodeAddr,
		Signer:      signer,
	}
}

func (m *MsgLeave) Route() string { return RouterKey }

func (m *MsgLeave) Type() string { return "leave" }

func (m *MsgLeave) ValidateBasic() error {
	if err := cosmos.VerifyAddressFormat(m.Signer); err != nil {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if err := cosmos.VerifyAddressFormat(m.NodeAddress); err != nil {
		return cosmos.ErrInvalidAddress(m.NodeAddress.String())
	}
	return nil
}

func (m *MsgLeave) GetSignBytes() []byte {
	return cosmos.MustSortJSON(ModuleCdc.MustMarshalJSON(m))
}

func (m *MsgLeave) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgLeaveCustomGetSigners(m proto.Message) ([][]byte, error) {
	msgLeave, ok := m.(*types.MsgLeave)
	if !ok {
		return nil, errors.New("can't cast as MsgLeave")
	}
	return [][]byte{msgLeave.Signer}, nil
}
