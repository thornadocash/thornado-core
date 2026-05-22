package keeperv1

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) GetOutboundTxFee(ctx cosmos.Context) cosmos.Uint {
	return cosmos.ZeroUint()
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
	return cosmos.ZeroUint()
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
