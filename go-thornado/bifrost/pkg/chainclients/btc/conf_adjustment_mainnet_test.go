//go:build !mocknet
// +build !mocknet

package btc

// mockBridge returns 0 for BTC_MaxConfirmations, so no cap is applied.
const expectedMaxConfAdjustment = uint64(10)
