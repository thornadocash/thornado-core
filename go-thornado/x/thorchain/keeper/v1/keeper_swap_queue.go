package keeperv1

import (
	"errors"
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) setMsgSwap(ctx cosmos.Context, key []byte, record MsgSwap) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getMsgSwap(ctx cosmos.Context, key []byte, record *MsgSwap) (bool, error) {
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

// SetSwapQueueItem - writes a swap item to the kv store
func (k KVStore) SetSwapQueueItem(ctx cosmos.Context, msg MsgSwap, i int) error {
	k.setMsgSwap(ctx, k.GetKey(prefixSwapQueueItem, formatSwapQueueItemKey(msg.Tx.ID, i)), msg)
	return nil
}

// GetSwapQueueIterator iterate swap queue
func (k KVStore) GetSwapQueueIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixSwapQueueItem)
}

// GetSwapQueueItem - write the given swap queue item information to key values tore
func (k KVStore) GetSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int) (MsgSwap, error) {
	record := MsgSwap{}
	ok, err := k.getMsgSwap(ctx, k.GetKey(prefixSwapQueueItem, formatSwapQueueItemKey(txID, i)), &record)
	if !ok {
		return record, errors.New("not found")
	}
	return record, err
}

// HasSwapQueueItem - checks if swap item already exists
func (k KVStore) HasSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int) bool {
	record := MsgSwap{}
	ok, _ := k.getMsgSwap(ctx, k.GetKey(prefixSwapQueueItem, formatSwapQueueItemKey(txID, i)), &record)
	return ok
}

// RemoveSwapQueueItem - removes a swap item from the kv store
func (k KVStore) RemoveSwapQueueItem(ctx cosmos.Context, txID common.TxID, i int) {
	k.del(ctx, k.GetKey(prefixSwapQueueItem, formatSwapQueueItemKey(txID, i)))
}
