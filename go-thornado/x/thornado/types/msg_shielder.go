package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"

	apitypes "github.com/thornadocash/go-thornado/api/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/go-wrappers/shielder"
)

var (
	_ sdk.Msg              = &MsgDepositRequestPow{}
	_ sdk.HasValidateBasic = &MsgDepositRequestPow{}
	_ sdk.LegacyMsg        = &MsgDepositRequestPow{}

	_ sdk.Msg              = &MsgShielderShield{}
	_ sdk.HasValidateBasic = &MsgShielderShield{}
	_ sdk.LegacyMsg        = &MsgShielderShield{}

	_ sdk.Msg              = &MsgShielderRedeem{}
	_ sdk.HasValidateBasic = &MsgShielderRedeem{}
	_ sdk.LegacyMsg        = &MsgShielderRedeem{}

	_ sdk.Msg              = &MsgShielderShieldFees{}
	_ sdk.HasValidateBasic = &MsgShielderShieldFees{}
	_ sdk.LegacyMsg        = &MsgShielderShieldFees{}

	_ sdk.Msg              = &MsgNodeOperatorFeeSet{}
	_ sdk.HasValidateBasic = &MsgNodeOperatorFeeSet{}
	_ sdk.LegacyMsg        = &MsgNodeOperatorFeeSet{}

	_ sdk.Msg              = &MsgNodeSlotAuctionCreate{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionCreate{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionCreate{}

	_ sdk.Msg              = &MsgNodeSlotAuctionBidCreate{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionBidCreate{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionBidCreate{}

	_ sdk.Msg              = &MsgNodeSlotAuctionSelectBid{}
	_ sdk.HasValidateBasic = &MsgNodeSlotAuctionSelectBid{}
	_ sdk.LegacyMsg        = &MsgNodeSlotAuctionSelectBid{}

	_ sdk.Msg              = &MsgNodeSaleShield{}
	_ sdk.HasValidateBasic = &MsgNodeSaleShield{}
	_ sdk.LegacyMsg        = &MsgNodeSaleShield{}

	_ sdk.Msg              = &MsgBondFromNotes{}
	_ sdk.HasValidateBasic = &MsgBondFromNotes{}
	_ sdk.LegacyMsg        = &MsgBondFromNotes{}
)

const (
	MaxPowTokenLength                 = 128
	MaxShielderCommitments            = 128
	MaxShielderCommitmentLength       = 512
	MaxShielderProofJSONLength        = 64 * 1024
	MaxShielderPublicJSONLength       = 4 * 1024
	MaxShieldSignatureHexLength       = 160
	MaxShielderOperatorSignatureBytes = 64
	MaxShielderIDLength               = 128
	MaxPowDurationMs                  = 24 * 60 * 60 * 1000
	MaxNodeOperatorFeeBasisPoints     = 10_000
)

func NewMsgDepositRequestPow(powToken, depositPubkey string) *MsgDepositRequestPow {
	return &MsgDepositRequestPow{
		PowToken:      strings.TrimSpace(powToken),
		DepositPubkey: strings.TrimSpace(depositPubkey),
	}
}

func (m *MsgDepositRequestPow) ValidateBasic() error {
	if strings.TrimSpace(m.PowToken) == "" {
		return fmt.Errorf("missing deposit pow token")
	}
	if len(strings.TrimSpace(m.PowToken)) > MaxPowTokenLength {
		return fmt.Errorf("deposit pow token too long")
	}
	if m.PowDurationMs > MaxPowDurationMs {
		return fmt.Errorf("deposit pow duration too long")
	}
	if err := validateCompressedSecpPubkey(m.DepositPubkey); err != nil {
		return fmt.Errorf("invalid deposit pubkey: %w", err)
	}
	return nil
}

func (m *MsgDepositRequestPow) GetSigners() []cosmos.AccAddress { return nil }

func MsgDepositRequestPowCustomGetSigners(m proto.Message) ([][]byte, error) {
	if _, ok := m.(*apitypes.MsgDepositRequestPow); !ok {
		return nil, fmt.Errorf("can't cast as MsgDepositRequestPow: %T", m)
	}
	return nil, nil
}

func NewMsgShielderShield(commitments []string, depositPubkey, signature, depositID string) *MsgShielderShield {
	return &MsgShielderShield{
		Commitments:   commitments,
		DepositPubkey: strings.TrimSpace(depositPubkey),
		Signature:     strings.TrimSpace(signature),
		DepositId:     strings.TrimSpace(depositID),
	}
}

func (m *MsgShielderShield) ValidateBasic() error {
	if strings.TrimSpace(m.DepositPubkey) == "" {
		return fmt.Errorf("missing deposit pubkey")
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing shielder commitments")
	}
	if err := validateShielderCommitmentList(m.Commitments, "shielder commitment"); err != nil {
		return err
	}
	if err := validateCompressedSecpPubkey(m.DepositPubkey); err != nil {
		return fmt.Errorf("invalid deposit pubkey: %w", err)
	}
	if len(strings.TrimSpace(m.Signature)) > MaxShieldSignatureHexLength {
		return fmt.Errorf("shield authorization signature too long")
	}
	if _, err := hex.DecodeString(strings.TrimSpace(m.Signature)); err != nil {
		return fmt.Errorf("invalid shield authorization signature")
	}
	if strings.TrimSpace(m.DepositId) == "" {
		return fmt.Errorf("missing deposit id")
	}
	return nil
}

func (m *MsgShielderShield) GetSigners() []cosmos.AccAddress { return nil }

func MsgShielderShieldCustomGetSigners(m proto.Message) ([][]byte, error) {
	if _, ok := m.(*apitypes.MsgShielderShield); !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderShield: %T", m)
	}
	return nil, nil
}

func NewMsgShielderRedeem(proof, public []byte) *MsgShielderRedeem {
	return &MsgShielderRedeem{
		Proof:  proof,
		Public: public,
	}
}

func (m *MsgShielderRedeem) ValidateBasic() error {
	if err := validateShielderRedeemJSON(m.Proof, m.Public); err != nil {
		return err
	}
	if !json.Valid(m.Proof) {
		return fmt.Errorf("invalid shielder proof json")
	}
	if !json.Valid(m.Public) {
		return fmt.Errorf("invalid shielder public json")
	}
	if err := shielder.ValidateRedeemPublicJSON(string(m.Public)); err != nil {
		return err
	}
	return nil
}

func (m *MsgShielderRedeem) GetSigners() []cosmos.AccAddress { return nil }

func MsgShielderRedeemCustomGetSigners(m proto.Message) ([][]byte, error) {
	if _, ok := m.(*apitypes.MsgShielderRedeem); !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderRedeem: %T", m)
	}
	return nil, nil
}

func validateCompressedSecpPubkey(value string) error {
	raw, err := hex.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("expected hex pubkey")
	}
	if len(raw) == 32 {
		return nil
	}
	if len(raw) == 33 && (raw[0] == 0x02 || raw[0] == 0x03) {
		return nil
	}
	if len(raw) == 33 {
		return fmt.Errorf("expected compressed secp256k1 prefix")
	}
	return fmt.Errorf("expected 32-byte x-only or 33-byte compressed pubkey")
}

func NewMsgShielderShieldFees(nodePubKey string, operatorSignature []byte, commitments, feeNotePubKeys []string, signer cosmos.AccAddress) *MsgShielderShieldFees {
	return &MsgShielderShieldFees{
		NodePubKey:        strings.TrimSpace(nodePubKey),
		OperatorSignature: operatorSignature,
		Commitments:       commitments,
		FeeNotePubKeys:    feeNotePubKeys,
		Signer:            signer,
	}
}

func (m *MsgShielderShieldFees) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.NodePubKey); err != nil {
		return fmt.Errorf("invalid node pubkey: %w", err)
	}
	if len(m.OperatorSignature) > MaxShielderOperatorSignatureBytes {
		return fmt.Errorf("shielder fee operator signature too long")
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing shielder fee commitments")
	}
	if len(m.FeeNotePubKeys) != len(m.Commitments) {
		return fmt.Errorf("shielder fee note pubkey count mismatch")
	}
	if err := validateShielderCommitmentList(m.Commitments, "shielder fee commitment"); err != nil {
		return err
	}
	seenPubKeys := make(map[string]struct{}, len(m.FeeNotePubKeys))
	for _, feeNotePubKey := range m.FeeNotePubKeys {
		feeNotePubKey = strings.TrimSpace(feeNotePubKey)
		if feeNotePubKey == "" {
			return fmt.Errorf("missing shielder fee note pubkey")
		}
		if _, err := hex.DecodeString(feeNotePubKey); err != nil || len(feeNotePubKey) != 66 {
			return fmt.Errorf("invalid shielder fee note pubkey")
		}
		if _, ok := seenPubKeys[feeNotePubKey]; ok {
			return fmt.Errorf("duplicate shielder fee note pubkey")
		}
		seenPubKeys[feeNotePubKey] = struct{}{}
	}
	return nil
}

