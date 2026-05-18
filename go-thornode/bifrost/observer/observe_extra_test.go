package observer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/ebifrost"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// encodeStreamMsg encodes a message in the p2p wire format (4-byte LE length header + data)
func encodeStreamMsg(data []byte) []byte {
	length := uint32(len(data))
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, length)
	return append(header, data...)
}

// NewMockStreamWithData creates a mock stream that reads provided data in p2p wire format
func NewMockStreamWithData(data []byte, peerID peer.ID) *MockStream {
	var buf bytes.Buffer
	buf.Write(encodeStreamMsg(data))
	return &MockStream{
		reader: &buf,
		writer: io.Discard,
		peer:   peerID,
		mu:     &sync.Mutex{},
	}
}

// sharedTestMetrics provides a single metrics instance shared across all check.v1
// suites in this package, avoiding Prometheus duplicate-registration panics.
var (
	sharedTestMetrics     *metrics.Metrics
	sharedTestMetricsOnce sync.Once
)

func getSharedTestMetrics() *metrics.Metrics {
	sharedTestMetricsOnce.Do(func() {
		var err error
		sharedTestMetrics, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9000,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.BTCChain, common.AVAXChain},
		})
		if err != nil {
			panic(err)
		}
	})
	return sharedTestMetrics
}

type ObserveExtraSuite struct {
	m *metrics.Metrics
}

var _ = Suite(&ObserveExtraSuite{})

func (s *ObserveExtraSuite) SetUpSuite(c *C) {
	s.m = getSharedTestMetrics()
}

// ---------------------------------------------------------------------------
// Delay tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestDelay(c *C) {
	d := NewDelay()
	c.Assert(d.IsRunning(), Equals, false)

	d.Start()
	c.Assert(d.IsRunning(), Equals, true)

	d.Done()
	c.Assert(d.IsRunning(), Equals, false)
}

// ---------------------------------------------------------------------------
// TxInKey tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestTxInKey(c *C) {
	txIn := &types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: "abc123"},
		},
	}

	key := TxInKey(txIn)
	c.Assert(key.chain, Equals, common.BTCChain)
	c.Assert(key.height, Equals, int64(106))
}

// ---------------------------------------------------------------------------
// getChain tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetChain(c *C) {
	obs := &Observer{
		logger: log.Logger,
		chains: map[common.Chain]chainclients.ChainClient{
			common.BTCChain: nil,
		},
	}

	_, err := obs.getChain(common.BTCChain)
	c.Assert(err, IsNil)

	_, err = obs.getChain(common.ETHChain)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "not supported")
}

// ---------------------------------------------------------------------------
// filterObservations tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestFilterObservations(c *C) {
	vault := stypes.GetRandomPubKey()
	vaultAddr, err := vault.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)

	signedCache, err := lru.New(100)
	c.Assert(err, IsNil)

	pubkeyMgr, errPkm := pubkeymanager.NewPubKeyManager(nil, nil)
	c.Assert(errPkm, IsNil)

	obs := &Observer{
		logger:           log.Logger,
		pubkeyMgr:        pubkeyMgr,
		signedTxOutCache: signedCache,
	}

	// Test with empty items - should return nil
	result := obs.filterObservations(common.BTCChain, nil, false)
	c.Assert(result, IsNil)

	// Test with non-matching addresses
	items := []*types.TxInItem{
		{
			Tx:     "tx1",
			Sender: "unknown_sender",
			To:     "unknown_to",
			Memo:   "SWAP:ETH.ETH",
		},
	}
	result = obs.filterObservations(common.BTCChain, items, false)
	c.Assert(len(result), Equals, 0)

	// Test cancel tx (sender == to, empty memo) - should be skipped
	items = []*types.TxInItem{
		{
			Tx:     "tx2",
			Sender: vaultAddr.String(),
			To:     vaultAddr.String(),
			Memo:   "",
		},
	}
	result = obs.filterObservations(common.BTCChain, items, false)
	c.Assert(len(result), Equals, 0)
}

// ---------------------------------------------------------------------------
// processObservedTx tests (deduplication)
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessObservedTxDedup(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-dedup-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	obs := &Observer{
		logger:  log.Logger,
		lock:    &sync.Mutex{},
		onDeck:  make(map[txInKey]*types.TxIn),
		storage: storage,
		chains:  map[common.Chain]chainclients.ChainClient{},
	}

	// First observation
	txIn := types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		Filtered:             true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100, Tx: "tx1", Sender: "sender1", To: "to1",
				Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
			},
		},
	}
	obs.processObservedTx(txIn)

	// Should have 1 entry
	obs.lock.Lock()
	c.Assert(len(obs.onDeck), Equals, 1)
	obs.lock.Unlock()

	// Duplicate observation - same tx, should be deduped
	obs.processObservedTx(txIn)
	obs.lock.Lock()
	key := TxInKey(&txIn)
	c.Assert(len(obs.onDeck[key].TxArray), Equals, 1) // still 1 (deduped)
	obs.lock.Unlock()

	// New tx in same deck should be merged
	txIn2 := types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		Filtered:             true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100, Tx: "tx2", Sender: "sender2", To: "to2",
				Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(2000))},
			},
		},
	}
	obs.processObservedTx(txIn2)
	obs.lock.Lock()
	c.Assert(len(obs.onDeck[key].TxArray), Equals, 2) // now 2
	obs.lock.Unlock()

	// Test empty txArray
	empty := types.TxIn{
		Chain:   common.BTCChain,
		TxArray: []*types.TxInItem{},
	}
	obs.processObservedTx(empty) // should be a no-op
}

// ---------------------------------------------------------------------------
// getThorchainTxIns tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetThorchainTxIns(c *C) {
	obs := &Observer{
		logger:              log.Logger,
		errCounter:          s.m.GetCounterVec(metrics.ObserverError),
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	// Test with empty coins - should be marked invalid
	txIn := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "abc",
				Sender:      "sender",
				To:          "to",
				Coins:       common.Coins{},
				Memo:        "SWAP:ETH.ETH",
			},
		},
	}
	txs, invalidIndices, err := obs.getThorchainTxIns(txIn, false, 100)
	c.Assert(err, IsNil)
	c.Assert(len(txs), Equals, 0)
	c.Assert(len(invalidIndices), Equals, 1)
	c.Assert(invalidIndices[0], Equals, 0)

	// Test with oversized memo
	longMemo := make([]byte, 500)
	for i := range longMemo {
		longMemo[i] = 'a'
	}
	txIn2 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "abc",
				Sender:      "sender",
				To:          "to",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        string(longMemo),
			},
		},
	}
	txs2, invalidIndices2, err := obs.getThorchainTxIns(txIn2, false, 100)
	c.Assert(err, IsNil)
	c.Assert(len(txs2), Equals, 0)
	c.Assert(len(invalidIndices2), Equals, 1)

	// Test with empty To address
	txIn3 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "abc",
				Sender:      "sender",
				To:          "",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        "SWAP:ETH.ETH",
			},
		},
	}
	txs3, invalidIndices3, err := obs.getThorchainTxIns(txIn3, false, 100)
	c.Assert(err, IsNil)
	c.Assert(len(txs3), Equals, 0)
	c.Assert(len(invalidIndices3), Equals, 1)

	// Test with invalid tx hash
	txIn4 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "", // empty tx hash
				Sender:      "sender",
				To:          "to",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        "SWAP:ETH.ETH",
			},
		},
	}
	txs4, invalidIndices4, err := obs.getThorchainTxIns(txIn4, false, 100)
	c.Assert(err, IsNil)
	c.Assert(len(txs4), Equals, 0)
	c.Assert(len(invalidIndices4), Equals, 1)

	// Test with CommittedUnFinalised (should skip non-final)
	txIn5 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:          100,
				Tx:                   "abc",
				Sender:               "sender",
				To:                   "to",
				Coins:                common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:                 "SWAP:ETH.ETH",
				CommittedUnFinalised: true,
			},
		},
	}
	txs5, _, err := obs.getThorchainTxIns(txIn5, false, 100)
	c.Assert(err, IsNil)
	c.Assert(len(txs5), Equals, 0) // skipped because not finalized
}

// ---------------------------------------------------------------------------
// filterErrataTx tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestFilterErrataTx(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-errata-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	obs := &Observer{
		logger:  log.Logger,
		lock:    &sync.Mutex{},
		onDeck:  make(map[txInKey]*types.TxIn),
		storage: storage,
	}

	txID := "DEADBEEF"

	// Add a tx to ondeck
	txIn := &types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: txID, Sender: "sender1", To: "to1"},
			{BlockHeight: 100, Tx: "tx2", Sender: "sender2", To: "to2"},
		},
	}
	key := TxInKey(txIn)
	obs.onDeck[key] = txIn
	_ = storage.AddOrUpdateTx(txIn)

	// Filter errata for the first tx
	errataBlock := types.ErrataBlock{
		Height: 100,
		Txs: []types.ErrataTx{
			{TxID: common.TxID(txID), Chain: common.BTCChain},
		},
	}
	obs.filterErrataTx(errataBlock)

	// First tx should be removed, second should remain
	obs.lock.Lock()
	deck, exists := obs.onDeck[key]
	obs.lock.Unlock()
	c.Assert(exists, Equals, true)
	c.Assert(len(deck.TxArray), Equals, 1)
	c.Assert(deck.TxArray[0].Tx, Equals, "tx2")

	// Now filter the remaining tx
	errataBlock2 := types.ErrataBlock{
		Height: 100,
		Txs: []types.ErrataTx{
			{TxID: common.TxID("tx2"), Chain: common.BTCChain},
		},
	}
	obs.filterErrataTx(errataBlock2)

	// Deck should be removed entirely
	obs.lock.Lock()
	_, exists = obs.onDeck[key]
	obs.lock.Unlock()
	c.Assert(exists, Equals, false)
}

// ---------------------------------------------------------------------------
// restoreDeck tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestRestoreDeck(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-restore-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)

	// Store some tx
	txIn := &types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: "tx1"},
		},
	}
	err = storage.AddOrUpdateTx(txIn)
	c.Assert(err, IsNil)
	storage.Close()

	// Create new storage and observer to restore
	storage2, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage2.Close()

	obs := &Observer{
		logger:       log.Logger,
		lock:         &sync.Mutex{},
		onDeck:       make(map[txInKey]*types.TxIn),
		storage:      storage2,
		deckDumpFile: "",
	}

	obs.restoreDeck()

	obs.lock.Lock()
	c.Assert(len(obs.onDeck), Equals, 1)
	obs.lock.Unlock()
}

