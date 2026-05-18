package xrp

import (
	"encoding/hex"
	"fmt"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"

	"gitlab.com/thorchain/thornode/v3/cmd"
	. "gopkg.in/check.v1"

	"github.com/Peersyst/xrpl-go/xrpl/rpc"
	"github.com/Peersyst/xrpl-go/xrpl/rpc/testutil"
	"github.com/Peersyst/xrpl-go/xrpl/transaction"
)

// -------------------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------------------

type BlockScannerTestSuite struct {
	m      *metrics.Metrics
	bridge thorclient.ThorchainBridge
	keys   *thorclient.Keys
}

var _ = Suite(&BlockScannerTestSuite{})

func (s *BlockScannerTestSuite) SetUpSuite(c *C) {
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
	_, _, err := kb.NewMnemonic(cfg.SignerName, cKeys.English, cmd.THORChainHDPath, cfg.SignerPasswd, hd.Secp256k1)
	c.Assert(err, IsNil)
	thorKeys := thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	c.Assert(err, IsNil)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, thorKeys)
	c.Assert(err, IsNil)
	s.keys = thorKeys
}

func (s *BlockScannerTestSuite) TestCalculateAverageFees(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain}
	blockScanner := XrpBlockScanner{cfg: cfg}

	blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(1_000))) // 10 drops
	c.Check(len(blockScanner.feeCache), Equals, 1)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(10))) // 10 drops

	blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(1_000))) // 10 drops
	c.Check(len(blockScanner.feeCache), Equals, 2)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(10))) // 10 drops

	// two txs at double fee should average to 75% of last
	blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(2_000))) // 20 drops
	blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(2_000))) // 20 drops
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15))) // 15 drops

	// skip transactions with zero fee
	blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(0)))
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15))) // 15 drops

	// ensure we only cache the transaction limit number of blocks
	for i := 0; i < FeeCacheTransactions; i++ {
		blockScanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(1_200))) // 12 drops
	}
	c.Check(len(blockScanner.feeCache), Equals, FeeCacheTransactions)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(12))) // 12 drops
}

func (s *BlockScannerTestSuite) TestProcessTxs(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain}

	blockScanner := XrpBlockScanner{
		cfg:    cfg,
		logger: log.Logger.With().Str("module", "blockscanner").Str("chain", common.XRPChain.String()).Logger(),
	}

	// 1. Decisively below threshold: 0.5 XRP (500,000 drops)
	txBelow := map[string]any{
		"tx_json": map[string]any{
			"Account":         "rs3xN42EFLE23gUDG2Rw4rwxhR9MnjwZKQ",
			"Destination":     "rELnd6Ae5ZYDhHkaqjSVg2vgtBnzjeDshm",
			"Fee":             "20",
			"SigningPubKey":   "03dd7eb4b0479d2898cabdd4ccfd30f2277e57cb9c13ccabb8ec5170ed13dd149a",
			"TransactionType": "Payment",
		},
		"hash": "HASH_BELOW",
		"meta": map[string]any{
			"TransactionResult": "tesSUCCESS",
			"delivered_amount":  "500000", // 0.5 XRP
		},
		"validated": true,
	}

	// 2. One drop below threshold: 0.999999 XRP (999,999 drops)
	txOneDropBelow := map[string]any{
		"tx_json": map[string]any{
			"Account":         "rs3xN42EFLE23gUDG2Rw4rwxhR9MnjwZKQ",
			"Destination":     "rELnd6Ae5ZYDhHkaqjSVg2vgtBnzjeDshm",
			"Fee":             "20",
			"SigningPubKey":   "03dd7eb4b0479d2898cabdd4ccfd30f2277e57cb9c13ccabb8ec5170ed13dd149a",
			"TransactionType": "Payment",
		},
		"hash": "HASH_ONE_DROP_BELOW",
		"meta": map[string]any{
			"TransactionResult": "tesSUCCESS",
			"delivered_amount":  "999999", // 0.999999 XRP
		},
		"validated": true,
	}

	// 3. Exact threshold: 1.0 XRP (1,000,000 drops)
	txExact := map[string]any{
		"tx_json": map[string]any{
			"Account":     "rs3xN42EFLE23gUDG2Rw4rwxhR9MnjwZKQ",
			"Destination": "rELnd6Ae5ZYDhHkaqjSVg2vgtBnzjeDshm",
			"Fee":         "20",
			"Memos": []any{
				map[string]any{
					"Memo": map[string]any{
						"MemoData": hex.EncodeToString([]byte("hello")),
					},
				},
			},
			"SigningPubKey":   "03dd7eb4b0479d2898cabdd4ccfd30f2277e57cb9c13ccabb8ec5170ed13dd149a",
			"TransactionType": "Payment",
		},
		"hash": "HASH_EXACT",
		"meta": map[string]any{
			"TransactionResult": "tesSUCCESS",
			"delivered_amount":  "1000000", // 1.0 XRP
		},
		"validated": true,
	}

	txInItems, err := blockScanner.processTxs(1, []transaction.FlatTransaction{txBelow, txOneDropBelow, txExact})
	c.Assert(err, IsNil)

	// processTxs should filter out the first two transactions (below threshold)
	// and only return the third one (exact threshold)
	c.Assert(len(txInItems), Equals, 1)
	c.Assert(txInItems[0].Tx, Equals, "HASH_EXACT")
	c.Assert(txInItems[0].Memo, Equals, "hello")
}

