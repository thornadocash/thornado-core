package tss

import (
	"testing"

	tssMessages "gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"
	"gitlab.com/thorchain/thornode/v3/common"
	thorchaintypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestCanonicalTssRoundSecp256k1(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thorchain.tsslib.ecdsa.keygen.KGRound1Message":     tssMessages.KEYGEN1,
		"thorchain.tsslib.ecdsa.keygen.KGRound2Message1":    tssMessages.KEYGEN2aUnicast,
		"thorchain.tsslib.ecdsa.signing.SignRound1Message1": tssMessages.KEYSIGN1aUnicast,
		"thorchain.tsslib.ecdsa.signing.SignRound2Message":  tssMessages.KEYSIGN2Unicast,
		tssMessages.KEYSIGN7:                                tssMessages.KEYSIGN7,
		"thorchain.tsslib.ecdsa.signing.SignRound7Message":  tssMessages.KEYSIGN7,
		"unrelated-round":                                   "unrelated-round",
	}

	for in, want := range tests {
		if got := thorchaintypes.CanonicalTssRound(in, common.SigningAlgoSecp256k1); got != want {
			t.Errorf("CanonicalTssRound(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalTssRoundEd25519(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"thorchain.tsslib.eddsa.keygen.KGRound1Message":    tssMessages.EDDSAKEYGEN1,
		"thorchain.tsslib.eddsa.keygen.KGRound2Message2":   tssMessages.EDDSAKEYGEN2b,
		"thorchain.tsslib.eddsa.signing.SignRound1Message": tssMessages.EDDSAKEYSIGN1,
		"thorchain.tsslib.eddsa.signing.SignRound3Message": tssMessages.EDDSAKEYSIGN3,
		tssMessages.EDDSAKEYSIGN2:                          tssMessages.EDDSAKEYSIGN2,
		"unrelated-round":                                  "unrelated-round",
	}

	for in, want := range tests {
		if got := thorchaintypes.CanonicalTssRound(in, common.SigningAlgoEd25519); got != want {
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
			round: "thorchain.tsslib.ecdsa.signing.SignRound7Message",
			want:  true,
		},
		{
			name:  "canonical eddsa last round",
			round: tssMessages.EDDSAKEYSIGN3,
			want:  true,
		},
		{
			name:  "raw eddsa last round",
			round: "thorchain.tsslib.eddsa.signing.SignRound3Message",
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
			err := NewKeysignError(thorchaintypes.Blame{Round: tc.round})
			if got := err.IsRound7(); got != tc.want {
				t.Fatalf("IsRound7(%q) = %v, want %v", tc.round, got, tc.want)
			}
		})
	}
}
