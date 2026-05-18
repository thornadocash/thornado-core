package thorchain

import (
	"strings"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type GasManagerTestSuite struct{}

var _ = Suite(&GasManagerTestSuite{})

// resetMultiplierForAsset resets the fee multiplier for an asset to the given multiplier
func resetMultiplierForAsset(ctx cosmos.Context, k keeper.Keeper, asset common.Asset, multiplier cosmos.Uint) error {
	surplus := k.GetSurplusForTargetMultiplier(ctx, multiplier)
	spent, err := k.GetOutboundFeeSpentRune(ctx, asset)
	if err != nil {
		return err
	}
	withheld, err := k.GetOutboundFeeWithheldRune(ctx, asset)
	if err != nil {
		return err
	}
	if surplus.GT(withheld.Sub(spent)) {
		return k.AddToOutboundFeeWithheldRune(ctx, asset, surplus.Sub(withheld.Sub(spent)))
	} else {
		return k.AddToOutboundFeeSpentRune(ctx, asset, withheld.Sub(spent).Sub(surplus))
	}
}

func (GasManagerTestSuite) TestGasManager(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.K
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)
	gasEvent := gasMgr.gasEvent
	c.Assert(gasMgr, NotNil)
	gasMgr.BeginBlock()
	c.Assert(gasEvent != gasMgr.gasEvent, Equals, true)

	pool := NewPool()
	pool.Asset = common.ETHAsset
	c.Assert(k.SetPool(ctx, pool), IsNil)
	pool.Asset = common.BTCAsset
	c.Assert(k.SetPool(ctx, pool), IsNil)

	gasMgr.AddGasAsset(common.EmptyAsset, common.Gas{
		common.NewCoin(common.ATOMAsset, cosmos.NewUint(37500)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)),
	}, true)
	c.Assert(gasMgr.GetGas(), HasLen, 2)
	gasMgr.AddGasAsset(common.EmptyAsset, common.Gas{
		common.NewCoin(common.ATOMAsset, cosmos.NewUint(38500)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(2000)),
	}, true)
	c.Assert(gasMgr.GetGas(), HasLen, 2)
	gasMgr.AddGasAsset(common.EmptyAsset, common.Gas{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(38500)),
	}, true)
	c.Assert(gasMgr.GetGas(), HasLen, 3)
	eventMgr := NewDummyEventMgr()
	gasMgr.EndBlock(ctx, k, eventMgr)
}

