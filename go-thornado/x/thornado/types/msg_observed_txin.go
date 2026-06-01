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
	_ sdk.Msg              = &MsgObservedTxIn{}
	_ sdk.HasValidateBasic = &MsgObservedTxIn{}
	_ sdk.LegacyMsg        = &MsgObservedTxIn{}
)

// NewMsgObservedTxIn is a constructor function for MsgObservedTxIn
func NewMsgObservedTxIn(txs common.ObservedTxs, signer cosmos.AccAddress) *MsgObservedTxIn {
	return &MsgObservedTxIn{
		Txs:    txs,
		Signer: signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgObservedTxIn) ValidateBasic() error {
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
		if !observedInboundAddressMatches(tx.ObservedPubKey, tx.Tx.Coins[0].Asset.GetChain(), tx.Tx.ToAddress, obAddr) {
			return cosmos.ErrUnknownRequest("request is not an inbound observed transaction")
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

func observedInboundAddressMatches(pubKey common.PubKey, chain common.Chain, txAddress, rootAddress common.Address) bool {
	return observedVaultAddressMatches(pubKey, chain, txAddress, rootAddress)
}

func observedVaultAddressMatches(pubKey common.PubKey, chain common.Chain, txAddress, rootAddress common.Address) bool {
	if txAddress.Equals(rootAddress) {
		return true
	}
	if !chain.Equals(common.BTCChain) {
		return false
	}
	for pathIndex := common.FirstDepositPathIndex; pathIndex <= common.DepositAddressLookahead; pathIndex++ {
		childAddress, err := common.DeriveBTCTaprootAddress(pubKey, pathIndex)
		if err != nil {
			return false
		}
		if txAddress.Equals(childAddress) {
			return true
		}
	}
	return false
}

// GetSigners defines whose signature is required
func (m *MsgObservedTxIn) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgObservedTxInCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgObservedTxIn)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgObservedTxIn: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
