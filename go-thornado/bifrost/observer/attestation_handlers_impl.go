package observer

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

const priceFeedsUpdateDelay = time.Millisecond * 100

// handleObservedTxAttestation processes attestations for observed transactions
func (s *AttestationGossip) handleObservedTxAttestation(ctx context.Context, tx common.AttestTx) {
	obsTx := tx.ObsTx

	k := txKey{
		Chain:                  obsTx.Tx.Chain,
		ID:                     obsTx.Tx.ID,
		UniqueHash:             obsTx.Tx.Hash(obsTx.BlockHeight),
		AllowFutureObservation: tx.AllowFutureObservation,
		Finalized:              obsTx.IsFinal(),
		Inbound:                tx.Inbound,
	}

	s.mu.Lock()
	state, ok := s.observedTxs[k]
	if !ok {
		state = s.observedTxsPool.NewAttestationState(&obsTx)
		s.observedTxs[k] = state
	}

	// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
	// This order must be maintained everywhere to avoid deadlock.
	state.mu.Lock()
	s.mu.Unlock()

	defer state.mu.Unlock()

	// Add the attestation
	if err := state.AddAttestation(tx.Attestation); err != nil {
		s.logger.Error().Err(err).Msg("fail to add attestation")
		return
	}

	// Determine the number of validators needed for attestation
	var total int
	if k.AllowFutureObservation {
		keysignParty, err := s.getKeysignParty(obsTx.ObservedPubKey)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to get key sign party")
			return
		}
		total = len(keysignParty)
	} else {
		total = s.activeValidatorCount()
	}

	hasSuperMajority := types.HasSuperMajority(state.AttestationCount(), total)

	// If we have a supermajority, send to thornado
	if hasSuperMajority {
		s.logger.Debug().Msgf("has supermajority: %d/%d", state.AttestationCount(), total)

		s.sendObservedTxAttestationsToThornado(ctx, obsTx, state, k.Inbound, k.AllowFutureObservation, true)
	} else {
		s.logger.Debug().Msgf("observed tx attestation received - %s, id: %s, inbound: %t, final: %t, quorum: %d/%d",
			k.Chain, k.ID, k.Inbound, k.Finalized, state.AttestationCount(), total)
	}
}

