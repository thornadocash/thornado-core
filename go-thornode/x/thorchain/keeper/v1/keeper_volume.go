package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper/types"
)

func (k KVStore) setVolumeBucket(ctx cosmos.Context, key []byte, record VolumeBucket) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getVolumeBucket(ctx cosmos.Context, key []byte, record *VolumeBucket) (bool, error) {
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

// GetVolumeBucket get the volume bucket for the given pool and timestamp from kv store, returns error, if it doesn't exists
func (k KVStore) GetVolumeBucket(ctx cosmos.Context, pool common.Asset, index int64) (VolumeBucket, error) {
	var record VolumeBucket
	pool = pool.GetLayer1Asset()
	key := fmt.Sprintf("%s/%d", pool.String(), index)
	found, err := k.getVolumeBucket(ctx, k.GetKey(prefixVolumeBucket, key), &record)
	if !found {
		return VolumeBucket{}, fmt.Errorf("Volume bucket not found: pool=%s, index=%d", pool.String(), index)
	}
	return record, err
}

// SetVolumeBucket save the bucket for the given pool to kv store
func (k KVStore) SetVolumeBucket(ctx cosmos.Context, bucket VolumeBucket) error {
	if err := bucket.Valid(); err != nil {
		return err
	}
	key := fmt.Sprintf("%s/%d", bucket.Asset.String(), bucket.Index)
	k.setVolumeBucket(ctx, k.GetKey(prefixVolumeBucket, key), bucket)
	return nil
}

func (k KVStore) GetVolumeBucketIterator(ctx cosmos.Context, pool common.Asset) cosmos.Iterator {
	key := prefixVolumeBucket + types.DbPrefix("/"+pool.String())
	return k.getIterator(ctx, key)
}

func (k KVStore) setVolume(ctx cosmos.Context, key []byte, record Volume) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getVolume(ctx cosmos.Context, key []byte, record *Volume) (bool, error) {
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

// GetVolume get the total (24h) volume for the given pool from kv store, returns error, if it doesn't exists
func (k KVStore) GetVolume(ctx cosmos.Context, pool common.Asset) (Volume, error) {
	var record Volume
	pool = pool.GetLayer1Asset()
	found, err := k.getVolume(ctx, k.GetKey(prefixVolume, pool.String()), &record)
	if !found {
		return Volume{}, fmt.Errorf("Total volume not found: pool=%s", pool.String())
	}
	return record, err
}

// SetVolume save the total (24h) volume for the given pool to kv store
func (k KVStore) SetVolume(ctx cosmos.Context, volume Volume) error {
	if err := volume.Valid(); err != nil {
		return err
	}
	k.setVolume(ctx, k.GetKey(prefixVolume, volume.Asset.String()), volume)
	return nil
}
