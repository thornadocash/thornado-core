//go:build !mocknet
// +build !mocknet

package tron

var (
	refBlocksMax           = 10 // 10*100 = 1000 blocks, ~50 minutes with 3s blocks
	refBlockInterval int64 = 100
)
