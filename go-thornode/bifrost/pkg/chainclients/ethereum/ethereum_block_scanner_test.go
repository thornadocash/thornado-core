package ethereum

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/evm/types"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	thorcommon "gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	tokenlist "gitlab.com/thorchain/thornode/v3/common/tokenlist"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/x/thorchain"
	types2 "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

const Mainnet = 1

type BlockScannerTestSuite struct {
	m      *metrics.Metrics
	bridge thorclient.ThorchainBridge
	keys   *thorclient.Keys
}

var _ = Suite(&BlockScannerTestSuite{})

func (s *BlockScannerTestSuite) SetUpSuite(c *C) {
	thorchain.SetupConfigForTest()
	s.m = GetMetricForTest(c)
	c.Assert(s.m, NotNil)
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)

	// Use static test mnemonic for deterministic test addresses
	// This mnemonic derives to ETH address: 0xd58610F89265a2fB637Ac40EDf59141Ff873b266
	testMnemonic := "dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog dog fossil"
	_, err := kb.NewAccount(cfg.SignerName, testMnemonic, cfg.SignerPasswd, cmd.THORChainHDPath, hd.Secp256k1)
	c.Assert(err, IsNil)
	thorKeys := thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	c.Assert(err, IsNil)
	s.keys = thorKeys
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, thorKeys)
	c.Assert(err, IsNil)
}

func getConfigForTest() config.BifrostBlockScannerConfiguration {
	return config.BifrostBlockScannerConfiguration{
		StartBlockHeight:           1, // avoids querying thorchain for block height
		BlockScanProcessors:        1,
		HTTPRequestTimeout:         time.Second,
		HTTPRequestReadTimeout:     time.Second * 30,
		HTTPRequestWriteTimeout:    time.Second * 30,
		MaxHTTPRequestRetry:        3,
		BlockHeightDiscoverBackoff: time.Second,
		BlockRetryInterval:         time.Second,
		Concurrency:                1,
		GasCacheBlocks:             40,
		GasPriceResolution:         10,                  // 10 gwei
		ChainID:                    thorcommon.ETHChain, // Important for determining gas rate units.
	}
}

func CreateBlock(height int) (*etypes.Header, error) {
	strHeight := fmt.Sprintf("%x", height)
	blockJson := `{
               "parentHash":"0x8b535592eb3192017a527bbf8e3596da86b3abea51d6257898b2ced9d3a83826",
               "difficulty": "0x31962a3fc82b",
               "extraData": "0x4477617266506f6f6c",
               "gasLimit": "0x47c3d8",
               "gasUsed": "0x0",
               "hash": "0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
               "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
               "miner": "0x2a65aca4d5fc5b5c859090a6c34d164135398226",
               "nonce": "0xa5e8fb780cc2cd5e",
               "number": "0x` + strHeight + `",
               "receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
               "sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
               "size": "0x20e",
               "stateRoot": "0xdc6ed0a382e50edfedb6bd296892690eb97eb3fc88fd55088d5ea753c48253dc",
               "timestamp": "0x579f4981",
               "totalDifficulty": "0x25cff06a0d96f4bee",
               "transactionsRoot": "0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"
       }`
	var header *etypes.Header
	if err := json.Unmarshal([]byte(blockJson), &header); err != nil {
		return nil, err
	}
	return header, nil
}

func (s *BlockScannerTestSuite) TestNewBlockScanner(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
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
		if rpcRequest.Method == "eth_chainId" {
			_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x539"}`))
			c.Assert(err, IsNil)
		}
		if rpcRequest.Method == "eth_gasPrice" {
			_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
			c.Assert(err, IsNil)
		}
	}))
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	pubKeyManager, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	solvencyReporter := func(height int64) error {
		return nil
	}
	bs, err := NewETHScanner(getConfigForTest(), nil, big.NewInt(int64(Mainnet)), ethClient, s.bridge, s.m, pubKeyManager, solvencyReporter, nil)
	c.Assert(err, NotNil)
	c.Assert(bs, IsNil)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, s.bridge, nil, pubKeyManager, solvencyReporter, nil)
	c.Assert(err, NotNil)
	c.Assert(bs, IsNil)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), nil, s.bridge, s.m, pubKeyManager, solvencyReporter, nil)
	c.Assert(err, NotNil)
	c.Assert(bs, IsNil)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, s.bridge, s.m, nil, solvencyReporter, nil)
	c.Assert(err, NotNil)
	c.Assert(bs, IsNil)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, s.bridge, s.m, pubKeyManager, solvencyReporter, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
}

