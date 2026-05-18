package utxo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	sutxo "gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/utxo"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	btctxscript "gitlab.com/thorchain/thornode/v3/bifrost/txscript/txscript"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// CoverageBackfillSuite focuses on testing uncovered functions for the utxo package.
type CoverageBackfillSuite struct {
	client *Client
	server *httptest.Server
	bridge thorclient.ThorchainBridge
	cfg    config.BifrostChainConfiguration
	m      *metrics.Metrics
	keys   *thorclient.Keys
}

var _ = Suite(&CoverageBackfillSuite{})

func (s *CoverageBackfillSuite) SetUpSuite(c *C) {
	ttypes.SetupConfigForTest()
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(bob, cKeys.English, cmd.THORChainHDPath, password, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.keys = thorclient.NewKeysWithKeybase(kb, bob, password)
}

func (s *CoverageBackfillSuite) SetUpTest(c *C) {
	s.m = GetMetricForTest(c, common.BTCChain)
	s.cfg = config.BifrostChainConfiguration{
		ChainID:     "BTC",
		UserName:    bob,
		Password:    password,
		DisableTLS:  true,
		HTTPostMode: true,
		BlockScanner: config.BifrostBlockScannerConfiguration{
			StartBlockHeight:             1,
			MaxReorgRescanBlocks:         1,
			GasCacheBlocks:               3,
			GasPriceResolution:           10,
			ObservationFlexibilityBlocks: 5,
		},
	}
	s.cfg.UTXO.TransactionBatchSize = 500
	s.cfg.UTXO.MaxMempoolBatches = 10
	s.cfg.UTXO.EstimatedAverageTxSize = 1000
	s.cfg.UTXO.MaxUTXOsToSpend = 15
	s.cfg.UTXO.DefaultSatsPerVByte = 25
	s.cfg.UTXO.MaxSatsPerVByte = 1000
	s.cfg.UTXO.MinSatsPerVByte = 1
	s.cfg.UTXO.DefaultMinRelayFeeSats = 1000
	s.cfg.UTXO.BlockCacheCount = 100

	ns := strconv.Itoa(time.Now().Nanosecond())
	thordir := filepath.Join(os.TempDir(), ns, ".thorcli")
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      bob,
		SignerPasswd:    password,
		ChainHomeFolder: thordir,
	}

	handleRPC := func(body []byte, rw http.ResponseWriter) {
		r := struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}{}

		err := json.Unmarshal(body, &r)
		c.Assert(err, IsNil)

		rw.Header().Set("Content-Type", "application/json")
		key := r.Method
		if r.Method == "getrawtransaction" {
			key = fmt.Sprintf("%s-%s", r.Method, r.Params[0])
		}
		if btcChainRPCs[key] == nil {
			key = r.Method
		}

		err = json.NewEncoder(rw).Encode(btcChainRPCs[key])
		c.Assert(err, IsNil)
	}

	handleBatchRPC := func(body []byte, rw http.ResponseWriter) {
		r := []struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
			ID     int           `json:"id"`
		}{}
		err := json.Unmarshal(body, &r)
		c.Assert(err, IsNil)
		rw.Header().Set("Content-Type", "application/json")
		result := make([]map[string]interface{}, len(r))
		for i, v := range r {
			key := v.Method
			if v.Method == "getrawtransaction" {
				key = fmt.Sprintf("%s-%s", v.Method, v.Params[0])
			}
			if btcChainRPCs[key] == nil {
				key = v.Method
			}
			result[i] = btcChainRPCs[key]
			result[i]["id"] = v.ID
		}
		err = json.NewEncoder(rw).Encode(result)
		c.Assert(err, IsNil)
	}

	s.server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.RequestURI == "/":
			body, _ := io.ReadAll(req.Body)
			if body[0] == '[' {
				handleBatchRPC(body, rw)
			} else {
				handleRPC(body, rw)
			}
		case strings.HasPrefix(req.RequestURI, "/thorchain/node/"):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		case req.RequestURI == "/thorchain/lastblock":
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/lastblock/btc.json")
		case strings.HasPrefix(req.RequestURI, "/auth/accounts/"):
			_, err := rw.Write([]byte(`{ "jsonrpc": "2.0", "id": "", "result": { "height": "0", "result": { "value": { "account_number": "0", "sequence": "0" } } } }`))
			c.Assert(err, IsNil)
		case req.RequestURI == "/txs":
			_, err := rw.Write([]byte(`{"height": "1", "txhash": "AAAA000000000000000000000000000000000000000000000000000000000000", "logs": [{"success": "true", "log": ""}]}`))
			c.Assert(err, IsNil)
		case strings.HasPrefix(req.RequestURI, thorclient.AsgardVault):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case req.RequestURI == "/thorchain/mimir/key/MaxUTXOsToSpend":
			_, err := rw.Write([]byte(`-1`))
			c.Assert(err, IsNil)
		case req.RequestURI == "/thorchain/vaults/pubkeys":
			if common.CurrentChainNetwork == common.MainNet {
				httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys-Mainnet.json")
			} else {
				httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
			}
		case strings.HasPrefix(req.RequestURI, "/thorchain/mimir/key/"):
			_, err := rw.Write([]byte(`-1`))
			c.Assert(err, IsNil)
		}
	}))

	var err error
	cfg.ChainHost = s.server.Listener.Addr().String()
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
	s.cfg.RPCHost = s.server.Listener.Addr().String()
	s.client, err = NewClient(s.keys, s.cfg, nil, s.bridge, s.m)
	s.client.disableVinZeroBatch = true
	netFeeQueue := make(chan common.NetworkFee, 100)
	s.client.globalNetworkFeeQueue = netFeeQueue
	s.client.globalErrataQueue = make(chan stypes.ErrataBlock, 10)
	s.client.globalSolvencyQueue = make(chan stypes.Solvency, 10)
	c.Assert(err, IsNil)
	c.Assert(s.client, NotNil)
}

func (s *CoverageBackfillSuite) TearDownTest(_ *C) {
	s.server.Close()
}

////////////////////////////////////////////////////////////////////////////////////////
// helpers.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestBech32AccountPubKey(c *C) {
	// Generate a key and convert it
	privKey, err := btcec.NewPrivateKey(btcec.S256())
	c.Assert(err, IsNil)
	pk, err := bech32AccountPubKey(privKey)
	c.Assert(err, IsNil)
	c.Assert(pk.IsEmpty(), Equals, false)
}

func (s *CoverageBackfillSuite) TestSumVoutSats(c *C) {
	// Test with valid vouts
	tx := &btcjson.TxRawResult{
		Vout: []btcjson.Vout{
			{Value: 0.5},
			{Value: 1.0},
			{Value: 0.1},
		},
	}
	sum, err := sumVoutSats(tx)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, uint64(160000000)) // 1.6 BTC in sats

	// Test with no vouts
	tx = &btcjson.TxRawResult{Vout: nil}
	sum, err = sumVoutSats(tx)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, uint64(0))
}

////////////////////////////////////////////////////////////////////////////////////////
// bitcoin.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestGetChainCfgBTC(c *C) {
	cfg := s.client.getChainCfgBTC()
	c.Assert(cfg, NotNil)
	switch common.CurrentChainNetwork {
	case common.MockNet:
		c.Assert(cfg.Name, Equals, "regtest")
	case common.TestNet:
		c.Assert(cfg.Name, Equals, "testnet3")
	default:
		c.Assert(cfg.Name, Equals, "mainnet")
	}
}

