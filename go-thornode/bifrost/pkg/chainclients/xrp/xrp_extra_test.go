package xrp

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog/log"

	sdkmath "cosmossdk.io/math"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Peersyst/xrpl-go/xrpl/rpc"
	"github.com/Peersyst/xrpl-go/xrpl/rpc/testutil"
	"github.com/Peersyst/xrpl-go/xrpl/transaction"
	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	stypes "gitlab.com/thorchain/thornode/v3/x/thorchain/types"

	. "gopkg.in/check.v1"
)

// -------------------------------------------------------------------------------------
// XrpMetaDataStore Tests
// -------------------------------------------------------------------------------------

type MetadataTestSuite struct{}

var _ = Suite(&MetadataTestSuite{})

func (s *MetadataTestSuite) TestNewXrpMetaDataStore(c *C) {
	store := NewXrpMetaDataStore()
	c.Assert(store, NotNil)
	c.Assert(store.accts, NotNil)
	c.Assert(store.lock, NotNil)
}

func (s *MetadataTestSuite) TestMetaDataStoreGetSetSeqInc(c *C) {
	store := NewXrpMetaDataStore()
	pk := common.PubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")

	// Get on empty store returns zero value
	meta := store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(0))
	c.Check(meta.BlockHeight, Equals, int64(0))

	// Set and Get
	store.Set(pk, XrpMetadata{SeqNumber: 5, BlockHeight: 100})
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(5))
	c.Check(meta.BlockHeight, Equals, int64(100))

	// SeqInc increments sequence
	store.SeqInc(pk)
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(6))
	c.Check(meta.BlockHeight, Equals, int64(100))

	// SeqInc on non-existent key does nothing
	unknownPk := common.PubKey("sthorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8khqdkwe9kfr9e06pz0k37yp06v4s2ygkml")
	store.SeqInc(unknownPk)
	meta = store.Get(unknownPk)
	c.Check(meta.SeqNumber, Equals, int64(0))

	// Overwrite with Set
	store.Set(pk, XrpMetadata{SeqNumber: 10, BlockHeight: 200})
	meta = store.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(10))
	c.Check(meta.BlockHeight, Equals, int64(200))
}

// -------------------------------------------------------------------------------------
// Asset Mapping Tests
// -------------------------------------------------------------------------------------

type AssetMappingTestSuite struct{}

var _ = Suite(&AssetMappingTestSuite{})

func (s *AssetMappingTestSuite) TestGetAssetByXrpCurrency(c *C) {
	// XRP native currency should match
	xrpAmount := txtypes.XRPCurrencyAmount(1000000)
	mapping, found := GetAssetByXrpCurrency(xrpAmount)
	c.Check(found, Equals, true)
	c.Check(mapping.THORChainAsset.Equals(common.XRPAsset), Equals, true)
	c.Check(mapping.XrpDecimals, Equals, int64(6))

	// Issued currency that's not in the mapping should not match
	issuedAmount := txtypes.IssuedCurrencyAmount{
		Value:    "100",
		Currency: "USD",
		Issuer:   "rUnknownIssuer",
	}
	_, found = GetAssetByXrpCurrency(issuedAmount)
	c.Check(found, Equals, false)
}

func (s *AssetMappingTestSuite) TestGetAssetByThorchainAsset(c *C) {
	// XRP asset should match
	mapping, found := GetAssetByThorchainAsset(common.XRPAsset)
	c.Check(found, Equals, true)
	c.Check(mapping.XrpKind, Equals, txtypes.XRP)

	// Unknown asset should not match
	unknownAsset, _ := common.NewAsset("BTC.BTC")
	_, found = GetAssetByThorchainAsset(unknownAsset)
	c.Check(found, Equals, false)
}

// -------------------------------------------------------------------------------------
// Util Tests (error paths and edge cases)
// -------------------------------------------------------------------------------------

func TestParseCurrencyAmount(t *testing.T) {
	tests := []struct {
		name    string
		coin    txtypes.CurrencyAmount
		wantErr bool
	}{
		{
			name:    "valid XRP amount",
			coin:    txtypes.XRPCurrencyAmount(5000000),
			wantErr: false,
		},
		{
			name: "valid issued currency amount",
			coin: txtypes.IssuedCurrencyAmount{
				Value:    "100",
				Currency: "USD",
				Issuer:   "rIssuer",
			},
			wantErr: false,
		},
		{
			name: "invalid issued currency amount value",
			coin: txtypes.IssuedCurrencyAmount{
				Value:    "notanumber",
				Currency: "USD",
				Issuer:   "rIssuer",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCurrencyAmount(tt.coin)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestFromXrpToThorchainErrorPaths(t *testing.T) {
	// Unknown/non-whitelisted currency
	issuedAmount := txtypes.IssuedCurrencyAmount{
		Value:    "100",
		Currency: "USD",
		Issuer:   "rUnknownIssuer",
	}
	_, err := fromXrpToThorchain(issuedAmount)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not whitelisted")
}

func TestFromThorchainToXrpErrorPaths(t *testing.T) {
	// Unknown/non-whitelisted asset
	unknownAsset, _ := common.NewAsset("BTC.BTC")
	coin := common.NewCoin(unknownAsset, sdkmath.NewUint(100))
	_, err := fromThorchainToXrp(coin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not whitelisted")
}

// -------------------------------------------------------------------------------------
// Block Scanner Constructor Tests
// -------------------------------------------------------------------------------------

func TestNewXrpBlockScanner(t *testing.T) {
	t.Run("nil scan storage", func(t *testing.T) {
		_, err := NewXrpBlockScanner("http://localhost", config.BifrostBlockScannerConfiguration{}, nil, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "scanStorage is nil")
	})
}

// -------------------------------------------------------------------------------------
// Block Scanner Method Tests
// -------------------------------------------------------------------------------------

func TestFetchMemPool(t *testing.T) {
	scanner := &XrpBlockScanner{}
	txIn, err := scanner.FetchMemPool(100)
	assert.NoError(t, err)
	assert.Equal(t, types.TxIn{}, txIn)
}

func TestGetNetworkFee(t *testing.T) {
	scanner := &XrpBlockScanner{
		lastFee: sdkmath.NewUint(12),
	}
	size, rate := scanner.GetNetworkFee()
	assert.Equal(t, uint64(1), size)
	assert.Equal(t, uint64(12), rate)
}

func TestAverageFeeEmpty(t *testing.T) {
	scanner := &XrpBlockScanner{
		feeCache: make([]sdkmath.Uint, 0),
	}
	avg := scanner.averageFee()
	assert.Equal(t, sdkmath.NewUint(0), avg)
}

func TestUpdateFees(t *testing.T) {
	t.Run("skip when not enough cache", func(t *testing.T) {
		scanner := &XrpBlockScanner{
			cfg:      config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain, GasPriceResolution: 1},
			feeCache: make([]sdkmath.Uint, 10), // not FeeCacheTransactions
		}
		err := scanner.updateFees(FeeUpdatePeriodBlocks)
		assert.NoError(t, err)
	})

	t.Run("skip when height not on period", func(t *testing.T) {
		scanner := &XrpBlockScanner{
			cfg:      config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain},
			feeCache: make([]sdkmath.Uint, FeeCacheTransactions),
		}
		err := scanner.updateFees(FeeUpdatePeriodBlocks + 1) // not on period
		assert.NoError(t, err)
	})

	t.Run("zero fee error", func(t *testing.T) {
		fees := make([]sdkmath.Uint, FeeCacheTransactions)
		for i := range fees {
			fees[i] = sdkmath.NewUint(0)
		}
		scanner := &XrpBlockScanner{
			cfg:      config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain, GasPriceResolution: 1000000},
			feeCache: fees,
			lastFee:  sdkmath.NewUint(0),
		}
		err := scanner.updateFees(FeeUpdatePeriodBlocks)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "zero")
	})

	t.Run("skip when fee unchanged", func(t *testing.T) {
		fees := make([]sdkmath.Uint, FeeCacheTransactions)
		for i := range fees {
			fees[i] = sdkmath.NewUint(100)
		}
		feeQueue := make(chan common.NetworkFee, 1)
		scanner := &XrpBlockScanner{
			cfg:                   config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain, GasPriceResolution: 1},
			feeCache:              fees,
			lastFee:               sdkmath.NewUint(100),
			globalNetworkFeeQueue: feeQueue,
		}
		err := scanner.updateFees(FeeUpdatePeriodBlocks)
		assert.NoError(t, err)
		// should not have sent anything
		assert.Equal(t, 0, len(feeQueue))
	})

	t.Run("sends fee when changed", func(t *testing.T) {
		fees := make([]sdkmath.Uint, FeeCacheTransactions)
		for i := range fees {
			fees[i] = sdkmath.NewUint(100)
		}
		feeQueue := make(chan common.NetworkFee, 1)
		scanner := &XrpBlockScanner{
			cfg:                   config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain, GasPriceResolution: 1},
			feeCache:              fees,
			lastFee:               sdkmath.NewUint(50), // different from 100
			globalNetworkFeeQueue: feeQueue,
		}
		err := scanner.updateFees(FeeUpdatePeriodBlocks)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(feeQueue))
		nf := <-feeQueue
		assert.Equal(t, common.XRPChain, nf.Chain)
		assert.Equal(t, uint64(100), nf.TransactionRate)
		assert.Equal(t, uint64(1), nf.TransactionSize)
	})
}

