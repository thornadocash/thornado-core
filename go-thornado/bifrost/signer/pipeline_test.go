package signer

import (
	"testing"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type deferredCleanupBridge struct {
	thornadoclient.ThornadoBridge
	height int64
}

func (b deferredCleanupBridge) GetBlockHeight() (int64, error) {
	return b.height, nil
}

func (b deferredCleanupBridge) GetVault(string) (ttypes.Vault, error) {
	return ttypes.Vault{Status: ttypes.VaultStatus_ActiveVault}, nil
}

type deferredCleanupSigner struct {
	items     []TxOutStoreItem
	terminal  map[int64]bool
	deferred  int
	processed int
}

func (s *deferredCleanupSigner) isStopped() bool {
	return false
}

func (s *deferredCleanupSigner) storageList() []TxOutStoreItem {
	return s.items
}

func (s *deferredCleanupSigner) processTerminalOrCompletedTransaction(item TxOutStoreItem) (TxOutStoreItem, bool) {
	if s.terminal[item.Index] {
		return item, true
	}
	return item, false
}

func (s *deferredCleanupSigner) processDeferredTransaction(TxOutStoreItem) {
	s.deferred++
}

func (s *deferredCleanupSigner) processTransaction(TxOutStoreItem) {
	s.processed++
}

func TestSpawnSigningsRunsDeferredCleanupWithoutSigning(t *testing.T) {
	item := TxOutStoreItem{
		TxOutItem:           stypes.TxOutItem{TxType: ttypes.TxOutTypeOut},
		DeferredUntilHeight: 100,
	}
	signer := &deferredCleanupSigner{items: []TxOutStoreItem{item}}
	pipeline, err := newPipeline(1)
	if err != nil {
		t.Fatal(err)
	}

	pipeline.SpawnSignings(signer, deferredCleanupBridge{height: 10})

	if signer.deferred != 1 {
		t.Fatalf("deferred cleanup calls = %d, want 1", signer.deferred)
	}
	if signer.processed != 0 {
		t.Fatalf("signing calls = %d, want 0", signer.processed)
	}
}

func TestSpawnSigningsPrunesTerminalBeforeSelectingVaultChain(t *testing.T) {
	items := []TxOutStoreItem{
		{
			Index:     0,
			TxOutItem: stypes.TxOutItem{Chain: common.BTCChain, TxType: ttypes.TxOutTypeOut},
		},
		{
			Index:     1,
			TxOutItem: stypes.TxOutItem{Chain: common.BTCChain, TxType: ttypes.TxOutTypeOut},
		},
		{
			Index:     2,
			TxOutItem: stypes.TxOutItem{Chain: common.BTCChain, TxType: ttypes.TxOutTypeOut},
		},
	}
	signer := &deferredCleanupSigner{
		items:    items,
		terminal: map[int64]bool{0: true, 1: true},
	}
	pipeline, err := newPipeline(1)
	if err != nil {
		t.Fatal(err)
	}

	pipeline.SpawnSignings(signer, deferredCleanupBridge{height: 10})
	pipeline.Wait()

	if signer.processed != 1 {
		t.Fatalf("signing calls = %d, want 1", signer.processed)
	}
}
