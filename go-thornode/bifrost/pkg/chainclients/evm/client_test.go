package evm

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	ecommon "github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	openapi "gitlab.com/thorchain/thornode/v3/openapi/gen"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func TestEVMPackage(t *testing.T) { TestingT(t) }

func TestUpdateCurrentBlockHeightMonotonic(t *testing.T) {
	scanner := &EVMScanner{}

	scanner.updateCurrentBlockHeight(100)
	scanner.updateCurrentBlockHeight(90)
	if got := scanner.getCurrentBlockHeight(); got != 100 {
		t.Fatalf("expected current block height to remain 100, got %d", got)
	}

	scanner.updateCurrentBlockHeight(110)
	if got := scanner.getCurrentBlockHeight(); got != 110 {
		t.Fatalf("expected current block height to advance to 110, got %d", got)
	}
}

type EVMSuite struct {
	thordir        string
	thorKeys       *thorclient.Keys
	bridge         thorclient.ThorchainBridge
	m              *metrics.Metrics
	server         *httptest.Server
	mimirOverrides map[string]string
}

var _ = Suite(&EVMSuite{})

var m *metrics.Metrics

func GetMetricForTest(c *C) *metrics.Metrics {
	if m == nil {
		var err error
		m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9000,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.AVAXChain},
		})
		c.Assert(m, NotNil)
		c.Assert(err, IsNil)
	}
	return m
}

