//go:build mocknet
// +build mocknet

package btc

// the mocknet stub always returns 1 regardless of the bridge.
const expectedMaxConfAdjustment = uint64(1)
