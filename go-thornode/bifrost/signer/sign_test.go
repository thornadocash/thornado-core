package signer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver"
	"github.com/cometbft/cometbft/crypto"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/rs/zerolog/log"
	tssMessages "gitlab.com/thorchain/thornode/v3/bifrost/p2p/messages"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/blame"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/keysign"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

////////////////////////////////////////////////////////////////////////////////////////
// Mocks
////////////////////////////////////////////////////////////////////////////////////////

// -------------------------------- bridge ---------------------------------

type fakeBridge struct {
	thorclient.ThorchainBridge
}

func (b fakeBridge) GetBlockHeight() (int64, error) {
	return 100, nil
}

func (b fakeBridge) GetThorchainVersion() (semver.Version, error) {
	return semver.MustParse("1.0.0"), nil
}

func (b fakeBridge) GetConstants() (map[string]int64, error) {
	return map[string]int64{
		constants.SigningTransactionPeriod.String(): 300,
	}, nil
}

func (b fakeBridge) GetMimir(key string) (int64, error) {
	upperKey := strings.ToUpper(key) // Case-insensitive for halt prefix
	if strings.HasPrefix(upperKey, "HALT") {
		return 0, nil
	}
	if key == constants.SignerConcurrency.String() {
		return 3, nil
	}
	if key == "MAXOUTBOUNDATTEMPTS" {
		return 100, nil
	}
	panic("not implemented")
}

func (b fakeBridge) GetMimirWithRef(template, ref string) (int64, error) {
	key := fmt.Sprintf(template, ref)
	return b.GetMimir(key)
}

func (b fakeBridge) GetVault(pubkey string) (types2.Vault, error) {
	pk, err := common.NewPubKey(pubkey)
	if err != nil {
		return types2.Vault{}, err
	}
	return types2.Vault{
		PubKey: pk,
		Status: types2.VaultStatus_ActiveVault,
	}, nil
}

// -------------------------------- tss ---------------------------------

type fakeTssServer struct {
	counter int
	results map[int]keysign.Response
	fixed   *keysign.Response
}

func (tss *fakeTssServer) KeySign(req keysign.Request) (keysign.Response, error) {
	tss.counter += 1

	if tss.fixed != nil {
		return *tss.fixed, nil
	}

	result, ok := tss.results[tss.counter]
	if ok {
		return result, nil
	}

	return keysign.Response{}, fmt.Errorf("unhandled counter")
}

func newFakeTss(msg string, succeedOnly bool) *fakeTssServer {
	success := keysign.Response{
		Status: 1, // 1 is success
		Signatures: []keysign.Signature{
			{
				R:   base64.StdEncoding.EncodeToString([]byte("R")),
				S:   base64.StdEncoding.EncodeToString([]byte("S")),
				Msg: base64.StdEncoding.EncodeToString([]byte(msg)),
			},
		},
	}

	if succeedOnly {
		return &fakeTssServer{
			fixed: &success,
		}
	}

	results := make(map[int]keysign.Response)
	results[1] = keysign.Response{
		Status: 2, // 2 is fail
		Blame: blame.Blame{
			Round: tssMessages.KEYSIGN7,
			BlameNodes: []blame.Node{
				{Pubkey: "node1"},
			},
		},
	}
	results[2] = keysign.Response{
		Status: 2, // 2 is fail
		Blame: blame.Blame{
			Round: tssMessages.KEYSIGN7,
			BlameNodes: []blame.Node{
				{Pubkey: "node2"},
			},
		},
	}
	results[3] = keysign.Response{
		Status: 2, // 2 is fail, as non-round7
		Blame: blame.Blame{
			Round: tssMessages.KEYSIGN3,
			BlameNodes: []blame.Node{
				{Pubkey: "node2"},
			},
		},
	}
	results[4] = keysign.Response{
		Status: 2, // 2 is fail
		Blame: blame.Blame{
			Round: tssMessages.KEYSIGN7,
			BlameNodes: []blame.Node{
				{Pubkey: "node3"},
			},
		},
	}
	results[5] = success
	results[6] = success
	results[7] = success

	return &fakeTssServer{
		counter: 0,
		results: results,
	}
}

