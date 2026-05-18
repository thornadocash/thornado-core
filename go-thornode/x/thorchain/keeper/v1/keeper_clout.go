package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setSwapperClout(ctx cosmos.Context, key []byte, record SwapperClout) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getSwapperClout(ctx cosmos.Context, key []byte, record *SwapperClout) (bool, error) {
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

func (k KVStore) SetSwapperClout(ctx cosmos.Context, record SwapperClout) error {
	k.setSwapperClout(ctx, k.GetKey(prefixSwapperClout, record.Address.String()), record)
	return nil
}

func (k KVStore) GetSwapperClout(ctx cosmos.Context, addr common.Address) (SwapperClout, error) {
	record := NewSwapperClout(addr)
	if addr.IsEmpty() {
		return record, nil
	}
	_, err := k.getSwapperClout(ctx, k.GetKey(prefixSwapperClout, addr.String()), &record)
	return record, err
}

func (k KVStore) GetSwapperCloutIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixSwapperClout)
}