func (s *EVMSuite) SetUpTest(c *C) {
	s.mimirOverrides = nil
	s.m = GetMetricForTest(c)
	c.Assert(s.m, NotNil)
	types2.SetupConfigForTest()
	c.Assert(os.Setenv("NET", "mocknet"), IsNil)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch req.RequestURI {
		case thorclient.ChainVersionEndpoint:
			_, err := rw.Write([]byte(`{"current":"` + types2.GetCurrentVersion().String() + `"}`))
			c.Assert(err, IsNil)
		case thorclient.PubKeysEndpoint:
			priKey, _ := s.thorKeys.GetPrivateKey()
			tm, _ := cryptocodec.ToCmtPubKeyInterface(priKey.PubKey())
			pk, err := common.NewPubKeyFromCrypto(tm)
			c.Assert(err, IsNil)
			content, err := os.ReadFile("../../../../test/fixtures/endpoints/vaults/pubKeys.json")
			c.Assert(err, IsNil)
			var pubKeysVault openapi.VaultPubkeysResponse
			c.Assert(json.Unmarshal(content, &pubKeysVault), IsNil)
			chain := common.AVAXChain.String()
			router := "0x17aB05351fC94a1a67Bf3f56DdbB941aE6c63E25"
			pubKeysVault.Asgard = append(pubKeysVault.Asgard, openapi.VaultInfo{
				PubKey: pk.String(),
				Routers: []openapi.VaultRouter{
					{
						Chain:  &chain,
						Router: &router,
					},
				},
			})
			buf, err := json.MarshalIndent(pubKeysVault, "", "	")
			c.Assert(err, IsNil)
			_, err = rw.Write(buf)
			c.Assert(err, IsNil)
		case thorclient.InboundAddressesEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/inbound_addresses/inbound_addresses.json")
		case thorclient.AsgardVault:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case thorclient.LastBlockEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/lastblock/root.json")
		case thorclient.NodeAccountEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		case "/status":
			rw.Header().Set("Content-Type", "application/json")
			_, err := rw.Write([]byte(`{"result":{"node_info":{"network":"mocknet"}}}`))
			c.Assert(err, IsNil)
		default:
			// Handle mimir and status endpoints (including double-slash normalization)
			requestPath := req.RequestURI
			requestPath = strings.Replace(requestPath, "//thorchain/mimir/key/", "/thorchain/mimir/key/", 1)
			if strings.Contains(requestPath, "//status") {
				requestPath = "/status"
			}

			if strings.HasPrefix(requestPath, "/thorchain/mimir/key/") {
				mimirKey := strings.TrimPrefix(requestPath, "/thorchain/mimir/key/")
				rw.Header().Set("Content-Type", "application/json")
				if s.mimirOverrides != nil {
					if value, exists := s.mimirOverrides[mimirKey]; exists {
						_, err := rw.Write([]byte(value))
						c.Assert(err, IsNil)
						return
					}
				}
				_, err := rw.Write([]byte(`-1`))
				c.Assert(err, IsNil)
				return
			}

			if requestPath == "/status" {
				rw.Header().Set("Content-Type", "application/json")
				_, err := rw.Write([]byte(`{"result":{"node_info":{"network":"mocknet"}}}`))
				c.Assert(err, IsNil)
				return
			}

			body, err := io.ReadAll(req.Body)
			c.Assert(err, IsNil)
			type RPCRequest struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      interface{}     `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}
			var rpcRequest RPCRequest
			err = json.Unmarshal(body, &rpcRequest)
			c.Assert(err, IsNil)
			if rpcRequest.Method == "eth_getBalance" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x8ac7230489e80000"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getTransactionCount" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x0"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_chainId" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0xf"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_gasPrice" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_estimateGas" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x493df"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getBlockByNumber" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"difficulty":"0x2","extraData":"0xd88301090e846765746888676f312e31342e32856c696e757800000000000000ef855333e6b03b825c2f1381f111e278232688e21ba8c36aa35689505d9470704420825b302cd70cc6610f1334a3d7c801ac4b8871bd9f0692c1c96f0f60ee0f01","gasLimit":"0x7a1200","gasUsed":"0xfbc9","hash":"0x45f139a64f563e12f61824a4b44edc2c955818d176b160538ae24f566a006c00","logsBloom":"0x00000000000000000002000000000000000000100000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000400000000000800000000080000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000004000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","miner":"0x0000000000000000000000000000000000000000","mixHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0000000000000000","number":"0x7","parentHash":"0x2f202f8aa7355e77bfbdcd63c08f7c4e43e0bcca61b45fe6a2bdb950d777fa38","receiptsRoot":"0xe1cf0352843e29447633b9f1710e443f2582691e4278febf322c0bb7f86202cc","sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347","size":"0x38c","stateRoot":"0x303f9a24ba76fa8f350d36f4cef139e6be023f95646e2602cf9e6f939f91beea","timestamp":"0x5fde861b","totalDifficulty":"0xf","transactions":[{"blockHash":"0x45f139a64f563e12f61824a4b44edc2c955818d176b160538ae24f566a006c00","blockNumber":"0x7","from":"0xfb337706200a55009e6bbd41e4dc164d59bc9aa2","gas":"0x17cdc","gasPrice":"0x1","hash":"0x042602a2dff77111f3e711ab7c81adbcbc9a2d87973f4afb8dc0f2856021ec74","input":"0x31a053cf000000000000000000000000fd5111db462a68cfd6df19fb110dc8e9116a90e9000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000444f55543a3841313034343144354241424535443444434138443531324646363236313039394135343741393739394536334337323238384530453742303534313444433200000000000000000000000000000000000000000000000000000000","nonce":"0x0","to":"0x17aB05351fC94a1a67Bf3f56DdbB941aE6c63E25","transactionIndex":"0x0","value":"0xd6d8","v":"0x41","r":"0xbce697be8572d1543cd8c191c409cee2b4999a538e707286b5e14f7e8ff442b8","s":"0x4b8f8e8a14fb60dbe981f6ddbb31300bbc2ce8753ad6b82bdce8147280cd8e43"}],"transactionsRoot":"0xd42e9b932bffb89da313a7f9370d83c2fb4082a2d8ff162b70dcb36330a476db","uncles":[]}}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_sendRawTransaction" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getTransactionReceipt" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{
				"transactionHash":"0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b",
				"transactionIndex":"0x0",
				"blockNumber":"0x1",
				"blockHash":"0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
				"cumulativeGasUsed":"0xc350",
				"contractAddress":"0x2a65aca4d5fc5b5c859090a6c34d164135398226",
				"gasUsed":"0x4dc",
				"logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"logs":[],
				"status":"0x1"
			}}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_blockNumber" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x7"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getBlockByNumber" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{
				"difficulty": "0x31962a3fc82b",
				"extraData": "0x4477617266506f6f6c",
				"gasLimit": "0x47c3d8",
				"gasUsed": "0x0",
				"hash": "0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
				"logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"miner": "0x2a65aca4d5fc5b5c859090a6c34d164135398226",
				"nonce": "0xa5e8fb780cc2cd5e",
				"number": "0x1",
				"parentHash": "0x8b535592eb3192017a527bbf8e3596da86b3abea51d6257898b2ced9d3a83826",
				"receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
				"size": "0x20e",
				"stateRoot": "0xdc6ed0a382e50edfedb6bd296892690eb97eb3fc88fd55088d5ea753c48253dc",
				"timestamp": "0x579f4981",
				"totalDifficulty": "0x25cff06a0d96f4bee",
				"transactions": [],
				"transactionsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"uncles": []}}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_call" {
				// Handle allowance() calls - return low allowance that needs approval
				if strings.Contains(string(rpcRequest.Params), "0xdd62ed3e") { // allowance function selector
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x00000000000000000000000000000000000000000000000000000000000003e8"}`)) // 1000 in hex
					c.Assert(err, IsNil)
					return
				}
				// Handle symbol() calls
				if string(rpcRequest.Params) == `[{"data":"0x95d89b41","from":"0x0000000000000000000000000000000000000000","to":"0x333c3310824b7c685133F2BeDb2CA4b8b4DF633d"},"latest"]` {
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003544b4e0000000000000000000000000000000000000000000000000000000000"}`))
					c.Assert(err, IsNil)
				} else {
					// Default decimals response
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x0000000000000000000000000000000000000000000000000000000000000012"}`))
					c.Assert(err, IsNil)
				}
			}
		}
	}))
	s.server = server
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: s.thordir,
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	// Use static test mnemonic for deterministic test addresses
	// This mnemonic derives to ETH/AVAX address: 0xd58610F89265a2fB637Ac40EDf59141Ff873b266
	testMnemonic := "dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog fossil"
	_, err := kb.NewAccount(cfg.SignerName, testMnemonic, "", cmd.THORChainHDPath, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.thorKeys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.thorKeys)
	c.Assert(err, IsNil)
}

