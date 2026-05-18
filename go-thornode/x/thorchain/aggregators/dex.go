package aggregators

import (
	"fmt"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
)

type Aggregator struct {
	Chain         common.Chain
	Address       string
	GasUnitsLimit uint64
}

// FetchDexAggregator - fetches a dex aggregator address that matches the given chain and suffix
func FetchDexAggregator(chain common.Chain, suffix string) (string, error) {
	contracts := DexAggregators()
	// If no whitelist contracts are set, fall through to the suffix
	if len(contracts) == 0 {
		return suffix, nil
	}
	for _, agg := range contracts {
		if !chain.Equals(agg.Chain) {
			continue
		}
		if strings.HasSuffix(agg.Address, suffix) {
			return agg.Address, nil
		}
	}

	return "", fmt.Errorf("%s aggregator not found", suffix)
}

// FetchDexAggregatorGasLimit - fetches a dex aggregator gas limit that matches the given chain and suffix
func FetchDexAggregatorGasLimit(chain common.Chain, suffix string) (uint64, error) {
	contracts := DexAggregators()
	// If no whitelist contracts are set, fall through to the default of 400_000
	if len(contracts) == 0 {
		return 400_000, nil
	}
	for _, agg := range contracts {
		if !chain.Equals(agg.Chain) {
			continue
		}
		if strings.HasSuffix(agg.Address, suffix) {
			return agg.GasUnitsLimit, nil
		}
	}

	return 0, fmt.Errorf("%s aggregator not found", suffix)
}
