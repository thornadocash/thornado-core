package btc

import (
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/shared/signercache"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	ttypes "github.com/thornadocash/go-thornado/x/thornado/types"
)

func newBatchCacheTestClient(t *testing.T) (*Client, func()) {
	t.Helper()
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := signercache.NewSignerCacheManager(db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return &Client{signerCacheManager: mgr}, func() { _ = db.Close() }
}

func batchCacheTestItem(t *testing.T, txid string, sats uint64) stypes.TxOutItem {
	t.Helper()
	inHash, err := common.NewTxID(txid)
	if err != nil {
		t.Fatal(err)
	}
	to, err := common.NewAddress("bcrt1pzw3dft08ts0r00y7lhpx50w7wfvqvhxal5pssdl9pkmv8mm5fjpsn4735s")
	if err != nil {
		t.Fatal(err)
	}
	return stypes.TxOutItem{
		Chain:     common.BTCChain,
		ToAddress: to,
		Coins:     common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(sats))),
		InHash:    inHash,
		TxType:    "out",
	}
}

func TestTxBatchAlreadySignedRequiresEveryItem(t *testing.T) {
	client, closeDB := newBatchCacheTestClient(t)
	defer closeDB()

	items := []stypes.TxOutItem{
		batchCacheTestItem(t, "0000000000000000000000000000000000000000000000000000000000000001", 1_000),
		batchCacheTestItem(t, "0000000000000000000000000000000000000000000000000000000000000002", 2_000),
	}
	if client.txBatchAlreadySigned(items) {
		t.Fatal("empty cache should not mark batch signed")
	}
	if err := client.signerCacheManager.SetSigned(items[1].CacheHash(), items[1].CacheVault(common.BTCChain), "txid"); err != nil {
		t.Fatal(err)
	}
	if client.txBatchAlreadySigned(items) {
		t.Fatal("partial cache hit should not mark the full batch signed")
	}
	if err := client.signerCacheManager.SetSigned(items[0].CacheHash(), items[0].CacheVault(common.BTCChain), "txid"); err != nil {
		t.Fatal(err)
	}
	if !client.txBatchAlreadySigned(items) {
		t.Fatal("batch should be signed when every member item is signed")
	}
}

func TestTxAlreadySignedDoesNotBlockInternalRecovery(t *testing.T) {
	client, closeDB := newBatchCacheTestClient(t)
	defer closeDB()

	item := batchCacheTestItem(t, "0000000000000000000000000000000000000000000000000000000000000010", 1_000)
	if err := client.signerCacheManager.SetSigned(item.CacheHash(), item.CacheVault(common.BTCChain), "txid"); err != nil {
		t.Fatal(err)
	}
	if !client.txAlreadySigned(item) {
		t.Fatal("non-migration tx should respect signed cache")
	}

	item.TxType = ttypes.TxOutTypeMigrate
	if client.txAlreadySigned(item) {
		t.Fatal("migration tx should keep retrying until chain state has an out hash")
	}

	item.TxType = ttypes.TxOutTypeSweep
	if client.txAlreadySigned(item) {
		t.Fatal("sweep tx should keep retrying until chain state has an out hash")
	}
}

func TestMarkTxBatchSignedMarksEveryItem(t *testing.T) {
	client, closeDB := newBatchCacheTestClient(t)
	defer closeDB()

	items := []stypes.TxOutItem{
		batchCacheTestItem(t, "0000000000000000000000000000000000000000000000000000000000000011", 1_000),
		batchCacheTestItem(t, "0000000000000000000000000000000000000000000000000000000000000012", 2_000),
	}
	if err := client.MarkTxBatchSigned(items, "txid"); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if !client.signerCacheManager.HasSigned(item.CacheHash()) {
			t.Fatalf("batch item %s was not marked signed", item.InHash)
		}
	}
}

func TestFilterUtxosBySourceInputsOnlySelectsMatchingOutpoint(t *testing.T) {
	utxos := []btcjson.ListUnspentResult{
		{TxID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Vout: 0, Amount: 0.12},
		{TxID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Vout: 0, Amount: 0.12},
		{TxID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Vout: 1, Amount: 0.12},
	}
	sourceTx, err := common.NewTxID("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}

	selected, err := filterUtxosBySourceInputs(
		utxos,
		[]stypes.TxOutInput{{TxID: sourceTx, Vout: 1, AmountSats: 12_000_000}},
		btcutil.Amount(12_000_000),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 {
		t.Fatalf("expected one UTXO, got %d", len(selected))
	}
	if selected[0].TxID != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || selected[0].Vout != 1 {
		t.Fatalf("selected wrong UTXO: %s:%d", selected[0].TxID, selected[0].Vout)
	}
}

func TestFilterUtxosBySourceInputsCanUseMultiplePrescribedInputs(t *testing.T) {
	utxos := []btcjson.ListUnspentResult{
		{TxID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Vout: 0, Amount: 0.05},
		{TxID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Vout: 1, Amount: 0.07},
		{TxID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Vout: 0, Amount: 0.12},
	}
	sourceTx, err := common.NewTxID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}

	selected, err := filterUtxosBySourceInputs(
		utxos,
		[]stypes.TxOutInput{
			{TxID: sourceTx, Vout: 0, AmountSats: 5_000_000},
			{TxID: sourceTx, Vout: 1, AmountSats: 7_000_000},
		},
		btcutil.Amount(12_000_000),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected two same-tx UTXOs, got %d", len(selected))
	}
	for _, utxo := range selected {
		if utxo.TxID != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("selected non-source UTXO: %s", utxo.TxID)
		}
	}
}

func TestMigrateOutputAmountSpendsSelectedInputsMinusGas(t *testing.T) {
	output, err := internalTxOutputAmount("migrate", 90_000_000, 7_500, 49_992_500)
	if err != nil {
		t.Fatal(err)
	}
	if output != 89_992_500 {
		t.Fatalf("expected migrate to output selected inputs minus gas, got %d", output)
	}
}

func TestNormalOutputAmountUsesScheduledCoin(t *testing.T) {
	output, err := internalTxOutputAmount("out", 90_000_000, 7_500, 49_992_500)
	if err != nil {
		t.Fatal(err)
	}
	if output != 49_992_500 {
		t.Fatalf("expected normal outbound to use scheduled coin amount, got %d", output)
	}
}
