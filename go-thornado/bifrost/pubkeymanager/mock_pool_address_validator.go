package pubkeymanager

import (
	"errors"

	"github.com/thornadocash/go-thornado/common"
)

var MockPubkey = "tthorpub1addwnpepqt8tnluxnk3y5quyq952klgqnlmz2vmaynm40fp592s0um7ucvjh5lc2l2z"

type MockPoolAddressValidator struct{}

func NewMockPoolAddressValidator() *MockPoolAddressValidator {
	return &MockPoolAddressValidator{}
}

func (mpa *MockPoolAddressValidator) GetPubKeys() common.PubKeys { return nil }
func (mpa *MockPoolAddressValidator) GetAlgoPubKeys(_ common.SigningAlgo, _ bool) common.PubKeys {
	return nil
}

func (mpa *MockPoolAddressValidator) GetSignPubKeys() common.PubKeys {
	pubKey, _ := common.NewPubKey(MockPubkey)
	return common.PubKeys{pubKey}
}

func (mpa *MockPoolAddressValidator) GetNodePubKey(_ common.SigningAlgo) common.PubKey {
	return common.EmptyPubKey
}

func (mpa *MockPoolAddressValidator) HasPubKey(pk common.PubKey) bool {
	return pk.String() == MockPubkey
}
func (mpa *MockPoolAddressValidator) AddPubKey(pk common.PubKey, _ bool, _ common.SigningAlgo) {}
func (mpa *MockPoolAddressValidator) AddNodePubKey(pk common.PubKey, _ common.SigningAlgo)     {}
func (mpa *MockPoolAddressValidator) RemovePubKey(pk common.PubKey)                            {}
func (mpa *MockPoolAddressValidator) Start() error                                             { return errors.New("kaboom") }
func (mpa *MockPoolAddressValidator) Stop() error                                              { return errors.New("kaboom") }

func (mpa *MockPoolAddressValidator) IsValidPoolAddress(addr string, chain common.Chain) (bool, common.ChainPoolInfo) {
	return false, common.NoChainPoolInfo
}

func (mpa *MockPoolAddressValidator) RegisterCallback(callback OnNewPubKey) {
}

func (mpa *MockPoolAddressValidator) RegisterPathCallback(callback OnNewPubKeyPath) {
}
