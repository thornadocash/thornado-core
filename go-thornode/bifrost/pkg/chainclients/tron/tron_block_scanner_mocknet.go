//go:build mocknet
// +build mocknet

package tron

// Keep shorter interval in mocknet to avoid long init before signing.

var (
	refBlocksMax           = 10
	refBlockInterval int64 = 10
)
