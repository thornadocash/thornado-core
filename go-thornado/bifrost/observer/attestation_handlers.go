package observer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/rs/zerolog"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/common"
)

// AttestObservedTx creates and broadcasts an attestation for an observed transaction
func (s *AttestationGossip) AttestObservedTx(ctx context.Context, obsTx *common.ObservedTx, inbound, allowFutureObservation bool) error {
	if !s.isActiveValidator(s.host.ID()) {
		return fmt.Errorf("skipping attest observed tx: not active")
	}

	k := newObservedTxKey(*obsTx, inbound, allowFutureObservation)
	haveLocalAttestation, committed := s.observedTxAttestationStatus(k, s.pubKey)
	if haveLocalAttestation {
		if committed && s.observerHandleObservedTxCommitted != nil && s.observedTxCommittedQuorum(k) {
			s.logger.Debug().Msg("local observed tx quorum already committed, removing from observer deck")
			s.observerHandleObservedTxCommitted(*obsTx)
		}
		return nil
	}

	signBz, err := obsTx.GetSignablePayloadWithInbound(inbound)
	if err != nil {
		return fmt.Errorf("fail to marshal tx sign payload: %w", err)
	}

	signature, err := s.privKey.Sign(signBz)
	if err != nil {
		return fmt.Errorf("fail to sign tx sign payload: %w", err)
	}

	msg := common.AttestTx{
		ObsTx:                  *obsTx,
		Inbound:                inbound,
		AllowFutureObservation: allowFutureObservation,
		Attestation: &common.Attestation{
			PubKey:    s.pubKey,
			Signature: signature,
		},
	}

	// Handle the attestation locally first
	s.logger.Debug().Msg("handling attestation locally")
	s.handleObservedTxAttestation(ctx, msg)

	s.batcher.AddObservedTx(msg)

	return nil
}

func (s *AttestationGossip) observedTxAttestationStatus(k txKey, pubKey []byte) (bool, bool) {
	s.mu.Lock()
	state, ok := s.observedTxs[k]
	if ok {
		state.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return false, false
	}
	defer state.mu.Unlock()

	return state.AttestationStatus(pubKey)
}

func closeStream(logger zerolog.Logger, stream network.Stream) {
	if err := stream.Close(); err != nil {
		logger.Error().Err(err).Msg("fail to close stream")
	}
	go func() {
		// Give some time for the remote side to process
		// NOTE: remove this after upgrading to libp2p v0.12.0+, because
		// then Close() will close both write and read sides of the stream.
		// In v0.11.0, it only closes the write side.
		time.Sleep(10 * time.Second)
		if err := stream.Reset(); err != nil {
			logger.Error().Err(err).Msg("fail to reset stream after delay")
		}
	}()
}

// AttestNetworkFee creates and broadcasts an attestation for a network fee
func (s *AttestationGossip) AttestNetworkFee(ctx context.Context, networkFee common.NetworkFee) error {
	if !s.isActiveValidator(s.host.ID()) {
		return fmt.Errorf("skipping attest network fee: not active")
	}

	signBz, err := networkFee.GetSignablePayload()
	if err != nil {
		return fmt.Errorf("fail to marshal network fee sign payload: %w", err)
	}

	signature, err := s.privKey.Sign(signBz)
	if err != nil {
		return fmt.Errorf("fail to sign network fee sign payload: %w", err)
	}

	msg := common.AttestNetworkFee{
		NetworkFee: &networkFee,
		Attestation: &common.Attestation{
			PubKey:    s.pubKey,
			Signature: signature,
		},
	}

	// Handle the attestation locally first
	s.logger.Debug().Msg("handling attestation locally")
	s.handleNetworkFeeAttestation(ctx, msg)

	s.batcher.AddNetworkFee(msg)

	return nil
}