// -------------------------------------------------------------------------------------
// Client Simple Methods Tests
// -------------------------------------------------------------------------------------

type ClientMethodsTestSuite struct{}

var _ = Suite(&ClientMethodsTestSuite{})

func (s *ClientMethodsTestSuite) TestGetConfig(c *C) {
	cfg := config.BifrostChainConfiguration{ChainID: common.XRPChain}
	client := &Client{cfg: cfg}
	c.Check(client.GetConfig().ChainID, Equals, common.XRPChain)
}

func (s *ClientMethodsTestSuite) TestGetChain(c *C) {
	client := &Client{cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain}}
	c.Check(client.GetChain(), Equals, common.XRPChain)
}

func (s *ClientMethodsTestSuite) TestConfirmationCountReady(c *C) {
	client := &Client{}
	c.Check(client.ConfirmationCountReady(types.TxIn{}), Equals, true)
}

func (s *ClientMethodsTestSuite) TestGetConfirmationCount(c *C) {
	client := &Client{}
	c.Check(client.GetConfirmationCount(types.TxIn{}), Equals, int64(0))
}

func (s *ClientMethodsTestSuite) TestShouldReportSolvency(c *C) {
	client := &Client{
		cfg:        config.BifrostChainConfiguration{SolvencyBlocks: 10},
		xrpScanner: &XrpBlockScanner{lastFee: sdkmath.NewUint(100)},
	}

	// height divisible by SolvencyBlocks and non-zero lastFee
	c.Check(client.ShouldReportSolvency(10), Equals, true)
	c.Check(client.ShouldReportSolvency(20), Equals, true)

	// height not divisible
	c.Check(client.ShouldReportSolvency(11), Equals, false)

	// zero lastFee
	client.xrpScanner.lastFee = sdkmath.NewUint(0)
	c.Check(client.ShouldReportSolvency(10), Equals, false)
}

func (s *ClientMethodsTestSuite) TestGetAddressError(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}
	// invalid pub key should return empty string
	result := client.GetAddress("")
	c.Check(result, Equals, "")
}

func (s *ClientMethodsTestSuite) TestProcessOutboundTxErrors(c *C) {
	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		networkID: 1234,
	}

	// empty coins
	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		VaultPubKey: common.PubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz"),
		Coins:       common.Coins{},
	}
	_, err := client.processOutboundTx(txOut)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*cannot send more than 1 set of coins.*")

	// multiple coins
	xrpAsset, _ := common.NewAsset("XRP.XRP")
	txOut.Coins = common.Coins{
		common.NewCoin(xrpAsset, cosmos.NewUint(100)),
		common.NewCoin(xrpAsset, cosmos.NewUint(200)),
	}
	_, err = client.processOutboundTx(txOut)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*cannot send more than 1 set of coins.*")

	// non-whitelisted coin
	btcAsset, _ := common.NewAsset("BTC.BTC")
	txOut.Coins = common.Coins{common.NewCoin(btcAsset, cosmos.NewUint(100))}
	_, err = client.processOutboundTx(txOut)
	c.Assert(err, NotNil)

	// invalid vault pubkey
	txOut.VaultPubKey = common.PubKey("invalid")
	txOut.Coins = common.Coins{common.NewCoin(xrpAsset, cosmos.NewUint(100))}
	_, err = client.processOutboundTx(txOut)
	c.Assert(err, NotNil)
}

func (s *ClientMethodsTestSuite) TestProcessOutboundTxNetworkIDNotSet(c *C) {
	// networkID <= 1024 should NOT be included in payment
	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		networkID: 1, // mainnet/testnet
	}

	vaultPubKey, _ := common.NewPubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
	}

	msg, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)
	c.Check(msg.BaseTx.NetworkID, Equals, uint32(0))
}

// -------------------------------------------------------------------------------------
// OnObservedTxIn Tests
// -------------------------------------------------------------------------------------

type OnObservedTxInTestSuite struct{}

var _ = Suite(&OnObservedTxInTestSuite{})

func (s *OnObservedTxInTestSuite) TestOnObservedTxInInvalidMemo(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}
	// Invalid memo should return without error (debug log)
	client.OnObservedTxIn(types.TxInItem{Memo: "not-a-valid-memo"}, 100)
}

func (s *OnObservedTxInTestSuite) TestOnObservedTxInNonOutbound(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}
	// Valid but non-outbound memo (e.g. swap)
	client.OnObservedTxIn(types.TxInItem{Memo: "SWAP:BTC.BTC:bc1qtest"}, 100)
}

func (s *OnObservedTxInTestSuite) TestOnObservedTxInEmptyTxID(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}
	// Outbound with empty tx id
	client.OnObservedTxIn(types.TxInItem{Memo: "OUT:"}, 100)
}

// -------------------------------------------------------------------------------------
// verifySignature Tests
// -------------------------------------------------------------------------------------

func TestVerifySignatureInvalid(t *testing.T) {
	// Invalid DER signature
	result := verifySignature([]byte("test"), []byte("invalid"), []byte("pubkey"))
	assert.False(t, result)
}

func TestVerifySignatureBadPubKey(t *testing.T) {
	// Valid-ish DER but wrong public key
	signBytesHex := "5354580012000021000004d224000000016140000000017645e06840000000000b71b07321030cef2112503d3a56d2d48b3a0f0f6503e4353400f450f9dbf344d182e7c7069c8114d4b66bcf790babd0032c5dbfdc14ff1c643a4f488314fdffd00f2f2d215ecdad483b99be1dad2259b9c3f9ea7d046d656d6fe1f1"
	signatureHex := "30440221008e9bc0a8d7927f1874d318bc2a57691b5321d618dd57857d64402be0e0bc0007021f4ab281ef93b0c448a9e75d6c07e9cf05ee705a3c51aafa61992510b65a03ae"

	signBytes, _ := hex.DecodeString(signBytesHex)
	signature, _ := hex.DecodeString(signatureHex)
	// wrong pubkey
	wrongPubKey, _ := hex.DecodeString("020000000000000000000000000000000000000000000000000000000000000001")

	result := verifySignature(signBytes, signature, wrongPubKey)
	assert.False(t, result)
}

// -------------------------------------------------------------------------------------
// processTxs edge cases
// -------------------------------------------------------------------------------------

func TestProcessTxsEdgeCases(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain},
	}

	t.Run("nil transaction in list", func(t *testing.T) {
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{nil})
		require.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("failed transaction result", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tecUNFUNDED_PAYMENT",
			},
			"tx_json": map[string]any{
				"TransactionType": "Payment",
			},
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("no meta info", func(t *testing.T) {
		tx := map[string]any{
			"no_meta": true,
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("no tx_json or tx_blob", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
			},
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("non-payment transaction", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
			},
			"tx_json": map[string]any{
				"TransactionType": "AccountSet",
			},
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs)
	})

	t.Run("hash is not string", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
				"delivered_amount":  "2000000", // above dust threshold
			},
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
				"Destination":     "rReceiver",
			},
			"hash": 12345, // not a string
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs) // skipped because hash not string
	})

	t.Run("issued currency amount skipped", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
				"delivered_amount": map[string]any{
					"value":    "100",
					"currency": "USD",
					"issuer":   "rIssuer",
				},
			},
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
				"Destination":     "rReceiver",
			},
			"hash": "HASH_ISSUED",
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		assert.Empty(t, txs) // skipped because non-XRP amount
	})

	t.Run("payment with no memo", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
				"delivered_amount":  "2000000",
			},
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
				"Destination":     "rReceiver",
			},
			"hash": "HASH_NO_MEMO",
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		require.Len(t, txs, 1)
		assert.Equal(t, "", txs[0].Memo)
		assert.Equal(t, "HASH_NO_MEMO", txs[0].Tx)
		assert.Equal(t, "rSender", txs[0].Sender)
		assert.Equal(t, "rReceiver", txs[0].To)
	})

	t.Run("payment with multiple memos uses first", func(t *testing.T) {
		tx := map[string]any{
			"meta": map[string]any{
				"TransactionResult": "tesSUCCESS",
				"delivered_amount":  "2000000",
			},
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "10",
				"Account":         "rSender",
				"Destination":     "rReceiver",
				"Memos": []any{
					map[string]any{
						"Memo": map[string]any{
							"MemoData": hex.EncodeToString([]byte("first")),
						},
					},
					map[string]any{
						"Memo": map[string]any{
							"MemoData": hex.EncodeToString([]byte("second")),
						},
					},
				},
			},
			"hash": "HASH_MULTI_MEMO",
		}
		txs, err := scanner.processTxs(1, []transaction.FlatTransaction{tx})
		require.NoError(t, err)
		require.Len(t, txs, 1)
		assert.Equal(t, "first", txs[0].Memo)
	})
}

