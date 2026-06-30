package btc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/rs/zerolog"
	"github.com/syndtr/goleveldb/leveldb"

	"github.com/thornadocash/go-thornado/bifrost/pkg/chainclients/btc/rpc"
	stypes "github.com/thornadocash/go-thornado/bifrost/thornadoclient/types"
	"github.com/thornadocash/go-thornado/common"
)

func TestIgnoreTxSkipsNulldataOutputs(t *testing.T) {
	client := &Client{}
	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{{
			Txid: "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
			Vout: 0,
		}},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{Type: "nulldata"},
			},
			{
				Value: 0.20000000,
				N:     1,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"bcrt1p9g0l2y7rvn2n30tj3nfnn6hwt73wgmlqhxgu52ytkx8dgayahg2q33j7u5"},
				},
			},
		},
	}
	if client.ignoreTx(tx, 100) {
		t.Fatal("OP_RETURN output caused valid value-bearing tx to be ignored")
	}

	tx.Vout = []btcjson.Vout{{
		ScriptPubKey: btcjson.ScriptPubKeyResult{Type: "nulldata"},
	}}
	if !client.ignoreTx(tx, 100) {
		t.Fatal("tx with only OP_RETURN outputs should be ignored")
	}
}

func TestCanonicalObservedHeightUsesKnownScannerHeight(t *testing.T) {
	client := &Client{}
	tx := &btcjson.TxRawResult{
		Txid:      "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2",
		BlockHash: "000000008de7a25f64f9780b6c894016d2c63716a89f7c9e704ebb7e8377a0c8",
	}

	if got := client.canonicalObservedHeight(tx, 1024, false); got != 1024 {
		t.Fatalf("expected scanner-provided height 1024, got %d", got)
	}
}

func TestConfirmTxClassifiesActiveMempoolStaleAndMissing(t *testing.T) {
	activeTxID := strings.Repeat("1", 64)
	mempoolTxID := strings.Repeat("2", 64)
	staleTxID := strings.Repeat("3", 64)
	missingTxID := strings.Repeat("4", 64)
	responses := map[string]map[string]any{
		activeTxID: {
			"result": map[string]any{
				"txid":          activeTxID,
				"hash":          activeTxID,
				"confirmations": 2,
				"blockhash":     strings.Repeat("a", 64),
				"vin":           []map[string]any{},
				"vout":          []map[string]any{},
			},
		},
		mempoolTxID: {
			"result": map[string]any{
				"txid": mempoolTxID,
				"hash": mempoolTxID,
				"vin":  []map[string]any{},
				"vout": []map[string]any{},
			},
		},
		staleTxID: {
			"result": map[string]any{
				"txid":      staleTxID,
				"hash":      staleTxID,
				"blockhash": strings.Repeat("b", 64),
				"vin":       []map[string]any{},
				"vout":      []map[string]any{},
			},
		},
		missingTxID: {
			"error": map[string]any{
				"code":    -5,
				"message": "No such mempool or blockchain transaction",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "getrawtransaction" || len(req.Params) < 2 || req.Params[1] != true {
			t.Fatalf("unexpected rpc request: %+v", req)
		}
		txid, _ := req.Params[0].(string)
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}
		for key, value := range responses[txid] {
			resp[key] = value
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode rpc response: %v", err)
		}
	}))
	defer server.Close()

	rpcClient, err := rpc.NewClient(server.URL, "thornado", "thornado", 0, 10*time.Second, common.BTCChain, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		rpc: rpcClient,
		log: zerolog.Nop(),
	}

	tests := []struct {
		name string
		txid string
		want bool
	}{
		{name: "active chain", txid: activeTxID, want: true},
		{name: "mempool", txid: mempoolTxID, want: true},
		{name: "stale block", txid: staleTxID, want: false},
		{name: "missing", txid: missingTxID, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := client.confirmTx(tt.txid); got != tt.want {
				t.Fatalf("confirmTx(%s) = %v, want %v", tt.txid, got, tt.want)
			}
		})
	}
}

