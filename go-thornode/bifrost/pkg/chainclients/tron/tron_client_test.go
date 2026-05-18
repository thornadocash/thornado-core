package tron

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/tron/rpc"
	"gitlab.com/thorchain/thornode/v3/bifrost/pubkeymanager"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common/tokenlist"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/tron/api"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"

	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type TronTestSuite struct {
	tmpdir  string
	metrics *metrics.Metrics
	client  *TronClient
	api     *httptest.Server
	rpc     *httptest.Server
}

// mockBridge is a mock implementation of ThorchainBridge for testing
type mockBridge struct {
	thorclient.ThorchainBridge
}

func (m *mockBridge) GetBlockTimestamp(height int64) (time.Time, error) {
	// Return a fixed timestamp that matches the test's refBlock
	return time.Unix(0, 1749121581000*int64(time.Millisecond)), nil
}

var _ = Suite(&TronTestSuite{})

var m *metrics.Metrics

func GetMetricForTest(c *C) *metrics.Metrics {
	if m == nil {
		var err error
		m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9000,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.TRONChain},
		})
		c.Assert(m, NotNil)
		c.Assert(err, IsNil)
	}
	return m
}

func (s *TronTestSuite) SetUpSuite(c *C) {
	cosmosSDKConfg := cosmos.GetConfig()
	cosmosSDKConfg.SetBech32PrefixForAccount("sthor", "sthorpub")

	s.api = api.NewMockServer()
	c.Assert(s.api, NotNil)

	s.rpc = rpc.NewMockServer()
	c.Assert(s.rpc, NotNil)

	s.metrics = GetMetricForTest(c)
	c.Assert(s.metrics, NotNil)
	ns := strconv.Itoa(time.Now().Nanosecond())
	c.Assert(os.Setenv("NET", "stagenet"), IsNil)

	s.tmpdir = filepath.Join(os.TempDir(), ns, ".thorcli")

	clientConfig := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: s.tmpdir,
	}

	chainConfig := config.BifrostChainConfiguration{
		ChainID: common.TRONChain,
		APIHost: s.api.URL,
		RPCHost: s.rpc.URL,
		BlockScanner: config.BifrostBlockScannerConfiguration{
			HTTPRequestTimeout: time.Second * 2,
			WhitelistTokens: []string{
				"TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs",
			},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)

	kb := keyring.NewInMemory(codec.NewProtoCodec(registry))
	_, _, err := kb.NewMnemonic(
		clientConfig.SignerName,
		keyring.English,
		cmd.THORChainHDPath,
		clientConfig.SignerPasswd,
		hd.Secp256k1,
	)
	c.Assert(err, IsNil)

	keys := thorclient.NewKeysWithKeybase(
		kb, clientConfig.SignerName, clientConfig.SignerPasswd,
	)
	c.Assert(err, IsNil)

	localKeyManager, err := NewLocalKeyManager(keys)
	c.Assert(err, IsNil)

	// Use mock bridge for tests to avoid needing a real THORChain node
	bridge := &mockBridge{}

	whitelist := map[string]tokenlist.ERC20Token{
		"TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs": {
			Name:     "USDT",
			Symbol:   "USDT",
			Address:  "TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs",
			Decimals: 6,
		},
	}

	pubkeyMgr := &pubkeymanager.MockPoolAddressValidator{}

	scanner, err := NewTronBlockScanner(
		chainConfig, bridge, pubkeyMgr, func(int64) error { return nil },
	)
	c.Assert(err, IsNil)

	// the mock block from getnowblock.json
	scanner.refBlocks = []RefBlock{{
		Timestamp: 1749121581000,
		Height:    55088560,
		Id:        "00000000034895b0e5029588a71f9689fb5c31e2c66989389d6ff09548726f6e",
	}}

	storage, err := blockscanner.NewBlockScannerStorage(s.tmpdir, config.LevelDBOptions{})
	c.Assert(err, IsNil)
	c.Assert(scanner, NotNil)

	signerCacheManager, err := signercache.NewSignerCacheManager(
		storage.GetInternalDb(),
	)
	c.Assert(err, IsNil)

	s.client = &TronClient{
		chainId:            common.TRONChain.String(),
		config:             chainConfig,
		bridge:             bridge,
		whitelist:          whitelist,
		api:                api.NewTronApi(chainConfig.APIHost, chainConfig.BlockScanner.HTTPRequestTimeout),
		rpc:                rpc.NewTronRpc(chainConfig.RPCHost, chainConfig.BlockScanner.HTTPRequestTimeout),
		tronScanner:        scanner,
		localKeyManager:    localKeyManager,
		signerCacheManager: signerCacheManager,
	}

	s.client.abi, err = abi.JSON(bytes.NewReader(trc20ContractABI))
	c.Assert(err, IsNil)
}

func (s *TronTestSuite) TearDownSuite(c *C) {
	c.Assert(os.Unsetenv("NET"), IsNil)
	if err := os.RemoveAll(s.tmpdir); err != nil {
		c.Error(err)
	}
}

