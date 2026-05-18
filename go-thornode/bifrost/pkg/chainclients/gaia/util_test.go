package gaia

import (
	sdkmath "cosmossdk.io/math"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/config"
	. "gopkg.in/check.v1"
)

type UtilTestSuite struct {
	scanner CosmosBlockScanner
}

var _ = Suite(&UtilTestSuite{})

func (s *UtilTestSuite) SetUpSuite(c *C) {
	cfg := config.BifrostBlockScannerConfiguration{
		ChainID:            common.GAIAChain,
		GasPriceResolution: 100_000,
		WhitelistCosmosAssets: []config.WhitelistCosmosAsset{
			{Denom: "uatom", Decimals: 6, THORChainSymbol: "ATOM"},
		},
	}
	s.scanner = CosmosBlockScanner{cfg: cfg}
}

func (s *UtilTestSuite) TestFromCosmosToThorchain(c *C) {
	// 5 ATOM, 6 decimals
	cosmosCoin := cosmos.NewCoin("uatom", sdkmath.NewInt(5000000))
	thorchainCoin, err := s.scanner.fromCosmosToThorchain(cosmosCoin)
	c.Assert(err, IsNil)

	// 5 ATOM, 8 decimals
	expectedThorchainAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	expectedThorchainAmount := sdkmath.NewUint(500000000)
	c.Check(thorchainCoin.Asset.Equals(expectedThorchainAsset), Equals, true)
	c.Check(thorchainCoin.Amount.BigInt().Int64(), Equals, expectedThorchainAmount.BigInt().Int64())
	c.Check(thorchainCoin.Decimals, Equals, int64(6))
}

func (s *UtilTestSuite) TestFromThorchainToCosmos(c *C) {
	// 6 GAIA.ATOM, 8 decimals
	thorchainAsset, err := common.NewAsset("GAIA.ATOM")
	c.Assert(err, IsNil)
	thorchainCoin := common.Coin{
		Asset:    thorchainAsset,
		Amount:   cosmos.NewUint(600000000),
		Decimals: 6,
	}
	cosmosCoin, err := s.scanner.fromThorchainToCosmos(thorchainCoin)
	c.Assert(err, IsNil)

	// 6 uatom, 6 decimals
	expectedCosmosDenom := "uatom"
	expectedCosmosAmount := int64(6000000)
	c.Check(cosmosCoin.Denom, Equals, expectedCosmosDenom)
	c.Check(cosmosCoin.Amount.Int64(), Equals, expectedCosmosAmount)
}
