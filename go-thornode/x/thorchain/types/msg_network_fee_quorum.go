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
	_ sdk.Msg              = &MsgNetworkFeeQuorum{}
	_ sdk.HasValidateBasic = &MsgNetworkFeeQuorum{}
	_ sdk.LegacyMsg        = &MsgNetworkFeeQuorum{}
)

// NewMsgNetworkFee create a new instance of MsgNetworkFee
func NewMsgNetworkFeeQuorum(quoNetFee *common.QuorumNetworkFee, signer cosmos.AccAddress) *MsgNetworkFeeQuorum {
	return &MsgNetworkFeeQuorum{
		QuoNetFee: quoNetFee,
		Signer:    signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgNetworkFeeQuorum) ValidateBasic() error {
	if m.QuoNetFee == nil || m.QuoNetFee.NetworkFee == nil {
		return cosmos.ErrUnknownRequest("QuoNetFee and NetworkFee cannot be nil")
	}
	nf := m.QuoNetFee.NetworkFee
	if nf.Height <= 0 {
		return cosmos.ErrUnknownRequest("block height can't be negative, or zero")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if nf.Chain.IsEmpty() {
		return cosmos.ErrUnknownRequest("chain can't be empty")
	}
	if nf.TransactionSize <= 0 {
		return cosmos.ErrUnknownRequest("invalid transaction size")
	}
	if nf.TransactionRate <= 0 {
		return cosmos.ErrUnknownRequest("invalid transaction fee rate")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgNetworkFeeQuorum) GetSigners() []cosmos.AccAddress {
	return quorumSignersCommon(m.QuoNetFee.Attestations)
}

func MsgNetworkFeeQuorumCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgNetworkFeeQuorum)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNetworkFeeQuorum: %T", m)
	}

	return quorumSignersApiCommon(msg.QuoNetFee.Attestations), nil
}
