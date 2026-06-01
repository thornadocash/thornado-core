package signer

import (
	"fmt"
	"sync"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/constants"
)

// ConstantsProvider which will query thornado to get the constants value per request
// it will also cache the constant values internally
type ConstantsProvider struct {
	requestHeight int64 // the block height last request to thornado to retrieve constant values
	bridge        thornadoclient.ThornadoBridge
	constantsLock *sync.Mutex
	constants     map[string]int64 // the constant values get from thornado and cached in memory
}

// NewConstantsProvider create a new instance of ConstantsProvider
func NewConstantsProvider(bridge thornadoclient.ThornadoBridge) *ConstantsProvider {
	return &ConstantsProvider{
		constants:     make(map[string]int64),
		requestHeight: 0,
		bridge:        bridge,
		constantsLock: &sync.Mutex{},
	}
}

// GetInt64Value get the constant value that match the given key
func (cp *ConstantsProvider) GetInt64Value(thornadoBlockHeight int64, key constants.ConfigName) (int64, error) {
	if err := cp.EnsureConstants(thornadoBlockHeight); err != nil {
		return 0, fmt.Errorf("fail to get constants from thornado: %w", err)
	}
	cp.constantsLock.Lock()
	defer cp.constantsLock.Unlock()
	return cp.constants[key.String()], nil
}

func (cp *ConstantsProvider) EnsureConstants(thornadoBlockHeight int64) error {
	if cp.requestHeight == 0 {
		return cp.getConstantsFromThornado(thornadoBlockHeight)
	}
	cp.constantsLock.Lock()
	churnInterval := constants.MinutesToBlocks(
		cp.constants[constants.Churn_IntervalMinutes.String()],
		cp.constants[constants.Chain_BlockTimeSeconds.String()],
	)
	cp.constantsLock.Unlock()
	// Thornado will have new version and constants only when new node get rotated in , and the new version get consensus
	if thornadoBlockHeight-cp.requestHeight < churnInterval {
		return nil
	}
	return cp.getConstantsFromThornado(thornadoBlockHeight)
}

func (cp *ConstantsProvider) getConstantsFromThornado(height int64) error {
	constants, err := cp.bridge.GetConstants()
	if err != nil {
		return fmt.Errorf("fail to get constants: %w", err)
	}
	cp.constantsLock.Lock()
	defer cp.constantsLock.Unlock()
	cp.constants = constants
	cp.requestHeight = height
	return nil
}