func (GasManagerTestSuite) TestGetAssetOutboundFee(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)
	gasMgr.BeginBlock()

	// when there is no network fee available, 0 fee and nil error should be returned
	fee, err := gasMgr.GetAssetOutboundFee(ctx, common.AVAXAsset, true)
	c.Assert(fee.Uint64(), Equals, cosmos.ZeroUint().Uint64())
	c.Assert(err, IsNil)

	// should still return nil error if inRune is false
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.AVAXAsset, false)
	c.Assert(fee.Uint64(), Equals, cosmos.ZeroUint().Uint64())
	c.Assert(err, IsNil)

	// set network fee
	networkFee := NewNetworkFee(common.AVAXChain, 1, 10_000) // 10,000 nAVAX
	c.Assert(k.SaveNetworkFee(ctx, common.AVAXChain, networkFee), IsNil)

	// when there is a network fee available, but no gas asset pool, the fee should still be returned
	pool, _ := k.GetPool(ctx, common.AVAXAsset)
	c.Assert(pool.IsEmpty(), Equals, true)
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.AVAXAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(1000))
	c.Assert(err, IsNil)

	// conversion to rune requires a pool, so should return 0 and no error
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.AVAXAsset, true)
	c.Assert(fee.Uint64(), Equals, uint64(0))
	c.Assert(err, IsNil)

	// set pool
	c.Assert(k.SetPool(ctx, Pool{
		BalanceRune:  cosmos.NewUint(100 * common.One),
		BalanceAsset: cosmos.NewUint(100 * common.One),
		Asset:        common.AVAXAsset,
		Status:       PoolAvailable,
	}), IsNil)

	// conversion to rune should now work
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.AVAXAsset, true)
	c.Assert(fee.Uint64(), Equals, uint64(1000), Commentf("%d vs %d", fee.Uint64(), uint64(1000)))
	c.Assert(err, IsNil)

	// BTC chain
	networkFee = NewNetworkFee(common.BTCChain, 70, 50)
	c.Assert(k.SaveNetworkFee(ctx, common.BTCChain, networkFee), IsNil)

	// No gas pool set, but not needed if no conversion is needed, network fee should be returned
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(err, IsNil)
	c.Assert(fee.Uint64(), Equals, uint64(70*50), Commentf("%d vs %d", fee.Uint64(), uint64(70*50)))

	c.Assert(k.SetPool(ctx, Pool{
		BalanceRune:  cosmos.NewUint(100 * common.One),
		BalanceAsset: cosmos.NewUint(100 * common.One),
		Asset:        common.BTCAsset,
		Status:       PoolAvailable,
	}), IsNil)
	fee, _ = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(70*50), Commentf("%d vs %d", fee.Uint64(), uint64(70*50)))

	// Synth asset (BTC/BTC)
	sBTC, err := common.NewAsset("BTC/BTC")
	c.Assert(err, IsNil)

	// change the pool balance
	c.Assert(k.SetPool(ctx, Pool{
		BalanceRune:  cosmos.NewUint(500 * common.One),
		BalanceAsset: cosmos.NewUint(100 * common.One),
		Asset:        common.BTCAsset,
		Status:       PoolAvailable,
	}), IsNil)
	synthAssetFee, err := gasMgr.GetAssetOutboundFee(ctx, sBTC, false)
	c.Assert(synthAssetFee.Uint64(), Equals, uint64(0))
	c.Assert(err, IsNil)

	// Trade asset
	tradeAsset, err := common.NewAsset("BTC~BTC")
	c.Assert(err, IsNil)
	tradeAssetFee, err := gasMgr.GetAssetOutboundFee(ctx, tradeAsset, false)
	c.Assert(tradeAssetFee.Uint64(), Equals, uint64(0))
	c.Assert(err, IsNil)

	// A trade asset with inRune true should return the fee in RUNE, not in the trade asset.
	runeFee, err := gasMgr.GetAssetOutboundFee(ctx, tradeAsset, true)
	c.Assert(err, IsNil)
	c.Assert(runeFee.String(), Equals, "0")

	// when MinimumL1OutboundFeeUSD set to something higher, it should override the network fee
	busdAsset, err := common.NewAsset("BSC.BUSD-BD1")
	c.Assert(err, IsNil)
	c.Assert(k.SetPool(ctx, Pool{
		BalanceRune:  cosmos.NewUint(500 * common.One),
		BalanceAsset: cosmos.NewUint(500 * common.One),
		Decimals:     8,
		Asset:        busdAsset,
		Status:       PoolAvailable,
	}), IsNil)
	k.SetMimir(ctx, constants.MinimumL1OutboundFeeUSD.String(), 1_0000_0000)
	k.SetMimir(ctx, "TorAnchor-BSC-BUSD-BD1", 1) // enable BUSD pool as a TOR anchor

	fee, _ = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(20000000), Commentf("%d", fee.Uint64()))

	// when network fee is higher than MinimumL1OutboundFeeUSD, then choose network fee
	networkFee = NewNetworkFee(common.BTCChain, 1000, 50000)
	c.Assert(k.SaveNetworkFee(ctx, common.BTCChain, networkFee), IsNil)
	fee, _ = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(50000000), Commentf("%d vs %d", fee.Uint64(), uint64(50000000)))

	// DynamicOutboundFeeMultiplier
	// set mimirs:
	// target surplus: 100 RUNE
	// min multiplier: 10_000
	// max multiplier: 30_000

	// k.SetMimir(ctx, constants.TargetOutboundFeeSurplusRune.String(), 100_00000000) // 100 $RUNE
	// k.SetMimir(ctx, constants.MinOutboundFeeMultiplierBasisPoints.String(), 10_000)
	// k.SetMimir(ctx, constants.MaxOutboundFeeMultiplierBasisPoints.String(), 30_000)

	// No surplus to start, initial surplus is set, fee should return with 1x multiplier
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(1000*50000), Commentf("%d vs %d", fee.Uint64(), uint64(1000*50000)))
	c.Assert(err, IsNil)

	// Add a surplus for BTC - multiplier should be 50% of max-min (i.e. 2x)
	c.Assert(resetMultiplierForAsset(ctx, k, common.BTCAsset, cosmos.NewUint(20_000)), IsNil)
	fee, err = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	c.Assert(fee.Uint64(), Equals, uint64(1000*50000*2), Commentf("%d vs %d", fee.Uint64(), uint64(1000*50000*2)))
	c.Assert(err, IsNil)

	// Add more surplus for BTC, should be at min multiplier
	c.Assert(k.AddToOutboundFeeWithheldRune(ctx, common.BTCAsset, cosmos.NewUint(50_00000000)), IsNil)
	fee, _ = gasMgr.GetAssetOutboundFee(ctx, common.BTCAsset, false)
	// This condition hits the minimum outbound fee threshold, so the fee is 99275000 instead of 1000*50000*0.1
	c.Assert(fee.Uint64(), Equals, uint64(99275000), Commentf("%d vs %d", fee.Uint64(), uint64(99275000)))
	c.Assert(err, IsNil)

	// Add a hypothetical asset on BTC, which should have a different multiplier than BTC
	btcUsd, err := common.NewAsset("BTC.USDC")
	c.Assert(err, IsNil)
	c.Assert(k.SetPool(ctx, Pool{
		BalanceRune:  cosmos.NewUint(500 * common.One),
		BalanceAsset: cosmos.NewUint(200 * common.One),
		Asset:        btcUsd,
		Status:       PoolAvailable,
	}), IsNil)

	fee, err = gasMgr.GetAssetOutboundFee(ctx, btcUsd, false)
	c.Assert(fee.Uint64(), Equals, uint64(1000*50000*2), Commentf("%d vs %d", fee.Uint64(), uint64(1000*50000*2))) // BTC.USDC should have 1x initial multiplier
	c.Assert(err, IsNil)

	// Add a surplus for BTC.USDC - multiplier should be 50% of max-min (i.e. 2x)
	c.Assert(resetMultiplierForAsset(ctx, k, btcUsd, cosmos.NewUint(20_000)), IsNil)
	fee, err = gasMgr.GetAssetOutboundFee(ctx, btcUsd, false)
	c.Assert(fee.Uint64(), Equals, uint64(2*(1000*50000*2)), Commentf("%d vs %d", fee.Uint64(), uint64(2*(1000*50000*2))))
	c.Assert(err, IsNil)

	// Add more surplus for BTC.USDC, should be at min multiplier
	c.Assert(k.AddToOutboundFeeWithheldRune(ctx, btcUsd, cosmos.NewUint(50_00000000)), IsNil)
	fee, err = gasMgr.GetAssetOutboundFee(ctx, btcUsd, false)
	// This condition hits the minimum outbound fee threshold, so the fee is 99275000*2 instead of 1000*50000*0.1
	c.Assert(fee.Uint64(), Equals, uint64(99275000*2), Commentf("%d vs %d", fee.Uint64(), uint64(99275000*2)))
	c.Assert(err, IsNil)
}

