package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) setFrostVoter(ctx cosmos.Context, key []byte, record FrostVoter) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getFrostVoter(ctx cosmos.Context, key []byte, record *FrostVoter) (bool, error) {
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

func (k KVStore) setFrostKeygenMetric(ctx cosmos.Context, key []byte, record FrostKeygenMetric) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getFrostKeygenMetric(ctx cosmos.Context, key []byte, record *FrostKeygenMetric) (bool, error) {
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

func (k KVStore) setFrostKeysignMetric(ctx cosmos.Context, key []byte, record FrostKeysignMetric) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getFrostKeysignMetric(ctx cosmos.Context, key []byte, record *FrostKeysignMetric) (bool, error) {
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

// SetFrostVoter - save a frost voter object
func (k KVStore) SetFrostVoter(ctx cosmos.Context, frost FrostVoter) {
	k.setFrostVoter(ctx, k.GetKey(prefixFrost, frost.String()), frost)
}

// GetFrostVoterIterator iterate tx in voters
func (k KVStore) GetFrostVoterIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixFrost)
}

// GetFrostVoter - gets information of a tx hash
func (k KVStore) GetFrostVoter(ctx cosmos.Context, id string) (FrostVoter, error) {
	record := FrostVoter{ID: id}
	_, err := k.getFrostVoter(ctx, k.GetKey(prefixFrost, id), &record)
	return record, err
}

// GetFrostKeygenMetric get frost keygen metric from key value store
func (k KVStore) GetFrostKeygenMetric(ctx cosmos.Context, pubkey common.PubKey) (*FrostKeygenMetric, error) {
	record := FrostKeygenMetric{PubKey: pubkey}
	_, err := k.getFrostKeygenMetric(ctx, k.GetKey(prefixFrostKeygenMetric, pubkey.String()), &record)
	return &record, err
}

// SetFrostKeygenMetric save FrostKeygenMetric to key value store
func (k KVStore) SetFrostKeygenMetric(ctx cosmos.Context, metric *FrostKeygenMetric) {
	k.setFrostKeygenMetric(ctx, k.GetKey(prefixFrostKeygenMetric, metric.PubKey.String()), *metric)
}

// GetFrostKeysignMetric get frost keygen metric from key value store
func (k KVStore) GetFrostKeysignMetric(ctx cosmos.Context, txID common.TxID) (*FrostKeysignMetric, error) {
	record := FrostKeysignMetric{
		TxID: txID,
	}
	_, err := k.getFrostKeysignMetric(ctx, k.GetKey(prefixFrostKeysignMetric, txID.String()), &record)
	return &record, err
}

// SetFrostKeysignMetric save FrostKeygenMetric to key value store
func (k KVStore) SetFrostKeysignMetric(ctx cosmos.Context, metric *FrostKeysignMetric) {
	// save the frost keysign metric against tx id
	k.setFrostKeysignMetric(ctx, k.GetKey(prefixFrostKeysignMetric, metric.TxID.String()), *metric)
	// save the latest keysign metric , it override previous
	k.setFrostKeysignMetric(ctx, k.GetKey(prefixFrostKeysignMetricLatest, "keysign"), *metric)
}

// GetLatestFrostKeysignMetric return the latest frost keysign metric
func (k KVStore) GetLatestFrostKeysignMetric(ctx cosmos.Context) (*FrostKeysignMetric, error) {
	record := FrostKeysignMetric{}
	_, err := k.getFrostKeysignMetric(ctx, k.GetKey(prefixFrostKeysignMetricLatest, "keysign"), &record)
	return &record, err
}
