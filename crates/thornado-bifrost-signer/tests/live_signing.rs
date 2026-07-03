//! Live signing pressure test: a Rust FROST party signs and BROADCASTS real BTC
//! taproot withdrawals to a bitcoind, in a loop, under load.
//!
//! Gated behind env `BITCOIND_RPC=host:port` (+ optional BITCOIND_USER/PASS).
//! Skips cleanly when unset, so `cargo test` stays hermetic. Run on a host with
//! a regtest bitcoind (e.g. the isolated node on the cluster):
//!   BITCOIND_RPC=127.0.0.1:24700 BITCOIND_USER=test BITCOIND_PASS=test \
//!   ITERS=50 cargo test -p thornado-bifrost-signer --test live_signing -- --ignored --nocapture

use std::collections::BTreeMap;

use bitcoin::hashes::Hash;
use bitcoin::{Network, Txid};
use serde_json::Value;

use thornado_bifrost_signer::frost_session::{
    normalize_participants, KeygenSession, ProtocolMessage, SignSession, StoredShare,
};
use thornado_bifrost_signer::tx_builder::{
    apply_taproot_witness, build_unsigned, taproot_sighash, BuildRequest, Recipient, TaprootVault,
    Utxo,
};

struct Rpc {
    url: String,
    user: String,
    pass: String,
    http: reqwest::blocking::Client,
}

impl Rpc {
    fn from_env() -> Option<Self> {
        let host = std::env::var("BITCOIND_RPC").ok()?;
        Some(Self {
            url: format!("http://{host}/"),
            user: std::env::var("BITCOIND_USER").unwrap_or_else(|_| "test".into()),
            pass: std::env::var("BITCOIND_PASS").unwrap_or_else(|_| "test".into()),
            http: reqwest::blocking::Client::new(),
        })
    }
    fn call(&self, wallet: Option<&str>, method: &str, params: Value) -> Value {
        let url = match wallet {
            Some(w) => format!("{}wallet/{w}", self.url),
            None => self.url.clone(),
        };
        let body = serde_json::json!({"jsonrpc":"1.0","id":"t","method":method,"params":params});
        let v: Value = self
            .http
            .post(&url)
            .basic_auth(&self.user, Some(&self.pass))
            .json(&body)
            .send()
            .unwrap()
            .json()
            .unwrap();
        if !v["error"].is_null() {
            panic!("rpc {method} error: {}", v["error"]);
        }
        v["result"].clone()
    }
}

