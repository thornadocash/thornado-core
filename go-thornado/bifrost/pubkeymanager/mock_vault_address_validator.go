package pubkeymanager

import (
	"errors"

	"github.com/thornadocash/go-thornado/common"
)

var MockPubkey = "tthorpub1addwnpepqt8tnluxnk3y5quyq952klgqnlmz2vmaynm40fp592s0um7ucvjh5lc2l2z"

type MockVaultAddressValidator struct{}

func NewMockVaultAddressValidator() *MockVaultAddressValidator {
	return &MockVaultAddressValidator{}
}

func (mpa *MockVaultAddressValidator) GetPubKeys() common.PubKeys { return nil }
func (mpa *MockVaultAddressValidator) GetAlgoPubKeys(_ common.SigningAlgo, _ bool) common.PubKeys {
	return nil
}

func (mpa *MockVaultAddressValidator) GetSignPubKeys() common.PubKeys {
	pubKey, _ := common.NewPubKey(MockPubkey)
	return common.PubKeys{pubKey}
}

func (mpa *MockVaultAddressValidator) GetNodePubKey(_ common.SigningAlgo) common.PubKey {
	return common.EmptyPubKey
}

func (mpa *MockVaultAddressValidator) HasPubKey(pk common.PubKey) bool {
	return pk.String() == MockPubkey
}
func (mpa *MockVaultAddressValidator) AddPubKey(pk common.PubKey, _ bool, _ common.SigningAlgo) {}
func (mpa *MockVaultAddressValidator) AddNodePubKey(pk common.PubKey, _ common.SigningAlgo)     {}
func (mpa *MockVaultAddressValidator) RemovePubKey(pk common.PubKey)                            {}
func (mpa *MockVaultAddressValidator) Start() error                                             { return errors.New("kaboom") }
func (mpa *MockVaultAddressValidator) Stop() error                                              { return errors.New("kaboom") }

func (mpa *MockVaultAddressValidator) IsValidVaultAddress(addr string, chain common.Chain) (bool, common.ChainVaultInfo) {
	return false, common.NoChainVaultInfo
}

func (mpa *MockVaultAddressValidator) RegisterCallback(callback OnNewPubKey) {
}

func (mpa *MockVaultAddressValidator) RegisterPathCallback(callback OnNewPubKeyPath) {
}
