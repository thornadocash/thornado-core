//! End-to-end taproot signing: distributed DKG → derive the vault's taproot
//! address from the group key → build a BTC key-path spend → FROST keysign with
//! the BIP341 tweak → verify the aggregate schnorr signature is valid under the
//! vault's taproot output key.
//!
//! This is the crypto proof that the Rust signing path produces a signature a
//! Bitcoin node will accept: rust-bitcoin's `verify_schnorr` is the oracle.

use std::collections::{BTreeMap, HashMap};

use bitcoin::hashes::Hash;
use bitcoin::secp256k1::{schnorr, Message, Secp256k1, XOnlyPublicKey};
use bitcoin::Txid;

use thornado_bifrost_signer::frost_session::{
    normalize_participants, KeygenSession, ProtocolMessage, SignSession, StoredShare,
};
use thornado_bifrost_signer::tx_builder::{
    apply_taproot_witness, build_unsigned, taproot_sighash, BuildRequest, Recipient, TaprootVault,
    Utxo,
};

fn party_names(n: usize) -> Vec<String> {
    normalize_participants(&(0..n).map(|i| format!("v{i}")).collect::<Vec<_>>())
}

/// Pump ProtocolMessages between sessions until quiescent.
fn pump<F: FnMut(&str, &ProtocolMessage) -> Vec<ProtocolMessage>>(
    mut inflight: Vec<ProtocolMessage>,
    mut deliver: F,
) {
    for _ in 0..500 {
        if inflight.is_empty() {
            break;
        }
        let mut next = Vec::new();
        for msg in inflight.drain(..) {
            for target in msg.to.clone() {
                next.extend(deliver(&target, &msg));
            }
        }
        inflight = next;
    }
}

fn distributed_keygen(names: &[String], min: u16) -> BTreeMap<String, StoredShare> {
    let mut sessions: BTreeMap<String, KeygenSession> = BTreeMap::new();
    let mut inflight = Vec::new();
    for name in names {
        let mut s = KeygenSession::new(name.clone(), names.to_vec(), min).unwrap();
        inflight.extend(s.drain_outputs());
        sessions.insert(name.clone(), s);
    }
    pump(inflight, |target, msg| {
        let mut out = Vec::new();
        if let Some(s) = sessions.get_mut(target) {
            s.handle(msg).unwrap();
            out.extend(s.drain_outputs());
        }
        out
    });
    sessions
        .iter()
        .map(|(n, s)| (n.clone(), s.stored_share().unwrap().clone()))
        .collect()
}

fn frost_keysign_taproot(
    shares: &BTreeMap<String, StoredShare>,
    signers: &[String],
    sighash: [u8; 32],
) -> [u8; 64] {
    let mut sessions: BTreeMap<String, SignSession> = BTreeMap::new();
    let mut inflight = Vec::new();
    for name in signers {
        let mut s = SignSession::new_taproot(
            &shares[name],
            name.clone(),
            signers.to_vec(),
            sighash.to_vec(),
        )
        .unwrap();
        inflight.extend(s.drain_outputs());
        sessions.insert(name.clone(), s);
    }
    let mut sig = None;
    pump(inflight, |target, msg| {
        let mut out = Vec::new();
        if let Some(s) = sessions.get_mut(target) {
            if s.handle(msg).unwrap() {
                sig = s.signature();
            }
            out.extend(s.drain_outputs());
        }
        out
    });
    sig.expect("aggregate taproot signature produced")
}

#[test]
fn frost_taproot_keypath_signature_is_valid_for_vault_address() {
    let names = party_names(4);
    let min = 3u16;
    let shares = distributed_keygen(&names, min);

    // Vault taproot output key derived from the DKG group key (path 0).
    let group_compressed = hex::decode(&shares[&names[0]].public_key_compressed).unwrap();
    let vault = TaprootVault::derive(&group_compressed, 0).unwrap();
    let vault_spk = vault.script_pubkey();
    assert!(vault_spk.is_p2tr());

    // Build a key-path spend FROM the vault to some recipient.
    let recipient_spk = {
        let mut v = vec![0x51, 0x20];
        v.extend_from_slice(&[0xABu8; 32]);
        bitcoin::ScriptBuf::from_bytes(v)
    };
    let req = BuildRequest {
        vault: vault.clone(),
        inputs: vec![Utxo {
            txid: Txid::from_byte_array([0x11; 32]),
            vout: 0,
            amount_sats: 100_000,
            confirmations: 10,
        }],
        recipients: vec![Recipient {
            script_pubkey: recipient_spk,
            amount_sats: 90_000,
        }],
        fee_rate: 5,
        spend_all: false,
                exact_fee_remainder: false,
    };
    let mut unsigned = build_unsigned(&req).unwrap();

    // Sighash for input 0.
    let sighash = taproot_sighash(&unsigned, 0).unwrap();

    // FROST keysign (3 of 4) with the taproot tweak.
    let signers: Vec<String> = names[..min as usize].to_vec();
    let sig = frost_keysign_taproot(&shares, &signers, sighash);

    // ORACLE: rust-bitcoin verifies the schnorr signature against the vault's
    // taproot output key + sighash. If this passes, a Bitcoin node accepts it.
    let secp = Secp256k1::verification_only();
    let xonly = XOnlyPublicKey::from_slice(&vault.output_key).unwrap();
    let msg = Message::from_digest(sighash);
    let schnorr_sig = schnorr::Signature::from_slice(&sig).unwrap();
    secp.verify_schnorr(&schnorr_sig, &msg, &xonly)
        .expect("FROST taproot signature must verify under the vault output key");

    // And the witness assembles to the 65-byte element (sig || 0x81).
    apply_taproot_witness(&mut unsigned, 0, &sig).unwrap();
    let w = &unsigned.tx.input[0].witness;
    assert_eq!(w.iter().next().unwrap().len(), 65);
}

// Determinism sanity: two independent signing runs both verify (nonces differ,
// but each aggregate is valid under the same output key).
#[test]
fn taproot_signing_is_repeatable() {
    let names = party_names(3);
    let shares = distributed_keygen(&names, 2);
    let group = hex::decode(&shares[&names[0]].public_key_compressed).unwrap();
    let vault = TaprootVault::derive(&group, 0).unwrap();
    let secp = Secp256k1::verification_only();
    let xonly = XOnlyPublicKey::from_slice(&vault.output_key).unwrap();
    let signers: Vec<String> = names[..2].to_vec();
    let _ = HashMap::<u8, u8>::new(); // keep import list stable

    for round in 0..3u8 {
        let sighash = [round.wrapping_add(1); 32];
        let sig = frost_keysign_taproot(&shares, &signers, sighash);
        let msg = Message::from_digest(sighash);
        let s = schnorr::Signature::from_slice(&sig).unwrap();
        secp.verify_schnorr(&s, &msg, &xonly)
            .unwrap_or_else(|_| panic!("round {round} signature invalid"));
    }
}
