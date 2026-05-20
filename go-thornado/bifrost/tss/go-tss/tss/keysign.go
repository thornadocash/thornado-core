package tss

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	tsslibcommon "github.com/binance-chain/tss-lib/common"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/p2p/messages"
	"github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/blame"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/common"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign/ecdsa"
	"github.com/thornadocash/go-thornado/bifrost/tss/go-tss/keysign/eddsa"
	tcommon "github.com/thornadocash/go-thornado/common"
)

func (t *TssServer) waitForSignatures(msgID, poolPubKey string, msgsToSign [][]byte, algo tcommon.SigningAlgo, sigChan chan string) (keysign.Response, error) {
	// TSS keysign include both form party and keysign itself, thus we wait twice of the timeout
	data, err := t.signatureNotifier.WaitForSignature(msgID, msgsToSign, poolPubKey, t.conf.KeySignTimeout, sigChan)
	if err != nil {
		return keysign.Response{}, err
	}
	// for gg20, it wrap the signature R,S into ECSignature structure
	if len(data) == 0 {
		return keysign.Response{}, errors.New("keysign failed")
	}

	return t.batchSignatures(algo, data, msgsToSign), nil
}

func (t *TssServer) generateSignature(
	msgID string,
	msgsToSign [][]byte,
	req keysign.Request,
	threshold int,
	allParticipants []string,
	localStateItem storage.KeygenLocalState,
	blameMgr *blame.Manager,
	keysignInstance keysign.TssKeySign,
	sigChan chan string,
) (keysign.Response, error) {
	allPeersID, err := conversion.GetPeerIDsFromPubKeys(allParticipants)
	if err != nil {
		t.logger.Error().Msg("invalid block height or public key")
		return keysign.Response{
			Status: common.Fail,
			Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
		}, nil
	}

	oldJoinParty, err := conversion.VersionLTCheck(req.Version, messages.NEWJOINPARTYVERSION)
	if err != nil {
		return keysign.Response{
			Status: common.Fail,
			Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
		}, errors.New("fail to parse the version")
	}

	ourPeerString := t.p2pCommunication.GetHost().ID().String()

	// we use the old join party
	if oldJoinParty {
		allParticipants = req.SignerPubKeys
		myPk, err := conversion.GetPubKeyFromPeerID(ourPeerString)
		if err != nil {
			t.logger.Info().Msgf("fail to convert the p2p id(%s) to pubkey, turn to wait for signature", ourPeerString)
			return keysign.Response{}, p2p.ErrNotActiveSigner
		}
		isSignMember := false
		for _, el := range allParticipants {
			if myPk == el {
				isSignMember = true
				break
			}
		}
		if !isSignMember {
			t.logger.Info().Msgf("we(%s) are not the active signer", ourPeerString)
			return keysign.Response{}, p2p.ErrNotActiveSigner
		}

	}

	joinPartyStartTime := time.Now()
	onlinePeers, leader, errJoinParty := t.joinParty(msgID, req.Version, req.BlockHeight, allParticipants, threshold, sigChan)
	joinPartyTime := time.Since(joinPartyStartTime)
	if errJoinParty != nil {
		// we received the signature from waiting for signature
		if errors.Is(errJoinParty, p2p.ErrSignReceived) {
			return keysign.Response{}, errJoinParty
		}
		t.tssMetrics.KeysignJoinParty(joinPartyTime, false)
		// this indicate we are processing the leaderness join party
		if leader == "NONE" {
			if onlinePeers == nil {
				t.logger.Error().Err(errJoinParty).Msg("error before we start join party")
				t.broadcastKeysignFailure(msgID, allPeersID)
				return keysign.Response{
					Status: common.Fail,
					Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
				}, nil
			}

			blameNodes, err := blameMgr.NodeSyncBlame(req.SignerPubKeys, onlinePeers)
			if err != nil {
				t.logger.Err(err).Msg("fail to get peers to blame")
			}
			t.broadcastKeysignFailure(msgID, allPeersID)
			// make sure we blame the leader as well
			t.logger.Error().Err(err).Msgf("fail to form keysign party with online:%v", onlinePeers)
			return keysign.Response{
				Status: common.Fail,
				Blame:  blameNodes,
			}, nil
		}

		var blameLeader blame.Blame
		leaderPubKey, err := conversion.GetPubKeyFromPeerID(leader)
		if err != nil {
			t.logger.Error().Err(errJoinParty).Msgf("fail to convert the peerID to public key %s", leader)
			blameLeader = blame.NewBlame(blame.TssSyncFail, []blame.Node{})
		} else {
			blameLeader = blame.NewBlame(blame.TssSyncFail, []blame.Node{{
				Pubkey:         leaderPubKey,
				BlameData:      nil,
				BlameSignature: nil,
			}})
		}

		t.broadcastKeysignFailure(msgID, allPeersID)
		// make sure we blame the leader as well
		t.logger.Error().Err(errJoinParty).Msgf("messagesID(%s)fail to form keysign party with online:%v", msgID, onlinePeers)
		return keysign.Response{
			Status: common.Fail,
			Blame:  blameLeader,
		}, nil

	}
	t.tssMetrics.KeysignJoinParty(joinPartyTime, true)
	isKeySignMember := false
	parsedPeers := make([]string, len(onlinePeers))
	for i, el := range onlinePeers {
		parsedPeers[i] = el.String()
		if el.String() == ourPeerString {
			isKeySignMember = true
		}
	}
	if !isKeySignMember {
		// we are not the keysign member so we quit keysign and waiting for signature
		t.logger.Info().Msgf("we(%s) are not the active signer", ourPeerString)
		return keysign.Response{}, p2p.ErrNotActiveSigner
	}

	signers, err := conversion.GetPubKeysFromPeerIDs(parsedPeers)
	if err != nil {
		sigChan <- "signature generated"
		return keysign.Response{
			Status: common.Fail,
			Blame:  blame.Blame{},
		}, nil
	}
	signatureData, err := keysignInstance.SignMessage(msgsToSign, localStateItem, signers)
	// the statistic of keygen only care about Tss it self, even if the following http response aborts,
	// it still counted as a successful keygen as the Tss model runs successfully.
	if err != nil {
		t.logger.Error().Err(err).Msg("err in keysign")
		sigChan <- "signature generated"
		t.broadcastKeysignFailure(msgID, allPeersID)
		blameNodes := *blameMgr.GetBlame()
		return keysign.Response{
			Status: common.Fail,
			Blame:  blameNodes,
		}, nil
	}

	// sigChan <- "signature generated"
	// update signature notification
	if err := t.signatureNotifier.BroadcastSignature(msgID, signatureData, allPeersID); err != nil {
		return keysign.Response{}, fmt.Errorf("fail to broadcast signature:%w", err)
	}

	return t.batchSignatures(tcommon.SigningAlgo(req.Algo), signatureData, msgsToSign), nil
}

