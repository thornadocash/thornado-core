package p2p

import (
	"fmt"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/bifrost/p2p/conversion"
	"gitlab.com/thorchain/thornode/v3/bifrost/thorclient"
	tctypes "gitlab.com/thorchain/thornode/v3/bifrost/thorclient/types"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/config"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type NodeGaterTestSuite struct {
	mockBridge *MockThorchainBridge
}

var _ = Suite(&NodeGaterTestSuite{})

// MockThorchainBridge is a mock implementation of ThorchainBridge for testing
type MockThorchainBridge struct {
	mu                sync.RWMutex
	nodeAccounts      []*types.QueryNodeResponse
	nodeAccountsError error
	mimirValues       map[string]int64
	mimirError        error
	constants         map[string]int64
}

func NewMockThorchainBridge() *MockThorchainBridge {
	return &MockThorchainBridge{
		mimirValues: make(map[string]int64),
	}
}

func (m *MockThorchainBridge) GetNodeAccounts() ([]*types.QueryNodeResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.nodeAccountsError != nil {
		return nil, m.nodeAccountsError
	}
	return m.nodeAccounts, nil
}

func (m *MockThorchainBridge) GetMimir(key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.mimirError != nil {
		return 0, m.mimirError
	}
	val, ok := m.mimirValues[key]
	if !ok {
		return 0, nil
	}
	return val, nil
}

func (m *MockThorchainBridge) SetNodeAccounts(accounts []*types.QueryNodeResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeAccounts = accounts
}

func (m *MockThorchainBridge) SetNodeAccountsError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeAccountsError = err
}

func (m *MockThorchainBridge) SetMimir(key string, value int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mimirValues[key] = value
}

func (m *MockThorchainBridge) SetMimirError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mimirError = err
}

// Stub implementations for the rest of the ThorchainBridge interface
func (m *MockThorchainBridge) EnsureNodeWhitelisted() error            { return nil }
func (m *MockThorchainBridge) EnsureNodeWhitelistedWithTimeout() error { return nil }
func (m *MockThorchainBridge) FetchNodeStatus() (types.NodeStatus, error) {
	return types.NodeStatus_Unknown, nil
}
func (m *MockThorchainBridge) FetchActiveNodes() ([]common.PubKey, error)  { return nil, nil }
func (m *MockThorchainBridge) GetAsgards() (types.Vaults, error)           { return nil, nil }
func (m *MockThorchainBridge) GetVault(pubkey string) (types.Vault, error) { return types.Vault{}, nil }
func (m *MockThorchainBridge) GetConfig() config.BifrostClientConfiguration {
	return config.BifrostClientConfiguration{}
}

func (m *MockThorchainBridge) GetConstants() (map[string]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.constants != nil {
		return m.constants, nil
	}
	return map[string]int64{}, nil
}

func (m *MockThorchainBridge) SetConstants(c map[string]int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.constants = c
}
func (m *MockThorchainBridge) GetContext() client.Context { return client.Context{} }
func (m *MockThorchainBridge) GetContractAddress() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}
func (m *MockThorchainBridge) GetErrataMsg(txID common.TxID, chain common.Chain) sdk.Msg { return nil }
func (m *MockThorchainBridge) GetKeygenStdTx(poolPubKey common.PubKey, secp256k1Signature, keysharesBackup []byte, blame []types.Blame, inputPks common.PubKeys, keygenType types.KeygenType, chains common.Chains, height, keygenTime int64, poolPubKeyEddsa common.PubKey, keysharesBackupEddsa []byte) (sdk.Msg, error) {
	return nil, nil
}

func (m *MockThorchainBridge) GetKeysignParty(vaultPubKey common.PubKey) (common.PubKeys, error) {
	return nil, nil
}
func (m *MockThorchainBridge) GetMimirWithRef(template, ref string) (int64, error) { return 0, nil }
func (m *MockThorchainBridge) GetInboundOutbound(txIns common.ObservedTxs) (common.ObservedTxs, common.ObservedTxs, error) {
	return nil, nil, nil
}
func (m *MockThorchainBridge) GetPools() (types.Pools, error) { return nil, nil }
func (m *MockThorchainBridge) GetPubKeys() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}

