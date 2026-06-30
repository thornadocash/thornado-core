package signer

import (
	"sort"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestPackage(t *testing.T) { TestingT(t) }

type SignerSuite struct{}

var _ = Suite(&SignerSuite{})

type signerBridgeStub struct {
	thornadoclient.ThornadoBridge
	vault  ttypes.Vault
	nodes  []*ttypes.QueryNodeResponse
	height int64
}

func (b signerBridgeStub) GetVault(string) (ttypes.Vault, error) {
	return b.vault, nil
}

func (b signerBridgeStub) GetNodeAccounts() ([]*ttypes.QueryNodeResponse, error) {
	return b.nodes, nil
}

func (b signerBridgeStub) GetBlockHeight() (int64, error) {
	return b.height, nil
}

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

func (s *SignerSuite) TestTxOutItemPresentRequiresExactCurrentTxOut(c *C) {
	item := NewTxOutStoreItem(127, types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        ttypes.GetRandomBTCAddress(),
		VaultPubKey:      ttypes.GetRandomPubKey(),
		VaultPubKeyEddsa: ttypes.GetRandomEd25519PubKey(),
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))),
		InHash:           ttypes.GetRandomTxHash(),
		TxType:           ttypes.TxOutTypeRefund,
	}, 0)
	txArrayItem := types.TxArrayItem{
		Chain:       item.TxOutItem.Chain,
		ToAddress:   item.TxOutItem.ToAddress,
		VaultPubKey: item.TxOutItem.VaultPubKey,
		Coin:        item.TxOutItem.Coins[0],
		InHash:      item.TxOutItem.InHash,
		TxType:      item.TxOutItem.TxType,
	}

	c.Assert(txOutItemPresent(item, types.TxOut{Height: 126, TxArray: []types.TxArrayItem{txArrayItem}}), Equals, false)
	c.Assert(txOutItemPresent(item, types.TxOut{Height: 127, TxArray: []types.TxArrayItem{txArrayItem}}), Equals, true)

	txArrayItem.InHash = ttypes.GetRandomTxHash()
	c.Assert(txOutItemPresent(item, types.TxOut{Height: 127, TxArray: []types.TxArrayItem{txArrayItem}}), Equals, false)
}

func (s *SignerSuite) TestCurrentTxOutItemForSigningRefreshesMutableFields(c *C) {
	inHash := ttypes.GetRandomTxHash()
	sourceTx := ttypes.GetRandomTxHash()
	vault := ttypes.GetRandomPubKey()
	to := ttypes.GetRandomBTCAddress()
	eddsa := ttypes.GetRandomEd25519PubKey()
	item := NewTxOutStoreItem(127, types.TxOutItem{
		Chain:            common.BTCChain,
		ToAddress:        to,
		VaultPubKey:      vault,
		VaultPubKeyEddsa: eddsa,
		Coins:            common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))),
		InHash:           inHash,
		GasRate:          10,
		TxType:           ttypes.TxOutTypeOut,
	}, 1)
	txOut := types.TxOut{
		Height: 127,
		TxArray: []types.TxArrayItem{
			{Chain: common.BTCChain, InHash: ttypes.GetRandomTxHash(), VaultPubKey: vault, TxType: ttypes.TxOutTypeOut},
			{
				Chain:       common.BTCChain,
				ToAddress:   to,
				VaultPubKey: vault,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(99_000)),
				MaxGas:      common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1_000))},
				GasRate:     25,
				InHash:      inHash,
				TxType:      ttypes.TxOutTypeOut,
				SourceInputs: []types.TxOutInput{
					{TxID: sourceTx, Vout: 1, AmountSats: 100_000},
				},
			},
		},
	}

	current, ok := currentTxOutItemForSigning(item, txOut)
	c.Assert(ok, Equals, true)
	c.Assert(current.VaultPubKeyEddsa.Equals(eddsa), Equals, true)
	c.Assert(current.GasRate, Equals, int64(25))
	c.Assert(current.MaxGas, HasLen, 1)
	c.Assert(current.SourceInputs, HasLen, 1)
	c.Assert(current.SourceInputs[0].TxID.Equals(sourceTx), Equals, true)
	c.Assert(current.Coins[0].Amount.Equal(cosmos.NewUint(99_000)), Equals, true)
}

