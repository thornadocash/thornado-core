package eddsa

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	bcrypto "github.com/binance-chain/tss-lib/crypto"
	eddsakg "github.com/binance-chain/tss-lib/eddsa/keygen"
	"github.com/binance-chain/tss-lib/tss"
	btss "github.com/binance-chain/tss-lib/tss"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/addr"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/storage"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/common"
	"gitlab.com/thorchain/thornode/v3/bifrost/tss/go-tss/keygen"
	tcommon "gitlab.com/thorchain/thornode/v3/common"
)

// mockTssMessage implements btss.Message interface for testing
type mockTssMessage struct {
	from        *btss.PartyID
	to          []*btss.PartyID
	isBroadcast bool
	msgType     string
}

func (m *mockTssMessage) Type() string                                    { return m.msgType }
func (m *mockTssMessage) GetFrom() *btss.PartyID                          { return m.from }
func (m *mockTssMessage) GetTo() []*btss.PartyID                          { return m.to }
func (m *mockTssMessage) IsBroadcast() bool                               { return m.isBroadcast }
func (m *mockTssMessage) WireBytes() ([]byte, *tss.MessageRouting, error) { return nil, nil, nil }
func (m *mockTssMessage) WireMsg() *tss.MessageWrapper                    { return nil }
func (m *mockTssMessage) String() string                                  { return "mock" }
func (m *mockTssMessage) ValidateBasic() bool                             { return true }
func (m *mockTssMessage) IsToOldAndNewCommittees() bool                   { return false }
func (m *mockTssMessage) IsToOldCommittee() bool                          { return false }

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

type TssEDDSAKeygenExtraSuite struct{}

var _ = Suite(&TssEDDSAKeygenExtraSuite{})

func (s *TssEDDSAKeygenExtraSuite) SetUpSuite(c *C) {
	conversion.SetupBech32Prefix()
}

func (s *TssEDDSAKeygenExtraSuite) TestNewTssKeyGen(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)
	c.Assert(instance, NotNil)
	c.Assert(instance.GetTssCommonStruct(), NotNil)
	c.Assert(instance.GetTssKeyGenChannels(), NotNil)
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenErrChan(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	close(errChan)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "error channel closed fail to start local party")
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenStopChan(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	stopChan := make(chan struct{})
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, stopChan, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	close(stopChan)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "received exit signal")
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenTimeoutNilLastMsg(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  100 * time.Millisecond,
		KeySignTimeout: 100 * time.Millisecond,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// No messages - will timeout with nil lastMsg
	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "timeout before shared message is generated")
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenEndChWithSaveError(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &errorSaveStateMgr{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

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
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Create valid ECPoint on Edwards curve
	curve := btss.Edwards()
	ecPoint, err := bcrypto.NewECPoint(curve, curve.Params().Gx, curve.Params().Gy)
	c.Assert(err, IsNil)

	endCh <- eddsakg.LocalPartySaveData{EDDSAPub: ecPoint}

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(strings.Contains(err.Error(), "fail to save keygen result to storage"), Equals, true)
}

func (s *TssEDDSAKeygenExtraSuite) TestGenerateNewKeyInvalidParties(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  5 * time.Second,
		KeySignTimeout: 5 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}

	instance := NewTssKeyGen("peer1", conf, "invalid_key", nil, nil, "msg1", stateManager, nil, nil)
	req := keygen.NewRequest([]string{"invalid_key"}, 10, "", tcommon.SigningAlgoEd25519)
	result, err := instance.GenerateNewKey(req)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(strings.Contains(err.Error(), "fail to get keygen parties"), Equals, true)
}

