package btc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	. "gopkg.in/check.v1"

	btypes "github.com/thornadocash/go-thornado/bifrost/blockscanner/types"
	"github.com/thornadocash/go-thornado/bifrost/frost"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	p2pstorage "github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/cmd"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

type BitcoinSuite struct {
	client *Client
	server *httptest.Server
	bridge thornadoclient.ThornadoBridge
	cfg    config.BifrostChainConfiguration
	m      *metrics.Metrics
	keys   *thornadoclient.Keys
}

var _ = Suite(&BitcoinSuite{})

func (s *BitcoinSuite) SetUpSuite(c *C) {
	ttypes.SetupConfigForTest()

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(bob, cKeys.English, cmd.ThornadoHDPath, password, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.keys = thornadoclient.NewKeysWithKeybase(kb, bob, password)
}

var btcChainRPCs = map[string]map[string]interface{}{}

func init() {
	// map the method and params to the loaded fixture
	loadFixture := func(path string) map[string]interface{} {
		f, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		var data map[string]interface{}
		err = json.NewDecoder(f).Decode(&data)
		if err != nil {
			panic(err)
		}
		return data
	}

	btcChainRPCs["getnetworkinfo"] = loadFixture("../../../../test/fixtures/btc/getnetworkinfo.json")
	btcChainRPCs["getblockhash"] = loadFixture("../../../../test/fixtures/btc/blockhash.json")
	btcChainRPCs["getblock"] = loadFixture("../../../../test/fixtures/btc/block_verbose.json")
	btcChainRPCs["getblockcount"] = loadFixture("../../../../test/fixtures/btc/blockcount.json")
	btcChainRPCs["importaddress"] = loadFixture("../../../../test/fixtures/btc/importaddress.json")
	btcChainRPCs["listunspent"] = loadFixture("../../../../test/fixtures/btc/listunspent.json")
	btcChainRPCs["getrawmempool"] = loadFixture("../../../../test/fixtures/btc/getrawmempool.json")
	btcChainRPCs["getblockstats"] = loadFixture("../../../../test/fixtures/btc/blockstats.json")
	btcChainRPCs["getrawtransaction-5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513"] = loadFixture("../../../../test/fixtures/btc/tx-5b08.json")
	btcChainRPCs["getrawtransaction-54ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528"] = loadFixture("../../../../test/fixtures/btc/tx-54ef.json")
	btcChainRPCs["getrawtransaction-64ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528"] = loadFixture("../../../../test/fixtures/btc/tx-64ef.json")
	btcChainRPCs["getrawtransaction-74ef2f4679fb90af42e8d963a5d85645d0fd86e5fe8ea4e69dbf2d444cb26528"] = loadFixture("../../../../test/fixtures/btc/tx-74ef.json")
	btcChainRPCs["getrawtransaction-27de3e1865c098cd4fded71bae1e8236fd27ce5dce6e524a9ac5cd1a17b5c241"] = loadFixture("../../../../test/fixtures/btc/tx-c241.json")
	btcChainRPCs["getrawtransaction"] = loadFixture("../../../../test/fixtures/btc/tx.json")
	btcChainRPCs["createwallet"] = loadFixture("../../../../test/fixtures/btc/createwallet.json")
	btcChainRPCs["sendrawtransaction"] = loadFixture("../../../../test/fixtures/btc/sendrawtransaction.json")
}

func (s *BitcoinSuite) SetUpTest(c *C) {
	s.m = GetMetricForTest(c, common.BTCChain)
	s.cfg = config.BifrostChainConfiguration{
		ChainID:     "BTC",
		UserName:    bob,
		Password:    password,
		DisableTLS:  true,
		HTTPostMode: true,
		BlockScanner: config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1, // avoids querying thornado for block height
		},
	}
	s.cfg.UTXO.TransactionBatchSize = 500
	s.cfg.UTXO.MaxMempoolBatches = 10
	s.cfg.UTXO.EstimatedAverageTxSize = 1000
	s.cfg.BlockScanner.MaxReorgRescanBlocks = 1
	ns := strconv.Itoa(time.Now().Nanosecond())

	thordir := filepath.Join(os.TempDir(), ns, ".thorcli")
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thornado",
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
		if req.RequestURI == "/" { // nolint
			body, _ := io.ReadAll(req.Body)
			if body[0] == '[' {
				handleBatchRPC(body, rw)
			} else {
				handleRPC(body, rw)
			}
		} else if strings.HasPrefix(req.RequestURI, "/thornado/node/") {
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		} else if req.RequestURI == "/thornado/lastblock" {
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/lastblock/btc.json")
		} else if strings.HasPrefix(req.RequestURI, "/auth/accounts/") {
			_, err := rw.Write([]byte(`{ "jsonrpc": "2.0", "id": "", "result": { "height": "0", "result": { "value": { "account_number": "0", "sequence": "0" } } } }`))
			c.Assert(err, IsNil)
		} else if req.RequestURI == "/txs" {
			_, err := rw.Write([]byte(`{"height": "1", "txhash": "AAAA000000000000000000000000000000000000000000000000000000000000", "logs": [{"success": "true", "log": ""}]}`))
			c.Assert(err, IsNil)
		} else if strings.HasPrefix(req.RequestURI, thornadoclient.BaseVaultEndpoint) {
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/base.json")
		} else if req.RequestURI == thornadoclient.StatusEndpoint {
			_, err := rw.Write([]byte(`{"result":{"sync_info":{"catching_up":false}}}`))
			c.Assert(err, IsNil)
		} else if req.RequestURI == "/thornado/config" {
			_, err := rw.Write([]byte(`{}`))
			c.Assert(err, IsNil)
		} else if req.RequestURI == "/thornado/config/defaults" {
			_, err := rw.Write([]byte(`{"UTXO":{"MaxSpendCount":-1}}`))
			c.Assert(err, IsNil)
		} else if req.RequestURI == "/thornado/vaults/pubkeys" {
			if common.CurrentChainNetwork == common.MainNet {
				httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys-Mainnet.json")
			} else {
				httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
			}
		}
	}))
	var err error
	cfg.ChainHost = s.server.Listener.Addr().String()
	cfg.ChainRPC = s.server.Listener.Addr().String()
	s.bridge, err = thornadoclient.NewThornadoBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
	s.cfg.RPCHost = s.server.Listener.Addr().String()
	s.client, err = NewClient(s.keys, s.cfg, s.bridge, &p2pstorage.MockLocalStateManager{}, s.m, &frost.InProcessSessionCoordinator{})
	s.client.disableVinZeroBatch = true
	s.client.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	c.Assert(err, IsNil)
	c.Assert(s.client, NotNil)
}

