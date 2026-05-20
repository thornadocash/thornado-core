package ecdsa

import (
	"errors"
	"sort"
	"strings"
	"time"

	bcrypto "github.com/binance-chain/tss-lib/crypto"
	bkg "github.com/binance-chain/tss-lib/ecdsa/keygen"
	btss "github.com/binance-chain/tss-lib/tss"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/addr"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keygen"
	tcommon "github.com/thornadocash/go-thornado/common"
)

// errorSaveStateMgr is a mock state manager that returns an error on SaveLocalState
type errorSaveStateMgr struct{}

func (s *errorSaveStateMgr) SaveLocalState(_ storage.KeygenLocalState) error {
	return errors.New("save error")
}

func (s *errorSaveStateMgr) GetLocalState(_ string) (storage.KeygenLocalState, error) {
	return storage.KeygenLocalState{}, nil
}

func (s *errorSaveStateMgr) SaveAddressBook(_ map[peer.ID]addr.AddrList) error {
	return nil
}

func (s *errorSaveStateMgr) RetrieveP2PAddresses() (addr.AddrList, error) {
	return nil, nil
}

type TssECDSAKeygenExtraSuite struct{}

var _ = Suite(&TssECDSAKeygenExtraSuite{})

func (s *TssECDSAKeygenExtraSuite) SetUpSuite(c *C) {
	conversion.SetupBech32Prefix()
}

func (s *TssECDSAKeygenExtraSuite) TestNewTssKeyGen(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, nil, "msg1", stateManager, nil, nil)
	c.Assert(instance, NotNil)
	c.Assert(instance.GetTssCommonStruct(), NotNil)
	c.Assert(instance.GetTssKeyGenChannels(), NotNil)
}

func (s *TssECDSAKeygenExtraSuite) TestNewRequest(c *C) {
	keys := []string{"key1", "key2"}
	req := keygen.NewRequest(keys, 100, "v1.0", tcommon.SigningAlgoSecp256k1)
	c.Assert(req.Keys, DeepEquals, keys)
	c.Assert(req.BlockHeight, Equals, int64(100))
	c.Assert(req.Version, Equals, "v1.0")
	c.Assert(req.Algo, Equals, tcommon.SigningAlgoSecp256k1)
}

func (s *TssECDSAKeygenExtraSuite) TestNewResponse(c *C) {
	resp := keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "pubkey", "addr", common.Success, blame.Blame{})
	c.Assert(resp.PubKey, Equals, "pubkey")
	c.Assert(resp.PoolAddress, Equals, "addr")
	c.Assert(resp.Status, Equals, common.Success)
}

func (s *TssECDSAKeygenExtraSuite) TestGenerateNewKeyNilPreParams(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  5 * time.Second,
		KeySignTimeout: 5 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	sort.Strings(testPubKeys)

	// Create instance with a valid localNodePubKey (in the keys list) but nil preParams
	instance := NewTssKeyGen("peer1", conf, testPubKeys[0], nil, nil, nil, "msg1", stateManager, nil, nil)

	req := keygen.NewRequest(testPubKeys, 10, "", tcommon.SigningAlgoSecp256k1)
	result, err := instance.GenerateNewKey(req)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "error, empty pre-parameters")
}

func (s *TssECDSAKeygenExtraSuite) TestProcessKeyGenErrChan(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan bkg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Close errChan immediately to trigger the error path
	close(errChan)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "error channel closed fail to start local party")
}

func (s *TssECDSAKeygenExtraSuite) TestProcessKeyGenStopChan(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	stopChan := make(chan struct{})
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, stopChan, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan bkg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Close stopChan to trigger the stop path
	close(stopChan)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "received exit signal")
}

func (s *TssECDSAKeygenExtraSuite) TestProcessKeyGenTimeout(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  100 * time.Millisecond, // Very short timeout
		KeySignTimeout: 100 * time.Millisecond,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan bkg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// No messages on any channel - will timeout
	// blameMgr.GetLastMsg() will be nil, so we get the "timeout before shared message" error
	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "timeout before shared message is generated")
}

func (s *TssECDSAKeygenExtraSuite) TestProcessKeyGenEndChSaveError(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	// Use the error-returning state manager so SaveLocalState fails
	stateManager := &errorSaveStateMgr{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, nil, "msg1", stateManager, nil, nil)

	// Set up party info so NotifyTaskDone can work
	partyIDMap := make(map[string]*btss.PartyID)
	partyIDMap["1"] = nil
	fakePartyInfo := &common.PartyInfo{
		PartyMap:   nil,
		PartyIDMap: partyIDMap,
	}
	instance.tssCommonStruct.SetPartyInfo(fakePartyInfo)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan bkg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Create a valid ECPoint on the secp256k1 curve (generator point)
	curve := btss.S256()
	ecPoint, err := bcrypto.NewECPoint(curve, curve.Params().Gx, curve.Params().Gy)
	c.Assert(err, IsNil)

	// Send LocalPartySaveData with valid ECDSAPub
	endCh <- bkg.LocalPartySaveData{ECDSAPub: ecPoint}

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(strings.Contains(err.Error(), "fail to save keygen result to storage"), Equals, true)
}

func (s *TssECDSAKeygenExtraSuite) TestGenerateNewKeyInvalidParties(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  5 * time.Second,
		KeySignTimeout: 5 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}

	// Use invalid keys that can't be parsed
	instance := NewTssKeyGen("peer1", conf, "invalid_key", nil, nil, nil, "msg1", stateManager, nil, nil)
	req := keygen.NewRequest([]string{"invalid_key"}, 10, "", tcommon.SigningAlgoSecp256k1)
	result, err := instance.GenerateNewKey(req)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(strings.Contains(err.Error(), "fail to get keygen parties"), Equals, true)
}

func (s *TssECDSAKeygenExtraSuite) TestGenerateNewKeyThresholdError(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  5 * time.Second,
		KeySignTimeout: 5 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}

	// Use only one key - GetThreshold requires at least 2 parties
	sort.Strings(testPubKeys)
	singleKey := testPubKeys[0:1]
	instance := NewTssKeyGen("peer1", conf, singleKey[0], nil, nil, nil, "msg1", stateManager, nil, nil)
	req := keygen.NewRequest(singleKey, 10, "", tcommon.SigningAlgoSecp256k1)
	result, err := instance.GenerateNewKey(req)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
}

func (s *TssECDSAKeygenExtraSuite) TestNewResponseFields(c *C) {
	b := blame.Blame{FailReason: "test"}
	resp := keygen.NewResponse(tcommon.SigningAlgoEd25519, "pk", "pool_addr", common.Fail, b)
	c.Assert(resp.Algo, Equals, tcommon.SigningAlgoEd25519)
	c.Assert(resp.PubKey, Equals, "pk")
	c.Assert(resp.PoolAddress, Equals, "pool_addr")
	c.Assert(resp.Status, Equals, common.Fail)
	c.Assert(resp.Blame.FailReason, Equals, "test")
}
