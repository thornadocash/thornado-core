package observer

import (
	"context"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// handleObservedTxAttestation processes attestations for observed transactions
func (s *AttestationGossip) handleObservedTxAttestation(ctx context.Context, tx common.AttestTx) {
	obsTx := tx.ObsTx

	k := newObservedTxKey(obsTx, tx.Inbound, tx.AllowFutureObservation)

	s.mu.Lock()
	state, ok := s.observedTxs[k]
	if !ok {
		state = s.observedTxsPool.NewAttestationState(&attestableObservedTx{
			ObservedTx: &obsTx,
			inbound:    tx.Inbound,
		})
		s.observedTxs[k] = state
	}

	// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
	// This order must be maintained everywhere to avoid deadlock.
	state.mu.Lock()
	s.mu.Unlock()

	defer state.mu.Unlock()

	// Add the attestation. Observed tx attestations must be verified against
	// the incoming message payload, since peers can race between non-final and
	// final observations for the same tx id.
	if err := ProcessAttestation(&state.attestations, &tx); err != nil {
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
		s.sendObservedTxAttestationsToThornado(ctx, obsTx, state, k.Inbound, k.AllowFutureObservation, false)
	}
}

// sendObservedTxAttestationsToThornado sends attestations to thornado via gRPC
func (s *AttestationGossip) sendObservedTxAttestationsToThornado(
	ctx context.Context,
	tx common.ObservedTx,
	state *AttestationState[*attestableObservedTx],
	inbound, allowFutureObservation, isQuorum bool,
) {
	attestations := state.UncommittedAttestations()
	if len(attestations) == 0 {
		s.logger.Debug().Msg("no uncommitted observed tx attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumTx(ctx, &common.QuorumTx{
		ObsTx:                  tx,
		Attestations:           attestations,
		Inbound:                inbound,
		AllowFutureObservation: allowFutureObservation,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum tx")
		return
	}

	s.logger.Info().Msgf("sent observed tx attestations to thornado - %s, id: %s, inbound: %t, final: %t, quorum: %t, attestations: %s",
		tx.Tx.Chain, tx.Tx.ID, inbound, tx.IsFinal(), isQuorum, state.State())

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
		s.sendNetworkFeeAttestationsToThornado(ctx, *state.Item, state, false)
	}
}

// sendNetworkFeeAttestationsToThornado sends network fee attestations to thornado via gRPC
func (s *AttestationGossip) sendNetworkFeeAttestationsToThornado(ctx context.Context, networkFee common.NetworkFee, state *AttestationState[*common.NetworkFee], isQuorum bool) {
	attestations := state.UncommittedAttestations()
	if len(attestations) == 0 {
		s.logger.Debug().Msg("no uncommitted network fee attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumNetworkFee(ctx, &common.QuorumNetworkFee{
		NetworkFee:   &networkFee,
		Attestations: attestations,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum network fee")
		return
	}

	s.logger.Info().Msgf("sent network fee attestations to thornado - %s, height: %d, quorum: %t, attestations: %s",
		networkFee.Chain, networkFee.Height, isQuorum, state.State())

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
		s.sendSolvencyAttestationsToThornado(ctx, *state.Item, state, false)
	}
}

// sendSolvencyAttestationsToThornado sends solvency attestations to thornado via gRPC
func (s *AttestationGossip) sendSolvencyAttestationsToThornado(ctx context.Context, solvency common.Solvency, state *AttestationState[*common.Solvency], isQuorum bool) {
	attestations := state.UncommittedAttestations()
	if len(attestations) == 0 {
		s.logger.Debug().Msg("no uncommitted solvency attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumSolvency(ctx, &common.QuorumSolvency{
		Solvency:     &solvency,
		Attestations: attestations,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum solvency")
		return
	}

	s.logger.Info().Msgf("sent solvency attestations to thornado - %s, height: %d, coins: %s, pubkey: %s, quorum: %t, attestations: %s",
		solvency.Chain, solvency.Height, solvency.Coins.String(), solvency.PubKey.String(), isQuorum, state.State())

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
		s.sendErrataAttestationsToThornado(ctx, *state.Item, state, false)
	}
}

// sendErrataAttestationsToThornado sends errata attestations to thornado via gRPC
func (s *AttestationGossip) sendErrataAttestationsToThornado(ctx context.Context, errata common.ErrataTx, state *AttestationState[*common.ErrataTx], isQuorum bool) {
	attestations := state.UncommittedAttestations()
	if len(attestations) == 0 {
		s.logger.Debug().Msg("no uncommitted errata attestations")
		return
	}
	// Send via gRPC to thornado
	if _, err := s.grpcClient.SendQuorumErrataTx(ctx, &common.QuorumErrataTx{
		ErrataTx:     &errata,
		Attestations: attestations,
	}); err != nil {
		s.logger.Error().Err(err).Msg("fail to send quorum errata")
		return
	}

	s.logger.Info().Msgf("sent errata attestations to thornado - %s - ID: %s - quorum: %t - attestations: %s", errata.Chain, errata.Id, isQuorum, state.State())

	// Mark attestations as sent
	state.MarkAttestationsSent(isQuorum)
}
