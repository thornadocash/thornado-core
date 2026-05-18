package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"google.golang.org/protobuf/proto"

	"gitlab.com/thorchain/thornode/v3/api/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgObservedTxOut{}
	_ sdk.HasValidateBasic = &MsgObservedTxOut{}
	_ sdk.LegacyMsg        = &MsgObservedTxOut{}
)

// NewMsgObservedTxOut is a constructor function for MsgObservedTxOut
func NewMsgObservedTxOut(txs common.ObservedTxs, signer cosmos.AccAddress) *MsgObservedTxOut {
	return &MsgObservedTxOut{
		Txs:    txs,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgObservedTxOut) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if len(m.Txs) == 0 {
		return cosmos.ErrUnknownRequest("Txs cannot be empty")
	}
	for _, tx := range m.Txs {
		if err := tx.Valid(); err != nil {
			return cosmos.ErrUnknownRequest(err.Error())
		}
		obAddr, err := tx.ObservedPubKey.GetAddress(tx.Tx.Coins[0].Asset.GetChain())
		if err != nil {
			return cosmos.ErrUnknownRequest(err.Error())
		}
		if !tx.Tx.FromAddress.Equals(obAddr) {
			return cosmos.ErrUnknownRequest("Request is not an outbound observed transaction")
		}
		if len(tx.Signers) > 0 {
			return cosmos.ErrUnknownRequest("signers must be empty")
		}
		if len(tx.OutHashes) > 0 {
			return cosmos.ErrUnknownRequest("out hashes must be empty")
		}
		if tx.Status != common.Status_incomplete {
			return cosmos.ErrUnknownRequest("status must be incomplete")
		}
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgObservedTxOut) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgObservedTxOutCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgObservedTxOut)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgObservedTxOut: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
