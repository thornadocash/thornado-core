package observer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"

	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/common"
)

// askForAttestationState will ask other validators for their attestation state
func (s *AttestationGossip) askForAttestationState(ctx context.Context) {
	allPeers := s.host.Peerstore().Peers()
	if len(allPeers) == 0 {
		s.logger.Debug().Msg("no peers to ask for attestation state")
		return
	}
	peersWithoutMe := make([]peer.ID, 0, len(allPeers)-1)
	// Skip self, send to 3 active vals that we are peered with
	activeVals := s.getActiveValidators()
	for _, peerID := range allPeers {
		if peerID == s.host.ID() {
			continue
		}
		if _, ok := activeVals[peerID]; ok {
			peersWithoutMe = append(peersWithoutMe, peerID)
		}
	}

	if len(peersWithoutMe) == 0 {
		s.logger.Debug().Msg("no active val peers without me to ask for attestation state")
		return
	}
	// ask 3 random peers for their attestation state
	rand.Shuffle(len(peersWithoutMe), func(i, j int) {
		peersWithoutMe[i], peersWithoutMe[j] = peersWithoutMe[j], peersWithoutMe[i]
	})
	numPeers := s.config.AskPeers
	if len(peersWithoutMe) < numPeers {
		numPeers = len(peersWithoutMe)
	}
	peers := peersWithoutMe[:numPeers]

	s.stateInitMu.Lock()
	s.stateInitPeers = make(map[peer.ID]bool, len(peers))
	for _, peer := range peers {
		s.stateInitPeers[peer] = true
	}
	s.stateInitMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, peer := range peers {
		go s.askPeerForState(ctx, peer, &wg)
	}
	wg.Wait()
	// Reset the state init peers
	s.stateInitMu.Lock()
	s.stateInitPeers = nil
	s.stateInitMu.Unlock()
}

