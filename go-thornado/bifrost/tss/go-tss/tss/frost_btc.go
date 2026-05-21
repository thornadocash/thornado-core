package tss

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tcrypto "github.com/cometbft/cometbft/crypto"
	tmsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	coskey "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cosmos "github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types/bech32/legacybech32"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keygen"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign"
	tcommon "github.com/thornadocash/go-thornado/common"
	frostsession "github.com/thornadocash/go-thornado/frost/go-wrappers/go-frost/sessions"
)

const (
	frostKeygenAbortRound  = "FrostKeygenAbort"
	frostKeysignAbortRound = "FrostKeysignAbort"

	frostAbortPhaseKeygen  = "keygen"
	frostAbortPhaseKeysign = "keysign"
)

type frostWireMessage struct {
	Kind    string `json:"kind,omitempty"`
	From    string `json:"from"`
	To      string `json:"to,omitempty"`
	Index   int    `json:"index,omitempty"`
	Message []byte `json:"message"`
	Sig     []byte `json:"sig,omitempty"`
}

type frostSignResultMessage struct {
	Signatures []keysign.Signature `json:"signatures"`
}

func frostIDs(participants []string) []string {
	ids := append([]string(nil), participants...)
	sort.Strings(ids)
	return ids
}

func frostThreshold(participantCount int) (int, error) {
	threshold, err := conversion.GetThreshold(participantCount)
	if err != nil {
		return 0, err
	}
	return threshold + 1, nil
}

func frostAbortConfigValue(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	data, err := os.ReadFile("/var/data/bifrost/frost-abort.env")
	if err != nil {
		return ""
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func frostAbortMode() string {
	return strings.ToLower(frostAbortConfigValue("BIFROST_FROST_ABORT_MODE"))
}

func (t *TssServer) shouldTriggerFrostFault(phase string, produced, input uint64, defaultTarget string) bool {
	if frostAbortConfigValue("BIFROST_FROST_ABORT_PHASE") != phase {
		return false
	}
	target := frostAbortConfigValue("BIFROST_FROST_ABORT_LOCAL_PUBKEY")
	if target == "" {
		target = defaultTarget
	}
	if target != "" && target != t.localNodePubKey {
		return false
	}
	afterProduced := uint64(1)
	if raw := frostAbortConfigValue("BIFROST_FROST_ABORT_AFTER_OUTPUTS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			afterProduced = uint64(parsed)
		}
	}
	afterInput := int64(-1)
	if raw := frostAbortConfigValue("BIFROST_FROST_ABORT_AFTER_INPUTS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			afterInput = int64(parsed)
		}
	}
	return produced >= afterProduced && (afterInput < 0 || input >= uint64(afterInput))
}

func (t *TssServer) maybeAbortFrostJoinHook(phase, msgID string) {
	if frostAbortMode() != "join" || !t.shouldTriggerFrostFault(phase, 1, 0, "") {
		return
	}
	t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Str("local_pubkey", t.localNodePubKey).Msg("FROST test abort hook exiting before join party")
	os.Exit(42)
}

func (t *TssServer) maybeAbortFrostHook(phase, msgID string, participants []string, produced, input uint64, defaultTarget string) error {
	switch frostAbortMode() {
	case "corrupt", "malformed", "auth", "result", "join":
		return nil
	}
	if !t.shouldTriggerFrostFault(phase, produced, input, defaultTarget) {
		return nil
	}
	idx := 0
	for i, participant := range participants {
		if participant == t.localNodePubKey {
			idx = i + 1
			break
		}
	}
	if idx == 0 {
		return nil
	}
	t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Str("local_pubkey", t.localNodePubKey).Int("party_index", idx).Msg("FROST test abort hook triggered")
	if frostAbortMode() == "error" {
		return fmt.Errorf("FROST test identifiable abort: protocol abort %d", idx)
	}
	os.Exit(42)
	return nil
}

func (t *TssServer) maybeCorruptFrostMessage(phase, msgID, receiver string, produced, input uint64, msg []byte) []byte {
	if frostAbortMode() != "corrupt" || !t.shouldTriggerFrostFault(phase, produced, input, "") {
		return msg
	}
	corrupted := append([]byte(nil), msg...)
	if len(corrupted) > 0 {
		corrupted[len(corrupted)-1] ^= 0x01
	}
	t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Str("receiver", receiver).Msg("FROST test hook corrupted protocol message")
	return corrupted
}