func (s *BlockScannerTestSuite) TestProcessBlock(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.RequestURI == thorclient.ChainVersionEndpoint:
			_, err := rw.Write([]byte(`{"current":"` + types2.GetCurrentVersion().String() + `"}`))
			c.Assert(err, IsNil)
		case req.RequestURI == thorclient.PubKeysEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
		case req.RequestURI == thorclient.InboundAddressesEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/inbound_addresses/inbound_addresses.json")
		case req.RequestURI == thorclient.AsgardVault:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case strings.HasPrefix(req.RequestURI, thorclient.NodeAccountEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		case strings.HasPrefix(req.RequestURI, thorclient.LastBlockEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/lastblock/eth.json")
		case strings.HasPrefix(req.RequestURI, thorclient.AuthAccountEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/auth/accounts/template.json")
		default:
			body, err := io.ReadAll(req.Body)
			c.Assert(err, IsNil)
			defer func() {
				c.Assert(req.Body.Close(), IsNil)
			}()
			type RPCRequest struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      interface{}     `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}
			var rpcRequest RPCRequest
			err = json.Unmarshal(body, &rpcRequest)
			if err != nil {
				return
			}
			if rpcRequest.Method == "eth_chainId" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_gasPrice" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x3b9aca00"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getTransactionReceipt" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0xe8c5","logsBloom":"0x00000000000000000002000000000000000000000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000000000000000000800000000000000000000000800000000000000000001000000000000800000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000004000000000000000000000000000000000400000000000000000000000000000020000020000000000000000000000000000000000000000000000000000000000000","logs":[{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395","0x000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b266","0x0000000000000000000000000000000000000000000000000000000000000000"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000384144443a4554482e4554483a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000","blockNumber":"0x22","transactionHash":"0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59","transactionIndex":"0x0","blockHash":"0x2383a22acdbe27d3c7c56a0452ae5e7edfbebeabe3a9a047c87716dafc8fa9d0","logIndex":"0x0","removed":false}],"transactionHash":"0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0xe8c5","effectiveGasPrice":"0x2540be400","blockHash":"0x2383a22acdbe27d3c7c56a0452ae5e7edfbebeabe3a9a047c87716dafc8fa9d0","blockNumber":"0x22","transactionIndex":"0x0"}}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_call" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x52554e45"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getBlockByNumber" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"difficulty":"0x2","extraData":"0xd88301091a846765746888676f312e31352e36856c696e757800000000000000e86d9af8b427b780cd1e6f7cabd2f9231ccac25d313ed475351ed64ac19f21491461ed1fae732d3bbf73a5866112aec23b0ca436185685b9baee4f477a950f9400","baseFeePerGas":"0x1","gasLimit":"0x9e0f54","gasUsed":"0xabd3","hash":"0xb273789207ce61a1ec0314fdb88efe6c6b554a9505a97ff3dff05aa691e220ac","logsBloom":"0x00010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000040000000000000000010000200020000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000000000000000040000020000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000010000000000000000000000000000000000000000000000020000000000000","miner":"0x0000000000000000000000000000000000000000","mixHash":"0x0000000000000000000000000000000000000000000000000000000000000000","nonce":"0x0000000000000000","number":"0x6b","parentHash":"0xf18470c54efec284fb5ad57c0ee4afe2774d61393bd5224ac5484b39a0a07556","receiptsRoot":"0x794a74d56ec50769a1400f7ae0887061b0ec3ea6702589a0b45b9102df2c9954","sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347","size":"0x30a","stateRoot":"0x1c84090d7f5dc8137d6762e3d4babe10b30bf61fa827618346ae1ba8600a9629","timestamp":"0x6008f03a","totalDifficulty":"0xd7","transactions":[{"blockHash":"0xb273789207ce61a1ec0314fdb88efe6c6b554a9505a97ff3dff05aa691e220ac","blockNumber":"0x6b","from":"0xfabb9cc6ec839b1214bb11c53377a56a6ed81762","gas":"0x23273","gasPrice":"0x1","hash":"0x501d0b7fc8fcdff367280dc8b0c077f6beb9e324ad9550e2c0e34a2fa8e99aed","input":"0x095ea7b3000000000000000000000000e65e9d372f8cacc7b6dfcd4af6507851ed31bb4400000000000000000000000000000000000000000000000000000000ee6b2800","nonce":"0x1","to":"0x40bcd4db8889a8bf0b1391d0c819dcd9627f9d0a","transactionIndex":"0x0","value":"0x0","v":"0xa95","r":"0x614fa842510a4293d25ce4799a01a3d3cfeada4b79d7157c755bb4872984fff","s":"0x351e831427ca7e2f1b5f45b5101cc1d170d6fd8e7129378c8d55a6a436f403dc"}],"transactionsRoot":"0x4247bb112edbe20ee8cf406864b335f4a3aa215f65ea686c9820f056c637aca6","uncles":[]}}`))
				c.Assert(err, IsNil)
			}
		}
	}))
	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	c.Assert(ethClient, NotNil)
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	u, err := url.Parse(server.URL)
	c.Assert(err, IsNil)
	bridge, err := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       u.Host,
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}, s.m, s.keys)
	c.Assert(err, IsNil)
	pubKeyMgr, err := pubkeymanager.NewPubKeyManager(bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pubKeyMgr.Start(), IsNil)
	defer func() {
		c.Assert(pubKeyMgr.Stop(), IsNil)
	}()

	bs, err := NewETHScanner(getConfigForTest(), storage, big.NewInt(1337), ethClient, bridge, s.m, pubKeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)
	whitelistSmartContractAddress = append(whitelistSmartContractAddress, "0x40bcd4dB8889a8Bf0b1391d0c819dcd9627f9d0a")
	txIn, err := bs.FetchTxs(int64(1), int64(1))
	c.Assert(err, IsNil)
	c.Check(len(txIn.TxArray), Equals, 1)
}

