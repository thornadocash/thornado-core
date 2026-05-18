package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

// GasMgr implement GasManager interface which will store the gas related events happened in thorchain to memory
// emit GasEvent per block if there are any
type GasMgr struct {
	gasEvent          *EventGas
	outAssetGas       []OutAssetGas
	gasCount          map[common.Asset]int64
	constantsAccessor constants.ConstantValues
	keeper            keeper.Keeper
}

// newGasMgr create a new instance of GasMgr
func newGasMgr(constantsAccessor constants.ConstantValues, k keeper.Keeper) *GasMgr {
	return &GasMgr{
		gasEvent:          NewEventGas(),
		outAssetGas:       []OutAssetGas{},
		gasCount:          make(map[common.Asset]int64),
		constantsAccessor: constantsAccessor,
		keeper:            k,
	}
}

func (gm *GasMgr) reset() {
	gm.gasEvent = NewEventGas()
	gm.outAssetGas = []OutAssetGas{}
	gm.gasCount = make(map[common.Asset]int64)
}

// BeginBlock need to be called when a new block get created , update the internal EventGas to new one
func (gm *GasMgr) BeginBlock() {
	gm.reset()
}

// AddGasAsset for EndBlock's ProcessGas;
// add the outbound-Asset-associated Gas to the gas manager's outAssetGas,
// and optionally increment the gas manager's gasCount.
func (gm *GasMgr) AddGasAsset(outAsset common.Asset, gas common.Gas, increaseTxCount bool) {
	matched := false
	for i := range gm.outAssetGas {
		if !gm.outAssetGas[i].outAsset.Equals(outAsset) {
			continue
		}
		matched = true
		gm.outAssetGas[i].gas = gm.outAssetGas[i].gas.Add(gas...)
		break
	}
	if !matched {
		outAssetGas := OutAssetGas{
			outAsset: outAsset,
			gas:      common.Gas(common.NewCoins(gas...)), // Copied contents
		}
		gm.outAssetGas = append(gm.outAssetGas, outAssetGas)
	}

	// Update transaction count for each gas asset.
	if !increaseTxCount {
		return
	}

	incremented := map[common.Asset]bool{}
	for i := range gas {
		// Only increment each distinct gas asset's count by 1 maximum.
		if incremented[gas[i].Asset] {
			continue
		}
		gm.gasCount[gas[i].Asset]++
		incremented[gas[i].Asset] = true
	}
}

// GetGas return gas
func (gm *GasMgr) GetGas() common.Gas {
	// Collect gas by gas asset.
	gas := common.Gas{}
	for i := range gm.outAssetGas {
		gas = gas.Add(gm.outAssetGas[i].gas...)
	}
	return gas
}

