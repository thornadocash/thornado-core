package solana

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/mr-tron/base58"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/solana/rpc"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/common/crypto/ed25519"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// -------------------------------------------------------------------------
// BackfillSuite — tests for SOLClient and SOLScanner methods
// -------------------------------------------------------------------------

type BackfillSuite struct {
	m      *metrics.Metrics
	bridge thorclient.ThorchainBridge
	keys   *thorclient.Keys
}

var _ = Suite(&BackfillSuite{})

func (s *BackfillSuite) SetUpSuite(c *C) {
	thorchain.SetupConfigForTest()
	s.m = GetMetricForTest(c)
	c.Assert(s.m, NotNil)
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob-ecdsa",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	thorKeys := thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	s.keys = thorKeys
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, thorKeys)
	c.Assert(err, IsNil)
}

// helper to create a scanner with a mock RPC server
func (s *BackfillSuite) newScannerWithServer(c *C, serverURL string) (*SOLScanner, *signercache.CacheManager) {
	storage, err := blockscanner.NewBlockScannerStorageSolana("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	scm, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)
	pubKeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)

	cfg := getConfigForTest(serverURL)
	rpcClient := rpc.NewSolRPC(serverURL, cfg.HTTPRequestTimeout)
	scanner, err := NewSOLScanner(make(chan struct{}), cfg, storage, s.bridge, s.m, rpcClient, pubKeyMgr, nil, scm)
	c.Assert(err, IsNil)
	return scanner, scm
}

// helper to make a simple mock server for SOL RPC calls
func makeSolRPCServer(c *C, handlers map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.RequestURI {
		case thorclient.ChainVersionEndpoint:
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"current":"` + types2.GetCurrentVersion().String() + `"}`))
		case thorclient.PubKeysEndpoint:
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(`{"asgard":[]}`))
		default:
			body, err := io.ReadAll(req.Body)
			c.Assert(err, IsNil)
			var rpcReq struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(body, &rpcReq)

			rw.Header().Set("Content-Type", "application/json")
			if resp, ok := handlers[rpcReq.Method]; ok {
				_, _ = rw.Write([]byte(resp))
			} else {
				_, _ = rw.Write([]byte(`{"jsonrpc":"2.0","result":null,"id":1}`))
			}
		}
	}))
}

type mockSOLSolvencyBridge struct {
	thorclient.ThorchainBridge
	asgards types2.Vaults
}

func (m *mockSOLSolvencyBridge) GetAsgards() (types2.Vaults, error) {
	return m.asgards, nil
}

// -------------------------------------------------------------------------
// NewSOLScanner error paths
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestNewSOLScannerNilStorage(c *C) {
	rpcClient := rpc.NewSolRPC("http://localhost", time.Second)
	pubKeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	_, err = NewSOLScanner(make(chan struct{}), config.BifrostBlockScannerConfiguration{}, nil, s.bridge, s.m, rpcClient, pubKeyMgr, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "storage is nil")
}

func (s *BackfillSuite) TestNewSOLScannerNilMetrics(c *C) {
	storage, err := blockscanner.NewBlockScannerStorageSolana("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	pubKeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	rpcClient := rpc.NewSolRPC("http://localhost", time.Second)
	_, err = NewSOLScanner(make(chan struct{}), config.BifrostBlockScannerConfiguration{}, storage, s.bridge, nil, rpcClient, pubKeyMgr, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "metrics manager is nil")
}

func (s *BackfillSuite) TestNewSOLScannerNilPubKeyMgr(c *C) {
	storage, err := blockscanner.NewBlockScannerStorageSolana("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	rpcClient := rpc.NewSolRPC("http://localhost", time.Second)
	_, err = NewSOLScanner(make(chan struct{}), config.BifrostBlockScannerConfiguration{}, storage, s.bridge, s.m, rpcClient, nil, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "pubkey manager is nil")
}

// -------------------------------------------------------------------------
// SOLScanner methods
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestScannerGetHeight(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":12345,"id":1}`,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	height, err := scanner.GetHeight()
	c.Assert(err, IsNil)
	c.Assert(height, Equals, int64(12345))
}