func TestMissingErrataReadySinceUsesMissingBlockHeight(t *testing.T) {
	client := &Client{log: zerolog.Nop()}
	txid := strings.Repeat("5", 64)

	if !client.missingErrataReadySince(txid, 100, 100+btcErrataDelayBlocks) {
		t.Fatal("reorg errata should be ready when the current height is past the missing block delay")
	}

	client.clearMissingErrata(txid)
	if client.missingErrataReady(txid, 100+btcErrataDelayBlocks) {
		t.Fatal("plain first-seen errata should not be ready on first observation")
	}
	if !client.missingErrataReady(txid, 100+(2*btcErrataDelayBlocks)) {
		t.Fatal("plain first-seen errata should be ready after the delay")
	}
}

func TestSweepSourceActuallyMissingTreatsInactiveBlockTxAsMissing(t *testing.T) {
	sourceTxID, err := common.NewTxID(strings.Repeat("6", 64))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode rpc request: %v", err)
		}
		if req.Method != "getrawtransaction" {
			t.Fatalf("unexpected rpc request: %s", req.Method)
		}
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"txid":      sourceTxID.String(),
				"hash":      sourceTxID.String(),
				"blockhash": strings.Repeat("a", 64),
				"vin":       []map[string]any{},
				"vout": []map[string]any{{
					"value": 0.2,
					"n":     0,
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode rpc response: %v", err)
		}
	}))
	defer server.Close()

	rpcClient, err := rpc.NewClient(server.URL, "thornado", "thornado", 0, 10*time.Second, common.BTCChain, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		rpc: rpcClient,
		log: zerolog.Nop(),
	}
	missing, err := client.sweepSourceActuallyMissing(stypes.TxOutItem{
		InHash: sourceTxID,
		SourceInputs: []stypes.TxOutInput{{
			TxID: sourceTxID,
			Vout: 0,
		}},
	})
	if err != nil {
		t.Fatalf("sweepSourceActuallyMissing failed: %v", err)
	}
	if !missing {
		t.Fatal("inactive block source tx should be classified as missing")
	}
}

func TestGetGasUsesProvidedVinTransactions(t *testing.T) {
	client := &Client{}
	client.cfg.ChainID = common.BTCChain
	prevTxID := "24ed2d26fd5d4e0e8fa86633e40faf1bdfc8d1903b1cd02855286312d48818a2"
	tx := &btcjson.TxRawResult{
		Vin: []btcjson.Vin{{
			Txid: prevTxID,
			Vout: 0,
		}},
		Vout: []btcjson.Vout{{
			Value: 0.90000000,
			N:     0,
		}},
	}
	vinTxs := map[string]*btcjson.TxRawResult{
		prevTxID: {
			Txid: prevTxID,
			Vout: []btcjson.Vout{{
				Value: 1.00000000,
				N:     0,
			}},
		},
	}

	gas, err := client.getGasWithVinTxs(tx, vinTxs)
	if err != nil {
		t.Fatalf("getGasWithVinTxs failed: %v", err)
	}
	if len(gas) != 1 {
		t.Fatalf("expected one gas coin, got %d", len(gas))
	}
	if got := gas[0].Amount.Uint64(); got != 10_000_000 {
		t.Fatalf("expected 10000000 sats gas, got %d", got)
	}
}

func BenchmarkGetSchnorrSourceScriptAtPath(b *testing.B) {
	vaultPubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		b.Fatal(err)
	}
	path, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 42, common.DepositPathCommitmentRoot)
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{}
	client.cfg.ChainID = common.BTCChain

	script, err := client.getSchnorrSourceScriptAtPath(vaultPubKey, path)
	if err != nil {
		b.Fatal(err)
	}
	if len(script) != 34 || script[0] != 0x51 || script[1] != 0x20 {
		b.Fatalf("unexpected source script: %x", script)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		script, err = client.getSchnorrSourceScriptAtPath(vaultPubKey, path)
		if err != nil {
			b.Fatal(err)
		}
		if len(script) != 34 {
			b.Fatalf("unexpected source script length: %d", len(script))
		}
	}
}

