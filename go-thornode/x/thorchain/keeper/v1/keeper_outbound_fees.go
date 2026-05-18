package keeperv1

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

func (k KVStore) GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint {
	if k.usdFeesEnabled(ctx) {
		return k.DollarConfigInRune(ctx, constants.NativeOutboundFeeUSD)
	}
	fee := k.GetConfigInt64(ctx, constants.OutboundTransactionFee)
	return cosmos.SafeUintFromInt64(fee)
}

// GetOutboundFeeWithheldRune - record of RUNE collected by the Reserve for an Asset's outbound fees
func (k KVStore) GetOutboundFeeWithheldRune(ctx cosmos.Context, outAsset common.Asset) (cosmos.Uint, error) {
	var record uint64
	_, err := k.getUint64(ctx, k.GetKey(prefixOutboundFeeWithheldRune, outAsset.String()), &record)
	if err != nil {
		return cosmos.ZeroUint(), err
	}
	withheld := cosmos.NewUint(record)

	// If the withheld amount is zero, initialize it to the initial withheld amount
	if withheld.IsZero() {
		withheld = k.GetSurplusForTargetMultiplier(ctx, cosmos.NewUint(10_000))
		// use set instead of AddToOutboundFeeWithheldRune to avoid infinite loop
		val, err := safeUint64(withheld)
		if err != nil {
			return cosmos.ZeroUint(), fmt.Errorf("outbound fee withheld rune overflow for %s: %w", outAsset, err)
		}
		k.setUint64(ctx, k.GetKey(prefixOutboundFeeWithheldRune, outAsset.String()), val)
		return withheld, nil
	}

	return withheld, nil
}

// AddToOutboundFeeWithheldRune - add to record of RUNE collected by the Reserve for an Asset's outbound fees
func (k KVStore) AddToOutboundFeeWithheldRune(ctx cosmos.Context, outAsset common.Asset, withheld cosmos.Uint) error {
	outboundFeeWithheldRune, err := k.GetOutboundFeeWithheldRune(ctx, outAsset)
	if err != nil {
		return err
	}

	outboundFeeWithheldRune = outboundFeeWithheldRune.Add(withheld)
	val, err := safeUint64(outboundFeeWithheldRune)
	if err != nil {
		return fmt.Errorf("outbound fee withheld rune overflow for %s: %w", outAsset, err)
	}
	k.setUint64(ctx, k.GetKey(prefixOutboundFeeWithheldRune, outAsset.String()), val)
	return nil
}

func (k KVStore) GetSurplusForTargetMultiplier(ctx cosmos.Context, targetMultiplierBps cosmos.Uint) cosmos.Uint {
	// Calculate the surplus level at which the multiplier reaches targetMultiplierBps.
	// The multiplier formula is:
	//
	//	multiplier = max - ((surplus / target) * (max - min))
	//
	// To find the equilibrium surplus where multiplier = 100% (i.e., 10_000 bps):
	//
	//	10_000 = max - ((surplus / target) * (max - min))
	//
	// Rearranged:
	//
	//	(max - 10_000) = (surplus / target) * (max - min)
	//	surplus = ((max - 10_000) / (max - min)) * target

	targetSurplus := cosmos.SafeUintFromInt64(k.GetConfigInt64(ctx, constants.TargetOutboundFeeSurplusRune))
	maxMultiplier := cosmos.SafeUintFromInt64(k.GetConfigInt64(ctx, constants.MaxOutboundFeeMultiplierBasisPoints))
	minMultiplier := cosmos.SafeUintFromInt64(k.GetConfigInt64(ctx, constants.MinOutboundFeeMultiplierBasisPoints))

	if targetMultiplierBps.LT(minMultiplier) {
		targetMultiplierBps = minMultiplier
	}

	if targetMultiplierBps.GT(maxMultiplier) {
		targetMultiplierBps = maxMultiplier
	}

	deltaToTarget := common.SafeSub(maxMultiplier, targetMultiplierBps)
	maxMinusMin := common.SafeSub(maxMultiplier, minMultiplier)

	// If max equals min, the multiplier is fixed and doesn't vary with surplus.
	// In this case, return target surplus as a reasonable default.
	if maxMinusMin.IsZero() {
		return targetSurplus
	}

	// Convert to cosmos.Dec to avoid integer division.
	deltaToTargetDec, err := cosmos.NewDecFromStr(deltaToTarget.String())
	if err != nil {
		return cosmos.ZeroUint()
	}

	maxMinusMinDec, err := cosmos.NewDecFromStr(maxMinusMin.String())
	if err != nil {
		return cosmos.ZeroUint()
	}

	targetSurplusDec, err := cosmos.NewDecFromStr(targetSurplus.String())
	if err != nil {
		return cosmos.ZeroUint()
	}

	// Perform decimal division: (deltaToTarget / maxMinusMin) * targetSurplus
	ratio := deltaToTargetDec.Quo(maxMinusMinDec)
	equilibriumSurplusDec := ratio.Mul(targetSurplusDec)
	equilibriumSurplus := cosmos.NewUintFromBigInt(equilibriumSurplusDec.RoundInt().BigInt())

	return equilibriumSurplus
}

// GetOutboundFeeWithheldRuneIterator to iterate through all Assets' OutboundFeeWithheldRune
// (e.g. for hard-fork GenesisState export)
func (k KVStore) GetOutboundFeeWithheldRuneIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixOutboundFeeWithheldRune)
}

// GetOutboundFeeSpentRune - record of RUNE spent by the Reserve for an Asset's outbounds' gas costs
func (k KVStore) GetOutboundFeeSpentRune(ctx cosmos.Context, outAsset common.Asset) (cosmos.Uint, error) {
	var record uint64
	_, err := k.getUint64(ctx, k.GetKey(prefixOutboundFeeSpentRune, outAsset.String()), &record)
	return cosmos.NewUint(record), err
}

// AddToOutboundFeeSpentRune - add to record of RUNE spent by the Reserve for an Asset's outbounds' gas costs
func (k KVStore) AddToOutboundFeeSpentRune(ctx cosmos.Context, outAsset common.Asset, spent cosmos.Uint) error {
	outboundFeeSpentRune, err := k.GetOutboundFeeSpentRune(ctx, outAsset)
	if err != nil {
		return err
	}

	outboundFeeSpentRune = outboundFeeSpentRune.Add(spent)
	val, err := safeUint64(outboundFeeSpentRune)
	if err != nil {
		return fmt.Errorf("outbound fee spent rune overflow for %s: %w", outAsset, err)
	}
	k.setUint64(ctx, k.GetKey(prefixOutboundFeeSpentRune, outAsset.String()), val)
	return nil
}

// GetOutboundFeeSpentRuneIterator to iterate through all Assets' OutboundFeeSpentRune
// (e.g. for hard-fork GenesisState export)
func (k KVStore) GetOutboundFeeSpentRuneIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixOutboundFeeSpentRune)
}

// safeUint64 converts a cosmos.Uint to uint64, returning an error if the value
// exceeds MaxUint64 so callers can log and surface the anomaly for investigation.
func safeUint64(v cosmos.Uint) (uint64, error) {
	if !v.BigInt().IsUint64() {
		return 0, fmt.Errorf("value %s overflows uint64", v.String())
	}
	return v.Uint64(), nil
}
