package shielder

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
