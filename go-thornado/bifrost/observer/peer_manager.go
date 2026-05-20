package observer

import (
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"
)

const semAcquireTimeout = 50 * time.Millisecond

type peerSemaphore struct {
	tokens   chan struct{}
	refCount int
	lastZero time.Time
	mu       sync.Mutex
}

type peerManager struct {
	logger zerolog.Logger

	semaphores map[peer.ID]*peerSemaphore
	mu         sync.Mutex
	limit      int
}

func newPeerManager(logger zerolog.Logger, limit int) *peerManager {
	return &peerManager{
		logger:     logger.With().Str("component", "peer_manager").Logger(),
		semaphores: make(map[peer.ID]*peerSemaphore),
		limit:      limit,
	}
}

func (m *peerManager) getLimit() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.limit
}

func (m *peerManager) getSemaphoreForAcquire(peer peer.ID) *peerSemaphore {
	m.mu.Lock()
	sem, exists := m.semaphores[peer]
	if !exists {
		// Create a new semaphore and fill it with tokens
		sem = &peerSemaphore{
			tokens:   make(chan struct{}, m.limit),
			refCount: 0,
			mu:       sync.Mutex{},
		}

		// Fill the channel with tokens
		for range m.limit {
			sem.tokens <- struct{}{}
		}

		m.semaphores[peer] = sem
	}
	m.mu.Unlock()

	sem.mu.Lock()
	sem.refCount++
	sem.mu.Unlock()
	return sem
}

func (m *peerManager) updateLimit(newLimit int) {
	if newLimit <= 0 {
		m.logger.Warn().Msgf("Attempted to update limit to invalid value: %d, ignoring", newLimit)
		return
	}

	m.mu.Lock()
	oldLimit := m.limit
	m.limit = newLimit

	// Update existing semaphores
	for peerID, sem := range m.semaphores {
		sem.mu.Lock()

		// Count how many tokens are currently in use
		inUseCount := cap(sem.tokens) - len(sem.tokens)

		// Create a new token channel with the new limit
		newTokens := make(chan struct{}, newLimit)

		// Fill available slots in the new channel
		availableSlots := newLimit - min(inUseCount, newLimit)
		for i := 0; i < availableSlots; i++ {
			newTokens <- struct{}{}
		}

		// Replace the tokens channel
		sem.tokens = newTokens
		sem.mu.Unlock()

		m.logger.Debug().
			Int("oldLimit", oldLimit).
			Int("newLimit", newLimit).
			Str("peer", peerID.String()).
			Int("tokensInUse", inUseCount).
			Int("availableSlots", availableSlots).
			Msg("Updated semaphore limit")
	}

	m.mu.Unlock()

	m.logger.Info().
		Int("oldLimit", oldLimit).
		Int("newLimit", newLimit).
		Int("updatedSemaphores", len(m.semaphores)).
		Msg("Updated peer manager limit")
}

func (m *peerManager) acquire(peer peer.ID) (*peerSemaphore, error) {
	sem := m.getSemaphoreForAcquire(peer)

	// Create a timeout channel
	timeout := time.After(semAcquireTimeout)

	sem.mu.Lock()
	tokens := sem.tokens
	sem.mu.Unlock()

	// Try to acquire token with timeout
	select {
	case <-tokens:
		// Successfully acquired token
		return sem, nil

	case <-timeout:
		// Timed out
		m.decRefCount(sem)
		return nil, fmt.Errorf("peer %s is busy", peer.String())
	}
}

func (m *peerManager) release(sem *peerSemaphore) {
	sem.mu.Lock()
	// If channel capacity has changed, we need to be careful about releasing
	if len(sem.tokens) < cap(sem.tokens) {
		sem.tokens <- struct{}{}
	}
	sem.mu.Unlock()

	// Clean up semaphore if this was the last reference
	m.decRefCount(sem)
}

func (m *peerManager) decRefCount(sem *peerSemaphore) {
	sem.mu.Lock()
	defer sem.mu.Unlock()
	sem.refCount--
	if sem.refCount <= 0 {
		// do not delete here, delete in main loop periodically if ref counts are zero
		sem.lastZero = time.Now()
	}
}

func (m *peerManager) prune() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for peerID, sem := range m.semaphores {
		sem.mu.Lock()
		if sem.refCount == 0 && time.Since(sem.lastZero) >= semaphorePruneInterval {
			delete(m.semaphores, peerID)
			m.logger.Debug().Msgf("pruned semaphore for peer: %s", peerID)
		}
		sem.mu.Unlock()
	}
}