func (s *BackfillSuite) TestScannerGetHeightError(c *C) {
	rpcClient := rpc.NewSolRPC("http://localhost:1", time.Millisecond)
	storage, err := blockscanner.NewBlockScannerStorageSolana("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	scm, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)
	pubKeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	cfg := getConfigForTest("http://localhost:1")
	scanner, err := NewSOLScanner(make(chan struct{}), cfg, storage, s.bridge, s.m, rpcClient, pubKeyMgr, nil, scm)
	c.Assert(err, IsNil)

	_, err = scanner.GetHeight()
	c.Assert(err, NotNil)
}

func (s *BackfillSuite) TestScannerScanHeightFromMemory(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	scanner.lastHeightMu.Lock()
	scanner.lastHeight = 999
	scanner.lastHeightMu.Unlock()

	h, err := scanner.ScanHeight()
	c.Assert(err, IsNil)
	c.Assert(h, Equals, int64(999))
}

func (s *BackfillSuite) TestScannerScanHeightFromDB(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	// lastHeight is 0, so it falls through to db
	h, err := scanner.ScanHeight()
	c.Assert(err, IsNil)
	c.Assert(h, Equals, int64(0)) // nothing stored yet
}

func (s *BackfillSuite) TestScannerIsHealthy(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	c.Assert(scanner.IsHealthy(), Equals, false)
	scanner.healthy.Store(true)
	c.Assert(scanner.IsHealthy(), Equals, true)
}

func (s *BackfillSuite) TestScannerGetSetRecentBlockHash(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	c.Assert(scanner.getRecentBlockHash(), Equals, "")
	scanner.setRecentBlockHash("abc123")
	c.Assert(scanner.getRecentBlockHash(), Equals, "abc123")
}

func (s *BackfillSuite) TestScannerGetNetworkFee(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	scanner.lastFeeRate = 5000
	size, rate := scanner.GetNetworkFee()
	c.Assert(size, Equals, uint64(1))
	c.Assert(rate, Equals, uint64(5000))
}

// -------------------------------------------------------------------------
// calculateFeeRatesFromTransactions
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestCalculateFeeRatesFromTransactions(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	// empty block - should add base fee
	count := scanner.calculateFeeRatesFromTransactions(nil)
	c.Assert(count, Equals, 0)
	c.Assert(len(scanner.feeRateCache), Equals, 1)
	c.Assert(scanner.feeRateCache[0], Equals, uint64(baseFeePerSigLamports))

	// reset
	scanner.feeRateCache = make([]uint64, 0)

	// single tx with 10000 fee and 1 sig
	txns := []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{
				Signatures: []string{"sig1"},
			},
			Meta: rpc.RPCMeta{
				Fee: 10000,
			},
		},
	}
	count = scanner.calculateFeeRatesFromTransactions(txns)
	c.Assert(count, Equals, 1)
	c.Assert(len(scanner.feeRateCache), Equals, 1)
	c.Assert(scanner.feeRateCache[0], Equals, uint64(10000))

	// reset
	scanner.feeRateCache = make([]uint64, 0)

	// multiple txs - median should be used
	txns = []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig1"}},
			Meta:        rpc.RPCMeta{Fee: 20000},
		},
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig2"}},
			Meta:        rpc.RPCMeta{Fee: 10000},
		},
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig3"}},
			Meta:        rpc.RPCMeta{Fee: 30000},
		},
	}
	count = scanner.calculateFeeRatesFromTransactions(txns)
	c.Assert(count, Equals, 3)
	c.Assert(len(scanner.feeRateCache), Equals, 1)
	c.Assert(scanner.feeRateCache[0], Equals, uint64(20000)) // median of 10k, 20k, 30k
}

func (s *BackfillSuite) TestCalculateFeeRatesSkipsFailedTxs(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	txns := []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig1"}},
			Meta:        rpc.RPCMeta{Fee: 10000, Err: "some error"},
		},
	}
	count := scanner.calculateFeeRatesFromTransactions(txns)
	c.Assert(count, Equals, 0) // skipped
}

func (s *BackfillSuite) TestCalculateFeeRatesFloorToBaseFee(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	// 3 sigs with 6000 fee = 2000 per sig, which is below baseFeePerSigLamports (5000)
	txns := []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig1", "sig2", "sig3"}},
			Meta:        rpc.RPCMeta{Fee: 6000},
		},
	}
	count := scanner.calculateFeeRatesFromTransactions(txns)
	c.Assert(count, Equals, 1)
	c.Assert(scanner.feeRateCache[0], Equals, uint64(baseFeePerSigLamports))
}

func (s *BackfillSuite) TestCalculateFeeRatesSkipsNoSigOrNoFee(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())

	txns := []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{}},
			Meta:        rpc.RPCMeta{Fee: 5000},
		},
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig1"}},
			Meta:        rpc.RPCMeta{Fee: 0},
		},
	}
	count := scanner.calculateFeeRatesFromTransactions(txns)
	c.Assert(count, Equals, 0) // both skipped
}

// -------------------------------------------------------------------------
// sendNetworkFeeIfNecessary
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestSendNetworkFeeIfNecessaryEmptyCache(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.feeRateCache = []uint64{}
	scanner.sendNetworkFeeIfNecessary(100)

	select {
	case <-scanner.globalNetworkFeeQueue:
		c.Fatal("should not have sent network fee")
	default:
		// expected
	}
}

func (s *BackfillSuite) TestSendNetworkFeeIfNecessaryNotEnoughTxsAfterInit(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.netFeeInit = true
	scanner.feeRateCache = []uint64{5000} // less than GasCacheBlocks
	scanner.sendNetworkFeeIfNecessary(100)

	select {
	case <-scanner.globalNetworkFeeQueue:
		c.Fatal("should not have sent network fee")
	default:
		// expected
	}
}

func (s *BackfillSuite) TestSendNetworkFeeIfNecessarySendsInitialFee(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.netFeeInit = false
	scanner.feeRateCache = []uint64{5000}
	scanner.cfg.GasPriceResolution = 1
	scanner.sendNetworkFeeIfNecessary(100)

	select {
	case fee := <-scanner.globalNetworkFeeQueue:
		c.Assert(fee.Chain, Equals, common.SOLChain)
		c.Assert(fee.TransactionSize, Equals, uint64(1))
		c.Assert(fee.TransactionRate, Equals, uint64(5000))
	default:
		c.Fatal("expected network fee to be sent")
	}
	c.Assert(scanner.netFeeInit, Equals, true)
}

func (s *BackfillSuite) TestSendNetworkFeeIfNecessarySkipsZeroFee(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.netFeeInit = false
	scanner.feeRateCache = []uint64{0}
	scanner.cfg.GasPriceResolution = 1
	scanner.sendNetworkFeeIfNecessary(100)

	select {
	case <-scanner.globalNetworkFeeQueue:
		c.Fatal("should not have sent zero network fee")
	default:
		// expected
	}
}