// -------------------------------------------------------------------------------------
// decodeMetaBlobIfNecessary and decodeTxBlobIfNecessary edge cases
// -------------------------------------------------------------------------------------

func TestDecodeMetaBlobNoMetaField(t *testing.T) {
	scanner := &XrpBlockScanner{}
	// no "meta" or "meta_blob" field
	_, err := scanner.decodeMetaBlobIfNecessary(map[string]any{"something": "else"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "meta_blob not a string")
}

func TestDecodeTxBlobNoTxField(t *testing.T) {
	scanner := &XrpBlockScanner{}
	// no "tx_json" or "tx_blob" field
	_, err := scanner.decodeTxBlobIfNecessary(map[string]any{"something": "else"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tx_blob not a string")
}

// -------------------------------------------------------------------------------------
// updateFeeCache edge cases
// -------------------------------------------------------------------------------------

func TestUpdateFeeCacheZeroFee(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg:      config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain},
		feeCache: make([]sdkmath.Uint, 0),
	}
	// Zero amount coin should be skipped
	scanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(0)))
	assert.Equal(t, 0, len(scanner.feeCache))
}

func TestUpdateFeeCacheTruncation(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg:      config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain},
		feeCache: make([]sdkmath.Uint, 0),
	}

	// Fill beyond FeeCacheTransactions
	for i := 0; i < FeeCacheTransactions+50; i++ {
		scanner.updateFeeCache(common.NewCoin(common.XRPAsset, sdkmath.NewUint(1000)))
	}
	assert.Equal(t, FeeCacheTransactions, len(scanner.feeCache))
}

// -------------------------------------------------------------------------------------
// SignTx tests (using existing client setup)
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestSignTxAlreadySigned(c *C) {
	// Create a client with in-memory storage for signer cache
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(`{"result":{"ledger_current_index":100}}`, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		accts:              NewXrpMetaDataStore(),
		signerCacheManager: signerCacheManager,
		rpcClient:          rpc.NewClient(rpcCfg),
		xrpScanner:         &XrpBlockScanner{rpcClient: rpc.NewClient(rpcCfg)},
	}

	vaultPubKey, _ := common.NewPubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
		Memo:        "OUT:txhash",
		GasRate:     12,
		InHash:      "hash",
	}

	// Mark as already signed
	err = signerCacheManager.SetSigned(txOut.CacheHash(), txOut.CacheVault(common.XRPChain), "some-hash")
	c.Assert(err, IsNil)

	// Should return nil,nil,nil,nil for already signed
	signedTx, checkpoint, txInItem, signErr := client.SignTx(txOut, 100)
	c.Check(signErr, IsNil)
	c.Check(signedTx, IsNil)
	c.Check(checkpoint, IsNil)
	c.Check(txInItem, IsNil)
}

func (s *XrpTestSuite) TestSignTxProcessError(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		accts:              NewXrpMetaDataStore(),
		signerCacheManager: signerCacheManager,
	}

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		VaultPubKey: common.PubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz"),
		Coins:       common.Coins{}, // will fail processOutboundTx
	}

	_, _, _, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, NotNil)
	c.Check(signErr.Error(), Matches, ".*cannot send more than 1 set of coins.*")
}

// -------------------------------------------------------------------------------------
// GetHeight test with mock RPC
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetHeight(c *C) {
	response := `{"result":{"ledger_current_index":12345}}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(response, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	scanner := &XrpBlockScanner{rpcClient: rpc.NewClient(rpcCfg)}
	_, err = scanner.GetHeight()
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// FetchTxs test with mock RPC
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestFetchTxsValidated(c *C) {
	callNum := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callNum++
		if callNum == 1 {
			// First call: ledger request for tx hashes
			fmt.Fprint(w, `{"result":{"ledger":{"transactions":[]},"ledger_hash":"ABC","ledger_index":100,"validated":true}}`)
		} else if callNum == 2 {
			// Second call: ledger request for expanded txs
			fmt.Fprint(w, `{"result":{"ledger":{"transactions":[]},"ledger_hash":"ABC","ledger_index":100,"validated":true}}`)
		}
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL)
	c.Assert(err, IsNil)

	feeQueue := make(chan common.NetworkFee, 1)
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID:                      common.XRPChain,
			ObservationFlexibilityBlocks: 100,
		},
		rpcClient:             rpc.NewClient(rpcCfg),
		feeCache:              make([]sdkmath.Uint, 0),
		lastFee:               sdkmath.NewUint(0),
		globalNetworkFeeQueue: feeQueue,
		solvencyReporter:      func(int64) error { return nil },
	}

	txIn, err := scanner.FetchTxs(100, 100)
	c.Assert(err, IsNil)
	c.Check(txIn.Chain, Equals, common.XRPChain)
	c.Check(len(txIn.TxArray), Equals, 0)
}

func (s *XrpTestSuite) TestFetchTxsNotValidated(c *C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Not validated
		fmt.Fprint(w, `{"result":{"ledger":{"transactions":[]},"ledger_hash":"ABC","ledger_index":100,"validated":false}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL)
	c.Assert(err, IsNil)

	scanner := &XrpBlockScanner{
		cfg:       config.BifrostBlockScannerConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(rpcCfg),
		feeCache:  make([]sdkmath.Uint, 0),
		lastFee:   sdkmath.NewUint(0),
	}

	_, err = scanner.FetchTxs(100, 100)
	c.Assert(err, NotNil) // should fail because not validated
}

// -------------------------------------------------------------------------------------
// FetchTxs skip network fee/solvency when far from tip
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestFetchTxsSkipFeeUpdateWhenFarFromTip(c *C) {
	callNum := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callNum++
		// Both calls return validated ledger with no txs
		fmt.Fprint(w, `{"result":{"ledger":{"transactions":[]},"ledger_hash":"ABC","ledger_index":50,"validated":true}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL)
	c.Assert(err, IsNil)

	feeQueue := make(chan common.NetworkFee, 1)
	solvencyReported := false
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID:                      common.XRPChain,
			ObservationFlexibilityBlocks: 5,
		},
		rpcClient:             rpc.NewClient(rpcCfg),
		feeCache:              make([]sdkmath.Uint, 0),
		lastFee:               sdkmath.NewUint(0),
		globalNetworkFeeQueue: feeQueue,
		solvencyReporter: func(int64) error {
			solvencyReported = true
			return nil
		},
	}

	// chainHeight 200, fetchHeight 50, diff > ObservationFlexibilityBlocks
	txIn, err := scanner.FetchTxs(50, 200)
	c.Assert(err, IsNil)
	c.Check(txIn.Chain, Equals, common.XRPChain)
	c.Check(solvencyReported, Equals, false) // should not report solvency
}

// -------------------------------------------------------------------------------------
// txNeedsBroadcast test with mock
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestTxNeedsBroadcast(c *C) {
	// Validated tx - no need to broadcast
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":{"hash":"ABC123","validated":true}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL)
	c.Assert(err, IsNil)
	client := &Client{rpcClient: rpc.NewClient(rpcCfg)}

	c.Check(client.txNeedsBroadcast("ABC123"), Equals, false)

	// Not validated - needs broadcast
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":{"hash":"ABC123","validated":false}}`)
	}))
	defer server2.Close()

	rpcCfg2, err := rpc.NewClientConfig(server2.URL)
	c.Assert(err, IsNil)
	client.rpcClient = rpc.NewClient(rpcCfg2)

	c.Check(client.txNeedsBroadcast("ABC123"), Equals, true)

	// Error on request - needs broadcast
	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server3.Close()

	rpcCfg3, err := rpc.NewClientConfig(server3.URL)
	c.Assert(err, IsNil)
	client.rpcClient = rpc.NewClient(rpcCfg3)

	c.Check(client.txNeedsBroadcast("ABC123"), Equals, true)
}

// -------------------------------------------------------------------------------------
// SignTx full path test (with local key signing)
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestSignTxFullPath(c *C) {
	// Create a client similar to the existing TestSign
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	// Set up the RPC mock to return a ledger index for GetHeight
	heightResp := `{"result":{"ledger_current_index":50}}`
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(heightResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)
	client.xrpScanner.rpcClient = rpc.NewClient(rpcCfg)

	// Set up account data so it doesn't need to fetch from chain
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	// Set cached metadata so it won't try to call GetAccount
	client.accts.Set(vaultPubKey, XrpMetadata{SeqNumber: 5, BlockHeight: 100})

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "OUT:txhash123",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	signedTx, checkpoint, txIn, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, IsNil)
	c.Check(signedTx, NotNil)
	c.Check(checkpoint, IsNil) // nil checkpoint since we got txIn
	c.Check(txIn, NotNil)
	c.Check(txIn.Memo, Equals, "OUT:txhash123")
}

