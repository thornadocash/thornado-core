package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgPriceFeedQuorum{}
	_ sdk.HasValidateBasic = &MsgPriceFeedQuorum{}
	_ sdk.LegacyMsg        = &MsgPriceFeedQuorum{}
)

// NewMsgPriceFeedQuorum creates a new instance of MsgPriceFeed
func NewMsgPriceFeedQuorum(quoPriceFeed *common.QuorumPriceFeed, signer cosmos.AccAddress) *MsgPriceFeedQuorum {
	return &MsgPriceFeedQuorum{
		QuoPriceFeed: quoPriceFeed,
		Signer:       signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgPriceFeedQuorum) ValidateBasic() error {
	pf := m.QuoPriceFeed.PriceFeed
	if pf.Time <= 0 {
		return cosmos.ErrUnknownRequest("block height is negative or zero")
	}
	if len(pf.Rates) == 0 {
		return cosmos.ErrUnknownRequest("rates is empty")
	}

	attestations := len(m.QuoPriceFeed.Attestations)
	if attestations == 0 {
		return cosmos.ErrUnknownRequest("no attestations found")
	}
	if attestations > 1 {
		return cosmos.ErrUnknownRequest("more than one attestations found")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgPriceFeedQuorum) GetSigners() []cosmos.AccAddress {
	return quorumSignersCommon(m.QuoPriceFeed.Attestations)
}

func MsgPriceFeedQuorumCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgPriceFeedQuorum)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgPriceFeedQuorum: %T", m)
	}

	return quorumSignersApiCommon(msg.QuoPriceFeed.Attestations), nil
}
