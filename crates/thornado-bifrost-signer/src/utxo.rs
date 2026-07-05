//! Runtime UTXO sourcing for the signer (Go `getUtxoToSpend`).
//!
//! Outbounds prescribed by the chain carry `source_inputs` (the exact UTXOs
//! every signer must spend) — those win, so all parties build the identical
//! transaction. When absent (single unbatched outbound), fall back to live
//! `listunspent` selection: filter spendable/confirmed/unspent, order with the
//! stable [`crate::tx_builder::sort_utxos`], and take inputs until the target
//! plus fee is covered.

use std::str::FromStr;

use bitcoin::Txid;

use crate::bitcoind::ListUnspentItem;
use crate::chain::TxOutItem;
use crate::extract::btc_to_sats;
use crate::tx_builder::{estimate_vsize, sort_utxos, utxo_key, TxError, Utxo};

/// Convert `listunspent` rows into signer UTXOs, skipping unparseable txids.
pub fn to_utxos(items: &[ListUnspentItem]) -> Vec<Utxo> {
    items
        .iter()
        .filter_map(|it| {
            let txid = Txid::from_str(&it.txid).ok()?;
            Some(Utxo {
                txid,
                vout: it.vout,
                amount_sats: btc_to_sats(it.amount),
                confirmations: it.confirmations.max(0) as u64,
                path_index: 0,
            })
        })
        .collect()
}

/// The union of the batch's prescribed spend inputs, deduped and in a
/// deterministic order (txid, vout). `None` when nothing is prescribed.
pub fn prescribed_inputs(batch: &[TxOutItem]) -> Option<Vec<Utxo>> {
    let mut seen = std::collections::BTreeMap::new();
    for item in batch {
        for input in &item.source_inputs {
            let txid = Txid::from_str(&input.tx_id).ok()?;
            seen.insert((input.tx_id.clone(), input.vout), Utxo {
                txid,
                vout: input.vout,
                amount_sats: input.amount_sats,
                confirmations: 0,
                path_index: input.path_index,
            });
        }
    }
    if seen.is_empty() {
        return None;
    }
    Some(seen.into_values().collect())
}

/// Select UTXOs covering `recipients_total` plus the estimated fee for a
/// transaction with `n_out` recipient outputs (+1 change), at `fee_rate`
/// sats/vbyte. Candidates below `min_conf` confirmations or already recorded
/// as spent are excluded. (`spendable` is NOT required: vault addresses live
/// in watch-only wallets, where bitcoind reports everything unspendable.)
pub fn select_utxos(
    candidates: Vec<Utxo>,
    recipients_total: u64,
    fee_rate: u64,
    n_out: usize,
    min_conf: u64,
    is_spent: impl Fn(&str) -> bool,
) -> Result<Vec<Utxo>, TxError> {
    let mut eligible: Vec<Utxo> = candidates
        .into_iter()
        .filter(|u| u.confirmations >= min_conf && !is_spent(&utxo_key(&u.txid, u.vout)))
        .collect();
    sort_utxos(&mut eligible);

    let mut chosen = Vec::new();
    let mut total = 0u64;
    for u in eligible {
        total += u.amount_sats;
        chosen.push(u);
        let fee = fee_rate.saturating_mul(estimate_vsize(chosen.len(), n_out + 1));
        if total >= recipients_total.saturating_add(fee) {
            return Ok(chosen);
        }
    }
    let final_fee = fee_rate.saturating_mul(estimate_vsize(chosen.len().max(1), n_out + 1));
    Err(TxError::Insufficient {
        need: recipients_total.saturating_add(final_fee),
        have: total,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chain::TxOutInput;

    fn txid_hex(byte: u8) -> String {
        hex::encode([byte; 32])
    }

    fn unspent(byte: u8, vout: u32, btc: f64, conf: i64, spendable: bool) -> ListUnspentItem {
        ListUnspentItem {
            txid: txid_hex(byte),
            vout,
            address: "bcrt1q".into(),
            script_pubkey: "5120aa".into(),
            amount: btc,
            confirmations: conf,
            spendable,
        }
    }

    #[test]
    fn converts_listunspent_rows() {
        let rows = vec![unspent(1, 0, 0.5, 6, true), unspent(2, 1, 0.001, 0, false)];
        let utxos = to_utxos(&rows);
        assert_eq!(utxos.len(), 2);
        assert_eq!(utxos[0].amount_sats, 50_000_000);
        assert_eq!(utxos[1].amount_sats, 100_000);
        assert_eq!(utxos[0].confirmations, 6);
    }

    #[test]
    fn skips_bad_txids() {
        let mut row = unspent(1, 0, 0.5, 6, true);
        row.txid = "not-hex".into();
        assert!(to_utxos(&[row]).is_empty());
    }

    #[test]
    fn prescribed_inputs_dedupe_and_order() {
        let mk = |b: u8, vout: u32, sats: u64| TxOutInput {
            path_index: 0,
            tx_id: txid_hex(b),
            vout,
            amount_sats: sats,
        };
        let a = TxOutItem {
            source_inputs: vec![mk(9, 0, 100), mk(1, 1, 200)],
            ..Default::default()
        };
        let b = TxOutItem {
            source_inputs: vec![mk(9, 0, 100)], // duplicate of a's first
            ..Default::default()
        };
        let inputs = prescribed_inputs(&[a, b]).unwrap();
        assert_eq!(inputs.len(), 2);
        // deterministic order: sorted by (tx_id, vout)
        assert_eq!(inputs[0].txid.to_string(), txid_hex(1));
        assert_eq!(inputs[1].txid.to_string(), txid_hex(9));
    }

    #[test]
    fn prescribed_inputs_none_when_absent() {
        let item = TxOutItem::default();
        assert!(prescribed_inputs(&[item]).is_none());
    }

    #[test]
    fn selection_prefers_confirmed_and_covers_fee() {
        let utxos = to_utxos(&[
            unspent(3, 0, 0.001, 1, true),  // 100k sats, newest
            unspent(1, 0, 0.002, 10, true), // 200k sats, oldest
            unspent(2, 0, 0.001, 5, true),  // 100k sats
        ]);
        let chosen = select_utxos(utxos, 150_000, 1, 1, 1, |_| false).unwrap();
        // Oldest first; 200k alone can't cover 150k + fee for cheap? 200k >
        // 150k + ~1sat/vb fee, so one input suffices.
        assert_eq!(chosen.len(), 1);
        assert_eq!(chosen[0].confirmations, 10);
    }

    #[test]
    fn selection_excludes_spent_and_unconfirmed() {
        let utxos = to_utxos(&[
            unspent(1, 0, 0.002, 0, true), // unconfirmed
            unspent(2, 0, 0.002, 3, true), // will be marked spent
            unspent(3, 0, 0.002, 3, true),
        ]);
        let spent_key = utxo_key(&Txid::from_str(&txid_hex(2)).unwrap(), 0);
        let chosen =
            select_utxos(utxos, 100_000, 1, 1, 1, |k| k == spent_key).unwrap();
        assert_eq!(chosen.len(), 1);
        assert_eq!(chosen[0].txid.to_string(), txid_hex(3));
    }

    #[test]
    fn selection_insufficient_reports_need() {
        let utxos = to_utxos(&[unspent(1, 0, 0.0005, 3, true)]); // 50k sats
        let err = select_utxos(utxos, 100_000, 10, 1, 1, |_| false).unwrap_err();
        assert!(matches!(err, TxError::Insufficient { .. }));
    }
}