func BenchmarkTaprootWitnessVerifyPubKey(b *testing.B) {
	vaultPubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		b.Fatal(err)
	}
	path, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 42, common.DepositPathCommitmentRoot)
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{}
	client.cfg.ChainID = common.BTCChain
	tx := stypes.TxOutItem{
		Chain:          common.BTCChain,
		VaultPubKey:    vaultPubKey,
		VaultPathIndex: path,
	}
	sourceScript, err := client.getSchnorrSourceScriptAtPath(vaultPubKey, path)
	if err != nil {
		b.Fatal(err)
	}
	pubkey, err := client.taprootWitnessVerifyPubKey(tx, sourceScript)
	if err != nil {
		b.Fatal(err)
	}
	if pubkey == nil {
		b.Fatal("nil taproot pubkey")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pubkey, err = client.taprootWitnessVerifyPubKey(tx, sourceScript)
		if err != nil {
			b.Fatal(err)
		}
		if pubkey == nil {
			b.Fatal("nil taproot pubkey")
		}
	}
}

func BenchmarkFilterUtxosBySourceInputs(b *testing.B) {
	const (
		utxoCount  = 2048
		inputCount = 128
	)
	utxos := make([]btcjson.ListUnspentResult, utxoCount)
	inputs := make([]stypes.TxOutInput, inputCount)
	for i := range utxos {
		txID := fmt.Sprintf("%064x", i+1)
		utxos[i] = btcjson.ListUnspentResult{
			TxID:   txID,
			Vout:   uint32(i % 4),
			Amount: 1.0,
		}
	}
	for i := range inputs {
		idx := i * (utxoCount / inputCount)
		txID, err := common.NewTxID(utxos[idx].TxID)
		if err != nil {
			b.Fatal(err)
		}
		inputs[i] = stypes.TxOutInput{
			TxID:       txID,
			Vout:       utxos[idx].Vout,
			AmountSats: 100_000_000,
		}
	}

	filtered, err := filterUtxosBySourceInputs(utxos, inputs, btcutil.Amount(inputCount))
	if err != nil {
		b.Fatal(err)
	}
	if len(filtered) != inputCount {
		b.Fatalf("expected %d filtered utxos, got %d", inputCount, len(filtered))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filtered, err = filterUtxosBySourceInputs(utxos, inputs, btcutil.Amount(inputCount))
		if err != nil {
			b.Fatal(err)
		}
		if len(filtered) != inputCount {
			b.Fatalf("expected %d filtered utxos, got %d", inputCount, len(filtered))
		}
	}
}

func BenchmarkRecoveredTxSpendsSourceInputs(b *testing.B) {
	const inputCount = 256

	raw := &btcjson.TxRawResult{
		Vin: make([]btcjson.Vin, inputCount),
	}
	inputs := make([]stypes.TxOutInput, inputCount)
	for i := range raw.Vin {
		txID := fmt.Sprintf("%064x", i+1)
		raw.Vin[i] = btcjson.Vin{
			Txid: txID,
			Vout: uint32(i % 4),
		}
		parsedTxID, err := common.NewTxID(txID)
		if err != nil {
			b.Fatal(err)
		}
		inputs[i] = stypes.TxOutInput{
			TxID:       parsedTxID,
			Vout:       raw.Vin[i].Vout,
			AmountSats: uint64(10_000_000 + i),
		}
	}
	if !recoveredTxSpendsSourceInputs(raw, inputs) {
		b.Fatal("expected recovered tx to spend source inputs")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !recoveredTxSpendsSourceInputs(raw, inputs) {
			b.Fatal("expected recovered tx to spend source inputs")
		}
	}
}

