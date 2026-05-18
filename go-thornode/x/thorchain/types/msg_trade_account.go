package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgTradeAccountDeposit{}
	_ sdk.HasValidateBasic = &MsgTradeAccountDeposit{}
	_ sdk.LegacyMsg        = &MsgTradeAccountDeposit{}

	_ sdk.Msg              = &MsgTradeAccountWithdrawal{}
	_ sdk.HasValidateBasic = &MsgTradeAccountWithdrawal{}
	_ sdk.LegacyMsg        = &MsgTradeAccountWithdrawal{}
)

// NewMsgTradeAccountDeposit is a constructor function for MsgTradeAccountDeposit
func NewMsgTradeAccountDeposit(asset common.Asset, amount cosmos.Uint, acc, signer cosmos.AccAddress, tx common.Tx) *MsgTradeAccountDeposit {
	return &MsgTradeAccountDeposit{
		Tx:      tx,
		Asset:   asset,
		Amount:  amount,
		Address: acc,
		Signer:  signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgTradeAccountDeposit) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset cannot be empty")
	}
	if m.Asset.GetChain().IsTHORChain() {
		return cosmos.ErrUnknownRequest("asset cannot be THORChain asset")
	}
	if m.Amount.IsZero() {
		return cosmos.ErrUnknownRequest("amount cannot be zero")
	}
	if m.Address.Empty() {
		return cosmos.ErrInvalidAddress(m.Address.String())
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Tx.ID.IsEmpty() {
		return cosmos.ErrUnknownRequest("txID cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTradeAccountDeposit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

// NewMsgTradeAccountWithdrawal is a constructor function for MsgTradeAccountWithdrawal
func NewMsgTradeAccountWithdrawal(asset common.Asset, amount cosmos.Uint, addr common.Address, signer cosmos.AccAddress, tx common.Tx) *MsgTradeAccountWithdrawal {
	return &MsgTradeAccountWithdrawal{
		Asset:        asset,
		Amount:       amount,
		AssetAddress: addr,
		Signer:       signer,
		Tx:           tx,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgTradeAccountWithdrawal) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset cannot be empty")
	}
	if !m.Asset.IsTradeAsset() {
		return cosmos.ErrUnknownRequest("asset must be a trade asset")
	}
	if m.Amount.IsZero() {
		return cosmos.ErrUnknownRequest("amount cannot be zero")
	}
	if m.AssetAddress.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset address cannot be empty")
	}
	if !m.AssetAddress.IsChain(m.Asset.GetLayer1Asset().GetChain()) {
		return cosmos.ErrUnknownRequest("asset address does not match asset chain")
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Tx.ID.IsEmpty() {
		return cosmos.ErrUnknownRequest("txID cannot be empty")
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgTradeAccountWithdrawal) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
