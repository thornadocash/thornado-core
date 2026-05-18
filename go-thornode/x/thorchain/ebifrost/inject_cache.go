package ebifrost

import (
	"time"

	"cosmossdk.io/log"
	sdk "github.com/cosmos/cosmos-sdk/types"
	common "gitlab.com/thorchain/thornode/v3/common"
)

type TimestampedItem[T any] struct {
	Item      T
	Timestamp time.Time
}

type InjectCache[T any] struct {
	items []TimestampedItem[T]
	mu    *PriorityRWLock

	// recentBlockItems is a map of block height to items that were included in that block.
	// This is used to keep track of recently processed items so we don't reprocess them.
	recentBlockItems map[int64][]T
}

// NewInjectCache creates a new inject cache for the given type
func NewInjectCache[T any]() *InjectCache[T] {
	return &InjectCache[T]{
		items:            make([]TimestampedItem[T], 0),
		recentBlockItems: make(map[int64][]T),
		mu:               NewPriorityRWLock(),
	}
}

// Add adds an item to the cache
func (c *InjectCache[T]) Add(item T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = append(c.items, TimestampedItem[T]{
		Item:      item,
		Timestamp: time.Now(),
	})
}

// Get returns all items in the cache (thread-safe)
func (c *InjectCache[T]) Get() []T {
	c.mu.RLockPriority()
	defer c.mu.RUnlock()

	result := make([]T, len(c.items))
	for i, item := range c.items {
		result[i] = item.Item
	}
	return result
}

// Lock locks the mutex
func (c *InjectCache[T]) Lock() {
	c.mu.Lock()
}

// Unlock unlocks the mutex
func (c *InjectCache[T]) Unlock() {
	c.mu.Unlock()
}

// RemoveAt removes the item at the given index
func (c *InjectCache[T]) RemoveAt(index int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.items) {
		return
	}

	c.items = append(c.items[:index], c.items[index+1:]...)
}

// AddToBlock adds items to the specified block height
func (c *InjectCache[T]) AddToBlock(height int64, item T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.recentBlockItems[height] = append(c.recentBlockItems[height], item)
}

// CleanOldBlocks removes blocks before the specified height
func (c *InjectCache[T]) CleanOldBlocks(currentHeight int64, keepBlocks int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// analyze-ignore(map-iteration)
	for h := range c.recentBlockItems {
		if h < currentHeight-keepBlocks {
			delete(c.recentBlockItems, h)
		}
	}
}

// CheckRecentBlocks checks if any item in the recent blocks matches the provided predicate
func (c *InjectCache[T]) CheckRecentBlocks(matches func(T) bool) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// analyze-ignore(map-iteration)
	for _, items := range c.recentBlockItems {
		for _, item := range items {
			if matches(item) {
				return true
			}
		}
	}

	return false
}

// markAttestationsConfirmedLocked is the internal implementation that assumes the lock is already held
func (c *InjectCache[T]) markAttestationsConfirmedLocked(
	item T,
	logger log.Logger,
	equals func(T, T) bool,
	getAttestations func(T) []*common.Attestation,
	removeAttestations func(T, []*common.Attestation) bool,
	logInfo func(T, log.Logger),
) bool {
	found := false
	for i := 0; i < len(c.items); i++ {
		cacheItem := c.items[i].Item
		if equals(cacheItem, item) {
			found = true
			logInfo(cacheItem, logger)
			if empty := removeAttestations(cacheItem, getAttestations(item)); empty {
				// Remove the element at index i
				c.items = append(c.items[:i], c.items[i+1:]...)
			} else {
				// Update the timestamp since we modified the item
				c.items[i].Timestamp = time.Now()
			}
			break
		}
	}

	return found
}

// MarkAttestationsConfirmedAndAddToBlock atomically marks attestations as confirmed and adds the item
// to recentBlockItems. This prevents race conditions where AddItem could check recentBlockItems
// between MarkAttestationsConfirmed and AddToBlock, potentially re-adding a just-confirmed item.
func (c *InjectCache[T]) MarkAttestationsConfirmedAndAddToBlock(
	item T,
	height int64,
	keepBlocks int64,
	logger log.Logger,
	equals func(T, T) bool,
	getAttestations func(T) []*common.Attestation,
	removeAttestations func(T, []*common.Attestation) bool,
	logInfo func(T, log.Logger),
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// First, mark attestations as confirmed (remove from c.items)
	found := c.markAttestationsConfirmedLocked(item, logger, equals, getAttestations, removeAttestations, logInfo)

	// Then, add to recentBlockItems (for deduplication in AddItem)
	c.recentBlockItems[height] = append(c.recentBlockItems[height], item)

	// Clean old blocks
	// analyze-ignore(map-iteration)
	for h := range c.recentBlockItems {
		if h < height-keepBlocks {
			delete(c.recentBlockItems, h)
		}
	}

	return found
}

