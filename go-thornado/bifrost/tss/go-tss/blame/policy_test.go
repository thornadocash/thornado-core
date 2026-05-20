package blame

import (
	"sort"
	"sync"
	"testing"

	bkg "github.com/binance-chain/tss-lib/ecdsa/keygen"
	btss "github.com/binance-chain/tss-lib/tss"
	"github.com/libp2p/go-libp2p-core/peer"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

var (
	testPubKeys = [...]string{"thorpub1addwnpepqtdklw8tf3anjz7nn5fly3uvq2e67w2apn560s4smmrt9e3x52nt2svmmu3", "thorpub1addwnpepqtspqyy6gk22u37ztra4hq3hdakc0w0k60sfy849mlml2vrpfr0wvm6uz09", "thorpub1addwnpepq2ryyje5zr09lq7gqptjwnxqsy2vcdngvwd6z7yt5yjcnyj8c8cn559xe69", "thorpub1addwnpepqfjcw5l4ay5t00c32mmlky7qrppepxzdlkcwfs2fd5u73qrwna0vzag3y4j"}

	testPeers = []string{
		"16Uiu2HAm4TmEzUqy3q3Dv7HvdoSboHk5sFj2FH3npiN5vDbJC6gh",
		"16Uiu2HAm2FzqoUdS6Y9Esg2EaGcAG5rVe1r6BFNnmmQr2H3bqafa",
		"16Uiu2HAmACG5DtqmQsHtXg4G2sLS65ttv84e7MrL4kapkjfmhxAp",
		"16Uiu2HAmAWKWf5vnpiAhfdSQebTbbB3Bg35qtyG7Hr4ce23VFA8V",
	}
)

func TestPackage(t *testing.T) { TestingT(t) }

type policyTestSuite struct {
	blameMgr *Manager
}

var _ = Suite(&policyTestSuite{})

func (p *policyTestSuite) SetUpTest(c *C) {
	p.blameMgr = NewBlameManager()
	conversion.SetupBech32Prefix()
	p1, err := peer.Decode(testPeers[0])
	c.Assert(err, IsNil)
	p2, err := peer.Decode(testPeers[1])
	c.Assert(err, IsNil)
	p3, err := peer.Decode(testPeers[2])
	c.Assert(err, IsNil)
	p.blameMgr.SetLastUnicastPeer(p1, "testType")
	p.blameMgr.SetLastUnicastPeer(p2, "testType")
	p.blameMgr.SetLastUnicastPeer(p3, "testType")
	localTestPubKeys := testPubKeys[:]
	sort.Strings(localTestPubKeys)
	partiesID, localPartyID, err := conversion.GetParties(localTestPubKeys, testPubKeys[0])
	c.Assert(err, IsNil)
	partyIDMap := conversion.SetupPartyIDMap(partiesID)
	err = conversion.SetupIDMaps(partyIDMap, p.blameMgr.PartyIDtoP2PID)
	c.Assert(err, IsNil)
	outCh := make(chan btss.Message, len(partiesID))
	endCh := make(chan bkg.LocalPartySaveData, len(partiesID))
	ctx := btss.NewPeerContext(partiesID)
	params := btss.NewParameters(btss.S256(), ctx, localPartyID, len(partiesID), 3)
	keyGenParty := bkg.NewLocalParty(params, outCh, endCh)

	testPartyMap := new(sync.Map)
	testPartyMap.Store("", keyGenParty)
	p.blameMgr.SetPartyInfo(testPartyMap, partyIDMap)
}

func (p *policyTestSuite) TestGetUnicastBlame(c *C) {
	_, err := p.blameMgr.GetUnicastBlame("testTypeWrong")
	c.Assert(err, NotNil)
	_, err = p.blameMgr.GetUnicastBlame("testType")
	c.Assert(err, IsNil)
}

func (p *policyTestSuite) TestGetBroadcastBlame(c *C) {
	pi := p.blameMgr.partyInfo

	r1 := btss.MessageRouting{
		From:                    pi.PartyIDMap["1"],
		To:                      nil,
		IsBroadcast:             false,
		IsToOldCommittee:        false,
		IsToOldAndNewCommittees: false,
	}
	msg := messages.WireMessage{
		Routing:   &r1,
		RoundInfo: "key1",
		Message:   nil,
	}

	p.blameMgr.roundMgr.Set("key1", &msg)
	blames, err := p.blameMgr.GetBroadcastBlame("key1")
	c.Assert(err, IsNil)
	var blamePubKeys []string
	for _, el := range blames {
		blamePubKeys = append(blamePubKeys, el.Pubkey)
	}
	sort.Strings(blamePubKeys)
	expected := testPubKeys[2:]
	sort.Strings(expected)
	c.Assert(blamePubKeys, DeepEquals, expected)
}

func (p *policyTestSuite) TestTssWrongShareBlame(c *C) {
	pi := p.blameMgr.partyInfo

	r1 := btss.MessageRouting{
		From:                    pi.PartyIDMap["1"],
		To:                      nil,
		IsBroadcast:             false,
		IsToOldCommittee:        false,
		IsToOldAndNewCommittees: false,
	}
	msg := messages.WireMessage{
		Routing:   &r1,
		RoundInfo: "key2",
		Message:   nil,
	}
	target, err := p.blameMgr.TssWrongShareBlame(&msg)
	c.Assert(err, IsNil)
	c.Assert(target, Equals, "thorpub1addwnpepqfjcw5l4ay5t00c32mmlky7qrppepxzdlkcwfs2fd5u73qrwna0vzag3y4j")
}

func (p *policyTestSuite) TestTssMissingShareBlame(c *C) {
	localTestPubKeys := testPubKeys[:]
	sort.Strings(localTestPubKeys)
	blameMgr := p.blameMgr
	acceptedShares := blameMgr.acceptedShares
	// we only allow a message be updated only once.
	blameMgr.acceptShareLocker.Lock()
	acceptedShares[RoundInfo{0, "testRound", "123:0"}] = []string{"1", "2"}
	acceptedShares[RoundInfo{1, "testRound", "123:0"}] = []string{"1"}
	blameMgr.acceptShareLocker.Unlock()
	nodes, _, err := blameMgr.TssMissingShareBlame(2, messages.ECDSAKEYGEN)
	c.Assert(err, IsNil)
	c.Assert(nodes[0].Pubkey, Equals, localTestPubKeys[3])
	// we test if the missing share happens in round2
	blameMgr.acceptShareLocker.Lock()
	acceptedShares[RoundInfo{0, "testRound", "123:0"}] = []string{"1", "2", "3"}
	blameMgr.acceptShareLocker.Unlock()
	nodes, _, err = blameMgr.TssMissingShareBlame(2, messages.ECDSAKEYGEN)
	c.Assert(err, IsNil)
	results := []string{nodes[0].Pubkey, nodes[1].Pubkey}
	sort.Strings(results)
	c.Assert(results, DeepEquals, localTestPubKeys[2:])
}

func (p *policyTestSuite) TestManagerGetters(c *C) {
	mgr := NewBlameManager()
	c.Assert(mgr.GetBlame(), NotNil)
	c.Assert(mgr.GetShareMgr(), NotNil)
	c.Assert(mgr.GetRoundMgr(), NotNil)
}

func (p *policyTestSuite) TestUpdateAcceptShareAndCheckDuplication(c *C) {
	mgr := NewBlameManager()
	round := RoundInfo{0, "round1", "msg1"}

	// Initially no duplication
	c.Assert(mgr.CheckMsgDuplication(round, "party1"), Equals, false)

	// Add a share
	mgr.UpdateAcceptShare(round, "party1")

	// Now it's a duplicate
	c.Assert(mgr.CheckMsgDuplication(round, "party1"), Equals, true)

	// Different party is not a duplicate
	c.Assert(mgr.CheckMsgDuplication(round, "party2"), Equals, false)

	// Add to existing round
	mgr.UpdateAcceptShare(round, "party2")
	c.Assert(mgr.CheckMsgDuplication(round, "party2"), Equals, true)
}

func (p *policyTestSuite) TestSetAndGetLastMsg(c *C) {
	mgr := NewBlameManager()
	c.Assert(mgr.GetLastMsg(), IsNil)

	mgr.SetLastMsg(nil)
	c.Assert(mgr.GetLastMsg(), IsNil)
}

func (p *policyTestSuite) TestSetLastUnicastPeer(c *C) {
	mgr := NewBlameManager()
	p1, err := peer.Decode(testPeers[0])
	c.Assert(err, IsNil)
	p2, err := peer.Decode(testPeers[1])
	c.Assert(err, IsNil)

	// Add to a new round
	mgr.SetLastUnicastPeer(p1, "round1")
	// Add to existing round
	mgr.SetLastUnicastPeer(p2, "round1")

	mgr.lastMsgLocker.RLock()
	peers := mgr.lastUnicastPeer["round1"]
	mgr.lastMsgLocker.RUnlock()
	c.Assert(peers, HasLen, 2)
}

func (p *policyTestSuite) TestNodeSyncBlame(c *C) {
	conversion.SetupBech32Prefix()

	p1, err := peer.Decode(testPeers[0])
	c.Assert(err, IsNil)
	p2, err := peer.Decode(testPeers[1])
	c.Assert(err, IsNil)
	p3, err := peer.Decode(testPeers[2])
	c.Assert(err, IsNil)
	p4, err := peer.Decode(testPeers[3])
	c.Assert(err, IsNil)

	// Test with all peers online - no blame
	onlinePeers := []peer.ID{p1, p2, p3, p4}
	keys := testPubKeys[:]
	blame, err := p.blameMgr.NodeSyncBlame(keys, onlinePeers)
	c.Assert(err, IsNil)
	c.Assert(blame.FailReason, Equals, TssSyncFail)
	c.Assert(blame.BlameNodes, HasLen, 0)

	// Test with one peer missing
	onlinePeers = []peer.ID{p1, p2, p3}
	blame, err = p.blameMgr.NodeSyncBlame(keys, onlinePeers)
	c.Assert(err, IsNil)
	c.Assert(blame.BlameNodes, HasLen, 1)

	// Test with invalid pubkey
	badKeys := []string{"invalid-key"}
	_, err = p.blameMgr.NodeSyncBlame(badKeys, onlinePeers)
	c.Assert(err, NotNil)
}

func (p *policyTestSuite) TestGetUnicastBlameEmptyPeers(c *C) {
	mgr := NewBlameManager()
	// No unicast peers set - should return nil, nil
	nodes, err := mgr.GetUnicastBlame("anyType")
	c.Assert(err, IsNil)
	c.Assert(nodes, IsNil)
}

func (p *policyTestSuite) TestTssWrongShareBlameUnknownParty(c *C) {
	pi := p.blameMgr.partyInfo
	// Use a valid partyID but one that's not in the partyIDMap
	// We need to manipulate partyIDMap to simulate "not found"
	// Use a partyID with an ID not in the map
	knownPartyID := pi.PartyIDMap["1"]
	r1 := btss.MessageRouting{
		From: knownPartyID,
	}
	msg := messages.WireMessage{
		Routing:   &r1,
		RoundInfo: "key3",
	}

	// Remove the partyID from the map temporarily to cause error
	savedMap := p.blameMgr.partyInfo.PartyIDMap
	p.blameMgr.partyInfo.PartyIDMap = make(map[string]*btss.PartyID)
	_, err := p.blameMgr.TssWrongShareBlame(&msg)
	c.Assert(err, NotNil)
	p.blameMgr.partyInfo.PartyIDMap = savedMap
}

func (p *policyTestSuite) TestTssMissingShareBlameECDSAKeysign(c *C) {
	blameMgr := p.blameMgr
	blameMgr.acceptShareLocker.Lock()
	blameMgr.acceptedShares = make(map[RoundInfo][]string)
	blameMgr.acceptedShares[RoundInfo{0, "testRound", "msg:0"}] = []string{"1", "2"}
	blameMgr.acceptedShares[RoundInfo{1, "testRound", "msg:0"}] = []string{"1"}
	blameMgr.acceptShareLocker.Unlock()
	_, isUnicast, err := blameMgr.TssMissingShareBlame(2, messages.ECDSAKEYSIGN)
	c.Assert(err, IsNil)
	// For ECDSAKEYSIGN, index < 5 means unicast
	c.Assert(isUnicast, Equals, true)
}

func (p *policyTestSuite) TestTssMissingShareBlameEDDSAKeygen(c *C) {
	blameMgr := p.blameMgr
	blameMgr.acceptShareLocker.Lock()
	blameMgr.acceptedShares = make(map[RoundInfo][]string)
	// index 2 is the unicast round for EDDSAKEYGEN, but it also needs to be missing shares
	// Put full shares in rounds 0,1 and missing share at index 2
	blameMgr.acceptedShares[RoundInfo{0, "testRound", "msg:0"}] = []string{"1", "2", "3"}
	blameMgr.acceptedShares[RoundInfo{1, "testRound", "msg:0"}] = []string{"1", "2", "3"}
	blameMgr.acceptedShares[RoundInfo{2, "testRound", "msg:0"}] = []string{"1"}
	blameMgr.acceptShareLocker.Unlock()
	_, isUnicast, err := blameMgr.TssMissingShareBlame(3, messages.EDDSAKEYGEN)
	c.Assert(err, IsNil)
	// For EDDSAKEYGEN, index == 2 means unicast
	c.Assert(isUnicast, Equals, true)
}

func (p *policyTestSuite) TestTssMissingShareBlameEDDSAKeysign(c *C) {
	blameMgr := p.blameMgr
	blameMgr.acceptShareLocker.Lock()
	blameMgr.acceptedShares = make(map[RoundInfo][]string)
	blameMgr.acceptedShares[RoundInfo{0, "testRound", "msg:0"}] = []string{"1"}
	blameMgr.acceptShareLocker.Unlock()
	_, isUnicast, err := blameMgr.TssMissingShareBlame(1, messages.EDDSAKEYSIGN)
	c.Assert(err, IsNil)
	// For EDDSAKEYSIGN, unicast is always false
	c.Assert(isUnicast, Equals, false)
}

func (p *policyTestSuite) TestTssMissingShareBlameEmptyShares(c *C) {
	blameMgr := p.blameMgr
	blameMgr.acceptShareLocker.Lock()
	blameMgr.acceptedShares = make(map[RoundInfo][]string)
	blameMgr.acceptShareLocker.Unlock()
	nodes, isUnicast, err := blameMgr.TssMissingShareBlame(2, messages.ECDSAKEYGEN)
	c.Assert(err, IsNil)
	c.Assert(nodes, IsNil)
	c.Assert(isUnicast, Equals, false)
}

func (p *policyTestSuite) TestGetBroadcastBlameNoStandbyNodes(c *C) {
	blames, err := p.blameMgr.GetBroadcastBlame("nonexistent_round")
	c.Assert(err, IsNil)
	c.Assert(blames, IsNil)
}

func (p *policyTestSuite) TestGetBlamePubKeysLists(c *C) {
	inList, notInList, err := p.blameMgr.GetBlamePubKeysLists([]string{testPeers[0], testPeers[1]})
	c.Assert(err, IsNil)
	c.Assert(len(inList)+len(notInList) > 0, Equals, true)
}

func (p *policyTestSuite) TestGetBlamePubKeysListsEmpty(c *C) {
	inList, notInList, err := p.blameMgr.GetBlamePubKeysLists(nil)
	c.Assert(err, IsNil)
	c.Assert(inList, HasLen, 0)
	// notInList should have all non-local parties
	c.Assert(len(notInList) > 0, Equals, true)
}
