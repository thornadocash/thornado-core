package frost

import (
	"fmt"
	"testing"

	frostMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/common"
	thornadotypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestCanonicalFrostRoundSecp256k1(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thornado.frostlib.ecdsa.keygen.KGRound1Message":     frostMessages.KEYGEN1,
		"thornado.frostlib.ecdsa.keygen.KGRound2Message1":    frostMessages.KEYGEN2aUnicast,
		"thornado.frostlib.ecdsa.signing.SignRound1Message1": frostMessages.KEYSIGN1aUnicast,
		"thornado.frostlib.ecdsa.signing.SignRound2Message":  frostMessages.KEYSIGN2Unicast,
		frostMessages.KEYSIGN7:                               frostMessages.KEYSIGN7,
		"thornado.frostlib.ecdsa.signing.SignRound7Message":  frostMessages.KEYSIGN7,
		"unrelated-round": "unrelated-round",
	}

	for in, want := range tests {
		if got := thornadotypes.CanonicalFrostRound(in, common.SigningAlgoSecp256k1); got != want {
			t.Errorf("CanonicalFrostRound(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFrostKindRound(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"sign_round1":                  1,
		"sign_round2":                  2,
		frostMessages.KEYSIGN1aUnicast: 1,
		frostMessages.KEYSIGN2Unicast:  2,
		frostMessages.KEYSIGN7:         7,
		"thornado.frostlib.ecdsa.signing.SignRound2Message": 2,
		"thornado.frostlib.eddsa.signing.SignRound3Message": 3,
		"thornado.frostlib.ecdsa.keygen.KGRound2Message1":   2,
		"thornado.frostlib.ecdsa.keygen.KGRound2Message2":   2,
		"FrostKeysignAbort": 0,
		"":                  0,
		"unrelated":         0,
	}
	for kind, want := range tests {
		if got := frostKindRound(kind); got != want {
			t.Fatalf("frostKindRound(%q) = %d, want %d", kind, got, want)
		}
	}
}

func TestOutOfOrderFrostInputIsRetryable(t *testing.T) {
	t.Parallel()

	if !isRetryableFrostInputError(fmt.Errorf("out-of-order frost message sign_round2 before local round 2 broadcast")) {
		t.Fatal("expected out-of-order FROST input to be retryable")
	}
}

func TestCanonicalFrostRoundEd25519(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thornado.frostlib.eddsa.keygen.KGRound1Message":    frostMessages.EDDSAKEYGEN1,
		"thornado.frostlib.eddsa.keygen.KGRound2Message2":   frostMessages.EDDSAKEYGEN2b,
		"thornado.frostlib.eddsa.signing.SignRound1Message": frostMessages.EDDSAKEYSIGN1,
		"thornado.frostlib.eddsa.signing.SignRound3Message": frostMessages.EDDSAKEYSIGN3,
		frostMessages.EDDSAKEYSIGN2:                         frostMessages.EDDSAKEYSIGN2,
		"unrelated-round":                                   "unrelated-round",
	}

	for in, want := range tests {
		if got := thornadotypes.CanonicalFrostRound(in, common.SigningAlgoEd25519); got != want {
			t.Errorf("CanonicalFrostRound(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestKeysignErrorIsRound7 verifies the signer retry path recognizes both the
// canonical and raw final-round names for secp256k1 and ed25519 keysigns.
func TestKeysignErrorIsRound7(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		round string
		want  bool
	}{
		{
			name:  "canonical secp round 7",
			round: frostMessages.KEYSIGN7,
			want:  true,
		},
		{
			name:  "raw secp round 7",
			round: "thornado.frostlib.ecdsa.signing.SignRound7Message",
			want:  true,
		},
		{
			name:  "canonical eddsa last round",
			round: frostMessages.EDDSAKEYSIGN3,
			want:  true,
		},
		{
			name:  "raw eddsa last round",
			round: "thornado.frostlib.eddsa.signing.SignRound3Message",
			want:  true,
		},
		{
			name:  "non-terminal round",
			round: frostMessages.KEYSIGN5,
			want:  false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := NewKeysignError(thornadotypes.Blame{Round: tc.round})
			if got := err.IsRound7(); got != tc.want {
				t.Fatalf("IsRound7(%q) = %v, want %v", tc.round, got, tc.want)
			}
		})
	}
}
