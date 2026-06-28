package p2p

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto" // nolint: staticcheck
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

var (
	ErrJoinPartyTimeout = errors.New("fail to join party, timeout")
	ErrLeaderNotReady   = errors.New("leader not reachable")
	ErrPartyClosed      = errors.New("party closed")
	ErrSignReceived     = errors.New("signature received")
	ErrNotActiveSigner  = errors.New("not active signer")
	ErrSigGenerated     = errors.New("signature generated")
)

type closedPartyResponse struct {
	response  *messages.JoinPartyLeaderComm
	expiresAt time.Time
}

type PartyCoordinator struct {
	logger             zerolog.Logger
	host               host.Host
	stopChan           chan struct{}
	timeout            time.Duration
	peersGroup         map[string]*peerStatus
	closedGroups       map[string]closedPartyResponse
	joinPartyGroupLock *sync.Mutex
	streamMgr          *StreamMgr
}

// NewPartyCoordinator create a new instance of PartyCoordinator
func NewPartyCoordinator(host host.Host, timeout time.Duration) *PartyCoordinator {
	// if no timeout is given, default to 10 seconds
	if timeout.Nanoseconds() == 0 {
		timeout = 10 * time.Second
	}
	pc := &PartyCoordinator{
		logger:             log.With().Str("module", "party_coordinator").Logger(),
		host:               host,
		stopChan:           make(chan struct{}),
		timeout:            timeout,
		peersGroup:         make(map[string]*peerStatus),
		closedGroups:       make(map[string]closedPartyResponse),
		joinPartyGroupLock: &sync.Mutex{},
		streamMgr:          NewStreamMgr(),
	}
	host.SetStreamHandler(joinPartyProtocol, pc.HandleStream)
	host.SetStreamHandler(joinPartyProtocolWithLeader, pc.HandleStreamWithLeader)
	return pc
}

// Stop the PartyCoordinator rune
func (pc *PartyCoordinator) Stop() {
	defer pc.logger.Info().Msg("stopping party coordinator")
	pc.host.RemoveStreamHandler(joinPartyProtocol)
	pc.host.RemoveStreamHandler(joinPartyProtocolWithLeader)
	close(pc.stopChan)
}

func (pc *PartyCoordinator) processRespMsg(respMsg *messages.JoinPartyLeaderComm, stream network.Stream) {
	pc.streamMgr.AddStream(respMsg.ID, stream)

	remotePeer := stream.Conn().RemotePeer()
	pc.joinPartyGroupLock.Lock()
	peerGroup, ok := pc.peersGroup[respMsg.ID]
	pc.joinPartyGroupLock.Unlock()
	if !ok {
		pc.logger.Debug().Msgf("message ID from peer(%s) can not be found", remotePeer)
		return
	}
	if remotePeer == peerGroup.getLeader() {
		peerGroup.setLeaderResponse(respMsg)
		peerGroup.notify <- true
		err := WriteStreamWithBuffer([]byte(StreamMsgDone), stream)
		if err != nil {
			pc.logger.Error().Err(err).Msgf("fail to write the reply to peer: %s", remotePeer)
			return
		}
	} else {
		pc.logger.Info().Msgf("this party(%s) is not the leader(%s) as expected", remotePeer, peerGroup.getLeader())
	}
}

