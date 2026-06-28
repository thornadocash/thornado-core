package observer

import (
	"encoding/binary"
	"testing"

	"github.com/thornadocash/go-thornado/common"
)

func TestBatchBeginTotalBatchesSkipsPrefix(t *testing.T) {
	data := append([]byte{}, prefixBatchBegin...)
	count := make([]byte, 4)
	binary.LittleEndian.PutUint32(count, 0)
	data = append(data, count...)

	total, err := batchBeginTotalBatches(data)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("total batches = %d, want 0", total)
	}

	binary.LittleEndian.PutUint32(data[len(prefixBatchBegin):], 2)
	total, err = batchBeginTotalBatches(data)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total batches = %d, want 2", total)
	}

	count = make([]byte, 4)
	binary.LittleEndian.PutUint32(count, 3)
	total, err = batchBeginTotalBatches(count)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total batches = %d, want 3", total)
	}
}

func TestQuorumStateEmptyIncludesAllAttestationTypes(t *testing.T) {
	if !quorumStateEmpty(common.QuorumState{}) {
		t.Fatal("empty quorum state reported non-empty")
	}

	state := common.QuorumState{
		QuoNetworkFees: []*common.QuorumNetworkFee{{NetworkFee: &common.NetworkFee{Chain: common.BTCChain}}},
	}
	if quorumStateEmpty(state) {
		t.Fatal("network-fee-only quorum state reported empty")
	}
}
