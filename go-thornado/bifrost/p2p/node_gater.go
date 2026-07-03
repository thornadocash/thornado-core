package p2p

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// NodeGater implements ConnectionGater to restrict P2P connections to only Thornado validator nodes.
// It periodically fetches the list of active node accounts from Thornado and caches their peer IDs.
type NodeGater struct {
	bridge          thornadoclient.ThornadoBridge
	logger          zerolog.Logger
	allowedPeers    map[peer.ID]string // maps peer ID to node address
	gateDisabled    bool
	mu              sync.RWMutex
	refreshInterval time.Duration
	stopChan        chan struct{}
	stopOnce        *sync.Once
	wg              sync.WaitGroup
}

// NewNodeGater creates a new ConnectionGater that only allows connections from Thornado nodes.
// refreshInterval determines how often the node list is refreshed from Thornado (e.g., 60 seconds).
func NewNodeGater(bridge thornadoclient.ThornadoBridge, refreshInterval time.Duration) *NodeGater {
	gater := &NodeGater{
		bridge:          bridge,
		logger:          log.With().Str("module", "node_gater").Logger(),
		allowedPeers:    make(map[peer.ID]string), // maps peer ID to node address
		refreshInterval: refreshInterval,
	}
	return gater
}

// Start begins the periodic refresh of the node allowlist.
func (g *NodeGater) Start() {
	g.logger.Info().Msg("starting node gater")

	// Initialize the stop channel and stopOnce for clean restarts
	g.stopChan = make(chan struct{})
	g.stopOnce = &sync.Once{}

	// Perform initial load
	g.refreshAllowlist()

	// Start periodic refresh
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		ticker := time.NewTicker(g.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				g.refreshAllowlist()
			case <-g.stopChan:
				g.logger.Info().Msg("node gater stopped")
				return
			}
		}
	}()
}

// Stop halts the periodic refresh of the node allowlist and waits for the goroutine to complete.
// It is safe to call Stop multiple times, or even if Start was never called.
func (g *NodeGater) Stop() {
	if g.stopOnce != nil && g.stopChan != nil {
		g.stopOnce.Do(func() {
			close(g.stopChan)
			g.wg.Wait()
		})
	}
}

// refreshAllowlist fetches the current list of node accounts from Thornado
// and updates the allowlist of peer IDs that are permitted to connect.
// Allows any node with bond >= the current minimum bond requirement.
// This ensures nodes that are jailed or otherwise temporarily excluded from
// preflight checks can still maintain p2p connectivity and recover.
func (g *NodeGater) refreshAllowlist() {
	nodes, err := g.bridge.GetNodeAccounts()
	if err != nil {
		g.logger.Error().Err(err).Msg("failed to fetch node accounts, keeping existing allowlist")
		return
	}

	// Get the minimum bond requirement
	minBond, err := g.getMinimumBond()
	if err != nil {
		g.logger.Error().Err(err).Msg("failed to fetch minimum bond, keeping existing allowlist")
		return
	}

	newAllowed := make(map[peer.ID]string) // maps peer ID to node address

	for _, node := range nodes {
		// Parse the node's total bond
		nodeBond, err := strconv.ParseInt(node.TotalBond, 10, 64)
		if err != nil {
			g.logger.Warn().Err(err).Str("node_addr", node.NodeAddress).Str("total_bond", node.TotalBond).Msg("failed to parse node bond")
			continue
		}

		// Allow any node with bond >= minimum bond
		if nodeBond >= minBond {
			if err := g.addNodeToAllowlist(node, newAllowed); err != nil {
				g.logger.Warn().Err(err).Str("node_addr", node.NodeAddress).Msg("failed to add node to allowlist")
			}
		}
	}

	g.mu.Lock()
	g.allowedPeers = newAllowed
	g.mu.Unlock()

	g.logger.Info().Int("count", len(newAllowed)).Int64("min_bond", minBond).Msg("refreshed node allowlist")
}

// getMinimumBond fetches the current minimum bond requirement.
func (g *NodeGater) getMinimumBond() (int64, error) {
	if os.Getenv("BIFROST_FROST_ALLOW_ZERO_BOND_NODES") == "true" {
		return 0, nil
	}
	configBond, err := g.bridge.GetConfigValue(constants.Node_BondStartAmountSats.String())
	if err == nil && configBond > 0 {
		return configBond, nil
	}
	return constants.NewConfigValue().GetInt64Value(constants.Node_BondStartAmountSats), nil
}