func (s *BitcoinSuite) TearDownTest(_ *C) {
	s.server.Close()
}

func (s *BitcoinSuite) TestGetBlock(c *C) {
	block, err := s.client.getBlock(1696761)
	c.Assert(err, IsNil)
	c.Assert(block.Hash, Equals, "000000008de7a25f64f9780b6c894016d2c63716a89f7c9e704ebb7e8377a0c8")
	c.Assert(block.Tx[0].Txid, Equals, "31f8699ce9028e9cd37f8a6d58a79e614a96e3fdd0f58be5fc36d2d95484716f")
	c.Assert(len(block.Tx), Equals, 112)
}

func (s *BitcoinSuite) TestFetchTxs(c *C) {
	txs, err := s.client.FetchTxs(0, 10)
	c.Assert(err, IsNil)
	c.Assert(txs.Chain, Equals, common.BTCChain)
	c.Assert(txs.TxArray, HasLen, 0)
	c.Assert(s.client.getCurrentBlockHeight(), Equals, int64(10))
}

func (s *BitcoinSuite) TestExtractTxsDoesNotToggleObservedTxCache(c *C) {
	block, err := s.client.getBlock(0)
	c.Assert(err, IsNil)
	s.markFixtureTxToBaseAddress(c, block, "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2")

	firstScan, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(firstScan.TxArray, HasLen, 1)
	c.Assert(firstScan.TxArray[0].Tx, Equals, "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2")

	secondScan, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(secondScan.TxArray, HasLen, 0)
	blockMeta, err := s.client.temporalStorage.GetBlockMeta(block.Height)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)
	c.Assert(blockMeta.CustomerTransactions, DeepEquals, []string{"24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2"})

	thirdScan, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(thirdScan.TxArray, HasLen, 0)
}

func (s *BitcoinSuite) TestExtractTxsPromotesMempoolObservationToFinal(c *C) {
	block, err := s.client.getBlock(0)
	c.Assert(err, IsNil)
	txid := "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2"
	s.markFixtureTxToBaseAddress(c, block, txid)

	_, _, err = s.client.temporalStorage.TrackObservedTxStage(txid, ObservedTxStageMempool)
	c.Assert(err, IsNil)

	finalScan, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(finalScan.TxArray, HasLen, 1)
	c.Assert(finalScan.TxArray[0].Tx, Equals, txid)

	duplicateFinalScan, err := s.client.extractTxs(block)
	c.Assert(err, IsNil)
	c.Assert(duplicateFinalScan.TxArray, HasLen, 0)
}

func (s *BitcoinSuite) markFixtureTxToBaseAddress(c *C, block *btcjson.GetBlockVerboseTxResult, txid string) {
	baseAddresses, err := s.client.getBaseAddress()
	c.Assert(err, IsNil)
	c.Assert(baseAddresses, Not(HasLen), 0)
	baseAddress := baseAddresses[0].String()

	for i := range block.Tx {
		if block.Tx[i].Txid != txid || len(block.Tx[i].Vout) == 0 {
			continue
		}
		block.Tx[i].Vout[0].ScriptPubKey.Addresses = []string{baseAddress}
		return
	}
	c.Fatalf("fixture tx %s not found", txid)
}

