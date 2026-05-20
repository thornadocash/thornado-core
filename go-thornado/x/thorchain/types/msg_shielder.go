package types

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	apitypes "github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

var (
	_ sdk.Msg              = &MsgShielderRegisterPow{}
	_ sdk.HasValidateBasic = &MsgShielderRegisterPow{}
	_ sdk.LegacyMsg        = &MsgShielderRegisterPow{}

	_ sdk.Msg              = &MsgShielderPostCommitments{}
	_ sdk.HasValidateBasic = &MsgShielderPostCommitments{}
	_ sdk.LegacyMsg        = &MsgShielderPostCommitments{}

	_ sdk.Msg              = &MsgShielderRequestWithdrawal{}
	_ sdk.HasValidateBasic = &MsgShielderRequestWithdrawal{}
	_ sdk.LegacyMsg        = &MsgShielderRequestWithdrawal{}
)

func NewMsgShielderRegisterPow(powToken string, signer cosmos.AccAddress) *MsgShielderRegisterPow {
	return &MsgShielderRegisterPow{
		PowToken: strings.TrimSpace(powToken),
		Signer:   signer,
	}
}

func (m *MsgShielderRegisterPow) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing shielder pow token")
	}
	return nil
}

func (m *MsgShielderRegisterPow) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderRegisterPowCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderRegisterPow)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderRegisterPow: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgShielderPostCommitments(depositID common.TxID, commitments []string, signer cosmos.AccAddress) *MsgShielderPostCommitments {
	return &MsgShielderPostCommitments{
		DepositId:   depositID.String(),
		Commitments: commitments,
		Signer:      signer,
	}
}

func (m *MsgShielderPostCommitments) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := common.NewTxID(m.DepositId); err != nil {
		return fmt.Errorf("invalid shielder deposit id: %w", err)
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing shielder commitments")
	}
	seen := make(map[string]struct{}, len(m.Commitments))
	for _, commitment := range m.Commitments {
		commitment = strings.TrimSpace(commitment)
		if commitment == "" {
			return fmt.Errorf("empty shielder commitment")
		}
		if _, ok := seen[commitment]; ok {
			return fmt.Errorf("duplicate shielder commitment")
		}
		seen[commitment] = struct{}{}
	}
	return nil
}

func (m *MsgShielderPostCommitments) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderPostCommitmentsCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderPostCommitments)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderPostCommitments: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgShielderRequestWithdrawal(proof, public []byte, signer cosmos.AccAddress) *MsgShielderRequestWithdrawal {
	return &MsgShielderRequestWithdrawal{
		Proof:  proof,
		Public: public,
		Signer: signer,
	}
}

func (m *MsgShielderRequestWithdrawal) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if !json.Valid(m.Proof) {
		return fmt.Errorf("invalid shielder proof json")
	}
	if !json.Valid(m.Public) {
		return fmt.Errorf("invalid shielder public json")
	}
	return nil
}

func (m *MsgShielderRequestWithdrawal) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderRequestWithdrawalCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderRequestWithdrawal)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderRequestWithdrawal: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