func (t *TssServer) maybeFaultFrostWireMessage(phase, msgID, receiver string, produced, input uint64, wire *frostWireMessage, encoded []byte) []byte {
	if !t.shouldTriggerFrostFault(phase, produced, input, "") {
		return encoded
	}
	switch frostAbortMode() {
	case "auth":
		if len(wire.Sig) > 0 {
			wire.Sig[len(wire.Sig)-1] ^= 0x01
		} else {
			wire.Sig = []byte{1}
		}
		faulted, err := json.Marshal(wire)
		if err != nil {
			return encoded
		}
		t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Str("receiver", receiver).Msg("FROST test hook corrupted wire signature")
		return faulted
	case "malformed":
		t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Str("receiver", receiver).Msg("FROST test hook corrupted wire payload")
		return []byte("{")
	default:
		return encoded
	}
}

func (t *TssServer) maybeFaultFrostSignResult(phase, msgID string, signatures []keysign.Signature) ([]keysign.Signature, bool) {
	if frostAbortMode() != "result" || !t.shouldTriggerFrostFault(phase, 1, 0, "") || len(signatures) == 0 {
		return signatures, false
	}
	faulted := append([]keysign.Signature(nil), signatures...)
	faulted[0].S = base64.StdEncoding.EncodeToString([]byte{1})
	t.logger.Error().Str("phase", phase).Str("msg_id", msgID).Msg("FROST test hook corrupted keysign result")
	return faulted, true
}

func frostWireSignBytes(_ string, wire frostWireMessage) ([]byte, error) {
	wire.Sig = nil
	return json.Marshal(wire)
}

func (t *TssServer) signFrostWireMessage(msgID string, wire *frostWireMessage) error {
	payload, err := frostWireSignBytes(msgID, *wire)
	if err != nil {
		return err
	}
	sig, err := frostGenerateSignature(payload, msgID, t.privateKey)
	if err != nil {
		return err
	}
	wire.Sig = sig
	return nil
}

func verifyFrostWireMessage(msgID string, wire frostWireMessage) error {
	if len(wire.Sig) == 0 {
		return errors.New("missing FROST wire signature")
	}
	pubKey, err := tcommon.NewPubKey(wire.From)
	if err != nil {
		return err
	}
	secpPubKey, err := pubKey.Secp256K1()
	if err != nil {
		return err
	}
	payload, err := frostWireSignBytes(msgID, wire)
	if err != nil {
		return err
	}
	if !frostVerifySignature(tmsecp256k1.PubKey(secpPubKey.SerializeCompressed()), payload, wire.Sig, msgID) {
		return fmt.Errorf("invalid FROST wire signature from %q", wire.From)
	}
	return nil
}

func frostGenerateSignature(msg []byte, msgID string, privKey tcrypto.PrivKey) ([]byte, error) {
	var dataForSigning bytes.Buffer
	dataForSigning.Write(msg)
	dataForSigning.WriteString(msgID)
	return privKey.Sign(dataForSigning.Bytes())
}

func frostVerifySignature(pubKey tcrypto.PubKey, message, sig []byte, msgID string) bool {
	var dataForSign bytes.Buffer
	dataForSign.Write(message)
	dataForSign.WriteString(msgID)
	return pubKey.VerifySignature(dataForSign.Bytes(), sig)
}

func frostPubKeyFromMessagePeer(raw *p2p.Message) (string, bool) {
	if raw == nil {
		return "", false
	}
	from, err := conversion.GetPubKeyFromPeerID(raw.PeerID.String())
	return from, err == nil && from != ""
}

func frostBlameError(reason string, pubkeys ...string) error {
	nodes := make([]string, 0, len(pubkeys))
	seen := make(map[string]struct{}, len(pubkeys))
	for _, pubkey := range pubkeys {
		pubkey = strings.TrimSpace(pubkey)
		if pubkey == "" {
			continue
		}
		if _, ok := seen[pubkey]; ok {
			continue
		}
		seen[pubkey] = struct{}{}
		nodes = append(nodes, pubkey)
	}
	if len(nodes) == 0 {
		return errors.New(reason)
	}
	sort.Strings(nodes)
	return fmt.Errorf("%s blame=%s", reason, strings.Join(nodes, ","))
}