func (s *AttestationGossip) sendAttestationState(stream network.Stream) {
	remotePeer := stream.Conn().RemotePeer()
	s.logger.Debug().Str("peer", remotePeer.String()).Msg("sending attestation state")
	s.mu.Lock()

	// Collect all QuorumTxs
	allQuorumTxs := make([]*common.QuorumTx, 0, len(s.observedTxs))
	for k, ots := range s.observedTxs {
		ots.mu.Lock()

		quorumTxs := &common.QuorumTx{
			ObsTx:                  *ots.Item.ObservedTx,
			Inbound:                k.Inbound,
			AllowFutureObservation: k.AllowFutureObservation,
			Attestations:           ots.AttestationsCopy(),
		}
		allQuorumTxs = append(allQuorumTxs, quorumTxs)

		ots.mu.Unlock()
	}

	allNetworkFees := make([]*common.QuorumNetworkFee, 0, len(s.networkFees))
	for _, nf := range s.networkFees {
		nf.mu.Lock()
		nfCopy := &common.NetworkFee{
			Chain:           nf.Item.Chain,
			Height:          nf.Item.Height,
			TransactionSize: nf.Item.TransactionSize,
			TransactionRate: nf.Item.TransactionRate,
		}
		allNetworkFees = append(allNetworkFees, &common.QuorumNetworkFee{
			NetworkFee:   nfCopy,
			Attestations: nf.AttestationsCopy(),
		})
		nf.mu.Unlock()
	}

	allSolvencies := make([]*common.QuorumSolvency, 0, len(s.solvencies))
	for _, solvency := range s.solvencies {
		solvency.mu.Lock()
		sCopy := &common.Solvency{
			Id:     solvency.Item.Id,
			Chain:  solvency.Item.Chain,
			Height: solvency.Item.Height,
			PubKey: solvency.Item.PubKey,
			Coins:  solvency.Item.Coins.Copy(),
		}
		allSolvencies = append(allSolvencies, &common.QuorumSolvency{
			Solvency:     sCopy,
			Attestations: solvency.AttestationsCopy(),
		})
		solvency.mu.Unlock()
	}

	allErrataTxs := make([]*common.QuorumErrataTx, 0, len(s.errataTxs))
	for _, errataTx := range s.errataTxs {
		errataTx.mu.Lock()
		eTxCopy := &common.ErrataTx{
			Chain: errataTx.Item.Chain,
			Id:    errataTx.Item.Id,
		}
		allErrataTxs = append(allErrataTxs, &common.QuorumErrataTx{
			ErrataTx:     eTxCopy,
			Attestations: errataTx.AttestationsCopy(),
		})
		errataTx.mu.Unlock()
	}

	s.mu.Unlock()

	// Calculate total number of batches
	totalTxs := len(allQuorumTxs)
	totalTxBatches := (totalTxs + maxQuorumTxsPerBatch - 1) / maxQuorumTxsPerBatch

	totalNfs := len(allNetworkFees)
	totalNfBatches := (totalNfs + maxQuorumTxsPerBatch - 1) / maxQuorumTxsPerBatch

	totalSolvencies := len(allSolvencies)
	totalSolvencyBatches := (totalSolvencies + maxQuorumTxsPerBatch - 1) / maxQuorumTxsPerBatch

	totalErrataTxs := len(allErrataTxs)
	totalErrataBatches := (totalErrataTxs + maxQuorumTxsPerBatch - 1) / maxQuorumTxsPerBatch

	totalBatches := max(
		totalTxBatches,
		totalNfBatches,
		totalSolvencyBatches,
		totalErrataBatches,
	)

	s.logger.Debug().
		Int("total_txs", totalTxs).
		Int("total_network_fees", totalNfs).
		Int("total_solvencies", totalSolvencies).
		Int("total_errata_txs", totalErrataTxs).
		Int("total_batches", totalBatches).
		Int("max_per_batch", maxQuorumTxsPerBatch).
		Msg("sending attestation state in batches")

	// Send batch begin signal with total number of batches
	beginInfo := make([]byte, 4)
	binary.LittleEndian.PutUint32(beginInfo, uint32(totalBatches))
	if err := p2p.WriteStreamWithBuffer(append(prefixBatchBegin, beginInfo...), stream); err != nil {
		s.logger.Error().Err(err).Msg("failed to send batch begin signal")
		return
	}

	// Wait for acknowledgment
	ack, err := p2p.ReadStreamWithBuffer(stream)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to read ack after batch begin")
		return
	}

	if totalBatches == 0 {
		if string(ack) != p2p.StreamMsgDone {
			s.logger.Error().Str("response", string(ack)).Msg("unexpected final response")
			return
		}
		s.logger.Debug().Msg("successfully sent empty attestation state")
		return
	}

	if string(ack) != streamAckBegin {
		s.logger.Error().Str("response", string(ack)).Msg("unexpected response to batch begin")
		return
	}

	// Send batches
	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		startIdx := batchNum * maxQuorumTxsPerBatch
		endIdx := startIdx + maxQuorumTxsPerBatch

		var batchTxs []*common.QuorumTx
		var batchNfs []*common.QuorumNetworkFee
		var batchSolvencies []*common.QuorumSolvency
		var batchErrataTxs []*common.QuorumErrataTx

		if totalTxs > 0 && startIdx < totalTxs {
			if endIdx > totalTxs {
				endIdx = totalTxs
			}
			batchTxs = allQuorumTxs[startIdx:endIdx]
		}

		if totalNfs > 0 && startIdx < totalNfs {
			if endIdx > totalNfs {
				endIdx = totalNfs
			}
			batchNfs = allNetworkFees[startIdx:endIdx]
		}

		if totalSolvencies > 0 && startIdx < totalSolvencies {
			if endIdx > totalSolvencies {
				endIdx = totalSolvencies
			}
			batchSolvencies = allSolvencies[startIdx:endIdx]
		}

		if totalErrataTxs > 0 && startIdx < totalErrataTxs {
			if endIdx > totalErrataTxs {
				endIdx = totalErrataTxs
			}
			batchErrataTxs = allErrataTxs[startIdx:endIdx]
		}

		// Create batch with current set of QuorumTxs
		batch := common.QuorumState{
			QuoTxs:         batchTxs,
			QuoNetworkFees: batchNfs,
			QuoSolvencies:  batchSolvencies,
			QuoErrataTxs:   batchErrataTxs,
		}

		// Marshal the batch (handle empty batches specially)
		var batchData []byte
		if quorumStateEmpty(batch) {
			// For empty batches, use an empty buffer rather than marshaling an empty struct
			batchData = []byte{}
		} else {
			batchData, err = batch.Marshal()
			if err != nil {
				s.logger.Error().Err(err).Int("batch", batchNum+1).Msg("failed to marshal batch")
				return
			}
		}

		// Send batch info (batch number and data size)
		batchInfo := make([]byte, 8) // 4 bytes for batch number, 4 bytes for data size
		binary.LittleEndian.PutUint32(batchInfo[0:4], uint32(batchNum))
		binary.LittleEndian.PutUint32(batchInfo[4:8], uint32(len(batchData)))

		// Send the batch data header
		if err = p2p.WriteStreamWithBuffer(append(prefixBatchHeader, batchInfo...), stream); err != nil {
			s.logger.Error().Err(err).Int("batch", batchNum+1).Msg("failed to send batch header")
			return
		}

		// Wait for acknowledgment of batch header
		var headerAck []byte
		headerAck, err = p2p.ReadStreamWithBuffer(stream)
		if err != nil {
			s.logger.Error().Err(err).Int("batch", batchNum+1).Msg("failed to read ack after batch header")
			return
		}
		if string(headerAck) != streamAckHeader {
			s.logger.Error().Str("response", string(headerAck)).Int("batch", batchNum+1).Msg("unexpected response to batch header")
			return
		}

		// Send the actual batch data
		if err = p2p.WriteStreamWithBuffer(append(prefixBatchData, batchData...), stream); err != nil {
			s.logger.Error().Err(err).Int("batch", batchNum+1).Msg("failed to send batch data")
			return
		}

		// Wait for acknowledgment of batch data
		var dataAck []byte
		dataAck, err = p2p.ReadStreamWithBuffer(stream)
		if err != nil {
			s.logger.Error().Err(err).Int("batch", batchNum+1).Msg("failed to read ack after batch data")
			return
		}
		if string(dataAck) != streamAckData {
			s.logger.Error().Str("response", string(dataAck)).Int("batch", batchNum+1).Msg("unexpected response to batch data")
			return
		}

		s.logger.Debug().
			Int("batch", batchNum+1).
			Int("total_batches", totalBatches).
			Int("batch_size", len(batch.QuoTxs)).
			Int("data_size_bytes", len(batchData)).
			Msg("batch sent")
	}

	// Send batch end signal
	if err = p2p.WriteStreamWithBuffer(prefixBatchEnd, stream); err != nil {
		s.logger.Error().Err(err).Msg("failed to send batch end signal")
		return
	}

	// Wait for the final acknowledgment
	var endAck []byte
	endAck, err = p2p.ReadStreamWithBuffer(stream)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to read final ack")
		return
	}
	if string(endAck) != p2p.StreamMsgDone {
		s.logger.Error().Str("response", string(endAck)).Msg("unexpected final response")
		return
	}

	s.logger.Debug().
		Int("total_txs", totalTxs).
		Int("total_network_fees", totalNfs).
		Int("total_solvencies", totalSolvencies).
		Int("total_errata_txs", totalErrataTxs).
		Int("total_batches", totalBatches).
		Msg("successfully sent attestation state in batches")
}