func (s *BackfillSuite) TestSendNetworkFeeIfNecessarySkipsSameRate(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.netFeeInit = true
	scanner.lastFeeRate = 5000
	scanner.cfg.GasPriceResolution = 1
	// enough txs to pass the check
	scanner.feeRateCache = make([]uint64, 101)
	for i := range scanner.feeRateCache {
		scanner.feeRateCache[i] = 5000
	}
	scanner.sendNetworkFeeIfNecessary(100)

	select {
	case <-scanner.globalNetworkFeeQueue:
		c.Fatal("should not resend same fee")
	default:
		// expected - fee rate hasn't changed
	}
}

func (s *BackfillSuite) TestSendNetworkFeeIfNecessaryTruncatesCache(c *C) {
	server := makeSolRPCServer(c, map[string]string{})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)

	scanner.netFeeInit = false
	scanner.cfg.GasPriceResolution = 1
	scanner.cfg.GasCacheBlocks = 5
	scanner.feeRateCache = []uint64{1000, 2000, 3000, 4000, 5000, 6000, 7000}
	scanner.sendNetworkFeeIfNecessary(100)

	// should truncate to last 5
	c.Assert(len(scanner.feeRateCache), Equals, 5)
}

// -------------------------------------------------------------------------
// getTxIn
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestGetTxIn(c *C) {
	scanner := newTestScanner()

	scanner.cfg.ChainID = common.SOLChain

	// empty transactions
	txIn := scanner.getTxIn(100, nil)
	c.Assert(len(txIn.TxArray), Equals, 0)
	c.Assert(txIn.Chain, Equals, common.SOLChain)

	// tx with no signatures - skipped
	txns := []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{}},
			Meta:        rpc.RPCMeta{Fee: 5000},
		},
	}
	txIn = scanner.getTxIn(100, txns)
	c.Assert(len(txIn.TxArray), Equals, 0)

	// tx with no fee - skipped
	txns = []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{Signatures: []string{"sig1"}},
			Meta:        rpc.RPCMeta{Fee: 0},
		},
	}
	txIn = scanner.getTxIn(100, txns)
	c.Assert(len(txIn.TxArray), Equals, 0)

	// valid transfer
	oneSol := uint64(1_000_000_000)
	txns = []*rpc.TransactionResult{
		{
			Transaction: rpc.RPCTxnData{
				Signatures: []string{testTxSig},
				Message: rpc.RPCMessage{
					AccountKeys: []string{testSender, testVault, testSystemProgram, testMemoProgram},
					Instructions: []rpc.RPCInstruction{
						{ProgramIdIndex: 2, Accounts: []int{0, 1}, Data: makeTransferData(oneSol)},
						{ProgramIdIndex: 3, Accounts: []int{0}, Data: makeMemoData("test memo")},
					},
				},
			},
			Meta: rpc.RPCMeta{Fee: 5000},
		},
	}
	txIn = scanner.getTxIn(100, txns)
	c.Assert(len(txIn.TxArray), Equals, 1)
	c.Assert(txIn.TxArray[0].Memo, Equals, "test memo")
}

// -------------------------------------------------------------------------
// convertToLamports / convertLamportsToTHORChain
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestConvertToLamports(c *C) {
	// 1 SOL in thorchain = 1e8
	amount := cosmos.NewUint(100_000_000)
	lamports := convertToLamports(amount)
	// 1e8 * 10 = 1e9
	c.Assert(lamports.Cmp(big.NewInt(1_000_000_000)), Equals, 0)
}

func (s *BackfillSuite) TestConvertLamportsToTHORChain(c *C) {
	// 1e9 lamports = 1 SOL = 1e8 in thorchain
	result := convertLamportsToTHORChain(1_000_000_000)
	c.Assert(result.Equal(cosmos.NewUint(100_000_000)), Equals, true)

	// 5000 lamports -> 500 in thorchain
	result = convertLamportsToTHORChain(5000)
	c.Assert(result.Equal(cosmos.NewUint(500)), Equals, true)
}

// -------------------------------------------------------------------------
// calculateMedian additional cases
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestCalculateMedianEvenCount(c *C) {
	arr := []uint64{100, 200, 300, 400}
	// sorted: 100, 200, 300, 400 -> median = (200+300)/2 = 250
	c.Assert(calculateMedian(arr), Equals, uint64(250))
}

func (s *BackfillSuite) TestCalculateMedianSingleElement(c *C) {
	c.Assert(calculateMedian([]uint64{42}), Equals, uint64(42))
}

// -------------------------------------------------------------------------
// SOLClient methods (using full client setup)
// -------------------------------------------------------------------------

type ClientBackfillSuite struct {
	m      *metrics.Metrics
	bridge thorclient.ThorchainBridge
	keys   *thorclient.Keys
}

var _ = Suite(&ClientBackfillSuite{})

func (s *ClientBackfillSuite) SetUpSuite(c *C) {
	thorchain.SetupConfigForTest()
	s.m = GetMetricForTest(c)
	c.Assert(s.m, NotNil)

	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "backfill-signer",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc, func(options *cKeys.Options) {
		options.SupportedAlgos = append(options.SupportedAlgos, ed25519.Ed25519)
	})

	edName := ed25519.SignerNameEDDSA(cfg.SignerName)
	_, mnemonic, err := kb.NewMnemonic(edName, cKeys.English, "", cfg.SignerPasswd, ed25519.Ed25519)
	c.Assert(err, IsNil)
	_, err = kb.NewAccount(cfg.SignerName, mnemonic, cfg.SignerPasswd, cmd.THORChainHDPath, hd.Secp256k1)
	c.Assert(err, IsNil)

	s.keys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
}