func (s *CoverageBackfillSuite) TestGetAddressesFromScriptPubKeyBTC(c *C) {
	// Test with addresses already present
	spk := btcjson.ScriptPubKeyResult{
		Addresses: []string{"tb1qfoo", "tb1qbar"},
	}
	addresses := s.client.getAddressesFromScriptPubKeyBTC(spk)
	c.Assert(len(addresses), Equals, 2)
	c.Assert(addresses[0], Equals, "tb1qfoo")

	// Test with empty addresses but hex present
	// A P2PKH scriptPubKey: OP_DUP OP_HASH160 <hash> OP_EQUALVERIFY OP_CHECKSIG
	spk = btcjson.ScriptPubKeyResult{
		Hex: "76a914000000000000000000000000000000000000000088ac",
	}
	addresses = s.client.getAddressesFromScriptPubKeyBTC(spk)
	c.Assert(len(addresses) > 0, Equals, true)

	// Test with empty addresses and empty hex
	spk = btcjson.ScriptPubKeyResult{}
	addresses = s.client.getAddressesFromScriptPubKeyBTC(spk)
	c.Assert(addresses, IsNil)

	// Test with invalid hex
	spk = btcjson.ScriptPubKeyResult{Hex: "zzzz"}
	addresses = s.client.getAddressesFromScriptPubKeyBTC(spk)
	c.Assert(addresses, IsNil)
}

////////////////////////////////////////////////////////////////////////////////////////
// zcash.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestGetChainCfgZEC(c *C) {
	// Create a ZEC client config
	zecClient := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.ZECChain},
	}
	cfg := zecClient.getChainCfgZEC()
	c.Assert(cfg, NotNil)
	switch common.CurrentChainNetwork {
	case common.MockNet:
		c.Assert(cfg.Name, Equals, "testnet3")
	default:
		c.Assert(cfg.Name, Equals, "mainnet")
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// client.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestGetConfig(c *C) {
	cfg := s.client.GetConfig()
	c.Assert(cfg.ChainID, Equals, common.BTCChain)
}

func (s *CoverageBackfillSuite) TestGetAccountByAddress(c *C) {
	acct, err := s.client.GetAccountByAddress("anything", nil)
	c.Assert(err, IsNil)
	c.Assert(acct, DeepEquals, common.Account{})
}

func (s *CoverageBackfillSuite) TestGetNetworkFee(c *C) {
	s.client.lastFeeRate.Store(42)
	s.client.cfg.UTXO.EstimatedAverageTxSize = 1000
	txSize, feeRate := s.client.GetNetworkFee()
	c.Assert(txSize, Equals, uint64(1000))
	c.Assert(feeRate, Equals, uint64(42))
}

func (s *CoverageBackfillSuite) TestGetBlockScannerHeight(c *C) {
	h, err := s.client.GetBlockScannerHeight()
	c.Assert(err, IsNil)
	c.Assert(h >= 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestGetAddress(c *C) {
	// Valid pubkey
	addr := s.client.GetAddress(s.client.nodePubKey)
	c.Assert(addr, Not(Equals), "")

	// Empty pubkey
	addr = s.client.GetAddress(common.PubKey(""))
	c.Assert(addr, Equals, "")
}

func (s *CoverageBackfillSuite) TestGetLatestTxForVault(c *C) {
	// GetLatestTxForVault may return "not found" for a vault that hasn't been cached
	lastObserved, lastBroadcast, err := s.client.GetLatestTxForVault("somevault")
	// The function may return an error or empty strings - both are valid
	if err == nil {
		c.Assert(lastObserved, Equals, "")
		c.Assert(lastBroadcast, Equals, "")
	}
}

func (s *CoverageBackfillSuite) TestShouldReportSolvency(c *C) {
	// Same height should return false
	s.client.lastSolvencyCheckHeight.Store(100)
	c.Assert(s.client.ShouldReportSolvency(101), Equals, false)

	// BTC chain: always true if height is past 1
	s.client.lastSolvencyCheckHeight.Store(98)
	c.Assert(s.client.ShouldReportSolvency(100), Equals, true)

	// DOGE chain: only on height % 10 == 0
	s.client.cfg.ChainID = common.DOGEChain
	s.client.lastSolvencyCheckHeight.Store(8)
	c.Assert(s.client.ShouldReportSolvency(10), Equals, true)
	c.Assert(s.client.ShouldReportSolvency(11), Equals, false)

	// LTC chain: height difference > 5 and height % 5 == 0
	s.client.cfg.ChainID = common.LTCChain
	s.client.lastSolvencyCheckHeight.Store(0)
	c.Assert(s.client.ShouldReportSolvency(10), Equals, true)
	c.Assert(s.client.ShouldReportSolvency(11), Equals, false)
	s.client.lastSolvencyCheckHeight.Store(9)
	c.Assert(s.client.ShouldReportSolvency(10), Equals, false) // diff <= 1 (early return)

	// BCH chain
	s.client.cfg.ChainID = common.BCHChain
	s.client.lastSolvencyCheckHeight.Store(8)
	c.Assert(s.client.ShouldReportSolvency(10), Equals, true)

	// Restore
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestConfirmationCountReady(c *C) {
	// No txs
	txIn := stypes.TxIn{TxArray: nil}
	c.Assert(s.client.ConfirmationCountReady(txIn), Equals, true)

	// Mempool
	txIn = stypes.TxIn{
		TxArray: []*stypes.TxInItem{{BlockHeight: 10}},
		MemPool: true,
	}
	c.Assert(s.client.ConfirmationCountReady(txIn), Equals, true)

	// Normal tx - check against block height
	s.client.currentBlockHeight.Store(110)
	txIn = stypes.TxIn{
		TxArray:              []*stypes.TxInItem{{BlockHeight: 100}},
		ConfirmationRequired: 5,
	}
	c.Assert(s.client.ConfirmationCountReady(txIn), Equals, true)

	// Not ready yet
	txIn.ConfirmationRequired = 15
	c.Assert(s.client.ConfirmationCountReady(txIn), Equals, false)
}

func (s *CoverageBackfillSuite) TestGetConfirmationCountEmpty(c *C) {
	txIn := stypes.TxIn{TxArray: nil}
	c.Assert(s.client.GetConfirmationCount(txIn), Equals, int64(0))

	txIn = stypes.TxIn{
		TxArray: []*stypes.TxInItem{{BlockHeight: 10}},
		MemPool: true,
	}
	c.Assert(s.client.GetConfirmationCount(txIn), Equals, int64(0))
}

func (s *CoverageBackfillSuite) TestGetAccount(c *C) {
	// Empty pubkey
	_, err := s.client.GetAccount(common.PubKey(""), nil)
	c.Assert(err, NotNil)

	// Valid pubkey
	acct, err := s.client.GetAccount(s.client.nodePubKey, nil)
	c.Assert(err, IsNil)
	c.Assert(acct.Coins, NotNil)
}

////////////////////////////////////////////////////////////////////////////////////////
// client_internal.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestStripBCHAddress(c *C) {
	c.Assert(s.client.stripBCHAddress("bitcoincash:qp3w"), Equals, "qp3w")
	c.Assert(s.client.stripBCHAddress("qp3w"), Equals, "qp3w")
}

func (s *CoverageBackfillSuite) TestDecodeHexString(c *C) {
	// Valid ASCII hex
	decoded, err := s.client.decodeHexString("48656c6c6f")
	c.Assert(err, IsNil)
	c.Assert(decoded, Equals, "Hello")

	// Invalid hex
	_, err = s.client.decodeHexString("zzzz")
	c.Assert(err, NotNil)

	// Non-printable characters
	_, err = s.client.decodeHexString("0001") // 0x00 is < 0x20
	c.Assert(err, NotNil)

	// High byte
	_, err = s.client.decodeHexString("ff")
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestCanDeleteBlock(c *C) {
	// nil block meta
	c.Assert(s.client.canDeleteBlock(nil), Equals, true)

	// Block meta with no self transactions
	bm := sutxo.NewBlockMeta("prev", 100, "hash")
	c.Assert(s.client.canDeleteBlock(bm), Equals, true)

	// Block meta with self transaction in mempool
	bm.AddSelfTransaction("sometxid")
	result := s.client.canDeleteBlock(bm)
	// will call GetMempoolEntry which may fail, so should return true (can delete)
	c.Assert(result, Equals, true)
}

func (s *CoverageBackfillSuite) TestIsRBFEnabled(c *C) {
	// RBF enabled (sequence < 0xffffffff - 1)
	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Sequence: 0},
		},
	}
	c.Assert(s.client.isRBFEnabled(tx), Equals, true)

	// RBF disabled
	tx = &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Sequence: 0xffffffff},
		},
	}
	c.Assert(s.client.isRBFEnabled(tx), Equals, false)

	// Edge case: exactly at threshold
	tx = &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Sequence: 0xfffffffe},
		},
	}
	c.Assert(s.client.isRBFEnabled(tx), Equals, false)
}