func frostBlameFromErrorWithDefault(err error, participants []string, round string, defaults []string) blame.Blame {
	b := frostBlameFromError(err, participants, round)
	if len(b.BlameNodes) != 0 || len(defaults) == 0 {
		return b
	}
	nodes := make([]blame.Node, 0, len(defaults))
	for _, pubkey := range defaults {
		if pubkey != "" {
			nodes = append(nodes, blame.Node{Pubkey: pubkey})
		}
	}
	b.BlameNodes = nodes
	return b
}

func frostBlameFromError(err error, participants []string, round string) blame.Blame {
	if err == nil {
		return blame.Blame{}
	}
	reason := err.Error()
	nodes := make([]blame.Node, 0)
	if idx := strings.LastIndex(reason, "blame="); idx >= 0 {
		for _, pubkey := range strings.Split(reason[idx+len("blame="):], ",") {
			pubkey = strings.TrimSpace(pubkey)
			if pubkey != "" {
				nodes = append(nodes, blame.Node{Pubkey: pubkey})
			}
		}
	}
	if len(nodes) == 0 {
		for _, marker := range []string{"protocol abort ", "party="} {
			if idx := strings.LastIndex(reason, marker); idx >= 0 {
				raw := reason[idx+len(marker):]
				raw = strings.TrimLeft(raw, " #")
				end := 0
				for end < len(raw) && raw[end] >= '0' && raw[end] <= '9' {
					end++
				}
				if end > 0 {
					if party, parseErr := strconv.Atoi(raw[:end]); parseErr == nil && party > 0 && party <= len(participants) {
						nodes = append(nodes, blame.Node{Pubkey: participants[party-1]})
					}
				}
				break
			}
		}
	}
	return blame.Blame{
		FailReason: reason + ": frost identifiable abort",
		Round:      round,
		BlameNodes: nodes,
	}
}

func frostNodeSyncBlame(keys []string, onlinePeers []peer.ID, round string, cause error) blame.Blame {
	nodes := make([]blame.Node, 0)
	for _, key := range keys {
		peerID, err := conversion.GetPeerIDFromPubKey(key)
		if err != nil {
			return frostBlameFromError(cause, keys, round)
		}
		found := false
		for _, online := range onlinePeers {
			if online == peerID {
				found = true
				break
			}
		}
		if !found {
			nodes = append(nodes, blame.Node{Pubkey: key})
		}
	}
	if len(nodes) == 0 {
		return frostBlameFromError(cause, keys, round)
	}
	reason := blame.TssSyncFail
	if cause != nil {
		reason = cause.Error() + ": frost node sync abort"
	}
	return blame.Blame{
		FailReason: reason,
		Round:      round,
		BlameNodes: nodes,
	}
}

func frostMissingParticipants(participants []string, localID string, inputFrom map[string]int) string {
	missing := make([]string, 0)
	for _, participant := range participants {
		if participant == localID {
			continue
		}
		if inputFrom[participant] == 0 {
			missing = append(missing, participant)
		}
	}
	sort.Strings(missing)
	return strings.Join(missing, ",")
}

func (t *TssServer) sendFrostMessage(msgID string, msgType messages.THORChainTSSMessageType, index int, from, to string, msg []byte, peers []peer.ID, phase string, produced, input uint64) error {
	wireMsg := frostWireMessage{Kind: "session", From: from, To: to, Index: index, Message: msg}
	if err := t.signFrostWireMessage(msgID, &wireMsg); err != nil {
		return err
	}
	wire, err := json.Marshal(wireMsg)
	if err != nil {
		return err
	}
	wire = t.maybeFaultFrostWireMessage(phase, msgID, to, produced, input, &wireMsg, wire)
	t.p2pCommunication.BroadcastMsgChan <- &messages.BroadcastMsgChan{
		WrappedMessage: messages.WrappedMessage{MsgID: msgID, MessageType: msgType, Payload: wire},
		PeersID:        peers,
	}
	return nil
}

