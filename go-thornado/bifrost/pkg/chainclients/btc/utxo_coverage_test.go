package btc

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// -------------------------------------------------------------------------------------
// Mock ThornadoBridge
// -------------------------------------------------------------------------------------

type mockBridge struct {
	thornadoclient.ThornadoBridge
	basePubKeys     []thornadoclient.PubKeyAddressPair
	baseErr         error
	postKeysignTxID common.TxID
	postKeysignErr  error
}

func (m *mockBridge) GetBasePubKeys() ([]thornadoclient.PubKeyAddressPair, error) {
	return m.basePubKeys, m.baseErr
}

func (m *mockBridge) GetConfigValue(key string) (int64, error) {
	switch key {
	case constants.BTC_ConfMultiplierBasisPoints.String(), constants.BTC_MaxConfirmations.String():
		return 1, nil
	default:
		return 0, nil
	}
}

func (m *mockBridge) PostKeysignFailure(_ types.Blame, _ int64, _ common.Coins, _ common.PubKey) (common.TxID, error) {
	return m.postKeysignTxID, m.postKeysignErr
}

// -------------------------------------------------------------------------------------
// GetBaseAddress Tests
// -------------------------------------------------------------------------------------

type GetBaseAddressSuite struct{}

var _ = Suite(&GetBaseAddressSuite{})

func (s *GetBaseAddressSuite) TestGetBaseAddress_BridgeError(c *C) {
	bridge := &mockBridge{
		baseErr: errors.New("fail to get baseVaults"),
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "fail to get baseVaults : fail to get baseVaults")
	c.Assert(addrs, IsNil)
}

func (s *GetBaseAddressSuite) TestGetBaseAddress_Empty(c *C) {
	bridge := &mockBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{},
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, IsNil)
	c.Assert(addrs, HasLen, 0)
}

func (s *GetBaseAddressSuite) TestGetBaseAddress_FiltersNonSecp256k1(c *C) {
	bridge := &mockBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{
			{
				PubKey: types.GetRandomPubKey(),
				Algo:   common.SigningAlgoEd25519,
			},
		},
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, IsNil)
	c.Assert(addrs, HasLen, 0)
}

func (s *GetBaseAddressSuite) TestGetBaseAddress_Secp256k1Success(c *C) {
	pk := types.GetRandomPubKey()
	bridge := &mockBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{
			{
				PubKey: pk,
				Algo:   common.SigningAlgoSecp256k1,
			},
		},
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, IsNil)
	c.Assert(len(addrs) > 0, Equals, true)
}

func (s *GetBaseAddressSuite) TestGetBaseAddress_BadPubKeySkipped(c *C) {
	bridge := &mockBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{
			{
				PubKey: common.PubKey("invalid-pub-key"),
				Algo:   common.SigningAlgoSecp256k1,
			},
		},
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, IsNil)
	// Bad pub key should be skipped, not cause an error
	c.Assert(addrs, HasLen, 0)
}

func (s *GetBaseAddressSuite) TestGetBaseAddress_MixedAlgos(c *C) {
	pk := types.GetRandomPubKey()
	bridge := &mockBridge{
		basePubKeys: []thornadoclient.PubKeyAddressPair{
			{
				PubKey: common.PubKey("invalid"),
				Algo:   common.SigningAlgoEd25519,
			},
			{
				PubKey: pk,
				Algo:   common.SigningAlgoSecp256k1,
			},
			{
				PubKey: common.PubKey("bad-key"),
				Algo:   common.SigningAlgoSecp256k1,
			},
		},
	}
	addrs, err := GetBaseAddress(common.BTCChain, bridge)
	c.Assert(err, IsNil)
	// First: skipped (ed25519), Second: base address plus user/node BTC deposit lookahead, Third: skipped (bad key)
	c.Assert(len(addrs), Equals, int(common.DepositAddressLookahead)*2+1)
}

// -------------------------------------------------------------------------------------
// PostKeysignFailure Tests
// -------------------------------------------------------------------------------------

type PostKeysignFailureSuite struct{}

var _ = Suite(&PostKeysignFailureSuite{})

func (s *PostKeysignFailureSuite) TestPostKeysignFailure_NonKeysignError(c *C) {
	bridge := &mockBridge{}
	tx := stypes.TxOutItem{}

	// Non-KeysignError should be returned as-is
	regularErr := errors.New("some random error")
	err := PostKeysignFailure(bridge, tx, zerolog.Nop(), 100, regularErr)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "some random error")
}

