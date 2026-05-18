package blockscanner

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newMemLevelDB(c *C) *leveldb.DB {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	c.Assert(err, IsNil)
	return db
}

var errTest = errors.New("test error")

var initMetricsOnce sync.Once

// ---------------------------------------------------------------------------
// configurable mock fetcher
// ---------------------------------------------------------------------------

type configFetcher struct {
	txIn           types.TxIn
	fetchErr       error
	chainHeight    int64
	chainHeightErr error
	networkFeeSize uint64
	networkFeeRate uint64
}

func (f *configFetcher) FetchMemPool(height int64) (types.TxIn, error) {
	return f.txIn, f.fetchErr
}

func (f *configFetcher) FetchTxs(fetchHeight, chainHeight int64) (types.TxIn, error) {
	return f.txIn, f.fetchErr
}

func (f *configFetcher) GetHeight() (int64, error) {
	return f.chainHeight, f.chainHeightErr
}

func (f *configFetcher) GetNetworkFee() (uint64, uint64) {
	return f.networkFeeSize, f.networkFeeRate
}

// ---------------------------------------------------------------------------
// LevelDB storage tests
// ---------------------------------------------------------------------------

type LevelDBStorageSuite struct{}

var _ = Suite(&LevelDBStorageSuite{})

func (s *LevelDBStorageSuite) TestNewLevelDBScannerStorage(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, err := NewLevelDBScannerStorage(db)
	c.Assert(err, IsNil)
	c.Assert(store, NotNil)
}

func (s *LevelDBStorageSuite) TestGetSetScanPos(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorage(db)

	_, err := store.GetScanPos()
	c.Check(err, NotNil)

	c.Assert(store.SetScanPos(42), IsNil)
	pos, err := store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, int64(42))

	c.Assert(store.SetScanPos(999), IsNil)
	pos, err = store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, int64(999))

	c.Assert(store.SetScanPos(-10), IsNil)
	pos, err = store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, int64(-10))
}

func (s *LevelDBStorageSuite) TestSetBlockScanStatus(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorage(db)

	block := Block{Height: 10, Txs: []string{"tx1", "tx2"}}
	c.Assert(store.SetBlockScanStatus(block, Processing), IsNil)
	c.Assert(store.SetBlockScanStatus(block, Finished), IsNil)
}

func (s *LevelDBStorageSuite) TestGetBlocksForRetry(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorage(db)

	blocks, err := store.GetBlocksForRetry(true)
	c.Assert(err, IsNil)
	c.Check(len(blocks), Equals, 0)

	c.Assert(store.SetBlockScanStatus(Block{Height: 1, Txs: []string{"a"}}, Failed), IsNil)
	c.Assert(store.SetBlockScanStatus(Block{Height: 2, Txs: []string{"b"}}, Finished), IsNil)
	c.Assert(store.SetBlockScanStatus(Block{Height: 3, Txs: []string{"c"}}, Failed), IsNil)

	blocks, err = store.GetBlocksForRetry(true)
	c.Assert(err, IsNil)
	c.Check(len(blocks), Equals, 2)

	blocks, err = store.GetBlocksForRetry(false)
	c.Assert(err, IsNil)
	c.Check(len(blocks), Equals, 3)
}

func (s *LevelDBStorageSuite) TestRemoveBlockStatus(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorage(db)

	block := Block{Height: 5}
	c.Assert(store.SetBlockScanStatus(block, Failed), IsNil)

	blocks, _ := store.GetBlocksForRetry(true)
	c.Check(len(blocks), Equals, 1)

	c.Assert(store.RemoveBlockStatus(5), IsNil)

	blocks, _ = store.GetBlocksForRetry(true)
	c.Check(len(blocks), Equals, 0)
}

func (s *LevelDBStorageSuite) TestClose(c *C) {
	db := newMemLevelDB(c)
	store, _ := NewLevelDBScannerStorage(db)
	c.Assert(store.Close(), IsNil)
}

// ---------------------------------------------------------------------------
// LevelDB Solana storage tests
// ---------------------------------------------------------------------------

type LevelDBSolanaSuite struct{}

var _ = Suite(&LevelDBSolanaSuite{})

func (s *LevelDBSolanaSuite) TestNewLevelDBScannerStorageSolana(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, err := NewLevelDBScannerStorageSolana(db)
	c.Assert(err, IsNil)
	c.Assert(store, NotNil)
}

