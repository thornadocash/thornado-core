package shielder

import "testing"

func TestValidateRedeemPublicJSONRejectsNoteCommitmentLeak(t *testing.T) {
	err := ValidateRedeemPublicJSON(`{
		"nullifier_hash":"1",
		"merkle_root":"2",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000,
		"note_commitment":"leak"
	}`)
	if err == nil {
		t.Fatal("expected note commitment leak to fail")
	}
}

func TestValidateRedeemPublicJSONRejectsRecipientFieldMismatch(t *testing.T) {
	expected, err := RecipientBindingField("bcrt1qrecipient", 1_000, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateRedeemPublicJSON(`{
		"nullifier_hash":"1",
		"merkle_root":"2",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000,
		"recipient_field":"999"
	}`)
	if err == nil {
		t.Fatal("expected recipient field mismatch to fail")
	}
	if expected == "999" {
		t.Fatal("test setup produced accidental match")
	}
}

func TestValidateRedeemPublicJSONAcceptsComputedRecipientField(t *testing.T) {
	expected, err := RecipientBindingField("bcrt1qrecipient", 1_000, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateRedeemPublicJSON(`{
		"nullifier_hash":"1",
		"merkle_root":"2",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000,
		"recipient_field":"` + expected + `"
	}`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateRedeemPublicJSONRejectsNonZeroRelayer(t *testing.T) {
	err := ValidateRedeemPublicJSON(`{
		"nullifier_hash":"1",
		"merkle_root":"2",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000,
		"relayer_field":"1"
	}`)
	if err == nil {
		t.Fatal("expected non-zero relayer field to fail")
	}
}
