package observer

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/thornadocash/go-thornado/bifrost/metrics"
	"github.com/thornadocash/go-thornado/bifrost/p2p"
	"github.com/thornadocash/go-thornado/common"
)

// we have one semaphore per peer (per active validator node), so we don't need to prune that often
// but we do need to prune it, because we don't want to keep semaphores of nodes that aren't
// online anymore around forever.
const semaphorePruneInterval = 5 * time.Minute

type AttestationBatcher struct {
	observedTxBatch []*common.AttestTx
	networkFeeBatch []*common.AttestNetworkFee
	solvencyBatch   []*common.AttestSolvency
	errataTxBatch   []*common.AttestErrataTx

	batchPool sync.Pool

	mu             sync.Mutex
	batchInterval  time.Duration
	maxBatchSize   int64
	maxBatchSizeMu sync.Mutex
	peerTimeout    time.Duration // Timeout for peer send

	getActiveValidators func() map[peer.ID]bool

	lastBatchSent time.Time
	batchTicker   *time.Ticker
	forceSendChan chan struct{} // Channel to signal immediate send

	host    host.Host
	logger  zerolog.Logger
	metrics *batchMetrics

	peerMgr *peerManager
}

// Metrics for the batcher
type batchMetrics struct {
	BatchSends      prometheus.Counter
	BatchClears     prometheus.Counter
	MessagesBatched *prometheus.CounterVec // By type
	BatchSize       prometheus.Histogram
	BatchSendTime   prometheus.Histogram
}

// NewAttestationBatcher creates a new instance of AttestationBatcher
func NewAttestationBatcher(
	host host.Host,
	logger zerolog.Logger,
	m *metrics.Metrics,
	batchInterval time.Duration,
	maxBatchSize int64,
	peerTimeout time.Duration,
	peerConcurrentSends int,
) *AttestationBatcher {
	// Create batch metrics
	batchMetrics := &batchMetrics{
		BatchSends:      m.GetCounter(metrics.BatchSends),
		BatchClears:     m.GetCounter(metrics.BatchClears),
		MessagesBatched: m.GetCounterVec(metrics.MessagesBatched),
		BatchSize:       m.GetHistograms(metrics.BatchSize),
		BatchSendTime:   m.GetHistograms(metrics.BatchSendTime),
	}

	logger = logger.With().Str("module", "attestation_batcher").Logger()

	b := &AttestationBatcher{
		// Initialize with empty slices with initial capacity
		observedTxBatch: make([]*common.AttestTx, 0, maxBatchSize),
		networkFeeBatch: make([]*common.AttestNetworkFee, 0, maxBatchSize),
		solvencyBatch:   make([]*common.AttestSolvency, 0, maxBatchSize),
		errataTxBatch:   make([]*common.AttestErrataTx, 0, maxBatchSize),

		batchInterval: batchInterval,
		maxBatchSize:  maxBatchSize,
		peerTimeout:   peerTimeout,

		peerMgr: newPeerManager(logger, peerConcurrentSends),

		lastBatchSent: time.Time{}, // Zero time

		host:    host,
		logger:  logger,
		metrics: batchMetrics,

		forceSendChan: make(chan struct{}, 1), // Buffer of 1 to avoid blocking
	}

	// sync.Pool for reusing AttestationBatch objects to reduce memory allocations.
	b.batchPool = sync.Pool{New: b.newAttestationBatch}

	return b
}

func (b *AttestationBatcher) getMaxBatchSize() int64 {
	b.maxBatchSizeMu.Lock()
	defer b.maxBatchSizeMu.Unlock()
	return b.maxBatchSize
}

func (b *AttestationBatcher) newAttestationBatch() interface{} {
	maxBatchSize := b.getMaxBatchSize()
	return &common.AttestationBatch{
		AttestTxs:         make([]*common.AttestTx, 0, maxBatchSize), // Preallocate with maxBatchSize capacity
		AttestNetworkFees: make([]*common.AttestNetworkFee, 0, maxBatchSize),
		AttestSolvencies:  make([]*common.AttestSolvency, 0, maxBatchSize),
		AttestErrataTxs:   make([]*common.AttestErrataTx, 0, maxBatchSize),
	}
}

