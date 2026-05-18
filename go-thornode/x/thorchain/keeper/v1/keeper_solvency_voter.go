package keeperv1

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/runtime"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (k KVStore) setSolvencyVoter(ctx cosmos.Context, key []byte, record SolvencyVoter) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getSolvencyVoter(ctx cosmos.Context, key []byte, record *SolvencyVoter) (bool, error) {
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

// SetSolvencyVoter - save a solvency voter object
func (k KVStore) SetSolvencyVoter(ctx cosmos.Context, solvencyVoter SolvencyVoter) {
	key := fmt.Sprintf("%s-%s", solvencyVoter.Chain, solvencyVoter.Id)
	k.setSolvencyVoter(ctx, k.GetKey(prefixSolvencyVoter, key), solvencyVoter)
}

// GetSolvencyVoter - gets information of solvency voter
func (k KVStore) GetSolvencyVoter(ctx cosmos.Context, txID common.TxID, chain common.Chain) (SolvencyVoter, error) {
	key := fmt.Sprintf("%s-%s", chain, txID)
	var solvencyVoter SolvencyVoter
	_, err := k.getSolvencyVoter(ctx, k.GetKey(prefixSolvencyVoter, key), &solvencyVoter)
	return solvencyVoter, err
}