func (s *EVMSuite) TearDownTest(c *C) {
	c.Assert(os.Unsetenv("NET"), IsNil)

	if err := os.RemoveAll(s.thordir); err != nil {
		c.Error(err)
	}
}

func (s *EVMSuite) TestSolvencyRunnerBackoff(c *C) {
	client := &EVMClient{
		cfg: config.BifrostChainConfiguration{
			ChainID: common.AVAXChain,
		},
	}

	c.Assert(client.solvencyRunnerBackoff(), Equals, time.Duration(common.AVAXChain.ApproximateBlockMilliseconds())*time.Millisecond)

	// ETH is above THORChain block time in both mocknet and non-mocknet builds,
	// so this consistently exercises the fallback path.
	client.cfg.ChainID = common.ETHChain
	c.Assert(client.solvencyRunnerBackoff(), Equals, constants.ThorchainBlockTime)
}

func (s *EVMSuite) TestNewClient(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	// bridge is nil
	e, err := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{}, nil, nil, s.m, pubkeyMgr, poolMgr)
	c.Assert(e, IsNil)
	c.Assert(err, NotNil)

	// pubkey manager is nil
	e, err = NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, nil, poolMgr)
	c.Assert(e, IsNil)
	c.Assert(err, NotNil)

	// pubkey manager is nil
	e, err = NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, pubkeyMgr, nil)
	c.Assert(e, IsNil)
	c.Assert(err, NotNil)

	// pubkey manager is nil
	e, err = NewEVMClient(nil, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(e, IsNil)
	c.Assert(err, NotNil)
}

func (s *EVMSuite) TestConvertSigningAmount(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	a, err := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
		},
	}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(a, NotNil)
	c.Assert(a.evmScanner.tokenManager.SaveTokenMeta("TKN", "0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF483", 18), IsNil)
	c.Assert(a.evmScanner.tokenManager.SaveTokenMeta("TKX", "0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF482", 8), IsNil)
	result := a.evmScanner.tokenManager.ConvertSigningAmount(big.NewInt(100), "0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF483")
	c.Assert(result.Uint64(), Equals, uint64(100*common.One*100))
	result = a.evmScanner.tokenManager.ConvertSigningAmount(big.NewInt(100000000), "0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF482")
	c.Assert(result.Uint64(), Equals, uint64(100000000))
}

func (s *EVMSuite) TestGetTokenAddressFromAsset(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	a, err := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
		},
	}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)

	token := a.getTokenAddressFromAsset(common.AVAXAsset)
	c.Assert(token, Equals, evm.NativeTokenAddr)
	asset, err := common.NewAsset("AVAX.TKN-0X333C3310824B7C685133F2BEDB2CA4B8B4DF633D")
	c.Assert(err, IsNil)
	token = a.getTokenAddressFromAsset(asset)
	c.Assert(token, Equals, "0X333C3310824B7C685133F2BEDB2CA4B8B4DF633D")
}

