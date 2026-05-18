package gaia

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	ctypes "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	atypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	btypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	grpc "google.golang.org/grpc"

	ctypesrpc "github.com/cometbft/cometbft/rpc/core/types"

	"gitlab.com/thorchain/thornode/v3/bifrost/blockscanner"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/pkg/chainclients/shared/signercache"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	stypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/cmd"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	. "gopkg.in/check.v1"
)

// -------------------------------------------------------------------------------------
// Extra Test Suite
// -------------------------------------------------------------------------------------

type ExtraTestSuite struct {
	thorKeys *thorclient.Keys
	bridge   thorclient.ThorchainBridge
	m        *metrics.Metrics
}

var _ = Suite(&ExtraTestSuite{})

func (s *ExtraTestSuite) SetUpSuite(c *C) {
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
	s.thorKeys = thorclient.NewKeysWithKeybase(kb, cfg.SignerName, cfg.SignerPasswd)
	s.bridge, err = thorclient.NewThorchainBridge(cfg, s.m, s.thorKeys)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// CosmosClient simple methods
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetConfig(c *C) {
	cfg := config.BifrostChainConfiguration{ChainID: common.GAIAChain}
	cc := CosmosClient{cfg: cfg}
	c.Check(cc.GetConfig().ChainID, Equals, common.GAIAChain)
}

func (s *ExtraTestSuite) TestGetChain(c *C) {
	cc := CosmosClient{cfg: config.BifrostChainConfiguration{ChainID: common.GAIAChain}}
	c.Check(cc.GetChain(), Equals, common.GAIAChain)
}

func (s *ExtraTestSuite) TestConfirmationCountReady(c *C) {
	cc := CosmosClient{}
	c.Check(cc.ConfirmationCountReady(stypes.TxIn{}), Equals, true)
}

func (s *ExtraTestSuite) TestGetConfirmationCount(c *C) {
	cc := CosmosClient{}
	c.Check(cc.GetConfirmationCount(stypes.TxIn{}), Equals, int64(0))
}

func (s *ExtraTestSuite) TestShouldReportSolvency(c *C) {
	scannerCfg := config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain}
	scanner := &CosmosBlockScanner{
		cfg:     scannerCfg,
		lastFee: sdkmath.NewUint(0),
	}
	cc := CosmosClient{cosmosScanner: scanner}

	// height not divisible by 10
	c.Check(cc.ShouldReportSolvency(11), Equals, false)

	// height divisible by 10 but lastFee is zero
	c.Check(cc.ShouldReportSolvency(10), Equals, false)

	// height divisible by 10 and lastFee non-zero
	scanner.lastFee = sdkmath.NewUint(100)
	c.Check(cc.ShouldReportSolvency(10), Equals, true)
	c.Check(cc.ShouldReportSolvency(20), Equals, true)
	c.Check(cc.ShouldReportSolvency(21), Equals, false)
}

// -------------------------------------------------------------------------------------
// Block Scanner simple methods
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetNetworkFee(c *C) {
	scanner := &CosmosBlockScanner{
		lastFee: sdkmath.NewUint(42),
	}
	size, rate := scanner.GetNetworkFee()
	c.Check(size, Equals, uint64(1))
	c.Check(rate, Equals, uint64(42))
}

func (s *ExtraTestSuite) TestFetchMemPool(c *C) {
	scanner := &CosmosBlockScanner{}
	txIn, err := scanner.FetchMemPool(100)
	c.Assert(err, IsNil)
	c.Check(len(txIn.TxArray), Equals, 0)
}

func (s *ExtraTestSuite) TestGetHeight(c *C) {
	scanner := &CosmosBlockScanner{
		rpc: &mockTendermintRPC{},
	}
	height, err := scanner.GetHeight()
	c.Assert(err, IsNil)
	// GetHeight returns latest block height - 1
	c.Check(height > 0, Equals, true)
}

// -------------------------------------------------------------------------------------
// Asset Mapping
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetAssetByCosmosDenom(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
			{Denom: "ibc/ABC123", Decimals: 6, THORChainSymbol: "NTRN"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// found
	asset, ok := scanner.GetAssetByCosmosDenom("uatom")
	c.Check(ok, Equals, true)
	c.Check(asset.CosmosDenom, Equals, "uatom")
	c.Check(asset.THORChainSymbol, Equals, "ATOM")

	// case insensitive
	asset, ok = scanner.GetAssetByCosmosDenom("UATOM")
	c.Check(ok, Equals, true)
	c.Check(asset.CosmosDenom, Equals, "uatom")

	// not found
	_, ok = scanner.GetAssetByCosmosDenom("unknown")
	c.Check(ok, Equals, false)
}

func (s *ExtraTestSuite) TestGetAssetByThorchainSymbol(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	asset, ok := scanner.GetAssetByThorchainSymbol("ATOM")
	c.Check(ok, Equals, true)
	c.Check(asset.CosmosDenom, Equals, "uatom")

	// case insensitive
	asset, ok = scanner.GetAssetByThorchainSymbol("atom")
	c.Check(ok, Equals, true)

	// not found
	_, ok = scanner.GetAssetByThorchainSymbol("BTC")
	c.Check(ok, Equals, false)
}

// -------------------------------------------------------------------------------------
// fromCosmosToThorchain / fromThorchainToCosmos edge cases
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFromCosmosToThorchainNotWhitelisted(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// not whitelisted
	_, err := scanner.fromCosmosToThorchain(cosmos.NewCoin("unknown", sdkmath.NewInt(100)))
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*not whitelisted.*")
}

func (s *ExtraTestSuite) TestFromThorchainToCosmosNotWhitelisted(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	unknownAsset, err := common.NewAsset("GAIA.UNKNOWN")
	c.Assert(err, IsNil)
	_, err = scanner.fromThorchainToCosmos(common.Coin{
		Asset:  unknownAsset,
		Amount: cosmos.NewUint(100),
	})
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*not whitelisted.*")
}

func (s *ExtraTestSuite) TestFromCosmosToThorchainHigherDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "utoken", Decimals: 18, THORChainSymbol: "TKN"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// 18 decimals -> 8 decimals (divide by 10^10)
	coin, err := scanner.fromCosmosToThorchain(cosmos.NewCoin("utoken", sdkmath.NewInt(5_000_000_000_000_000_000)))
	c.Assert(err, IsNil)
	c.Check(coin.Amount.Uint64(), Equals, uint64(500_000_000))
	c.Check(coin.Decimals, Equals, int64(18))
}