func httpTestHandler(c *C, rw http.ResponseWriter, fixture string) {
	var content []byte
	var err error

	switch fixture {
	case "500":
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		content, err = os.ReadFile(fixture)
		if err != nil {
			c.Fatal(err)
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	if _, err = rw.Write(content); err != nil {
		c.Fatal(err)
	}
}

func (s *BlockScannerTestSuite) TestFromTxToTxIn(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.RequestURI == thorclient.ChainVersionEndpoint:
			_, err := rw.Write([]byte(`{"current":"` + types2.GetCurrentVersion().String() + `"}`))
			c.Assert(err, IsNil)
		case req.RequestURI == "/thorchain/mimir/key/EVMDisableContractWhitelist":
			_, err := rw.Write([]byte(`1`))
			c.Assert(err, IsNil)
		case req.RequestURI == thorclient.PubKeysEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
		case req.RequestURI == thorclient.InboundAddressesEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/inbound_addresses/inbound_addresses.json")
		case req.RequestURI == thorclient.AsgardVault:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case strings.HasPrefix(req.RequestURI, thorclient.NodeAccountEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		default:
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
			if err != nil {
				return
			}
			switch rpcRequest.Method {
			case "eth_chainId":
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			case "eth_gasPrice":
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			case "eth_call":
				switch string(rpcRequest.Params) {
				case `[{"from":"0x0000000000000000000000000000000000000000","input":"0x95d89b41","to":"0x3b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483"},"latest"]`,
					`[{"data":"0x95d89b41","from":"0x0000000000000000000000000000000000000000","to":"0x40bcd4db8889a8bf0b1391d0c819dcd9627f9d0a"},"latest"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":3,"result":"0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003544b4e0000000000000000000000000000000000000000000000000000000000"}`))
					c.Assert(err, IsNil)
					return
				case `[{"from":"0x0000000000000000000000000000000000000000","input":"0x313ce567","to":"0x3b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483"},"latest"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":4,"result":"0x0000000000000000000000000000000000000000000000000000000000000012"}`))
					c.Assert(err, IsNil)
					return
				case `[{"from":"0x0000000000000000000000000000000000000000","input":"0x95d89b41","to":"0x2260fac5e5542a773aa44fbcfedf7c193bc2c599"},"latest"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":5,"result":"0x00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000003544b4e0000000000000000000000000000000000000000000000000000000000"}`))
					c.Assert(err, IsNil)
					return
				case `[{"from":"0x0000000000000000000000000000000000000000","input":"0x313ce567","to":"0x2260fac5e5542a773aa44fbcfedf7c193bc2c599"},"latest"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":6,"result":"0x0000000000000000000000000000000000000000000000000000000000000008"}`))
					c.Assert(err, IsNil)
					return
				default:
					fmt.Printf("======> rpcRequest.Params: %s\n", string(rpcRequest.Params))
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x52554e45"}`))
					c.Assert(err, IsNil)
				}
			case "eth_getTransactionReceipt":
				switch string(rpcRequest.Params) {
				case `["0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0xe8c5","logsBloom":"0x00000000000000000002000000000000000000000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000020000000000000000000800000000000000000000000800000000000000000001000000000000800000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000000000004000000000000000000000000000000000400000000000000000000000000000020000020000000000000000000000000000000000000000000000000000000000000","logs":[{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395","0x000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b266","0x0000000000000000000000000000000000000000000000000000000000000000"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000384144443a4554482e4554483a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000","blockNumber":"0x22","transactionHash":"0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59","transactionIndex":"0x0","blockHash":"0x2383a22acdbe27d3c7c56a0452ae5e7edfbebeabe3a9a047c87716dafc8fa9d0","logIndex":"0x0","removed":false}],"transactionHash":"0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0xe8c5","effectiveGasPrice":"0x2540be400","blockHash":"0x2383a22acdbe27d3c7c56a0452ae5e7edfbebeabe3a9a047c87716dafc8fa9d0","blockNumber":"0x22","transactionIndex":"0x0"}}`))
					c.Assert(err, IsNil)
					return
				case `["0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0x13d20","logsBloom":"0x00000000000000000002000020000000000000000000000000000000000000000000000000004000000000000000000000000800000000000000000000000000000000000000000000000008000000000000000000000000000000000000000010000200000000000000000000000000000000000000000000000810000000000000000001010000000000800000000000000000000000000000000000040000000000000000002000000000000000000000000000003400000000000004000000000002000000000000000000000400000000000000000000000000000000000020000000000000000000002000000000000000010000800000000000000000","logs":[{"address":"0x3b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000c96aaa54e2d44c299564da76e1cd3184a2386b8d","0x000000000000000000000000e65e9d372f8cacc7b6dfcd4af6507851ed31bb44"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000","blockNumber":"0x20","transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","transactionIndex":"0x0","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","logIndex":"0x0","removed":false},{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395","0x000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b266","0x0000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000634144443a4554482e544b4e2d3078336237464134646432316336663942413363613337353231374541443743416239443662463438333a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000000000000000000000000000000000000000000000","blockNumber":"0x20","transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","transactionIndex":"0x0","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","logIndex":"0x1","removed":false}],"transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0x13d20","effectiveGasPrice":"0x2540be400","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","blockNumber":"0x20","transactionIndex":"0x0"}}`))
					c.Assert(err, IsNil)
					return
				case `["0x4b8845b0d99c13bae6716b3c422cdb61aa141c0db04cfb18bcc031b76471595b"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0xecc1","logsBloom":"0x00000000000000000002010000000000000000000000000000000000000000000000000000000000000000000000000400000100000000000000002000000000000000000000000000000000000000000000000000040000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001000000100000004000000000000000000000000000000000000000000000000000000000000000000080000000000020000000000000000000000000000000000000000000000000000","logs":[{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0xa9cd03aa3c1b4515114539cd53d22085129d495cb9e9f9af77864526240f1bf7","0x0000000000000000000000005dcd69c5a0e2a6ccf7416c1c259063b88668a5ca","0x0000000000000000000000008d8bba78a27881294b34c82fb5978596e2df66dd"],"data":"0x0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000031f2ffcfc1f7c00000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000444f55543a4332323337423935393946434332443337323434383644414641363042413139343036353030393244333135353144383538343536314236303042434246343300000000000000000000000000000000000000000000000000000000","blockNumber":"0x60","transactionHash":"0x4b8845b0d99c13bae6716b3c422cdb61aa141c0db04cfb18bcc031b76471595b","transactionIndex":"0x0","blockHash":"0x8a60816fdc9649f754994aae1cb3ca952d14274d38fea797decc47c0c7a29188","logIndex":"0x0","removed":false}],"transactionHash":"0x4b8845b0d99c13bae6716b3c422cdb61aa141c0db04cfb18bcc031b76471595b","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0xecc1","effectiveGasPrice":"0x2540be400","blockHash":"0x8a60816fdc9649f754994aae1cb3ca952d14274d38fea797decc47c0c7a29188","blockNumber":"0x60","transactionIndex":"0x0"}}`))
					c.Assert(err, IsNil)
					return
				case `["0xe8d7b5ff2e2f3ae814dfd422444196a72349e03a761eda5452fcc244291fc599"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0x9a91","logsBloom":"0x00000000000000000002000020000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004000000000000000008000000000000000000000000000000000000000002000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000000000000000000800000000000000000000000000000000000000000000000000000000000000001000000000000004000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000010000800000000000000000","logs":[{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0x05b90458f953d3fcb2d7fb25616a2fddeca749d0c47cc5c9832d0266b5346eea","0x0000000000000000000000003fd2d4ce97b082d4bce3f9fee2a3d60668d2f473","0x0000000000000000000000009f4aab49a9cd8fc54dcb3701846f608a6f2c44da"],"data":"0x0000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf48300000000000000000000000000000000000000000000000011572680468e44000000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000000c4d4947524154453a31303234","blockNumber":"0x1a6","transactionHash":"0xe8d7b5ff2e2f3ae814dfd422444196a72349e03a761eda5452fcc244291fc599","transactionIndex":"0x0","blockHash":"0x39b72c414a032e8172f871c94e2382065c3e848ae69bb68f60114cb5b8fa7868","logIndex":"0x0","removed":false}],"transactionHash":"0xe8d7b5ff2e2f3ae814dfd422444196a72349e03a761eda5452fcc244291fc599","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0x9a91","effectiveGasPrice":"0x2540be400","blockHash":"0x39b72c414a032e8172f871c94e2382065c3e848ae69bb68f60114cb5b8fa7868","blockNumber":"0x1a6","transactionIndex":"0x0"}}`))
					c.Assert(err, IsNil)
					return
				case `["0xbd6e8494e18d627d2331c625203bcea6fb5acb9cdc8a82628f1bc894e90884e2"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"root":"0x","status":"0x1","cumulativeGasUsed":"0x13d20","logsBloom":"0x00000000000000000002000020000000000000000000000000000000000000000000000000004000000000000000000000000800000000000000000000000000000000000000000000000008000000000000000000000000000000000000000010000200000000000000000000000000000000000000000000000810000000000000000001010000000000800000000000000000000000000000000000040000000000000000002000000000000000000000000000003400000000000004000000000002000000000000000000000400000000000000000000000000000000000020000000000000000000002000000000000000010000800000000000000000","logs":[{"address":"0x3b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000c96aaa54e2d44c299564da76e1cd3184a2386b8d","0x000000000000000000000000e65e9d372f8cacc7b6dfcd4af6507851ed31bb44"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000","blockNumber":"0x20","transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","transactionIndex":"0x0","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","logIndex":"0x0","removed":false},{"address":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","topics":["0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395","0x000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b266","0x0000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483"],"data":"0x0000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000634144443a4554482e544b4e2d3078336237464134646432316336663942413363613337353231374541443743416239443662463438333a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000000000000000000000000000000000000000000000","blockNumber":"0x20","transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","transactionIndex":"0x0","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","logIndex":"0x1","removed":false}],"transactionHash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c","contractAddress":"0x0000000000000000000000000000000000000000","gasUsed":"0x13d20","effectiveGasPrice":"0x2540be400","blockHash":"0xe2ac172ea4c9b390adff7b21a4fe134251e60ba1d31a1acc0fb0d3bad350e34f","blockNumber":"0x20","transactionIndex":"0x0"}}`))
					c.Assert(err, IsNil)
					return
				case `["0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe"]`:
					_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":"0","result":{"blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","blockNumber":"0x157b930","contractAddress":null,"cumulativeGasUsed":"0xb2acb7","effectiveGasPrice":"0x8d31ce9c","from":"0x52d3c44dbb43dde4ed5c9bec0c8b226269057e1c","gasUsed":"0x279df","logs":[{"address":"0x2260fac5e5542a773aa44fbcfedf7c193bc2c599","topics":["0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925","0x00000000000000000000000052d3c44dbb43dde4ed5c9bec0c8b226269057e1c","0x0000000000000000000000001231deb6f5749ef6ce6943a275a1d3e7486f4eae"],"data":"0x00000000000000000000000000000000000000000000000000000000000fec6d","blockNumber":"0x157b930","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","logIndex":"0x137","removed":false},{"address":"0x2260fac5e5542a773aa44fbcfedf7c193bc2c599","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x00000000000000000000000052d3c44dbb43dde4ed5c9bec0c8b226269057e1c","0x0000000000000000000000001231deb6f5749ef6ce6943a275a1d3e7486f4eae"],"data":"0x00000000000000000000000000000000000000000000000000000000000fec6d","blockNumber":"0x157b930","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","logIndex":"0x138","removed":false},{"address":"0x2260fac5e5542a773aa44fbcfedf7c193bc2c599","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x0000000000000000000000001231deb6f5749ef6ce6943a275a1d3e7486f4eae","0x000000000000000000000000d37bbe5744d730a1d98d8dc97c42f0ca46ad7146"],"data":"0x00000000000000000000000000000000000000000000000000000000000fec6d","blockNumber":"0x157b930","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","logIndex":"0x139","removed":false},{"address":"0xd37bbe5744d730a1d98d8dc97c42f0ca46ad7146","topics":["0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395","0x000000000000000000000000c9146363d39cd45b11f6df465cee68d014c962a0","0x0000000000000000000000002260fac5e5542a773aa44fbcfedf7c193bc2c599"],"data":"0x00000000000000000000000000000000000000000000000000000000000fec6d000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000433d3a623a6263317138306e7a33687834773664707a77776d75646479796430793375776a30736a7872746d6c67753a313033333635353a6c6966692f2d5f3a302f32300000000000000000000000000000000000000000000000000000000000","blockNumber":"0x157b930","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","logIndex":"0x13a","removed":false},{"address":"0x1231deb6f5749ef6ce6943a275a1d3e7486f4eae","topics":["0xcba69f43792f9f399347222505213b55af8e0b0b54b893085c2e27ecbe1644f1"],"data":"0x000000000000000000000000000000000000000000000000000000000000002072e6ff6ab6ed490d2dca91c591190efbce30c835d8ca66677c0afc1a64e3c13d0000000000000000000000000000000000000000000000000000000000000140000000000000000000000000000000000000000000000000000000000000018000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002260fac5e5542a773aa44fbcfedf7c193bc2c59900000000000000000000000011f111f111f111f111f111f111f111f111f111f100000000000000000000000000000000000000000000000000000000000fec6d000000000000000000000000000000000000000000000000000012309ce5400100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000874686f7273776170000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000f6a756d7065722e65786368616e67650000000000000000000000000000000000","blockNumber":"0x157b930","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","logIndex":"0x13b","removed":false}],"logsBloom":"0x00000000000000010000000820800800000000000000000000000000000000008000000000000000000000000000000000000800000000000000000000200000000000000000000000000008000000000004000000000000000000000000000004000000000000000000000000000000021000020000000000000810008000000000000000000000000000000000000000000000000000000000000000000008030000020000800000000000080000000000000000000000000000002002008000001002000000004000000000000480000000000004000000000000000000000050000000000000000000800000000000200000000000000000000000000000","status":"0x1","to":"0x52d3c44dbb43dde4ed5c9bec0c8b226269057e1c","transactionHash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","transactionIndex":"0x4b","type":"0x4"}}`))
					c.Assert(err, IsNil)
					return
				}
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{
				"transactionHash":"0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b",
				"transactionIndex":"0x0",
				"blockNumber":"0x1",
				"blockHash":"0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
				"cumulativeGasUsed":"0xc350",
				"gasUsed":"0x4dc",
				"effectiveGasPrice":"0x2540be400",
				"logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"logs":[],
				"status":"0x1"
			}}`))
				c.Assert(err, IsNil)
			}
		}
	}))
	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	c.Assert(ethClient, NotNil)
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	u, err := url.Parse(server.URL)
	c.Assert(err, IsNil)

	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       u.Host,
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}
	bridge, err := thorclient.NewThorchainBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
	c.Assert(bridge, NotNil)
	pkeyMgr, err := pubkeymanager.NewPubKeyManager(bridge, s.m)
	c.Assert(pkeyMgr.Start(), IsNil)
	defer func() {
		c.Assert(pkeyMgr.Stop(), IsNil)
	}()
	c.Assert(err, IsNil)
	bs, err := NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)

	// send directly to ETH address (outbound from vault)
	// This transaction is signed with the test mnemonic's private key
	// Sender: 0xd58610F89265a2fB637Ac40EDf59141Ff873b266 (derived from test mnemonic)
	encodedTx := `{
		"blockHash":"0x1d59ff54b1eb26b013ce3cb5fc9dab3705b415a67127a003c3e61eb445bb8df2",
		"blockNumber":"0x5daf3b",
		"from":"0xd58610F89265a2fB637Ac40EDf59141Ff873b266",
		"gas":"0xc350",
		"gasPrice":"0x4a817c800",
		"hash":"0x60d7d8f81f885db074b19b57b77959b951e9ecc078e0117b08a3beaae2ed133d",
		"input":"0x68656c6c6f21",
		"nonce":"0x15",
		"to":"0xf02c1c8e6114b1dbe8937a39260b5b0a374432bb",
		"transactionIndex":"0x41",
		"value":"0xf3dbb76162000",
		"v":"0x26",
		"r":"0x5a5c10a37eb9f673e085cadff4cd3cebc15402c5861f787b5cfac2a5722f146e",
		"s":"0x162b4ba7d55a7177bf30816e27e32d5942861dec57a968451eec800f51e832ce"
	}`
	tx := etypes.NewTransaction(0, common.HexToAddress(ethToken), nil, 0, nil, nil)
	err = tx.UnmarshalJSON([]byte(encodedTx))
	c.Assert(err, IsNil)

	txInItem, err := bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Check(txInItem.Sender, Equals, "0xd58610f89265a2fb637ac40edf59141ff873b266")
	c.Check(txInItem.To, Equals, "0xf02c1c8e6114b1dbe8937a39260b5b0a374432bb")
	c.Check(len(txInItem.Coins), Equals, 1)

	c.Check(txInItem.Coins[0].Asset.String(), Equals, "ETH.ETH")
	c.Check(
		txInItem.Coins[0].Amount.Equal(cosmos.NewUint(429000)),
		Equals,
		true,
	)
	c.Check(
		txInItem.Gas[0].Amount.Equal(cosmos.NewUint(1244)), // from receipt GasUsed and EffectiveGasPrice rather than tx gas limit and max gas price
		Equals,
		true,
	)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(1337), ethClient, bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)
	bs.whitelistTokens = append(bs.whitelistTokens, tokenlist.ERC20Token{
		Address: "0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF483",
		Symbol:  "TKN",
	})

	// smart contract - deposit
	encodedTx = `{"nonce":"0x4","gasPrice":"0x1","gas":"0x177b8","to":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","value":"0x0","input":"0x1fece7b4000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b2660000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf4830000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000634144443a4554482e544b4e2d3078336237464134646432316336663942413363613337353231374541443743416239443662463438333a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000000000000000000000000000000000000000000000","v":"0xa96","r":"0x6f45018cb52c8c75d71894140a6c65d8c13bdd745e6395ede8a6123ea291bc04","s":"0x56e23a0971d52079f1713cc3b8a4193c98105d2e9f9f67d050f42557a390b16b","hash":"0xe18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c"}`
	tx = etypes.NewTransaction(0, common.HexToAddress(ethToken), nil, 0, nil, nil)
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0xc96aaa54e2d44c299564da76e1cd3184a2386b8d")
	c.Assert(txInItem.To, Equals, "0xd58610F89265a2fB637Ac40EDf59141Ff873b266")
	c.Assert(txInItem.Memo, Equals, "ADD:ETH.TKN-0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF483:tthor16xxn0cadruuw6a2qwpv35av0mehryvdzzjz3af")
	c.Assert(txInItem.Tx, Equals, "e18c76466b7ad0c08d6bd6d7e4d50d2d4b86cdca4092cc7b28fbb3b6a5c2565c")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.TKN-0X3B7FA4DD21C6F9BA3CA375217EAD7CAB9D6BF483")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(500000000)), Equals, true)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(1337), ethClient, bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	// whitelist the address for test
	whitelistSmartContractAddress = append(whitelistSmartContractAddress,
		"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44",
		"0x81a392e6a757d58a7eb6781a775a3449da3b9df5")
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)
	// smart contract - deposit via smart contract (transaction to != router)
	encodedTx = `{"nonce":"0x4","gasPrice":"0x1","gas":"0x177b8","to":"0x81a392e6a757d58a7eb6781a775a3449da3b9df5","value":"0x0","input":"0x1fece7b4000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b2660000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf4830000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000634144443a4554482e544b4e2d3078336237464134646432316336663942413363613337353231374541443743416239443662463438333a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000000000000000000000000000000000000000000000","v":"0xa96","r":"0x7fbfee9939a7e6de1e9dec44f4ac8cd9496de934dff9abe74c9ba2ecc69ac66e","s":"0x606c5d6510558513b6425099892fc2269663abbd11266b7087c6ecb899827b11","hash":"0xbd6e8494e18d627d2331c625203bcea6fb5acb9cdc8a82628f1bc894e90884e2"}`
	tx = etypes.NewTransaction(0, common.HexToAddress(ethToken), nil, 0, nil, nil)
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0xc96aaa54e2d44c299564da76e1cd3184a2386b8d")
	c.Assert(txInItem.To, Equals, "0xd58610F89265a2fB637Ac40EDf59141Ff873b266")
	c.Assert(txInItem.Memo, Equals, "ADD:ETH.TKN-0x3b7FA4dd21c6f9BA3ca375217EAD7CAb9D6bF483:tthor16xxn0cadruuw6a2qwpv35av0mehryvdzzjz3af")
	c.Assert(txInItem.Tx, Equals, "bd6e8494e18d627d2331c625203bcea6fb5acb9cdc8a82628f1bc894e90884e2")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.TKN-0X3B7FA4DD21C6F9BA3CA375217EAD7CAB9D6BF483")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(500000000)), Equals, true)

	// smart contract - depositETH
	encodedTx = `{"nonce":"0x5","gasPrice":"0x1","gas":"0xe8c5","to":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","value":"0x4563918244f40000","input":"0x1fece7b4000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b26600000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000384144443a4554482e4554483a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000","v":"0xa95","r":"0x9a182e8c6dbc09b6d57248ce02e0ecd84243ff1dce86fc5ed487087dcf72cc4a","s":"0x308038d1221253bcdf60829f8aa112baa152d5c3fcc72f88e662df9cd6100fc","hash":"0xf4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59"}`
	tx = &etypes.Transaction{}
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0xc96aaa54e2d44c299564da76e1cd3184a2386b8d")
	c.Assert(txInItem.To, Equals, "0xd58610F89265a2fB637Ac40EDf59141Ff873b266")
	c.Assert(txInItem.Memo, Equals, "ADD:ETH.ETH:tthor16xxn0cadruuw6a2qwpv35av0mehryvdzzjz3af")
	c.Assert(txInItem.Tx, Equals, "f4b44594788df2b4e04ea3f23594a85cb64388220274a828aab96b083647fc59")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.ETH")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(500000000)), Equals, true)

	// smart contract - transferOut
	encodedTx = `{"nonce":"0x0","gasPrice":"0x2540be400","gas":"0xecc1","to":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","value":"0x31f2ffcfc1f7c00","input":"0x574da7170000000000000000000000008d8bba78a27881294b34c82fb5978596e2df66dd0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000031d13d4898b6000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000444f55543a4332323337423935393946434332443337323434383644414641363042413139343036353030393244333135353144383538343536314236303042434246343300000000000000000000000000000000000000000000000000000000","v":"0xa96","r":"0xb27f9fff5cc936d5918aa557c9c4df559e3e4f6c4ac5b0b79d43c4e3bdcb91e","s":"0x1417cedea6a9b879bd24d547b29c05d214100bfc586a32a1c24de3a090528f62","hash":"0x4b8845b0d99c13bae6716b3c422cdb61aa141c0db04cfb18bcc031b76471595b"}`
	tx = &etypes.Transaction{}
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0x5dcd69c5a0e2a6ccf7416c1c259063b88668a5ca")
	c.Assert(txInItem.To, Equals, "0x8d8Bba78A27881294b34c82Fb5978596e2DF66dD")
	c.Assert(txInItem.Memo, Equals, "OUT:C2237B9599FCC2D3724486DAFA60BA1940650092D31551D8584561B600BCBF43")
	c.Assert(txInItem.Tx, Equals, "4b8845b0d99c13bae6716b3c422cdb61aa141c0db04cfb18bcc031b76471595b")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.ETH")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(22495127)), Equals, true)

	// smart contract - allowance
	encodedTx = `{"nonce":"0xb","gasPrice":"0x1","gas":"0xd529","to":"0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44","value":"0x0","input":"0x1b738b32000000000000000000000000e65e9d372f8cacc7b6dfcd4af6507851ed31bb440000000000000000000000009f4aab49a9cd8fc54dcb3701846f608a6f2c44da0000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf483000000000000000000000000000000000000000000000000ad67810426efff1800000000000000000000000000000000000000000000000000000000000000a0000000000000000000000000000000000000000000000000000000000000000568656c6c6f000000000000000000000000000000000000000000000000000000","v":"0xa96","r":"0x967771b4ec53f895b6f6a2e8b4febbfd04fba079b5f1ab3c6476d9d612cc23d5","s":"0x2cc999ea73cd67cac387a0c5fa49cf6eeab8de1b4602ad376f788a3b700b97fa","hash":"0xe8d7b5ff2e2f3ae814dfd422444196a72349e03a761eda5452fcc244291fc599"}`
	tx = &etypes.Transaction{}
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0x3fd2d4ce97b082d4bce3f9fee2a3d60668d2f473")
	c.Assert(txInItem.To, Equals, "0x9F4AaB49A9cd8FC54Dcb3701846f608a6f2C44dA")
	c.Assert(txInItem.Memo, Equals, "MIGRATE:1024")
	c.Assert(txInItem.Tx, Equals, "e8d7b5ff2e2f3ae814dfd422444196a72349e03a761eda5452fcc244291fc599")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.TKN-0X3B7FA4DD21C6F9BA3CA375217EAD7CAB9D6BF483")
	c.Logf("======> %+v \n", txInItem)
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(124950975)), Equals, true)

	bs, err = NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)

	// smart account - deposit - eip-7702
	encodedTx = `{"blockHash":"0x669991f7f1e6f99b928c136ad69e63b206004cba11bf2aa6aa0ec11e05bdbd32","blockNumber":"0x157b930","from":"0x52d3c44dbb43dde4ed5c9bec0c8b226269057e1c","gas":"0x3d657","gasPrice":"0x8d31ce9c","maxFeePerGas":"0xc48631b4","maxPriorityFeePerGas":"0x295b31b6","hash":"0xba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe","input":"0xe9ae5c530100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000004e000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000001200000000000000000000000002260fac5e5542a773aa44fbcfedf7c193bc2c599000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044095ea7b30000000000000000000000001231deb6f5749ef6ce6943a275a1d3e7486f4eae00000000000000000000000000000000000000000000000000000000000fec6d000000000000000000000000000000000000000000000000000000000000000000000000000000001231deb6f5749ef6ce6943a275a1d3e7486f4eae0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000002e42541ec570000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000020072e6ff6ab6ed490d2dca91c591190efbce30c835d8ca66677c0afc1a64e3c13d0000000000000000000000000000000000000000000000000000000000000140000000000000000000000000000000000000000000000000000000000000018000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002260fac5e5542a773aa44fbcfedf7c193bc2c59900000000000000000000000011f111f111f111f111f111f111f111f111f111f100000000000000000000000000000000000000000000000000000000000fec6d000000000000000000000000000000000000000000000000000012309ce5400100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000874686f7273776170000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000f6a756d7065722e65786368616e67650000000000000000000000000000000000000000000000000000000000c9146363d39cd45b11f6df465cee68d014c962a0000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000682ce7bf00000000000000000000000000000000000000000000000000000000000000433d3a623a6263317138306e7a33687834773664707a77776d75646479796430793375776a30736a7872746d6c67753a313033333635353a6c6966692f2d5f3a302f3230000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000","nonce":"0x3","to":"0x52d3c44dbb43dde4ed5c9bec0c8b226269057e1c","transactionIndex":"0x4b","value":"0x0","type":"0x4","accessList":[],"chainId":"0x1","authorizationList":[{"chainId":"0x1","address":"0x63c0c19a282a1b52b07dd5a65b58948a07dae32b","nonce":"0x4","yParity":"0x0","r":"0xc264d3bfb5a1a70d763d9819a588f664c2d358d6cb50e3b5e86aabd88acd0b77","s":"0x1645b9900516de5796ac55a12ac16817eddd7e71de46d5c2e4f4e76bbcf93a07"}],"v":"0x1","r":"0xb593202f4b991be181fce8b1ea03e4f48b8dd83dd42b4e28faea17c47dc24e4a","s":"0x4f58dee1189e1fe1e6ae2377f34ae43b45fc0dc8bece2d0eb584cd59090abce4","yParity":"0x1"}`
	tx = &etypes.Transaction{}
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)

	// add thorchain router
	useWhitelistSmartContract = true
	whitelistSmartContractAddress = append(whitelistSmartContractAddress,
		"0xd37bbe5744d730a1d98d8dc97c42f0ca46ad7146",
	)

	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil)
	c.Assert(txInItem.Sender, Equals, "0x52d3c44dbb43dde4ed5c9bec0c8b226269057e1c")
	c.Assert(txInItem.To, Equals, "0xc9146363D39cd45B11f6df465Cee68D014C962A0")
	c.Assert(txInItem.Memo, Equals, "=:b:bc1q80nz3hx4w6dpzwwmuddyyd0y3uwj0sjxrtmlgu:1033655:lifi/-_:0/20")
	c.Assert(txInItem.Tx, Equals, "ba43fa28957a7aaf961bc0bbf5b321a3218e99e408d2b5a491274e2125988bbe")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.TKN-0X2260FAC5E5542A773AA44FBCFEDF7C193BC2C599")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(1043565)), Equals, true)
	c.Assert(txInItem.Gas[0].Asset.String(), Equals, "ETH.ETH")
	c.Assert(txInItem.Gas[0].Amount.Equal(cosmos.NewUint(38440)), Equals, true)
}