func (m *MockThorchainBridge) GetAsgardPubKeys() ([]thorclient.PubKeyContractAddressPair, error) {
	return nil, nil
}

func (m *MockThorchainBridge) GetSolvencyMsg(height int64, chain common.Chain, pubKey common.PubKey, coins common.Coins) *types.MsgSolvency {
	return nil
}

func (m *MockThorchainBridge) GetTHORName(name string) (types.THORName, error) {
	return types.THORName{}, nil
}

func (m *MockThorchainBridge) GetThorchainVersion() (semver.Version, error) {
	return semver.Version{}, nil
}
func (m *MockThorchainBridge) IsCatchingUp() (bool, error)                    { return false, nil }
func (m *MockThorchainBridge) HasNetworkFee(chain common.Chain) (bool, error) { return false, nil }
func (m *MockThorchainBridge) GetNetworkFee(chain common.Chain) (transactionSize, transactionFeeRate uint64, err error) {
	return 0, 0, nil
}

func (m *MockThorchainBridge) PostKeysignFailure(blame types.Blame, height int64, memo string, coins common.Coins, pubkey common.PubKey) (common.TxID, error) {
	return "", nil
}

func (m *MockThorchainBridge) PostNetworkFee(height int64, chain common.Chain, transactionSize, transactionRate uint64) (common.TxID, error) {
	return "", nil
}
func (m *MockThorchainBridge) RagnarokInProgress() (bool, error) { return false, nil }
func (m *MockThorchainBridge) WaitToCatchUp() error              { return nil }
func (m *MockThorchainBridge) GetBlockHeight() (int64, error)    { return 0, nil }
func (m *MockThorchainBridge) GetLastObservedInHeight(chain common.Chain) (int64, error) {
	return 0, nil
}

func (m *MockThorchainBridge) GetLastSignedOutHeight(chain common.Chain) (int64, error) {
	return 0, nil
}
func (m *MockThorchainBridge) Broadcast(msgs ...sdk.Msg) (common.TxID, error) { return "", nil }
func (m *MockThorchainBridge) BroadcastWithBlocking(msgs ...sdk.Msg) (common.TxID, error) {
	return "", nil
}

func (m *MockThorchainBridge) GetKeysign(blockHeight int64, pk string) (tctypes.TxOut, error) {
	return tctypes.TxOut{}, nil
}
func (m *MockThorchainBridge) GetNodeAccount(string) (*types.NodeAccount, error) { return nil, nil }
func (m *MockThorchainBridge) GetKeygenBlock(int64, string) (types.KeygenBlock, error) {
	return types.KeygenBlock{}, nil
}

func (m *MockThorchainBridge) GetReferenceMemo(chain common.Chain, ref string) (string, error) {
	return "", nil
}
func (m *MockThorchainBridge) GetReferenceMemoByTxHash(hash string) (string, error) { return "", nil }
func (m *MockThorchainBridge) GetBlockTimestamp(height int64) (time.Time, error) {
	return time.Time{}, nil
}

func (s *NodeGaterTestSuite) SetUpTest(c *C) {
	conversion.SetupBech32Prefix()
	s.mockBridge = NewMockThorchainBridge()
	// Set default minimum bond for tests
	s.mockBridge.SetConstants(map[string]int64{
		constants.MinimumBondInRune.String(): 100_000_000, // 1 RUNE
	})
}

// Test valid secp256k1 pubkeys for creating test nodes (from conversion_test.go)
const (
	testPubKey1 = "thorpub1addwnpepqtdklw8tf3anjz7nn5fly3uvq2e67w2apn560s4smmrt9e3x52nt2svmmu3"
	testPubKey2 = "thorpub1addwnpepqtspqyy6gk22u37ztra4hq3hdakc0w0k60sfy849mlml2vrpfr0wvm6uz09"
	testPubKey3 = "thorpub1addwnpepq2ryyje5zr09lq7gqptjwnxqsy2vcdngvwd6z7yt5yjcnyj8c8cn559xe69"
)

func (s *NodeGaterTestSuite) TestNewNodeGater(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Second*10)
	c.Assert(gater, NotNil)
	c.Assert(gater.bridge, Equals, s.mockBridge)
	c.Assert(gater.refreshInterval, Equals, time.Second*10)
	c.Assert(gater.allowedPeers, NotNil)
	c.Assert(len(gater.allowedPeers), Equals, 0)
}

