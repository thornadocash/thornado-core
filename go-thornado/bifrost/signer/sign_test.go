package signer

import (
	"testing"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type SignerSuite struct{}

var _ = Suite(&SignerSuite{})

func (s *SignerSuite) TestTxOutCompletionMatchIgnoresMutableSigningFields(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199982710))),
		InHash:           common.BlankTxID,
		GasRate:          13,
		TxType:           "migrate",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 99_997_855},
		},
	}
	completed := item
	completed.SourceInputs = append([]types.TxOutInput(nil), item.SourceInputs...)
	completed.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199992681)))
	completed.GasRate = 19

	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].AmountSats++
	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].Vout++
	c.Assert(txOutCompletionMatch(item, completed), Equals, true)
}

func (s *SignerSuite) TestTxOutCompletionMatchKeepsExternalTxoutsStrict(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))),
		InHash:           ttypes.GetRandomTxHash(),
		TxType:           "outbound",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 100_000},
		},
	}
	completed := item
	completed.SourceInputs = append([]types.TxOutInput(nil), item.SourceInputs...)

	c.Assert(txOutCompletionMatch(item, completed), Equals, true)

	completed.SourceInputs[0].AmountSats++
	c.Assert(txOutCompletionMatch(item, completed), Equals, false)
}

func (s *SignerSuite) TestCompletedTxOutItemUsesHeightScopedHistory(c *C) {
	sourceTx := ttypes.GetRandomTxHash()
	outHash := ttypes.GetRandomTxHash()
	item := types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(199982710))),
		InHash:           common.BlankTxID,
		GasRate:          13,
		TxType:           "migrate",
		SourceInputs: []types.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 99_997_855},
		},
	}
	completed := types.TxArrayItem{
		Chain:        item.Chain,
		ToAddress:    item.ToAddress,
		VaultPubKey:  item.VaultPubKey,
		Coin:         item.Coins[0],
		GasRate:      item.GasRate,
		InHash:       item.InHash,
		OutHash:      outHash,
		TxType:       item.TxType,
		SourceInputs: item.SourceInputs,
	}

	matched, matchedOutHash := completedTxOutItem(NewTxOutStoreItem(127, item, 0), []types.TxOut{
		{Height: 126, TxArray: []types.TxArrayItem{completed}},
	})
	c.Assert(matched, Equals, false)
	c.Assert(matchedOutHash.IsEmpty(), Equals, true)

	matched, matchedOutHash = completedTxOutItem(NewTxOutStoreItem(127, item, 0), []types.TxOut{
		{Height: 127, TxArray: []types.TxArrayItem{completed}},
	})
	c.Assert(matched, Equals, true)
	c.Assert(matchedOutHash.Equals(outHash), Equals, true)
}

func (s *SignerSuite) TestTxOutBatchTerminalStatus(c *C) {
	c.Assert(txOutBatchTerminalStatus(ttypes.TxOutStatusComplete), Equals, true)
	c.Assert(txOutBatchTerminalStatus(ttypes.TxOutStatusCancelled), Equals, true)
	c.Assert(txOutBatchTerminalStatus(ttypes.TxOutStatusPendingSign), Equals, false)
	c.Assert(txOutBatchTerminalStatus(ttypes.TxOutStatusPendingRetry), Equals, false)
}

func (s *SignerSuite) TestMergeStoredTxOutItemPreservesRetryState(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	item := NewTxOutStoreItem(3997, types.TxOutItem{TxType: ttypes.TxOutTypeRefund}, 0)
	c.Assert(storage.Set(TxOutStoreItem{
		TxOutItem:           item.TxOutItem,
		Height:              item.Height,
		Index:               item.Index,
		DeferredUntilHeight: 1_010_189,
		Round7Retry:         true,
		Checkpoint:          []byte("checkpoint"),
	}), IsNil)

	merged := mergeStoredTxOutItem(storage, item)
	c.Assert(merged.DeferredUntilHeight, Equals, int64(1_010_189))
	c.Assert(merged.Round7Retry, Equals, true)
	c.Assert(merged.Checkpoint, DeepEquals, []byte("checkpoint"))

}

func (s *SignerSuite) TestSignerStoreBatchPreservesRetryState(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	item := NewTxOutStoreItem(3997, types.TxOutItem{TxType: ttypes.TxOutTypeOut}, 0)
	key := item.Key()
	stored := item
	stored.DeferredUntilHeight = 1_010_189
	stored.Round7Retry = true
	stored.Checkpoint = []byte("checkpoint")
	c.Assert(storage.Set(stored), IsNil)

	incoming := NewTxOutStoreItem(3997, types.TxOutItem{TxType: ttypes.TxOutTypeOut}, 0)
	incoming.BatchStatus = ttypes.TxOutStatusPendingSign
	c.Assert(storage.Batch([]TxOutStoreItem{incoming}), IsNil)

	merged, err := storage.Get(key)
	c.Assert(err, IsNil)
	c.Assert(merged.DeferredUntilHeight, Equals, int64(1_010_189))
	c.Assert(merged.Round7Retry, Equals, true)
	c.Assert(merged.Checkpoint, DeepEquals, []byte("checkpoint"))
	c.Assert(merged.BatchStatus, Equals, ttypes.TxOutStatusPendingSign)
}

