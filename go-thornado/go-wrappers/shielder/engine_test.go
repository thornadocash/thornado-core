package shielder

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"
)

// hexRootToDecimal mirrors thornado.ComputeShielderMerkleRoot: MerkleRoot returns a
// little-endian field-hex root, which the keeper stores as a big-endian decimal.
func hexRootToDecimal(t *testing.T, rootHex string) string {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(rootHex), "0x"))
	if err != nil {
		t.Fatalf("decode root hex: %v", err)
	}
	for left, right := 0, len(raw)-1; left < right; left, right = left+1, right-1 {
		raw[left], raw[right] = raw[right], raw[left]
	}
	return new(big.Int).SetBytes(raw).String()
}

func TestMerkleAppendMatchesFullRoot(t *testing.T) {
	var leaves []string
	for i := range 5 {
		receiptJSON, err := DeriveShieldReceipt(fmt.Sprintf("append-%d", i), 100_000_000, "seed")
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Notes []struct {
				Commitment string `json:"commitment"`
			} `json:"notes"`
		}
		if err := json.Unmarshal([]byte(receiptJSON), &receipt); err != nil {
			t.Fatal(err)
		}
		leaves = append(leaves, receipt.Notes[0].Commitment)
	}

	filled := []string{}
	var incrementalRoot string
	for i, leaf := range leaves {
		reqJSON, err := json.Marshal(map[string]any{
			"filled_subtrees": filled,
			"next_index":      i,
			"leaf":            leaf,
		})
		if err != nil {
			t.Fatal(err)
		}
		respJSON, err := MerkleAppend(string(reqJSON))
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		var resp struct {
			Root           string   `json:"root"`
			FilledSubtrees []string `json:"filled_subtrees"`
		}
		if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
			t.Fatal(err)
		}
		filled = resp.FilledSubtrees

		// After each append the incremental root must equal a full recompute over the
		// same prefix, so a client rebuilding the tree from the leaf list agrees.
		prefixJSON, err := json.Marshal(leaves[:i+1])
		if err != nil {
			t.Fatal(err)
		}
		fullHex, err := MerkleRoot(string(prefixJSON))
		if err != nil {
			t.Fatal(err)
		}
		if want := hexRootToDecimal(t, fullHex); resp.Root != want {
			t.Fatalf("append %d root mismatch: got %s want %s", i, resp.Root, want)
		}
		incrementalRoot = resp.Root
	}
	if incrementalRoot == "" {
		t.Fatal("expected a root")
	}
}

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