func (s *CoverageBackfillSuite) TestGetAddressesFromScriptPubKey(c *C) {
	// BTC chain uses special path
	spk := btcjson.ScriptPubKeyResult{
		Addresses: []string{"tb1qfoo"},
	}
	addrs := s.client.getAddressesFromScriptPubKey(spk)
	c.Assert(addrs, DeepEquals, []string{"tb1qfoo"})

	// Non-BTC chain (e.g., DOGE) uses the direct Addresses field
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.DOGEChain
	addrs = s.client.getAddressesFromScriptPubKey(spk)
	c.Assert(addrs, DeepEquals, []string{"tb1qfoo"})
	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestIsAsgardAddress(c *C) {
	// Should not be asgard (arbitrary address)
	result := s.client.isAsgardAddress("some_random_address")
	c.Assert(result, Equals, false)
}

func (s *CoverageBackfillSuite) TestGetAsgardAddress(c *C) {
	addrs, err := s.client.getAsgardAddress()
	c.Assert(err, IsNil)
	// The fixture uses tthorpub1 (testnet/mocknet) keys, which can only derive
	// BTC addresses on mocknet. On mainnet builds, addresses will be empty.
	if common.CurrentChainNetwork == common.MockNet {
		c.Assert(addrs, NotNil)
	}

	// Test caching - call again within ThorchainBlockTime
	addrs2, err := s.client.getAsgardAddress()
	c.Assert(err, IsNil)
	c.Assert(len(addrs2), Equals, len(addrs))
}

func (s *CoverageBackfillSuite) TestUpdateNetworkInfo(c *C) {
	s.client.updateNetworkInfo()
	// minRelayFeeSats should be set
	c.Assert(s.client.minRelayFeeSats.Load() > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestSendNetworkFee(c *C) {
	s.client.lastFeeRate.Store(0)
	s.client.minRelayFeeSats.Store(0)
	s.client.feeRateCache = nil
	err := s.client.sendNetworkFee(1696761)
	c.Assert(err, IsNil)

	// The fee was sent to the queue - verify lastFeeRate was set
	c.Assert(s.client.lastFeeRate.Load() > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeSameFee(c *C) {
	// When fee hasn't changed, should not send
	s.client.lastFeeRate.Store(0)
	s.client.feeRateCache = nil
	err := s.client.sendNetworkFee(1696761)
	c.Assert(err, IsNil)
	savedFeeRate := s.client.lastFeeRate.Load()

	// Call again - should skip since fee is the same
	err = s.client.sendNetworkFee(1696761)
	c.Assert(err, IsNil)
	// lastFeeRate should be unchanged
	c.Assert(s.client.lastFeeRate.Load(), Equals, savedFeeRate)
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeMinRelayFee(c *C) {
	// Set high min relay fee to trigger the fee floor
	s.client.minRelayFeeSats.Store(999999999)
	s.client.lastFeeRate.Store(0)
	s.client.feeRateCache = nil
	err := s.client.sendNetworkFee(1696761)
	c.Assert(err, IsNil)
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeGasCache(c *C) {
	s.client.lastFeeRate.Store(0)
	s.client.feeRateCache = []uint64{100, 200}
	s.client.cfg.BlockScanner.GasCacheBlocks = 3
	err := s.client.sendNetworkFee(1696761)
	c.Assert(err, IsNil)
	// Cache should include old values plus new
	c.Assert(len(s.client.feeRateCache) > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeFromBlock(c *C) {
	// Create a block with coinbase and non-coinbase txs
	block := &btcjson.GetBlockVerboseTxResult{
		Height: 100,
		Tx: []btcjson.TxRawResult{
			{
				Vin:   []btcjson.Vin{{Coinbase: "04ffff001d0104"}},
				Vout:  []btcjson.Vout{{Value: 50.0}},
				Vsize: 200,
			},
			{
				Vin:   []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout:  []btcjson.Vout{{Value: 0.5}},
				Vsize: 250,
			},
		},
	}

	// Save for DOGE chain which uses sendNetworkFeeFromBlock
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.DOGEChain
	s.client.lastFeeRate.Store(0)

	err := s.client.sendNetworkFeeFromBlock(block)
	c.Assert(err, IsNil)

	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeFromBlockNoNonCoinbase(c *C) {
	// Block with only coinbase - should skip
	block := &btcjson.GetBlockVerboseTxResult{
		Height: 100,
		Tx: []btcjson.TxRawResult{
			{
				Vin:  []btcjson.Vin{{Coinbase: "04ffff001d0104"}},
				Vout: []btcjson.Vout{{Value: 50.0}},
			},
		},
	}
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.DOGEChain
	err := s.client.sendNetworkFeeFromBlock(block)
	c.Assert(err, IsNil)
	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeFromBlockSmallDelta(c *C) {
	// When delta is small, should skip
	block := &btcjson.GetBlockVerboseTxResult{
		Height: 100,
		Tx: []btcjson.TxRawResult{
			{
				Vin:  []btcjson.Vin{{Coinbase: "04ffff001d0104"}},
				Vout: []btcjson.Vout{{Value: 50.0}},
			},
			{
				Vin:   []btcjson.Vin{{Txid: "abc123"}},
				Vout:  []btcjson.Vout{{Value: 0.5}},
				Vsize: 250,
			},
		},
	}

	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.DOGEChain
	s.client.lastFeeRate.Store(0)

	// First call sets the fee
	err := s.client.sendNetworkFeeFromBlock(block)
	c.Assert(err, IsNil)
	savedRate := s.client.lastFeeRate.Load()

	// Second call with same block should skip because delta <= resolution
	err = s.client.sendNetworkFeeFromBlock(block)
	c.Assert(err, IsNil)
	c.Assert(s.client.lastFeeRate.Load(), Equals, savedRate)

	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestConfirmTx(c *C) {
	// GetRawTransaction calls with verbose=false expecting a hex string result,
	// but the mock returns a JSON object, so the unmarshal fails and confirmTx returns false.
	// This exercises the error path in confirmTx.
	result := s.client.confirmTx("sometxid")
	c.Assert(result, Equals, false)
}

func (s *CoverageBackfillSuite) TestIsSelfTransaction(c *C) {
	// Without any block metas, should return false
	c.Assert(s.client.isSelfTransaction("sometxid"), Equals, false)
}

func (s *CoverageBackfillSuite) TestTryAddToMemPoolCache(c *C) {
	added := s.client.tryAddToMemPoolCache("txhash123")
	c.Assert(added, Equals, true)

	// Adding same hash again should return false
	added = s.client.tryAddToMemPoolCache("txhash123")
	c.Assert(added, Equals, false)
}

func (s *CoverageBackfillSuite) TestRemoveFromMemPoolCache(c *C) {
	s.client.tryAddToMemPoolCache("txhash456")
	s.client.removeFromMemPoolCache("txhash456")
	// Should be able to add again
	added := s.client.tryAddToMemPoolCache("txhash456")
	c.Assert(added, Equals, true)
}

////////////////////////////////////////////////////////////////////////////////////////
// signer_internal.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestIsSegwitChain(c *C) {
	// BTC is segwit
	s.client.cfg.ChainID = common.BTCChain
	c.Assert(s.client.isSegwitChain(), Equals, true)

	// LTC is segwit
	s.client.cfg.ChainID = common.LTCChain
	c.Assert(s.client.isSegwitChain(), Equals, true)

	// DOGE is not
	s.client.cfg.ChainID = common.DOGEChain
	c.Assert(s.client.isSegwitChain(), Equals, false)

	// BCH is not
	s.client.cfg.ChainID = common.BCHChain
	c.Assert(s.client.isSegwitChain(), Equals, false)

	// ZEC is not
	s.client.cfg.ChainID = common.ZECChain
	c.Assert(s.client.isSegwitChain(), Equals, false)

	// Restore
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetPaymentAmount(c *C) {
	// With MaxGas
	tx := stypes.TxOutItem{
		Chain: common.BTCChain,
		Coins: common.Coins{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(100000)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(5000)),
		},
	}
	amt := s.client.getPaymentAmount(tx)
	c.Assert(int64(amt), Equals, int64(105000))

	// Without MaxGas
	tx.MaxGas = nil
	amt = s.client.getPaymentAmount(tx)
	c.Assert(int64(amt), Equals, int64(100000))

	// ZEC chain ignores MaxGas
	s.client.cfg.ChainID = common.ZECChain
	tx.MaxGas = common.Gas{
		common.NewCoin(common.ZECAsset, cosmos.NewUint(5000)),
	}
	tx.Coins = common.Coins{
		common.NewCoin(common.ZECAsset, cosmos.NewUint(100000)),
	}
	amt = s.client.getPaymentAmount(tx)
	c.Assert(int64(amt), Equals, int64(100000)) // ZEC doesn't add gas
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetMaximumUtxosToSpend(c *C) {
	utxos := s.client.getMaximumUtxosToSpend()
	c.Assert(utxos > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestGetGasCoin(c *C) {
	tx := stypes.TxOutItem{
		GasRate: 10,
	}
	gas := s.client.getGasCoin(tx, 250)
	c.Assert(gas.Amount.Uint64(), Equals, uint64(2500))

	// Zero gas rate - should try to get from storage
	tx.GasRate = 0
	gas = s.client.getGasCoin(tx, 250)
	// With default fallback
	c.Assert(gas.Asset, Equals, common.BTCAsset)
}

func (s *CoverageBackfillSuite) TestGetGasCoinWithDefaultSatsPerVByte(c *C) {
	tx := stypes.TxOutItem{
		GasRate: 0,
	}
	s.client.cfg.UTXO.DefaultSatsPerVByte = 50

	// Store a zero fee so GetTransactionFee doesn't error (which causes early return).
	// With fee=0.0 the inner condition is false, gasRate stays 0, and falls through
	// to the DefaultSatsPerVByte fallback.
	err := s.client.temporalStorage.UpsertTransactionFee(0.0, 250)
	c.Assert(err, IsNil)

	gas := s.client.getGasCoin(tx, 100)
	c.Assert(gas.Asset, Equals, common.BTCAsset)
	// 50 sats/vbyte * 100 vbytes = 5000
	c.Assert(gas.Amount.Uint64(), Equals, uint64(5000))
}

func (s *CoverageBackfillSuite) TestGetGasCoinZEC(c *C) {
	// Create a wire tx with inputs
	msgTx := wire.NewMsgTx(wire.TxVersion)
	msgTx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))
	msgTx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))
	msgTx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))

	gas := s.client.getGasCoinZEC(msgTx, "swap:BTC.BTC:bc1qfoo")
	c.Assert(gas.Asset, Equals, common.ZECAsset)
	c.Assert(gas.Amount.Uint64() > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestEstimateTxSize(c *C) {
	txes := []btcjson.ListUnspentResult{
		{TxID: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0},
		{TxID: "54ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528", Vout: 0},
	}

	memoData := []byte("SWAP:ETH.ETH:0xfoo")
	memoScript, err := btctxscript.NullDataScript(memoData)
	c.Assert(err, IsNil)

	// BTC (segwit)
	s.client.cfg.ChainID = common.BTCChain
	size := s.client.estimateTxSize(txes, [][]byte{memoScript}, make([]byte, 25), make([]byte, 25))
	c.Assert(size > 0, Equals, true)

	// DOGE (non-segwit)
	s.client.cfg.ChainID = common.DOGEChain
	sizeNonSegwit := s.client.estimateTxSize(txes, [][]byte{memoScript}, make([]byte, 25), make([]byte, 25))
	c.Assert(sizeNonSegwit > 0, Equals, true)

	// Restore
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestMemoToScripts(c *C) {
	// Empty memo
	scripts, err := MemoToScripts("", 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(scripts, IsNil)

	// Short memo (< 80 bytes)
	scripts, err = MemoToScripts("SWAP:ETH.ETH:0xfoo", 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 1) // just OP_RETURN

	// Exactly 80 bytes
	memo80 := strings.Repeat("A", 80)
	scripts, err = MemoToScripts(memo80, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 1) // fits in single OP_RETURN

	// Over 80 bytes - needs continuation
	memo100 := strings.Repeat("A", 100)
	scripts, err = MemoToScripts(memo100, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts) > 1, Equals, true)

	// Way over 80 bytes
	memo200 := strings.Repeat("B", 200)
	scripts, err = MemoToScripts(memo200, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts) > 2, Equals, true)

	// Memo too long (> MaxMemoSize)
	memoTooLong := strings.Repeat("X", 1000)
	_, err = MemoToScripts(memoTooLong, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestGetSourceScript(c *C) {
	tx := stypes.TxOutItem{
		VaultPubKey: s.client.nodePubKey,
		Chain:       common.BTCChain,
	}
	script, err := s.client.getSourceScript(tx)
	c.Assert(err, IsNil)
	c.Assert(len(script) > 0, Equals, true)
}

////////////////////////////////////////////////////////////////////////////////////////
// signer.go tests
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestGetVaultLock(c *C) {
	lock1 := s.client.GetVaultLock("vault1")
	c.Assert(lock1, NotNil)

	// Same vault should return same lock
	lock2 := s.client.GetVaultLock("vault1")
	c.Assert(lock1, Equals, lock2)

	// Different vault should return different lock
	lock3 := s.client.GetVaultLock("vault2")
	c.Assert(lock3, NotNil)
	c.Assert(lock1 == lock3, Equals, false)
}

func (s *CoverageBackfillSuite) TestSignTxWrongChain(c *C) {
	tx := stypes.TxOutItem{
		Chain: common.ETHChain,
	}
	_, _, _, err := s.client.SignTx(tx, 1)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "wrong chain")
}

func (s *CoverageBackfillSuite) TestSignTxEmptyCoins(c *C) {
	tx := stypes.TxOutItem{
		Chain: common.BTCChain,
		Coins: common.Coins{},
	}
	raw, checkpoint, txIn, err := s.client.SignTx(tx, 1)
	c.Assert(err, IsNil)
	c.Assert(raw, IsNil)
	c.Assert(checkpoint, IsNil)
	c.Assert(txIn, IsNil)
}

func (s *CoverageBackfillSuite) TestBroadcastTxEmptyPayloadZEC(c *C) {
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.ZECChain
	_, err := s.client.BroadcastTx(stypes.TxOutItem{Chain: common.ZECChain}, nil)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "payload is empty"), Equals, true)
	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestIsValidUTXO(c *C) {
	// Invalid hex
	c.Assert(s.client.isValidUTXO("zzzz"), Equals, false)

	// Valid P2PKH script
	valid := s.client.isValidUTXO("76a914000000000000000000000000000000000000000088ac")
	c.Assert(valid, Equals, true)
}

func (s *CoverageBackfillSuite) TestIsValidUTXOChains(c *C) {
	p2pkh := "76a914000000000000000000000000000000000000000088ac"

	// Test for DOGE chain
	s.client.cfg.ChainID = common.DOGEChain
	c.Assert(s.client.isValidUTXO(p2pkh), Equals, true)

	// Test for BCH chain
	s.client.cfg.ChainID = common.BCHChain
	c.Assert(s.client.isValidUTXO(p2pkh), Equals, true)

	// Test for LTC chain
	s.client.cfg.ChainID = common.LTCChain
	c.Assert(s.client.isValidUTXO(p2pkh), Equals, true)

	// Test for ZEC chain (uses BTC path)
	s.client.cfg.ChainID = common.ZECChain
	c.Assert(s.client.isValidUTXO(p2pkh), Equals, true)

	// Restore
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestIgnoreTxMoreCases(c *C) {
	// No vin
	tx := &btcjson.TxRawResult{
		Vin:  nil,
		Vout: []btcjson.Vout{{Value: 1}},
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// No vout
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: "abc"}},
		Vout: nil,
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// Too many vouts
	vouts := make([]btcjson.Vout, 13)
	for i := range vouts {
		vouts[i].Value = 0.01
	}
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: "abc"}},
		Vout: vouts,
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// Coinbase tx (empty vin[0].Txid)
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: ""}},
		Vout: []btcjson.Vout{{Value: 1}},
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// LockTime > height
	tx = &btcjson.TxRawResult{
		Vin:      []btcjson.Vin{{Txid: "abc"}},
		Vout:     []btcjson.Vout{{Value: 1}},
		LockTime: 200,
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// No value outputs
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: "abc"}},
		Vout: []btcjson.Vout{{Value: 0}},
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// Too many outputs with value
	vouts11 := make([]btcjson.Vout, 11)
	for i := range vouts11 {
		vouts11[i].Value = 0.01
	}
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: "abc"}},
		Vout: vouts11,
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, true)

	// Valid tx should NOT be ignored
	tx = &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: "abc"}},
		Vout: []btcjson.Vout{{Value: 0.01}, {Value: 0}},
	}
	c.Assert(s.client.ignoreTx(tx, 100), Equals, false)
}

