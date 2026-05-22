package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) setPrice(ctx cosmos.Context, key []byte, record OraclePrice) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getPrice(ctx cosmos.Context, key []byte, record *OraclePrice) (bool, error) {
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

// GetPrice get the oracle price the asset from kv store, returns error, if it doesn't exists
func (k KVStore) GetPrice(ctx cosmos.Context, symbol string) (OraclePrice, error) {
	var record OraclePrice
	found, err := k.getPrice(ctx, k.GetKey(prefixOraclePrice, symbol), &record)
	if !found {
		return OraclePrice{}, fmt.Errorf("Price not found: %s", symbol)
	}
	return record, err
}

// SetPrice save the oracle price to kv store
func (k KVStore) SetPrice(ctx cosmos.Context, oraclePrice OraclePrice) error {
	if err := oraclePrice.Valid(); err != nil {
		return err
	}
	key := k.GetKey(prefixOraclePrice, oraclePrice.Symbol)
	k.setPrice(ctx, key, oraclePrice)
	return nil
}

// DelPrice deletes the oracle price for a given symbol
func (k KVStore) DelPrice(ctx cosmos.Context, symbol string) {
	k.del(ctx, k.GetKey(prefixOraclePrice, symbol))
}

// GetPriceIterator
func (k KVStore) GetPriceIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixOraclePrice)
}
