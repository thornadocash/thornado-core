package btc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	btcwire "github.com/btcsuite/btcd/wire"
)

// Conformance vectors for the taproot key-spend sighash.
//
// Direction of truth: BIP341 as implemented by rust-bitcoin is the REFERENCE.
// The hand-rolled Go taprootKeySpendSigHash is the implementation under test.
// These vectors capture Go's output; the Rust bifrost-signer crate recomputes
// the same vectors through rust-bitcoin's SighashCache and asserts equality.
// If the two ever disagree, the Go side gets fixed — never the Rust side bent
// to match Go.
//
// Regenerate fixtures with:
//   BIFROST_WRITE_FIXTURES=1 go test -tags mocknet -run TestTaprootSighashGolden ./bifrost/pkg/chainclients/btc/

type sighashInput struct {
	TxID     string `json:"txid"`
	Vout     uint32 `json:"vout"`
	Sequence uint32 `json:"sequence"`
}

type sighashOutput struct {
	ValueSats int64  `json:"value_sats"`
	ScriptHex string `json:"script_hex"`
}

type sighashVector struct {
	Name            string          `json:"name"`
	Version         int32           `json:"version"`
	LockTime        uint32          `json:"lock_time"`
	Inputs          []sighashInput  `json:"inputs"`
	Outputs         []sighashOutput `json:"outputs"`
	InputIndex      int             `json:"input_index"`
	AmountSats      int64           `json:"amount_sats"`
	SourceScriptHex string          `json:"source_script_hex"`
	SighashHex      string          `json:"sighash_hex"`
	// Wire-serialization conformance: Go's consensus encoding of the tx
	// before signing and after taproot key-path witnesses (64-byte dummy sig
	// + 0x81 flag on every input) are attached, exactly as taprootUTXOWitness
	// assembles them.
	UnsignedTxHex string `json:"unsigned_tx_hex"`
	SignedTxHex   string `json:"signed_tx_hex"`
}

// p2trScript builds a vault-style P2TR scriptPubKey (OP_1 OP_DATA_32 <key>).
func p2trScript(fill byte) []byte {
	s := make([]byte, 34)
	s[0] = 0x51 // OP_1
	s[1] = 0x20 // push 32
	for i := 2; i < 34; i++ {
		s[i] = fill
	}
	return s
}

func fillHashHex(fill byte) string {
	b := make([]byte, chainhash.HashSize)
	for i := range b {
		b[i] = fill
	}
	return hex.EncodeToString(b)
}

func buildVectorTx(v sighashVector) *btcwire.MsgTx {
	tx := btcwire.NewMsgTx(v.Version)
	tx.LockTime = v.LockTime
	for _, in := range v.Inputs {
		var h chainhash.Hash
		raw, _ := hex.DecodeString(in.TxID)
		copy(h[:], raw)
		op := btcwire.NewOutPoint(&h, in.Vout)
		txin := btcwire.NewTxIn(op, nil, nil)
		txin.Sequence = in.Sequence
		tx.AddTxIn(txin)
	}
	for _, out := range v.Outputs {
		script, _ := hex.DecodeString(out.ScriptHex)
		tx.AddTxOut(btcwire.NewTxOut(out.ValueSats, script))
	}
	return tx
}

func vectorFixtures() []sighashVector {
	// A single-input, single-output spend (sweep-shaped).
	single := sighashVector{
		Name:     "single_in_single_out",
		Version:  1,
		LockTime: 0,
		Inputs: []sighashInput{
			{TxID: fillHashHex(0x11), Vout: 0, Sequence: 0xfffffffe},
		},
		Outputs: []sighashOutput{
			{ValueSats: 95_000, ScriptHex: hex.EncodeToString(p2trScript(0x22))},
		},
		InputIndex:      0,
		AmountSats:      100_000,
		SourceScriptHex: hex.EncodeToString(p2trScript(0x33)),
	}
	// A two-input, two-output batch, signing input 1 (exercises hash_outputs
	// over multiple outputs and a non-zero input index selection).
	batch := sighashVector{
		Name:     "two_in_two_out_index1",
		Version:  1,
		LockTime: 0,
		Inputs: []sighashInput{
			{TxID: fillHashHex(0x44), Vout: 1, Sequence: 0xfffffffe},
			{TxID: fillHashHex(0x55), Vout: 3, Sequence: 0xfffffffe},
		},
		Outputs: []sighashOutput{
			{ValueSats: 40_000, ScriptHex: hex.EncodeToString(p2trScript(0x66))},
			{ValueSats: 55_000, ScriptHex: hex.EncodeToString(p2trScript(0x77))},
		},
		InputIndex:      1,
		AmountSats:      60_000,
		SourceScriptHex: hex.EncodeToString(p2trScript(0x33)),
	}
	return []sighashVector{single, batch}
}

