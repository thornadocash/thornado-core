package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper/types"
)

func (k KVStore) setStreamingSwap(ctx cosmos.Context, key []byte, record StreamingSwap) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getStreamingSwap(ctx cosmos.Context, key []byte, record *StreamingSwap) (bool, error) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return false, nil
	}

	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, record); err != nil {
		return true, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return true, nil
}

// GetStreamingSwapIterator iterate streaming swaps
func (k KVStore) GetStreamingSwapIterator(ctx cosmos.Context) cosmos.Iterator {
	key := k.GetKey(prefixStreamingSwap, "")
	return k.getIterator(ctx, types.DbPrefix(key))
}

// GetStreamingSwap retrieve streaming swap from the data store
func (k KVStore) GetStreamingSwap(ctx cosmos.Context, hash common.TxID) (StreamingSwap, error) {
	record := NewStreamingSwap(hash, 0, 0, cosmos.ZeroUint(), cosmos.ZeroUint())
	_, err := k.getStreamingSwap(ctx, k.GetKey(prefixStreamingSwap, hash.String()), &record)
	return record, err
}

// StreamingSwapExists check whether the given hash is associated with a swap
func (k KVStore) StreamingSwapExists(ctx cosmos.Context, hash common.TxID) bool {
	return k.has(ctx, k.GetKey(prefixStreamingSwap, hash.String()))
}

// SetStreamingSwap save the streaming swap to kv store
func (k KVStore) SetStreamingSwap(ctx cosmos.Context, swp StreamingSwap) {
	// Defensive validation: reject streaming swaps with invalid intervals
	// to prevent hollowed-out records that cause division by zero panics
	if err := swp.Valid(); err != nil {
		ctx.Logger().Error("attempt to save invalid streaming swap", "txid", swp.TxID, "error", err)
		return
	}
	k.setStreamingSwap(ctx, k.GetKey(prefixStreamingSwap, swp.TxID.String()), swp)
}

// RemoveStreamingSwap remove the streaming swap from kv store
func (k KVStore) RemoveStreamingSwap(ctx cosmos.Context, hash common.TxID) {
	k.del(ctx, k.GetKey(prefixStreamingSwap, hash.String()))
}
