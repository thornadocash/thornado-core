package xrp

import (
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"

	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	. "gopkg.in/check.v1"

	addresscodec "github.com/Peersyst/xrpl-go/address-codec"
	"github.com/Peersyst/xrpl-go/xrpl/rpc"
	"github.com/Peersyst/xrpl-go/xrpl/rpc/testutil"
	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type XrpTestSuite struct {
	thordir  string
	thorKeys *thorclient.Keys
	bridge   thorclient.ThorchainBridge
	m        *metrics.Metrics
}

var _ = Suite(&XrpTestSuite{})

var m *metrics.Metrics

func GetMetricForTest(c *C) *metrics.Metrics {
	if m == nil {
		var err error
		m, err = metrics.NewMetrics(config.BifrostMetricsConfiguration{
			Enabled:      false,
			ListenPort:   9000,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
			Chains:       common.Chains{common.XRPChain},
		})
		c.Assert(m, NotNil)
		c.Assert(err, IsNil)
	}
	return m
}

func (s *XrpTestSuite) SetUpSuite(c *C) {
	cosmosSDKConfg := cosmos.GetConfig()
	cosmosSDKConfg.SetBech32PrefixForAccount("sthor", "sthorpub")

	s.m = GetMetricForTest(c)
	c.Assert(s.m, NotNil)
	ns := strconv.Itoa(time.Now().Nanosecond())
	c.Assert(os.Setenv("NET", "stagenet"), IsNil)

	s.thordir = filepath.Join(os.TempDir(), ns, ".thorcli")
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      "bob",
		SignerPasswd:    "password",
		ChainHomeFolder: s.thordir,
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.thorKeys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.thorKeys)
	c.Assert(err, IsNil)
}

func (s *XrpTestSuite) TearDownSuite(c *C) {
	c.Assert(os.Unsetenv("NET"), IsNil)
	if err := os.RemoveAll(s.thordir); err != nil {
		c.Error(err)
	}
}