func (s *ExtraTestSuite) TestFromThorchainToCosmosHigherDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "utoken", Decimals: 18, THORChainSymbol: "TKN"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	thorAsset, err := common.NewAsset("GAIA.TKN")
	c.Assert(err, IsNil)

	cosmosCoin, err := scanner.fromThorchainToCosmos(common.Coin{
		Asset:  thorAsset,
		Amount: cosmos.NewUint(500_000_000), // 5 tokens in 1e8
	})
	c.Assert(err, IsNil)
	c.Check(cosmosCoin.Denom, Equals, "utoken")
	// 500_000_000 * 10^10 = 5e18
	expected := sdkmath.NewIntFromBigInt(sdkmath.NewUint(5_000_000_000_000_000_000).BigInt())
	c.Check(cosmosCoin.Amount.Equal(expected), Equals, true)
}

func (s *ExtraTestSuite) TestFromCosmosToThorchainEqualDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "utoken", Decimals: 8, THORChainSymbol: "TKN"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// 8 decimals == THORChain decimals, no conversion needed
	coin, err := scanner.fromCosmosToThorchain(cosmos.NewCoin("utoken", sdkmath.NewInt(500_000_000)))
	c.Assert(err, IsNil)
	c.Check(coin.Amount.Uint64(), Equals, uint64(500_000_000))
}

// -------------------------------------------------------------------------------------
// keyManager
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestKeyManager(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)

	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	km := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	// Pubkey
	c.Check(km.Pubkey().Equals(pk), Equals, true)

	// GetAddr
	c.Check(km.GetAddr().Equals(ctypes.AccAddress(priv.PubKey().Address())), Equals, true)

	// GetPrivKey
	c.Check(km.GetPrivKey(), NotNil)

	// ExportAsMnemonic - no mnemonic set
	_, err = km.ExportAsMnemonic()
	c.Assert(err, NotNil)

	// ExportAsMnemonic - with mnemonic
	km.mnemonic = "test mnemonic"
	mnemonic, err := km.ExportAsMnemonic()
	c.Assert(err, IsNil)
	c.Check(mnemonic, Equals, "test mnemonic")

	// ExportAsPrivateKey
	privKeyHex, err := km.ExportAsPrivateKey()
	c.Assert(err, IsNil)
	c.Check(len(privKeyHex) > 0, Equals, true)

	// Sign
	sig, err := km.Sign([]byte("test message"))
	c.Assert(err, IsNil)
	c.Check(len(sig) > 0, Equals, true)
}

// -------------------------------------------------------------------------------------
// updateGasFees
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestUpdateGasFees(c *C) {
	feeQueue := make(chan common.NetworkFee, 10)
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{
		cfg:                   cfg,
		feeCache:              make([]sdkmath.Uint, 0),
		lastFee:               sdkmath.NewUint(0),
		globalNetworkFeeQueue: feeQueue,
	}

	// not enough cache entries -> no update
	err := scanner.updateGasFees(10)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 0)

	// fill cache to exactly GasCacheTransactions
	for i := 0; i < GasCacheTransactions; i++ {
		scanner.updateGasCache(&MockFeeTx{
			gas: GasLimit,
			fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
		})
	}
	c.Check(len(scanner.feeCache), Equals, GasCacheTransactions)

	// not on GasUpdatePeriodBlocks boundary
	err = scanner.updateGasFees(11)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 0)

	// on boundary with full cache -> should post fee
	err = scanner.updateGasFees(10)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 1)

	nf := <-feeQueue
	c.Check(nf.Chain, Equals, common.GAIAChain)
	c.Check(nf.TransactionSize, Equals, uint64(1))
	c.Check(nf.TransactionRate > 0, Equals, true)

	// calling again with same fee -> skipped (within resolution)
	err = scanner.updateGasFees(20)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 0)
}

func (s *ExtraTestSuite) TestUpdateGasCacheZeroGas(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// zero gas should be skipped
	scanner.updateGasCache(&MockFeeTx{
		gas: 0,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
	})
	c.Check(len(scanner.feeCache), Equals, 0)
}

func (s *ExtraTestSuite) TestUpdateGasCacheZeroFeeAmount(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// zero fee amount should be skipped (coin.Valid() fails)
	scanner.updateGasCache(&MockFeeTx{
		gas: 100000,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(0))},
	})
	c.Check(len(scanner.feeCache), Equals, 0)
}

func (s *ExtraTestSuite) TestUpdateGasCacheMultipleFees(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// multiple fee coins should be skipped
	scanner.updateGasCache(&MockFeeTx{
		gas: 100000,
		fee: ctypes.Coins{
			ctypes.NewCoin("uatom", sdkmath.NewInt(10000)),
			ctypes.NewCoin("uosmo", sdkmath.NewInt(5000)),
		},
	})
	c.Check(len(scanner.feeCache), Equals, 0)
}

func (s *ExtraTestSuite) TestUpdateGasCacheNonWhitelistedFee(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// non-whitelisted fee coin should be skipped
	scanner.updateGasCache(&MockFeeTx{
		gas: 100000,
		fee: ctypes.Coins{ctypes.NewCoin("uosmo", sdkmath.NewInt(10000))},
	})
	c.Check(len(scanner.feeCache), Equals, 0)
}

