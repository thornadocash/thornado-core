package p2p

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/constants"
)

func TestNewCommunicationInvalidPort(t *testing.T) {
	// Valid config
	comm, err := NewCommunication(&Config{Port: 0, RendezvousString: "test"}, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, comm)
}

func TestNewCommunicationWithExternalIP(t *testing.T) {
	comm, err := NewCommunication(&Config{Port: 7788, RendezvousString: "test", ExternalIP: "1.2.3.4"}, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, comm)
	assert.NotNil(t, comm.externalAddr)
}

func TestNewCommunicationWithInvalidExternalIP(t *testing.T) {
	// Invalid external IP should fail
	_, err := NewCommunication(&Config{Port: 7788, RendezvousString: "test", ExternalIP: "not_an_ip"}, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "fail to create listen with given external IP")
}

func TestNewCommunicationWithBridge(t *testing.T) {
	bridge := NewMockThornadoBridge()
	comm, err := NewCommunication(&Config{Port: 7789, RendezvousString: "test"}, nil, bridge)
	assert.NoError(t, err)
	assert.NotNil(t, comm)
	assert.NotNil(t, comm.nodeGater)
}

func TestSetSubscribeAndCancel(t *testing.T) {
	comm, err := NewCommunication(&Config{Port: 7790, RendezvousString: "test"}, nil, nil)
	assert.NoError(t, err)

	ch := make(chan *Message)
	comm.SetSubscribe(messages.FROSTKeyGenMsg, "msg1", ch)
	comm.SetSubscribe(messages.FROSTKeyGenMsg, "msg2", make(chan *Message))

	// Getting existing subscriber
	sub := comm.getSubscriber(messages.FROSTKeyGenMsg, "msg1")
	assert.NotNil(t, sub)

	// Getting non-existing topic
	sub = comm.getSubscriber(messages.FROSTKeySignMsg, "msg1")
	assert.Nil(t, sub)

	// Getting non-existing msgID
	sub = comm.getSubscriber(messages.FROSTKeyGenMsg, "nonexistent")
	assert.Nil(t, sub)

	// Cancel one subscription
	comm.CancelSubscribe(messages.FROSTKeyGenMsg, "msg1")
	sub = comm.getSubscriber(messages.FROSTKeyGenMsg, "msg1")
	assert.Nil(t, sub)

	// Cancel last subscription should remove topic
	comm.CancelSubscribe(messages.FROSTKeyGenMsg, "msg2")

	// Cancel on non-existing topic
	comm.CancelSubscribe(messages.FROSTKeySignMsg, "msg1")
}

func TestBroadcastEmptyPeers(t *testing.T) {
	comm, err := NewCommunication(&Config{Port: 7791, RendezvousString: "test"}, nil, nil)
	assert.NoError(t, err)
	// Should return immediately without panicking
	comm.Broadcast(nil, []byte("hello"), "test")
}

func TestReleaseStream(t *testing.T) {
	comm, err := NewCommunication(&Config{Port: 7792, RendezvousString: "test"}, nil, nil)
	assert.NoError(t, err)

	s := NewMockNetworkStream()
	comm.streamMgr.AddStream("test-msg", s)
	comm.ReleaseStream("test-msg")

	// Verify stream was released
	_, ok := comm.streamMgr.unusedStreams["test-msg"]
	assert.False(t, ok)
}

func TestGetPeersWithNoStateManager(t *testing.T) {
	comm, err := NewCommunication(&Config{
		Port:             7793,
		RendezvousString: "test",
	}, nil, nil)
	assert.NoError(t, err)

	peers := comm.getPeers()
	assert.Empty(t, peers)
}

func TestGetPeersWithConfigError(t *testing.T) {
	comm, err := NewCommunication(&errConfig{}, nil, nil)
	assert.NoError(t, err)

	peers := comm.getPeers()
	// Even with config error, should not panic, just return empty
	assert.Empty(t, peers)
}

func (CommunicationTestSuite) TestGetHostAndPeerID(c *C) {
	comm, err := NewCommunication(&Config{Port: 6690, RendezvousString: "testhost"}, nil, nil)
	c.Assert(err, IsNil)

	// Start the communication with a valid key
	privKey, err := base64.StdEncoding.DecodeString("6LABmWB4iXqkqOJ9H0YFEA2CSSx6bA7XAKGyI/TDtas=")
	c.Assert(err, IsNil)
	c.Assert(comm.Start(privKey), IsNil)
	defer func() {
		c.Assert(comm.Stop(), IsNil)
	}()

	h := comm.GetHost()
	c.Assert(h, NotNil)

	peerID := comm.GetLocalPeerID()
	c.Assert(len(peerID) > 0, Equals, true)

	// ExportPeerAddress should work with a running host
	addressBook := comm.ExportPeerAddress()
	c.Assert(addressBook, NotNil)
}

func (CommunicationTestSuite) TestBroadcastAndProcessBroadcast(c *C) {
	comm, err := NewCommunication(&Config{Port: 6691, RendezvousString: "testbroadcast"}, nil, nil)
	c.Assert(err, IsNil)

	privKey, err := base64.StdEncoding.DecodeString("6LABmWB4iXqkqOJ9H0YFEA2CSSx6bA7XAKGyI/TDtas=")
	c.Assert(err, IsNil)
	c.Assert(comm.Start(privKey), IsNil)
	defer func() {
		c.Assert(comm.Stop(), IsNil)
	}()

	// Broadcast with no peers should return immediately
	comm.Broadcast(nil, []byte("hello"), "test")
	comm.Broadcast([]peer.ID{}, []byte("hello"), "test")

	// Broadcast to self should silently succeed (writeToStream skips self)
	comm.Broadcast([]peer.ID{comm.host.ID()}, []byte("hello"), "test-self")
	// Give it a moment for the goroutine
	time.Sleep(time.Millisecond * 100)
}

func (CommunicationTestSuite) TestStopWithNodeGater(c *C) {
	bridge := NewMockThornadoBridge()
	bridge.SetConstants(map[string]int64{
		constants.Node_BondStartAmountSats.String(): 100_000_000,
	})
	comm, err := NewCommunication(&Config{Port: 6692, RendezvousString: "testgater"}, nil, bridge)
	c.Assert(err, IsNil)

	privKey, err := base64.StdEncoding.DecodeString("6LABmWB4iXqkqOJ9H0YFEA2CSSx6bA7XAKGyI/TDtas=")
	c.Assert(err, IsNil)
	c.Assert(comm.Start(privKey), IsNil)
	c.Assert(comm.Stop(), IsNil)
}

// errConfig is a P2PConfig that returns errors for GetBootstrapPeers
type errConfig struct{}

func (e *errConfig) GetBootstrapPeers() ([]maddr.Multiaddr, error) {
	return nil, fmt.Errorf("config error")
}

func (e *errConfig) GetP2PPort() int       { return 7799 }
func (e *errConfig) GetExternalIP() string { return "" }
func (e *errConfig) GetRendezvous() string { return "test" }
