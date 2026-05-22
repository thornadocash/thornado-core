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
	_ sdk.Msg              = &MsgPriceFeedQuorumBatch{}
	_ sdk.HasValidateBasic = &MsgPriceFeedQuorumBatch{}
	_ sdk.LegacyMsg        = &MsgPriceFeedQuorumBatch{}
)

// NewMsgPriceFeedQuorumBatch creates a new instance of MsgPriceFeed
func NewMsgPriceFeedQuorumBatch(quoPriceFeeds []*common.QuorumPriceFeed, signer cosmos.AccAddress) *MsgPriceFeedQuorumBatch {
	return &MsgPriceFeedQuorumBatch{
		QuoPriceFeeds: quoPriceFeeds,
		Signer:        signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgPriceFeedQuorumBatch) ValidateBasic() error {
	if len(m.QuoPriceFeeds) == 0 {
		return cosmos.ErrUnknownRequest("no price feeds provided")
	}
	for i := range m.QuoPriceFeeds {
		if m.QuoPriceFeeds[i] == nil || m.QuoPriceFeeds[i].PriceFeed == nil {
			return cosmos.ErrUnknownRequest("nil price feed")
		}
		pf := m.QuoPriceFeeds[i].PriceFeed
		if pf.Time <= 0 {
			return cosmos.ErrUnknownRequest("block height is negative or zero")
		}
		if len(pf.Rates) == 0 {
			return cosmos.ErrUnknownRequest("rates is empty")
		}

		attestations := len(m.QuoPriceFeeds[i].Attestations)
		if attestations == 0 {
			return cosmos.ErrUnknownRequest("no attestations found")
		}
		if attestations > 1 {
			return cosmos.ErrUnknownRequest("more than one attestations found")
		}
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgPriceFeedQuorumBatch) GetSigners() []cosmos.AccAddress {
	var addresses []cosmos.AccAddress
	for _, pf := range m.QuoPriceFeeds {
		addresses = append(addresses, quorumSignersCommon(pf.Attestations)...)
	}
	return addresses
}

func MsgPriceFeedQuorumBatchCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgPriceFeedQuorumBatch)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgPriceFeedQuorumBatch: %T", m)
	}

	var signers [][]byte
	for _, pf := range msg.QuoPriceFeeds {
		signers = append(signers, quorumSignersApiCommon(pf.Attestations)...)
	}

	return signers, nil
}