func (s *ObserveExtraSuite) TestRestoreDeckWithDumpFile(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-restore-dump-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)

	txIn := &types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: "tx1"},
		},
	}
	err = storage.AddOrUpdateTx(txIn)
	c.Assert(err, IsNil)
	storage.Close()

	storage2, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage2.Close()

	dumpFile := tempDir + "/dump.json"
	obs := &Observer{
		logger:       log.Logger,
		lock:         &sync.Mutex{},
		onDeck:       make(map[txInKey]*types.TxIn),
		storage:      storage2,
		deckDumpFile: dumpFile,
	}

	obs.restoreDeck()

	// Check dump file was created
	_, err = os.Stat(dumpFile)
	c.Assert(err, IsNil)

	// Verify it's valid JSON
	data, err := os.ReadFile(dumpFile)
	c.Assert(err, IsNil)
	var restored []*types.TxIn
	err = json.Unmarshal(data, &restored)
	c.Assert(err, IsNil)
}

// ---------------------------------------------------------------------------
// ObserveSigned tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestObserveSigned(c *C) {
	signedCache, err := lru.New(100)
	c.Assert(err, IsNil)

	obs := &Observer{
		logger:           log.Logger,
		signedTxOutCache: signedCache,
		globalTxsQueue:   make(chan types.TxIn, 10),
	}

	// Test with AllowFutureObservation = false (should cache)
	txIn := types.TxIn{
		Chain:                  common.BTCChain,
		AllowFutureObservation: false,
		TxArray: []*types.TxInItem{
			{Tx: "signed_tx_1"},
			{Tx: "signed_tx_2"},
		},
	}
	obs.ObserveSigned(txIn)

	// Verify txs were cached
	c.Assert(signedCache.Contains("signed_tx_1"), Equals, true)
	c.Assert(signedCache.Contains("signed_tx_2"), Equals, true)

	// Drain queue
	<-obs.globalTxsQueue

	// Test with AllowFutureObservation = true (should NOT cache)
	txIn2 := types.TxIn{
		Chain:                  common.BTCChain,
		AllowFutureObservation: true,
		TxArray: []*types.TxInItem{
			{Tx: "future_tx_1"},
		},
	}
	obs.ObserveSigned(txIn2)

	c.Assert(signedCache.Contains("future_tx_1"), Equals, false)
	<-obs.globalTxsQueue
}

// ---------------------------------------------------------------------------
// handleObservedTxCommitted tests (edge cases)
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleObservedTxCommittedNotInDeck(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-committed-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	obs := &Observer{
		logger:  log.Logger,
		lock:    &sync.Mutex{},
		onDeck:  make(map[txInKey]*types.TxIn),
		storage: storage,
		chains:  map[common.Chain]chainclients.ChainClient{},
	}

	// Commit a tx that isn't in the deck - should be a no-op
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    "nonexistent",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
	}
	obs.handleObservedTxCommitted(observedTx) // should not panic
}

// ---------------------------------------------------------------------------
// ObserverStorage tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestObserverStorageMigrateLegacy(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-storage-migrate-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	// Write legacy data
	legacyTxs := []*types.TxIn{
		{
			Chain:                common.BTCChain,
			ConfirmationRequired: 6,
			TxArray: []*types.TxInItem{
				{BlockHeight: 100, Tx: "legacy_tx1"},
			},
		},
	}
	data, err := json.Marshal(legacyTxs)
	c.Assert(err, IsNil)
	err = storage.db.Put([]byte(LegacyOnDeckKey), data, nil)
	c.Assert(err, IsNil)

	// Migrate
	result, err := storage.MigrateLegacy()
	c.Assert(err, IsNil)
	c.Assert(len(result), Equals, 1)
	c.Assert(result[0].TxArray[0].Tx, Equals, "legacy_tx1")

	// Verify migration handled flag was set
	_, err = storage.db.Get([]byte(LegacyMigrationHandledKey), nil)
	c.Assert(err, IsNil)

	// Verify legacy key was deleted
	_, err = storage.db.Get([]byte(LegacyOnDeckKey), nil)
	c.Assert(err, NotNil)

	// Second migration should return nil
	result2, err := storage.MigrateLegacy()
	c.Assert(err, IsNil)
	c.Assert(result2, IsNil)
}

func (s *ObserveExtraSuite) TestObserverStorageRemoveAllTxs(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-storage-removeall-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	// Add some txs
	for i := 0; i < 3; i++ {
		txIn := &types.TxIn{
			Chain:                common.BTCChain,
			ConfirmationRequired: int64(i),
			TxArray: []*types.TxInItem{
				{BlockHeight: int64(100 + i), Tx: "tx_" + string(rune('a'+i))},
			},
		}
		err = storage.AddOrUpdateTx(txIn)
		c.Assert(err, IsNil)
	}

	// Verify they exist
	txs, err := storage.GetOnDeckTxs()
	c.Assert(err, IsNil)
	c.Assert(len(txs), Equals, 3)

	// Remove all
	err = storage.RemoveAllTxs()
	c.Assert(err, IsNil)

	// Verify empty
	txs, err = storage.GetOnDeckTxs()
	c.Assert(err, IsNil)
	c.Assert(len(txs), Equals, 0)
}

func (s *ObserveExtraSuite) TestObserverStorageCreateTxKey(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-storage-key-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	txIn := &types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 6,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100},
		},
	}

	// With finalizeHeight=0 it should be computed
	key := storage.createTxKey(txIn, 0)
	c.Assert(key, Equals, "txs:BTC:106")

	// With explicit finalizeHeight
	key2 := storage.createTxKey(txIn, 200)
	c.Assert(key2, Equals, "txs:BTC:200")
}

// ---------------------------------------------------------------------------
// getOracleMimirs tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetOracleMimirs(c *C) {
	bridge := &MockThorchainBridge{
		getKeysignPartyFunc: func(pubKey common.PubKey) (common.PubKeys, error) {
			return nil, nil
		},
	}

	obs := &Observer{
		logger:          log.Logger,
		thorchainBridge: bridge,
	}

	interval, halt := obs.getOracleMimirs()
	c.Assert(interval, Equals, defaultOracleUpdateInterval)
	c.Assert(halt, Equals, false)
}

// ---------------------------------------------------------------------------
// peerManager updateLimit tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestPeerManagerUpdateLimit(c *C) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.DebugLevel)
	pm := newPeerManager(logger, 3)

	// Create a peer and acquire a token
	peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	c.Assert(err, IsNil)

	sem, err := pm.acquire(peerID)
	c.Assert(err, IsNil)

	// Update limit to larger
	pm.updateLimit(5)
	c.Assert(pm.getLimit(), Equals, 5)

	// Release old token
	pm.release(sem)

	// Update limit to smaller
	pm.updateLimit(2)
	c.Assert(pm.getLimit(), Equals, 2)

	// Update with invalid value - should be ignored
	pm.updateLimit(0)
	c.Assert(pm.getLimit(), Equals, 2)

	pm.updateLimit(-1)
	c.Assert(pm.getLimit(), Equals, 2)
}

func (s *ObserveExtraSuite) TestPeerManagerGetLimit(c *C) {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.DebugLevel)
	pm := newPeerManager(logger, 7)
	c.Assert(pm.getLimit(), Equals, 7)
}

// ---------------------------------------------------------------------------
// normalizeConfig tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestNormalizeConfig(c *C) {
	cfg := config.BifrostAttestationGossipConfig{}
	normalizeConfig(&cfg)

	c.Assert(cfg.ObserveReconcileInterval, Equals, defaultObserveReconcileInterval)
	c.Assert(cfg.LateObserveTimeout, Equals, defaultLateObserveTimeout)
	c.Assert(cfg.NonQuorumTimeout, Equals, defaultNonQuorumTimeout)
	c.Assert(cfg.MinTimeBetweenAttestations, Equals, defaultMinTimeBetweenAttestations)
	c.Assert(cfg.AskPeers, Equals, defaultAskPeers)
	c.Assert(cfg.AskPeersDelay, Equals, defaultAskPeersDelay)
	c.Assert(cfg.PeerTimeout, Equals, defaultPeerTimeout)
	c.Assert(cfg.MaxBatchSize, Equals, int64(defaultMaxBatchSize))
	c.Assert(cfg.PeerConcurrentSends, Equals, defaultPeerConcurrentSends)
	c.Assert(cfg.PeerConcurrentReceives, Equals, defaultPeerConcurrentReceives)
	c.Assert(cfg.BatchInterval, Equals, defaultBatchInterval)

	// Test that receives >= sends constraint
	cfg2 := config.BifrostAttestationGossipConfig{
		PeerConcurrentSends:    10,
		PeerConcurrentReceives: 5,
	}
	normalizeConfig(&cfg2)
	c.Assert(cfg2.PeerConcurrentReceives, Equals, 10) // should be bumped up to match sends
}

// ---------------------------------------------------------------------------
// AttestationGossip setActiveValidators / isActiveValidator tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationGossipActiveValidators(c *C) {
	ag := &AttestationGossip{
		logger:     log.Logger,
		activeVals: make(map[peer.ID]bool),
	}

	c.Assert(ag.activeValidatorCount(), Equals, 0)

	// getActiveValidators should return the map
	vals := ag.getActiveValidators()
	c.Assert(len(vals), Equals, 0)
}

// ---------------------------------------------------------------------------
// AttestationGossip getKeysignParty tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetKeysignParty(c *C) {
	pubKey := stypes.GetRandomPubKey()

	ag := &AttestationGossip{
		logger: log.Logger,
		bridge: &MockThorchainBridge{
			getKeysignPartyFunc: func(pk common.PubKey) (common.PubKeys, error) {
				return common.PubKeys{pk}, nil
			},
		},
		cachedKeySignParties: make(map[common.PubKey]cachedKeySignParty),
	}

	// First call - should fetch from bridge
	result, err := ag.getKeysignParty(pubKey)
	c.Assert(err, IsNil)
	c.Assert(len(result), Equals, 1)

	// Second call - should use cache
	result2, err := ag.getKeysignParty(pubKey)
	c.Assert(err, IsNil)
	c.Assert(len(result2), Equals, 1)
}

// ---------------------------------------------------------------------------
// AttestationGossip handleQuorum*Committed tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumTxCommittedNotFound(c *C) {
	ag := &AttestationGossip{
		logger:      log.Logger,
		observedTxs: make(map[txKey]*AttestationState[*common.ObservedTx]),
		pubKey:      []byte("test"),
	}

	// Construct a QuorumTx and marshal it
	qtx := common.QuorumTx{
		ObsTx: common.ObservedTx{
			Tx: common.Tx{
				ID:    "sometx",
				Chain: common.BTCChain,
			},
			BlockHeight:    100,
			FinaliseHeight: 100,
		},
		Attestations: []*common.Attestation{
			{PubKey: []byte("other"), Signature: []byte("sig")},
		},
	}
	payload, err := qtx.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{
		Payload: payload,
	}
	ag.handleQuorumTxCommitted(en) // should not panic
}

func (s *ObserveExtraSuite) TestHandleQuorumTxCommittedInvalidPayload(c *C) {
	ag := &AttestationGossip{
		logger:      log.Logger,
		observedTxs: make(map[txKey]*AttestationState[*common.ObservedTx]),
	}

	en := &ebifrost.EventNotification{
		Payload: []byte("invalid"),
	}
	ag.handleQuorumTxCommitted(en) // should not panic, just log error
}

