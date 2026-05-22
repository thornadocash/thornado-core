package tss

import (
	"testing"

	tssMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/common"
	thornadotypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestCanonicalTssRoundSecp256k1(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thornado.tsslib.ecdsa.keygen.KGRound1Message":     tssMessages.KEYGEN1,
		"thornado.tsslib.ecdsa.keygen.KGRound2Message1":    tssMessages.KEYGEN2aUnicast,
		"thornado.tsslib.ecdsa.signing.SignRound1Message1": tssMessages.KEYSIGN1aUnicast,
		"thornado.tsslib.ecdsa.signing.SignRound2Message":  tssMessages.KEYSIGN2Unicast,
		tssMessages.KEYSIGN7:                                tssMessages.KEYSIGN7,
		"thornado.tsslib.ecdsa.signing.SignRound7Message":  tssMessages.KEYSIGN7,
		"unrelated-round":                                   "unrelated-round",
	}

	for in, want := range tests {
		if got := thornadotypes.CanonicalTssRound(in, common.SigningAlgoSecp256k1); got != want {
			t.Errorf("CanonicalTssRound(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalTssRoundEd25519(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thornado.tsslib.eddsa.keygen.KGRound1Message":    tssMessages.EDDSAKEYGEN1,
		"thornado.tsslib.eddsa.keygen.KGRound2Message2":   tssMessages.EDDSAKEYGEN2b,
		"thornado.tsslib.eddsa.signing.SignRound1Message": tssMessages.EDDSAKEYSIGN1,
		"thornado.tsslib.eddsa.signing.SignRound3Message": tssMessages.EDDSAKEYSIGN3,
		tssMessages.EDDSAKEYSIGN2:                          tssMessages.EDDSAKEYSIGN2,
		"unrelated-round":                                  "unrelated-round",
	}

	for in, want := range tests {
		if got := thornadotypes.CanonicalTssRound(in, common.SigningAlgoEd25519); got != want {
			t.Errorf("CanonicalTssRound(%q) = %q, want %q", in, got, want)
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
			round: tssMessages.KEYSIGN7,
			want:  true,
		},
		{
			name:  "raw secp round 7",
			round: "thornado.tsslib.ecdsa.signing.SignRound7Message",
			want:  true,
		},
		{
			name:  "canonical eddsa last round",
			round: tssMessages.EDDSAKEYSIGN3,
			want:  true,
		},
		{
			name:  "raw eddsa last round",
			round: "thornado.tsslib.eddsa.signing.SignRound3Message",
			want:  true,
		},
		{
			name:  "non-terminal round",
			round: tssMessages.KEYSIGN5,
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
