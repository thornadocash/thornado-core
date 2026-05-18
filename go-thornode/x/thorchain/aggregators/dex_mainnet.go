//go:build !stagenet && !chainnet && !mocknet
// +build !stagenet,!chainnet,!mocknet

package aggregators

func DexAggregators() []Aggregator {
	return DexAggregatorsList()
}
