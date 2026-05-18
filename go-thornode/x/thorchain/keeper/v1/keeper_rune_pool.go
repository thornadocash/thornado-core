package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

////////////////////////////////////////////////////////////////////////////////////////
// RUNEPool
////////////////////////////////////////////////////////////////////////////////////////

func (k KVStore) GetRUNEPool(ctx cosmos.Context) (RUNEPool, error) {
	record := NewRUNEPool()
	key := k.GetKey(prefixRUNEPool, "")

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return record, nil
	}

	bz := store.Get(key)
	if err := k.cdc.Unmarshal(bz, &record); err != nil {
		return record, dbError(ctx, fmt.Sprintf("Unmarshal kvstore: (%T) %s", record, key), err)
	}
	return record, nil
}

func (k KVStore) SetRUNEPool(ctx cosmos.Context, pool RUNEPool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := k.GetKey(prefixRUNEPool, "")
	buf := k.cdc.MustMarshal(&pool)
	store.Set(key, buf)
}

////////////////////////////////////////////////////////////////////////////////////////
// RUNEProviders
////////////////////////////////////////////////////////////////////////////////////////

func (k KVStore) setRUNEProvider(ctx cosmos.Context, key []byte, record RUNEProvider) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getRUNEProvider(ctx cosmos.Context, key []byte, record *RUNEProvider) (bool, error) {
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

// GetRUNEProviderIterator iterate RUNE providers
func (k KVStore) GetRUNEProviderIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixRUNEProvider)
}

// GetRUNEProvider retrieve RUNE provider from the data store
func (k KVStore) GetRUNEProvider(ctx cosmos.Context, addr cosmos.AccAddress) (RUNEProvider, error) {
	record := RUNEProvider{
		RuneAddress:    addr,
		DepositAmount:  cosmos.ZeroUint(),
		WithdrawAmount: cosmos.ZeroUint(),
		Units:          cosmos.ZeroUint(),
	}

	_, err := k.getRUNEProvider(ctx, k.GetKey(prefixRUNEProvider, record.Key()), &record)
	return record, err
}

// SetRUNEProvider save the RUNE provider to kv store
func (k KVStore) SetRUNEProvider(ctx cosmos.Context, rp RUNEProvider) {
	k.setRUNEProvider(ctx, k.GetKey(prefixRUNEProvider, rp.Key()), rp)
}

// RemoveRUNEProvider remove the RUNE provider from the kv store
func (k KVStore) RemoveRUNEProvider(ctx cosmos.Context, rp RUNEProvider) {
	k.del(ctx, k.GetKey(prefixRUNEProvider, rp.Key()))
}
