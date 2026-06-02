package thornado

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// GasMgr implement GasManager interface which will store the gas related events happened in thornado to memory
// emit GasEvent per block if there are any
type GasMgr struct {
	gasEvent          *EventGas
	outAssetGas       []OutAssetGas
	gasCount          map[common.Asset]int64
	constantsAccessor constants.ConfigValues
	keeper            keeper.Keeper
}

// newGasMgr create a new instance of GasMgr
func newGasMgr(constantsAccessor constants.ConfigValues, k keeper.Keeper) *GasMgr {
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

// GetAssetOutboundFee returns current outbound fee for the asset in asset units.
func (gm *GasMgr) GetAssetOutboundFee(ctx cosmos.Context, asset common.Asset) (cosmos.Uint, error) {
	if asset.IsNative() {
		return cosmos.ZeroUint(), nil
	}
	chainOutboundFee, err := gm.keeper.GetNetworkFee(ctx, asset.GetChain())
	if err != nil {
		return cosmos.ZeroUint(), err
	}
	if err = chainOutboundFee.Valid(); err != nil {
		return cosmos.ZeroUint(), nil
	}
	_, gasRateUnitsPerOne := asset.GetChain().GetGasUnits()
	fee := cosmos.NewUint(chainOutboundFee.TransactionSize).MulUint64(chainOutboundFee.TransactionFeeRate).MulUint64(common.One).Quo(gasRateUnitsPerOne)
	return fee, nil
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
	if !chain.Equals(common.BTCChain) {
		// BTCChain has exactly-knowable gas costs, but otherwise overestimate the gas rate by 1.5x
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
	return gm.keeper.GetNetworkFee(ctx, chain)
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

// ProcessGas records per-block gas accounting.
func (gm *GasMgr) ProcessGas(ctx cosmos.Context, keeper keeper.Keeper) {
}