func (m *MsgShielderShieldFees) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgShielderShieldFeesCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgShielderShieldFees)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgShielderShieldFees: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeOperatorFeeSet(nodePubKey string, operatorFeeBasisPoints uint64, signer cosmos.AccAddress) *MsgNodeOperatorFeeSet {
	return &MsgNodeOperatorFeeSet{
		NodePubKey:             strings.TrimSpace(nodePubKey),
		OperatorFeeBasisPoints: operatorFeeBasisPoints,
		Signer:                 signer,
	}
}

func (m *MsgNodeOperatorFeeSet) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.NodePubKey); err != nil {
		return fmt.Errorf("invalid node pubkey: %w", err)
	}
	if m.OperatorFeeBasisPoints > MaxNodeOperatorFeeBasisPoints {
		return fmt.Errorf("node operator fee basis points must be <= %d", MaxNodeOperatorFeeBasisPoints)
	}
	return nil
}

func (m *MsgNodeOperatorFeeSet) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeOperatorFeeSetCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeOperatorFeeSet)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeOperatorFeeSet: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgNodeSlotAuctionCreate(nodePubKey string, reserveSats uint64, expiryHeight int64, signer cosmos.AccAddress) *MsgNodeSlotAuctionCreate {
	return &MsgNodeSlotAuctionCreate{
		NodePubKey:   strings.TrimSpace(nodePubKey),
		ReserveSats:  reserveSats,
		ExpiryHeight: expiryHeight,
		Signer:       signer,
	}
}