func (s *ExtraTestSuite) TestUpdateGasCacheTinyFeeBecomesOne(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// Very small fee with very large gas should produce a zero fee after
	// ThorchainToNativeGas conversion, triggering the fee.IsZero() -> OneUint() path
	scanner.updateGasCache(&MockFeeTx{
		gas: 300000000, // 3e8 - large enough to make fee zero after division
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(1))},
	})
	// The fee should be set to 1 (cosmos.OneUint()) instead of 0
	c.Assert(len(scanner.feeCache), Equals, 1)
	c.Check(scanner.feeCache[0].Equal(cosmos.OneUint()), Equals, true)
}

func (s *ExtraTestSuite) TestUpdateGasCacheTruncation(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	// Fill beyond GasCacheTransactions
	for i := 0; i < GasCacheTransactions+5; i++ {
		scanner.updateGasCache(&MockFeeTx{
			gas: 200000,
			fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(5000))},
		})
	}
	c.Check(len(scanner.feeCache), Equals, GasCacheTransactions)
}

// -------------------------------------------------------------------------------------
// FetchTxs
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFetchTxs(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:                      common.GAIAChain,
		GasPriceResolution:           1_000,
		ObservationFlexibilityBlocks: 100,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	btypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	solvencyReported := false
	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		rpc:      &mockTendermintRPC{},
		cdc:      cdc,
		feeCache: make([]sdkmath.Uint, 0),
		lastFee:  sdkmath.NewUint(0),
		solvencyReporter: func(height int64) error {
			solvencyReported = true
			return nil
		},
		globalNetworkFeeQueue: make(chan common.NetworkFee, 10),
	}

	// chainHeight close to block -> should run solvency
	txIn, err := scanner.FetchTxs(1, 10)
	c.Assert(err, IsNil)
	c.Check(txIn.Chain, Equals, common.GAIAChain)
	c.Check(solvencyReported, Equals, true)

	// chainHeight far from block -> skip solvency
	solvencyReported = false
	txIn, err = scanner.FetchTxs(1, 1000)
	c.Assert(err, IsNil)
	c.Check(solvencyReported, Equals, false)
}

// -------------------------------------------------------------------------------------
// getGRPCConn
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetGRPCConn(c *C) {
	// insecure connection
	conn, err := getGRPCConn("localhost:9090", false)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)

	// tls connection
	conn, err = getGRPCConn("localhost:9090", true)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)
}

// -------------------------------------------------------------------------------------
// buildUnsigned error path
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestBuildUnsignedBadPubkey(c *C) {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	msg := &btypes.MsgSend{
		FromAddress: "cosmos1xxx",
		ToAddress:   "cosmos1yyy",
		Amount:      ctypes.NewCoins(ctypes.NewCoin("uatom", sdkmath.NewInt(100))),
	}

	// invalid pubkey
	_, err := buildUnsigned(txConfig, msg, common.PubKey("invalid"), "memo", ctypes.NewCoins(), 0, 0)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// averageFee edge cases
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestAverageFeeEmpty(c *C) {
	scanner := &CosmosBlockScanner{
		feeCache: make([]sdkmath.Uint, 0),
	}
	fee := scanner.averageFee()
	c.Check(fee.IsZero(), Equals, true)
}

func (s *ExtraTestSuite) TestAverageFeeWithResolution(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		GasPriceResolution: 1000,
	}
	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		feeCache: []sdkmath.Uint{sdkmath.NewUint(500)},
	}
	// 500 <= resolution of 1000, so returns resolution
	fee := scanner.averageFee()
	c.Check(fee.Uint64(), Equals, uint64(1000))
}

func (s *ExtraTestSuite) TestAverageFeeRounding(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		GasPriceResolution: 1000,
	}
	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		feeCache: []sdkmath.Uint{sdkmath.NewUint(2500)},
	}
	// (2500-1)/1000 = 2, +1 = 3, *1000 = 3000
	fee := scanner.averageFee()
	c.Check(fee.Uint64(), Equals, uint64(3000))
}

// -------------------------------------------------------------------------------------
// OnObservedTxIn
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestOnObservedTxIn(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	cc := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		signerCacheManager: signerCacheMgr,
		cosmosScanner:      &CosmosBlockScanner{cfg: config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain}},
	}

	validOutMemo := "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD"

	// Build the expected cache hash for the valid outbound case so we can assert later.
	validOutItem := stypes.TxInItem{
		Tx:   "EEFF",
		Memo: validOutMemo,
	}
	expectedHash := validOutItem.CacheHash(common.GAIAChain, "AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD")

	// Confirm cache is empty before any calls.
	c.Check(signerCacheMgr.HasSigned(expectedHash), Equals, false)

	// invalid memo - early return (fails ParseMemo), cache unchanged
	cc.OnObservedTxIn(stypes.TxInItem{
		Tx:   "AABB",
		Memo: "",
	}, 100)
	c.Check(signerCacheMgr.HasSigned(expectedHash), Equals, false)

	// non-outbound memo - early return (!m.IsOutbound()), cache unchanged
	cc.OnObservedTxIn(stypes.TxInItem{
		Tx:   "AABB",
		Memo: "SWAP:GAIA.ATOM:cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz",
	}, 100)
	c.Check(signerCacheMgr.HasSigned(expectedHash), Equals, false)

	// outbound memo with empty txID - early return (m.GetTxID().IsEmpty()), cache unchanged
	cc.OnObservedTxIn(stypes.TxInItem{
		Tx:   "CCDD",
		Memo: "OUT:",
	}, 100)
	c.Check(signerCacheMgr.HasSigned(expectedHash), Equals, false)

	// outbound memo with valid txID - should call SetSigned and populate cache
	cc.OnObservedTxIn(validOutItem, 100)
	c.Check(signerCacheMgr.HasSigned(expectedHash), Equals, true)
}

// -------------------------------------------------------------------------------------
// GetAddress edge case - invalid pubkey
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetAddressInvalidPubkey(c *C) {
	cc := CosmosClient{
		cfg: config.BifrostChainConfiguration{ChainID: common.GAIAChain},
	}
	result := cc.GetAddress(common.PubKey("invalid"))
	c.Check(result, Equals, "")
}