// Simulates requesting txs by hash
// Number of expanded transactions required == 2, number of expanded transactions provided == 1
// Mock response will return the second transaction
func (s *BlockScannerTestSuite) TestFetchTxsByHash(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain}

	blockScanner := XrpBlockScanner{
		cfg:    cfg,
		logger: log.Logger.With().Str("module", "blockscanner").Str("chain", common.XRPChain.String()).Logger(),
	}

	response := `{
		"result": {
			"hash":"A1AD0D91A89B6FEE60DDE9345B0283997B94DD59718F9530A93C502876416F7E",
			"meta":{
				"AffectedNodes":[
					{
						"ModifiedNode":{
							"FinalFields":{
								"Account":"rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
								"Balance":"99999997999999990",
								"Flags":0,
								"OwnerCount":0,
								"Sequence":2
							},
							"LedgerEntryType":"AccountRoot",
							"LedgerIndex":"2B6AC232AA4C4BE41BF49D2459FA4A0347E1B543A4C92FCEE0821C0201E2E9A8",
							"PreviousFields":{
								"Balance":"100000000000000000",
								"Sequence":1
							}
						}
					},
					{
						"CreatedNode":{
							"LedgerEntryType":"AccountRoot",
							"LedgerIndex":"47208AAE3FD66F51AB18CFFF71CD4DF36E6E6E4BA7A106D7F21A86A556C9D57E",
							"NewFields":{
								"Account":"rLt2RdcMjZ9tUfzV5JqgEWHdeP9mWJYZv4",
								"Balance":"2000000000",
								"Sequence":1
							}
						}
					}
				],
				"TransactionIndex":0,
				"TransactionResult":"tesSUCCESS",
				"delivered_amount":"2000000000"
			},
			"tx_json":{
				"Account":"rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh",
				"DeliverMax":"2000000000",
				"Destination":"rLt2RdcMjZ9tUfzV5JqgEWHdeP9mWJYZv4",
				"Fee":"10",
				"LastLedgerSequence":41,
				"NetworkID":1234,
				"Sequence":1,
				"SigningPubKey":"0330E7FC9D56BB25D6893BA3F317AE5BCF33B3291BD63DB32654A313222F7FD020",
				"TransactionType":"Payment",
				"TxnSignature":"3045022100B251F7110899526DCEE938B8BF39A9E105C662BC0848E890520524FFE763ED0D022060E5D7D837FF1C7153AB55551A504FDB8146BCF6E45A4AF1028B79A89E7D8635",
				"date":791478842,
				"ledger_index":22
			},
			"validated":true
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

	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	blockScanner.rpcClient = rpc.NewClient(rpcCfg)

	txHashes := []string{
		"5541C443A1D4BD4E1BE0550C24B440FCC3B89CE6ECE08433CF7A2573E4B6D800",
		"55EEDF4516D16ACEC210E5BFDAF25E330E07A19D3A8F61F30E94BF522778D431",
	}
	txsFromLedger := []transaction.FlatTransaction{
		{
			"hash":      "5541C443A1D4BD4E1BE0550C24B440FCC3B89CE6ECE08433CF7A2573E4B6D800",
			"meta_blob": "201C00000063F8E5110061250000011155F028573AE0CD59767665FEE6283F34AFD95AEF9FACF537B16BA89458356219E2567C6A1ADA196308FB97B8975DC0A4F8517CF9963361FADF1E22AC839D8CCF34CEE66240000000057BCED8E1E7220000000024000000052D000000006240000000059A5358811478BAB4EAB9BEA87099E7382683A82C9409BD1E5AE1E1E5110061250000011155FBFDCCE43AB01A1657939FB97D4B6D12E040347541BB45FBDF92D426D50CC76656BD775007FE9D9A22B97570BA7D57FD0A052ECBE6FF7ABE810EC1F09886E16C3CE624000000046240000000059A5362E1E7220000000024000000052D000000006240000000057BCED88114776FDCAB500131BB6C6D91B20B8D779DA5CA3F21E1E1F1031000",
			"tx_blob":   "12000021000004D22400000004201B000001246140000000001E848068400000000000000A7321ED258BBCB9D302EF46F3CB4C11D384FC6F147A929535A35F9CDF0E4F6829DC28317440FD2B05C8A63D87946B01A94B3B7C9E754B1BE43FA2BA093248AAD07590CBA2DD857C0B7DE269FB4018FF59BC8046EBD91880E15FB9EC4B79408245D4AA639A0E8114776FDCAB500131BB6C6D91B20B8D779DA5CA3F21831478BAB4EAB9BEA87099E7382683A82C9409BD1E5A",
		},
	}
	finalTxs, err := blockScanner.fetchTxsByHash(txHashes, txsFromLedger)
	c.Assert(err, IsNil)

	fmt.Println("len of final txs:", len(finalTxs))

	txInItems, err := blockScanner.processTxs(1, finalTxs)
	c.Assert(err, IsNil)

	// processTxs should filter out everything besides the valid Payments
	c.Assert(len(txInItems), Equals, 2)
}