func (s *NodeGaterTestSuite) TestStartStop(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Millisecond*100)

	// Set up some test nodes with sufficient bond
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

	// Start the gater
	gater.Start()
	c.Assert(gater.stopChan, NotNil)
	c.Assert(gater.stopOnce, NotNil)

	// Give it a moment to start
	time.Sleep(time.Millisecond * 50)

	// Stop should block until goroutine completes
	done := make(chan bool)
	go func() {
		gater.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Stop completed successfully
	case <-time.After(time.Second * 2):
		c.Fatal("Stop() did not complete within timeout")
	}
}

func (s *NodeGaterTestSuite) TestStartStopMultipleTimes(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Millisecond*100)

	// Set up some test nodes with sufficient bond
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

	// First start/stop cycle
	gater.Start()
	time.Sleep(time.Millisecond * 50)
	gater.Stop()

	// Calling Stop again should be safe (idempotent)
	gater.Stop()

	// Second start/stop cycle - should work cleanly
	gater.Start()
	time.Sleep(time.Millisecond * 50)
	gater.Stop()
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistBondBasedFiltering(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up nodes with various statuses but all with sufficient bond
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
		{
			NodeAddress: "thor1test2",
			Status:      "Ready",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey2),
			},
		},
	})

	gater.refreshAllowlist()

	c.Assert(len(gater.allowedPeers), Equals, 2)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistJailedNodeWithBond(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// A jailed standby node with sufficient bond should still be allowed for p2p
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Standby",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
			PreflightStatus: &types.NodePreflightStatus{
				Status: "Standby",
				Reason: "node account is currently jailed",
			},
		},
		{
			NodeAddress: "thor1test2",
			Status:      "Standby",
			TotalBond:   "50000000", // below minimum bond
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey2),
			},
			PreflightStatus: &types.NodePreflightStatus{
				Status: "Standby",
				Reason: "Not enough bond",
			},
		},
	})

	gater.refreshAllowlist()

	// Only the node with sufficient bond should be allowed, regardless of jail status
	c.Assert(len(gater.allowedPeers), Equals, 1)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistInsufficientBond(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up nodes with various statuses but all with insufficient bond
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Disabled",
			TotalBond:   "50000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
		{
			NodeAddress: "thor1test2",
			Status:      "Whitelisted",
			TotalBond:   "0",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey2),
			},
		},
		{
			NodeAddress: "thor1test3",
			Status:      "Active",
			TotalBond:   "50000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey3),
			},
		},
	})

	gater.refreshAllowlist()

	// None of these should be allowed (all below minimum bond of 100_000_000)
	c.Assert(len(gater.allowedPeers), Equals, 0)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistMimirBondOverride(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set mimir override for minimum bond (lower than constants)
	s.mockBridge.SetMimir(constants.MinimumBondInRune.String(), 50_000_000)

	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Standby",
			TotalBond:   "75000000", // above mimir override, below constants
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
	})

	gater.refreshAllowlist()

	// Should be allowed because mimir override is 50M and node has 75M
	c.Assert(len(gater.allowedPeers), Equals, 1)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistGatingDisabled(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Enable the P2PGateDisabled mimir
	s.mockBridge.SetMimir(constants.P2PGateDisabled.String(), 1)

	// Set up active nodes with bond
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

	// Gating is disabled, so the allowlist should not be updated
	c.Assert(gater.gateDisabled, Equals, true)
	// The allowlist should be empty because refresh exits early when gating is disabled
	c.Assert(len(gater.allowedPeers), Equals, 0)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistError(c *C) {
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
	initialCount := len(gater.allowedPeers)
	c.Assert(initialCount, Equals, 1)

	// Set error for GetNodeAccounts
	s.mockBridge.SetNodeAccountsError(fmt.Errorf("network error"))

	gater.refreshAllowlist()

	// Allowlist should remain unchanged
	c.Assert(len(gater.allowedPeers), Equals, initialCount)
}

func (s *NodeGaterTestSuite) TestRefreshAllowlistEmptyPubkey(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up node with empty pubkey but sufficient bond
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(""),
			},
		},
		{
			NodeAddress: "thor1test2",
			Status:      "Active",
			TotalBond:   "200000000",
			PubKeySet: common.PubKeySet{
				Secp256k1: common.PubKey(testPubKey1),
			},
		},
	})

	gater.refreshAllowlist()

	// Only the node with valid pubkey should be in the allowlist
	c.Assert(len(gater.allowedPeers), Equals, 1)
}

