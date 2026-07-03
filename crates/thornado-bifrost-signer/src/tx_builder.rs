//! BTC transaction construction and taproot key-path signing.
//!
//! Ports the Go `utxo_signer.go` / `signer_internal.go` path onto rust-bitcoin
//! 0.32. Vaults are P2TR key-path; the FROST engine produces the 64-byte
//! Schnorr signature over the BIP341 sighash, to which we append the sighash
//! flag byte and assemble the witness.

use bitcoin::absolute::LockTime;
use bitcoin::hashes::Hash;
use bitcoin::sighash::{Prevouts, SighashCache, TapSighashType};
use bitcoin::transaction::Version;
use bitcoin::{
    Amount, OutPoint, ScriptBuf, Sequence, Transaction, TxIn, TxOut, Txid, Witness,
};

/// SIGHASH_ALL | SIGHASH_ANYONECANPAY, matching Go `schnorrSigHashAllAnyoneCanPay`.
pub const SCHNORR_SIGHASH_ALL_ACP: u8 = 0x81;
/// Non-final sequence used for all inputs (Go default).
pub const SEQUENCE: u32 = 0xffff_fffe;

#[derive(Debug, thiserror::Error)]
pub enum TxError {
    #[error("bitcoin: {0}")]
    Bitcoin(String),
    #[error("insufficient available UTXOs: need {need}, have {have}")]
    Insufficient { need: u64, have: u64 },
    #[error("output would be non-positive after fees")]
    NonPositiveOutput,
    #[error("signature length {0}, expected 64")]
    BadSignatureLen(usize),
}

type Result<T> = std::result::Result<T, TxError>;

/// A spendable UTXO under a vault.
#[derive(Debug, Clone)]
pub struct Utxo {
    pub txid: Txid,
    pub vout: u32,
    pub amount_sats: u64,
    pub confirmations: u64,
}

/// Format matching Go `formatUtxoKey`: "<txid_lowercase>-<vout>".
pub fn utxo_key(txid: &Txid, vout: u32) -> String {
    format!("{txid}-{vout}")
}

/// Stable ordering: confirmations descending (oldest first), then txid ascending.
pub fn sort_utxos(utxos: &mut [Utxo]) {
    utxos.sort_by(|a, b| {
        b.confirmations
            .cmp(&a.confirmations)
            .then_with(|| a.txid.to_string().cmp(&b.txid.to_string()))
    });
}

/// Domain separator for the thornado BIP86-style child-key tweak (matches Go
/// `vaultPathDomain`).
pub const VAULT_PATH_DOMAIN: &[u8] = b"thornado:frost-bip86-child:v1";
/// The main vault uses the base key directly (no child tweak).
pub const MAIN_VAULT_PATH_INDEX: u64 = 0;

/// A taproot vault: the 32-byte x-only output key (BIP341-tweaked and
/// path-derived), exactly what Go passes to the FROST engine.
#[derive(Debug, Clone)]
pub struct TaprootVault {
    pub output_key: [u8; 32],
}

impl TaprootVault {
    /// Construct from an already-derived 32-byte x-only output key.
    pub fn from_output_key(output_key: [u8; 32]) -> Self {
        Self { output_key }
    }

    /// Derive the vault's taproot output key from the compressed secp256k1
    /// vault pubkey and a path index, matching Go `DeriveBTCTaprootPubKey`:
    ///
    /// 1. internal key = base pubkey, or (path != 0) base + t·G where
    ///    t = SHA256(domain ‖ compressed_base ‖ big-endian(path));
    /// 2. output key = BIP341 taproot tweak of the internal key (no script tree).
    pub fn derive(compressed_pubkey: &[u8], path_index: u64) -> Result<Self> {
        use bitcoin::key::TapTweak;
        use bitcoin::secp256k1::{PublicKey, Scalar, Secp256k1};
        use sha2::{Digest, Sha256};

        let secp = Secp256k1::verification_only();
        let base = PublicKey::from_slice(compressed_pubkey)
            .map_err(|e| TxError::Bitcoin(format!("bad vault pubkey: {e}")))?;

        let internal = if path_index == MAIN_VAULT_PATH_INDEX {
            base
        } else {
            let mut h = Sha256::new();
            h.update(VAULT_PATH_DOMAIN);
            h.update(base.serialize()); // compressed, 33 bytes
            h.update(path_index.to_be_bytes());
            let digest: [u8; 32] = h.finalize().into();
            let scalar = Scalar::from_be_bytes(digest)
                .map_err(|e| TxError::Bitcoin(format!("child tweak scalar: {e}")))?;
            base.add_exp_tweak(&secp, &scalar)
                .map_err(|e| TxError::Bitcoin(format!("child tweak add: {e}")))?
        };

        let (xonly, _parity) = internal.x_only_public_key();
        let (tweaked, _p) = xonly.tap_tweak(&secp, None);
        Ok(Self {
            output_key: tweaked.to_x_only_public_key().serialize(),
        })
    }

