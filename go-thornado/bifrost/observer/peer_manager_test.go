package observer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerManager(t *testing.T) {
	// Create a test logger
	logger := zerolog.Nop()

	t.Run("acquires and releases semaphore", func(t *testing.T) {
		pm := newPeerManager(logger, 2)
		peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)

		// First acquisition should succeed
		sem1, err := pm.acquire(peerID)
		require.NoError(t, err)
		require.NotNil(t, sem1)

		// Second acquisition should also succeed (limit is 2)
		sem2, err := pm.acquire(peerID)
		require.NoError(t, err)
		require.NotNil(t, sem2)

		// Third acquisition should timeout/fail
		sem3, err := pm.acquire(peerID)
		assert.Error(t, err)
		assert.Nil(t, sem3)

		// Release one token
		pm.release(sem1)

		// Now acquisition should succeed again
		sem4, err := pm.acquire(peerID)
		require.NoError(t, err)
		require.NotNil(t, sem4)

		// Release remaining tokens
		pm.release(sem2)
		pm.release(sem4)
	})

	t.Run("prunes unused semaphores", func(t *testing.T) {
		pm := newPeerManager(logger, 2)

		// Create two different peer IDs
		peerID1, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)
		peerID2, err := peer.Decode("QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")
		require.NoError(t, err)

		// Acquire and release for both peers
		sem1, err := pm.acquire(peerID1)
		require.NoError(t, err)
		sem2, err := pm.acquire(peerID2)
		require.NoError(t, err)

		pm.release(sem1)
		pm.release(sem2)

		// Verify both peers have semaphores
		pm.mu.Lock()
		assert.Len(t, pm.semaphores, 2)
		pm.mu.Unlock()

		// Override the lastZero time to simulate that peerID1's semaphore has been unused for longer than the prune interval
		pm.mu.Lock()
		pm.semaphores[peerID1].lastZero = time.Now().Add(-2 * semaphorePruneInterval)
		pm.mu.Unlock()

		// Run prune
		pm.prune()

		// Verify peerID1's semaphore was pruned, but peerID2's remains
		pm.mu.Lock()
		assert.Len(t, pm.semaphores, 1)
		_, exists := pm.semaphores[peerID1]
		assert.False(t, exists)
		_, exists = pm.semaphores[peerID2]
		assert.True(t, exists)
		pm.mu.Unlock()
	})

	t.Run("handles concurrent operations", func(t *testing.T) {
		pm := newPeerManager(logger, 5)
		peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)

		var wg sync.WaitGroup
		activeCount := 0
		maxActive := 0
		var countMu sync.Mutex

		// Launch 20 concurrent goroutines all trying to acquire
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				sem, err := pm.acquire(peerID)
				if err == nil {
					// Successfully acquired, track concurrent usage
					countMu.Lock()
					activeCount++
					if activeCount > maxActive {
						maxActive = activeCount
					}
					countMu.Unlock()

					// Simulate work
					time.Sleep(10 * time.Millisecond)

					countMu.Lock()
					activeCount--
					countMu.Unlock()

					pm.release(sem)
				}
			}()
		}

		wg.Wait()

		// Verify concurrency was limited
		assert.LessOrEqual(t, maxActive, 5, "Concurrency limit should be respected")
		assert.Greater(t, maxActive, 0, "At least some operations should succeed")

		// After all operations, semaphore should still exist but have 0 active tokens
		pm.mu.Lock()
		defer pm.mu.Unlock()
		sem, exists := pm.semaphores[peerID]
		assert.True(t, exists)
		assert.Equal(t, 0, sem.refCount)
	})
}