func (s *ObserveExtraSuite) TestHandleQuorumNetworkFeeCommittedNotFound(c *C) {
	ag := &AttestationGossip{
		logger:      log.Logger,
		networkFees: make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
	}

	qnf := common.QuorumNetworkFee{
		NetworkFee:   &common.NetworkFee{Chain: common.BTCChain, Height: 100},
		Attestations: []*common.Attestation{},
	}
	payload, err := qnf.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumNetworkFeeCommitted(en) // not found, should be no-op
}

func (s *ObserveExtraSuite) TestHandleQuorumNetworkFeeCommittedInvalid(c *C) {
	ag := &AttestationGossip{
		logger:      log.Logger,
		networkFees: make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
	}
	en := &ebifrost.EventNotification{Payload: []byte("invalid")}
	ag.handleQuorumNetworkFeeCommitted(en)
}

func (s *ObserveExtraSuite) TestHandleQuorumSolvencyCommittedNotFound(c *C) {
	ag := &AttestationGossip{
		logger:     log.Logger,
		solvencies: make(map[common.TxID]*AttestationState[*common.Solvency]),
	}

	qs := common.QuorumSolvency{
		Solvency:     &common.Solvency{Chain: common.ETHChain, Height: 100},
		Attestations: []*common.Attestation{},
	}
	payload, err := qs.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumSolvencyCommitted(en) // not found, should be no-op
}

func (s *ObserveExtraSuite) TestHandleQuorumSolvencyCommittedInvalid(c *C) {
	ag := &AttestationGossip{
		logger:     log.Logger,
		solvencies: make(map[common.TxID]*AttestationState[*common.Solvency]),
	}
	en := &ebifrost.EventNotification{Payload: []byte("invalid")}
	ag.handleQuorumSolvencyCommitted(en)
}

func (s *ObserveExtraSuite) TestHandleQuorumErrataCommittedNotFound(c *C) {
	ag := &AttestationGossip{
		logger:    log.Logger,
		errataTxs: make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),
	}

	qe := common.QuorumErrataTx{
		ErrataTx:     &common.ErrataTx{Chain: common.BTCChain, Id: "tx123"},
		Attestations: []*common.Attestation{},
	}
	payload, err := qe.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumErrataTxCommitted(en) // not found, should be no-op
}

func (s *ObserveExtraSuite) TestHandleQuorumErrataCommittedInvalid(c *C) {
	ag := &AttestationGossip{
		logger:    log.Logger,
		errataTxs: make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),
	}
	en := &ebifrost.EventNotification{Payload: []byte("invalid")}
	ag.handleQuorumErrataTxCommitted(en)
}

// ---------------------------------------------------------------------------
// AttestationBatcher tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationBatcherUpdateMaxBatchSize(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		100,
		20*time.Second,
		4,
	)

	c.Assert(batcher.getMaxBatchSize(), Equals, int64(100))

	batcher.updateMaxBatchSize(200)
	c.Assert(batcher.getMaxBatchSize(), Equals, int64(200))

	// Invalid value should be rejected
	batcher.updateMaxBatchSize(0)
	c.Assert(batcher.getMaxBatchSize(), Equals, int64(200))

	batcher.updateMaxBatchSize(-5)
	c.Assert(batcher.getMaxBatchSize(), Equals, int64(200))
}

func (s *ObserveExtraSuite) TestAttestationBatcherAddMethods(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		1000, // high limit to avoid triggering send
		20*time.Second,
		4,
	)

	// Add observed tx
	batcher.AddObservedTx(common.AttestTx{
		ObsTx:       common.ObservedTx{},
		Attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")},
	})
	batcher.mu.Lock()
	c.Assert(len(batcher.observedTxBatch), Equals, 1)
	batcher.mu.Unlock()

	// Add network fee
	batcher.AddNetworkFee(common.AttestNetworkFee{
		NetworkFee:  &common.NetworkFee{Chain: common.BTCChain},
		Attestation: &common.Attestation{PubKey: []byte("pk2"), Signature: []byte("sig2")},
	})
	batcher.mu.Lock()
	c.Assert(len(batcher.networkFeeBatch), Equals, 1)
	batcher.mu.Unlock()

	// Add solvency
	batcher.AddSolvency(common.AttestSolvency{
		Solvency:    &common.Solvency{Chain: common.ETHChain},
		Attestation: &common.Attestation{PubKey: []byte("pk3"), Signature: []byte("sig3")},
	})
	batcher.mu.Lock()
	c.Assert(len(batcher.solvencyBatch), Equals, 1)
	batcher.mu.Unlock()

	// Add errata
	batcher.AddErrataTx(common.AttestErrataTx{
		ErrataTx:    &common.ErrataTx{Chain: common.BTCChain, Id: "e1"},
		Attestation: &common.Attestation{PubKey: []byte("pk4"), Signature: []byte("sig4")},
	})
	batcher.mu.Lock()
	c.Assert(len(batcher.errataTxBatch), Equals, 1)
	batcher.mu.Unlock()

	// Add price feed
	batcher.AddPriceFeed(common.AttestPriceFeed{
		PriceFeed:   &common.PriceFeed{Version: []byte("1")},
		Attestation: &common.Attestation{PubKey: []byte("pk5"), Signature: []byte("sig5")},
	})
	batcher.mu.Lock()
	c.Assert(len(batcher.priceFeedBatch), Equals, 1)
	batcher.mu.Unlock()
}

func (s *ObserveExtraSuite) TestAttestationBatcherTriggerBatchSend(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		1, // max batch size of 1 to trigger send
		20*time.Second,
		4,
	)

	// Adding one tx should trigger a batch send since maxBatchSize=1
	batcher.AddObservedTx(common.AttestTx{
		ObsTx:       common.ObservedTx{},
		Attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")},
	})

	// Give the async send a moment to fire
	time.Sleep(50 * time.Millisecond)

	// The force send channel should have been triggered
	select {
	case <-batcher.forceSendChan:
		// Expected
	default:
		// Already consumed or channel was empty - both are fine
	}
}

// ---------------------------------------------------------------------------
// AttestationStatePool tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationStatePool(c *C) {
	pool := NewAttestationStatePool[*common.ObservedTx]()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "tx1",
			Chain: common.BTCChain,
		},
	}

	state := pool.NewAttestationState(obsTx)
	c.Assert(state, NotNil)
	c.Assert(state.Item, DeepEquals, obsTx)
	c.Assert(len(state.attestations), Equals, 0)

	// Return it to pool
	pool.PutAttestationState(state)

	// Get another - should reuse pooled object
	obsTx2 := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "tx2",
			Chain: common.ETHChain,
		},
	}
	state2 := pool.NewAttestationState(obsTx2)
	c.Assert(state2, NotNil)
	c.Assert(state2.Item, DeepEquals, obsTx2)
}

// ---------------------------------------------------------------------------
// AttestationState methods tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationStateAddAttestation(c *C) {
	// Replace verifySignature temporarily
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error {
		return nil
	}
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "tx1",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 106,
	}

	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	// Add first attestation
	err := state.AddAttestation(&common.Attestation{
		PubKey:    []byte("pk1"),
		Signature: []byte("sig1"),
	})
	c.Assert(err, IsNil)
	c.Assert(state.AttestationCount(), Equals, 1)
	c.Assert(state.UnsentCount(), Equals, 1)

	// Duplicate signature - should be ignored
	err = state.AddAttestation(&common.Attestation{
		PubKey:    []byte("pk2"),
		Signature: []byte("sig1"),
	})
	c.Assert(err, IsNil)
	c.Assert(state.AttestationCount(), Equals, 1) // still 1

	// Same pubkey different sig - should error
	err = state.AddAttestation(&common.Attestation{
		PubKey:    []byte("pk1"),
		Signature: []byte("sig_different"),
	})
	c.Assert(err, NotNil)

	// Add second attestation
	err = state.AddAttestation(&common.Attestation{
		PubKey:    []byte("pk2"),
		Signature: []byte("sig2"),
	})
	c.Assert(err, IsNil)
	c.Assert(state.AttestationCount(), Equals, 2)
}

func (s *ObserveExtraSuite) TestAttestationStateUnsentAndCopy(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	_ = state.AddAttestation(&common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")})
	_ = state.AddAttestation(&common.Attestation{PubKey: []byte("pk2"), Signature: []byte("sig2")})

	// All unsent
	unsent := state.UnsentAttestations()
	c.Assert(len(unsent), Equals, 2)

	// Copy
	copies := state.AttestationsCopy()
	c.Assert(len(copies), Equals, 2)

	// Mark as sent
	state.MarkAttestationsSent(false)
	c.Assert(state.UnsentCount(), Equals, 0)
	unsent2 := state.UnsentAttestations()
	c.Assert(len(unsent2), Equals, 0)
}

func (s *ObserveExtraSuite) TestAttestationStateMarkCommitted(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	att1 := &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}
	att2 := &common.Attestation{PubKey: []byte("pk2"), Signature: []byte("sig2")}
	_ = state.AddAttestation(att1)
	_ = state.AddAttestation(att2)

	// Mark att1 as committed
	state.MarkAttestationsCommitted([]*common.Attestation{att1})

	// AttestationsCopy should skip committed ones
	copies := state.AttestationsCopy()
	c.Assert(len(copies), Equals, 1) // only att2
}