func (s *BitcoinSuite) TestGetSender(c *C) {
	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "31f8699ce9028e9cd37f8a6d58a79e614a96e3fdd0f58be5fc36d2d95484716f",
				Vout: 0,
			},
		},
	}
	sender, err := s.client.getSender(&tx, nil)
	c.Assert(err, IsNil)
	c.Assert(sender, Equals, "n3jYBjCzgGNydQwf83Hz6GBzGBhMkKfgL1")

	tx.Vin[0].Vout = 1
	sender, err = s.client.getSender(&tx, nil)
	c.Assert(err, IsNil)
	c.Assert(sender, Equals, "tb1qdxxlx4r4jk63cve3rjpj428m26xcukjn5yegff")

	assertSenderUTXOValidation(c, s.client)
}

func (s *BitcoinSuite) TestIgnoreTx(c *C) {
	var currentHeight int64 = 100

	// valid tx that will NOT be ignored
	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.12345678,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:       "",
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
					Type:      "",
				},
			},
		},
	}
	ignored := s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)

	// tx with LockTime later than current height should still be inspected.
	// Bitcoin Core handles finality for mempool/block acceptance; Bifrost's
	// local height can lag and should not permanently skip an otherwise valid tx.
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.12345678,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:       "",
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
					Type:      "",
				},
			},
		},
		LockTime: uint32(currentHeight) + 1,
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)

	// tx with LockTime equal to current height, so should not be ignored
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.12345678,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:       "",
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
					Type:      "",
				},
			},
		},
		LockTime: uint32(currentHeight),
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)

	// invalid tx missing Vout
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, true)

	// invalid tx missing vout[0].Value == no coins
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "nulldata",
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, true)

	// invalid tx missing vin[0].Txid means coinbase
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "nulldata",
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, true)

	// invalid tx missing vin
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, true)

	// invalid tx > 10 vout with coins we only expect 10 max
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, true)

	// valid tx == 2 vout with coins, 1 to vault, 1 with change back to user
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)

	// data output at first position should not affect filtering
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
					},
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)

	// data output in the middle should not affect filtering
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
					},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
					},
				},
			},
		},
	}
	ignored = s.client.ignoreTx(&tx, currentHeight)
	c.Assert(ignored, Equals, false)
}

func (s *BitcoinSuite) TestGetGas(c *C) {
	// vin[0] returns value 0.19590108
	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.12345678,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm: "",
				},
			},
		},
	}
	gas, err := s.client.getGas(&tx, true)
	c.Assert(err, IsNil)
	c.Assert(gas.Equals(common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(7244430))}), Equals, true)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm: "",
				},
			},
		},
	}
	gas, err = s.client.getGas(&tx, true)
	c.Assert(err, IsNil)
	c.Assert(gas.Equals(common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(149013))}), Equals, true)
}

func (s *BitcoinSuite) TestGetChain(c *C) {
	chain := s.client.GetChain()
	c.Assert(chain, Equals, common.BTCChain)
}

func (s *BitcoinSuite) TestGetHeight(c *C) {
	height, err := s.client.GetHeight()
	c.Assert(err, IsNil)
	c.Assert(height, Equals, int64(10))
}

