package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

var (
	_ sdk.Msg              = &MsgRunePoolDeposit{}
	_ sdk.HasValidateBasic = &MsgRunePoolDeposit{}
	_ sdk.LegacyMsg        = &MsgRunePoolDeposit{}

	_ sdk.Msg              = &MsgRunePoolWithdraw{}
	_ sdk.HasValidateBasic = &MsgRunePoolWithdraw{}
	_ sdk.LegacyMsg        = &MsgRunePoolWithdraw{}
)

// NewMsgRunePoolDeposit create new MsgRunePoolDeposit message
func NewMsgRunePoolDeposit(signer cosmos.AccAddress, tx common.Tx) *MsgRunePoolDeposit {
	return &MsgRunePoolDeposit{
		Signer: signer,
		Tx:     tx,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgRunePoolDeposit) ValidateBasic() error {
	if !m.Tx.Chain.Equals(common.THORChain) {
		return cosmos.ErrUnauthorized("chain must be THORChain")
	}
	if len(m.Tx.Coins) != 1 {
		return cosmos.ErrInvalidCoins("coins must be length 1 (RUNE)")
	}
	if !m.Tx.Coins[0].Asset.Chain.IsTHORChain() {
		return cosmos.ErrInvalidCoins("coin chain must be THORChain")
	}
	if !m.Tx.Coins[0].IsRune() {
		return cosmos.ErrInvalidCoins("coin must be RUNE")
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
func (m *MsgRunePoolDeposit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

// NewMsgRunePoolWithdraw create new MsgRunePoolWithdraw message
func NewMsgRunePoolWithdraw(signer cosmos.AccAddress, tx common.Tx, basisPoints cosmos.Uint, affAddr common.Address, affBps cosmos.Uint) *MsgRunePoolWithdraw {
	return &MsgRunePoolWithdraw{
		Signer:               signer,
		Tx:                   tx,
		BasisPoints:          basisPoints,
		AffiliateAddress:     affAddr,
		AffiliateBasisPoints: affBps,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgRunePoolWithdraw) ValidateBasic() error {
	if !m.Tx.Coins.IsEmpty() {
		return cosmos.ErrInvalidCoins("coins must be empty (zero amount)")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("signer must not be empty")
	}
	if m.BasisPoints.IsZero() || m.BasisPoints.GT(cosmos.NewUint(constants.MaxBasisPts)) {
		return cosmos.ErrUnknownRequest("invalid basis points")
	}
	if m.AffiliateBasisPoints.GT(cosmos.NewUint(constants.MaxBasisPts)) {
		return cosmos.ErrUnknownRequest("invalid affiliate basis points")
	}
	if !m.AffiliateBasisPoints.IsZero() && m.AffiliateAddress.IsEmpty() {
		return cosmos.ErrInvalidAddress("affiliate basis points with no affiliate address")
	}

	return nil
}

// GetSigners defines whose signature is required
func (m *MsgRunePoolWithdraw) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