func (s *SignerSuite) TestCurrentTxOutItemForSigningRejectsAmbiguousFallback(c *C) {
	inHash := ttypes.GetRandomTxHash()
	vault := ttypes.GetRandomPubKey()
	to := ttypes.GetRandomBTCAddress()
	item := NewTxOutStoreItem(127, types.TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   to,
		VaultPubKey: vault,
		InHash:      inHash,
		TxType:      ttypes.TxOutTypeOut,
	}, 9)
	txArrayItem := types.TxArrayItem{
		Chain:       common.BTCChain,
		ToAddress:   to,
		VaultPubKey: vault,
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000)),
		InHash:      inHash,
		TxType:      ttypes.TxOutTypeOut,
	}

	_, ok := currentTxOutItemForSigning(item, types.TxOut{
		Height:  127,
		TxArray: []types.TxArrayItem{txArrayItem, txArrayItem},
	})
	c.Assert(ok, Equals, false)
}

func (s *SignerSuite) TestUnsignedLocalTxOutRequiresNoLocalSigningState(c *C) {
	item := TxOutStoreItem{}
	c.Assert(unsignedLocalTxOut(item), Equals, true)

	item.Checkpoint = []byte("checkpoint")
	c.Assert(unsignedLocalTxOut(item), Equals, false)

	item.Checkpoint = nil
	item.SignedTx = []byte("signed")
	c.Assert(unsignedLocalTxOut(item), Equals, false)

	item.SignedTx = nil
	item.Observation = &types.TxInItem{Tx: "observed"}
	c.Assert(unsignedLocalTxOut(item), Equals, false)
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
		SigningLeaderRetry:  2,
		Round7Retry:         true,
		Checkpoint:          []byte("checkpoint"),
	}), IsNil)

	merged := mergeStoredTxOutItem(storage, item)
	c.Assert(merged.DeferredUntilHeight, Equals, int64(1_010_189))
	c.Assert(merged.SigningLeaderRetry, Equals, uint64(2))
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
	stored.SigningLeaderRetry = 2
	stored.Round7Retry = true
	stored.Checkpoint = []byte("checkpoint")
	c.Assert(storage.Set(stored), IsNil)

	incoming := NewTxOutStoreItem(3997, types.TxOutItem{TxType: ttypes.TxOutTypeOut}, 0)
	incoming.BatchStatus = ttypes.TxOutStatusPendingSign
	c.Assert(storage.Batch([]TxOutStoreItem{incoming}), IsNil)

	merged, err := storage.Get(key)
	c.Assert(err, IsNil)
	c.Assert(merged.DeferredUntilHeight, Equals, int64(1_010_189))
	c.Assert(merged.SigningLeaderRetry, Equals, uint64(2))
	c.Assert(merged.Round7Retry, Equals, true)
	c.Assert(merged.Checkpoint, DeepEquals, []byte("checkpoint"))
	c.Assert(merged.BatchStatus, Equals, ttypes.TxOutStatusPendingSign)
}

