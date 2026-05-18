package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setBanVoter(ctx cosmos.Context, key []byte, record BanVoter) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getBanVoter(ctx cosmos.Context, key []byte, record *BanVoter) (bool, error) {
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

// SetBanVoter - save a ban voter object
func (k KVStore) SetBanVoter(ctx cosmos.Context, ban BanVoter) {
	k.setBanVoter(ctx, k.GetKey(prefixBanVoter, ban.String()), ban)
}

// GetBanVoter - gets information of ban voter
func (k KVStore) GetBanVoter(ctx cosmos.Context, addr cosmos.AccAddress) (BanVoter, error) {
	record := NewBanVoter(addr)
	_, err := k.getBanVoter(ctx, k.GetKey(prefixBanVoter, record.String()), &record)
	return record, err
}

// GetBanVoterIterator - get an iterator for ban voter
func (k KVStore) GetBanVoterIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixBanVoter)
}