func (b *AttestationBatcher) setActiveValGetter(getter func() map[peer.ID]bool) {
	b.getActiveValidators = getter
}

func (b *AttestationBatcher) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.observedTxBatch = b.observedTxBatch[:0]
	b.networkFeeBatch = b.networkFeeBatch[:0]
	b.solvencyBatch = b.solvencyBatch[:0]
	b.errataTxBatch = b.errataTxBatch[:0]
}

// updateMaxBatchSize updates the maximum batch size at runtime.
func (b *AttestationBatcher) updateMaxBatchSize(newSize int64) {
	if newSize <= 0 {
		b.logger.Error().Int64("new_size", newSize).Msg("invalid batch size, must be greater than 0")
		return
	}

	b.maxBatchSizeMu.Lock()
	defer b.maxBatchSizeMu.Unlock()

	oldSize := b.maxBatchSize
	// Set the new max batch size
	b.maxBatchSize = newSize

	b.logger.Info().Int64("old_size", oldSize).Int64("new_size", newSize).Msg("max batch size updated")
}

func (b *AttestationBatcher) Start(ctx context.Context) {
	b.batchTicker = time.NewTicker(b.batchInterval)

	defer func() {
		b.batchTicker.Stop()
		close(b.forceSendChan)
	}()

	for {
		select {
		case <-ctx.Done():
			// Context is done, exit the loop
			// don't need to worry about sending any batches,
			// as next startup will send from the deck again.
			return
		case <-b.batchTicker.C:
			b.sendBatches(ctx, false)
		case <-b.forceSendChan:
			b.sendBatches(ctx, true)
		}
	}
}

func (b *AttestationBatcher) sendBatches(ctx context.Context, force bool) {
	b.mu.Lock()

	// Only send if we have messages to send or enough time has passed
	hasMessages := len(b.observedTxBatch) > 0 ||
		len(b.networkFeeBatch) > 0 ||
		len(b.solvencyBatch) > 0 ||
		len(b.errataTxBatch) > 0
	timeThresholdMet := time.Since(b.lastBatchSent) >= b.batchInterval

	if !hasMessages || (!timeThresholdMet && !force) {
		b.mu.Unlock()
		return
	}

	max := b.getMaxBatchSize()

	start := time.Now()
	// Get a batched message from the pool
	batch, ok := b.batchPool.Get().(*common.AttestationBatch)
	if !ok {
		batch = &common.AttestationBatch{
			AttestTxs:         make([]*common.AttestTx, 0, max),
			AttestNetworkFees: make([]*common.AttestNetworkFee, 0, max),
			AttestSolvencies:  make([]*common.AttestSolvency, 0, max),
			AttestErrataTxs:   make([]*common.AttestErrataTx, 0, max),
		}
	}

	clear := true
	batchSizeTx := int64(len(b.observedTxBatch))
	if batchSizeTx > max {
		clear = false
		batchSizeTx = max
	}
	batchSizeNF := int64(len(b.networkFeeBatch))
	if batchSizeNF > max {
		clear = false
		batchSizeNF = max
	}
	batchSizeSolvency := int64(len(b.solvencyBatch))
	if batchSizeSolvency > max {
		clear = false
		batchSizeSolvency = max
	}
	batchSizeErrata := int64(len(b.errataTxBatch))
	if batchSizeErrata > max {
		clear = false
		batchSizeErrata = max
	}
	// Populate the batch
	batch.AttestTxs = append(batch.AttestTxs[:0], b.observedTxBatch[:batchSizeTx]...)
	batch.AttestNetworkFees = append(batch.AttestNetworkFees[:0], b.networkFeeBatch[:batchSizeNF]...)
	batch.AttestSolvencies = append(batch.AttestSolvencies[:0], b.solvencyBatch[:batchSizeSolvency]...)
	batch.AttestErrataTxs = append(batch.AttestErrataTxs[:0], b.errataTxBatch[:batchSizeErrata]...)

	txCount := len(batch.AttestTxs)
	nfCount := len(batch.AttestNetworkFees)
	solvencyCount := len(batch.AttestSolvencies)
	errataCount := len(batch.AttestErrataTxs)

	b.observedTxBatch = b.observedTxBatch[batchSizeTx:]
	b.networkFeeBatch = b.networkFeeBatch[batchSizeNF:]
	b.solvencyBatch = b.solvencyBatch[batchSizeSolvency:]
	b.errataTxBatch = b.errataTxBatch[batchSizeErrata:]

	b.mu.Unlock()

	// Send to all peers
	b.broadcastToAllPeers(ctx, *batch)

	// Return the batch to the pool after clearing it
	batch.AttestTxs = batch.AttestTxs[:0]
	batch.AttestNetworkFees = batch.AttestNetworkFees[:0]
	batch.AttestSolvencies = batch.AttestSolvencies[:0]
	batch.AttestErrataTxs = batch.AttestErrataTxs[:0]
	b.batchPool.Put(batch)

	batchDuration := time.Since(start)
	b.metrics.MessagesBatched.WithLabelValues("observed_tx").Add(float64(txCount))
	b.metrics.MessagesBatched.WithLabelValues("network_fee").Add(float64(nfCount))
	b.metrics.MessagesBatched.WithLabelValues("solvency").Add(float64(solvencyCount))
	b.metrics.MessagesBatched.WithLabelValues("errata_tx").Add(float64(errataCount))
	b.metrics.BatchSendTime.Observe(batchDuration.Seconds())
	b.metrics.BatchSends.Inc()

	if clear {
		// Log batch clear
		b.logger.Debug().Msg("attestation batches cleared")

		// Update metrics
		b.metrics.BatchClears.Inc()

		return
	}
	b.lastBatchSent = time.Now()
}