func (m *MsgNodeSlotAuctionCreate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.NodePubKey); err != nil {
		return fmt.Errorf("invalid seller node pubkey: %w", err)
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

func NewMsgNodeSlotAuctionBidCreate(auctionID, operatorPubKey, nodePubKey string, signer cosmos.AccAddress) *MsgNodeSlotAuctionBidCreate {
	return &MsgNodeSlotAuctionBidCreate{
		AuctionId:      strings.TrimSpace(auctionID),
		OperatorPubKey: strings.TrimSpace(operatorPubKey),
		NodePubKey:     strings.TrimSpace(nodePubKey),
		Signer:         signer,
	}
}

func (m *MsgNodeSlotAuctionBidCreate) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.AuctionId) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if len(strings.TrimSpace(m.AuctionId)) > MaxShielderIDLength {
		return fmt.Errorf("node slot auction id too long")
	}
	if _, err := common.NewPubKey(m.OperatorPubKey); err != nil {
		return fmt.Errorf("invalid bidder operator pubkey: %w", err)
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.NodePubKey); err != nil {
		return fmt.Errorf("invalid bidder node pubkey: %w", err)
	}
	return nil
}

func (m *MsgNodeSlotAuctionBidCreate) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSlotAuctionBidCreateCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSlotAuctionBidCreate)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSlotAuctionBidCreate: %T", m)
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
	if len(strings.TrimSpace(m.AuctionId)) > MaxShielderIDLength {
		return fmt.Errorf("node slot auction id too long")
	}
	if strings.TrimSpace(m.BidId) == "" {
		return fmt.Errorf("missing node slot bid id")
	}
	if len(strings.TrimSpace(m.BidId)) > MaxShielderIDLength {
		return fmt.Errorf("node slot bid id too long")
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

func NewMsgNodeSaleShield(auctionID, bidID string, commitments []string, depositPubkey, signature string, signer cosmos.AccAddress) *MsgNodeSaleShield {
	return &MsgNodeSaleShield{
		AuctionId:     strings.TrimSpace(auctionID),
		BidId:         strings.TrimSpace(bidID),
		Commitments:   commitments,
		DepositPubkey: strings.TrimSpace(depositPubkey),
		Signature:     strings.TrimSpace(signature),
		Signer:        signer,
	}
}

func (m *MsgNodeSaleShield) ValidateBasic() error {
	if m.Signer.Empty() {
		return cosmos.ErrInvalidAddress(m.Signer.String())
	}
	if strings.TrimSpace(m.AuctionId) == "" {
		return fmt.Errorf("missing node slot auction id")
	}
	if len(strings.TrimSpace(m.AuctionId)) > MaxShielderIDLength {
		return fmt.Errorf("node slot auction id too long")
	}
	if strings.TrimSpace(m.BidId) == "" {
		return fmt.Errorf("missing node slot bid id")
	}
	if len(strings.TrimSpace(m.BidId)) > MaxShielderIDLength {
		return fmt.Errorf("node slot bid id too long")
	}
	if len(m.Commitments) == 0 {
		return fmt.Errorf("missing node slot seller commitments")
	}
	if err := validateShielderCommitmentList(m.Commitments, "node slot seller commitment"); err != nil {
		return err
	}
	if err := validateCompressedSecpPubkey(m.DepositPubkey); err != nil {
		return fmt.Errorf("invalid deposit pubkey: %w", err)
	}
	if len(strings.TrimSpace(m.Signature)) > MaxShieldSignatureHexLength {
		return fmt.Errorf("node sale shield authorization signature too long")
	}
	if _, err := hex.DecodeString(strings.TrimSpace(m.Signature)); err != nil {
		return fmt.Errorf("invalid node sale shield authorization signature")
	}
	return nil
}

func (m *MsgNodeSaleShield) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgNodeSaleShieldCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgNodeSaleShield)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgNodeSaleShield: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func NewMsgBondFromNotes(nodePubKey, operatorPubKey string, proof, public []byte, signer cosmos.AccAddress) *MsgBondFromNotes {
	return &MsgBondFromNotes{
		NodePubKey:     strings.TrimSpace(nodePubKey),
		OperatorPubKey: strings.TrimSpace(operatorPubKey),
		Proof:          proof,
		Public:         public,
		Signer:         signer,
	}
}