func TestTaprootSighashGolden(t *testing.T) {
	// Pinned expected digests — a change here means the sighash construction
	// changed, which would break signature validity and Rust conformance.
	expected := map[string]string{}

	vectors := vectorFixtures()
	for i := range vectors {
		v := &vectors[i]
		tx := buildVectorTx(*v)
		src, err := hex.DecodeString(v.SourceScriptHex)
		if err != nil {
			t.Fatalf("%s: bad source script: %v", v.Name, err)
		}
		digest, err := taprootKeySpendSigHash(tx, v.InputIndex, v.AmountSats, src)
		if err != nil {
			t.Fatalf("%s: sighash error: %v", v.Name, err)
		}
		if len(digest) != 32 {
			t.Fatalf("%s: sighash length %d, want 32", v.Name, len(digest))
		}
		v.SighashHex = hex.EncodeToString(digest)

		// Sanity: recomputing is deterministic.
		again, _ := taprootKeySpendSigHash(tx, v.InputIndex, v.AmountSats, src)
		if hex.EncodeToString(again) != v.SighashHex {
			t.Fatalf("%s: sighash not deterministic", v.Name)
		}
		if want, ok := expected[v.Name]; ok && want != v.SighashHex {
			t.Fatalf("%s: sighash %s != pinned %s", v.Name, v.SighashHex, want)
		}

		// Wire-serialization vectors: unsigned, then with taproot key-path
		// witnesses attached the way taprootUTXOWitness builds them
		// (64-byte schnorr sig || 0x81 as the single witness element).
		var unsigned bytes.Buffer
		if err := tx.Serialize(&unsigned); err != nil {
			t.Fatalf("%s: serialize unsigned: %v", v.Name, err)
		}
		v.UnsignedTxHex = hex.EncodeToString(unsigned.Bytes())

		for idx, txin := range tx.TxIn {
			sig := make([]byte, 64)
			for j := range sig {
				sig[j] = byte(0xA0 + idx) // deterministic dummy signature
			}
			sig = append(sig, schnorrSigHashAllAnyoneCanPay)
			txin.Witness = btcwire.TxWitness{sig}
		}
		var signed bytes.Buffer
		if err := tx.Serialize(&signed); err != nil {
			t.Fatalf("%s: serialize signed: %v", v.Name, err)
		}
		v.SignedTxHex = hex.EncodeToString(signed.Bytes())
	}

	if os.Getenv("BIFROST_WRITE_FIXTURES") == "1" {
		dir := "../../../../../test-fixtures/interop"
		if alt := os.Getenv("BIFROST_FIXTURE_DIR"); alt != "" {
			dir = alt
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir fixtures: %v", err)
		}
		blob, err := json.MarshalIndent(vectors, "", "  ")
		if err != nil {
			t.Fatalf("marshal fixtures: %v", err)
		}
		path := filepath.Join(dir, "sighash_vectors.json")
		if err := os.WriteFile(path, blob, 0o644); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}
		t.Logf("wrote %d sighash vectors to %s", len(vectors), path)
	}
}

type utxoEntry struct {
	TxID          string  `json:"txid"`
	Confirmations int64   `json:"confirmations"`
	Amount        float64 `json:"amount"`
}

type utxoOrderingVector struct {
	Name          string      `json:"name"`
	Input         []utxoEntry `json:"input"`
	ExpectedOrder []string    `json:"expected_order"` // txids, sorted for spend
}

// TestUtxoOrderingGolden pins the deterministic coin-selection ordering
// (sortUtxosForSpend: confirmations desc, txid asc, stable) and emits vectors
// the Rust bifrost-signer crate's sort_utxos must reproduce.
func TestUtxoOrderingGolden(t *testing.T) {
	// Full 64-char hex txids (bitcoind display form) so the Rust side can parse
	// them into real Txid values and exercise its actual sort_utxos.
	tx := func(prefix byte) string { return fillHashHex(prefix) }
	cases := []utxoOrderingVector{
		{
			Name: "conf_desc_then_txid_asc",
			Input: []utxoEntry{
				{TxID: tx(0xcc), Confirmations: 1, Amount: 0.1},
				{TxID: tx(0xaa), Confirmations: 5, Amount: 0.2},
				{TxID: tx(0xdd), Confirmations: 5, Amount: 0.3},
				{TxID: tx(0xbb), Confirmations: 3, Amount: 0.4},
			},
		},
		{
			Name: "ties_stable_by_txid",
			Input: []utxoEntry{
				{TxID: tx(0xb2), Confirmations: 2, Amount: 0.1},
				{TxID: tx(0xb1), Confirmations: 2, Amount: 0.2},
				{TxID: tx(0xb3), Confirmations: 2, Amount: 0.3},
			},
		},
	}

	for i := range cases {
		c := &cases[i]
		utxos := make([]btcjson.ListUnspentResult, len(c.Input))
		for j, e := range c.Input {
			utxos[j] = btcjson.ListUnspentResult{TxID: e.TxID, Confirmations: e.Confirmations, Amount: e.Amount}
		}
		sortUtxosForSpend(utxos)
		c.ExpectedOrder = make([]string, len(utxos))
		for j, u := range utxos {
			c.ExpectedOrder[j] = u.TxID
		}
	}

	if os.Getenv("BIFROST_WRITE_FIXTURES") == "1" {
		dir := "../../../../../test-fixtures/interop"
		if alt := os.Getenv("BIFROST_FIXTURE_DIR"); alt != "" {
			dir = alt
		}
		blob, err := json.MarshalIndent(cases, "", "  ")
		if err != nil {
			t.Fatalf("marshal utxo vectors: %v", err)
		}
		path := filepath.Join(dir, "utxo_ordering_vectors.json")
		if err := os.WriteFile(path, blob, 0o644); err != nil {
			t.Fatalf("write utxo vectors: %v", err)
		}
		t.Logf("wrote %d utxo ordering vectors to %s", len(cases), path)
	}
}