func (s *NodeGaterTestSuite) TestIsPeerAllowed(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up nodes with sufficient bond
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

	// Get the peer ID for the test pubkey
	peerID, err := getPeerIDFromPubKey(testPubKey1)
	c.Assert(err, IsNil)

	// Check if allowed
	allowed, nodeAddr := gater.isPeerAllowed(peerID)
	c.Assert(allowed, Equals, true)
	c.Assert(nodeAddr, Equals, "thor1test1")

	// Check with a random peer ID that's not in the list
	randomPeerID := peer.ID("12D3KooWRandom")
	allowed, nodeAddr = gater.isPeerAllowed(randomPeerID)
	c.Assert(allowed, Equals, false)
	c.Assert(nodeAddr, Equals, "")
}

func (s *NodeGaterTestSuite) TestInterceptPeerDial(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)
	randomPeerID := peer.ID("12D3KooWRandom")

	// Outbound dials should always be allowed
	allowed := gater.InterceptPeerDial(randomPeerID)
	c.Assert(allowed, Equals, true)
}

func (s *NodeGaterTestSuite) TestInterceptAddrDial(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)
	randomPeerID := peer.ID("12D3KooWRandom")

	// Outbound dials should always be allowed
	allowed := gater.InterceptAddrDial(randomPeerID, nil)
	c.Assert(allowed, Equals, true)
}

func (s *NodeGaterTestSuite) TestInterceptAccept(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// All inbound connections should be accepted at this stage
	// (authentication happens later in InterceptSecured)
	mockConnMultiaddrs := &mockConnMultiaddrs{}
	allowed := gater.InterceptAccept(mockConnMultiaddrs)
	c.Assert(allowed, Equals, true)
}

func (s *NodeGaterTestSuite) TestInterceptSecured(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up nodes with sufficient bond
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

	// Get the peer ID for the test pubkey
	allowedPeerID, err := getPeerIDFromPubKey(testPubKey1)
	c.Assert(err, IsNil)

	// Test outbound connection (should always be allowed)
	allowed := gater.InterceptSecured(network.DirOutbound, allowedPeerID, nil)
	c.Assert(allowed, Equals, true)

	// Test inbound connection from allowed peer
	allowed = gater.InterceptSecured(network.DirInbound, allowedPeerID, nil)
	c.Assert(allowed, Equals, true)

	// Test inbound connection from non-allowed peer
	randomPeerID := peer.ID("12D3KooWRandom")
	allowed = gater.InterceptSecured(network.DirInbound, randomPeerID, nil)
	c.Assert(allowed, Equals, false)
}

func (s *NodeGaterTestSuite) TestInterceptSecuredGatingDisabled(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Enable the P2PGateDisabled mimir
	s.mockBridge.SetMimir(constants.P2PGateDisabled.String(), 1)
	gater.refreshAllowlist()

	// Test inbound connection from random peer (should be allowed because gating is disabled)
	randomPeerID := peer.ID("12D3KooWRandom")
	allowed := gater.InterceptSecured(network.DirInbound, randomPeerID, nil)
	c.Assert(allowed, Equals, true)
}

