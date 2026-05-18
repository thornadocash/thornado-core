package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgNodePauseChain{}
	_ sdk.HasValidateBasic = &MsgNodePauseChain{}
	_ sdk.LegacyMsg        = &MsgNodePauseChain{}
)

// NewMsgNodePauseChain is a constructor function for NewMsgNodePauseChain
func NewMsgNodePauseChain(val int64, signer cosmos.AccAddress) *MsgNodePauseChain {
	return &MsgNodePauseChain{
		Value:  val,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgNodePauseChain) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	return nil
}

// GetSigners return all the signer who signed this message
func (m *MsgNodePauseChain) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodePauseChainCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgNodePauseChain)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodePauseChain: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
