package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	tcrypto "github.com/cometbft/cometbft/crypto"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-peerstore/addr"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	maddr "github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
)

var (
	joinPartyProtocol           protocol.ID = "/p2p/join-party"
	joinPartyProtocolWithLeader protocol.ID = "/p2p/join-party-leader"
)

// FROSTProtocolID protocol id used for frost
var (
	FROSTProtocolID      protocol.ID = "/p2p/frost"
	ObservedTxProtocolID protocol.ID = "/p2p/observed-tx"
)

const (
	// TimeoutConnecting maximum time for wait for peers to connect
	TimeoutConnecting = time.Second * 20

	StreamUnknown = "UNKNOWN"
	StreamMsgDone = "done"
)

// Message that get transfer across the wire
type Message struct {
	PeerID  peer.ID
	Payload []byte
}

// Communication use p2p to broadcast messages among all the FROST nodes
type Communication struct {
	config           P2PConfig
	stateManager     *storage.FileStateMgr
	logger           zerolog.Logger
	listenAddr       maddr.Multiaddr
	host             host.Host
	wg               *sync.WaitGroup
	stopChan         chan struct{} // channel to indicate whether we should stop
	subscribers      map[messages.ThornadoFROSTMessageType]*MessageIDSubscriber
	subscriberLocker *sync.Mutex
	streamCount      int64
	BroadcastMsgChan chan *messages.BroadcastMsgChan
	externalAddr     maddr.Multiaddr
	streamMgr        *StreamMgr
	nodeGater        *NodeGater // gater to restrict connections to allowed nodes only
}

type P2PConfig interface {
	GetBootstrapPeers() ([]maddr.Multiaddr, error)
	GetP2PPort() int
	GetExternalIP() string
	GetRendezvous() string
}

func StartP2P(
	cfg P2PConfig,
	priKey tcrypto.PrivKey,
	baseFolder string,
) (*Communication, *storage.FileStateMgr, error) {
	return StartP2PWithBridge(cfg, priKey, baseFolder, nil)
}

// StartP2PWithBridge starts the P2P communication layer with optional node gating.
// If bridge is provided, only active or ready nodes will be allowed for inbound connections.
func StartP2PWithBridge(
	cfg P2PConfig,
	priKey tcrypto.PrivKey,
	baseFolder string,
	bridge thornadoclient.ThornadoBridge,
) (*Communication, *storage.FileStateMgr, error) {
	stateManager, err := storage.NewFileStateMgr(baseFolder)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to create file state manager")
	}

	comm, err := NewCommunication(cfg, stateManager, bridge)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to create communication layer: %w", err)
	}

	priKeyRawBytes, err := conversion.GetPriKeyRawBytes(priKey)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to get private key")
	}
	if err := comm.Start(priKeyRawBytes); nil != err {
		return nil, nil, fmt.Errorf("fail to start p2p network: %w", err)
	}

	return comm, stateManager, nil
}

// NewCommunication create a new instance of Communication
func NewCommunication(cfg P2PConfig, stateManager *storage.FileStateMgr, bridge thornadoclient.ThornadoBridge) (*Communication, error) {
	port, externalIP := cfg.GetP2PPort(), cfg.GetExternalIP()
	addr, err := maddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port))
	if err != nil {
		return nil, fmt.Errorf("fail to create listen addr: %w", err)
	}
	var externalAddr maddr.Multiaddr = nil
	if len(externalIP) != 0 {
		externalAddr, err = maddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", externalIP, port))
		if err != nil {
			return nil, fmt.Errorf("fail to create listen with given external IP: %w", err)
		}
	}

	comm := &Communication{
		config:           cfg,
		stateManager:     stateManager,
		logger:           log.With().Str("module", "communication").Logger(),
		listenAddr:       addr,
		wg:               &sync.WaitGroup{},
		stopChan:         make(chan struct{}),
		subscribers:      make(map[messages.ThornadoFROSTMessageType]*MessageIDSubscriber),
		subscriberLocker: &sync.Mutex{},
		streamCount:      0,
		BroadcastMsgChan: make(chan *messages.BroadcastMsgChan, 1024),
		externalAddr:     externalAddr,
		streamMgr:        NewStreamMgr(),
	}

	// If bridge is provided, create the node gater
	if bridge != nil {
		// Create gater with 60 second refresh interval
		gater := NewNodeGater(bridge, 60*time.Second)
		comm.nodeGater = gater
		comm.logger.Info().Msg("node gater enabled - only allowed nodes can connect")
	}

	return comm, nil
}

// GetHost return the host
func (c *Communication) GetHost() host.Host {
	return c.host
}

// GetLocalPeerID from p2p host
func (c *Communication) GetLocalPeerID() string {
	return c.host.ID().String()
}

