package frost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"
	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
)

var (
	frostSessionLocks        sync.Map // sessionID -> *sync.Mutex
	ErrLocalPartyNotSelected = errors.New("local FROST party not selected")
)

const postJoinSync = 0

// P2PSessionCoordinator runs distributed FROST DKG and signing over libp2p.
type P2PSessionCoordinator struct {
	comm           *p2p.Communication
	party          *p2p.PartyCoordinator
	keygenTimeout  time.Duration
	keysignTimeout time.Duration
	logger         zerolog.Logger
	debugMu        sync.Mutex
	debugOrder     []string
	debugSessions  map[string]*DebugSession
}

func NewP2PSessionCoordinator(
	comm *p2p.Communication,
	party *p2p.PartyCoordinator,
	keygenTimeout time.Duration,
	keysignTimeout time.Duration,
) *P2PSessionCoordinator {
	if keygenTimeout <= 0 {
		keygenTimeout = 5 * time.Minute
	}
	if keysignTimeout <= 0 {
		keysignTimeout = 5 * time.Minute
	}
	return &P2PSessionCoordinator{
		comm:           comm,
		party:          party,
		keygenTimeout:  keygenTimeout,
		keysignTimeout: keysignTimeout,
		logger:         zerolog.Nop(),
		debugSessions:  make(map[string]*DebugSession),
	}
}

func (sc *P2PSessionCoordinator) SetLogger(logger zerolog.Logger) {
	sc.logger = logger
}

