package observer

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/ebifrost"
	stypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

// Mock implementations of external dependencies
type MockHost struct {
	host.Host
	peers         []peer.ID
	streamHandler map[protocol.ID]network.StreamHandler
	streamFunc    func(context.Context, peer.ID, ...protocol.ID) (network.Stream, error)
}

func NewMockHost(peers []peer.ID) *MockHost {
	return &MockHost{
		peers:         peers,
		streamHandler: make(map[protocol.ID]network.StreamHandler),
		streamFunc: func(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
			return &MockStream{mu: new(sync.Mutex)}, nil
		},
	}
}

func (m *MockHost) ID() peer.ID {
	return m.peers[0]
}

func (m *MockHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	return m.streamFunc(ctx, p, pids...)
}

func (m *MockHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	m.streamHandler[pid] = handler
}

func (m *MockHost) Peerstore() peerstore.Peerstore {
	return &MockPeerstore{peers: m.peers}
}

type MockPeerstore struct {
	peers []peer.ID
}

func (m *MockPeerstore) Peers() peer.IDSlice {
	return m.peers
}

func (m *MockPeerstore) PeerInfo(peer.ID) peer.AddrInfo {
	return peer.AddrInfo{}
}

func (m *MockPeerstore) AddAddr(p peer.ID, addr multiaddr.Multiaddr, ttl time.Duration) {
	// Mock implementation, do nothing
}

func (m *MockPeerstore) AddAddrs(id peer.ID, addrs []multiaddr.Multiaddr, ttl time.Duration) {
	// Mock implementation, do nothing
}

func (m *MockPeerstore) AddPrivKey(peer.ID, ic.PrivKey) error {
	// Mock implementation, do nothing
	return nil
}

func (m *MockPeerstore) AddProtocols(peer.ID, ...protocol.ID) error {
	// Mock implementation, do nothing
	return nil
}

func (m *MockPeerstore) AddPubKey(peer.ID, ic.PubKey) error {
	// Mock implementation, do nothing
	return nil
}

func (m *MockPeerstore) AddrStream(context.Context, peer.ID) <-chan multiaddr.Multiaddr {
	return nil
}

func (m *MockPeerstore) Addrs(p peer.ID) []multiaddr.Multiaddr {
	return nil
}
func (m *MockPeerstore) ClearAddrs(p peer.ID) {}
func (m *MockPeerstore) Close() error {
	return nil
}

func (m *MockPeerstore) FirstSupportedProtocol(peer.ID, ...protocol.ID) (protocol.ID, error) {
	return "", nil
}

func (m *MockPeerstore) Get(p peer.ID, key string) (interface{}, error) {
	return nil, nil
}

func (m *MockPeerstore) GetProtocols(peer.ID) ([]protocol.ID, error) {
	return nil, nil
}

func (m *MockPeerstore) LatencyEWMA(peer.ID) time.Duration {
	return 0
}

func (m *MockPeerstore) PeersWithAddrs() peer.IDSlice {
	return m.peers
}

func (m *MockPeerstore) PeersWithKeys() peer.IDSlice {
	return m.peers
}

func (m *MockPeerstore) PrivKey(peer.ID) ic.PrivKey {
	return nil
}

func (m *MockPeerstore) PubKey(peer.ID) ic.PubKey {
	return nil
}

func (m *MockPeerstore) Put(p peer.ID, key string, val interface{}) error {
	return nil
}

func (m *MockPeerstore) RecordLatency(peer.ID, time.Duration) {
	// Mock implementation, do nothing
}

func (m *MockPeerstore) RemovePeer(peer.ID) {}

func (m *MockPeerstore) RemoveProtocols(peer.ID, ...protocol.ID) error {
	return nil
}

func (m *MockPeerstore) SetAddr(p peer.ID, addr multiaddr.Multiaddr, ttl time.Duration) {
	// Mock implementation, do nothing
}

func (m *MockPeerstore) SetAddrs(p peer.ID, addrs []multiaddr.Multiaddr, ttl time.Duration) {
	// Mock implementation, do nothing
}

func (m *MockPeerstore) SetProtocols(peer.ID, ...protocol.ID) error {
	return nil
}

func (m *MockPeerstore) SupportsProtocols(peer.ID, ...protocol.ID) ([]protocol.ID, error) {
	return nil, nil
}

func (m *MockPeerstore) UpdateAddrs(p peer.ID, oldTTL, newTTL time.Duration) {
	// Mock implementation, do nothing
}

type MockKeys struct {
	thornadoclient.Keys
	privKey *secp256k1.PrivKey
}

func (m *MockKeys) GetPrivateKey() (*secp256k1.PrivKey, error) {
	return m.privKey, nil
}

type MockPrivKey struct {
	cryptotypes.PrivKey
	signFunc func(msg []byte) ([]byte, error)
}

func (m *MockPrivKey) Sign(msg []byte) ([]byte, error) {
	if m.signFunc != nil {
		return m.signFunc(msg)
	}
	return m.PrivKey.Sign(msg)
}

