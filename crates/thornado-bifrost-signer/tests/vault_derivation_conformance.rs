//! Vault taproot key/script derivation conformance.
//!
//! Reads vectors emitted by the Go `DeriveBTCTaprootPubKey` /
//! `DeriveBTCBIP86InternalPubKey` and asserts the Rust crate derives the
//! identical output x-only key and scriptPubKey from the same compressed vault
//! pubkey + path index. rust-bitcoin's BIP341/BIP86 is the oracle; a mismatch
//! is a Go bug, never a Rust workaround.

use serde::Deserialize;

use thornado_bifrost_signer::tx_builder::TaprootVault;

#[derive(Debug, Deserialize)]
struct DerivationVector {
    compressed_pubkey_hex: String,
    path_index: u64,
    output_xonly_hex: String,
    script_hex: String,
}

fn fixture_path() -> String {
    format!(
        "{}/../../test-fixtures/interop/vault_derivation_vectors.json",
        env!("CARGO_MANIFEST_DIR")
    )
}

#[test]
fn vault_derivation_matches_go() {
    let raw = std::fs::read(fixture_path()).expect("read vault_derivation_vectors.json");
    let vectors: Vec<DerivationVector> = serde_json::from_slice(&raw).unwrap();
    assert!(!vectors.is_empty());

    for v in &vectors {
        let pubkey = hex::decode(&v.compressed_pubkey_hex).unwrap();
        let vault = TaprootVault::derive(&pubkey, v.path_index)
            .unwrap_or_else(|e| panic!("path {}: derive failed: {e}", v.path_index));

        assert_eq!(
            hex::encode(vault.output_key),
            v.output_xonly_hex,
            "path {}: output x-only key diverges from Go",
            v.path_index
        );
        assert_eq!(
            hex::encode(vault.script_pubkey().as_bytes()),
            v.script_hex,
            "path {}: scriptPubKey diverges from Go",
            v.path_index
        );
    }
}