// AttestSolvency creates and broadcasts an attestation for a solvency proof
func (s *AttestationGossip) AttestSolvency(ctx context.Context, solvency common.Solvency) error {
	if !s.isActiveValidator(s.host.ID()) {
		return fmt.Errorf("skipping attest solvency: not active")
	}

	signBz, err := solvency.GetSignablePayload()
	if err != nil {
		return fmt.Errorf("fail to marshal solvency sign payload: %w", err)
	}

	signature, err := s.privKey.Sign(signBz)
	if err != nil {
		return fmt.Errorf("fail to sign solvency sign payload: %w", err)
	}

	msg := common.AttestSolvency{
		Solvency: &solvency,
		Attestation: &common.Attestation{
			PubKey:    s.pubKey,
			Signature: signature,
		},
	}

	// Handle the attestation locally first
	s.logger.Debug().Msg("handling attestation locally")
	s.handleSolvencyAttestation(ctx, msg)

	s.batcher.AddSolvency(msg)

	return nil
}

// AttestErrata creates and broadcasts an attestation for an errata transaction
func (s *AttestationGossip) AttestErrata(ctx context.Context, errata common.ErrataTx) error {
	// First remove any observed transactions for this chain/tx ID
	s.mu.Lock()
	for k := range s.observedTxs {
		if k.Chain.Equals(errata.Chain) && k.ID.Equals(errata.Id) {
			// Remove this tx from the list as we've observed an error
			delete(s.observedTxs, k)
		}
	}
	s.mu.Unlock()

	if !s.isActiveValidator(s.host.ID()) {
		return fmt.Errorf("skipping attest errata tx: not active")
	}

	signBz, err := errata.GetSignablePayload()
	if err != nil {
		return fmt.Errorf("fail to marshal errata sign payload: %w", err)
	}

	signature, err := s.privKey.Sign(signBz)
	if err != nil {
		return fmt.Errorf("fail to sign errata sign payload: %w", err)
	}

	msg := common.AttestErrataTx{
		ErrataTx: &errata,
		Attestation: &common.Attestation{
			PubKey:    s.pubKey,
			Signature: signature,
		},
	}

	// Handle the attestation locally first
	s.logger.Debug().Msg("handling attestation locally")
	s.handleErrataAttestation(ctx, msg)

	s.batcher.AddErrataTx(msg)

	return nil
}

// handleStreamAttestationState handles incoming observed transaction streams
func (s *AttestationGossip) handleStreamAttestationState(stream network.Stream) {
	remotePeer := stream.Conn().RemotePeer()

	logger := s.logger.With().Str("remote_peer", remotePeer.String()).Logger()
	logger.Debug().Msg("reading attestation state message")

	defer closeStream(logger, stream)

	// Read and process the message
	data, err := p2p.ReadStreamWithBuffer(stream)
	if err != nil {
		if err != io.EOF {
			logger.Error().Err(err).Msg("fail to read payload from stream")
		}
		return
	}

	// Check message type and handle accordingly
	if len(data) == 0 {
		logger.Error().Msg("empty payload")
		return
	}

	// Handle based on prefix
	switch {
	case data[0] == prefixSendState[0]:
		// Send state request
		if len(data) != 1 {
			logger.Error().Msg("unexpected payload length for send state request")
			return
		}
		logger.Debug().Msg("handling send state request")
		s.sendAttestationState(stream)

	case data[0] == prefixBatchBegin[0]:

		s.stateInitMu.Lock()
		if s.stateInitPeers == nil {
			// we are not asking for attestation state
			s.stateInitMu.Unlock()
			s.logger.Warn().Msg("skipping attestation state stream, not asking for it")
			err := p2p.WriteStreamWithBuffer([]byte("error: not asking for attestation state"), stream)
			if err != nil {
				logger.Error().Err(err).Msgf("fail to write error reply to peer: %s", remotePeer)
			}
			return
		}
		_, ok := s.stateInitPeers[remotePeer]
		s.stateInitMu.Unlock()
		if !ok {
			// unauthorized peer
			s.logger.Warn().Msg("skipping attestation state stream from unexpected peer")
			err := p2p.WriteStreamWithBuffer([]byte("error: unauthorized peer for attestation state"), stream)
			if err != nil {
				logger.Error().Err(err).Msgf("fail to write error reply to peer: %s", remotePeer)
			}
			return
		}

		// Batched state transmission
		logger.Debug().Msg("handling batched attestation state")
		err := s.receiveBatchedAttestationState(stream, data[1:])
		if err != nil {
			logger.Error().Err(err).Msg("failed to receive batched attestation state")
		}

	default:
		logger.Error().Msgf("unknown message type: %d", data[0])
		err := p2p.WriteStreamWithBuffer([]byte("error: unknown message type"), stream)
		if err != nil {
			logger.Error().Err(err).Msgf("fail to write error reply to peer: %s", remotePeer)
		}
	}
}

