package p2p

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	maddr "github.com/multiformats/go-multiaddr"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
)

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
	if c.host == nil {
		return fmt.Errorf("p2p host is not started")
	}
	peerIDs, err := conversion.GetPeerIDs(pubkeys)
	if err != nil {
		return fmt.Errorf("parse party peer ids: %w", err)
	}

	bootstrapPeers := c.getPeers()
	addrsByPeer := bootstrapAddrsByPeerID(bootstrapPeers)

	var connectErrs []error
	for _, pID := range peerIDs {
		if pID == c.host.ID() {
			continue
		}
		if c.host.Network().Connectedness(pID) == network.Connected {
			continue
		}
		addrs := addrsByPeer[pID]
		if len(addrs) == 0 {
			connectErrs = append(connectErrs, fmt.Errorf("no bootstrap address for peer %s", pID))
			continue
		}
		pi := peer.AddrInfo{ID: pID, Addrs: addrs}
		ctx, cancel := context.WithTimeout(context.Background(), TimeoutConnecting)
		err := c.host.Connect(ctx, pi)
		cancel()
		if err != nil {
			connectErrs = append(connectErrs, fmt.Errorf("connect peer %s: %w", pID, err))
			continue
		}
		c.logger.Info().Str("peer", pID.String()).Msg("connected party peer before join")
	}
	if len(connectErrs) > 0 {
		return fmt.Errorf("ensure party peers: %v", connectErrs)
	}
	return nil
}
