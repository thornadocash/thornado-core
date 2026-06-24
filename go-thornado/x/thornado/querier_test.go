package thornado

import (
	"fmt"
	"testing"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func TestSelectQueryObservedTxVoterPrefersFinalOutbound(t *testing.T) {
	txID := common.TxID("32D369E844293F90DBA733BE55B28A784B8646D8380A3938B0AA55B67A315254")
	tx := common.NewTx(
		txID,
		common.Address("bcrt1pfrom"),
		common.Address("bcrt1pto"),
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(1))},
	)
	pubKey := common.PubKey("tthorpub1addwnpepqdlv7gm58tu30x20ftxpzq9vsg63p0j0h3l8d5qcgz0qz9zenal0y2v72lz")

	inbound := types.NewObservedTxVoter(txID, common.ObservedTxs{
		common.NewObservedTx(tx, 0, pubKey, 10),
	})
	outbound := types.NewObservedTxVoter(txID, common.ObservedTxs{
		common.NewObservedTx(tx, 10, pubKey, 10),
	})
	outbound.FinalisedHeight = 12
	outbound.Tx = outbound.Txs[0]

	got := selectQueryObservedTxVoter(inbound, outbound)
	if got.FinalisedHeight != outbound.FinalisedHeight {
		t.Fatalf("expected finalized outbound voter, got finalised height %d", got.FinalisedHeight)
	}
}

func TestShielderSyncBirthdayAndCursor(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	_ = k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "AA",
		DenominationSats: 1_000_000,
		CreatedHeight:    10,
	})
	_ = k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "BB",
		DenominationSats: 1_000_000,
		CreatedHeight:    20,
	})
	_ = k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "CC",
		DenominationSats: 1_000_000,
		CreatedHeight:    30,
	})

	notes, cursor, _, more, err := queryShielderSyncNotes(ctx, k, "", 1, false, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Commitment != "BB" || cursor != "BB" || !more {
		t.Fatalf("unexpected first notes page: notes=%v cursor=%q more=%v", notes, cursor, more)
	}
	notes, cursor, _, more, err = queryShielderSyncNotes(ctx, k, cursor, 1, false, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Commitment != "CC" || cursor != "" || more {
		t.Fatalf("unexpected second notes page: notes=%v cursor=%q more=%v", notes, cursor, more)
	}

	_ = k.SetShielderRedeem(ctx, types.ShielderRedeem{WithdrawalID: "old", RequestedHeight: 10})
	_ = k.SetShielderRedeem(ctx, types.ShielderRedeem{WithdrawalID: "new", RequestedHeight: 30})
	_ = k.SetShielderNullifierSpent(ctx, "N1", "old")
	_ = k.SetShielderNullifierSpent(ctx, "N2", "new")

	nullifiers, _, _, more, err := queryShielderSyncNullifiers(ctx, k, "", 10, false, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(nullifiers) != 1 || nullifiers[0].NullifierHash != "N2" || more {
		t.Fatalf("unexpected nullifier page: nullifiers=%v more=%v", nullifiers, more)
	}
}

func BenchmarkShielderSync1000NotesAndNullifiers(b *testing.B) {
	ctx, k := setupShielderSyncBenchmark(b, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkShielderSync1000(b, ctx, k)
	}
}

func BenchmarkShielderSync1000NotesAndNullifiersWithTxOuts(b *testing.B) {
	ctx, k := setupShielderSyncBenchmark(b, true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkShielderSync1000(b, ctx, k)
	}
}

func setupShielderSyncBenchmark(b *testing.B, includeTxOuts bool) (cosmos.Context, *shielderFlowTestKeeper) {
	b.Helper()
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	for i := 0; i < 1000; i++ {
		commitment := fmt.Sprintf("commitment-%06d", i)
		if err := k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
			Commitment:       commitment,
			DenominationSats: 1_000_000,
			CreatedHeight:    100 + int64(i),
		}); err != nil {
			b.Fatal(err)
		}
		withdrawalID := fmt.Sprintf("withdrawal-%06d", i)
		if err := k.SetShielderRedeem(ctx, types.ShielderRedeem{
			WithdrawalID:    withdrawalID,
			NullifierHash:   fmt.Sprintf("nullifier-%06d", i),
			RequestedHeight: 100 + int64(i),
			AmountSats:      1_000_000,
			FeeSats:         10_000,
		}); err != nil {
			b.Fatal(err)
		}
		if err := k.SetShielderNullifierSpent(ctx, fmt.Sprintf("nullifier-%06d", i), withdrawalID); err != nil {
			b.Fatal(err)
		}
		if includeTxOuts {
			outHash := common.TxID(fmt.Sprintf("%064d", i+1))
			if err := k.SetTxOut(ctx, &TxOut{
				Height: int64(10_000 + i),
				Status: TxOutStatusComplete,
				TxArray: []TxOutItem{
					{
						Chain:   common.BTCChain,
						InHash:  common.TxID(withdrawalID),
						OutHash: outHash,
						OutVout: uint32(i % 4),
						TxType:  types.TxOutTypeOut,
						Coin:    common.NewCoin(common.BTCAsset, cosmos.NewUint(990_000)),
						GasRate: 1,
						MaxGas:  common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(10_000))},
					},
				},
			}); err != nil {
				b.Fatal(err)
			}
		}
	}
	return ctx, k
}

func benchmarkShielderSync1000(b *testing.B, ctx cosmos.Context, k *shielderFlowTestKeeper) {
	b.Helper()
	notes, nextNoteCursor, totalNotes, moreNotes, err := queryShielderSyncNotes(ctx, k, "", 1000, false, 1)
	if err != nil {
		b.Fatal(err)
	}
	nullifiers, nextNullifierCursor, totalNullifiers, moreNullifiers, err := queryShielderSyncNullifiers(ctx, k, "", 1000, false, 1)
	if err != nil {
		b.Fatal(err)
	}
	if len(notes) != 1000 || len(nullifiers) != 1000 || nextNoteCursor != "" || nextNullifierCursor != "" || moreNotes || moreNullifiers || totalNotes != 1000 || totalNullifiers != 1000 {
		b.Fatalf("unexpected sync shape: notes=%d nullifiers=%d note_cursor=%q nullifier_cursor=%q more=%v/%v totals=%d/%d",
			len(notes), len(nullifiers), nextNoteCursor, nextNullifierCursor, moreNotes, moreNullifiers, totalNotes, totalNullifiers)
	}
}