// AddObservedTx adds an observed transaction attestation to the batch
func (b *AttestationBatcher) AddObservedTx(tx common.AttestTx) {
	maxBatchSize := b.getMaxBatchSize()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.observedTxBatch = append(b.observedTxBatch, &tx)

	// If we've reached the maximum batch size, trigger an immediate send
	if int64(len(b.observedTxBatch)) >= maxBatchSize {
		b.logger.Debug().
			Int("batch_size", len(b.observedTxBatch)).
			Msg("observed tx batch reached max size, triggering immediate send")

		// Use a separate goroutine to avoid deadlock since sendBatches also acquires the mu
		go b.triggerBatchSend()
	}
}

// AddNetworkFee adds a network fee attestation to the batch
func (b *AttestationBatcher) AddNetworkFee(fee common.AttestNetworkFee) {
	maxBatchSize := b.getMaxBatchSize()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.networkFeeBatch = append(b.networkFeeBatch, &fee)

	// If we've reached the maximum batch size, trigger an immediate send
	if int64(len(b.networkFeeBatch)) >= maxBatchSize {
		b.logger.Debug().
			Int("batch_size", len(b.networkFeeBatch)).
			Msg("network fee batch reached max size, triggering immediate send")

		go b.triggerBatchSend()
	}
}

// AddSolvency adds a solvency attestation to the batch
func (b *AttestationBatcher) AddSolvency(solvency common.AttestSolvency) {
	maxBatchSize := b.getMaxBatchSize()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.solvencyBatch = append(b.solvencyBatch, &solvency)

	// If we've reached the maximum batch size, trigger an immediate send
	if int64(len(b.solvencyBatch)) >= maxBatchSize {
		b.logger.Debug().
			Int("batch_size", len(b.solvencyBatch)).
			Msg("solvency batch reached max size, triggering immediate send")

		go b.triggerBatchSend()
	}
}

// AddErrataTx adds an errata transaction attestation to the batch
func (b *AttestationBatcher) AddErrataTx(errata common.AttestErrataTx) {
	maxBatchSize := b.getMaxBatchSize()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.errataTxBatch = append(b.errataTxBatch, &errata)

	// If we've reached the maximum batch size, trigger an immediate send
	if int64(len(b.errataTxBatch)) >= maxBatchSize {
		b.logger.Debug().
			Int("batch_size", len(b.errataTxBatch)).
			Msg("errata tx batch reached max size, triggering immediate send")

		go b.triggerBatchSend()
	}
}