// Broadcast message to Peers
func (c *Communication) Broadcast(peers []peer.ID, msg []byte, msgID string) {
	if len(peers) == 0 {
		return
	}
	// try to discover all peers and then broadcast the messages
	c.wg.Add(1)
	go c.broadcastToPeers(peers, msg, msgID)
}

// BroadcastSync delivers a FROST session message and blocks until all peer
// writes finish or fail.
func (c *Communication) BroadcastSync(peers []peer.ID, msg []byte, msgID string) {
	if len(peers) == 0 {
		return
	}
	c.wg.Add(1)
	c.broadcastToPeers(peers, msg, msgID)
}

func (c *Communication) broadcastToPeers(peers []peer.ID, msg []byte, msgID string) {
	defer c.wg.Done()
	defer func() {
		c.logger.Debug().Msgf("finished sending message to peer(%v)", peers)
	}()
	var wgSend sync.WaitGroup
	wgSend.Add(len(peers))
	for _, p := range peers {
		go func(p peer.ID) {
			defer wgSend.Done()
			if err := c.writeToStream(p, msg, msgID); nil != err {
				c.logger.Error().Err(err).Msg("fail to write to stream")
			}
		}(p)
	}
	wgSend.Wait()
}

func (c *Communication) writeToStream(pID peer.ID, msg []byte, msgID string) error {
	// don't send to ourselves
	if pID == c.host.ID() {
		return nil
	}
	stream, err := c.connectToOnePeer(pID)
	if err != nil {
		return fmt.Errorf("fail to open stream to peer(%s): %w", pID, err)
	}
	if nil == stream {
		return nil
	}

	defer func() {
		c.streamMgr.AddStream(msgID, stream)
	}()
	c.logger.Debug().Msgf(">>>writing messages to peer(%s)", pID)

	return WriteStreamWithBuffer(msg, stream)
}

func (c *Communication) handleStreamFrost(stream network.Stream) {
	peerID := stream.Conn().RemotePeer().String()
	c.logger.Debug().Msgf("reading from frost stream of peer: %s", peerID)

	select {
	case <-c.stopChan:
		return
	default:
		dataBuf, err := ReadStreamWithBuffer(stream)
		if err != nil {
			c.logger.Error().Err(err).Msgf("fail to read from stream,peerID: %s", peerID)
			c.streamMgr.AddStream(StreamUnknown, stream)
			return
		}
		var wrappedMsg messages.WrappedMessage
		if err := json.Unmarshal(dataBuf, &wrappedMsg); nil != err {
			c.logger.Error().Err(err).Msg("fail to unmarshal wrapped message bytes")
			c.streamMgr.AddStream(StreamUnknown, stream)
			return
		}
		var payloadMeta struct {
			Kind string `json:"kind"`
			From string `json:"from"`
		}
		_ = json.Unmarshal(wrappedMsg.Payload, &payloadMeta)
		c.logger.Debug().
			Str("message_id", wrappedMsg.MsgID).
			Str("message_type", wrappedMsg.MessageType.String()).
			Str("kind", payloadMeta.Kind).
			Str("from", payloadMeta.From).
			Msg("received frost message")
		c.streamMgr.AddStream(wrappedMsg.MsgID, stream)
		channel := c.getSubscriber(wrappedMsg.MessageType, wrappedMsg.MsgID)
		if nil == channel {
			c.logger.Debug().
				Str("message_id", wrappedMsg.MsgID).
				Str("message_type", wrappedMsg.MessageType.String()).
				Str("kind", payloadMeta.Kind).
				Msg("no subscriber for frost message")
			return
		}
		channel <- &Message{
			PeerID:  stream.Conn().RemotePeer(),
			Payload: dataBuf,
		}

	}
}

func (c *Communication) getPeers() addr.AddrList {
	var bootstrapPeers addr.AddrList

	cfgBoostrapPeers, err := c.config.GetBootstrapPeers()
	if err != nil {
		c.logger.Error().Err(err).Msg("fail to get bootstrap peers from config")
	} else {
		bootstrapPeers = cfgBoostrapPeers
	}

	if c.stateManager != nil {
		savedPeers, err := c.stateManager.RetrieveP2PAddresses()
		if err != nil {
			if os.IsNotExist(err) {
				c.logger.Trace().Msg("no saved peers in state manager yet")
			} else {
				c.logger.Error().Err(err).Msg("fail to get saved peers from state manager")
			}
		} else {
			bootstrapPeers = append(bootstrapPeers, savedPeers...)
		}
	}

	return bootstrapPeers
}

