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

	_ sdk.Msg              = &MsgShielderSettleDeposit{}
	_ sdk.HasValidateBasic = &MsgShielderSettleDeposit{}
	_ sdk.LegacyMsg        = &MsgShielderSettleDeposit{}

	_ sdk.Msg              = &MsgShielderRequestWithdrawal{}
	_ sdk.HasValidateBasic = &MsgShielderRequestWithdrawal{}
	_ sdk.LegacyMsg        = &MsgShielderRequestWithdrawal{}

	_ sdk.Msg              = &MsgShielderSettleFees{}
	_ sdk.HasValidateBasic = &MsgShielderSettleFees{}
	_ sdk.LegacyMsg        = &MsgShielderSettleFees{}

	_ sdk.Msg              = &MsgShielderSplitFees{}
	_ sdk.HasValidateBasic = &MsgShielderSplitFees{}
	_ sdk.LegacyMsg        = &MsgShielderSplitFees{}

	_ sdk.Msg              = &MsgNodeSlotAuctionCreate{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionCreate{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionCreate{}

	_ sdk.Msg              = &MsgNodeSlotAuctionBidPow{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionBidPow{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionBidPow{}

	_ sdk.Msg              = &MsgNodeSlotAuctionSelectBid{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionSelectBid{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionSelectBid{}

	_ sdk.Msg              = &MsgNodeSlotAuctionSplit{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionSplit{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionSplit{}
)

func NewMsgShielderRegisterPow(powToken string, signer cosmos.AccAddress, extra ...string) *MsgShielderRegisterPow {
	operatorPubKey := ""
	validatorPubKey := ""
	if len(extra) > 0 {
		operatorPubKey = strings.TrimSpace(extra[0])
	}
	if len(extra) > 1 {
		validatorPubKey = strings.TrimSpace(extra[1])
	}
	return &MsgShielderRegisterPow{
		PowToken:        strings.TrimSpace(powToken),
		Signer:          signer,
		OperatorPubKey:  operatorPubKey,
		ValidatorPubKey: validatorPubKey,
	}
}

func (m *MsgShielderRegisterPow) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing shielder pow token")
	}
	if strings.TrimSpace(m.ValidatorPubKey) != "" {
		if strings.TrimSpace(m.OperatorPubKey) == "" {
			return fmt.Errorf("bond deposits require operator pubkey")
		}
		if _, err := common.NewPubKey(m.OperatorPubKey); err != nil {
			return fmt.Errorf("invalid shielder operator pubkey: %w", err)
		}
		if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorPubKey); err != nil {
			return fmt.Errorf("invalid shielder validator pubkey: %w", err)
		}
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

func NewMsgShielderSettleDeposit(depositID common.TxID, signer cosmos.AccAddress) *MsgShielderSettleDeposit {
	return &MsgShielderSettleDeposit{
		DepositId: depositID.String(),
		Signer:    signer,
	}
}

func (m *MsgShielderSettleDeposit) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := common.NewTxID(m.DepositId); err != nil {
		return fmt.Errorf("invalid shielder deposit id: %w", err)
	}
	return nil
}

func (m *MsgShielderSettleDeposit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderSettleDepositCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderSettleDeposit)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderSettleDeposit: %T", m)
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

func NewMsgShielderSettleFees(validatorPubKey string, operatorSignature []byte, signer cosmos.AccAddress) *MsgShielderSettleFees {
	return &MsgShielderSettleFees{
		ValidatorPubKey:   strings.TrimSpace(validatorPubKey),
		OperatorSignature: operatorSignature,
		Signer:            signer,
	}
}

func (m *MsgShielderSettleFees) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorPubKey); err != nil {
		return fmt.Errorf("invalid shielder validator pubkey: %w", err)
	}
	if len(m.OperatorSignature) == 0 {
		return fmt.Errorf("missing shielder fee operator signature")
	}
	return nil
}

func (m *MsgShielderSettleFees) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderSettleFeesCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderSettleFees)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderSettleFees: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgShielderSplitFees(validatorPubKey string, operatorSignature []byte, commitments, feeNotePubKeys []string, signer cosmos.AccAddress) *MsgShielderSplitFees {
	return &MsgShielderSplitFees{
		ValidatorPubKey:   strings.TrimSpace(validatorPubKey),
		OperatorSignature: operatorSignature,
		Commitments:       commitments,
		FeeNotePubKeys:    feeNotePubKeys,
		Signer:            signer,
	}
}

func (m *MsgShielderSplitFees) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorPubKey); err != nil {
		return fmt.Errorf("invalid shielder validator pubkey: %w", err)
	}
	if len(m.OperatorSignature) == 0 {
		return fmt.Errorf("missing shielder fee operator signature")
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing shielder fee commitments")
	}
	if len(m.FeeNotePubKeys) != len(m.Commitments) {
		return fmt.Errorf("shielder fee note pubkey count mismatch")
	}
	seen := make(map[string]struct{}, len(m.Commitments))
	for _, commitment := range m.Commitments {
		commitment = strings.TrimSpace(commitment)
		if commitment == "" {
			return fmt.Errorf("empty shielder fee commitment")
		}
		if _, ok := seen[commitment]; ok {
			return fmt.Errorf("duplicate shielder fee commitment")
		}
		seen[commitment] = struct{}{}
	}
	seenPubKeys := make(map[string]struct{}, len(m.FeeNotePubKeys))
	for _, feeNotePubKey := range m.FeeNotePubKeys {
		feeNotePubKey = strings.TrimSpace(feeNotePubKey)
		pubKey, err := common.NewPubKey(feeNotePubKey)
		if err != nil {
			return fmt.Errorf("invalid shielder fee note pubkey: %w", err)
		}
		if pubKey.IsEmpty() {
			return fmt.Errorf("missing shielder fee note pubkey")
		}
		if _, ok := seenPubKeys[pubKey.String()]; ok {
			return fmt.Errorf("duplicate shielder fee note pubkey")
		}
		seenPubKeys[pubKey.String()] = struct{}{}
	}
	return nil
}