func (pc *PartyCoordinator) processReqMsg(requestMsg *messages.JoinPartyLeaderComm, stream network.Stream) {
	pc.streamMgr.AddStream(requestMsg.ID, stream)
	pc.joinPartyGroupLock.Lock()
	peerGroup, ok := pc.peersGroup[requestMsg.ID]
	closedResp, closed := pc.closedGroupResponseLocked(requestMsg.ID)
	pc.joinPartyGroupLock.Unlock()
	if !ok {
		if closed {
			remotePeer := stream.Conn().RemotePeer()
			pc.logger.Trace().
				Str("msg_id", requestMsg.ID).
				Str("remote_peer", remotePeer.String()).
				Msg("join party already closed")
			pc.sendResponseToPeer(closedResp, remotePeer)
			return
		}
		pc.logger.Debug().Msg("this party is not ready")
		return
	}
	remotePeer := stream.Conn().RemotePeer()
	if peerGroup.hasSelectedThreshold() {
		onlinePeers, _ := peerGroup.getPeersStatus()
		selected := append(onlinePeers, pc.host.ID())
		if peerIDListContains(selected, remotePeer) {
			pc.sendResponseToPeer(pc.successJoinPartyResponse(requestMsg.ID, selected), remotePeer)
		} else {
			pc.sendResponseToPeer(&messages.JoinPartyLeaderComm{
				ID:      requestMsg.ID,
				MsgType: "response",
				Type:    messages.JoinPartyLeaderComm_LeaderNotReady,
			}, remotePeer)
		}
		return
	}
	partyFormed, err := peerGroup.updatePeer(remotePeer)
	if err != nil {
		pc.logger.Error().Err(err).Msg("receive msg from unknown peer")
		return
	}
	if partyFormed {
		peerGroup.notify <- true
	}
}

func peerIDListContains(peers []peer.ID, target peer.ID) bool {
	for _, p := range peers {
		if p == target {
			return true
		}
	}
	return false
}

func (pc *PartyCoordinator) successJoinPartyResponse(msgID string, onlinePeers []peer.ID) *messages.JoinPartyLeaderComm {
	frostNodes := make([]string, 0, len(onlinePeers))
	for _, el := range onlinePeers {
		pubKey, err := conversion.GetPubKeyFromPeerID(el.String())
		if err != nil {
			pc.logger.Error().Err(err).Str("peer_id", el.String()).Msg("fail to convert online peer to pubkey")
			continue
		}
		frostNodes = append(frostNodes, pubKey)
	}
	return &messages.JoinPartyLeaderComm{
		ID:      msgID,
		Type:    messages.JoinPartyLeaderComm_Success,
		PeerIDs: frostNodes,
	}
}

func (pc *PartyCoordinator) HandleStream(stream network.Stream) {
	remotePeer := stream.Conn().RemotePeer()
	logger := pc.logger.With().Str("remote peer", remotePeer.String()).Logger()
	logger.Debug().Msg("reading from join party request")
	payload, err := ReadStreamWithBuffer(stream)
	if err != nil {
		logger.Err(err).Msgf("fail to read payload from stream")
		pc.streamMgr.AddStream(StreamUnknown, stream)
		return
	}
	var msg messages.JoinPartyRequest
	if err = proto.Unmarshal(payload, &msg); err != nil {
		logger.Err(err).Msg("fail to unmarshal join party request")
		pc.streamMgr.AddStream(StreamUnknown, stream)
		return
	}
	pc.streamMgr.AddStream(msg.ID, stream)
	pc.joinPartyGroupLock.Lock()
	peerGroup, ok := pc.peersGroup[msg.ID]
	pc.joinPartyGroupLock.Unlock()
	if !ok {
		pc.logger.Debug().Msg("this party is not ready")
		return
	}
	_, err = peerGroup.updatePeer(remotePeer)
	if err != nil {
		pc.logger.Error().Err(err).Msg("receive msg from unknown peer")
		return
	}
}

// HandleStream handle party coordinate stream
func (pc *PartyCoordinator) HandleStreamWithLeader(stream network.Stream) {
	remotePeer := stream.Conn().RemotePeer()
	logger := pc.logger.With().Str("remote peer", remotePeer.String()).Logger()
	logger.Debug().Msg("reading from join party request")
	payload, err := ReadStreamWithBuffer(stream)
	if err != nil {
		logger.Err(err).Msgf("fail to read payload from stream")
		pc.streamMgr.AddStream(StreamUnknown, stream)
		return
	}

	var msg messages.JoinPartyLeaderComm
	err = proto.Unmarshal(payload, &msg)
	if err != nil {
		logger.Err(err).Msg("fail to unmarshal party data")
		pc.streamMgr.AddStream(StreamUnknown, stream)
		return
	}

	pc.logger.Debug().Msgf("received message type=%s", msg.MsgType)

	switch msg.MsgType {
	case "request":
		pc.processReqMsg(&msg, stream)
		return
	case "response":
		pc.processRespMsg(&msg, stream)
		err = WriteStreamWithBuffer([]byte(StreamMsgDone), stream)
		if err != nil {
			pc.logger.Error().Err(err).Msgf("fail to send response to leader")
		}
		return
	default:
		logger.Err(err).Msg("fail to process this message")
		pc.streamMgr.AddStream(StreamUnknown, stream)
		return
	}
}