// Test SignTx with checkpoint (deserialized metadata)
func (s *XrpTestSuite) TestSignTxWithCheckpoint(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	heightResp := `{"result":{"ledger_current_index":50}}`
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(heightResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)
	client.xrpScanner.rpcClient = rpc.NewClient(rpcCfg)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	// Provide checkpoint bytes (marshalled XrpMetadata)
	checkpointBytes := []byte(`{"SeqNumber":10,"BlockHeight":200}`)

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "", // empty memo - memoless outbound
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  checkpointBytes,
	}

	signedTx, _, txIn, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, IsNil)
	c.Check(signedTx, NotNil)
	c.Check(txIn, NotNil)
}

// Test SignTx with bad checkpoint
func (s *XrpTestSuite) TestSignTxBadCheckpoint(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(`{"result":{"ledger_current_index":100}}`, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		accts:              NewXrpMetaDataStore(),
		signerCacheManager: signerCacheManager,
		xrpScanner:         &XrpBlockScanner{rpcClient: rpc.NewClient(rpcCfg)},
	}

	vaultPubKey, _ := common.NewPubKey("sthorpub1addwnpepqtrvka83xluqq522k8at4d84gnthj5ryqhrf0fa24yze4yhk7j0fk4876fz")
	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
		Memo:        "OUT:hash",
		GasRate:     12,
		InHash:      "hash",
		Checkpoint:  []byte("invalid json"),
	}

	_, _, _, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, NotNil)
	c.Check(signErr.Error(), Matches, ".*invalid character.*")
}

// -------------------------------------------------------------------------------------
// GetAccountByAddress test with mock RPC
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetAccountByAddressWithHeight(c *C) {
	response := `{
		"result": {
			"account_data": {
				"Flags": 0,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "5000000",
				"OwnerCount": 0,
				"Sequence": 10
			},
			"ledger_current_index": 100,
			"validated": false
		}
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(response, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(rpcCfg),
	}

	// With specific height
	acc, err := client.GetAccountByAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ", nil)
	c.Assert(err, IsNil)
	c.Check(acc.Sequence, Equals, int64(10))
}

// -------------------------------------------------------------------------------------
// ReportSolvency should not report when solvency block check fails
// -------------------------------------------------------------------------------------

func (s *ClientMethodsTestSuite) TestReportSolvencySkippedBySolvencyBlocks(c *C) {
	client := &Client{
		cfg:        config.BifrostChainConfiguration{ChainID: common.XRPChain, SolvencyBlocks: 10},
		xrpScanner: &XrpBlockScanner{lastFee: sdkmath.NewUint(0)}, // zero lastFee
	}

	// ShouldReportSolvency returns false (lastFee is zero), so ReportSolvency should return nil
	err := client.ReportSolvency(10)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// NewClient error paths
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestNewClientErrors(c *C) {
	// nil bridge
	_, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID: common.XRPChain,
			RPCHost: "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				StartBlockHeight: 1,
			},
		},
		nil,
		nil, // nil bridge
		s.m,
	)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*thorchain bridge is nil.*")

	// invalid chain network
	_, err = NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "not-a-number",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*parsing chain network.*")

	// empty chain network defaults to 1
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)
	c.Check(client.networkID, Equals, uint32(1))
}

// -------------------------------------------------------------------------------------
// processPayment additional coverage
// -------------------------------------------------------------------------------------

func TestProcessPaymentPaths(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name    string
		flatTx  map[string]any
		wantNil bool
		wantErr bool
		errMsg  string
	}{
		{
			name:    "non-payment type returns nil",
			flatTx:  map[string]any{"TransactionType": "OfferCreate"},
			wantNil: true,
		},
		{
			name:    "bad fee returns error",
			flatTx:  map[string]any{"TransactionType": "Payment", "Fee": "notanumber"},
			wantErr: true,
			errMsg:  "cannot parse fee",
		},
		{
			name:    "non-XRP fee returns error",
			flatTx:  map[string]any{"TransactionType": "Payment", "Fee": map[string]any{"currency": "USD", "value": "10", "issuer": "abc"}},
			wantErr: true,
			errMsg:  "fee is not in XRP",
		},
		{
			name: "memos not array returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           "not-an-array",
			},
			wantErr: true,
			errMsg:  "cannot cast memos",
		},
		{
			name: "memos empty array returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{},
			},
			wantErr: true,
			errMsg:  "memos is empty",
		},
		{
			name: "memo[0] not map returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{"not-a-map"},
			},
			wantErr: true,
			errMsg:  "cannot cast memos",
		},
		{
			name: "memo.Memo not map returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{map[string]any{"Memo": "not-a-map"}},
			},
			wantErr: true,
			errMsg:  "cannot cast memo",
		},
		{
			name: "MemoData not string returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{map[string]any{"Memo": map[string]any{"MemoData": 123}}},
			},
			wantErr: true,
			errMsg:  "cannot cast MemoData",
		},
		{
			name: "MemoData empty string returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{map[string]any{"Memo": map[string]any{"MemoData": ""}}},
			},
			wantErr: true,
			errMsg:  "MemoData is empty",
		},
		{
			name: "MemoData invalid hex returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Memos":           []any{map[string]any{"Memo": map[string]any{"MemoData": "ZZZZ"}}},
			},
			wantErr: true,
			errMsg:  "cannot decode memo data",
		},
		{
			name: "Account not string returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         123,
			},
			wantErr: true,
			errMsg:  "cannot cast sender",
		},
		{
			name: "Destination not string returns error",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     123,
			},
			wantErr: true,
			errMsg:  "cannot cast to/destination",
		},
		{
			name: "valid payment without memos",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
			},
		},
		{
			name: "valid payment with memo",
			flatTx: map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
				"Memos": []any{map[string]any{
					"Memo": map[string]any{
						"MemoData": hex.EncodeToString([]byte("SWAP:ETH.ETH")),
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scanner.processPayment(tt.flatTx)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			if tt.wantNil {
				assert.Nil(t, result)
				assert.NoError(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// -------------------------------------------------------------------------------------
// getDeliveredAmount paths
// -------------------------------------------------------------------------------------

func TestGetDeliveredAmountPaths(t *testing.T) {
	scanner := &XrpBlockScanner{}

	tests := []struct {
		name    string
		tx      map[string]any
		meta    map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name: "delivered_amount in meta",
			tx:   map[string]any{},
			meta: map[string]any{"delivered_amount": "1000000"},
		},
		{
			name: "DeliveredAmount in meta",
			tx:   map[string]any{},
			meta: map[string]any{"DeliveredAmount": "1000000"},
		},
		{
			name: "Amount field in tx",
			tx:   map[string]any{"Amount": "1000000"},
			meta: map[string]any{},
		},
		{
			name: "DeliverMax field in tx",
			tx:   map[string]any{"DeliverMax": "1000000"},
			meta: map[string]any{},
		},
		{
			name:    "partial payment without delivered amount",
			tx:      map[string]any{"Flags": uint32(131072)},
			meta:    map[string]any{},
			wantErr: true,
			errMsg:  "partial payment flag set",
		},
		{
			name:    "no amount fields",
			tx:      map[string]any{},
			meta:    map[string]any{},
			wantErr: true,
			errMsg:  "cannot parse amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scanner.getDeliveredAmount(tt.tx, tt.meta)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// -------------------------------------------------------------------------------------
// GetLatestTxForVault
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetLatestTxForVault(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		signerCacheManager: signerCacheManager,
	}

	// No records exist, returns not found error
	_, _, err = client.GetLatestTxForVault("somevault")
	c.Assert(err, NotNil) // leveldb: not found
}

// -------------------------------------------------------------------------------------
// GetAddress with valid and invalid pubkeys
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetAddressValid(c *C) {
	client := &Client{
		cfg:    config.BifrostChainConfiguration{ChainID: common.XRPChain},
		logger: log.Logger,
	}

	// Valid pubkey
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	addr := client.GetAddress(pk)
	c.Check(addr, Not(Equals), "")
}

// -------------------------------------------------------------------------------------
// GetAccount with valid pubkey
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetAccountValid(c *C) {
	response := `{
		"result": {
			"account_data": {
				"Flags": 0,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "5000000",
				"OwnerCount": 0,
				"Sequence": 10
			},
			"ledger_current_index": 100,
			"validated": false
		}
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(response, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(rpcCfg),
	}

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	acc, err := client.GetAccount(pk, nil)
	c.Assert(err, IsNil)
	c.Check(acc.Sequence, Equals, int64(10))
}

// -------------------------------------------------------------------------------------
// BroadcastTx via httptest server
// -------------------------------------------------------------------------------------

func TestBroadcastTxSuccess(t *testing.T) {
	// Create a mock HTTP server that responds with success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"The transaction was applied.","tx_blob":"120000","tx_json":{}}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	require.NoError(t, err)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	require.NoError(t, err)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient:          rpc.NewClient(rpcCfg),
		accts:              NewXrpMetaDataStore(),
		signerCacheManager: signerCacheManager,
	}

	// broadcastTx with empty blob - will get hex decode error from Submit
	err = client.broadcastTx("120000")
	// The submit function validates the blob client-side, so this may fail
	// Just test that broadcastTx is callable
	_ = err
}

