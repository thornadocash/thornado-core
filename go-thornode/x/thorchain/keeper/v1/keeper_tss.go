package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setTssVoter(ctx cosmos.Context, key []byte, record TssVoter) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTssVoter(ctx cosmos.Context, key []byte, record *TssVoter) (bool, error) {
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

func (k KVStore) setTssKeygenMetric(ctx cosmos.Context, key []byte, record TssKeygenMetric) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTssKeygenMetric(ctx cosmos.Context, key []byte, record *TssKeygenMetric) (bool, error) {
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

func (k KVStore) setTssKeysignMetric(ctx cosmos.Context, key []byte, record TssKeysignMetric) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTssKeysignMetric(ctx cosmos.Context, key []byte, record *TssKeysignMetric) (bool, error) {
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

// SetTssVoter - save a tss voter object
func (k KVStore) SetTssVoter(ctx cosmos.Context, tss TssVoter) {
	k.setTssVoter(ctx, k.GetKey(prefixTss, tss.String()), tss)
}

// GetTssVoterIterator iterate tx in voters
func (k KVStore) GetTssVoterIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixTss)
}

// GetTssVoter - gets information of a tx hash
func (k KVStore) GetTssVoter(ctx cosmos.Context, id string) (TssVoter, error) {
	record := TssVoter{ID: id}
	_, err := k.getTssVoter(ctx, k.GetKey(prefixTss, id), &record)
	return record, err
}

// GetTssKeygenMetric get tss keygen metric from key value store
func (k KVStore) GetTssKeygenMetric(ctx cosmos.Context, pubkey common.PubKey) (*TssKeygenMetric, error) {
	record := TssKeygenMetric{PubKey: pubkey}
	_, err := k.getTssKeygenMetric(ctx, k.GetKey(prefixTssKeygenMetric, pubkey.String()), &record)
	return &record, err
}

// SetTssKeygenMetric save TssKeygenMetric to key value store
func (k KVStore) SetTssKeygenMetric(ctx cosmos.Context, metric *TssKeygenMetric) {
	k.setTssKeygenMetric(ctx, k.GetKey(prefixTssKeygenMetric, metric.PubKey.String()), *metric)
}

// GetTssKeysignMetric get tss keygen metric from key value store
func (k KVStore) GetTssKeysignMetric(ctx cosmos.Context, txID common.TxID) (*TssKeysignMetric, error) {
	record := TssKeysignMetric{
		TxID: txID,
	}
	_, err := k.getTssKeysignMetric(ctx, k.GetKey(prefixTssKeysignMetric, txID.String()), &record)
	return &record, err
}

// SetTssKeysignMetric save TssKeygenMetric to key value store
func (k KVStore) SetTssKeysignMetric(ctx cosmos.Context, metric *TssKeysignMetric) {
	// save the tss keysign metric against tx id
	k.setTssKeysignMetric(ctx, k.GetKey(prefixTssKeysignMetric, metric.TxID.String()), *metric)
	// save the latest keysign metric , it override previous
	k.setTssKeysignMetric(ctx, k.GetKey(prefixTssKeysignMetricLatest, "keysign"), *metric)
}

// GetLatestTssKeysignMetric return the latest tss keysign metric
func (k KVStore) GetLatestTssKeysignMetric(ctx cosmos.Context) (*TssKeysignMetric, error) {
	record := TssKeysignMetric{}
	_, err := k.getTssKeysignMetric(ctx, k.GetKey(prefixTssKeysignMetricLatest, "keysign"), &record)
	return &record, err
}
