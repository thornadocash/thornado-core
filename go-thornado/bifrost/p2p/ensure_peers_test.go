package p2p

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
)

func TestBootstrapAddrsByPeerID(t *testing.T) {
	sk, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw, err := sk.Raw()
	require.NoError(t, err)
	comm, err := NewCommunication(&Config{Port: 19240, RendezvousString: "ensure-peers-test"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm.Start(raw))
	defer func() { _ = comm.Stop() }()

	pubkey, err := conversion.GetPubKeyFromPeerID(comm.host.ID().String())
	require.NoError(t, err)

	ma, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19240/p2p/" + comm.host.ID().String())
	require.NoError(t, err)
	addrs := bootstrapAddrsByPeerID([]maddr.Multiaddr{ma})
	require.Contains(t, addrs, comm.host.ID())
	require.NotEmpty(t, addrs[comm.host.ID()])

	require.NoError(t, comm.EnsurePeersConnected([]string{pubkey}))
}

func TestEnsurePeersConnectedDialsStalePeerstoreAddress(t *testing.T) {
	cfg1 := &Config{Port: 19241, RendezvousString: "ensure-peers-stale-1"}
	sk1, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw1, err := sk1.Raw()
	require.NoError(t, err)
	comm1, err := NewCommunication(cfg1, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm1.Start(raw1))
	defer func() { _ = comm1.Stop() }()

	sk2, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw2, err := sk2.Raw()
	require.NoError(t, err)
	comm2, err := NewCommunication(&Config{Port: 19242, RendezvousString: "ensure-peers-stale-2"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm2.Start(raw2))
	defer func() { _ = comm2.Stop() }()

	pubkey2, err := conversion.GetPubKeyFromPeerID(comm2.host.ID().String())
	require.NoError(t, err)
	ma2, err := maddr.NewMultiaddr(comm2.host.Addrs()[0].String() + "/p2p/" + comm2.host.ID().String())
	require.NoError(t, err)
	info2, err := peer.AddrInfoFromP2pAddr(ma2)
	require.NoError(t, err)
	cfg1.BootstrapPeers = addrList{ma2}
	comm1.host.Peerstore().AddAddrs(info2.ID, info2.Addrs, peerstore.PermanentAddrTTL)
	require.NotEqual(t, network.Connected, comm1.host.Network().Connectedness(info2.ID))

	require.NoError(t, comm1.EnsurePeersConnected([]string{pubkey2}))
	require.Eventually(t, func() bool {
		return comm1.host.Network().Connectedness(info2.ID) == network.Connected
	}, 2*time.Second, 50*time.Millisecond)
}

func TestEnsurePeersConnectedUsesPeerstoreWithoutBootstrap(t *testing.T) {
	sk1, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw1, err := sk1.Raw()
	require.NoError(t, err)
	comm1, err := NewCommunication(&Config{Port: 19243, RendezvousString: "ensure-peers-peerstore-1"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm1.Start(raw1))
	defer func() { _ = comm1.Stop() }()

	sk2, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw2, err := sk2.Raw()
	require.NoError(t, err)
	comm2, err := NewCommunication(&Config{Port: 19244, RendezvousString: "ensure-peers-peerstore-2"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm2.Start(raw2))
	defer func() { _ = comm2.Stop() }()

	pubkey2, err := conversion.GetPubKeyFromPeerID(comm2.host.ID().String())
	require.NoError(t, err)
	ma2, err := maddr.NewMultiaddr(comm2.host.Addrs()[0].String() + "/p2p/" + comm2.host.ID().String())
	require.NoError(t, err)
	info2, err := peer.AddrInfoFromP2pAddr(ma2)
	require.NoError(t, err)
	comm1.host.Peerstore().AddAddrs(info2.ID, info2.Addrs, peerstore.PermanentAddrTTL)

	require.NoError(t, comm1.EnsurePeersConnected([]string{pubkey2}))
	require.Eventually(t, func() bool {
		return comm1.host.Network().Connectedness(info2.ID) == network.Connected
	}, 2*time.Second, 50*time.Millisecond)
}

func TestEnsurePeersConnectedWithinWaitsForDelayedPeer(t *testing.T) {
	cfg1 := &Config{Port: 19245, RendezvousString: "ensure-peers-delayed-1"}
	sk1, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw1, err := sk1.Raw()
	require.NoError(t, err)
	comm1, err := NewCommunication(cfg1, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm1.Start(raw1))
	defer func() { _ = comm1.Stop() }()

	sk2, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw2, err := sk2.Raw()
	require.NoError(t, err)
	peer2, err := peer.IDFromPrivateKey(sk2)
	require.NoError(t, err)
	pubkey2, err := conversion.GetPubKeyFromPeerID(peer2.String())
	require.NoError(t, err)
	ma2, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19246/p2p/" + peer2.String())
	require.NoError(t, err)
	cfg1.BootstrapPeers = addrList{ma2}

	comm2, err := NewCommunication(&Config{Port: 19246, RendezvousString: "ensure-peers-delayed-2"}, nil, nil)
	require.NoError(t, err)
	defer func() { _ = comm2.Stop() }()

	startErr := make(chan error, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		startErr <- comm2.Start(raw2)
	}()

	require.NoError(t, comm1.EnsurePeersConnectedWithin([]string{pubkey2}, 5*time.Second))
	require.NoError(t, <-startErr)
	require.Eventually(t, func() bool {
		return comm1.host.Network().Connectedness(peer2) == network.Connected
	}, 2*time.Second, 50*time.Millisecond)
}

func TestEnsurePeersConnectedThresholdAllowsOfflinePeer(t *testing.T) {
	cfg1 := &Config{Port: 19247, RendezvousString: "ensure-peers-threshold-1"}
	sk1, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw1, err := sk1.Raw()
	require.NoError(t, err)
	comm1, err := NewCommunication(cfg1, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm1.Start(raw1))
	defer func() { _ = comm1.Stop() }()

	sk2, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw2, err := sk2.Raw()
	require.NoError(t, err)
	comm2, err := NewCommunication(&Config{Port: 19248, RendezvousString: "ensure-peers-threshold-2"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm2.Start(raw2))
	defer func() { _ = comm2.Stop() }()

	sk3, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw3, err := sk3.Raw()
	require.NoError(t, err)
	comm3, err := NewCommunication(&Config{Port: 19249, RendezvousString: "ensure-peers-threshold-3"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm3.Start(raw3))
	defer func() { _ = comm3.Stop() }()

	sk4, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	peer4, err := peer.IDFromPrivateKey(sk4)
	require.NoError(t, err)

	pubkey1, err := conversion.GetPubKeyFromPeerID(comm1.host.ID().String())
	require.NoError(t, err)
	pubkey2, err := conversion.GetPubKeyFromPeerID(comm2.host.ID().String())
	require.NoError(t, err)
	pubkey3, err := conversion.GetPubKeyFromPeerID(comm3.host.ID().String())
	require.NoError(t, err)
	pubkey4, err := conversion.GetPubKeyFromPeerID(peer4.String())
	require.NoError(t, err)

	ma2, err := maddr.NewMultiaddr(comm2.host.Addrs()[0].String() + "/p2p/" + comm2.host.ID().String())
	require.NoError(t, err)
	ma3, err := maddr.NewMultiaddr(comm3.host.Addrs()[0].String() + "/p2p/" + comm3.host.ID().String())
	require.NoError(t, err)
	ma4, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19250/p2p/" + peer4.String())
	require.NoError(t, err)
	cfg1.BootstrapPeers = addrList{ma2, ma3, ma4}

	participants := []string{pubkey1, pubkey2, pubkey3, pubkey4}
	require.NoError(t, comm1.EnsurePeersConnectedThreshold(participants, 3))
	require.Eventually(t, func() bool {
		return comm1.host.Network().Connectedness(comm2.host.ID()) == network.Connected &&
			comm1.host.Network().Connectedness(comm3.host.ID()) == network.Connected
	}, 2*time.Second, 50*time.Millisecond)
	require.Error(t, comm1.EnsurePeersConnected(participants))
}

func TestEnsurePeersConnectedThresholdFailsBelowThreshold(t *testing.T) {
	cfg1 := &Config{Port: 19251, RendezvousString: "ensure-peers-threshold-fail-1"}
	sk1, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw1, err := sk1.Raw()
	require.NoError(t, err)
	comm1, err := NewCommunication(cfg1, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm1.Start(raw1))
	defer func() { _ = comm1.Stop() }()

	sk2, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	raw2, err := sk2.Raw()
	require.NoError(t, err)
	comm2, err := NewCommunication(&Config{Port: 19252, RendezvousString: "ensure-peers-threshold-fail-2"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, comm2.Start(raw2))
	defer func() { _ = comm2.Stop() }()

	sk3, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	peer3, err := peer.IDFromPrivateKey(sk3)
	require.NoError(t, err)
	sk4, _, err := crypto.GenerateSecp256k1Key(nil)
	require.NoError(t, err)
	peer4, err := peer.IDFromPrivateKey(sk4)
	require.NoError(t, err)

	pubkey1, err := conversion.GetPubKeyFromPeerID(comm1.host.ID().String())
	require.NoError(t, err)
	pubkey2, err := conversion.GetPubKeyFromPeerID(comm2.host.ID().String())
	require.NoError(t, err)
	pubkey3, err := conversion.GetPubKeyFromPeerID(peer3.String())
	require.NoError(t, err)
	pubkey4, err := conversion.GetPubKeyFromPeerID(peer4.String())
	require.NoError(t, err)

	ma2, err := maddr.NewMultiaddr(comm2.host.Addrs()[0].String() + "/p2p/" + comm2.host.ID().String())
	require.NoError(t, err)
	ma3, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19253/p2p/" + peer3.String())
	require.NoError(t, err)
	ma4, err := maddr.NewMultiaddr("/ip4/127.0.0.1/tcp/19254/p2p/" + peer4.String())
	require.NoError(t, err)
	cfg1.BootstrapPeers = addrList{ma2, ma3, ma4}

	err = comm1.EnsurePeersConnectedThreshold([]string{pubkey1, pubkey2, pubkey3, pubkey4}, 3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "threshold 3")
}