type DebugSession struct {
	ID            string              `json:"id"`
	Type          string              `json:"type"`
	Height        int64               `json:"height"`
	LocalParty    string              `json:"local_party"`
	Leader        string              `json:"leader,omitempty"`
	LeaderKnownAt *time.Time          `json:"leader_known_at,omitempty"`
	Threshold     int                 `json:"threshold"`
	Participants  []string            `json:"participants"`
	StartedAt     time.Time           `json:"started_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	FinishedAt    *time.Time          `json:"finished_at,omitempty"`
	DurationMs    int64               `json:"duration_ms"`
	LastEvent     string              `json:"last_event"`
	LastError     string              `json:"last_error,omitempty"`
	Broadcasts    map[string]int      `json:"broadcasts"`
	Receives      map[string]int      `json:"receives"`
	Pending       int                 `json:"pending"`
	Phases        []DebugSessionPhase `json:"phases"`
}

type DebugSessionPhase struct {
	Event        string    `json:"event"`
	Kind         string    `json:"kind,omitempty"`
	At           time.Time `json:"at"`
	SinceStartMs int64     `json:"since_start_ms"`
	Count        int       `json:"count,omitempty"`
	Targets      int       `json:"targets,omitempty"`
}

func (sc *P2PSessionCoordinator) DebugSessions() []DebugSession {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	out := make([]DebugSession, 0, len(sc.debugOrder))
	for _, id := range sc.debugOrder {
		session := sc.debugSessions[id]
		if session == nil {
			continue
		}
		cp := *session
		cp.Participants = append([]string(nil), session.Participants...)
		cp.Broadcasts = copyDebugCounts(session.Broadcasts)
		cp.Receives = copyDebugCounts(session.Receives)
		cp.Phases = append([]DebugSessionPhase(nil), session.Phases...)
		if session.FinishedAt != nil {
			cp.DurationMs = session.FinishedAt.Sub(session.StartedAt).Milliseconds()
		} else if !session.StartedAt.IsZero() {
			cp.DurationMs = time.Since(session.StartedAt).Milliseconds()
		}
		out = append(out, cp)
	}
	return out
}

func copyDebugCounts(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (sc *P2PSessionCoordinator) debugStart(id, typ string, height int64, participants []string, localParty, leader string, threshold int) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	sc.ensureDebugMap()
	now := time.Now().UTC()
	if _, ok := sc.debugSessions[id]; !ok {
		sc.debugOrder = append(sc.debugOrder, id)
		if len(sc.debugOrder) > 128 {
			delete(sc.debugSessions, sc.debugOrder[0])
			sc.debugOrder = sc.debugOrder[1:]
		}
	}
	sc.debugSessions[id] = &DebugSession{
		ID:           id,
		Type:         typ,
		Height:       height,
		LocalParty:   localParty,
		Leader:       leader,
		Threshold:    threshold,
		Participants: append([]string(nil), participants...),
		StartedAt:    now,
		UpdatedAt:    now,
		LastEvent:    "started",
		Broadcasts:   make(map[string]int),
		Receives:     make(map[string]int),
	}
	session := sc.debugSessions[id]
	appendDebugSessionPhaseLocked(session, "started", "", 0, 0, now)
	if leader != "" {
		session.LeaderKnownAt = &now
		appendDebugSessionPhaseLocked(session, "leader_appointed", "", 0, 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugEvent(id, event string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		session.LastEvent = event
		now := time.Now().UTC()
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, event, "", 0, 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugLeader(id, leader string) {
	if leader == "" {
		return
	}
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		now := time.Now().UTC()
		session.Leader = leader
		if session.LeaderKnownAt == nil {
			session.LeaderKnownAt = &now
			appendDebugSessionPhaseLocked(session, "leader_appointed", "", 0, 0, now)
		}
		session.LastEvent = "leader_appointed"
		session.UpdatedAt = now
	}
}

func (sc *P2PSessionCoordinator) debugSelectedParticipants(id string, participants []string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		now := time.Now().UTC()
		session.Participants = append([]string(nil), participants...)
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "participants_selected", "", len(participants), 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugBroadcast(id, kind string, targets int) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		if kind == "" {
			kind = "unknown"
		}
		session.Broadcasts[kind] += targets
		session.LastEvent = "broadcast:" + kind
		now := time.Now().UTC()
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "broadcast", kind, session.Broadcasts[kind], targets, now)
	}
}

func (sc *P2PSessionCoordinator) debugReceive(id, kind string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		if kind == "" {
			kind = "unknown"
		}
		session.Receives[kind]++
		session.LastEvent = "receive:" + kind
		now := time.Now().UTC()
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "receive", kind, session.Receives[kind], 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugPending(id string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		session.Pending++
		session.LastEvent = "pending"
		now := time.Now().UTC()
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "pending", "", session.Pending, 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugFinish(id string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		now := time.Now().UTC()
		session.FinishedAt = &now
		switch session.Type {
		case "keysign":
			appendDebugSessionPhaseLocked(session, "signature_produced", "", 0, 0, now)
		case "keygen":
			appendDebugSessionPhaseLocked(session, "keyshare_produced", "", 0, 0, now)
		default:
			appendDebugSessionPhaseLocked(session, "result_produced", "", 0, 0, now)
		}
		appendDebugSessionPhaseLocked(session, "finished", "", 0, 0, now)
		session.LastEvent = "finished"
		session.UpdatedAt = now
	}
}

func (sc *P2PSessionCoordinator) debugError(id string, err error) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		now := time.Now().UTC()
		session.LastError = err.Error()
		session.LastEvent = "error"
		if session.FinishedAt == nil {
			session.FinishedAt = &now
		}
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "error", "", 0, 0, now)
	}
}

func (sc *P2PSessionCoordinator) debugClosed(id string) {
	sc.debugMu.Lock()
	defer sc.debugMu.Unlock()
	if session := sc.debugSessions[id]; session != nil {
		now := time.Now().UTC()
		session.LastError = ""
		session.LastEvent = "closed"
		if session.FinishedAt == nil {
			session.FinishedAt = &now
		}
		session.UpdatedAt = now
		appendDebugSessionPhaseLocked(session, "closed", "", 0, 0, now)
	}
}

func (sc *P2PSessionCoordinator) ensureDebugMap() {
	if sc.debugSessions == nil {
		sc.debugSessions = make(map[string]*DebugSession)
	}
}

func appendDebugSessionPhaseLocked(session *DebugSession, event, kind string, count, targets int, at time.Time) {
	if session == nil {
		return
	}
	phase := DebugSessionPhase{
		Event:   event,
		Kind:    kind,
		At:      at,
		Count:   count,
		Targets: targets,
	}
	if !session.StartedAt.IsZero() {
		phase.SinceStartMs = at.Sub(session.StartedAt).Milliseconds()
	}
	session.Phases = append(session.Phases, phase)
	if len(session.Phases) > 256 {
		session.Phases = session.Phases[len(session.Phases)-256:]
	}
}

func sortedParticipants(participants []string) []string {
	return sortedParticipantStrings(participants)
}

func sessionMessageID(prefix string, height int64, minSigners uint16, participants []string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", prefix, height, minSigners, strings.Join(sortedParticipants(participants), ","))))
	return hex.EncodeToString(sum[:])
}

func (sc *P2PSessionCoordinator) joinParty(msgID string, height int64, participants []string, threshold int, initiator string) ([]peer.ID, string, error) {
	if sc.party == nil {
		return nil, "", fmt.Errorf("FROST party coordinator is not configured")
	}
	if sc.comm != nil {
		sc.debugEvent(msgID, "ensure_peers_start")
		if err := sc.comm.EnsurePeersConnected(participants); err != nil {
			sc.debugEvent(msgID, "ensure_peers_error")
			return nil, "", fmt.Errorf("ensure party peers before join: %w", err)
		}
		sc.debugEvent(msgID, "ensure_peers_done")
	}
	sigChan := make(chan string, 1)
	var online []peer.ID
	var leader string
	var err error
	if initiator != "" {
		online, leader, err = sc.party.JoinPartyWithLeaderInitiator(msgID, height, participants, threshold, sigChan, initiator)
	} else {
		online, leader, err = sc.party.JoinPartyWithLeader(msgID, height, participants, threshold, sigChan)
	}
	if err != nil {
		return nil, leader, fmt.Errorf("join FROST party: %w", err)
	}
	if len(online) < threshold {
		return nil, leader, fmt.Errorf("insufficient FROST party members online: got %d want %d", len(online), threshold)
	}
	return online, leader, nil
}

func (sc *P2PSessionCoordinator) RunKeygen(
	ctx context.Context,
	height int64,
	participants []string,
	localParty string,
	minSigners uint16,
) (localShare []byte, pubKeyCompressed []byte, err error) {
	participants = sortedParticipants(participants)
	msgID := sessionMessageID("keygen", height, minSigners, participants)
	sc.debugStart(msgID, "keygen", height, participants, localParty, "", int(minSigners))
	inbox := make(chan *p2p.Message, 256)
	sc.comm.SetSubscribe(messages.FROSTFrostKeyGenMsg, msgID, inbox)
	defer sc.comm.CancelSubscribe(messages.FROSTFrostKeyGenMsg, msgID)

	keygenThreshold := int(minSigners)
	sc.debugEvent(msgID, "party_join_start")
	var leader string
	if _, leader, err = sc.joinParty(msgID, height, participants, keygenThreshold, ""); err != nil {
		if errors.Is(err, p2p.ErrPartyClosed) {
			sc.debugClosed(msgID)
		} else {
			sc.debugError(msgID, err)
		}
		return nil, nil, err
	}
	sc.debugLeader(msgID, leader)
	sc.debugEvent(msgID, "party_joined")
	defer sc.comm.ReleaseStream(msgID)

	sc.debugEvent(msgID, "post_join_sync_start")
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-time.After(postJoinSync):
	}
	sc.debugEvent(msgID, "post_join_sync_done")

	sc.debugEvent(msgID, "session_init_start")
	handle, err := frostsessions.KeygenSessionNew(participants, localParty, minSigners)
	if err != nil {
		sc.debugError(msgID, err)
		return nil, nil, err
	}
	sc.debugEvent(msgID, "session_init_done")
	defer func() { _ = frostsessions.SessionFree(handle) }()

	sc.debugEvent(msgID, "rounds_start")
	result, err := sc.runSession(ctx, messages.FROSTFrostKeyGenMsg, msgID, participants, localParty, handle, sc.keygenTimeout, inbox)
	if err != nil {
		sc.debugError(msgID, err)
		return nil, nil, err
	}
	sc.debugFinish(msgID)
	decoded, err := frostsessions.DecodeKeyshare(result)
	if err != nil {
		return nil, nil, err
	}
	pubKeyCompressed, err = hex.DecodeString(decoded.PublicKeyCompressed)
	if err != nil {
		return nil, nil, fmt.Errorf("decode FROST vault pubkey: %w", err)
	}
	return result, pubKeyCompressed, nil
}

func (sc *P2PSessionCoordinator) RunSign(
	ctx context.Context,
	sessionID string,
	height int64,
	participants []string,
	localParty string,
	share []byte,
	msg []byte,
	taprootKeyPath bool,
	scriptRoot []byte,
	childTweak []byte,
	partyLeader string,
) (signature []byte, err error) {
	if len(msg) != 32 {
		return nil, fmt.Errorf("FROST messages must be 32 bytes, got %d", len(msg))
	}
	lock, _ := frostSessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	participants = sortedParticipants(participants)
	threshold := frostMinSigners(len(participants))
	sc.debugStart(sessionID, "keysign", height, participants, localParty, partyLeader, int(threshold))
	inbox := make(chan *p2p.Message, 256)
	sc.comm.SetSubscribe(messages.FROSTFrostKeySignMsg, sessionID, inbox)
	defer sc.comm.CancelSubscribe(messages.FROSTFrostKeySignMsg, sessionID)

	sc.debugEvent(sessionID, "party_join_start")
	var leader string
	var online []peer.ID
	if online, leader, err = sc.joinParty(sessionID, height, participants, int(threshold), partyLeader); err != nil {
		if errors.Is(err, p2p.ErrPartyClosed) {
			sc.debugClosed(sessionID)
		} else {
			sc.debugError(sessionID, err)
		}
		return nil, err
	}
	participants, err = peerIDsToParticipantPubKeys(online)
	if err != nil {
		sc.debugError(sessionID, err)
		return nil, err
	}
	if len(participants) < int(threshold) {
		err = fmt.Errorf("insufficient selected FROST signers: got %d want %d", len(participants), threshold)
		sc.debugError(sessionID, err)
		return nil, err
	}
	sc.debugSelectedParticipants(sessionID, participants)
	if !participantListContains(participants, localParty) {
		sc.debugClosed(sessionID)
		return nil, ErrLocalPartyNotSelected
	}
	sc.debugLeader(sessionID, leader)
	sc.debugEvent(sessionID, "party_joined")
	defer sc.comm.ReleaseStream(sessionID)

	sc.debugEvent(sessionID, "session_init_start")
	handle, err := frostsessions.SignSessionNewWithTweak(participants, localParty, share, msg, taprootKeyPath, scriptRoot, childTweak)
	if err != nil {
		sc.debugError(sessionID, err)
		return nil, err
	}
	sc.debugEvent(sessionID, "session_init_done")
	defer func() { _ = frostsessions.SessionFree(handle) }()

	sc.debugEvent(sessionID, "post_join_sync_start")
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(postJoinSync):
	}
	sc.debugEvent(sessionID, "post_join_sync_done")

	sc.debugEvent(sessionID, "rounds_start")
	signature, err = sc.runSession(ctx, messages.FROSTFrostKeySignMsg, sessionID, participants, localParty, handle, sc.keysignTimeout, inbox)
	if err != nil {
		sc.debugError(sessionID, err)
		return nil, err
	}
	sc.debugFinish(sessionID)
	return signature, nil
}

func peerIDsToParticipantPubKeys(peers []peer.ID) ([]string, error) {
	participants := make([]string, 0, len(peers))
	for _, p := range peers {
		pubKey, err := conversion.GetPubKeyFromPeerID(p.String())
		if err != nil {
			return nil, fmt.Errorf("map selected peer %s to pubkey: %w", p.String(), err)
		}
		participants = append(participants, pubKey)
	}
	return sortedParticipants(participants), nil
}

func participantListContains(participants []string, localParty string) bool {
	for _, participant := range participants {
		if strings.EqualFold(participant, localParty) {
			return true
		}
	}
	return false
}

func SignSessionID(vaultPubKey string, msg []byte) string {
	sum := sha256.Sum256(append([]byte(vaultPubKey), msg...))
	return hex.EncodeToString(sum[:])
}

func (sc *P2PSessionCoordinator) runSession(
	ctx context.Context,
	msgType messages.ThornadoFROSTMessageType,
	msgID string,
	participants []string,
	localParty string,
	handle frostsessions.Handle,
	timeout time.Duration,
	inbox <-chan *p2p.Message,
) ([]byte, error) {
	if sc.comm == nil {
		return nil, fmt.Errorf("FROST communication layer is not configured")
	}
	peerByParty := make(map[string]peer.ID, len(participants))
	for _, party := range participants {
		peerID, err := conversion.GetPeerIDFromPubKey(party)
		if err != nil {
			return nil, fmt.Errorf("map participant %s to peer id: %w", party, err)
		}
		peerByParty[party] = peerID
	}

	sessionCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pending := make([]*p2p.Message, 0)
	wait := 50 * time.Millisecond
	localMaxBroadcastRound := 0
	for {
		if err := sessionCtx.Err(); err != nil {
			return nil, err
		}

		progress := false
		for {
			step := false

			if retried, err := sc.retryPending(msgType, msgID, handle, localMaxBroadcastRound, &pending); err != nil {
				return nil, err
			} else if retried {
				step = true
			}

			if drained, err := sc.drainInbox(sessionCtx, msgType, msgID, handle, localMaxBroadcastRound, inbox, &pending); err != nil {
				return nil, err
			} else if drained {
				step = true
			}

			for {
				out, err := frostsessions.SessionOutputMessage(handle)
				if err != nil {
					return nil, err
				}
				if len(out) == 0 {
					break
				}
				step = true
				targets := broadcastTargets(handle, out, participants, peerByParty)
				kind := frostPayloadKind(out)
				if round := frostKindRound(kind); round > localMaxBroadcastRound {
					localMaxBroadcastRound = round
				}
				sc.debugBroadcast(msgID, kind, len(targets))
				sc.broadcast(msgType, msgID, out, targets)
				if err := sc.ingestLocalOutbound(handle, out, localParty); err != nil {
					return nil, err
				}
			}

			if !step {
				break
			}
			progress = true
		}

		if result, err := sc.trySessionFinish(handle); err != nil {
			return nil, err
		} else if result != nil {
			sc.debugEvent(msgID, "result_ready")
			return result, nil
		}

		if !progress {
			select {
			case <-sessionCtx.Done():
				return nil, sessionCtx.Err()
			case raw := <-inbox:
				if err := sc.ingestSessionMessage(msgType, msgID, handle, raw, localMaxBroadcastRound); err != nil {
					if isRetryableFrostInputError(err) {
						sc.debugPending(msgID)
						pending = append(pending, raw)
					} else {
						return nil, err
					}
				}
			case <-time.After(wait):
			}
		}
	}
}

func broadcastTargets(
	handle frostsessions.Handle,
	out []byte,
	participants []string,
	peerByParty map[string]peer.ID,
) []peer.ID {
	targets := make([]peer.ID, 0, len(participants))
	seen := make(map[peer.ID]struct{}, len(participants))
	for idx := 0; idx < len(participants); idx++ {
		to, err := frostsessions.SessionMessageReceiver(handle, out, idx)
		if err != nil {
			return nil
		}
		if to == "" {
			break
		}
		peerID, ok := peerByParty[to]
		if !ok {
			return nil
		}
		if _, dup := seen[peerID]; dup {
			continue
		}
		seen[peerID] = struct{}{}
		targets = append(targets, peerID)
	}
	return targets
}

func (sc *P2PSessionCoordinator) trySessionFinish(handle frostsessions.Handle) ([]byte, error) {
	result, err := frostsessions.SessionFinish(handle)
	if err == nil {
		return result, nil
	}
	return nil, nil
}

func (sc *P2PSessionCoordinator) ingestLocalOutbound(
	handle frostsessions.Handle,
	out []byte,
	localParty string,
) error {
	if localParty == "" {
		return nil
	}
	for idx := 0; ; idx++ {
		to, err := frostsessions.SessionMessageReceiver(handle, out, idx)
		if err != nil {
			return err
		}
		if to == "" {
			return nil
		}
		if strings.EqualFold(to, localParty) {
			if _, err := frostsessions.SessionInputMessage(handle, out); err != nil {
				return err
			}
			return nil
		}
	}
}

func (sc *P2PSessionCoordinator) ingestSessionMessage(
	msgType messages.ThornadoFROSTMessageType,
	msgID string,
	handle frostsessions.Handle,
	raw *p2p.Message,
	localMaxBroadcastRound int,
) error {
	payload, kind, err := decodeWrapped(msgType, raw)
	if err != nil {
		return err
	}
	if round := frostKindRound(kind); round > 1 && round > localMaxBroadcastRound {
		return fmt.Errorf("out-of-order frost message %s before local round %d broadcast", kind, round)
	}
	_, err = frostsessions.SessionInputMessage(handle, payload)
	if err == nil {
		sc.debugReceive(msgID, kind)
	}
	return err
}

func (sc *P2PSessionCoordinator) retryPending(
	msgType messages.ThornadoFROSTMessageType,
	msgID string,
	handle frostsessions.Handle,
	localMaxBroadcastRound int,
	pending *[]*p2p.Message,
) (bool, error) {
	if len(*pending) == 0 {
		return false, nil
	}
	remaining := (*pending)[:0]
	retried := false
	for _, raw := range *pending {
		err := sc.ingestSessionMessage(msgType, msgID, handle, raw, localMaxBroadcastRound)
		switch {
		case err == nil:
			retried = true
		case isRetryableFrostInputError(err):
			remaining = append(remaining, raw)
		default:
			return retried, err
		}
	}
	*pending = remaining
	return retried, nil
}

func (sc *P2PSessionCoordinator) drainInbox(
	ctx context.Context,
	msgType messages.ThornadoFROSTMessageType,
	msgID string,
	handle frostsessions.Handle,
	localMaxBroadcastRound int,
	inbox <-chan *p2p.Message,
	pending *[]*p2p.Message,
) (bool, error) {
	drained := false
	for {
		select {
		case <-ctx.Done():
			return drained, ctx.Err()
		case raw := <-inbox:
			drained = true
			if err := sc.ingestSessionMessage(msgType, msgID, handle, raw, localMaxBroadcastRound); err != nil {
				if isRetryableFrostInputError(err) {
					sc.debugPending(msgID)
					*pending = append(*pending, raw)
					continue
				}
				return drained, err
			}
		default:
			return drained, nil
		}
	}
}

func isRetryableFrostInputError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "missing round2 secret") ||
		strings.Contains(err.Error(), "out-of-order frost message"))
}

func decodeWrapped(expected messages.ThornadoFROSTMessageType, raw *p2p.Message) ([]byte, string, error) {
	if raw == nil {
		return nil, "", fmt.Errorf("nil frost inbound message")
	}
	var wrapped messages.WrappedMessage
	if err := json.Unmarshal(raw.Payload, &wrapped); err != nil {
		return nil, "", fmt.Errorf("decode wrapped frost message: %w", err)
	}
	if wrapped.MessageType != expected {
		return nil, "", fmt.Errorf("unexpected frost message type %s", wrapped.MessageType)
	}
	return wrapped.Payload, frostPayloadKind(wrapped.Payload), nil
}

func (sc *P2PSessionCoordinator) broadcast(msgType messages.ThornadoFROSTMessageType, msgID string, payload []byte, peers []peer.ID) {
	if len(peers) == 0 {
		return
	}
	wrapped := messages.WrappedMessage{
		MessageType: msgType,
		MsgID:       msgID,
		Payload:     payload,
	}
	raw, err := json.Marshal(wrapped)
	if err != nil {
		sc.logger.Error().Err(err).Msg("marshal frost wrapped message")
		return
	}
	sc.comm.BroadcastSync(peers, raw, msgID)
}

func frostPayloadKind(payload []byte) string {
	var msg struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return ""
	}
	return msg.Kind
}

func frostKindRound(kind string) int {
	lower := strings.ToLower(kind)
	idx := strings.LastIndex(lower, "round")
	if idx < 0 {
		return 0
	}
	n := 0
	for _, r := range lower[idx+len("round"):] {
		if r < '0' || r > '9' {
			if n > 0 {
				break
			}
			continue
		}
		n = n*10 + int(r-'0')
	}
	return n
}