// GetAssetOutboundFee returns current outbound fee for the asset. fee = chainBaseFee *
// assetDOFM (asset-specific Dynamic Outbound Fee Multiplier)
// - asset: the asset to calculate the fee for
// - inRune: whether the fee should be returned in RUNE. If false the fee is returned in
// asset units.
func (gm *GasMgr) GetAssetOutboundFee(ctx cosmos.Context, asset common.Asset, inRune bool) (cosmos.Uint, error) {
	// If the asset is native to THORChain, no need to charge an outbound fee.
	if asset.IsNative() {
		return cosmos.ZeroUint(), nil
	}

	chainOutboundFee, err := gm.keeper.GetNetworkFee(ctx, asset.GetChain())
	if err != nil {
		return cosmos.ZeroUint(), err
	}
	if err = chainOutboundFee.Valid(); err != nil {
		// If the network fee is invalid, usually because consensus hasn't been reached, a
		// fee can't be deducted. So return 0 and no error
		return cosmos.ZeroUint(), nil
	}

	gasPool, err := gm.keeper.GetPool(ctx, asset.GetChain().GetGasAsset())
	if err != nil {
		return cosmos.ZeroUint(), err
	}
	minOutboundUSD, err := gm.keeper.GetMimir(ctx, constants.MinimumL1OutboundFeeUSD.String())
	if minOutboundUSD < 0 || err != nil {
		minOutboundUSD = gm.constantsAccessor.GetInt64Value(constants.MinimumL1OutboundFeeUSD)
	}
	runeUSDPrice := gm.keeper.DollarsPerRune(ctx)
	minAsset := cosmos.ZeroUint()
	if !runeUSDPrice.IsZero() {
		// since MinOutboundUSD is in USD value , thus need to figure out how much RUNE
		// here use GetShare instead GetSafeShare it is because minOutboundUSD can set to more than $1
		minOutboundInRune := common.GetUncappedShare(cosmos.NewUint(uint64(minOutboundUSD)),
			runeUSDPrice,
			cosmos.NewUint(common.One))

		minAsset = gasPool.RuneValueInAsset(minOutboundInRune)
	}

	outboundFeeWithheldRune, err := gm.keeper.GetOutboundFeeWithheldRune(ctx, asset)
	if err != nil {
		ctx.Logger().Error("fail to get outbound fee withheld rune", "outbound asset", asset, "error", err)
		outboundFeeWithheldRune = cosmos.ZeroUint()
	}
	outboundFeeSpentRune, err := gm.keeper.GetOutboundFeeSpentRune(ctx, asset)
	if err != nil {
		ctx.Logger().Error("fail to get outbound fee spent rune", "outbound asset", asset, "error", err)
		outboundFeeSpentRune = cosmos.ZeroUint()
	}

	targetOutboundFeeSurplus := gm.keeper.GetConfigInt64(ctx, constants.TargetOutboundFeeSurplusRune)
	maxMultiplierBasisPoints := gm.keeper.GetConfigInt64(ctx, constants.MaxOutboundFeeMultiplierBasisPoints)
	minMultiplierBasisPoints := gm.keeper.GetConfigInt64(ctx, constants.MinOutboundFeeMultiplierBasisPoints)

	// Calculate outbound fee based on current fee multiplier
	_, gasRateUnitsPerOne := asset.GetChain().GetGasUnits()
	chainBaseFee := cosmos.NewUint(chainOutboundFee.TransactionSize).MulUint64(chainOutboundFee.TransactionFeeRate).MulUint64(common.One).Quo(gasRateUnitsPerOne)
	feeMultiplierBps := gm.CalcOutboundFeeMultiplier(ctx, cosmos.NewUint(uint64(targetOutboundFeeSurplus)), outboundFeeSpentRune, outboundFeeWithheldRune, cosmos.NewUint(uint64(maxMultiplierBasisPoints)), cosmos.NewUint(uint64(minMultiplierBasisPoints)))
	finalFee := common.GetUncappedShare(feeMultiplierBps, cosmos.NewUint(constants.MaxBasisPts), chainBaseFee)

	fee := cosmos.RoundToDecimal(
		finalFee,
		gasPool.Decimals,
	)

	// Ensure fee is always more than minAsset
	if fee.LT(minAsset) {
		fee = minAsset
	}

	// If feeAsset = gas asset && inRune = false, we are in the correct units, return
	if asset.Equals(asset.GetChain().GetGasAsset()) && !inRune {
		return fee, nil
	}

	if gasPool.BalanceAsset.IsZero() || gasPool.BalanceRune.IsZero() {
		ctx.Logger().Error("fail to calculate fee as gas pool balance is zero, returning 0 fee", "pool", gasPool.Asset.String(), "rune", gasPool.BalanceRune.String(), "asset", gasPool.BalanceAsset.String())
		return cosmos.ZeroUint(), nil
	}

	// Convert gas asset fee to rune, if inRune = true, return
	fee = gasPool.AssetValueInRune(fee)
	if inRune {
		return fee, nil
	}

	// convert rune value into non-gas asset value
	assetPool, err := gm.keeper.GetPool(ctx, asset)
	if err != nil {
		return cosmos.ZeroUint(), err
	}
	if assetPool.BalanceAsset.IsZero() || assetPool.BalanceRune.IsZero() {
		ctx.Logger().Error("fail to calculate fee as asset pool balance is zero, returning 0 fee", "pool", assetPool.Asset.String(), "rune", assetPool.BalanceRune.String(), "asset", assetPool.BalanceAsset.String())
		return cosmos.ZeroUint(), nil
	}

	return assetPool.RuneValueInAsset(fee), nil
}

