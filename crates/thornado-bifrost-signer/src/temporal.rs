//! Temporal storage: block metadata, mempool/observed tracking, spent-UTXO
//! anti-double-spend, and the last transaction fee.
//!
//! Ports `bifrost/pkg/chainclients/btc/temporal_storage.go` onto redb, keeping
//! the same logical key spaces (block-meta by height, spent-utxo by id and by
//! height) and semantics: dedup of spent ids per height, prune by height, and
//! self/customer transaction records on each block.

use std::collections::BTreeSet;
use std::path::Path;

use redb::{Database, ReadableTable, TableDefinition};
use serde::{Deserialize, Serialize};

// Logical equivalents of the Go leveldb prefixes.
const BLOCK_META: TableDefinition<i64, &[u8]> = TableDefinition::new("blockmeta");
const SPENT_BY_ID: TableDefinition<&str, u8> = TableDefinition::new("spentutxobyid");
const SPENT_BY_HEIGHT: TableDefinition<i64, &[u8]> = TableDefinition::new("spentutxobyheight");
const MEMPOOL: TableDefinition<&str, u8> = TableDefinition::new("mempool");
const OBSERVED: TableDefinition<&str, &str> = TableDefinition::new("observed");
const META: TableDefinition<&str, &[u8]> = TableDefinition::new("meta");

const TX_FEE_KEY: &str = "transaction-fee";

pub const OBSERVED_STAGE_MEMPOOL: &str = "mempool";
pub const OBSERVED_STAGE_FINAL: &str = "final";

#[derive(Debug, thiserror::Error)]
pub enum TemporalError {
    #[error("db: {0}")]
    Db(String),
    #[error("codec: {0}")]
    Codec(String),
}

type Result<T> = std::result::Result<T, TemporalError>;

/// Block metadata (Go `BlockMeta`).
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct BlockMeta {
    pub height: i64,
    #[serde(default)]
    pub previous_hash: String,
    #[serde(default)]
    pub block_hash: String,
    /// txids our vaults have broadcast.
    #[serde(default)]
    pub self_transactions: Vec<String>,
    /// txids our vaults have received.
    #[serde(default)]
    pub customer_transactions: Vec<String>,
}

/// Last transaction fee (Go `TransactionFee`).
#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, PartialEq)]
pub struct TransactionFee {
    pub fee: f64,
    pub v_size: i32,
}

#[derive(Debug, Serialize, Deserialize)]
struct SpentUtxosByHeight {
    height: i64,
    ids: Vec<String>,
}

/// redb-backed temporal store.
pub struct TemporalStore {
    db: Database,
}

impl TemporalStore {
    fn init(db: Database) -> Result<Self> {
        let w = db.begin_write().map_err(dberr)?;
        {
            w.open_table(BLOCK_META).map_err(dberr)?;
            w.open_table(SPENT_BY_ID).map_err(dberr)?;
            w.open_table(SPENT_BY_HEIGHT).map_err(dberr)?;
            w.open_table(MEMPOOL).map_err(dberr)?;
            w.open_table(OBSERVED).map_err(dberr)?;
            w.open_table(META).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(Self { db })
    }

    pub fn open(path: impl AsRef<Path>) -> Result<Self> {
        Self::init(Database::create(path).map_err(dberr)?)
    }

    pub fn in_memory() -> Result<Self> {
        Self::init(
            Database::builder()
                .create_with_backend(redb::backends::InMemoryBackend::new())
                .map_err(dberr)?,
        )
    }

    // ---- block meta ----

    pub fn save_block_meta(&self, meta: &BlockMeta) -> Result<()> {
        let bytes = serde_json::to_vec(meta).map_err(codec)?;
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut t = w.open_table(BLOCK_META).map_err(dberr)?;
            t.insert(meta.height, bytes.as_slice()).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }

    pub fn get_block_meta(&self, height: i64) -> Result<Option<BlockMeta>> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(BLOCK_META).map_err(dberr)?;
        match t.get(height).map_err(dberr)? {
            Some(v) => Ok(Some(serde_json::from_slice(v.value()).map_err(codec)?)),
            None => Ok(None),
        }
    }

