package thorchain

import (
	"cosmossdk.io/math"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type NetworkManagerTestSuite struct{}

var _ = Suite(&NetworkManagerTestSuite{})

func (s *NetworkManagerTestSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (s *NetworkManagerTestSuite) TestUpdateNetwork(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)
	helper := NewVaultGenesisSetupTestHelper(mgr.Keeper())
	mgr.K = helper
	networkMgr := newNetworkMgr(helper, mgr.TxOutStore(), mgr.EventMgr())

	// fail to get Network should return error
	helper.failGetNetwork = true
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.gasMgr, mgr.eventMgr), NotNil)
	helper.failGetNetwork = false

	// TotalReserve is zero , should not doing anything
	vd := NewNetwork()
	err := mgr.Keeper().SetNetwork(ctx, vd)
	c.Assert(err, IsNil)
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), IsNil)

	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), IsNil)

	p := NewPool()
	p.Asset = common.ETHAsset
	p.BalanceRune = cosmos.NewUint(common.One * 100)
	p.BalanceAsset = cosmos.NewUint(common.One * 100)
	p.Status = PoolAvailable
	c.Assert(helper.SetPool(ctx, p), IsNil)
	// no active node , thus no bond
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), IsNil)

	// Vault for getVaultsLiquidityRune.
	vault := NewVault(0, ActiveVault, AsgardVault, GetRandomPubKey(), []string{p.Asset.GetChain().String()}, []ChainContract{})
	vault.Coins = common.NewCoins(common.NewCoin(p.Asset, p.BalanceAsset))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// with liquidity fee , and bonds
	c.Assert(helper.Keeper.AddToLiquidityFees(ctx, common.ETHAsset, cosmos.NewUint(50*common.One)), IsNil)

	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), IsNil)
	// add bond
	c.Assert(helper.Keeper.SetNodeAccount(ctx, GetRandomValidatorNode(NodeActive)), IsNil)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, GetRandomValidatorNode(NodeActive)), IsNil)
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), IsNil)

	// fail to get total liquidity fee should result an error
	helper.failGetTotalLiquidityFee = true
	if common.RuneAsset().Equals(common.RuneNative) {
		FundModule(c, ctx, helper, ReserveName, 100*common.One)
	}
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), NotNil)
	helper.failGetTotalLiquidityFee = false

	helper.failToListActiveAccounts = true
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.GasMgr(), mgr.EventMgr()), NotNil)
}

func (s *NetworkManagerTestSuite) TestCalcBlockRewards(c *C) {
	ctx, k := setupKeeperForTest(c)
	mgr := NewDummyMgr()
	networkMgr := newNetworkMgr(k, mgr.TxOutStore(), mgr.EventMgr())

	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)

	// calcBlockRewards arguments: availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees cosmos.Uint, emissionCurve, blocksPerYear int64

	vaultsLiquidityRune := cosmos.NewUint(1000 * common.One)
	availablePoolsRune := vaultsLiquidityRune.QuoUint64(2) // vaultsLiquidityRune used for availablePoolsRune usually, but *1/2 when testing different values.
	effectiveSecurityBond := cosmos.NewUint(2000 * common.One)
	// Equilibrium state where effectiveSecurityBond is double vaultsLiquidityRune,
	// so expecting equal rewards for vaultsLiquidityRune and the effectiveSecurityBond portion of totalEffectiveBond.

	totalEffectiveBond := effectiveSecurityBond.MulUint64(3).QuoUint64(2) // effectiveSecurityBond used for totalEffectiveBond usually, but *3/2 when testing different values.
	totalReserve := cosmos.NewUint(1000 * common.One)
	totalLiquidityFees := cosmos.ZeroUint() // No liquidity fees unless explicitly specified.
	emissionCurve := constAccessor.GetInt64Value(constants.EmissionCurve)
	blocksPerYear := constAccessor.GetInt64Value(constants.BlocksPerYear)

	// For each example, first totalEffectiveBond = effectiveSecurityBond, as though there were only one node;
	// then totalEffectiveBond = 1.5 * effectiveSecurityBond, as though multiple nodes all with the same bond.

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// First, thorough testing with PendulumUseEffectiveSecurity and PendulumUseVaultAssets both true.
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 1)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 1)

	bondR, poolR, lpShare, _, _, _, _, _ := networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1586), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1585), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(4998), Commentf("%d", lpShare.Uint64())) // Equilibrium
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1903), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR := bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1268), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1268), Commentf("%d", poolR.Uint64()))     // Equilibrium
	c.Check(lpShare.Uint64(), Equals, uint64(3999), Commentf("%d", lpShare.Uint64())) // ~40% for availablePoolsRune, ~40% for effectiveSecurityBond (equilibrium), ~60% for totalEffectiveBond

	// vaultsLiquidityRune more than availablePoolsRune.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	// TODO: poolR here is intended to be non-zero; find out what's strange.
	c.Check(bondR.Uint64(), Equals, uint64(2114), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1057), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(3333), Commentf("%d", lpShare.Uint64())) // 500 availablePoolsRune (1000 rune value asset+rune liquidity) is getting half the rewards of 2000 effectiveSecurityBond; same yield)
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(2378), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1585), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(793), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(2501), Commentf("%d", lpShare.Uint64())) // 500 availablePoolsRune (1000 rune value asset+rune liquidity) is getting a third the rewards of 3000 totalEffectiveBond; same yield)

	// Liquidity fees non-zero.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, cosmos.NewUint(3000), emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3086), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(3085), Commentf("%d", poolR.Uint64()))     // Equilibrium with bondR.  (Liquidity fees sent to Reserve in swap, some received back in EndBlock rewards.)
	c.Check(lpShare.Uint64(), Equals, uint64(4999), Commentf("%d", lpShare.Uint64())) // Equilibrium
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, cosmos.NewUint(3000), emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3703), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(2468), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(2468), Commentf("%d", poolR.Uint64()))     // Equilibrium with effectiveSecurityBondR.  (Liquidity fees sent to Reserve in swap, some received back in EndBlock rewards.)
	c.Check(lpShare.Uint64(), Equals, uint64(3999), Commentf("%d", lpShare.Uint64())) // ~40% for availablePoolsRune, ~40% for effectiveSecurityBond (equilibrium), ~60% for totalEffectiveBond

	// Empty Reserve and no liquidity fees (all rewards zero).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, cosmos.ZeroUint(), totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(0), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, cosmos.ZeroUint(), totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(0), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(0), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	// Now, half-size of effectiveSecurityBond.
	effectiveSecurityBond = cosmos.NewUint(1000 * common.One)

	// Provided liquidity equal to effectiveSecurityBond (no pool rewards).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1057), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	// Zero provided liquidity (incapable of receiving pool rewards).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.ZeroUint(), cosmos.ZeroUint(), effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64())) // No pools are capable of receiving rewards, so should not transfer any RUNE to the Pool Module (broken invariant).
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.ZeroUint(), cosmos.ZeroUint(), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1057), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	// Provided liquidity more than effectiveSecurityBond.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.NewUint(2001*common.One), cosmos.NewUint(2001*common.One), effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.NewUint(2001*common.One), cosmos.NewUint(2001*common.One), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1057), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Next, thorough testing with PendulumUseEffectiveSecurity and PendulumUseVaultAssets both false.
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 0)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 1)
	effectiveSecurityBond = cosmos.NewUint(2000 * common.One) // Resetting from the half-size comparison.

	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1586), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1585), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(4998), Commentf("%d", lpShare.Uint64())) // Equilibrium
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1057), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(704), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(2114), Commentf("%d", poolR.Uint64()))     // Not equilibrium; double bondR.
	c.Check(lpShare.Uint64(), Equals, uint64(6667), Commentf("%d", lpShare.Uint64())) // ~67% for availablePoolsRune, ~22% for effectiveSecurityBond (equilibrium), ~33% for totalEffectiveBond

	// vaultsLiquidityRune more than availablePoolsRune.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	// TODO: poolR here is intended to be non-zero; find out what's strange.
	c.Check(bondR.Uint64(), Equals, uint64(2114), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1057), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(3333), Commentf("%d", lpShare.Uint64())) // 500 availablePoolsRune (1000 rune value asset+rune liquidity) is getting half the rewards of 2000 effectiveSecurityBond; same yield)
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1585), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1056), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1586), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(5002), Commentf("%d", lpShare.Uint64())) // 500 availablePoolsRune (1000 rune value asset+rune liquidity) is getting half the rewards of 3000 totalEffectiveBond; 3/2 higher yield)

	// Liquidity fees non-zero.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, cosmos.NewUint(3000), emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3086), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(3085), Commentf("%d", poolR.Uint64()))     // Equilibrium with bondR.  (Liquidity fees sent to Reserve in swap, some received back in EndBlock rewards.)
	c.Check(lpShare.Uint64(), Equals, uint64(4999), Commentf("%d", lpShare.Uint64())) // Equilibrium
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, cosmos.NewUint(3000), emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(2057), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1371), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(4114), Commentf("%d", poolR.Uint64()))     // 3000 liquidity fees to Reserve in swap and 4114 rewards back, triple that of effectiveSecurityBondR.
	c.Check(lpShare.Uint64(), Equals, uint64(6667), Commentf("%d", lpShare.Uint64())) // ~67% for availablePoolsRune, ~22% for effectiveSecurityBond (equilibrium), ~33% for totalEffectiveBond

	// Empty Reserve and no liquidity fees (all rewards zero).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, cosmos.ZeroUint(), totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(0), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, cosmos.ZeroUint(), totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(0), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(0), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	// Now, half-size of effectiveSecurityBond.
	effectiveSecurityBond = cosmos.NewUint(1000 * common.One)

	// Provided liquidity equal to effectiveSecurityBond (still pool rewards since less than totalEffectiveRune).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, vaultsLiquidityRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(1057), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(352), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(2114), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(6667), Commentf("%d", lpShare.Uint64()))

	// Zero provided liquidity (incapable of receiving pool rewards).
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.ZeroUint(), cosmos.ZeroUint(), effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64())) // No pools are capable of receiving rewards, so should not transfer any RUNE to the Pool Module (broken invariant).
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.ZeroUint(), cosmos.ZeroUint(), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(1057), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))

	// Provided liquidity more than effectiveSecurityBond.
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.NewUint(2001*common.One), cosmos.NewUint(2001*common.One), effectiveSecurityBond, effectiveSecurityBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(3171), Commentf("%d", bondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(0), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(0), Commentf("%d", lpShare.Uint64()))
	// With totalEffectiveBond = 1.5 * effectiveSecurityBond:
	bondR, poolR, lpShare, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, cosmos.NewUint(2001*common.One), cosmos.NewUint(2001*common.One), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(bondR.Uint64(), Equals, uint64(2115), Commentf("%d", bondR.Uint64()))
	effectiveSecurityBondR = bondR.Mul(effectiveSecurityBond).Quo(totalEffectiveBond)
	c.Check(effectiveSecurityBondR.Uint64(), Equals, uint64(705), Commentf("%d", effectiveSecurityBondR.Uint64()))
	c.Check(poolR.Uint64(), Equals, uint64(1056), Commentf("%d", poolR.Uint64()))
	c.Check(lpShare.Uint64(), Equals, uint64(3330), Commentf("%d", lpShare.Uint64()))

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Testing of different settings for PendulumAssetsBasisPoints, PendulumUseEffectiveSecurity, PendulumUseVaultAssets.
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 10_000)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 1)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 1)

	c.Check(totalEffectiveBond.String(), Equals, cosmos.NewUint(3000*common.One).String())
	effectiveSecurityBond = cosmos.NewUint(2000 * common.One) // Resetting from the half-size comparison.
	c.Check(vaultsLiquidityRune.String(), Equals, cosmos.NewUint(1000*common.One).String())
	c.Check(availablePoolsRune.String(), Equals, cosmos.NewUint(500*common.One).String())

	bondR, poolR, _, devFundDeduct, systemIncomeBurnDeduct, tcyStakeDeduct, _, _ := networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(devFundDeduct.String(), Equals, "0")
	c.Check(systemIncomeBurnDeduct.String(), Equals, "0")
	c.Check(tcyStakeDeduct.String(), Equals, "0")
	c.Check(bondR.String(), Equals, "2378")
	c.Check(poolR.String(), Equals, "793")

	e12BondYield := bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield := poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "7926")
	c.Check(e12PoolYield.String(), Equals, "7930")
	c.Check(e12BondYield.QuoUint64(100).String(), Equals, e12PoolYield.QuoUint64(100).String())
	// Approximately equilibrium (affected slightly by rounding steps).

	///////////////////////////////////////////////////////////////////////////////
	// PendulumAssetsBasisPoints 13,333:
	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 13_333)

	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "9060")
	c.Check(e12PoolYield.String(), Equals, "4530")
	// The pendulum perceives secured liquidity as being 2/3rds of securing liquidity rather than 1/2, so nodes get more than the yield of pools (double).

	///////////////////////////////////////////////////////////////////////////////
	// PendulumAssetsBasisPoints 6,666:
	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 6666)
	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "6343")
	c.Check(e12PoolYield.String(), Equals, "12680")
	// The pendulum perceives secured liquidity as being 1/3rd of securing liquidity rather than 1/2, so nodes get less than the yield of pools (half).

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// PendulumUseEffectiveSecurity false, PendulumUseVaultAssets.String() true:
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 10_000)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 0)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 1)

	c.Check(totalEffectiveBond.String(), Equals, cosmos.NewUint(3000*common.One).String())
	c.Check(vaultsLiquidityRune.String(), Equals, cosmos.NewUint(1000*common.One).String())

	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "5283")
	c.Check(e12PoolYield.String(), Equals, "15860")
	// The pendulum perceives the secured vaults liquidity as being 1/3rd of the securing total effective bond, so nodes get 1/3rd the yield of pools.

	// Equilbrium yield when vaultsLiquidityRune is 3/2 greater:
	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune.MulUint64(3).QuoUint64(2), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "7930")
	c.Check(e12PoolYield.String(), Equals, "7920")
	c.Check(e12BondYield.QuoUint64(100).String(), Equals, e12PoolYield.QuoUint64(100).String())

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// PendulumUseEffectiveSecurity true, PendulumUseVaultAssets.String() false:
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 10_000)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 1)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 0)

	c.Check(effectiveSecurityBond.String(), Equals, cosmos.NewUint(2000*common.One).String())
	c.Check(availablePoolsRune.String(), Equals, cosmos.NewUint(500*common.One).String())

	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "3526")
	c.Check(e12PoolYield.String(), Equals, "21130")
	// The pendulum perceives secured liquidity as being 1/4th of securing liquidity rather than 1/2, so nodes get less than the yield of pools (1/6th).

	// Equilbrium yield when availablePoolsRune is doubled:
	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune.MulUint64(2), vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2).MulUint64(2)) // The (doubled) pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "6343")
	c.Check(e12PoolYield.String(), Equals, "6340")
	c.Check(e12BondYield.QuoUint64(100).String(), Equals, e12PoolYield.QuoUint64(100).String())

	// No change when ignoring L1 Assets when vaultsLiquidityRune is equally increased in order to increase availablePoolsRune:
	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune.MulUint64(2), vaultsLiquidityRune.Add(availablePoolsRune), effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2).MulUint64(2)) // The (doubled) pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "6343")
	c.Check(e12PoolYield.String(), Equals, "6340")
	c.Check(e12BondYield.QuoUint64(100).String(), Equals, e12PoolYield.QuoUint64(100).String())

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// PendulumUseEffectiveSecurity false, PendulumUseVaultAssets false:
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	networkMgr.k.SetMimir(ctx, constants.PendulumAssetsBasisPoints.String(), 10_000)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseEffectiveSecurity.String(), 0)
	networkMgr.k.SetMimir(ctx, constants.PendulumUseVaultAssets.String(), 0)

	c.Check(totalEffectiveBond.String(), Equals, cosmos.NewUint(3000*common.One).String())
	c.Check(availablePoolsRune.String(), Equals, cosmos.NewUint(500*common.One).String())

	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(2)) // The pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "1763")
	c.Check(e12PoolYield.String(), Equals, "26420")
	// The pendulum perceives secured liquidity as being 1/6th of securing liquidity rather than 1/2, so nodes get less than the yield of pools (1/15th).

	// Equilbrium yield when availablePoolsRune is tripled:
	bondR, poolR, _, _, _, _, _, _ = networkMgr.calcBlockRewards(ctx, availablePoolsRune.MulUint64(3), vaultsLiquidityRune, effectiveSecurityBond, totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve, blocksPerYear, 0, 0, 0, 0, 0)
	e12BondYield = bondR.MulUint64(1e12).Quo(totalEffectiveBond)
	e12PoolYield = poolR.MulUint64(1e12).Quo(availablePoolsRune.MulUint64(3).MulUint64(2)) // The (tripled) pool liquidity experiencing yield is the total value, both Asset and RUNE depths.
	c.Check(e12BondYield.String(), Equals, "5286")
	c.Check(e12PoolYield.String(), Equals, "5283")
	c.Check(e12BondYield.QuoUint64(100).String(), Equals, e12PoolYield.QuoUint64(100).String())
}