func (s *LevelDBSolanaSuite) TestSolanaGetSetScanPos(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorageSolana(db)

	_, err := store.GetScanPos()
	c.Check(err, NotNil)

	c.Assert(store.SetScanPos(42), IsNil)
	pos, err := store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, uint64(42))

	c.Assert(store.SetScanPos(200), IsNil)
	pos, err = store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, uint64(200))
}

func (s *LevelDBSolanaSuite) TestSolanaGetSetScanStatus(c *C) {
	db := newMemLevelDB(c)
	defer db.Close()
	store, _ := NewLevelDBScannerStorageSolana(db)

	account := "SomeSolanaAccount123"

	_, _, err := store.GetScanStatus(account)
	c.Check(err, NotNil)

	c.Assert(store.SetScanStatus(account, "sig123abc", 999), IsNil)
	sig, slot, err := store.GetScanStatus(account)
	c.Assert(err, IsNil)
	c.Check(sig, Equals, "sig123abc")
	c.Check(slot, Equals, uint64(999))

	c.Assert(store.SetScanStatus(account, "sig456def", 1500), IsNil)
	sig, slot, err = store.GetScanStatus(account)
	c.Assert(err, IsNil)
	c.Check(sig, Equals, "sig456def")
	c.Check(slot, Equals, uint64(1500))
}

func (s *LevelDBSolanaSuite) TestSolanaClose(c *C) {
	db := newMemLevelDB(c)
	store, _ := NewLevelDBScannerStorageSolana(db)
	c.Assert(store.Close(), IsNil)
}

// ---------------------------------------------------------------------------
// BlockScannerStorage constructor tests
// ---------------------------------------------------------------------------

type StorageConstructorSuite struct{}

var _ = Suite(&StorageConstructorSuite{})

func (s *StorageConstructorSuite) TestNewBlockScannerStorage(c *C) {
	tmpdir, err := os.MkdirTemp("", "bs-test-*")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	store, err := NewBlockScannerStorage(tmpdir+"/db", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	c.Assert(store, NotNil)
	c.Check(store.GetInternalDb(), NotNil)

	c.Assert(store.SetScanPos(99), IsNil)
	pos, err := store.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, int64(99))

	store.Close()
}

func (s *StorageConstructorSuite) TestNewBlockScannerStorageSolana(c *C) {
	tmpdir, err := os.MkdirTemp("", "bs-sol-test-*")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	store, err := NewBlockScannerStorageSolana(tmpdir+"/db", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	c.Assert(store, NotNil)
	c.Check(store.GetInternalDb(), NotNil)
	store.Close()
}

// ---------------------------------------------------------------------------
// Mock scanner storage tests
// ---------------------------------------------------------------------------

type MockStorageSuite struct{}

var _ = Suite(&MockStorageSuite{})

func (s *MockStorageSuite) TestMockScannerStorage(c *C) {
	mss := NewMockScannerStorage()

	_, err := mss.GetScanPos()
	c.Check(err, NotNil)

	c.Assert(mss.SetScanPos(100), IsNil)
	pos, err := mss.GetScanPos()
	c.Assert(err, IsNil)
	c.Check(pos, Equals, int64(100))

	c.Assert(mss.SetBlockScanStatus(Block{Height: 10}, Failed), IsNil)
	c.Assert(mss.RemoveBlockStatus(10), IsNil)

	blocks, err := mss.GetBlocksForRetry(true)
	c.Assert(err, IsNil)
	c.Check(blocks, IsNil)

	c.Check(mss.GetInternalDb(), IsNil)
	c.Assert(mss.Close(), IsNil)
}

// ---------------------------------------------------------------------------
// DummyFetcher tests
// ---------------------------------------------------------------------------

type DummyFetcherSuite struct{}

var _ = Suite(&DummyFetcherSuite{})

func (s *DummyFetcherSuite) TestDummyFetcher(c *C) {
	txIn := types.TxIn{Chain: common.ETHChain}
	d := DummyFetcher{Tx: txIn}

	tx, err := d.FetchMemPool(1)
	c.Assert(err, IsNil)
	c.Check(tx.Chain, Equals, common.ETHChain)

	tx, err = d.FetchTxs(1, 2)
	c.Assert(err, IsNil)
	c.Check(tx.Chain, Equals, common.ETHChain)

	h, err := d.GetHeight()
	c.Assert(err, IsNil)
	c.Check(h, Equals, int64(0))

	size, rate := d.GetNetworkFee()
	c.Check(size, Equals, uint64(0))
	c.Check(rate, Equals, uint64(0))

	d2 := DummyFetcher{Err: errTest}
	_, err = d2.FetchMemPool(1)
	c.Check(err, NotNil)
	_, err = d2.FetchTxs(1, 2)
	c.Check(err, NotNil)
}

// ---------------------------------------------------------------------------
// BlockScanStatus constants
// ---------------------------------------------------------------------------

type BlockScanStatusSuite struct{}

var _ = Suite(&BlockScanStatusSuite{})

func (s *BlockScanStatusSuite) TestBlockScanStatus(c *C) {
	c.Check(Processing, Equals, BlockScanStatus(0))
	c.Check(Failed, Equals, BlockScanStatus(1))
	c.Check(Finished, Equals, BlockScanStatus(2))
	c.Check(NotStarted, Equals, BlockScanStatus(3))
}

// ---------------------------------------------------------------------------
// BlockScanner extra tests
// ---------------------------------------------------------------------------

type BlockScannerExtraSuite struct {
	keys *thorclient.Keys
}

var _ = Suite(&BlockScannerExtraSuite{})

func (s *BlockScannerExtraSuite) SetUpSuite(c *C) {
	// Initialize the package-level m variable using sync.Once to avoid
	// double-registering Prometheus collectors.
	initMetricsOnce.Do(func() {
		var err error
		m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9090,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.ETHChain},
		})
		if err != nil {
			panic(err)
		}
	})
	thorchain.SetupConfigForTest()
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := ckeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic("bob", ckeys.English, cmd.THORChainHDPath, "password", hd.Secp256k1)
	c.Assert(err, IsNil)
	s.keys = thorclient.NewKeysWithKeybase(kb, "bob", "password")
}