    /// The scriptPubKey for this vault: `OP_1 <32-byte key>` (34 bytes).
    pub fn script_pubkey(&self) -> ScriptBuf {
        let mut v = Vec::with_capacity(34);
        v.push(0x51); // OP_1 (witness v1)
        v.push(0x20); // push 32
        v.extend_from_slice(&self.output_key);
        ScriptBuf::from_bytes(v)
    }
}

/// A single recipient output.
#[derive(Debug, Clone)]
pub struct Recipient {
    pub script_pubkey: ScriptBuf,
    pub amount_sats: u64,
}

/// Inputs to building one (possibly batched) outbound transaction.
pub struct BuildRequest {
    pub vault: TaprootVault,
    pub inputs: Vec<Utxo>,
    /// One output per batched TxOutItem.
    pub recipients: Vec<Recipient>,
    /// sats/vbyte, already resolved and capped by the caller.
    pub fee_rate: u64,
    /// If true (sweep/migrate/consolidate), spend all inputs; else send exact
    /// recipient amounts plus a change output.
    pub spend_all: bool,
    /// Internal txouts (migrate) with prescribed inputs: pay recipients exactly
    /// and burn the full remainder (the chain's MaxGas allocation) as fee, no
    /// change output. The chain's internal-outbound matcher requires
    /// amount+gas == coin+max_gas exactly, which only holds with this shape.
    pub exact_fee_remainder: bool,
}

/// Below this, change is folded into the fee instead of creating an output
/// (Go bifrost behavior; also keeps internal-outbound matching exact when the
/// chain's fee estimate differs slightly from ours).
pub const DUST_CHANGE_SATS: u64 = 546;

/// An unsigned transaction plus the per-input prevout amounts needed for the
/// BIP341 sighash (this is the checkpoint content in Go).
pub struct UnsignedTx {
    pub tx: Transaction,
    pub prevouts: Vec<TxOut>,
}

/// Build the unsigned transaction (inputs, recipient outputs, change).
pub fn build_unsigned(req: &BuildRequest) -> Result<UnsignedTx> {
    let total_in: u64 = req.inputs.iter().map(|u| u.amount_sats).sum();
    let vault_spk = req.vault.script_pubkey();

    let tx_in: Vec<TxIn> = req
        .inputs
        .iter()
        .map(|u| TxIn {
            previous_output: OutPoint::new(u.txid, u.vout),
            script_sig: ScriptBuf::new(),
            sequence: Sequence(SEQUENCE),
            witness: Witness::new(),
        })
        .collect();

    let prevouts: Vec<TxOut> = req
        .inputs
        .iter()
        .map(|u| TxOut {
            value: Amount::from_sat(u.amount_sats),
            script_pubkey: vault_spk.clone(),
        })
        .collect();

    let mut tx_out: Vec<TxOut> = req
        .recipients
        .iter()
        .map(|r| TxOut {
            value: Amount::from_sat(r.amount_sats),
            script_pubkey: r.script_pubkey.clone(),
        })
        .collect();

    // Fee via vsize estimate (accounting for a possible change output).
    let est_vsize = estimate_vsize(tx_in.len(), tx_out.len() + 1);
    let fee = req.fee_rate.saturating_mul(est_vsize);

    if req.exact_fee_remainder {
        let recipient_total: u64 = req.recipients.iter().map(|r| r.amount_sats).sum();
        if total_in <= recipient_total {
            return Err(TxError::Insufficient {
                need: recipient_total + 1,
                have: total_in,
            });
        }
    } else if req.spend_all {
        let recipient_total: u64 = req.recipients.iter().map(|r| r.amount_sats).sum();
        // Sweep semantics: single recipient gets everything minus fee.
        let payout = total_in
            .checked_sub(fee)
            .ok_or(TxError::NonPositiveOutput)?;
        if payout == 0 {
            return Err(TxError::NonPositiveOutput);
        }
        // For sweeps the caller supplies exactly one recipient placeholder.
        let _ = recipient_total;
        if let Some(first) = tx_out.first_mut() {
            first.value = Amount::from_sat(payout);
        }
    } else {
        let recipient_total: u64 = req.recipients.iter().map(|r| r.amount_sats).sum();
        let need = recipient_total + fee;
        if total_in < need {
            return Err(TxError::Insufficient {
                need,
                have: total_in,
            });
        }
        let change = total_in - recipient_total - fee;
        if change >= DUST_CHANGE_SATS {
            tx_out.push(TxOut {
                value: Amount::from_sat(change),
                script_pubkey: vault_spk.clone(),
            });
        }
    }

    let tx = Transaction {
        version: Version::ONE,
        lock_time: LockTime::ZERO,
        input: tx_in,
        output: tx_out,
    };

    Ok(UnsignedTx { tx, prevouts })
}

