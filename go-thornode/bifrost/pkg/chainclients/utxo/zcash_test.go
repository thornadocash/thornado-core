package utxo

import (
	"encoding/hex"
	"fmt"
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
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/mock"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	ttypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type ZcashSuite struct {
	client   *Client
	chainRpc *httptest.Server
	thorApi  *httptest.Server
	bridge   thorclient.ThorchainBridge
	cfg      config.BifrostChainConfiguration
	m        *metrics.Metrics
	keys     *thorclient.Keys
}

var _ = Suite(&ZcashSuite{})

func (s *ZcashSuite) SetUpSuite(c *C) {
	ttypes.SetupConfigForTest()

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kb := cKeys.NewInMemory(cdc)
	_, _, err := kb.NewMnemonic(bob, cKeys.English, cmd.THORChainHDPath, password, hd.Secp256k1)
	c.Assert(err, IsNil)
	s.keys = thorclient.NewKeysWithKeybase(kb, bob, password)
}

func (s *ZcashSuite) SetUpTest(c *C) {
	s.m = GetMetricForTest(c, common.ZECChain)
	s.cfg = config.BifrostChainConfiguration{
		ChainID:     "ZEC",
		UserName:    bob,
		Password:    password,
		DisableTLS:  true,
		HTTPostMode: true,
		BlockScanner: config.BifrostBlockScannerConfiguration{
			StartBlockHeight: 1, // avoids querying thorchain for block height
		},
	}
	s.cfg.UTXO.TransactionBatchSize = 100
	s.cfg.UTXO.MaxMempoolBatches = 10
	s.cfg.UTXO.EstimatedAverageTxSize = 250
	s.cfg.BlockScanner.MaxReorgRescanBlocks = 1
	ns := strconv.Itoa(time.Now().Nanosecond())

	thordir := filepath.Join(os.TempDir(), ns, ".thorcli")
	cfg := config.BifrostClientConfiguration{
		ChainID:         "thorchain",
		ChainHost:       "localhost",
		SignerName:      bob,
		SignerPasswd:    password,
		ChainHomeFolder: thordir,
	}

	s.chainRpc = mock.NewChainRpc(common.ZECChain)
	s.thorApi = mock.NewThornodeApi()

	var err error
	cfg.ChainHost = s.thorApi.Listener.Addr().String()
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.keys)
	c.Assert(err, IsNil)
	s.cfg.RPCHost = s.chainRpc.Listener.Addr().String()
	s.client, err = NewClient(s.keys, s.cfg, nil, s.bridge, s.m)
	s.client.disableVinZeroBatch = true
	s.client.globalNetworkFeeQueue = make(chan common.NetworkFee, 1)
	c.Assert(err, IsNil)
	c.Assert(s.client, NotNil)
}

func (s *ZcashSuite) TearDownTest(_ *C) {
	s.chainRpc.Close()
	s.thorApi.Close()
}

