package thornadoclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	openapi "github.com/thornadocash/go-thornado/openapi/gen"
)

// GetLastObservedInHeight returns the lastobservedin value for the chain past in
func (b *thornadoBridge) GetLastObservedInHeight(chain common.Chain) (int64, error) {
	lastblock, err := b.getLastBlock(chain)
	if err != nil {
		return 0, fmt.Errorf("failed to GetLastObservedInHeight: %w", err)
	}
	for _, item := range lastblock {
		if item.Chain == chain.String() {
			return item.LastObservedIn, nil
		}
	}
	return 0, fmt.Errorf("fail to GetLastObservedInHeight,chain(%s)", chain)
}

// GetLastSignedOutHeight returns the lastsignedout value for thornado
func (b *thornadoBridge) GetLastSignedOutHeight(chain common.Chain) (int64, error) {
	lastblock, err := b.getLastBlock(chain)
	if err != nil {
		return 0, fmt.Errorf("failed to GetLastSignedOutHeight: %w", err)
	}
	for _, item := range lastblock {
		if item.Chain == chain.String() {
			return item.LastSignedOut, nil
		}
	}
	return 0, fmt.Errorf("fail to GetLastSignedOutHeight,chain(%s)", chain)
}

// GetBlockHeight returns the current height for thornado blocks
func (b *thornadoBridge) GetBlockHeight() (int64, error) {
	var chain common.Chain
	latestBlocks, err := b.getLastBlock(chain)
	if err != nil {
		return 0, fmt.Errorf("failed to GetThornadoHeight: %w", err)
	}
	for _, item := range latestBlocks {
		return item.Thornado, nil
	}

	ctx := b.GetContext()
	status, err := ctx.Client.Status(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to GetThornadoHeight: %w", err)
	}
	return status.SyncInfo.LatestBlockHeight, nil
}

// getLastBlock calls the /lastblock/{chain} endpoint and Unmarshal's into the QueryResLastBlockHeights type
func (b *thornadoBridge) getLastBlock(chain common.Chain) ([]openapi.LastBlock, error) {
	path := LastBlockEndpoint
	buf, _, err := b.getWithPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get lastblock: %w", err)
	}
	var lastBlock []openapi.LastBlock
	if err = json.Unmarshal(buf, &lastBlock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal last block: %w", err)
	}
	if !chain.IsEmpty() {
		filtered := lastBlock[:0]
		for _, item := range lastBlock {
			if item.Chain == chain.String() {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}
	return lastBlock, nil
}