func (s *BitcoinSuite) TestOnObservedTxIn(c *C) {
	pkey := ttypes.GetRandomPubKey()
	txIn := types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 1,
				Tx:          "31f8699ce9028e9cd37f8a6d58a79e614a96e3fdd0f58be5fc36d2d95484716f",
				Sender:      "bc1q2gjc0rnhy4nrxvuklk6ptwkcs9kcr59mcl2q9j",
				To:          "bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
				Coins: common.Coins{
					common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)),
				},
				ObservedVaultPubKey: pkey,
			},
		},
	}
	blockMeta := NewBlockMeta("000000001ab8a8484eb89f04b87d90eb88e2cbb2829e84eb36b966dcb28af90b", 1, "00000000ffa57c95f4f226f751114e9b24fdf8dbe2dbc02a860da9320bebd63e")
	c.Assert(s.client.temporalStorage.SaveBlockMeta(blockMeta.Height, blockMeta), IsNil)
	s.client.OnObservedTxIn(*txIn.TxArray[0], 1)
	blockMeta, err := s.client.temporalStorage.GetBlockMeta(1)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)

	txIn = types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 2,
				Tx:          "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Sender:      "bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
				To:          "bc1q2gjc0rnhy4nrxvuklk6ptwkcs9kcr59mcl2q9j",
				Coins: common.Coins{
					common.NewCoin(common.BTCAsset, cosmos.NewUint(123456)),
				},
				ObservedVaultPubKey: pkey,
			},
		},
	}
	blockMeta = NewBlockMeta("000000001ab8a8484eb89f04b87d90eb88e2cbb2829e84eb36b966dcb28af90b", 2, "00000000ffa57c95f4f226f751114e9b24fdf8dbe2dbc02a860da9320bebd63e")
	c.Assert(s.client.temporalStorage.SaveBlockMeta(blockMeta.Height, blockMeta), IsNil)
	s.client.OnObservedTxIn(*txIn.TxArray[0], 2)
	blockMeta, err = s.client.temporalStorage.GetBlockMeta(2)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)

	txIn = types.TxIn{
		Chain: common.BTCChain,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 3,
				Tx:          "44ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Sender:      "bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
				To:          "bc1q2gjc0rnhy4nrxvuklk6ptwkcs9kcr59mcl2q9j",
				Coins: common.Coins{
					common.NewCoin(common.BTCAsset, cosmos.NewUint(12345678)),
				},
				ObservedVaultPubKey: pkey,
			},
			{
				BlockHeight: 3,
				Tx:          "54ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Sender:      "bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
				To:          "bc1q2gjc0rnhy4nrxvuklk6ptwkcs9kcr59mcl2q9j",
				Coins: common.Coins{
					common.NewCoin(common.BTCAsset, cosmos.NewUint(123456)),
				},
				ObservedVaultPubKey: pkey,
			},
		},
	}
	blockMeta = NewBlockMeta("000000001ab8a8484eb89f04b87d90eb88e2cbb2829e84eb36b966dcb28af90b", 3, "00000000ffa57c95f4f226f751114e9b24fdf8dbe2dbc02a860da9320bebd63e")
	c.Assert(s.client.temporalStorage.SaveBlockMeta(blockMeta.Height, blockMeta), IsNil)
	for _, item := range txIn.TxArray {
		s.client.OnObservedTxIn(*item, 3)
	}

	blockMeta, err = s.client.temporalStorage.GetBlockMeta(3)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)
}

func (s *BitcoinSuite) TestProcessReOrg(c *C) {
	// can't get previous block meta should not error
	type response struct {
		Result btcjson.GetBlockVerboseResult `json:"result"`
	}
	res := response{}
	blockContent, err := os.ReadFile("../../../../test/fixtures/btc/block.json")
	c.Assert(err, IsNil)
	c.Assert(json.Unmarshal(blockContent, &res), IsNil)
	result := btcjson.GetBlockVerboseTxResult{
		Hash:         res.Result.Hash,
		PreviousHash: res.Result.PreviousHash,
		Height:       res.Result.Height,
	}
	// should not trigger re-org process
	reorgedItems, err := s.client.processReorg(&result)
	c.Assert(err, IsNil)
	c.Assert(reorgedItems, IsNil)

	// add one UTXO which will trigger the re-org process next
	previousHeight := result.Height - 1
	blockMeta := NewBlockMeta(ttypes.GetRandomTxHash().String(), previousHeight, ttypes.GetRandomTxHash().String())
	hash := "27de3e1865c098cd4fded71bae1e8236fd27ce5dce6e524a9ac5cd1a17b5c241"
	blockMeta.AddCustomerTransaction(hash)
	c.Assert(s.client.temporalStorage.SaveBlockMeta(previousHeight, blockMeta), IsNil)
	s.client.globalErrataQueue = make(chan types.ErrataBlock, 1)
	s.client.updateCurrentBlockHeight(previousHeight)
	reorgedItems, err = s.client.processReorg(&result)
	c.Assert(errors.Is(err, btypes.ErrPendingErrataDelay), Equals, true)
	c.Assert(reorgedItems, IsNil)
	c.Assert(s.client.globalErrataQueue, HasLen, 0)

	s.client.updateCurrentBlockHeight(previousHeight + btcErrataDelayBlocks)
	reorgedItems, err = s.client.processReorg(&result)
	c.Assert(err, IsNil)
	c.Assert(reorgedItems, NotNil)
	c.Assert(s.client.globalErrataQueue, HasLen, 1)
	blockMeta, err = s.client.temporalStorage.GetBlockMeta(previousHeight)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)
}

func (s *BitcoinSuite) TestGetMemPool(c *C) {
	txIns, err := s.client.FetchMemPool(1024)
	c.Assert(err, IsNil)
	c.Assert(txIns.TxArray, HasLen, 0)

	// Fixture mempool txs do not pay to a Thornado address.
	txIns, err = s.client.FetchMemPool(1024)
	c.Assert(err, IsNil)
	c.Assert(txIns.TxArray, HasLen, 0)
}

