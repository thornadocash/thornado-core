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

func (s *SignerSuite) TestInternalTxOutIgnoresFutureDeferral(c *C) {
	item := TxOutStoreItem{
		TxOutItem:           types.TxOutItem{TxType: ttypes.TxOutTypeSweep},
		DeferredUntilHeight: 1_000,
	}
	c.Assert(txOutDeferredPast(item, 10), Equals, false)

	item.TxOutItem.TxType = ttypes.TxOutTypeMigrate
	c.Assert(txOutDeferredPast(item, 10), Equals, false)

	item.TxOutItem.TxType = ttypes.TxOutTypeOut
	c.Assert(txOutDeferredPast(item, 10), Equals, true)
}
