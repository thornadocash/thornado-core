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