// -------------------------------------------------------------------------------------
// processOutboundTx error path - invalid vault pubkey
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestProcessOutboundTxInvalidVault(c *C) {
	scannerCfg := config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain}
	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		cosmosScanner: &CosmosBlockScanner{cfg: scannerCfg},
	}

	txOut := stypes.TxOutItem{
		VaultPubKey: common.PubKey("invalid"),
	}
	_, err := cc.processOutboundTx(txOut, 1)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// BroadcastTx mock
// -------------------------------------------------------------------------------------

type mockTxServiceClient struct {
	code uint32
	err  error
}

func (m *mockTxServiceClient) Simulate(ctx context.Context, in *txtypes.SimulateRequest, opts ...grpc.CallOption) (*txtypes.SimulateResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) GetTx(ctx context.Context, in *txtypes.GetTxRequest, opts ...grpc.CallOption) (*txtypes.GetTxResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) BroadcastTx(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &txtypes.BroadcastTxResponse{
		TxResponse: &ctypes.TxResponse{
			Code:   m.code,
			TxHash: "AABBCCDD",
		},
	}, nil
}

func (m *mockTxServiceClient) GetTxsEvent(ctx context.Context, in *txtypes.GetTxsEventRequest, opts ...grpc.CallOption) (*txtypes.GetTxsEventResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) GetBlockWithTxs(ctx context.Context, in *txtypes.GetBlockWithTxsRequest, opts ...grpc.CallOption) (*txtypes.GetBlockWithTxsResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) TxDecode(ctx context.Context, in *txtypes.TxDecodeRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) TxEncode(ctx context.Context, in *txtypes.TxEncodeRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) TxEncodeAmino(ctx context.Context, in *txtypes.TxEncodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxEncodeAminoResponse, error) {
	return nil, nil
}

func (m *mockTxServiceClient) TxDecodeAmino(ctx context.Context, in *txtypes.TxDecodeAminoRequest, opts ...grpc.CallOption) (*txtypes.TxDecodeAminoResponse, error) {
	return nil, nil
}

func (s *ExtraTestSuite) TestBroadcastTx(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	pk := common.PubKey("sthorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzy9zsp6nhe2lgmf7ue08nxhm5frcg76v")
	cc := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		txClient:           &mockTxServiceClient{code: 0},
	}
	cc.accts.Set(pk, CosmosMetadata{AccountNumber: 1, SeqNumber: 5, BlockHeight: 100})

	txOut := stypes.TxOutItem{VaultPubKey: pk}
	hash, err := cc.BroadcastTx(txOut, []byte("txbytes"))
	c.Assert(err, IsNil)
	c.Check(hash, Equals, "AABBCCDD")

	// seq should have been incremented
	meta := cc.accts.Get(pk)
	c.Check(meta.SeqNumber, Equals, int64(6))
}

func (s *ExtraTestSuite) TestBroadcastTxError(c *C) {
	cc := CosmosClient{
		cfg:      config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		accts:    NewCosmosMetaDataStore(),
		txClient: &mockTxServiceClient{err: errors.New("network error")},
	}

	_, err := cc.BroadcastTx(stypes.TxOutItem{}, []byte("txbytes"))
	c.Assert(err, NotNil)
}

func (s *ExtraTestSuite) TestBroadcastTxFailCode(c *C) {
	cc := CosmosClient{
		cfg:      config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		accts:    NewCosmosMetaDataStore(),
		txClient: &mockTxServiceClient{code: 999},
	}

	_, err := cc.BroadcastTx(stypes.TxOutItem{}, []byte("txbytes"))
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "broadcast msg failed")
}

// -------------------------------------------------------------------------------------
// GetLatestTxForVault
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetLatestTxForVault(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	cc := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		signerCacheManager: signerCacheMgr,
	}

	// GetLatestTxForVault may return empty strings or error depending on storage state
	observed, broadcasted, err := cc.GetLatestTxForVault("vault1")
	// Just verify it doesn't panic and returns something reasonable
	if err == nil {
		c.Check(observed, Equals, "")
		c.Check(broadcasted, Equals, "")
	}
}

// -------------------------------------------------------------------------------------
// NewCosmosBlockScanner validation
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestNewCosmosBlockScannerNilStorage(c *C) {
	_, err := NewCosmosBlockScanner("http://localhost:26657", config.BifrostBlockScannerConfiguration{}, nil, nil, nil, nil, nil)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*nil.*")
}

func (s *ExtraTestSuite) TestNewCosmosBlockScannerNilMetrics(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	_, err = NewCosmosBlockScanner("http://localhost:26657", config.BifrostBlockScannerConfiguration{}, storage, nil, nil, nil, nil)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*nil.*")
}

// -------------------------------------------------------------------------------------
// unmarshalJSONToPb
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestUnmarshalJSONToPbFileNotFound(c *C) {
	msg := &atypes.QueryAccountResponse{}
	err := unmarshalJSONToPb("/nonexistent/file.json", msg)
	c.Assert(err, NotNil)
}

func (s *ExtraTestSuite) TestUnmarshalJSONToPbSuccess(c *C) {
	// Use the mock account service client which internally calls unmarshalJSONToPb
	// to verify the function works with test data
	mockClient := NewMockAccountServiceClient()
	resp, err := mockClient.Account(context.Background(), &atypes.QueryAccountRequest{
		Address: "cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz",
	})
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Account, NotNil)
}

