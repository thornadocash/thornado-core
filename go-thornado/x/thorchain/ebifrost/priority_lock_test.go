package ebifrost

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBasicLockUnlock tests simple locking and unlocking
func TestBasicLockUnlock(t *testing.T) {
	lock := NewPriorityRWLock()

	// Should be able to acquire a write lock
	lock.Lock()

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	// Should be able to release the write lock
	lock.Unlock()
}

// TestMultipleWriters tests that writers have exclusive access
func TestMultipleWriters(t *testing.T) {
	lock := NewPriorityRWLock()
	counter := 0
	writerActive := int32(0)
	concurrentWrites := int32(0)

	var wg sync.WaitGroup
	numWriters := 5

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			lock.Lock()
			// Check and set writer active
			if atomic.AddInt32(&writerActive, 1) > 1 {
				atomic.AddInt32(&concurrentWrites, 1)
				t.Errorf("Multiple writers active simultaneously")
			}

			// Simulate work
			time.Sleep(20 * time.Millisecond)
			counter++

			atomic.AddInt32(&writerActive, -1)
			lock.Unlock()
		}(i)
	}

	wg.Wait()

	if counter != numWriters {
		t.Errorf("Expected counter to be %d, got %d", numWriters, counter)
	}

	if concurrentWrites > 0 {
		t.Errorf("Detected %d concurrent writes", concurrentWrites)
	}
}

// TestMultipleReaders tests that readers can access concurrently
func TestMultipleReaders(t *testing.T) {
	lock := NewPriorityRWLock()
	readersActive := int32(0)
	maxConcurrentReaders := int32(0)

	var wg sync.WaitGroup
	numReaders := 10

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			lock.RLock()
			// Track concurrent readers
			current := atomic.AddInt32(&readersActive, 1)

			// Update max if needed
			for {
				max := atomic.LoadInt32(&maxConcurrentReaders)
				if current <= max {
					break
				}
				if atomic.CompareAndSwapInt32(&maxConcurrentReaders, max, current) {
					break
				}
			}

			// Simulate reading
			time.Sleep(50 * time.Millisecond)

			atomic.AddInt32(&readersActive, -1)
			lock.RUnlock()
		}(i)
	}

	wg.Wait()

	if maxConcurrentReaders < int32(numReaders/2) {
		t.Errorf("Expected multiple concurrent readers, max was only %d", maxConcurrentReaders)
	}
}

// TestReadersBlockWriter tests that active readers block writers
func TestReadersBlockWriter(t *testing.T) {
	lock := NewPriorityRWLock()
	readerDone := make(chan struct{})
	writerStarted := make(chan struct{})
	writerGotLock := make(chan struct{})

	// Start a reader
	go func() {
		lock.RLock()
		// Signal that we're holding the read lock
		close(writerStarted)

		// Hold the lock for a while
		time.Sleep(100 * time.Millisecond)

		lock.RUnlock()
		close(readerDone)
	}()

	// Wait for reader to get lock
	<-writerStarted

	// Start a writer, which should block
	go func() {
		lock.Lock()
		// Signal that we got the lock
		close(writerGotLock)

		// Hold the lock briefly
		time.Sleep(10 * time.Millisecond)

		lock.Unlock()
	}()

	// Writer should not get lock before reader is done
	select {
	case <-writerGotLock:
		t.Errorf("Writer got lock while reader was active")
	case <-readerDone:
		// This is expected - reader finished
	case <-time.After(200 * time.Millisecond):
		t.Errorf("Test timed out")
	}

	// After reader is done, writer should get the lock
	select {
	case <-writerGotLock:
		// This is expected - writer got the lock
	case <-time.After(50 * time.Millisecond):
		t.Errorf("Writer didn't get lock after reader was done")
	}
}