// triggerBatchSend triggers an immediate batch send outside the regular interval
func (b *AttestationBatcher) triggerBatchSend() {
	select {
	case b.forceSendChan <- struct{}{}:
		// Successfully triggered a send
	default:
		// Channel is full, a send is already pending
	}
}

// broadcastToAllPeers sends the batch payload to all connected peers without blocking on slow peers
func (b *AttestationBatcher) broadcastToAllPeers(ctx context.Context, batch common.AttestationBatch) {
	// Marshal the batch
	payload, err := batch.Marshal()
	if err != nil {
		b.logger.Error().Err(err).Msg("failed to marshal attestation batch")
		return
	}

	peers := b.host.Peerstore().Peers()

	if b.getActiveValidators == nil {
		b.logger.Warn().Msg("active validator getter not set – skipping broadcast")
		return
	}
	// Skip self, send to all other active vals that we are peered with
	activeVals := b.getActiveValidators()
	var peersToSend []peer.ID
	for _, peerID := range peers {
		if peerID == b.host.ID() {
			continue
		}
		if _, ok := activeVals[peerID]; !ok {
			continue
		}
		peersToSend = append(peersToSend, peerID)
	}

	if len(peersToSend) == 0 {
		b.logger.Debug().Msg("no peers to broadcast to")
		return
	}

	b.logger.Debug().
		Int("peer_count", len(peersToSend)).
		Int("payload_bytes", len(payload)).
		Msg("broadcasting attestation batch to peers")

	// Launch each send operation in its own goroutine and don't wait for completion
	for _, p := range peersToSend {
		// Launch a goroutine for each peer and don't wait for completion
		go b.broadcastToPeer(ctx, p, payload)
	}

	// Function returns immediately without waiting for sends to complete
	b.logger.Debug().Msg("broadcast initiated to all peers")
}

func (b *AttestationBatcher) broadcastToPeer(ctx context.Context, peer peer.ID, payload []byte) {
	// Limit the number of concurrent sends to this peer
	sem, err := b.peerMgr.acquire(peer)
	if err != nil {
		b.logger.Error().Err(err).Msgf("fail to acquire semaphore for peer: %s", peer)
		return
	}

	defer b.peerMgr.release(sem)

	b.logger.Debug().Msgf("starting attestation send to peer: %s", peer)

	// Create a context with timeout for this specific peer
	peerCtx, cancel := context.WithTimeout(ctx, b.peerTimeout)
	defer cancel()
	stream, err := b.host.NewStream(peerCtx, peer, batchedAttestationProtocol)
	if err != nil {
		logAttestationStreamOpenError(b.logger, err, peer, string(batchedAttestationProtocol))
		return
	}

	b.sendPayloadToStream(peerCtx, stream, payload)

	b.logger.Debug().Msgf("completed attestation send to peer: %s", peer)
}

func (b *AttestationBatcher) sendPayloadToStream(ctx context.Context, stream network.Stream, payload []byte) {
	peer := stream.Conn().RemotePeer()
	logger := b.logger.With().Str("remote_peer", peer.String()).Logger()

	defer closeStream(logger, stream)

	if err := p2p.WriteStreamWithBufferWithContext(ctx, payload, stream); err != nil {
		b.logger.Error().Err(err).Msgf("fail to write payload to peer: %s", peer)
		return
	}

	// Wait for acknowledgment
	reply, err := p2p.ReadStreamWithBufferWithContext(ctx, stream)
	if err != nil {
		b.logger.Error().Err(err).Msgf("fail to read reply from peer: %s", peer)
		return
	}

	if string(reply) != p2p.StreamMsgDone {
		b.logger.Error().Msgf("unexpected reply from peer: %s", peer)
		return
	}

	b.logger.Debug().Msgf("attestation sent to peer: %s", peer)
}