func (t *TssServer) runFrostSession(msgID string, msgType messages.THORChainTSSMessageType, index int, ch chan *p2p.Message, session frostsession.Handle, localID string, participants []string, timeout time.Duration, phase, defaultAbortPubKey string) ([]byte, error) {
	defer frostsession.SessionFree(session)
	participantSet := make(map[string]struct{}, len(participants))
	for _, p := range participants {
		participantSet[p] = struct{}{}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	sessionFinished := false
	var produced, sent, received, input, selfInput uint64
	inputFrom := make(map[string]int, len(participants))

	var drainOutput func() ([]byte, error)
	drainOutput = func() ([]byte, error) {
		for {
			out, err := frostsession.SessionOutputMessage(session)
			if err != nil {
				return nil, err
			}
			if len(out) == 0 {
				if sessionFinished {
					return frostsession.SessionFinish(session)
				}
				return nil, nil
			}
			produced++
			for idx := 0; idx < len(participants); idx++ {
				receiver, err := frostsession.SessionMessageReceiver(session, out, idx)
				if err != nil {
					return nil, err
				}
				if receiver == "" {
					break
				}
				if _, ok := participantSet[receiver]; !ok {
					return nil, fmt.Errorf("FROST receiver %q is not a participant", receiver)
				}
				if receiver == localID {
					if sessionFinished {
						continue
					}
					finished, err := frostsession.SessionInputMessage(session, out)
					if err != nil {
						return nil, err
					}
					input++
					selfInput++
					if err := t.maybeAbortFrostHook(phase, msgID, participants, produced, input, defaultAbortPubKey); err != nil {
						return nil, err
					}
					if finished {
						sessionFinished = true
					}
					if result, err := drainOutput(); err != nil || result != nil {
						return result, err
					}
					continue
				}
				peerID, err := conversion.GetPeerIDFromPubKey(receiver)
				if err != nil {
					return nil, err
				}
				outgoing := t.maybeCorruptFrostMessage(phase, msgID, receiver, produced, input, out)
				if err := t.sendFrostMessage(msgID, msgType, index, localID, receiver, outgoing, []peer.ID{peerID}, phase, produced, input); err != nil {
					return nil, err
				}
				sent++
				if err := t.maybeAbortFrostHook(phase, msgID, participants, produced, input, defaultAbortPubKey); err != nil {
					return nil, err
				}
			}
		}
	}
	if result, err := drainOutput(); err != nil || result != nil {
		return result, err
	}

	for {
		select {
		case <-ticker.C:
			if result, err := drainOutput(); err != nil || result != nil {
				return result, err
			}
		case raw := <-ch:
			received++
			from, hasFrom := frostPubKeyFromMessagePeer(raw)
			var wrapped messages.WrappedMessage
			if err := json.Unmarshal(raw.Payload, &wrapped); err != nil {
				if hasFrom {
					return nil, frostBlameError(fmt.Sprintf("invalid FROST wrapper from %q: %v", from, err), from)
				}
				return nil, err
			}
			var wire frostWireMessage
			if err := json.Unmarshal(wrapped.Payload, &wire); err != nil {
				if hasFrom {
					return nil, frostBlameError(fmt.Sprintf("invalid FROST wire from %q: %v", from, err), from)
				}
				return nil, err
			}
			if wire.To != "" && wire.To != localID {
				continue
			}
			if sessionFinished {
				continue
			}
			if !hasFrom {
				return nil, fmt.Errorf("fail to get FROST pubkey from peer %q", raw.PeerID)
			}
			if from != wire.From {
				return nil, frostBlameError(fmt.Sprintf("FROST sender mismatch: peer %q is %q but payload says %q", raw.PeerID, from, wire.From), from)
			}
			if _, ok := participantSet[wire.From]; !ok {
				return nil, frostBlameError(fmt.Sprintf("FROST sender %q is not a participant", wire.From), from)
			}
			if wire.Kind != "session" {
				return nil, frostBlameError(fmt.Sprintf("unexpected FROST message kind from %q: %q", from, wire.Kind), from)
			}
			if wire.Index != index {
				return nil, frostBlameError(fmt.Sprintf("unexpected FROST message index from %q: got %d want %d", from, wire.Index, index), from)
			}
			if err := verifyFrostWireMessage(wrapped.MsgID, wire); err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST wire signature from %q: %v", from, err), from)
			}
			finished, err := frostsession.SessionInputMessage(session, wire.Message)
			if err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST protocol message from %q: %v", from, err), from)
			}
			input++
			inputFrom[wire.From]++
			if err := t.maybeAbortFrostHook(phase, msgID, participants, produced, input, defaultAbortPubKey); err != nil {
				return nil, err
			}
			if finished {
				sessionFinished = true
			}
			if result, err := drainOutput(); err != nil || result != nil {
				return result, err
			}
		case <-timer.C:
			froms := make([]string, 0, len(inputFrom))
			for from, count := range inputFrom {
				froms = append(froms, fmt.Sprintf("%s=%d", from, count))
			}
			sort.Strings(froms)
			missing := frostMissingParticipants(participants, localID, inputFrom)
			reason := fmt.Sprintf("FROST session timeout: produced=%d sent=%d received=%d input=%d self_input=%d finished=%t queued=%d froms=%s missing=%s", produced, sent, received, input, selfInput, sessionFinished, len(ch), strings.Join(froms, ","), missing)
			if missing != "" {
				return nil, frostBlameError(reason, strings.Split(missing, ",")...)
			}
			return nil, errors.New(reason)
		case <-t.stopChan:
			return nil, errors.New("received exit signal")
		}
	}
}