func (s *BitcoinSuite) TestGetTxInAllowsRBFMempoolInbound(c *C) {
	baseAddresses, err := s.client.getBaseAddress()
	c.Assert(err, IsNil)
	c.Assert(baseAddresses, Not(HasLen), 0)

	tx := btcjson.TxRawResult{
		Txid: "rbf-mempool-inbound",
		Hash: "rbf-mempool-inbound",
		Vin: []btcjson.Vin{
			{
				Txid:     "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
				Vout:     0,
				Sequence: 0xfffffffd,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.11000000,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{baseAddresses[0].String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
		},
	}

	txIn, err := s.client.getTxIn(&tx, 1024, true, nil)
	c.Assert(err, IsNil)
	c.Assert(txIn.IsEmpty(), Equals, false)
	c.Assert(txIn.Tx, Equals, tx.Txid)
	c.Assert(txIn.BlockHeight, Equals, int64(1024))
}

func (s *BitcoinSuite) TestConfirmedMempoolTxRetainsMarker(c *C) {
	txid := ttypes.GetRandomTxHash().String()
	_, err := s.client.temporalStorage.TrackMempoolTx(txid)
	c.Assert(err, IsNil)

	key := fmt.Sprintf("getrawtransaction-%s", txid)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"result": map[string]interface{}{
			"txid": txid,
			"hash": txid,
			"vin":  []map[string]interface{}{},
			"vout": []map[string]interface{}{},
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	errataQueue := make(chan types.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	s.client.errataDroppedMempoolTxs(1024, map[string]struct{}{})

	c.Assert(len(errataQueue), Equals, 0)
	exists, err := s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *BitcoinSuite) TestDroppedMempoolTxQueuesErrata(c *C) {
	txid := ttypes.GetRandomTxHash().String()
	_, err := s.client.temporalStorage.TrackMempoolTx(txid)
	c.Assert(err, IsNil)

	key := fmt.Sprintf("getrawtransaction-%s", txid)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -5,
			"message": "No such mempool or blockchain transaction",
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	errataQueue := make(chan types.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	s.client.errataDroppedMempoolTxs(1024, map[string]struct{}{})

	c.Assert(errataQueue, HasLen, 0)
	exists, err := s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	s.client.errataDroppedMempoolTxs(1024+btcErrataDelayBlocks, map[string]struct{}{})

	c.Assert(errataQueue, HasLen, 1)
	errata := <-errataQueue
	c.Assert(errata.Height, Equals, int64(1024+btcErrataDelayBlocks))
	c.Assert(errata.Txs, HasLen, 1)
	c.Assert(errata.Txs[0].TxID.String(), Equals, txid)
	c.Assert(errata.Txs[0].Chain, Equals, common.BTCChain)

	exists, err = s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)
}

func (s *BitcoinSuite) TestConfirmationCountReadyQueuesErrataForMissingObservedTx(c *C) {
	txid := ttypes.GetRandomTxHash().String()
	_, err := s.client.temporalStorage.TrackMempoolTx(txid)
	c.Assert(err, IsNil)
	_, _, err = s.client.temporalStorage.TrackObservedTxStage(txid, ObservedTxStageMempool)
	c.Assert(err, IsNil)

	key := fmt.Sprintf("getrawtransaction-%s", txid)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -5,
			"message": "No such mempool or blockchain transaction",
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	errataQueue := make(chan types.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	s.client.updateCurrentBlockHeight(1024)
	ready := s.client.ConfirmationCountReady(types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 1,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 1024,
				Tx:          txid,
			},
		},
	})

	c.Assert(ready, Equals, false)
	c.Assert(errataQueue, HasLen, 0)
	exists, err := s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	s.client.updateCurrentBlockHeight(1024 + btcErrataDelayBlocks)
	ready = s.client.ConfirmationCountReady(types.TxIn{
		Chain:                common.BTCChain,
		ConfirmationRequired: 1,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 1024,
				Tx:          txid,
			},
		},
	})

	c.Assert(ready, Equals, false)
	c.Assert(errataQueue, HasLen, 1)
	errata := <-errataQueue
	c.Assert(errata.Height, Equals, int64(1024))
	c.Assert(errata.Txs, HasLen, 1)
	c.Assert(errata.Txs[0].TxID.String(), Equals, txid)
	c.Assert(errata.Txs[0].Chain, Equals, common.BTCChain)

	exists, err = s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)
	added, previousStage, err := s.client.temporalStorage.TrackObservedTxStage(txid, ObservedTxStageFinal)
	c.Assert(err, IsNil)
	c.Assert(added, Equals, true)
	c.Assert(previousStage, Equals, "")
}

