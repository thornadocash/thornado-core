package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) setFrostKeysignFailVoter(ctx cosmos.Context, key []byte, record FrostKeysignFailVoter) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getFrostKeysignFailVoter(ctx cosmos.Context, key []byte, record *FrostKeysignFailVoter) (bool, error) {
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

// SetFrostKeysignFailVoter - save a frost keysign fail voter object
func (k KVStore) SetFrostKeysignFailVoter(ctx cosmos.Context, frost FrostKeysignFailVoter) {
	k.setFrostKeysignFailVoter(ctx, k.GetKey(prefixFrostKeysignFailure, frost.String()), frost)
}

// GetFrostKeysignFailVoterIterator iterate tx in voters
func (k KVStore) GetFrostKeysignFailVoterIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixFrostKeysignFailure)
}

// GetFrostKeysignFailVoter - gets information of a frost keysign failure voter object
func (k KVStore) GetFrostKeysignFailVoter(ctx cosmos.Context, id string) (FrostKeysignFailVoter, error) {
	record := FrostKeysignFailVoter{ID: id}
	_, err := k.getFrostKeysignFailVoter(ctx, k.GetKey(prefixFrostKeysignFailure, id), &record)
	return record, err
}