func (s *ZcashSuite) TestGetMemo(c *C) {
	tx := btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:       "OP_RETURN 74686f72636861696e3a636f6e736f6c6964617465",
					Hex:       "6a1574686f72636861696e3a636f6e736f6c6964617465",
					ReqSigs:   0,
					Type:      "nulldata",
					Addresses: nil,
				},
			},
		},
	}
	memo, err := s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "thorchain:consolidate")

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a30786335346331353132363936463345413739353662643961443431",
					Type: "nulldata",
					Hex:  "6a4c50737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a30786335346331353132363936463345413739353662643961443431",
				},
			},
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 30383138654563414443466666663a3130303030303030303030",
					Type: "nulldata",
					Hex:  "6a1a30383138654563414443466666663a3130303030303030303030",
				},
			},
		},
	}
	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "swap:eth.0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:10000000000")

	tx = btcjson.TxRawResult{
		Vin:  []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{},
	}
	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "")

	// OP_RETURN + data encoded in subsequent vout addresses
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "swap:eth.0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:0xc54c1512696F3EA7956bd9aD4^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a3078633534633135313236393646334541373935366264396144345e",
					Type: "nulldata",
					Hex:  "6a4c50737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a3078633534633135313236393646334541373935366264396144345e",
				},
			},
			{
				// data: "10818eEcADCFfff:1000"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 3130383138654563414443466666663a31303030",
					Type: "witness_v0_keyhash",
					Hex:  "00143130383138654563414443466666663a31303030",
				},
			},
			{
				// data: "0000000"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 3030303030303000000000000000000000000000",
					Type: "witness_v0_keyhash",
					Hex:  "00143030303030303000000000000000000000000000",
				},
			},
		},
	}
	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "swap:eth.0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:10000000000")

	// OP_RETURN + data encoded in subsequent vout addresses off different kind
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "swap:eth.0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:0xc54c1512696F3EA7956bd9aD4^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a3078633534633135313236393646334541373935366264396144345e",
					Type: "nulldata",
					Hex:  "6a4c50737761703a6574682e3078633534633135313236393646334541373935366264396144343130383138654563414443466666663a3078633534633135313236393646334541373935366264396144345e",
				},
			},
			{
				// data: "10818eEcADCFfff:1000"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_DUP OP_HASH160 3130383138654563414443466666663a31303030 OP_EQUALVERIFY OP_CHECKSIG",
					Type: "pubkeyhash",
					Hex:  "76a9143130383138654563414443466666663a3130303088ac",
				},
			},
			{
				// data: "0000000"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 3030303030303000000000000000000000000000",
					Type: "witness_v0_keyhash",
					Hex:  "00143030303030303000000000000000000000000000",
				},
			},
		},
	}
	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "swap:eth.0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:0xc54c1512696F3EA7956bd9aD410818eEcADCFfff:10000000000")

	// OP_RETURN exactly 80 chars
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "SWAP:AVAX.USDC-C48A6E:0x2BBA9D4B62A3673146C36FE3B31C36AF02648E99:0/1/0:-_/t:5/50"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 535741503a415641582e555344432d4334384136453a3078324242413944344236324133363733313436433336464533423331433336414630323634384539393a302f312f303a2d5f2f743a352f3530",
					Type: "nulldata",
					Hex:  "6a4c50535741503a415641582e555344432d4334384136453a3078324242413944344236324133363733313436433336464533423331433336414630323634384539393a302f312f303a2d5f2f743a352f3530",
				},
			},
			{
				// no marker at position >= 79, ignore this vout
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 47715D6877ACF46FAEADDFE010DBDEA83FC19577",
					Type: "witness_v0_keyhash",
					Hex:  "001447715D6877ACF46FAEADDFE010DBDEA83FC19577",
				},
			},
		},
	}
	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "SWAP:AVAX.USDC-C48A6E:0x2BBA9D4B62A3673146C36FE3B31C36AF02648E99:0/1/0:-_/t:5/50")

	// OP_RETURN + OP_RETURN + data encoded in subsequent vout address
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "SWAP:"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 535741503a",
					Type: "nulldata",
					Hex:  "6a05535741503a",
				},
			},
			{
				// no marker at position >= 79, ignore this vout
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 47715D6877ACF46FAEADDFE010DBDEA83FC19577",
					Type: "witness_v0_keyhash",
					Hex:  "001447715D6877ACF46FAEADDFE010DBDEA83FC19577",
				},
			},
			{
				// data: "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48:0x2BBA9D4B62A3673146C36FE3B^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 4554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a3078324242413944344236324133363733313436433336464533425e",
					Type: "nulldata",
					Hex:  "6a4c504554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a3078324242413944344236324133363733313436433336464533425e",
				},
			},
			{
				// data: "31C36AF02648E99"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 3331433336414630323634384539390000000000",
					Type: "witness_v0_keyhash",
					Hex:  "00143331433336414630323634384539390000000000",
				},
			},
		},
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "SWAP:ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48:0x2BBA9D4B62A3673146C36FE3B31C36AF02648E99")

	// OP_RETURN with marker + invalid encoded data (eg. real address)
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48:0x2BBA9D4B62A3673146C36FE3B^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 4554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a3078324242413944344236324133363733313436433336464533425e",
					Type: "nulldata",
					Hex:  "6a4c504554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a3078324242413944344236324133363733313436433336464533425e",
				},
			},
			{
				// real address, containing non alphanumeric chars
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 47715D6877ACF46FAEADDFE010DBDEA83FC19577",
					Type: "witness_v0_keyhash",
					Hex:  "001447715D6877ACF46FAEADDFE010DBDEA83FC19577",
				},
			},
		},
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "")

	// 2 x OP_RETURN with multiple markers + data encoded in subsequent vout address
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "SWAP:ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB4^^8:0x2BBA9D4B62A3673146^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 535741503a4554482e555344432d30584130423836393931433632313842333643314431394434413245394542304345333630364542345e5e383a30783242424139443442363241333637333134365e",
					Type: "nulldata",
					Hex:  "6a4c50535741503a4554482e555344432d30584130423836393931433632313842333643314431394434413245394542304345333630364542345e5e383a30783242424139443442363241333637333134365e",
				},
			},
			{
				// data: "^C3^6F^E3^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 5e43335e36465e45335e",
					Type: "nulldata",
					Hex:  "6a0a5e43335e36465e45335e",
				},
			},
			{
				// data: "B31C36AF02648E99"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 4233314333364146303236343845393900000000",
					Type: "witness_v0_keyhash",
					Hex:  "00144233314333364146303236343845393900000000",
				},
			},
		},
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "SWAP:ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB4^8:0x2BBA9D4B62A3673146^^C3^6F^E3^B31C36AF02648E99")

	// 2 x OP_RETURN every allowed chars
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 2122232425262728292a2b2c2d2e2f3a3b3c3d3e3f405b5c5d5e5f607b7c7d7e",
					Type: "nulldata",
					Hex:  "6a202122232425262728292a2b2c2d2e2f3a3b3c3d3e3f405b5c5d5e5f607b7c7d7e",
				},
			},
			{
				// data: "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 303132333435363738394142434445464748494a4b4c4d4e4f505152535455565758595a6162636465666768696a6b6c6d6e6f707172737475767778797a",
					Type: "nulldata",
					Hex:  "6a3e303132333435363738394142434445464748494a4b4c4d4e4f505152535455565758595a6162636465666768696a6b6c6d6e6f707172737475767778797a",
				},
			},
		},
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	// ^ marker not removed
	c.Assert(memo, Equals, "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

	// OP_RETURN + data address, end parsing on first 00 terminated address
	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "=:ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48:0x2BBA9D4B62A3673146C36FE^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 3d3a4554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a307832424241394434423632413336373331343643333646455e",
					Type: "nulldata",
					Hex:  "6a4c503d3a4554482e555344432d3058413042383639393143363231384233364331443139443441324539454230434533363036454234383a307832424241394434423632413336373331343643333646455e",
				},
			},
			{
				// data: "3B31C36AF02648E99"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "0 3342333143333641463032363438453939000000",
					Type: "witness_v0_keyhash",
					Hex:  "00143342333143333641463032363438453939000000",
				},
			},
			{
				// previous address ending with 00, parsing already stopped
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "3342333143333641463032363438453939000000",
					Type: "witness_v0_keyhash",
					Hex:  "00143342333143333641463032363438453939000000",
				},
			},
		},
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, "=:ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48:0x2BBA9D4B62A3673146C36FE3B31C36AF02648E99")

	// OP_RETURN + 10 data addresses,
	// stop processing after the 9th address, MaxMemoSize (250) already reached
	expectedMemo := "0000000000000000000000000000000000000000000000000000000000000000000000000000000"

	tx = btcjson.TxRawResult{
		Vin: []btcjson.Vin{{}}, // dummy, otherwise treated as shielded tx
		Vout: []btcjson.Vout{
			{
				// data: "0000000000000000000000000000000000000000000000000000000000000000000000000000000^"
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Asm:  "OP_RETURN 303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030305e",
					Type: "nulldata",
					Hex:  "6a4c50303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030303030305e",
				},
			},
		},
	}

	for i := 1; i <= 10; i++ {
		text := strings.Repeat(fmt.Sprintf("%d", i), 20)
		encoded := hex.EncodeToString([]byte(text))

		// memo only processed up to the 9th vout
		if i <= 9 {
			expectedMemo += text
		}

		tx.Vout = append(tx.Vout, btcjson.Vout{
			ScriptPubKey: btcjson.ScriptPubKeyResult{
				Asm:  "0 " + encoded,
				Type: "witness_v0_keyhash",
				Hex:  "0014" + encoded,
			},
		})
	}

	memo, err = s.client.getMemo(&tx)
	c.Assert(err, IsNil)
	c.Assert(memo, Equals, expectedMemo)
}