func (s *BitcoinSuite) TestConfirmationCountReadyWaitsForMissingSignedObservedTx(c *C) {
	txid := ttypes.GetRandomTxHash().String()
	_, err := s.client.temporalStorage.TrackMempoolTx(txid)
	c.Assert(err, IsNil)
	_, _, err = s.client.temporalStorage.TrackObservedTxStage(txid, ObservedTxStageMempool)
	c.Assert(err, IsNil)

	key := fmt.Sprintf("getrawtransaction-%s", txid)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -5,
			"message": "No such mempool or blockchain transaction",
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	errataQueue := make(chan types.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	s.client.updateCurrentBlockHeight(1024)
	ready := s.client.ConfirmationCountReady(types.TxIn{
		Chain:                  common.BTCChain,
		ConfirmationRequired:   1,
		AllowFutureObservation: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 1024,
				Tx:          txid,
			},
		},
	})

	c.Assert(ready, Equals, false)
	c.Assert(errataQueue, HasLen, 0)
	exists, err := s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *BitcoinSuite) TestConfirmationCountReadyQueuesErrataForMissingMempoolObservedTx(c *C) {
	txid := ttypes.GetRandomTxHash().String()
	_, err := s.client.temporalStorage.TrackMempoolTx(txid)
	c.Assert(err, IsNil)
	_, _, err = s.client.temporalStorage.TrackObservedTxStage(txid, ObservedTxStageMempool)
	c.Assert(err, IsNil)

	key := fmt.Sprintf("getrawtransaction-%s", txid)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -5,
			"message": "No such mempool or blockchain transaction",
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	errataQueue := make(chan types.ErrataBlock, 1)
	s.client.globalErrataQueue = errataQueue
	s.client.updateCurrentBlockHeight(2048)
	ready := s.client.ConfirmationCountReady(types.TxIn{
		Chain:   common.BTCChain,
		MemPool: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 0,
				Tx:          txid,
			},
		},
	})

	c.Assert(ready, Equals, false)
	c.Assert(errataQueue, HasLen, 0)
	exists, err := s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	s.client.updateCurrentBlockHeight(2048 + btcErrataDelayBlocks)
	ready = s.client.ConfirmationCountReady(types.TxIn{
		Chain:   common.BTCChain,
		MemPool: true,
		TxArray: []*types.TxInItem{
			{
				BlockHeight: 0,
				Tx:          txid,
			},
		},
	})

	c.Assert(ready, Equals, false)
	c.Assert(errataQueue, HasLen, 1)
	errata := <-errataQueue
	c.Assert(errata.Height, Equals, int64(2048+btcErrataDelayBlocks))
	c.Assert(errata.Txs, HasLen, 1)
	c.Assert(errata.Txs[0].TxID.String(), Equals, txid)
	c.Assert(errata.Txs[0].Chain, Equals, common.BTCChain)

	exists, err = s.client.temporalStorage.HasMempoolTx(txid)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)
}

func (s *BitcoinSuite) TestGetOutput(c *C) {
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs") // from PubKeys-Mainnet.json
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks") // from PubKeys.json
	}
	c.Assert(err, IsNil, Commentf(vaultPubKey.String()))
	vaultAddress, err := vaultPubKey.GetAddress(s.client.GetChain())
	c.Assert(err, IsNil)
	vaultAddressString := vaultAddress.String()

	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	out, err := s.client.getOutput(vaultAddressString, &tx, false)
	c.Assert(err, IsNil, Commentf(vaultAddressString))
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, "tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835")
	c.Assert(out.Value, Equals, 1.49655603)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
				},
			},
		},
	}
	out, err = s.client.getOutput(vaultAddressString, &tx, false)
	c.Assert(err, IsNil)
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, "tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835")
	c.Assert(out.Value, Equals, 1.49655603)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
				},
			},
		},
	}
	out, err = s.client.getOutput(vaultAddressString, &tx, false)
	c.Assert(err, IsNil)
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, "tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835")
	c.Assert(out.Value, Equals, 1.49655603)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
				},
			},
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	out, err = s.client.getOutput(vaultAddressString, &tx, false)
	c.Assert(err, IsNil)
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, "tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835")
	c.Assert(out.Value, Equals, 1.49655603)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				Value: 0.00195384,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddressString},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	out, err = s.client.getOutput(vaultAddressString, &tx, true)
	c.Assert(err, IsNil)
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, vaultAddressString)
	c.Assert(out.Value, Equals, 1.49655603)

	childPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	childAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, childPath)
	c.Assert(err, IsNil)
	childAddressString := childAddress.String()
	s.client.rememberVaultPath(vaultPubKey, childPath)
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 1.49655603,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{childAddressString},
				},
			},
		},
	}
	out, err = s.client.getOutput(childAddressString, &tx, true)
	c.Assert(err, IsNil)
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, childAddressString)
	c.Assert(out.Value, Equals, 1.49655603)

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Type: "nulldata",
				},
			},
			{
				Value: 0.01000000,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
				},
			},
			{
				Value: 0.20000000,
				N:     2,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{childAddressString},
				},
			},
			{
				Value: 0.02000000,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
		},
	}
	out, err = s.client.getOutput("tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835", &tx, false)
	c.Assert(err, IsNil)
	c.Assert(out.N, Equals, uint32(2))
	c.Assert(out.ScriptPubKey.Addresses[0], Equals, childAddressString)
	c.Assert(out.Value, Equals, 0.20000000)

	// invalid tx only multiple (positive-value) vout Addresses
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{
			{
				Txid: "5b0876dcc027d2f0c671fc250460ee388df39697c3ff082007b6ddd9cb9a7513",
				Vout: 1,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.1234565,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{
						"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6",
						"bc1q0s4mg25tu6termrk8egltfyme4q7sg3h0e56p3",
					},
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "",
					Type: "",
				},
			},
		},
	}
	out, err = s.client.getOutput(vaultAddressString, &tx, true)
	c.Assert(err, NotNil)
}