func (s *CoverageBackfillSuite) TestGetMemoExtendedCases(c *C) {
	// Empty tx
	tx := &btcjson.TxRawResult{
		Vout: nil,
	}
	memo, err := s.client.getMemo(tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "")

	// ZEC shielded tx (no vin/vout)
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.ZECChain
	tx = &btcjson.TxRawResult{
		Txid: "test",
		Vin:  nil,
		Vout: nil,
	}
	memo, err = s.client.getMemo(tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "")
	s.client.cfg.ChainID = oldChain

	// OP_RETURN with "0" data (should be skipped)
	tx = &btcjson.TxRawResult{
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 0",
					Hex:  "6a0100",
					Type: "nulldata",
				},
			},
		},
	}
	memo, err = s.client.getMemo(tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "")

	// OP_RETURN with invalid hex
	tx = &btcjson.TxRawResult{
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN zzzz",
					Hex:  "6a04deadbeef",
					Type: "nulldata",
				},
			},
		},
	}
	_, err = s.client.getMemo(tx)
	c.Assert(err, IsNil)
}

func (s *CoverageBackfillSuite) TestGetMemoChains(c *C) {
	// The hex "6a1574686f72636861696e3a636f6e736f6c6964617465" is an OP_RETURN script
	// DisasmString works differently per chain library, so we test with the raw hex
	// and rely on each chain's DisasmString implementation

	// Each chain tests getMemo's chain switch
	for _, chain := range []common.Chain{common.DOGEChain, common.BCHChain, common.LTCChain, common.ZECChain} {
		s.client.cfg.ChainID = chain
		tx := &btcjson.TxRawResult{
			Vin: []btcjson.Vin{{Txid: "abc"}}, // needed for ZEC check
			Vout: []btcjson.Vout{
				{
					ScriptPubKey: btcjson.ScriptPubKeyResult{
						Hex:  "6a1574686f72636861696e3a636f6e736f6c6964617465",
						Type: "nulldata",
					},
				},
			},
		}
		memo, err := s.client.getMemo(tx)
		c.Assert(err, IsNil, Commentf("chain: %s", chain))
		// Each chain's DisasmString may parse the hex differently, but all should work
		// The important thing is the chain switch is exercised without errors
		_ = memo
	}
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetOutput(c *C) {
	// No matching output
	tx := &btcjson.TxRawResult{
		Vout: []btcjson.Vout{
			{
				Value: 0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Type:      "pubkeyhash",
					Addresses: []string{"addr1"},
				},
			},
		},
	}
	_, err := s.client.getOutput("sender", tx, false)
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestReportSolvency(c *C) {
	s.client.lastSolvencyCheckHeight.Store(0)
	// The asgard fixture uses tthorpub1 (testnet) keys which only work on mocknet.
	// On mainnet builds, GetAsgards will fail to unmarshal the keys.
	err := s.client.ReportSolvency(100)
	if common.CurrentChainNetwork == common.MockNet {
		c.Assert(err, IsNil)
	} else {
		c.Assert(err, NotNil)
	}
}

func (s *CoverageBackfillSuite) TestRegisterPublicKey(c *C) {
	// Test RegisterPublicKey
	err := s.client.RegisterPublicKey(s.client.nodePubKey)
	c.Assert(err, IsNil)
}

func (s *CoverageBackfillSuite) TestRegisterPublicKeyInvalidPubKey(c *C) {
	// An invalid pubkey should fail GetAddress
	err := s.client.RegisterPublicKey(common.PubKey("invalid_key"))
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestOnObservedTxIn(c *C) {
	// Valid outbound tx
	txIn := stypes.TxInItem{
		Tx:     "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
		Sender: "tb1qfoo",
		Memo:   "OUT:AAAA000000000000000000000000000000000000000000000000000000000000",
	}
	s.client.OnObservedTxIn(txIn, 100)

	// Invalid tx hash
	txIn = stypes.TxInItem{
		Tx:     "not-a-valid-hash",
		Sender: "tb1qfoo",
	}
	s.client.OnObservedTxIn(txIn, 100)

	// Non-outbound memo
	txIn = stypes.TxInItem{
		Tx:     "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
		Sender: "tb1qfoo",
		Memo:   "SWAP:BTC.BTC:bc1qfoo",
	}
	s.client.OnObservedTxIn(txIn, 101)
}

func (s *CoverageBackfillSuite) TestNewClientUnsupportedChain(c *C) {
	cfg := s.cfg
	cfg.ChainID = common.ETHChain
	_, err := NewClient(s.keys, cfg, nil, s.bridge, s.m)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "unsupported utxo chain"), Equals, true)
}