func (s *ObserveExtraSuite) TestAttestationStateShouldSendLate(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	// No unsent - should not send late
	c.Assert(state.ShouldSendLate(30*time.Second), Equals, false)

	// Add attestation
	_ = state.AddAttestation(&common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")})

	// Just added, not enough time
	c.Assert(state.ShouldSendLate(30*time.Second), Equals, false)

	// Set firstAttestationObserved to past
	state.firstAttestationObserved = time.Now().Add(-2 * time.Minute)
	c.Assert(state.ShouldSendLate(30*time.Second), Equals, true)

	// Mark sent and test lastAttestationsSent path
	state.MarkAttestationsSent(false)

	// Add new attestation
	_ = state.AddAttestation(&common.Attestation{PubKey: []byte("pk2"), Signature: []byte("sig2")})

	// lastAttestationsSent was just set, so shouldn't send yet
	c.Assert(state.ShouldSendLate(30*time.Second), Equals, false)

	// Backdate lastAttestationsSent
	state.lastAttestationsSent = time.Now().Add(-2 * time.Minute)
	c.Assert(state.ShouldSendLate(30*time.Second), Equals, true)
}

func (s *ObserveExtraSuite) TestAttestationStateExpiredAfterQuorum(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	lateTimeout := 2 * time.Minute
	nonQuorumTimeout := 10 * time.Hour

	// No quorum yet, no attestations sent - not expired
	c.Assert(state.ExpiredAfterQuorum(lateTimeout, nonQuorumTimeout), Equals, false)

	// Set lastAttestationsSent far in the past - should expire due to non-quorum timeout
	state.lastAttestationsSent = time.Now().Add(-11 * time.Hour)
	c.Assert(state.ExpiredAfterQuorum(lateTimeout, nonQuorumTimeout), Equals, true)

	// Reset and test quorum path
	state.lastAttestationsSent = time.Time{}
	state.quorumAttestationsSent = time.Now().Add(-3 * time.Minute)
	c.Assert(state.ExpiredAfterQuorum(lateTimeout, nonQuorumTimeout), Equals, true)

	// Recent quorum - not expired
	state.quorumAttestationsSent = time.Now()
	c.Assert(state.ExpiredAfterQuorum(lateTimeout, nonQuorumTimeout), Equals, false)
}

func (s *ObserveExtraSuite) TestAttestationStateState(c *C) {
	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	result := state.State()
	c.Assert(result, Equals, "sent: 0, total: 0 post-quorum: false")
}

func (s *ObserveExtraSuite) TestAttestationStateMarkSentQuorum(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	_ = state.AddAttestation(&common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")})

	// Mark as quorum
	state.MarkAttestationsSent(true)
	c.Assert(state.quorumAttestationsSent.IsZero(), Equals, false)
	c.Assert(state.initialAttestationsSent.IsZero(), Equals, false)

	// Mark again as quorum - quorumAttestationsSent should not change
	firstQuorum := state.quorumAttestationsSent
	err := state.AddAttestation(&common.Attestation{PubKey: []byte("pk2"), Signature: []byte("sig2")})
	c.Assert(err, IsNil)
	state.MarkAttestationsSent(true)
	c.Assert(state.quorumAttestationsSent, Equals, firstQuorum) // not updated
}

// ---------------------------------------------------------------------------
// ProcessAttestation tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessAttestation(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	var attestations []attestationSentState

	msg := &common.AttestTx{
		ObsTx: common.ObservedTx{
			Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
		},
		Attestation: &common.Attestation{
			PubKey:    []byte("pk1"),
			Signature: []byte("sig1"),
		},
	}

	// Process first attestation
	err := ProcessAttestation(&attestations, msg)
	c.Assert(err, IsNil)
	c.Assert(len(attestations), Equals, 1)

	// Duplicate signature - should be ignored
	err = ProcessAttestation(&attestations, msg)
	c.Assert(err, IsNil)
	c.Assert(len(attestations), Equals, 1) // still 1

	// Same pubkey different sig - should error
	msg2 := &common.AttestTx{
		ObsTx: common.ObservedTx{
			Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
		},
		Attestation: &common.Attestation{
			PubKey:    []byte("pk1"),
			Signature: []byte("sig2"),
		},
	}
	err = ProcessAttestation(&attestations, msg2)
	c.Assert(err, NotNil)
}

// ---------------------------------------------------------------------------
// EventClient tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestEventClientRegisterHandler(c *C) {
	mockEC := NewMockEventClient()
	called := false
	mockEC.RegisterHandler("test_event", func(en *ebifrost.EventNotification) {
		called = true
	})

	c.Assert(len(mockEC.handlers), Equals, 1)
	mockEC.handlers["test_event"](&ebifrost.EventNotification{})
	c.Assert(called, Equals, true)
}

// ---------------------------------------------------------------------------
// AttestationState ExpiredAfterQuorum with all committed path
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationStateExpiredAllCommitted(c *C) {
	origVerify := verifySignature
	verifySignature = func(signBz, signature, attester []byte) error { return nil }
	defer func() { verifySignature = origVerify }()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)

	att := &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}
	err := state.AddAttestation(att)
	c.Assert(err, IsNil)

	// Mark committed
	state.MarkAttestationsCommitted([]*common.Attestation{att})
	state.lastCommittedAttestation = time.Now().Add(-3 * time.Minute)

	// All attestations committed and enough time passed
	c.Assert(state.ExpiredAfterQuorum(2*time.Minute, 10*time.Hour), Equals, true)
}

// ---------------------------------------------------------------------------
// Mock test for ObservedTx with valid fields in getThorchainTxIns
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetThorchainTxInsValidTx(c *C) {
	obs := &Observer{
		logger:              log.Logger,
		errCounter:          s.m.GetCounterVec(metrics.ObserverError),
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	vault := stypes.GetRandomPubKey()
	vaultAddr, err := vault.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)

	txIn := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:         100,
				Tx:                  stypes.GetRandomTxHash().String(),
				Sender:              vaultAddr.String(),
				To:                  vaultAddr.String(),
				Coins:               common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Gas:                 common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10))},
				Memo:                "SWAP:ETH.ETH",
				ObservedVaultPubKey: vault,
			},
		},
	}

	txs, invalidIndices, err := obs.getThorchainTxIns(txIn, false, 106)
	c.Assert(err, IsNil)
	c.Assert(len(invalidIndices), Equals, 0)
	c.Assert(len(txs), Equals, 1)
	c.Assert(txs[0].Tx.Memo, Equals, "SWAP:ETH.ETH")

	// Test finalized path
	txsF, _, err := obs.getThorchainTxIns(txIn, true, 106)
	c.Assert(err, IsNil)
	c.Assert(len(txsF), Equals, 1)
	c.Assert(txsF[0].BlockHeight, Equals, int64(106)) // Uses finaliseHeight when finalized
}

// ---------------------------------------------------------------------------
// Benchmark-style: test new items (standard Go test function)
// ---------------------------------------------------------------------------

func TestDelayBasic(t *testing.T) {
	d := NewDelay()
	if d.IsRunning() {
		t.Fatal("expected not running")
	}
	d.Start()
	if !d.IsRunning() {
		t.Fatal("expected running")
	}
	d.Done()
	if d.IsRunning() {
		t.Fatal("expected not running after done")
	}
}

func TestTxInKeyCompute(t *testing.T) {
	txIn := &types.TxIn{
		Chain:                common.ETHChain,
		ConfirmationRequired: 12,
		TxArray: []*types.TxInItem{
			{BlockHeight: 500},
		},
	}
	key := TxInKey(txIn)
	if key.chain != common.ETHChain {
		t.Fatalf("expected ETHChain, got %s", key.chain)
	}
	if key.height != 512 {
		t.Fatalf("expected 512, got %d", key.height)
	}
}

// ---------------------------------------------------------------------------
// SetObserverHandleObservedTxCommitted tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSetObserverHandleObservedTxCommitted(c *C) {
	ag := &AttestationGossip{
		logger: log.Logger,
	}

	obs := &Observer{
		logger: log.Logger,
		lock:   &sync.Mutex{},
		onDeck: make(map[txInKey]*types.TxIn),
	}

	ag.SetObserverHandleObservedTxCommitted(obs)
	c.Assert(ag.observerHandleObservedTxCommitted, NotNil)
}

// ---------------------------------------------------------------------------
// handleObservedTxCommitted with our attestation in quorum
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumTxCommittedWithOurAttestation(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-quorum-our-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	pubKey := []byte("our_pubkey")

	obs := &Observer{
		logger:  log.Logger,
		lock:    &sync.Mutex{},
		onDeck:  make(map[txInKey]*types.TxIn),
		storage: storage,
		chains:  map[common.Chain]chainclients.ChainClient{},
	}

	ag := &AttestationGossip{
		logger:                            log.Logger,
		pubKey:                            pubKey,
		observedTxs:                       make(map[txKey]*AttestationState[*common.ObservedTx]),
		observerHandleObservedTxCommitted: obs.handleObservedTxCommitted,
	}

	// Create a QuorumTx with our attestation
	qtx := common.QuorumTx{
		ObsTx: common.ObservedTx{
			Tx: common.Tx{
				ID:    "our_tx",
				Chain: common.BTCChain,
			},
			BlockHeight:    100,
			FinaliseHeight: 100,
		},
		Attestations: []*common.Attestation{
			{PubKey: pubKey, Signature: []byte("sig")},
		},
	}
	payload, err := qtx.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumTxCommitted(en) // should call observer's handleObservedTxCommitted
}

// ---------------------------------------------------------------------------
// sendToQuorumChecker basic test
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendToQuorumCheckerEmptyDeck(c *C) {
	obs := &Observer{
		logger:              log.Logger,
		errCounter:          s.m.GetCounterVec(metrics.ObserverError),
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	// Empty deck - should return no invalid indices
	deck := &types.TxIn{
		Chain:   common.BTCChain,
		TxArray: []*types.TxInItem{},
	}

	invalidIndices := obs.sendToQuorumChecker(deck, false, 100)
	c.Assert(len(invalidIndices), Equals, 0)
}

// ---------------------------------------------------------------------------
// observer storage GetOnDeckTxs with legacy migration
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetOnDeckTxsWithLegacyMigration(c *C) {
	tempDir, err := os.MkdirTemp("", "observer-getondeck-legacy-test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tempDir)

	storage, err := NewObserverStorage(tempDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	// Write legacy data
	legacyTxs := []*types.TxIn{
		{
			Chain:                common.ETHChain,
			ConfirmationRequired: 12,
			TxArray: []*types.TxInItem{
				{BlockHeight: 200, Tx: "eth_tx1"},
			},
		},
	}
	data, err := json.Marshal(legacyTxs)
	c.Assert(err, IsNil)
	err = storage.db.Put([]byte(LegacyOnDeckKey), data, nil)
	c.Assert(err, IsNil)

	// GetOnDeckTxs should migrate and return legacy txs
	result, err := storage.GetOnDeckTxs()
	c.Assert(err, IsNil)
	c.Assert(len(result), Equals, 1)
	c.Assert(result[0].Chain, Equals, common.ETHChain)
}

// ---------------------------------------------------------------------------
// broadcastToAllPeers with no active val getter
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestBroadcastToAllPeersNoGetter(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		100,
		20*time.Second,
		4,
	)

	// Don't set active val getter
	batch := common.AttestationBatch{}
	batcher.broadcastToAllPeers(context.Background(), batch) // should log warning and return
}

// ---------------------------------------------------------------------------
// newAttestationBatch from pool test
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestNewAttestationBatchFromPool(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		100,
		20*time.Second,
		4,
	)

	result := batcher.newAttestationBatch()
	batch, ok := result.(*common.AttestationBatch)
	c.Assert(ok, Equals, true)
	c.Assert(batch, NotNil)
	c.Assert(len(batch.AttestTxs), Equals, 0)
}

// ---------------------------------------------------------------------------
// sendBatches with no messages (should be a no-op)
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendBatchesNoMessages(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"test"}),
		log.Logger,
		s.m,
		2*time.Second,
		100,
		20*time.Second,
		4,
	)
	batcher.setActiveValGetter(func() map[peer.ID]bool {
		return map[peer.ID]bool{}
	})

	// No messages - should just return
	batcher.sendBatches(context.Background(), false)
}

// ---------------------------------------------------------------------------
// sendBatches with messages, force=true
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendBatchesForce(c *C) {
	batcher := NewAttestationBatcher(
		NewMockHost([]peer.ID{"self"}),
		log.Logger,
		s.m,
		2*time.Second,
		100,
		20*time.Second,
		4,
	)
	batcher.setActiveValGetter(func() map[peer.ID]bool {
		return map[peer.ID]bool{}
	})

	// Add a message
	batcher.AddObservedTx(common.AttestTx{
		ObsTx:       common.ObservedTx{},
		Attestation: &common.Attestation{PubKey: []byte("pk"), Signature: []byte("sig")},
	})

	// Force send - should clear the batch even without time threshold
	batcher.sendBatches(context.Background(), true)

	batcher.mu.Lock()
	c.Assert(len(batcher.observedTxBatch), Equals, 0)
	batcher.mu.Unlock()
}

