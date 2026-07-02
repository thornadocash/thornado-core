package observer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/p2p/conversion"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	"github.com/thornadocash/go-thornado/x/thornado/ebifrost"
	thornadotypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

const (
	// validators can get credit for observing a tx for up to this amount of time after it is committed, after which it count against a penalty.
	defaultLateObserveTimeout = 2 * time.Minute

	// retry local reconstruction when this node has peer attestations but no local attestation.
	defaultObservedTxRecoveryRetry = 5 * time.Second

	// Prune observed txs after this amount of time, even if they are not yet committed.
	// Gives some time for longer chain halts.
	// If chain halts for longer than this, validators will need to restart their bifrosts to re-share their attestations.
	defaultNonQuorumTimeout = 10 * time.Hour

	// minTimeBetweenAttestations is the minimum time between sending batches of attestations for a quorum tx to thornado.
	defaultMinTimeBetweenAttestations = 30 * time.Second

	// how often to prune old observed txs and check if late attestations should be sent.
	// should be less than lateObserveTimeout and minTimeBetweenAttestations by at least a factor of 2.
	defaultObserveReconcileInterval = 15 * time.Second

	// defaultAskPeers is the number of random peers to ask for their attestation state on startup.
	defaultAskPeers = 10

	// defaultAskPeersDelay is the delay before asking peers for their attestation state on startup.
	defaultAskPeersDelay = 5 * time.Second

	cachedKeysignPartyTTL = 1 * time.Minute

	defaultPeerTimeout = 20 * time.Second

	defaultPeerConcurrentSends    = 4
	defaultPeerConcurrentReceives = 5
	defaultMaxBatchSize           = 100
	defaultBatchInterval          = 2 * time.Second

	streamAckBegin  = "ack_begin"
	streamAckHeader = "ack_header"
	streamAckData   = "ack_data"
)

var (
	attestationStateProtocol   protocol.ID = "/p2p/attestation-state/v2"
	batchedAttestationProtocol protocol.ID = "/p2p/batched-attestations"

	// AttestationState protocol prefixes
	prefixSendState = []byte{0x01} // request

	prefixBatchBegin  = []byte{0x02} // start of a batch
	prefixBatchHeader = []byte{0x03} // header of a batch
	prefixBatchData   = []byte{0x04} // data of a batch
	prefixBatchEnd    = []byte{0x05} // end of a batch

	// Maximum number of QuorumTxs to send in a single batch when sending attestation state.
	maxQuorumTxsPerBatch = 100
)

// txKey contains the properties that are required to uniquely identify an observed tx
type txKey struct {
	Chain                  common.Chain
	ID                     common.TxID
	ObservedPubKey         common.PubKey
	UniqueHash             string
	AllowFutureObservation bool
	Finalized              bool
	Inbound                bool
}

func newObservedTxKey(tx common.ObservedTx, inbound, allowFutureObservation bool) txKey {
	uniqueHash := ""
	if tx.Tx.ID.IsEmpty() {
		uniqueHash = tx.Tx.Hash(tx.BlockHeight)
	}
	if tx.IsFinal() {
		allowFutureObservation = false
	}
	signBz, err := tx.GetSignablePayloadWithInbound(inbound)
	if err == nil {
		payloadHash := sha256.Sum256(signBz)
		uniqueHash = fmt.Sprintf("%X", payloadHash[:])
	}
	return txKey{
		Chain:                  tx.Tx.Chain,
		ID:                     tx.Tx.ID,
		ObservedPubKey:         tx.ObservedPubKey,
		UniqueHash:             uniqueHash,
		AllowFutureObservation: allowFutureObservation,
		Finalized:              tx.IsFinal(),
		Inbound:                inbound,
	}
}

type attestableObservedTx struct {
	*common.ObservedTx
	inbound bool
}

func (a *attestableObservedTx) GetSignablePayload() ([]byte, error) {
	return a.ObservedTx.GetSignablePayloadWithInbound(a.inbound)
}

func (a *attestableObservedTx) IsValid() bool {
	return a != nil && a.ObservedTx != nil
}

type KeysInterface interface {
	GetPrivateKey() (*secp256k1.PrivKey, error)
}

type EventClientInterface interface {
	Start()
	Stop()
	RegisterHandler(eventType string, handler func(*ebifrost.EventNotification))
}

