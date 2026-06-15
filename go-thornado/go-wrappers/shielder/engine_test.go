package shielder

import (
	"encoding/json"
	"testing"
)

func TestDeriveShieldReceiptThroughRustFFI(t *testing.T) {
	receiptJSON, err := DeriveShieldReceipt("dep-1", 100_000_000, "client-seed")
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
	receiptJSON, err := DeriveShieldReceipt("fee-claim", 100_000, "operator-seed")
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

func TestDeriveShieldReceiptBTCDecimalDenominations(t *testing.T) {
	tests := []struct {
		name      string
		amount    uint64
		expected  []uint64
		remainder uint64
	}{
		{
			name:      "0.11 BTC",
			amount:    11_000_000,
			expected:  []uint64{10_000_000, 1_000_000},
			remainder: 0,
		},
		{
			name:      "0.12 BTC",
			amount:    12_000_000,
			expected:  []uint64{10_000_000, 1_000_000, 1_000_000},
			remainder: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiptJSON, err := DeriveShieldReceipt("decimal-denom", tt.amount, "client-seed")
			if err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				Notes []struct {
					DenominationSats uint64 `json:"denomination_sats"`
				} `json:"notes"`
				RemainderSats uint64 `json:"remainder_sats"`
			}
			if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.RemainderSats != tt.remainder {
				t.Fatalf("unexpected remainder: got %d want %d", receipt.RemainderSats, tt.remainder)
			}
			if len(receipt.Notes) != len(tt.expected) {
				t.Fatalf("unexpected note count: got %d want %d", len(receipt.Notes), len(tt.expected))
			}
			for i, want := range tt.expected {
				if got := receipt.Notes[i].DenominationSats; got != want {
					t.Fatalf("note %d denomination: got %d want %d", i, got, want)
				}
			}
		})
	}
}
