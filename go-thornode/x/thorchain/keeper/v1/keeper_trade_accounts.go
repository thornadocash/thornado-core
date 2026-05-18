package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper/types"
)

func (k KVStore) setTradeAccount(ctx cosmos.Context, key []byte, record TradeAccount) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTradeAccount(ctx cosmos.Context, key []byte, record *TradeAccount) (bool, error) {
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

func (k KVStore) GetTradeAccountIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixTradeAccount)
}

func (k KVStore) GetTradeAccountIteratorWithAddress(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Iterator {
	key := k.GetKey(prefixTradeAccount, addr.String())
	return k.getIterator(ctx, types.DbPrefix(key))
}

func (k KVStore) GetTradeAccount(ctx cosmos.Context, addr cosmos.AccAddress, asset common.Asset) (TradeAccount, error) {
	record := NewTradeAccount(addr, asset)
	_, err := k.getTradeAccount(ctx, k.GetKey(prefixTradeAccount, record.Key()), &record)
	return record, err
}

func (k KVStore) SetTradeAccount(ctx cosmos.Context, tr TradeAccount) {
	key := k.GetKey(prefixTradeAccount, tr.Key())
	k.setTradeAccount(ctx, key, tr)
}

func (k KVStore) RemoveTradeAccount(ctx cosmos.Context, tr TradeAccount) {
	k.del(ctx, k.GetKey(prefixTradeAccount, tr.Key()))
}

func (k KVStore) setTradeUnit(ctx cosmos.Context, key []byte, record TradeUnit) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTradeUnit(ctx cosmos.Context, key []byte, record *TradeUnit) (bool, error) {
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

func (k KVStore) GetTradeUnitIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixTradeUnit)
}

func (k KVStore) GetTradeUnitIteratorWithAddress(ctx cosmos.Context, addr cosmos.AccAddress) cosmos.Iterator {
	key := k.GetKey(prefixTradeUnit, addr.String())
	return k.getIterator(ctx, types.DbPrefix(key))
}

func (k KVStore) GetTradeUnit(ctx cosmos.Context, asset common.Asset) (TradeUnit, error) {
	record := NewTradeUnit(asset)
	_, err := k.getTradeUnit(ctx, k.GetKey(prefixTradeUnit, record.Key()), &record)
	return record, err
}

func (k KVStore) SetTradeUnit(ctx cosmos.Context, tu TradeUnit) {
	k.setTradeUnit(ctx, k.GetKey(prefixTradeUnit, tu.Key()), tu)
}
