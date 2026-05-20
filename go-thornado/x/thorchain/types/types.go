package types

const (
	// SuperMajorityFactor - super majority 2/3
	SuperMajorityFactor = 3
	// SimpleMajorityFactor - simple majority 1/2
	SimpleMajorityFactor = 2
)

// AdvSwapQueueMode defines the operating mode for the advanced swap queue
type AdvSwapQueueMode int64

const (
	// AdvSwapQueueModeDisabled - Advanced swap queue is disabled
	AdvSwapQueueModeDisabled AdvSwapQueueMode = 0
	// AdvSwapQueueModeEnabled - Advanced swap queue is enabled for all swap types
	AdvSwapQueueModeEnabled AdvSwapQueueMode = 1
	// AdvSwapQueueModeMarketOnly - Advanced swap queue processes market swaps only, blocks limit swaps
	AdvSwapQueueModeMarketOnly AdvSwapQueueMode = 2
)

// HasSuperMajority return true when it has 2/3 majority
func HasSuperMajority(signers, total int) bool {
	if signers > total {
		return false // will not have majority if THORNode have more signers than node accounts. This shouldn't be possible
	}
	if signers <= 0 {
		return false // edge case
	}
	min := total * 2 / SuperMajorityFactor
	if (total*2)%SuperMajorityFactor > 0 {
		min += 1
	}

	return signers >= min
}

// HasSimpleMajority return true when it has more than 1/2
// this method replace HasSimpleMajority, which is not correct
func HasSimpleMajority(signers, total int) bool {
	if signers > total {
		return false // will not have majority if THORNode have more signers than node accounts. This shouldn't be possible
	}
	if signers <= 0 {
		return false // edge case
	}
	min := total / SimpleMajorityFactor
	if total%SimpleMajorityFactor > 0 {
		min += 1
	}
	return signers >= min
}

// HasMinority return true when it has more than 1/3
func HasMinority(signers, total int) bool {
	if signers > total {
		return false // if THORNode have more signers than node accounts. This shouldn't be possible
	}
	if signers <= 0 {
		return false // edge case
	}
	min := total / SuperMajorityFactor
	if total%SuperMajorityFactor > 0 {
		min += 1
	}

	return signers >= min
}
