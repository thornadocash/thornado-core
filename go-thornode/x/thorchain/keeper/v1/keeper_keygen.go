package keeperv1

import (
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/runtime"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setKeygenBlock(ctx cosmos.Context, key []byte, record KeygenBlock) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getKeygenBlock(ctx cosmos.Context, key []byte, record *KeygenBlock) (bool, error) {
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

// SetKeygenBlock save the KeygenBlock to kv store
func (k KVStore) SetKeygenBlock(ctx cosmos.Context, keygen KeygenBlock) {
	k.setKeygenBlock(ctx, k.GetKey(prefixKeygen, strconv.FormatInt(keygen.Height, 10)), keygen)
}

// GetKeygenBlockIterator return an iterator
func (k KVStore) GetKeygenBlockIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixKeygen)
}

// GetKeygenBlock from a given height
func (k KVStore) GetKeygenBlock(ctx cosmos.Context, height int64) (KeygenBlock, error) {
	record := NewKeygenBlock(height)
	_, err := k.getKeygenBlock(ctx, k.GetKey(prefixKeygen, strconv.FormatInt(height, 10)), &record)
	return record, err
}