// --------------------------------- chain client ---------------------------------

type MockChainClient struct {
	account            common.Account
	signCount          int
	broadcastCount     int
	ks                 *tss.KeySign
	assertCheckpoint   bool
	broadcastFailCount int
}

func (b *MockChainClient) IsBlockScannerHealthy() bool {
	return true
}

func (b *MockChainClient) SignTx(tai types.TxOutItem, height int64) ([]byte, []byte, *types.TxInItem, error) {
	if b.ks == nil {
		return nil, nil, nil, nil
	}

	// assert that this signing should have the checkpoint set
	if b.assertCheckpoint {
		if !bytes.Equal(tai.Checkpoint, []byte(tai.Memo)) {
			panic("checkpoint should be set")
		}
	} else {
		if bytes.Equal(tai.Checkpoint, []byte(tai.Memo)) {
			panic("checkpoint should not be set")
		}
	}

	b.signCount += 1
	sig, _, err := b.ks.RemoteSign([]byte(tai.Memo), common.SigningAlgoSecp256k1, tai.VaultPubKey.String())

	return sig, []byte(tai.Memo), nil, err
}

func (b *MockChainClient) GetConfig() config.BifrostChainConfiguration {
	return config.BifrostChainConfiguration{}
}

func (b *MockChainClient) GetHeight() (int64, error) {
	return 0, nil
}

func (b *MockChainClient) GetGasFee(count uint64) common.Gas {
	return common.Gas{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
	}
}

func (b *MockChainClient) CheckIsTestNet() (string, bool) {
	return "", true
}

func (b *MockChainClient) GetChain() common.Chain {
	return common.ETHChain
}

func (b *MockChainClient) GetBlockScannerHeight() (int64, error) {
	return 0, nil
}

// RollbackBlockScanner rolls back the block scanner to the last observed block
func (c *MockChainClient) RollbackBlockScanner() error {
	return nil
}

func (b *MockChainClient) GetLatestTxForVault(vault string) (string, string, error) {
	return "", "", nil
}

func (b *MockChainClient) Churn(pubKey common.PubKey, height int64) error {
	return nil
}

func (b *MockChainClient) BroadcastTx(_ types.TxOutItem, tx []byte) (string, error) {
	b.broadcastCount += 1
	if b.broadcastCount > b.broadcastFailCount {
		return "", nil
	}
	return "", fmt.Errorf("broadcast failed")
}

func (b *MockChainClient) GetAddress(poolPubKey common.PubKey) string {
	return "0dd3d0a4a6eacc98cc4894791702e46c270bde76"
}

func (b *MockChainClient) GetAccount(poolPubKey common.PubKey, _ *big.Int) (common.Account, error) {
	return b.account, nil
}

func (b *MockChainClient) GetAccountByAddress(address string, _ *big.Int) (common.Account, error) {
	return b.account, nil
}

func (b *MockChainClient) GetPubKey() crypto.PubKey {
	return nil
}

func (b *MockChainClient) OnObservedTxIn(txIn types.TxInItem, blockHeight int64) {
}

func (b *MockChainClient) Start(globalTxsQueue chan types.TxIn, globalErrataQueue chan types.ErrataBlock, globalSolvencyQueue chan types.Solvency, globalNetworkFeeQueue chan common.NetworkFee) {
}

func (b *MockChainClient) Stop() {}
func (b *MockChainClient) ConfirmationCountReady(txIn types.TxIn) bool {
	return true
}

func (b *MockChainClient) GetConfirmationCount(txIn types.TxIn) int64 {
	return 0
}

////////////////////////////////////////////////////////////////////////////////////////
// Tests
////////////////////////////////////////////////////////////////////////////////////////

var m *metrics.Metrics

func GetMetricForTest(c *C) *metrics.Metrics {
	if m == nil {
		var err error
		m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9000,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.ETHChain},
		})
		c.Assert(m, NotNil)
		c.Assert(err, IsNil)
	}
	return m
}

type SignSuite struct {
	thorKeys *thorclient.Keys
	bridge   thorclient.ThorchainBridge
	metrics  *metrics.Metrics
	rpcHost  string
	storage  *SignerStore
}

var _ = Suite(&SignSuite{})