func (s *ClientBackfillSuite) makeClient(c *C, server *httptest.Server) *SOLClient {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	client, err := NewSOLClient(s.keys,
		config.BifrostChainConfiguration{
			ChainID: common.SOLChain,
			RPCHost: "http://" + server.Listener.Addr().String(),
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:            common.SOLChain,
				StartBlockHeight:   1,
				HTTPRequestTimeout: time.Second,
				MaxGasLimit:        15000,
				GasCacheBlocks:     100,
				GasPriceResolution: 1,
			},
			SolvencyBlocks: 10,
		}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(client, NotNil)
	return client
}

// -------------------------------------------------------------------------
// NewSOLClient error paths
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestNewSOLClientNilKeys(c *C) {
	_, err := NewSOLClient(nil, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "thor keys empty")
}

func (s *ClientBackfillSuite) TestNewSOLClientNilBridge(c *C) {
	_, err := NewSOLClient(s.keys, config.BifrostChainConfiguration{}, nil, nil, s.m, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "thorchain bridge is nil")
}

func (s *ClientBackfillSuite) TestNewSOLClientNilPubKeyMgr(c *C) {
	_, err := NewSOLClient(s.keys, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "pubkey manager is nil")
}

func (s *ClientBackfillSuite) TestNewSOLClientNilPoolMgr(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	_, err = NewSOLClient(s.keys, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, pubkeyMgr, nil)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "pool manager is nil")
}

// -------------------------------------------------------------------------
// Simple getters
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestGetChain(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	c.Assert(client.GetChain(), Equals, common.SOLChain)
}

func (s *ClientBackfillSuite) TestGetConfig(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	cfg := client.GetConfig()
	c.Assert(cfg.ChainID, Equals, common.SOLChain)
}

func (s *ClientBackfillSuite) TestGetHeight(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":54321,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	h, err := client.GetHeight()
	c.Assert(err, IsNil)
	c.Assert(h, Equals, int64(54321))
}

func (s *ClientBackfillSuite) TestGetBlockScannerHeight(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.solScanner.lastHeightMu.Lock()
	client.solScanner.lastHeight = 777
	client.solScanner.lastHeightMu.Unlock()
	h, err := client.GetBlockScannerHeight()
	c.Assert(err, IsNil)
	c.Assert(h, Equals, int64(777))
}

func (s *ClientBackfillSuite) TestRollbackBlockScanner(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	c.Assert(client.RollbackBlockScanner(), IsNil)
}

func (s *ClientBackfillSuite) TestIsBlockScannerHealthy(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	c.Assert(client.IsBlockScannerHealthy(), Equals, false)
	client.solScanner.healthy.Store(true)
	c.Assert(client.IsBlockScannerHealthy(), Equals, true)
}

func (s *ClientBackfillSuite) TestGetConfirmationCount(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	c.Assert(client.GetConfirmationCount(stypes.TxIn{}), Equals, int64(0))
}

func (s *ClientBackfillSuite) TestConfirmationCountReady(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	c.Assert(client.ConfirmationCountReady(stypes.TxIn{}), Equals, true)
}

// -------------------------------------------------------------------------
// GetAddress
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestGetAddress(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// empty pubkey
	addr := client.GetAddress("")
	c.Assert(addr, Equals, "")
}

// -------------------------------------------------------------------------
// GetAccount / GetAccountByAddress
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestGetAccountByAddress(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":    `{"jsonrpc":"2.0","result":1,"id":1}`,
		"getBalance": `{"jsonrpc":"2.0","result":{"context":{"slot":1},"value":1000000000},"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	acct, err := client.GetAccountByAddress("someaddr", nil)
	c.Assert(err, IsNil)
	// 1e9 lamports / 10 = 1e8 thorchain units
	c.Assert(acct.Coins[0].Asset, Equals, common.SOLAsset)
	c.Assert(acct.Coins[0].Amount.Equal(cosmos.NewUint(100_000_000)), Equals, true)
}

func (s *ClientBackfillSuite) TestGetAccountByAddressError(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()

	// close server to force error
	client := s.makeClient(c, server)
	server.Close()
	_, err := client.GetAccountByAddress("someaddr", nil)
	c.Assert(err, NotNil)
}

func (s *ClientBackfillSuite) TestGetAccountBadPubKey(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	_, err := client.GetAccount("", nil)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------
// GetLatestTxForVault
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestGetLatestTxForVault(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// When no tx has been recorded, GetLatestRecordedTx returns empty string with leveldb not found error
	observed, broadcasted, err := client.GetLatestTxForVault("testvault")
	// err may be leveldb not found, which is expected
	if err != nil {
		c.Assert(observed, Equals, "")
	}
	_ = broadcasted
}

// -------------------------------------------------------------------------
// ShouldReportSolvency
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestShouldReportSolvency(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// SolvencyBlocks = 10
	c.Assert(client.ShouldReportSolvency(0), Equals, false)

	// solvencySlotMultiple is 0, so threshold is (0+1)*10 = 10
	c.Assert(client.ShouldReportSolvency(11), Equals, true)
	c.Assert(client.ShouldReportSolvency(10), Equals, false)

	// test with SolvencyBlocks = 0
	client.cfg.SolvencyBlocks = 0
	c.Assert(client.ShouldReportSolvency(100), Equals, false)

	// negative
	client.cfg.SolvencyBlocks = -1
	c.Assert(client.ShouldReportSolvency(100), Equals, false)
}

// -------------------------------------------------------------------------
// BroadcastTx
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestBroadcastTx(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":         `{"jsonrpc":"2.0","result":1,"id":1}`,
		"sendTransaction": `{"jsonrpc":"2.0","result":"txsig123","id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	txOutItem := stypes.TxOutItem{
		Chain:   common.SOLChain,
		Memo:    "test",
		Coins:   common.Coins{common.NewCoin(common.SOLAsset, cosmos.NewUint(1000))},
		MaxGas:  common.Gas{common.NewCoin(common.SOLAsset, cosmos.NewUint(5000))},
		GasRate: 1,
	}
	sig, err := client.BroadcastTx(txOutItem, []byte("rawbytes"))
	c.Assert(err, IsNil)
	c.Assert(sig, Equals, "txsig123")
}

