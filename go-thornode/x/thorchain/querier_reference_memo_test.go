package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
	. "gopkg.in/check.v1"
)

type QuerierReferenceMemoSuite struct{}

var _ = Suite(&QuerierReferenceMemoSuite{})

func (s *QuerierReferenceMemoSuite) TestPreflightGasAssetEligible(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Create BTC pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Enable memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Gas asset should pass preflight
	resp, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
		Asset:  "BTC.BTC",
		Amount: "100012345",
	})
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Available, Equals, true)
	c.Assert(resp.CanRegister, Equals, true)
}

func (s *QuerierReferenceMemoSuite) TestPreflightNonGasAssetEligible(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Enable memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// ERC20 token (non-gas asset) should pass preflight when pool exists.
	asset, err := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(err, IsNil)

	pool := NewPool()
	pool.Asset = asset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Decimals = 6
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	resp, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
		Asset:  asset.String(),
		Amount: "100012345",
	})
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Available, Equals, true)
	c.Assert(resp.CanRegister, Equals, true)
}

func (s *QuerierReferenceMemoSuite) TestPreflightSynthAssetEligible(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Enable memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Synth asset should pass preflight when pool exists.
	asset, err := common.NewAsset("BTC/BTC")
	c.Assert(err, IsNil)

	pool := NewPool()
	pool.Asset = asset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	resp, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
		Asset:  "BTC/BTC",
		Amount: "100012345",
	})
	c.Assert(err, IsNil)
	c.Assert(resp, NotNil)
	c.Assert(resp.Available, Equals, true)
	c.Assert(resp.CanRegister, Equals, true)
}

func (s *QuerierReferenceMemoSuite) TestPreflightMemolessHalted(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Enable memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Halt memoless
	mgr.Keeper().SetMimir(ctx, constants.HaltMemoless.String(), 1)

	_, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
		Asset:  "BTC.BTC",
		Amount: "100012345",
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*currently halted.*")
}

func (s *QuerierReferenceMemoSuite) TestPreflightMemolessDisabled(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Set TTL to 0 to disable memoless
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 0)

	_, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
		Asset:  "BTC.BTC",
		Amount: "100012345",
	})
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*currently disabled.*")
}

func (s *QuerierReferenceMemoSuite) TestPreflightMultipleGasAssets(c *C) {
	ctx, mgr := setupManagerForTest(c)
	qs := &queryServer{mgr: mgr}

	// Enable memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Test multiple gas assets pass preflight
	gasAssets := []common.Asset{
		common.BTCAsset,
		common.ETHAsset,
		common.DOGEAsset,
	}

	for _, asset := range gasAssets {
		pool := NewPool()
		pool.Asset = asset
		pool.BalanceAsset = cosmos.NewUint(100 * common.One)
		pool.BalanceRune = cosmos.NewUint(100 * common.One)
		pool.Decimals = asset.Chain.GetGasAssetDecimal()
		c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

		resp, err := qs.queryReferenceMemoPreflight(ctx, &types.QueryReferenceMemoPreflightRequest{
			Asset:  asset.String(),
			Amount: "100012345",
		})
		c.Assert(err, IsNil, Commentf("gas asset %s should pass preflight", asset))
		c.Assert(resp, NotNil)
		c.Assert(resp.Available, Equals, true)
	}
}