// addNodeToAllowlist converts a node's pubkey to a peer ID and adds it to the allowlist
func (g *NodeGater) addNodeToAllowlist(node *types.QueryNodeResponse, allowlist map[peer.ID]string) error {
	if node.PubKeySet.Secp256k1.IsEmpty() {
		return fmt.Errorf("node has empty secp256k1 pubkey")
	}

	peerID, err := conversion.GetPeerIDFromPubKey(node.PubKeySet.Secp256k1.String())
	if err != nil {
		return fmt.Errorf("failed to convert pubkey to peer ID: %w", err)
	}

	allowlist[peerID] = node.NodeAddress
	return nil
}

// isPeerAllowed checks if a peer ID is in the current allowlist.
// Returns the node address if allowed, empty string if not.
func (g *NodeGater) isPeerAllowed(p peer.ID) (bool, string) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nodeAddr, ok := g.allowedPeers[p]
	return ok, nodeAddr
}

// InterceptPeerDial tests whether we're permitted to dial the specified peer.
// For outbound connections, we allow all dials (we trust our own node to dial correctly).
func (g *NodeGater) InterceptPeerDial(p peer.ID) bool {
	return true
}

// InterceptAddrDial tests whether we're permitted to dial the specified multiaddr for the given peer.
// For outbound connections, we allow all dials.
func (g *NodeGater) InterceptAddrDial(p peer.ID, m ma.Multiaddr) bool {
	return true
}

// InterceptAccept tests whether an incipient inbound connection is allowed.
// At this stage, the peer hasn't been authenticated yet, so we don't have a peer ID.
// We perform the actual gating in InterceptSecured after authentication.
func (g *NodeGater) InterceptAccept(connMultiaddrs network.ConnMultiaddrs) bool {
	// Allow all connections at this stage - we'll gate after authentication
	g.logger.Debug().
		Str("remote", connMultiaddrs.RemoteMultiaddr().String()).
		Msg("accepting inbound connection for authentication")
	return true
}

// InterceptSecured tests whether a given connection, now authenticated, is allowed.
// We perform a secondary check here after the peer has been cryptographically authenticated.
func (g *NodeGater) InterceptSecured(dir network.Direction, p peer.ID, connMultiaddrs network.ConnMultiaddrs) bool {
	// Only gate inbound connections
	if dir == network.DirOutbound {
		return true
	}

	// Check if gating is disabled
	g.mu.RLock()
	disabled := g.gateDisabled
	g.mu.RUnlock()
	if disabled {
		g.logger.Info().
			Str("peer_id", p.String()).
			Msg("accepted inbound connection (gating disabled by config)")
		return true
	}

	allowed, nodeAddr := g.isPeerAllowed(p)
	if allowed {
		g.logger.Info().
			Str("node_addr", nodeAddr).
			Str("peer_id", p.String()).
			Msg("accepted inbound connection from allowed node")
		return true
	}

	g.logger.Warn().
		Str("peer", p.String()).
		Str("direction", dir.String()).
		Msg("rejecting secured connection from non-allowed peer")
	return false
}

// InterceptUpgraded tests whether a fully capable connection is allowed.
// At this point, the connection has a multiplexer selected.
func (g *NodeGater) InterceptUpgraded(conn network.Conn) (bool, control.DisconnectReason) {
	// Only gate inbound connections
	if conn.Stat().Direction == network.DirOutbound {
		return true, 0
	}

	// Check if gating is disabled
	g.mu.RLock()
	disabled := g.gateDisabled
	g.mu.RUnlock()
	if disabled {
		remotePeer := conn.RemotePeer()
		g.logger.Info().
			Str("peer_id", remotePeer.String()).
			Msg("upgraded connection (gating disabled by config)")
		return true, 0
	}

	remotePeer := conn.RemotePeer()
	allowed, nodeAddr := g.isPeerAllowed(remotePeer)
	if allowed {
		g.logger.Info().
			Str("node_addr", nodeAddr).
			Str("peer_id", remotePeer.String()).
			Msg("upgraded connection from allowed node")
		return true, 0
	}

	g.logger.Warn().
		Str("peer", remotePeer.String()).
		Msg("rejecting upgraded connection from non-allowed peer")
	return false, 0
}

// Ensure NodeGater implements connmgr.ConnectionGater
var _ connmgr.ConnectionGater = (*NodeGater)(nil)