func batchBeginTotalBatches(data []byte) (int, error) {
	if len(data) >= len(prefixBatchBegin)+4 && bytes.HasPrefix(data, prefixBatchBegin) {
		offset := len(prefixBatchBegin)
		return int(binary.LittleEndian.Uint32(data[offset : offset+4])), nil
	}
	if len(data) >= 4 {
		return int(binary.LittleEndian.Uint32(data[:4])), nil
	}
	return 0, fmt.Errorf("invalid batch begin format")
}

func quorumStateEmpty(batch common.QuorumState) bool {
	return len(batch.QuoTxs) == 0 &&
		len(batch.QuoNetworkFees) == 0 &&
		len(batch.QuoSolvencies) == 0 &&
		len(batch.QuoErrataTxs) == 0
}

// Add a function to handle receiving batched attestation state
func (s *AttestationGossip) receiveBatchedAttestationState(stream network.Stream, initialData []byte) error {
	remotePeer := stream.Conn().RemotePeer()
	logger := s.logger.With().Str("peer", remotePeer.String()).Str("who", "receiver").Logger()

	// Process batch begin
	totalBatches, err := batchBeginTotalBatches(initialData)
	if err != nil {
		return err
	}

	logger.Debug().Int("total_batches", totalBatches).Msg("receiving batched attestation state")

	if totalBatches == 0 {
		// If there are no transactions, we still need to send the end signal
		if err := p2p.WriteStreamWithBuffer([]byte(p2p.StreamMsgDone), stream); err != nil {
			return fmt.Errorf("failed to send final ack: %w", err)
		}
		return nil
	}

	// Acknowledge receipt of begin signal
	if err := p2p.WriteStreamWithBuffer([]byte(streamAckBegin), stream); err != nil {
		return fmt.Errorf("failed to acknowledge batch begin: %w", err)
	}

	total := 0
	// Receive all batches
	for {
		// Read next message
		message, err := p2p.ReadStreamWithBuffer(stream)
		if err != nil {
			return fmt.Errorf("failed to read batch message: %w", err)
		}

		// Check if this is the end marker
		if len(message) >= 1 && bytes.Equal(message[:1], prefixBatchEnd) {
			// Send final acknowledgment
			if err = p2p.WriteStreamWithBuffer([]byte(p2p.StreamMsgDone), stream); err != nil {
				return fmt.Errorf("failed to send final ack: %w", err)
			}
			break
		}

		// Must be a batch data header
		if len(message) < 9 || !bytes.Equal(message[:1], prefixBatchHeader) {
			return fmt.Errorf("invalid batch header format")
		}

		// Extract batch information
		batchNum := int(binary.LittleEndian.Uint32(message[1:5]))
		dataSize := int(binary.LittleEndian.Uint32(message[5:9]))

		logger.Debug().
			Int("batch", batchNum+1).
			Int("total_batches", totalBatches).
			Int("data_size_bytes", dataSize).
			Msg("receiving batch")

		// Acknowledge the batch header
		if err = p2p.WriteStreamWithBuffer([]byte(streamAckHeader), stream); err != nil {
			return fmt.Errorf("failed to acknowledge batch header: %w", err)
		}

		// Read the batch data
		message, err = p2p.ReadStreamWithBuffer(stream)
		if err != nil {
			return fmt.Errorf("failed to read batch data: %w", err)
		}
		if !bytes.HasPrefix(message, prefixBatchData) {
			return fmt.Errorf("invalid batch data format")
		}
		batchData := message[1:]

		logger.Debug().Int("len_bz", len(batchData)).Msg("received batch data")

		if len(batchData) != dataSize {
			return fmt.Errorf("batch data size mismatch: expected %d, got %d", dataSize, len(batchData))
		}

		// Acknowledge the batch data
		if err := p2p.WriteStreamWithBuffer([]byte(streamAckData), stream); err != nil {
			return fmt.Errorf("failed to acknowledge batch data: %w", err)
		}

		// In receiveBatchedAttestationState function
		if len(batchData) == 0 {
			// This is an empty batch, no need to unmarshal
			continue
		}

		// Unmarshal the batch
		var batch common.QuorumState
		if err := batch.Unmarshal(batchData); err != nil {
			return fmt.Errorf("failed to unmarshal batch: %w", err)
		}

		// Add the QuorumTxs to our collection
		go s.processAttestationStateBatch(logger, batch)

		total += len(batch.QuoTxs)

		logger.Debug().
			Int("batch", batchNum+1).
			Int("total_batches", totalBatches).
			Int("batch_size", len(batch.QuoTxs)).
			Int("total_collected", total).
			Msg("batch received")
	}

	logger.Debug().Int("total_txs", total).Msg("finished receiving attestation state")

	return nil
}