func (pc *PartyCoordinator) releaseJoinPartyGroup(messageID string) {
	pc.joinPartyGroupLock.Lock()
	defer pc.joinPartyGroupLock.Unlock()
	peerGroup, ok := pc.peersGroup[messageID]
	if !ok {
		return
	}
	peerGroup.peerStatusLock.Lock()
	peerGroup.joiners--
	remaining := peerGroup.joiners
	peerGroup.peerStatusLock.Unlock()
	if remaining <= 0 {
		delete(pc.peersGroup, messageID)
	}
}

func cloneJoinPartyLeaderComm(msg *messages.JoinPartyLeaderComm) *messages.JoinPartyLeaderComm {
	if msg == nil {
		return nil
	}
	cp := *msg
	cp.PeerIDs = append([]string(nil), msg.PeerIDs...)
	return &cp
}

func (pc *PartyCoordinator) rememberClosedGroup(messageID string) {
	pc.joinPartyGroupLock.Lock()
	defer pc.joinPartyGroupLock.Unlock()
	pc.closedGroups[messageID] = closedPartyResponse{
		response: &messages.JoinPartyLeaderComm{
			ID:      messageID,
			MsgType: "response",
			Type:    messages.JoinPartyLeaderComm_LeaderNotReady,
		},
		// Keep this long enough to answer slow joiners from the completed attempt,
		// but short enough that a later signer retry can reform the party.
		expiresAt: time.Now().Add(pc.timeout),
	}
}

func (pc *PartyCoordinator) closedGroupResponseLocked(messageID string) (*messages.JoinPartyLeaderComm, bool) {
	closed, ok := pc.closedGroups[messageID]
	if !ok {
		return nil, false
	}
	if time.Now().After(closed.expiresAt) {
		delete(pc.closedGroups, messageID)
		return nil, false
	}
	return cloneJoinPartyLeaderComm(closed.response), true
}

func (pc *PartyCoordinator) acquireJoinPartyGroup(messageID string, leaderID peer.ID, peerIDs []peer.ID, threshold int) (*peerStatus, error) {
	pc.joinPartyGroupLock.Lock()
	defer pc.joinPartyGroupLock.Unlock()
	if existing, ok := pc.peersGroup[messageID]; ok {
		existing.peerStatusLock.Lock()
		existing.joiners++
		existing.peerStatusLock.Unlock()
		return existing, nil
	}
	peerGroup := newPeerStatus(peerIDs, pc.host.ID(), leaderID, threshold)
	peerGroup.joiners = 1
	pc.peersGroup[messageID] = peerGroup
	return peerGroup, nil
}

func (pc *PartyCoordinator) getPeerIDs(ids []string) ([]peer.ID, error) {
	return conversion.GetPeerIDs(ids)
}

func (pc *PartyCoordinator) sendResponseToAll(msg *messages.JoinPartyLeaderComm, peers []peer.ID) {
	msg.MsgType = "response"
	msgSend, err := proto.Marshal(msg)
	if err != nil {
		pc.logger.Error().Err(err).Msg("error marshalling response")
		return
	}
	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, el := range peers {
		go func(peer peer.ID) {
			defer wg.Done()
			if peer == pc.host.ID() {
				return
			}
			if err := pc.sendMsgToPeer(msgSend, msg.ID, peer, joinPartyProtocolWithLeader, true); err != nil {
				pc.logger.Error().Err(err).Msg("error in send the join party request to peer")
			}
		}(el)
	}
	wg.Wait()
}

