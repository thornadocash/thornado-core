package types

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
)

func NewReferenceMemo(asset common.Asset, memo, reference string, height int64) ReferenceMemo {
	return ReferenceMemo{
		Asset:     asset,
		Memo:      memo,
		Reference: reference,
		Height:    height,
	}
}

func (ref ReferenceMemo) Key() string {
	return fmt.Sprintf("%s/%s", ref.Asset, ref.Reference)
}

func (ref ReferenceMemo) IsExpired(height, ttl int64) bool {
	return ref.Height+ttl < height
}

// AddUsage adds a transaction ID to the usage list if it's not already present
// Returns true if the txID was added, false if it was already present
func (ref *ReferenceMemo) AddUsage(txID common.TxID) bool {
	// Check if already exists
	if ref.HasBeenUsedBy(txID) {
		return false // Already exists
	}
	// Add new txID
	ref.UsedByTxs = append(ref.UsedByTxs, txID)
	return true
}

// GetUsageCount returns the number of unique transactions that have used this reference memo
func (ref ReferenceMemo) GetUsageCount() int64 {
	return int64(len(ref.UsedByTxs))
}

// HasBeenUsedBy checks if a specific transaction ID has used this reference memo
func (ref ReferenceMemo) HasBeenUsedBy(txID common.TxID) bool {
	for _, existingTxID := range ref.UsedByTxs {
		if existingTxID.Equals(txID) {
			return true
		}
	}
	return false
}