    pub fn get_block_metas(&self) -> Result<Vec<BlockMeta>> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(BLOCK_META).map_err(dberr)?;
        let mut out = Vec::new();
        for row in t.iter().map_err(dberr)? {
            let (_, v) = row.map_err(dberr)?;
            out.push(serde_json::from_slice(v.value()).map_err(codec)?);
        }
        Ok(out)
    }

    /// Prune block metas at or below `height` that the filter marks prunable
    /// (Go `PruneBlockMeta`).
    pub fn prune_block_meta<F: Fn(&BlockMeta) -> bool>(
        &self,
        height: i64,
        prunable: F,
    ) -> Result<usize> {
        let w = self.db.begin_write().map_err(dberr)?;
        let mut removed = 0;
        {
            let mut t = w.open_table(BLOCK_META).map_err(dberr)?;
            let victims: Vec<i64> = t
                .iter()
                .map_err(dberr)?
                .filter_map(|row| row.ok())
                .filter_map(|(k, v)| {
                    let h = k.value();
                    if h > height {
                        return None;
                    }
                    let meta: BlockMeta = serde_json::from_slice(v.value()).ok()?;
                    prunable(&meta).then_some(h)
                })
                .collect();
            for h in victims {
                t.remove(h).map_err(dberr)?;
                removed += 1;
            }
        }
        w.commit().map_err(dberr)?;
        Ok(removed)
    }

    // ---- transaction fee ----

    pub fn upsert_transaction_fee(&self, fee: f64, v_size: i32) -> Result<()> {
        let bytes = serde_json::to_vec(&TransactionFee { fee, v_size }).map_err(codec)?;
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut t = w.open_table(META).map_err(dberr)?;
            t.insert(TX_FEE_KEY, bytes.as_slice()).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }

    pub fn get_transaction_fee(&self) -> Result<Option<TransactionFee>> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(META).map_err(dberr)?;
        match t.get(TX_FEE_KEY).map_err(dberr)? {
            Some(v) => Ok(Some(serde_json::from_slice(v.value()).map_err(codec)?)),
            None => Ok(None),
        }
    }

    // ---- mempool / observed tracking ----

    /// Returns true if newly inserted (Go `TrackMempoolTx`).
    pub fn track_mempool_tx(&self, txid: &str) -> Result<bool> {
        self.track(MEMPOOL, txid)
    }
    pub fn untrack_mempool_tx(&self, txid: &str) -> Result<()> {
        self.untrack(MEMPOOL, txid)
    }
    pub fn has_mempool_tx(&self, txid: &str) -> Result<bool> {
        self.has(MEMPOOL, txid)
    }
    pub fn list_mempool_txs(&self) -> Result<Vec<String>> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(MEMPOOL).map_err(dberr)?;
        let mut out = Vec::new();
        for row in t.iter().map_err(dberr)? {
            let (k, _) = row.map_err(dberr)?;
            out.push(k.value().to_string());
        }
        Ok(out)
    }

    pub fn track_observed_tx(&self, txid: &str) -> Result<bool> {
        // presence-only; stage defaults empty
        let existed = self.has_observed_tx(txid)?;
        if !existed {
            self.set_observed_stage(txid, "")?;
        }
        Ok(!existed)
    }
    pub fn has_observed_tx(&self, txid: &str) -> Result<bool> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(OBSERVED).map_err(dberr)?;
        Ok(t.get(txid).map_err(dberr)?.is_some())
    }
    /// Advance an observed tx to `stage`; returns (changed, previous_stage).
    pub fn track_observed_tx_stage(&self, txid: &str, stage: &str) -> Result<(bool, String)> {
        let prev = {
            let r = self.db.begin_read().map_err(dberr)?;
            let t = r.open_table(OBSERVED).map_err(dberr)?;
            t.get(txid).map_err(dberr)?.map(|v| v.value().to_string())
        };
        let prev_stage = prev.clone().unwrap_or_default();
        if prev.as_deref() == Some(stage) {
            return Ok((false, prev_stage));
        }
        self.set_observed_stage(txid, stage)?;
        Ok((true, prev_stage))
    }
    pub fn untrack_observed_tx(&self, txid: &str) -> Result<()> {
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut t = w.open_table(OBSERVED).map_err(dberr)?;
            t.remove(txid).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }

    fn set_observed_stage(&self, txid: &str, stage: &str) -> Result<()> {
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut t = w.open_table(OBSERVED).map_err(dberr)?;
            t.insert(txid, stage).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }

    // ---- spent utxos ----

    /// Record spent UTXO ids ("<txid>-<vout>") at a height, deduped, and index
    /// each by id (Go `SetSpentUtxos`).
    pub fn set_spent_utxos(&self, ids: &[String], height: i64) -> Result<()> {
        let ids: Vec<String> = ids.iter().filter(|s| !s.is_empty()).cloned().collect();
        if ids.is_empty() {
            return Ok(());
        }
        // merge with any already stored at this height, dedup (BTreeSet = sorted)
        let mut merged: BTreeSet<String> = self
            .get_spent_utxos_by_height(height)?
            .into_iter()
            .collect();
        merged.extend(ids.iter().cloned());
        let record = SpentUtxosByHeight {
            height,
            ids: merged.into_iter().collect(),
        };
        let bytes = serde_json::to_vec(&record).map_err(codec)?;
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut byh = w.open_table(SPENT_BY_HEIGHT).map_err(dberr)?;
            byh.insert(height, bytes.as_slice()).map_err(dberr)?;
            let mut byid = w.open_table(SPENT_BY_ID).map_err(dberr)?;
            for id in &ids {
                byid.insert(id.as_str(), 1u8).map_err(dberr)?;
            }
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }

    pub fn get_spent_utxos_by_height(&self, height: i64) -> Result<Vec<String>> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(SPENT_BY_HEIGHT).map_err(dberr)?;
        match t.get(height).map_err(dberr)? {
            Some(v) => {
                let rec: SpentUtxosByHeight = serde_json::from_slice(v.value()).map_err(codec)?;
                Ok(rec.ids)
            }
            None => Ok(Vec::new()),
        }
    }

    pub fn has_spent_utxo(&self, id: &str) -> Result<bool> {
        self.has(SPENT_BY_ID, id)
    }

    /// Heights at which `id` was spent, ascending (Go `FindSpentUtxoHeights`).
    pub fn find_spent_utxo_heights(&self, id: &str) -> Result<Vec<i64>> {
        if id.is_empty() {
            return Ok(Vec::new());
        }
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(SPENT_BY_HEIGHT).map_err(dberr)?;
        let mut heights = Vec::new();
        for row in t.iter().map_err(dberr)? {
            let (_, v) = row.map_err(dberr)?;
            let rec: SpentUtxosByHeight = serde_json::from_slice(v.value()).map_err(codec)?;
            if rec.ids.iter().any(|s| s == id) {
                heights.push(rec.height);
            }
        }
        heights.sort_unstable();
        Ok(heights)
    }

    /// Prune spent-utxo records at or below `height`, clearing both indexes
    /// (Go `PruneSpentUtxos`).
    pub fn prune_spent_utxos(&self, height: i64) -> Result<usize> {
        let w = self.db.begin_write().map_err(dberr)?;
        let mut pruned = 0;
        {
            let mut byh = w.open_table(SPENT_BY_HEIGHT).map_err(dberr)?;
            let stale: Vec<(i64, Vec<String>)> = byh
                .iter()
                .map_err(dberr)?
                .filter_map(|row| row.ok())
                .filter(|(k, _)| k.value() <= height)
                .filter_map(|(k, v)| {
                    let rec: SpentUtxosByHeight = serde_json::from_slice(v.value()).ok()?;
                    Some((k.value(), rec.ids))
                })
                .collect();
            let mut byid = w.open_table(SPENT_BY_ID).map_err(dberr)?;
            for (h, ids) in stale {
                byh.remove(h).map_err(dberr)?;
                for id in ids {
                    byid.remove(id.as_str()).map_err(dberr)?;
                }
                pruned += 1;
            }
        }
        w.commit().map_err(dberr)?;
        Ok(pruned)
    }

    // ---- generic set helpers ----

    fn track(&self, table: TableDefinition<&str, u8>, key: &str) -> Result<bool> {
        let existed = self.has(table, key)?;
        if !existed {
            let w = self.db.begin_write().map_err(dberr)?;
            {
                let mut t = w.open_table(table).map_err(dberr)?;
                t.insert(key, 1u8).map_err(dberr)?;
            }
            w.commit().map_err(dberr)?;
        }
        Ok(!existed)
    }
    fn untrack(&self, table: TableDefinition<&str, u8>, key: &str) -> Result<()> {
        let w = self.db.begin_write().map_err(dberr)?;
        {
            let mut t = w.open_table(table).map_err(dberr)?;
            t.remove(key).map_err(dberr)?;
        }
        w.commit().map_err(dberr)?;
        Ok(())
    }
    fn has(&self, table: TableDefinition<&str, u8>, key: &str) -> Result<bool> {
        let r = self.db.begin_read().map_err(dberr)?;
        let t = r.open_table(table).map_err(dberr)?;
        Ok(t.get(key).map_err(dberr)?.is_some())
    }
}

