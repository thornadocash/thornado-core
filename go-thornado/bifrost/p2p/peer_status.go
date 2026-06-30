package p2p

import (
	"errors"
	"sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog/log"

	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

type peerStatus struct {
	peersResponse  map[peer.ID]bool
	peerStatusLock *sync.RWMutex
	allPeers       []peer.ID
	notify         chan bool
	leaderResponse *messages.JoinPartyLeaderComm
	leader         peer.ID
	threshold      int
	reqCount       int
	joiners        int
}

func (ps *peerStatus) getLeaderResponse() *messages.JoinPartyLeaderComm {
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	return ps.leaderResponse
}

func (ps *peerStatus) setLeaderResponse(resp *messages.JoinPartyLeaderComm) {
	ps.peerStatusLock.Lock()
	defer ps.peerStatusLock.Unlock()
	ps.leaderResponse = resp
}

func (ps *peerStatus) getLeader() peer.ID {
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	return ps.leader
}

func (ps *peerStatus) matchesParty(leaderID peer.ID, peerNodes []peer.ID, threshold int) bool {
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	if ps.leader != leaderID || ps.threshold != threshold || len(ps.allPeers) != len(peerNodes) {
		return false
	}
	seen := make(map[peer.ID]struct{}, len(ps.allPeers))
	for _, peerNode := range ps.allPeers {
		seen[peerNode] = struct{}{}
	}
	for _, peerNode := range peerNodes {
		if _, ok := seen[peerNode]; !ok {
			return false
		}
	}
	return true
}

func (ps *peerStatus) closeParty() {
	ps.peerStatusLock.Lock()
	defer ps.peerStatusLock.Unlock()
	ps.leaderResponse = &messages.JoinPartyLeaderComm{
		MsgType: "response",
		Type:    messages.JoinPartyLeaderComm_LeaderNotReady,
	}
	for peerNode := range ps.peersResponse {
		ps.peersResponse[peerNode] = false
	}
	ps.reqCount = 0
	select {
	case ps.notify <- true:
	default:
	}
}

func newPeerStatus(peerNodes []peer.ID, myPeerID, leaderID peer.ID, threshold int) *peerStatus {
	dat := make(map[peer.ID]bool)
	for _, el := range peerNodes {
		if el == myPeerID {
			continue
		}
		dat[el] = false
	}
	peerStatus := &peerStatus{
		peersResponse:  dat,
		peerStatusLock: &sync.RWMutex{},
		notify:         make(chan bool, len(peerNodes)),
		allPeers:       peerNodes,
		leader:         leaderID,
		threshold:      threshold,
		reqCount:       0,
	}
	return peerStatus
}

func (ps *peerStatus) getCoordinationStatus() bool {
	_, offline := ps.getPeersStatus()
	return len(offline) == 0
}

func (ps *peerStatus) getAllPeers() []peer.ID {
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	return ps.allPeers
}

func (ps *peerStatus) getPeersStatus() ([]peer.ID, []peer.ID) {
	var online []peer.ID
	var offline []peer.ID
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	for peerNode, val := range ps.peersResponse {
		if val {
			online = append(online, peerNode)
		} else {
			offline = append(offline, peerNode)
		}
	}

	return online, offline
}

func (ps *peerStatus) hasSelectedThreshold() bool {
	ps.peerStatusLock.RLock()
	defer ps.peerStatusLock.RUnlock()
	requiredPeers := ps.threshold
	if requiredPeers > 0 && ps.leader != "NONE" {
		requiredPeers--
	}
	return ps.reqCount >= requiredPeers
}

func (ps *peerStatus) updatePeer(peerNode peer.ID) (bool, error) {
	ps.peerStatusLock.Lock()
	defer ps.peerStatusLock.Unlock()
	val, ok := ps.peersResponse[peerNode]
	if !ok {
		return false, errors.New("key not found")
	}

	if ps.leader == "NONE" {
		if !val {
			ps.peersResponse[peerNode] = true
			return true, nil
		}
		return false, nil
	}

	requiredPeers := ps.threshold
	if requiredPeers > 0 {
		// The leader is part of the signing set but is not counted in reqCount,
		// which only tracks remote peers that joined this leader.
		requiredPeers--
	}

	// we already have enough participants
	if ps.reqCount >= requiredPeers {
		return false, nil
	}
	if !val {
		ps.peersResponse[peerNode] = true
		ps.reqCount++
		log.Debug().Msgf("leader has %d out of %d remote participants", ps.reqCount, requiredPeers)
		if ps.reqCount >= requiredPeers {
			return true, nil
		}
	}
	return false, nil
}