func (s *TssEDDSAKeygenExtraSuite) TestGenerateNewKeyThresholdError(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  5 * time.Second,
		KeySignTimeout: 5 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}

	sort.Strings(testPubKeys)
	singleKey := testPubKeys[0:1]
	instance := NewTssKeyGen("peer1", conf, singleKey[0], nil, nil, "msg1", stateManager, nil, nil)
	req := keygen.NewRequest(singleKey, 10, "", tcommon.SigningAlgoEd25519)
	result, err := instance.GenerateNewKey(req)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenTimeoutWithBlame(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  100 * time.Millisecond,
		KeySignTimeout: 100 * time.Millisecond,
	}
	stateManager := &storage.MockLocalStateManager{}
	sort.Strings(testPubKeys)
	instance := NewTssKeyGen("peer1", conf, testPubKeys[0], nil, nil, "msg1", stateManager, nil, nil)

	// Set up real parties so blame manager doesn't panic
	partiesID, localPartyID, err := conversion.GetParties(testPubKeys[:], testPubKeys[0])
	c.Assert(err, IsNil)
	partyIDMap := conversion.SetupPartyIDMap(partiesID)
	err = conversion.SetupIDMaps(partyIDMap, instance.tssCommonStruct.PartyIDtoP2PID)
	c.Assert(err, IsNil)

	// Create a real EDDSA keygen party for the partyMap
	threshold, err := conversion.GetThreshold(len(partiesID))
	c.Assert(err, IsNil)
	ctx := btss.NewPeerContext(partiesID)
	params := btss.NewParameters(tss.Edwards(), ctx, localPartyID, len(partiesID), threshold)
	outChParty := make(chan btss.Message, len(partiesID)*10)
	endChParty := make(chan eddsakg.LocalPartySaveData, len(partiesID)*10)
	keyGenParty := eddsakg.NewLocalParty(params, outChParty, endChParty)

	partyMap := new(sync.Map)
	partyMap.Store("", keyGenParty)
	blameMgr := instance.tssCommonStruct.GetBlameMgr()
	blameMgr.SetPartyInfo(partyMap, partyIDMap)
	err = conversion.SetupIDMaps(partyIDMap, blameMgr.PartyIDtoP2PID)
	c.Assert(err, IsNil)

	// Set up P2PPeers so GetThreshold works
	instance.tssCommonStruct.P2PPeers = conversion.GetPeersID(instance.tssCommonStruct.PartyIDtoP2PID, instance.tssCommonStruct.GetLocalPeerID())

	// Set lastMsg to non-nil so we enter the blame logic path
	mockMsg := &mockTssMessage{
		isBroadcast: true,
		msgType:     "test-round",
	}
	blameMgr.SetLastMsg(mockMsg)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Will timeout and go through blame logic with non-nil lastMsg
	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	// Should return ErrTssTimeOut after going through blame logic
	c.Assert(err.Error(), Equals, "error Tss Timeout")
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenEndChInvalidEddsaPub(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

	// Set up party info
	partyIDMap := make(map[string]*btss.PartyID)
	partyIDMap["1"] = nil
	fakePartyInfo := &common.PartyInfo{
		PartyMap:   nil,
		PartyIDMap: partyIDMap,
	}
	instance.tssCommonStruct.SetPartyInfo(fakePartyInfo)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	// Create ECPoint on secp256k1 curve (NOT Edwards) - should fail GetTssPubKeyEDDSA
	curve := btss.S256()
	ecPoint, err := bcrypto.NewECPoint(curve, curve.Params().Gx, curve.Params().Gy)
	c.Assert(err, IsNil)

	endCh <- eddsakg.LocalPartySaveData{EDDSAPub: ecPoint}

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(strings.Contains(err.Error(), "fail to get thorchain pubkey"), Equals, true)
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenEndChClosed(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	close(endCh)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "endCh closed unexpectedly")
}

func (s *TssEDDSAKeygenExtraSuite) TestProcessKeyGenOutChClosed(c *C) {
	conf := common.TssConfig{
		KeyGenTimeout:  60 * time.Second,
		KeySignTimeout: 60 * time.Second,
	}
	stateManager := &storage.MockLocalStateManager{}
	instance := NewTssKeyGen("peer1", conf, "localPub", nil, nil, "msg1", stateManager, nil, nil)

	errChan := make(chan struct{})
	outCh := make(chan btss.Message, 1)
	endCh := make(chan eddsakg.LocalPartySaveData, 1)
	localState := storage.KeygenLocalState{}

	close(outCh)

	result, err := instance.processKeyGen(errChan, outCh, endCh, localState)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Equals, "outCh closed unexpectedly")
}
