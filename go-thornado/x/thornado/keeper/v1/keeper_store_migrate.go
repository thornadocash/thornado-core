package keeperv1

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/runtime"

	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// GetStoreMigrateVotes returns all recorded votes for a migration key.
func (k KVStore) GetStoreMigrateVotes(ctx cosmos.Context, key string) types.StoreMigrateVotes {
	record := types.StoreMigrateVotes{Votes: map[string]string{}}
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	kvkey := k.GetKey(prefixStoreMigrateVote, key)
	if !store.Has(kvkey) {
		return record
	}
	bz := store.Get(kvkey)
	if err := json.Unmarshal(bz, &record); err != nil || record.Votes == nil {
		return types.StoreMigrateVotes{Votes: map[string]string{}}
	}
	return record
}

// SetStoreMigrateVote records (or overwrites) one node's vote for a migration.
func (k KVStore) SetStoreMigrateVote(ctx cosmos.Context, key, value string, acc cosmos.AccAddress) {
	record := k.GetStoreMigrateVotes(ctx, key)
	record.Votes[acc.String()] = value
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	bz, err := json.Marshal(record)
	if err != nil {
		return
	}
	store.Set(k.GetKey(prefixStoreMigrateVote, key), bz)
}

// DeleteStoreMigrateVotes clears all votes for a migration key (after apply).
func (k KVStore) DeleteStoreMigrateVotes(ctx cosmos.Context, key string) {
	k.del(ctx, k.GetKey(prefixStoreMigrateVote, key))
}

// GetStoreMigrateApplied returns the value at which a migration key was last
// applied, and whether any application has occurred.
func (k KVStore) GetStoreMigrateApplied(ctx cosmos.Context, key string) (string, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	kvkey := k.GetKey(prefixStoreMigrateApplied, key)
	if !store.Has(kvkey) {
		return "", false
	}
	return string(store.Get(kvkey)), true
}

// SetStoreMigrateApplied records that a migration key was applied at value.
func (k KVStore) SetStoreMigrateApplied(ctx cosmos.Context, key, value string) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(k.GetKey(prefixStoreMigrateApplied, key), []byte(value))
}
