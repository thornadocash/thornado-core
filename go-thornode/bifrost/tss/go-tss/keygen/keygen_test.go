package keygen

import (
	"testing"

	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/blame"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/common"
	tcommon "gitlab.com/thorchain/thornode/v3/common"
)

func TestNewRequest(t *testing.T) {
	keys := []string{"key1", "key2", "key3"}
	blockHeight := int64(100)
	version := "0.14.0"
	algo := tcommon.SigningAlgoSecp256k1

	req := NewRequest(keys, blockHeight, version, algo)

	if len(req.Keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(req.Keys))
	}
	if req.Keys[0] != "key1" || req.Keys[1] != "key2" || req.Keys[2] != "key3" {
		t.Fatalf("unexpected keys: %v", req.Keys)
	}
	if req.BlockHeight != blockHeight {
		t.Fatalf("expected block height %d, got %d", blockHeight, req.BlockHeight)
	}
	if req.Version != version {
		t.Fatalf("expected version %s, got %s", version, req.Version)
	}
	if req.Algo != algo {
		t.Fatalf("expected algo %s, got %s", algo, req.Algo)
	}
}

func TestNewRequest_Empty(t *testing.T) {
	req := NewRequest(nil, 0, "", "")
	if req.Keys != nil {
		t.Fatalf("expected nil keys, got %v", req.Keys)
	}
	if req.BlockHeight != 0 {
		t.Fatalf("expected block height 0, got %d", req.BlockHeight)
	}
}

func TestNewResponse(t *testing.T) {
	algo := tcommon.SigningAlgoEd25519
	pubKey := "thorpub1addwnpepqtest"
	poolAddr := "thor1test"
	status := common.Success
	bl := blame.Blame{
		FailReason: "test reason",
	}

	resp := NewResponse(algo, pubKey, poolAddr, status, bl)

	if resp.Algo != algo {
		t.Fatalf("expected algo %s, got %s", algo, resp.Algo)
	}
	if resp.PubKey != pubKey {
		t.Fatalf("expected pubkey %s, got %s", pubKey, resp.PubKey)
	}
	if resp.PoolAddress != poolAddr {
		t.Fatalf("expected pool address %s, got %s", poolAddr, resp.PoolAddress)
	}
	if resp.Status != status {
		t.Fatalf("expected status %d, got %d", status, resp.Status)
	}
	if resp.Blame.FailReason != "test reason" {
		t.Fatalf("expected blame reason 'test reason', got %s", resp.Blame.FailReason)
	}
}

func TestNewResponse_Fail(t *testing.T) {
	bl := blame.Blame{
		FailReason: "node offline",
		BlameNodes: []blame.Node{{Pubkey: "key1"}},
	}

	resp := NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, bl)

	if resp.Status != common.Fail {
		t.Fatalf("expected fail status, got %d", resp.Status)
	}
	if resp.PubKey != "" {
		t.Fatalf("expected empty pubkey, got %s", resp.PubKey)
	}
	if len(resp.Blame.BlameNodes) != 1 {
		t.Fatalf("expected 1 blame node, got %d", len(resp.Blame.BlameNodes))
	}
}
