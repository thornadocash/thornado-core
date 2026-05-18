package common

// ChainNetwork is to indicate which chain environment THORNode are working with
type ChainNetwork uint8

const (
	// TestNet network for test - DO NOT USE
	// TODO: remove on hard fork
	TestNet ChainNetwork = iota
	// MainNet network for mainnet
	MainNet
	// MockNet network for mocknet
	MockNet
	// Stagenet network for stagenet
	StageNet
	// ChainNet network for chainnet
	ChainNet
)

// SoftEquals groups networks into MainNet vs non-MainNet.
// Returns true if both are MainNet, or both are non-MainNet.
func (net ChainNetwork) SoftEquals(net2 ChainNetwork) bool {
	if net == MainNet && net2 == MainNet {
		return true
	}
	if net != MainNet && net2 != MainNet {
		return true
	}

	return false
}