func (s *NetworkManagerTestSuite) TestCalcPoolDeficit(c *C) {
	pool1Fees := cosmos.NewUint(1000)
	pool2Fees := cosmos.NewUint(3000)
	totalFees := cosmos.NewUint(4000)

	mgr := NewDummyMgr()
	networkMgr := newNetworkMgr(keeper.KVStoreDummy{}, mgr.TxOutStore(), mgr.EventMgr())

	lpDeficit := cosmos.NewUint(1120)
	amt1 := networkMgr.calcPoolDeficit(lpDeficit, totalFees, pool1Fees)
	amt2 := networkMgr.calcPoolDeficit(lpDeficit, totalFees, pool2Fees)

	c.Check(amt1.Equal(cosmos.NewUint(280)), Equals, true, Commentf("%d", amt1.Uint64()))
	c.Check(amt2.Equal(cosmos.NewUint(840)), Equals, true, Commentf("%d", amt2.Uint64()))
}

func (*NetworkManagerTestSuite) TestProcessGenesisSetup(c *C) {
	ctx, mgr := setupManagerForTest(c)
	helper := NewVaultGenesisSetupTestHelper(mgr.Keeper())
	ctx = ctx.WithBlockHeight(1)
	mgr.K = helper
	networkMgr := newNetworkMgr(helper, mgr.TxOutStore(), mgr.EventMgr())
	// no active account
	c.Assert(networkMgr.EndBlock(ctx, mgr), NotNil)

	nodeAccount := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, nodeAccount), IsNil)
	c.Assert(networkMgr.EndBlock(ctx, mgr), IsNil)
	// make sure asgard vault get created
	vaults, err := mgr.Keeper().GetAsgardVaults(ctx)
	c.Assert(err, IsNil)
	c.Assert(vaults, HasLen, 1)

	// fail to get asgard vaults should return an error
	helper.failToGetAsgardVaults = true
	c.Assert(networkMgr.EndBlock(ctx, mgr), NotNil)
	helper.failToGetAsgardVaults = false

	// vault already exist , it should not do anything , and should not error
	c.Assert(networkMgr.EndBlock(ctx, mgr), IsNil)

	ctx, mgr = setupManagerForTest(c)
	helper = NewVaultGenesisSetupTestHelper(mgr.Keeper())
	ctx = ctx.WithBlockHeight(1)
	mgr.K = helper
	networkMgr = newNetworkMgr(helper, mgr.TxOutStore(), mgr.EventMgr())
	helper.failToListActiveAccounts = true
	c.Assert(networkMgr.EndBlock(ctx, mgr), NotNil)
	helper.failToListActiveAccounts = false

	helper.failToSetVault = true
	c.Assert(networkMgr.EndBlock(ctx, mgr), NotNil)
	helper.failToSetVault = false

	helper.failGetRetiringAsgardVault = true
	ctx = ctx.WithBlockHeight(1024)
	c.Assert(networkMgr.migrateFunds(ctx, mgr), NotNil)
	helper.failGetRetiringAsgardVault = false

	helper.failGetActiveAsgardVault = true
	c.Assert(networkMgr.migrateFunds(ctx, mgr), NotNil)
	helper.failGetActiveAsgardVault = false
}

func (*NetworkManagerTestSuite) TestGetAvailablePoolsRune(c *C) {
	ctx, mgr := setupManagerForTest(c)
	helper := NewVaultGenesisSetupTestHelper(mgr.Keeper())
	mgr.K = helper
	networkMgr := newNetworkMgr(helper, mgr.TxOutStore(), mgr.EventMgr())
	p := NewPool()
	p.Asset = common.ETHAsset
	p.BalanceRune = cosmos.NewUint(common.One * 100)
	p.BalanceAsset = cosmos.NewUint(common.One * 100)
	p.Status = PoolAvailable
	c.Assert(helper.SetPool(ctx, p), IsNil)
	pools, totalLiquidity, err := getAvailablePoolsRune(ctx, networkMgr.k)
	c.Assert(err, IsNil)
	c.Assert(pools, HasLen, 1)
	c.Assert(totalLiquidity.Equal(p.BalanceRune), Equals, true)
}

func (*NetworkManagerTestSuite) TestPayPoolRewards(c *C) {
	ctx, mgr := setupManagerForTest(c)
	helper := NewVaultGenesisSetupTestHelper(mgr.Keeper())
	mgr.K = helper
	networkMgr := newNetworkMgr(helper, mgr.TxOutStore(), mgr.EventMgr())
	p := NewPool()
	p.Asset = common.ETHAsset
	p.BalanceRune = cosmos.NewUint(common.One * 100)
	p.BalanceAsset = cosmos.NewUint(common.One * 100)
	p.Status = PoolAvailable
	c.Assert(helper.SetPool(ctx, p), IsNil)
	c.Assert(networkMgr.payPoolRewards(ctx, []cosmos.Uint{cosmos.NewUint(100 * common.One)}, Pools{p}), IsNil)
	helper.failToSetPool = true
	c.Assert(networkMgr.payPoolRewards(ctx, []cosmos.Uint{cosmos.NewUint(100 * common.One)}, Pools{p}), NotNil)
}

func (s *NetworkManagerTestSuite) TestSaverYieldFunc(c *C) {
	var err error
	ctx, mgr := setupManagerForTest(c)
	net := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())
	mgr.Keeper().SetMimir(ctx, constants.SynthYieldCycle.String(), 5_000)

	// mint synths
	coin := common.NewCoin(common.BTCAsset.GetSyntheticAsset(), cosmos.NewUint(10*common.One))
	c.Assert(mgr.Keeper().MintToModule(ctx, ModuleName, coin), IsNil)
	c.Assert(mgr.Keeper().SendFromModuleToModule(ctx, ModuleName, AsgardName, common.NewCoins(coin)), IsNil)

	spool := NewPool()
	spool.Asset = common.BTCAsset.GetSyntheticAsset()
	spool.BalanceAsset = coin.Amount
	spool.LPUnits = cosmos.NewUint(100)
	c.Assert(mgr.Keeper().SetPool(ctx, spool), IsNil)

	// first pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.LPUnits = cosmos.NewUint(100)
	pool.CalcUnits(coin.Amount)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(net.paySaverYield(ctx, common.BTCAsset, cosmos.NewUint(50*common.One)), IsNil)
	spool, err = mgr.Keeper().GetPool(ctx, spool.Asset)
	c.Assert(err, IsNil)
	c.Assert(spool.BalanceAsset.String(), Equals, "1113100000", Commentf("%d", spool.BalanceAsset.Uint64()))
}

func (s *NetworkManagerTestSuite) TestSaverYieldCall(c *C) {
	var err error
	ctx, mgr := setupManagerForTest(c)
	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)

	na := GetRandomValidatorNode(NodeActive)
	na.Bond = cosmos.NewUint(500000 * common.One)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	coin := common.NewCoin(common.BTCAsset.GetSyntheticAsset(), cosmos.NewUint(10*common.One))
	spool := NewPool()
	spool.Asset = common.BTCAsset.GetSyntheticAsset()
	spool.BalanceAsset = coin.Amount
	spool.LPUnits = cosmos.NewUint(100)
	c.Assert(mgr.Keeper().SetPool(ctx, spool), IsNil)

	// layer 1 pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.LPUnits = cosmos.NewUint(100)
	pool.CalcUnits(coin.Amount)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Vault for getVaultsLiquidityRune.
	vault := NewVault(0, ActiveVault, AsgardVault, GetRandomPubKey(), []string{pool.Asset.GetChain().String()}, []ChainContract{})
	vault.Coins = common.NewCoins(common.NewCoin(pool.Asset, pool.BalanceAsset))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	// test no fees collected
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.gasMgr, mgr.eventMgr), IsNil)
	spool, err = mgr.Keeper().GetPool(ctx, spool.Asset.GetSyntheticAsset())
	c.Assert(err, IsNil)
	c.Check(spool.BalanceAsset.Uint64(), Equals, uint64(10_07925863), Commentf("%d", spool.BalanceAsset.Uint64()))

	// mgr.Keeper().SetMimir(ctx, constants.IncentiveCurve.String(), 50)
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, pool.Asset, cosmos.NewUint(50*common.One)), IsNil)
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.gasMgr, mgr.eventMgr), IsNil)
	spool, err = mgr.Keeper().GetPool(ctx, spool.Asset.GetSyntheticAsset())
	c.Assert(err, IsNil)
	c.Check(spool.BalanceAsset.Uint64(), Equals, uint64(12_59882082), Commentf("%d", spool.BalanceAsset.Uint64()))

	// check we don't give yield when synth utilization is too high
	// add some synths
	coins := cosmos.NewCoins(cosmos.NewCoin("btc/btc", cosmos.NewInt(101*common.One))) // 51% utilization
	c.Assert(mgr.coinKeeper.MintCoins(ctx, ModuleName, coins), IsNil)
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, pool.Asset, cosmos.NewUint(50*common.One)), IsNil)
	c.Assert(networkMgr.UpdateNetwork(ctx, constAccessor, mgr.gasMgr, mgr.eventMgr), IsNil)
	spool, err = mgr.Keeper().GetPool(ctx, spool.Asset.GetSyntheticAsset())
	c.Assert(err, IsNil)
	c.Check(spool.BalanceAsset.Uint64(), Equals, uint64(12_59882082), Commentf("%d", spool.BalanceAsset.Uint64()))
}

