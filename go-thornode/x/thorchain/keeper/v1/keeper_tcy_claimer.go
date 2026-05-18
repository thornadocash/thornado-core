package keeperv1

import (
	"fmt"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper/types"
)

func (k KVStore) setTCYClaimer(ctx cosmos.Context, key []byte, record TCYClaimer) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTCYClaimer(ctx cosmos.Context, key []byte, record *TCYClaimer) (bool, error) {
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

// GetTCYClaimer - gets tcy claimer
func (k KVStore) GetTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset) (TCYClaimer, error) {
	record := NewTCYClaimer(l1Address, asset, math.ZeroUint())
	key := fmt.Sprintf("%s/%s", l1Address.String(), asset.String())
	ok, err := k.getTCYClaimer(ctx, k.GetKey(prefixTCYClaimer, key), &record)
	if !ok {
		return record, fmt.Errorf("TCYClaimer doesn't exist: %s", l1Address.String())
	}
	return record, err
}

// SetTCYClaimer - update the tcy claimer
func (k KVStore) SetTCYClaimer(ctx cosmos.Context, record TCYClaimer) error {
	key := fmt.Sprintf("%s/%s", record.L1Address.String(), record.Asset.String())
	k.setTCYClaimer(ctx, k.GetKey(prefixTCYClaimer, key), record)
	return nil
}

// GetTCYClaimerIteratorFromL1Address iterate TCY claimers
func (k KVStore) GetTCYClaimerIteratorFromL1Address(ctx cosmos.Context, l1Address common.Address) cosmos.Iterator {
	key := k.GetKey(prefixTCYClaimer, l1Address.String())
	return k.getIterator(ctx, types.DbPrefix(key))
}

// GetTCYClaimerIterator iterate TCY claimers
func (k KVStore) GetTCYClaimerIterator(ctx cosmos.Context) cosmos.Iterator {
	key := k.GetKey(prefixTCYClaimer, "")
	return k.getIterator(ctx, types.DbPrefix(key))
}

// DeleteTCYClaimer - deletes the tcy claimer
func (k KVStore) DeleteTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset) {
	key := fmt.Sprintf("%s/%s", l1Address.String(), asset.String())
	k.del(ctx, k.GetKey(prefixTCYClaimer, key))
}

// ListTCYClaimersFromL1Address gets claims from l1 address
func (k KVStore) ListTCYClaimersFromL1Address(ctx cosmos.Context, l1Address common.Address) ([]TCYClaimer, error) {
	var claimers []TCYClaimer
	iterator := k.GetTCYClaimerIteratorFromL1Address(ctx, l1Address)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var claimer TCYClaimer
		if err := k.cdc.Unmarshal(iterator.Value(), &claimer); err != nil {
			ctx.Logger().Error("fail to unmarshal tcy claimer", "error", err)
			continue
		}
		claimers = append(claimers, claimer)
	}

	if len(claimers) == 0 {
		return []TCYClaimer{}, fmt.Errorf("l1 address: (%s) doesn't have any tcy to claim", l1Address.String())
	}
	return claimers, nil
}

// TCYClaimerExists checks if claimer already exists
func (k KVStore) TCYClaimerExists(ctx cosmos.Context, l1Address common.Address, asset common.Asset) bool {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := k.GetKey(prefixTCYClaimer, fmt.Sprintf("%s/%s", l1Address.String(), asset.String()))
	return store.Has(key)
}

// UpdateTCYClaimer will update value if claimer exists, if not it will create a new one
func (k KVStore) UpdateTCYClaimer(ctx cosmos.Context, l1Address common.Address, asset common.Asset, amount math.Uint) error {
	if k.TCYClaimerExists(ctx, l1Address, asset) {
		claimer, err := k.GetTCYClaimer(ctx, l1Address, asset)
		if err != nil {
			return err
		}
		amount = amount.Add(claimer.Amount)
	}

	return k.SetTCYClaimer(ctx, NewTCYClaimer(l1Address, asset, amount))
}