fn pump<F: FnMut(&str, &ProtocolMessage) -> Vec<ProtocolMessage>>(
    mut inflight: Vec<ProtocolMessage>,
    mut deliver: F,
) {
    for _ in 0..800 {
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

fn dkg(names: &[String], min: u16) -> BTreeMap<String, StoredShare> {
    let mut s: BTreeMap<String, KeygenSession> = BTreeMap::new();
    let mut inflight = Vec::new();
    for n in names {
        let mut k = KeygenSession::new(n.clone(), names.to_vec(), min).unwrap();
        inflight.extend(k.drain_outputs());
        s.insert(n.clone(), k);
    }
    pump(inflight, |t, m| {
        let mut o = Vec::new();
        if let Some(k) = s.get_mut(t) {
            k.handle(m).unwrap();
            o.extend(k.drain_outputs());
        }
        o
    });
    s.iter()
        .map(|(n, k)| (n.clone(), k.stored_share().unwrap().clone()))
        .collect()
}

fn keysign(shares: &BTreeMap<String, StoredShare>, signers: &[String], sighash: [u8; 32]) -> [u8; 64] {
    let mut s: BTreeMap<String, SignSession> = BTreeMap::new();
    let mut inflight = Vec::new();
    for n in signers {
        let mut k =
            SignSession::new_taproot(&shares[n], n.clone(), signers.to_vec(), sighash.to_vec())
                .unwrap();
        inflight.extend(k.drain_outputs());
        s.insert(n.clone(), k);
    }
    let mut sig = None;
    pump(inflight, |t, m| {
        let mut o = Vec::new();
        if let Some(k) = s.get_mut(t) {
            if k.handle(m).unwrap() {
                sig = k.signature();
            }
            o.extend(k.drain_outputs());
        }
        o
    });
    sig.expect("signature")
}

#[ignore = "needs a live regtest bitcoind; set BITCOIND_RPC"]
#[test]
fn party_signs_and_broadcasts_withdrawals_under_load() {
    let Some(rpc) = Rpc::from_env() else {
        eprintln!("BITCOIND_RPC unset — skipping live signing test");
        return;
    };
    let iters: usize = std::env::var("ITERS").ok().and_then(|s| s.parse().ok()).unwrap_or(30);
    let n = 4usize;
    let min = 3u16;
    let names = normalize_participants(&(0..n).map(|i| format!("v{i}")).collect::<Vec<_>>());
    let signers: Vec<String> = names[..min as usize].to_vec();

    // Wallet for funding + mining.
    let w = "signtest";
    let _ = rpc.call(None, "createwallet", serde_json::json!([w]));
    let miner: String = serde_json::from_value(rpc.call(Some(w), "getnewaddress", serde_json::json!([]))).unwrap();
    rpc.call(Some(w), "generatetoaddress", serde_json::json!([101, miner]));

    // DKG the vault, derive its taproot address.
    let shares = dkg(&names, min);
    let group = hex::decode(&shares[&names[0]].public_key_compressed).unwrap();
    let vault = TaprootVault::derive(&group, 0).unwrap();
    let vault_addr = bitcoin::Address::from_script(vault.script_pubkey().as_script(), Network::Regtest)
        .unwrap()
        .to_string();
    eprintln!("vault taproot address: {vault_addr}");

    let start = std::time::Instant::now();
    let mut accepted = 0usize;
    for i in 0..iters {
        // Fund the vault with one output, mine it.
        let fund_txid: String = serde_json::from_value(
            rpc.call(Some(w), "sendtoaddress", serde_json::json!([vault_addr, 0.01])),
        )
        .unwrap();
        rpc.call(Some(w), "generatetoaddress", serde_json::json!([1, miner]));

        // Find the vault vout in that tx.
        let raw = rpc.call(None, "getrawtransaction", serde_json::json!([fund_txid, true]));
        let (vout, amount_sats) = raw["vout"]
            .as_array()
            .unwrap()
            .iter()
            .find_map(|o| {
                let spk = &o["scriptPubKey"];
                let a = spk["address"].as_str().unwrap_or("");
                if a == vault_addr {
                    let sats = (o["value"].as_f64().unwrap() * 1e8).round() as u64;
                    Some((o["n"].as_u64().unwrap() as u32, sats))
                } else {
                    None
                }
            })
            .expect("vault output present");

        // Build a sweep of that UTXO back to the vault (spend_all - fee).
        let mut txid_bytes = hex::decode(&fund_txid).unwrap();
        txid_bytes.reverse(); // display hex -> internal
        let req = BuildRequest {
            vault: vault.clone(),
            inputs: vec![Utxo {
                txid: Txid::from_slice(&txid_bytes).unwrap(),
                vout,
                amount_sats,
                confirmations: 1,
            }],
            recipients: vec![Recipient {
                script_pubkey: vault.script_pubkey(),
                amount_sats: 0,
            }],
            fee_rate: 5,
            spend_all: true,
                exact_fee_remainder: false,
        };
        let mut unsigned = build_unsigned(&req).unwrap();
        let sighash = taproot_sighash(&unsigned, 0).unwrap();

        // FROST keysign (threshold) with taproot tweak, assemble witness.
        let sig = keysign(&shares, &signers, sighash);
        apply_taproot_witness(&mut unsigned, 0, &sig).unwrap();
        let tx_hex = hex::encode(bitcoin::consensus::encode::serialize(&unsigned.tx));

        // Broadcast — bitcoind is the ORACLE. Acceptance proves the signature.
        let res = rpc.call(None, "sendrawtransaction", serde_json::json!([tx_hex]));
        let txid = res.as_str().expect("sendrawtransaction returns txid");
        assert_eq!(txid.len(), 64);
        rpc.call(Some(w), "generatetoaddress", serde_json::json!([1, miner]));
        accepted += 1;
        if i % 10 == 0 {
            eprintln!("iter {i}: broadcast+accepted, txid {}", &txid[..12]);
        }
    }
    let elapsed = start.elapsed();
    eprintln!(
        "LIVE SIGNING: {accepted}/{iters} withdrawals signed + broadcast + accepted by bitcoind in {:.1}s ({:.0}ms/withdrawal)",
        elapsed.as_secs_f64(),
        elapsed.as_secs_f64() * 1000.0 / iters as f64
    );
    assert_eq!(accepted, iters, "every withdrawal must be accepted by bitcoind");
}