func (s *BitcoinSuite) TestGetTxInObservesRegisteredPathSelfConsolidation(c *C) {
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs")
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks")
	}
	c.Assert(err, IsNil)
	childPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	childAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, childPath)
	c.Assert(err, IsNil)
	childAddressString := childAddress.String()
	s.client.rememberVaultPath(vaultPubKey, childPath)

	prevTxID := strings.Repeat("a", 64)
	selfTxID := strings.Repeat("b", 64)
	key := fmt.Sprintf("getrawtransaction-%s", prevTxID)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"error":   nil,
		"result": map[string]interface{}{
			"txid": prevTxID,
			"hash": prevTxID,
			"vin": []map[string]interface{}{
				{"coinbase": "00", "sequence": 4294967295},
			},
			"vout": []map[string]interface{}{
				{
					"value": 1.0,
					"n":     0,
					"scriptPubKey": map[string]interface{}{
						"hex":       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
						"type":      "witness_v1_taproot",
						"addresses": []string{childAddressString},
					},
				},
			},
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	tx := btcjson.TxRawResult{
		Txid: selfTxID,
		Hash: selfTxID,
		Vin: []btcjson.Vin{
			{
				Txid: prevTxID,
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				Value: 0.997,
				N:     0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{childAddressString},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
		},
	}
	txIn, err := s.client.getTxIn(&tx, 1024, false, nil)
	c.Assert(err, IsNil)
	c.Assert(txIn.IsEmpty(), Equals, false)
	c.Assert(txIn.Tx, Equals, selfTxID)
	c.Assert(txIn.Sender, Equals, childAddressString)
	c.Assert(txIn.To, Equals, childAddressString)
	c.Assert(txIn.SourceInputs, HasLen, 1)
	c.Assert(txIn.SourceInputs[0].TxID.String(), Equals, strings.ToUpper(prevTxID))
	c.Assert(txIn.SourceInputs[0].Vout, Equals, uint32(0))
	c.Assert(txIn.SourceInputs[0].AmountSats, Equals, uint64(100_000_000))
}

func (s *BitcoinSuite) TestGetTxInsObservesMultipleRegisteredVaultOutputs(c *C) {
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs")
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks")
	}
	c.Assert(err, IsNil)
	firstPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 1, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	secondPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 2, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	firstAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, firstPath)
	c.Assert(err, IsNil)
	secondAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, secondPath)
	c.Assert(err, IsNil)
	s.client.rememberVaultPath(vaultPubKey, firstPath)
	s.client.rememberVaultPath(vaultPubKey, secondPath)

	prevTxID := strings.Repeat("c", 64)
	txID := strings.Repeat("d", 64)
	sender := "tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"
	key := fmt.Sprintf("getrawtransaction-%s", prevTxID)
	previous, hadPrevious := btcChainRPCs[key]
	btcChainRPCs[key] = map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"error":   nil,
		"result": map[string]interface{}{
			"txid": prevTxID,
			"hash": prevTxID,
			"vin": []map[string]interface{}{
				{"coinbase": "00", "sequence": 4294967295},
			},
			"vout": []map[string]interface{}{
				{
					"value": 1.0,
					"n":     0,
					"scriptPubKey": map[string]interface{}{
						"hex":       "00140653096f54ae1ae2d73291d15854aef08ebcfa8c",
						"type":      "witness_v0_keyhash",
						"addresses": []string{sender},
					},
				},
			},
		},
	}
	defer func() {
		if hadPrevious {
			btcChainRPCs[key] = previous
		} else {
			delete(btcChainRPCs, key)
		}
	}()

	tx := btcjson.TxRawResult{
		Txid: txID,
		Hash: txID,
		Vin: []btcjson.Vin{
			{
				Txid: prevTxID,
				Vout: 0,
			},
		},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{Type: "nulldata"},
			},
			{
				Value: 0.10000000,
				N:     1,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				Value: 0.20000000,
				N:     2,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{firstAddress.String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
			{
				Value: 0.05000000,
				N:     3,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1q6v28g5n75j6mnf3j4dqly5gdp9ztmn33xvay0u"},
				},
			},
			{
				Value: 0.30000000,
				N:     4,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{secondAddress.String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
		},
	}
	txIns, err := s.client.getTxIns(&tx, 1024, false, nil)
	c.Assert(err, IsNil)
	c.Assert(txIns, HasLen, 2)
	c.Assert(txIns[0].Tx, Equals, txID)
	c.Assert(txIns[0].To, Equals, firstAddress.String())
	c.Assert(txIns[0].SourceVout, Equals, uint32(2))
	c.Assert(txIns[0].Coins[0].Amount.Uint64(), Equals, uint64(20_000_000))
	c.Assert(txIns[1].To, Equals, secondAddress.String())
	c.Assert(txIns[1].SourceVout, Equals, uint32(4))
	c.Assert(txIns[1].Coins[0].Amount.Uint64(), Equals, uint64(30_000_000))
	c.Assert(s.client.observedTxCacheKey(txIns[0]), Equals, txID+":2")
	c.Assert(s.client.observedTxCacheKey(txIns[1]), Equals, txID+":4")
}

