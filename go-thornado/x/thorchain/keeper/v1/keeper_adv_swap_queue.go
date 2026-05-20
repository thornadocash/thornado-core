package keeperv1

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

// ratioLength ensures that the character length of the ratio store in the key
// of the index is always the same length. This is to ensure that the kvstore
// can iterate over the numbers numerically, even though it actually iterates
// over alphabetically (the two become the same). I suspect this number will
// never change as it does give a large granularity to attempt to swap. The
// amount of tokens emitted is, in the end, still respected by the swap limit.
// In the event that this number is changed, it has to be version'ed, and also
// a kvstore migration updating all ratios in the keys to be updated with the
// new length.
// A value of 18 means that granularity is maxed out at 1 trillion to 1 ratio.
const ratioLength int = 18

// formatSwapQueueItemKey formats a swap queue item key from a TxID and index
func formatSwapQueueItemKey(txID common.TxID, index int) string {
	return fmt.Sprintf("%s-%d", txID.String(), index)
}

// AdvSwapQueueEnabled return true if the adv swap queue feature is enabled
func (k KVStore) AdvSwapQueueEnabled(ctx cosmos.Context) bool {
	val := k.GetConfigInt64(ctx, constants.EnableAdvSwapQueue)
	return types.AdvSwapQueueMode(val) != types.AdvSwapQueueModeDisabled
}

// SetAdvSwapQueueItem - writes an adv swap queue item to the kv store
func (k KVStore) SetAdvSwapQueueItem(ctx cosmos.Context, msg MsgSwap) error {
	if msg.Tx.Coins == nil || len(msg.Tx.Coins) != 1 {
		return fmt.Errorf("incorrect number of coins in transaction (%d)", len(msg.Tx.Coins))
	}
	if msg.Tx.ID.IsEmpty() {
		return fmt.Errorf("invalid tx hash")
	}
	if err := k.SetAdvSwapQueueIndex(ctx, msg); err != nil {
		return err
	}
	k.setMsgSwap(ctx, k.GetKey(prefixAdvSwapQueueItem, formatSwapQueueItemKey(msg.Tx.ID, int(msg.Index))), msg)

	return nil
}

// GetAdvSwapQueueItemIterator iterate adv swap queue items
func (k KVStore) GetAdvSwapQueueItemIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixAdvSwapQueueItem)
}

// GetAdvSwapQueueItem - read the given adv swap queue item information from key values store
func (k KVStore) GetAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) (MsgSwap, error) {
	record := MsgSwap{}
	ok, err := k.getMsgSwap(ctx, k.GetKey(prefixAdvSwapQueueItem, formatSwapQueueItemKey(txID, index)), &record)
	if !ok {
		return record, errors.New("not found")
	}
	return record, err
}

// HasAdvSwapQueueItem - checks if adv swap queue item already exists
func (k KVStore) HasAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) bool {
	record := MsgSwap{}
	ok, _ := k.getMsgSwap(ctx, k.GetKey(prefixAdvSwapQueueItem, formatSwapQueueItemKey(txID, index)), &record)
	return ok
}

// RemoveAdvSwapQueueItem - removes a adv swap queue item from the kv store
func (k KVStore) RemoveAdvSwapQueueItem(ctx cosmos.Context, txID common.TxID, index int) error {
	msg, err := k.GetAdvSwapQueueItem(ctx, txID, index)
	if err != nil {
		_ = dbError(ctx, "failed to fetch adv swap queue item", err)
	} else {
		err = k.RemoveAdvSwapQueueIndex(ctx, msg)
	}
	k.del(ctx, k.GetKey(prefixAdvSwapQueueItem, formatSwapQueueItemKey(txID, index)))
	return err
}

///----------------------------------------------------------------------///

///-------------------------- Adv Swap Queue Index --------------------------///

// SetAdvSwapQueueIndex - writes a adv swap queue index to the kv store
func (k KVStore) SetAdvSwapQueueIndex(ctx cosmos.Context, msg MsgSwap) error {
	ok, err := k.HasAdvSwapQueueIndex(ctx, msg)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	key := k.getAdvSwapQueueIndexKey(ctx, msg)
	record := make([]string, 0)
	_, err = k.getStrings(ctx, key, &record)
	if err != nil {
		return err
	}
	record = append(record, formatSwapQueueItemKey(msg.Tx.ID, int(msg.Index)))
	k.setStrings(ctx, key, record)
	return nil
}

