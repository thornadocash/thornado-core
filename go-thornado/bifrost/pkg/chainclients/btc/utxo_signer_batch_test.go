package btc

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	btcwire "github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	cmtsecp256k1 "github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/thornadocash/go-thornado/bifrost/frost"
	p2pstorage "github.com/thornadocash/go-thornado/bifrost/p2p/storage"
	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/shared/signercache"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/config"
	frostsessions "github.com/thornadocash/go-thornado/go-wrappers/frost/go-frost/sessions"
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

func TestBTCBatchMaxGasSumsItemShares(t *testing.T) {
	txs := []stypes.TxOutItem{
		{MaxGas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(101))}},
		{MaxGas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(202))}},
		{MaxGas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(303))}},
	}
	got := btcBatchMaxGas(txs).ToCoins().GetCoin(common.BTCAsset).Amount.Uint64()
	if got != 606 {
		t.Fatalf("expected summed batch max gas, got %d", got)
	}
}

func TestRecoveredOutputAmountAllowsInternalActualGasBelowMaxGas(t *testing.T) {
	item := stypes.TxOutItem{
		Chain:  common.BTCChain,
		Coins:  common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(19_996_700))),
		MaxGas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_300))},
		TxType: ttypes.TxOutTypeSweep,
	}
	if !recoveredOutputAmountMatchesTxOut(item, 19_997_690) {
		t.Fatal("internal recovered output should allow actual gas below max gas")
	}
	if recoveredOutputAmountMatchesTxOut(item, 20_000_001) {
		t.Fatal("internal recovered output must not exceed coin plus max gas")
	}
	if recoveredOutputAmountMatchesTxOut(item, 19_996_699) {
		t.Fatal("internal recovered output must not be below instructed coin")
	}
}

func TestRecoveredOutputAmountKeepsExternalTxOutExact(t *testing.T) {
	item := stypes.TxOutItem{
		Chain:  common.BTCChain,
		Coins:  common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(19_996_700))),
		MaxGas: common.Gas{common.NewCoin(common.BTCAsset, cosmos.NewUint(3_300))},
		TxType: ttypes.TxOutTypeOut,
	}
	if !recoveredOutputAmountMatchesTxOut(item, 19_996_700) {
		t.Fatal("external recovered output should accept exact amount")
	}
	if recoveredOutputAmountMatchesTxOut(item, 19_997_690) {
		t.Fatal("external recovered output must not overpay")
	}
}

type concurrentSignCoordinator struct {
	shares    map[string][]byte
	mu        sync.Mutex
	active    int
	maxActive int
}

func (c *concurrentSignCoordinator) RunKeygen(
	ctx context.Context,
	height int64,
	participants []string,
	localParty string,
	minSigners uint16,
) (localShare []byte, pubKeyCompressed []byte, err error) {
	in := &frost.InProcessSessionCoordinator{}
	return in.RunKeygen(ctx, height, participants, localParty, minSigners)
}

func (c *concurrentSignCoordinator) RunSign(
	ctx context.Context,
	_ string,
	_ int64,
	participants []string,
	localParty string,
	_ []byte,
	msg []byte,
	taprootKeyPath bool,
	scriptRoot []byte,
	childTweak []byte,
	_ string,
) ([]byte, error) {
	c.mu.Lock()
	c.active++
	if c.active > c.maxActive {
		c.maxActive = c.active
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active--
		c.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(25 * time.Millisecond):
	}
	return frost.RunInProcessSign(participants, c.shares, localParty, msg, taprootKeyPath, scriptRoot, childTweak)
}

func (c *concurrentSignCoordinator) MaxActive() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxActive
}

func TestSignRedeemTxInputsRunsFrostInputsSequentially(t *testing.T) {
	participants := []string{"node-a", "node-b", "node-c"}
	allShares, err := frost.RunInProcessKeygenAll(participants, 2)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := frostsessions.DecodeKeyshare(allShares["node-a"])
	if err != nil {
		t.Fatal(err)
	}
	pubKeyBytes, err := hex.DecodeString(decoded.PublicKeyCompressed)
	if err != nil {
		t.Fatal(err)
	}
	vaultPubKey, err := common.NewPubKeyFromCrypto(cmtsecp256k1.PubKey(pubKeyBytes))
	if err != nil {
		t.Fatal(err)
	}

	coordinator := &concurrentSignCoordinator{shares: allShares}
	client := &Client{
		cfg: config.BifrostChainConfiguration{ChainID: common.BTCChain},
		frostKeySigner: &frostVaultSigner{
			localState: &memoryLocalState{states: map[string]p2pstorage.KeygenLocalState{
				vaultPubKey.String(): {
					PubKey:          vaultPubKey.String(),
					LocalData:       allShares["node-a"],
					ParticipantKeys: participants,
					LocalPartyKey:   "node-a",
					SigningEngine:   p2pstorage.SigningEngineFrost,
				},
			}},
			log:         zerolog.Nop(),
			coordinator: coordinator,
			bridge:      &stubKeysignBridge{party: participants[:2]},
			localParty:  "node-a",
		},
		log: zerolog.Nop(),
	}
	sourceScript, err := client.getSchnorrSourceScriptAtPath(vaultPubKey, common.MainVaultPathIndex)
	if err != nil {
		t.Fatal(err)
	}

	redeemTx := btcwire.NewMsgTx(2)
	signings := make([]utxoSigning, 0, 3)
	for i := 0; i < 3; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%064x", i+1))
		if err != nil {
			t.Fatal(err)
		}
		redeemTx.AddTxIn(btcwire.NewTxIn(btcwire.NewOutPoint(hash, uint32(i)), nil, nil))
		signings = append(signings, utxoSigning{idx: int64(i), amount: 100_000})
	}
	redeemTx.AddTxOut(btcwire.NewTxOut(250_000, []byte{0x51}))
	tx := stypes.TxOutItem{
		Chain:          common.BTCChain,
		VaultPubKey:    vaultPubKey,
		VaultPathIndex: common.MainVaultPathIndex,
	}

	if err := client.signRedeemTxInputs(redeemTx, tx, signings, sourceScript); err != nil {
		t.Fatal(err)
	}
	if coordinator.MaxActive() != 1 {
		t.Fatalf("expected sequential FROST input signing, max active sessions was %d", coordinator.MaxActive())
	}
	for i, txIn := range redeemTx.TxIn {
		if len(txIn.Witness) != 1 || len(txIn.Witness[0]) != 65 {
			t.Fatalf("input %d witness was not signed: %#v", i, txIn.Witness)
		}
		if got := txIn.Witness[0][64]; got != schnorrSigHashAllAnyoneCanPay {
			t.Fatalf("input %d sighash byte = %x", i, got)
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

func TestRecoveredTxMustSpendPrescribedSourceInputs(t *testing.T) {
	sourceTx, err := common.NewTxID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	raw := &btcjson.TxRawResult{
		Txid: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Vin: []btcjson.Vin{
			{Txid: sourceTx.String(), Vout: 1},
		},
	}
	if recoveredTxSpendsSourceInputs(raw, []stypes.TxOutInput{{TxID: sourceTx, Vout: 0}}) {
		t.Fatal("recovered tx matched the wrong prescribed source vout")
	}
	if !recoveredTxSpendsSourceInputs(raw, []stypes.TxOutInput{{TxID: sourceTx, Vout: 1}}) {
		t.Fatal("recovered tx did not match its prescribed source input")
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