func (s *BlockScannerTestSuite) TestRouterValidation(c *C) {
	// This test validates the full swap-in flow through fromTxToTxIn for aggregator
	// deposits where tx.To() is an aggregator contract (not the router) but the
	// Deposit event is emitted by the correct router.

	correctRouter := "0xe65e9d372f8cacc7b6dfcd4af6507851ed31bb44"
	wrongRouter := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// Vault derived from tthorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzyulk9wh4n946jv5w2vpwdn2yuyp6sp4
	vaultAddr := "0xd58610F89265a2fB637Ac40EDf59141Ff873b266"
	vaultAddrLower := strings.ToLower(vaultAddr[2:])

	// Pre-signed aggregator tx: tx.To()=0x81a392... (aggregator), not a vault or router.
	// sender=0xc96aaa54..., chainID=1337
	encodedTx := `{"nonce":"0x4","gasPrice":"0x1","gas":"0x177b8","to":"0x81a392e6a757d58a7eb6781a775a3449da3b9df5","value":"0x0","input":"0x1fece7b4000000000000000000000000d58610f89265a2fb637ac40edf59141ff873b2660000000000000000000000003b7fa4dd21c6f9ba3ca375217ead7cab9d6bf4830000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000000634144443a4554482e544b4e2d3078336237464134646432316336663942413363613337353231374541443743416239443662463438333a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000000000000000000000000000000000000000000000","v":"0xa96","r":"0x7fbfee9939a7e6de1e9dec44f4ac8cd9496de934dff9abe74c9ba2ecc69ac66e","s":"0x606c5d6510558513b6425099892fc2269663abbd11266b7087c6ecb899827b11","hash":"0xbd6e8494e18d627d2331c625203bcea6fb5acb9cdc8a82628f1bc894e90884e2"}`
	txHash := "0xbd6e8494e18d627d2331c625203bcea6fb5acb9cdc8a82628f1bc894e90884e2"

	// Build a JSON-RPC receipt response with a Deposit event from the given router.
	makeReceiptJSON := func(routerAddr string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{
			"status":"0x1",
			"cumulativeGasUsed":"0xe8c5",
			"gasUsed":"0xe8c5",
			"effectiveGasPrice":"0x2540be400",
			"transactionHash":"%s",
			"transactionIndex":"0x0",
			"blockHash":"0x0000000000000000000000000000000000000000000000000000000000000001",
			"blockNumber":"0x22",
			"contractAddress":"0x0000000000000000000000000000000000000000",
			"logsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
			"logs":[{
				"address":"%s",
				"topics":[
					"0xef519b7eb82aaf6ac376a6df2d793843ebfd593de5f1a0601d3cc6ab49ebb395",
					"0x000000000000000000000000%s",
					"0x0000000000000000000000000000000000000000000000000000000000000000"
				],
				"data":"0x0000000000000000000000000000000000000000000000004563918244f40000000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000384144443a4554482e4554483a7474686f72313678786e30636164727575773661327177707633356176306d6568727976647a7a6a7a3361660000000000000000",
				"blockNumber":"0x22",
				"transactionHash":"%s",
				"transactionIndex":"0x0",
				"blockHash":"0x0000000000000000000000000000000000000000000000000000000000000001",
				"logIndex":"0x0",
				"removed":false
			}]
		}}`, txHash, routerAddr, vaultAddrLower, txHash)
	}

	// receiptResponse controls which receipt the mock server returns.
	var receiptResponse string

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.RequestURI == thorclient.ChainVersionEndpoint:
			_, err := rw.Write([]byte(`{"current":"` + types2.GetCurrentVersion().String() + `"}`))
			c.Assert(err, IsNil)
		case req.RequestURI == "/thorchain/mimir/key/EVMDisableContractWhitelist":
			_, err := rw.Write([]byte(`1`))
			c.Assert(err, IsNil)
		case req.RequestURI == thorclient.PubKeysEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
		case req.RequestURI == thorclient.InboundAddressesEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/inbound_addresses/inbound_addresses.json")
		case req.RequestURI == thorclient.AsgardVault:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case strings.HasPrefix(req.RequestURI, thorclient.NodeAccountEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		default:
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
			if err != nil {
				return
			}
			switch rpcRequest.Method {
			case "eth_chainId":
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x539"}`))
				c.Assert(err, IsNil)
			case "eth_gasPrice":
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			case "eth_getTransactionReceipt":
				_, err = rw.Write([]byte(receiptResponse))
				c.Assert(err, IsNil)
			}
		}
	}))
	defer server.Close()

	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	u, err := url.Parse(server.URL)
	c.Assert(err, IsNil)
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       u.Host,
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}
	bridge, err := thorclient.NewThorchainBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
	pkeyMgr, err := pubkeymanager.NewPubKeyManager(bridge, s.m)
	c.Assert(pkeyMgr.Start(), IsNil)
	defer func() { c.Assert(pkeyMgr.Stop(), IsNil) }()
	c.Assert(err, IsNil)

	bs, err := NewETHScanner(getConfigForTest(), storage, big.NewInt(1337), ethClient, bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)

	// Ensure both router addresses pass the log parser's address validator.
	useWhitelistSmartContract = true
	whitelistSmartContractAddress = append(whitelistSmartContractAddress,
		thorcommon.Address(correctRouter),
		thorcommon.Address(wrongRouter),
	)

	tx := &etypes.Transaction{}
	c.Assert(tx.UnmarshalJSON([]byte(encodedTx)), IsNil)

	// Test 1: Aggregator swap-in with correct router — full fromTxToTxIn flow.
	// tx.To() is the aggregator (not vault, not router), so the disableWhitelist=1
	// path routes through: IsSmartContractCall → getTxInFromSmartContract → router validation passes.
	receiptResponse = makeReceiptJSON(correctRouter)
	txInItem, err := bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, NotNil, Commentf("aggregator deposit with correct router should be observed"))
	c.Assert(txInItem.To, Equals, vaultAddr)
	c.Assert(txInItem.Memo, Equals, "ADD:ETH.ETH:tthor16xxn0cadruuw6a2qwpv35av0mehryvdzzjz3af")
	c.Assert(txInItem.Coins[0].Asset.String(), Equals, "ETH.ETH")
	c.Assert(txInItem.Coins[0].Amount.Equal(cosmos.NewUint(500000000)), Equals, true)

	// Test 2: Aggregator swap-in with wrong router — full fromTxToTxIn flow.
	// The deposit event is emitted by the wrong router, so validation rejects it.
	receiptResponse = makeReceiptJSON(wrongRouter)
	txInItem, err = bs.fromTxToTxIn(tx)
	c.Assert(err, IsNil)
	c.Assert(txInItem, IsNil, Commentf("deposit from wrong router should be dropped"))
}