func (s *AttestationGossip) processAttestationStateBatch(logger zerolog.Logger, batch common.QuorumState) {
	ctx := context.TODO()
	for _, qt := range batch.QuoTxs {
		for _, att := range qt.Attestations {
			logger.Debug().Msg("handling attestation from peer state dump")
			s.handleObservedTxAttestation(ctx, common.AttestTx{
				ObsTx:                  qt.ObsTx,
				Inbound:                qt.Inbound,
				Attestation:            att,
				AllowFutureObservation: qt.AllowFutureObservation,
			})
		}
	}
	for _, nf := range batch.QuoNetworkFees {
		for _, att := range nf.Attestations {
			logger.Debug().Msg("handling network fee from peer state dump")
			s.handleNetworkFeeAttestation(ctx, common.AttestNetworkFee{
				NetworkFee:  nf.NetworkFee,
				Attestation: att,
			})
		}
	}
	for _, solvency := range batch.QuoSolvencies {
		for _, att := range solvency.Attestations {
			logger.Debug().Msg("handling solvency from peer state dump")
			s.handleSolvencyAttestation(ctx, common.AttestSolvency{
				Solvency:    solvency.Solvency,
				Attestation: att,
			})
		}
	}
	for _, errataTx := range batch.QuoErrataTxs {
		for _, att := range errataTx.Attestations {
			logger.Debug().Msg("handling errata tx from peer state dump")
			s.handleErrataAttestation(ctx, common.AttestErrataTx{
				ErrataTx:    errataTx.ErrataTx,
				Attestation: att,
			})
		}
	}
}

