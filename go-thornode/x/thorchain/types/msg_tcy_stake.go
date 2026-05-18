package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgTCYStake{}
	_ sdk.HasValidateBasic = &MsgTCYStake{}
	_ sdk.LegacyMsg        = &MsgTCYStake{}
)

// NewMsgTCYStake create new MsgTCYStake message
func NewMsgTCYStake(tx common.Tx, signer sdk.AccAddress) *MsgTCYStake {
	return &MsgTCYStake{
		Tx:     tx,
		Signer: signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgTCYStake) ValidateBasic() error {
	if !m.Tx.Chain.Equals(common.THORChain) {
		return cosmos.ErrUnauthorized("chain must be THORChain")
	}
	if len(m.Tx.Coins) != 1 {
		return cosmos.ErrInvalidCoins("coins must be length 1 (TCY)")
	}
	if !m.Tx.Coins[0].IsTCY() {
		return cosmos.ErrInvalidCoins("coin must be TCY")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer must not be empty")
	}
	if m.Tx.Coins[0].Amount.IsZero() {
		return cosmos.ErrUnknownRequest("coins amount must not be zero")
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTCYStake) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