func (s *NetworkManagerTestSuite) TestRagnarokPool(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(100000)
	na := GetRandomValidatorNode(NodeActive)
	c.Assert(k.SetNodeAccount(ctx, na), IsNil)
	activeVault := GetRandomVault()
	activeVault.StatusSince = ctx.BlockHeight() - 10
	activeVault.Coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(k.SetVault(ctx, activeVault), IsNil)
	retireVault := GetRandomVault()
	retireVault.Chains = common.Chains{common.ETHChain, common.BTCChain}.Strings()
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(1000 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(10 * common.One)
	btcPool.LPUnits = cosmos.NewUint(1600)
	btcPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, btcPool), IsNil)
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceRune = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceAsset = cosmos.NewUint(10 * common.One)
	ethPool.LPUnits = cosmos.NewUint(1600)
	ethPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, ethPool), IsNil)
	addr := GetRandomRUNEAddress()
	lps := LiquidityProviders{
		{
			Asset:             common.BTCAsset,
			RuneAddress:       addr,
			AssetAddress:      GetRandomBTCAddress(),
			LastAddHeight:     5,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
		{
			Asset:             common.BTCAsset,
			RuneAddress:       GetRandomRUNEAddress(),
			AssetAddress:      GetRandomBTCAddress(),
			LastAddHeight:     10,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
	}
	k.SetLiquidityProvider(ctx, lps[0])
	k.SetLiquidityProvider(ctx, lps[1])
	mgr := NewDummyMgrWithKeeper(k)
	networkMgr := newNetworkMgr(k, mgr.TxOutStore(), mgr.EventMgr())

	ctx = ctx.WithBlockHeight(1)
	// block height not correct , doesn't take any actions
	err := networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)
	for _, a := range []common.Asset{common.BTCAsset, common.ETHAsset} {
		tempPool, getErr := k.GetPool(ctx, a)
		c.Assert(getErr, IsNil)
		c.Assert(tempPool.Status, Equals, PoolAvailable)
	}
	interval := mgr.GetConstants().GetInt64Value(constants.FundMigrationInterval)
	// mimir didn't set , it should not take any actions
	ctx = ctx.WithBlockHeight(interval * 5)
	err = networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)

	// happy path
	networkMgr.k.SetMimir(ctx, "RagnarokProcessNumOfLPPerIteration", 1)
	networkMgr.k.SetMimir(ctx, "RAGNAROK-BTC-BTC", 1)
	// first round
	err = networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)
	items, _ := mgr.txOutStore.GetOutboundItems(ctx)
	c.Assert(items, HasLen, 1, Commentf("%d", len(items)))

	ctx = ctx.WithBlockHeight(interval * 6)
	err = networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)
	items, _ = mgr.txOutStore.GetOutboundItems(ctx)
	c.Assert(items, HasLen, 2, Commentf("%d", len(items)))

	tempPool, err := k.GetPool(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Assert(tempPool.Status, Equals, PoolStaged)

	ctx = ctx.WithBlockHeight(interval * 7)
	err = networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)
	items, _ = mgr.txOutStore.GetOutboundItems(ctx)
	c.Assert(items, HasLen, 2, Commentf("%d", len(items)))

	tempPool, err = k.GetPool(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Assert(tempPool.Status, Equals, PoolSuspended)

	tempPool, err = k.GetPool(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	c.Assert(tempPool.Status, Equals, PoolAvailable)

	// when there are none gas token pool , and it is active , gas asset token pool should not be ragnarok
	usdcPool := NewPool()
	usdcAsset, err := common.NewAsset("ETH.USDC-0X9999999999999999999999999999999999999999")
	c.Assert(err, IsNil)
	usdcPool.Asset = usdcAsset
	usdcPool.BalanceRune = cosmos.NewUint(1000 * common.One)
	usdcPool.BalanceAsset = cosmos.NewUint(10 * common.One)
	usdcPool.LPUnits = cosmos.NewUint(1600)
	usdcPool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, usdcPool), IsNil)

	networkMgr.k.SetMimir(ctx, "RAGNAROK-ETH-ETH", 1)
	err = networkMgr.checkPoolRagnarok(ctx, mgr)
	c.Assert(err, IsNil)
	tempPool, err = k.GetPool(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	c.Assert(tempPool.Status, Equals, PoolAvailable)
}

func (s *NetworkManagerTestSuite) TestCleanupAsgardIndex(c *C) {
	ctx, k := setupKeeperForTest(c)
	vault1 := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	c.Assert(k.SetVault(ctx, vault1), IsNil)
	vault2 := NewVault(1024, RetiringVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	c.Assert(k.SetVault(ctx, vault2), IsNil)
	vault3 := NewVault(1024, InitVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	c.Assert(k.SetVault(ctx, vault3), IsNil)
	vault4 := NewVault(1024, InactiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	c.Assert(k.SetVault(ctx, vault4), IsNil)
	mgr := NewDummyMgrWithKeeper(k)
	networkMgr := newNetworkMgr(k, mgr.TxOutStore(), mgr.EventMgr())
	c.Assert(networkMgr.cleanupAsgardIndex(ctx), IsNil)
	containsVault := func(vaults Vaults, pubKey common.PubKey) bool {
		for _, item := range vaults {
			if item.PubKey.Equals(pubKey) {
				return true
			}
		}
		return false
	}
	asgards, err := k.GetAsgardVaults(ctx)
	c.Assert(err, IsNil)
	c.Assert(containsVault(asgards, vault1.PubKey), Equals, true)
	c.Assert(containsVault(asgards, vault2.PubKey), Equals, true)
	c.Assert(containsVault(asgards, vault3.PubKey), Equals, true)
	c.Assert(containsVault(asgards, vault4.PubKey), Equals, false)
}

func (*NetworkManagerTestSuite) TestPOLLiquidityAdd(c *C) {
	ctx, mgr := setupManagerForTest(c)

	net := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())
	max := cosmos.NewUint(10000)

	polAddress, err := mgr.Keeper().GetModuleAddress(ReserveName)
	c.Assert(err, IsNil)
	asgardAddress, err := mgr.Keeper().GetModuleAddress(AsgardName)
	c.Assert(err, IsNil)
	na := GetRandomValidatorNode(NodeActive)
	signer := na.NodeAddress
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(2000 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(20 * common.One)
	btcPool.LPUnits = cosmos.NewUint(1600)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// hit max
	util := cosmos.NewUint(1500)
	target := cosmos.NewUint(1000)
	c.Assert(net.addPOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err := mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(7), Commentf("%d", lp.Units.Uint64()))

	// doesn't hit max
	util = cosmos.NewUint(1050)
	c.Assert(net.addPOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err = mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(10), Commentf("%d", lp.Units.Uint64()))

	// no change needed
	util = cosmos.NewUint(1000)
	c.Assert(net.addPOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err = mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(10), Commentf("%d", lp.Units.Uint64()))

	// not enough balance in the reserve module
	max = cosmos.NewUint(1000000)
	util = cosmos.NewUint(50_000)
	btcPool.BalanceRune = cosmos.NewUint(90000000000 * common.One)
	c.Assert(net.addPOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err = mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(10), Commentf("%d", lp.Units.Uint64()))
}

func (*NetworkManagerTestSuite) TestPOLLiquidityWithdraw(c *C) {
	ctx, mgr := setupManagerForTest(c)

	net := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())
	max := cosmos.NewUint(10000)

	polAddress, err := mgr.Keeper().GetModuleAddress(ReserveName)
	c.Assert(err, IsNil)
	asgardAddress, err := mgr.Keeper().GetModuleAddress(AsgardName)
	c.Assert(err, IsNil)
	na := GetRandomValidatorNode(NodeActive)
	signer := na.NodeAddress
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	vault := GetRandomVault()
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(2000 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(20 * common.One)
	btcPool.LPUnits = cosmos.NewUint(1600)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	lps := LiquidityProviders{
		{
			Asset:             btcPool.Asset,
			RuneAddress:       GetRandomETHAddress(),
			AssetAddress:      GetRandomBTCAddress(),
			LastAddHeight:     5,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
		{
			Asset:             btcPool.Asset,
			RuneAddress:       polAddress,
			AssetAddress:      common.NoAddress,
			LastAddHeight:     10,
			Units:             btcPool.LPUnits.QuoUint64(2),
			PendingRune:       cosmos.ZeroUint(),
			PendingAsset:      cosmos.ZeroUint(),
			AssetDepositValue: cosmos.ZeroUint(),
			RuneDepositValue:  cosmos.ZeroUint(),
		},
	}
	for _, lp := range lps {
		mgr.Keeper().SetLiquidityProvider(ctx, lp)
	}

	// hit max
	util := cosmos.NewUint(500)
	target := cosmos.NewUint(1000)
	c.Assert(net.removePOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err := mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(792), Commentf("%d", lp.Units.Uint64()))
	// To withdraw max 1% (100 basis points) of the pool RUNE depth, asymmetrically withdraw as RUNE 0.5% of all pool units.
	// 0.5% of 1600 is 8; 800 minus 8 is 792.

	// doesn't hit max
	util = cosmos.NewUint(950)
	c.Assert(net.removePOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err = mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(788), Commentf("%d", lp.Units.Uint64()))
	// To withdraw 0.5% of the pool RUNE depth, asymmetrically withdraw as RUNE 0.25% of all pool units.
	// 0.25% of 1592 is 3.98 which rounds to 4; 792 minus 4 is 788.

	// no change needed
	util = cosmos.NewUint(1000)
	c.Assert(net.removePOLLiquidity(ctx, btcPool, polAddress, asgardAddress, signer, max, util, target, mgr), IsNil)
	lp, err = mgr.Keeper().GetLiquidityProvider(ctx, btcPool.Asset, polAddress)
	c.Assert(err, IsNil)
	c.Check(lp.Units.Uint64(), Equals, uint64(788), Commentf("%d", lp.Units.Uint64()))
}

func (*NetworkManagerTestSuite) TestFairMergePOLCycle(c *C) {
	ctx, mgr := setupManagerForTest(c)
	net := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// cycle should do nothing when target is 0
	err := net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err := mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.Uint64(), Equals, uint64(0))
	c.Assert(pol.RuneWithdrawn.Uint64(), Equals, uint64(0))

	// cycle should error when target is greater than 0 with no node accounts
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 1000) // 10% liability
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, ErrorMatches, "dev err: no active node accounts")

	// create dummy eth pool
	pool := NewPool()
	pool.Asset = common.ETHAsset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	pool.LPUnits = cosmos.NewUint(100 * common.One)
	err = mgr.Keeper().SetPool(ctx, pool)
	c.Assert(err, IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	btcPool.LPUnits = cosmos.NewUint(100 * common.One)
	err = mgr.Keeper().SetPool(ctx, btcPool)
	c.Assert(err, IsNil)

	// cycle should error since there are no pol enabled pools
	err = mgr.Keeper().SetNodeAccount(ctx, GetRandomValidatorNode(NodeActive))
	c.Assert(err, IsNil)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, ErrorMatches, "no POL pools")

	// cycle should silently succeed when there is a pool enabled
	mgr.Keeper().SetMimir(ctx, "POL-ETH-ETH", 1)
	mgr.Keeper().SetMimir(ctx, "POL-BTC-BTC", 1)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// pol should still be zero since there are no synths
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.Uint64(), Equals, uint64(0))
	c.Assert(pol.RuneWithdrawn.Uint64(), Equals, uint64(0))

	// add some synths
	coins := cosmos.NewCoins(
		cosmos.NewCoin("eth/eth", cosmos.NewInt(20*common.One)),
		cosmos.NewCoin("btc/btc", cosmos.NewInt(20*common.One)),
	) // 20% utilization, 10% liability
	err = mgr.coinKeeper.MintCoins(ctx, ModuleName, coins)
	c.Assert(err, IsNil)
	err = mgr.Keeper().SetPool(ctx, pool)
	c.Assert(err, IsNil)

	// synth liability should be 10%
	synthSupply := mgr.Keeper().GetTotalSupply(ctx, pool.Asset.GetSyntheticAsset())
	pool.CalcUnits(synthSupply)
	liability := common.GetUncappedShare(pool.SynthUnits, pool.GetPoolUnits(), cosmos.NewUint(10_000))
	c.Assert(liability.String(), Equals, "1000")

	// cycle should succeed, still no rune deposited since max is 0
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// pol should still be zero
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "0")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "0")

	// synth liability should still be 10%
	synthSupply = mgr.Keeper().GetTotalSupply(ctx, pool.Asset.GetSyntheticAsset())
	pool.CalcUnits(synthSupply)
	liability = common.GetUncappedShare(pool.SynthUnits, pool.GetPoolUnits(), cosmos.NewUint(10_000))
	c.Assert(liability.String(), Equals, "1000")

	// set pol utilization to 5% should deposit up to the max
	mgr.Keeper().SetMimir(ctx, constants.POLMaxNetworkDeposit.String(), common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 500)
	mgr.Keeper().SetMimir(ctx, constants.POLMaxPoolMovement.String(), 10000)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "200000000")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "0")

	// there needs to be one vault or the withdraw handler fails
	vault := NewVault(0, ActiveVault, types.VaultType_AsgardVault, GetRandomPubKey(), []string{"ETH", "BTC"}, nil)
	err = mgr.Keeper().SetVault(ctx, vault)
	c.Assert(err, IsNil)

	// synth liability should still be 10%
	synthSupply = mgr.Keeper().GetTotalSupply(ctx, pool.Asset.GetSyntheticAsset())
	pool.CalcUnits(synthSupply)
	liability = common.GetUncappedShare(pool.SynthUnits, pool.GetPoolUnits(), cosmos.NewUint(10_000))
	c.Assert(liability.String(), Equals, "1000")

	// withdraw entire pol position
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 10000)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "200000000")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199395540")
	// only XYK constant-depths-product withdraw slip, no implicit slip fee

	// synth liability should still be 10%
	synthSupply = mgr.Keeper().GetTotalSupply(ctx, pool.Asset.GetSyntheticAsset())
	pool.CalcUnits(synthSupply)
	liability = common.GetUncappedShare(pool.SynthUnits, pool.GetPoolUnits(), cosmos.NewUint(10_000))
	c.Assert(liability.String(), Equals, "1000")

	synthSupply = mgr.Keeper().GetTotalSupply(ctx, btcPool.Asset.GetSyntheticAsset())
	btcPool.CalcUnits(synthSupply)
	liability = common.GetUncappedShare(btcPool.SynthUnits, btcPool.GetPoolUnits(), cosmos.NewUint(10_000))
	c.Assert(liability.String(), Equals, "1000")

	// deposit entire pol position
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 500)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199395540")

	// withdraw entire pol position 1 basis point of rune depth at a time
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 10000)
	mgr.Keeper().SetMimir(ctx, constants.POLMaxPoolMovement.String(), 1)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199415528")
	// another basis point
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199435514")

	// set the buffer to 100% to stop any movement
	mgr.Keeper().SetMimir(ctx, constants.POLBuffer.String(), 10000)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199435514")

	// current liability is at 10%, so buffer at 40% and target of 50% should still not move
	mgr.Keeper().SetMimir(ctx, constants.POLBuffer.String(), 4000)
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 5000)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199435514")

	// any smaller buffer should withdraw one basis point of rune
	mgr.Keeper().SetMimir(ctx, constants.POLBuffer.String(), 3999)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "199455500")

	// withdraw everything
	mgr.Keeper().SetMimir(ctx, constants.POLTargetSynthPerPoolDepth.String(), 10000)
	mgr.Keeper().SetMimir(ctx, constants.POLBuffer.String(), 0)
	mgr.Keeper().SetMimir(ctx, constants.POLMaxPoolMovement.String(), 10000)
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "398797134")

	// should be nothing left to withdraw again
	err = net.POLCycle(ctx, mgr)
	c.Assert(err, IsNil)
	pol, err = mgr.Keeper().GetPOL(ctx)
	c.Assert(err, IsNil)
	c.Assert(pol.RuneDeposited.String(), Equals, "400006044")
	c.Assert(pol.RuneWithdrawn.String(), Equals, "398797134")
}