func (c *Communication) bootStrapConnectivityCheck() error {
	bootstrapPeers := c.getPeers()

	if len(bootstrapPeers) == 0 {
		c.logger.Error().Msg("we do not have the bootstrap node set, quit the connectivity check")
		return nil
	}

	var onlineNodes uint32
	var wg sync.WaitGroup
	for _, el := range bootstrapPeers {
		peer, err := peer.AddrInfoFromP2pAddr(el)
		if err != nil {
			c.logger.Error().Err(err).Msg("error in decode the bootstrap node, skip it")
			continue
		}
		wg.Add(1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
			defer cancel()
			defer wg.Done()
			outChan := ping.Ping(ctx, c.host, peer.ID)
			select {
			case ret, ok := <-outChan:
				if !ok {
					return
				}
				if ret.Error == nil {
					c.logger.Debug().Msgf("connect to peer %v with RTT %v\n", peer.ID, ret.RTT)
					atomic.AddUint32(&onlineNodes, 1)
				}
			case <-ctx.Done():
				c.logger.Error().Msgf("fail to ping the node %s within 2 seconds", peer.ID)
			}
		}()
	}
	wg.Wait()

	if onlineNodes > 0 {
		c.logger.Info().Msgf("we have successfully ping pong %d nodes", onlineNodes)
		return nil
	}
	for _, el := range bootstrapPeers {
		peerInfo, err := peer.AddrInfoFromP2pAddr(el)
		if err == nil && peerInfo.ID == c.host.ID() {
			c.logger.Info().Msg("bootstrap list includes self; skipping remote bootstrap ping check")
			return nil
		}
	}
	c.logger.Error().Msg("fail to ping any bootstrap node")
	return errors.New("the node cannot ping any bootstrap node")
}

func (c *Communication) startChannel(privKeyBytes []byte) error {
	ctx := context.Background()
	p2pPriKey, err := crypto.UnmarshalSecp256k1PrivateKey(privKeyBytes)
	if err != nil {
		c.logger.Error().Msgf("error is %f", err)
		return err
	}

	addressFactory := func(addrs []maddr.Multiaddr) []maddr.Multiaddr {
		if c.externalAddr != nil {
			return []maddr.Multiaddr{c.externalAddr}
		}
		return addrs
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.ListenAddrs([]maddr.Multiaddr{c.listenAddr}...),
		libp2p.Identity(p2pPriKey),
		libp2p.AddrsFactory(addressFactory),
	}

	// Add connection gater if configured
	if c.nodeGater != nil {
		c.logger.Info().Msg("configuring libp2p with node gater")
		opts = append(opts, libp2p.ConnectionGater(c.nodeGater))
	}

	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("fail to create p2p host: %w", err)
	}
	c.host = h
	c.logger.Info().Msgf("Host created, we are: %s, at: %s", h.ID(), h.Addrs())
	h.SetStreamHandler(FROSTProtocolID, c.handleStreamFrost)

	var connectionErr error
	for i := 0; i < 5; i++ {
		connectionErr = c.connectToBootstrapPeers()
		if connectionErr == nil {
			break
		}
		c.logger.Info().
			Err(connectionErr).
			Int("attempt", i+1).
			Msg("bootstrap peers not reachable yet; retrying")
		time.Sleep(time.Second * 5)
	}
	if connectionErr != nil {
		return fmt.Errorf("fail to connect to bootstrap peer: %w", connectionErr)
	}

	err = c.bootStrapConnectivityCheck()
	if err != nil {
		return err
	}

	c.logger.Info().Msg("Successfully connected to bootstrap peers")
	return nil
}

func (c *Communication) connectToOnePeer(pID peer.ID) (network.Stream, error) {
	c.logger.Debug().Msgf("peer:%s,current:%s", pID, c.host.ID())
	// dont connect to itself
	if pID == c.host.ID() {
		return nil, nil
	}
	c.logger.Debug().Msgf("connect to peer : %s", pID.String())
	ctx, cancel := context.WithTimeout(context.Background(), TimeoutConnecting)
	defer cancel()
	stream, err := c.host.NewStream(ctx, pID, FROSTProtocolID)
	if err != nil {
		return nil, fmt.Errorf("fail to create new stream to peer: %s, %w", pID, err)
	}
	return stream, nil
}

