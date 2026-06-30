package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	swarm "github.com/libp2p/go-libp2p-swarm"
	maddr "github.com/multiformats/go-multiaddr"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
)

const PartyPeerConnectTimeout = 4 * time.Second

func bootstrapAddrsByPeerID(bootstrapPeers []maddr.Multiaddr) map[peer.ID][]maddr.Multiaddr {
	addrsByPeer := make(map[peer.ID][]maddr.Multiaddr)
	for _, bootstrapPeer := range bootstrapPeers {
		pi, err := peer.AddrInfoFromP2pAddr(bootstrapPeer)
		if err != nil {
			continue
		}
		addrsByPeer[pi.ID] = append(addrsByPeer[pi.ID], pi.Addrs...)
	}
	return addrsByPeer
}

// EnsurePeersConnected dials any party members that are not yet reachable.
// Party join uses validator pubkeys as peer IDs; bootstrap multiaddrs are the
// authoritative source of dial addresses in our deployment model.
func (c *Communication) EnsurePeersConnected(pubkeys []string) error {
	return c.ensurePeersConnected(pubkeys, len(pubkeys))
}

// EnsurePeersConnectedThreshold dials party members until the local node can
// reach enough peers to satisfy the FROST signing threshold.
func (c *Communication) EnsurePeersConnectedThreshold(pubkeys []string, threshold int) error {
	return c.ensurePeersConnected(pubkeys, threshold)
}

func (c *Communication) ensurePeersConnected(pubkeys []string, threshold int) error {
	if c.host == nil {
		return fmt.Errorf("p2p host is not started")
	}
	peerIDs, err := conversion.GetPeerIDs(pubkeys)
	if err != nil {
		return fmt.Errorf("parse party peer ids: %w", err)
	}
	peerIDs = uniquePeerIDs(peerIDs)
	if threshold <= 0 {
		threshold = len(peerIDs)
	}
	if threshold > len(peerIDs) {
		return fmt.Errorf("threshold %d exceeds party size %d", threshold, len(peerIDs))
	}

	bootstrapPeers := c.getPeers()
	addrsByPeer := bootstrapAddrsByPeerID(bootstrapPeers)

	var wg sync.WaitGroup
	errCh := make(chan error, len(peerIDs))
	var connectedMu sync.Mutex
	connected := 0
	markConnected := func() {
		connectedMu.Lock()
		connected++
		connectedMu.Unlock()
	}
	for _, pID := range peerIDs {
		if pID == c.host.ID() {
			markConnected()
			continue
		}
		if c.host.Network().Connectedness(pID) == network.Connected {
			markConnected()
			continue
		}
		addrs := addrsByPeer[pID]
		if len(addrs) == 0 {
			addrs = c.host.Peerstore().Addrs(pID)
		}
		if len(addrs) == 0 {
			errCh <- fmt.Errorf("no bootstrap address for peer %s", pID)
			continue
		}
		c.host.Peerstore().AddAddrs(pID, addrs, peerstore.TempAddrTTL)
		pi := peer.AddrInfo{ID: pID, Addrs: addrs}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), PartyPeerConnectTimeout)
			defer cancel()
			if err := c.host.Connect(ctx, pi); err != nil {
				errCh <- fmt.Errorf("connect peer %s: %w", pi.ID, err)
				return
			}
			markConnected()
			c.logger.Info().Str("peer", pi.ID.String()).Msg("connected party peer before join")
		}()
	}
	wg.Wait()
	close(errCh)
	var connectErrs []error
	for err := range errCh {
		connectErrs = append(connectErrs, err)
	}
	if connected < threshold {
		return fmt.Errorf("ensure party peers: connected %d/%d, threshold %d: %v", connected, len(peerIDs), threshold, connectErrs)
	}
	if len(connectErrs) > 0 {
		c.logger.Debug().
			Int("connected", connected).
			Int("party_size", len(peerIDs)).
			Int("threshold", threshold).
			Err(fmt.Errorf("%v", connectErrs)).
			Msg("party peer preconnect met threshold with unreachable peers")
	}
	return nil
}

func uniquePeerIDs(peerIDs []peer.ID) []peer.ID {
	seen := make(map[peer.ID]struct{}, len(peerIDs))
	out := make([]peer.ID, 0, len(peerIDs))
	for _, pID := range peerIDs {
		if _, ok := seen[pID]; ok {
			continue
		}
		seen[pID] = struct{}{}
		out = append(out, pID)
	}
	return out
}

func (c *Communication) clearDialBackoff(peerIDs []peer.ID) {
	if c.host == nil {
		return
	}
	sw, ok := c.host.Network().(*swarm.Swarm)
	if !ok {
		return
	}
	for _, pID := range peerIDs {
		sw.Backoff().Clear(pID)
	}
}

func (c *Communication) EnsurePeersConnectedWithin(pubkeys []string, timeout time.Duration) error {
	if timeout <= 0 {
		return c.EnsurePeersConnected(pubkeys)
	}
	peerIDs, err := conversion.GetPeerIDs(pubkeys)
	if err != nil {
		return fmt.Errorf("parse party peer ids: %w", err)
	}
	peerIDs = uniquePeerIDs(peerIDs)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := c.EnsurePeersConnected(pubkeys); err != nil {
			lastErr = err
		} else {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return lastErr
		}
		c.clearDialBackoff(peerIDs)
		sleep := 250 * time.Millisecond
		if remaining < sleep {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
}