// AddItem is a generic method that handles the common pattern for sending items to the cache.
// It filters out attestations that already exist in recent blocks and merges with existing items.
// This method is atomic - it holds the lock across both the check and the merge to prevent
// race conditions where the state could change between checking recentBlockItems and modifying c.items.
func (c *InjectCache[T]) AddItem(
	newItem T,
	getAttestations func(T) []*common.Attestation,
	setAttestations func(T, []*common.Attestation) T,
	itemsEqual func(T, T) bool,
) error {
	// Hold the lock for the entire operation to prevent race conditions.
	// This ensures atomicity between checking recentBlockItems and modifying c.items.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Filter out attestations that are already confirmed in recent blocks
	incomingAtts := getAttestations(newItem)
	newAttestations := make([]*common.Attestation, 0, len(incomingAtts))

	for _, a := range incomingAtts {
		// Check if this attestation exists in any recent block (inline check while holding lock)
		found := false
		// analyze-ignore(map-iteration)
		for _, blockItems := range c.recentBlockItems {
			for _, blockItem := range blockItems {
				if !itemsEqual(blockItem, newItem) {
					continue
				}
				for _, att := range getAttestations(blockItem) {
					if a.Equals(att) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			newAttestations = append(newAttestations, a)
		}
	}

	if len(newAttestations) == 0 {
		// No new attestations to add
		return nil
	}

	// Create a new item with only the new attestations
	itemToAdd := setAttestations(newItem, newAttestations)

	// Try to merge with an existing item or add as new (inline merge while holding lock)
	merged := false
	for i, existing := range c.items {
		if itemsEqual(existing.Item, itemToAdd) {
			// Merge attestations that don't already exist in the cached item
			existingAtts := getAttestations(c.items[i].Item)
			for _, newAtt := range getAttestations(itemToAdd) {
				attExists := false
				for _, existingAtt := range existingAtts {
					if newAtt.Equals(existingAtt) {
						attExists = true
						break
					}
				}
				if !attExists {
					existingAtts = append(existingAtts, newAtt)
				}
			}
			// Update the attestations on the cached item
			_ = setAttestations(c.items[i].Item, existingAtts)
			// Update the timestamp since we modified the item
			c.items[i].Timestamp = time.Now()
			merged = true
			break
		}
	}

	if !merged {
		// No existing item to merge with, add as new
		c.items = append(c.items, TimestampedItem[T]{
			Item:      itemToAdd,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// BroadcastEvent handles the common pattern of broadcasting events
func (c *InjectCache[T]) BroadcastEvent(
	item T,
	marshal func(T) ([]byte, error),
	broadcast func(string, []byte),
	eventType string,
	logger log.Logger,
) {
	itemBz, err := marshal(item)
	if err != nil {
		logger.Error("Failed to marshal item", "error", err)
		return
	}

	broadcast(eventType, itemBz)
}

// ProcessForProposal processes items for the proposal
func (c *InjectCache[T]) ProcessForProposal(
	createMsg func(T) (sdk.Msg, error),
	createTx func(sdk.Msg) ([]byte, error),
	logItem func(T, log.Logger),
	logger log.Logger,
) [][]byte {
	var injectTxs [][]byte

	items := c.Get()
	for _, item := range items {
		msg, err := createMsg(item)
		if err != nil {
			logger.Error("Failed to create message", "error", err)
			continue
		}

		txBz, err := createTx(msg)
		if err != nil {
			logger.Error("Failed to marshal tx", "error", err)
			continue
		}

		injectTxs = append(injectTxs, txBz)
		logItem(item, logger)
	}

	return injectTxs
}

// PruneExpiredItems removes items that have been in the cache longer than the TTL
func (c *InjectCache[T]) PruneExpiredItems(ttl time.Duration) []T {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var newItems []TimestampedItem[T]

	var prunedItems []T

	for _, item := range c.items {
		if now.Sub(item.Timestamp) < ttl {
			newItems = append(newItems, item)
		} else {
			prunedItems = append(prunedItems, item.Item)
		}
	}

	c.items = newItems

	return prunedItems
}