func (s *SignSuite) SetUpSuite(c *C) {
	thorchain.SetupConfigForTest()
	s.metrics = GetMetricForTest(c)
	c.Assert(s.metrics, NotNil)

	server := httptest.NewServer(
		http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			c.Logf("requestUri:%s", req.RequestURI)
			switch {
			case strings.HasPrefix(req.RequestURI, thorclient.MimirEndpoint):
				buf, err := os.ReadFile("../../test/fixtures/endpoints/mimir/mimir.json")
				c.Assert(err, IsNil)
				_, err = rw.Write(buf)
				c.Assert(err, IsNil)
			case strings.HasPrefix(req.RequestURI, "/thorchain/lastblock"):
				_, err := rw.Write([]byte(`[
          {
            "chain": "NOOP",
            "lastobservedin": 0,
            "lastsignedout": 0,
            "thorchain": 0
          }
        ]`))
				c.Assert(err, IsNil)
			case strings.HasPrefix(req.RequestURI, "/thorchain/keysign"):
				// unused since tests override signer storage, but must return valid json
				_, err := rw.Write([]byte(`{}`))
				c.Assert(err, IsNil)
			}
		}))

	split := strings.SplitAfter(server.URL, ":")
	s.rpcHost = split[len(split)-1]
	cfg := config.BifrostClientConfiguration{
		ChainID:      "thorchain",
		ChainHost:    "localhost:" + s.rpcHost,
		SignerName:   "bob",
		SignerPasswd: "password",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.thorKeys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	c.Assert(err, IsNil)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.metrics, s.thorKeys)
	c.Assert(err, IsNil)
	s.storage, err = NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
}

func (s *SignSuite) TestProcess(c *C) {
	cfg := config.BifrostSignerConfiguration{
		SignerDbPath: filepath.Join(os.TempDir(), "/var/data/bifrost/signer"),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:                    "ThorChain",
			StartBlockHeight:           1,
			EnforceBlockHeight:         true,
			BlockScanProcessors:        1,
			BlockHeightDiscoverBackoff: time.Second,
			BlockRetryInterval:         10 * time.Second,
		},
		RetryInterval: 2 * time.Second,
	}

	chains := map[common.Chain]chainclients.ChainClient{
		common.ETHChain: &MockChainClient{
			account: common.Account{
				Coins: common.Coins{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
					common.NewCoin(common.RuneAsset(), cosmos.NewUint(1000000)),
				},
			},
		},
	}

	blockScan, err := NewThorchainBlockScan(cfg.BlockScanner, s.storage, s.bridge, s.metrics, pubkeymanager.NewMockPoolAddressValidator())
	c.Assert(err, IsNil)

	blockScanner, err := blockscanner.NewBlockScanner(cfg.BlockScanner, s.storage, m, s.bridge, blockScan)
	c.Assert(err, IsNil)

	sign := &Signer{
		logger:                log.With().Str("module", "signer").Logger(),
		cfg:                   config.Bifrost{Signer: cfg},
		wg:                    &sync.WaitGroup{},
		stopChan:              make(chan struct{}),
		blockScanner:          blockScanner,
		thorchainBlockScanner: blockScan,
		chains:                chains,
		m:                     s.metrics,
		storage:               s.storage,
		errCounter:            s.metrics.GetCounterVec(metrics.SignerError),
		pubkeyMgr:             pubkeymanager.NewMockPoolAddressValidator(),
		thorchainBridge:       s.bridge,
	}
	c.Assert(sign, NotNil)
	err = sign.Start()
	c.Assert(err, IsNil)
	time.Sleep(time.Second * 2)
	// nolint
	go sign.Stop()
}