func (GasManagerTestSuite) TestDifferentValidations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)
	gasMgr.BeginBlock()
	helper := newGasManagerTestHelper(k)
	eventMgr := NewDummyEventMgr()
	gasMgr.EndBlock(ctx, helper, eventMgr)

	helper.failGetNetwork = true
	gasMgr.EndBlock(ctx, helper, eventMgr)
	helper.failGetNetwork = false

	helper.failGetPool = true
	gasMgr.AddGasAsset(common.EmptyAsset, common.Gas{
		common.NewCoin(common.ATOMAsset, cosmos.NewUint(37500)),
		common.NewCoin(common.BTCAsset, cosmos.NewUint(1000)),
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
	}, true)
	gasMgr.EndBlock(ctx, helper, eventMgr)
	helper.failGetPool = false
	helper.failSetPool = true
	p := NewPool()
	p.Asset = common.ATOMAsset
	p.BalanceAsset = cosmos.NewUint(common.One * 100)
	p.BalanceRune = cosmos.NewUint(common.One * 100)
	p.Status = PoolAvailable
	c.Assert(helper.Keeper.SetPool(ctx, p), IsNil)
	gasMgr.AddGasAsset(common.EmptyAsset, common.Gas{
		common.NewCoin(common.ATOMAsset, cosmos.NewUint(37500)),
	}, true)
	gasMgr.EndBlock(ctx, helper, eventMgr)
}