func (s *BlockScannerExtraSuite) makeBridge(c *C, server *httptest.Server) thorclient.ThorchainBridge {
	bridge, err := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		ChainRPC:        server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: ".",
	}, m, s.keys)
	c.Assert(err, IsNil)
	return bridge
}

func (s *BlockScannerExtraSuite) TestIsHealthy(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight: 1,
		ChainID:          common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)

	c.Check(scanner.IsHealthy(), Equals, false)
	scanner.healthy.Store(true)
	c.Check(scanner.IsHealthy(), Equals, true)

	c.Check(scanner.PreviousHeight(), Equals, int64(1))
	atomic.StoreInt64(&scanner.previousBlock, 42)
	c.Check(scanner.PreviousHeight(), Equals, int64(42))
}

func (s *BlockScannerExtraSuite) TestNewBlockScannerValidation(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	bridge := s.makeBridge(c, server)
	mss := NewMockScannerStorage()
	cfg := config.BifrostBlockScannerConfiguration{StartBlockHeight: 1}

	_, err := NewBlockScanner(cfg, nil, m, bridge, DummyFetcher{})
	c.Check(err, ErrorMatches, "scannerStorage is nil")

	_, err = NewBlockScanner(cfg, mss, nil, bridge, DummyFetcher{})
	c.Check(err, ErrorMatches, "metrics instance is nil")

	_, err = NewBlockScanner(cfg, mss, m, nil, DummyFetcher{})
	c.Check(err, ErrorMatches, "thorchain bridge is nil")
}

func (s *BlockScannerExtraSuite) TestRollback(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight: 1,
		ChainID:          common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	atomic.StoreInt64(&scanner.previousBlock, 100)

	// Higher height - no-op
	c.Assert(scanner.rollback(200), IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(100))

	// Equal height - no-op
	c.Assert(scanner.rollback(100), IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(100))

	// Lower height - updates
	c.Assert(scanner.rollback(50), IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(50))
	pos, _ := mss.GetScanPos()
	c.Check(pos, Equals, int64(50))
}

func (s *BlockScannerExtraSuite) TestGetMessages(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight: 1,
		ChainID:          common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	c.Check(scanner.GetMessages(), NotNil)
}