func (s *NetworkManagerTestSuite) TestSpawnDerivedAssets(c *C) {
	ctx, mgr := setupManagerForTest(c)

	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	vault := GetRandomVault()
	vault.Chains = append(vault.Chains, common.BSCChain.String())
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	mgr.Keeper().SetMimir(ctx, "DerivedDepthBasisPts", 10_000)
	mgr.Keeper().SetMimir(ctx, "TorAnchor-ETH-BUSD-BD1", 1) // enable BUSD pool as a TOR anchor
	maxAnchorSlip := mgr.Keeper().GetConfigInt64(ctx, constants.MaxAnchorSlip)
	ethBusd, err := common.NewAsset("ETH.BUSD-BD1")
	c.Assert(err, IsNil)

	pool := NewPool()
	pool.Asset = ethBusd
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(187493559385369)
	pool.BalanceAsset = cosmos.NewUint(925681680182301)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	eth, err := common.NewAsset("ETH.ETH")
	c.Assert(err, IsNil)

	pool = NewPool()
	pool.Asset = eth
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(110119961610327)
	pool.BalanceAsset = cosmos.NewUint(2343330836117)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	bscBnb, err := common.NewAsset("BSC.BNB")
	c.Assert(err, IsNil)

	// should not have any affect on THOR.BNB
	bscPool := NewPool()
	bscPool.Asset = bscBnb
	bscPool.Status = PoolAvailable
	bscPool.BalanceRune = cosmos.NewUint(510119961610327)
	bscPool.BalanceAsset = cosmos.NewUint(4343330836117)
	bscPool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, bscPool), IsNil)

	// happy path
	err = nmgr.spawnDerivedAssets(ctx, mgr)
	c.Assert(err, IsNil)
	usd, err := mgr.Keeper().GetPool(ctx, common.TOR)
	c.Assert(err, IsNil)
	c.Check(usd.BalanceAsset.Uint64(), Equals, uint64(925681680182301), Commentf("%d", usd.BalanceAsset.Uint64()))
	c.Check(usd.BalanceRune.Uint64(), Equals, uint64(187493559385369), Commentf("%d", usd.BalanceRune.Uint64()))
	dbnb, _ := common.NewAsset("THOR.BNB")
	bnbPool, err := mgr.Keeper().GetPool(ctx, dbnb)
	c.Assert(err, IsNil)
	c.Check(bnbPool.BalanceAsset.Uint64(), Equals, uint64(4343330836117), Commentf("%d", bnbPool.BalanceAsset.Uint64()))
	c.Check(bnbPool.BalanceRune.Uint64(), Equals, uint64(510119961610327), Commentf("%d", bnbPool.BalanceRune.Uint64()))

	// happy path, but some trade volume triggers a lower pool depth
	newctx := ctx.WithBlockHeight(ctx.BlockHeight() - 1)
	err = mgr.Keeper().AddToSwapSlip(newctx, ethBusd, cosmos.NewInt(maxAnchorSlip/4))
	c.Assert(err, IsNil)
	err = nmgr.spawnDerivedAssets(ctx, mgr)
	c.Assert(err, IsNil)
	usd, err = mgr.Keeper().GetPool(ctx, common.TOR)
	c.Assert(err, IsNil)
	c.Check(usd.Status.String(), Equals, "Available")
	c.Check(usd.BalanceAsset.Uint64(), Equals, uint64(694261260136726), Commentf("%d", usd.BalanceAsset.Uint64()))
	c.Check(usd.BalanceRune.Uint64(), Equals, uint64(140620169539027), Commentf("%d", usd.BalanceRune.Uint64()))

	// unhappy path, too much liquidity fees collected in the anchor pools, goes to 1% depth
	err = mgr.Keeper().AddToSwapSlip(newctx, ethBusd, cosmos.NewInt(10_000))
	c.Assert(err, IsNil)
	err = nmgr.spawnDerivedAssets(ctx, mgr)
	c.Assert(err, IsNil)
	usd, err = mgr.Keeper().GetPool(ctx, common.TOR)
	c.Assert(err, IsNil)
	c.Assert(usd.Status.String(), Equals, "Available")
	c.Assert(usd.BalanceAsset.Uint64(), Equals, uint64(9256816801824), Commentf("%d", usd.BalanceAsset.Uint64()))
	c.Assert(usd.BalanceRune.Uint64(), Equals, uint64(1874935593854), Commentf("%d", usd.BalanceRune.Uint64()))
	// ensure layer1 bnb pool is NOT suspended
	bnbPool, err = mgr.Keeper().GetPool(ctx, ethBusd)
	c.Assert(err, IsNil)
	c.Assert(bnbPool.Status.String(), Equals, "Available")
	c.Assert(bnbPool.BalanceAsset.Uint64(), Equals, uint64(925681680182301), Commentf("%d", bnbPool.BalanceAsset.Uint64()))
	c.Assert(bnbPool.BalanceRune.Uint64(), Equals, uint64(187493559385369), Commentf("%d", bnbPool.BalanceRune.Uint64()))
}

func (s *NetworkManagerTestSuite) TestSpawnDerivedAssetsBasisPoints(c *C) {
	ctx, mgr := setupManagerForTest(c)

	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	vault := GetRandomVault()
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	mgr.Keeper().SetMimir(ctx, "TorAnchor-ETH-BUSD-BD1", 1) // enable ETH.BUSD pool as a TOR anchor
	ethBusd, err := common.NewAsset("ETH.BUSD-BD1")
	c.Assert(err, IsNil)

	pool := NewPool()
	pool.Asset = ethBusd
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(187493559385369)
	pool.BalanceAsset = cosmos.NewUint(925681680182301)
	pool.Decimals = 8
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// test that DerivedDepthBasisPts affects the pool depth
	mgr.Keeper().SetMimir(ctx, "DerivedDepthBasisPts", 20000)
	err = nmgr.spawnDerivedAssets(ctx, mgr)
	c.Assert(err, IsNil)
	usd, err := mgr.Keeper().GetPool(ctx, common.TOR)
	c.Assert(err, IsNil)
	c.Assert(usd.Status.String(), Equals, "Available")
	c.Check(usd.BalanceAsset.Uint64(), Equals, uint64(1851363360364602), Commentf("%d", usd.BalanceAsset.Uint64()))
	c.Check(usd.BalanceRune.Uint64(), Equals, uint64(374987118770738), Commentf("%d", usd.BalanceRune.Uint64()))

	// test that DerivedDepthBasisPts set to zero will cause the pools to
	// become suspended
	mgr.Keeper().SetMimir(ctx, "DerivedDepthBasisPts", 0)
	err = nmgr.spawnDerivedAssets(ctx, mgr)
	c.Assert(err, IsNil)
	usd, err = mgr.Keeper().GetPool(ctx, common.TOR)
	c.Assert(err, IsNil)
	c.Assert(usd.Status.String(), Equals, "Suspended")
	c.Assert(usd.BalanceAsset.Uint64(), Equals, uint64(1851363360364602), Commentf("%d", usd.BalanceAsset.Uint64()))
	c.Assert(usd.BalanceRune.Uint64(), Equals, uint64(374987118770738), Commentf("%d", usd.BalanceRune.Uint64()))
}

func (s *NetworkManagerTestSuite) TestFetchMeanSlip(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())
	asset := common.BTCAsset

	var slip int64
	var err error
	slip = nmgr.fetchWeightedMeanSlip(ctx, asset, mgr)
	c.Check(slip, Equals, int64(0))

	// setup slip history
	ctx = ctx.WithBlockHeight(14400 * 14)
	maxAnchorBlocks := mgr.Keeper().GetConfigInt64(ctx, constants.MaxAnchorBlocks)
	dynamicMaxAnchorSlipBlocks := mgr.Keeper().GetConfigInt64(ctx, constants.DynamicMaxAnchorSlipBlocks)
	for i := ctx.BlockHeight(); i > ctx.BlockHeight()-dynamicMaxAnchorSlipBlocks; i -= maxAnchorBlocks {
		if i <= 0 {
			break // dynamicMaxAnchorSlipBlocks > ctx.BlockHeight, end of chain history
		}

		mgr.Keeper().SetSwapSlipSnapShot(ctx, asset, i, i)
	}

	// mean slip will be 0 if the asset has no available pools
	slip = nmgr.fetchWeightedMeanSlip(ctx, asset, mgr)
	c.Check(slip, Equals, int64(0))
	slip, err = mgr.Keeper().GetLongRollup(ctx, asset)
	c.Assert(err, IsNil)
	c.Check(slip, Equals, int64(0))

	// create corresponding pool
	pool := NewPool()
	pool.Asset = asset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.LPUnits = cosmos.NewUint(100)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// mean slip is available after pool creation, should set long rollup
	slip = nmgr.fetchWeightedMeanSlip(ctx, asset, mgr)
	c.Check(slip, Equals, int64(100950))
	slip, err = mgr.Keeper().GetLongRollup(ctx, asset)
	c.Assert(err, IsNil)
	c.Check(slip, Equals, int64(100950))
}

