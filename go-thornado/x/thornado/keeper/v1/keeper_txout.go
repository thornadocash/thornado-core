package keeperv1

import (
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func (k KVStore) setTxOut(ctx cosmos.Context, key []byte, record TxOut) {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	buf := k.cdc.MustMarshal(&record)
	if buf == nil {
		store.Delete(key)
	} else {
		store.Set(key, buf)
	}
}

func (k KVStore) getTxOut(ctx cosmos.Context, key []byte, record *TxOut) (bool, error) {
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

// AppendTxOut - append the given item to a txOut block at or after the given height.
// Batchable outbounds are grouped per vault by the x/thornado batch flow before
// reaching the keeper; here every item gets a free block so it can never join a
// pending batch it does not belong to.
func (k KVStore) AppendTxOut(ctx cosmos.Context, height int64, item TxOutItem) error {
	item.TxType = item.GetTxType()
	var block *TxOut
	for offset := int64(0); offset < 1000; offset++ {
		candidate, err := k.GetTxOut(ctx, height+offset)
		if err != nil {
			return err
		}
		if candidate.IsEmpty() || candidate.Status == "" {
			block = candidate
			break
		}
	}
	if block == nil {
		return fmt.Errorf("fail to find empty txout slot from height %d", height)
	}
	block.Status = TxOutStatusPendingSign
	block.TxArray = append(block.TxArray, item)
	return k.SetTxOut(ctx, block)
}

// ClearTxOut - remove the txout of the given height from key value  store
func (k KVStore) ClearTxOut(ctx cosmos.Context, height int64) error {
	k.del(ctx, k.GetKey(prefixTxOut, strconv.FormatInt(height, 10)))
	return nil
}

// SetTxOut - write the given txout information to key value store
func (k KVStore) SetTxOut(ctx cosmos.Context, blockOut *TxOut) error {
	if blockOut == nil || blockOut.IsEmpty() {
		return nil
	}
	if blockOut.Status == "" {
		blockOut.Status = TxOutStatusPendingSign
	}
	if txOutComplete(*blockOut) {
		blockOut.Status = TxOutStatusComplete
	}
	k.setTxOut(ctx, k.GetKey(prefixTxOut, strconv.FormatInt(blockOut.Height, 10)), *blockOut)
	return nil
}

func txOutComplete(txOut TxOut) bool {
	if len(txOut.TxArray) == 0 {
		return false
	}
	for _, item := range txOut.TxArray {
		if item.OutHash.IsEmpty() {
			return false
		}
	}
	return true
}

// GetTxOutIterator iterate tx out
func (k KVStore) GetTxOutIterator(ctx cosmos.Context) cosmos.Iterator {
	return k.getIterator(ctx, prefixTxOut)
}

// GetTxOut - write the given txout information to key value store
func (k KVStore) GetTxOut(ctx cosmos.Context, height int64) (*TxOut, error) {
	record := NewTxOut(height)
	_, err := k.getTxOut(ctx, k.GetKey(prefixTxOut, strconv.FormatInt(height, 10)), record)
	return record, err
}

func (k KVStore) GetTxOutValue(ctx cosmos.Context, height int64) (cosmos.Uint, cosmos.Uint, error) {
	txout, err := k.GetTxOut(ctx, height)
	if err != nil {
		return cosmos.ZeroUint(), cosmos.ZeroUint(), err
	}
	assetValue, cloutValue := k.GetTOIsValue(ctx, txout.TxArray...)
	return assetValue, cloutValue, nil
}

func (k KVStore) GetTOIsValue(ctx cosmos.Context, tois ...TxOutItem) (cosmos.Uint, cosmos.Uint) {
	assetValue := cosmos.ZeroUint()
	for i := range tois {
		for _, coin := range append(common.Coins{tois[i].Coin}, tois[i].MaxGas...) {
			assetValue = assetValue.Add(coin.Amount)
		}
	}

	return assetValue, cosmos.ZeroUint()
}