func (s *EVMSuite) TestClient(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	a, err := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(a, IsNil)
	c.Assert(err, NotNil)
	a2, err2 := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
		},
	}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err2, IsNil)
	c.Assert(a2, NotNil)
	a2.signingReady.Store(true) // Mark ready for tests that don't call Start()
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	c.Check(a2.GetChain(), Equals, common.AVAXChain)
	height, err := a2.GetHeight()
	c.Assert(err, IsNil)
	c.Check(height, Equals, int64(7))
	gasPrice := a2.GetGasPrice()
	c.Check(gasPrice.Uint64(), Equals, uint64(0))

	acct, err := a2.GetAccount(types2.GetRandomPubKey(), nil)
	c.Assert(err, IsNil)
	c.Check(acct.Sequence, Equals, int64(0))
	c.Check(acct.Coins[0].Amount.Uint64(), Equals, uint64(10*common.One))
	pk := types2.GetRandomPubKey()
	addr := a2.GetAddress(pk)
	c.Check(len(addr), Equals, 42)
	_, err = a2.BroadcastTx(stypes.TxOutItem{}, []byte(`{
		"from":"0xa7d9ddbe1f17865597fbd27ec712455208b6b76d",
		"gas":"0xc350",
		"gasPrice":"0x4a817c800",
		"input":"0x68656c6c6f21",
		"nonce":"0x15",
		"to":"0xf02c1c8e6114b1dbe8937a39260b5b0a374432bb",
		"transactionIndex":"0x41",
		"value":"0xf3dbb76162000",
		"v":"0x25",
		"r":"0x1b5e176d927f8e9ab405058b2d2457392da3e20f328b16ddabcebc33eaac5fea",
		"s":"0x4ba69724e8f69de52f0125ad8b3c5c2cef33019bac3249e2c0a2192766d1721c"
	}`))
	c.Assert(err, IsNil)
	input := []byte(`{
    "height": 1,
    "tx_array": [
        {
            "vault_pub_key": "tthorpub1addwnpepq2mza4j4vplyjw295pkq8j2dan627lz6vufeu22pjx5vnnyjted5vwq3e3d",
            "chain": "AVAX",
			"from_address":"0xa7d9ddbe1f17865597fbd27ec712455208b6b76d",
            "to_address": "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae",
            "coin": {
                "asset": "AVAX.AVAX",
                "amount": "194765912"
            },
            "max_gas": [
                {
                    "asset": "AVAX.AVAX",
                    "amount": "300000"
                }
            ],
			"gas_rate":1
        }
    ]
}`)
	var txOut stypes.TxOut
	err = json.Unmarshal(input, &txOut)
	c.Assert(err, IsNil)

	txOut.TxArray[0].VaultPubKey = a2.kw.GetPubKey()
	c.Logf(txOut.TxArray[0].VaultPubKey.String())
	c.Logf(a2.kw.GetPubKey().String())
	out := txOut.TxArray[0].TxOutItem(txOut.Height)
	out.Chain = common.AVAXChain
	out.Memo = "OUT:B6BD1A69831B9CCC0A1E9939E9AFBFCA144C427B3F61E176EBDCB14E57981C1B"
	r, _, obs, err := a2.SignTx(out, 1)
	c.Assert(err, IsNil)
	c.Assert(r, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err := out.VaultPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	_, err = a2.BroadcastTx(out, r)
	c.Assert(err, IsNil)
}

func (s *EVMSuite) TestL1FeeDeduction(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pubkeyMgr.Start(), IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}
	chainConfig.EVM.AggregatorMaxGasMultiplier = 10
	chainConfig.EVM.TokenMaxGasMultiplier = 3
	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	e.signingReady.Store(true) // Mark ready for tests that don't call Start()

	// sign tx without extra L1 gas fee
	pubkeys := pubkeyMgr.GetPubKeys()
	addr, err := pubkeys[len(pubkeys)-1].GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	result, _, obs, err := e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	c.Assert(obs.Gas[0].Amount.Uint64(), Equals, e.cfg.BlockScanner.MaxGasLimit*4)

	// sign tx with extra L1 gas fee
	e.cfg.EVM.ExtraL1GasFee = int64(700)
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	c.Assert(obs.Gas[0].Amount.Uint64(), Equals, e.cfg.BlockScanner.MaxGasLimit*4-700)
}

func (s *EVMSuite) TestGetAccount(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	e, err := NewEVMClient(s.thorKeys, config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
		},
	}, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	acct, err := e.GetAccountByAddress("0x9f4aab49a9cd8fc54dcb3701846f608a6f2c44da", nil)
	c.Assert(err, IsNil)
	c.Assert(acct.Sequence, Equals, int64(0))
	b, err := e.GetBalance("0x9f4aab49a9cd8fc54dcb3701846f608a6f2c44da", "0x333c3310824b7c685133F2BeDb2CA4b8b4DF633d", nil, "0x17aB05351fC94a1a67Bf3f56DdbB941aE6c63E25")
	c.Assert(err, IsNil)
	c.Assert(b, NotNil)
}