func (s *NetworkManagerTestSuite) TestDistributeTCYStake(c *C) {
	ctx, mgr := setupManagerForTest(c)
	mgr.K.SetMimir(ctx, "TCYStakeDistributionHalt", 0)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())
	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)

	address1 := GetRandomRUNEAddress()
	acc1, err := address1.AccAddress()
	c.Assert(err, IsNil)
	address2 := GetRandomRUNEAddress()
	acc2, err := address2.AccAddress()
	c.Assert(err, IsNil)
	address3 := GetRandomRUNEAddress()
	acc3, err := address3.AccAddress()
	c.Assert(err, IsNil)
	address4 := GetRandomRUNEAddress()
	acc4, err := address4.AccAddress()
	c.Assert(err, IsNil)
	tcyStakeAddress := mgr.Keeper().GetModuleAccAddress(TCYStakeName)

	// Add TCYStaker to accounts -> acc1 ~ 75%, acc2 ~ 25%, acc4 = less than MinTCYForTCYStakeDistribution
	amountAddr1 := cosmos.NewUint(157_499_999_99950000)
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address1,
		Amount:  amountAddr1,
	})
	c.Assert(err, IsNil)

	amountAddr2 := cosmos.NewUint(52_499_999_99950001)
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address2,
		Amount:  amountAddr2,
	})
	c.Assert(err, IsNil)

	// Should be deleted since won't have amount this first run
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address3,
		Amount:  cosmos.ZeroUint(),
	})
	c.Assert(err, IsNil)

	// Set staking amount less than MinTCYForTCYStakeDistribution to address 4
	amt := constAccessor.GetInt64Value(constants.MinTCYForTCYStakeDistribution) - 1
	amountAddr4 := cosmos.NewUint(uint64(amt))
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address4,
		Amount:  amountAddr4,
	})
	c.Assert(err, IsNil)

	// Send staking amounts from address 1, 2 and 4 to tcy staking module
	stakingModuleTCYAmount := amountAddr1.Add(amountAddr2).Add(amountAddr4)
	coin := common.NewCoin(common.TCY, stakingModuleTCYAmount)
	err = mgr.Keeper().MintToModule(ctx, ModuleName, coin)
	c.Assert(err, IsNil)
	err = mgr.Keeper().SendFromModuleToModule(ctx, ModuleName, TCYStakeName, common.NewCoins(coin))
	c.Assert(err, IsNil)

	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address1), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address2), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address3), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address4), Equals, true)

	// Mint less than MinRuneForTCYStakeDistribution on TCYStakeName
	tcyStakeFeeAmount := constAccessor.GetInt64Value(constants.MinRuneForTCYStakeDistribution) - 1
	FundModule(c, ctx, mgr.Keeper(), TCYStakeName, uint64(tcyStakeFeeAmount))

	nmgr.distributeTCYStake(ctx, mgr)

	// Check balances, accounts should not receive funds from TCYStake since funds
	// are less than MinRuneForTCYStakeDistribution
	balanceAcc1 := mgr.Keeper().GetBalanceOf(ctx, acc1, common.RuneNative)
	c.Assert(balanceAcc1.IsZero(), Equals, true)
	balanceAcc2 := mgr.Keeper().GetBalanceOf(ctx, acc2, common.RuneNative)
	c.Assert(balanceAcc2.IsZero(), Equals, true)
	balanceAcc3 := mgr.Keeper().GetBalanceOf(ctx, acc3, common.RuneNative)
	c.Assert(balanceAcc3.IsZero(), Equals, true)
	balanceAcc4 := mgr.Keeper().GetBalanceOf(ctx, acc4, common.RuneNative)
	c.Assert(balanceAcc4.IsZero(), Equals, true)

	balanceTCY := mgr.Keeper().GetBalanceOf(ctx, tcyStakeAddress, common.RuneNative)
	c.Assert(balanceTCY.Amount.Equal(math.NewInt(tcyStakeFeeAmount)), Equals, true)
	c.Assert(balanceTCY.Denom, Equals, common.RuneNative.Native())

	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address1), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address2), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address3), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address4), Equals, true)

	// Mint 210M RUNE to TCYStakeName (fund already has MinRuneForTCYStakeDistribution)
	tcyStakeFeeAmount = 210_000_000_00000000 - tcyStakeFeeAmount
	c.Assert(tcyStakeFeeAmount > 0, Equals, true)
	FundModule(c, ctx, mgr.Keeper(), TCYStakeName, uint64(tcyStakeFeeAmount))
	balanceTCY = mgr.Keeper().GetBalanceOf(ctx, tcyStakeAddress, common.RuneNative)
	c.Assert(balanceTCY.Amount.Equal(math.NewInt(210_000_000_00000000)), Equals, true)

	nmgr.distributeTCYStake(ctx, mgr)

	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address1), Equals, true)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address2), Equals, true)
	// Staking for address 3 should be deleted
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address3), Equals, false)
	c.Assert(mgr.Keeper().TCYStakerExists(ctx, address4), Equals, true)

	// Check balances, accounts should have their corresponding part: 75% to acc1,
	// 25% to acc2, 0% to acc4 and claiming the corresponding part of acc4.
	// TCYStake should not have funds after the distribution
	balanceAcc1 = mgr.Keeper().GetBalanceOf(ctx, acc1, common.RuneNative)
	c.Assert(balanceAcc1.Amount.Equal(math.NewInt(157_499_999_99950000)), Equals, true)
	c.Assert(balanceAcc1.Denom, Equals, common.RuneNative.Native())

	balanceAcc2 = mgr.Keeper().GetBalanceOf(ctx, acc2, common.RuneNative)
	c.Assert(balanceAcc2.Amount.Equal(math.NewInt(52_499_999_99950001)), Equals, true)
	c.Assert(balanceAcc2.Denom, Equals, common.RuneNative.Native())

	balanceAcc4 = mgr.Keeper().GetBalanceOf(ctx, acc4, common.RuneNative)
	c.Assert(balanceAcc4.IsZero(), Equals, true)

	balanceClaiming := mgr.Keeper().GetBalanceOfModule(ctx, TCYClaimingName, common.RuneNative.Native())
	c.Assert(balanceClaiming.Equal(math.NewUint(99999)), Equals, true)

	balanceTCY = mgr.Keeper().GetBalanceOf(ctx, tcyStakeAddress, common.RuneNative)
	c.Assert(balanceTCY.Amount.IsZero(), Equals, true)

	// Move acc1, acc2 and claiming module RUNE balances to zero
	coin = common.NewCoin(common.RuneNative, cosmos.NewUint(balanceAcc1.Amount.Uint64()))
	err = mgr.Keeper().SendFromAccountToModule(ctx, acc1, ModuleName, common.NewCoins(coin))
	c.Assert(err, IsNil)
	balanceAcc1 = mgr.Keeper().GetBalanceOf(ctx, acc1, common.RuneNative)
	c.Assert(balanceAcc1.Amount.IsZero(), Equals, true)

	coin = common.NewCoin(common.RuneNative, cosmos.NewUint(balanceAcc2.Amount.Uint64()))
	err = mgr.Keeper().SendFromAccountToModule(ctx, acc2, ModuleName, common.NewCoins(coin))
	c.Assert(err, IsNil)
	balanceAcc2 = mgr.Keeper().GetBalanceOf(ctx, acc2, common.RuneNative)
	c.Assert(balanceAcc2.Amount.IsZero(), Equals, true)

	coin = common.NewCoin(common.RuneNative, cosmos.NewUint(balanceClaiming.Uint64()))
	err = mgr.Keeper().SendFromModuleToModule(ctx, TCYClaimingName, ModuleName, common.NewCoins(coin))
	c.Assert(err, IsNil)
	balanceClaiming = mgr.Keeper().GetBalanceOfModule(ctx, TCYClaimingName, common.RuneNative.Native())
	c.Assert(balanceClaiming.IsZero(), Equals, true)

	// Change distribution to acc1 = 50%, acc2 = 25% and acc4 = 25%
	// remove amountAddr4
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address1,
		Amount:  math.NewUint(105_000_000_00000000),
	})
	c.Assert(err, IsNil)
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address2,
		Amount:  math.NewUint(52_500_000_00000000),
	})
	c.Assert(err, IsNil)
	err = mgr.Keeper().SetTCYStaker(ctx, TCYStaker{
		Address: address3,
		Amount:  math.NewUint(52_500_000_00000000),
	})
	c.Assert(err, IsNil)

	// Mint 420M RUNE to TCYStakeName
	tcyStakeFeeAmount = 420_000_000_00000000
	FundModule(c, ctx, mgr.Keeper(), TCYStakeName, uint64(tcyStakeFeeAmount))

	nmgr.distributeTCYStake(ctx, mgr)

	// Check balances, accounts should have their corresponding part: 50% to acc1,
	// 25% to acc2, 25% to acc3 and acc4 should not receive rune.
	// TCYStake should not have funds after the distribution
	balanceAcc1 = mgr.Keeper().GetBalanceOf(ctx, acc1, common.RuneNative)
	c.Assert(balanceAcc1.Amount.Equal(math.NewInt(210_000_000_00000000)), Equals, true)
	c.Assert(balanceAcc1.Denom, Equals, common.RuneNative.Native())

	balanceAcc2 = mgr.Keeper().GetBalanceOf(ctx, acc2, common.RuneNative)
	c.Assert(balanceAcc2.Amount.Equal(math.NewInt(105_000_000_00000000)), Equals, true)
	c.Assert(balanceAcc2.Denom, Equals, common.RuneNative.Native())

	balanceAcc3 = mgr.Keeper().GetBalanceOf(ctx, acc3, common.RuneNative)
	c.Assert(balanceAcc3.Amount.Equal(math.NewInt(105_000_000_00000000)), Equals, true)
	c.Assert(balanceAcc3.Denom, Equals, common.RuneNative.Native())

	balanceAcc4 = mgr.Keeper().GetBalanceOf(ctx, acc4, common.RuneNative)
	c.Assert(balanceAcc4.IsZero(), Equals, true)

	balanceTCY = mgr.Keeper().GetBalanceOf(ctx, tcyStakeAddress, common.RuneNative)
	c.Assert(balanceTCY.Amount.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestGetTCYStakeAmountToDistribute(c *C) {
	_, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())
	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)
	minRune := constAccessor.GetInt64Value(constants.MinRuneForTCYStakeDistribution)

	// If funds are less than MinRuneForTCYStakeDistribution it should NOT distribute that amount
	amount := uint64(minRune - 1)
	tcyStakeAmount := cosmos.NewUint(amount)
	result := nmgr.getTCYStakeAmountToDistribute(tcyStakeAmount, minRune)
	c.Assert(result.IsZero(), Equals, true)

	// If funds are equal to MinRuneForTCYStakeDistribution it should distribute that amount
	amount = uint64(minRune)
	tcyStakeAmount = cosmos.NewUint(amount)
	result = nmgr.getTCYStakeAmountToDistribute(tcyStakeAmount, minRune)
	c.Assert(result.IsZero(), Equals, false)
	c.Assert(result.Equal(tcyStakeAmount), Equals, true)

	// If funds are equal to 2x MinRuneForTCYStakeDistribution it should distribute that amount
	amount = uint64(minRune * 2)
	tcyStakeAmount = cosmos.NewUint(amount)
	result = nmgr.getTCYStakeAmountToDistribute(tcyStakeAmount, minRune)
	c.Assert(result.IsZero(), Equals, false)
	c.Assert(result.Equal(tcyStakeAmount), Equals, true)

	// If funds are equal to 2.5x MinRuneForTCYStakeDistribution it should only distribute 2x
	amount = uint64(float64(minRune) * 2.5)
	amoutMul2 := uint64(minRune * 2)
	tcyStakeAmount = cosmos.NewUint(amount)
	result = nmgr.getTCYStakeAmountToDistribute(tcyStakeAmount, minRune)
	c.Assert(result.IsZero(), Equals, false)
	c.Assert(result.Equal(cosmos.NewUint(amoutMul2)), Equals, true)
}

func (s *NetworkManagerTestSuite) TestCalculateNetworkSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool with some balance
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.PendingInboundAsset = cosmos.NewUint(10 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create an active vault with funds
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Test 1: Calculate solvency - should show over-solvency
	// Vault has 150, Pool needs 100 + 10 pending = 110, so 40 over-solvent
	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	c.Assert(solvency[0].Asset.Equals(common.BTCAsset), Equals, true)
	expectedOverSolvency := math.NewInt(40 * common.One)
	c.Assert(solvency[0].Amount.Equal(expectedOverSolvency), Equals, true)

	// Test 2: Under-solvency scenario
	// Reduce vault balance so it's under-solvent
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(50*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	expectedUnderSolvency := math.NewInt(-60 * common.One)
	c.Assert(solvency[0].Amount.Equal(expectedUnderSolvency), Equals, true)

	// Test 3: Multiple assets
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(200 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(200 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(110*common.One)),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(250*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 2)

	// Test 4: With swap queue items
	swapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.BTCChain,
			Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(5*common.One))},
		},
		TargetAsset: common.RuneAsset(),
	}
	c.Assert(mgr.Keeper().SetSwapQueueItem(ctx, swapMsg, 0), IsNil)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 2)
	// Should subtract the swap queue amount from vault balance

	// Test 5: Empty vaults
	vault.Coins = common.Coins{}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	// All should be under-solvent (negative)
	for _, s := range solvency {
		c.Assert(s.Amount.IsNegative(), Equals, true)
	}

	// Test 6: All THORChain native assets should be excluded (synths, derived, whitelisted)
	// Create a synth pool (BTC/BTC)
	synthBTC := common.BTCAsset.GetSyntheticAsset()
	synthPool := NewPool()
	synthPool.Asset = synthBTC
	synthPool.BalanceAsset = cosmos.NewUint(80 * common.One)
	synthPool.BalanceRune = cosmos.NewUint(80 * common.One)
	synthPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, synthPool), IsNil)

	// Create a derived asset pool (THOR.BTC)
	derivedBTCAsset, err := common.NewAsset("THOR.BTC")
	c.Assert(err, IsNil)
	derivedPool := NewPool()
	derivedPool.Asset = derivedBTCAsset
	derivedPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	derivedPool.BalanceRune = cosmos.NewUint(100 * common.One)
	derivedPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, derivedPool), IsNil)

	// Create a whitelisted native asset pool (RUJI)
	rujiPool := NewPool()
	rujiPool.Asset = common.RUJI
	rujiPool.BalanceAsset = cosmos.NewUint(50 * common.One)
	rujiPool.BalanceRune = cosmos.NewUint(50 * common.One)
	rujiPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, rujiPool), IsNil)

	// Reset vault with only L1 BTC and ETH
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(110*common.One)),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(200*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	// Should only have BTC and ETH, excluding all THORChain native assets
	c.Assert(len(solvency), Equals, 2)
	for _, s := range solvency {
		c.Assert(s.Asset.IsNative(), Equals, false, Commentf("THORChain native asset %s should not be in solvency", s.Asset))
		c.Assert(s.Asset.Equals(common.BTCAsset) || s.Asset.Equals(common.ETHAsset), Equals, true)
	}
}

func (s *NetworkManagerTestSuite) TestCalculateNetworkSolvencyWithStreamingSwaps(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool with some balance
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One) // Pool has 100 BTC from LPs + processed streaming swaps
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create an active vault with funds
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)), // Vault has 150 BTC (includes 50 BTC streaming deposit)
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Test 1: Non-streaming swap - should deduct full amount
	txID := GetRandomTxHash()
	swapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    txID,
			Chain: common.BTCChain,
			Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(10*common.One))},
		},
		TargetAsset: common.RuneAsset(),
	}
	c.Assert(mgr.Keeper().SetSwapQueueItem(ctx, swapMsg, 0), IsNil)

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	// Vault: 150, Pool: 100, SwapQueue: 10 → Solvency: 150 - 100 - 10 = 40
	c.Assert(solvency[0].Amount.Equal(math.NewInt(40*common.One)), Equals, true,
		Commentf("Non-streaming swap should deduct full amount, expected 40, got %s", solvency[0].Amount))

	// Clean up non-streaming swap
	mgr.Keeper().RemoveSwapQueueItem(ctx, txID, 0)

	// Test 2: Legacy streaming swap with partial progress - should only deduct remaining amount
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.BTCChain,
			Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(50*common.One))}, // Full deposit: 50 BTC
		},
		TargetAsset:    common.RuneAsset(),
		StreamInterval: 10, // Makes it a legacy streaming swap (V1)
		Version:        types.SwapVersion_v1,
	}
	c.Assert(mgr.Keeper().SetSwapQueueItem(ctx, streamingSwapMsg, 0), IsNil)

	// Create streaming swap state showing 20 BTC has been processed
	streamingSwap := types.NewStreamingSwap(streamingTxID, 5, 10, cosmos.ZeroUint(), cosmos.NewUint(50*common.One))
	streamingSwap.In = cosmos.NewUint(20 * common.One) // 20 BTC processed, 30 BTC remaining
	mgr.Keeper().SetStreamingSwap(ctx, streamingSwap)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	// Vault: 150, Pool: 100 (includes 20 BTC from processed streaming swap), SwapQueue: 30 (remaining)
	// Solvency: 150 - 100 - 30 = 20
	// If we incorrectly deducted full deposit: 150 - 100 - 50 = 0 (wrong!)
	c.Assert(solvency[0].Amount.Equal(math.NewInt(20*common.One)), Equals, true,
		Commentf("Streaming swap should only deduct remaining amount, expected 20, got %s", solvency[0].Amount))

	// Clean up
	mgr.Keeper().RemoveSwapQueueItem(ctx, streamingTxID, 0)
	mgr.Keeper().RemoveStreamingSwap(ctx, streamingTxID)

	// Test 3: Advanced swap queue with streaming swap (V2)
	advStreamingTxID := GetRandomTxHash()
	advStreamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    advStreamingTxID,
			Chain: common.BTCChain,
			Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(30*common.One))}, // Full deposit: 30 BTC
		},
		TargetAsset: common.RuneAsset(),
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10, // Makes it a streaming swap (State.Quantity > 1)
			Count:    3,
			Interval: 5,
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, advStreamingSwapMsg), IsNil)

	// Create streaming swap state showing 12 BTC has been processed
	advStreamingSwap := types.NewStreamingSwap(advStreamingTxID, 10, 5, cosmos.ZeroUint(), cosmos.NewUint(30*common.One))
	advStreamingSwap.In = cosmos.NewUint(12 * common.One) // 12 BTC processed, 18 BTC remaining
	mgr.Keeper().SetStreamingSwap(ctx, advStreamingSwap)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	// Vault: 150, Pool: 100, AdvSwapQueue: 18 (remaining)
	// Solvency: 150 - 100 - 18 = 32
	c.Assert(solvency[0].Amount.Equal(math.NewInt(32*common.One)), Equals, true,
		Commentf("V2 streaming swap should only deduct remaining amount, expected 32, got %s", solvency[0].Amount))

	// Clean up
	c.Assert(mgr.Keeper().RemoveAdvSwapQueueItem(ctx, advStreamingTxID, 0), IsNil)
	mgr.Keeper().RemoveStreamingSwap(ctx, advStreamingTxID)

	// Test 4: Streaming swap with no progress yet - should deduct full amount
	newStreamingTxID := GetRandomTxHash()
	newStreamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    newStreamingTxID,
			Chain: common.BTCChain,
			Coins: common.Coins{common.NewCoin(common.BTCAsset, cosmos.NewUint(25*common.One))},
		},
		TargetAsset:    common.RuneAsset(),
		StreamInterval: 10,
		Version:        types.SwapVersion_v1,
	}
	c.Assert(mgr.Keeper().SetSwapQueueItem(ctx, newStreamingSwapMsg, 0), IsNil)

	// Create streaming swap state with zero progress
	newStreamingSwap := types.NewStreamingSwap(newStreamingTxID, 5, 10, cosmos.ZeroUint(), cosmos.NewUint(25*common.One))
	// In is already zero from NewStreamingSwap
	mgr.Keeper().SetStreamingSwap(ctx, newStreamingSwap)

	solvency, err = nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	// Vault: 150, Pool: 100, SwapQueue: 25 (full, since In=0)
	// Solvency: 150 - 100 - 25 = 25
	c.Assert(solvency[0].Amount.Equal(math.NewInt(25*common.One)), Equals, true,
		Commentf("Streaming swap with no progress should deduct full amount, expected 25, got %s", solvency[0].Amount))
}

