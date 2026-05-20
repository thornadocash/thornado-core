package ebifrost

import (
	"sync"
)

// PriorityRWLock provides a read-write lock where high-priority
// read requests can be served before waiting write requests
type PriorityRWLock struct {
	mu                  sync.Mutex
	readerCount         int32
	writerActive        int32
	writersWaiting      int32
	highPriorityReaders int32
	writerCond          *sync.Cond
	priorityReaderCond  *sync.Cond
	readerCond          *sync.Cond
}

// New creates a new PriorityRWLock
func NewPriorityRWLock() *PriorityRWLock {
	l := &PriorityRWLock{}
	l.writerCond = sync.NewCond(&l.mu)
	l.priorityReaderCond = sync.NewCond(&l.mu)
	l.readerCond = sync.NewCond(&l.mu)
	return l
}

// Lock acquires a write lock
func (l *PriorityRWLock) Lock() {
	l.mu.Lock()
	// If there are readers or another writer, we need to wait
	l.writersWaiting++
	for l.readerCount > 0 || l.writerActive == 1 {
		l.writerCond.Wait()
	}
	l.writersWaiting--
	l.writerActive = 1
	l.mu.Unlock()
}

// Unlock releases a write lock
func (l *PriorityRWLock) Unlock() {
	l.mu.Lock()
	l.writerActive = 0

	// Signal all waiting goroutines appropriately
	switch {
	case l.highPriorityReaders > 0:
		l.priorityReaderCond.Broadcast() // Signal all waiting priority readers
	case l.writersWaiting > 0:
		l.writerCond.Signal() // Signal one waiting writer
	default:
		l.readerCond.Broadcast() // Signal regular readers if no writers
	}
	l.mu.Unlock()
}

// RLock acquires a normal priority read lock
func (l *PriorityRWLock) RLock() {
	l.mu.Lock()

	// Regular readers wait for active writers and writers waiting
	for l.writerActive == 1 || l.writersWaiting > 0 {
		l.readerCond.Wait() // Wait on the reader condition
	}

	l.readerCount++
	l.mu.Unlock()
}

// RLockPriority acquires a high-priority read lock
// that jumps ahead of waiting writers
func (l *PriorityRWLock) RLockPriority() {
	l.mu.Lock()
	// High priority readers only wait for active writers, not waiting writers
	l.highPriorityReaders++
	for l.writerActive == 1 {
		l.priorityReaderCond.Wait()
	}

	l.readerCount++
	l.highPriorityReaders--
	l.mu.Unlock()
}

// RUnlock releases a read lock
func (l *PriorityRWLock) RUnlock() {
	l.mu.Lock()
	l.readerCount--

	// If this was the last reader, signal writers and readers
	if l.readerCount == 0 {
		// Signal priority readers first if any are waiting
		if l.highPriorityReaders > 0 {
			l.priorityReaderCond.Broadcast()
		}
		l.writerCond.Signal()    // Signal a waiting writer
		l.readerCond.Broadcast() // Also signal waiting readers
	}
	l.mu.Unlock()
}
