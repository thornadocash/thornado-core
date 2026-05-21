package tss

import (
	"reflect"
	"testing"

	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign"
)

func TestSchnorrBlameFromIdentifiableParty(t *testing.T) {
	participants := []string{"thorpub-a", "thorpub-b", "thorpub-c"}
	blame := schnorrBlameFromError(assertErr("IDENTIFIABLE_ABORT keysign party=2: bad share"), participants, schnorrKeysignAbortRound)
	if blame.Round != schnorrKeysignAbortRound {
		t.Fatalf("unexpected round: %s", blame.Round)
	}
	if len(blame.BlameNodes) != 1 || blame.BlameNodes[0].Pubkey != "thorpub-b" {
		t.Fatalf("unexpected blame: %+v", blame.BlameNodes)
	}
}

func TestSchnorrBlameFallbackDefault(t *testing.T) {
	participants := []string{"leader", "node-b", "node-c"}
	blame := schnorrBlameFromErrorWithDefault(assertErr("timeout"), participants, schnorrKeysignAbortRound, []string{"leader"})
	if len(blame.BlameNodes) != 1 || blame.BlameNodes[0].Pubkey != "leader" {
		t.Fatalf("unexpected blame: %+v", blame.BlameNodes)
	}
}

func TestSchnorrSignerPubKeysFallbackToLocalState(t *testing.T) {
	localState := storage.KeygenLocalState{ParticipantKeys: []string{"node-a", "node-b", "node-c"}}

	got := schnorrSignerPubKeys(keysign.Request{}, localState)
	if !reflect.DeepEqual(got, localState.ParticipantKeys) {
		t.Fatalf("expected local state participants, got %#v", got)
	}

	req := keysign.Request{SignerPubKeys: []string{"node-b", "node-c"}}
	got = schnorrSignerPubKeys(req, localState)
	if !reflect.DeepEqual(got, req.SignerPubKeys) {
		t.Fatalf("expected request signers, got %#v", got)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