// CalcOutboundFeeMultiplier returns the current outbound fee multiplier based on current and target outbound fee surplus
func (gm *GasMgr) CalcOutboundFeeMultiplier(ctx cosmos.Context, targetSurplusRune, gasSpentRune, gasWithheldRune, maxMultiplier, minMultiplier cosmos.Uint) cosmos.Uint {
	// Sanity check
	if targetSurplusRune.Equal(cosmos.ZeroUint()) {
		ctx.Logger().Error("target gas surplus is zero")
		return maxMultiplier
	}
	if minMultiplier.GT(maxMultiplier) {
		ctx.Logger().Error("min multiplier greater than max multiplier", "minMultiplier", minMultiplier, "maxMultiplier", maxMultiplier)
		return cosmos.NewUint(30_000) // should never happen, return old default
	}

	// Find current surplus (gas withheld from user - gas spent by the reserve)
	surplusRune := common.SafeSub(gasWithheldRune, gasSpentRune)

	// How many BPs to reduce the multiplier
	multiplierReducedBps := common.GetSafeShare(surplusRune, targetSurplusRune, common.SafeSub(maxMultiplier, minMultiplier))
	return common.SafeSub(maxMultiplier, multiplierReducedBps)
}

// TODO: Replace combined GetMaxGas/GetGasRate calls with single GetGasDetails calls, so GetNetworkFee called only once.
// (If done completely, perhaps mark GetMaxGas/GetGasRate to be removed on hard fork.)
//
// GetGasDetails calculates a consistent MaxGas Coin and GasRate for the network's TransactionSize.
func (gm *GasMgr) GetGasDetails(ctx cosmos.Context, chain common.Chain) (common.Coin, int64, error) {
	networkFee, err := gm.GetNetworkFee(ctx, chain)
	if err != nil {
		ctx.Logger().Error("fail to get network fee", "error", err, "chain", chain)
		return common.NoCoin, 0, fmt.Errorf("fail to get network fee for chain(%s): %w", chain, err)
	}
	if err := networkFee.Valid(); err != nil {
		ctx.Logger().Error("network fee is invalid", "error", err, "chain", chain)
		return common.NoCoin, 0, fmt.Errorf("network fee for chain(%s) is invalid: %w", chain, err)
	}

	gasRate := cosmos.NewUint(networkFee.TransactionFeeRate)
	if !chain.IsTHORChain() {
		// THORChain has exactly-knowable gas costs, but otherwise overestimate the gas rate by 1.5x
		// to increase the likelihood of transaction acceptance.
		gasRate = gasRate.MulUint64(3).QuoUint64(2)
	}
	chainGasAssetPrecision := chain.GetGasAssetDecimal()

	// convert to 1e8 decimals for the max gas coin
	_, gasRateUnitsPerOne := chain.GetGasUnits()
	maxGasAmount := gasRate.MulUint64(networkFee.TransactionSize)
	maxGasAmount1e8 := maxGasAmount.MulUint64(common.One).Quo(gasRateUnitsPerOne)
	maxGasAmount1e8 = cosmos.RoundToDecimal(
		maxGasAmount1e8,
		chainGasAssetPrecision,
	)

	maxGasCoin := common.NewCoin(chain.GetGasAsset(), maxGasAmount1e8)
	maxGasCoin.Decimals = chainGasAssetPrecision

	return maxGasCoin, int64(gasRate.Uint64()), nil
}

// GetGasRate return the gas rate
func (gm *GasMgr) GetGasRate(ctx cosmos.Context, chain common.Chain) cosmos.Uint {
	_, gasRate, err := gm.GetGasDetails(ctx, chain)
	if err != nil {
		ctx.Logger().Error("fail to get gas rate", "chain", chain, "error", err)
		return cosmos.ZeroUint()
	}
	return cosmos.NewUint(uint64(gasRate))
}

func (gm *GasMgr) GetNetworkFee(ctx cosmos.Context, chain common.Chain) (types.NetworkFee, error) {
	switch chain {
	case common.THORChain:
		transactionFee := gm.keeper.GetOutboundTxFee(ctx)
		return types.NewNetworkFee(common.THORChain, 1, transactionFee.Uint64()), nil
	default:
		return gm.keeper.GetNetworkFee(ctx, chain)
	}
}

// GetMaxGas will calculate the maximum gas fee a tx can use
func (gm *GasMgr) GetMaxGas(ctx cosmos.Context, chain common.Chain) (common.Coin, error) {
	maxGasCoin, _, err := gm.GetGasDetails(ctx, chain)
	return maxGasCoin, err
}

