package types

import (
	"errors"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	apitypes "github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgOperatorRotate{}
	_ sdk.HasValidateBasic = &MsgOperatorRotate{}
	_ sdk.LegacyMsg        = &MsgOperatorRotate{}
)

// NewMsgOperatorRotate is a constructor function for MsgOperatorRotate
func NewMsgOperatorRotate(signer, operatorAddress cosmos.AccAddress, operatorPubKey string, coin common.Coin) *MsgOperatorRotate {
	return &MsgOperatorRotate{
		Signer:          signer,
		OperatorAddress: operatorAddress,
		OperatorPubKey:  operatorPubKey,
		Coin:            coin,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgOperatorRotate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrUnknownRequest("signer cannot be empty")
	}
	if m.OperatorAddress.Empty() {
		return cosmos.ErrUnknownRequest("operator address cannot be empty")
	}
	operatorPubKey, err := common.NewPubKey(m.OperatorPubKey)
	if err != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("invalid operator pubkey: %s", err))
	}
	operatorAddress, err := operatorPubKey.GetThorAddress()
	if err != nil {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("invalid operator pubkey address: %s", err))
	}
	if !operatorAddress.Equals(m.OperatorAddress) {
		return cosmos.ErrUnknownRequest("operator pubkey does not match operator address")
	}
	if !m.Coin.Asset.IsEmpty() || (!m.Coin.Amount.IsNil() && !m.Coin.Amount.IsZero()) {
		return cosmos.ErrUnknownRequest("coin amount must be zero")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgOperatorRotate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgOperatorRotateCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgOperatorRotate)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgOperatorRotate: %T", m)
	}
	if len(msg.Signer) == 0 {
		return nil, errors.New("empty signer")
	}
	return [][]byte{msg.Signer}, nil
}