func (t *TssServer) FrostBTCKeygen(req keygen.Request) (keygen.Response, error) {
	t.tssKeyGenLocker.Lock()
	defer t.tssKeyGenLocker.Unlock()
	if len(req.Keys) == 0 {
		return keygen.Response{}, fmt.Errorf("frost keygen requires participants")
	}
	msgID, err := t.requestToMsgId(req)
	if err != nil {
		return keygen.Response{}, err
	}
	msgID += ":" + storage.SigningEngineFrost
	t.maybeAbortFrostJoinHook(frostAbortPhaseKeygen, msgID)

	msgChan := make(chan *p2p.Message, 512)
	sigChan := make(chan string, 1)
	t.p2pCommunication.SetSubscribe(messages.TSSFrostKeyGenMsg, msgID, msgChan)
	t.p2pCommunication.SetSubscribe(messages.TSSControlMsg, msgID, msgChan)
	t.p2pCommunication.SetSubscribe(messages.TSSTaskDone, msgID, msgChan)
	defer func() {
		t.p2pCommunication.CancelSubscribe(messages.TSSFrostKeyGenMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSControlMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSTaskDone, msgID)
		t.p2pCommunication.ReleaseStream(msgID)
		t.partyCoordinator.ReleaseStream(msgID)
		close(sigChan)
	}()

	start := time.Now()
	participants := frostIDs(req.Keys)
	onlinePeers, _, errJoinParty := t.joinParty(msgID, req.Version, req.BlockHeight, participants, len(participants)-1, sigChan)
	t.tssMetrics.KeygenJoinParty(time.Since(start), errJoinParty == nil)
	if errJoinParty != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, frostNodeSyncBlame(participants, onlinePeers, frostKeygenAbortRound, errJoinParty)), nil
	}
	threshold, err := frostThreshold(len(participants))
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, blame.NewBlame(err.Error(), nil)), nil
	}
	t.logger.Info().
		Str("msg_id", msgID).
		Int("participants", len(participants)).
		Strs("participant_keys", participants).
		Int("threshold", threshold).
		Bool("leader", len(participants) > 0 && participants[0] == t.localNodePubKey).
		Msg("FROST keygen ceremony started")
	session, err := frostsession.KeygenSessionNew(participants, t.localNodePubKey, uint16(threshold))
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, frostBlameFromErrorWithDefault(err, participants, frostKeygenAbortRound, []string{t.localNodePubKey})), nil
	}
	share, err := t.runFrostSession(msgID, messages.TSSFrostKeyGenMsg, 0, msgChan, session, t.localNodePubKey, participants, t.conf.KeyGenTimeout, frostAbortPhaseKeygen, t.localNodePubKey)
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, frostBlameFromErrorWithDefault(err, participants, frostKeygenAbortRound, []string{t.localNodePubKey})), nil
	}
	decoded, err := frostsession.DecodeKeyshare(share)
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, blame.NewBlame(err.Error(), nil)), nil
	}
	pubKeyBytes, err := hex.DecodeString(decoded.PublicKeyCompressed)
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, blame.NewBlame(err.Error(), nil)), nil
	}
	pubKey, addr, err := frostPubKeyFromCompressed(pubKeyBytes)
	if err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, blame.NewBlame(err.Error(), nil)), nil
	}
	state := storage.KeygenLocalState{
		PubKey:          pubKey,
		LocalData:       share,
		ParticipantKeys: participants,
		LocalPartyKey:   t.localNodePubKey,
		SigningEngine:   storage.SigningEngineFrost,
	}
	if err := t.stateManager.SaveLocalState(state); err != nil {
		t.tssMetrics.UpdateKeyGen(0, false)
		return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, "", "", common.Fail, blame.NewBlame(err.Error(), nil)), nil
	}
	t.persistPeerAddressBook()
	t.tssMetrics.UpdateKeyGen(time.Since(start), true)
	t.logger.Info().Str("msg_id", msgID).Str("pubkey", pubKey).Msg("FROST keygen ceremony party produced")
	return keygen.NewResponse(tcommon.SigningAlgoSecp256k1, pubKey, addr.String(), common.Success, blame.Blame{}), nil
}

