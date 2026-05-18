package tss

import (
	"fmt"

	tssMessages "gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// KeysignError is a custom error create to include which party to blame
type KeysignError struct {
	Blame types.Blame
}

// NewKeysignError create a new instance of KeysignError
func NewKeysignError(blame types.Blame) KeysignError {
	return KeysignError{
		Blame: blame,
	}
}

// Error implement error interface
func (k KeysignError) Error() string {
	return fmt.Sprintf("fail to complete TSS keysign, reason:%s, round:%s, culprit:%+v", k.Blame.FailReason, k.Blame.Round, k.Blame.BlameNodes)
}

func (k KeysignError) IsRound7() bool {
	round := types.CanonicalTssRound(k.Blame.Round, "")
	// Ed25519 reaches its terminal keysign phase at round 3, which is handled the
	// same way as secp256k1 round 7 by the retry/freeze paths.
	return round == tssMessages.KEYSIGN7 || round == tssMessages.EDDSAKEYSIGN3
}