// EndBlock emit the events
func (gm *GasMgr) EndBlock(ctx cosmos.Context, keeper keeper.Keeper, eventManager EventManager) {
	gm.ProcessGas(ctx, keeper)

	if len(gm.gasEvent.Pools) == 0 {
		return
	}
	if err := eventManager.EmitGasEvent(ctx, gm.gasEvent); nil != err {
		ctx.Logger().Error("fail to emit gas event", "error", err)
	}
	gm.reset() // do not remove, will cause consensus failures
}

// ProcessGas to subsidise the gas asset pools with RUNE for the gas they have spent
func (gm *GasMgr) ProcessGas(ctx cosmos.Context, keeper keeper.Keeper) {
	if keeper.RagnarokInProgress(ctx) {
		// ragnarok is in progress , stop
		return
	}

	reserveRune := keeper.GetRuneBalanceOfModule(ctx, ReserveName)
	poolCache := map[common.Asset]Pool{}
	for i := range gm.outAssetGas {
		feeSpentRune := cosmos.ZeroUint()
		for _, coin := range gm.outAssetGas[i].gas {
			// if the coin is empty, don't need to do anything
			if coin.IsEmpty() {
				continue
			}

			pool, ok := poolCache[coin.Asset]
			if !ok {
				var err error // Declare error variable to prevent 'pool' shadowing
				pool, err = keeper.GetPool(ctx, coin.Asset)
				if err != nil {
					ctx.Logger().Error("fail to get pool", "pool", coin.Asset, "error", err)
					continue
				}
				if err = pool.Valid(); err != nil {
					// Cache the invalid pool, logging only when added to the cache.
					ctx.Logger().Error("invalid pool", "pool", coin.Asset, "error", err)
				}
				poolCache[coin.Asset] = pool
			}
			if err := pool.Valid(); err != nil {
				continue
			}

			// TODO:  Use RuneReimbursementForAssetWithdrawal and, within this range, do cached pool updates before a single later SetPool?
			// Currently this uses a constant AssetValueInRune ratio without ensuring a constant depths-product,
			// as a result of which asset-associated reimbursement order does not matter.
			runeGas := pool.AssetValueInRune(coin.Amount) // Convert to Rune (gas will never be RUNE)
			if runeGas.IsZero() {
				continue
			}
			// Keep track of whether the Reserve RUNE will be enough for all reimbursements.
			reserveRune = common.SafeSub(reserveRune, runeGas)
			if reserveRune.IsZero() {
				// since we don't have enough in the reserve to cover the gas used,
				// no further rune is added to gas pools, sorry LPs!
				runeGas = cosmos.ZeroUint()
			}

			gasPool := GasPool{
				Asset:    coin.Asset,
				AssetAmt: coin.Amount,
				RuneAmt:  runeGas,
				Count:    gm.gasCount[coin.Asset],
			}
			gm.gasEvent.UpsertGasPool(gasPool)

			feeSpentRune = feeSpentRune.Add(runeGas)
		}
		// Add RUNE spent on gas by the reserve
		if err := keeper.AddToOutboundFeeSpentRune(ctx, gm.outAssetGas[i].outAsset, feeSpentRune); err != nil {
			ctx.Logger().Error("fail to add to outbound fee spent rune", "outbound asset", gm.outAssetGas[i].outAsset, "error", err)
		}
	}

	// Carry out the actual reimbursement and Set the pools.
	for i := range gm.gasEvent.Pools {
		pool, ok := poolCache[gm.gasEvent.Pools[i].Asset]
		if !ok {
			// This should never happen.
			ctx.Logger().Error("pool asset in gas event for which no cached pool", "asset", gm.gasEvent.Pools[i].Asset)
			continue
		}
		if !gm.gasEvent.Pools[i].RuneAmt.IsZero() {
			coin := common.NewCoin(common.RuneNative, gm.gasEvent.Pools[i].RuneAmt)
			if err := keeper.SendFromModuleToModule(ctx, ReserveName, AsgardName, common.NewCoins(coin)); err != nil {
				ctx.Logger().Error("fail to transfer funds from reserve to asgard", "pool", gm.gasEvent.Pools[i].Asset, "error", err)
			} else {
				pool.BalanceRune = pool.BalanceRune.Add(gm.gasEvent.Pools[i].RuneAmt)
			}
		}
		pool.BalanceAsset = common.SafeSub(pool.BalanceAsset, gm.gasEvent.Pools[i].AssetAmt)
		if err := keeper.SetPool(ctx, pool); err != nil {
			ctx.Logger().Error("fail to set pool", "pool", pool.Asset, "error", err)
		}
	}
}