func (s *ClientBackfillSuite) TestBroadcastTxError(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":         `{"jsonrpc":"2.0","result":1,"id":1}`,
		"sendTransaction": `{"jsonrpc":"2.0","error":{"code":-1,"message":"tx failed"},"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	_, err := client.BroadcastTx(stypes.TxOutItem{}, []byte("rawbytes"))
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------
// OnObservedTxIn
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestOnObservedTxInInvalidMemo(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// invalid memo - should not panic
	client.OnObservedTxIn(stypes.TxInItem{Memo: "not-a-valid-memo"}, 100)
}

func (s *ClientBackfillSuite) TestOnObservedTxInNonOutbound(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// inbound swap memo - should return early (not outbound)
	client.OnObservedTxIn(stypes.TxInItem{Memo: "=:ETH.ETH:0x1234"}, 100)
}

func (s *ClientBackfillSuite) TestOnObservedTxInOutbound(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// valid outbound memo
	txHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	client.OnObservedTxIn(stypes.TxInItem{
		Memo: fmt.Sprintf("OUT:%s", txHash),
		Tx:   "sometxsig",
	}, 100)
}

func (s *ClientBackfillSuite) TestOnObservedTxInEmptyTxID(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// outbound memo with empty txid - should return early
	client.OnObservedTxIn(stypes.TxInItem{Memo: "OUT:"}, 100)
}

// -------------------------------------------------------------------------
// SignTx error paths
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestSignTxNotSOLChain(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	_, _, _, err := client.SignTx(stypes.TxOutItem{Chain: common.BTCChain}, 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "tx chain is not solana")
}

func (s *ClientBackfillSuite) TestSignTxEmptyToAddress(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	_, _, _, err := client.SignTx(stypes.TxOutItem{Chain: common.SOLChain, ToAddress: ""}, 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "to address is empty")
}

func (s *ClientBackfillSuite) TestSignTxEmptyVaultPubKey(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	_, _, _, err := client.SignTx(stypes.TxOutItem{
		Chain:     common.SOLChain,
		ToAddress: "D9A6eE2pZ6oSiGb8BPkag4gvAeEhHvc3eEYPBoLSMshG",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "vault public key is empty")
}

// -------------------------------------------------------------------------
// CreateTx error paths
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestCreateTxEmptyBlockhash(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// recentBlockHash is empty
	client.solScanner.setRecentBlockHash("")
	_, err := client.CreateTx(stypes.TxOutItem{}, "")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "recent blockhash is empty")
}

func (s *ClientBackfillSuite) TestCreateTxInboundMemo(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.solScanner.setRecentBlockHash("blockhash123")

	// Use ADD memo which is inbound and parses successfully
	_, err := client.CreateTx(stypes.TxOutItem{
		Coins: common.Coins{common.NewCoin(common.SOLAsset, cosmos.NewUint(1000))},
	}, "ADD:ETH.ETH")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "inbound memo should not be used for outbound tx")
}

func (s *ClientBackfillSuite) TestCreateTxMultipleCoins(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.solScanner.setRecentBlockHash("blockhash123")

	txHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := client.CreateTx(stypes.TxOutItem{
		Coins: common.Coins{
			common.NewCoin(common.SOLAsset, cosmos.NewUint(1000)),
			common.NewCoin(common.SOLAsset, cosmos.NewUint(2000)),
		},
	}, fmt.Sprintf("OUT:%s", txHash))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "only support one coin per transaction, received: 2")
}

func (s *ClientBackfillSuite) TestCreateTxZeroAmount(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.solScanner.setRecentBlockHash("blockhash123")

	// Use a known test pubkey
	testVaultPubKey := common.PubKey("tthorpub1zcjduepq829qzzhllcktltnfdudyupw5qjtyyjzcmv9wx2jvpah4d6nxtl2s7vwca8")

	txHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, err := client.CreateTx(stypes.TxOutItem{
		VaultPubKeyEddsa: testVaultPubKey,
		ToAddress:        "D9A6eE2pZ6oSiGb8BPkag4gvAeEhHvc3eEYPBoLSMshG",
		Coins:            common.Coins{common.NewCoin(common.SOLAsset, cosmos.NewUint(0))},
	}, fmt.Sprintf("OUT:%s", txHash))
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "sol value is zero")
}

func (s *ClientBackfillSuite) TestCreateTxEmptyMemoDefaultsToOutbound(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.solScanner.setRecentBlockHash("blockhash123")

	testVaultPubKey := common.PubKey("tthorpub1zcjduepq829qzzhllcktltnfdudyupw5qjtyyjzcmv9wx2jvpah4d6nxtl2s7vwca8")

	txHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tx, err := client.CreateTx(stypes.TxOutItem{
		VaultPubKeyEddsa: testVaultPubKey,
		ToAddress:        "D9A6eE2pZ6oSiGb8BPkag4gvAeEhHvc3eEYPBoLSMshG",
		Coins:            common.Coins{common.NewCoin(common.SOLAsset, cosmos.NewUint(1000))},
		InHash:           common.TxID(txHash),
		Memo:             "test memo",
	}, "")
	c.Assert(err, IsNil)
	c.Assert(tx.Message.Accounts, HasLen, 4)
}

// -------------------------------------------------------------------------
// ReportSolvency
// -------------------------------------------------------------------------

func (s *ClientBackfillSuite) TestReportSolvencyNotTime(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot": `{"jsonrpc":"2.0","result":1,"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)

	// height 0 should not report
	err := client.ReportSolvency(0)
	c.Assert(err, IsNil)
}

func (s *ClientBackfillSuite) TestReportSolvencyUnhealthyScannerSameHeight(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":    `{"jsonrpc":"2.0","result":1,"id":1}`,
		"getBalance": `{"jsonrpc":"2.0","result":{"context":{"slot":1},"value":1000000000},"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.globalSolvencyQueue = make(chan stypes.Solvency, 10)

	// Set scanner unhealthy and lastHeight = 100
	client.solScanner.healthy = &atomic.Bool{}
	client.solScanner.healthy.Store(false)
	client.solScanner.lastHeightMu.Lock()
	client.solScanner.lastHeight = 100
	client.solScanner.lastHeightMu.Unlock()

	// height = 100 (same as lastHeight), scanner unhealthy -> skip
	err := client.ReportSolvency(100)
	c.Assert(err, IsNil)
}

func (s *ClientBackfillSuite) TestReportSolvencyAllSolventIgnoresUnrelatedVaults(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":    `{"jsonrpc":"2.0","result":11,"id":1}`,
		"getBalance": `{"jsonrpc":"2.0","result":{"context":{"slot":11},"value":10000000000},"id":1}`,
	})
	defer server.Close()
	client := s.makeClient(c, server)
	client.globalSolvencyQueue = make(chan stypes.Solvency, 10)
	client.solScanner.healthy.Store(false)
	client.solScanner.lastFeeRate = 1

	solVault := types2.Vault{
		PubKey:      types2.GetRandomPubKey(),
		PubKeyEddsa: types2.GetRandomEd25519PubKey(),
		Coins:       common.NewCoins(common.NewCoin(common.SOLAsset, cosmos.NewUint(1_000_000_000))),
	}
	unrelatedBTCVault := types2.Vault{
		PubKey: types2.GetRandomPubKey(),
		Coins:  common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000_000))),
	}
	client.bridge = &mockSOLSolvencyBridge{
		asgards: types2.Vaults{solVault, unrelatedBTCVault},
	}

	err := client.ReportSolvency(11)
	c.Assert(err, IsNil)

	select {
	case msg := <-client.globalSolvencyQueue:
		c.Assert(msg.Height, Equals, int64(10))
		c.Assert(msg.Chain, Equals, common.SOLChain)
		c.Assert(msg.PubKey.Equals(solVault.PubKey), Equals, true)
	case <-time.After(time.Second):
		c.Fatal("expected all-solvent report for the SOL vault")
	}

	select {
	case msg := <-client.globalSolvencyQueue:
		c.Fatalf("unexpected extra solvency report for %s", msg.PubKey)
	default:
	}
}

// Note: Full ReportSolvency with vault fetching requires bridge to be pointed at test server.
// The bridge is created once in SetUpSuite and can't be easily repointed. We test the simpler paths above.

// -------------------------------------------------------------------------
// checkRecentBlocksAndUpdateNetworkFee
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestCheckRecentBlocksAndUpdateNetworkFee(c *C) {
	// Create a server that returns blocks with transactions
	blockResponse := `{"jsonrpc":"2.0","result":{"blockhash":"testblockhash","previousBlockhash":"prev","parentSlot":99,"transactions":[` +
		`{"transaction":{"signatures":["sig1"],"message":{"accountKeys":["a","b","c"],"instructions":[],"recentBlockhash":"bh"}},"meta":{"fee":10000,"preBalances":[100,200],"postBalances":[90,210],"status":{"Ok":null}}}` +
		`]},"id":1}`

	blockHeightsResponse := `{"jsonrpc":"2.0","result":[100,101,102,103,104,105,106,107,108,109,110,111,112,113,114,115,116,117,118,119,120,121,122,123,124,125,126,127,128,129,130,131,132,133,134,135,136,137,138,139,140,141,142,143,144,145,146,147,148,149,150,151,152,153,154,155,156,157,158,159,160,161,162,163,164,165,166,167,168,169,170,171,172,173,174,175,176,177,178,179,180,181,182,183,184,185,186,187,188,189,190,191,192,193,194,195,196,197,198,199,200],"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":   `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getBlocks": blockHeightsResponse,
		"getBlock":  blockResponse,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	scanner.cfg.Solana.FeeSampleSlots = 100
	scanner.cfg.GasCacheBlocks = 5

	err := scanner.checkRecentBlocksAndUpdateNetworkFee(200)
	c.Assert(err, IsNil)

	// Should have sent a network fee
	select {
	case fee := <-scanner.globalNetworkFeeQueue:
		c.Assert(fee.Chain, Equals, common.SOLChain)
		c.Assert(fee.TransactionRate > 0, Equals, true)
	default:
		c.Fatal("expected network fee")
	}
}

func (s *BackfillSuite) TestCheckRecentBlocksNotEnoughTxs(c *C) {
	// Returns empty blocks (no transactions)
	blockResponse := `{"jsonrpc":"2.0","result":{"blockhash":"testblockhash","previousBlockhash":"prev","parentSlot":99,"transactions":[]},"id":1}`
	blockHeightsResponse := `{"jsonrpc":"2.0","result":[100],"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":   `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getBlocks": blockHeightsResponse,
		"getBlock":  blockResponse,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	scanner.cfg.Solana.FeeSampleSlots = 100
	scanner.cfg.GasCacheBlocks = 100

	err := scanner.checkRecentBlocksAndUpdateNetworkFee(200)
	c.Assert(err, NotNil) // "not enough transactions"
}

func (s *BackfillSuite) TestCheckRecentBlocksNilBlock(c *C) {
	// Returns null block (skipped slot)
	blockHeightsResponse := `{"jsonrpc":"2.0","result":[100],"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":   `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getBlocks": blockHeightsResponse,
		"getBlock":  fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":-1,"message":"Slot %d was skipped, or missing due to ledger jump to recent snapshot"},"id":1}`, 100),
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	scanner.cfg.Solana.FeeSampleSlots = 100
	scanner.cfg.GasCacheBlocks = 100

	err := scanner.checkRecentBlocksAndUpdateNetworkFee(200)
	c.Assert(err, NotNil) // "not enough transactions"
}

func (s *BackfillSuite) TestCheckRecentBlocksBlockStatusNotAvailable(c *C) {
	blockHeightsResponse := `{"jsonrpc":"2.0","result":[100],"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":   `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getBlocks": blockHeightsResponse,
		"getBlock":  `{"jsonrpc":"2.0","error":{"code":-1,"message":"Block status not yet available for slot 100"},"id":1}`,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	scanner.cfg.Solana.FeeSampleSlots = 100
	scanner.cfg.GasCacheBlocks = 100

	err := scanner.checkRecentBlocksAndUpdateNetworkFee(200)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "block status not yet available for slot 100")
}

func (s *BackfillSuite) TestCheckRecentBlocksGetBlocksError(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":   `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getBlocks": `{"jsonrpc":"2.0","error":{"code":-1,"message":"rpc error"},"id":1}`,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	scanner.cfg.Solana.FeeSampleSlots = 100

	err := scanner.checkRecentBlocksAndUpdateNetworkFee(200)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------
// scanVault
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestScanVaultNoNewTxs(c *C) {
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":                 `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getSignaturesForAddress": `{"jsonrpc":"2.0","result":[],"id":1}`,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalTxsQueue = make(chan stypes.TxIn, 10)
	scanner.cfg.ChainID = common.SOLChain

	var wg sync.WaitGroup
	wg.Add(1)
	txCounts := make([]int, 1)
	successfulQueries := make([]bool, 1)
	newLastSigs := make([]string, 1)
	healthy := true
	setUnhealthy := func() { healthy = false }

	scanner.scanVault("testvault", 0, "", 0, setUnhealthy, txCounts, successfulQueries, newLastSigs, &wg)

	wg.Wait()
	c.Assert(successfulQueries[0], Equals, true)
	c.Assert(txCounts[0], Equals, 0)
	c.Assert(healthy, Equals, true)
}

func (s *BackfillSuite) TestScanVaultWithTxs(c *C) {
	txResult := `{"jsonrpc":"2.0","result":{"slot":100,"transaction":{"signatures":["txsig1"],"message":{"accountKeys":["` +
		testSender + `","` + testVault + `","` + testSystemProgram + `","` + testMemoProgram + `"],"instructions":[` +
		`{"programIdIndex":2,"accounts":[0,1],"data":"` + makeTransferData(1_000_000_000) + `"},` +
		`{"programIdIndex":3,"accounts":[0],"data":"` + makeMemoData("test:memo") + `"}` +
		`],"recentBlockhash":"bh"}},"meta":{"fee":5000,"preBalances":[100,200],"postBalances":[90,210],"status":{"Ok":null}}},"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":                 `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getSignaturesForAddress": `{"jsonrpc":"2.0","result":[{"signature":"txsig1","slot":100}],"id":1}`,
		"getTransaction":          txResult,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalTxsQueue = make(chan stypes.TxIn, 10)
	scanner.cfg.ChainID = common.SOLChain

	var wg sync.WaitGroup
	wg.Add(1)
	txCounts := make([]int, 1)
	successfulQueries := make([]bool, 1)
	newLastSigs := make([]string, 1)
	healthy := true
	setUnhealthy := func() { healthy = false }

	scanner.scanVault("testvault", 0, "", 0, setUnhealthy, txCounts, successfulQueries, newLastSigs, &wg)

	wg.Wait()
	c.Assert(successfulQueries[0], Equals, true)
	c.Assert(txCounts[0], Equals, 1)
	c.Assert(newLastSigs[0], Equals, "txsig1")
	c.Assert(healthy, Equals, true)
}

func (s *BackfillSuite) TestScanVaultGetSigsError(c *C) {
	// Server returns error for getSignaturesForAddress
	server := makeSolRPCServer(c, map[string]string{
		"getSlot":                 `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getSignaturesForAddress": `{"jsonrpc":"2.0","error":{"code":-1,"message":"rpc error"},"id":1}`,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalTxsQueue = make(chan stypes.TxIn, 10)

	var wg sync.WaitGroup
	wg.Add(1)
	txCounts := make([]int, 1)
	successfulQueries := make([]bool, 1)
	newLastSigs := make([]string, 1)
	healthy := true
	setUnhealthy := func() { healthy = false }

	scanner.scanVault("testvault", 0, "", 0, setUnhealthy, txCounts, successfulQueries, newLastSigs, &wg)

	wg.Wait()
	c.Assert(successfulQueries[0], Equals, false) // not successful
	c.Assert(healthy, Equals, false)              // set unhealthy
}

func (s *BackfillSuite) TestScanVaultFailedTxSkipped(c *C) {
	// Transaction with error in meta
	txResult := `{"jsonrpc":"2.0","result":{"slot":100,"transaction":{"signatures":["txsig1"],"message":{"accountKeys":["a"],"instructions":[],"recentBlockhash":"bh"}},"meta":{"fee":5000,"err":"InstructionError","preBalances":[100],"postBalances":[90],"status":{"Ok":null}}},"id":1}`

	server := makeSolRPCServer(c, map[string]string{
		"getSlot":                 `{"jsonrpc":"2.0","result":200,"id":1}`,
		"getSignaturesForAddress": `{"jsonrpc":"2.0","result":[{"signature":"txsig1","slot":100}],"id":1}`,
		"getTransaction":          txResult,
	})
	defer server.Close()
	scanner, _ := s.newScannerWithServer(c, "http://"+server.Listener.Addr().String())
	scanner.globalTxsQueue = make(chan stypes.TxIn, 10)
	scanner.cfg.ChainID = common.SOLChain

	var wg sync.WaitGroup
	wg.Add(1)
	txCounts := make([]int, 1)
	successfulQueries := make([]bool, 1)
	newLastSigs := make([]string, 1)

	scanner.scanVault("testvault", 0, "", 0, func() {}, txCounts, successfulQueries, newLastSigs, &wg)

	wg.Wait()
	c.Assert(successfulQueries[0], Equals, true)
	c.Assert(txCounts[0], Equals, 1)
}

// -------------------------------------------------------------------------
// getTxInItem additional edge cases
// -------------------------------------------------------------------------

func (s *BackfillSuite) TestGetTxInItemBadMemoDecodeError(c *C) {
	scanner := newTestScanner()
	scanner.cfg.ChainID = common.SOLChain

	// Create memo data that's not valid base58
	txn := &rpc.TransactionResult{
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram, testMemoProgram},
				Instructions: []rpc.RPCInstruction{
					{ProgramIdIndex: 2, Accounts: []int{0, 1}, Data: makeTransferData(1_000_000_000)},
					{ProgramIdIndex: 3, Accounts: []int{0}, Data: "!!!invalid-base58!!!"},
				},
			},
		},
		Meta: rpc.RPCMeta{Fee: 5000},
	}

	// Should still parse the transfer even if memo decode fails
	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, NotNil)
	c.Assert(item.Memo, Equals, "") // memo failed to decode
}

func (s *BackfillSuite) TestGetTxInItemBadTransferDataDecode(c *C) {
	scanner := newTestScanner()
	scanner.cfg.ChainID = common.SOLChain

	txn := &rpc.TransactionResult{
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{ProgramIdIndex: 2, Accounts: []int{0, 1}, Data: "!!!bad-base58!!!"},
				},
			},
		},
		Meta: rpc.RPCMeta{Fee: 5000},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil) // no valid transfer
}

func (s *BackfillSuite) TestGetTxInItemInsufficientAccounts(c *C) {
	scanner := newTestScanner()
	scanner.cfg.ChainID = common.SOLChain

	txn := &rpc.TransactionResult{
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{ProgramIdIndex: 1, Accounts: []int{0}, Data: makeTransferData(1_000_000_000)},
				},
			},
		},
		Meta: rpc.RPCMeta{Fee: 5000},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil) // not enough accounts
}

func (s *BackfillSuite) TestGetTxInItemNonTransferSystemInstruction(c *C) {
	scanner := newTestScanner()
	scanner.cfg.ChainID = common.SOLChain

	// System program instruction with opcode != 2 (e.g., CreateAccount = 0)
	data := make([]byte, 12)
	data[0] = 0 // not a transfer

	txn := &rpc.TransactionResult{
		Transaction: rpc.RPCTxnData{
			Signatures: []string{testTxSig},
			Message: rpc.RPCMessage{
				AccountKeys: []string{testSender, testVault, testSystemProgram},
				Instructions: []rpc.RPCInstruction{
					{ProgramIdIndex: 2, Accounts: []int{0, 1}, Data: base58.Encode(data)},
				},
			},
		},
		Meta: rpc.RPCMeta{Fee: 5000},
	}

	item := scanner.getTxInItem(txn, 100)
	c.Assert(item, IsNil) // not a transfer instruction
}

// -------------------------------------------------------------------------
// ReportSolvency with asgard vaults
// -------------------------------------------------------------------------

// Note: TestReportSolvencyWithVaults and TestReportSolvencyEmptyPubKey require
// bridge.GetAsgards() to reach our test server. The bridge is constructed once in
// SetUpSuite pointing to localhost, so these deep solvency paths aren't testable
// without modifying the bridge or production code.
