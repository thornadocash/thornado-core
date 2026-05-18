package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgErrataTx{}
	_ sdk.HasValidateBasic = &MsgErrataTx{}
	_ sdk.LegacyMsg        = &MsgErrataTx{}
)

// NewMsgErrataTx is a constructor function for NewMsgErrataTx
func NewMsgErrataTx(txID common.TxID, chain common.Chain, signer cosmos.AccAddress) *MsgErrataTx {
	return &MsgErrataTx{
		TxID:   txID,
		Chain:  chain,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgErrataTx) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.TxID.IsEmpty() {
		return cosmos.ErrUnknownRequest("Tx ID cannot be empty")
	}
	if m.Chain.IsEmpty() {
		return cosmos.ErrUnknownRequest("chain cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgErrataTx) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgErrataCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgErrataTx)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgErrataTx: %T", m)
	}

	return [][]byte{msg.Signer}, nil
}