func BenchmarkGetTxInsWithBatchedVinTransactions(b *testing.B) {
	vaultPubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		b.Fatal(err)
	}
	firstPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 1, common.DepositPathCommitmentRoot)
	if err != nil {
		b.Fatal(err)
	}
	secondPath, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 2, common.DepositPathCommitmentRoot)
	if err != nil {
		b.Fatal(err)
	}
	firstAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, firstPath)
	if err != nil {
		b.Fatal(err)
	}
	secondAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, secondPath)
	if err != nil {
		b.Fatal(err)
	}

	client := &Client{
		bridge:     &mockBaseBridge{},
		vaultPaths: make(map[string]map[uint64]struct{}),
	}
	client.cfg.ChainID = common.BTCChain
	client.rememberVaultPath(vaultPubKey, firstPath)
	client.rememberVaultPath(vaultPubKey, secondPath)

	prevTxID := strings.Repeat("c", 64)
	txID := strings.Repeat("d", 64)
	vinTxs := map[string]*btcjson.TxRawResult{
		prevTxID: {
			Txid: prevTxID,
			Vout: []btcjson.Vout{{
				Value: 1.00000000,
				N:     0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
					Hex:       "00140653096f54ae1ae2d73291d15854aef08ebcfa8c",
					Type:      "witness_v0_keyhash",
				},
			}},
		},
	}
	tx := btcjson.TxRawResult{
		Txid: txID,
		Hash: txID,
		Vin: []btcjson.Vin{{
			Txid: prevTxID,
			Vout: 0,
		}},
		Vout: []btcjson.Vout{
			{
				ScriptPubKey: btcjson.ScriptPubKeyResult{Type: "nulldata"},
			},
			{
				Value: 0.10000000,
				N:     1,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			},
			{
				Value: 0.20000000,
				N:     2,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{firstAddress.String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
			{
				Value: 0.30000000,
				N:     3,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{secondAddress.String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			},
		},
	}

	txIns, err := client.getTxIns(&tx, 1024, false, vinTxs)
	if err != nil {
		b.Fatal(err)
	}
	if len(txIns) != 2 {
		b.Fatalf("expected two txins, got %d", len(txIns))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		txIns, err = client.getTxIns(&tx, 1024, false, vinTxs)
		if err != nil {
			b.Fatal(err)
		}
		if len(txIns) != 2 {
			b.Fatalf("expected two txins, got %d", len(txIns))
		}
	}
}

func BenchmarkGetVinZeroTxsWithDuplicateInputs(b *testing.B) {
	const txCount = 120
	const uniqueInputs = 12

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var batch []struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			b.Fatalf("decode rpc batch: %v", err)
		}
		responses := make([]map[string]any, 0, len(batch))
		for _, req := range batch {
			if req.Method != "getrawtransaction" || len(req.Params) == 0 {
				b.Fatalf("unexpected rpc request: %+v", req)
			}
			txid, _ := req.Params[0].(string)
			responses = append(responses, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"txid": txid,
					"hash": txid,
					"vin": []map[string]any{{
						"coinbase": "00",
						"sequence": 4294967295,
					}},
					"vout": []map[string]any{{
						"value": 1.0,
						"n":     0,
						"scriptPubKey": map[string]any{
							"hex":       "00140653096f54ae1ae2d73291d15854aef08ebcfa8c",
							"type":      "witness_v0_keyhash",
							"addresses": []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
						},
					}},
				},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(responses); err != nil {
			b.Fatalf("encode rpc response: %v", err)
		}
	}))
	defer server.Close()

	rpcClient, err := rpc.NewClient(server.URL, "thornado", "thornado", 0, 10*time.Second, common.BTCChain, zerolog.Nop())
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{
		rpc: rpcClient,
		log: zerolog.Nop(),
	}
	client.cfg.ChainID = common.BTCChain
	client.cfg.UTXO.TransactionBatchSize = txCount

	block := &btcjson.GetBlockVerboseTxResult{
		Height: 1024,
		Tx:     make([]btcjson.TxRawResult, txCount),
	}
	for i := range block.Tx {
		prevTxID := fmt.Sprintf("%064x", i%uniqueInputs+1)
		txID := fmt.Sprintf("%064x", i+uniqueInputs+1)
		block.Tx[i] = btcjson.TxRawResult{
			Txid: txID,
			Hash: txID,
			Vin: []btcjson.Vin{{
				Txid: prevTxID,
				Vout: 0,
			}},
			Vout: []btcjson.Vout{{
				Value: 0.01000000,
				N:     0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{"tb1qkq7weysjn6ljc2ywmjmwp8ttcckg8yyxjdz5k6"},
				},
			}},
		}
	}

	vinZeroTxs, err := client.getVinZeroTxs(block)
	if err != nil {
		b.Fatal(err)
	}
	if len(vinZeroTxs) != uniqueInputs {
		b.Fatalf("expected %d vin zero txs, got %d", uniqueInputs, len(vinZeroTxs))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vinZeroTxs, err = client.getVinZeroTxs(block)
		if err != nil {
			b.Fatal(err)
		}
		if len(vinZeroTxs) != uniqueInputs {
			b.Fatalf("expected %d vin zero txs, got %d", uniqueInputs, len(vinZeroTxs))
		}
	}
}

