package shielder

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// bn254ScalarFieldModulus is the order of the bn254 (alt_bn128) scalar field
// that all groth16 public inputs live in. A client-supplied decimal field
// element at or above this value would be silently reduced mod the field when
// parsed on the Rust side (field_from_decimal), yielding a different in-circuit
// value than the caller wrote. Rejecting out-of-range values up front eliminates
// that footgun.
//
// The note pre-image material (nullifier/secret) is separately constrained to
// 248 bits by the circuit's Num2Bits(248) / 31-byte Pedersen packing, but those
// are private inputs and never appear in the redeem public inputs. The public
// inputs here (nullifier_hash, merkle_root, recipient/relayer/refund binding
// fields) are hash outputs that legitimately occupy up to ~254 bits, so the
// correct upper bound for them is the field modulus, not 2^248.
var bn254ScalarFieldModulus, _ = new(big.Int).SetString(
	"21888242871839275222246405745257275088548364400416034343698204186575808495617", 10,
)

func validateFieldElementInRange(label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, ok := new(big.Int).SetString(trimmed, 10)
	if !ok {
		return fmt.Errorf("shielder %s field is not a decimal field element", label)
	}
	if parsed.Sign() < 0 {
		return fmt.Errorf("shielder %s field must be non-negative", label)
	}
	if parsed.Cmp(bn254ScalarFieldModulus) >= 0 {
		return fmt.Errorf("shielder %s field is out of range for the scalar field", label)
	}
	return nil
}

type withdrawalPublicInputs struct {
	NullifierHash    string `json:"nullifier_hash"`
	MerkleRoot       string `json:"merkle_root"`
	DenominationSats uint64 `json:"denomination_sats"`
	Recipient        string `json:"recipient"`
	FeeSats          uint64 `json:"fee_sats"`
	RecipientField   string `json:"recipient_field,omitempty"`
	RelayerField     string `json:"relayer_field,omitempty"`
	RefundField      string `json:"refund_field,omitempty"`
	NoteCommitment   string `json:"note_commitment,omitempty"`
}

func ValidateRedeemPublicJSON(publicJSON string) error {
	var public withdrawalPublicInputs
	if err := json.Unmarshal([]byte(publicJSON), &public); err != nil {
		return fmt.Errorf("invalid shielder public inputs: %w", err)
	}
	if strings.TrimSpace(public.NullifierHash) == "" {
		return fmt.Errorf("missing shielder nullifier hash")
	}
	if strings.TrimSpace(public.MerkleRoot) == "" {
		return fmt.Errorf("missing shielder merkle root")
	}
	if public.DenominationSats == 0 {
		return fmt.Errorf("missing shielder denomination")
	}
	if public.FeeSats >= public.DenominationSats {
		return fmt.Errorf("shielder fee exceeds denomination")
	}
	if strings.TrimSpace(public.Recipient) == "" {
		return fmt.Errorf("missing shielder redeem recipient")
	}
	if strings.TrimSpace(public.NoteCommitment) != "" {
		return fmt.Errorf("shielder public inputs must not carry note commitment")
	}
	if err := validateFieldElementInRange("nullifier_hash", public.NullifierHash); err != nil {
		return err
	}
	if err := validateFieldElementInRange("merkle_root", public.MerkleRoot); err != nil {
		return err
	}
	if err := validateFieldElementInRange("recipient", public.RecipientField); err != nil {
		return err
	}
	if err := validateFieldElementInRange("relayer", public.RelayerField); err != nil {
		return err
	}
	if err := validateFieldElementInRange("refund", public.RefundField); err != nil {
		return err
	}
	if err := validateZeroBindingField("relayer", public.RelayerField); err != nil {
		return err
	}
	if err := validateZeroBindingField("refund", public.RefundField); err != nil {
		return err
	}
	expectedRecipientField, err := RecipientBindingField(
		strings.TrimSpace(public.Recipient),
		public.FeeSats,
		public.DenominationSats,
	)
	if err != nil {
		return fmt.Errorf("invalid shielder recipient binding: %w", err)
	}
	providedRecipientField := strings.TrimSpace(public.RecipientField)
	if providedRecipientField != "" && providedRecipientField != expectedRecipientField {
		return fmt.Errorf("shielder recipient field mismatch")
	}
	return ValidateWithdrawalPublic(publicJSON)
}

func validateZeroBindingField(label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "0" {
		return nil
	}
	return fmt.Errorf("shielder %s field must be zero", label)
}