func (s *TronTestSuite) TestGetAddress(c *C) {
	pubKey := common.PubKey("sthorpub1addwnpepqf72ur2e8zk8r5augtrly40cuy94f7e663zh798tyms6pu2k8qdswf4es66")
	address := s.client.GetAddress(pubKey)
	c.Assert(address, Equals, "TJWs2NWfr4y61S6FbUYhUBJmMhK1SYaEFi")

	address = s.client.GetAddress("nonsense")
	c.Assert(address, Equals, "")
}

func (s *TronTestSuite) TestGetAccountByAddress(c *C) {
	address := "TU6nEM4GTca2L5AuDTnY1qp1rkQ2t8NxvM"

	account, err := s.client.GetAccountByAddress(address, nil)
	c.Assert(err, IsNil)
	c.Assert(len(account.Coins), Equals, 2)
	missing := 0
	for _, coin := range account.Coins {
		switch coin.Asset.Symbol.String() {
		case "TRX":
			c.Assert(coin.Amount.String(), Equals, "212331370000")
		case "USDT-TG3XXYEXBKPP9NZDAJDZSOZEU4BKASJOZS":
			c.Assert(coin.Amount.String(), Equals, "351560000000")
		default:
			missing++
		}
	}
	c.Assert(missing, Equals, 0)
}

func (s *TronTestSuite) TestGetConfirmationCountAndBlocks(c *C) {
	c.Assert(s.client.GetConfirmationCount(types.TxIn{}), Equals, int64(0))
	c.Assert(ConfirmationBlocks, Equals, int64(1))
}

func (s *TronTestSuite) TestGetConfig(c *C) {
	cfg := s.client.GetConfig()
	c.Assert(cfg, NotNil)
}

func (s *TronTestSuite) TestSignAndBroadcastTx(c *C) {
	coins := []common.Coin{{
		Asset:    common.TRXAsset,
		Amount:   cosmos.NewUint(150_000_000_000),
		Decimals: 6,
	}}

	gas := common.Gas{{
		Asset:    common.TRXAsset,
		Amount:   cosmos.NewUint(1_100_000),
		Decimals: 6,
	}}

	txOutItem := types.TxOutItem{
		Chain:       common.TRONChain,
		ToAddress:   "TU6nEM4GTca2L5AuDTnY1qp1rkQ2t8NxvM",
		VaultPubKey: s.client.localKeyManager.Pubkey(),
		Coins:       coins,
		MaxGas:      gas,
		GasRate:     1,
	}

	//
	signed, _, _, err := s.client.SignTx(txOutItem, 1)
	c.Assert(err, IsNil)
	c.Assert(signed, NotNil)

	var tx api.Transaction
	err = json.Unmarshal(signed, &tx)
	c.Assert(err, IsNil)

	// values used from scanner.refBlocks (api/test-tron/getnowblock.json)
	c.Assert(tx.RawData.RefBlockBytes, Equals, "95b0")
	c.Assert(tx.RawData.RefBlockHash, Equals, "e5029588a71f9689")
	c.Assert(tx.RawData.Timestamp, Equals, int64(1749121581000))
	c.Assert(tx.RawData.Expiration, Equals, 1749121581000+TimestampValidity.Milliseconds())
	c.Assert(tx.RawData.Contract[0].Type, Equals, "TransferContract")
	c.Assert(len(tx.Signature), Equals, 1)
	c.Assert(len(tx.Signature[0]), Equals, 130)

	txId, err := s.client.BroadcastTx(txOutItem, signed)
	c.Assert(err, IsNil)
	// tx id returned by api/test-tron/broadcasttransaction.json
	c.Assert(txId, Equals, "77ddfa7093cc5f745c0d3a54abb89ef070f983343c05e0f89e5a52f3e5401299")
}

func (s *TronTestSuite) TestShouldReportSolvency(c *C) {
	c.Assert(s.client.ShouldReportSolvency(1), Equals, false)
	c.Assert(s.client.ShouldReportSolvency(9570), Equals, true)
}

func (s *TronTestSuite) TestCreateTrc20Transaction(c *C) {
	testCases := []struct {
		From     string
		To       string
		Contract string
		Fail     bool
	}{
		{
			From:     "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
			To:       "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
			Contract: "TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs",
			Fail:     false,
		},
		{
			From:     "TWfHpcbBVTcQaakccHt3d1d41iNgzaPwow",
			To:       "bc1qelysxslyl86yq57yx9lzhl50pfayj9wng54p3g",
			Contract: "TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs",
			Fail:     true,
		},
	}

	amount := big.NewInt(234_560_000_000)
	gas := big.NewInt(4_000_000)

	for _, tc := range testCases {
		tx, err := s.client.createTrc20Transaction(
			tc.From, tc.To, tc.Contract, *amount, *gas,
		)
		if tc.Fail {
			c.Assert(err, NotNil)
			continue
		}
		// returns mock data from api/test-tron/triggersmartcontract.json
		c.Assert(err, IsNil)
		c.Assert(tx.TxId, Equals, "482b1a3b61894f75ea25bd10b14335a4db86c7e2c642ae07abc5a8ae45fb0027")
	}
}