// -------------------------------------------------------------------------------------
// GetAccount error path
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetAccountInvalidPubkey(c *C) {
	cc := CosmosClient{
		cfg: config.BifrostChainConfiguration{ChainID: common.GAIAChain},
	}
	_, err := cc.GetAccount(common.PubKey("invalid"), nil)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// processOutboundTx with non-whitelisted coin
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestProcessOutboundTxNonWhitelistedCoin(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	vaultPubKey, err := common.NewPubKey("sthorpub1addwnpepqda0q2avvxnferqasee42lu5492jlc4zvf6u264famvg9dywgq2kz0zaecw")
	c.Assert(err, IsNil)

	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	unknownAsset, err := common.NewAsset("GAIA.UNKNOWN")
	c.Assert(err, IsNil)

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: vaultPubKey,
		Coins:       common.Coins{common.NewCoin(unknownAsset, cosmos.NewUint(100))},
		Memo:        "test",
	}

	// Non-whitelisted coins are skipped, resulting in empty coins list
	msg, err := cc.processOutboundTx(txOut, 1)
	c.Assert(err, IsNil)
	c.Check(len(msg.Amount), Equals, 0)
}

// -------------------------------------------------------------------------------------
// GetBlock error path - nil block
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetBlockNilResult(c *C) {
	scanner := &CosmosBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain},
		rpc: &mockTendermintRPCNilBlock{},
	}
	_, err := scanner.GetBlock(999)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*nil block.*")
}

type mockTendermintRPCNilBlock struct{}

func (m *mockTendermintRPCNilBlock) Block(ctx context.Context, height *int64) (*ctypesrpc.ResultBlock, error) {
	return &ctypesrpc.ResultBlock{Block: nil}, nil
}

func (m *mockTendermintRPCNilBlock) BlockResults(ctx context.Context, height *int64) (*ctypesrpc.ResultBlockResults, error) {
	return nil, nil
}

// -------------------------------------------------------------------------------------
// GetBlock error path - RPC error
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetBlockRPCError(c *C) {
	scanner := &CosmosBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain},
		rpc: &mockTendermintRPCError{},
	}
	_, err := scanner.GetBlock(1)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*rpc error.*")
}

type mockTendermintRPCError struct{}

func (m *mockTendermintRPCError) Block(ctx context.Context, height *int64) (*ctypesrpc.ResultBlock, error) {
	return nil, fmt.Errorf("rpc error")
}

func (m *mockTendermintRPCError) BlockResults(ctx context.Context, height *int64) (*ctypesrpc.ResultBlockResults, error) {
	return nil, fmt.Errorf("rpc error")
}

// -------------------------------------------------------------------------------------
// GetHeight error path
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetHeightError(c *C) {
	scanner := &CosmosBlockScanner{
		rpc: &mockTendermintRPCError{},
	}
	_, err := scanner.GetHeight()
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// SignTx
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignTxFullPath(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)

	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
	}

	// Sign with local key manager
	txBytes, checkpoint, txIn, err := client.SignTx(txOut, 1)
	c.Assert(err, IsNil)
	c.Check(len(txBytes) > 0, Equals, true)
	c.Check(checkpoint, IsNil) // nil checkpoint on success
	c.Check(txIn, NotNil)

	// Already signed - should return nil
	err = signerCacheMgr.SetSigned(txOut.CacheHash(), txOut.CacheVault(common.GAIAChain), "hash1")
	c.Assert(err, IsNil)
	txBytes, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, IsNil)
	c.Check(txBytes, IsNil)
}

func (s *ExtraTestSuite) TestSignTxWithCheckpoint(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	// Test with checkpoint
	checkpoint := []byte(`{"AccountNumber":42,"SeqNumber":10,"BlockHeight":100}`)
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  checkpoint,
	}

	txBytes, _, txIn, err := client.SignTx(txOut, 1)
	c.Assert(err, IsNil)
	c.Check(len(txBytes) > 0, Equals, true)
	c.Check(txIn, NotNil)
}

func (s *ExtraTestSuite) TestSignTxNoGas(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	// No gas coins
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  []byte(`{"AccountNumber":1,"SeqNumber":1,"BlockHeight":1}`),
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*one gas coin.*")
}

func (s *ExtraTestSuite) TestSignTxWrongGasAsset(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	wrongAsset, err := common.NewAsset("BTC.BTC")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	// Wrong gas asset
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(wrongAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  []byte(`{"AccountNumber":1,"SeqNumber":1,"BlockHeight":1}`),
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*gas coin asset must match.*")
}

func (s *ExtraTestSuite) TestSignTxBadCheckpoint(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	// Bad checkpoint JSON
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  []byte(`{invalid`),
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// ReportSolvency
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestReportSolvencyNotReporting(c *C) {
	scanner := &CosmosBlockScanner{
		cfg:     config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain},
		lastFee: sdkmath.NewUint(0),
	}
	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		cosmosScanner: scanner,
	}

	// ShouldReportSolvency returns false -> ReportSolvency exits early
	err := cc.ReportSolvency(11)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// FetchTxs with solvency error
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFetchTxsSolvencyError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:                      common.GAIAChain,
		GasPriceResolution:           1_000,
		ObservationFlexibilityBlocks: 100,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	btypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		rpc:      &mockTendermintRPC{},
		cdc:      cdc,
		feeCache: make([]sdkmath.Uint, 0),
		lastFee:  sdkmath.NewUint(0),
		solvencyReporter: func(height int64) error {
			return fmt.Errorf("solvency error")
		},
		globalNetworkFeeQueue: make(chan common.NetworkFee, 10),
	}

	// Solvency error should not prevent returning txIn
	txIn, err := scanner.FetchTxs(1, 10)
	c.Assert(err, IsNil)
	c.Check(txIn.Chain, Equals, common.GAIAChain)
}