func (s *XrpTestSuite) TestGetAddress(c *C) {
	response := `{
		"result": {
			"account_data": {
				"Flags": 8388608,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "999999999960",
				"OwnerCount": 0,
				"PreviousTxnID": "4294BEBE5B569A18C0A2702387C9B1E7146DC3A5850C1E87204951C6FDAA4C42",
				"PreviousTxnLgrSeq": 3,
				"Sequence": 6
			},
			"ledger_current_index": 4,
			"queue_data": {
				"txn_count": 5,
				"auth_change_queued": true,
				"lowest_sequence": 6,
				"highest_sequence": 10,
				"max_spend_drops_total": "500",
				"transactions": [
					{
						"auth_change": false,
						"fee": "100",
						"fee_level": "2560",
						"max_spend_drops": "100",
						"seq": 6
					},
					{
						"auth_change": true,
						"fee": "100",
						"fee_level": "2560",
						"max_spend_drops": "100",
						"seq": 10
					}
				]
			},
			"validated": false
		},
		"warning": "none",
		"warnings":
		[{
			"id": 1,
			"message": "message"
		}]
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(response, 200, mc)

	cfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	xrpclient := Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(cfg),
	}

	addr := "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ"
	xrp, _ := common.NewAsset("XRP.XRP")
	expectedCoins := common.NewCoins(
		common.NewCoin(xrp, cosmos.NewUint(99999999996000)),
	)

	acc, err := xrpclient.GetAccountByAddress(addr, big.NewInt(0))
	c.Assert(err, IsNil)
	c.Check(acc.AccountNumber, Equals, int64(0))
	c.Check(acc.Sequence, Equals, int64(6))
	c.Check(acc.Coins.EqualsEx(expectedCoins), Equals, true)

	pk := common.PubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	acc, err = xrpclient.GetAccount(pk, big.NewInt(0))
	c.Assert(err, IsNil)
	c.Check(acc.AccountNumber, Equals, int64(0))
	c.Check(acc.Sequence, Equals, int64(6))
	c.Check(acc.Coins.EqualsEx(expectedCoins), Equals, true)

	resultAddr := xrpclient.GetAddress(pk)

	c.Logf(resultAddr)
	c.Check(addr, Equals, resultAddr)
}

func (s *XrpTestSuite) TestProcessOutboundTx(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				StartBlockHeight: 1, // avoids querying thorchain for block height
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)
	c.Check(client.networkID, Equals, uint32(1234))

	vaultPubKey, err := common.NewPubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	c.Assert(err, IsNil)
	outAsset, err := common.NewAsset("XRP.XRP")
	c.Assert(err, IsNil)
	toAddress, err := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")
	c.Assert(err, IsNil)

	paymentAmount := uint64(24528300)
	txOut := stypes.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(paymentAmount))},
		Memo:        "memo",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	msg, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)

	c.Check(msg.Amount, Equals, txtypes.XRPCurrencyAmount(paymentAmount/100))
	c.Check(msg.BaseTx.Account, Equals, txtypes.Address("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ"))
	c.Check(msg.Destination, Equals, txtypes.Address(toAddress.String()))
	c.Check(msg.BaseTx.NetworkID, Equals, uint32(1234))

	signingPubKey, err := vaultPubKey.Secp256K1()
	c.Assert(err, IsNil)
	signingPubKeyHex := hex.EncodeToString(signingPubKey.SerializeCompressed())
	c.Check(msg.BaseTx.SigningPubKey, Equals, signingPubKeyHex)
}

func (s *XrpTestSuite) TestXAddresses(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				StartBlockHeight: 1, // avoids querying thorchain for block height
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)
	c.Check(client.networkID, Equals, uint32(1234))

	vaultPubKey, err := common.NewPubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	c.Assert(err, IsNil)
	outAsset, err := common.NewAsset("XRP.XRP")
	c.Assert(err, IsNil)
	classicAddr := "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ"
	destTag := uint32(177)
	toAddress, err := addresscodec.ClassicAddressToXAddress(classicAddr, destTag, true, false)
	c.Assert(err, IsNil)

	paymentAmount := uint64(24528300)
	txOut := stypes.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   common.Address(toAddress),
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(paymentAmount))},
		Memo:        "memo",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	msg, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)

	c.Check(msg.Amount, Equals, txtypes.XRPCurrencyAmount(paymentAmount/100))
	c.Check(msg.BaseTx.Account, Equals, txtypes.Address("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ"))
	c.Check(msg.Destination, Not(Equals), txtypes.Address(classicAddr))
}

func (s *XrpTestSuite) TestSign(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1, // avoids querying thorchain for block height
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)
	c.Check(client.networkID, Equals, uint32(1234))

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)
	outAsset, err := common.NewAsset("XRP.XRP")
	c.Assert(err, IsNil)
	toAddress, err := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")
	c.Assert(err, IsNil)
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "memo",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	msg, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)

	meta := client.accts.Get(vaultPubKey)
	c.Check(meta.SeqNumber, Equals, int64(0))

	msg.Sequence = uint32(meta.SeqNumber + 1)
	msg.Fee = txtypes.XRPCurrencyAmount(txOut.GasRate)
	if txOut.Memo != "" {
		msg.BaseTx.Memos = []txtypes.MemoWrapper{
			{
				Memo: txtypes.Memo{
					MemoData: hex.EncodeToString([]byte(txOut.Memo)),
				},
			},
		}
	}

	// Sign the message (verifies signature within)
	_, err = client.signMsg(msg, vaultPubKey)
	c.Assert(err, IsNil)
}

// Test signature verification with a signature that needs padding
func (s *XrpTestSuite) TestSignatureRequiringPadding(c *C) {
	signBytesHex := "5354580012000021000004d224000000016140000000017645e06840000000000b71b07321030cef2112503d3a56d2d48b3a0f0f6503e4353400f450f9dbf344d182e7c7069c8114d4b66bcf790babd0032c5dbfdc14ff1c643a4f488314fdffd00f2f2d215ecdad483b99be1dad2259b9c3f9ea7d046d656d6fe1f1"
	signatureHex := "30440221008e9bc0a8d7927f1874d318bc2a57691b5321d618dd57857d64402be0e0bc0007021f4ab281ef93b0c448a9e75d6c07e9cf05ee705a3c51aafa61992510b65a03ae"
	compressedPubKeyHex := "030cef2112503d3a56d2d48b3a0f0f6503e4353400f450f9dbf344d182e7c7069c"

	signBytes, err := hex.DecodeString(signBytesHex)
	c.Assert(err, IsNil)
	signature, err := hex.DecodeString(signatureHex)
	c.Assert(err, IsNil)
	compressedPubKey, err := hex.DecodeString(compressedPubKeyHex)
	c.Assert(err, IsNil)

	verified := verifySignature(signBytes, signature, compressedPubKey)
	c.Check(verified, Equals, true)
}
