package p2p

import (
	"fmt"
	"time"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

type NodeGaterExtraTestSuite struct {
	mockBridge *MockThornadoBridge
}

var _ = Suite(&NodeGaterExtraTestSuite{})

func (s *NodeGaterExtraTestSuite) SetUpTest(c *C) {
	conversion.SetupBech32Prefix()
	s.mockBridge = NewMockThornadoBridge()
	s.mockBridge.SetConstants(map[string]int64{
		"BondStartAmountSats": 100_000_000,
	})
}

func (s *NodeGaterExtraTestSuite) TestGetMinimumBondFromMimir(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set mimir override
	s.mockBridge.SetMimir("BondStartAmountSats", 50_000_000)

	bond, err := gater.getMinimumBond()
	c.Assert(err, IsNil)
	c.Assert(bond, Equals, int64(50_000_000))
}

func (s *NodeGaterExtraTestSuite) TestGetMinimumBondFromConstants(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	bond, err := gater.getMinimumBond()
	c.Assert(err, IsNil)
	c.Assert(bond, Equals, int64(100_000_000))
}

func (s *NodeGaterExtraTestSuite) TestGetMinimumBondConstantsError(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// No mimir override falls back to Thornado's fixed 1 BTC start bond.
	s.mockBridge.SetConstants(map[string]int64{})

	bond, err := gater.getMinimumBond()
	c.Assert(err, IsNil)
	c.Assert(bond, Equals, int64(100_000_000))
}

func (s *NodeGaterExtraTestSuite) TestGetMinimumBondMimirError(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Mimir returns error, should fall back to constants
	s.mockBridge.SetMimirError(fmt.Errorf("mimir error"))

	bond, err := gater.getMinimumBond()
	c.Assert(err, IsNil)
	c.Assert(bond, Equals, int64(100_000_000))
}

func (s *NodeGaterExtraTestSuite) TestRefreshAllowlistInvalidBondString(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "not_a_number",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
		{
			NodeAddress: "thor1test2",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey2),
			},
		},
	})

	gater.refreshAllowlist()

	// Only the valid node should be in the list
	c.Assert(len(gater.allowedPeers), Equals, 1)
}

func (s *NodeGaterExtraTestSuite) TestRefreshAllowlistMimirError(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Mimir error should default to gating enabled
	s.mockBridge.SetMimirError(fmt.Errorf("mimir error"))

	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
	})

	gater.refreshAllowlist()
	c.Assert(len(gater.allowedPeers), Equals, 1)
	c.Assert(gater.gateDisabled, Equals, false)
}

func (s *NodeGaterExtraTestSuite) TestRefreshAllowlistGetMinBondError(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up initial allowlist
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
	})
	gater.refreshAllowlist()
	c.Assert(len(gater.allowedPeers), Equals, 1)

	// Now make getMinimumBond fail - remove constants
	s.mockBridge.SetConstants(map[string]int64{})
	s.mockBridge.SetMimirError(fmt.Errorf("mimir error"))

	gater.refreshAllowlist()

	// Should keep existing allowlist
	c.Assert(len(gater.allowedPeers), Equals, 1)
}

func (s *NodeGaterExtraTestSuite) TestStopBeforeStart(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)
	// Stop without Start should not panic
	gater.Stop()
}

func (s *NodeGaterExtraTestSuite) TestAddNodeToAllowlistInvalidPubkey(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Test node with invalid pubkey via refreshAllowlist
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey("invalid_pubkey"),
			},
		},
	})

	gater.refreshAllowlist()
	c.Assert(len(gater.allowedPeers), Equals, 0)
}