func (s *PostKeysignFailureSuite) TestPostKeysignFailure_KeysignError_EmptyBlameNodes(c *C) {
	bridge := &mockBridge{}
	tx := stypes.TxOutItem{}
	keysignErr := frost.NewKeysignError(types.Blame{
		FailReason: "some failure",
		BlameNodes: []types.Node{}, // empty
	})
	err := PostKeysignFailure(bridge, tx, zerolog.Nop(), 100, keysignErr)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "(?s).*fail to sign the message.*")
}

func (s *PostKeysignFailureSuite) TestPostKeysignFailure_KeysignError_PostSuccess(c *C) {
	bridge := &mockBridge{
		postKeysignTxID: common.TxID("ABCDEF"),
	}
	tx := stypes.TxOutItem{}
	keysignErr := frost.NewKeysignError(types.Blame{
		FailReason: "some failure",
		BlameNodes: []types.Node{{Pubkey: "blame-node"}},
	})
	err := PostKeysignFailure(bridge, tx, zerolog.Nop(), 100, keysignErr)
	// After successful post, the original keysign error is returned (not nil)
	c.Assert(err, NotNil)
	var keysignErr2 frost.KeysignError
	c.Assert(errors.As(err, &keysignErr2), Equals, true)
}

func (s *PostKeysignFailureSuite) TestPostKeysignFailure_KeysignError_PostError(c *C) {
	bridge := &mockBridge{
		postKeysignErr: errors.New("post failed"),
	}
	tx := stypes.TxOutItem{}
	keysignErr := frost.NewKeysignError(types.Blame{
		FailReason: "some failure",
		BlameNodes: []types.Node{{Pubkey: "blame-node"}},
	})
	err := PostKeysignFailure(bridge, tx, zerolog.Nop(), 100, keysignErr)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, "(?s).*fail to sign the message.*")
	c.Assert(err.Error(), Matches, "(?s).*fail to post keysign failure.*")
}

// -------------------------------------------------------------------------------------
// GetConfMulBasisPoint / MaxConfAdjustment Tests (mocknet)
// -------------------------------------------------------------------------------------

type ConfAdjustmentSuite struct{}

var _ = Suite(&ConfAdjustmentSuite{})

func (s *ConfAdjustmentSuite) TestGetConfMulBasisPoint(c *C) {
	bridge := &mockBridge{}
	val, err := GetConfMulBasisPoint("BTC", bridge)
	c.Assert(err, IsNil)
	c.Assert(val.Uint64(), Equals, uint64(1))
}

func (s *ConfAdjustmentSuite) TestMaxConfAdjustment(c *C) {
	bridge := &mockBridge{}
	val, err := MaxConfAdjustment(10, "BTC", bridge)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, uint64(1))
}

// -------------------------------------------------------------------------------------
// Additional BlockMeta Tests
// -------------------------------------------------------------------------------------

type BlockMetaExtraSuite struct{}

var _ = Suite(&BlockMetaExtraSuite{})

func (s *BlockMetaExtraSuite) TestTransactionHashExists_CustomerTx(c *C) {
	bm := NewBlockMeta("prev", 100, "hash")
	bm.AddCustomerTransaction("tx1")
	c.Assert(bm.TransactionHashExists("tx1"), Equals, true)
	c.Assert(bm.TransactionHashExists("TX1"), Equals, true) // case-insensitive
	c.Assert(bm.TransactionHashExists("tx2"), Equals, false)
}

func (s *BlockMetaExtraSuite) TestTransactionHashExists_SelfTx(c *C) {
	bm := NewBlockMeta("prev", 100, "hash")
	bm.AddSelfTransaction("tx1")
	c.Assert(bm.TransactionHashExists("tx1"), Equals, true)
	c.Assert(bm.TransactionHashExists("TX1"), Equals, true)
}

func (s *BlockMetaExtraSuite) TestAddSelfTransaction_Dedup(c *C) {
	bm := NewBlockMeta("prev", 100, "hash")
	bm.AddSelfTransaction("tx1")
	bm.AddSelfTransaction("tx1")
	bm.AddSelfTransaction("TX1")
	c.Assert(bm.SelfTransactions, HasLen, 1)
}

func (s *BlockMetaExtraSuite) TestAddCustomerTransaction_Dedup(c *C) {
	bm := NewBlockMeta("prev", 100, "hash")
	bm.AddCustomerTransaction("tx1")
	bm.AddCustomerTransaction("tx1")
	bm.AddCustomerTransaction("TX1")
	c.Assert(bm.CustomerTransactions, HasLen, 1)
}

func (s *BlockMetaExtraSuite) TestRemoveCustomerTransaction_NotExist(c *C) {
	bm := NewBlockMeta("prev", 100, "hash")
	bm.AddCustomerTransaction("tx1")
	bm.RemoveCustomerTransaction("tx2")
	c.Assert(bm.CustomerTransactions, HasLen, 1)
}