func (t *TssServer) updateKeySignResult(result keysign.Response, timeSpent time.Duration) {
	if result.Status == common.Success {
		t.tssMetrics.UpdateKeySign(timeSpent, true)
		return
	}
	t.tssMetrics.UpdateKeySign(timeSpent, false)
}

func (t *TssServer) KeySign(req keysign.Request) (keysign.Response, error) {
	t.logger.Info().Str("pool pub key", req.PoolPubKey).
		Str("signer pub keys", strings.Join(req.SignerPubKeys, ",")).
		Str("msg", strings.Join(req.Messages, ",")).
		Msg("received keysign request")
	emptyResp := keysign.Response{}
	msgID, err := t.requestToMsgId(req)
	if err != nil {
		return emptyResp, err
	}

	var keysignInstance keysign.TssKeySign

	algo := tcommon.SigningAlgo(req.Algo)

	ourPeerID := t.p2pCommunication.GetLocalPeerID()

	switch algo {
	case tcommon.SigningAlgoSecp256k1:
		keysignInstance = ecdsa.NewTssKeySign(
			ourPeerID,
			t.conf,
			t.p2pCommunication.BroadcastMsgChan,
			t.stopChan,
			msgID,
			t.privateKey,
			t.p2pCommunication,
			t.stateManager,
			len(req.Messages),
		)
	case tcommon.SigningAlgoEd25519:
		keysignInstance = eddsa.NewTssKeySign(
			ourPeerID,
			t.conf,
			t.p2pCommunication.BroadcastMsgChan,
			t.stopChan,
			msgID,
			t.privateKey,
			t.p2pCommunication,
			t.stateManager,
			len(req.Messages),
		)
	default:
		return keysign.Response{}, fmt.Errorf("unsupported signing algo: %s", algo)
	}

	keySignChannels := keysignInstance.GetTssKeySignChannels()
	t.p2pCommunication.SetSubscribe(messages.TSSKeySignMsg, msgID, keySignChannels)
	t.p2pCommunication.SetSubscribe(messages.TSSKeySignVerMsg, msgID, keySignChannels)
	t.p2pCommunication.SetSubscribe(messages.TSSControlMsg, msgID, keySignChannels)
	t.p2pCommunication.SetSubscribe(messages.TSSTaskDone, msgID, keySignChannels)

	defer func() {
		t.p2pCommunication.CancelSubscribe(messages.TSSKeySignMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSKeySignVerMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSControlMsg, msgID)
		t.p2pCommunication.CancelSubscribe(messages.TSSTaskDone, msgID)

		t.p2pCommunication.ReleaseStream(msgID)
		t.signatureNotifier.ReleaseStream(msgID)
		t.partyCoordinator.ReleaseStream(msgID)
	}()

	localStateItem, err := t.stateManager.GetLocalState(req.PoolPubKey)
	if err != nil {
		return emptyResp, fmt.Errorf("fail to get local keygen state: %w", err)
	}

	var msgsToSign [][]byte
	for _, val := range req.Messages {
		msgToSign, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			return keysign.Response{}, fmt.Errorf("fail to decode message(%s): %w", strings.Join(req.Messages, ","), err)
		}
		msgsToSign = append(msgsToSign, msgToSign)
	}

	sort.SliceStable(msgsToSign, func(i, j int) bool {
		ma, err := common.MsgToHashInt(msgsToSign[i], algo)
		if err != nil {
			t.logger.Error().Err(err).Msgf("fail to convert the hash value")
		}
		mb, err := common.MsgToHashInt(msgsToSign[j], algo)
		if err != nil {
			t.logger.Error().Err(err).Msgf("fail to convert the hash value")
		}
		return ma.Cmp(mb) == -1
	})

	oldJoinParty, err := conversion.VersionLTCheck(req.Version, messages.NEWJOINPARTYVERSION)
	if err != nil {
		return keysign.Response{
			Status: common.Fail,
			Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
		}, errors.New("fail to parse the version")
	}

	if len(req.SignerPubKeys) == 0 && oldJoinParty {
		return emptyResp, errors.New("empty signer pub keys")
	}

	threshold, err := conversion.GetThreshold(len(localStateItem.ParticipantKeys))
	if err != nil {
		t.logger.Error().Err(err).Msg("fail to get the threshold")
		return emptyResp, errors.New("fail to get threshold")
	}
	if len(req.SignerPubKeys) <= threshold && oldJoinParty {
		t.logger.Error().Msgf("not enough signers, threshold=%d and signers=%d", threshold, len(req.SignerPubKeys))
		return emptyResp, errors.New("not enough signers")
	}

	blameMgr := keysignInstance.GetTssCommonStruct().GetBlameMgr()

	var receivedSig, generatedSig keysign.Response
	var errWait, errGen error
	sigChan := make(chan string, 2)
	wg := sync.WaitGroup{}
	wg.Add(2)
	keysignStartTime := time.Now()
	msgsCopy := make([][]byte, len(msgsToSign))
	for i, msg := range msgsToSign {
		msgsCopy[i] = make([]byte, len(msg))
		copy(msgsCopy[i], msg)
	}
	// we wait for signatures
	go func() {
		defer wg.Done()
		receivedSig, errWait = t.waitForSignatures(msgID, req.PoolPubKey, msgsCopy, algo, sigChan)
		// we received an valid signature indeed
		if errWait == nil {
			sigChan <- "signature received"
			t.logger.Debug().Msgf("received signature for messageID (%s) from peer", msgID)
			return
		}
		if errWait != p2p.ErrSigGenerated {
			t.logger.Error().Err(errWait).Msg("waitForSignatures returned error")
		}
	}()

	// we generate the signature ourselves
	go func() {
		defer wg.Done()
		generatedSig, errGen = t.generateSignature(msgID, msgsToSign, req, threshold, localStateItem.ParticipantKeys, localStateItem, blameMgr, keysignInstance, sigChan)
	}()
	wg.Wait()
	close(sigChan)
	keysignTime := time.Since(keysignStartTime)
	// we received the generated verified signature, so we return
	if errWait == nil {
		t.updateKeySignResult(receivedSig, keysignTime)
		return receivedSig, nil
	}
	// for this round, we are not the active signer
	if errors.Is(errGen, p2p.ErrSignReceived) || errors.Is(errGen, p2p.ErrNotActiveSigner) {
		t.updateKeySignResult(receivedSig, keysignTime)
		return receivedSig, nil
	}
	// we get the signature from our tss keysign
	t.updateKeySignResult(generatedSig, keysignTime)
	return generatedSig, errGen
}