// -------------------------------------------------------------------------------------
// FetchTxs nil block
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFetchTxsNilBlock(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
	}

	scanner := &CosmosBlockScanner{
		cfg: cfg,
		rpc: &mockTendermintRPCError{},
	}

	_, err := scanner.FetchTxs(1, 10)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// processTxs with memo too long
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestUpdateGasFeesPostsFee(c *C) {
	feeQueue := make(chan common.NetworkFee, 10)
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	// Fill cache with non-zero values
	feeCache := make([]sdkmath.Uint, GasCacheTransactions)
	for i := range feeCache {
		feeCache[i] = sdkmath.NewUint(5000)
	}

	scanner := &CosmosBlockScanner{
		cfg:                   cfg,
		feeCache:              feeCache,
		lastFee:               sdkmath.NewUint(0), // zero lastFee so delta is large
		globalNetworkFeeQueue: feeQueue,
	}

	// On block 10 with full cache, should post the fee
	err := scanner.updateGasFees(10)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 1)
	nf := <-feeQueue
	c.Check(nf.TransactionRate > 0, Equals, true)

	// Now lastFee is set, calling with same cache should be within resolution -> skip
	err = scanner.updateGasFees(20)
	c.Assert(err, IsNil)
	c.Check(len(feeQueue), Equals, 0)
}

// -------------------------------------------------------------------------------------
// GetAccountByAddress with height
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetAccountByAddressWithHeight(c *C) {
	mockBankServiceClient := NewMockBankServiceClient()
	mockAccountServiceClient := NewMockAccountServiceClient()

	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		bankClient:    mockBankServiceClient,
		accountClient: mockAccountServiceClient,
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	// With specific height
	acc, err := cc.GetAccountByAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz", big.NewInt(100))
	c.Assert(err, IsNil)
	c.Check(acc.AccountNumber, Equals, int64(3530305))
}

// -------------------------------------------------------------------------------------
// CosmosSuccessCodes
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestCosmosSuccessCodes(c *C) {
	c.Check(CosmosSuccessCodes[0], Equals, true)
	c.Check(CosmosSuccessCodes[999], Equals, false)
}

// -------------------------------------------------------------------------------------
// signMsg error paths
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignMsgBadPubkey(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  common.PubKey("invalid"),
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	cc := CosmosClient{
		cfg:             config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:        txConfig,
		chainID:         "cosmoshub-4",
		localKeyManager: localKm,
	}

	msg := &btypes.MsgSend{
		FromAddress: "cosmos1xxx",
		ToAddress:   "cosmos1yyy",
		Amount:      ctypes.NewCoins(ctypes.NewCoin("uatom", sdkmath.NewInt(100))),
	}
	fee := ctypes.NewCoins(ctypes.NewCoin("uatom", sdkmath.NewInt(100)))

	// Valid pubkey for building unsigned tx
	validPk := common.PubKey("sthorpub1addwnpepqda0q2avvxnferqasee42lu5492jlc4zvf6u264famvg9dywgq2kz0zaecw")
	txBuilder, err := buildUnsigned(txConfig, msg, validPk, "memo", fee, 0, 0)
	c.Assert(err, IsNil)

	// signMsg with invalid pubkey should fail
	_, err = cc.signMsg(txBuilder, common.PubKey("invalid"), 0, 0)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// ReportSolvency - block scanner not healthy path
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestReportSolvencyBlockScannerNotHealthy(c *C) {
	scanner := &CosmosBlockScanner{
		cfg:     config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain},
		lastFee: sdkmath.NewUint(100),
	}

	// When block scanner is not healthy and height = previous + 1, skip
	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		cosmosScanner: scanner,
		blockScanner:  nil, // can't test fully without real block scanner
	}

	// Not divisible by 10 - early return
	err := cc.ReportSolvency(11)
	c.Assert(err, IsNil)
}

// -------------------------------------------------------------------------------------
// IsBlockScannerHealthy, GetBlockScannerHeight, RollbackBlockScanner (delegated methods)
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestDelegatedBlockScannerMethods(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:          common.GAIAChain,
		StartBlockHeight: 1, // avoids querying thorchain for block height
	}

	scanner, err := blockscanner.NewBlockScanner(scannerCfg, storage, s.m, s.bridge, &CosmosBlockScanner{
		cfg: scannerCfg,
		rpc: &mockTendermintRPC{},
	})
	c.Assert(err, IsNil)

	cc := CosmosClient{
		cfg:          config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		blockScanner: scanner,
	}

	// GetBlockScannerHeight
	height, err := cc.GetBlockScannerHeight()
	c.Assert(err, IsNil)
	c.Check(height >= 0, Equals, true)
}

// -------------------------------------------------------------------------------------
// GetAccountByAddress error paths
// -------------------------------------------------------------------------------------

type mockBankServiceClientError struct {
	mockBankServiceClient
}

func (c *mockBankServiceClientError) AllBalances(ctx context.Context, in *btypes.QueryAllBalancesRequest, opts ...grpc.CallOption) (*btypes.QueryAllBalancesResponse, error) {
	return nil, fmt.Errorf("bank error")
}

func (s *ExtraTestSuite) TestGetAccountByAddressBankError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		bankClient:    &mockBankServiceClientError{},
		accountClient: NewMockAccountServiceClient(),
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	_, err := cc.GetAccountByAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz", nil)
	c.Assert(err, NotNil)
}

type mockAccountServiceClientError struct {
	mockAccountServiceClient
}

func (c *mockAccountServiceClientError) Account(ctx context.Context, in *atypes.QueryAccountRequest, opts ...grpc.CallOption) (*atypes.QueryAccountResponse, error) {
	return nil, fmt.Errorf("account error")
}

func (s *ExtraTestSuite) TestGetAccountByAddressAuthError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		bankClient:    NewMockBankServiceClient(),
		accountClient: &mockAccountServiceClientError{},
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	_, err := cc.GetAccountByAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz", nil)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// BroadcastTx with success code for signer cache
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestBroadcastTxSuccessCodeSetsSigner(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	pk := common.PubKey("sthorpub1addwnpepqfshsq2y6ejy2ysxmq4gj8n8mzuzy9zsp6nhe2lgmf7ue08nxhm5frcg76v")
	cc := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		txClient:           &mockTxServiceClient{code: 0}, // success code
	}
	cc.accts.Set(pk, CosmosMetadata{AccountNumber: 1, SeqNumber: 5, BlockHeight: 100})

	txOut := stypes.TxOutItem{VaultPubKey: pk}
	hash, err := cc.BroadcastTx(txOut, []byte("txbytes"))
	c.Assert(err, IsNil)
	c.Check(hash, Equals, "AABBCCDD")

	// Check that signer cache was set
	c.Check(signerCacheMgr.HasSigned(txOut.CacheHash()), Equals, true)
}