type MockGRPCClient struct {
	ebifrost.LocalhostBifrostClient
	sendQuorumTxFunc         func(ctx context.Context, quorumTx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error)
	sendQuorumNetworkFeeFunc func(ctx context.Context, quorumNetworkFee *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error)
	sendQuorumSolvencyFunc   func(ctx context.Context, quorumSolvency *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error)
	sendQuorumErrataFunc     func(ctx context.Context, quorumErrata *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error)
}

func (m *MockGRPCClient) SendQuorumTx(ctx context.Context, tx *common.QuorumTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumTxResult, error) {
	return m.sendQuorumTxFunc(ctx, tx, opts...)
}

func (m *MockGRPCClient) SendQuorumNetworkFee(ctx context.Context, quorumNetworkFee *common.QuorumNetworkFee, opts ...grpc.CallOption) (*ebifrost.SendQuorumNetworkFeeResult, error) {
	return m.sendQuorumNetworkFeeFunc(ctx, quorumNetworkFee, opts...)
}

func (m *MockGRPCClient) SendQuorumSolvency(ctx context.Context, quorumSolvency *common.QuorumSolvency, opts ...grpc.CallOption) (*ebifrost.SendQuorumSolvencyResult, error) {
	return m.sendQuorumSolvencyFunc(ctx, quorumSolvency, opts...)
}

func (m *MockGRPCClient) SendQuorumErrataTx(ctx context.Context, quorumErrata *common.QuorumErrataTx, opts ...grpc.CallOption) (*ebifrost.SendQuorumErrataTxResult, error) {
	return m.sendQuorumErrataFunc(ctx, quorumErrata, opts...)
}

type MockThornadoBridge struct {
	thornadoclient.ThornadoBridge
	getKeysignPartyFunc func(pubKey common.PubKey) (common.PubKeys, error)
}

func (m *MockThornadoBridge) GetKeysignParty(pubKey common.PubKey) (common.PubKeys, error) {
	return m.getKeysignPartyFunc(pubKey)
}

func (m *MockThornadoBridge) GetConfigValue(key string) (int64, error) {
	return 0, nil
}

type MockStream struct {
	network.Stream
	reader io.Reader
	writer io.Writer
	peer   peer.ID

	mu *sync.Mutex
}

func NewMockStream(reader io.Reader, writer io.Writer, peer peer.ID) *MockStream {
	mu := &sync.Mutex{}
	return &MockStream{
		reader: reader,
		writer: writer,
		peer:   peer,
		mu:     mu,
	}
}

func (m *MockStream) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Perform the read
	for range 5 {
		n, err = m.reader.Read(p)
		if err == nil {
			break
		}
		if err != io.EOF {
			return n, err
		}
		m.mu.Unlock()
		<-time.After(100 * time.Millisecond)
		m.mu.Lock()
	}

	return n, err
}

func (m *MockStream) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writer.Write(p)
}

func (m *MockStream) Close() error {
	return nil
}

func (m *MockStream) Conn() network.Conn {
	return &MockConn{peer: m.peer}
}

func (m *MockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *MockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockStream) Reset() error {
	return nil
}

type MockConn struct {
	network.Conn
	peer peer.ID
}

func (m *MockConn) RemotePeer() peer.ID {
	return m.peer
}

type MockEventClient struct {
	handlers map[string]func(*ebifrost.EventNotification)
}

func (m *MockEventClient) RegisterHandler(event string, handler func(*ebifrost.EventNotification)) {
	if m.handlers == nil {
		m.handlers = make(map[string]func(*ebifrost.EventNotification))
	}
	m.handlers[event] = handler
}

func (m *MockEventClient) Start() {}
func (m *MockEventClient) Stop()  {}

func NewMockEventClient() *MockEventClient {
	return &MockEventClient{
		handlers: make(map[string]func(*ebifrost.EventNotification)),
	}
}

// MockThornadoBridge2 implements a broader set of ThornadoBridge methods for observer tests
type MockThornadoBridge2 struct {
	thornadoclient.ThornadoBridge
	nodeStatus    stypes.NodeStatus
	activeNodes   common.PubKeys
	getConfigFunc func(key string) (int64, error)
}

func (m *MockThornadoBridge2) FetchNodeStatus() (stypes.NodeStatus, error) {
	return m.nodeStatus, nil
}

func (m *MockThornadoBridge2) FetchActiveNodes() ([]common.PubKey, error) {
	return m.activeNodes, nil
}

func (m *MockThornadoBridge2) GetConfigValue(key string) (int64, error) {
	if m.getConfigFunc != nil {
		return m.getConfigFunc(key)
	}
	return 0, nil
}

func (m *MockThornadoBridge2) GetKeysignParty(pubKey common.PubKey) (common.PubKeys, error) {
	return nil, nil
}

// Define a custom pipe that implements the stream interface
type streamPair struct {
	clientToServer *bytes.Buffer
	serverToClient *bytes.Buffer
	peer           peer.ID
}