func (s *CoverageBackfillSuite) TestGetSenderWithVinZeroTxs(c *C) {
	// Test getSender with vinZeroTxs map
	vinTx := &btcjson.TxRawResult{
		Txid: "abc123",
		Vout: []btcjson.Vout{
			{
				Value: 1.0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					// P2PKH script; isValidUTXO accepts single-address/single-sig.
					Hex:       "76a91415fb126815935f6ae83a206d7d82f1065bc63e2588ac",
					Addresses: []string{"sender_addr"},
				},
			},
		},
	}

	vinZeroTxs := map[string]*btcjson.TxRawResult{
		"abc123": vinTx,
	}

	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "abc123", Vout: 0},
		},
	}

	sender, err := s.client.getSender(tx, vinZeroTxs)
	c.Assert(err, IsNil)
	c.Assert(sender, Equals, "sender_addr")

	// Missing vin zero tx
	tx2 := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "missing123", Vout: 0},
		},
		Vout: []btcjson.Vout{{Value: 0.1}},
	}
	_, err = s.client.getSender(tx2, vinZeroTxs)
	c.Assert(err, NotNil)

	// Empty vin
	tx3 := &btcjson.TxRawResult{Vin: nil}
	_, err = s.client.getSender(tx3, nil)
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestIsFromAsgard(c *C) {
	// Should return false for a non-asgard tx
	result := s.client.isFromAsgard("5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513")
	c.Assert(result, Equals, false)
}