// AttestationGossip handles observed tx attestations to/from other nodes
type AttestationGossip struct {
	logger zerolog.Logger
	host   host.Host

	grpcClient  ebifrost.LocalhostBifrostClient
	eventClient EventClientInterface
	bridge      thornadoclient.ThornadoBridge

	privKey cryptotypes.PrivKey // our private key, cached for performance
	pubKey  []byte              // our public key, cached for performance

	config config.BifrostAttestationGossipConfig

	// Generic maps for different attestation types
	observedTxs map[txKey]*AttestationState[*attestableObservedTx]
	networkFees map[common.NetworkFee]*AttestationState[*common.NetworkFee]
	solvencies  map[common.TxID]*AttestationState[*common.Solvency]
	errataTxs   map[common.ErrataTx]*AttestationState[*common.ErrataTx]
	mu          sync.Mutex

	observedTxsPool *AttestationStatePool[*attestableObservedTx]
	networkFeesPool *AttestationStatePool[*common.NetworkFee]
	solvenciesPool  *AttestationStatePool[*common.Solvency]
	errataTxsPool   *AttestationStatePool[*common.ErrataTx]

	activeVals map[peer.ID]bool // active val peer IDs
	avMu       sync.Mutex

	observerHandleObservedTxCommitted func(tx common.ObservedTx)
	observerRecoverObservedTx         func(ctx context.Context, tx common.ObservedTx, inbound, allowFutureObservation bool)
	observedTxRecoveryMu              sync.Mutex
	observedTxRecovery                map[txKey]time.Time

	cachedKeySignParties map[common.PubKey]cachedKeySignParty
	cachedKeySignMu      sync.Mutex

	batcher *AttestationBatcher

	// peerManager is used to limit the number of concurrent receives from a peer
	peerMgr *peerManager

	stateInitPeers map[peer.ID]bool // peers that we have asked for their attestation state
	stateInitMu    sync.Mutex
}

type cachedKeySignParty struct {
	keySignParty common.PubKeys
	lastUpdated  time.Time
}

// NewAttestationGossip create a new instance of AttestationGossip
func NewAttestationGossip(
	host host.Host,
	keys *thornadoclient.Keys,
	thornadoBifrostGRPCAddress string,
	bridge thornadoclient.ThornadoBridge,
	m *metrics.Metrics,
	config config.BifrostAttestationGossipConfig,
) (*AttestationGossip, error) {
	cc, err := grpc.NewClient(thornadoBifrostGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	normalizeConfig(&config)

	grpcClient := ebifrost.NewLocalhostBifrostClient(cc)
	eventClient := NewEventClient(grpcClient)

	pk, err := keys.GetPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to get private key: %w", err)
	}

	batcher := NewAttestationBatcher(
		host,
		log.With().Str("module", "attestation_batcher").Logger(),
		m,
		config.BatchInterval,
		config.MaxBatchSize,
		config.PeerTimeout,
		config.PeerConcurrentSends,
	)

	logger := log.With().Str("module", "attestation_gossip").Logger()

	s := &AttestationGossip{
		logger:      logger,
		host:        host,
		privKey:     pk,
		pubKey:      pk.PubKey().Bytes(),
		grpcClient:  grpcClient,
		config:      config,
		bridge:      bridge,
		eventClient: eventClient,

		// Initialize generic maps
		observedTxs:        make(map[txKey]*AttestationState[*attestableObservedTx]),
		networkFees:        make(map[common.NetworkFee]*AttestationState[*common.NetworkFee]),
		solvencies:         make(map[common.TxID]*AttestationState[*common.Solvency]),
		errataTxs:          make(map[common.ErrataTx]*AttestationState[*common.ErrataTx]),
		observedTxRecovery: make(map[txKey]time.Time),

		peerMgr: newPeerManager(logger, config.PeerConcurrentReceives),

		observedTxsPool: NewAttestationStatePool[*attestableObservedTx](),
		networkFeesPool: NewAttestationStatePool[*common.NetworkFee](),
		solvenciesPool:  NewAttestationStatePool[*common.Solvency](),
		errataTxsPool:   NewAttestationStatePool[*common.ErrataTx](),

		cachedKeySignParties: make(map[common.PubKey]cachedKeySignParty),

		batcher: batcher,
	}
	batcher.setActiveValGetter(s.getActiveValidators)
	// Register event handlers
	eventClient.RegisterHandler(ebifrost.EventQuorumTxCommitted, s.handleQuorumTxCommitted)
	eventClient.RegisterHandler(ebifrost.EventQuorumNetworkFeeCommitted, s.handleQuorumNetworkFeeCommitted)
	eventClient.RegisterHandler(ebifrost.EventQuorumSolvencyCommitted, s.handleQuorumSolvencyCommitted)
	eventClient.RegisterHandler(ebifrost.EventQuorumErrataTxCommitted, s.handleQuorumErrataTxCommitted)

	// Register stream handlers
	host.SetStreamHandler(attestationStateProtocol, s.handleStreamAttestationState)
	host.SetStreamHandler(batchedAttestationProtocol, s.handleStreamBatchedAttestations)

	return s, nil
}