func (s *NodeGaterTestSuite) TestInterceptUpgraded(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Set up nodes with sufficient bond
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

	// Get the peer ID for the test pubkey
	allowedPeerID, err := getPeerIDFromPubKey(testPubKey1)
	c.Assert(err, IsNil)

	// Create mock connections
	mockOutboundConn := &mockConn{direction: network.DirOutbound, remotePeer: allowedPeerID}
	mockInboundAllowed := &mockConn{direction: network.DirInbound, remotePeer: allowedPeerID}
	randomPeerID := peer.ID("12D3KooWRandom")
	mockInboundDenied := &mockConn{direction: network.DirInbound, remotePeer: randomPeerID}

	// Test outbound connection (should always be allowed)
	allowed, reason := gater.InterceptUpgraded(mockOutboundConn)
	c.Assert(allowed, Equals, true)
	c.Assert(int(reason), Equals, 0)

	// Test inbound connection from allowed peer
	allowed, reason = gater.InterceptUpgraded(mockInboundAllowed)
	c.Assert(allowed, Equals, true)
	c.Assert(int(reason), Equals, 0)

	// Test inbound connection from non-allowed peer
	allowed, reason = gater.InterceptUpgraded(mockInboundDenied)
	c.Assert(allowed, Equals, false)
	c.Assert(int(reason), Equals, 0)
}

func (s *NodeGaterTestSuite) TestInterceptUpgradedGatingDisabled(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Minute)

	// Enable the P2PGateDisabled mimir
	s.mockBridge.SetMimir(constants.P2PGateDisabled.String(), 1)
	gater.refreshAllowlist()

	// Test inbound connection from random peer (should be allowed because gating is disabled)
	randomPeerID := peer.ID("12D3KooWRandom")
	mockInboundConn := &mockConn{direction: network.DirInbound, remotePeer: randomPeerID}
	allowed, reason := gater.InterceptUpgraded(mockInboundConn)
	c.Assert(allowed, Equals, true)
	c.Assert(int(reason), Equals, 0)
}

func (s *NodeGaterTestSuite) TestPeriodicRefresh(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Millisecond*100)

	// Set up initial nodes with sufficient bond
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

	gater.Start()
	time.Sleep(time.Millisecond * 50)

	// Verify initial allowlist
	c.Assert(len(gater.allowedPeers), Equals, 1)

	// Update the node list
	s.mockBridge.SetNodeAccounts([]*types.QueryNodeResponse{
		{
			NodeAddress: "thor1test1",
			Status:      "Active",
			TotalBond:   "200000000",
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

	// Wait for a refresh cycle
	time.Sleep(time.Millisecond * 150)

	// Verify allowlist was updated
	c.Assert(len(gater.allowedPeers), Equals, 2)

	gater.Stop()
}

func (s *NodeGaterTestSuite) TestConcurrentAccess(c *C) {
	gater := NewNodeGater(s.mockBridge, time.Millisecond*50)

	// Set up nodes with sufficient bond
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

	gater.Start()

	// Concurrently check peer access while refreshes are happening
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peerID, err := getPeerIDFromPubKey(testPubKey1)
			if err != nil {
				return
			}
			for j := 0; j < 100; j++ {
				gater.isPeerAllowed(peerID)
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	gater.Stop()
}

// Helper function to get peer ID from pubkey string
func getPeerIDFromPubKey(pubkey string) (peer.ID, error) {
	return conversion.GetPeerIDFromPubKey(pubkey)
}

// mockConn is a mock implementation of network.Conn for testing
type mockConn struct {
	direction  network.Direction
	remotePeer peer.ID
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalPeer() peer.ID                 { return "" }
func (m *mockConn) RemotePeer() peer.ID                { return m.remotePeer }
func (m *mockConn) RemotePublicKey() crypto.PubKey     { return nil }
func (m *mockConn) LocalMultiaddr() ma.Multiaddr       { return nil }
func (m *mockConn) RemoteMultiaddr() ma.Multiaddr      { return nil }
func (m *mockConn) LocalPrivateKey() crypto.PrivKey    { return nil }
func (m *mockConn) Stat() network.Stat                 { return network.Stat{Direction: m.direction} }
func (m *mockConn) ID() string                         { return "" }
func (m *mockConn) NewStream() (network.Stream, error) { return nil, nil }
func (m *mockConn) GetStreams() []network.Stream       { return nil }

// mockConnMultiaddrs is a mock implementation of network.ConnMultiaddrs for testing
type mockConnMultiaddrs struct{}

func (m *mockConnMultiaddrs) LocalMultiaddr() ma.Multiaddr {
	addr, _ := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	return addr
}

func (m *mockConnMultiaddrs) RemoteMultiaddr() ma.Multiaddr {
	addr, _ := ma.NewMultiaddr("/ip4/192.168.1.1/tcp/5678")
	return addr
}