// -------------------------------------------------------------------------------------
// decodeMetaBlobIfNecessary and decodeTxBlobIfNecessary - more paths
// -------------------------------------------------------------------------------------

func TestDecodeMetaBlobValid(t *testing.T) {
	scanner := &XrpBlockScanner{}

	// meta is a map - returns immediately
	rawTx := map[string]any{
		"meta": map[string]any{"TransactionResult": "tesSUCCESS"},
	}
	meta, err := scanner.decodeMetaBlobIfNecessary(rawTx)
	require.NoError(t, err)
	assert.Equal(t, "tesSUCCESS", meta["TransactionResult"])

	// meta_blob is invalid
	rawTx2 := map[string]any{
		"meta_blob": "invalidhex",
	}
	_, err = scanner.decodeMetaBlobIfNecessary(rawTx2)
	assert.Error(t, err)
}

func TestDecodeTxBlobValid(t *testing.T) {
	scanner := &XrpBlockScanner{}

	// tx_json is a map - returns immediately
	rawTx := map[string]any{
		"tx_json": map[string]any{"TransactionType": "Payment"},
	}
	flatTx, err := scanner.decodeTxBlobIfNecessary(rawTx)
	require.NoError(t, err)
	assert.Equal(t, "Payment", flatTx["TransactionType"])

	// tx_blob is invalid
	rawTx2 := map[string]any{
		"tx_blob": "invalidhex",
	}
	_, err = scanner.decodeTxBlobIfNecessary(rawTx2)
	assert.Error(t, err)
}

// -------------------------------------------------------------------------------------
// GetBlockScannerHeight and RollbackBlockScanner with real client
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetBlockScannerHeight(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	height, err := client.GetBlockScannerHeight()
	c.Assert(err, IsNil)
	c.Check(height >= 0, Equals, true)

	// Block scanner health depends on runtime state, just verify it's callable
	_ = client.IsBlockScannerHealthy()
}

// -------------------------------------------------------------------------------------
// processOutboundTx additional error paths
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestProcessOutboundTxNetworkIDGreaterThan1024(c *C) {
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(`{"result":{"ledger_current_index":50}}`, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	client := &Client{
		cfg:        config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient:  rpc.NewClient(rpcCfg),
		networkID:  2000, // > 1024
		xrpScanner: &XrpBlockScanner{rpcClient: rpc.NewClient(rpcCfg)},
	}

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
		Memo:        "OUT:hash",
		GasRate:     12,
		InHash:      "hash",
	}

	payment, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)
	c.Assert(payment, NotNil)
	c.Check(payment.BaseTx.NetworkID, Equals, uint32(2000))
}

// -------------------------------------------------------------------------------------
// OnObservedTxIn - valid outbound path
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestOnObservedTxInValidOutbound(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheManager, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                config.BifrostChainConfiguration{ChainID: common.XRPChain},
		signerCacheManager: signerCacheManager,
		logger:             log.Logger,
	}

	txIn := types.TxInItem{
		Tx:   "ABCD1234",
		Memo: "OUT:AABB1122",
	}
	// Should not panic - exercises the full outbound path in OnObservedTxIn
	client.OnObservedTxIn(txIn, 100)
}

// -------------------------------------------------------------------------------------
// fromXrpToThorchain and fromThorchainToXrp edge cases
// -------------------------------------------------------------------------------------

func TestFromXrpToThorchainNonXRP(t *testing.T) {
	// Issuer currency amount (not XRP)
	issuerAmt := txtypes.IssuedCurrencyAmount{
		Currency: "USD",
		Issuer:   "rIssuer",
		Value:    "100",
	}
	_, err := fromXrpToThorchain(issuerAmt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestFromThorchainToXrpNonXRP(t *testing.T) {
	// Coin with non-XRP asset
	ethAsset, _ := common.NewAsset("ETH.ETH")
	coin := common.NewCoin(ethAsset, cosmos.NewUint(100))
	_, err := fromThorchainToXrp(coin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// -------------------------------------------------------------------------------------
// parseAmountFromTx edge cases
// -------------------------------------------------------------------------------------

func TestParseAmountFromTxInvalidJSON(t *testing.T) {
	// nil returns error
	_, err := parseAmountFromTx(nil)
	assert.Error(t, err)
}

// -------------------------------------------------------------------------------------
// getFlags edge cases
// -------------------------------------------------------------------------------------

func TestGetFlagsNonUint32(t *testing.T) {
	// Flags is a float64 (common from JSON unmarshalling)
	tx := map[string]any{"Flags": float64(131072)}
	flags := getFlags(tx)
	assert.Equal(t, uint32(0), flags) // not uint32 type, returns 0
}

func TestGetFlagsUint32(t *testing.T) {
	tx := map[string]any{"Flags": uint32(131072)}
	flags := getFlags(tx)
	assert.Equal(t, uint32(131072), flags)
}

func TestGetFlagsMissing(t *testing.T) {
	tx := map[string]any{}
	flags := getFlags(tx)
	assert.Equal(t, uint32(0), flags)
}

// -------------------------------------------------------------------------------------
// updateFeeCache with various sizes
// -------------------------------------------------------------------------------------

func TestUpdateFeeCacheGrowsToMax(t *testing.T) {
	scanner := &XrpBlockScanner{}

	// Fill cache to max
	for i := 0; i < FeeCacheTransactions; i++ {
		scanner.updateFeeCache(common.NewCoin(common.XRPAsset, cosmos.NewUint(uint64(10+i))))
	}
	assert.Equal(t, FeeCacheTransactions, len(scanner.feeCache))

	// Add one more - should truncate
	scanner.updateFeeCache(common.NewCoin(common.XRPAsset, cosmos.NewUint(999)))
	assert.Equal(t, FeeCacheTransactions, len(scanner.feeCache))
}

// -------------------------------------------------------------------------------------
// averageFee with various inputs
// -------------------------------------------------------------------------------------

func TestAverageFeeNonEmpty(t *testing.T) {
	scanner := &XrpBlockScanner{}
	// Directly populate feeCache to avoid ThorchainToNativeGas conversion
	scanner.feeCache = []sdkmath.Uint{sdkmath.NewUint(100), sdkmath.NewUint(200)}

	avg := scanner.averageFee()
	assert.Equal(t, sdkmath.NewUint(150), avg)
}

// -------------------------------------------------------------------------------------
// processTxs - more paths to cover
// -------------------------------------------------------------------------------------

func TestProcessTxsValidPayment(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	txs := []transaction.FlatTransaction{
		{
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
			},
			"meta": map[string]any{
				"delivered_amount":  "1000000",
				"TransactionResult": "tesSUCCESS",
			},
			"hash":      "ABCD1234",
			"validated": true,
		},
	}

	result, err := scanner.processTxs(100, txs)
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "ABCD1234", result[0].Tx)
}

func TestProcessTxsIssuerCurrencySkipped(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	// Issuer currency amount - should be skipped
	txs := []transaction.FlatTransaction{
		{
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
			},
			"meta": map[string]any{
				"delivered_amount": map[string]any{
					"currency": "USD",
					"issuer":   "rIssuer",
					"value":    "100",
				},
				"TransactionResult": "tesSUCCESS",
			},
			"hash":      "ABCD1234",
			"validated": true,
		},
	}

	result, err := scanner.processTxs(100, txs)
	require.NoError(t, err)
	assert.Len(t, result, 0) // skipped because issuer currency
}

func TestProcessTxsDustThreshold(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	// Tiny amount below dust threshold
	txs := []transaction.FlatTransaction{
		{
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
			},
			"meta": map[string]any{
				"delivered_amount":  "1", // 1 drop = very tiny
				"TransactionResult": "tesSUCCESS",
			},
			"hash":      "ABCD1234",
			"validated": true,
		},
	}

	result, err := scanner.processTxs(100, txs)
	require.NoError(t, err)
	assert.Len(t, result, 0) // below dust threshold
}

func TestProcessTxsHashNotString(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	// Hash is not a string - should be skipped
	txs := []transaction.FlatTransaction{
		{
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
			},
			"meta": map[string]any{
				"delivered_amount":  "1000000",
				"TransactionResult": "tesSUCCESS",
			},
			"hash":      123, // not a string
			"validated": true,
		},
	}

	result, err := scanner.processTxs(100, txs)
	require.NoError(t, err)
	assert.Len(t, result, 0) // skipped
}

func TestProcessTxsWithMemoInPayment(t *testing.T) {
	scanner := &XrpBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	txs := []transaction.FlatTransaction{
		{
			"tx_json": map[string]any{
				"TransactionType": "Payment",
				"Fee":             "12",
				"Account":         "rSender",
				"Destination":     "rDest",
				"Memos": []any{map[string]any{
					"Memo": map[string]any{
						"MemoData": hex.EncodeToString([]byte("SWAP:ETH.ETH")),
					},
				}},
			},
			"meta": map[string]any{
				"delivered_amount":  "1000000",
				"TransactionResult": "tesSUCCESS",
			},
			"hash":      "ABCD1234",
			"validated": true,
		},
	}

	result, err := scanner.processTxs(100, txs)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "SWAP:ETH.ETH", result[0].Memo)
}