func (c *Communication) connectToBootstrapPeers() error {
	bootstrapPeers := c.getPeers()
	// Let's connect to the bootstrap nodes first. They will tell us about the
	// other nodes in the network.
	if len(bootstrapPeers) == 0 {
		c.logger.Info().Msg("no bootstrap node set, we skip the connection")
		return nil
	}
	var wg sync.WaitGroup
	selfBootstrap := false
	remoteBootstrap := 0
	connRet := make(chan bool, len(bootstrapPeers))
	for _, peerAddr := range bootstrapPeers {
		pi, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			return fmt.Errorf("fail to add peer: %w", err)
		}
		if pi.ID == c.host.ID() {
			selfBootstrap = true
			c.logger.Debug().Msgf("skipping connection to self: %s", pi.ID)
			continue
		}
		remoteBootstrap++
		wg.Add(1)
		go func(pi *peer.AddrInfo) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), TimeoutConnecting)
			defer cancel()
			if err := c.host.Connect(ctx, *pi); err != nil {
				c.logger.Debug().Err(err).Msgf("bootstrap peer not reachable yet: %s", pi.String())
				connRet <- false
				return
			}
			connRet <- true
			c.logger.Info().Msgf("Connection established with bootstrap node: %s", *pi)
		}(pi)
	}
	wg.Wait()
	connected := 0
	for i := 0; i < remoteBootstrap; i++ {
		if <-connRet {
			connected++
		}
	}
	if connected == 0 {
		if selfBootstrap || remoteBootstrap == 0 {
			c.logger.Info().
				Bool("self_bootstrap", selfBootstrap).
				Int("remote_bootstrap", remoteBootstrap).
				Msg("continuing without remote bootstrap connections")
			return nil
		}
		return errors.New("fail to connect to any peer")
	}
	c.logger.Info().Int("connected", connected).Int("bootstrap_peers", len(bootstrapPeers)).Msg("connected bootstrap peers")
	return nil
}

// Start will start the communication
func (c *Communication) Start(priKeyBytes []byte) error {
	// Start the node gater if configured
	if c.nodeGater != nil {
		c.nodeGater.Start()
	}

	err := c.startChannel(priKeyBytes)
	if err == nil {
		c.wg.Add(1)
		go c.ProcessBroadcast()
	} else if c.nodeGater != nil {
		c.nodeGater.Stop()
	}
	return err
}

// Stop communication
func (c *Communication) Stop() error {
	// Stop the node gater if configured
	if c.nodeGater != nil {
		c.nodeGater.Stop()
	}

	// we need to stop the handler and the p2p services firstly, then terminate the our communication threads
	if err := c.host.Close(); err != nil {
		c.logger.Err(err).Msg("fail to close host network")
	}

	close(c.stopChan)
	c.wg.Wait()
	return nil
}

func (c *Communication) SetSubscribe(topic messages.ThornadoFROSTMessageType, msgID string, channel chan *Message) {
	c.subscriberLocker.Lock()
	defer c.subscriberLocker.Unlock()

	messageIDSubscribers, ok := c.subscribers[topic]
	if !ok {
		messageIDSubscribers = NewMessageIDSubscriber()
		c.subscribers[topic] = messageIDSubscribers
	}
	messageIDSubscribers.Subscribe(msgID, channel)
}

func (c *Communication) getSubscriber(topic messages.ThornadoFROSTMessageType, msgID string) chan *Message {
	c.subscriberLocker.Lock()
	defer c.subscriberLocker.Unlock()
	messageIDSubscribers, ok := c.subscribers[topic]
	if !ok {
		c.logger.Debug().Msgf("fail to find subscribers for %s", topic)
		return nil
	}
	return messageIDSubscribers.GetSubscriber(msgID)
}

func (c *Communication) CancelSubscribe(topic messages.ThornadoFROSTMessageType, msgID string) {
	c.subscriberLocker.Lock()
	defer c.subscriberLocker.Unlock()

	messageIDSubscribers, ok := c.subscribers[topic]
	if !ok {
		c.logger.Debug().Msgf("cannot find the given channels %s", topic.String())
		return
	}
	if nil == messageIDSubscribers {
		return
	}
	messageIDSubscribers.UnSubscribe(msgID)
	if messageIDSubscribers.IsEmpty() {
		delete(c.subscribers, topic)
	}
}

func (c *Communication) ProcessBroadcast() {
	c.logger.Debug().Msg("start to process broadcast message channel")
	defer c.logger.Debug().Msg("stop process broadcast message channel")
	defer c.wg.Done()
	for {
		select {
		case msg := <-c.BroadcastMsgChan:
			wrappedMsgBytes, err := json.Marshal(msg.WrappedMessage)
			if err != nil {
				c.logger.Error().Err(err).Msg("fail to marshal a wrapped message to json bytes")
				continue
			}
			c.logger.Debug().Msgf("broadcast message %s to %+v", msg.WrappedMessage, msg.PeersID)
			c.Broadcast(msg.PeersID, wrappedMsgBytes, msg.WrappedMessage.MsgID)

		case <-c.stopChan:
			return
		}
	}
}

func (c *Communication) ReleaseStream(msgID string) {
	c.streamMgr.ReleaseStream(msgID)
}