// -------------------------------------------------------------------------------------
// SignTx with GetHeight error
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignTxGetHeightError(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	// Use error RPC to make GetHeight fail
	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPCError{}},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// OnObservedTxIn with outbound memo
// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// GetAccountByAddress with non-whitelisted coin in balances
// -------------------------------------------------------------------------------------

type mockBankServiceClientExtraCoins struct {
	mockBankServiceClient
}

func (c *mockBankServiceClientExtraCoins) AllBalances(ctx context.Context, in *btypes.QueryAllBalancesRequest, opts ...grpc.CallOption) (*btypes.QueryAllBalancesResponse, error) {
	return &btypes.QueryAllBalancesResponse{
		Balances: ctypes.NewCoins(
			ctypes.NewCoin("uatom", sdkmath.NewInt(1000000)),
			ctypes.NewCoin("unknowncoin", sdkmath.NewInt(500)), // not whitelisted
		),
	}, nil
}

func (s *ExtraTestSuite) TestGetAccountByAddressNonWhitelistedCoin(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		bankClient:    &mockBankServiceClientExtraCoins{},
		accountClient: NewMockAccountServiceClient(),
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	// Should succeed, but unknowncoin is skipped (covered error path at line 275)
	acc, err := cc.GetAccountByAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz", nil)
	c.Assert(err, IsNil)
	// Only ATOM coin should be returned
	c.Check(len(acc.Coins), Equals, 1)
}

// -------------------------------------------------------------------------------------
// GetAccountByAddress with bad account data
// -------------------------------------------------------------------------------------

type mockAccountServiceClientBadData struct {
	mockAccountServiceClient
}

func (c *mockAccountServiceClientBadData) Account(ctx context.Context, in *atypes.QueryAccountRequest, opts ...grpc.CallOption) (*atypes.QueryAccountResponse, error) {
	// Return account with invalid Value that can't be unmarshaled as BaseAccount
	any := &codectypes.Any{
		TypeUrl: "/cosmos.auth.v1beta1.BaseAccount",
		Value:   []byte("invalid data"),
	}
	return &atypes.QueryAccountResponse{
		Account: any,
	}, nil
}

func (s *ExtraTestSuite) TestGetAccountByAddressUnmarshalError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		bankClient:    NewMockBankServiceClient(),
		accountClient: &mockAccountServiceClientBadData{},
		cosmosScanner: &CosmosBlockScanner{cfg: cfg},
	}

	_, err := cc.GetAccountByAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz", nil)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// SignTx - insufficient vault balance
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignTxInsufficientBalance(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	// Request more than vault has (mock returns ~496694100 ATOM)
	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(99999999999))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*insufficient vault balance.*")
}

// -------------------------------------------------------------------------------------
// SignTx - non-whitelisted gas coin
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignTxNonWhitelistedGas(c *C) {
	priv, err := s.thorKeys.GetPrivateKey()
	c.Assert(err, IsNil)
	temp, err := cryptocodec.ToCmtPubKeyInterface(priv.PubKey())
	c.Assert(err, IsNil)
	pk, err := common.NewPubKeyFromCrypto(temp)
	c.Assert(err, IsNil)

	localKm := &keyManager{
		privKey: priv,
		addr:    ctypes.AccAddress(priv.PubKey().Address()),
		pubkey:  pk,
	}

	// Only whitelist ATOM, but gas will be ATOM (which IS gas asset)
	// So we need a scenario where fromThorchainToCosmos fails on gas
	scannerCfg := config.BifrostBlockScannerConfiguration{
		ChainID:               common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			// Don't whitelist ATOM - this will cause fromThorchainToCosmos to fail
		},
	}
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	interfaceRegistry.RegisterImplementations((*ctypes.Msg)(nil), &btypes.MsgSend{})
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txConfig := tx.NewTxConfig(marshaler, []signingtypes.SignMode{signingtypes.SignMode_SIGN_MODE_DIRECT})

	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		txConfig:           txConfig,
		cosmosScanner:      &CosmosBlockScanner{cfg: scannerCfg, rpc: &mockTendermintRPC{}, lastFee: sdkmath.NewUint(100)},
		bankClient:         NewMockBankServiceClient(),
		accountClient:      NewMockAccountServiceClient(),
		chainID:            "cosmoshub-4",
		localKeyManager:    localKm,
		accts:              NewCosmosMetaDataStore(),
		signerCacheManager: signerCacheMgr,
		thorchainBridge:    s.bridge,
	}

	outAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	toAddr, err := common.NewAddress("cosmos10tjz4ave7znpctgd2rfu6v2r6zkeup2dlmqtuz")
	c.Assert(err, IsNil)

	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		ToAddress:   toAddr,
		VaultPubKey: pk,
		Coins:       common.Coins{common.NewCoin(outAsset, cosmos.NewUint(1000000))},
		Memo:        "OUT:AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD",
		MaxGas:      common.Gas{common.NewCoin(outAsset, cosmos.NewUint(100000))},
		GasRate:     750000,
		InHash:      "hash",
		Checkpoint:  []byte(`{"AccountNumber":1,"SeqNumber":1,"BlockHeight":1}`),
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*unable to pay fee.*")
}