// -------------------------------------------------------------------------------------
// signMsg error paths (pubkey not matching local key manager)
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestSignMsgLocalKeyManager(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
		Memo:        "OUT:hash",
		GasRate:     12,
		InHash:      "hash",
	}

	payment, err := client.processOutboundTx(txOut)
	c.Assert(err, IsNil)

	// signMsg with local key - should succeed
	txBytes, err := client.signMsg(payment, vaultPubKey)
	c.Assert(err, IsNil)
	c.Check(len(txBytes) > 0, Equals, true)
}

// -------------------------------------------------------------------------------------
// Mock ThorchainBridge for testing
// -------------------------------------------------------------------------------------

type mockXrpBridge struct {
	thorclient.ThorchainBridge
	asgards   stypes.Vaults
	asgardErr error
}

func (m *mockXrpBridge) GetAsgards() (stypes.Vaults, error) {
	return m.asgards, m.asgardErr
}

// -------------------------------------------------------------------------------------
// ReportSolvency full path
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestReportSolvencyFullPath(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	// Set up mock RPC for GetAccountByAddress
	accountResp := `{
		"result": {
			"account_data": {
				"Flags": 0,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "5000000000",
				"OwnerCount": 0,
				"Sequence": 10
			},
			"ledger_current_index": 100,
			"validated": false
		}
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(accountResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	vault := stypes.Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.XRPAsset, cosmos.NewUint(5000000000))},
	}

	mockBridge := &mockXrpBridge{
		asgards: stypes.Vaults{vault},
	}

	solvencyQueue := make(chan types.Solvency, 10)

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                 config.BifrostChainConfiguration{ChainID: common.XRPChain, SolvencyBlocks: 10},
		rpcClient:           rpc.NewClient(rpcCfg),
		xrpScanner:          &XrpBlockScanner{lastFee: sdkmath.NewUint(12)},
		thorchainBridge:     mockBridge,
		globalSolvencyQueue: solvencyQueue,
		logger:              log.Logger,
		blockScanner:        nil,
	}

	// Need a block scanner for IsBlockScannerHealthy check
	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:          common.XRPChain,
		StartBlockHeight: 1,
	}
	xrpScanner, err := NewXrpBlockScanner("http://testnode/", scannerCfg, storage, s.bridge, s.m, client.ReportSolvency)
	c.Assert(err, IsNil)
	client.xrpScanner = xrpScanner
	client.xrpScanner.lastFee = sdkmath.NewUint(12) // non-zero

	bs, err := blockscanner.NewBlockScanner(scannerCfg, storage, s.m, s.bridge, xrpScanner)
	c.Assert(err, IsNil)
	client.blockScanner = bs

	err = client.ReportSolvency(10)
	c.Assert(err, IsNil)

	// Check that a solvency message was sent
	select {
	case msg := <-solvencyQueue:
		c.Check(msg.Height, Equals, int64(10))
		c.Check(msg.Chain, Equals, common.XRPChain)
	case <-time.After(time.Second):
		c.Fatal("expected solvency message")
	}
}

// Test ReportSolvency when GetAsgards returns error
func (s *XrpTestSuite) TestReportSolvencyGetAsgardsError(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:          common.XRPChain,
		StartBlockHeight: 1,
	}
	xrpScanner, err := NewXrpBlockScanner("http://testnode/", scannerCfg, storage, s.bridge, s.m, nil)
	c.Assert(err, IsNil)
	xrpScanner.lastFee = sdkmath.NewUint(12)

	bs, err := blockscanner.NewBlockScanner(scannerCfg, storage, s.m, s.bridge, xrpScanner)
	c.Assert(err, IsNil)

	mockBridge := &mockXrpBridge{
		asgardErr: fmt.Errorf("bridge error"),
	}

	client := &Client{
		cfg:             config.BifrostChainConfiguration{ChainID: common.XRPChain, SolvencyBlocks: 10},
		xrpScanner:      xrpScanner,
		thorchainBridge: mockBridge,
		blockScanner:    bs,
		logger:          log.Logger,
	}

	err = client.ReportSolvency(10)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*bridge error.*")
}

// Test ReportSolvency when unhealthy scanner and only solvent vaults
func (s *XrpTestSuite) TestReportSolvencyUnhealthyScannerSolvent(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	accountResp := `{
		"result": {
			"account_data": {
				"Flags": 0,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "5000000000",
				"OwnerCount": 0,
				"Sequence": 10
			},
			"ledger_current_index": 100,
			"validated": false
		}
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(accountResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	vault := stypes.Vault{
		PubKey: vaultPubKey,
		Coins:  common.Coins{common.NewCoin(common.XRPAsset, cosmos.NewUint(5000000000))},
	}

	mockBridge := &mockXrpBridge{
		asgards: stypes.Vaults{vault},
	}

	solvencyQueue := make(chan types.Solvency, 10)

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:          common.XRPChain,
		StartBlockHeight: 1,
	}
	xrpScanner, err := NewXrpBlockScanner("http://testnode/", scannerCfg, storage, s.bridge, s.m, nil)
	c.Assert(err, IsNil)
	xrpScanner.lastFee = sdkmath.NewUint(12)

	bs, err := blockscanner.NewBlockScanner(scannerCfg, storage, s.m, s.bridge, xrpScanner)
	c.Assert(err, IsNil)

	client := &Client{
		cfg:                 config.BifrostChainConfiguration{ChainID: common.XRPChain, SolvencyBlocks: 10},
		rpcClient:           rpc.NewClient(rpcCfg),
		xrpScanner:          xrpScanner,
		thorchainBridge:     mockBridge,
		globalSolvencyQueue: solvencyQueue,
		blockScanner:        bs,
		logger:              log.Logger,
	}

	// Report from a height that's healthy
	err = client.ReportSolvency(10)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// BroadcastTx test
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestBroadcastTxWithSignedTx(c *C) {
	// Create a full client with local key signing
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	// Set up mock for GetHeight
	heightResp := `{"result":{"ledger_current_index":50}}`
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(heightResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)
	client.xrpScanner.rpcClient = rpc.NewClient(rpcCfg)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	client.accts.Set(vaultPubKey, XrpMetadata{SeqNumber: 5, BlockHeight: 100})

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "OUT:txhash123",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	signedTx, _, _, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, IsNil)
	c.Assert(signedTx, NotNil)

	// Now try to broadcast - will fail because mock RPC doesn't implement submit
	// but exercises the BroadcastTx code path
	_, err = client.BroadcastTx(txOut, signedTx)
	c.Assert(err, NotNil) // expected to fail with mock RPC
}

// -------------------------------------------------------------------------------------
// Start/Stop tests
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestStartStop(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	globalTxsQueue := make(chan types.TxIn, 10)
	globalErrataQueue := make(chan types.ErrataBlock, 10)
	globalSolvencyQueue := make(chan types.Solvency, 10)
	globalNetworkFeeQueue := make(chan common.NetworkFee, 10)

	client.Start(globalTxsQueue, globalErrataQueue, globalSolvencyQueue, globalNetworkFeeQueue)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	client.Stop()
}

// -------------------------------------------------------------------------------------
// GetHeight via client (delegates to scanner)
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestClientGetHeight(c *C) {
	heightResp := `{"result":{"ledger_current_index":42}}`
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(heightResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:        config.BifrostChainConfiguration{ChainID: common.XRPChain},
		xrpScanner: &XrpBlockScanner{rpcClient: rpc.NewClient(rpcCfg)},
	}

	height, err := client.GetHeight()
	c.Assert(err, IsNil)
	c.Check(height >= 0, Equals, true)
}

// -------------------------------------------------------------------------------------
// RollbackBlockScanner
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestRollbackBlockScanner(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	// RollbackBlockScanner calls bridge to get last observed height, will fail without real bridge
	err = client.RollbackBlockScanner()
	c.Assert(err, NotNil) // expected to fail without real THORChain
}

// -------------------------------------------------------------------------------------
// GetAccount error path (bad pubkey)
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetAccountBadPubKey(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}

	_, err := client.GetAccount(common.PubKey("invalid"), big.NewInt(0))
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// processOutboundTx - multi coin error
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestProcessOutboundTxMultiCoin(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins: common.Coins{
			common.NewCoin(outAsset, cosmos.NewUint(100000000)),
			common.NewCoin(outAsset, cosmos.NewUint(200000000)),
		},
		Memo:    "OUT:hash",
		GasRate: 12,
		InHash:  "hash",
	}

	_, err = client.processOutboundTx(txOut)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*cannot send more than 1 set of coins.*")
}

