package p2p

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

type PeerStatusExtraTestSuite struct{}

var _ = Suite(&PeerStatusExtraTestSuite{})

func (s *PeerStatusExtraTestSuite) TestNewPeerStatusExcludesSelf(c *C) {
	peers := generateRandomPeers(c, 3)
	myPeer := peers[0]
	leader := peers[1]

	ps := newPeerStatus(peers, myPeer, leader, 2)
	// Self should be excluded from peersResponse
	_, ok := ps.peersResponse[myPeer]
	c.Assert(ok, Equals, false)
	c.Assert(len(ps.peersResponse), Equals, 2)
}

func (s *PeerStatusExtraTestSuite) TestGetAllPeers(c *C) {
	peers := generateRandomPeers(c, 4)
	ps := newPeerStatus(peers, peers[0], peers[1], 2)

	allPeers := ps.getAllPeers()
	c.Assert(len(allPeers), Equals, 4)
}

func (s *PeerStatusExtraTestSuite) TestGetSetLeaderResponse(c *C) {
	peers := generateRandomPeers(c, 3)
	ps := newPeerStatus(peers, peers[0], peers[1], 2)

	// Initially nil
	c.Assert(ps.getLeaderResponse(), IsNil)

	// Set and get
	resp := &messages.JoinPartyLeaderComm{ID: "test"}
	ps.setLeaderResponse(resp)
	c.Assert(ps.getLeaderResponse(), NotNil)
	c.Assert(ps.getLeaderResponse().ID, Equals, "test")
}

func (s *PeerStatusExtraTestSuite) TestGetLeader(c *C) {
	peers := generateRandomPeers(c, 3)
	ps := newPeerStatus(peers, peers[0], peers[1], 2)

	c.Assert(ps.getLeader(), Equals, peers[1])
}

func (s *PeerStatusExtraTestSuite) TestUpdatePeerWithNoneLeader(c *C) {
	peers := generateRandomPeers(c, 4)
	sortPeers(peers)

	// When leader is "NONE", updatePeer always returns true on first update
	ps := newPeerStatus(peers, peers[0], "NONE", 2)

	// First update should return true
	ret, err := ps.updatePeer(peers[1])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, true)

	// Second update of same peer should return false (already marked true)
	ret, err = ps.updatePeer(peers[1])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, false)

	// Different peer update should return true
	ret, err = ps.updatePeer(peers[2])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, true)
}

func (s *PeerStatusExtraTestSuite) TestGetCoordinationStatusAllOnline(c *C) {
	peers := generateRandomPeers(c, 3)
	sortPeers(peers)
	ps := newPeerStatus(peers, peers[0], "NONE", 2)

	// Initially not all coordinated
	c.Assert(ps.getCoordinationStatus(), Equals, false)

	// Update all peers except self
	for _, p := range peers[1:] {
		_, _ = ps.updatePeer(p)
	}

	// Now all should be coordinated
	c.Assert(ps.getCoordinationStatus(), Equals, true)
}

func (s *PeerStatusExtraTestSuite) TestUpdatePeerThresholdReached(c *C) {
	peers := generateRandomPeers(c, 5)
	sortPeers(peers)
	ps := newPeerStatus(peers, peers[0], peers[0], 2)

	// First peer - reqCount becomes 1
	ret, err := ps.updatePeer(peers[1])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, false)

	// Second peer - reqCount becomes 2, equals threshold
	ret, err = ps.updatePeer(peers[2])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, true)

	// Third peer - should be rejected (already at threshold)
	ret, err = ps.updatePeer(peers[3])
	c.Assert(err, IsNil)
	c.Assert(ret, Equals, false)
}
