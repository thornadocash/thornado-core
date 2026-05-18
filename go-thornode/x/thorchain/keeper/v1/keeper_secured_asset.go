package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setSecuredAsset(ctx cosmos.Context, key []byte, record SecuredAsset) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getSecuredAsset(ctx cosmos.Context, key []byte, record *SecuredAsset) (bool, error) {
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

func (k KVStore) GetSecuredAssetIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixSecuredAsset)
}

func (k KVStore) GetSecuredAsset(ctx cosmos.Context, asset common.Asset) (SecuredAsset, error) {
	record := NewSecuredAsset(asset)
	_, err := k.getSecuredAsset(ctx, k.GetKey(prefixSecuredAsset, record.Key()), &record)
	return record, err
}

func (k KVStore) SetSecuredAsset(ctx cosmos.Context, ba SecuredAsset) {
	k.setSecuredAsset(ctx, k.GetKey(prefixSecuredAsset, ba.Key()), ba)
}