// sendObservedTxAttestationsToThornado sends attestations to thornado via gRPC
func (s *AttestationGossip) sendObservedTxAttestationsToThornado(
	ctx context.Context,
	tx common.ObservedTx,
	state *AttestationState[*common.ObservedTx],
	inbound, allowFutureObservation, isQuorum bool,
) {
	unsent := state.UnsentAttestations()
	if len(unsent) == 0 {
		s.logger.Debug().Msg("no unsent observed tx attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumTx(ctx, &common.QuorumTx{
		ObsTx:                  tx,
		Attestations:           unsent,
		Inbound:                inbound,
		AllowFutureObservation: allowFutureObservation,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum tx")
		return
	}

	s.logger.Info().Msgf("sent quorum tx to thornado - %s, id: %s, inbound: %t, final: %t, attestations: %s",
		tx.Tx.Chain, tx.Tx.ID, inbound, tx.IsFinal(), state.State())

	// Mark attestations as sent
	state.MarkAttestationsSent(isQuorum)
}

// handleNetworkFeeAttestation processes attestations for network fees
func (s *AttestationGossip) handleNetworkFeeAttestation(ctx context.Context, anf common.AttestNetworkFee) {
	// Use the network fee as the map key
	k := *anf.NetworkFee

	s.mu.Lock()
	state, ok := s.networkFees[k]
	if !ok {
		// Create a new attestation state
		state = s.networkFeesPool.NewAttestationState(anf.NetworkFee)
		s.networkFees[k] = state
	}

	// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
	// This order must be maintained everywhere to avoid deadlock.
	state.mu.Lock()
	s.mu.Unlock()

	defer state.mu.Unlock()

	// Add the attestation
	if err := state.AddAttestation(anf.Attestation); err != nil {
		s.logger.Error().Err(err).Msg("fail to add attestation")
		return
	}

	// Get the active validator count
	activeValCount := s.activeValidatorCount()
	hasSuperMajority := types.HasSuperMajority(state.AttestationCount(), activeValCount)

	// If we have a supermajority, send to thornado
	if hasSuperMajority {
		s.logger.Debug().Msgf("has supermajority: %d/%d", state.AttestationCount(), activeValCount)
		s.sendNetworkFeeAttestationsToThornado(ctx, *state.Item, state, true)
	} else {
		s.logger.Debug().Msgf("network fee attestation received - %s, height: %d, quorum: %d/%d",
			k.Chain, k.Height, state.AttestationCount(), activeValCount)
	}
}

// sendNetworkFeeAttestationsToThornado sends network fee attestations to thornado via gRPC
func (s *AttestationGossip) sendNetworkFeeAttestationsToThornado(ctx context.Context, networkFee common.NetworkFee, state *AttestationState[*common.NetworkFee], isQuorum bool) {
	unsent := state.UnsentAttestations()
	if len(unsent) == 0 {
		s.logger.Debug().Msg("no unsent network fee attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumNetworkFee(ctx, &common.QuorumNetworkFee{
		NetworkFee:   &networkFee,
		Attestations: unsent,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum network fee")
		return
	}

	s.logger.Info().Msgf("sent quorum network fee to thornado - %s, height: %d, attestations: %s",
		networkFee.Chain, networkFee.Height, state.State())

	// Mark attestations as sent
	state.MarkAttestationsSent(isQuorum)
}

// handleSolvencyAttestation processes attestations for solvency proofs
func (s *AttestationGossip) handleSolvencyAttestation(ctx context.Context, ats common.AttestSolvency) {
	// Calculate the hash for the solvency to use as key
	k, err := ats.Solvency.Hash()
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to hash solvency")
		return
	}

	s.mu.Lock()
	state, ok := s.solvencies[k]
	if !ok {
		// Create a new attestation state
		state = s.solvenciesPool.NewAttestationState(ats.Solvency)
		s.solvencies[k] = state
	}

	// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
	// This order must be maintained everywhere to avoid deadlock.
	state.mu.Lock()
	s.mu.Unlock()

	defer state.mu.Unlock()

	// Add the attestation
	if err := state.AddAttestation(ats.Attestation); err != nil {
		s.logger.Error().Err(err).Msg("fail to add attestation")
		return
	}

	// Get the active validator count
	activeValCount := s.activeValidatorCount()
	hasSuperMajority := types.HasSuperMajority(state.AttestationCount(), activeValCount)

	// If we have a supermajority, send to thornado
	if hasSuperMajority {
		s.logger.Debug().Msgf("has supermajority: %d/%d", state.AttestationCount(), activeValCount)
		s.sendSolvencyAttestationsToThornado(ctx, *state.Item, state, true)
	} else {
		s.logger.Debug().Msgf("solvency attestation received - %s, height: %d, quorum: %d/%d",
			ats.Solvency.Chain, ats.Solvency.Height, state.AttestationCount(), activeValCount)
	}
}

// sendSolvencyAttestationsToThornado sends solvency attestations to thornado via gRPC
func (s *AttestationGossip) sendSolvencyAttestationsToThornado(ctx context.Context, solvency common.Solvency, state *AttestationState[*common.Solvency], isQuorum bool) {
	unsent := state.UnsentAttestations()
	if len(unsent) == 0 {
		s.logger.Debug().Msg("no unsent solvency attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumSolvency(ctx, &common.QuorumSolvency{
		Solvency:     &solvency,
		Attestations: unsent,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum solvency")
		return
	}

	s.logger.Info().Msgf("sent quorum solvency to thornado - %s, height: %d, coins: %s, pubkey: %s, attestations: %s",
		solvency.Chain, solvency.Height, solvency.Coins.String(), solvency.PubKey.String(), state.State())

	// Mark attestations as sent
	state.MarkAttestationsSent(isQuorum)
}

// handleErrataAttestation processes attestations for errata transactions
func (s *AttestationGossip) handleErrataAttestation(ctx context.Context, aet common.AttestErrataTx) {
	// Use the errata tx as the map key
	k := *aet.ErrataTx

	s.mu.Lock()
	state, ok := s.errataTxs[k]
	if !ok {
		// Create a new attestation state
		state = s.errataTxsPool.NewAttestationState(aet.ErrataTx)
		s.errataTxs[k] = state
	}

	// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
	// This order must be maintained everywhere to avoid deadlock.
	state.mu.Lock()
	s.mu.Unlock()

	defer state.mu.Unlock()

	// Add the attestation
	if err := state.AddAttestation(aet.Attestation); err != nil {
		s.logger.Error().Err(err).Msg("fail to add attestation")
		return
	}

	// Get the active validator count
	activeValCount := s.activeValidatorCount()
	hasSuperMajority := types.HasSuperMajority(state.AttestationCount(), activeValCount)

	// If we have a supermajority, send to thornado
	if hasSuperMajority {
		s.logger.Debug().Msgf("has supermajority: %d/%d", state.AttestationCount(), activeValCount)
		s.sendErrataAttestationsToThornado(ctx, *state.Item, state, true)
	} else {
		s.logger.Debug().Msgf("errata attestation received - %s, id: %s, quorum: %d/%d",
			k.Chain, k.Id, state.AttestationCount(), activeValCount)
	}
}

// sendErrataAttestationsToThornado sends errata attestations to thornado via gRPC
func (s *AttestationGossip) sendErrataAttestationsToThornado(ctx context.Context, errata common.ErrataTx, state *AttestationState[*common.ErrataTx], isQuorum bool) {
	unsent := state.UnsentAttestations()
	if len(unsent) == 0 {
		s.logger.Debug().Msg("no unsent errata attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumErrataTx(ctx, &common.QuorumErrataTx{
		ErrataTx:     &errata,
		Attestations: unsent,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum errata")
		return
	}

	s.logger.Info().Msgf("sent quorum errata to thornado - %s - ID: %s - attestations: %s", errata.Chain, errata.Id, state.State())

	// Mark attestations as sent
	state.MarkAttestationsSent(isQuorum)
}

// handlePriceFeedAttestation processes attestations for price feeds
func (s *AttestationGossip) handlePriceFeedAttestation(ctx context.Context, apf common.AttestPriceFeed) {
	// Use the pubkey as key
	k := hex.EncodeToString(apf.Attestation.PubKey)

	// Get the marshaled data for signature verification
	signBz, err := apf.PriceFeed.GetSignablePayload()
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to get signable payload")
		return
	}

	// Verify the signature
	att := apf.Attestation
	err = verifySignature(signBz, att.Signature, att.PubKey)
	if err != nil {
		s.logger.Error().Err(err).Msgf("signature verification failed for %x", att.PubKey)
		return
	}

	// We don't collect multiple attestations for price feeds, rather want
	// to store the most recent price we received for every node
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.priceFeeds[k]
	if !ok || state.Item.Time < apf.PriceFeed.Time {
		// Set new attestation state
		state = s.priceFeedsPool.NewAttestationState(apf.PriceFeed)
		state.attestations = []attestationSentState{
			{attestation: apf.Attestation, sent: false},
		}
		s.priceFeeds[k] = state

		// we received an updated price feed, start delay to maybe collect
		// more updated values before sending them to thornado
		if s.priceFeedsDelay.IsRunning() {
			return
		}

		s.priceFeedsDelay.Start()
		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(priceFeedsUpdateDelay):
				s.sendPriceFeedAttestationsToThornado(ctx)
				s.priceFeedsDelay.Done()
			}
		}()
	}
}

// sendPriceFeedAttestationsToThornado sends price feed attestations
// to thornado via gRPC
//
// To avoid calling the endpoint hundred times per update, all
// (quorum) price feeds are batched into a single request
func (s *AttestationGossip) sendPriceFeedAttestationsToThornado(
	ctx context.Context,
) {
	qpfb := common.QuorumPriceFeedBatch{
		QuorumPriceFeeds: []*common.QuorumPriceFeed{},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.priceFeeds {
		state.mu.Lock()
		defer state.mu.Unlock()

		qpfb.QuorumPriceFeeds = append(qpfb.QuorumPriceFeeds, &common.QuorumPriceFeed{
			PriceFeed:    state.Item,
			Attestations: []*common.Attestation{state.attestations[0].attestation},
		})
	}

	if len(qpfb.QuorumPriceFeeds) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := s.grpcClient.SendQuorumPriceFeedBatch(ctx, &qpfb)
	if err != nil {
		s.logger.Error().Err(err).Msg("fail to send price feed")
		return
	}

	// delete price feeds
	for k := range s.priceFeeds {
		delete(s.priceFeeds, k)
	}
}