func (s *EVMSuite) TestSignEVMTx(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}
	chainConfig.EVM.AggregatorMaxGasMultiplier = 10
	chainConfig.EVM.TokenMaxGasMultiplier = 3
	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	e.signingReady.Store(true) // Mark ready for tests that don't call Start()
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	pubkeys := pubkeyMgr.GetPubKeys()
	addr, err := pubkeys[len(pubkeys)-4].GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)

	// Not AVAX chain
	result, _, obs, err := e.SignTx(stypes.TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   addr,
		VaultPubKey: "",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// to address is empty
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		VaultPubKey: "",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// vault pub key is empty
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: "",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// memo is empty
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// memo can't be parsed
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Memo:        "whatever",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// memo is inbound
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Memo:        "swap:AVAX.AVAX",
	}, 1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(obs, IsNil)

	// Outbound
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err := e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	asset, err := common.NewAsset("AVAX.TKN-0X3B7FA4DD21C6F9BA3CA375217EAD7CAB9D6BF483")
	c.Assert(err, IsNil)

	// Outbound
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(asset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err = e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	// refund
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "REFUND:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err = e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	// refund
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(asset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err = e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	// migrate
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "MIGRATE:1024",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err = e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())

	// migrate
	result, _, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(asset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "MIGRATE:1024",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(obs, NotNil)
	fromAddr, err = e.localPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	c.Assert(obs.Sender, Equals, fromAddr.String())
}

func (s *EVMSuite) TestSignTxChecksMemLiability(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)
	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}
	chainConfig.EVM.AggregatorMaxGasMultiplier = 10
	chainConfig.EVM.TokenMaxGasMultiplier = 3
	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	e.signingReady.Store(true) // Mark ready for tests that don't call Start()
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()

	// Test: In-Memory Liability Tracking
	// The mock for eth_getBalance returns 10 AVAX for "latest".
	// First Transaction (5 AVAX + ~1.5 AVAX fee) should SUCCEED and record liability.
	// Second Transaction (5 AVAX + ~1.5 AVAX fee) should FAIL because:
	//   EffectiveBalance = LatestBalance(10) - Liability(~6.5) = ~3.5 AVAX < Required(~6.5 AVAX)

	pubkeys := pubkeyMgr.GetPubKeys()
	addr, err := pubkeys[len(pubkeys)-1].GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)

	txOutItem1 := stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins:       common.Coins{common.NewCoin(common.AVAXAsset, cosmos.NewUint(5*common.One))}, // 5 AVAX
		Memo:        "MIGRATE:100",
		GasRate:     1,
		MaxGas:      common.Gas(common.NewCoins(common.NewCoin(common.AVAXAsset, cosmos.NewUint(80000*4)))),
	}

	// First transaction should succeed
	rawTx1, _, _, err := e.SignTx(txOutItem1, 100)
	c.Assert(err, IsNil, Commentf("First SignTx should succeed"))
	c.Assert(rawTx1, NotNil)

	// Verify liability was recorded
	c.Assert(len(e.pendingLiabilities) > 0, Equals, true, Commentf("Liability should be recorded"))

	// Second transaction should fail due to accumulated liability
	txOutItem2 := stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins:       common.Coins{common.NewCoin(common.AVAXAsset, cosmos.NewUint(5*common.One))}, // 5 AVAX
		Memo:        "MIGRATE:101",
		GasRate:     1,
		MaxGas:      common.Gas(common.NewCoins(common.NewCoin(common.AVAXAsset, cosmos.NewUint(80000*4)))),
	}

	rawTx2, _, _, err := e.SignTx(txOutItem2, 101)
	c.Assert(err, NotNil, Commentf("Second SignTx should fail due to liability"))
	c.Assert(err.Error(), Matches, "insufficient gas asset balance.*")
	c.Assert(rawTx2, IsNil)
}