// askPeerForState will ask a specific peer for its attestation state
func (s *AttestationGossip) askPeerForState(ctx context.Context, peer peer.ID, wg *sync.WaitGroup) {
	defer wg.Done()
	stream, err := s.host.NewStream(ctx, peer, attestationStateProtocol)
	if err != nil {
		logAttestationStreamOpenError(s.logger, err, peer, string(attestationStateProtocol))
		return
	}

	s.askStreamForState(stream)
}

// askStreamForState will ask a specific stream for its attestation state
func (s *AttestationGossip) askStreamForState(stream network.Stream) {
	peer := stream.Conn().RemotePeer()

	defer closeStream(s.logger, stream)

	if err := p2p.WriteStreamWithBuffer(prefixSendState, stream); err != nil {
		s.logger.Error().Err(err).Msgf("fail to write payload to peer: %s", peer)
		return
	}

	// Read initial response from peer
	initialData, err := p2p.ReadStreamWithBuffer(stream)
	if err != nil {
		s.logger.Error().Err(err).Msgf("fail to read reply from peer: %s", peer)
		return
	}

	// If it's not the batch begin prefix, it's an error
	if !bytes.HasPrefix(initialData, prefixBatchBegin) {
		s.logger.Error().Msgf("unexpected attestation state format from peer: %s", peer)
		return
	}

	// Process the batched attestation state
	if err := s.receiveBatchedAttestationState(stream, initialData); err != nil {
		s.logger.Error().Err(err).Msgf("fail to receive batched attestation state from peer: %s", peer)
	}
}
