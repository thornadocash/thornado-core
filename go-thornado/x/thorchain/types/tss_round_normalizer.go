package types

import (
	"fmt"
	"strings"

	tssMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/common"
)

// CanonicalTssRound normalizes raw TSS round names into the canonical constants
// used in Thornode consensus and telemetry. When algo is empty, it is inferred
// from the raw round name when possible.
func CanonicalTssRound(round string, algo common.SigningAlgo) string {
	if round == "" {
		return ""
	}

	switch {
	case strings.Contains(round, ".eddsa."):
		if algo == "" {
			algo = common.SigningAlgoEd25519
		}
	case strings.Contains(round, ".ecdsa."):
		if algo == "" {
			algo = common.SigningAlgoSecp256k1
		}
	}

	switch algo {
	case common.SigningAlgoSecp256k1:
		return canonicalSecp256k1TssRound(round)
	case common.SigningAlgoEd25519:
		return canonicalEd25519TssRound(round)
	default:
		return round
	}
}

// NormalizeTssKeysignRound canonicalizes a keysign round and verifies that the
// result is one of the supported keysign-round identifiers.
func NormalizeTssKeysignRound(round string) (string, error) {
	canonical := CanonicalTssRound(round, "")
	if canonical == "" {
		return "", nil
	}
	if !tssMessages.ValidTssKeysignRounds[canonical] {
		return "", fmt.Errorf("invalid blame round: %s", round)
	}
	return canonical, nil
}

// canonicalSecp256k1TssRound maps secp256k1 TSS library phase names into the
// stable round identifiers used in messages and state.
func canonicalSecp256k1TssRound(round string) string {
	switch {
	case round == tssMessages.KEYGEN1 || strings.HasSuffix(round, "KGRound1Message"):
		return tssMessages.KEYGEN1
	case round == tssMessages.KEYGEN2aUnicast || strings.HasSuffix(round, "KGRound2Message1"):
		return tssMessages.KEYGEN2aUnicast
	case round == tssMessages.KEYGEN2b || strings.HasSuffix(round, "KGRound2Message2"):
		return tssMessages.KEYGEN2b
	case round == tssMessages.KEYGEN3 || strings.HasSuffix(round, "KGRound3Message"):
		return tssMessages.KEYGEN3
	case round == tssMessages.KEYSIGN1aUnicast || strings.HasSuffix(round, "SignRound1Message1"):
		return tssMessages.KEYSIGN1aUnicast
	case round == tssMessages.KEYSIGN1b || strings.HasSuffix(round, "SignRound1Message2"):
		return tssMessages.KEYSIGN1b
	case round == tssMessages.KEYSIGN2Unicast || strings.HasSuffix(round, "SignRound2Message"):
		return tssMessages.KEYSIGN2Unicast
	case round == tssMessages.KEYSIGN3 || strings.HasSuffix(round, "SignRound3Message"):
		return tssMessages.KEYSIGN3
	case round == tssMessages.KEYSIGN4 || strings.HasSuffix(round, "SignRound4Message"):
		return tssMessages.KEYSIGN4
	case round == tssMessages.KEYSIGN5 || strings.HasSuffix(round, "SignRound5Message"):
		return tssMessages.KEYSIGN5
	case round == tssMessages.KEYSIGN6 || strings.HasSuffix(round, "SignRound6Message"):
		return tssMessages.KEYSIGN6
	case round == tssMessages.KEYSIGN7 || strings.HasSuffix(round, "SignRound7Message"):
		return tssMessages.KEYSIGN7
	default:
		return round
	}
}

// canonicalEd25519TssRound maps ed25519 TSS library phase names into the
// stable round identifiers used in messages and state.
func canonicalEd25519TssRound(round string) string {
	switch {
	case round == tssMessages.EDDSAKEYGEN1 || strings.HasSuffix(round, "KGRound1Message"):
		return tssMessages.EDDSAKEYGEN1
	case round == tssMessages.EDDSAKEYGEN2a || strings.HasSuffix(round, "KGRound2Message1"):
		return tssMessages.EDDSAKEYGEN2a
	case round == tssMessages.EDDSAKEYGEN2b || strings.HasSuffix(round, "KGRound2Message2"):
		return tssMessages.EDDSAKEYGEN2b
	case round == tssMessages.EDDSAKEYSIGN1 || strings.HasSuffix(round, "SignRound1Message"):
		return tssMessages.EDDSAKEYSIGN1
	case round == tssMessages.EDDSAKEYSIGN2 || strings.HasSuffix(round, "SignRound2Message"):
		return tssMessages.EDDSAKEYSIGN2
	case round == tssMessages.EDDSAKEYSIGN3 || strings.HasSuffix(round, "SignRound3Message"):
		return tssMessages.EDDSAKEYSIGN3
	default:
		return round
	}
}
