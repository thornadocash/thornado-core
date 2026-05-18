package types

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

// MsgReferenceMemo create new MsgReferenceMemo message
func NewMsgReferenceMemo(asset common.Asset, memo string, signer cosmos.AccAddress) *MsgReferenceMemo {
	return &MsgReferenceMemo{
		Asset:  asset,
		Memo:   memo,
		Signer: signer,
	}
}

// Route should return the router key of the module
func (m *MsgReferenceMemo) Route() string { return RouterKey }

// Type should return the action
func (m MsgReferenceMemo) Type() string { return "ref_memo" }

// ValidateBasic runs stateless checks on the message
func (m *MsgReferenceMemo) ValidateBasic() error {
	if m.Asset.IsEmpty() {
		return cosmos.ErrInvalidAddress("asset cannot be empty")
	}
	if m.Memo == "" {
		return cosmos.ErrInvalidAddress("memo cannot be empty")
	}
	if len(m.Memo) > constants.MaxMemoSize {
		return fmt.Errorf("memo cannot exceed %d: it is %d", constants.MaxMemoSize, len(m.Memo))
	}
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress("empty signer address")
	}
	return nil
}

// GetSignBytes encodes the message for signing
func (m *MsgReferenceMemo) GetSignBytes() []byte {
	return cosmos.MustSortJSON(ModuleCdc.MustMarshalJSON(m))
}

// GetSigners defines whose signature is required
func (m *MsgReferenceMemo) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}