func (m *MsgShielderSplitFees) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderSplitFeesCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderSplitFees)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderSplitFees: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeSlotAuctionCreate(validatorPubKey string, reserveSats uint64, expiryHeight int64, signer cosmos.AccAddress) *MsgNodeSlotAuctionCreate {
	return &MsgNodeSlotAuctionCreate{
		ValidatorPubKey: strings.TrimSpace(validatorPubKey),
		ReserveSats:     reserveSats,
		ExpiryHeight:    expiryHeight,
		Signer:          signer,
	}
}

func (m *MsgNodeSlotAuctionCreate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorPubKey); err != nil {
		return fmt.Errorf("invalid seller validator pubkey: %w", err)
	}
	if m.ExpiryHeight <= 0 {
		return fmt.Errorf("missing node slot auction expiry")
	}
	return nil
}

func (m *MsgNodeSlotAuctionCreate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSlotAuctionCreateCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSlotAuctionCreate)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSlotAuctionCreate: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeSlotAuctionBidPow(auctionID, powToken, operatorPubKey, validatorPubKey string, signer cosmos.AccAddress) *MsgNodeSlotAuctionBidPow {
	return &MsgNodeSlotAuctionBidPow{
		AuctionId:       strings.TrimSpace(auctionID),
		PowToken:        strings.TrimSpace(powToken),
		OperatorPubKey:  strings.TrimSpace(operatorPubKey),
		ValidatorPubKey: strings.TrimSpace(validatorPubKey),
		Signer:          signer,
	}
}

func (m *MsgNodeSlotAuctionBidPow) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.AuctionId) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing shielder pow token")
	}
	if _, err := common.NewPubKey(m.OperatorPubKey); err != nil {
		return fmt.Errorf("invalid bidder operator pubkey: %w", err)
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.ValidatorPubKey); err != nil {
		return fmt.Errorf("invalid bidder validator pubkey: %w", err)
	}
	return nil
}

func (m *MsgNodeSlotAuctionBidPow) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSlotAuctionBidPowCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSlotAuctionBidPow)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSlotAuctionBidPow: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeSlotAuctionSelectBid(auctionID, bidID string, signer cosmos.AccAddress) *MsgNodeSlotAuctionSelectBid {
	return &MsgNodeSlotAuctionSelectBid{
		AuctionId: strings.TrimSpace(auctionID),
		BidId:     strings.TrimSpace(bidID),
		Signer:    signer,
	}
}

func (m *MsgNodeSlotAuctionSelectBid) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.AuctionId) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if strings.TrimSpace(m.BidId) == "" {
		return fmt.Errorf("missing node slot bid id")
	}
	return nil
}

func (m *MsgNodeSlotAuctionSelectBid) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSlotAuctionSelectBidCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSlotAuctionSelectBid)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSlotAuctionSelectBid: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeSlotAuctionSplit(auctionID, bidID string, commitments []string, signer cosmos.AccAddress) *MsgNodeSlotAuctionSplit {
	return &MsgNodeSlotAuctionSplit{
		AuctionId:   strings.TrimSpace(auctionID),
		BidId:       strings.TrimSpace(bidID),
		Commitments: commitments,
		Signer:      signer,
	}
}

func (m *MsgNodeSlotAuctionSplit) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.AuctionId) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if strings.TrimSpace(m.BidId) == "" {
		return fmt.Errorf("missing node slot bid id")
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing node slot seller commitments")
	}
	return nil
}

func (m *MsgNodeSlotAuctionSplit) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSlotAuctionSplitCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSlotAuctionSplit)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSlotAuctionSplit: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}
