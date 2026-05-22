package types

const (
	SuperMajorityFactor  = 3
	SimpleMajorityFactor = 2
)

func HasSuperMajority(signers, total int) bool {
	if signers > total || signers <= 0 {
		return false
	}
	min := total * 2 / SuperMajorityFactor
	if (total*2)%SuperMajorityFactor > 0 {
		min++
	}
	return signers >= min
}

func HasSimpleMajority(signers, total int) bool {
	if signers > total || signers <= 0 {
		return false
	}
	min := total / SimpleMajorityFactor
	if total%SimpleMajorityFactor > 0 {
		min++
	}
	return signers >= min
}

func HasMinority(signers, total int) bool {
	if signers > total || signers <= 0 {
		return false
	}
	min := total / SuperMajorityFactor
	if total%SuperMajorityFactor > 0 {
		min++
	}
	return signers >= min
}