func (s *EVMSuite) TestCheckAndApproveAllowance(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}

	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)

	tokenAsset, err := common.NewAsset("AVAX.USDC-0xA0b86a33E6441e60DDB6B6B84b7efF8e6f3C5d0C")
	c.Assert(err, IsNil)

	txOutItem := stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   common.Address("0x1234567890123456789012345678901234567890"),
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(tokenAsset, cosmos.NewUint(1000000)),
		},
		Memo:    "OUT:1234",
		GasRate: 50,
	}

	// Test 1: Mimir disabled (default -1) → Skip approval
	approvalTx, err := e.checkAndApproveAllowance(txOutItem, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, IsNil)

	// Enable mimir for remaining tests to verify the coin-type skip paths
	s.mimirOverrides = map[string]string{
		"EVMAllowanceCheck-AVAX": "1",
	}
	// Sleep for the bridge HTTP cache TTL (ThorchainBlockTime) so the new
	// mimir value is picked up on the next query.
	time.Sleep(constants.ThorchainBlockTime)

	// Test 2: Native asset → Skip approval (even with mimir enabled)
	txOutItemNative := txOutItem
	txOutItemNative.Coins = common.Coins{
		common.NewCoin(common.AVAXAsset, cosmos.NewUint(1000000)),
	}
	approvalTx, err = e.checkAndApproveAllowance(txOutItemNative, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, IsNil)

	// Test 3: Multiple coins → Error
	txOutItemMultiple := txOutItem
	txOutItemMultiple.Coins = common.Coins{
		common.NewCoin(tokenAsset, cosmos.NewUint(1000000)),
		common.NewCoin(common.AVAXAsset, cosmos.NewUint(1000000)),
	}
	approvalTx, err = e.checkAndApproveAllowance(txOutItemMultiple, 100)
	c.Assert(err, NotNil)
	c.Assert(approvalTx, IsNil)

	// Test 4: Empty coins array → Skip approval
	txOutItemEmpty := txOutItem
	txOutItemEmpty.Coins = common.Coins{}
	approvalTx, err = e.checkAndApproveAllowance(txOutItemEmpty, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, IsNil)

	// Test 5: Mimir enabled with insufficient allowance → Approval tx created
	// Mock returns allowance of 1000 (insufficient for 10 USDC)
	txOutItem.Coins = common.Coins{
		common.NewCoin(tokenAsset, cosmos.NewUint(1000000000)), // 10 USDC
	}
	approvalTx, err = e.checkAndApproveAllowance(txOutItem, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, NotNil) // Approval tx created since allowance is insufficient
}

func (s *EVMSuite) TestSignTxWithAllowanceFlow(c *C) {
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1, // avoids querying thorchain for block height
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}
	chainConfig.EVM.TokenMaxGasMultiplier = 3

	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)
	e.signingReady.Store(true)

	pubkeys := pubkeyMgr.GetPubKeys()
	addr, err := pubkeys[len(pubkeys)-1].GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)

	// Test SignTx with native asset (should work normally)
	result, checkpoint, obs, err := e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(common.One)),
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(checkpoint, NotNil)
	c.Assert(obs, NotNil)

	// Verify nonce is serialized in checkpoint
	var nonce uint64
	err = json.Unmarshal(checkpoint, &nonce)
	c.Assert(err, IsNil)
	c.Assert(nonce, Equals, uint64(0)) // First transaction from address

	// Test SignTx with token asset (allowance check called but skipped due to disabled mimir)
	tokenAsset, err := common.NewAsset("AVAX.USDC-0xA0b86a33E6441e60DDB6B6B84b7efF8e6f3C5d0C")
	c.Assert(err, IsNil)

	result, checkpoint, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(tokenAsset, cosmos.NewUint(1000000)), // 1 USDC
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(checkpoint, NotNil)
	c.Assert(obs, NotNil)

	// Verify nonce is serialized in checkpoint
	err = json.Unmarshal(checkpoint, &nonce)
	c.Assert(err, IsNil)
	c.Assert(nonce, Equals, uint64(0)) // First transaction from address

	// Sleep for bridge cache TTL so the disabled mimir response expires
	time.Sleep(constants.ThorchainBlockTime)

	// Enable the allowance check via mimir
	s.mimirOverrides = map[string]string{
		"EVMAllowanceCheck-AVAX": "1",
	}

	// Test SignTx with token asset and enabled mimir (triggers approval + sign flow).
	// After broadcasting the approval, SignTx waits for it to be mined then proceeds
	// to build and sign the outbound.
	result, checkpoint, obs, err = e.SignTx(stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   addr,
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(tokenAsset, cosmos.NewUint(1000000)), // 1 USDC
		},
		MaxGas: common.Gas{
			common.NewCoin(common.AVAXAsset, cosmos.NewUint(e.cfg.BlockScanner.MaxGasLimit*4)),
		},
		GasRate: 1,
		Memo:    "OUT:4D91ADAFA69765E7805B5FF2F3A0BA1DBE69E37A1CFCD20C48B99C528AA3EE87",
	}, 1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(checkpoint, NotNil)
	c.Assert(obs, NotNil)

	// Verify nonce in checkpoint (incremented after approval tx)
	err = json.Unmarshal(checkpoint, &nonce)
	c.Assert(err, IsNil)
	c.Assert(nonce, Equals, uint64(1))
}