func (s *NetworkManagerTestSuite) TestSolvencyStreamingSwapTargetOutput(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create ETH and BTC pools
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(100 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceAsset = cosmos.NewUint(80 * common.One) // Pool already emitted 20 BTC to streaming swap output
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Create an active vault
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(150*common.One)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Create a V2 streaming swap ETH -> BTC with accumulated output
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.ETHChain,
			Coins: common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(50*common.One))}, // 50 ETH deposit
		},
		TargetAsset: common.BTCAsset,
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10,
			Count:    6,
			Interval: 5,
			Deposit:  cosmos.NewUint(50 * common.One),
			In:       cosmos.NewUint(30 * common.One), // 30 ETH processed
			Out:      cosmos.NewUint(20 * common.One), // 20 BTC emitted from pool
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, streamingSwapMsg), IsNil)

	// No legacy SetStreamingSwap here — this test must exercise the V2 msg.State path only.

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 2)

	// Find ETH and BTC solvency
	var ethSolvency, btcSolvency math.Int
	for _, s := range solvency {
		if s.Asset.Equals(common.ETHAsset) {
			ethSolvency = s.Amount
		}
		if s.Asset.Equals(common.BTCAsset) {
			btcSolvency = s.Amount
		}
	}

	// ETH: Vault 150 - Pool 100 - SwapQueueRemaining 20 = 30
	c.Assert(ethSolvency.Equal(math.NewInt(30*common.One)), Equals, true,
		Commentf("ETH solvency: expected 30, got %s", ethSolvency))

	// BTC: Vault 100 - Pool 80 - StreamingOutput 20 = 0
	// Without the fix, it would be: Vault 100 - Pool 80 = 20 (incorrect over-solvency)
	c.Assert(btcSolvency.Equal(math.NewInt(0)), Equals, true,
		Commentf("BTC solvency: expected 0 (streaming output subtracted), got %s", btcSolvency))
}

// TestSolvencyStreamingSwapTargetOutputFromNativeSource reproduces the case
// where a V2 streaming swap starts from native RUNE and emits external-chain
// target assets. The current implementation skips queue processing too early
// for native sources, so the emitted target liability is not deducted.
func (s *NetworkManagerTestSuite) TestSolvencyStreamingSwapTargetOutputFromNativeSource(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// The BTC pool has already emitted 20 BTC to the in-flight stream, but the
	// vault still holds the coins because settleSwap has not created an outbound yet.
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceAsset = cosmos.NewUint(80 * common.One) // Pool already emitted 20 BTC
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Intentionally rely on inline msg.State only. This exercises the V2 path the
	// MR 4640 changed, without falling back to the legacy streaming swap record.
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.THORChain,
			Coins: common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(50*common.One))},
		},
		TargetAsset: common.BTCAsset,
		SwapType:    types.SwapType_market,
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10,
			Count:    6,
			Interval: 5,
			Deposit:  cosmos.NewUint(50 * common.One),
			In:       cosmos.NewUint(30 * common.One),
			Out:      cosmos.NewUint(20 * common.One),
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, streamingSwapMsg), IsNil)

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)
	c.Assert(solvency[0].Asset.Equals(common.BTCAsset), Equals, true)

	// BTC: Vault 100 - Pool 80 - StreamingOutput 20 = 0
	c.Assert(solvency[0].Amount.Equal(math.ZeroInt()), Equals, true,
		Commentf("BTC solvency: expected 0 when native-source streaming output is deducted, got %s", solvency[0].Amount))
}

// TestSolvencyStreamingSwapTargetSynthOutput reproduces the case where a
// streaming swap target is synth. Synth output should not create an external
// Layer1 liability, because the swap path mints synths without reducing the L1
// pool asset depth.
func (s *NetworkManagerTestSuite) TestSolvencyStreamingSwapTargetSynthOutput(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(100 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Keep the BTC pool unchanged: synth output should not reduce L1 BTC depth.
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One) // Synth output should not reduce this
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(150*common.One)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Again, rely on inline msg.State only so this test covers the new V2
	// streaming solvency accounting path directly.
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.ETHChain,
			Coins: common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(50*common.One))},
		},
		TargetAsset: common.BTCAsset.GetSyntheticAsset(),
		SwapType:    types.SwapType_market,
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10,
			Count:    6,
			Interval: 5,
			Deposit:  cosmos.NewUint(50 * common.One),
			In:       cosmos.NewUint(30 * common.One),
			Out:      cosmos.NewUint(20 * common.One),
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, streamingSwapMsg), IsNil)

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 2)

	var ethSolvency, btcSolvency math.Int
	for _, s := range solvency {
		if s.Asset.Equals(common.ETHAsset) {
			ethSolvency = s.Amount
		}
		if s.Asset.Equals(common.BTCAsset) {
			btcSolvency = s.Amount
		}
	}

	// ETH: Vault 150 - Pool 100 - SwapQueueRemaining 20 = 30
	c.Assert(ethSolvency.Equal(math.NewInt(30*common.One)), Equals, true,
		Commentf("ETH solvency: expected 30, got %s", ethSolvency))

	// BTC synth output should not create an L1 BTC liability.
	c.Assert(btcSolvency.Equal(math.ZeroInt()), Equals, true,
		Commentf("BTC solvency: expected 0 when synth output is ignored for L1 solvency, got %s", btcSolvency))
}

// TestSolvencyStreamingSwapNativeSourceTargetOutput verifies that RUNE -> L1 streaming swaps
// still subtract accumulated target output even though the source asset (RUNE) is native.
func (s *NetworkManagerTestSuite) TestSolvencyStreamingSwapNativeSourceTargetOutput(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: BTC pool where BalanceAsset is already reduced by streaming swap output
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceAsset = cosmos.NewUint(80 * common.One) // Pool emitted 20 BTC
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Vault holds 100 BTC
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// V2 streaming swap: RUNE -> BTC (native source, L1 target)
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.THORChain,
			Coins: common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(50*common.One))},
		},
		TargetAsset: common.BTCAsset,
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10,
			Count:    6,
			Interval: 5,
			Deposit:  cosmos.NewUint(50 * common.One),
			In:       cosmos.NewUint(30 * common.One),
			Out:      cosmos.NewUint(20 * common.One), // 20 BTC emitted
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, streamingSwapMsg), IsNil)

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	var btcSolvency math.Int
	for _, s := range solvency {
		if s.Asset.Equals(common.BTCAsset) {
			btcSolvency = s.Amount
		}
	}

	// BTC: Vault 100 - Pool 80 - StreamingOutput 20 = 0
	// Without the fix, the native-source continue skips target output accounting,
	// yielding: Vault 100 - Pool 80 = 20 (incorrectly over-solvent)
	c.Assert(btcSolvency.Equal(math.NewInt(0)), Equals, true,
		Commentf("BTC solvency: expected 0 (RUNE->BTC streaming output subtracted), got %s", btcSolvency))
}

// TestSolvencyStreamingSwapSynthTargetNotSubtracted verifies that streaming swaps targeting
// synth/trade/secured assets do NOT subtract output from L1 solvency, because those assets
// are minted without reducing pool.BalanceAsset.
func (s *NetworkManagerTestSuite) TestSolvencyStreamingSwapSynthTargetNotSubtracted(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: BTC pool with full balance (no L1 assets emitted)
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	btcPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Vault holds 100 BTC
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// V2 streaming swap: RUNE -> BTC/BTC (synth target)
	// Synth minting does NOT reduce pool.BalanceAsset, so output should NOT be subtracted.
	synthTarget := common.BTCAsset.GetSyntheticAsset()
	streamingTxID := GetRandomTxHash()
	streamingSwapMsg := types.MsgSwap{
		Tx: common.Tx{
			ID:    streamingTxID,
			Chain: common.THORChain,
			Coins: common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(50*common.One))},
		},
		TargetAsset: synthTarget,
		Version:     types.SwapVersion_v2,
		State: &types.SwapState{
			Quantity: 10,
			Count:    6,
			Interval: 5,
			Deposit:  cosmos.NewUint(50 * common.One),
			In:       cosmos.NewUint(30 * common.One),
			Out:      cosmos.NewUint(20 * common.One), // 20 synth BTC minted (not L1)
		},
	}
	c.Assert(mgr.Keeper().SetAdvSwapQueueItem(ctx, streamingSwapMsg), IsNil)

	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	var btcSolvency math.Int
	for _, s := range solvency {
		if s.Asset.Equals(common.BTCAsset) {
			btcSolvency = s.Amount
		}
	}

	// BTC: Vault 100 - Pool 100 = 0
	// The synth streaming output (20) must NOT be subtracted from L1 solvency.
	// Without the fix, it would be: Vault 100 - Pool 100 - SynthOutput 20 = -20 (incorrectly under-solvent)
	c.Assert(btcSolvency.Equal(math.NewInt(0)), Equals, true,
		Commentf("BTC solvency: expected 0 (synth output NOT subtracted), got %s", btcSolvency))
}

func (s *NetworkManagerTestSuite) TestProcessPostChurnSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create an active vault with over-solvent funds
	activeVault := GetRandomVault()
	activeVault.Type = AsgardVault
	activeVault.Status = ActiveVault
	activeVault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, activeVault), IsNil)

	// Test 1: Not at interval block - should not process
	ctx = ctx.WithBlockHeight(100)
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyCheckInterval.String(), 432000)
	err := nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	// Test 2: At interval block - should process
	ctx = ctx.WithBlockHeight(432000)

	// Set OverSolvencyToTreasuryBps to 0 - should emit event but not create swaps
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyToTreasuryBps.String(), 0)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	// Test 3: With OverSolvencyToTreasuryBps set to 5000 (50%)
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyToTreasuryBps.String(), 5000)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	// Verify swap was added to queue
	// Note: This depends on which queue is enabled
	if mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		iter := mgr.Keeper().GetAdvSwapQueueItemIterator(ctx)
		defer iter.Close()
		hasSwap := false
		for ; iter.Valid(); iter.Next() {
			hasSwap = true
			break
		}
		c.Assert(hasSwap, Equals, true)
	} else {
		iter := mgr.Keeper().GetSwapQueueIterator(ctx)
		defer iter.Close()
		hasSwap := false
		for ; iter.Valid(); iter.Next() {
			hasSwap = true
			break
		}
		c.Assert(hasSwap, Equals, true)
	}

	// Test 4: No over-solvency case
	activeVault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(50*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, activeVault), IsNil)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	// Should complete without error even when under-solvent
}