func (t *TssServer) FrostBTCKeySign(req keysign.Request, localStateItem storage.KeygenLocalState) (keysign.Response, error) {
	msgID, err := t.requestToMsgId(req)
	if err != nil {
		return keysign.Response{}, err
	}
	msgID += ":" + storage.SigningEngineFrost
	t.maybeAbortFrostJoinHook(frostAbortPhaseKeysign, msgID)
	allParticipants := frostSignerPubKeys(req, localStateItem)
	if len(allParticipants) == 0 {
		allParticipants = localStateItem.ParticipantKeys
	}
	allParticipants = frostIDs(allParticipants)
	defaultBlame := t.localNodePubKey
	if len(allParticipants) > 0 {
		defaultBlame = allParticipants[0]
	}
	threshold, err := conversion.GetThreshold(len(allParticipants))
	if err != nil {
		return keysign.Response{}, err
	}

	msgChan := make(chan *p2p.Message, 512)
	resultChan := make(chan *p2p.Message, 32)
	t.p2pCommunication.SetSubscribe(messages.TSSFrostKeySignMsg, msgID, msgChan)
	t.p2pCommunication.SetSubscribe(messages.TSSFrostKeySignResultMsg, msgID, resultChan)
	defer func() {
		t.p2pCommunication.CancelSubscribe(messages.TSSFrostKeySignMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSFrostKeySignResultMsg, msgID)
	}()
	sigChan := make(chan string, 1)
	onlinePeers, _, err := t.joinParty(msgID, req.Version, req.BlockHeight, allParticipants, threshold, sigChan)
	if err != nil {
		return keysign.Response{
			Status: common.Fail,
			Blame:  frostNodeSyncBlame(allParticipants, onlinePeers, frostKeysignAbortRound, err),
		}, nil
	}
	signers, err := conversion.GetPubKeysFromPeerIDs(peerIDsToStrings(onlinePeers))
	if err != nil {
		return keysign.Response{}, err
	}
	signers = frostIDs(signers)
	if len(signers) == 0 {
		signers = allParticipants
	}
	defaultBlame = signers[0]

	resultWaitDone := make(chan struct{})
	defer close(resultWaitDone)
	type frostResult struct {
		signatures []keysign.Signature
		err        error
	}
	resultDone := make(chan frostResult, 1)
	go func() {
		sigs, err := t.receiveFrostSignResult(resultChan, allParticipants, req.PoolPubKey, req.Messages, resultWaitDone)
		resultDone <- frostResult{signatures: sigs, err: err}
	}()
	if !frostContains(signers, t.localNodePubKey) {
		result := <-resultDone
		if result.err != nil {
			return keysign.Response{Status: common.Fail, Blame: frostBlameFromErrorWithDefault(result.err, signers, frostKeysignAbortRound, []string{defaultBlame})}, nil
		}
		return keysign.NewResponse(result.signatures, common.Success, blame.Blame{}), nil
	}

	signatures := make([]keysign.Signature, 0, len(req.Messages))
	var generatedErr error
	for idx, encoded := range req.Messages {
		msg, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			generatedErr = fmt.Errorf("fail to decode frost message: %w", err)
			break
		}
		if len(msg) != 32 {
			digest := sha256.Sum256(msg)
			msg = digest[:]
		}
		session, err := frostsession.SignSessionNew(signers, t.localNodePubKey, localStateItem.LocalData, msg)
		if err != nil {
			generatedErr = err
			break
		}
		t.logger.Info().
			Str("msg_id", msgID).
			Str("pool_pub_key", req.PoolPubKey).
			Int("message_index", idx).
			Int("participants", len(signers)).
			Bool("leader", len(signers) > 0 && signers[0] == t.localNodePubKey).
			Msg("FROST keysign ceremony started")
		sig, err := t.runFrostSession(msgID, messages.TSSFrostKeySignMsg, idx, msgChan, session, t.localNodePubKey, signers, t.conf.KeySignTimeout, frostAbortPhaseKeysign, defaultBlame)
		if err != nil {
			generatedErr = err
			break
		}
		share, err := frostsession.DecodeKeyshare(localStateItem.LocalData)
		if err != nil {
			generatedErr = err
			break
		}
		pubKeyBytes, err := hex.DecodeString(share.PublicKeyCompressed)
		if err != nil {
			generatedErr = err
			break
		}
		if err := frostsession.Verify(pubKeyBytes, msg, sig); err != nil {
			generatedErr = err
			break
		}
		signatures = append(signatures, keysign.NewSignature(encoded, "", base64.StdEncoding.EncodeToString(sig), ""))
	}
	if generatedErr == nil {
		if err := t.broadcastFrostSignResult(msgID, t.localNodePubKey, signatures, allParticipants); err != nil {
			return keysign.NewResponse(nil, common.Fail, frostBlameFromErrorWithDefault(err, signers, frostKeysignAbortRound, []string{t.localNodePubKey})), nil
		}
		t.logger.Info().Str("pool_pub_key", req.PoolPubKey).Int("messages", len(signatures)).Msg("FROST keysign ceremony party produced")
		return keysign.NewResponse(signatures, common.Success, blame.Blame{}), nil
	}
	result := <-resultDone
	if result.err == nil {
		return keysign.NewResponse(result.signatures, common.Success, blame.Blame{}), nil
	}
	return keysign.NewResponse(nil, common.Fail, frostBlameFromErrorWithDefault(generatedErr, signers, frostKeysignAbortRound, []string{defaultBlame})), nil
}

