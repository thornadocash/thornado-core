package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	cosmos "github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgObservedTxQuorum{}
	_ sdk.HasValidateBasic = &MsgObservedTxQuorum{}
	_ sdk.LegacyMsg        = &MsgObservedTxQuorum{}
)

// NewMsgObservedTxIn is a constructor function for MsgObservedTxQuorum
func NewMsgObservedTxQuorum(tx *common.QuorumTx, signer cosmos.AccAddress) *MsgObservedTxQuorum {
	return &MsgObservedTxQuorum{
		QuoTx:  tx,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgObservedTxQuorum) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.QuoTx == nil {
		return cosmos.ErrUnknownRequest("QuoTx cannot be nil")
	}

	tx := m.QuoTx.ObsTx

	if err := tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	obAddr, err := tx.ObservedPubKey.GetAddress(tx.Tx.Coins[0].Asset.GetChain())
	if err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if m.QuoTx.Inbound {
		if !tx.Tx.ToAddress.Equals(obAddr) {
			return cosmos.ErrUnknownRequest("request is not an inbound observed transaction")
		}
	} else {
		if !tx.Tx.FromAddress.Equals(obAddr) {
			return cosmos.ErrUnknownRequest("request is not an outbound observed transaction")
		}
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

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgObservedTxQuorum) GetSigners() []cosmos.AccAddress {
	return quorumSignersCommon(m.QuoTx.Attestations)
}

func MsgObservedTxQuorumCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgObservedTxQuorum)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgObservedTxQuorum: %T", m)
	}

	return quorumSignersApiCommon(msg.QuoTx.Attestations), nil
}
