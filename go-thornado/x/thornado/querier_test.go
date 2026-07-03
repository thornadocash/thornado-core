package thornado

import (
	"fmt"
	"testing"

	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

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

func TestShielderSyncStreamSelection(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	qs := queryServer{mgr: &Mgrs{K: k}}

	_ = k.SetDepositRecord(ctx, types.DepositRecord{
		DepositID:     common.TxID("0100000000000000000000000000000000000000000000000000000000000000"),
		AmountSats:    1_000_000,
		CreatedHeight: 10,
		Status:        "matched",
	})
	_ = k.SetShielderNoteRecord(ctx, types.StoredShielderNoteRecord{
		Commitment:       "AA",
		DenominationSats: 1_000_000,
		CreatedHeight:    10,
	})
	_ = k.SetShielderRedeem(ctx, types.ShielderRedeem{WithdrawalID: "withdraw-1", RequestedHeight: 10})
	_ = k.SetShielderNullifierSpent(ctx, "N1", "withdraw-1")

	notesOnly, err := qs.queryShielderSync(ctx, &types.QueryShielderSyncRequest{
		Limit:        10,
		IncludeNotes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(notesOnly.Notes) != 1 || len(notesOnly.Deposits) != 0 || len(notesOnly.Nullifiers) != 0 {
		t.Fatalf("notes-only sync included wrong streams: deposits=%d notes=%d nullifiers=%d", len(notesOnly.Deposits), len(notesOnly.Notes), len(notesOnly.Nullifiers))
	}

	nullifiersOnly, err := qs.queryShielderSync(ctx, &types.QueryShielderSyncRequest{
		Limit:             10,
		IncludeNullifiers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nullifiersOnly.Nullifiers) != 1 || len(nullifiersOnly.Deposits) != 0 || len(nullifiersOnly.Notes) != 0 {
		t.Fatalf("nullifier-only sync included wrong streams: deposits=%d notes=%d nullifiers=%d", len(nullifiersOnly.Deposits), len(nullifiersOnly.Notes), len(nullifiersOnly.Nullifiers))
	}

	full, err := qs.queryShielderSync(ctx, &types.QueryShielderSyncRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Deposits) != 1 || len(full.Notes) != 1 || len(full.Nullifiers) != 1 {
		t.Fatalf("full sync lost legacy streams: deposits=%d notes=%d nullifiers=%d", len(full.Deposits), len(full.Notes), len(full.Nullifiers))
	}
}

func TestQueryKeysignIncludesPendingRetry(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	height := ctx.BlockHeight()
	txOut := &TxOut{
		Height: height,
		Status: TxOutStatusPendingRetry,
		TxArray: []TxOutItem{{
			Chain:       common.BTCChain,
			ToAddress:   common.Address("bcrt1prefund"),
			VaultPubKey: vaultPubKey,
			Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(100_000)),
			InHash:      common.TxID("4E72D0DECDB0F5B39F0025047BD951D80C658079C04C18C02CFF5A2E31D974FD"),
			TxType:      types.TxOutTypeRefund,
		}},
	}
	if err := k.SetTxOut(ctx, txOut); err != nil {
		t.Fatal(err)
	}

	qs := queryServer{
		mgr: &Mgrs{K: k},
		kbs: cosmos.KeybaseStore{SignerPrivKey: sdksecp256k1.GenPrivKey()},
	}
	resp, err := qs.queryKeysign(ctx, fmt.Sprintf("%d", height), vaultPubKey.String())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Keysign.Status != TxOutStatusPendingRetry || len(resp.Keysign.TxArray) != 1 {
		t.Fatalf("pending retry keysign omitted tx array: %+v", resp.Keysign)
	}
}

func TestQueryProtocolGasSpentBreakdown(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	vaultPubKey := GetRandomPubKey()
	from := common.Address("bcrt1pfrom")
	to := common.Address("bcrt1pto")
	sweepHash := common.TxID("1000000000000000000000000000000000000000000000000000000000000001")
	migrateHash := common.TxID("1000000000000000000000000000000000000000000000000000000000000002")
	consolidateHash := common.TxID("1000000000000000000000000000000000000000000000000000000000000003")
	txOutHash := common.TxID("1000000000000000000000000000000000000000000000000000000000000004")

	k.SetObservedTxOutVoter(ctx, observedGasVoter(sweepHash, from, to, vaultPubKey, 900, 100))
	k.SetObservedTxOutVoter(ctx, observedGasVoter(migrateHash, from, to, vaultPubKey, 800, 200))
	k.SetObservedTxOutVoter(ctx, observedGasVoter(consolidateHash, from, to, vaultPubKey, 700, 300))
	k.SetObservedTxOutVoter(ctx, observedGasVoter(txOutHash, from, to, vaultPubKey, 1_000, 1_000))

	if err := k.SetTxOut(ctx, &TxOut{
		Height: 100,
		Status: TxOutStatusComplete,
		TxArray: []TxOutItem{
			gasTestInternalTxOut(types.TxOutTypeSweep, sweepHash, 1_000, 900),
			gasTestInternalTxOut(types.TxOutTypeMigrate, migrateHash, 1_000, 800),
			gasTestInternalTxOut(types.TxOutTypeConsolidate, consolidateHash, 1_000, 700),
			{
				Chain:       common.BTCChain,
				ToAddress:   to,
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(600)),
				OutHash:     txOutHash,
				TxType:      types.TxOutTypeOut,
			},
			{
				Chain:       common.BTCChain,
				ToAddress:   common.Address("bcrt1prefund"),
				VaultPubKey: vaultPubKey,
				Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(400)),
				OutHash:     txOutHash,
				TxType:      types.TxOutTypeRefund,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := queryProtocolGasSpent(ctx, k)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TotalSats != 1_600 {
		t.Fatalf("total gas = %d", resp.TotalSats)
	}
	got := map[string]*types.QueryGasBreakdown{}
	for _, row := range resp.Breakdown {
		got[row.TxType] = row
	}
	assertGasBreakdown(t, got, types.TxOutTypeSweep, 100, 1)
	assertGasBreakdown(t, got, types.TxOutTypeMigrate, 200, 1)
	assertGasBreakdown(t, got, types.TxOutTypeConsolidate, 300, 1)
	assertGasBreakdown(t, got, protocolGasTxOutBucket, 1_000, 1)
}

func TestCalculateNetworkSolvencyReportsLatestSolvencyAmount(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	vault := solvencyTestVault(1_000)
	k.baseVaults = Vaults{vault}
	k.SetSolvencyVoter(ctx, solvencyTestVoter(
		"1000000000000000000000000000000000000000000000000000000000000011",
		vault.PubKey,
		900,
		10,
		20,
	))
	k.SetSolvencyVoter(ctx, solvencyTestVoter(
		"1000000000000000000000000000000000000000000000000000000000000012",
		vault.PubKey,
		950,
		11,
		21,
	))

	amounts, err := newNetworkMgr(k, nil, nil).calculateNetworkSolvency(ctx, newShielderFlowTestManager(k))
	if err != nil {
		t.Fatal(err)
	}
	if len(amounts) != 1 || !amounts[0].Asset.Equals(common.BTCAsset) {
		t.Fatalf("unexpected solvency rows: %+v", amounts)
	}
	if got, want := amounts[0].Amount.Int64(), int64(950); got != want {
		t.Fatalf("solvency amount = %d, want %d", got, want)
	}
}

func TestCalculateNetworkSolvencyReturnsZeroWithoutReport(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	k.baseVaults = Vaults{solvencyTestVault(100)}

	amounts, err := newNetworkMgr(k, nil, nil).calculateNetworkSolvency(ctx, newShielderFlowTestManager(k))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := amounts[0].Amount.Int64(), int64(0); got != want {
		t.Fatalf("solvency amount = %d, want %d", got, want)
	}
}

func TestCalculateNetworkSolvencyIgnoresInactiveVaultReports(t *testing.T) {
	ctx := flowTestContext()
	k := newShielderFlowTestKeeper()
	inactive := solvencyTestVaultWithStatus(10_000, InactiveVault)
	active := solvencyTestVaultWithStatus(1_000, ActiveVault)
	k.baseVaults = Vaults{inactive, active}
	k.SetSolvencyVoter(ctx, solvencyTestVoter(
		"1000000000000000000000000000000000000000000000000000000000000021",
		inactive.PubKey,
		10_000,
		20,
		30,
	))
	k.SetSolvencyVoter(ctx, solvencyTestVoter(
		"1000000000000000000000000000000000000000000000000000000000000022",
		active.PubKey,
		900,
		20,
		30,
	))

	amounts, err := newNetworkMgr(k, nil, nil).calculateNetworkSolvency(ctx, newShielderFlowTestManager(k))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := amounts[0].Amount.Int64(), int64(900); got != want {
		t.Fatalf("solvency amount = %d, want %d", got, want)
	}
}

func solvencyTestVault(sats uint64) Vault {
	return solvencyTestVaultWithStatus(sats, ActiveVault)
}

func solvencyTestVaultWithStatus(sats uint64, status VaultStatus) Vault {
	vault := NewVaultV2(1, status, BaseVault, GetRandomPubKey(), common.Chains{common.BTCChain}.Strings(), GetRandomPubKey())
	vault.AddFunds(common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(sats))))
	return vault
}

func solvencyTestVoter(id string, pubKey common.PubKey, sats uint64, height, consensusHeight int64) types.SolvencyVoter {
	return types.SolvencyVoter{
		Id:                   common.TxID(id),
		Chain:                common.BTCChain,
		PubKey:               pubKey,
		Coins:                common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(sats))),
		Height:               height,
		ConsensusBlockHeight: consensusHeight,
		Signers:              []string{GetRandomBech32Addr().String()},
	}
}

func observedGasVoter(txID common.TxID, from, to common.Address, pubKey common.PubKey, amountSats, gasSats uint64) ObservedTxVoter {
	tx := common.NewTx(
		txID,
		from,
		to,
		common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(amountSats))),
		common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(gasSats))},
	)
	observed := common.NewObservedTx(tx, 100, pubKey, 100)
	return ObservedTxVoter{TxID: txID, Tx: observed, Txs: common.ObservedTxs{observed}}
}

func gasTestInternalTxOut(txType string, outHash common.TxID, sourceSats, outputSats uint64) TxOutItem {
	return TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   common.Address("bcrt1pinternal"),
		VaultPubKey: GetRandomPubKey(),
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(outputSats)),
		OutHash:     outHash,
		TxType:      txType,
		SourceInputs: []types.TxOutInput{
			{TxId: common.TxID(outHash.String()[:63] + "0"), Vout: 0, AmountSats: sourceSats},
		},
	}
}

func assertGasBreakdown(t *testing.T, got map[string]*types.QueryGasBreakdown, txType string, gasSats, txCount uint64) {
	t.Helper()
	row, ok := got[txType]
	if !ok {
		t.Fatalf("missing gas row %s", txType)
	}
	if row.GasSats != gasSats || row.TxCount != txCount {
		t.Fatalf("gas row %s = gas %d count %d", txType, row.GasSats, row.TxCount)
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