func (s *SignSuite) TestBroadcastRetry(c *C) {
	vaultPubKey, err := common.NewPubKey(pubkeymanager.MockPubkey)
	c.Assert(err, IsNil)

	// start a mock keysign
	msg := "foobar"
	tssServer := newFakeTss(msg, true)
	bridge := fakeBridge{s.bridge}
	ks, err := tss.NewKeySign(tssServer, bridge)
	c.Assert(err, IsNil)
	ks.Start()

	// creat mock chain client and signer
	cc := &MockChainClient{ks: ks, broadcastFailCount: 2}
	sign := &Signer{
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: cc,
		},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		stopChan:            make(chan struct{}),
		wg:                  &sync.WaitGroup{},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
		logger:              log.With().Str("module", "signer").Logger(),
	}

	// create a signer store with fake txouts
	sign.storage, err = NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	err = sign.storage.Set(TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        msg,
			VaultPubKey: vaultPubKey,
			Coins: common.Coins{ // must be set or signer overrides memo
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
	})
	c.Assert(err, IsNil)

	// first attempt should fail broadcast and set signed tx
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 1)
	c.Assert(tssServer.counter, Equals, 1)
	c.Assert(cc.broadcastCount, Equals, 1)
	tois := sign.storage.List()
	c.Assert(err, IsNil)
	c.Assert(tois, HasLen, 1)
	c.Assert(tois[0].Checkpoint, IsNil)
	c.Assert(len(tois[0].SignedTx), Equals, 64)

	// second attempt should not sign and still fail broadcast
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 1)
	c.Assert(tssServer.counter, Equals, 1)
	c.Assert(cc.broadcastCount, Equals, 2)
	tois = sign.storage.List()
	c.Assert(err, IsNil)
	c.Assert(tois, HasLen, 1)
	c.Assert(tois[0].Checkpoint, IsNil)
	c.Assert(len(tois[0].SignedTx), Equals, 64)

	// third attempt should not sign and succeed broadcast
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 1)
	c.Assert(tssServer.counter, Equals, 1)
	c.Assert(cc.broadcastCount, Equals, 3)
	tois = sign.storage.List()
	c.Assert(err, IsNil)
	c.Assert(tois, HasLen, 0)

	// stop signer
	close(sign.stopChan)
	sign.wg.Wait()
	ks.Stop()
}

