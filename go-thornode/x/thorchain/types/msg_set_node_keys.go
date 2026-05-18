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
	_ sdk.Msg              = &MsgSetNodeKeys{}
	_ sdk.HasValidateBasic = &MsgSetNodeKeys{}
	_ sdk.LegacyMsg        = &MsgSetNodeKeys{}
)

// NewMsgSetNodeKeys is a constructor function for NewMsgAddNodeKeys
func NewMsgSetNodeKeys(nodePubKeySet common.PubKeySet, validatorConsPubKey string, signer cosmos.AccAddress) *MsgSetNodeKeys {
	return &MsgSetNodeKeys{
		PubKeySetSet:        nodePubKeySet,
		ValidatorConsPubKey: validatorConsPubKey,
		Signer:              signer,
	}
}

// ValidateBasic implements HasValidateBasic
// ValidateBasic is now ran in the message service router handler for messages that
// used to be routed using the external handler and only when HasValidateBasic is implemented.
// No versioning is used there.
func (m *MsgSetNodeKeys) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorConsPubKey); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if m.PubKeySetSet.IsEmpty() {
		return cosmos.ErrUnknownRequest("node pub keys cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgSetNodeKeys) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgSetNodeKeysCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*types.MsgSetNodeKeys)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgSetNodeKeys: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
