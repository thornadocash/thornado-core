package keeperv1

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
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

// rawStorePrefixCheck validates that raw bytes decode as the exact type that
// consensus code reads under a store prefix. Every KVSET must pass this — a
// raw write that a reader cannot decode would panic every node at the same
// block, so writes under prefixes without a registered check are refused.
type rawStorePrefixCheck struct {
	prefix string
	check  func(k KVStore, value []byte) error
}

func protoCheck[T any, PT interface {
	*T
	codec.ProtoMarshaler
}]() func(k KVStore, value []byte) error {
	return func(k KVStore, value []byte) error {
		var record T
		return k.cdc.Unmarshal(value, PT(&record))
	}
}

func jsonCheck(_ KVStore, value []byte) error {
	if !json.Valid(value) {
		return fmt.Errorf("value is not valid JSON")
	}
	return nil
}

func rawStoreChecks() []rawStorePrefixCheck {
	return []rawStorePrefixCheck{
		{string(prefixConfig), protoCheck[types.ProtoInt64]()},
		{string(prefixNodeConfig), protoCheck[types.NodeConfigs]()},
		{string(prefixVault), protoCheck[types.Vault]()},
		{string(prefixTxOut), protoCheck[types.TxOut]()},
		{string(prefixObservedTxIn), protoCheck[types.ObservedTxVoter]()},
		{string(prefixObservedTxOut), protoCheck[types.ObservedTxVoter]()},
		{string(prefixNodeAccount), protoCheck[types.NodeAccount]()},
		{string(prefixNetworkFee), protoCheck[types.NetworkFee]()},
		{string(prefixLastChainHeight), protoCheck[types.ProtoInt64]()},
		{string(prefixLastSignedHeight), protoCheck[types.ProtoInt64]()},
		{string(prefixLastObserveHeight), protoCheck[types.ProtoInt64]()},
		{string(prefixStoreMigrateVote), jsonCheck},
	}
}

// matchRawStorePrefix returns the registered check for a key's prefix, or an
// error if the prefix is not on the raw-write allowlist. Only allowlisted
// prefixes may be touched raw — an unrecognised prefix is refused rather than
// risk a write/delete that a reader cannot survive.
func (k KVStore) matchRawStorePrefix(key []byte) (rawStorePrefixCheck, error) {
	if len(key) == 0 {
		return rawStorePrefixCheck{}, fmt.Errorf("empty store key")
	}
	for _, c := range rawStoreChecks() {
		if strings.HasPrefix(string(key), c.prefix) {
			return c, nil
		}
	}
	return rawStorePrefixCheck{}, fmt.Errorf("store prefix of %q is not on the raw-write allowlist; use a typed target or extend rawStoreChecks", string(key))
}

// ValidateRawStoreValue refuses any (key, value) whose bytes would not decode
// as the type read under that key's prefix, and any key under a prefix with
// no registered check. This closes the chain-panic vector of raw writes.
func (k KVStore) ValidateRawStoreValue(key, value []byte) error {
	c, err := k.matchRawStorePrefix(key)
	if err != nil {
		return err
	}
	if err := c.check(k, value); err != nil {
		return fmt.Errorf("value does not decode as the %q store type: %w", c.prefix, err)
	}
	return nil
}

// ValidateRawStoreKey refuses a delete of a key outside the allowlist.
func (k KVStore) ValidateRawStoreKey(key []byte) error {
	_, err := k.matchRawStorePrefix(key)
	return err
}

// SetRawStoreValue writes validated bytes at an arbitrary allowlisted store
// key — the KVSET store-migrate escape hatch.
func (k KVStore) SetRawStoreValue(ctx cosmos.Context, key, value []byte) error {
	if err := k.ValidateRawStoreValue(key, value); err != nil {
		return err
	}
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	store.Set(key, value)
	return nil
}

// GetRawStoreValue reads arbitrary bytes at an arbitrary store key.
func (k KVStore) GetRawStoreValue(ctx cosmos.Context, key []byte) ([]byte, bool) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	if !store.Has(key) {
		return nil, false
	}
	return store.Get(key), true
}

// DeleteRawStoreValue deletes an allowlisted store key (KVDEL). Callers must
// pre-validate with ValidateRawStoreKey; deletes outside the allowlist are
// no-ops here as a defensive backstop.
func (k KVStore) DeleteRawStoreValue(ctx cosmos.Context, key []byte) {
	if k.ValidateRawStoreKey(key) != nil {
		return
	}
	k.del(ctx, key)
}