func BenchmarkExtractTxsManyObservedOutputs(b *testing.B) {
	const txCount = 32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var batch []struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params []any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			b.Fatalf("decode rpc batch: %v", err)
		}
		responses := make([]map[string]any, 0, len(batch))
		for _, req := range batch {
			txid, _ := req.Params[0].(string)
			responses = append(responses, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"txid": txid,
					"hash": txid,
					"vin": []map[string]any{{
						"coinbase": "00",
						"sequence": 4294967295,
					}},
					"vout": []map[string]any{{
						"value": 1.0,
						"n":     0,
						"scriptPubKey": map[string]any{
							"hex":       "00140653096f54ae1ae2d73291d15854aef08ebcfa8c",
							"type":      "witness_v0_keyhash",
							"addresses": []string{"tb1qj08ys4ct2hzzc2hcz6h2hgrvlmsjynaw43s835"},
						},
					}},
				},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(responses); err != nil {
			b.Fatalf("encode rpc response: %v", err)
		}
	}))
	defer server.Close()

	rpcClient, err := rpc.NewClient(server.URL, "thornado", "thornado", 0, 10*time.Second, common.BTCChain, zerolog.Nop())
	if err != nil {
		b.Fatal(err)
	}
	db, err := leveldb.OpenFile(b.TempDir(), nil)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	storage, err := NewTemporalStorage(db, 0)
	if err != nil {
		b.Fatal(err)
	}

	vaultPubKey, err := common.NewPubKeyFromCrypto(secp256k1.GenPrivKey().PubKey())
	if err != nil {
		b.Fatal(err)
	}
	path, err := common.VaultDepositPathIndex(common.VaultDepositPathUser, 1, common.DepositPathCommitmentRoot)
	if err != nil {
		b.Fatal(err)
	}
	vaultAddress, err := common.DeriveBTCTaprootAddress(vaultPubKey, path)
	if err != nil {
		b.Fatal(err)
	}
	client := &Client{
		rpc:             rpcClient,
		log:             zerolog.Nop(),
		bridge:          &mockBaseBridge{},
		temporalStorage: storage,
		vaultPaths:      make(map[string]map[uint64]struct{}),
	}
	client.cfg.ChainID = common.BTCChain
	client.cfg.UTXO.TransactionBatchSize = txCount
	client.rememberVaultPath(vaultPubKey, path)

	block := &btcjson.GetBlockVerboseTxResult{
		Height: 1024,
		Tx:     make([]btcjson.TxRawResult, txCount),
	}
	for i := range block.Tx {
		prevTxID := fmt.Sprintf("%064x", i+1)
		block.Tx[i] = btcjson.TxRawResult{
			Vin: []btcjson.Vin{{
				Txid: prevTxID,
				Vout: 0,
			}},
			Vout: []btcjson.Vout{{
				Value: 0.01000000,
				N:     0,
				ScriptPubKey: btcjson.ScriptPubKeyResult{
					Addresses: []string{vaultAddress.String()},
					Hex:       "5120f01002397e3cb9179d41f1e25412bd29fc8d22f8fe786758aeeacf137a4cbc5f",
					Type:      "witness_v1_taproot",
				},
			}},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range block.Tx {
			txID := fmt.Sprintf("%056x%08x", i+1, j+1)
			block.Tx[j].Txid = txID
			block.Tx[j].Hash = txID
		}
		txIn, err := client.extractTxs(block)
		if err != nil {
			b.Fatal(err)
		}
		if len(txIn.TxArray) != txCount {
			b.Fatalf("expected %d txins, got %d", txCount, len(txIn.TxArray))
		}
	}
}