func (s *SignSuite) TestRound7Retry(c *C) {
	vaultPubKey, err := common.NewPubKey(pubkeymanager.MockPubkey)
	c.Assert(err, IsNil)

	// start a mock keysign, succeeds on 5th try
	msg := "foobar"
	tssServer := newFakeTss(msg, false)
	bridge := fakeBridge{s.bridge}
	ks, err := tss.NewKeySign(tssServer, bridge)
	c.Assert(err, IsNil)
	ks.Start()

	// creat mock chain client and signer
	cc := &MockChainClient{ks: ks}
	sign := &Signer{
		chains: map[common.Chain]chainclients.ChainClient{
			common.ETHChain: cc,
		},
		pubkeyMgr:           pubkeymanager.NewMockPoolAddressValidator(),
		stopChan:            make(chan struct{}),
		wg:                  &sync.WaitGroup{},
		thorchainBridge:     bridge,
		constantsProvider:   NewConstantsProvider(bridge),
		tssKeysignMetricMgr: metrics.NewTssKeysignMetricMgr(),
		logger:              log.With().Str("module", "signer").Logger(),
	}

	// create a signer store with fake txouts
	sign.storage, err = NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	err = sign.storage.Set(TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        msg,
			VaultPubKey: vaultPubKey,
			Coins: common.Coins{ // must be set or signer overrides memo
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
	})
	c.Assert(err, IsNil)
	err = sign.storage.Set(TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			ToAddress:   "0xe3c64974c78f5693bd2bc68b3221d58df5c6e877",
			Memo:        msg,
			VaultPubKey: vaultPubKey,
			Coins: common.Coins{ // must be set or signer overrides memo
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
	})
	c.Assert(err, IsNil)
	err = sign.storage.Set(TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			ToAddress:   "0xd58610f89265a2fb637ac40edf59141ff873b266",
			Memo:        msg,
			VaultPubKey: vaultPubKey,
			Coins: common.Coins{ // must be set or signer overrides memo
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
	})
	c.Assert(err, IsNil)

	// this will be ignored entirely since the vault pubkey is different
	err = sign.storage.Set(TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.ETHChain,
			ToAddress:   "0x90f2b1ae50e6018230e90a33f98c7844a0ab635a",
			Memo:        msg,
			VaultPubKey: "tthorpub1addwnpepqfup3y8p0egd7ml7vrnlxgl3wvnp89mpn0tjpj0p2nm2gh0n9hlrvrtylay",
			Coins: common.Coins{ // must be set or signer overrides memo
				common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
			},
		},
	})
	c.Assert(err, IsNil)

	// create the same on different chain, should move independently
	msg2 := "foobar2"
	tssServer2 := newFakeTss(msg2, true) // this one succeeds on first try
	bridge2 := fakeBridge{s.bridge}
	ks2, err := tss.NewKeySign(tssServer2, bridge2)
	c.Assert(err, IsNil)
	ks2.Start()
	cc2 := &MockChainClient{ks: ks2}
	sign.chains[common.BTCChain] = cc2
	tois2 := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.BTCChain,
			ToAddress:   "tbtc1yycn4mh6ffwpjf584t8lpp7c27ghu03gpvqkfj",
			VaultPubKey: vaultPubKey,
			Memo:        msg2,
			Coins: common.Coins{
				common.NewCoin(common.BTCAsset, cosmos.NewUint(1000000)),
			},
		},
	}
	err = sign.storage.Set(tois2)
	c.Assert(err, IsNil)

	// first round only btc tx should go through
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 1)
	c.Assert(cc.broadcastCount, Equals, 0)
	c.Assert(tssServer.counter, Equals, 1)
	c.Assert(cc2.signCount, Equals, 1)
	c.Assert(cc2.broadcastCount, Equals, 1)
	c.Assert(tssServer2.counter, Equals, 1)

	// all eth txs should be remaining, first marked round 7
	tois := sign.storage.List()
	c.Assert(len(tois), Equals, 3)
	c.Assert(tois[0].Round7Retry, Equals, true)
	c.Assert(bytes.Equal(tois[0].Checkpoint, []byte(msg)), Equals, true)
	c.Assert(tois[1].Round7Retry, Equals, false)
	c.Assert(tois[2].Round7Retry, Equals, false)

	// process transactions 3 more times
	cc.assertCheckpoint = true // the following signs should pass checkpoint
	for i := 0; i < 3; i++ {
		sign.processTransactions()
		sign.pipeline.Wait()
	}

	// first tx should have been retried 3 times, no broadcast yet
	c.Assert(cc.signCount, Equals, 4)
	c.Assert(cc.broadcastCount, Equals, 0)
	c.Assert(tssServer.counter, Equals, 4)

	// this round should sign and broadcast the round 7 retry
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 5)
	c.Assert(cc.broadcastCount, Equals, 1)
	c.Assert(tssServer.counter, Equals, 5)
	tois = sign.storage.List()
	c.Assert(len(tois), Equals, 2)
	c.Assert(tois[0].Round7Retry, Equals, false)
	c.Assert(tois[1].Round7Retry, Equals, false)

	// the next 2 rounds should sign and broadcast the remaining

	cc.assertCheckpoint = false // the following signs should not pass checkpoint

	// only processes 1 per vault/chain in the pipeline
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 6)
	c.Assert(cc.broadcastCount, Equals, 2)
	c.Assert(tssServer.counter, Equals, 6)
	c.Assert(len(sign.storage.List()), Equals, 1)

	// last one
	sign.processTransactions()
	sign.pipeline.Wait()
	c.Assert(cc.signCount, Equals, 7)
	c.Assert(cc.broadcastCount, Equals, 3)
	c.Assert(tssServer.counter, Equals, 7)
	c.Assert(len(sign.storage.List()), Equals, 0)

	// nothing more should have happened on btc
	for i := 0; i < 3; i++ {
		sign.processTransactions()
		sign.pipeline.Wait()
	}
	c.Assert(cc.signCount, Equals, 7)
	c.Assert(cc.broadcastCount, Equals, 3)
	c.Assert(tssServer.counter, Equals, 7)
	c.Assert(cc2.signCount, Equals, 1)
	c.Assert(cc2.broadcastCount, Equals, 1)
	c.Assert(tssServer2.counter, Equals, 1)
	c.Assert(len(sign.storage.List()), Equals, 0)

	// stop signer
	close(sign.stopChan)
	sign.wg.Wait()
	ks.Stop()
	ks2.Stop()
}