func frostSignerPubKeys(req keysign.Request, localStateItem storage.KeygenLocalState) []string {
	if len(req.SignerPubKeys) > 0 {
		return req.SignerPubKeys
	}
	return localStateItem.ParticipantKeys
}

func (t *TssServer) broadcastFrostSignResult(msgID, from string, signatures []keysign.Signature, participants []string) error {
	signatures, faulted := t.maybeFaultFrostSignResult(frostAbortPhaseKeysign, msgID, signatures)
	result, err := json.Marshal(frostSignResultMessage{Signatures: signatures})
	if err != nil {
		return err
	}
	wireMsg := frostWireMessage{Kind: "result", From: from, Message: result}
	if err := t.signFrostWireMessage(msgID, &wireMsg); err != nil {
		return err
	}
	wire, err := json.Marshal(wireMsg)
	if err != nil {
		return err
	}
	peers, err := conversion.GetPeerIDsFromPubKeys(participants)
	if err != nil {
		return err
	}
	t.p2pCommunication.BroadcastMsgChan <- &messages.BroadcastMsgChan{
		WrappedMessage: messages.WrappedMessage{MsgID: msgID, MessageType: messages.TSSFrostKeySignResultMsg, Payload: wire},
		PeersID:        peers,
	}
	if faulted {
		return frostBlameError("FROST test bad keysign result broadcaster", from)
	}
	return nil
}