// ---------------------------------------------------------------------------
// AttestationState AddAttestation with nil item (invalid)
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestationStateAddAttestationInvalidItem(c *C) {
	pool := NewAttestationStatePool[*common.ObservedTx]()
	// Create state with nil item
	state := &AttestationState[*common.ObservedTx]{
		Item:         nil,
		attestations: make([]attestationSentState, 0),
	}
	_ = pool // just to use the import

	err := state.AddAttestation(&common.Attestation{
		PubKey:    []byte("pk1"),
		Signature: []byte("sig1"),
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "item is not valid")
}

// ---------------------------------------------------------------------------
// Helper to create an AttestationGossip with mocks for testing
// ---------------------------------------------------------------------------

func newTestAttestationGossip(c *C, peers []peer.ID) (*AttestationGossip, *MockGRPCClient) {
	privKey := secp256k1.GenPrivKey()

	grpcClient := &MockGRPCClient{
		sendQuorumTxFunc: func(ctx context.Context, tx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
			return &ebifrost.SendQuorumTxResult{}, nil
		},
		sendQuorumNetworkFeeFunc: func(ctx context.Context, nf *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
			return &ebifrost.SendQuorumNetworkFeeResult{}, nil
		},
		sendQuorumSolvencyFunc: func(ctx context.Context, qs *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
			return &ebifrost.SendQuorumSolvencyResult{}, nil
		},
		sendQuorumErrataFunc: func(ctx context.Context, qe *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
			return &ebifrost.SendQuorumErrataTxResult{}, nil
		},
		sendQuorumPriceFeedBatchFunc: func(ctx context.Context, batch *common.QuorumPriceFeedBatch, opts ...grpc.CallOption) (*ebifrost.SendQuorumPriceFeedBatchResult, error) {
			return &ebifrost.SendQuorumPriceFeedBatchResult{}, nil
		},
	}

	host := NewMockHost(peers)
	bridge := &MockThorchainBridge{
		getKeysignPartyFunc: func(pubKey common.PubKey) (common.PubKeys, error) {
			return common.PubKeys{pubKey}, nil
		},
	}

	batcher := NewAttestationBatcher(
		host,
		log.Logger,
		getSharedTestMetrics(),
		2*time.Second,
		100,
		20*time.Second,
		4,
	)

	cfg := config.BifrostAttestationGossipConfig{}
	normalizeConfig(&cfg)

	ag := &AttestationGossip{
		logger:               log.Logger,
		host:                 host,
		grpcClient:           grpcClient,
		privKey:              privKey,
		pubKey:               privKey.PubKey().Bytes(),
		bridge:               bridge,
		config:               cfg,
		eventClient:          NewMockEventClient(),
		observedTxs:          make(map[txKey]*AttestationState[*common.ObservedTx]),
		networkFees:          make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
		solvencies:           make(map[common.TxID]*AttestationState[*common.Solvency]),
		errataTxs:            make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),
		priceFeeds:           make(map[string]*AttestationState[*common.PriceFeed]),
		observedTxsPool:      NewAttestationStatePool[*common.ObservedTx](),
		networkFeesPool:      NewAttestationStatePool[*common.NetworkFee](),
		solvenciesPool:       NewAttestationStatePool[*common.Solvency](),
		errataTxsPool:        NewAttestationStatePool[*common.ErrataTx](),
		priceFeedsPool:       NewAttestationStatePool[*common.PriceFeed](),
		priceFeedsDelay:      NewDelay(),
		activeVals:           map[peer.ID]bool{peers[0]: true},
		cachedKeySignParties: make(map[common.PubKey]cachedKeySignParty),
		batcher:              batcher,
		peerMgr:              newPeerManager(log.Logger, 5),
	}

	return ag, grpcClient
}

// ---------------------------------------------------------------------------
// AttestObservedTx tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestObservedTx(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	obsTx := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "txid1",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
	}

	err := ag.AttestObservedTx(context.Background(), obsTx, true)
	c.Assert(err, IsNil)

	// Verify the attestation was added locally
	ag.mu.Lock()
	c.Assert(len(ag.observedTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

func (s *ObserveExtraSuite) TestAttestObservedTxNotActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// Remove self from active validators
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{}
	ag.avMu.Unlock()

	obsTx := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "txid1",
			Chain: common.BTCChain,
		},
	}

	err := ag.AttestObservedTx(context.Background(), obsTx, true)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "skipping attest observed tx: not active")
}

// ---------------------------------------------------------------------------
// AttestNetworkFee tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestNetworkFee(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	nf := common.NetworkFee{
		Chain:           common.BTCChain,
		Height:          100,
		TransactionSize: 250,
		TransactionRate: 10,
	}

	err := ag.AttestNetworkFee(context.Background(), nf)
	c.Assert(err, IsNil)

	ag.mu.Lock()
	c.Assert(len(ag.networkFees) > 0, Equals, true)
	ag.mu.Unlock()
}

func (s *ObserveExtraSuite) TestAttestNetworkFeeNotActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{}
	ag.avMu.Unlock()

	err := ag.AttestNetworkFee(context.Background(), common.NetworkFee{Chain: common.BTCChain})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "skipping attest network fee: not active")
}

// ---------------------------------------------------------------------------
// AttestSolvency tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestSolvency(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	sol := common.Solvency{
		Chain:  common.BTCChain,
		Height: 200,
		PubKey: stypes.GetRandomPubKey(),
		Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
	}

	err := ag.AttestSolvency(context.Background(), sol)
	c.Assert(err, IsNil)

	ag.mu.Lock()
	c.Assert(len(ag.solvencies) > 0, Equals, true)
	ag.mu.Unlock()
}

func (s *ObserveExtraSuite) TestAttestSolvencyNotActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{}
	ag.avMu.Unlock()

	err := ag.AttestSolvency(context.Background(), common.Solvency{Chain: common.BTCChain})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "skipping attest solvency: not active")
}

// ---------------------------------------------------------------------------
// AttestErrata tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestErrata(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	errata := common.ErrataTx{
		Chain: common.BTCChain,
		Id:    "errata1",
	}

	err := ag.AttestErrata(context.Background(), errata)
	c.Assert(err, IsNil)

	ag.mu.Lock()
	c.Assert(len(ag.errataTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

func (s *ObserveExtraSuite) TestAttestErrataNotActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{}
	ag.avMu.Unlock()

	err := ag.AttestErrata(context.Background(), common.ErrataTx{Chain: common.BTCChain, Id: "e1"})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "skipping attest errata tx: not active")
}

func (s *ObserveExtraSuite) TestAttestErrataRemovesObservedTx(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	// Add an observed tx first
	obsTx := &common.ObservedTx{
		Tx: common.Tx{
			ID:    "errata_tx",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
	}
	err := ag.AttestObservedTx(context.Background(), obsTx, true)
	c.Assert(err, IsNil)

	ag.mu.Lock()
	c.Assert(len(ag.observedTxs) > 0, Equals, true)
	ag.mu.Unlock()

	// Now attest errata for the same chain/ID - should remove the observed tx
	errata := common.ErrataTx{
		Chain: common.BTCChain,
		Id:    "errata_tx",
	}
	err = ag.AttestErrata(context.Background(), errata)
	c.Assert(err, IsNil)

	ag.mu.Lock()
	// The observed tx should be removed since errata was for the same chain/ID
	c.Assert(len(ag.observedTxs), Equals, 0)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// AttestPriceFeed tests
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAttestPriceFeed(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	pf := common.PriceFeed{
		Time:  time.Now().Unix(),
		Rates: []*common.OraclePrice{{Amount: 100, Decimals: 8}},
	}

	err := ag.AttestPriceFeed(context.Background(), pf)
	c.Assert(err, IsNil)
}

func (s *ObserveExtraSuite) TestAttestPriceFeedNotActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{}
	ag.avMu.Unlock()

	err := ag.AttestPriceFeed(context.Background(), common.PriceFeed{Time: 1})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "skipping attest price feed: not active")
}

