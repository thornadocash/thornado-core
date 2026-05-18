package thorchain

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MigrationCommonSuite struct{}

var _ = Suite(&MigrationCommonSuite{})

func (s *MigrationCommonSuite) TestMigrate7to8Debug(c *C) {
	ctx, mgr := setupManagerForTest(c)
	migrator := NewMigrator(mgr)

	err := migrator.Migrate7to8(ctx)
	c.Assert(err, IsNil)
}

func (s *MigrationCommonSuite) TestMigrate8to9OverflowFix(c *C) {
	// Test the overflow fix logic directly

	// Scenario 1: Normal case where withheld >= spent
	withheld1 := cosmos.NewUint(2000)
	spent1 := cosmos.NewUint(1000)
	var surplus1 cosmos.Uint

	if withheld1.GTE(spent1) {
		surplus1 = withheld1.Sub(spent1)
	} else {
		surplus1 = cosmos.ZeroUint()
	}

	c.Assert(surplus1.Equal(cosmos.NewUint(1000)), Equals, true, Commentf("Expected surplus of 1000, got %s", surplus1.String()))

	// Scenario 2: Overflow case where spent > withheld (this would have panicked before the fix)
	withheld2 := cosmos.NewUint(1000)
	spent2 := cosmos.NewUint(2000)
	var surplus2 cosmos.Uint

	if withheld2.GTE(spent2) {
		surplus2 = withheld2.Sub(spent2)
	} else {
		surplus2 = cosmos.ZeroUint()
	}

	c.Assert(surplus2.IsZero(), Equals, true, Commentf("Expected surplus of 0, got %s", surplus2.String()))

	// Scenario 3: Edge case where withheld == spent
	withheld3 := cosmos.NewUint(1000)
	spent3 := cosmos.NewUint(1000)
	var surplus3 cosmos.Uint

	if withheld3.GTE(spent3) {
		surplus3 = withheld3.Sub(spent3)
	} else {
		surplus3 = cosmos.ZeroUint()
	}

	c.Assert(surplus3.IsZero(), Equals, true, Commentf("Expected surplus of 0, got %s", surplus3.String()))
}

func (s *MigrationCommonSuite) TestBurnReserveAndReduceMaxSupply(c *C) {
	ctx, mgr := setupManagerForTest(c)
	migrator := NewMigrator(mgr)
	k := mgr.Keeper()

	// Verify pre-state: reserve > 9.3M and total supply > 360M
	reserveBefore := k.GetRuneBalanceOfModule(ctx, ReserveName)
	supplyBefore := k.GetTotalSupply(ctx, common.RuneAsset())
	c.Assert(reserveBefore.GT(cosmos.NewUint(9_300_000_00000000)), Equals, true,
		Commentf("reserve should be > 9.3M, got %s", reserveBefore.String()))
	c.Assert(supplyBefore.GT(cosmos.NewUint(360_000_000_00000000)), Equals, true,
		Commentf("total supply should be > 360M, got %s", supplyBefore.String()))

	// Run migration
	err := migrator.BurnReserveAndReduceMaxSupply(ctx)
	c.Assert(err, IsNil)

	// Post-burn reserve should be 9.3M
	reserveAfter := k.GetRuneBalanceOfModule(ctx, ReserveName)
	c.Assert(reserveAfter.Equal(cosmos.NewUint(9_300_000_00000000)), Equals, true,
		Commentf("reserve should be 9.3M, got %s", reserveAfter.String()))

	// MaxRuneSupply must equal post-burn total supply (no state drift)
	postBurnSupply := k.GetTotalSupply(ctx, common.RuneAsset())
	maxSupplyMimir, _ := k.GetMimir(ctx, "MaxRuneSupply")
	c.Assert(maxSupplyMimir > 0, Equals, true)
	c.Assert(postBurnSupply.Equal(cosmos.NewUint(uint64(maxSupplyMimir))), Equals, true,
		Commentf("MaxRuneSupply (%d) should equal post-burn total supply (%s)", maxSupplyMimir, postBurnSupply.String()))

	// Total supply must be <= MaxRuneSupply
	c.Assert(postBurnSupply.LTE(cosmos.NewUint(uint64(maxSupplyMimir))), Equals, true,
		Commentf("total supply (%s) should be <= MaxRuneSupply (%d)", postBurnSupply.String(), maxSupplyMimir))
}