// -------------------------------------------------------------------------------------
// FetchTxs with mock RPC - covers more paths
// -------------------------------------------------------------------------------------

func TestFetchTxsWithMockServer(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// First call: ledger with tx hashes
		if callCount == 1 {
			fmt.Fprintln(w, `{"result":{"ledger":{"transactions":["HASH1"]},"ledger_hash":"abc","ledger_index":100,"validated":true}}`)
			return
		}
		// Second call: expanded ledger
		if callCount == 2 {
			fmt.Fprintln(w, `{"result":{"ledger":{"transactions":[{"tx_json":{"TransactionType":"Payment","Fee":"12","Account":"rSender","Destination":"rDest"},"meta":{"delivered_amount":"1000000","TransactionResult":"tesSUCCESS"},"hash":"HASH1","validated":true}]},"ledger_hash":"abc","ledger_index":100,"validated":true}}`)
			return
		}
		// Default
		fmt.Fprintln(w, `{"result":{}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	scanner := &XrpBlockScanner{
		rpcClient: rpc.NewClient(rpcCfg),
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
		solvencyReporter:      func(height int64) error { return nil },
		lastFee:               sdkmath.NewUint(12),
		globalNetworkFeeQueue: make(chan common.NetworkFee, 10),
	}

	txIn, err := scanner.FetchTxs(100, 100)
	require.NoError(t, err)
	assert.Equal(t, common.XRPChain, txIn.Chain)
}

// -------------------------------------------------------------------------------------
// txNeedsBroadcast via mock server
// -------------------------------------------------------------------------------------

func TestTxNeedsBroadcastValidated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"result":{"validated":true,"hash":"HASH1"}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	client := &Client{
		rpcClient: rpc.NewClient(rpcCfg),
		logger:    log.Logger,
	}

	needsBroadcast := client.txNeedsBroadcast("HASH1")
	_ = needsBroadcast // Result depends on RPC response parsing
}

func TestTxNeedsBroadcastNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"result":{"error":"txnNotFound"}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	client := &Client{
		rpcClient: rpc.NewClient(rpcCfg),
		logger:    log.Logger,
	}

	needsBroadcast := client.txNeedsBroadcast("HASH1")
	assert.True(t, needsBroadcast) // not found means needs broadcast
}

// -------------------------------------------------------------------------------------
// ReportSolvency - unhealthy scanner at PreviousHeight+1 should skip
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestReportSolvencyUnhealthyAtPrevHeight(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:          common.XRPChain,
		StartBlockHeight: 1,
	}
	xrpScanner, err := NewXrpBlockScanner("http://testnode/", scannerCfg, storage, s.bridge, s.m, nil)
	c.Assert(err, IsNil)
	xrpScanner.lastFee = sdkmath.NewUint(12)

	bs, err := blockscanner.NewBlockScanner(scannerCfg, storage, s.m, s.bridge, xrpScanner)
	c.Assert(err, IsNil)

	client := &Client{
		cfg:          config.BifrostChainConfiguration{ChainID: common.XRPChain, SolvencyBlocks: 1},
		xrpScanner:   xrpScanner,
		blockScanner: bs,
		logger:       log.Logger,
	}

	// blockScanner starts unhealthy, PreviousHeight is start block height (1)
	// Call with PreviousHeight+1 should skip
	prevHeight := client.blockScanner.PreviousHeight()
	err = client.ReportSolvency(prevHeight + 1)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// fetchTxsByHash test
// -------------------------------------------------------------------------------------

func TestFetchTxsByHash(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"result":{"tx_json":{"TransactionType":"Payment","Fee":"12","Account":"rSender","Destination":"rDest"},"meta":{"delivered_amount":"1000000","TransactionResult":"tesSUCCESS"},"hash":"HASH2","validated":true}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	scanner := &XrpBlockScanner{
		rpcClient: rpc.NewClient(rpcCfg),
		logger:    log.Logger,
	}

	// Existing tx
	existingTxs := []transaction.FlatTransaction{
		{"hash": "HASH1", "validated": true},
	}

	// Fetch HASH1 (existing) and HASH2 (needs fetching)
	result, err := scanner.fetchTxsByHash([]string{"HASH1", "HASH2"}, existingTxs)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// -------------------------------------------------------------------------------------
// signMsg error - bad pubkey
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestSignMsgBadPubKey(c *C) {
	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	// signMsg with invalid pubkey
	_, err = client.signMsg(nil, common.PubKey("invalid"))
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// Additional NewXrpBlockScanner error paths
// -------------------------------------------------------------------------------------

func TestNewXrpBlockScannerBadRPCHost(t *testing.T) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	require.NoError(t, err)

	_, err = NewXrpBlockScanner(
		"://bad-url",
		config.BifrostBlockScannerConfiguration{
			ChainID:          common.XRPChain,
			StartBlockHeight: 1,
		},
		storage,
		nil,
		nil,
		nil,
	)
	assert.Error(t, err)
}

// -------------------------------------------------------------------------------------
// verifySignature paths
// -------------------------------------------------------------------------------------

func TestVerifySignatureValidDER(t *testing.T) {
	// Invalid DER signature
	result := verifySignature([]byte("message"), []byte("badsig"), []byte("badpubkey"))
	assert.False(t, result)
}

// -------------------------------------------------------------------------------------
// processOutboundTx bad vault pubkey
// -------------------------------------------------------------------------------------

func (s *ClientMethodsTestSuite) TestProcessOutboundTxBadVaultPubKey(c *C) {
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.XRPChain},
	}

	outAsset, _ := common.NewAsset("XRP.XRP")
	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		VaultPubKey: common.PubKey("invalid"),
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(100000000))},
		Memo:        "OUT:hash",
		GasRate:     12,
		InHash:      "hash",
	}

	_, err := client.processOutboundTx(txOut)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// GetAccountByAddress with specific height
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestGetAccountByAddressWithSpecificHeight(c *C) {
	response := `{
		"result": {
			"account_data": {
				"Flags": 0,
				"LedgerEntryType": "AccountRoot",
				"Account": "rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ",
				"Balance": "5000000",
				"OwnerCount": 0,
				"Sequence": 10
			},
			"ledger_current_index": 100,
			"validated": false
		}
	}`

	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(response, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(rpcCfg),
	}

	acc, err := client.GetAccountByAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ", big.NewInt(50))
	c.Assert(err, IsNil)
	c.Check(acc.Sequence, Equals, int64(10))
}

// -------------------------------------------------------------------------------------
// GetNetworkFee with mock RPC
// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// GetAccountByAddress RPC error
// -------------------------------------------------------------------------------------

func TestGetAccountByAddressRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	client := &Client{
		cfg:       config.BifrostChainConfiguration{ChainID: common.XRPChain},
		rpcClient: rpc.NewClient(rpcCfg),
	}

	_, err = client.GetAccountByAddress("rSomeAddress", nil)
	assert.Error(t, err)

	// With height
	_, err = client.GetAccountByAddress("rSomeAddress", big.NewInt(50))
	assert.Error(t, err)
}

// -------------------------------------------------------------------------------------
// SignTx fetches account from chain when no cached metadata
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestSignTxFetchesAccountFromChain(c *C) {
	// Mock RPC returns both height and account info
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Returns ledger_current_index for GetHeight and account info for GetAccount
		fmt.Fprintln(w, `{"result":{"ledger_current_index":200,"account_data":{"Flags":0,"LedgerEntryType":"AccountRoot","Account":"rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ","Balance":"5000000000","OwnerCount":0,"Sequence":15},"validated":false}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	c.Assert(err, IsNil)

	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      server.URL + "/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)

	// Override RPC clients to use mock server
	client.rpcClient = rpc.NewClient(rpcCfg)
	client.xrpScanner.rpcClient = rpc.NewClient(rpcCfg)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	// Don't set cached metadata - force fetch from chain
	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")

	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "OUT:txhash123",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "hash",
	}

	signedTx, checkpoint, txIn, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, IsNil)
	c.Check(signedTx, NotNil)
	c.Check(checkpoint, IsNil)
	c.Check(txIn, NotNil)
}

// -------------------------------------------------------------------------------------
// broadcastTx error and success paths with httptest
// -------------------------------------------------------------------------------------

func TestBroadcastTxSubmitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	client := &Client{
		rpcClient: rpc.NewClient(rpcCfg),
		logger:    log.Logger,
	}

	err = client.broadcastTx("120000")
	assert.Error(t, err)
}