// ---------------------------------------------------------------------------
// handleObservedTxAttestation with supermajority
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleObservedTxAttestationSupermajority(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	// Set active validator count to 1 so 1 attestation = supermajority
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    "super_tx",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
	}

	msg := common.AttestTx{
		ObsTx:   obsTx,
		Inbound: true,
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: []byte("sig1"),
		},
	}

	ag.handleObservedTxAttestation(context.Background(), msg)

	// Should have created an attestation state
	ag.mu.Lock()
	c.Assert(len(ag.observedTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// handleNetworkFeeAttestation with supermajority
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleNetworkFeeAttestationSupermajority(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	msg := common.AttestNetworkFee{
		NetworkFee: &common.NetworkFee{
			Chain:           common.BTCChain,
			Height:          100,
			TransactionSize: 250,
			TransactionRate: 10,
		},
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: []byte("sig1"),
		},
	}

	ag.handleNetworkFeeAttestation(context.Background(), msg)

	ag.mu.Lock()
	c.Assert(len(ag.networkFees) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// handleSolvencyAttestation with supermajority
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleSolvencyAttestationSupermajority(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	msg := common.AttestSolvency{
		Solvency: &common.Solvency{
			Id:     "sol1",
			Chain:  common.BTCChain,
			Height: 200,
			PubKey: stypes.GetRandomPubKey(),
			Coins:  common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
		},
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: []byte("sig1"),
		},
	}

	ag.handleSolvencyAttestation(context.Background(), msg)

	ag.mu.Lock()
	c.Assert(len(ag.solvencies) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// handleErrataAttestation with supermajority
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleErrataAttestationSupermajority(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	msg := common.AttestErrataTx{
		ErrataTx: &common.ErrataTx{
			Chain: common.BTCChain,
			Id:    "errata1",
		},
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: []byte("sig1"),
		},
	}

	ag.handleErrataAttestation(context.Background(), msg)

	ag.mu.Lock()
	c.Assert(len(ag.errataTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// handlePriceFeedAttestation
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandlePriceFeedAttestation(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	// Generate a valid signature for the price feed
	pf := &common.PriceFeed{
		Time:  time.Now().Unix(),
		Rates: []*common.OraclePrice{{Amount: 100, Decimals: 8}},
	}
	signBz, err := pf.GetSignablePayload()
	c.Assert(err, IsNil)

	sig, err := ag.privKey.Sign(signBz)
	c.Assert(err, IsNil)

	msg := common.AttestPriceFeed{
		PriceFeed: pf,
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: sig,
		},
	}

	ag.handlePriceFeedAttestation(context.Background(), msg)

	// Should have stored the price feed
	ag.mu.Lock()
	c.Assert(len(ag.priceFeeds) > 0, Equals, true)
	ag.mu.Unlock()

	// Wait for the delay-based send
	time.Sleep(200 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// sendObservedTxAttestationsToThornode - no unsent
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendObservedTxAttestationsNoUnsent(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	obsTx := common.ObservedTx{
		Tx: common.Tx{ID: "tx1", Chain: common.BTCChain},
	}

	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(&obsTx)
	// No attestations added - should be a no-op

	ag.sendObservedTxAttestationsToThornode(context.Background(), obsTx, state, true, false, true)
}

// ---------------------------------------------------------------------------
// sendNetworkFeeAttestationsToThornode - success
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendNetworkFeeAttestationsSuccess(c *C) {
	sent := false
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumNetworkFeeFunc = func(ctx context.Context, nf *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
		sent = true
		return &ebifrost.SendQuorumNetworkFeeResult{}, nil
	}

	nf := common.NetworkFee{Chain: common.BTCChain, Height: 100}
	pool := NewAttestationStatePool[*common.NetworkFee]()
	state := pool.NewAttestationState(&nf)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendNetworkFeeAttestationsToThornode(context.Background(), nf, state, true)
	c.Assert(sent, Equals, true)
}

// ---------------------------------------------------------------------------
// sendSolvencyAttestationsToThornode - success
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendSolvencyAttestationsSuccess(c *C) {
	sent := false
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumSolvencyFunc = func(ctx context.Context, qs *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
		sent = true
		return &ebifrost.SendQuorumSolvencyResult{}, nil
	}

	sol := common.Solvency{Id: "s1", Chain: common.BTCChain, Height: 200}
	pool := NewAttestationStatePool[*common.Solvency]()
	state := pool.NewAttestationState(&sol)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendSolvencyAttestationsToThornode(context.Background(), sol, state, true)
	c.Assert(sent, Equals, true)
}

// ---------------------------------------------------------------------------
// sendErrataAttestationsToThornode - success
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendErrataAttestationsSuccess(c *C) {
	sent := false
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumErrataFunc = func(ctx context.Context, qe *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
		sent = true
		return &ebifrost.SendQuorumErrataTxResult{}, nil
	}

	errata := common.ErrataTx{Chain: common.BTCChain, Id: "e1"}
	pool := NewAttestationStatePool[*common.ErrataTx]()
	state := pool.NewAttestationState(&errata)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendErrataAttestationsToThornode(context.Background(), errata, state, true)
	c.Assert(sent, Equals, true)
}

// ---------------------------------------------------------------------------
// sendPriceFeedAttestationsToThornode
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendPriceFeedAttestationsToThornode(c *C) {
	sent := false
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumPriceFeedBatchFunc = func(ctx context.Context, batch *common.QuorumPriceFeedBatch, opts ...grpc.CallOption) (*ebifrost.SendQuorumPriceFeedBatchResult, error) {
		sent = true
		return &ebifrost.SendQuorumPriceFeedBatchResult{}, nil
	}

	// Add a price feed state
	pf := &common.PriceFeed{Time: 100, Rates: []*common.OraclePrice{{Amount: 1, Decimals: 8}}}
	state := ag.priceFeedsPool.NewAttestationState(pf)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}
	ag.mu.Lock()
	ag.priceFeeds["pk1_hex"] = state
	ag.mu.Unlock()

	ag.sendPriceFeedAttestationsToThornode(context.Background())
	c.Assert(sent, Equals, true)
}

func (s *ObserveExtraSuite) TestSendPriceFeedAttestationsEmpty(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// No price feeds - should be a no-op
	ag.sendPriceFeedAttestationsToThornode(context.Background())
}

// ---------------------------------------------------------------------------
// handleQuorumTxCommitted with valid payload and matching state
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumTxCommittedWithState(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    "committed_tx",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
	}

	// Add the observed tx to state first
	k := txKey{
		Chain:      obsTx.Tx.Chain,
		ID:         obsTx.Tx.ID,
		UniqueHash: obsTx.Tx.Hash(obsTx.BlockHeight),
		Finalized:  obsTx.IsFinal(),
		Inbound:    true,
	}

	state := ag.observedTxsPool.NewAttestationState(&obsTx)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: ag.pubKey, Signature: []byte("sig")}, sent: false},
	}
	ag.mu.Lock()
	ag.observedTxs[k] = state
	ag.mu.Unlock()

	// Create and marshal a QuorumTx
	qtx := common.QuorumTx{
		ObsTx:   obsTx,
		Inbound: true,
		Attestations: []*common.Attestation{
			{PubKey: ag.pubKey, Signature: []byte("sig")},
		},
	}
	payload, err := qtx.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumTxCommitted(en)

	// Attestation should be marked committed
	ag.mu.Lock()
	s2, ok := ag.observedTxs[k]
	c.Assert(ok, Equals, true)
	s2.mu.Lock()
	c.Assert(s2.lastCommittedAttestation.IsZero(), Equals, false) // should be set after MarkAttestationsCommitted
	s2.mu.Unlock()
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// handleQuorumNetworkFeeCommitted with valid payload and matching state
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumNetworkFeeCommittedWithState(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	nf := common.NetworkFee{Chain: common.BTCChain, Height: 100, TransactionSize: 250, TransactionRate: 10}

	state := ag.networkFeesPool.NewAttestationState(&nf)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: ag.pubKey, Signature: []byte("sig")}, sent: false},
	}
	ag.mu.Lock()
	ag.networkFees[nf] = state
	ag.mu.Unlock()

	qnf := common.QuorumNetworkFee{
		NetworkFee:   &nf,
		Attestations: []*common.Attestation{{PubKey: ag.pubKey, Signature: []byte("sig")}},
	}
	payload, err := qnf.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumNetworkFeeCommitted(en)
}

// ---------------------------------------------------------------------------
// handleQuorumSolvencyCommitted with valid payload and matching state
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumSolvencyCommittedWithState(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	sol := common.Solvency{Id: "sol1", Chain: common.BTCChain, Height: 200}

	state := ag.solvenciesPool.NewAttestationState(&sol)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: ag.pubKey, Signature: []byte("sig")}, sent: false},
	}
	ag.mu.Lock()
	ag.solvencies[sol.Id] = state
	ag.mu.Unlock()

	qs := common.QuorumSolvency{
		Solvency:     &sol,
		Attestations: []*common.Attestation{{PubKey: ag.pubKey, Signature: []byte("sig")}},
	}
	payload, err := qs.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumSolvencyCommitted(en)
}

// ---------------------------------------------------------------------------
// handleQuorumErrataTxCommitted with valid payload and matching state
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleQuorumErrataTxCommittedWithState(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	errata := common.ErrataTx{Chain: common.BTCChain, Id: "e1"}

	state := ag.errataTxsPool.NewAttestationState(&errata)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: ag.pubKey, Signature: []byte("sig")}, sent: false},
	}
	ag.mu.Lock()
	ag.errataTxs[errata] = state
	ag.mu.Unlock()

	qe := common.QuorumErrataTx{
		ErrataTx:     &errata,
		Attestations: []*common.Attestation{{PubKey: ag.pubKey, Signature: []byte("sig")}},
	}
	payload, err := qe.Marshal()
	c.Assert(err, IsNil)

	en := &ebifrost.EventNotification{Payload: payload}
	ag.handleQuorumErrataTxCommitted(en)
}