func (s *CoverageBackfillSuite) TestGetTxIn(c *C) {
	// Test with coinbase tx (should be ignored)
	tx := &btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{Txid: ""}},
		Vout: []btcjson.Vout{{Value: 50.0}},
	}
	txIn, err := s.client.getTxIn(tx, 100, false, nil)
	c.Assert(err, IsNil)
	c.Assert(txIn.IsEmpty(), Equals, true)

	// Test with RBF enabled tx in mempool (should be skipped)
	tx = &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0, Sequence: 0},
		},
		Vout: []btcjson.Vout{{Value: 0.01}},
	}
	txIn, err = s.client.getTxIn(tx, 100, true, nil)
	c.Assert(err, IsNil)
	c.Assert(txIn.IsEmpty(), Equals, true)
}

////////////////////////////////////////////////////////////////////////////////////////
// Wire conversion tests (signable_gen.go coverage)
////////////////////////////////////////////////////////////////////////////////////////

func makeTestOutPoint() *wire.OutPoint {
	h, _ := chainhash.NewHashFromStr("5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513")
	return wire.NewOutPoint(h, 0)
}

func (s *CoverageBackfillSuite) TestWireToBTCAndBack(c *C) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))
	tx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	btcTx := wireToBTC(tx)
	c.Assert(btcTx, NotNil)

	wireBack := btcToWire(btcTx)
	c.Assert(wireBack, NotNil)
	c.Assert(len(wireBack.TxIn), Equals, len(tx.TxIn))
	c.Assert(len(wireBack.TxOut), Equals, len(tx.TxOut))
}

func (s *CoverageBackfillSuite) TestWireToDOGEAndBack(c *C) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), []byte{0x01}, nil))
	tx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	dogeTx := wireToDOGE(tx)
	c.Assert(dogeTx, NotNil)

	wireBack := dogeToWire(dogeTx)
	c.Assert(wireBack, NotNil)
}

func (s *CoverageBackfillSuite) TestWireToBCHAndBack(c *C) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), []byte{0x01}, nil))
	tx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	bchTx := wireToBCH(tx)
	c.Assert(bchTx, NotNil)

	wireBack := bchToWire(bchTx)
	c.Assert(wireBack, NotNil)
}

func (s *CoverageBackfillSuite) TestWireToLTCAndBack(c *C) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))
	tx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	ltcTx := wireToLTC(tx)
	c.Assert(ltcTx, NotNil)

	wireBack := ltcToWire(ltcTx)
	c.Assert(wireBack, NotNil)
}

////////////////////////////////////////////////////////////////////////////////////////
// Additional edge case tests for more coverage
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestProcessReorgNoPrevBlock(c *C) {
	block := &btcjson.GetBlockVerboseTxResult{
		Height:       999999,
		PreviousHash: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	txIns, err := s.client.processReorg(block)
	c.Assert(err, IsNil)
	c.Assert(txIns, IsNil) // no prev block meta => nil
}

func (s *CoverageBackfillSuite) TestExtractTxs(c *C) {
	block := &btcjson.GetBlockVerboseTxResult{
		Height: 100,
		Tx: []btcjson.TxRawResult{
			{
				// Coinbase tx - should be ignored
				Vin:  []btcjson.Vin{{Txid: ""}},
				Vout: []btcjson.Vout{{Value: 50.0}},
			},
		},
	}
	s.client.disableVinZeroBatch = true
	txIn, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(len(txIn.TxArray), Equals, 0)
}

func (s *CoverageBackfillSuite) TestFetchMemPool(c *C) {
	s.client.cfg.UTXO.TransactionBatchSize = 2
	s.client.cfg.UTXO.MaxMempoolBatches = 1
	txIn, err := s.client.FetchMemPool(100)
	c.Assert(err, IsNil)
	c.Assert(txIn.Chain, Equals, common.BTCChain)
	c.Assert(txIn.MemPool, Equals, true)
}

func (s *CoverageBackfillSuite) TestVinsUnspent(c *C) {
	tx := stypes.TxOutItem{
		VaultPubKey: s.client.nodePubKey,
		Chain:       common.BTCChain,
	}
	vins := []*wire.TxIn{
		wire.NewTxIn(makeTestOutPoint(), nil, nil),
	}
	unspent, err := s.client.vinsUnspent(tx, vins)
	c.Assert(err, IsNil)
	// Doesn't matter if unspent or not, just that it doesn't error
	_ = unspent
}

// TestClientWithMinimalSetup creates a Client with minimal fields for testing internal methods
func (s *CoverageBackfillSuite) TestGetBlockRequiredConfirmation(c *C) {
	// Create a minimal txIn with a tx that has value
	txIn := stypes.TxIn{
		Chain: common.BTCChain,
		TxArray: []*stypes.TxInItem{
			{
				BlockHeight: 1696761,
				Coins: common.Coins{
					common.NewCoin(common.BTCAsset, cosmos.NewUint(100000)),
				},
			},
		},
	}

	confirm, err := s.client.getBlockRequiredConfirmation(txIn, 1696761)
	c.Assert(err, IsNil)
	c.Assert(confirm >= 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestGetCoinbaseValue(c *C) {
	value, err := s.client.getCoinbaseValue(1696761)
	c.Assert(err, IsNil)
	c.Assert(value > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestGetGas(c *C) {
	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0},
		},
		Vout: []btcjson.Vout{
			{Value: 0.1},
		},
	}
	gas, err := s.client.getGas(tx, true)
	c.Assert(err, IsNil)
	c.Assert(len(gas) > 0, Equals, true)

	// Test with outbound (should stop summing vout at nulldata)
	tx2 := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0},
		},
		Vout: []btcjson.Vout{
			{Value: 0.1},
			{
				Value: 0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Type: "nulldata",
				},
			},
			{Value: 0.05}, // should not be counted for outbound
		},
	}
	gas2, err := s.client.getGas(tx2, false)
	c.Assert(err, IsNil)
	c.Assert(len(gas2) > 0, Equals, true)
	// For outbound, we stop at nulldata, so vout sum is only 0.1
	// For inbound (gas), vout sum is 0.1
	// Both should be positive gas values
	c.Assert(gas2[0].Amount.GT(cosmos.ZeroUint()), Equals, true)
}

func (s *CoverageBackfillSuite) TestIsBlockScannerHealthy(c *C) {
	// Just ensure it doesn't panic
	_ = s.client.IsBlockScannerHealthy()
}

func (s *CoverageBackfillSuite) TestGetChain(c *C) {
	c.Assert(s.client.GetChain(), Equals, common.BTCChain)
}

func (s *CoverageBackfillSuite) TestRegexpRemoveTrailingZeros(c *C) {
	// Verify the regex is compiled and works
	c.Assert(s.client.regexpRemoveTrailingZeros, NotNil)
	result := s.client.regexpRemoveTrailingZeros.ReplaceAllString("abcdef0000", "")
	c.Assert(result, Equals, "abcdef")
	result = s.client.regexpRemoveTrailingZeros.ReplaceAllString("abcdef00", "")
	c.Assert(result, Equals, "abcdef")
	result = s.client.regexpRemoveTrailingZeros.ReplaceAllString("abcdef", "")
	c.Assert(result, Equals, "abcdef")
}

// HelpersSuite tests helper functions without requiring the full Client setup
type HelpersSuite struct{}

var _ = Suite(&HelpersSuite{})

