package thorclient

import (
	"context"
	"fmt"
	"time"
)

// GetBlockTimestamp returns the timestamp for a specific THORChain block height.
// This is used to derive deterministic timestamps for transaction construction
// across all nodes in the network.
func (b *thorchainBridge) GetBlockTimestamp(height int64) (time.Time, error) {
	ctx := b.GetContext()
	if ctx.Client == nil {
		return time.Time{}, fmt.Errorf("rpc client is nil")
	}

	// Query the block at the specified height
	result, err := ctx.Client.Block(context.Background(), &height)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to query block at height %d: %w", height, err)
	}

	if result == nil || result.Block == nil {
		return time.Time{}, fmt.Errorf("block result is nil for height %d", height)
	}

	return result.Block.Time, nil
}
