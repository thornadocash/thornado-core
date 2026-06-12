package types

import (
	"fmt"
	"strings"

	frostMessages "github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/common"
)

// CanonicalFrostRound normalizes raw FROST round names into the canonical constants
// used in Thornado consensus and telemetry. When algo is empty, it is inferred
// from the raw round name when possible.
func CanonicalFrostRound(round string, algo common.SigningAlgo) string {
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
		return canonicalSecp256k1FrostRound(round)
	case common.SigningAlgoEd25519:
		return canonicalEd25519FrostRound(round)
	default:
		return round
	}
}

// NormalizeFrostKeysignRound canonicalizes a keysign round and verifies that the
// result is one of the supported keysign-round identifiers.
func NormalizeFrostKeysignRound(round string) (string, error) {
	canonical := CanonicalFrostRound(round, "")
	if canonical == "" {
		return "", nil
	}
	if !frostMessages.ValidFrostKeysignRounds[canonical] {
		return "", fmt.Errorf("invalid blame round: %s", round)
	}
	return canonical, nil
}

// canonicalSecp256k1FrostRound maps secp256k1 FROST library phase names into the
// stable round identifiers used in messages and state.
func canonicalSecp256k1FrostRound(round string) string {
	switch {
	case round == frostMessages.KEYGEN1 || strings.HasSuffix(round, "KGRound1Message"):
		return frostMessages.KEYGEN1
	case round == frostMessages.KEYGEN2aUnicast || strings.HasSuffix(round, "KGRound2Message1"):
		return frostMessages.KEYGEN2aUnicast
	case round == frostMessages.KEYGEN2b || strings.HasSuffix(round, "KGRound2Message2"):
		return frostMessages.KEYGEN2b
	case round == frostMessages.KEYGEN3 || strings.HasSuffix(round, "KGRound3Message"):
		return frostMessages.KEYGEN3
	case round == frostMessages.KEYSIGN1aUnicast || strings.HasSuffix(round, "SignRound1Message1"):
		return frostMessages.KEYSIGN1aUnicast
	case round == frostMessages.KEYSIGN1b || strings.HasSuffix(round, "SignRound1Message2"):
		return frostMessages.KEYSIGN1b
	case round == frostMessages.KEYSIGN2Unicast || strings.HasSuffix(round, "SignRound2Message"):
		return frostMessages.KEYSIGN2Unicast
	case round == frostMessages.KEYSIGN3 || strings.HasSuffix(round, "SignRound3Message"):
		return frostMessages.KEYSIGN3
	case round == frostMessages.KEYSIGN4 || strings.HasSuffix(round, "SignRound4Message"):
		return frostMessages.KEYSIGN4
	case round == frostMessages.KEYSIGN5 || strings.HasSuffix(round, "SignRound5Message"):
		return frostMessages.KEYSIGN5
	case round == frostMessages.KEYSIGN6 || strings.HasSuffix(round, "SignRound6Message"):
		return frostMessages.KEYSIGN6
	case round == frostMessages.KEYSIGN7 || strings.HasSuffix(round, "SignRound7Message"):
		return frostMessages.KEYSIGN7
	default:
		return round
	}
}

// canonicalEd25519FrostRound maps ed25519 FROST library phase names into the
// stable round identifiers used in messages and state.
func canonicalEd25519FrostRound(round string) string {
	switch {
	case round == frostMessages.EDDSAKEYGEN1 || strings.HasSuffix(round, "KGRound1Message"):
		return frostMessages.EDDSAKEYGEN1
	case round == frostMessages.EDDSAKEYGEN2a || strings.HasSuffix(round, "KGRound2Message1"):
		return frostMessages.EDDSAKEYGEN2a
	case round == frostMessages.EDDSAKEYGEN2b || strings.HasSuffix(round, "KGRound2Message2"):
		return frostMessages.EDDSAKEYGEN2b
	case round == frostMessages.EDDSAKEYSIGN1 || strings.HasSuffix(round, "SignRound1Message"):
		return frostMessages.EDDSAKEYSIGN1
	case round == frostMessages.EDDSAKEYSIGN2 || strings.HasSuffix(round, "SignRound2Message"):
		return frostMessages.EDDSAKEYSIGN2
	case round == frostMessages.EDDSAKEYSIGN3 || strings.HasSuffix(round, "SignRound3Message"):
		return frostMessages.EDDSAKEYSIGN3
	default:
		return round
	}
}