// TestGetChain verifies the client returns the correct chain identifier (ZECChain).
// This ensures the client is properly configured for Zcash operations.
func (s *ZcashSuite) TestGetChain(c *C) {
	chain := s.client.GetChain()
	c.Assert(chain, Equals, common.ZECChain)
}

// TestGetConfig verifies the client returns the configuration with correct ChainID.
// This ensures the configuration was properly passed to and stored by the client.
func (s *ZcashSuite) TestGetConfig(c *C) {
	cfg := s.client.GetConfig()
	c.Assert(cfg.ChainID.String(), Equals, "ZEC")
}

// TestGetHeight verifies the client can retrieve the current block height from the RPC.
// The fixture returns height 3203735
func (s *ZcashSuite) TestGetHeight(c *C) {
	height, err := s.client.GetHeight()
	c.Assert(err, IsNil)
	// blockcount.json returns 3203735
	c.Assert(height, Equals, int64(3203735))
}

// TestIgnoreTx tests the transaction filtering logic for various edge cases.
// Zcash-specific: shielded transactions (empty vin or vout) must be ignored
// since they cannot be processed by the client implementation.
// Also tests standard UTXO filtering.
func (s *ZcashSuite) TestIgnoreTx(c *C) {
	testCases := []struct {
		Name          string
		Tx            btcjson.TxRawResult
		CurrentHeight int64
		ExpectIgnored bool
		Reason        string
	}{
		{
			Name: "Shielded transaction (empty vin)",
			Tx: btcjson.TxRawResult{
				Vin:  []btcjson.Vin{},
				Vout: []btcjson.Vout{{Value: 1.0}},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Shielded transactions have no transparent inputs and cannot be processed",
		},
		{
			Name: "Shielded transaction (empty vout)",
			Tx: btcjson.TxRawResult{
				Vin:  []btcjson.Vin{{Txid: "abc123"}},
				Vout: []btcjson.Vout{},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Shielded transactions have no transparent outputs and cannot be processed",
		},
		{
			Name: "Coinbase transaction",
			Tx: btcjson.TxRawResult{
				Vin: []btcjson.Vin{
					{Coinbase: "0397e23000"}, // Coinbase field present
				},
				Vout: []btcjson.Vout{{Value: 1.254376}},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Coinbase transactions are mining rewards and should be ignored",
		},

		{
			Name: "Transaction with future locktime",
			Tx: btcjson.TxRawResult{
				Vin:      []btcjson.Vin{{Txid: "abc123"}},
				Vout:     []btcjson.Vout{{Value: 1.0}},
				LockTime: 500000000, // Block height in the future
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Transactions with locktime in the future are not yet valid",
		},
		{
			Name: "Valid transparent transaction",
			Tx: btcjson.TxRawResult{
				Vin:  []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{{Value: 1.0}},
			},
			CurrentHeight: 100,
			ExpectIgnored: false,
			Reason:        "Standard transparent transaction should be processed",
		},
		{
			Name: "Transaction with more than 12 outputs",
			Tx: btcjson.TxRawResult{
				Vin: []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0},
					{Value: 1.0}, // 13th output
				},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Transactions with >12 outputs are not THORChain format",
		},
		{
			Name: "Transaction with all zero-value outputs",
			Tx: btcjson.TxRawResult{
				Vin:  []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{{Value: 0}, {Value: 0}, {Value: 0}},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Transactions with no value outputs have nothing to transfer",
		},
		{
			Name: "Transaction with more than 10 non-zero outputs",
			Tx: btcjson.TxRawResult{
				Vin: []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1}, // 11 non-zero outputs
				},
			},
			CurrentHeight: 100,
			ExpectIgnored: true,
			Reason:        "Transactions with >10 non-zero outputs are not THORChain format",
		},
		{
			Name: "Transaction with exactly 10 non-zero outputs (boundary)",
			Tx: btcjson.TxRawResult{
				Vin: []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1}, // exactly 10 non-zero outputs
				},
			},
			CurrentHeight: 100,
			ExpectIgnored: false,
			Reason:        "Exactly 10 non-zero outputs is valid THORChain format",
		},
		{
			Name: "Transaction with exactly 12 outputs (boundary)",
			Tx: btcjson.TxRawResult{
				Vin: []btcjson.Vin{{Txid: "abc123", Vout: 0}},
				Vout: []btcjson.Vout{
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0.1},
					{Value: 0},
					{Value: 0},
					{Value: 0},
					{Value: 0},
					{Value: 0},
					{Value: 0}, // 12 outputs, 6 non-zero
				},
			},
			CurrentHeight: 100,
			ExpectIgnored: false,
			Reason:        "Exactly 12 outputs is valid if <=10 have value",
		},
	}

	for _, tc := range testCases {
		result := s.client.ignoreTx(&tc.Tx, tc.CurrentHeight)
		c.Assert(result, Equals, tc.ExpectIgnored, Commentf("Test case: %s - %s", tc.Name, tc.Reason))
	}
}