func (s *BlockMetaExtraSuite) TestRemoveTransaction_Internal(c *C) {
	hashes := []string{"a", "b", "c"}
	hashes = removeTransaction(hashes, "b")
	c.Assert(hashes, HasLen, 2)
	c.Assert(hashes[0], Equals, "a")
	c.Assert(hashes[1], Equals, "c")

	// remove non-existing
	hashes = removeTransaction(hashes, "d")
	c.Assert(hashes, HasLen, 2)
}

// -------------------------------------------------------------------------------------
// Additional TemporalStorage Tests (mempool, observed tx, cache)
// -------------------------------------------------------------------------------------

type TemporalStorageExtraSuite struct{}

var _ = Suite(&TemporalStorageExtraSuite{})

func (s *TemporalStorageExtraSuite) newStorage(c *C, cacheSize int) (*TemporalStorage, *leveldb.DB) {
	memStorage := storage.NewMemStorage()
	db, err := leveldb.Open(memStorage, nil)
	c.Assert(err, IsNil)
	store, err := NewTemporalStorage(db, cacheSize)
	c.Assert(err, IsNil)
	return store, db
}

func (s *TemporalStorageExtraSuite) TestNewTemporalStorage_WithCache(c *C) {
	store, db := s.newStorage(c, 100)
	c.Assert(store, NotNil)
	c.Assert(store.mempoolTxIDCache, NotNil)
	c.Assert(db.Close(), IsNil)
}

func (s *TemporalStorageExtraSuite) TestNewTemporalStorage_NoCache(c *C) {
	store, db := s.newStorage(c, 0)
	c.Assert(store, NotNil)
	c.Assert(store.mempoolTxIDCache, IsNil)
	c.Assert(db.Close(), IsNil)
}

func (s *TemporalStorageExtraSuite) TestTrackMempoolTx_NoCache(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	added, err := store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)

	// second add should return false (already tracked)
	added, err = store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, false)
}

func (s *TemporalStorageExtraSuite) TestTrackMempoolTx_WithCache(c *C) {
	store, db := s.newStorage(c, 100)
	defer func() { c.Assert(db.Close(), IsNil) }()

	added, err := store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)

	// Now it's in the cache, so next call should hit cache
	added, err = store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, false)
}

func (s *TemporalStorageExtraSuite) TestUntrackMempoolTx_NoCache(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	_, _ = store.TrackMempoolTx("tx1")
	err := store.UntrackMempoolTx("tx1")
	c.Assert(err, IsNil)

	// After untracking, should be able to track again
	added, err := store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
}

func (s *TemporalStorageExtraSuite) TestUntrackMempoolTx_WithCache(c *C) {
	store, db := s.newStorage(c, 100)
	defer func() { c.Assert(db.Close(), IsNil) }()

	_, _ = store.TrackMempoolTx("tx1")
	err := store.UntrackMempoolTx("tx1")
	c.Assert(err, IsNil)

	// After untracking (including cache removal), should be trackable again
	added, err := store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
}

func (s *TemporalStorageExtraSuite) TestMempoolTxLookupAndList(c *C) {
	store, db := s.newStorage(c, 100)
	defer func() { c.Assert(db.Close(), IsNil) }()

	exists, err := store.HasMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	added, err := store.TrackMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)

	exists, err = store.HasMempoolTx("tx1")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	txids, err := store.ListMempoolTxs()
	c.Assert(err, IsNil)
	c.Assert(txids, DeepEquals, []string{"tx1"})
}

func (s *TemporalStorageExtraSuite) TestTrackObservedTx(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	added, previous, err := store.TrackObservedTxStage("obs1", ObservedTxStageMempool)
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
	c.Assert(previous, Equals, "")

	added, previous, err = store.TrackObservedTxStage("obs1", ObservedTxStageFinal)
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
	c.Assert(previous, Equals, ObservedTxStageMempool)

	added, previous, err = store.TrackObservedTxStage("obs1", ObservedTxStageFinal)
	c.Assert(err, IsNil)
	c.Assert(added, Equals, false)
	c.Assert(previous, Equals, ObservedTxStageFinal)
}

func (s *TemporalStorageExtraSuite) TestUntrackObservedTx(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	_, _ = store.TrackObservedTx("obs1")
	err := store.UntrackObservedTx("obs1")
	c.Assert(err, IsNil)

	// After untracking, should be trackable again
	added, err := store.TrackObservedTx("obs1")
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
}

func (s *TemporalStorageExtraSuite) TestSetSpentUtxos_EmptyId(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	// setSpentUtxoById with empty id should be a no-op
	err := store.setSpentUtxoById("")
	c.Assert(err, IsNil)
}