func (GasManagerTestSuite) TestGetMaxGas(c *C) {
	ctx, k := setupKeeperForTest(c)
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)

	gasCoin, err := gasMgr.GetMaxGas(ctx, common.GAIAChain)
	c.Assert(gasCoin.Amount.IsZero(), Equals, true)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "network fee for chain(GAIA) is invalid"), Equals, true)
	// Code relying on GetMaxGas should not proceed when not able to do a valid calculation.

	var transactionSize uint64 = 1000
	var transactionFeeRate uint64 = 127 // 127 uatom gas rate
	networkFee := NewNetworkFee(common.GAIAChain, transactionSize, transactionFeeRate)
	c.Assert(k.SaveNetworkFee(ctx, common.GAIAChain, networkFee), IsNil)
	gasCoin, err = gasMgr.GetMaxGas(ctx, common.GAIAChain)
	c.Assert(err, IsNil)
	decimals := gasCoin.Asset.Chain.GetGasAssetDecimal()
	c.Assert(decimals, Equals, int64(6))
	c.Assert(gasCoin.Amount.Uint64(), Equals, uint64(1000*(127*3/2)*1e2))
	// 1e2 uatom (1e6) -> THORChain Amount (1e8) conversion

	networkFee = NewNetworkFee(common.ETHChain, 123, 127) // 127 gwei gas rate
	c.Assert(k.SaveNetworkFee(ctx, common.ETHChain, networkFee), IsNil)
	gasCoin, err = gasMgr.GetMaxGas(ctx, common.ETHChain)
	c.Assert(err, IsNil)
	c.Assert(gasCoin.Amount.String(), Equals, "2337")
	// 10 gwei (1e9) -> THORChain Amount (1e8) conversion
}

func (GasManagerTestSuite) TestOutboundFeeMultiplier(c *C) {
	ctx, k := setupKeeperForTest(c)
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)

	targetSurplus := cosmos.NewUint(100_00000000) // 100 $RUNE
	minMultiplier := cosmos.NewUint(15_000)
	maxMultiplier := cosmos.NewUint(20_000)
	gasSpent := cosmos.ZeroUint()
	gasWithheld := cosmos.ZeroUint()

	// No surplus to start, should return maxMultiplier
	m := gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, maxMultiplier.Uint64())

	// More gas spent than withheld, use maxMultiplier
	gasSpent = cosmos.NewUint(1000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, maxMultiplier.Uint64())

	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(110_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(19_500), Commentf("%d", m.Uint64()))

	// 50% surplus vs target, reduce multiplier by 50%
	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(150_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(17_500), Commentf("%d", m.Uint64()))

	// 75% surplus vs target, reduce multiplier by 75%
	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(175_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(16_250), Commentf("%d", m.Uint64()))

	// 99% surplus vs target, reduce multiplier by 99%
	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(199_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(15_050), Commentf("%d", m.Uint64()))

	// 100% surplus vs target, reduce multiplier by 100%
	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(200_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(15_000), Commentf("%d", m.Uint64()))

	// 110% surplus vs target, still reduce multiplier by 100%
	gasSpent = cosmos.NewUint(100_00000000)
	gasWithheld = cosmos.NewUint(210_00000000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(15_000))

	// If min multiplier somehow gets set above max multiplier, multiplier should return old default (3x)
	maxMultiplier = cosmos.NewUint(10_000)
	m = gasMgr.CalcOutboundFeeMultiplier(ctx, targetSurplus, gasSpent, gasWithheld, maxMultiplier, minMultiplier)
	c.Assert(m.Uint64(), Equals, uint64(30_000), Commentf("%d", m.Uint64()))
}

func (GasManagerTestSuite) TestGetGasDetailsLowRate(c *C) {
	ctx, k := setupKeeperForTest(c)
	constAccessor := constants.GetConstantValues(GetCurrentVersion())
	gasMgr := newGasMgr(constAccessor, k)

	// test with stagenet state case with observed issue on BASE (1e12 gas rate)

	var transactionSize uint64 = 100000
	var transactionFeeRate uint64 = 1000
	networkFee := NewNetworkFee(common.BASEChain, transactionSize, transactionFeeRate)
	c.Assert(k.SaveNetworkFee(ctx, common.BASEChain, networkFee), IsNil)

	maxGasCoin, gasRate, err := gasMgr.GetGasDetails(ctx, common.BASEChain)
	c.Assert(err, IsNil)
	c.Assert(maxGasCoin.Amount.Uint64(), Equals, uint64(15000))
	c.Assert(gasRate, Equals, int64(1500))
}