func (s *NetworkManagerTestSuite) TestCreateSwapToOverSolvency(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create an active vault with BTC asset
	vaultPubKey := GetRandomPubKey()
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Setup: Create a pool for the asset
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Test 1: Create swap to over-solvency
	swapAmount := cosmos.NewUint(10 * common.One)
	err := nmgr.createSwapToOverSolvency(ctx, mgr, common.BTCAsset, swapAmount)
	c.Assert(err, IsNil)

	// Verify swap was created in the queue
	var foundSwap *types.MsgSwap
	if mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		iter := mgr.Keeper().GetAdvSwapQueueItemIterator(ctx)
		defer iter.Close()
		for ; iter.Valid(); iter.Next() {
			var msg types.MsgSwap
			unmarshalErr := mgr.Keeper().Cdc().Unmarshal(iter.Value(), &msg)
			c.Assert(unmarshalErr, IsNil)
			foundSwap = &msg
			break
		}
	} else {
		iter := mgr.Keeper().GetSwapQueueIterator(ctx)
		defer iter.Close()
		for ; iter.Valid(); iter.Next() {
			var msg types.MsgSwap
			unmarshalErr := mgr.Keeper().Cdc().Unmarshal(iter.Value(), &msg)
			c.Assert(unmarshalErr, IsNil)
			foundSwap = &msg
			break
		}
	}

	c.Assert(foundSwap, NotNil)
	c.Assert(foundSwap.Tx.Coins[0].Asset.Equals(common.BTCAsset), Equals, true)
	c.Assert(foundSwap.Tx.Coins[0].Amount.Equal(swapAmount), Equals, true)
	c.Assert(foundSwap.TargetAsset.Equals(common.RuneAsset()), Equals, true)

	// Verify streaming swap parameters
	c.Assert(foundSwap.StreamInterval, Equals, uint64(1))
	c.Assert(foundSwap.StreamQuantity, Equals, uint64(1))

	// Verify sender uses chain-appropriate address from vault
	btcAddr, err := vaultPubKey.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)
	c.Assert(foundSwap.Tx.FromAddress.String(), Equals, btcAddr.String())
	c.Assert(foundSwap.Tx.ToAddress.String(), Equals, btcAddr.String())

	// Verify destination is over-solvency address
	overSolvencyAddr := mgr.Keeper().GetConstants().GetStringValue(constants.OverSolvencyAddress)
	c.Assert(foundSwap.Destination.String(), Equals, overSolvencyAddr)

	// Test 2: Multiple swaps - add vault for ETH chain
	ethVaultPubKey := GetRandomPubKey()
	ethVault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, ethVaultPubKey, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	ethVault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, ethVault), IsNil)

	err = nmgr.createSwapToOverSolvency(ctx, mgr, common.ETHAsset, cosmos.NewUint(5*common.One))
	c.Assert(err, IsNil)
	// Should succeed and add another swap to queue
}

func (s *NetworkManagerTestSuite) TestSolvencyWithScheduledOutbounds(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create vault with funds
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Add scheduled outbound transaction
	txOut := &TxOut{
		Height: ctx.BlockHeight() + 1,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(20*common.One)),
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: vault.PubKey,
			},
		},
	}
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	// Calculate solvency - should account for scheduled outbound
	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)

	// Vault has 150, pool needs 100, scheduled outbound 20
	// Expected: 150 - 100 - 20 = 30 over-solvent
	expectedSolvency := math.NewInt(30 * common.One)
	c.Assert(solvency[0].Amount.Equal(expectedSolvency), Equals, true)
}

func (s *NetworkManagerTestSuite) TestSolvencyWithPendingOutboundsAtPastHeight(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create vault with funds
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Add a pending outbound at a PAST block height (OutHash empty - not yet signed).
	// This simulates an outbound that was scheduled a few blocks ago but Bifrost
	// hasn't signed it yet. The solvency calc should still deduct it.
	pastHeight := ctx.BlockHeight() - 5
	if pastHeight < 1 {
		pastHeight = 1
	}
	txOut := &TxOut{
		Height: pastHeight,
		TxArray: []TxOutItem{
			{
				Chain:       common.BTCChain,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(20*common.One)),
				ToAddress:   GetRandomBTCAddress(),
				VaultPubKey: vault.PubKey,
				// OutHash is empty - not yet signed
			},
		},
	}
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	// Calculate solvency - should account for pending outbound at past height
	solvency, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency), Equals, 1)

	// Vault has 150, pool needs 100, pending outbound 20
	// Expected: 150 - 100 - 20 = 30 over-solvent
	expectedSolvency := math.NewInt(30 * common.One)
	c.Assert(solvency[0].Amount.Equal(expectedSolvency), Equals, true,
		Commentf("expected %s, got %s", expectedSolvency.String(), solvency[0].Amount.String()))

	// Now simulate the outbound being signed (OutHash set).
	// When OutHash is set, vault.SubFunds has already been called in the same block,
	// so the item should NOT be deducted anymore.
	txOut.TxArray[0].OutHash = GetRandomTxHash()
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	// Also decrement vault coins (as vault.SubFunds would do on observation)
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(130*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Calculate solvency again - should still be 30 (130 - 100 = 30)
	// The signed outbound should be skipped, and vault coins already reflect the send
	solvency2, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency2), Equals, 1)
	c.Assert(solvency2[0].Amount.Equal(expectedSolvency), Equals, true,
		Commentf("expected %s, got %s", expectedSolvency.String(), solvency2[0].Amount.String()))
}

func (s *NetworkManagerTestSuite) TestSolvencyWithTradeAndSecuredAssets(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create a pool
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Create vault with funds
	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(150*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Calculate solvency - verifies calculation logic
	solvency1, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency1), Equals, 1)

	// Vault 150 - Pool 100 = 50 over-solvent
	expected := math.NewInt(50 * common.One)
	c.Assert(solvency1[0].Amount.Equal(expected), Equals, true)

	// Test with trade assets: Add trade asset deposits
	// Trade assets are tracked in KVStore (not bank keeper)
	tradeAsset := common.BTCAsset.GetTradeAsset()
	tu := types.NewTradeUnit(tradeAsset)
	tu.Units = cosmos.NewUint(10 * common.One)
	tu.Depth = cosmos.NewUint(10 * common.One)
	mgr.Keeper().SetTradeUnit(ctx, tu)

	// Calculate solvency with trade assets
	solvency2, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency2), Equals, 1)

	// Vault 150 - Pool 100 - TradeDepth 10 = 40 over-solvent
	expected = math.NewInt(40 * common.One)
	c.Assert(solvency2[0].Amount.Equal(expected), Equals, true)

	// Test with secured assets: Add secured asset deposits
	// Secured assets are minted as bank keeper tokens
	securedAsset := common.BTCAsset.GetSecuredAsset()
	addr := GetRandomBech32Addr()
	coin := common.NewCoin(securedAsset, cosmos.NewUint(15*common.One))
	err = mgr.Keeper().MintAndSendToAccount(ctx, addr, coin)
	c.Assert(err, IsNil)

	// Create secured asset pool metadata
	securedPool := types.NewSecuredAsset(securedAsset)
	securedPool.Depth = cosmos.NewUint(15 * common.One)
	mgr.Keeper().SetSecuredAsset(ctx, securedPool)

	// Calculate solvency with both trade and secured assets
	solvency3, err := nmgr.calculateNetworkSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	c.Assert(len(solvency3), Equals, 1)

	// Vault 150 - Pool 100 - TradeDepth 10 - SecuredDepth 15 = 25 over-solvent
	expected = math.NewInt(25 * common.One)
	c.Assert(solvency3[0].Amount.Equal(expected), Equals, true)
}

func (s *NetworkManagerTestSuite) TestProcessPostChurnSolvencyEdgeCases(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Set block height to interval so checks will run
	ctx = ctx.WithBlockHeight(432000)
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyCheckInterval.String(), 432000)

	// Test 1: No pools - should not error
	err := nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	// Test 2: No vaults - should not error
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)

	// Test 3: OverSolvencyToTreasuryBps exceeds maximum (should cap at 10000)
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyToTreasuryBps.String(), 15000)

	vault := GetRandomVault()
	vault.Type = AsgardVault
	vault.Status = ActiveVault
	vault.Coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(200*common.One)),
	}
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	// Should cap at 100% and not error

	// Test 4: Negative mimir value (should treat as 0)
	mgr.Keeper().SetMimir(ctx, constants.OverSolvencyToTreasuryBps.String(), -100)

	err = nmgr.processOverSolvency(ctx, mgr)
	c.Assert(err, IsNil)
	// Should not create any swaps when set to 0
}

func (s *NetworkManagerTestSuite) TestSwapToOverSolvencyIncome(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create an active vault with BTC asset
	vaultPubKey := GetRandomPubKey()
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Setup: Create a pool for BTC
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool.BalanceRune = cosmos.NewUint(2000 * common.One) // 2:1 RUNE:BTC ratio
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Test 1: Process L1 asset (BTC) as income
	coin := common.NewCoin(common.BTCAsset, cosmos.NewUint(10*common.One))
	err := nmgr.SwapToOverSolvencyIncome(ctx, mgr, coin)
	if mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		c.Assert(err, IsNil)
		// Verify swap was created in the queue (fee tracking happens during swap execution)
		iter := mgr.Keeper().GetAdvSwapQueueItemIterator(ctx)
		defer iter.Close()
		var foundSwap bool
		for ; iter.Valid(); iter.Next() {
			var msg types.MsgSwap
			unmarshalErr := mgr.Keeper().Cdc().Unmarshal(iter.Value(), &msg)
			c.Assert(unmarshalErr, IsNil)
			if msg.Tx.Coins[0].Asset.Equals(common.BTCAsset) {
				foundSwap = true
				c.Assert(msg.Tx.Coins[0].Amount.Equal(coin.Amount), Equals, true)
				c.Assert(msg.TargetAsset.Equals(common.RuneAsset()), Equals, true)
			}
		}
		c.Assert(foundSwap, Equals, true, Commentf("swap should have been created in queue"))
	} else {
		c.Assert(err, NotNil, Commentf("should return error when AdvSwapQueue is disabled"))
	}

	// Test 2: Process RUNE directly to over-solvency address
	FundModule(c, ctx, mgr.Keeper(), AsgardName, 100*common.One)
	runeCoin := common.NewCoin(common.RuneAsset(), cosmos.NewUint(50*common.One))
	overSolvencyAddrStr := mgr.Keeper().GetConstants().GetStringValue(constants.OverSolvencyAddress)
	overSolvencyAccAddr, addrErr := cosmos.AccAddressFromBech32(overSolvencyAddrStr)
	c.Assert(addrErr, IsNil)
	runeBalanceBefore := mgr.Keeper().GetBalanceOf(ctx, overSolvencyAccAddr, common.RuneAsset())

	err = nmgr.SwapToOverSolvencyIncome(ctx, mgr, runeCoin)
	c.Assert(err, IsNil)

	// Verify RUNE was transferred to over-solvency address
	runeBalanceAfter := mgr.Keeper().GetBalanceOf(ctx, overSolvencyAccAddr, common.RuneAsset())
	c.Assert(runeBalanceAfter.Amount.Sub(runeBalanceBefore.Amount).Uint64(), Equals, runeCoin.Amount.Uint64())

	// Test 3: Zero amount should return nil without error
	zeroCoin := common.NewCoin(common.BTCAsset, cosmos.ZeroUint())
	err = nmgr.SwapToOverSolvencyIncome(ctx, mgr, zeroCoin)
	c.Assert(err, IsNil)

	// Test 4: Empty coin should return nil without error
	emptyCoin := common.Coin{}
	err = nmgr.SwapToOverSolvencyIncome(ctx, mgr, emptyCoin)
	c.Assert(err, IsNil)

	// Test 5: Asset with no vault should return an error (no active vault found)
	noPoolCoin := common.NewCoin(common.LTCAsset, cosmos.NewUint(10*common.One))
	err = nmgr.SwapToOverSolvencyIncome(ctx, mgr, noPoolCoin)
	c.Assert(err, NotNil) // No active vault for LTC, so swap creation fails
}

func (s *NetworkManagerTestSuite) TestSwapToOverSolvencyIncomeTradeAsset(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create an active vault with BTC asset
	vaultPubKey := GetRandomPubKey()
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Setup: Create a pool for BTC (L1)
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool.BalanceRune = cosmos.NewUint(3000 * common.One) // 3:1 RUNE:BTC ratio
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Test: Process trade asset - swap created for later execution
	tradeAsset := common.BTCAsset.GetTradeAsset()
	tradeCoin := common.NewCoin(tradeAsset, cosmos.NewUint(5*common.One))
	err := nmgr.SwapToOverSolvencyIncome(ctx, mgr, tradeCoin)
	c.Assert(err, IsNil)

	// Verify swap was created (fee tracking happens during swap execution)
	if mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		iter := mgr.Keeper().GetAdvSwapQueueItemIterator(ctx)
		defer iter.Close()
		var foundSwap bool
		for ; iter.Valid(); iter.Next() {
			var msg types.MsgSwap
			unmarshalErr := mgr.Keeper().Cdc().Unmarshal(iter.Value(), &msg)
			c.Assert(unmarshalErr, IsNil)
			if msg.Tx.Coins[0].Asset.Equals(tradeAsset) {
				foundSwap = true
				c.Assert(msg.Tx.Coins[0].Amount.Equal(tradeCoin.Amount), Equals, true)
				c.Assert(msg.TargetAsset.Equals(common.RuneAsset()), Equals, true)
			}
		}
		c.Assert(foundSwap, Equals, true, Commentf("swap should have been created in queue"))
	}
}

