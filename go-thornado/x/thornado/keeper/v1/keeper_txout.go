package keeperv1

import (
	"fmt"
	"strconv"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thornado/types"
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

// AppendTxOut - append the given item to txOut
func (k KVStore) AppendTxOut(ctx cosmos.Context, height int64, item TxOutItem) error {
	item.TxType = item.GetTxType()
	if types.IsBatchableTxOutType(item.TxType) {
		height, epoch := k.GetTxOutBatch(ctx, height)
		block, err := k.GetTxOut(ctx, height)
		if err != nil {
			return err
		}
		block.Epoch = epoch
		if block.Status == "" {
			block.Status = TxOutStatusPendingBatch
		}
		block.TxArray = append(block.TxArray, item)
		return k.SetTxOut(ctx, block)
	}

	block, err := k.GetTxOut(ctx, height)
	if err != nil {
		return err
	}
	if block.Status == "" {
		block.Status = TxOutStatusPendingSign
	}
	block.TxArray = append(block.TxArray, item)
	return k.SetTxOut(ctx, block)
}

func (k KVStore) GetTxOutBatch(ctx cosmos.Context, height int64) (int64, uint64) {
	windowBlocks := constants.MinutesToBlocks(
		k.GetConfigInt64(ctx, constants.Withdrawal_BatchWindowMinutes),
		k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds),
	)
	if windowBlocks <= 0 {
		return height, 0
	}
	origin := k.getTxOutBatchOrigin(ctx, windowBlocks)
	epoch := uint64((height - origin) / windowBlocks)
	closeHeight := origin + (int64(epoch)+1)*windowBlocks
	return closeHeight, epoch
}

func (k KVStore) getTxOutBatchOrigin(ctx cosmos.Context, windowBlocks int64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	origin := maxInt64
	iterator := k.GetTxOutIterator(ctx)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		var txOut TxOut
		if err := k.cdc.Unmarshal(iterator.Value(), &txOut); err != nil {
			continue
		}
		if txOut.Status == "" {
			continue
		}
		candidate := txOut.Height - (int64(txOut.Epoch)+1)*windowBlocks
		if candidate < origin {
			origin = candidate
		}
	}
	if origin == maxInt64 {
		return ctx.BlockHeight()
	}
	return origin
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