// GetAdvSwapQueueIterator iterate adv swap queue items
func (k KVStore) GetAdvSwapQueueIndexIterator(ctx cosmos.Context, swapType types.SwapType, source, target common.Asset) cosmos.Iterator {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	switch swapType {
	case types.SwapType_limit:
		// Normalize to layer1 assets for consistent key format
		sourceLayer1 := source.GetLayer1Asset()
		targetLayer1 := target.GetLayer1Asset()
		prefixStr := fmt.Sprintf("%s>%s/", sourceLayer1, targetLayer1)
		prefix := k.GetKey(prefixAdvSwapQueueLimitIndex, prefixStr)
		return cosmos.KVStoreReversePrefixIterator(store, prefix)
	case types.SwapType_market:
		return nil
	default:
		return nil
	}
}

// GetAdvSwapQueueIndex - read the given adv swap queue index information from key values tore
func (k KVStore) GetAdvSwapQueueIndex(ctx cosmos.Context, msg MsgSwap) ([]types.AdvSwapQueueIndexItem, error) {
	key := k.getAdvSwapQueueIndexKey(ctx, msg)
	record := make([]string, 0)
	_, err := k.getStrings(ctx, key, &record)
	if err != nil {
		return nil, err
	}
	result := make([]types.AdvSwapQueueIndexItem, 0, len(record))
	for _, rec := range record {
		// Parse format "txID-index" using last hyphen to handle Cosmos indexed TxIDs
		lastHyphenIndex := strings.LastIndex(rec, "-")
		if lastHyphenIndex == -1 {
			_ = dbError(ctx, fmt.Sprintf("invalid swap queue index format - no hyphen found: (%s)", rec), nil)
			continue
		}
		parts := []string{rec[:lastHyphenIndex], rec[lastHyphenIndex+1:]}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			_ = dbError(ctx, fmt.Sprintf("invalid swap queue index format: (%s)", rec), nil)
			continue
		}

		// Parse TxID
		hash, err := common.NewTxID(parts[0])
		if err != nil {
			_ = dbError(ctx, fmt.Sprintf("failed to parse tx hash: (%s)", parts[0]), err)
			continue
		}

		// Parse index
		index, err := strconv.Atoi(parts[1])
		if err != nil {
			_ = dbError(ctx, fmt.Sprintf("failed to parse index: (%s)", parts[1]), err)
			continue
		}

		result = append(result, types.AdvSwapQueueIndexItem{
			TxID:  hash,
			Index: index,
		})
	}
	return result, nil
}

// HasAdvSwapQueueIndex - checks if adv swap queue item already exists
func (k KVStore) HasAdvSwapQueueIndex(ctx cosmos.Context, msg MsgSwap) (bool, error) {
	key := k.getAdvSwapQueueIndexKey(ctx, msg)
	record := make([]string, 0)
	_, err := k.getStrings(ctx, key, &record)
	if err != nil {
		return false, err
	}
	idStr := formatSwapQueueItemKey(msg.Tx.ID, int(msg.Index))
	for _, r := range record {
		if strings.EqualFold(idStr, r) {
			return true, nil
		}
	}
	return false, nil
}

// RemoveAdvSwapQueueIndex - removes a adv swap queue item from the kv store
func (k KVStore) RemoveAdvSwapQueueIndex(ctx cosmos.Context, msg MsgSwap) error {
	if len(msg.Tx.Coins) == 0 {
		return fmt.Errorf("cannot remove swap queue index: msg has no coins")
	}
	key := k.getAdvSwapQueueIndexKey(ctx, msg)
	record := make([]string, 0)
	_, err := k.getStrings(ctx, key, &record)
	if err != nil {
		return err
	}

	found := false
	idStr := formatSwapQueueItemKey(msg.Tx.ID, int(msg.Index))
	for i, rec := range record {
		if strings.EqualFold(rec, idStr) {
			record = removeString(record, i)
			found = true
			break
		}
	}

	if len(record) == 0 {
		k.del(ctx, key)
		return nil
	}
	if found {
		k.setStrings(ctx, key, record)
	}
	return nil
}

func (k KVStore) getAdvSwapQueueIndexKey(ctx cosmos.Context, msg MsgSwap) []byte {
	switch msg.SwapType {
	case types.SwapType_limit:
		ra := rewriteRatio(ratioLength, getRatio(msg.Tx.Coins[0].Amount, msg.TradeTarget))
		f := msg.Tx.Coins[0].Asset
		t := msg.TargetAsset
		// Normalize to layer1 assets for consistent key format
		fLayer1 := f.GetLayer1Asset()
		tLayer1 := t.GetLayer1Asset()
		keyStr := fmt.Sprintf("%s>%s/%s/", fLayer1.String(), tLayer1.String(), ra)
		key := k.GetKey(prefixAdvSwapQueueLimitIndex, keyStr)
		return key
	case types.SwapType_market:
		return k.GetKey(prefixAdvSwapQueueMarketIndex, "")
	default:
		return []byte{}
	}
}