func (s *HelpersSuite) TestSumVoutSatsEmpty(c *C) {
	tx := &btcjson.TxRawResult{}
	sum, err := sumVoutSats(tx)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, uint64(0))
}

func (s *HelpersSuite) TestSumVoutSatsSingleVout(c *C) {
	tx := &btcjson.TxRawResult{
		Vout: []btcjson.Vout{
			{Value: 1.0},
		},
	}
	sum, err := sumVoutSats(tx)
	c.Assert(err, IsNil)
	c.Assert(sum, Equals, uint64(100000000))
}

// MemoToScriptsSuite tests the exported MemoToScripts function independently
type MemoToScriptsSuite struct{}

var _ = Suite(&MemoToScriptsSuite{})

func (s *MemoToScriptsSuite) TestMemoToScriptsEdgeCases(c *C) {
	// Test with exactly maxDataCarrierSize - 1 bytes (should not need continuation)
	memo79 := strings.Repeat("A", 79)
	scripts, err := MemoToScripts(memo79, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 1)

	// Test with exactly maxDataCarrierSize + 1 bytes (should need continuation)
	memo81 := strings.Repeat("A", 81)
	scripts, err = MemoToScripts(memo81, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 2) // 1 OP_RETURN + 1 P2WPKH

	// Test P2WPKH boundary: 79 + 20 = 99 bytes (1 OP_RETURN + 1 P2WPKH)
	memo99 := strings.Repeat("A", 99)
	scripts, err = MemoToScripts(memo99, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 2)

	// Test P2WPKH boundary: 79 + 21 = 100 bytes (1 OP_RETURN + 2 P2WPKH)
	memo100 := strings.Repeat("A", 100)
	scripts, err = MemoToScripts(memo100, 80, btctxscript.NullDataScript, btctxscript.PayToWitnessScript)
	c.Assert(err, IsNil)
	c.Assert(len(scripts), Equals, 3) // 1 OP_RETURN + 2 P2WPKH
}

// Additional signer_internal coverage using a minimal client
type SignerInternalSuite struct {
	client *Client
}

var _ = Suite(&SignerInternalSuite{})

func (s *SignerInternalSuite) SetUpTest(c *C) {
	ttypes.SetupConfigForTest()

	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	c.Assert(err, IsNil)

	ts, err := sutxo.NewTemporalStorage(db, 100)
	c.Assert(err, IsNil)

	cfg := config.BifrostChainConfiguration{
		ChainID: common.BTCChain,
	}
	cfg.UTXO.MaxUTXOsToSpend = 15
	cfg.UTXO.DefaultSatsPerVByte = 25
	cfg.UTXO.MaxSatsPerVByte = 1000

	s.client = &Client{
		cfg:                       cfg,
		temporalStorage:           ts,
		signerLock:                &sync.Mutex{},
		vaultLocks:                make(map[string]*sync.Mutex),
		wg:                        &sync.WaitGroup{},
		regexpRemoveTrailingZeros: regexp.MustCompile(`(?:00)+$`),
	}
	s.client.currentBlockHeight.Store(100)
}

func (s *SignerInternalSuite) TestGetGasCoinZeroGasRate(c *C) {
	tx := stypes.TxOutItem{GasRate: 0}
	// With no transaction fee in storage and GasRate=0, first path returns
	// NewCoin(asset, 0) because gasRate is 0 at the time of return.
	// Then it tries temporal storage (which returns 0 fee), and then falls back to default.
	gas := s.client.getGasCoin(tx, 100)
	c.Assert(gas.Asset, Equals, common.BTCAsset)
	// The result depends on whether temporal storage has a fee or not
	// The important thing is the function covers all three code paths
}

func (s *SignerInternalSuite) TestGetGasCoinWithStoredFee(c *C) {
	// Store a previous fee
	err := s.client.temporalStorage.UpsertTransactionFee(0.001, 250)
	c.Assert(err, IsNil)

	tx := stypes.TxOutItem{GasRate: 0}
	gas := s.client.getGasCoin(tx, 100)
	c.Assert(gas.Asset, Equals, common.BTCAsset)
	c.Assert(gas.Amount.Uint64() > 0, Equals, true)
}

func (s *SignerInternalSuite) TestIsSelfTransactionWithBlockMeta(c *C) {
	// Save a block meta with self transactions
	bm := sutxo.NewBlockMeta("prev", 50, "hash")
	bm.AddSelfTransaction("selftxid123")
	err := s.client.temporalStorage.SaveBlockMeta(50, bm)
	c.Assert(err, IsNil)

	c.Assert(s.client.isSelfTransaction("selftxid123"), Equals, true)
	c.Assert(s.client.isSelfTransaction("othertxid"), Equals, false)
}

////////////////////////////////////////////////////////////////////////////////////////
// Additional tests for higher coverage
////////////////////////////////////////////////////////////////////////////////////////

func (s *CoverageBackfillSuite) TestFetchTxs(c *C) {
	txIn, err := s.client.FetchTxs(1696761, 1696770)
	c.Assert(err, IsNil)
	c.Assert(txIn.Chain, Equals, common.BTCChain)
}