func (s *SignerSuite) TestSignerStoreBatchRemovesSupersededTxOutKey(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	inHash := ttypes.GetRandomTxHash()
	vault := ttypes.GetRandomPubKey()
	oldItem := NewTxOutStoreItem(3997, types.TxOutItem{
		Chain:       common.BTCChain,
		VaultPubKey: vault,
		InHash:      inHash,
		TxType:      ttypes.TxOutTypeOut,
		GasRate:     10,
	}, 0)
	oldKey := oldItem.Key()
	oldItem.DeferredUntilHeight = 4_100
	c.Assert(storage.Set(oldItem), IsNil)

	newItem := NewTxOutStoreItem(3997, types.TxOutItem{
		Chain:       common.BTCChain,
		VaultPubKey: vault,
		InHash:      inHash,
		TxType:      ttypes.TxOutTypeOut,
		GasRate:     14,
	}, 0)
	newKey := newItem.Key()
	c.Assert(newKey == oldKey, Equals, false)
	c.Assert(storage.Batch([]TxOutStoreItem{newItem}), IsNil)

	c.Assert(storage.Has(oldKey), Equals, false)
	c.Assert(storage.Has(newKey), Equals, true)
	listed := storage.List()
	c.Assert(listed, HasLen, 1)
	c.Assert(listed[0].TxOutItem.GasRate, Equals, int64(14))
	c.Assert(listed[0].DeferredUntilHeight, Equals, int64(0))
}

func (s *SignerSuite) TestRemoveTxOutBatchItemsUsesStoredKeys(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	vault := ttypes.GetRandomPubKey()
	itemA := NewTxOutStoreItem(3997, types.TxOutItem{
		Chain:          common.BTCChain,
		VaultPubKey:    vault,
		VaultPathIndex: common.MainVaultPathIndex,
		InHash:         ttypes.GetRandomTxHash(),
		TxType:         ttypes.TxOutTypeOut,
	}, 0)
	itemB := NewTxOutStoreItem(3997, types.TxOutItem{
		Chain:          common.BTCChain,
		VaultPubKey:    vault,
		VaultPathIndex: common.MainVaultPathIndex,
		InHash:         ttypes.GetRandomTxHash(),
		TxType:         ttypes.TxOutTypeRefund,
	}, 1)
	c.Assert(storage.Set(itemA), IsNil)
	c.Assert(storage.Set(itemB), IsNil)

	listed := storage.List()
	c.Assert(listed, HasLen, 2)
	signer := &Signer{storage: storage}
	signer.removeTxOutBatchItems(listed[0])

	c.Assert(storage.List(), HasLen, 0)
}

func (s *SignerSuite) TestTxOutHonorsFutureDeferral(c *C) {
	item := TxOutStoreItem{
		TxOutItem:           types.TxOutItem{TxType: ttypes.TxOutTypeSweep},
		DeferredUntilHeight: 1_000,
	}
	c.Assert(txOutDeferredPast(item, 10), Equals, true)

	item.TxOutItem.TxType = ttypes.TxOutTypeMigrate
	c.Assert(txOutDeferredPast(item, 10), Equals, true)

	item.TxOutItem.TxType = ttypes.TxOutTypeOut
	c.Assert(txOutDeferredPast(item, 10), Equals, true)
}

func (s *SignerSuite) TestDeferredRecoveredObservationTxInRequiresPreSignObservation(c *C) {
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain: common.BTCChain,
		},
		Observation: &types.TxInItem{
			Tx:          "recovered",
			BlockHeight: 100,
		},
	}

	txIn, ok := deferredRecoveredObservationTxIn(item)
	c.Assert(ok, Equals, true)
	c.Assert(txIn.Chain, Equals, common.BTCChain)
	c.Assert(txIn.MemPool, Equals, false)
	c.Assert(txIn.Filtered, Equals, true)
	c.Assert(txIn.ConfirmationRequired, Equals, int64(0))
	c.Assert(txIn.TxArray, HasLen, 1)
	c.Assert(txIn.TxArray[0].Tx, Equals, "recovered")

	item.Checkpoint = []byte("checkpoint")
	_, ok = deferredRecoveredObservationTxIn(item)
	c.Assert(ok, Equals, false)

	item.Checkpoint = nil
	item.SignedTx = []byte("signed")
	_, ok = deferredRecoveredObservationTxIn(item)
	c.Assert(ok, Equals, false)
}
