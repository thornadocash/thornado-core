//go:build chainnet
// +build chainnet

package aggregators

// No whitelisted aggregator contracts on chainnet yet.
func DexAggregators() []Aggregator {
	return []Aggregator{}
}