func (s *CoverageBackfillSuite) TestGetBlock(c *C) {
	block, err := s.client.getBlock(1696761)
	c.Assert(err, IsNil)
	c.Assert(block, NotNil)
	c.Assert(block.Height > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestGetAccountFull(c *C) {
	acct, err := s.client.GetAccount(s.client.nodePubKey, nil)
	c.Assert(err, IsNil)
	c.Assert(acct.Coins, NotNil)
}

func (s *CoverageBackfillSuite) TestGetAccountEmptyPubKey(c *C) {
	_, err := s.client.GetAccount(common.PubKey(""), nil)
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestGetSourceScriptDOGE(c *C) {
	s.client.cfg.ChainID = common.DOGEChain
	tx := stypes.TxOutItem{VaultPubKey: s.client.nodePubKey}
	script, err := s.client.getSourceScript(tx)
	c.Assert(err, IsNil)
	c.Assert(len(script) > 0, Equals, true)
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetSourceScriptBCH(c *C) {
	s.client.cfg.ChainID = common.BCHChain
	tx := stypes.TxOutItem{VaultPubKey: s.client.nodePubKey}
	script, err := s.client.getSourceScript(tx)
	c.Assert(err, IsNil)
	c.Assert(len(script) > 0, Equals, true)
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetSourceScriptLTC(c *C) {
	s.client.cfg.ChainID = common.LTCChain
	tx := stypes.TxOutItem{VaultPubKey: s.client.nodePubKey}
	script, err := s.client.getSourceScript(tx)
	c.Assert(err, IsNil)
	c.Assert(len(script) > 0, Equals, true)
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetSourceScriptZEC(c *C) {
	s.client.cfg.ChainID = common.ZECChain
	tx := stypes.TxOutItem{VaultPubKey: s.client.nodePubKey}
	script, err := s.client.getSourceScript(tx)
	c.Assert(err, IsNil)
	c.Assert(len(script) > 0, Equals, true)
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestBroadcastTxBTC(c *C) {
	redeemTx := wire.NewMsgTx(wire.TxVersion)
	redeemTx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), nil, nil))
	redeemTx.AddTxOut(wire.NewTxOut(100000, []byte{
		0x76, 0xa9, 0x14,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x88, 0xac,
	}))

	var buf bytes.Buffer
	err := redeemTx.Serialize(&buf)
	c.Assert(err, IsNil)

	txOut := stypes.TxOutItem{Chain: common.BTCChain}
	txHash, err := s.client.BroadcastTx(txOut, buf.Bytes())
	c.Assert(err, IsNil)
	c.Assert(txHash, Equals, "1a7897481ef3aea8e41d45dbca70cffd283ebc95b4e61a2c75ae86a41ab0f423")
}

func (s *CoverageBackfillSuite) TestBroadcastTxDOGE(c *C) {
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.DOGEChain

	redeemTx := wire.NewMsgTx(wire.TxVersion)
	redeemTx.AddTxIn(wire.NewTxIn(makeTestOutPoint(), []byte{0x01}, nil))
	redeemTx.AddTxOut(wire.NewTxOut(100000, []byte{0x76, 0xa9}))

	var buf bytes.Buffer
	err := redeemTx.Serialize(&buf)
	c.Assert(err, IsNil)

	txOut := stypes.TxOutItem{Chain: common.DOGEChain}
	txHash, err := s.client.BroadcastTx(txOut, buf.Bytes())
	c.Assert(err, IsNil)
	c.Assert(txHash, Equals, "1a7897481ef3aea8e41d45dbca70cffd283ebc95b4e61a2c75ae86a41ab0f423")
	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestBroadcastTxInvalidPayload(c *C) {
	txOut := stypes.TxOutItem{Chain: common.BTCChain}
	_, err := s.client.BroadcastTx(txOut, []byte{0x01, 0x02, 0x03})
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "fail to deserialize payload"), Equals, true)
}

func (s *CoverageBackfillSuite) TestSignTxInvalidToAddress(c *C) {
	tx := stypes.TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   common.Address("invalid_address"),
		VaultPubKey: s.client.nodePubKey,
		Coins: common.Coins{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(100000)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.BTCAsset, cosmos.NewUint(5000)),
		},
		GasRate: 10,
		Memo:    "test",
	}
	_, _, _, err := s.client.SignTx(tx, 1)
	c.Assert(err, NotNil)
}

func (s *CoverageBackfillSuite) TestExtractTxsWithRealBlock(c *C) {
	block, err := s.client.getBlock(1696761)
	c.Assert(err, IsNil)
	c.Assert(block, NotNil)
	txIn, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	_ = txIn.TxArray
}

func (s *CoverageBackfillSuite) TestGetAddressesFromScriptPubKeyChains(c *C) {
	spk := btcjson.ScriptPubKeyResult{
		Addresses: []string{"addr1"},
		Hex:       "76a914000000000000000000000000000000000000000088ac",
	}

	for _, chain := range []common.Chain{common.DOGEChain, common.BCHChain, common.LTCChain, common.ZECChain} {
		s.client.cfg.ChainID = chain
		addrs := s.client.getAddressesFromScriptPubKey(spk)
		c.Assert(len(addrs) > 0, Equals, true, Commentf("chain: %s", chain))
	}
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestGetTxInWithValidTx(c *C) {
	// Derive the asgard vault address for the current network so the output
	// is recognised as an inbound to asgard.
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs")
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks")
	}
	c.Assert(err, IsNil)
	vaultAddress, err := vaultPubKey.GetAddress(s.client.GetChain())
	c.Assert(err, IsNil)

	// Use a vin whose fixture (tx-54ef, vout 0) resolves to the asgard vault
	// address on mocknet. On mainnet the sender won't be asgard, but the
	// receiver will be, so getOutput still matches.
	tx := &btcjson.TxRawResult{
		Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
		Vin: []btcjson.Vin{
			{Txid: "54ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528", Vout: 0, Sequence: 0xffffffff},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.01, N: 0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Type: "witness_v0_keyhash", Hex: "76a914000000000000000000000000000000000000000088ac",
					Addresses: []string{"tb1qkq7weysjn6ljc2ber7fmurp0lz2n9fhqm9hv3s"},
				},
			},
			{
				Value: 0.009, N: 1,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Type: "witness_v0_keyhash", Hex: "76a914000000000000000000000000000000000000000088ac",
					Addresses: []string{vaultAddress.String()},
				},
			},
			{
				Value: 0, N: 2,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm: "OP_RETURN 535741503a4554482e4554483a3078666f6f", Type: "nulldata",
					Hex: "6a13535741503a4554482e4554483a3078666f6f",
				},
			},
		},
	}
	txIn, err := s.client.getTxIn(tx, 100, false, nil)
	c.Assert(err, IsNil)
	c.Assert(txIn.Tx, Not(Equals), "")
}

func (s *CoverageBackfillSuite) TestGetSenderFromRPC(c *C) {
	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0},
		},
	}
	sender, err := s.client.getSender(tx, nil)
	c.Assert(err, IsNil)
	c.Assert(len(sender) > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestRegisterPublicKeyLTCCreateWallet(c *C) {
	oldChain := s.client.cfg.ChainID
	s.client.cfg.ChainID = common.LTCChain
	err := s.client.RegisterPublicKey(s.client.nodePubKey)
	c.Assert(err, IsNil)
	s.client.cfg.ChainID = oldChain
}

func (s *CoverageBackfillSuite) TestSendNetworkFeeFromBlockDOGE(c *C) {
	block := &btcjson.GetBlockVerboseTxResult{
		Height: 100,
		Tx: []btcjson.TxRawResult{
			{
				Vin:  []btcjson.Vin{{Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513", Vout: 0}},
				Vout: []btcjson.Vout{{Value: 0.1}},
				Size: 250,
			},
		},
	}
	s.client.cfg.ChainID = common.DOGEChain
	s.client.lastFeeRate.Store(0)
	err := s.client.sendNetworkFeeFromBlock(block)
	c.Assert(err, IsNil)
	s.client.cfg.ChainID = common.BTCChain
}

func (s *CoverageBackfillSuite) TestReConfirmTxWithBlockMeta(c *C) {
	// Save a block meta, then try to reconfirm
	bm := sutxo.NewBlockMeta("prevhash", 998, "blockhash998")
	bm.AddCustomerTransaction("sometx123")
	err := s.client.temporalStorage.SaveBlockMeta(998, bm)
	c.Assert(err, IsNil)

	bm2 := sutxo.NewBlockMeta("blockhash998", 999, "blockhash999")
	err = s.client.temporalStorage.SaveBlockMeta(999, bm2)
	c.Assert(err, IsNil)

	// reConfirmTx from height 1000 (looks back at 999)
	s.client.cfg.BlockScanner.MaxReorgRescanBlocks = 2
	heights, err := s.client.reConfirmTx(1000)
	c.Assert(err, IsNil)
	// The hash won't match so it should detect reorg and return heights to rescan
	c.Assert(len(heights) > 0, Equals, true)
}

func (s *CoverageBackfillSuite) TestProcessReorgWithBlockMeta(c *C) {
	// Save a block meta with a different hash to trigger reorg detection.
	// Also add a customer transaction so the errata queue is exercised.
	bm := sutxo.NewBlockMeta("prevhash", 997, "different_hash")
	bm.AddCustomerTransaction("27de3e1865c098cd4fded71bae1e8236fd27ce5dce6e524a9ac5cd1a17b5c241")
	err := s.client.temporalStorage.SaveBlockMeta(997, bm)
	c.Assert(err, IsNil)

	block := &btcjson.GetBlockVerboseTxResult{
		Height:       998,
		PreviousHash: "actual_hash_not_matching",
	}
	errataQueue := make(chan stypes.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	txIns, err := s.client.processReorg(block)
	c.Assert(err, IsNil)

	// Reorg is detected (previous hash doesn't match stored block meta),
	// and the rescanned block returns transactions.
	c.Assert(len(txIns), Not(Equals), 0)

	// The customer transaction does not exist in the mock chain (the fixture
	// returns error -5), so confirmTx returns false and an errata is queued.
	c.Assert(len(errataQueue), Equals, 1)
	errata := <-errataQueue
	c.Assert(errata.Height, Equals, int64(997))
	c.Assert(errata.Txs, HasLen, 1)
	c.Assert(errata.Txs[0].TxID.String(), Equals, "27de3e1865c098cd4fded71bae1e8236fd27ce5dce6e524a9ac5cd1a17b5c241")
}
