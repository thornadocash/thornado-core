package xrp

import (
	sdkmath "cosmossdk.io/math"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"

	txtypes "github.com/Peersyst/xrpl-go/xrpl/transaction/types"
)

type UtilTestSuite struct{}

var _ = Suite(&UtilTestSuite{})

func (s *UtilTestSuite) SetUpSuite(c *C) {}

func (s *UtilTestSuite) TestFromXrpToThorchain(c *C) {
	// 5 XRP, 6 decimals
	thorchainCoin, err := fromXrpToThorchain(txtypes.XRPCurrencyAmount(uint64(5000000)))
	c.Assert(err, IsNil)

	// 5 XRP, 8 decimals
	expectedCoin := common.Coin{
		Asset:    common.XRPAsset,
		Amount:   sdkmath.NewUint(500000000),
		Decimals: 6,
	}
	c.Check(thorchainCoin.Asset.Equals(expectedCoin.Asset), Equals, true)
	c.Check(thorchainCoin.Amount.String(), Equals, expectedCoin.Amount.String())
	c.Check(thorchainCoin.Decimals, Equals, expectedCoin.Decimals)
}

func (s *UtilTestSuite) TestFromThorchainToXrp(c *C) {
	// 6 XRP, 8 decimals
	thorchainCoin := common.NewCoin(common.XRPAsset, sdkmath.NewUint(600000000))
	xrpCurrency, err := fromThorchainToXrp(thorchainCoin)
	c.Assert(err, IsNil)

	// 6 XRP, 6 decimals
	xrpCoin, ok := xrpCurrency.(txtypes.XRPCurrencyAmount)
	c.Check(ok, Equals, true)
	c.Check(xrpCoin, Equals, txtypes.XRPCurrencyAmount(6000000))
}
