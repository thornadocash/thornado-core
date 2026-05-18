package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgSecuredAssetDeposit{}
	_ sdk.HasValidateBasic = &MsgSecuredAssetDeposit{}
	_ sdk.LegacyMsg        = &MsgSecuredAssetDeposit{}

	_ sdk.Msg              = &MsgSecuredAssetWithdraw{}
	_ sdk.HasValidateBasic = &MsgSecuredAssetWithdraw{}
	_ sdk.LegacyMsg        = &MsgSecuredAssetWithdraw{}
)

// NewMsgSecuredAssetDeposit is a constructor function for MsgSecuredAssetDeposit
func NewMsgSecuredAssetDeposit(asset common.Asset, amount cosmos.Uint, acc, signer cosmos.AccAddress, tx common.Tx) *MsgSecuredAssetDeposit {
	return &MsgSecuredAssetDeposit{
		Tx:      tx,
		Asset:   asset,
		Amount:  amount,
		Address: acc,
		Signer:  signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgSecuredAssetDeposit) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset cannot be empty")
	}
	if m.Asset.IsNative() {
		return cosmos.ErrUnknownRequest("native assets cannot be deposited")
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
func (m *MsgSecuredAssetDeposit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

// NewMsgSecuredAssetWithdraw is a constructor function for MsgSecuredAssetWithdraw
func NewMsgSecuredAssetWithdraw(asset common.Asset, amount cosmos.Uint, addr common.Address, signer cosmos.AccAddress, tx common.Tx) *MsgSecuredAssetWithdraw {
	return &MsgSecuredAssetWithdraw{
		Asset:        asset,
		Amount:       amount,
		AssetAddress: addr,
		Signer:       signer,
		Tx:           tx,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgSecuredAssetWithdraw) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("asset cannot be empty")
	}
	if !m.Asset.IsSecuredAsset() {
		return cosmos.ErrUnknownRequest("asset must be a secured asset")
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
func (m *MsgSecuredAssetWithdraw) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