// TestPriorityReaderCutsLine tests that priority readers get access before waiting writers
func TestPriorityReaderCutsLine(t *testing.T) {
	lock := NewPriorityRWLock()
	writerHoldingLock := make(chan struct{})
	writerReleasedLock := make(chan struct{})
	priorityReaderGotLock := make(chan struct{})
	normalReaderGotLock := make(chan struct{})
	secondWriterGotLock := make(chan struct{})

	// Start a writer that holds the lock
	go func() {
		lock.Lock()
		close(writerHoldingLock)

		// Hold the lock for a bit
		time.Sleep(50 * time.Millisecond)

		lock.Unlock()
		close(writerReleasedLock)
	}()

	<-writerHoldingLock

	// Start another writer that will be waiting
	go func() {
		lock.Lock()
		close(secondWriterGotLock)

		// Hold briefly
		time.Sleep(10 * time.Millisecond)

		lock.Unlock()
	}()

	// Give the second writer time to queue up
	time.Sleep(10 * time.Millisecond)

	// Start a priority reader
	go func() {
		lock.RLockPriority()
		close(priorityReaderGotLock)

		// Hold briefly
		time.Sleep(30 * time.Millisecond)

		lock.RUnlock()
	}()

	// Start a normal reader
	go func() {
		lock.RLock()
		close(normalReaderGotLock)

		lock.RUnlock()
	}()

	// Wait for first writer to release
	<-writerReleasedLock

	// The priority reader should get access before the waiting writer
	select {
	case <-priorityReaderGotLock:
		// This is expected
	case <-secondWriterGotLock:
		t.Errorf("Second writer got lock before priority reader")
	case <-normalReaderGotLock:
		t.Errorf("Normal reader got lock before priority reader")
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Test timed out")
	}

	// Normal reader should still be blocked because of waiting writer
	select {
	case <-normalReaderGotLock:
		t.Errorf("Normal reader got lock before second writer")
	case <-time.After(10 * time.Millisecond):
		// This is expected
	}

	// After priority reader is done, the waiting writer should get access
	// and only after that should the normal reader get access
	timer1 := time.NewTimer(200 * time.Millisecond)

	var gotLock []string

	for i := 0; i < 2; i++ {
		select {
		case <-secondWriterGotLock:
			secondWriterGotLock = nil // prevent duplicate
			gotLock = append(gotLock, "writer")
		case <-normalReaderGotLock:
			normalReaderGotLock = nil // prevent duplicate
			gotLock = append(gotLock, "reader")
		case <-timer1.C:
			t.Errorf("Test timed out waiting for locks")
			return
		}
	}

	if len(gotLock) != 2 || gotLock[0] != "writer" || gotLock[1] != "reader" {
		t.Errorf("Expected writer then reader, got %v", gotLock)
	}
}

// TestConcurrentAccess simulates concurrent readers, writers and priority readers
func TestConcurrentAccess(t *testing.T) {
	lock := NewPriorityRWLock()
	data := int32(0)

	iterations := 10
	readers := 3
	writers := 2
	priorityReaders := 2

	var wg sync.WaitGroup

	// Set a timeout for the entire test using a context
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start writer goroutines
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Check if test timed out
				select {
				case <-ctx.Done():
					t.Logf("Writer %d aborted due to timeout", id)
					return
				default:
					// Continue with test
				}

				lock.Lock()
				// Update data
				atomic.AddInt32(&data, 1)

				// Hold lock briefly
				time.Sleep(1 * time.Millisecond)

				lock.Unlock()

				// Sleep between operations
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Start regular reader goroutines
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Check if test timed out
				select {
				case <-ctx.Done():
					t.Logf("Reader %d aborted due to timeout", id)
					return
				default:
					// Continue with test
				}

				lock.RLock()
				// Read data (no modification)
				_ = atomic.LoadInt32(&data)

				// Hold lock briefly
				time.Sleep(1 * time.Millisecond)

				lock.RUnlock()

				// Sleep between operations
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Start priority reader goroutines
	for i := 0; i < priorityReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Check if test timed out
				select {
				case <-ctx.Done():
					t.Logf("Priority reader %d aborted due to timeout", id)
					return
				default:
					// Continue with test
				}

				lock.RLockPriority()
				// Read data (no modification)
				_ = atomic.LoadInt32(&data)

				// Hold lock briefly
				time.Sleep(1 * time.Millisecond)

				lock.RUnlock()

				// Sleep between operations
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Wait for completion or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("Test timed out after 3 seconds")
	}

	// Verify data was updated correctly
	expectedData := int32(writers * iterations)
	if atomic.LoadInt32(&data) != expectedData {
		t.Errorf("Expected data to be %d, got %d", expectedData, atomic.LoadInt32(&data))
	}
}

// TestWritersBlockEachOther tests that writers have exclusive access
func TestWritersBlockEachOther(t *testing.T) {
	lock := NewPriorityRWLock()
	firstWriterHasLock := make(chan struct{})
	firstWriterDone := make(chan struct{})
	secondWriterHasLock := make(chan struct{})

	// First writer takes lock
	go func() {
		lock.Lock()
		close(firstWriterHasLock)

		// Hold the lock
		time.Sleep(100 * time.Millisecond)

		lock.Unlock()
		close(firstWriterDone)
	}()

	<-firstWriterHasLock

	// Second writer should block
	go func() {
		lock.Lock()
		close(secondWriterHasLock)

		// Hold briefly
		time.Sleep(10 * time.Millisecond)

		lock.Unlock()
	}()

	// Second writer shouldn't get lock while first writer has it
	select {
	case <-secondWriterHasLock:
		t.Errorf("Second writer got lock while first writer had it")
	case <-time.After(50 * time.Millisecond):
		// This is expected
	}

	// After first writer releases, second writer should get it
	<-firstWriterDone

	select {
	case <-secondWriterHasLock:
		// This is expected
	case <-time.After(50 * time.Millisecond):
		t.Errorf("Second writer didn't get lock after first writer released")
	}
}