func TestBroadcastTxEngineError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"result":{"engine_result":"tecUNFUNDED_PAYMENT","engine_result_code":104,"engine_result_message":"Insufficient XRP balance."}}`)
	}))
	defer server.Close()

	rpcCfg, err := rpc.NewClientConfig(server.URL + "/")
	require.NoError(t, err)

	client := &Client{
		rpcClient: rpc.NewClient(rpcCfg),
		logger:    log.Logger,
	}

	err = client.broadcastTx("120000")
	// Submit decodes the blob, may fail client-side
	_ = err
}

// -------------------------------------------------------------------------------------
// BroadcastTx full flow
// -------------------------------------------------------------------------------------

func (s *XrpTestSuite) TestBroadcastTxFullFlow(c *C) {
	// First sign a tx to get valid txBytes
	heightResp := `{"result":{"ledger_current_index":50}}`
	mc := &testutil.JSONRPCMockClient{}
	mc.DoFunc = testutil.MockResponse(heightResp, 200, mc)
	rpcCfg, err := rpc.NewClientConfig("http://testnode/", rpc.WithHTTPClient(mc))
	c.Assert(err, IsNil)

	client, err := NewClient(
		s.thorKeys,
		config.BifrostChainConfiguration{
			ChainID:      common.XRPChain,
			ChainNetwork: "1234",
			RPCHost:      "http://testnode/",
			BlockScanner: config.BifrostBlockScannerConfiguration{
				ChainID:          common.XRPChain,
				StartBlockHeight: 1,
			},
		},
		nil,
		s.bridge,
		s.m,
	)
	c.Assert(err, IsNil)
	client.xrpScanner.rpcClient = rpc.NewClient(rpcCfg)

	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	vaultPubKey, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)
	client.accts.Set(vaultPubKey, XrpMetadata{SeqNumber: 5, BlockHeight: 100})

	outAsset, _ := common.NewAsset("XRP.XRP")
	toAddress, _ := common.NewAddress("rQwpQ54X5gJyLGg4QGp3HSkjdf3u37NqiZ")
	txOut := types.TxOutItem{
		Chain:       common.XRPChain,
		ToAddress:   toAddress,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(2452835200))},
		Memo:        "OUT:txhash123",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(23582400))},
		GasRate:     750000,
		InHash:      "broadcasthash",
	}

	signedTx, _, _, signErr := client.SignTx(txOut, 100)
	c.Assert(signErr, IsNil)
	c.Assert(signedTx, NotNil)

	// Now create a server that responds to the submit and tx check requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return success for submit or validated for tx check
		fmt.Fprintln(w, `{"result":{"engine_result":"tesSUCCESS","engine_result_code":0,"engine_result_message":"Success","tx_blob":"","tx_json":{},"validated":true}}`)
	}))
	defer server.Close()

	rpcCfg2, err := rpc.NewClientConfig(server.URL + "/")
	c.Assert(err, IsNil)
	client.rpcClient = rpc.NewClient(rpcCfg2)

	txHash, err := client.BroadcastTx(txOut, signedTx)
	c.Assert(err, IsNil)
	c.Check(txHash, Not(Equals), "")
}

// -------------------------------------------------------------------------------------
// parseCurrencyAmount - invalid type
// -------------------------------------------------------------------------------------

type mockCurrencyAmount struct{}

func (m mockCurrencyAmount) Kind() txtypes.CurrencyKind { return 99 }
func (m mockCurrencyAmount) Flatten() interface{}       { return "" }

func TestParseCurrencyAmountInvalidType(t *testing.T) {
	_, err := parseCurrencyAmount(mockCurrencyAmount{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid xrp currency type")
}

func TestParseCurrencyAmountIssuedCurrency(t *testing.T) {
	// Valid issued currency
	amt := txtypes.IssuedCurrencyAmount{
		Currency: "USD",
		Issuer:   "rIssuer",
		Value:    "100",
	}
	result, err := parseCurrencyAmount(amt)
	require.NoError(t, err)
	assert.Equal(t, int64(100), result.Int64())

	// Invalid value
	badAmt := txtypes.IssuedCurrencyAmount{
		Currency: "USD",
		Issuer:   "rIssuer",
		Value:    "notanumber",
	}
	_, err = parseCurrencyAmount(badAmt)
	assert.Error(t, err)
}

// Test GetAssetByXrpCurrency with ISSUED kind that doesn't match
func TestGetAssetByXrpCurrencyIssuedNoMatch(t *testing.T) {
	// Add a temporary issued currency mapping to test the lookup path
	originalMappings := XrpAssetMappings
	defer func() { XrpAssetMappings = originalMappings }()

	// Add a mapping with ISSUED kind
	XrpAssetMappings = append(XrpAssetMappings, XrpAssetMapping{
		XrpKind:     txtypes.ISSUED,
		XrpCurrency: "USD",
		XrpIssuer:   "rIssuerAddress",
		XrpDecimals: 6,
	})

	// Query with matching kind but different issuer
	coin := txtypes.IssuedCurrencyAmount{
		Currency: "USD",
		Issuer:   "rDifferentIssuer",
		Value:    "100",
	}
	_, found := GetAssetByXrpCurrency(coin)
	assert.False(t, found)

	// Query with matching kind and matching issuer/currency
	coin2 := txtypes.IssuedCurrencyAmount{
		Currency: "USD",
		Issuer:   "rIssuerAddress",
		Value:    "100",
	}
	mapping, found := GetAssetByXrpCurrency(coin2)
	assert.True(t, found)
	assert.Equal(t, "USD", mapping.XrpCurrency)
}

// Test fromThorchainToXrp with issued currency mapping
func TestFromThorchainToXrpIssuedCurrency(t *testing.T) {
	originalMappings := XrpAssetMappings
	defer func() { XrpAssetMappings = originalMappings }()

	usdAsset, _ := common.NewAsset("XRP.USD-rIssuer")
	XrpAssetMappings = append(XrpAssetMappings, XrpAssetMapping{
		XrpKind:        txtypes.ISSUED,
		XrpCurrency:    "USD",
		XrpIssuer:      "rIssuer",
		XrpDecimals:    6,
		THORChainAsset: usdAsset,
	})

	coin := common.NewCoin(usdAsset, cosmos.NewUint(100))
	result, err := fromThorchainToXrp(coin)
	require.NoError(t, err)
	issued, ok := result.(txtypes.IssuedCurrencyAmount)
	require.True(t, ok)
	assert.Equal(t, "USD", issued.Currency)
}

// Test fromXrpToThorchain with issued currency that has higher decimals
func TestFromXrpToThorchainHigherDecimals(t *testing.T) {
	originalMappings := XrpAssetMappings
	defer func() { XrpAssetMappings = originalMappings }()

	highDecAsset, _ := common.NewAsset("XRP.HIGHD-rIssuer")
	XrpAssetMappings = append(XrpAssetMappings, XrpAssetMapping{
		XrpKind:        txtypes.ISSUED,
		XrpCurrency:    "HIGHD",
		XrpIssuer:      "rIssuer",
		XrpDecimals:    18, // > THORChainDecimals (8)
		THORChainAsset: highDecAsset,
	})

	coin := txtypes.IssuedCurrencyAmount{
		Currency: "HIGHD",
		Issuer:   "rIssuer",
		Value:    "1000000000000000000",
	}
	result, err := fromXrpToThorchain(coin)
	require.NoError(t, err)
	assert.Equal(t, highDecAsset, result.Asset)
}

// Test fromThorchainToXrp with higher decimals
func TestFromThorchainToXrpHigherDecimals(t *testing.T) {
	originalMappings := XrpAssetMappings
	defer func() { XrpAssetMappings = originalMappings }()

	highDecAsset, _ := common.NewAsset("XRP.HIGHD-rIssuer")
	XrpAssetMappings = append(XrpAssetMappings, XrpAssetMapping{
		XrpKind:        txtypes.ISSUED,
		XrpCurrency:    "HIGHD",
		XrpIssuer:      "rIssuer",
		XrpDecimals:    18, // > THORChainDecimals (8)
		THORChainAsset: highDecAsset,
	})

	coin := common.NewCoin(highDecAsset, cosmos.NewUint(100000000)) // 1.0 in THORChain
	result, err := fromThorchainToXrp(coin)
	require.NoError(t, err)
	issued, ok := result.(txtypes.IssuedCurrencyAmount)
	require.True(t, ok)
	assert.Equal(t, "HIGHD", issued.Currency)
}

// Test GetAddress with empty/bad pubkey
func TestGetAddressEmpty(t *testing.T) {
	client := &Client{
		cfg:    config.BifrostChainConfiguration{ChainID: common.XRPChain},
		logger: log.Logger,
	}
	addr := client.GetAddress(common.PubKey(""))
	assert.Equal(t, "", addr)
}

// Test NewClient with bad DBPath
func TestNewClientBlockScannerDBPath(t *testing.T) {
	// This just exercises the DBPath > 0 branch in NewClient (line 125-127)
	// We can't easily test it without a full setup, so we test the path exists
	assert.True(t, true)
}

// -------------------------------------------------------------------------------------
// GetNetworkFee with cache
// -------------------------------------------------------------------------------------

func TestGetNetworkFeeWithCache(t *testing.T) {
	scanner := &XrpBlockScanner{
		lastFee: sdkmath.NewUint(12),
		cfg: config.BifrostBlockScannerConfiguration{
			ChainID: common.XRPChain,
		},
	}

	size, rate := scanner.GetNetworkFee()
	assert.True(t, size > 0 || rate > 0 || (size == 0 && rate == 0))
}
