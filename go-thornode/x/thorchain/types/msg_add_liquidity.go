package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgAddLiquidity{}
	_ sdk.HasValidateBasic = &MsgAddLiquidity{}
	_ sdk.LegacyMsg        = &MsgAddLiquidity{}
)

// NewMsgAddLiquidity is a constructor function for MsgAddLiquidity
func NewMsgAddLiquidity(tx common.Tx, asset common.Asset, r, amount cosmos.Uint, runeAddr, assetAddr, affAddr common.Address, affPts cosmos.Uint, signer cosmos.AccAddress) *MsgAddLiquidity {
	return &MsgAddLiquidity{
		Tx:                   tx,
		Asset:                asset,
		AssetAmount:          amount,
		RuneAmount:           r,
		RuneAddress:          runeAddr,
		AssetAddress:         assetAddr,
		AffiliateAddress:     affAddr,
		AffiliateBasisPoints: affPts,
		Signer:               signer,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgAddLiquidity) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if m.Asset.IsEmpty() {
		return cosmos.ErrUnknownRequest("add liquidity asset cannot be empty")
	}
	if err := m.Tx.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	// There is no dedicate pool for RUNE, because every pool will have RUNE, that's by design
	if m.Asset.IsRune() {
		return cosmos.ErrUnknownRequest("asset cannot be rune")
	}
	// test scenario we get two coins, but none are rune, invalid liquidity provider
	if len(m.Tx.Coins) == 2 && (m.AssetAmount.IsZero() || m.RuneAmount.IsZero()) {
		return cosmos.ErrUnknownRequest("did not find both coins")
	}
	if len(m.Tx.Coins) > 2 {
		return cosmos.ErrUnknownRequest("not expecting more than two coins in adding liquidity")
	}
	if m.RuneAddress.IsEmpty() && m.AssetAddress.IsEmpty() {
		return cosmos.ErrUnknownRequest("rune address and asset address cannot be empty")
	}
	if m.AffiliateAddress.IsEmpty() && !m.AffiliateBasisPoints.IsZero() {
		return cosmos.ErrUnknownRequest("affiliate address is empty while affiliate basis points is non-zero")
	}
	if !m.AffiliateBasisPoints.IsZero() && m.AffiliateBasisPoints.GT(cosmos.NewUint(MaxAffiliateFeeBasisPoints)) {
		return cosmos.ErrUnknownRequest(fmt.Sprintf("affiliate fee basis points can't be more than %d", MaxAffiliateFeeBasisPoints))
	}
	if m.Asset.IsSecuredAsset() {
		if m.AssetAddress.IsEmpty() {
			return cosmos.ErrUnknownRequest("asset address cannot be empty")
		}
		if m.RuneAddress.IsEmpty() {
			return cosmos.ErrUnknownRequest("rune address cannot be empty")
		}
		if !m.AssetAddress.IsChain(common.THORChain) {
			return cosmos.ErrUnknownRequest("asset address must be thor address")
		}
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgAddLiquidity) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