/// Compute the BIP341 taproot key-spend sighash for input `index` using
/// SIGHASH_ALL|ANYONECANPAY (0x81), matching the Go digest.
pub fn taproot_sighash(unsigned: &UnsignedTx, index: usize) -> Result<[u8; 32]> {
    let mut cache = SighashCache::new(&unsigned.tx);
    // ANYONECANPAY commits only to this input's prevout.
    let prevout = &unsigned.prevouts[index];
    let sighash = cache
        .taproot_key_spend_signature_hash(
            index,
            &Prevouts::One(index, prevout.clone()),
            TapSighashType::AllPlusAnyoneCanPay,
        )
        .map_err(|e| TxError::Bitcoin(e.to_string()))?;
    Ok(sighash.to_byte_array())
}

/// Assemble the witness for a taproot key-path input from a 64-byte FROST
/// Schnorr signature: append the 0x81 sighash flag → 65-byte witness element.
pub fn apply_taproot_witness(
    unsigned: &mut UnsignedTx,
    index: usize,
    frost_sig_64: &[u8],
) -> Result<()> {
    if frost_sig_64.len() != 64 {
        return Err(TxError::BadSignatureLen(frost_sig_64.len()));
    }
    let mut sig = Vec::with_capacity(65);
    sig.extend_from_slice(frost_sig_64);
    sig.push(SCHNORR_SIGHASH_ALL_ACP);

    let mut witness = Witness::new();
    witness.push(sig);
    unsigned.tx.input[index].witness = witness;
    Ok(())
}

/// BIP141 vsize estimate for `n_in` taproot key-path inputs and `n_out`
/// outputs. Witness per input: one 65-byte element.
pub fn estimate_vsize(n_in: usize, n_out: usize) -> u64 {
    // Non-witness (stripped) bytes.
    let base = 4  // version
        + varint_len(n_in as u64)
        + n_in * (32 + 4 + 1 + 4) // outpoint + empty scriptSig len + sequence
        + varint_len(n_out as u64)
        + n_out * (8 + 1 + 34) // value + spk len + p2tr spk
        + 4; // locktime
    // Witness bytes: marker+flag (2) + per-input [items=1][len=65][65].
    let witness = 2 + n_in * (1 + 1 + 65);
    let total = base + witness;
    // weight = base*3 + total; vsize = ceil(weight/4)
    let weight = base * 3 + total;
    weight.div_ceil(4) as u64
}