func (s *TemporalStorageExtraSuite) TestSetSpentUtxosByHeight_EmptyIdsDedup(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	// Set utxos with empty string mixed in - should be filtered
	err := store.SetSpentUtxos([]string{"id1", ""}, 100)
	c.Assert(err, IsNil)

	// setSpentUtxosByHeight with empty string in ids list
	ids, err := store.GetSpentUtxosByHeight(100)
	c.Assert(err, IsNil)
	// empty string should be deduplicated out
	c.Assert(ids, HasLen, 1)
	c.Assert(ids[0], Equals, "id1")
}

func (s *TemporalStorageExtraSuite) TestPruneBlockMeta_NilFilter(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	// Save some block metas
	for i := int64(0); i < 5; i++ {
		bm := NewBlockMeta(fmt.Sprintf("prev%d", i), i, fmt.Sprintf("hash%d", i))
		c.Assert(store.SaveBlockMeta(i, bm), IsNil)
	}

	// Prune with nil filter - all below height should be pruned
	c.Assert(store.PruneBlockMeta(3, nil), IsNil)

	metas, err := store.GetBlockMetas()
	c.Assert(err, IsNil)
	// Heights 0,1,2 pruned; 3,4 remain
	c.Assert(metas, HasLen, 2)
}

func (s *TemporalStorageExtraSuite) TestPruneBlockMeta_FilterPrevents(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	for i := int64(0); i < 5; i++ {
		bm := NewBlockMeta(fmt.Sprintf("prev%d", i), i, fmt.Sprintf("hash%d", i))
		c.Assert(store.SaveBlockMeta(i, bm), IsNil)
	}

	// Filter returns false for all, preventing any pruning
	c.Assert(store.PruneBlockMeta(3, func(meta *BlockMeta) bool {
		return false
	}), IsNil)

	metas, err := store.GetBlockMetas()
	c.Assert(err, IsNil)
	c.Assert(metas, HasLen, 5) // nothing pruned
}

func (s *TemporalStorageExtraSuite) TestGetSpentUtxosByHeight_NotFound(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	ids, err := store.GetSpentUtxosByHeight(999)
	c.Assert(err, IsNil)
	c.Assert(ids, IsNil)
}

func (s *TemporalStorageExtraSuite) TestKeyFormats(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	c.Assert(store.getBlockMetaKey(100), Equals, "blockmeta-100")
	c.Assert(store.getMemPoolKey("txhash"), Equals, "mempool-txhash")
	c.Assert(store.getObservedTxKey("txhash"), Equals, "observed-txhash")
	c.Assert(store.getSpentUtxoByIdKey("utxoid"), Equals, "spentutxobyid-utxoid")
	c.Assert(store.getSpentUtxoByHeightKey(50), Equals, "spentutxobyheight--50")
}

func (s *TemporalStorageExtraSuite) TestPruneSpentUtxos_Full(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	// Set up utxos at multiple heights
	c.Assert(store.SetSpentUtxos([]string{"a1", "a2"}, 10), IsNil)
	c.Assert(store.SetSpentUtxos([]string{"b1"}, 20), IsNil)
	c.Assert(store.SetSpentUtxos([]string{"c1"}, 30), IsNil)

	// Prune up to and including height 20
	c.Assert(store.PruneSpentUtxos(20), IsNil)

	// Heights 10 and 20 should be pruned
	has, err := store.HasSpentUtxo("a1")
	c.Assert(err, IsNil)
	c.Assert(has, Equals, false)

	has, err = store.HasSpentUtxo("b1")
	c.Assert(err, IsNil)
	c.Assert(has, Equals, false)

	// Height 30 should remain
	has, err = store.HasSpentUtxo("c1")
	c.Assert(err, IsNil)
	c.Assert(has, Equals, true)
}

func (s *TemporalStorageExtraSuite) TestPruneSpentUtxos_EmptyStorage(c *C) {
	store, db := s.newStorage(c, 0)
	defer func() { c.Assert(db.Close(), IsNil) }()

	// Should not error on empty storage
	c.Assert(store.PruneSpentUtxos(100), IsNil)
}

// -------------------------------------------------------------------------------------
// SignCheckpoint type test
// -------------------------------------------------------------------------------------

type SignCheckpointSuite struct{}

var _ = Suite(&SignCheckpointSuite{})

func (s *SignCheckpointSuite) TestSignCheckpoint(c *C) {
	sc := SignCheckpoint{
		UnsignedTx:        []byte("raw-tx"),
		IndividualAmounts: map[string]int64{"addr1": 100, "addr2": 200},
	}
	c.Assert(sc.UnsignedTx, HasLen, 6)
	c.Assert(sc.IndividualAmounts["addr1"], Equals, int64(100))
	c.Assert(sc.IndividualAmounts["addr2"], Equals, int64(200))
}
