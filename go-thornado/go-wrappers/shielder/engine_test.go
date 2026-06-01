package shielder

import (
	"encoding/json"
	"testing"
)

func TestDeriveSplitReceiptThroughRustFFI(t *testing.T) {
	receiptJSON, err := DeriveSplitReceipt("dep-1", 100_000_000, "client-seed")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Notes []struct {
			DenominationSats uint64 `json:"denomination_sats"`
			Commitment       string `json:"commitment"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Notes) != 1 {
		t.Fatalf("expected one note, got %d", len(receipt.Notes))
	}
	if receipt.Notes[0].DenominationSats != 100_000_000 {
		t.Fatalf("unexpected denomination: %d", receipt.Notes[0].DenominationSats)
	}
	if receipt.Notes[0].Commitment == "" {
		t.Fatal("expected note commitment")
	}
}

func TestInvalidWithdrawalReportsRustError(t *testing.T) {
	err := VerifyWithdrawal(`{}`, `{}`)
	if err == nil {
		t.Fatal("expected invalid proof error")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestMinimumFeeDenominationSupported(t *testing.T) {
	receiptJSON, err := DeriveSplitReceipt("fee-claim", 100_000, "operator-seed")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Notes []struct {
			DenominationSats uint64 `json:"denomination_sats"`
		} `json:"notes"`
	}
	if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Notes) != 1 || receipt.Notes[0].DenominationSats != 100_000 {
		t.Fatalf("unexpected fee receipt notes: %#v", receipt.Notes)
	}
}