func TestPeerManagerUpdateLimit(t *testing.T) {
	logger := zerolog.Nop()

	t.Run("expanding limit allows more concurrent operations", func(t *testing.T) {
		pm := newPeerManager(logger, 2)
		peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)

		// Acquire tokens up to the initial limit
		sem1, err := pm.acquire(peerID)
		require.NoError(t, err)
		sem2, err := pm.acquire(peerID)
		require.NoError(t, err)

		// Third acquisition should fail with initial limit
		_, err = pm.acquire(peerID)
		assert.Error(t, err, "Should not be able to acquire more than the limit")

		// Expand the limit
		pm.updateLimit(4)

		// Now we should be able to acquire 2 more tokens
		sem3, err := pm.acquire(peerID)
		require.NoError(t, err)
		sem4, err := pm.acquire(peerID)
		require.NoError(t, err)

		// Fifth acquisition should still fail
		_, err = pm.acquire(peerID)
		assert.Error(t, err, "Should not be able to acquire more than the new limit")

		// Clean up
		pm.release(sem1)
		pm.release(sem2)
		pm.release(sem3)
		pm.release(sem4)
	})

	t.Run("contracting limit drops excess operations", func(t *testing.T) {
		pm := newPeerManager(logger, 5)
		peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)

		// Acquire 4 tokens
		var semaphores []*peerSemaphore
		for i := 0; i < 4; i++ {
			var sem *peerSemaphore
			sem, err = pm.acquire(peerID)
			require.NoError(t, err)
			semaphores = append(semaphores, sem)
		}

		// Reduce limit to 2
		pm.updateLimit(2)

		// Check capacity of the channel
		pm.mu.Lock()
		capacity := cap(pm.semaphores[peerID].tokens)
		available := len(pm.semaphores[peerID].tokens)
		inUse := capacity - available
		pm.mu.Unlock()

		assert.Equal(t, 2, capacity, "Channel capacity should be 2 after reduction")
		assert.Equal(t, 0, available, "All tokens should be in use")
		assert.Equal(t, 2, inUse, "Should have 2 tokens in use after reduction")

		// Try to acquire - should fail as the new limit is fully utilized
		_, err = pm.acquire(peerID)
		assert.Error(t, err, "Should not be able to acquire more tokens after limit reduction")

		// Release one token
		pm.release(semaphores[0])

		// Now we should be able to acquire one more token
		newSem, err := pm.acquire(peerID)
		require.NoError(t, err)
		semaphores = append(semaphores, newSem)

		// Clean up remaining tokens
		for _, sem := range semaphores {
			// Skip already released tokens
			if sem == semaphores[0] {
				continue
			}
			// Release up to the max capacity
			if inUse > 0 {
				pm.release(sem)
				inUse--
			}
		}
	})

	t.Run("rejects invalid limit values", func(t *testing.T) {
		pm := newPeerManager(logger, 3)

		// Initial state
		assert.Equal(t, 3, pm.limit)

		// Try to set zero limit
		pm.updateLimit(0)
		assert.Equal(t, 3, pm.limit, "Limit should not change when updated to 0")

		// Try to set negative limit
		pm.updateLimit(-5)
		assert.Equal(t, 3, pm.limit, "Limit should not change when updated to negative value")

		// Valid update
		pm.updateLimit(10)
		assert.Equal(t, 10, pm.limit, "Limit should update to valid value")
	})

	t.Run("updates multiple peer semaphores consistently", func(t *testing.T) {
		pm := newPeerManager(logger, 3)

		// Create two different peer IDs
		peerID1, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)
		peerID2, err := peer.Decode("QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ")
		require.NoError(t, err)

		// Acquire tokens for both peers
		sem1, err := pm.acquire(peerID1)
		require.NoError(t, err)
		sem2, err := pm.acquire(peerID1)
		require.NoError(t, err)

		sem3, err := pm.acquire(peerID2)
		require.NoError(t, err)
		sem4, err := pm.acquire(peerID2)
		require.NoError(t, err)

		// Update limit to 4
		pm.updateLimit(4)

		// Both peers should be able to acquire additional tokens
		sem5, err := pm.acquire(peerID1)
		require.NoError(t, err)
		sem6, err := pm.acquire(peerID2)
		require.NoError(t, err)

		// Clean up
		pm.release(sem1)
		pm.release(sem2)
		pm.release(sem3)
		pm.release(sem4)
		pm.release(sem5)
		pm.release(sem6)
	})

	t.Run("concurrent limit updates and acquisitions", func(t *testing.T) {
		pm := newPeerManager(logger, 5)
		peerID, err := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
		require.NoError(t, err)

		var wg sync.WaitGroup
		concurrentOps := 50
		successCount := int32(0)

		// Start some goroutines that acquire/release in a loop
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for j := 0; j < concurrentOps; j++ {
					sem, err := pm.acquire(peerID)
					if err == nil {
						// Simulate work
						time.Sleep(1 * time.Millisecond)
						pm.release(sem)
						atomic.AddInt32(&successCount, 1)
					}
					// Brief pause to create more interleaving
					time.Sleep(1 * time.Millisecond)
				}
			}()
		}

		// While those are running, update the limit several times
		limitUpdates := []int{2, 10, 3, 8, 5}
		for _, newLimit := range limitUpdates {
			time.Sleep(5 * time.Millisecond)
			pm.updateLimit(newLimit)
		}

		wg.Wait()

		// Verify we had some successes
		assert.Greater(t, int(successCount), 0, "Some operations should have succeeded")
	})
}