// normalizeConfig ensures that the config has values for all fields.
func normalizeConfig(config *config.BifrostAttestationGossipConfig) {
	if config.ObserveReconcileInterval == 0 {
		config.ObserveReconcileInterval = defaultObserveReconcileInterval
	}
	if config.LateObserveTimeout == 0 {
		config.LateObserveTimeout = defaultLateObserveTimeout
	}
	if config.NonQuorumTimeout == 0 {
		config.NonQuorumTimeout = defaultNonQuorumTimeout
	}
	if config.MinTimeBetweenAttestations == 0 {
		config.MinTimeBetweenAttestations = defaultMinTimeBetweenAttestations
	}
	if config.AskPeers == 0 {
		config.AskPeers = defaultAskPeers
	}
	if config.AskPeersDelay == 0 {
		config.AskPeersDelay = defaultAskPeersDelay
	}
	if config.PeerTimeout == 0 {
		config.PeerTimeout = defaultPeerTimeout
	}
	if config.MaxBatchSize == 0 {
		config.MaxBatchSize = defaultMaxBatchSize
	}
	if config.PeerConcurrentSends == 0 {
		config.PeerConcurrentSends = defaultPeerConcurrentSends
	}
	if config.PeerConcurrentReceives == 0 {
		config.PeerConcurrentReceives = defaultPeerConcurrentReceives
	}
	if config.PeerConcurrentReceives < config.PeerConcurrentSends {
		// ensure that the number of concurrent receives is at least as large as the number of concurrent sends
		config.PeerConcurrentReceives = config.PeerConcurrentSends
	}
	if config.BatchInterval == 0 {
		config.BatchInterval = defaultBatchInterval
	}
}

// Set the active validators list
func (s *AttestationGossip) setActiveValidators(activeVals common.PubKeys) {
	activePeers := make(map[peer.ID]bool, len(activeVals))
	for _, pub := range activeVals {
		pk, err := cosmos.GetPubKeyFromBech32(cosmos.Bech32PubKeyTypeAccPub, pub.String())
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to convert bech32 pubkey to raw secp256k1 pubkey")
			continue
		}
		peerID, err := conversion.GetPeerIDFromSecp256PubKey(pk.Bytes())
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to convert secp256k1 pubkey to peer ID")
			continue
		}
		activePeers[peerID] = true
	}

	s.setActiveValidatorPeers(activePeers)
}

func (s *AttestationGossip) setActiveValidatorPeers(activePeers map[peer.ID]bool) {
	s.avMu.Lock()
	changed := !samePeerSet(s.activeVals, activePeers)
	s.activeVals = activePeers
	s.avMu.Unlock()

	if changed {
		s.clearAttestationState()
	}
}

func samePeerSet(a, b map[peer.ID]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for p := range a {
		if !b[p] {
			return false
		}
	}
	return true
}

func (s *AttestationGossip) clearAttestationState() {
	s.mu.Lock()
	for k, state := range s.observedTxs {
		delete(s.observedTxs, k)
		state.mu.Lock()
		s.observedTxsPool.PutAttestationState(state)
		state.mu.Unlock()
	}
	for k, state := range s.networkFees {
		delete(s.networkFees, k)
		state.mu.Lock()
		s.networkFeesPool.PutAttestationState(state)
		state.mu.Unlock()
	}
	for k, state := range s.solvencies {
		delete(s.solvencies, k)
		state.mu.Lock()
		s.solvenciesPool.PutAttestationState(state)
		state.mu.Unlock()
	}
	for k, state := range s.errataTxs {
		delete(s.errataTxs, k)
		state.mu.Lock()
		s.errataTxsPool.PutAttestationState(state)
		state.mu.Unlock()
	}
	s.mu.Unlock()

	if s.batcher != nil {
		s.batcher.Clear()
	}

	s.stateInitMu.Lock()
	s.stateInitPeers = nil
	s.stateInitMu.Unlock()

	s.observedTxRecoveryMu.Lock()
	s.observedTxRecovery = make(map[txKey]time.Time)
	s.observedTxRecoveryMu.Unlock()
}

