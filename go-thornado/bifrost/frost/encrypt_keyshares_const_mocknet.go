//go:build mocknet
// +build mocknet

package frost

// MinimumMnemonicEntropy is set low to allow for the fake mnemonics used in mocknet.
const MinimumMnemonicEntropy = 2