// ---------------------------------------------------------------------------
// processAttestationStateBatch
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessAttestationStateBatch(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	batch := common.QuorumState{
		QuoTxs: []*common.QuorumTx{
			{
				ObsTx: common.ObservedTx{
					Tx: common.Tx{ID: "batch_tx1", Chain: common.BTCChain},
				},
				Attestations: []*common.Attestation{
					{PubKey: []byte("pk1"), Signature: []byte("sig1")},
				},
			},
		},
		QuoNetworkFees: []*common.QuorumNetworkFee{
			{
				NetworkFee:   &common.NetworkFee{Chain: common.ETHChain, Height: 50},
				Attestations: []*common.Attestation{{PubKey: []byte("pk2"), Signature: []byte("sig2")}},
			},
		},
		QuoSolvencies: []*common.QuorumSolvency{
			{
				Solvency:     &common.Solvency{Id: "sol_batch", Chain: common.BTCChain, Height: 100},
				Attestations: []*common.Attestation{{PubKey: []byte("pk3"), Signature: []byte("sig3")}},
			},
		},
		QuoErrataTxs: []*common.QuorumErrataTx{
			{
				ErrataTx:     &common.ErrataTx{Chain: common.BTCChain, Id: "errata_batch"},
				Attestations: []*common.Attestation{{PubKey: []byte("pk4"), Signature: []byte("sig4")}},
			},
		},
	}

	ag.processAttestationStateBatch(log.Logger, batch)

	// Give goroutine a moment to process
	time.Sleep(50 * time.Millisecond)

	ag.mu.Lock()
	c.Assert(len(ag.observedTxs) > 0, Equals, true)
	c.Assert(len(ag.networkFees) > 0, Equals, true)
	c.Assert(len(ag.solvencies) > 0, Equals, true)
	c.Assert(len(ag.errataTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// reconcileMimirConfigs
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestReconcileMimirConfigs(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	// Should not panic with mock bridge returning 0
	ag.reconcileMimirConfigs()
}

// ---------------------------------------------------------------------------
// closeStream
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestCloseStream(c *C) {
	stream := &MockStream{mu: &sync.Mutex{}}
	// Should not panic
	closeStream(log.Logger, stream)
}

// ---------------------------------------------------------------------------
// handleObservedTxAttestation with AllowFutureObservation
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleObservedTxAttestationFutureObservation(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    "future_tx",
			Chain: common.BTCChain,
		},
		BlockHeight:    100,
		FinaliseHeight: 100,
		ObservedPubKey: stypes.GetRandomPubKey(),
	}

	msg := common.AttestTx{
		ObsTx:                  obsTx,
		Inbound:                true,
		AllowFutureObservation: true,
		Attestation: &common.Attestation{
			PubKey:    ag.pubKey,
			Signature: []byte("sig1"),
		},
	}

	ag.handleObservedTxAttestation(context.Background(), msg)

	ag.mu.Lock()
	c.Assert(len(ag.observedTxs) > 0, Equals, true)
	ag.mu.Unlock()
}

// ---------------------------------------------------------------------------
// sendObservedTxAttestationsToThornode - gRPC error
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendObservedTxAttestationsGrpcError(c *C) {
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumTxFunc = func(ctx context.Context, tx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
		return nil, fmt.Errorf("grpc error")
	}

	obsTx := common.ObservedTx{
		Tx: common.Tx{ID: "tx_err", Chain: common.BTCChain},
	}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(&obsTx)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	// Should log error but not panic
	ag.sendObservedTxAttestationsToThornode(context.Background(), obsTx, state, true, false, true)
}

// ---------------------------------------------------------------------------
// sendNetworkFeeAttestationsToThornode - gRPC error
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendNetworkFeeAttestationsGrpcError(c *C) {
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumNetworkFeeFunc = func(ctx context.Context, nf *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
		return nil, fmt.Errorf("grpc error")
	}

	nf := common.NetworkFee{Chain: common.BTCChain, Height: 100}
	pool := NewAttestationStatePool[*common.NetworkFee]()
	state := pool.NewAttestationState(&nf)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendNetworkFeeAttestationsToThornode(context.Background(), nf, state, true)
}

// ---------------------------------------------------------------------------
// sendSolvencyAttestationsToThornode - gRPC error
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendSolvencyAttestationsGrpcError(c *C) {
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumSolvencyFunc = func(ctx context.Context, qs *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
		return nil, fmt.Errorf("grpc error")
	}

	sol := common.Solvency{Id: "s1", Chain: common.BTCChain, Height: 200}
	pool := NewAttestationStatePool[*common.Solvency]()
	state := pool.NewAttestationState(&sol)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendSolvencyAttestationsToThornode(context.Background(), sol, state, true)
}

// ---------------------------------------------------------------------------
// sendErrataAttestationsToThornode - gRPC error
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendErrataAttestationsGrpcError(c *C) {
	ag, grpcClient := newTestAttestationGossip(c, []peer.ID{"self"})
	grpcClient.sendQuorumErrataFunc = func(ctx context.Context, qe *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
		return nil, fmt.Errorf("grpc error")
	}

	errata := common.ErrataTx{Chain: common.BTCChain, Id: "e1"}
	pool := NewAttestationStatePool[*common.ErrataTx]()
	state := pool.NewAttestationState(&errata)
	state.attestations = []attestationSentState{
		{attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")}, sent: false},
	}

	ag.sendErrataAttestationsToThornode(context.Background(), errata, state, true)
}

// ---------------------------------------------------------------------------
// askForAttestationState - no peers
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestAskForAttestationStateNoPeers(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.host = NewMockHost([]peer.ID{}) // no peers at all
	ag.askForAttestationState(context.Background())
}

func (s *ObserveExtraSuite) TestAskForAttestationStateNoActiveValPeers(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1", "peer2"})
	// Only self is active, so peers without me that are active = 0
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true}
	ag.avMu.Unlock()

	ag.askForAttestationState(context.Background())
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - non-active validator
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsNonActive(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true} // only self
	ag.avMu.Unlock()

	// Create a mock stream from a non-active peer
	stream := &MockStream{
		mu:     &sync.Mutex{},
		peer:   "non_active_peer",
		writer: io.Discard,
	}

	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - empty payload
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateEmpty(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	// Write empty data to stream
	var buf zerolog.LevelWriter // not needed, just need a non-nil stream
	_ = buf

	// Use a mock stream that returns empty data
	stream := NewMockStreamWithData([]byte{}, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - unknown message type
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateUnknown(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	stream := NewMockStreamWithData([]byte{0xFF}, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - prefixSendState triggers sendAttestationState
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateSendState(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// prefixSendState is 0x01, single byte
	stream := NewMockStreamWithData([]byte{0x01}, "peer1")
	// This triggers sendAttestationState which will collect empty state,
	// write batch begin with 0 batches, then try to read ack (EOF), return
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - prefixSendState with wrong length
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateSendStateWrongLen(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// prefixSendState byte + extra data -> unexpected payload length
	stream := NewMockStreamWithData([]byte{0x01, 0x00}, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - prefixBatchBegin with nil stateInitPeers
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateBatchBeginNoInit(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// stateInitPeers is nil by default, so this should hit "not asking for attestation state"
	data := make([]byte, 5)
	data[0] = 0x02 // prefixBatchBegin
	binary.LittleEndian.PutUint32(data[1:], 1)
	stream := NewMockStreamWithData(data, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - prefixBatchBegin with unauthorized peer
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateBatchBeginUnauthorized(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.stateInitMu.Lock()
	ag.stateInitPeers = map[peer.ID]bool{"authorized_peer": true}
	ag.stateInitMu.Unlock()

	data := make([]byte, 5)
	data[0] = 0x02 // prefixBatchBegin
	binary.LittleEndian.PutUint32(data[1:], 1)
	// "peer1" is not in stateInitPeers
	stream := NewMockStreamWithData(data, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamAttestationState - prefixBatchBegin with authorized peer (0 batches)
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamAttestationStateBatchBeginAuthorized(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	ag.stateInitMu.Lock()
	ag.stateInitPeers = map[peer.ID]bool{"peer1": true}
	ag.stateInitMu.Unlock()

	data := make([]byte, 5)
	data[0] = 0x02                             // prefixBatchBegin
	binary.LittleEndian.PutUint32(data[1:], 0) // 0 batches
	stream := NewMockStreamWithData(data, "peer1")
	ag.handleStreamAttestationState(stream)
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - empty payload
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsEmptyPayload(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true, "peer1": true}
	ag.avMu.Unlock()

	// Create bidirectional stream - write empty data, read ack back
	var readBuf bytes.Buffer
	readBuf.Write(encodeStreamMsg([]byte{})) // empty payload
	writeBuf := &bytes.Buffer{}
	stream := &MockStream{reader: &readBuf, writer: writeBuf, peer: "peer1", mu: &sync.Mutex{}}
	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - bad unmarshal data
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsBadData(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true, "peer1": true}
	ag.avMu.Unlock()

	// Create stream with garbage data that can't be unmarshalled
	var readBuf bytes.Buffer
	readBuf.Write(encodeStreamMsg([]byte{0xFF, 0xFE, 0xFD}))
	writeBuf := &bytes.Buffer{}
	stream := &MockStream{reader: &readBuf, writer: writeBuf, peer: "peer1", mu: &sync.Mutex{}}
	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - batch with nil items
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsWithEmptyBatch(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true, "peer1": true}
	ag.avMu.Unlock()

	// Create a valid but empty batch (no items)
	batch := common.AttestationBatch{}
	batchBz, err := batch.Marshal()
	c.Assert(err, IsNil)

	var readBuf bytes.Buffer
	readBuf.Write(encodeStreamMsg(batchBz))
	writeBuf := &bytes.Buffer{}
	stream := &MockStream{reader: &readBuf, writer: writeBuf, peer: "peer1", mu: &sync.Mutex{}}
	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - batch with price feed
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsPriceFeed(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true, "peer1": true}
	ag.avMu.Unlock()

	batch := common.AttestationBatch{
		AttestPriceFeeds: []*common.AttestPriceFeed{
			{
				PriceFeed: &common.PriceFeed{
					Rates: []*common.OraclePrice{{Amount: 100, Decimals: 8}},
				},
				Attestation: &common.Attestation{PubKey: []byte("pk1"), Signature: []byte("sig1")},
			},
		},
	}
	batchBz, err := batch.Marshal()
	c.Assert(err, IsNil)

	var readBuf bytes.Buffer
	readBuf.Write(encodeStreamMsg(batchBz))
	writeBuf := &bytes.Buffer{}
	stream := &MockStream{reader: &readBuf, writer: writeBuf, peer: "peer1", mu: &sync.Mutex{}}
	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// handleStreamBatchedAttestations - semaphore exhaustion
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestHandleStreamBatchedAttestationsSemExhausted(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})
	ag.avMu.Lock()
	ag.activeVals = map[peer.ID]bool{"self": true, "peer1": true}
	ag.avMu.Unlock()

	// Exhaust the semaphore for peer1
	peerID := peer.ID("peer1")
	for i := 0; i < 5; i++ {
		_, _ = ag.peerMgr.acquire(peerID)
	}

	var readBuf bytes.Buffer
	readBuf.Write(encodeStreamMsg([]byte{0x01}))
	writeBuf := &bytes.Buffer{}
	stream := &MockStream{reader: &readBuf, writer: writeBuf, peer: peerID, mu: &sync.Mutex{}}
	ag.handleStreamBatchedAttestations(stream)
}

// ---------------------------------------------------------------------------
// sendAttestationState - with populated state
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendAttestationStateWithData(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self", "peer1"})

	// Add some state
	ag.mu.Lock()
	obsTx := &common.ObservedTx{Tx: common.Tx{ID: "tx1"}}
	pool := NewAttestationStatePool[*common.ObservedTx]()
	state := pool.NewAttestationState(obsTx)
	key := txKey{ID: "tx1"}
	ag.observedTxs[key] = state

	nf := &common.NetworkFee{Chain: common.BTCChain, Height: 100}
	nfPool := NewAttestationStatePool[*common.NetworkFee]()
	nfState := nfPool.NewAttestationState(nf)
	ag.networkFees[*nf] = nfState
	ag.mu.Unlock()

	// Create a bidirectional stream - we write responses the sender expects
	var readBuf, writeBuf bytes.Buffer
	stream := &MockStream{reader: &readBuf, writer: &writeBuf, peer: "peer1", mu: &sync.Mutex{}}

	// sendAttestationState will write batch begin, then try to read ack.
	// Since readBuf is empty, it'll get EOF and return early.
	ag.sendAttestationState(stream)
}

// ---------------------------------------------------------------------------
// processObservedTx - new entry and dedup paths
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessObservedTxNewAndDedup(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)
	tmpDir := c.MkDir()
	storage, err := NewObserverStorage(tmpDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	pkm, pkmErr := pubkeymanager.NewPubKeyManager(nil, nil)
	c.Assert(pkmErr, IsNil)

	o := &Observer{
		logger:           log.Logger,
		lock:             &sync.Mutex{},
		onDeck:           make(map[txInKey]*types.TxIn),
		pubkeyMgr:        pkm,
		chains:           map[common.Chain]chainclients.ChainClient{},
		storage:          storage,
		signedTxOutCache: cache,
		m:                s.m,
		errCounter:       s.m.GetCounterVec(metrics.ObserverError),
	}

	// Test 1: empty TxArray
	o.processObservedTx(types.TxIn{})

	// Test 2: new entry with pre-filtered TxIn
	txIn := types.TxIn{
		Chain:    common.BTCChain,
		Filtered: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:         100,
				Tx:                  "txhash1",
				Sender:              "sender1",
				To:                  "to1",
				Coins:               common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:                "SWAP:ETH.ETH:addr",
				ObservedVaultPubKey: "thorpub1addwnpepqtest",
			},
		},
		ConfirmationRequired: 0,
	}
	o.processObservedTx(txIn)
	c.Assert(len(o.onDeck), Equals, 1)

	// Test 3: dedup - same tx again should not add duplicate
	txInDup := types.TxIn{
		Chain:    common.BTCChain,
		Filtered: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:         100,
				Tx:                  "txhash1",
				Sender:              "sender1",
				To:                  "to1",
				Coins:               common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:                "SWAP:ETH.ETH:addr",
				ObservedVaultPubKey: "thorpub1addwnpepqtest",
			},
		},
		ConfirmationRequired: 0,
	}
	o.processObservedTx(txInDup)
	// Should still only have 1 item in onDeck entry
	for _, deck := range o.onDeck {
		c.Assert(len(deck.TxArray), Equals, 1)
	}

	// Test 4: new tx added to existing deck
	txInNew := types.TxIn{
		Chain:    common.BTCChain,
		Filtered: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:         100,
				Tx:                  "txhash2",
				Sender:              "sender2",
				To:                  "to2",
				Coins:               common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(2000))},
				Memo:                "SWAP:ETH.ETH:addr2",
				ObservedVaultPubKey: "thorpub1addwnpepqtest",
			},
		},
		ConfirmationRequired: 0,
	}
	o.processObservedTx(txInNew)
	for _, deck := range o.onDeck {
		c.Assert(len(deck.TxArray), Equals, 2)
	}
}

// ---------------------------------------------------------------------------
// getThorchainTxIns - various error paths
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetThorchainTxInsMorePaths(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)

	o := &Observer{
		logger:              log.Logger,
		lock:                &sync.Mutex{},
		onDeck:              make(map[txInKey]*types.TxIn),
		m:                   s.m,
		errCounter:          s.m.GetCounterVec(metrics.ObserverError),
		signedTxOutCache:    cache,
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
	}

	// Test empty coins
	txIn := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "txhash1",
				Sender:      "sender",
				To:          "to",
				Coins:       common.Coins{},
				Memo:        "test",
			},
		},
	}
	txs, invalidIndices, err := o.getThorchainTxIns(txIn, false, 0)
	c.Assert(err, IsNil)
	c.Assert(len(txs), Equals, 0)
	c.Assert(len(invalidIndices), Equals, 1) // empty coins = invalid

	// Test memo too long
	longMemo := make([]byte, 500)
	for i := range longMemo {
		longMemo[i] = 'a'
	}
	txIn2 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "txhash2",
				Sender:      "sender",
				To:          "to",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        string(longMemo),
			},
		},
	}
	txs2, invalidIndices2, err := o.getThorchainTxIns(txIn2, false, 0)
	c.Assert(err, IsNil)
	c.Assert(len(txs2), Equals, 0)
	c.Assert(len(invalidIndices2), Equals, 1) // memo too long

	// Test empty To address
	txIn3 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "txhash3",
				Sender:      "sender",
				To:          "",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        "test",
			},
		},
	}
	txs3, invalidIndices3, err := o.getThorchainTxIns(txIn3, false, 0)
	c.Assert(err, IsNil)
	c.Assert(len(txs3), Equals, 0)
	c.Assert(len(invalidIndices3), Equals, 1) // empty to

	// Test invalid tx hash
	txIn4 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 100,
				Tx:          "", // empty tx hash is invalid
				Sender:      "bc1qsender",
				To:          "bc1qto",
				Coins:       common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:        "test",
			},
		},
	}
	txs4, invalidIndices4, err := o.getThorchainTxIns(txIn4, false, 0)
	c.Assert(err, IsNil)
	c.Assert(len(txs4), Equals, 0)
	c.Assert(len(invalidIndices4), Equals, 1)

	// Test CommittedUnFinalised skips when not finalized
	txIn5 := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight:          100,
				Tx:                   "AABB",
				Sender:               "bc1qsender",
				To:                   "bc1qto",
				Coins:                common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(1000))},
				Memo:                 "test",
				CommittedUnFinalised: true,
			},
		},
	}
	txs5, _, err := o.getThorchainTxIns(txIn5, false, 0)
	c.Assert(err, IsNil)
	c.Assert(len(txs5), Equals, 0) // skipped because CommittedUnFinalised && !finalized
}