// Get the number of active validators
func (s *AttestationGossip) activeValidatorCount() int {
	s.avMu.Lock()
	defer s.avMu.Unlock()
	return len(s.activeVals)
}

// Check if a public key belongs to an active validator
func (s *AttestationGossip) isActiveValidator(p peer.ID) bool {
	s.avMu.Lock()
	defer s.avMu.Unlock()
	_, ok := s.activeVals[p]
	return ok
}

func (s *AttestationGossip) getActiveValidators() map[peer.ID]bool {
	s.avMu.Lock()
	defer s.avMu.Unlock()
	return s.activeVals
}

// Get the keysign party for a specific public key
func (s *AttestationGossip) getKeysignParty(pubKey common.PubKey) (common.PubKeys, error) {
	s.cachedKeySignMu.Lock()
	defer s.cachedKeySignMu.Unlock()

	if cached, ok := s.cachedKeySignParties[pubKey]; ok {
		return cached.keySignParty, nil
	}

	keySignParty, err := s.bridge.GetKeysignParty(pubKey)
	if err != nil {
		return nil, fmt.Errorf("fail to get key sign party: %w", err)
	}

	s.cachedKeySignParties[pubKey] = cachedKeySignParty{
		keySignParty: keySignParty,
		lastUpdated:  time.Now(),
	}

	return keySignParty, nil
}

func (s *AttestationGossip) observedTxQuorumTotal(allowFutureObservation bool, pubKey common.PubKey) (int, bool) {
	if allowFutureObservation {
		keysignParty, err := s.getKeysignParty(pubKey)
		if err != nil {
			s.logger.Error().Err(err).Msg("fail to get key sign party")
			return 0, false
		}
		return len(keysignParty), true
	}
	return s.activeValidatorCount(), true
}

func (s *AttestationGossip) quorumTxObservedTxQuorum(qtx common.QuorumTx) (bool, int) {
	total, ok := s.observedTxQuorumTotal(qtx.AllowFutureObservation, qtx.ObsTx.ObservedPubKey)
	if !ok {
		return false, total
	}
	return thornadotypes.HasSuperMajority(len(qtx.Attestations), total), total
}

func (s *AttestationGossip) observedTxCommittedQuorum(k txKey) bool {
	total, ok := s.observedTxQuorumTotal(k.AllowFutureObservation, k.ObservedPubKey)
	if !ok {
		return false
	}

	s.mu.Lock()
	state, ok := s.observedTxs[k]
	if ok {
		state.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return false
	}
	defer state.mu.Unlock()

	return thornadotypes.HasSuperMajority(state.CommittedCount(), total)
}

func (s *AttestationGossip) SetObserverHandleObservedTxCommitted(o *Observer) {
	s.observerHandleObservedTxCommitted = o.handleObservedTxCommitted
	s.observerRecoverObservedTx = o.recoverObservedTx
}