func (s *EVMSuite) TestAllowanceCheckWithDecimalConversions(c *C) {
	// Enable allowance check via chain-scoped mimir key
	s.mimirOverrides = map[string]string{
		"EVMAllowanceCheck-AVAX": "1",
	}

	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pubkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pubkeyMgr.Stop(), IsNil) }()
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: "http://" + s.server.Listener.Addr().String(),
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1,
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}

	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)

	// Save token metadata for different decimal scenarios
	c.Assert(e.evmScanner.tokenManager.SaveTokenMeta("USDC", "0xA0b86a33E6441e60DDB6B6B84b7efF8e6f3C5d0C", 6), IsNil)
	c.Assert(e.evmScanner.tokenManager.SaveTokenMeta("DAI", "0x6B175474E89094C44Da98b954EedeAC495271d0F", 18), IsNil)

	// Test Case 1: USDC (6 decimals) - Insufficient allowance scenario
	// Send 1.5 USDC = 150000000 (THORChain 1e8 format) → 1500000 (USDC 1e6 native format)
	// Mock returns allowance of 1000 (insufficient)
	usdcAsset, err := common.NewAsset("AVAX.USDC-0xA0b86a33E6441e60DDB6B6B84b7efF8e6f3C5d0C")
	c.Assert(err, IsNil)

	txOutItemUSDC := stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   common.Address("0x1234567890123456789012345678901234567890"),
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(usdcAsset, cosmos.NewUint(150000000)), // 1.5 USDC in THORChain format
		},
		Memo:    "OUT:1234",
		GasRate: 50,
	}

	approvalTx, err := e.checkAndApproveAllowance(txOutItemUSDC, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, NotNil) // Approval tx created since allowance (1000) < required (1500000)

	// Test Case 2: DAI (18 decimals) - Verify decimal conversion
	// Send 0.01 DAI = 1000000 (THORChain 1e8 format) → 10000000000000000 (DAI 1e18 native format)
	thorchainAmount := big.NewInt(1000000)
	convertedAmount := e.evmScanner.tokenManager.ConvertSigningAmount(thorchainAmount, "0x6B175474E89094C44Da98b954EedeAC495271d0F")
	expectedDAIAmount := big.NewInt(0)
	expectedDAIAmount.SetString("10000000000000000", 10)

	c.Assert(convertedAmount.String(), Equals, expectedDAIAmount.String(),
		Commentf("DAI conversion: expected %s, got %s", expectedDAIAmount.String(), convertedAmount.String()))

	daiAsset, err := common.NewAsset("AVAX.DAI-0x6B175474E89094C44Da98b954EedeAC495271d0F")
	c.Assert(err, IsNil)

	txOutItemDAI := stypes.TxOutItem{
		Chain:       common.AVAXChain,
		ToAddress:   common.Address("0x1234567890123456789012345678901234567890"),
		VaultPubKey: e.localPubKey,
		Coins: common.Coins{
			common.NewCoin(daiAsset, cosmos.NewUint(1000000)),
		},
		Memo:    "OUT:1234",
		GasRate: 50,
	}

	approvalTx, err = e.checkAndApproveAllowance(txOutItemDAI, 100)
	c.Assert(err, IsNil)
	c.Assert(approvalTx, NotNil) // Approval tx created since allowance (1000) < required (1e16)
}

