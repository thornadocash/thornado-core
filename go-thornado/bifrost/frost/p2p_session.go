package frost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

var frostSessionLocks sync.Map // sessionID -> *sync.Mutex

const postJoinSync = 5 * time.Second

// P2PSessionCoordinator runs distributed FROST DKG and signing over libp2p.
type P2PSessionCoordinator struct {
	comm           *p2p.Communication
	party          *p2p.PartyCoordinator
	keygenTimeout  time.Duration
	keysignTimeout time.Duration
	logger         zerolog.Logger
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
	}
}

func (sc *P2PSessionCoordinator) SetLogger(logger zerolog.Logger) {
	sc.logger = logger
}

func sortedParticipants(participants []string) []string {
	return sortedParticipantStrings(participants)
}

func sessionMessageID(prefix string, height int64, minSigners uint16, participants []string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s", prefix, height, minSigners, strings.Join(sortedParticipants(participants), ","))))
	return hex.EncodeToString(sum[:])
}

func (sc *P2PSessionCoordinator) joinParty(msgID string, height int64, participants []string, threshold int, initiator string) ([]peer.ID, error) {
	if sc.party == nil {
		return nil, fmt.Errorf("FROST party coordinator is not configured")
	}
	if sc.comm != nil {
		if err := sc.comm.EnsurePeersConnected(participants); err != nil {
			return nil, fmt.Errorf("ensure party peers before join: %w", err)
		}
	}
	sigChan := make(chan string, 1)
	var online []peer.ID
	var err error
	if initiator != "" {
		online, _, err = sc.party.JoinPartyWithLeaderInitiator(msgID, height, participants, threshold, sigChan, initiator)
	} else {
		online, _, err = sc.party.JoinPartyWithLeader(msgID, height, participants, threshold, sigChan)
	}
	if err != nil {
		return nil, fmt.Errorf("join FROST party: %w", err)
	}
	if len(online) < threshold {
		return nil, fmt.Errorf("insufficient FROST party members online: got %d want %d", len(online), threshold)
	}
	return online, nil
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
	inbox := make(chan *p2p.Message, 256)
	sc.comm.SetSubscribe(messages.FROSTFrostKeyGenMsg, msgID, inbox)
	defer sc.comm.CancelSubscribe(messages.FROSTFrostKeyGenMsg, msgID)

	keygenThreshold := len(participants) - 1
	if _, err = sc.joinParty(msgID, height, participants, keygenThreshold, ""); err != nil {
		return nil, nil, err
	}
	defer sc.comm.ReleaseStream(msgID)

	// Straggling signers may still be subscribing when the first joiners enter
	// runSession; pause briefly so round-1 messages are not dropped.
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-time.After(postJoinSync):
	}

	handle, err := frostsessions.KeygenSessionNew(participants, localParty, minSigners)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = frostsessions.SessionFree(handle) }()

	result, err := sc.runSession(ctx, messages.FROSTFrostKeyGenMsg, msgID, participants, localParty, handle, sc.keygenTimeout, inbox)
	if err != nil {
		return nil, nil, err
	}
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
	inbox := make(chan *p2p.Message, 256)
	sc.comm.SetSubscribe(messages.FROSTFrostKeySignMsg, sessionID, inbox)
	defer sc.comm.CancelSubscribe(messages.FROSTFrostKeySignMsg, sessionID)

	if _, err = sc.joinParty(sessionID, height, participants, int(threshold), partyLeader); err != nil {
		return nil, err
	}
	defer sc.comm.ReleaseStream(sessionID)

	handle, err := frostsessions.SignSessionNewWithTweak(participants, localParty, share, msg, taprootKeyPath, scriptRoot, childTweak)
	if err != nil {
		return nil, err
	}
	defer func() { _ = frostsessions.SessionFree(handle) }()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(postJoinSync):
	}

	return sc.runSession(ctx, messages.FROSTFrostKeySignMsg, sessionID, participants, localParty, handle, sc.keysignTimeout, inbox)
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
	for {
		if err := sessionCtx.Err(); err != nil {
			return nil, err
		}

		progress := false
		for {
			step := false

			if retried, err := sc.retryPending(msgType, handle, &pending); err != nil {
				return nil, err
			} else if retried {
				step = true
			}

			if drained, err := sc.drainInbox(sessionCtx, msgType, handle, inbox, &pending); err != nil {
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
			return result, nil
		}

		if !progress {
			select {
			case <-sessionCtx.Done():
				return nil, sessionCtx.Err()
			case raw := <-inbox:
				if err := sc.ingestSessionMessage(msgType, handle, raw); err != nil {
					if isRetryableFrostInputError(err) {
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
	handle frostsessions.Handle,
	raw *p2p.Message,
) error {
	payload, err := decodeWrapped(msgType, raw)
	if err != nil {
		return err
	}
	_, err = frostsessions.SessionInputMessage(handle, payload)
	return err
}

func (sc *P2PSessionCoordinator) retryPending(
	msgType messages.ThornadoFROSTMessageType,
	handle frostsessions.Handle,
	pending *[]*p2p.Message,
) (bool, error) {
	if len(*pending) == 0 {
		return false, nil
	}
	remaining := (*pending)[:0]
	retried := false
	for _, raw := range *pending {
		err := sc.ingestSessionMessage(msgType, handle, raw)
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
	handle frostsessions.Handle,
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
			if err := sc.ingestSessionMessage(msgType, handle, raw); err != nil {
				if isRetryableFrostInputError(err) {
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
	return err != nil && strings.Contains(err.Error(), "missing round2 secret")
}

func decodeWrapped(expected messages.ThornadoFROSTMessageType, raw *p2p.Message) ([]byte, error) {
	if raw == nil {
		return nil, fmt.Errorf("nil frost inbound message")
	}
	var wrapped messages.WrappedMessage
	if err := json.Unmarshal(raw.Payload, &wrapped); err != nil {
		return nil, fmt.Errorf("decode wrapped frost message: %w", err)
	}
	if wrapped.MessageType != expected {
		return nil, fmt.Errorf("unexpected frost message type %s", wrapped.MessageType)
	}
	return wrapped.Payload, nil
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
