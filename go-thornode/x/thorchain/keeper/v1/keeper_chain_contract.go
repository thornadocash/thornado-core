package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setChainContract(ctx cosmos.Context, key []byte, record ChainContract) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getChainContract(ctx cosmos.Context, key []byte, record *ChainContract) (bool, error) {
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

// SetChainContract - save chain contract address
func (k KVStore) SetChainContract(ctx cosmos.Context, cc ChainContract) {
	k.setChainContract(ctx, k.GetKey(prefixChainContract, cc.Chain.String()), cc)
}

// GetChainContract - gets chain contract
func (k KVStore) GetChainContract(ctx cosmos.Context, chain common.Chain) (ChainContract, error) {
	var record ChainContract
	_, err := k.getChainContract(ctx, k.GetKey(prefixChainContract, chain.String()), &record)
	return record, err
}

// GetChainContractIterator - get an iterator for chain contract
func (k KVStore) GetChainContractIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixChainContract)
}

// GetChainContracts return a list of chain contracts , which match the requested chains
func (k KVStore) GetChainContracts(ctx cosmos.Context, chains common.Chains) []ChainContract {
	contracts := make([]ChainContract, 0, len(chains))
	for _, item := range chains {
		cc, err := k.GetChainContract(ctx, item)
		if err != nil {
			ctx.Logger().Error("fail to get chain contract", "err", err, "chain", item.String())
			continue
		}
		if cc.IsEmpty() {
			continue
		}
		contracts = append(contracts, cc)
	}
	return contracts
}