func (s *AttestationGossip) handleStreamBatchedAttestations(stream network.Stream) {
	remotePeer := stream.Conn().RemotePeer()
	logger := s.logger.With().Str("remote_peer", remotePeer.String()).Logger()

	defer closeStream(logger, stream)

	if !s.isActiveValidator(remotePeer) {
		logger.Debug().Msg("skipping batched attestation from non-active validator")
		if err := p2p.WriteStreamWithBuffer([]byte(p2p.StreamMsgDone), stream); err != nil {
			logger.Error().Err(err).Msgf("fail to write reply to peer: %s", remotePeer)
		}
		return
	}

	sem, err := s.peerMgr.acquire(remotePeer)
	if err != nil {
		logger.Error().Err(err).Msgf("fail to acquire semaphore for peer: %s", remotePeer)
		if err = p2p.WriteStreamWithBuffer([]byte(p2p.StreamMsgDone), stream); err != nil {
			logger.Error().Err(err).Msgf("fail to write reply to peer: %s", remotePeer)
		}
		return
	}
	defer s.peerMgr.release(sem)

	logger.Debug().Msg("reading batched attestations message")

	// Read batch data
	data, err := p2p.ReadStreamWithBuffer(stream)
	if err != nil {
		if err != io.EOF {
			logger.Error().Err(err).Msg("fail to read payload from stream")
		}
		return
	}

	// Send acknowledgment
	if err := p2p.WriteStreamWithBuffer([]byte(p2p.StreamMsgDone), stream); err != nil {
		logger.Error().Err(err).Msg("fail to write acknowledgment")
		return
	}

	if len(data) == 0 {
		logger.Error().Msg("empty payload")
		return
	}

	// Unmarshal batch
	var batch common.AttestationBatch
	if err := batch.Unmarshal(data); err != nil {
		logger.Error().Err(err).Msg("fail to unmarshal attestation batch")
		return
	}

	// slight tolerance above max
	max := s.batcher.getMaxBatchSize()

	// Process each attestation in the batch
	for i, tx := range batch.AttestTxs {
		if int64(i) >= max {
			logger.Error().Msgf("tx batch size %d exceeds max size %d", len(batch.AttestTxs), max)
			break
		}
		if tx == nil {
			logger.Error().Msgf("tx is nil at index %d", i)
			continue
		}
		s.handleObservedTxAttestation(context.Background(), *tx)
	}

	for i, nf := range batch.AttestNetworkFees {
		if int64(i) >= max {
			logger.Error().Msgf("net fee batch size %d exceeds max size %d", len(batch.AttestNetworkFees), max)
			break
		}
		if nf == nil {
			logger.Error().Msgf("network fee is nil at index %d", i)
			continue
		}
		s.handleNetworkFeeAttestation(context.Background(), *nf)
	}

	for i, solvency := range batch.AttestSolvencies {
		if int64(i) >= max {
			logger.Error().Msgf("solvency batch size %d exceeds max size %d", len(batch.AttestSolvencies), max)
			break
		}
		if solvency == nil {
			logger.Error().Msgf("solvency is nil at index %d", i)
			continue
		}
		s.handleSolvencyAttestation(context.Background(), *solvency)
	}

	for i, errata := range batch.AttestErrataTxs {
		if int64(i) >= max {
			logger.Error().Msgf("errata batch size %d exceeds max size %d", len(batch.AttestErrataTxs), max)
			break
		}
		if errata == nil {
			logger.Error().Msgf("errata is nil at index %d", i)
			continue
		}
		s.handleErrataAttestation(context.Background(), *errata)
	}

}