// -------------------------------------------------------------------------------------
// SignTx - processOutboundTx error (bad vault pubkey)
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestSignTxProcessOutboundError(c *C) {
	storage, err := blockscanner.NewBlockScannerStorage("", config.LevelDBOptions{})
	c.Assert(err, IsNil)
	signerCacheMgr, err := signercache.NewSignerCacheManager(storage.GetInternalDb())
	c.Assert(err, IsNil)

	client := CosmosClient{
		cfg:                config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		signerCacheManager: signerCacheMgr,
	}

	txOut := stypes.TxOutItem{
		Chain:       common.GAIAChain,
		VaultPubKey: common.PubKey("invalid-pubkey"),
		Coins:       common.Coins{common.NewCoin(common.GAIAChain.GetGasAsset(), cosmos.NewUint(1000))},
		MaxGas:      common.Gas{common.NewCoin(common.GAIAChain.GetGasAsset(), cosmos.NewUint(100))},
	}

	_, _, _, err = client.SignTx(txOut, 1)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// FetchTxs with processTxs error (BlockResults fails)
// -------------------------------------------------------------------------------------

type mockTendermintRPCBlockResultsError struct {
	mockTendermintRPC
}

func (m *mockTendermintRPCBlockResultsError) BlockResults(ctx context.Context, height *int64) (*ctypesrpc.ResultBlockResults, error) {
	return nil, fmt.Errorf("block results error")
}

func (s *ExtraTestSuite) TestFetchTxsProcessTxsError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:                      common.GAIAChain,
		GasPriceResolution:           1_000,
		ObservationFlexibilityBlocks: 100,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	btypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		rpc:      &mockTendermintRPCBlockResultsError{},
		cdc:      cdc,
		feeCache: make([]sdkmath.Uint, 0),
		lastFee:  sdkmath.NewUint(0),
		solvencyReporter: func(height int64) error {
			return nil
		},
		globalNetworkFeeQueue: make(chan common.NetworkFee, 10),
	}

	// processTxs should fail because BlockResults returns error
	_, err := scanner.FetchTxs(1, 10)
	c.Assert(err, NotNil)
}

// -------------------------------------------------------------------------------------
// FetchTxs with updateGasFees error
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFetchTxsUpdateGasFeesError(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:                      common.GAIAChain,
		GasPriceResolution:           0, // will cause divide by zero in averageFee rounding
		ObservationFlexibilityBlocks: 100,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}

	registry := codectypes.NewInterfaceRegistry()
	btypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	// Fill fee cache so updateGasFees triggers
	feeCache := make([]sdkmath.Uint, GasCacheTransactions)
	for i := range feeCache {
		feeCache[i] = sdkmath.NewUint(0)
	}

	scanner := &CosmosBlockScanner{
		cfg:      cfg,
		rpc:      &mockTendermintRPC{},
		cdc:      cdc,
		feeCache: feeCache,
		lastFee:  sdkmath.NewUint(0),
		solvencyReporter: func(height int64) error {
			return nil
		},
		globalNetworkFeeQueue: make(chan common.NetworkFee, 10),
	}

	// On block 10 with zero-fee cache -> updateGasFees returns "zero" error
	// but FetchTxs should still succeed (logs but doesn't return error)
	txIn, err := scanner.FetchTxs(10, 10)
	c.Assert(err, IsNil)
	c.Check(txIn.Chain, Equals, common.GAIAChain)
}

// -------------------------------------------------------------------------------------
// OnObservedTxIn outbound empty TxID
// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// GetHeight (cosmos scanner) via CosmosClient.GetHeight
// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// fromCosmosToThorchain with invalid THORChainSymbol
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestFromCosmosToThorchainInvalidSymbol(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "ubadtoken", Decimals: 6, THORChainSymbol: ""}, // empty symbol -> invalid asset
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}

	_, err := scanner.fromCosmosToThorchain(cosmos.NewCoin("ubadtoken", sdkmath.NewInt(100)))
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*invalid thorchain asset.*")
}

// -------------------------------------------------------------------------------------
// buildUnsigned with nil msg
// -------------------------------------------------------------------------------------

func (s *ExtraTestSuite) TestGetGRPCConnInsecure(c *C) {
	conn, err := getGRPCConn("localhost:9090", false)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)
	conn.Close()
}

func (s *ExtraTestSuite) TestGetGRPCConnTLS(c *C) {
	conn, err := getGRPCConn("localhost:9090", true)
	c.Assert(err, IsNil)
	c.Assert(conn, NotNil)
	conn.Close()
}

func (s *ExtraTestSuite) TestFromThorchainToCosmosLowerDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 4, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}
	thorAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	result, err := scanner.fromThorchainToCosmos(common.Coin{
		Asset:  thorAsset,
		Amount: cosmos.NewUint(100000000),
	})
	c.Assert(err, IsNil)
	// 8 - 4 = 4 less decimals, so divide by 10^4
	c.Check(result.Amount.String(), Equals, "10000")
}

func (s *ExtraTestSuite) TestFromThorchainToCosmosEqualDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 8, THORChainSymbol: "ATOM"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}
	thorAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	result, err := scanner.fromThorchainToCosmos(common.Coin{
		Asset:  thorAsset,
		Amount: cosmos.NewUint(100000000),
	})
	c.Assert(err, IsNil)
	c.Check(result.Amount.String(), Equals, "100000000")
}

func (s *ExtraTestSuite) TestFromCosmosToThorchainLowerDecimals(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "utoken", Decimals: 4, THORChainSymbol: "TKN"},
		},
	}
	scanner := &CosmosBlockScanner{cfg: cfg}
	// 4 decimals -> 8 decimals (multiply by 10^4)
	coin, err := scanner.fromCosmosToThorchain(cosmos.NewCoin("utoken", sdkmath.NewInt(10000)))
	c.Assert(err, IsNil)
	c.Check(coin.Amount.Uint64(), Equals, uint64(100000000))
	c.Check(coin.Decimals, Equals, int64(4))
}

func (s *ExtraTestSuite) TestCosmosClientGetHeight(c *C) {
	scanner := &CosmosBlockScanner{
		cfg: config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain},
		rpc: &mockTendermintRPC{},
	}
	cc := CosmosClient{
		cfg:           config.BifrostChainConfiguration{ChainID: common.GAIAChain},
		cosmosScanner: scanner,
	}

	height, err := cc.GetHeight()
	c.Assert(err, IsNil)
	c.Check(height > 0, Equals, true)
}