fn dberr<E: std::fmt::Display>(e: E) -> TemporalError {
    TemporalError::Db(e.to_string())
}
fn codec<E: std::fmt::Display>(e: E) -> TemporalError {
    TemporalError::Codec(e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn block_meta_roundtrip_and_prune() {
        let s = TemporalStore::in_memory().unwrap();
        for h in 1..=5 {
            s.save_block_meta(&BlockMeta {
                height: h,
                block_hash: format!("hash{h}"),
                self_transactions: vec![format!("selftx{h}")],
                ..Default::default()
            })
            .unwrap();
        }
        assert_eq!(s.get_block_metas().unwrap().len(), 5);
        assert_eq!(s.get_block_meta(3).unwrap().unwrap().block_hash, "hash3");

        // prune everything at/below height 3
        let pruned = s.prune_block_meta(3, |_| true).unwrap();
        assert_eq!(pruned, 3);
        assert_eq!(s.get_block_metas().unwrap().len(), 2);
        assert!(s.get_block_meta(2).unwrap().is_none());
        assert!(s.get_block_meta(4).unwrap().is_some());
    }

    #[test]
    fn prune_respects_filter() {
        let s = TemporalStore::in_memory().unwrap();
        s.save_block_meta(&BlockMeta { height: 1, block_hash: "keep".into(), ..Default::default() }).unwrap();
        s.save_block_meta(&BlockMeta { height: 2, block_hash: "drop".into(), ..Default::default() }).unwrap();
        // only prune the "drop" one even though both are <= 5
        let pruned = s.prune_block_meta(5, |m| m.block_hash == "drop").unwrap();
        assert_eq!(pruned, 1);
        assert!(s.get_block_meta(1).unwrap().is_some());
    }

    #[test]
    fn transaction_fee_roundtrip() {
        let s = TemporalStore::in_memory().unwrap();
        assert!(s.get_transaction_fee().unwrap().is_none());
        s.upsert_transaction_fee(0.00012, 141).unwrap();
        let f = s.get_transaction_fee().unwrap().unwrap();
        assert_eq!(f.v_size, 141);
        s.upsert_transaction_fee(0.00020, 200).unwrap(); // overwrite
        assert_eq!(s.get_transaction_fee().unwrap().unwrap().v_size, 200);
    }

    #[test]
    fn mempool_and_observed_tracking() {
        let s = TemporalStore::in_memory().unwrap();
        assert!(s.track_mempool_tx("tx1").unwrap()); // newly inserted
        assert!(!s.track_mempool_tx("tx1").unwrap()); // already present
        assert!(s.has_mempool_tx("tx1").unwrap());
        assert_eq!(s.list_mempool_txs().unwrap(), vec!["tx1"]);
        s.untrack_mempool_tx("tx1").unwrap();
        assert!(!s.has_mempool_tx("tx1").unwrap());

        let (changed, prev) = s.track_observed_tx_stage("obs1", OBSERVED_STAGE_MEMPOOL).unwrap();
        assert!(changed);
        assert_eq!(prev, "");
        let (changed2, prev2) = s.track_observed_tx_stage("obs1", OBSERVED_STAGE_FINAL).unwrap();
        assert!(changed2);
        assert_eq!(prev2, OBSERVED_STAGE_MEMPOOL);
        let (changed3, _) = s.track_observed_tx_stage("obs1", OBSERVED_STAGE_FINAL).unwrap();
        assert!(!changed3); // no-op when stage unchanged
    }

    #[test]
    fn spent_utxo_index_dedup_find_prune() {
        let s = TemporalStore::in_memory().unwrap();
        s.set_spent_utxos(&["txa-0".into(), "txb-1".into()], 100).unwrap();
        s.set_spent_utxos(&["txa-0".into(), "txc-0".into()], 100).unwrap(); // dedup at height
        s.set_spent_utxos(&["txa-0".into()], 200).unwrap(); // same id, different height

        assert!(s.has_spent_utxo("txa-0").unwrap());
        assert!(!s.has_spent_utxo("nope").unwrap());
        // height 100 has the deduped union
        let mut at100 = s.get_spent_utxos_by_height(100).unwrap();
        at100.sort();
        assert_eq!(at100, vec!["txa-0", "txb-1", "txc-0"]);
        // txa-0 spent at both heights, ascending
        assert_eq!(s.find_spent_utxo_heights("txa-0").unwrap(), vec![100, 200]);

        let pruned = s.prune_spent_utxos(150).unwrap();
        assert_eq!(pruned, 1); // only height 100 record
        assert!(s.get_spent_utxos_by_height(100).unwrap().is_empty());
        assert_eq!(s.find_spent_utxo_heights("txa-0").unwrap(), vec![200]);
    }
}