func (s *BitcoinSuite) TestIsBaseAddressFallsBackToRegisteredVaultPath(c *C) {
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs")
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks")
	}
	c.Assert(err, IsNil)
	childPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 0, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	childAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, childPath)
	c.Assert(err, IsNil)

	s.client.bridge = &mockBaseBridge{}
	s.client.rememberVaultPath(vaultPubKey, childPath)

	c.Assert(s.client.isBaseAddress(childAddress.String()), Equals, true)
}

func (s *BitcoinSuite) TestIsBaseAddressAcceptsBridgeDepositAddress(c *C) {
	var vaultPubKey common.PubKey
	var err error
	if common.CurrentChainNetwork == common.MainNet {
		vaultPubKey, err = common.NewPubKey("thorpub1addwnpepqwprh5vd0rrk78kd98qjruuazwvapnxft7f86w7hlf768whxytpn5quf2gs")
	} else {
		vaultPubKey, err = common.NewPubKey("tthorpub1addwnpepqflvfv08t6qt95lmttd6wpf3ss8wx63e9vf6fvyuj2yy6nnyna576rfzjks")
	}
	c.Assert(err, IsNil)
	childPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 1, common.DepositPathCommitmentRoot)
	c.Assert(err, IsNil)
	childAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, childPath)
	c.Assert(err, IsNil)

	s.client.bridge = &mockBaseBridge{depositAddr: childAddress}

	c.Assert(s.client.isBaseAddress(childAddress.String()), Equals, true)
}

func (s *BitcoinSuite) TestIsValidUTXO(c *C) {
	// normal pay to pubkey hash segwit
	c.Assert(s.client.isValidUTXO("00140653096f54ae1ae2d73291d15854aef08ebcfa8c"), Equals, true)
	// pubkey hash , bitcoin client doesn't use it
	c.Assert(s.client.isValidUTXO("76a91415fb126815935f6ae83a206d7d82f1065bc63e2588ac"), Equals, true)

	c.Assert(s.client.isValidUTXO("a914e51a3dd98ded55718ad2cf2ce7c8ff056394445787"), Equals, true)
	c.Assert(s.client.isValidUTXO("00483045022100995187373cabc9ef02e5dd2770519704054bff6e3b42f8eeeb1f08a40db527b50220380d4d1f471087c35ebdde4251a0c8fa38db688600020a189d2b19343d079c100147304402205c8a886fece4c40c96c47ee51cf8e32ff75251375a47c4ec0cec9193a8a747620220703045742cdec7a19e16aa071a4fe4333a6c1b587783b864d0a64ef87783b13a014c695221039b3fa7e3dd5f9caab777f0dd15a03f1011063a2bf205f96ad2b01540506109432103e7e00ea57b70cfd9493f1d7e482a2bfe4c785d8e9bef25eb1fd3a528bc452e072103f01a388aecf967af2d21a8578635e745a3990afdfde8099ac44bab3ecd9c042153ae"), Equals, false)
	c.Assert(s.client.isValidUTXO("51210281feb90c058c3436f8bc361930ae99fcfb530a699cdad141d7244bfcad521a1f51ae"), Equals, false)
	c.Assert(s.client.isValidUTXO("5121037953dbf08030f67352134992643d033417eaa6fcfb770c038f364ff40d7615882100bd2fda4cf456d64386a0756f580101a607c25bd8d6814693bdf16e2a7ba3e45c52ae"), Equals, false)
	c.Assert(s.client.isValidUTXO("524104d81fd577272bbe73308c93009eec5dc9fc319fc1ee2e7066e17220a5d47a18314578be2faea34b9f1f8ca078f8621acd4bc22897b03daa422b9bf56646b342a24104ec3afff0b2b66e8152e9018fe3be3fc92b30bf886b3487a525997d00fd9da2d012dce5d5275854adc3106572a5d1e12d4211b228429f5a7b2f7ba92eb0475bb14104b49b496684b02855bc32f5daefa2e2e406db4418f3b86bca5195600951c7d918cdbe5e6d3736ec2abf2dd7610995c3086976b2c0c7b4e459d10b34a316d5a5e753ae"), Equals, false)

	// V1_P2TR (pay-to-taproot) output
	c.Assert(s.client.isValidUTXO("5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f"), Equals, true)
}
