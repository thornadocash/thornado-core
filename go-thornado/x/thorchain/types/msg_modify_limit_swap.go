package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	"github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgModifyLimitSwap{}
	_ sdk.HasValidateBasic = &MsgModifyLimitSwap{}
	_ sdk.LegacyMsg        = &MsgModifyLimitSwap{}
)

// NewMsgModifyLimitSwap is a constructor function for MsgModifyLimitSwap
func NewMsgModifyLimitSwap(from common.Address, source, target common.Coin, mod cosmos.Uint, signer cosmos.AccAddress, depositAsset common.Asset, depositAmount cosmos.Uint) *MsgModifyLimitSwap {
	return &MsgModifyLimitSwap{
		From:                 from,
		Source:               source,
		Target:               target,
		ModifiedTargetAmount: mod,
		Signer:               signer,
		DepositAsset:         depositAsset,
		DepositAmount:        depositAmount,
	}
}

// ValidateBasic runs stateless checks on the message
func (m *MsgModifyLimitSwap) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if err := m.Source.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if err := m.Target.Valid(); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if ok := m.Source.Asset.Equals(m.Target.Asset); ok {
		return cosmos.ErrUnknownRequest("source asset and target asset cannot be the same")
	}
	if ok := m.From.IsChain(m.Source.Asset.GetChain()); !ok {
		return cosmos.ErrUnknownRequest("from address and source asset do not match")
	}
	if m.Source.Asset.GetChain().IsTHORChain() {
		frm, err := m.From.AccAddress()
		if err != nil {
			return cosmos.ErrUnknownRequest(err.Error())
		}
		if !frm.Equals(m.Signer) {
			return cosmos.ErrUnknownRequest("from and signer address must match when source asset is native")
		}
	}
	return nil
}

// GetSigners defines whose signature is required
func (m *MsgModifyLimitSwap) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgModifyLimitSwapCustomGetSigners(m proto.Message) ([][]byte, error) {
	msgModifyLimitSwap, ok := m.(*types.MsgModifyLimitSwap)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgModifyLimitSwap: %T", m)
	}
	return [][]byte{msgModifyLimitSwap.Signer}, nil
}