// Handle a committed quorum transaction event
func (s *AttestationGossip) handleQuorumTxCommitted(en *ebifrost.EventNotification) {
	s.logger.Debug().Msg("handling quorum tx committed event")

	var qtx common.QuorumTx
	if err := qtx.Unmarshal(en.Payload); err != nil {
		s.logger.Error().Err(err).Msg("fail to unmarshal quorum tx")
		return
	}

	hasObservedTxQuorum, quorumTotal := s.quorumTxObservedTxQuorum(qtx)
	if s.observerHandleObservedTxCommitted != nil {
		// If our attestation is in a committed observed tx quorum, remove it from our observer deck.
		for _, att := range qtx.Attestations {
			if bytes.Equal(att.PubKey, s.pubKey) {
				if hasObservedTxQuorum {
					s.logger.Debug().Msg("our attestation is in a committed observed tx quorum, passing to observer to remove from ondeck")
					s.observerHandleObservedTxCommitted(qtx.ObsTx)
				} else {
					s.logger.Debug().Msgf("our attestation committed without observed tx quorum, keeping observer deck: %d/%d", len(qtx.Attestations), quorumTotal)
				}
				break
			}
		}
	}

	k := newObservedTxKey(qtx.ObsTx, qtx.Inbound, qtx.AllowFutureObservation)

	s.mu.Lock()
	as, ok := s.observedTxs[k]
	if ok {
		// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
		// This order must be maintained everywhere to avoid deadlock.
		as.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	defer as.mu.Unlock()
	as.MarkAttestationsCommitted(qtx.Attestations)
}

// Handle a committed quorum network fee event
func (s *AttestationGossip) handleQuorumNetworkFeeCommitted(en *ebifrost.EventNotification) {
	s.logger.Debug().Msg("handling quorum network fee committed event")

	var qnf common.QuorumNetworkFee
	if err := qnf.Unmarshal(en.Payload); err != nil {
		s.logger.Error().Err(err).Msg("fail to unmarshal quorum network fee")
		return
	}

	s.mu.Lock()
	as, ok := s.networkFees[*qnf.NetworkFee]
	if ok {
		// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
		// This order must be maintained everywhere to avoid deadlock.
		as.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	defer as.mu.Unlock()
	as.MarkAttestationsCommitted(qnf.Attestations)
}

// Handle a committed quorum solvency event
func (s *AttestationGossip) handleQuorumSolvencyCommitted(en *ebifrost.EventNotification) {
	s.logger.Debug().Msg("handling quorum solvency committed event")

	var qs common.QuorumSolvency
	if err := qs.Unmarshal(en.Payload); err != nil {
		s.logger.Error().Err(err).Msg("fail to unmarshal quorum solvency")
		return
	}

	s.mu.Lock()
	as, ok := s.solvencies[qs.Solvency.Id]
	if ok {
		// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
		// This order must be maintained everywhere to avoid deadlock.
		as.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	defer as.mu.Unlock()
	as.MarkAttestationsCommitted(qs.Attestations)
}

// Handle a committed quorum errata tx event
func (s *AttestationGossip) handleQuorumErrataTxCommitted(en *ebifrost.EventNotification) {
	s.logger.Debug().Msg("handling quorum errata tx committed event")

	var qe common.QuorumErrataTx
	if err := qe.Unmarshal(en.Payload); err != nil {
		s.logger.Error().Err(err).Msg("fail to unmarshal quorum errata")
		return
	}

	s.mu.Lock()
	as, ok := s.errataTxs[*qe.ErrataTx]
	if ok {
		// note, purposefully locking the single state mutex prior to releasing the global mutex to avoid race in between.
		// This order must be maintained everywhere to avoid deadlock.
		as.mu.Lock()
	}
	s.mu.Unlock()
	if !ok {
		return
	}

	defer as.mu.Unlock()
	as.MarkAttestationsCommitted(qe.Attestations)
}

type observedTxRecoveryRequest struct {
	tx                     common.ObservedTx
	inbound                bool
	allowFutureObservation bool
}

func (s *AttestationGossip) maybeRecoverObservedTx(k txKey, state *AttestationState[*attestableObservedTx]) (observedTxRecoveryRequest, bool) {
	if s.observerRecoverObservedTx == nil || !s.isActiveValidator(s.host.ID()) || k.ID.IsEmpty() {
		return observedTxRecoveryRequest{}, false
	}
	if state == nil || state.Item == nil || state.Item.ObservedTx == nil || state.AttestationCount() == 0 {
		return observedTxRecoveryRequest{}, false
	}
	haveLocalAttestation, _ := state.AttestationStatus(s.pubKey)
	if haveLocalAttestation {
		return observedTxRecoveryRequest{}, false
	}

	now := time.Now()
	s.observedTxRecoveryMu.Lock()
	defer s.observedTxRecoveryMu.Unlock()
	if s.observedTxRecovery == nil {
		s.observedTxRecovery = make(map[txKey]time.Time)
	}
	if last, ok := s.observedTxRecovery[k]; ok && now.Sub(last) < defaultObservedTxRecoveryRetry {
		return observedTxRecoveryRequest{}, false
	}
	s.observedTxRecovery[k] = now

	return observedTxRecoveryRequest{
		tx:                     *state.Item.ObservedTx,
		inbound:                k.Inbound,
		allowFutureObservation: k.AllowFutureObservation,
	}, true
}

// Start the attestation gossip service
func (s *AttestationGossip) Start(ctx context.Context) {
	ticker := time.NewTicker(s.config.ObserveReconcileInterval)

	startupDelay := s.config.AskPeersDelay
	delayTimer := time.NewTimer(startupDelay)
	semPruneTicker := time.NewTicker(semaphorePruneInterval)

	defer func() {
		ticker.Stop()
		delayTimer.Stop()
		semPruneTicker.Stop()
	}()

	s.reconcileConfigConfigs()

	go s.batcher.Start(ctx)

	for {
		select {
		case <-ticker.C:
			var recoveryRequests []observedTxRecoveryRequest

			// prune old attestations and check for late ones to send
			s.mu.Lock()

			// Prune observed transactions
			for k, state := range s.observedTxs {
				state.mu.Lock()
				if state.ExpiredAfterQuorum(s.config.LateObserveTimeout, s.config.NonQuorumTimeout) {
					delete(s.observedTxs, k)
					s.observedTxRecoveryMu.Lock()
					delete(s.observedTxRecovery, k)
					s.observedTxRecoveryMu.Unlock()
					s.observedTxsPool.PutAttestationState(state)
				} else {
					if req, ok := s.maybeRecoverObservedTx(k, state); ok {
						recoveryRequests = append(recoveryRequests, req)
					}
					if state.ShouldSendLate(s.config.MinTimeBetweenAttestations) {
						s.logger.Debug().Msg("sending late observed tx attestations")

						s.sendObservedTxAttestationsToThornado(ctx, *state.Item.ObservedTx, state, k.Inbound, k.AllowFutureObservation, false)
					}
				}
				state.mu.Unlock()
			}

			// Prune network fees
			for k, state := range s.networkFees {
				state.mu.Lock()
				if state.ExpiredAfterQuorum(s.config.LateObserveTimeout, s.config.NonQuorumTimeout) {
					delete(s.networkFees, k)
					s.networkFeesPool.PutAttestationState(state)
				} else if state.ShouldSendLate(s.config.MinTimeBetweenAttestations) {
					s.logger.Debug().Msg("sending late network fee attestations")
					s.sendNetworkFeeAttestationsToThornado(ctx, *state.Item, state, false)
				}
				state.mu.Unlock()
			}

			// Prune solvencies
			for k, state := range s.solvencies {
				state.mu.Lock()
				if state.ExpiredAfterQuorum(s.config.LateObserveTimeout, s.config.NonQuorumTimeout) {
					delete(s.solvencies, k)
					s.solvenciesPool.PutAttestationState(state)
				} else if state.ShouldSendLate(s.config.MinTimeBetweenAttestations) {
					s.logger.Debug().Msg("sending late solvency attestations")
					s.sendSolvencyAttestationsToThornado(ctx, *state.Item, state, false)
				}
				state.mu.Unlock()
			}

			// Prune errata transactions
			for k, state := range s.errataTxs {
				state.mu.Lock()
				if state.ExpiredAfterQuorum(s.config.LateObserveTimeout, s.config.NonQuorumTimeout) {
					delete(s.errataTxs, k)
					s.errataTxsPool.PutAttestationState(state)
				} else if state.ShouldSendLate(s.config.MinTimeBetweenAttestations) {
					s.logger.Debug().Msg("sending late errata attestations")
					s.sendErrataAttestationsToThornado(ctx, *state.Item, state, false)
				}
				state.mu.Unlock()
			}

			s.mu.Unlock()

			for _, req := range recoveryRequests {
				s.observerRecoverObservedTx(ctx, req.tx, req.inbound, req.allowFutureObservation)
			}

			// Prune cached keysign parties
			s.cachedKeySignMu.Lock()
			for pk, cached := range s.cachedKeySignParties {
				if time.Since(cached.lastUpdated) > cachedKeysignPartyTTL {
					delete(s.cachedKeySignParties, pk)
				}
			}
			s.cachedKeySignMu.Unlock()

			s.reconcileConfigConfigs()

		case <-delayTimer.C:
			s.eventClient.Start()

		case <-semPruneTicker.C:
			// Periodically prune semaphores that have been idle for a while
			s.peerMgr.prune()
			s.batcher.peerMgr.prune()
		case <-ctx.Done():
			s.eventClient.Stop()
			return
		}
	}
}

func (s *AttestationGossip) reconcileConfigConfigs() {
}