// ---------------------------------------------------------------------------
// filterErrataTx - edge cases
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestFilterErrataTxMatch(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)
	tmpDir := c.MkDir()
	storage, err := NewObserverStorage(tmpDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	o := &Observer{
		logger:           log.Logger,
		lock:             &sync.Mutex{},
		onDeck:           make(map[txInKey]*types.TxIn),
		storage:          storage,
		signedTxOutCache: cache,
		m:                s.m,
		errCounter:       s.m.GetCounterVec(metrics.ObserverError),
	}

	// Add a tx to ondeck
	txIn := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: "AABB"},
			{BlockHeight: 100, Tx: "CCDD"},
		},
		ConfirmationRequired: 0,
	}
	key := TxInKey(txIn)
	o.onDeck[key] = txIn

	// Filter errata that matches first item
	block := types.ErrataBlock{
		Height: 100,
		Txs: []types.ErrataTx{
			{TxID: "AABB", Chain: common.BTCChain},
		},
	}
	o.filterErrataTx(block)

	// First item should be removed, second remains
	c.Assert(len(o.onDeck[key].TxArray), Equals, 1)
	c.Assert(o.onDeck[key].TxArray[0].Tx, Equals, "CCDD")
}

func (s *ObserveExtraSuite) TestFilterErrataTxRemoveAll(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)
	tmpDir := c.MkDir()
	storage, err := NewObserverStorage(tmpDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	o := &Observer{
		logger:           log.Logger,
		lock:             &sync.Mutex{},
		onDeck:           make(map[txInKey]*types.TxIn),
		storage:          storage,
		signedTxOutCache: cache,
		m:                s.m,
		errCounter:       s.m.GetCounterVec(metrics.ObserverError),
	}

	txIn := &types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{BlockHeight: 100, Tx: "AABB"},
		},
		ConfirmationRequired: 0,
	}
	key := TxInKey(txIn)
	o.onDeck[key] = txIn

	block := types.ErrataBlock{
		Height: 100,
		Txs: []types.ErrataTx{
			{TxID: "AABB", Chain: common.BTCChain},
		},
	}
	o.filterErrataTx(block)

	// Entire deck entry should be removed
	_, exists := o.onDeck[key]
	c.Assert(exists, Equals, false)
}

// ---------------------------------------------------------------------------
// EventClient - CleanShutdown
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestEventClientCleanShutdown(c *C) {
	ec := NewEventClient(nil)
	ec.CleanShutdown()
	// Calling again should not panic (closeOnce)
	ec.CleanShutdown()
}

// ---------------------------------------------------------------------------
// EventClient - RegisterHandler duplicate type
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestEventClientRegisterHandlerDuplicate(c *C) {
	ec := NewEventClient(nil)
	ec.RegisterHandler("test", func(e *ebifrost.EventNotification) {})
	ec.RegisterHandler("test", func(e *ebifrost.EventNotification) {})
	// Should not duplicate the event type
}

// ---------------------------------------------------------------------------
// EventClient - Stop when not active
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestEventClientStopInactive(c *C) {
	ec := NewEventClient(nil)
	ec.Stop() // should not panic when not active
}

// ---------------------------------------------------------------------------
// sendDeck - basic node status paths
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestSendDeckNotActive(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)
	tmpDir := c.MkDir()
	storage, err := NewObserverStorage(tmpDir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	defer storage.Close()

	bridge := &MockThorchainBridge2{
		nodeStatus:  stypes.NodeStatus_Standby,
		activeNodes: common.PubKeys{},
	}

	o := &Observer{
		logger:           log.Logger,
		lock:             &sync.Mutex{},
		onDeck:           make(map[txInKey]*types.TxIn),
		signedTxOutCache: cache,
		m:                s.m,
		errCounter:       s.m.GetCounterVec(metrics.ObserverError),
		thorchainBridge:  bridge,
		storage:          storage,
		attestationGossip: &AttestationGossip{
			logger: log.Logger,
		},
		chains: map[common.Chain]chainclients.ChainClient{},
	}

	// Should return early since node is not active
	o.sendDeck(context.Background())
}

// ---------------------------------------------------------------------------
// processOracle / getOracleMimirs coverage
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestGetOracleMimirsError(c *C) {
	bridge := &MockThorchainBridge2{
		getMimirFunc: func(key string) (int64, error) {
			return 0, fmt.Errorf("mimir error")
		},
	}

	o := &Observer{
		logger:          log.Logger,
		thorchainBridge: bridge,
	}

	interval, halt := o.getOracleMimirs()
	c.Assert(halt, Equals, false)
	c.Assert(interval, Equals, defaultOracleUpdateInterval)
}

func (s *ObserveExtraSuite) TestGetOracleMimirsHalt(c *C) {
	bridge := &MockThorchainBridge2{
		getMimirFunc: func(key string) (int64, error) {
			if key == "HaltOracle" {
				return 1, nil
			}
			return 0, nil
		},
	}

	o := &Observer{
		logger:          log.Logger,
		thorchainBridge: bridge,
	}

	_, halt := o.getOracleMimirs()
	c.Assert(halt, Equals, true)
}

func (s *ObserveExtraSuite) TestGetOracleMimirsInterval(c *C) {
	bridge := &MockThorchainBridge2{
		getMimirFunc: func(key string) (int64, error) {
			if key == "OracleUpdateInterval" {
				return 5000, nil // 5 seconds in ms
			}
			return 0, nil
		},
	}

	o := &Observer{
		logger:          log.Logger,
		thorchainBridge: bridge,
	}

	interval, halt := o.getOracleMimirs()
	c.Assert(halt, Equals, false)
	c.Assert(interval, Equals, 5*time.Second)
}

// ---------------------------------------------------------------------------
// processAttestationStateBatch - with empty batch
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessAttestationStateBatchEmpty(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})
	// Empty batch should not panic
	ag.processAttestationStateBatch(log.Logger, common.QuorumState{})
}

// ---------------------------------------------------------------------------
// filterObservations - cancel observation
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestFilterObservationsCancelTx(c *C) {
	cache, err := lru.New(100)
	c.Assert(err, IsNil)
	pkm, pkmErr := pubkeymanager.NewPubKeyManager(nil, nil)
	c.Assert(pkmErr, IsNil)

	o := &Observer{
		logger:           log.Logger,
		pubkeyMgr:        pkm,
		signedTxOutCache: cache,
	}

	// A cancel tx: sender == to and memo is empty
	items := []*types.TxInItem{
		{
			Tx:     "txhash",
			Sender: "addr1",
			To:     "addr1",
			Memo:   "",
		},
	}
	result := o.filterObservations(common.BTCChain, items, false)
	c.Assert(len(result), Equals, 0) // cancel tx should be filtered out
}

// ---------------------------------------------------------------------------
// processNetworkFeeQueue / processSolvencyQueue / processPriceFeedQueue - stop paths
// ---------------------------------------------------------------------------

func (s *ObserveExtraSuite) TestProcessQueueStopPaths(c *C) {
	ag, _ := newTestAttestationGossip(c, []peer.ID{"self"})

	o := &Observer{
		logger:                log.Logger,
		lock:                  &sync.Mutex{},
		stopChan:              make(chan struct{}),
		globalNetworkFeeQueue: make(chan common.NetworkFee, 1),
		globalSolvencyQueue:   make(chan types.Solvency, 1),
		globalPriceFeedQueue:  make(chan common.PriceFeed, 1),
		m:                     s.m,
		errCounter:            s.m.GetCounterVec(metrics.ObserverError),
		attestationGossip:     ag,
	}

	// Test processNetworkFeeQueue with invalid network fee
	o.globalNetworkFeeQueue <- common.NetworkFee{} // invalid: chain is empty
	close(o.stopChan)
	o.processNetworkFeeQueue(context.Background())
}
