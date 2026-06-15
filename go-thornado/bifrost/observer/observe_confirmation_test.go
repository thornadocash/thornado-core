package observer

import (
	"testing"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
)

func TestFinaliseHeightCountsObservedBlockAsFirstConfirmation(t *testing.T) {
	tests := []struct {
		name     string
		block    int64
		required int64
		want     int64
	}{
		{name: "zero confirmations", block: 42, required: 0, want: 42},
		{name: "one confirmation", block: 42, required: 1, want: 42},
		{name: "two confirmations", block: 42, required: 2, want: 43},
		{name: "six confirmations", block: 42, required: 6, want: 47},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := finaliseHeight(tt.block, tt.required); got != tt.want {
				t.Fatalf("finaliseHeight(%d, %d) = %d, want %d", tt.block, tt.required, got, tt.want)
			}
		})
	}
}

func TestTxInKeySeparatesMempoolAndBlockObservations(t *testing.T) {
	item := types.NewTxInItem(
		100,
		"54ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528",
		"from",
		"to",
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))},
		common.Gas{},
		common.EmptyPubKey,
		"",
		"",
		nil,
	)

	mempool := types.TxIn{Chain: common.BTCChain, TxArray: []*types.TxInItem{item.Copy()}, MemPool: true, ConfirmationRequired: 2}
	block := types.TxIn{Chain: common.BTCChain, TxArray: []*types.TxInItem{item.Copy()}, MemPool: false, ConfirmationRequired: 2}

	mempoolKey := TxInKey(&mempool)
	blockKey := TxInKey(&block)

	if mempoolKey.height != 101 || blockKey.height != 101 {
		t.Fatalf("unexpected finalise heights: mempool=%d block=%d", mempoolKey.height, blockKey.height)
	}
	if mempoolKey == blockKey {
		t.Fatalf("mempool and block observations must not share a deck key")
	}
	if !mempoolKey.mempool || blockKey.mempool {
		t.Fatalf("unexpected mempool flags: mempool=%v block=%v", mempoolKey.mempool, blockKey.mempool)
	}
}

func TestPromoteMempoolObservationCarriesPendingCommit(t *testing.T) {
	storage, err := NewObserverStorage(t.TempDir(), config.LevelDBOptions{})
	if err != nil {
		t.Fatalf("failed to create observer storage: %v", err)
	}
	defer storage.Close()

	pendingItem := types.NewTxInItem(
		100,
		"54ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528",
		"from",
		"to",
		common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))},
		common.Gas{},
		common.EmptyPubKey,
		"",
		"",
		nil,
	)
	pendingItem.CommittedUnFinalised = true
	mempoolDeck := &types.TxIn{
		Chain:                common.BTCChain,
		TxArray:              []*types.TxInItem{pendingItem},
		MemPool:              true,
		ConfirmationRequired: 2,
	}
	observer := &Observer{
		onDeck:  map[txInKey]*types.TxIn{TxInKey(mempoolDeck): mempoolDeck},
		storage: storage,
	}

	blockItem := pendingItem.Copy()
	blockItem.BlockHeight = 101
	blockItem.CommittedUnFinalised = false
	blockDeck := &types.TxIn{
		Chain:                common.BTCChain,
		TxArray:              []*types.TxInItem{blockItem},
		MemPool:              false,
		ConfirmationRequired: 2,
	}

	observer.promoteMempoolObservationsLocked(blockDeck)

	if len(observer.onDeck) != 0 {
		t.Fatalf("mempool deck should be removed after promotion")
	}
	if !blockItem.CommittedUnFinalised {
		t.Fatalf("block observation should inherit committed pending state")
	}
}