func (s *BlockScannerTestSuite) TestProcessReOrg(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		switch {
		case req.RequestURI == thorclient.PubKeysEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/pubKeys.json")
		case req.RequestURI == thorclient.InboundAddressesEndpoint:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/inbound_addresses/inbound_addresses.json")
		case req.RequestURI == thorclient.AsgardVault:
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/vaults/asgard.json")
		case strings.HasPrefix(req.RequestURI, thorclient.NodeAccountEndpoint):
			httpTestHandler(c, rw, "../../../../test/fixtures/endpoints/nodeaccount/template.json")
		default:
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
			if rpcRequest.Method == "eth_chainId" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_gasPrice" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getTransactionReceipt" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Not found tx"},"id": null}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_call" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x52554e45"}`))
				c.Assert(err, IsNil)
			}
			if rpcRequest.Method == "eth_getBlockByNumber" {
				_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{
				"parentHash":"0x8b535592eb3192017a527bbf8e3596da86b3abea51d6257898b2ced9d3a83826",
				"difficulty": "0x31962a3fc82b",
				"extraData": "0x4477617266506f6f6c",
				"gasLimit": "0x47c3d8",
				"gasUsed": "0x0",
				"hash": "0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
				"logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"miner": "0x2a65aca4d5fc5b5c859090a6c34d164135398226",
				"nonce": "0xa5e8fb780cc2cd5e",
				"number": "0x0",
				"receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
				"size": "0x20e",
				"stateRoot": "0xdc6ed0a382e50edfedb6bd296892690eb97eb3fc88fd55088d5ea753c48253dc",
				"timestamp": "0x579f4981",
				"totalDifficulty": "0x25cff06a0d96f4bee",
				"transactions": [{
					"blockHash":"0x78bfef68fccd4507f9f4804ba5c65eb2f928ea45b3383ade88aaa720f1209cba",
					"blockNumber":"0x1",
					"from":"0xa7d9ddbe1f17865597fbd27ec712455208b6b76d",
					"gas":"0xc350",
					"gasPrice":"0x4a817c800",
					"hash":"0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b",
					"input":"0x68656c6c6f21",
					"nonce":"0x15",
					"to":"0xf02c1c8e6114b1dbe8937a39260b5b0a374432bb",
					"transactionIndex":"0x0",
					"value":"0xf3dbb76162000",
					"v":"0x25",
					"r":"0x1b5e176d927f8e9ab405058b2d2457392da3e20f328b16ddabcebc33eaac5fea",
					"s":"0x4ba69724e8f69de52f0125ad8b3c5c2cef33019bac3249e2c0a2192766d1721c"
				}],
				"transactionsRoot": "0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b",
				"uncles": [
			]}}`))
				c.Assert(err, IsNil)
			}
		}
	}))
	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	c.Assert(ethClient, NotNil)
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	bridge, err := thorclient.NewThorchainBridge(config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       server.Listener.Addr().String(),
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: "",
	}, s.m, s.keys)
	c.Assert(err, IsNil)
	c.Assert(bridge, NotNil)
	pkeyMgr, err := pubkeymanager.NewPubKeyManager(bridge, s.m)
	c.Assert(err, IsNil)
	c.Assert(pkeyMgr.Start(), IsNil)
	defer func() {
		c.Assert(pkeyMgr.Stop(), IsNil)
	}()
	bs, err := NewETHScanner(getConfigForTest(), storage, big.NewInt(int64(Mainnet)), ethClient, s.bridge, s.m, pkeyMgr, func(height int64) error {
		return nil
	}, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)
	block, err := CreateBlock(0)
	c.Assert(err, IsNil)
	c.Assert(block, NotNil)
	blockNew, err := CreateBlock(1)
	c.Assert(err, IsNil)
	c.Assert(blockNew, NotNil)
	blockMeta := types.NewBlockMeta(block, stypes.TxIn{TxArray: []*stypes.TxInItem{{Tx: "0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"}}})
	// add one UTXO which will trigger the re-org process next
	c.Assert(bs.blockMetaAccessor.SaveBlockMeta(0, blockMeta), IsNil)
	bs.globalErrataQueue = make(chan stypes.ErrataBlock, 1)
	reorgedBlocks, err := bs.processReorg(blockNew)
	c.Assert(err, IsNil)
	c.Assert(reorgedBlocks, IsNil)
	// make sure there is errata block in the queue
	c.Assert(bs.globalErrataQueue, HasLen, 1)
	blockMeta, err = bs.blockMetaAccessor.GetBlockMeta(0)
	c.Assert(err, IsNil)
	c.Assert(blockMeta, NotNil)
}

// -------------------------------------------------------------------------------------
// GasPrice
// -------------------------------------------------------------------------------------

func (s *BlockScannerTestSuite) TestGasPrice(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
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
		if rpcRequest.Method == "eth_chainId" {
			_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x539"}`))
			c.Assert(err, IsNil)
		}
		if rpcRequest.Method == "eth_gasPrice" {
			_, err = rw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
			c.Assert(err, IsNil)
		}
	}))
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	ethClient, err := ethclient.Dial(server.URL)
	c.Assert(err, IsNil)
	pubKeyManager, err := pubkeymanager.NewPubKeyManager(s.bridge, s.m)
	c.Assert(err, IsNil)
	solvencyReporter := func(height int64) error {
		return nil
	}
	conf := getConfigForTest()
	bs, err := NewETHScanner(conf, storage, big.NewInt(int64(Mainnet)), ethClient, s.bridge, s.m, pubKeyManager, solvencyReporter, nil)
	c.Assert(err, IsNil)
	c.Assert(bs, NotNil)
	bs.globalNetworkFeeQueue = make(chan thorcommon.NetworkFee, 1)

	baseFee := big.NewInt(0)
	var resolution int64 = 10 // 10 gwei
	resolution *= 1e9         // gwei to wei, for the below usage.

	// almost fill gas cache
	for i := 0; i < 39; i++ {
		bs.updateGasPrice(baseFee, []*big.Int{big.NewInt(1 * resolution), big.NewInt(2 * resolution), big.NewInt(3 * resolution), big.NewInt(4 * resolution)})
	}

	// empty blocks should not count
	bs.updateGasPrice(baseFee, []*big.Int{})
	c.Assert(len(bs.gasCache), Equals, 39)
	c.Assert(bs.gasPrice.Cmp(big.NewInt(initialGasPrice)), Equals, 0)

	// now we should get the average of the 25th percentile gas (2)
	bs.updateGasPrice(baseFee, []*big.Int{big.NewInt(1 * resolution), big.NewInt(2 * resolution), big.NewInt(3 * resolution), big.NewInt(4 * resolution)})
	c.Assert(len(bs.gasCache), Equals, 40)
	c.Assert(bs.gasPrice.Uint64(), Equals, big.NewInt(2*resolution).Uint64())

	// add 20 more blocks with 2x the 25th percentile and we should get 6 (3 + 3x stddev)
	for i := 0; i < 20; i++ {
		bs.updateGasPrice(baseFee, []*big.Int{big.NewInt(2 * resolution), big.NewInt(4 * resolution), big.NewInt(6 * resolution), big.NewInt(8 * resolution)})
	}
	c.Assert(len(bs.gasCache), Equals, 40)
	c.Assert(bs.gasPrice.Uint64(), Equals, big.NewInt(6*resolution).Uint64())

	// add 20 more blocks with 2x the 25th percentile and we should get 4
	for i := 0; i < 20; i++ {
		bs.updateGasPrice(baseFee, []*big.Int{big.NewInt(2 * resolution), big.NewInt(4 * resolution), big.NewInt(6 * resolution), big.NewInt(8 * resolution)})
	}
	c.Assert(len(bs.gasCache), Equals, 40)
	c.Assert(bs.gasPrice.Uint64(), Equals, big.NewInt(4*resolution).Uint64())

	// add 20 more blocks with 2x the 25th percentile and we should get 12 (6 + 3x stddev)
	for i := 0; i < 20; i++ {
		bs.updateGasPrice(baseFee, []*big.Int{big.NewInt(4 * resolution), big.NewInt(8 * resolution), big.NewInt(12 * resolution), big.NewInt(16 * resolution)})
	}
	c.Assert(len(bs.gasCache), Equals, 40)
	c.Assert(bs.gasPrice.Uint64(), Equals, big.NewInt(12*resolution).Uint64())
}