func (t *TssServer) receiveFrostSignResult(ch chan *p2p.Message, participants []string, poolPubKey string, encodedMsgs []string, done <-chan struct{}) ([]keysign.Signature, error) {
	participantSet := make(map[string]struct{}, len(participants))
	for _, p := range participants {
		participantSet[p] = struct{}{}
	}
	timer := time.NewTimer(t.conf.KeySignTimeout)
	defer timer.Stop()
	for {
		select {
		case raw := <-ch:
			from, hasFrom := frostPubKeyFromMessagePeer(raw)
			var wrapped messages.WrappedMessage
			if err := json.Unmarshal(raw.Payload, &wrapped); err != nil {
				if hasFrom {
					return nil, frostBlameError(fmt.Sprintf("invalid FROST keysign result wrapper from %q: %v", from, err), from)
				}
				return nil, err
			}
			if wrapped.MessageType != messages.TSSFrostKeySignResultMsg {
				return nil, fmt.Errorf("unexpected FROST keysign result message type: %s", wrapped.MessageType)
			}
			if !hasFrom {
				return nil, fmt.Errorf("fail to get FROST keysign result pubkey from peer %q", raw.PeerID)
			}
			if _, ok := participantSet[from]; !ok {
				return nil, frostBlameError(fmt.Sprintf("FROST keysign result sender %q is not a participant", from), from)
			}
			var wire frostWireMessage
			if err := json.Unmarshal(wrapped.Payload, &wire); err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST keysign result wire from %q: %v", from, err), from)
			}
			if wire.Kind != "result" {
				return nil, frostBlameError(fmt.Sprintf("unexpected FROST keysign result kind from %q: %q", from, wire.Kind), from)
			}
			if from != wire.From {
				return nil, frostBlameError(fmt.Sprintf("FROST keysign result sender mismatch: peer %q is %q but payload says %q", raw.PeerID, from, wire.From), from)
			}
			if err := verifyFrostWireMessage(wrapped.MsgID, wire); err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST keysign result signature from %q: %v", from, err), from)
			}
			var result frostSignResultMessage
			if err := json.Unmarshal(wire.Message, &result); err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST keysign result payload from %q: %v", from, err), from)
			}
			if err := validateFrostSignatures(poolPubKey, result.Signatures, encodedMsgs); err != nil {
				return nil, frostBlameError(fmt.Sprintf("invalid FROST keysign result signatures from %q: %v", from, err), from)
			}
			return result.Signatures, nil
		case <-timer.C:
			return nil, errors.New("timeout waiting for FROST keysign result")
		case <-done:
			return nil, errors.New("FROST keysign result wait cancelled")
		case <-t.stopChan:
			return nil, errors.New("received exit signal")
		}
	}
}

func validateFrostSignatures(poolPubKey string, signatures []keysign.Signature, encodedMsgs []string) error {
	if len(signatures) != len(encodedMsgs) {
		return fmt.Errorf("FROST keysign result count mismatch: got %d want %d", len(signatures), len(encodedMsgs))
	}
	pk, err := tcommon.NewPubKey(poolPubKey)
	if err != nil {
		return err
	}
	secpPubKey, err := pk.Secp256K1()
	if err != nil {
		return err
	}
	pubKeyBytes := secpPubKey.SerializeCompressed()
	msgs := make(map[string][]byte, len(encodedMsgs))
	remaining := make(map[string]int, len(encodedMsgs))
	for _, encoded := range encodedMsgs {
		msg, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return err
		}
		if len(msg) != 32 {
			digest := sha256.Sum256(msg)
			msg = digest[:]
		}
		msgs[encoded] = msg
		remaining[encoded]++
	}
	for _, sig := range signatures {
		msg, ok := msgs[sig.Msg]
		if !ok {
			return errors.New("FROST keysign result contains unknown message")
		}
		if remaining[sig.Msg] == 0 {
			return errors.New("FROST keysign result contains too many signatures for message")
		}
		remaining[sig.Msg]--
		sigBytes, err := base64.StdEncoding.DecodeString(sig.S)
		if err != nil {
			return err
		}
		if err := frostsession.Verify(pubKeyBytes, msg, sigBytes); err != nil {
			return err
		}
	}
	for encoded, count := range remaining {
		if count != 0 {
			return fmt.Errorf("FROST keysign result missing %d signature(s) for message %q", count, encoded)
		}
	}
	return nil
}

func frostPubKeyFromCompressed(pubKeyBytes []byte) (string, cosmos.AccAddress, error) {
	compressedPubkey := coskey.PubKey{Key: pubKeyBytes}
	pubKey, err := sdk.MarshalPubKey(sdk.AccPK, &compressedPubkey) //nolint:staticcheck
	if err != nil {
		return "", nil, err
	}
	return pubKey, cosmos.AccAddress(compressedPubkey.Address().Bytes()), nil
}

func frostContains(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}