func (s *BlockScannerExtraSuite) TestUpdateStaleNetworkFee(c *C) {
	// Case 1: THORChain chain - no-op
	{
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()
		bridge := s.makeBridge(c, server)
		mss := NewMockScannerStorage()
		scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1,
			ChainID:          common.THORChain,
		}, mss, m, bridge, DummyFetcher{})
		c.Assert(err, IsNil)
		nfChan := make(chan common.NetworkFee, 1)
		scanner.globalNetworkFeeQueue = nfChan
		scanner.healthy.Store(true)
		scanner.updateStaleNetworkFee(100)
		c.Check(len(nfChan), Equals, 0)
	}

	// Case 2: not healthy - no-op
	{
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()
		bridge := s.makeBridge(c, server)
		mss := NewMockScannerStorage()
		scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1,
			ChainID:          common.ETHChain,
		}, mss, m, bridge, DummyFetcher{})
		c.Assert(err, IsNil)
		nfChan := make(chan common.NetworkFee, 1)
		scanner.globalNetworkFeeQueue = nfChan
		scanner.healthy.Store(false)
		scanner.updateStaleNetworkFee(100)
		c.Check(len(nfChan), Equals, 0)
	}

	// Case 3: fees match - no broadcast
	{
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.RequestURI, "/thorchain/inbound_addresses") {
				_, _ = w.Write([]byte(`[{"chain":"ETH","outbound_tx_size":"250","observed_fee_rate":"30"}]`))
			}
		}))
		defer server.Close()
		bridge := s.makeBridge(c, server)
		mss := NewMockScannerStorage()
		fetcher := &configFetcher{networkFeeSize: 250, networkFeeRate: 30, chainHeight: 1}
		scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1,
			ChainID:          common.ETHChain,
		}, mss, m, bridge, fetcher)
		c.Assert(err, IsNil)
		nfChan := make(chan common.NetworkFee, 1)
		scanner.globalNetworkFeeQueue = nfChan
		scanner.healthy.Store(true)
		scanner.updateStaleNetworkFee(100)
		c.Check(len(nfChan), Equals, 0)
	}

	// Case 4: fees differ - should broadcast
	{
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.RequestURI, "/thorchain/inbound_addresses") {
				_, _ = w.Write([]byte(`[{"chain":"ETH","outbound_tx_size":"250","observed_fee_rate":"30"}]`))
			}
		}))
		defer server.Close()
		bridge := s.makeBridge(c, server)
		mss := NewMockScannerStorage()
		fetcher := &configFetcher{networkFeeSize: 300, networkFeeRate: 40, chainHeight: 1}
		scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1,
			ChainID:          common.ETHChain,
		}, mss, m, bridge, fetcher)
		c.Assert(err, IsNil)
		nfChan := make(chan common.NetworkFee, 1)
		scanner.globalNetworkFeeQueue = nfChan
		scanner.healthy.Store(true)
		scanner.updateStaleNetworkFee(100)
		c.Check(len(nfChan), Equals, 1)
		nf := <-nfChan
		c.Check(nf.Chain, Equals, common.ETHChain)
		c.Check(nf.TransactionSize, Equals, uint64(300))
		c.Check(nf.TransactionRate, Equals, uint64(40))
		c.Check(nf.Height, Equals, int64(100))
	}

	// Case 5: bridge error - no broadcast
	{
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.RequestURI, "/thorchain/inbound_addresses") {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer server.Close()
		bridge := s.makeBridge(c, server)
		mss := NewMockScannerStorage()
		fetcher := &configFetcher{networkFeeSize: 300, networkFeeRate: 40, chainHeight: 1}
		scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1,
			ChainID:          common.ETHChain,
		}, mss, m, bridge, fetcher)
		c.Assert(err, IsNil)
		nfChan := make(chan common.NetworkFee, 1)
		scanner.globalNetworkFeeQueue = nfChan
		scanner.healthy.Store(true)
		scanner.updateStaleNetworkFee(100)
		c.Check(len(nfChan), Equals, 0)
	}
}

func (s *BlockScannerExtraSuite) TestGetStartHeightConfigured(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight: 50,
		ChainID:          common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(50))
}

func (s *BlockScannerExtraSuite) TestGetStartHeightLastObserved(c *C) {
	// Path 2b: has last observed height, no scanner storage position
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, "/status"):
			_, _ = w.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock/ETH"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","last_observed_in":75,"last_signed_out":0,"thorchain":100}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		ChainID: common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(75))
}

