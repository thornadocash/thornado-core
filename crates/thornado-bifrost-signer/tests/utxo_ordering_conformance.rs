//! UTXO selection-ordering conformance.
//!
//! Reads vectors emitted by the Go bifrost's `sortUtxosForSpend`
//! (confirmations desc, txid asc, stable) and asserts the Rust crate's real
//! `sort_utxos` produces the identical order. Both sides sort by the txid's
//! display (big-endian) hex, so the orderings must agree.

use std::str::FromStr;

use bitcoin::Txid;
use serde::Deserialize;

use thornado_bifrost_signer::tx_builder::{sort_utxos, Utxo};

#[derive(Debug, Deserialize)]
struct UtxoEntry {
    txid: String,
    confirmations: i64,
    amount: f64,
}

#[derive(Debug, Deserialize)]
struct OrderingVector {
    name: String,
    input: Vec<UtxoEntry>,
    expected_order: Vec<String>,
}

fn fixture_path() -> String {
    format!(
        "{}/../../test-fixtures/interop/utxo_ordering_vectors.json",
        env!("CARGO_MANIFEST_DIR")
    )
}

#[test]
fn sort_utxos_matches_go_selection_order() {
    let raw = std::fs::read(fixture_path()).expect("read utxo_ordering_vectors.json");
    let vectors: Vec<OrderingVector> = serde_json::from_slice(&raw).unwrap();
    assert!(!vectors.is_empty());

    for v in &vectors {
        let mut utxos: Vec<Utxo> = v
            .input
            .iter()
            .map(|e| Utxo {
                path_index: 0,
                txid: Txid::from_str(&e.txid).expect("valid txid hex"),
                vout: 0,
                amount_sats: (e.amount * 100_000_000.0).round() as u64,
                confirmations: e.confirmations as u64,
            })
            .collect();

        sort_utxos(&mut utxos);

        let got: Vec<String> = utxos.iter().map(|u| u.txid.to_string()).collect();
        assert_eq!(
            got, v.expected_order,
            "vector `{}`: Rust sort_utxos order diverges from Go",
            v.name
        );
    }
}
