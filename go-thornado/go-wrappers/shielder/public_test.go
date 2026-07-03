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

// fieldModulusPlusOne is bn254 scalar field modulus r, which is the smallest
// out-of-range value (>= r reduces mod field on the Rust side).
const fieldModulus = "21888242871839275222246405745257275088548364400416034343698204186575808495617"

func TestValidateRedeemPublicJSONRejectsOutOfRangeNullifierHash(t *testing.T) {
	err := ValidateRedeemPublicJSON(`{
		"nullifier_hash":"` + fieldModulus + `",
		"merkle_root":"2",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000
	}`)
	if err == nil {
		t.Fatal("expected out-of-range nullifier hash to fail")
	}
}

func TestValidateRedeemPublicJSONRejectsOutOfRangeMerkleRoot(t *testing.T) {
	err := ValidateRedeemPublicJSON(`{
		"nullifier_hash":"1",
		"merkle_root":"` + fieldModulus + `",
		"denomination_sats":100000,
		"recipient":"bcrt1qrecipient",
		"fee_sats":1000
	}`)
	if err == nil {
		t.Fatal("expected out-of-range merkle root to fail")
	}
}

func TestValidateFieldElementRangeBoundary(t *testing.T) {
	// r - 1 is the largest in-range value and must be accepted.
	maxInRange := "21888242871839275222246405745257275088548364400416034343698204186575808495616"
	if err := validateFieldElementInRange("nullifier_hash", maxInRange); err != nil {
		t.Fatalf("r-1 should be in range: %v", err)
	}
	if err := validateFieldElementInRange("nullifier_hash", fieldModulus); err == nil {
		t.Fatal("r (field modulus) should be rejected")
	}
	// A legitimate ~254-bit hash output well below r must pass.
	legit254bit := "18469994758345480986819134831648759157984834799237192869752968023694011797977"
	if err := validateFieldElementInRange("recipient", legit254bit); err != nil {
		t.Fatalf("legitimate 254-bit binding field should be in range: %v", err)
	}
	if err := validateFieldElementInRange("nullifier_hash", "not-a-number"); err == nil {
		t.Fatal("non-decimal field element should be rejected")
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