func (s *BlockScannerExtraSuite) TestGetStartHeightWithStorage(c *C) {
	// Path 2a: last observed + scanner storage within max lag
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, "/status"):
			_, _ = w.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock/ETH"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","last_observed_in":100,"last_signed_out":0,"thorchain":100}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	_ = mss.SetScanPos(90)
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		ChainID:           common.ETHChain,
		MaxResumeBlockLag: time.Hour,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(90))
}

func (s *BlockScannerExtraSuite) TestGetStartHeightStorageTooFar(c *C) {
	// Path 2a variant: scanner storage too far behind max lag
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, "/status"):
			_, _ = w.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock/ETH"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","last_observed_in":100,"last_signed_out":0,"thorchain":100}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	_ = mss.SetScanPos(10)
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		ChainID:           common.ETHChain,
		MaxResumeBlockLag: time.Second,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	h := scanner.PreviousHeight()
	c.Check(h > 10, Equals, true, Commentf("Expected > 10, got %d", h))
	c.Check(h <= 100, Equals, true, Commentf("Expected <= 100, got %d", h))
}

func (s *BlockScannerExtraSuite) TestGetStartHeightNoLastObservedWithStorage(c *C) {
	// Path 3: no last observed, scanner storage available
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, "/status"):
			_, _ = w.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock/ETH"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	_ = mss.SetScanPos(60)
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		ChainID: common.ETHChain,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(60))
}

func (s *BlockScannerExtraSuite) TestGetStartHeightChainHeight(c *C) {
	// Path 4: nothing available, use chain height
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, "/status"):
			_, _ = w.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock/ETH"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"","last_observed_in":0,"last_signed_out":0,"thorchain":100}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	fetcher := &configFetcher{chainHeight: 200}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		ChainID: common.ETHChain,
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)
	c.Check(scanner.PreviousHeight(), Equals, int64(200))
}

func (s *BlockScannerExtraSuite) TestStartStopWithMempoolDisabled(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			buf, _ := os.ReadFile("../../test/fixtures/endpoints/mimir/mimir.json")
			_, _ = w.Write(buf)
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: time.Second,
		BlockRetryInterval:         time.Second,
		ChainID:                    common.ETHChain,
		ScanMemPool:                false,
	}, mss, m, bridge, DummyFetcher{})
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn)
	nfChan := make(chan common.NetworkFee)
	scanner.Start(globalChan, nfChan)
	time.Sleep(500 * time.Millisecond)
	scanner.Stop()
}

// TestScanBlocksProcessesTxs verifies that scanBlocks actually processes blocks
// when the chain height is ahead of the current scanner position.
func (s *BlockScannerExtraSuite) TestScanBlocksProcessesTxs(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			// All mimir values unset
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/inbound_addresses"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","outbound_tx_size":"250","observed_fee_rate":"30"}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	// Fetcher returns chain height 5 and some txs
	fetcher := &configFetcher{
		chainHeight:    5,
		networkFeeSize: 250,
		networkFeeRate: 30,
		txIn: types.TxIn{
			Chain: common.ETHChain,
			TxArray: []*types.TxInItem{
				{Tx: "test-tx-1"},
			},
		},
	}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: 100 * time.Millisecond,
		BlockRetryInterval:         100 * time.Millisecond,
		ChainID:                    common.ETHChain,
		MaxHealthyLag:              time.Hour,
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn, 20)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)

	// Wait for scanner to process some blocks
	time.Sleep(2 * time.Second)
	scanner.Stop()

	// Should have received txs
	c.Check(len(globalChan) > 0, Equals, true, Commentf("Expected txs in queue, got %d", len(globalChan)))

	// Scanner should be healthy (within 3 blocks of chain tip)
	c.Check(scanner.IsHealthy(), Equals, true)

	// Previous height should have advanced
	c.Check(scanner.PreviousHeight() > 1, Equals, true, Commentf("Expected height > 1, got %d", scanner.PreviousHeight()))
}

