package storage

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	maddr "github.com/multiformats/go-multiaddr"
)

// MockLocalStateManager is a mock use for test purpose
type MockLocalStateManager struct{}

func (s *MockLocalStateManager) SaveLocalState(state KeygenLocalState) error {
	return nil
}

func (s *MockLocalStateManager) GetLocalState(pubKey string) (KeygenLocalState, error) {
	return KeygenLocalState{}, fmt.Errorf("missing local state: %s", pubKey)
}

func (s *MockLocalStateManager) SaveAddressBook(address map[peer.ID][]maddr.Multiaddr) error {
	return nil
}

func (s *MockLocalStateManager) RetrieveP2PAddresses() ([]maddr.Multiaddr, error) {
	return nil, nil
}