func (pc *PartyCoordinator) sendResponseToPeer(msg *messages.JoinPartyLeaderComm, remotePeer peer.ID) {
	msg = cloneJoinPartyLeaderComm(msg)
	if msg == nil {
		return
	}
	msg.MsgType = "response"
	msgSend, err := proto.Marshal(msg)
	if err != nil {
		pc.logger.Error().Err(err).Msg("error marshalling response")
		return
	}
	if err := pc.sendMsgToPeer(msgSend, msg.ID, remotePeer, joinPartyProtocolWithLeader, true); err != nil {
		pc.logger.Debug().Err(err).Msg("error sending closed party response to peer")
	}
}

func (pc *PartyCoordinator) sendRequestToLeader(msg *messages.JoinPartyLeaderComm, leader peer.ID) error {
	msg.MsgType = "request"
	msgSend, err := proto.Marshal(msg)
	if err != nil {
		pc.logger.Error().Err(err).Msg("error marshalling request")
		return err
	}

	if err := pc.sendMsgToPeer(msgSend, msg.ID, leader, joinPartyProtocolWithLeader, false); err != nil {
		pc.logger.Error().Err(err).Msg("error in send the join party request to leader")
		return errors.New("fail to send request to leader")
	}

	return nil
}

func (pc *PartyCoordinator) sendRequestToAll(msgID string, msgSend []byte, peers []peer.ID) {
	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, el := range peers {
		go func(peer peer.ID) {
			defer wg.Done()
			if peer == pc.host.ID() {
				return
			}
			if err := pc.sendMsgToPeer(msgSend, msgID, peer, joinPartyProtocol, false); err != nil {
				pc.logger.Error().Err(err).Msg("error in send the join party request to peer")
			}
		}(el)
	}
	wg.Wait()
}

func (pc *PartyCoordinator) sendMsgToPeer(msgBuf []byte, msgID string, remotePeer peer.ID, protoc protocol.ID, needResponse bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
	defer cancel()

	pc.logger.Debug().Msgf("try to open stream to (%s) ", remotePeer)
	stream, err := pc.host.NewStream(ctx, remotePeer, protoc)
	if err != nil {
		streamError := fmt.Errorf("fail to create stream to peer(%s):%w", remotePeer, err)
		return streamError
	}
	defer func() {
		pc.streamMgr.AddStream(msgID, stream)
		if closeErr := stream.Close(); closeErr != nil {
			pc.logger.Error().Err(closeErr).Msg("fail to close stream")
		}
	}()
	pc.logger.Debug().Msgf("open stream to (%s) successfully", remotePeer)
	err = WriteStreamWithBuffer(msgBuf, stream)
	if err != nil {
		return fmt.Errorf("fail to write message to stream:%w", err)
	}

	if needResponse {
		_, err = ReadStreamWithBuffer(stream)
		if err != nil {
			pc.logger.Error().Err(err).Msgf("fail to get the ")
		}
	}

	return nil
}

