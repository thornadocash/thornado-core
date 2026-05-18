package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgBond{}
	_ sdk.HasValidateBasic = &MsgBond{}
	_ sdk.LegacyMsg        = &MsgBond{}
)

// NewMsgBond create new MsgBond message
func NewMsgBond(txin common.Tx, nodeAddr cosmos.AccAddress, bond cosmos.Uint, bondAddress common.Address, provider, signer cosmos.AccAddress, operatorFee int64) *MsgBond {
	return &MsgBond{
		TxIn:                txin,
		NodeAddress:         nodeAddr,
		Bond:                bond,
		BondAddress:         bondAddress,
		BondProviderAddress: provider,
		Signer:              signer,
		OperatorFee:         operatorFee,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgBond) ValidateBasic() error {
	if m.NodeAddress.Empty() {
		return cosmos.ErrInvalidAddress("node address cannot be empty")
	}
	if m.Bond.IsZero() {
		return cosmos.ErrUnknownRequest("bond cannot be zero")
	}
	if m.BondAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("bond address cannot be empty")
	}
	if err := m.TxIn.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if len(m.TxIn.Coins) > 1 {
		return cosmos.ErrUnknownRequest("cannot bond more than one coin")
	}
	if len(m.TxIn.Coins) == 0 {
		return cosmos.ErrUnknownRequest("must bond with rune")
	}
	if !m.TxIn.Coins[0].IsRune() {
		return cosmos.ErrUnknownRequest("cannot bond non-native rune asset")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("empty signer address")
	}
	if m.OperatorFee < -1 || m.OperatorFee > 10000 {
		return cosmos.ErrUnknownRequest("operator fee must be 0-10000")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgBond) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
