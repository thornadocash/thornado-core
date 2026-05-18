package keeperv1

import (
	"fmt"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/common/tcysmartcontract"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

func (k KVStore) setTCYStaker(ctx cosmos.Context, key []byte, record TCYStaker) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTCYStaker(ctx cosmos.Context, key []byte, record *TCYStaker) (bool, error) {
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

// GetTCYStaker - gets tcy staker
func (k KVStore) GetTCYStaker(ctx cosmos.Context, address common.Address) (TCYStaker, error) {
	if tcysmartcontract.IsTCYSmartContractAddress(address) {
		return k.getTCYSmartContractAddressStaker(address, ctx)
	}
	record := NewTCYStaker(address, math.ZeroUint())
	ok, err := k.getTCYStaker(ctx, k.GetKey(prefixTCYStaker, address.String()), &record)
	if !ok {
		return record, fmt.Errorf("TCYStaker doesn't exist: %s", address.String())
	}
	return record, err
}

// SetTCYStaker - update the tcy staker
func (k KVStore) SetTCYStaker(ctx cosmos.Context, record TCYStaker) error {
	// We don't modify smart contract staker
	if tcysmartcontract.IsTCYSmartContractAddress(record.Address) {
		return nil
	}

	k.setTCYStaker(ctx, k.GetKey(prefixTCYStaker, record.Address.String()), record)
	return nil
}

// getTCYStakerIterator iterate TCY stakers
func (k KVStore) getTCYStakerIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixTCYStaker)
}

// DeleteTCYStaker - deletes the tcy staker
func (k KVStore) DeleteTCYStaker(ctx cosmos.Context, address common.Address) {
	// We don't modify smart contract staker
	if tcysmartcontract.IsTCYSmartContractAddress(address) {
		return
	}
	k.del(ctx, k.GetKey(prefixTCYStaker, address.String()))
}

// ListTCYStakers gets stakers
func (k KVStore) ListTCYStakers(ctx cosmos.Context) ([]TCYStaker, error) {
	var stakers []TCYStaker
	iterator := k.getTCYStakerIterator(ctx)
	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		var staker TCYStaker
		if err := k.cdc.Unmarshal(iterator.Value(), &staker); err != nil {
			ctx.Logger().Error("fail to unmarshal tcy staker", "error", err)
			continue
		}
		stakers = append(stakers, staker)
	}

	// Add TCY smart contract stakers
	tcySCStakers, err := k.getTCYSmartContractAddressStakers(ctx)
	if err == nil {
		stakers = append(stakers, tcySCStakers...)
	}

	return stakers, nil
}

func (k KVStore) TCYStakerExists(ctx cosmos.Context, address common.Address) bool {
	if tcysmartcontract.IsTCYSmartContractAddress(address) {
		return true
	}

	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	key := k.GetKey(prefixTCYStaker, address.String())
	return store.Has(key)
}

func (k KVStore) UpdateTCYStaker(ctx cosmos.Context, address common.Address, amount math.Uint) error {
	if k.TCYStakerExists(ctx, address) {
		staker, err := k.GetTCYStaker(ctx, address)
		if err != nil {
			return err
		}
		amount = amount.Add(staker.Amount)
	}

	return k.SetTCYStaker(ctx, types.NewTCYStaker(address, amount))
}

func (k KVStore) getTCYSmartContractAddressStaker(address common.Address, ctx cosmos.Context) (TCYStaker, error) {
	accAddress, err := address.AccAddress()
	if err != nil {
		return TCYStaker{}, err
	}
	coin := k.GetBalanceOf(ctx, accAddress, common.TCY)
	if coin.IsNil() {
		return NewTCYStaker(address, math.ZeroUint()), nil
	}

	return NewTCYStaker(address, math.NewUint(coin.Amount.Uint64())), nil
}

func (k KVStore) getTCYSmartContractAddressStakers(ctx cosmos.Context) ([]TCYStaker, error) {
	addresses, err := tcysmartcontract.GetTCYSmartContractAddresses()
	if err != nil {
		return nil, err
	}

	var stakers []TCYStaker
	for _, address := range addresses {
		accAddress, err := address.AccAddress()
		if err != nil {
			return nil, err
		}
		coin := k.GetBalanceOf(ctx, accAddress, common.TCY)
		balance := math.ZeroUint()
		if !coin.IsNil() {
			balance = math.NewUint(coin.Amount.Uint64())
		}
		staker := NewTCYStaker(address, balance)
		stakers = append(stakers, staker)
	}
	return stakers, nil
}