func (pc *PartyCoordinator) joinPartyMember(msgID string, peerGroup *peerStatus, sigChan chan string) ([]peer.ID, error) {
	leaderID := peerGroup.getLeader()
	msg := messages.JoinPartyLeaderComm{
		ID: msgID,
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				pc.logger.Trace().Msg("sending request message to leader")
				err := pc.sendRequestToLeader(&msg, leaderID)
				if err != nil {
					pc.logger.Error().Err(err).Msg("error sending request to leader")
				}
			}
			time.Sleep(time.Millisecond * 500)
		}
	}()

	var stopped bool
	var sigNotify string
	// now we wait for the leader to notify us who we do the keygen/keysign with
	select {
	case <-pc.stopChan:
		// promptly tear down this goroutine if partyCoordinator is stopped
		pc.logger.Debug().Msg("party coordinator stopped")
		stopped = true
	case <-peerGroup.notify:
		pc.logger.Debug().Msg("received a response from the leader")
	case <-time.After(pc.timeout):
		pc.logger.Debug().Msgf("timed out waiting for a response from the leader after %s", pc.timeout)
	case result := <-sigChan:
		pc.logger.Debug().Msgf("received %s from sigChan", result)
		sigNotify = result
	}

	close(done)
	wg.Wait()

	if sigNotify == "signature received" {
		return nil, ErrSignReceived
	}

	if peerGroup.getLeaderResponse() == nil {
		leaderPk, err := conversion.GetPubKeyFromPeerID(leaderID.String())
		if err != nil {
			pc.logger.Error().Msg("received no response from the leader")
		} else {
			pc.logger.Error().Msgf("received no response from the leader (%s)", leaderPk)
		}
		return nil, ErrLeaderNotReady
	}

	if peerGroup.getLeaderResponse().Type == messages.JoinPartyLeaderComm_LeaderNotReady {
		return nil, ErrPartyClosed
	}

	onlineNodes := peerGroup.getLeaderResponse().PeerIDs
	// we trust the returned nodes returned by the leader, if frost fail, the leader
	// also will get blamed.
	pIDs, err := pc.getPeerIDs(onlineNodes)
	if err != nil {
		pc.logger.Error().Err(err).Msg("fail to parse peer ids")
		return nil, err
	}

	pc.logger.Trace().Msgf("leader response message type=%s", peerGroup.getLeaderResponse().Type.String())
	if peerGroup.getLeaderResponse().Type == messages.JoinPartyLeaderComm_Success {
		if len(pIDs) < peerGroup.threshold {
			return pIDs, errors.New("not enough peers")
		}
		return pIDs, nil
	}

	if stopped {
		pc.logger.Trace().Msg("join party stopped")
	} else {
		pc.logger.Trace().Msg("join party timedout")
	}
	return pIDs, ErrJoinPartyTimeout
}

func (pc *PartyCoordinator) joinPartyLeader(msgID string, peerGroup *peerStatus, sigChan chan string) ([]peer.ID, error) {
	var sigNotify string
	select {
	case <-pc.stopChan:
		// promptly tear down this goroutine if partyCoordinator is stopped
		pc.logger.Debug().Msg("leader's party coordinator stopped")
	case <-peerGroup.notify:
		pc.logger.Debug().Msg("we have enough participants")
	case <-time.After(pc.timeout):
		pc.logger.Debug().Msgf("leader timedout waiting for peers after %s", pc.timeout)
	case result := <-sigChan:
		sigNotify = result
	}
	if sigNotify == "signature received" {
		return nil, ErrSignReceived
	}
	allPeers := peerGroup.getAllPeers()
	onlinePeers, _ := peerGroup.getPeersStatus()
	onlinePeers = append(onlinePeers, pc.host.ID())

	msg := *pc.successJoinPartyResponse(msgID, onlinePeers)
	if len(onlinePeers) < peerGroup.threshold {
		// we notify the failure of the join party to everyone
		msg.Type = messages.JoinPartyLeaderComm_Timeout
		pc.logger.Debug().Msgf("sending timeout response to %d all peers", len(onlinePeers))
		pc.sendResponseToAll(&msg, allPeers)
		return onlinePeers, ErrJoinPartyTimeout
	}
	// we notify all the peers who to run keygen/keysign
	// if a nodes is not in the list, it means he is not selected by the leader to run the frost
	pc.logger.Debug().Msgf("sending success response to %d all peers", len(allPeers))
	pc.sendResponseToAll(&msg, allPeers)
	pc.rememberClosedGroup(msgID)
	return onlinePeers, nil
}

func (pc *PartyCoordinator) JoinPartyWithLeader(msgID string, blockHeight int64, peers []string, threshold int, sigChan chan string) ([]peer.ID, string, error) {
	return pc.joinPartyWithLeader(msgID, blockHeight, peers, threshold, sigChan, "")
}