// TestPriorityReaderStarvation ensures that priority readers don't starve writers indefinitely
func TestPriorityReaderStarvation(t *testing.T) {
	lock := NewPriorityRWLock()
	const numPriorityReaders = 10
	writerGotLock := make(chan struct{})

	// First, acquire a write lock and then release it to ensure
	// waiting writers are properly queued
	lock.Lock()
	lock.Unlock() // nolint:staticcheck

	// Start a writer that will initially block
	go func() {
		lock.Lock()
		close(writerGotLock)
		lock.Unlock()
	}()

	// Start a bunch of priority readers that keep getting the lock
	for i := 0; i < numPriorityReaders; i++ {
		go func(id int) {
			lock.RLockPriority()
			time.Sleep(10 * time.Millisecond)
			lock.RUnlock()
		}(i)

		// Small delay to ensure readers queue up
		time.Sleep(5 * time.Millisecond)
	}

	// Writer should eventually get the lock, despite priority readers
	select {
	case <-writerGotLock:
		// This is expected - writer eventually got the lock
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Writer appears to be starved by priority readers")
	}
}

// TestHighContention creates an extremely contentious scenario to stress test the lock
func TestHighContention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high contention test in short mode")
	}

	lock := NewPriorityRWLock()

	// Use atomic counters to track operations
	var (
		writeOps             int32 = 0
		readOps              int32 = 0
		priorityReadOps      int32 = 0
		blockedWrites        int32 = 0
		blockedReads         int32 = 0
		blockedPriorityReads int32 = 0
	)

	// Configuration for high contention
	const (
		numWriters         = 15
		numReaders         = 30
		numPriorityReaders = 10
		testDuration       = 2 * time.Second
		maxHoldTime        = 5 * time.Millisecond
	)

	// Create a shared resource that goroutines will access
	sharedCounter := int32(0)

	// Use a context for clean shutdown
	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Create a separate context for actual test execution that we'll cancel early
	testCtx, testCancel := context.WithTimeout(context.Background(), testDuration-100*time.Millisecond)
	defer testCancel()

	// Create sync.WaitGroup to track all goroutines
	var wg sync.WaitGroup

	// Create a lock for synchronized logging
	var logMu sync.Mutex
	// Safe logging function that respects test state
	safeLogf := func(format string, args ...interface{}) {
		logMu.Lock()
		defer logMu.Unlock()
		// Only log if test context hasn't been canceled
		select {
		case <-testCtx.Done():
			// Don't log if test is shutting down
			return
		default:
			t.Logf(format, args...)
		}
	}

	// Start a goroutine to monitor and report progress
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				safeLogf("Stats - Writes: %d (blocked: %d), Reads: %d (blocked: %d), Priority Reads: %d (blocked: %d)",
					atomic.LoadInt32(&writeOps),
					atomic.LoadInt32(&blockedWrites),
					atomic.LoadInt32(&readOps),
					atomic.LoadInt32(&blockedReads),
					atomic.LoadInt32(&priorityReadOps),
					atomic.LoadInt32(&blockedPriorityReads))
			}
		}
	}()

	// Start writers that sometimes hold locks for longer periods
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create a dedicated RNG for this goroutine
			rng := rand.New(rand.NewSource(int64(id))) //nolint:gosec

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Continue with the test
				}

				// Random delay between operations
				time.Sleep(time.Duration(rng.Intn(1000)) * time.Microsecond)

				// Pre-calculate random values
				holdTime := time.Duration(rng.Intn(int(maxHoldTime)))
				if rng.Intn(10) == 0 {
					holdTime = maxHoldTime
				}

				// Try to acquire lock with a timeout
				lockAcquired := make(chan struct{})
				writerDone := make(chan struct{})

				// Create separate context for this operation
				opCtx, opCancel := context.WithCancel(ctx)

				// Start writer goroutine
				var writerWg sync.WaitGroup
				writerWg.Add(1)
				go func(sleepTime time.Duration) {
					defer writerWg.Done()
					defer close(writerDone)

					select {
					case <-opCtx.Done():
						return
					default:
						lock.Lock()
						close(lockAcquired)

						select {
						case <-opCtx.Done():
							lock.Unlock()
							return
						default:
							// Hold the lock for the predetermined time
							time.Sleep(sleepTime)

							// Update shared counter
							newVal := atomic.AddInt32(&sharedCounter, 1)

							// Verify no other writer has modified it concurrently
							time.Sleep(time.Microsecond) // Force a potential race
							if atomic.LoadInt32(&sharedCounter) != newVal {
								select {
								case <-testCtx.Done():
									// Don't log errors during test shutdown
								default:
									safeLogf("Writer %d: data race detected on shared counter", id)
								}
							}

							lock.Unlock()
						}
					}
				}(holdTime)

				// See if we block
				select {
				case <-lockAcquired:
					// Lock acquired without blocking (or very short block)
					atomic.AddInt32(&writeOps, 1)
				case <-time.After(time.Millisecond):
					// We're blocked, cancel the attempt if it takes too long
					select {
					case <-lockAcquired:
						atomic.AddInt32(&writeOps, 1)
						atomic.AddInt32(&blockedWrites, 1)
					case <-time.After(50 * time.Millisecond):
						// Lock acquisition is taking too long, might be stalled
						atomic.AddInt32(&blockedWrites, 1)
						select {
						case <-testCtx.Done():
							// Don't log during test shutdown
						default:
							safeLogf("Writer %d: lock acquisition timed out", id)
						}

						// Cancel and wait for the writer goroutine
						opCancel()

						// Continue with next iteration
						continue
					case <-ctx.Done():
						// Test is shutting down
						opCancel()
						return
					}
				case <-ctx.Done():
					// Test is shutting down
					opCancel()
					return
				}

				// Wait for writer to finish
				select {
				case <-writerDone:
					// Writer completed normally
				case <-ctx.Done():
					// Test is shutting down, cancel the operation
					opCancel()
					return
				}

				// Clean up
				opCancel()

				// Make sure writer goroutine has exited
				writerWg.Wait()
			}
		}(i)
	}

	// Start regular readers (similar pattern to writers but with RLock)
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create a dedicated RNG for this goroutine
			rng := rand.New(rand.NewSource(int64(100 + id))) //nolint:gosec

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Continue with the test
				}

				// Random delay between operations
				time.Sleep(time.Duration(rng.Intn(500)) * time.Microsecond)

				// Pre-calculate random values
				holdTime := time.Duration(rng.Intn(int(maxHoldTime / 2)))

				// Try to acquire read lock with a timeout
				lockAcquired := make(chan struct{})
				readerDone := make(chan struct{})

				// Create separate context for this operation
				opCtx, opCancel := context.WithCancel(ctx)

				var readerWg sync.WaitGroup
				readerWg.Add(1)
				go func(sleepTime time.Duration) {
					defer readerWg.Done()
					defer close(readerDone)

					select {
					case <-opCtx.Done():
						return
					default:
						lock.RLock()
						close(lockAcquired)

						select {
						case <-opCtx.Done():
							lock.RUnlock()
							return
						default:
							// Read the shared counter
							_ = atomic.LoadInt32(&sharedCounter)

							// Hold the lock for the predetermined time
							time.Sleep(sleepTime)

							lock.RUnlock()
						}
					}
				}(holdTime)

				// See if we block
				select {
				case <-lockAcquired:
					// Lock acquired without blocking
					atomic.AddInt32(&readOps, 1)
				case <-time.After(time.Millisecond):
					// We're blocked
					select {
					case <-lockAcquired:
						atomic.AddInt32(&readOps, 1)
						atomic.AddInt32(&blockedReads, 1)
					case <-time.After(100 * time.Millisecond):
						// This is too long for a regular reader
						atomic.AddInt32(&blockedReads, 1)
						select {
						case <-testCtx.Done():
							// Don't log during test shutdown
						default:
							safeLogf("Reader %d: lock acquisition timed out", id)
						}

						// Cancel and wait for the reader goroutine
						opCancel()

						continue
					case <-ctx.Done():
						// Test is shutting down
						opCancel()
						return
					}
				case <-ctx.Done():
					// Test is shutting down
					opCancel()
					return
				}

				// Wait for reader to finish
				select {
				case <-readerDone:
					// Reader completed normally
				case <-ctx.Done():
					// Test is shutting down, cancel the operation
					opCancel()
					return
				}

				// Clean up
				opCancel()

				// Make sure reader goroutine has exited
				readerWg.Wait()
			}
		}(i)
	}

	// Start priority readers (similar pattern to regular readers but with RLockPriority)
	for i := 0; i < numPriorityReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create a dedicated RNG for this goroutine
			rng := rand.New(rand.NewSource(int64(200 + id))) //nolint:gosec

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Continue with the test
				}

				// Random delay between operations
				time.Sleep(time.Duration(rng.Intn(800)) * time.Microsecond)

				// Pre-calculate random values
				holdTime := time.Duration(rng.Intn(int(maxHoldTime / 2)))

				// Try to acquire priority read lock with a timeout
				lockAcquired := make(chan struct{})
				readerDone := make(chan struct{})

				// Create separate context for this operation
				opCtx, opCancel := context.WithCancel(ctx)

				var readerWg sync.WaitGroup
				readerWg.Add(1)
				go func(sleepTime time.Duration) {
					defer readerWg.Done()
					defer close(readerDone)

					select {
					case <-opCtx.Done():
						return
					default:
						lock.RLockPriority()
						close(lockAcquired)

						select {
						case <-opCtx.Done():
							lock.RUnlock()
							return
						default:
							// Read the shared counter
							_ = atomic.LoadInt32(&sharedCounter)

							// Hold the lock for the predetermined time
							time.Sleep(sleepTime)

							lock.RUnlock()
						}
					}
				}(holdTime)

				// See if we block
				select {
				case <-lockAcquired:
					// Lock acquired without blocking
					atomic.AddInt32(&priorityReadOps, 1)
				case <-time.After(time.Millisecond):
					// We're blocked, but priority readers should unblock faster than normal readers
					select {
					case <-lockAcquired:
						atomic.AddInt32(&priorityReadOps, 1)
						atomic.AddInt32(&blockedPriorityReads, 1)
					case <-time.After(50 * time.Millisecond):
						// This is too long for a priority reader
						atomic.AddInt32(&blockedPriorityReads, 1)
						select {
						case <-testCtx.Done():
							// Don't log during test shutdown
						default:
							safeLogf("Priority reader %d: lock acquisition timed out", id)
						}

						// Cancel and wait for the reader goroutine
						opCancel()

						continue
					case <-ctx.Done():
						// Test is shutting down
						opCancel()
						return
					}
				case <-ctx.Done():
					// Test is shutting down
					opCancel()
					return
				}

				// Wait for reader to finish
				select {
				case <-readerDone:
					// Reader completed normally
				case <-ctx.Done():
					// Test is shutting down, cancel the operation
					opCancel()
					return
				}

				// Clean up
				opCancel()

				// Make sure reader goroutine has exited
				readerWg.Wait()
			}
		}(i)
	}

	// Wait for the test to run for the desired duration
	<-testCtx.Done()

	// Cancel test context slightly early to stop logging and prepare for cleanup
	testCancel()

	// Log final statistics - now it's safe because we've disabled logging
	// and we're still in the main test goroutine
	t.Logf("Final Stats - Writes: %d (blocked: %d), Reads: %d (blocked: %d), Priority Reads: %d (blocked: %d)",
		atomic.LoadInt32(&writeOps),
		atomic.LoadInt32(&blockedWrites),
		atomic.LoadInt32(&readOps),
		atomic.LoadInt32(&blockedReads),
		atomic.LoadInt32(&priorityReadOps),
		atomic.LoadInt32(&blockedPriorityReads))

	// Verify that priority readers were blocked less often than regular readers (as a percentage)
	if atomic.LoadInt32(&readOps) > 0 && atomic.LoadInt32(&priorityReadOps) > 0 {
		priorityReadBlockRate := float64(atomic.LoadInt32(&blockedPriorityReads)) / float64(atomic.LoadInt32(&priorityReadOps))
		regularReadBlockRate := float64(atomic.LoadInt32(&blockedReads)) / float64(atomic.LoadInt32(&readOps))

		t.Logf("Block rates - Regular readers: %.2f%%, Priority readers: %.2f%%",
			regularReadBlockRate*100, priorityReadBlockRate*100)

		// Skip statistical check if too few operations occurred
		if atomic.LoadInt32(&readOps) > 100 && atomic.LoadInt32(&priorityReadOps) > 100 {
			if priorityReadBlockRate >= regularReadBlockRate {
				t.Errorf("Expected priority readers to be blocked less often than regular readers")
			}
		}
	} else {
		t.Logf("Block rates - insufficient data to calculate rates")
	}

	// Cancel the context to signal all goroutines to exit
	cancel()

	// Set a timeout for all goroutines to finish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Give goroutines time to finish, but don't block test indefinitely
	select {
	case <-done:
		t.Log("All goroutines successfully terminated")
	case <-time.After(200 * time.Millisecond):
		t.Log("Some goroutines may still be running but we're shutting down anyway")
	}
}
