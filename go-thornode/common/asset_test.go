package common

import (
	"strings"

	"github.com/blang/semver"
	. "gopkg.in/check.v1"
)

type AssetSuite struct{}

var _ = Suite(&AssetSuite{})

func (s AssetSuite) TestAsset(c *C) {
	asset, err := NewAsset("thor.rune")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Equals(RuneNative), Equals, true)
	c.Check(asset.IsRune(), Equals, true)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.Synth, Equals, false)
	c.Check(asset.String(), Equals, "THOR.RUNE")

	asset, err = NewAsset("thor/rune")
	c.Assert(err, IsNil)
	err = asset.Valid()
	c.Check(err, NotNil)
	c.Check(err.Error(), Equals, "synth asset cannot have chain THOR: THOR/RUNE")

	asset, err = NewAsset("ETH.SWIPE.B-DC0")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.String(), Equals, "ETH.SWIPE.B-DC0")
	c.Check(asset.Chain.Equals(ETHChain), Equals, true)
	c.Check(asset.Symbol.Equals(Symbol("SWIPE.B-DC0")), Equals, true)
	c.Check(asset.Ticker.Equals(Ticker("SWIPE.B")), Equals, true)

	// parse without chain
	asset, err = NewAsset("rune")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Equals(RuneNative), Equals, true)

	// ETH test
	asset, err = NewAsset("eth.knc")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(ETHChain), Equals, true)
	c.Check(asset.Symbol.Equals(Symbol("KNC")), Equals, true)
	c.Check(asset.Ticker.Equals(Ticker("KNC")), Equals, true)
	asset, err = NewAsset("ETH.RUNE-0x3155ba85d5f96b2d030a4966af206230e46849cb")
	c.Assert(err, IsNil)

	// DOGE test
	asset, err = NewAsset("doge.doge")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(DOGEChain), Equals, true)
	c.Check(asset.Equals(DOGEAsset), Equals, true)
	c.Check(asset.IsRune(), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "DOGE.DOGE")

	// BCH test
	asset, err = NewAsset("bch.bch")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(BCHChain), Equals, true)
	c.Check(asset.Equals(BCHAsset), Equals, true)
	c.Check(asset.IsRune(), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "BCH.BCH")

	// LTC test
	asset, err = NewAsset("ltc.ltc")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(LTCChain), Equals, true)
	c.Check(asset.Equals(LTCAsset), Equals, true)
	c.Check(asset.IsRune(), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "LTC.LTC")

	// GAIA test
	asset, err = NewAsset("gaia.atom")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(GAIAChain), Equals, true)
	c.Check(asset.Equals(ATOMAsset), Equals, true)
	c.Check(asset.IsRune(), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "GAIA.ATOM")

	// NOBLE test
	asset, err = NewAsset("noble.usdc")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(NOBLEChain), Equals, true)
	c.Check(asset.Equals(USDCAsset), Equals, true)
	c.Check(asset.IsRune(), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "NOBLE.USDC")
	c.Check(asset.IsGasAsset(), Equals, true) // USDC is gas asset for Noble

	// btc/btc
	asset, err = NewAsset("btc/btc")
	c.Check(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(BTCChain), Equals, true)
	c.Check(asset.Equals(BTCAsset), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "BTC/BTC")

	// btc~btc
	asset, err = NewAsset("btc~btc")
	c.Check(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.Chain.Equals(BTCChain), Equals, true)
	c.Check(asset.Equals(BTCAsset), Equals, false)
	c.Check(asset.IsEmpty(), Equals, false)
	c.Check(asset.String(), Equals, "BTC~BTC")

	asset, err = NewAsset("thor~rune")
	c.Assert(err, IsNil)
	err = asset.Valid()
	c.Check(err, NotNil)
	c.Check(err.Error(), Equals, "trade asset cannot have chain THOR: THOR~RUNE")

	// btc~btc with invalid synth flag
	asset.Synth = true
	err = asset.Valid()
	c.Check(err, NotNil)
	c.Check(err.Error(), Equals, "assets can only be one of trade, synth or secured")

	asset.Synth = false
	asset.Secured = true
	err = asset.Valid()
	c.Check(err, NotNil)
	c.Check(err.Error(), Equals, "assets can only be one of trade, synth or secured")

	// test shorts
	asset, err = NewAssetWithShortCodes(semver.MustParse("999.0.0"), "b")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.String(), Equals, "BTC.BTC")
	asset, err = NewAssetWithShortCodes(semver.MustParse("999.0.0"), "BLAH.BLAH")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.String(), Equals, "BLAH.BLAH")

	asset, err = NewAssetWithShortCodes(semver.MustParse("3.0.0"), "BTC.BTC")
	c.Assert(err, IsNil)
	c.Check(asset.Valid(), IsNil)
	c.Check(asset.String(), Equals, "BTC.BTC")

	asset = Asset{
		Chain:  "THOR.ETH",
		Symbol: "ETH",
	}
	err = asset.Valid()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "invalid chain"), Equals, true)

	asset = Asset{
		Chain:  "THOR",
		Symbol: "ETH~ETH",
	}
	err = asset.Valid()
	c.Assert(err, NotNil)
	c.Check(strings.Contains(err.Error(), "invalid symbol"), Equals, true)

	asset, err = NewAsset("ETH-RUNE-0x3155ba85d5f96b2d030a4966af206230e46849cb")
	c.Assert(err, IsNil)
	c.Check(asset.IsNative(), Equals, true)
	c.Check(asset.IsSecuredAsset(), Equals, true)
	c.Check(asset.IsTradeAsset(), Equals, false)
	c.Check(asset.IsSyntheticAsset(), Equals, false)
	c.Check(asset.IsExternalL1Asset(), Equals, false)
	c.Assert(asset.Chain, Equals, ETHChain)
	c.Check(asset.Symbol.Equals(Symbol("RUNE-0X3155BA85D5F96B2D030A4966AF206230E46849CB")), Equals, true)
	c.Check(asset.Ticker.Equals(Ticker("RUNE")), Equals, true)

	asset, err = NewAsset("ETH.RUNE-0x3155ba85d5f96b2d030a4966af206230e46849cb")
	c.Assert(err, IsNil)
	c.Check(asset.IsNative(), Equals, false)
	c.Check(asset.IsSecuredAsset(), Equals, false)
	c.Check(asset.IsTradeAsset(), Equals, false)
	c.Check(asset.IsSyntheticAsset(), Equals, false)
	c.Check(asset.IsExternalL1Asset(), Equals, true)
	c.Assert(asset.Chain, Equals, ETHChain)
	c.Check(asset.Symbol.Equals(Symbol("RUNE-0X3155BA85D5F96B2D030A4966AF206230E46849CB")), Equals, true)
	c.Check(asset.Ticker.Equals(Ticker("RUNE")), Equals, true)

	// Ensure that x/denom assets are not interpreted as synth assets
	asset, err = NewAsset("x/custom")
	c.Assert(err, NotNil)

	// Ensure that x/denom assets cannot be interpreted by MsgDeposit
	asset, err = NewAsset("THOR.X/CUSTOM")
	c.Assert(err, NotNil)
}