func (t *TssServer) broadcastKeysignFailure(messageID string, peers []peer.ID) {
	if err := t.signatureNotifier.BroadcastFailed(messageID, peers); err != nil {
		t.logger.Err(err).Msg("fail to broadcast keysign failure")
	}
}

func (t *TssServer) batchSignatures(algo tcommon.SigningAlgo, sigs []*tsslibcommon.SignatureData, msgsToSign [][]byte) keysign.Response {
	// SECURITY FIX (Layer 2): Content-based signature pairing instead of index-based.
	// Each signature contains the hash of the message it signed in the M field.
	// We must match signatures to messages by content, not by array index,
	// because array ordering may differ across nodes due to goroutine timing.

	// Create a map of message hash (as string) to original message for content-based pairing
	msgMap := make(map[string][]byte)
	for _, msg := range msgsToSign {
		hashInt, err := common.MsgToHashInt(msg, algo)
		if err != nil {
			t.logger.Error().Err(err).Msg("fail to convert message to hash")
			return keysign.Response{
				Status: common.Fail,
				Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
			}
		}
		hashKey := hashInt.String()
		msgMap[hashKey] = msg
	}

	var signatures []keysign.Signature
	// Pair signatures with messages using the M field (message hash)
	for _, sig := range sigs {
		sigg := sig.GetSignature()

		// Find the original message that matches this signature's hash
		sigHashInt := new(big.Int).SetBytes(sigg.M)
		sigHashKey := sigHashInt.String()
		originalMsg, found := msgMap[sigHashKey]
		if !found {
			t.logger.Error().
				Str("sig_hash", sigHashKey).
				Msg("signature hash does not match any input message - signature pairing failed")
			return keysign.Response{
				Status: common.Fail,
				Blame:  blame.NewBlame(blame.InternalError, []blame.Node{}),
			}
		}

		var signature keysign.Signature
		switch algo {
		case tcommon.SigningAlgoSecp256k1:
			msg := base64.StdEncoding.EncodeToString(originalMsg)
			r := base64.StdEncoding.EncodeToString(sigg.R)
			s := base64.StdEncoding.EncodeToString(sigg.S)
			recovery := base64.StdEncoding.EncodeToString(sigg.GetSignatureRecovery())
			signature = keysign.NewSignature(msg, r, s, recovery)
		case tcommon.SigningAlgoEd25519:
			msg := base64.StdEncoding.EncodeToString(originalMsg)
			s := base64.StdEncoding.EncodeToString(sigg.Signature)
			signature = keysign.NewSignature(msg, "", s, "")
		}
		signatures = append(signatures, signature)
	}
	return keysign.NewResponse(
		signatures,
		common.Success,
		blame.Blame{},
	)
}
