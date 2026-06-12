package types

import (
	"sort"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// NewFrostKeygenMetric create a new instance of FrostKeygenMetric
func NewFrostKeygenMetric(pubkey common.PubKey) *FrostKeygenMetric {
	return &FrostKeygenMetric{PubKey: pubkey}
}

// AddNodeFrostTime add node frost time
func (m *FrostKeygenMetric) AddNodeFrostTime(addr cosmos.AccAddress, keygenTime int64) {
	for i, item := range m.NodeFrostTimes {
		if item.Address.Equals(addr) {
			m.NodeFrostTimes[i].FrostTime = keygenTime
			return
		}
	}
	m.NodeFrostTimes = append(m.NodeFrostTimes, NodeFrostTime{Address: addr, FrostTime: keygenTime})
}

// GetMedianTime return the median time
func (m *FrostKeygenMetric) GetMedianTime() int64 {
	return getMedianTime(m.NodeFrostTimes)
}

// NewFrostKeysignMetric create a new instance of FrostKeysignMetric
func NewFrostKeysignMetric(txID common.TxID) *FrostKeysignMetric {
	return &FrostKeysignMetric{
		TxID: txID,
	}
}

// AddNodeFrostTime add node frost time
func (m *FrostKeysignMetric) AddNodeFrostTime(addr cosmos.AccAddress, keygenTime int64) {
	for i, item := range m.NodeFrostTimes {
		if item.Address.Equals(addr) {
			m.NodeFrostTimes[i].FrostTime = keygenTime
			return
		}
	}
	m.NodeFrostTimes = append(m.NodeFrostTimes, NodeFrostTime{Address: addr, FrostTime: keygenTime})
}

func getMedianTime(nodeFrostTimes []NodeFrostTime) int64 {
	if len(nodeFrostTimes) == 0 {
		return 0
	}
	sort.SliceStable(nodeFrostTimes, func(i, j int) bool {
		return nodeFrostTimes[i].FrostTime < nodeFrostTimes[j].FrostTime
	})

	totalLen := len(nodeFrostTimes)
	mid := len(nodeFrostTimes) / 2
	if totalLen%2 != 0 {
		return nodeFrostTimes[mid].FrostTime
	}
	return (nodeFrostTimes[mid-1].FrostTime + nodeFrostTimes[mid].FrostTime) / 2
}

// GetMedianTime return median time
func (m *FrostKeysignMetric) GetMedianTime() int64 {
	return getMedianTime(m.NodeFrostTimes)
}