func (s *NetworkManagerTestSuite) TestSwapToOverSolvencyIncomeSecuredAsset(c *C) {
	ctx, mgr := setupManagerForTest(c)
	nmgr := newNetworkMgr(mgr.Keeper(), NewTxStoreDummy(), NewDummyEventMgr())

	// Setup: Create an active vault with BTC asset
	vaultPubKey := GetRandomPubKey()
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, vaultPubKey, common.Chains{common.BTCChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Setup: Create a pool for BTC (L1)
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	pool.BalanceRune = cosmos.NewUint(4000 * common.One) // 4:1 RUNE:BTC ratio
	pool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	// Test: Process secured asset - swap created for later execution
	securedAsset := common.BTCAsset.GetSecuredAsset()
	securedCoin := common.NewCoin(securedAsset, cosmos.NewUint(2*common.One))
	err := nmgr.SwapToOverSolvencyIncome(ctx, mgr, securedCoin)
	c.Assert(err, IsNil)

	// Verify swap was created (fee tracking happens during swap execution)
	if mgr.Keeper().AdvSwapQueueEnabled(ctx) {
		iter := mgr.Keeper().GetAdvSwapQueueItemIterator(ctx)
		defer iter.Close()
		var foundSwap bool
		for ; iter.Valid(); iter.Next() {
			var msg types.MsgSwap
			unmarshalErr := mgr.Keeper().Cdc().Unmarshal(iter.Value(), &msg)
			c.Assert(unmarshalErr, IsNil)
			if msg.Tx.Coins[0].Asset.Equals(securedAsset) {
				foundSwap = true
				c.Assert(msg.Tx.Coins[0].Amount.Equal(securedCoin.Amount), Equals, true)
				c.Assert(msg.TargetAsset.Equals(common.RuneAsset()), Equals, true)
			}
		}
		c.Assert(foundSwap, Equals, true, Commentf("swap should have been created in queue"))
	}
}

func (s *NetworkManagerTestSuite) TestCalcBlockRewardsPOLReserveDeduct(c *C) {
	ctx, k := setupKeeperForTest(c)
	mgr := NewDummyMgr()
	networkMgr := newNetworkMgr(k, mgr.TxOutStore(), mgr.EventMgr())

	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)

	availablePoolsRune := cosmos.NewUint(1000 * common.One)
	vaultsLiquidityRune := cosmos.NewUint(1000 * common.One)
	effectiveSecurityBond := cosmos.NewUint(2000 * common.One)
	totalEffectiveBond := cosmos.NewUint(2000 * common.One)
	totalReserve := cosmos.NewUint(10000 * common.One)
	totalLiquidityFees := cosmos.NewUint(1000)
	emissionCurve := constAccessor.GetInt64Value(constants.EmissionCurve)
	blocksPerYear := constAccessor.GetInt64Value(constants.BlocksPerYear)

	// With 0 bps, polReserveDeduct should be zero
	_, _, _, _, _, _, _, polReserveDeduct := networkMgr.calcBlockRewards(ctx,
		availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond,
		totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve,
		blocksPerYear, 0, 0, 0, 0, 0)
	c.Check(polReserveDeduct.IsZero(), Equals, true)

	// With 500 bps (5%), polReserveDeduct should reduce bond+pool rewards
	bondR1, poolR1, _, _, _, _, _, _ := networkMgr.calcBlockRewards(ctx,
		availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond,
		totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve,
		blocksPerYear, 0, 0, 0, 0, 0)
	totalWithout := bondR1.Add(poolR1)

	bondR2, poolR2, _, _, _, _, _, polReserveDeduct := networkMgr.calcBlockRewards(ctx,
		availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond,
		totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve,
		blocksPerYear, 0, 0, 0, 0, 500)
	totalWith := bondR2.Add(poolR2)
	c.Check(polReserveDeduct.IsZero(), Equals, false)
	// The deducted amount plus remaining should equal original
	c.Check(totalWith.Add(polReserveDeduct).String(), Equals, totalWithout.String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleDisabled(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	// maxDeployment = 0 means disabled
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), 0)
	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleZeroBalance(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	// Enable deployment but don't fund the module
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), 10_000_000_000)
	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleBestPool(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	poolBTC := NewPool()
	poolBTC.Asset = common.BTCAsset
	poolBTC.Status = PoolAvailable
	poolBTC.BalanceRune = cosmos.NewUint(1000 * common.One)
	poolBTC.BalanceAsset = cosmos.NewUint(10 * common.One)
	poolBTC.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, poolBTC), IsNil)

	poolETH := NewPool()
	poolETH.Asset = common.ETHAsset
	poolETH.Status = PoolAvailable
	poolETH.BalanceRune = cosmos.NewUint(500 * common.One)
	poolETH.BalanceAsset = cosmos.NewUint(50 * common.One)
	poolETH.LPUnits = cosmos.NewUint(500 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, poolETH), IsNil)

	// BTC score = 100 / 100000000000 ~= 0
	// ETH score = 200 / 50000000000 ~= 0 but higher ratio
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.ETHAsset, cosmos.NewUint(200)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// ETH pool should have received LP since it has higher fees/depth
	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.String(), Equals, cosmos.NewUint(50*common.One).String())

	// BTC pool should have no deposit
	depositBTC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(depositBTC.RuneDeposited.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleCapEnforced(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 500*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.String(), Equals, cosmos.NewUint(100*common.One).String())

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(400*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleSkipsPausedPools(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	mgr.Keeper().SetMimir(ctx, "PauseLPBTC", ctx.BlockHeight())

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.IsZero(), Equals, true)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleSkipsGlobalTradingHalt(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))
	mgr.Keeper().SetMimir(ctx, "HaltTrading", ctx.BlockHeight())

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.IsZero(), Equals, true)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleSkipsChainTradingHalt(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))
	mgr.Keeper().SetMimir(ctx, "HaltBTCTrading", ctx.BlockHeight())

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.IsZero(), Equals, true)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleNoEligiblePools(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleSkipsRagnarokedPools(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	mgr.Keeper().SetMimir(ctx, "RAGNAROK-BTC-BTC", 1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleNegativeMimir(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	// Setting -1 (common disable pattern) should be treated as 0 by SafeUintFromInt64
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), -1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil) // Should exit early with no error, not wrap to MaxUint64
}

// polReserveNonGasAsset returns a non-gas, non-anchor ERC20 pool asset used
// by the eligibility filter tests.
func polReserveNonGasAsset(c *C) common.Asset {
	asset, err := common.NewAsset("ETH.USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48")
	c.Assert(err, IsNil)
	return asset
}

// polReserveSetupTwoPoolCycle creates a BTC.BTC pool (gas asset) and a
// higher-scoring non-gas ETH.USDC pool, funds the pol_reserve module, and
// sets POLReserveMaxDeployment. Without any other mimirs the filter's
// default set (gas + TOR anchor) restricts deployment to BTC.BTC.
//
// An ETH.ETH pool is also created (with no rolling fees so it never scores)
// because the add-liquidity handler requires the ERC20 asset's gas-asset
// pool to exist before it will accept a deposit into ETH.USDC.
func polReserveSetupTwoPoolCycle(c *C, ctx cosmos.Context, mgr Manager) (common.Asset, common.Asset) {
	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	poolBTC := NewPool()
	poolBTC.Asset = common.BTCAsset
	poolBTC.Status = PoolAvailable
	poolBTC.BalanceRune = cosmos.NewUint(1000 * common.One)
	poolBTC.BalanceAsset = cosmos.NewUint(10 * common.One)
	poolBTC.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, poolBTC), IsNil)

	// ETH.ETH exists solely to satisfy the add-liquidity handler's gas-pool
	// requirement for ERC20 deposits. It gets no rolling fees, so it never
	// scores in POLReserveCycle's selection.
	poolETH := NewPool()
	poolETH.Asset = common.ETHAsset
	poolETH.Status = PoolAvailable
	poolETH.BalanceRune = cosmos.NewUint(1000 * common.One)
	poolETH.BalanceAsset = cosmos.NewUint(10 * common.One)
	poolETH.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, poolETH), IsNil)

	usdc := polReserveNonGasAsset(c)
	poolUSDC := NewPool()
	poolUSDC.Asset = usdc
	poolUSDC.Status = PoolAvailable
	poolUSDC.BalanceRune = cosmos.NewUint(500 * common.One)
	poolUSDC.BalanceAsset = cosmos.NewUint(50 * common.One)
	poolUSDC.LPUnits = cosmos.NewUint(500 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, poolUSDC), IsNil)

	// USDC has the higher score (more fees on a smaller pool), so without the
	// eligibility filter it would win the deployment.
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)
	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, usdc, cosmos.NewUint(10000)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	return common.BTCAsset, usdc
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleDefaultEligibleSet(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	btc, usdc := polReserveSetupTwoPoolCycle(c, ctx, mgr)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// BTC is a gas asset => eligible and funded despite having the lower score.
	depositBTC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, btc)
	c.Assert(err, IsNil)
	c.Check(depositBTC.RuneDeposited.String(), Equals, cosmos.NewUint(50*common.One).String())

	// USDC is neither a gas asset nor a TOR anchor => excluded by default.
	depositUSDC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, usdc)
	c.Assert(err, IsNil)
	c.Check(depositUSDC.RuneDeposited.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleTorAnchorEligible(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	btc, usdc := polReserveSetupTwoPoolCycle(c, ctx, mgr)

	// Promote USDC into the default set via the TorAnchor mimir.
	mgr.Keeper().SetMimir(ctx, "TorAnchor-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// USDC is now eligible and its higher score beats BTC.
	depositUSDC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, usdc)
	c.Assert(err, IsNil)
	c.Check(depositUSDC.RuneDeposited.String(), Equals, cosmos.NewUint(50*common.One).String())

	depositBTC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, btc)
	c.Assert(err, IsNil)
	c.Check(depositBTC.RuneDeposited.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleWhitelistOverride(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	btc, usdc := polReserveSetupTwoPoolCycle(c, ctx, mgr)

	// Force-include USDC without setting a TorAnchor mimir.
	mgr.Keeper().SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	depositUSDC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, usdc)
	c.Assert(err, IsNil)
	c.Check(depositUSDC.RuneDeposited.String(), Equals, cosmos.NewUint(50*common.One).String())

	depositBTC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, btc)
	c.Assert(err, IsNil)
	c.Check(depositBTC.RuneDeposited.IsZero(), Equals, true)
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleBlacklistOverridesDefault(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.Status = PoolAvailable
	pool.BalanceRune = cosmos.NewUint(1000 * common.One)
	pool.BalanceAsset = cosmos.NewUint(10 * common.One)
	pool.LPUnits = cosmos.NewUint(1000 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	c.Assert(mgr.Keeper().AddToLiquidityFees(ctx, common.BTCAsset, cosmos.NewUint(100)), IsNil)

	FundModule(c, ctx, mgr.Keeper(), POLReserveName, 50*common.One)
	mgr.Keeper().SetMimir(ctx, constants.POLReserveMaxDeployment.String(), int64(100*common.One))

	// Blacklist the only gas-asset pool.
	mgr.Keeper().SetMimir(ctx, "POLReserveBlacklist-BTC-BTC", 1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	deposit, err := mgr.Keeper().GetPOLReserveDeposit(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(deposit.RuneDeposited.IsZero(), Equals, true)

	remaining := mgr.Keeper().GetRuneBalanceOfModule(ctx, POLReserveName)
	c.Check(remaining.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestPOLReserveCycleBlacklistOverridesWhitelist(c *C) {
	ctx, mgr := setupManagerForTest(c)
	networkMgr := newNetworkMgr(mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())

	btc, usdc := polReserveSetupTwoPoolCycle(c, ctx, mgr)

	// Whitelist AND blacklist the same pool => blacklist wins.
	mgr.Keeper().SetMimir(ctx, "POLReserveWhitelist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)
	mgr.Keeper().SetMimir(ctx, "POLReserveBlacklist-ETH-USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", 1)

	err := networkMgr.POLReserveCycle(ctx, mgr)
	c.Assert(err, IsNil)

	// USDC blocked; BTC (gas asset) is the only remaining candidate and gets funded.
	depositUSDC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, usdc)
	c.Assert(err, IsNil)
	c.Check(depositUSDC.RuneDeposited.IsZero(), Equals, true)

	depositBTC, err := mgr.Keeper().GetPOLReserveDeposit(ctx, btc)
	c.Assert(err, IsNil)
	c.Check(depositBTC.RuneDeposited.String(), Equals, cosmos.NewUint(50*common.One).String())
}

func (s *NetworkManagerTestSuite) TestCalcBlockRewardsPOLReserveHighBPSClamping(c *C) {
	ctx, k := setupKeeperForTest(c)
	mgr := NewDummyMgr()
	networkMgr := newNetworkMgr(k, mgr.TxOutStore(), mgr.EventMgr())

	ver := GetCurrentVersion()
	constAccessor := constants.GetConstantValues(ver)

	availablePoolsRune := cosmos.NewUint(1000 * common.One)
	vaultsLiquidityRune := cosmos.NewUint(1000 * common.One)
	effectiveSecurityBond := cosmos.NewUint(2000 * common.One)
	totalEffectiveBond := cosmos.NewUint(2000 * common.One)
	totalReserve := cosmos.NewUint(10000 * common.One)
	totalLiquidityFees := cosmos.NewUint(1000)
	emissionCurve := constAccessor.GetInt64Value(constants.EmissionCurve)
	blocksPerYear := constAccessor.GetInt64Value(constants.BlocksPerYear)

	// Set all five deductions to 3000 BPS each (15000 total > 10000)
	// The clamping cascade should ensure POL reserve gets whatever remains
	_, _, _, devFundDeduct, burnDeduct, tcyDeduct, marketingDeduct, polReserveDeduct := networkMgr.calcBlockRewards(ctx,
		availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond,
		totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve,
		blocksPerYear, 3000, 3000, 3000, 3000, 3000)

	// Total deducted cannot exceed original system income
	bondR0, poolR0, _, _, _, _, _, _ := networkMgr.calcBlockRewards(ctx,
		availablePoolsRune, vaultsLiquidityRune, effectiveSecurityBond,
		totalEffectiveBond, totalReserve, totalLiquidityFees, emissionCurve,
		blocksPerYear, 0, 0, 0, 0, 0)
	originalSystemIncome := bondR0.Add(poolR0)

	totalDeducted := devFundDeduct.Add(burnDeduct).Add(tcyDeduct).Add(marketingDeduct).Add(polReserveDeduct)
	c.Check(totalDeducted.LTE(originalSystemIncome), Equals, true,
		Commentf("total deducted %s exceeds system income %s", totalDeducted.String(), originalSystemIncome.String()))

	// POL reserve (last in chain) should be squeezed — less than the naive 3000/10000 share
	naivePOLShare := common.GetSafeShare(cosmos.NewUint(3000), cosmos.NewUint(10_000), originalSystemIncome)
	c.Check(polReserveDeduct.LT(naivePOLShare), Equals, true,
		Commentf("polReserveDeduct %s should be less than naive %s", polReserveDeduct.String(), naivePOLShare.String()))
}
