//! Sighash conformance: rust-bitcoin (BIP341) is the ORACLE.
//!
//! Reads the vectors emitted by the Go bifrost's hand-rolled
//! `taprootKeySpendSigHash` (go-thornado/cmd/.. sighash_conformance_test.go) and
//! recomputes each through rust-bitcoin's `SighashCache`. Agreement proves the
//! Go implementation is a correct BIP341 SIGHASH_ALL|ANYONECANPAY sighash. A
//! mismatch is a Go bug — the Rust port stays on rust-bitcoin, never bent to
//! reproduce a Go defect.

use bitcoin::absolute::LockTime;
use bitcoin::hashes::Hash;
use bitcoin::sighash::{Prevouts, SighashCache, TapSighashType};
use bitcoin::transaction::Version;
use bitcoin::{Amount, OutPoint, ScriptBuf, Sequence, Transaction, TxIn, TxOut, Txid, Witness};
use serde::Deserialize;

#[derive(Debug, Deserialize)]
struct VecInput {
    txid: String,
    vout: u32,
    sequence: u32,
}

#[derive(Debug, Deserialize)]
struct VecOutput {
    value_sats: i64,
    script_hex: String,
}

#[derive(Debug, Deserialize)]
struct SighashVector {
    name: String,
    version: i32,
    lock_time: u32,
    inputs: Vec<VecInput>,
    outputs: Vec<VecOutput>,
    input_index: usize,
    amount_sats: i64,
    source_script_hex: String,
    sighash_hex: String,
    unsigned_tx_hex: String,
    signed_tx_hex: String,
}

fn fixture_path() -> String {
    format!(
        "{}/../../test-fixtures/interop/sighash_vectors.json",
        env!("CARGO_MANIFEST_DIR")
    )
}

fn txid_from_hex(s: &str) -> Txid {
    // Go serializes the internal (little-endian) hash bytes as hex; rebuild the
    // Txid from those raw bytes directly so the outpoint matches Go's tx.
    let raw = hex::decode(s).unwrap();
    let mut arr = [0u8; 32];
    arr.copy_from_slice(&raw);
    Txid::from_byte_array(arr)
}

fn build_tx(v: &SighashVector) -> Transaction {
    let input = v
        .inputs
        .iter()
        .map(|i| TxIn {
            previous_output: OutPoint::new(txid_from_hex(&i.txid), i.vout),
            script_sig: ScriptBuf::new(),
            sequence: Sequence(i.sequence),
            witness: Witness::new(),
        })
        .collect();
    let output = v
        .outputs
        .iter()
        .map(|o| TxOut {
            value: Amount::from_sat(o.value_sats as u64),
            script_pubkey: ScriptBuf::from_bytes(hex::decode(&o.script_hex).unwrap()),
        })
        .collect();
    Transaction {
        version: Version(v.version),
        lock_time: LockTime::from_consensus(v.lock_time),
        input,
        output,
    }
}

#[test]
fn go_sighash_matches_rust_bitcoin_oracle() {
    let raw = std::fs::read(fixture_path()).expect("read sighash_vectors.json");
    let vectors: Vec<SighashVector> = serde_json::from_slice(&raw).unwrap();
    assert!(!vectors.is_empty(), "no vectors in fixture");

    for v in &vectors {
        let tx = build_tx(v);
        let source_script =
            ScriptBuf::from_bytes(hex::decode(&v.source_script_hex).unwrap());
        let prevout = TxOut {
            value: Amount::from_sat(v.amount_sats as u64),
            script_pubkey: source_script,
        };

        // The oracle: rust-bitcoin BIP341 key-spend sighash, ALL|ANYONECANPAY.
        let mut cache = SighashCache::new(&tx);
        let oracle = cache
            .taproot_key_spend_signature_hash(
                v.input_index,
                &Prevouts::One(v.input_index, prevout),
                TapSighashType::AllPlusAnyoneCanPay,
            )
            .expect("rust-bitcoin sighash");
        let oracle_hex = hex::encode(oracle.to_byte_array());

        assert_eq!(
            v.sighash_hex, oracle_hex,
            "vector `{}`: Go sighash {} disagrees with rust-bitcoin {} — Go is the bug",
            v.name, v.sighash_hex, oracle_hex
        );
    }
}

/// Wire-serialization conformance: the tx we construct must consensus-encode
/// to the exact bytes Go's btcwire produces, unsigned and witness-applied.
#[test]
fn tx_serialization_matches_go_wire_bytes() {
    let raw = std::fs::read(fixture_path()).expect("read sighash_vectors.json");
    let vectors: Vec<SighashVector> = serde_json::from_slice(&raw).unwrap();

    for v in &vectors {
        let mut tx = build_tx(v);

        let unsigned = bitcoin::consensus::encode::serialize(&tx);
        assert_eq!(
            hex::encode(&unsigned),
            v.unsigned_tx_hex,
            "vector `{}`: unsigned tx bytes diverge from Go",
            v.name
        );

        // Witness assembly exactly as both sides do it: 64-byte sig || 0x81 as
        // the single witness element per input (dummy sig fill 0xA0 + index).
        for (idx, input) in tx.input.iter_mut().enumerate() {
            let mut sig = vec![0xA0u8 + idx as u8; 64];
            sig.push(0x81);
            let mut w = Witness::new();
            w.push(sig);
            input.witness = w;
        }
        let signed = bitcoin::consensus::encode::serialize(&tx);
        assert_eq!(
            hex::encode(&signed),
            v.signed_tx_hex,
            "vector `{}`: witness-applied tx bytes diverge from Go",
            v.name
        );
    }
}