// JoinPartyWithLeaderInitiator coordinates a party where initiator is the leader.
// Keysign uses this because only the designated signer joins the party; keygen keeps
// the hash-selected leader so all participants can rendezvous concurrently.
func (pc *PartyCoordinator) JoinPartyWithLeaderInitiator(msgID string, blockHeight int64, peers []string, threshold int, sigChan chan string, initiator string) ([]peer.ID, string, error) {
	return pc.joinPartyWithLeader(msgID, blockHeight, peers, threshold, sigChan, initiator)
}

func (pc *PartyCoordinator) joinPartyWithLeader(msgID string, blockHeight int64, peers []string, threshold int, sigChan chan string, initiator string) ([]peer.ID, string, error) {
	var leader string
	var err error
	if initiator != "" {
		leader = initiator
	} else {
		leader, err = LeaderNode(msgID, blockHeight, peers)
		if err != nil {
			return nil, "", err
		}
	}
	leaderID, err := conversion.GetPeerIDFromPubKey(leader)
	if err != nil {
		return nil, "", err
	}
	peerIDs, err := pc.getPeerIDs(peers)
	if err != nil {
		return nil, "", err
	}

	peerGroup, err := pc.acquireJoinPartyGroup(msgID, leaderID, peerIDs, threshold)
	if err != nil {
		pc.logger.Error().Err(err).Msg("error creating peerStatus")
		return nil, leader, err
	}
	defer pc.releaseJoinPartyGroup(msgID)

	var onlines []peer.ID
	if pc.host.ID() == leaderID {
		onlines, err = pc.joinPartyLeader(msgID, peerGroup, sigChan)
		return onlines, leader, err
	}
	// now we are just the normal peer
	onlines, err = pc.joinPartyMember(msgID, peerGroup, sigChan)
	return onlines, leader, err
}

// JoinPartyWithRetry this method provide the functionality to join party with retry and back off
func (pc *PartyCoordinator) JoinPartyWithRetry(msgID string, peers []string) ([]peer.ID, error) {
	msg := messages.JoinPartyRequest{
		ID: msgID,
	}
	msgSend, err := proto.Marshal(&msg)
	if err != nil {
		pc.logger.Error().Msg("fail to marshal the message")
		return nil, err
	}
	peerIDs, err := pc.getPeerIDs(peers)
	if err != nil {
		pc.logger.Error().Msg("fail to parse peer.IDs")
		return nil, err
	}

	peerGroup, err := pc.acquireJoinPartyGroup(msg.ID, "NONE", peerIDs, 1)
	if err != nil {
		pc.logger.Error().Err(err).Msg("fail to create the join party group")
		return nil, err
	}
	defer pc.releaseJoinPartyGroup(msg.ID)
	_, offline := peerGroup.getPeersStatus()
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				pc.sendRequestToAll(msgID, msgSend, offline)
			}
			time.Sleep(time.Second)
		}
	}()
	// this is the total time FROST will wait for the party to form
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-pc.stopChan:
				// promptly tear down this goroutine if partyCoordinator is stopped
				pc.logger.Trace().Msg("party coordinator stopped")
				return
			case <-peerGroup.notify:
				pc.logger.Debug().Msg("we have found the new peer")
				if peerGroup.getCoordinationStatus() {
					close(done)
					return
				}
			case <-time.After(pc.timeout):
				// timeout
				close(done)
				return
			}
		}
	}()

	wg.Wait()
	onlinePeers, _ := peerGroup.getPeersStatus()
	pc.sendRequestToAll(msgID, msgSend, onlinePeers)
	// we always set ourselves as online
	onlinePeers = append(onlinePeers, pc.host.ID())
	if len(onlinePeers) == len(peers) {
		return onlinePeers, nil
	}
	return onlinePeers, ErrJoinPartyTimeout
}

func (pc *PartyCoordinator) ReleaseStream(msgID string) {
	pc.streamMgr.ReleaseStream(msgID)
}
