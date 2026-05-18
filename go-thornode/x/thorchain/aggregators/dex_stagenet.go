//go:build stagenet
// +build stagenet

package aggregators

// If the contract whitelist is not (as in stagenet),
// use a default max gas and fall through to the suffix
// that is passed in. This should help dex agg contract devs test
// their work without having to run a mocknet or stagenet.
func DexAggregators() []Aggregator {
	return []Aggregator{}
}