fn varint_len(n: u64) -> usize {
    match n {
        0..=0xfc => 1,
        0xfd..=0xffff => 3,
        0x1_0000..=0xffff_ffff => 5,
        _ => 9,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bitcoin::hashes::Hash;

    fn dummy_txid(byte: u8) -> Txid {
        Txid::from_byte_array([byte; 32])
    }

    fn vault() -> TaprootVault {
        TaprootVault {
            output_key: [7u8; 32],
        }
    }

    #[test]
    fn vault_script_is_p2tr_34_bytes() {
        let spk = vault().script_pubkey();
        let b = spk.as_bytes();
        assert_eq!(b.len(), 34);
        assert_eq!(b[0], 0x51);
        assert_eq!(b[1], 0x20);
        assert!(spk.is_p2tr());
    }

    #[test]
    fn utxo_sort_is_stable_by_conf_then_txid() {
        let mut u = vec![
            Utxo { txid: dummy_txid(2), vout: 0, amount_sats: 10, confirmations: 1 },
            Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 10, confirmations: 5 },
            Utxo { txid: dummy_txid(3), vout: 0, amount_sats: 10, confirmations: 5 },
        ];
        sort_utxos(&mut u);
        assert_eq!(u[0].confirmations, 5);
        assert_eq!(u[0].txid, dummy_txid(1)); // lower txid first at same conf
        assert_eq!(u[2].confirmations, 1);
    }

    #[test]
    fn build_with_change_and_sighash_and_witness() {
        let recipient = ScriptBuf::from_bytes({
            let mut v = vec![0x51, 0x20];
            v.extend_from_slice(&[9u8; 32]);
            v
        });
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![
                Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 100_000, confirmations: 6 },
                Utxo { txid: dummy_txid(2), vout: 1, amount_sats: 50_000, confirmations: 6 },
            ],
            recipients: vec![Recipient { script_pubkey: recipient, amount_sats: 120_000 }],
            fee_rate: 10,
            spend_all: false,
                exact_fee_remainder: false,
        };
        let mut unsigned = build_unsigned(&req).unwrap();
        // recipient + change
        assert_eq!(unsigned.tx.output.len(), 2);
        assert!(unsigned.tx.output[1].script_pubkey.is_p2tr()); // change to vault

        // sighash is deterministic 32 bytes for each input
        let h0 = taproot_sighash(&unsigned, 0).unwrap();
        let h1 = taproot_sighash(&unsigned, 1).unwrap();
        assert_ne!(h0, h1); // ANYONECANPAY commits distinct prevouts

        // witness assembly: 64-byte sig -> 65-byte element
        apply_taproot_witness(&mut unsigned, 0, &[1u8; 64]).unwrap();
        let w = &unsigned.tx.input[0].witness;
        assert_eq!(w.len(), 1);
        assert_eq!(w.iter().next().unwrap().len(), 65);
        assert_eq!(*w.iter().next().unwrap().last().unwrap(), 0x81);

        // wrong length rejected
        assert!(apply_taproot_witness(&mut unsigned, 1, &[0u8; 63]).is_err());
    }

    #[test]
    fn insufficient_funds_detected() {
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 1000, confirmations: 6 }],
            recipients: vec![Recipient {
                script_pubkey: vault().script_pubkey(),
                amount_sats: 5000,
            }],
            fee_rate: 10,
            spend_all: false,
                exact_fee_remainder: false,
        };
        assert!(matches!(build_unsigned(&req), Err(TxError::Insufficient { .. })));
    }

    /// The migrate shape the chain's internal matcher requires: exact recipient
    /// amount, remainder (= the chain's MaxGas) burned as fee, NO change even
    /// when the remainder exceeds dust.
    #[test]
    fn exact_fee_remainder_burns_max_gas_no_change() {
        let recipient = ScriptBuf::from_bytes({
            let mut v = vec![0x51, 0x20];
            v.extend_from_slice(&[9u8; 32]);
            v
        });
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 70_000_000, confirmations: 6 }],
            recipients: vec![Recipient { script_pubkey: recipient, amount_sats: 69_994_225 }],
            fee_rate: 35,
            spend_all: false,
            exact_fee_remainder: true,
        };
        let unsigned = build_unsigned(&req).unwrap();
        assert_eq!(unsigned.tx.output.len(), 1);
        assert_eq!(unsigned.tx.output[0].value.to_sat(), 69_994_225);
        let fee: u64 = 70_000_000 - 69_994_225;
        let out_total: u64 = unsigned.tx.output.iter().map(|o| o.value.to_sat()).sum();
        assert_eq!(70_000_000 - out_total, fee); // remainder = 5775 = MaxGas
    }

    #[test]
    fn exact_fee_remainder_rejects_zero_fee() {
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 1000, confirmations: 6 }],
            recipients: vec![Recipient {
                script_pubkey: vault().script_pubkey(),
                amount_sats: 1000,
            }],
            fee_rate: 1,
            spend_all: false,
            exact_fee_remainder: true,
        };
        assert!(matches!(build_unsigned(&req), Err(TxError::Insufficient { .. })));
    }

    /// Sub-dust change is folded into the fee (Go bifrost behavior) instead of
    /// creating an unmatched 350-sat output.
    #[test]
    fn dust_change_folds_into_fee() {
        let recipient = ScriptBuf::from_bytes({
            let mut v = vec![0x51, 0x20];
            v.extend_from_slice(&[7u8; 32]);
            v
        });
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 100_000, confirmations: 6 }],
            recipients: vec![Recipient { script_pubkey: recipient.clone(), amount_sats: 98_000 }],
            fee_rate: 10,
            spend_all: false,
            exact_fee_remainder: false,
        };
        // est fee = 10 * vsize(1 in, 2 out); change = 2000 - fee < 546 → folded.
        let unsigned = build_unsigned(&req).unwrap();
        assert_eq!(unsigned.tx.output.len(), 1);
        assert_eq!(unsigned.tx.output[0].value.to_sat(), 98_000);
    }

    #[test]
    fn sweep_spends_all_minus_fee() {
        let req = BuildRequest {
            vault: vault(),
            inputs: vec![Utxo { txid: dummy_txid(1), vout: 0, amount_sats: 100_000, confirmations: 6 }],
            recipients: vec![Recipient {
                script_pubkey: vault().script_pubkey(),
                amount_sats: 0,
            }],
            fee_rate: 10,
            spend_all: true,
                exact_fee_remainder: false,
        };
        let unsigned = build_unsigned(&req).unwrap();
        assert_eq!(unsigned.tx.output.len(), 1);
        assert!(unsigned.tx.output[0].value.to_sat() < 100_000);
        assert!(unsigned.tx.output[0].value.to_sat() > 90_000);
    }
}
