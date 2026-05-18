package keeperv1

import (
	"gitlab.com/thorchain/thornode/v3/common"

	. "gopkg.in/check.v1"
)

type KeeperPOLReserveSuite struct{}

var _ = Suite(&KeeperPOLReserveSuite{})

const polReserveTestUSDC = "ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48"

func polReserveTestNonGasAsset(c *C) common.Asset {
	asset, err := common.NewAsset(polReserveTestUSDC)
	c.Assert(err, IsNil)
	return asset
}

// TestIsPOLReserveEligiblePool_DefaultGasAsset ensures that, with no mimirs
// set, any gas asset is accepted into the POL Reserve default set.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_DefaultGasAsset(c *C) {
	ctx, k := setupKeeperForTest(c)

	c.Check(k.IsPOLReserveEligiblePool(ctx, common.BTCAsset), Equals, true)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.ETHAsset), Equals, true)
}

// TestIsPOLReserveEligiblePool_DefaultNonGasExcluded ensures that arbitrary
// non-gas, non-anchor tokens are excluded by default.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_DefaultNonGasExcluded(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_TorAnchorPromotes covers the fourth eligibility
// rule: any non-gas pool whose TorAnchor-* mimir is > 0 joins the default set.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_TorAnchorPromotes(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	k.SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, true)
}

// TestIsPOLReserveEligiblePool_TorAnchorZeroDoesNotPromote guards against
// treating a cleared (zero) or unset (-1) TorAnchor mimir as an anchor.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_TorAnchorZeroDoesNotPromote(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	k.SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 0)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)

	k.SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", -1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_WhitelistForceIncludes confirms the whitelist
// override on a pool that would otherwise be excluded.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_WhitelistForceIncludes(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	k.SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, true)
}

// TestIsPOLReserveEligiblePool_WhitelistZeroIsNoop guards against non-positive
// whitelist values changing eligibility.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_WhitelistZeroIsNoop(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)

	k.SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 0)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)

	k.SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", -1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_WhitelistPerAsset ensures a whitelist entry on
// one asset does not leak eligibility to another asset.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_WhitelistPerAsset(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)

	// Whitelist BTC.BTC (it's already a gas asset) — USDC must remain excluded.
	k.SetMimir(ctx, "POLReserveWhitelist-BTC-BTC", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_BlacklistOverridesGasDefault confirms the
// blacklist beats the gas-asset default (highest precedence rule).
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_BlacklistOverridesGasDefault(c *C) {
	ctx, k := setupKeeperForTest(c)

	k.SetMimir(ctx, "POLReserveBlacklist-BTC-BTC", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.BTCAsset), Equals, false)
}

// TestIsPOLReserveEligiblePool_BlacklistOverridesTorAnchor confirms the
// blacklist beats a TorAnchor promotion.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_BlacklistOverridesTorAnchor(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	k.SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	k.SetMimir(ctx, "POLReserveBlacklist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_BlacklistOverridesWhitelist confirms blacklist
// has strictly higher precedence than whitelist when both are set.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_BlacklistOverridesWhitelist(c *C) {
	ctx, k := setupKeeperForTest(c)

	usdc := polReserveTestNonGasAsset(c)
	k.SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	k.SetMimir(ctx, "POLReserveBlacklist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, usdc), Equals, false)
}

// TestIsPOLReserveEligiblePool_BlacklistZeroIsNoop guards against non-positive
// blacklist values blocking eligibility.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_BlacklistZeroIsNoop(c *C) {
	ctx, k := setupKeeperForTest(c)

	k.SetMimir(ctx, "POLReserveBlacklist-BTC-BTC", 0)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.BTCAsset), Equals, true)

	k.SetMimir(ctx, "POLReserveBlacklist-BTC-BTC", -1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.BTCAsset), Equals, true)
}

// TestIsPOLReserveEligiblePool_BlacklistPerAsset ensures a blacklist entry on
// one asset does not leak exclusion to another asset.
func (s *KeeperPOLReserveSuite) TestIsPOLReserveEligiblePool_BlacklistPerAsset(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Blacklist ETH.ETH; BTC.BTC must remain eligible.
	k.SetMimir(ctx, "POLReserveBlacklist-ETH-ETH", 1)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.BTCAsset), Equals, true)
	c.Check(k.IsPOLReserveEligiblePool(ctx, common.ETHAsset), Equals, false)
}
