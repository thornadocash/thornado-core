package gaia

import (
	"fmt"
	"sort"

	ibccoretypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	ibclightclient "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	ctypes "github.com/cosmos/cosmos-sdk/types"
	btypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	protov2 "google.golang.org/protobuf/proto"

	"github.com/rs/zerolog/log"
	"gitlab.com/thorchain/thornode/v3/bifrost/metrics"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"

	"gitlab.com/thorchain/thornode/v3/cmd"
	. "gopkg.in/check.v1"
)

// -------------------------------------------------------------------------------------
// Mock FeeTx
// -------------------------------------------------------------------------------------

var _ ctypes.FeeTx = &MockFeeTx{}

type MockFeeTx struct {
	fee ctypes.Coins
	gas uint64
}

func (m *MockFeeTx) GetMsgs() []ctypes.Msg {
	return nil
}

func (m *MockFeeTx) GetMsgsV2() ([]protov2.Message, error) {
	return nil, nil
}

func (m *MockFeeTx) ValidateBasic() error {
	return nil
}

func (m *MockFeeTx) GetGas() uint64 {
	return m.gas
}

func (m *MockFeeTx) GetFee() ctypes.Coins {
	return m.fee
}

func (m *MockFeeTx) FeePayer() []byte {
	return nil
}

func (m *MockFeeTx) FeeGranter() []byte {
	return nil
}

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

func (s *BlockScannerTestSuite) TestCalculateAverageGasFees(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 1_000, // 1,000 uatom
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	blockScanner := CosmosBlockScanner{cfg: cfg}

	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit / 2,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
	})
	c.Check(len(blockScanner.feeCache), Equals, 1)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(20000)))

	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit / 2,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
	})
	c.Check(len(blockScanner.feeCache), Equals, 2)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(20000)))

	// two blocks at half fee should average to 75% of last
	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
	})
	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit,
		fee: ctypes.Coins{ctypes.NewCoin("uatom", sdkmath.NewInt(10000))},
	})
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15000)))

	// skip transactions with multiple coins
	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit,
		fee: ctypes.Coins{
			ctypes.NewCoin("uatom", sdkmath.NewInt(10000)),
			ctypes.NewCoin("uusd", sdkmath.NewInt(10000)),
		},
	})
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15000)))

	// skip transactions with fees not in uatom
	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit,
		fee: ctypes.Coins{
			ctypes.NewCoin("uusd", sdkmath.NewInt(10000)),
		},
	})
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15000)))

	// skip transactions with zero fee
	blockScanner.updateGasCache(&MockFeeTx{
		gas: GasLimit,
		fee: ctypes.Coins{
			ctypes.NewCoin("uusd", sdkmath.NewInt(0)),
		},
	})
	c.Check(len(blockScanner.feeCache), Equals, 4)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(15000)))

	// ensure we only cache the transaction limit number of blocks
	for i := 0; i < GasCacheTransactions; i++ {
		blockScanner.updateGasCache(&MockFeeTx{
			gas: GasLimit,
			fee: ctypes.Coins{
				ctypes.NewCoin("uatom", sdkmath.NewInt(10000)),
			},
		})
	}
	c.Check(len(blockScanner.feeCache), Equals, GasCacheTransactions)
	c.Check(blockScanner.averageFee().String(), Equals, fmt.Sprintf("%d", uint64(10000)))
}

func (s *BlockScannerTestSuite) TestGetBlock(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{ChainID: common.GAIAChain}
	blockScanner := CosmosBlockScanner{
		cfg: cfg,
		rpc: &mockTendermintRPC{},
	}

	block, err := blockScanner.GetBlock(1)

	c.Assert(err, IsNil)
	c.Assert(len(block.Data.Txs), Equals, 1)
	c.Assert(block.Header.Height, Equals, int64(6509672))
}

func (s *BlockScannerTestSuite) TestProcessTxs(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID: common.GAIAChain,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
			{Denom: "ibc/5DB84693CB3F67A2D25D133C177B06786F423D6D1AFF2B8E080F0753B2E4D585", Decimals: 6, THORChainSymbol: "NTRN"},
		},
	}
	registry := s.bridge.GetContext().InterfaceRegistry
	btypes.RegisterInterfaces(registry)
	ibcchanneltypes.RegisterInterfaces(registry)
	ibccoretypes.RegisterInterfaces(registry)
	ibclightclient.RegisterInterfaces(registry)

	cdc := codec.NewProtoCodec(registry)

	blockScanner := CosmosBlockScanner{
		cfg:    cfg,
		rpc:    &mockTendermintRPC{},
		cdc:    cdc,
		logger: log.Logger.With().Str("module", "blockscanner").Str("chain", common.GAIAChain.String()).Logger(),
	}

	block, err := blockScanner.GetBlock(1)
	c.Assert(err, IsNil)

	txInItems, err := blockScanner.processTxs(1, block.Data.Txs)
	c.Assert(err, IsNil)

	// proccessTxs should filter out everything besides the valid MsgSend
	c.Assert(len(txInItems), Equals, 1)

	// single tx in single ibc message
	// ------------------------------------------------------------------------

	block, err = blockScanner.GetBlock(11350886)
	c.Assert(err, IsNil)

	txInItems, err = blockScanner.processTxs(11350886, block.Data.Txs)
	c.Assert(err, IsNil)

	c.Assert(len(txInItems), Equals, 1)
	c.Assert(txInItems[0].Tx, Equals, "935638acbd9be5ae7861ad6932d1967a35db0e682f602a275611e5ed62d70cae")

	// multiple tx in single ibc message
	// ------------------------------------------------------------------------

	block, err = blockScanner.GetBlock(11350935)
	c.Assert(err, IsNil)

	txInItems, err = blockScanner.processTxs(11350935, block.Data.Txs)
	c.Assert(err, IsNil)

	c.Assert(len(txInItems), Equals, 2)
	c.Assert(txInItems[0].Tx, Equals, "17471479c1cb868818dfdfde25711f13ec448ffe08893cb47b4c7b60b1429b25-0")
	c.Assert(txInItems[1].Tx, Equals, "17471479c1cb868818dfdfde25711f13ec448ffe08893cb47b4c7b60b1429b25-1")

	// two ibc txs sending atom back to gaia (from osmosis & secret)
	// ------------------------------------------------------------------------

	block, err = blockScanner.GetBlock(26750027)
	c.Assert(err, IsNil)

	txInItems, err = blockScanner.processTxs(26750027, block.Data.Txs)
	c.Assert(err, IsNil)

	c.Assert(len(txInItems), Equals, 2)

	sort.Slice(txInItems, func(i, j int) bool {
		return txInItems[i].Tx < txInItems[j].Tx
	})

	c.Assert(txInItems[0].Tx, Equals, "5a9eedadf67048dc569eec2fa312a8f85393afdf5f0e84b629ffd2587abad73e")
	c.Assert(txInItems[1].Tx, Equals, "7144c5fc683482e93d373783832a0cd460b21e1d4dfc2770b3cb76732edf6897")

	// ibc usdc from noble to gaia + atom transfer
	// ------------------------------------------------------------------------

	block, err = blockScanner.GetBlock(26757930)
	c.Assert(err, IsNil)

	txInItems, err = blockScanner.processTxs(26757930, block.Data.Txs)
	c.Assert(err, IsNil)

	// only reports atom tx, usdc is not whitelisted
	c.Assert(len(txInItems), Equals, 1)

	c.Assert(txInItems[0].Tx, Equals, "d1490eb0303aa7f4612f46826230b14102ab6d3ee77645c3710ed53be31ad230")
}