func (m *MsgBondFromNotes) ValidateBasic() error {
	if strings.TrimSpace(m.NodePubKey) == "" {
		return fmt.Errorf("missing node pubkey")
	}
	if _, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeConsPub, m.NodePubKey); err != nil {
		return fmt.Errorf("invalid node pubkey: %w", err)
	}
	if strings.TrimSpace(m.OperatorPubKey) == "" {
		return fmt.Errorf("missing operator pubkey")
	}
	if _, err := common.NewPubKey(m.OperatorPubKey); err != nil {
		return fmt.Errorf("invalid operator pubkey: %w", err)
	}
	if m.Signer.Empty() {
		return fmt.Errorf("missing bond signer")
	}
	if err := validateShielderRedeemJSON(m.Proof, m.Public); err != nil {
		return err
	}
	if !json.Valid(m.Proof) || !json.Valid(m.Public) {
		return fmt.Errorf("invalid bond proof or public json")
	}
	var publicInputs struct {
		RecipientPolicy string `json:"recipient_policy"`
		NodePubKey      string `json:"node_pub_key"`
		Recipient       string `json:"recipient"`
		FeeSats         uint64 `json:"fee_sats"`
	}
	if err := json.Unmarshal(m.Public, &publicInputs); err != nil {
		return fmt.Errorf("invalid bond public json: %w", err)
	}
	if normalizeBondPublicPolicy(publicInputs.RecipientPolicy) != ShielderRedeemPolicyBondEscrow {
		return fmt.Errorf("bond notes require bond_escrow recipient policy")
	}
	if strings.TrimSpace(publicInputs.NodePubKey) != strings.TrimSpace(m.NodePubKey) {
		return fmt.Errorf("bond public node pubkey mismatch")
	}
	if strings.TrimSpace(publicInputs.Recipient) != common.BondEscrowAddress.String() {
		return fmt.Errorf("bond notes require bond_escrow recipient")
	}
	if publicInputs.FeeSats != 0 {
		return fmt.Errorf("bond notes must not pay withdrawal fee")
	}
	if err := shielder.ValidateRedeemPublicJSON(string(m.Public)); err != nil {
		return err
	}
	return nil
}

func (m *MsgBondFromNotes) GetSigners() []cosmos.AccAddress {
	return []cosmos.AccAddress{m.Signer}
}

func MsgBondFromNotesCustomGetSigners(m proto.Message) ([][]byte, error) {
	msg, ok := m.(*apitypes.MsgBondFromNotes)
	if !ok {
		return nil, fmt.Errorf("can't cast as MsgBondFromNotes: %T", m)
	}
	return [][]byte{msg.Signer}, nil
}

func normalizeBondPublicPolicy(policy string) string {
	switch strings.TrimSpace(policy) {
	case "", ShielderRedeemPolicyUserBTC:
		return ShielderRedeemPolicyUserBTC
	default:
		return strings.TrimSpace(policy)
	}
}

func validateShielderCommitmentList(commitments []string, label string) error {
	if len(commitments) > MaxShielderCommitments {
		return fmt.Errorf("too many %ss: %d, max %d", label, len(commitments), MaxShielderCommitments)
	}
	seen := make(map[string]struct{}, len(commitments))
	for _, commitment := range commitments {
		commitment = strings.TrimSpace(commitment)
		if commitment == "" {
			return fmt.Errorf("empty %s", label)
		}
		if len(commitment) > MaxShielderCommitmentLength {
			return fmt.Errorf("%s too long", label)
		}
		if _, ok := seen[commitment]; ok {
			return fmt.Errorf("duplicate %s", label)
		}
		seen[commitment] = struct{}{}
	}
	return nil
}

func validateShielderRedeemJSON(proof, public []byte) error {
	if len(proof) == 0 {
		return fmt.Errorf("missing shielder proof json")
	}
	if len(public) == 0 {
		return fmt.Errorf("missing shielder public json")
	}
	if len(proof) > MaxShielderProofJSONLength {
		return fmt.Errorf("shielder proof json too large")
	}
	if len(public) > MaxShielderPublicJSONLength {
		return fmt.Errorf("shielder public json too large")
	}
	return nil
}