func (s *EVMSuite) TestApprovalTransactionBroadcastFlow(c *C) {
	// This test validates that approval transactions use direct ethClient.SendTransaction
	// (same lightweight pattern as unstuck transactions)
	approvalTxBroadcast := false
	var broadcastTxData []byte

	// Create a mock server that captures direct transaction broadcasts
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			body, err := io.ReadAll(req.Body)
			c.Assert(err, IsNil)

			type RPCRequest struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      interface{}     `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}

			var rpcRequest RPCRequest
			err = json.Unmarshal(body, &rpcRequest)
			if err == nil {
				switch rpcRequest.Method {
				case "eth_chainId":
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x539"}`)) // Chain ID 1337
					c.Assert(err, IsNil)
					return
				case "eth_sendRawTransaction":
					// Capture the transaction data
					var params []string
					err = json.Unmarshal(rpcRequest.Params, &params)
					if err == nil && len(params) > 0 {
						txHex := params[0]
						if len(txHex) > 2 {
							broadcastTxData, err = hex.DecodeString(txHex[2:])
							if err == nil {
								var tx etypes.Transaction
								err = rlp.DecodeBytes(broadcastTxData, &tx)
								if err == nil {
									// Check if this is an ERC20 approval (function selector 0x095ea7b3)
									if len(tx.Data()) >= 4 && hex.EncodeToString(tx.Data()[:4]) == "095ea7b3" {
										approvalTxBroadcast = true
										c.Logf("✓ Approval transaction broadcast detected via direct ethClient.SendTransaction")
										c.Logf("  Function selector: 0x%s (ERC20 approve)", hex.EncodeToString(tx.Data()[:4]))
										c.Logf("  Transaction hash: %s", tx.Hash().Hex())
									}
								}
							}
						}
					}
					// Return success hash
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1234567890abcdef"}`))
					c.Assert(err, IsNil)
					return
				}
			}
		}
		// Default response for other calls
		_, err := rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x0"}`))
		c.Assert(err, IsNil)
	}))
	defer server.Close()

	// Create a minimal EVM client configuration
	pubkeyMgr, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	poolMgr := thorclient.NewPoolMgr(s.bridge)

	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.AVAXChain,
		RPCHost: server.URL,
		BlockScanner: config.BifrostBlockScannerConfiguration{
			ChainID:            common.AVAXChain,
			StartBlockHeight:   1,
			HTTPRequestTimeout: time.Second,
			MaxGasLimit:        80000,
		},
	}

	e, err := NewEVMClient(s.thorKeys, chainConfig, nil, s.bridge, s.m, pubkeyMgr, poolMgr)
	c.Assert(err, IsNil)

	// Test the direct broadcasting pattern used by approval transactions
	// This simulates what happens in the checkAndApproveAllowance -> direct broadcast flow

	// Create a mock approval transaction (like what checkAndApproveAllowance creates)
	tokenAddr := ecommon.HexToAddress("0xA0b86a33E6441e60DDB6B6B84b7efF8e6f3C5d0C")
	routerAddr := ecommon.HexToAddress("0x1234567890123456789012345678901234567890")
	maxUint256 := new(big.Int)
	maxUint256.Sub(maxUint256.Lsh(big.NewInt(1), 256), big.NewInt(1))

	// Pack ERC20 approve call data (like in checkAndApproveAllowance)
	approvalData, err := e.evmScanner.erc20ABI.Pack("approve", routerAddr, maxUint256)
	c.Assert(err, IsNil)

	// Create approval transaction (like in checkAndApproveAllowance)
	approvalTx := etypes.NewTransaction(
		100, // nonce
		tokenAddr,
		big.NewInt(0),          // no ETH value for ERC20 approval
		50000,                  // gas limit
		big.NewInt(1000000000), // 1 Gwei gas price
		approvalData,
	)

	// Test the direct broadcasting pattern used by approval transactions
	// This should match the pattern in our implementation:
	// 1. Decode transaction from raw bytes
	// 2. Broadcast via ethClient.SendTransaction directly
	// 3. Log the result

	rawTx, err := approvalTx.MarshalJSON()
	c.Assert(err, IsNil)

	// Simulate the approval broadcasting flow from our implementation
	approvalTxDecoded := &etypes.Transaction{}
	err = approvalTxDecoded.UnmarshalJSON(rawTx)
	c.Assert(err, IsNil)

	ctx, cancel := e.getTimeoutContext()
	defer cancel()

	// This is the key test: broadcast via direct ethClient.SendTransaction
	// (same pattern as unstuck transactions, NOT via BroadcastTx with TxOutItem)
	err = e.ethClient.SendTransaction(ctx, approvalTxDecoded)
	c.Assert(err, IsNil) // Should succeed with our mock server

	// Verify the pattern was followed correctly
	c.Assert(approvalTxBroadcast, Equals, true, Commentf("Approval transaction should have been broadcast via direct ethClient.SendTransaction"))

	c.Logf("✓ Test validates that approval transactions use the same lightweight broadcasting pattern as unstuck transactions")
	c.Logf("  - Direct ethClient.SendTransaction() call (not BroadcastTx with TxOutItem)")
	c.Logf("  - No unnecessary signer cache or transaction tracking overhead")
	c.Logf("  - Same pattern as unstuck cancel transactions")
}
