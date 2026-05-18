package keeperv1

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type KeeperBackfillSuite struct{}

var _ = Suite(&KeeperBackfillSuite{})

// ---------------------------------------------------------------------------
// Oracle Price tests
// ---------------------------------------------------------------------------

func (s *KeeperBackfillSuite) TestOraclePrice(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Get non-existent price
	_, err := k.GetPrice(ctx, "BTC")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*Price not found.*")

	// Set and get a price
	price := OraclePrice{
		Symbol: "BTC",
		Price:  "50000",
	}
	err = k.SetPrice(ctx, price)
	c.Assert(err, IsNil)

	got, err := k.GetPrice(ctx, "BTC")
	c.Assert(err, IsNil)
	c.Assert(got.Symbol, Equals, "BTC")

	// Delete price
	k.DelPrice(ctx, "BTC")
	_, err = k.GetPrice(ctx, "BTC")
	c.Assert(err, NotNil)

	// Iterator
	iter := k.GetPriceIterator(ctx)
	c.Assert(iter, NotNil)
	iter.Close()
}

// ---------------------------------------------------------------------------
// Secured Asset tests
// ---------------------------------------------------------------------------

func (s *KeeperBackfillSuite) TestSecuredAsset(c *C) {
	ctx, k := setupKeeperForTest(c)
	asset := common.BTCAsset

	// Get non-existent secured asset (returns zero-value, no error)
	sa, err := k.GetSecuredAsset(ctx, asset)
	c.Assert(err, IsNil)
	c.Assert(sa.Asset.Equals(asset), Equals, true)

	// Set secured asset
	sa.Depth = cosmos.NewUint(1000)
	k.SetSecuredAsset(ctx, sa)

	got, err := k.GetSecuredAsset(ctx, asset)
	c.Assert(err, IsNil)
	c.Assert(got.Depth.Equal(cosmos.NewUint(1000)), Equals, true)

	// Iterator
	iter := k.GetSecuredAssetIterator(ctx)
	c.Assert(iter, NotNil)
	iter.Close()
}

// ---------------------------------------------------------------------------
// Solvency Voter tests
// ---------------------------------------------------------------------------

func (s *KeeperBackfillSuite) TestSolvencyVoter(c *C) {
	ctx, k := setupKeeperForTest(c)
	txID := GetRandomTxHash()
	chain := common.ETHChain

	// Get non-existent
	sv, err := k.GetSolvencyVoter(ctx, txID, chain)
	c.Assert(err, IsNil)
	c.Assert(sv.Id.IsEmpty(), Equals, true)

	// Set and get
	voter := SolvencyVoter{
		Id:    txID,
		Chain: chain,
	}
	k.SetSolvencyVoter(ctx, voter)

	got, err := k.GetSolvencyVoter(ctx, txID, chain)
	c.Assert(err, IsNil)
	c.Assert(got.Id.Equals(txID), Equals, true)
	c.Assert(got.Chain.Equals(chain), Equals, true)
}

// ---------------------------------------------------------------------------
// RUNE Pool tests
// ---------------------------------------------------------------------------

func (s *KeeperBackfillSuite) TestRUNEPool(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Get default (empty)
	pool, err := k.GetRUNEPool(ctx)
	c.Assert(err, IsNil)
	c.Assert(pool.ReserveUnits.IsZero(), Equals, true)

	// Set
	pool.ReserveUnits = cosmos.NewUint(1000)
	k.SetRUNEPool(ctx, pool)

	got, err := k.GetRUNEPool(ctx)
	c.Assert(err, IsNil)
	c.Assert(got.ReserveUnits.Equal(cosmos.NewUint(1000)), Equals, true)
}

func (s *KeeperBackfillSuite) TestRUNEProvider(c *C) {
	ctx, k := setupKeeperForTest(c)
	addr := GetRandomBech32Addr()

	// Get non-existent
	rp, err := k.GetRUNEProvider(ctx, addr)
	c.Assert(err, IsNil)
	c.Assert(rp.RuneAddress.Equals(addr), Equals, true)
	c.Assert(rp.Units.IsZero(), Equals, true)

	// Set
	rp.Units = cosmos.NewUint(500)
	k.SetRUNEProvider(ctx, rp)

	got, err := k.GetRUNEProvider(ctx, addr)
	c.Assert(err, IsNil)
	c.Assert(got.Units.Equal(cosmos.NewUint(500)), Equals, true)

	// Remove
	k.RemoveRUNEProvider(ctx, rp)
	removed, err := k.GetRUNEProvider(ctx, addr)
	c.Assert(err, IsNil)
	c.Assert(removed.Units.IsZero(), Equals, true)

	// Iterator
	iter := k.GetRUNEProviderIterator(ctx)
	c.Assert(iter, NotNil)
	iter.Close()
}

// ---------------------------------------------------------------------------
// Anchors tests
// ---------------------------------------------------------------------------

func (s *KeeperBackfillSuite) TestGetAnchorsNonTOR(c *C) {
	ctx, k := setupKeeperForTest(c)
	_ = k

	// Non-TOR asset should return the layer1 asset
	assets := k.GetAnchors(ctx, common.BTCAsset)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Equals(common.BTCAsset.GetLayer1Asset()), Equals, true)
}

func (s *KeeperBackfillSuite) TestGetAnchorsTOR(c *C) {
	ctx, k := setupKeeperForTest(c)

	// TOR with no pools returns empty
	assets := k.GetAnchors(ctx, common.TOR)
	c.Assert(len(assets), Equals, 0)

	// Set up a pool and TorAnchor mimir
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.Status = types.PoolStatus_Available
	c.Assert(k.SetPool(ctx, pool), IsNil)

	k.SetMimir(ctx, "TorAnchor-BTC-BTC", 1)

	assets = k.GetAnchors(ctx, common.TOR)
	c.Assert(len(assets), Equals, 1)
	c.Assert(assets[0].Equals(common.BTCAsset), Equals, true)
}

func (s *KeeperBackfillSuite) TestDollarsPerRune(c *C) {
	ctx, k := setupKeeperForTest(c)

	// With mimir override
	k.SetMimir(ctx, "DollarsPerRune", 100000000) // 1 USD per rune
	dpr := k.DollarsPerRune(ctx)
	c.Assert(dpr.Equal(cosmos.NewUint(100000000)), Equals, true)
}

func (s *KeeperBackfillSuite) TestRunePerDollar(c *C) {
	ctx, k := setupKeeperForTest(c)

	// With mimir override for dollars per rune
	k.SetMimir(ctx, "DollarsPerRune", 100000000) // 1 USD per rune = 1 rune per USD
	rpd := k.RunePerDollar(ctx)
	c.Assert(rpd.GT(cosmos.ZeroUint()), Equals, true)
}

func (s *KeeperBackfillSuite) TestAnchorMedian(c *C) {
	ctx, k := setupKeeperForTest(c)

	// No assets returns zero
	result := k.AnchorMedian(ctx, nil)
	c.Assert(result.IsZero(), Equals, true)

	// Empty list
	result = k.AnchorMedian(ctx, []common.Asset{})
	c.Assert(result.IsZero(), Equals, true)
}
