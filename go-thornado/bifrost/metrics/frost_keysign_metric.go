package metrics

import (
	"sync"
)

// FrostKeysignMetricMgr is a struct to manage frost keysign metric in memory
type FrostKeysignMetricMgr struct {
	lock          *sync.Mutex
	keysignMetric map[string]int64
}

// NewFrostKeysignMetricMgr create a new instance of FrostKeysignMetricMgr
func NewFrostKeysignMetricMgr() *FrostKeysignMetricMgr {
	return &FrostKeysignMetricMgr{
		lock:          &sync.Mutex{},
		keysignMetric: make(map[string]int64),
	}
}

// SetFrostKeysignMetric save the frost keysign time metric against the given hash
func (m *FrostKeysignMetricMgr) SetFrostKeysignMetric(hash string, elapseInMs int64) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.keysignMetric[hash] = elapseInMs
}

// GetFrostKeysignMetric get the metric of the given hash , and delete it after
func (m *FrostKeysignMetricMgr) GetFrostKeysignMetric(hash string) int64 {
	m.lock.Lock()
	defer m.lock.Unlock()
	elapse, ok := m.keysignMetric[hash]
	if ok {
		delete(m.keysignMetric, hash)
	}
	return elapse
}