func getRatio(input, output cosmos.Uint) string {
	if output.IsZero() {
		return "0"
	}
	return input.MulUint64(1e8).Quo(output).String()
}

// rewriteRatio. In order to ensure these ratios are stored in alphabetical
// order (instead of numerological order), the length of the string always
// needs to be consistent (ie 18 chars). If the length is larger than this,
// then we start to lose precision by chopping the end of the string off.
func rewriteRatio(length int, str string) string {
	switch {
	case len(str) < length:
		var b strings.Builder
		for i := 1; i <= length-len(str); i += 1 {
			b.WriteString("0")
		}
		b.WriteString(str)
		return b.String()
	case len(str) > length:
		return str[:length]
	}
	return str
}

// removeString - remove a string from the slice. Does NOT maintain order, but
// is faster.
func removeString(a []string, i int) []string {
	if i > len(a)-1 || i < 0 {
		return a
	}
	a[i] = a[len(a)-1]  // Copy last element to index i.
	a[len(a)-1] = ""    // Erase last element (write zero value).
	return a[:len(a)-1] // Truncate slice.
}

///-------------------------- Limit Swap TTL Management --------------------------///

// SetLimitSwapTTL - stores a list of transaction hashes that expire at the given block height
func (k KVStore) SetLimitSwapTTL(ctx cosmos.Context, blockHeight int64, txHashes []common.TxID) error {
	if blockHeight <= 0 {
		return fmt.Errorf("invalid block height: %d", blockHeight)
	}

	key := k.GetKey(prefixAdvSwapQueueTTL, fmt.Sprintf("%d", blockHeight))
	txHashStrings := make([]string, len(txHashes))
	for i, hash := range txHashes {
		txHashStrings[i] = hash.String()
	}
	k.setStrings(ctx, key, txHashStrings)
	return nil
}

// GetLimitSwapTTL - retrieves the list of transaction hashes that expire at the given block height
func (k KVStore) GetLimitSwapTTL(ctx cosmos.Context, blockHeight int64) ([]common.TxID, error) {
	if blockHeight <= 0 {
		return nil, fmt.Errorf("invalid block height: %d", blockHeight)
	}

	key := k.GetKey(prefixAdvSwapQueueTTL, fmt.Sprintf("%d", blockHeight))
	var txHashStrings []string
	_, err := k.getStrings(ctx, key, &txHashStrings)
	if err != nil {
		return nil, err
	}

	txHashes := make([]common.TxID, 0, len(txHashStrings))
	for _, hashStr := range txHashStrings {
		hash, err := common.NewTxID(hashStr)
		if err != nil {
			_ = dbError(ctx, fmt.Sprintf("failed to parse tx hash from TTL: %s", hashStr), err)
			continue
		}
		txHashes = append(txHashes, hash)
	}

	return txHashes, nil
}

// RemoveLimitSwapTTL - removes the TTL entry for the given block height (tombstone)
func (k KVStore) RemoveLimitSwapTTL(ctx cosmos.Context, blockHeight int64) {
	if blockHeight <= 0 {
		return
	}
	key := k.GetKey(prefixAdvSwapQueueTTL, fmt.Sprintf("%d", blockHeight))
	k.del(ctx, key)
}

// AddToLimitSwapTTL - adds a transaction hash to the TTL list for the given block height
func (k KVStore) AddToLimitSwapTTL(ctx cosmos.Context, blockHeight int64, txHash common.TxID) error {
	if blockHeight <= 0 {
		return fmt.Errorf("invalid block height: %d", blockHeight)
	}

	// Get existing TTL list
	existingHashes, err := k.GetLimitSwapTTL(ctx, blockHeight)
	if err != nil {
		return fmt.Errorf("fail to get existing TTL list: %w", err)
	}

	// Check if hash already exists
	for _, existing := range existingHashes {
		if existing.Equals(txHash) {
			return nil // Already exists, no need to add
		}
	}

	// Add the new hash and save
	existingHashes = append(existingHashes, txHash)
	return k.SetLimitSwapTTL(ctx, blockHeight, existingHashes)
}