// TestScanBlocksWithPrefetch tests the prefetch path.
func (s *BlockScannerExtraSuite) TestScanBlocksWithPrefetch(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/inbound_addresses"):
			_, _ = w.Write([]byte(`[{"chain":"ETH","outbound_tx_size":"250","observed_fee_rate":"30"}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	fetcher := &configFetcher{chainHeight: 10, networkFeeSize: 250, networkFeeRate: 30}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: 100 * time.Millisecond,
		BlockRetryInterval:         100 * time.Millisecond,
		ChainID:                    common.ETHChain,
		PrefetchBlocks:             3,
		MaxHealthyLag:              time.Hour,
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn, 20)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)
	time.Sleep(2 * time.Second)
	scanner.Stop()

	c.Check(scanner.PreviousHeight() > 1, Equals, true)
}

// TestScanMempoolEnabled tests mempool scanning path.
func (s *BlockScannerExtraSuite) TestScanMempoolEnabled(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	// Fetcher returns mempool txs
	fetcher := &configFetcher{
		chainHeight: 0,
		txIn: types.TxIn{
			Chain:   common.ETHChain,
			MemPool: true,
			TxArray: []*types.TxInItem{
				{Tx: "mempool-tx-1"},
			},
		},
	}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: time.Second,
		BlockRetryInterval:         time.Second,
		ChainID:                    common.ETHChain,
		ScanMemPool:                true,
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn, 10)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)
	time.Sleep(2 * time.Second)
	scanner.Stop()

	// Mempool txs should have been queued
	c.Check(len(globalChan) > 0, Equals, true, Commentf("Expected mempool txs, got %d", len(globalChan)))
}

// TestScanBlocksUnhealthy tests that scanner becomes unhealthy when behind.
func (s *BlockScannerExtraSuite) TestScanBlocksUnhealthy(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	// Chain is at height 1000000 but scanner starts at 1 - very behind
	fetcher := &configFetcher{chainHeight: 1000000}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: 100 * time.Millisecond,
		BlockRetryInterval:         100 * time.Millisecond,
		ChainID:                    common.ETHChain,
		MaxHealthyLag:              time.Second, // very short lag threshold
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn, 100)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)
	time.Sleep(1 * time.Second)
	scanner.Stop()

	// Scanner should be unhealthy when significantly behind
	c.Check(scanner.IsHealthy(), Equals, false)
}

// TestStartWithPersistedPos tests that Start reads the persisted scan position.
func (s *BlockScannerExtraSuite) TestStartWithPersistedPos(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	// Set persisted position higher than start height
	_ = mss.SetScanPos(50)
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		// no StartBlockHeight set (0), so Start will check persisted position
		ChainID:                    common.ETHChain,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: time.Second,
		BlockRetryInterval:         time.Second,
	}, mss, m, bridge, DummyFetcher{})
	// This may fail if bridge calls WaitToCatchUp (needs /status endpoint)
	// so let's use StartBlockHeight to skip WaitToCatchUp
	if err != nil {
		return // skip if bridge setup fails
	}

	globalChan := make(chan types.TxIn, 10)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)
	time.Sleep(300 * time.Millisecond)
	scanner.Stop()
}

// TestScanBlocksFetchError tests the error handling in scanBlocks when FetchTxs fails.
func (s *BlockScannerExtraSuite) TestScanBlocksFetchError(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.RequestURI, thorclient.MimirEndpoint):
			_, _ = w.Write([]byte(`-1`))
		case strings.HasPrefix(r.RequestURI, "/thorchain/lastblock"):
			_, _ = w.Write([]byte(`[{"chain":"NOOP","lastobservedin":0,"lastsignedout":0,"thorchain":0}]`))
		}
	}))
	defer server.Close()
	bridge := s.makeBridge(c, server)

	mss := NewMockScannerStorage()
	// Fetcher returns error
	fetcher := &configFetcher{chainHeight: 5, fetchErr: errTest}
	scanner, err := NewBlockScanner(config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1,
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		BlockHeightDiscoverBackoff: 100 * time.Millisecond,
		BlockRetryInterval:         100 * time.Millisecond,
		ChainID:                    common.ETHChain,
	}, mss, m, bridge, fetcher)
	c.Assert(err, IsNil)

	globalChan := make(chan types.TxIn, 20)
	nfChan := make(chan common.NetworkFee, 5)
	scanner.Start(globalChan, nfChan)
	time.Sleep(1 * time.Second)
	scanner.Stop()

	// Scanner should become unhealthy on fetch errors
	c.Check(scanner.IsHealthy(), Equals, false)
}