func (s *SignerSuite) TestFrostPartyLeaderFallbackIgnoresLocalRetryAndHeight(c *C) {
	vaultPubKey := ttypes.GetRandomPubKey()
	members := []common.PubKey{
		ttypes.GetRandomPubKey(),
		ttypes.GetRandomPubKey(),
		ttypes.GetRandomPubKey(),
		ttypes.GetRandomPubKey(),
	}
	memberStrings := make([]string, 0, len(members))
	nodes := make([]*ttypes.QueryNodeResponse, 0, len(members))
	for _, member := range members {
		memberStrings = append(memberStrings, member.String())
		nodes = append(nodes, &ttypes.QueryNodeResponse{
			Status:           "active",
			PubKeySet:        common.NewPubKeySet(member),
			SignerMembership: []string{vaultPubKey.String()},
		})
	}
	sort.Strings(memberStrings)
	signer := &Signer{
		thornadoBridge: signerBridgeStub{
			vault: ttypes.Vault{
				PubKey:     vaultPubKey,
				Membership: memberStrings,
			},
			nodes: nodes,
		},
	}
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.BTCChain,
			VaultPubKey: vaultPubKey,
			InHash:      ttypes.GetRandomTxHash(),
			ToAddress:   ttypes.GetRandomBTCAddress(),
			Coins:       common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000))),
		},
		Height: 100,
		Epoch:  7,
	}

	first, err := signer.frostPartyLeader(item, 120, 10)
	c.Assert(err, IsNil)
	item.SigningLeaderRetry = 1
	second, err := signer.frostPartyLeader(item, 120, 10)
	c.Assert(err, IsNil)
	third, err := signer.frostPartyLeader(item, 130, 10)
	c.Assert(err, IsNil)

	c.Assert(first, Equals, second)
	c.Assert(first, Equals, third)
	c.Assert(memberStrings, DeepEquals, sortedStrings(memberStrings))
	c.Assert(memberStrings, HasLen, 4)
}

func (s *SignerSuite) TestFrostPartyLeaderUsesAssignedTxOutLeader(c *C) {
	assigned := ttypes.GetRandomPubKey()
	item := TxOutStoreItem{
		TxOutItem: types.TxOutItem{
			Chain:       common.BTCChain,
			VaultPubKey: ttypes.GetRandomPubKey(),
		},
		SigningLeader: assigned,
	}

	leader, err := (&Signer{}).frostPartyLeader(item, 120, 10)
	c.Assert(err, IsNil)
	c.Assert(leader, Equals, assigned.String())
}

func (s *SignerSuite) TestFrostLeaderRetryPersistsForInternalTxOut(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	item := NewTxOutStoreItem(3997, types.TxOutItem{
		Chain:       common.BTCChain,
		VaultPubKey: ttypes.GetRandomPubKey(),
		InHash:      ttypes.GetRandomTxHash(),
		TxType:      ttypes.TxOutTypeSweep,
	}, 0)
	signer := &Signer{
		thornadoBridge: signerBridgeStub{height: 4_100},
		storage:        storage,
	}

	c.Assert(signer.deferFrostKeysignRetryWithNextLeader(item), IsNil)

	stored, err := storage.Get(item.Key())
	c.Assert(err, IsNil)
	c.Assert(stored.SigningLeaderRetry, Equals, uint64(0))
	c.Assert(stored.DeferredUntilHeight, Equals, int64(4_101))
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

func (s *SignerSuite) TestTxOutUnsignedStoreItemsSkipsAlreadySignedItems(c *C) {
	storage, err := NewSignerStore("", config.LevelDBOptions{}, "")
	c.Assert(err, IsNil)
	defer storage.Close()

	unsignedInHash := ttypes.GetRandomTxHash()
	signedInHash := ttypes.GetRandomTxHash()
	txOut := types.TxOut{
		Height: 3997,
		Status: ttypes.TxOutStatusPendingSign,
		TxArray: []types.TxArrayItem{
			{
				Chain:       common.BTCChain,
				InHash:      signedInHash,
				OutHash:     ttypes.GetRandomTxHash(),
				TxType:      ttypes.TxOutTypeSweep,
				VaultPubKey: ttypes.GetRandomPubKey(),
			},
			{
				Chain:       common.BTCChain,
				InHash:      unsignedInHash,
				TxType:      ttypes.TxOutTypeSweep,
				VaultPubKey: ttypes.GetRandomPubKey(),
			},
		},
	}

	items := txOutUnsignedStoreItems(storage, txOut)
	c.Assert(items, HasLen, 1)
	c.Assert(items[0].Height, Equals, int64(3997))
	c.Assert(items[0].Index, Equals, int64(1))
	c.Assert(items[0].TxOutItem.InHash.Equals(unsignedInHash), Equals, true)
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
