//! Signer persistence (redb), replacing the Go leveldb signer store.
//!
//! Tracks txout work items through their status lifecycle and records spent
//! UTXOs for anti-double-spend, mirroring the Go `TxOutStoreItem` and
//! `spentutxo` key spaces.

use std::collections::BTreeSet;
use std::path::Path;

use redb::{Database, ReadableTable, TableDefinition};
use serde::{Deserialize, Serialize};

use crate::chain::TxOutItem;

const TXOUT: TableDefinition<&str, &[u8]> = TableDefinition::new("txout-v4");
const SPENT_UTXO: TableDefinition<&str, u64> = TableDefinition::new("spent-utxo");

#[derive(Debug, thiserror::Error)]
pub enum StoreError {
    #[error("db: {0}")]
    Db(String),
    #[error("codec: {0}")]
    Codec(String),
}

type Result<T> = std::result::Result<T, StoreError>;

/// Status lifecycle for a stored txout, matching Go `TxStatus`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum TxStatus {
    Available,
    Signed,
    Broadcast,
    Spent,
}

/// A unit of signing work plus its checkpoint/retry state (Go `TxOutStoreItem`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TxOutStoreItem {
    pub item: TxOutItem,
    pub status: TxStatus,
    pub height: i64,
    pub index: i64,
    pub epoch: u64,
    pub signing_leader: String,
    pub round7_retry: bool,
    pub deferred_until_height: i64,
    /// Serialized checkpoint (unsigned tx + per-input amounts).
    pub checkpoint: Option<Vec<u8>>,
    /// Serialized signed transaction, once produced.
    pub signed_tx: Option<Vec<u8>>,
}

impl TxOutStoreItem {
    pub fn new(item: TxOutItem, height: i64, index: i64, epoch: u64) -> Self {
        Self {
            item,
            status: TxStatus::Available,
            height,
            index,
            epoch,
            signing_leader: String::new(),
            round7_retry: false,
            deferred_until_height: 0,
            checkpoint: None,
            signed_tx: None,
        }
    }

    /// Storage key: `txout-v4-<sha256(in_hash|height|index)>` (hex), matching
    /// the shape of the Go key derivation.
    pub fn key(&self) -> String {
        use sha2::{Digest, Sha256};
        let mut h = Sha256::new();
        h.update(self.item.in_hash.as_bytes());
        h.update(self.height.to_le_bytes());
        h.update(self.index.to_le_bytes());
        format!("txout-v4-{}", hex::encode(h.finalize()))
    }
}

/// The signer store.
pub struct SignerStore {
    db: Database,
}

impl SignerStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self> {
        let db = Database::create(path).map_err(|e| StoreError::Db(e.to_string()))?;
        // Ensure tables exist.
        let w = db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        {
            w.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
            w.open_table(SPENT_UTXO)
                .map_err(|e| StoreError::Db(e.to_string()))?;
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(Self { db })
    }

    /// In-memory instance for tests.
    pub fn in_memory() -> Result<Self> {
        let db = Database::builder()
            .create_with_backend(redb::backends::InMemoryBackend::new())
            .map_err(|e| StoreError::Db(e.to_string()))?;
        let w = db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        {
            w.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
            w.open_table(SPENT_UTXO)
                .map_err(|e| StoreError::Db(e.to_string()))?;
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(Self { db })
    }

    pub fn put(&self, item: &TxOutStoreItem) -> Result<()> {
        let bytes = serde_json::to_vec(item).map_err(|e| StoreError::Codec(e.to_string()))?;
        let w = self.db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        {
            let mut t = w.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
            t.insert(item.key().as_str(), bytes.as_slice())
                .map_err(|e| StoreError::Db(e.to_string()))?;
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(())
    }

    pub fn get(&self, key: &str) -> Result<Option<TxOutStoreItem>> {
        let r = self.db.begin_read().map_err(|e| StoreError::Db(e.to_string()))?;
        let t = r.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
        let Some(v) = t.get(key).map_err(|e| StoreError::Db(e.to_string()))? else {
            return Ok(None);
        };
        let item = serde_json::from_slice(v.value()).map_err(|e| StoreError::Codec(e.to_string()))?;
        Ok(Some(item))
    }

    pub fn list(&self) -> Result<Vec<TxOutStoreItem>> {
        let r = self.db.begin_read().map_err(|e| StoreError::Db(e.to_string()))?;
        let t = r.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
        let mut out = Vec::new();
        for row in t.iter().map_err(|e| StoreError::Db(e.to_string()))? {
            let (_, v) = row.map_err(|e| StoreError::Db(e.to_string()))?;
            out.push(
                serde_json::from_slice(v.value()).map_err(|e| StoreError::Codec(e.to_string()))?,
            );
        }
        Ok(out)
    }

    pub fn remove(&self, key: &str) -> Result<()> {
        let w = self.db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        {
            let mut t = w.open_table(TXOUT).map_err(|e| StoreError::Db(e.to_string()))?;
            t.remove(key).map_err(|e| StoreError::Db(e.to_string()))?;
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(())
    }

    /// Record spent UTXO ids ("<txid>-<vout>") at a block height.
    pub fn mark_spent(&self, ids: &[String], height: i64) -> Result<()> {
        let w = self.db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        {
            let mut t = w
                .open_table(SPENT_UTXO)
                .map_err(|e| StoreError::Db(e.to_string()))?;
            for id in ids {
                t.insert(id.as_str(), height as u64)
                    .map_err(|e| StoreError::Db(e.to_string()))?;
            }
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(())
    }

    pub fn is_spent(&self, id: &str) -> Result<bool> {
        let r = self.db.begin_read().map_err(|e| StoreError::Db(e.to_string()))?;
        let t = r
            .open_table(SPENT_UTXO)
            .map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(t.get(id).map_err(|e| StoreError::Db(e.to_string()))?.is_some())
    }

    /// Prune spent-UTXO records at or below `height`.
    pub fn prune_spent(&self, height: i64) -> Result<usize> {
        let w = self.db.begin_write().map_err(|e| StoreError::Db(e.to_string()))?;
        let mut removed = BTreeSet::new();
        {
            let mut t = w
                .open_table(SPENT_UTXO)
                .map_err(|e| StoreError::Db(e.to_string()))?;
            let stale: Vec<String> = t
                .iter()
                .map_err(|e| StoreError::Db(e.to_string()))?
                .filter_map(|row| row.ok())
                .filter(|(_, v)| v.value() as i64 <= height)
                .map(|(k, _)| k.value().to_string())
                .collect();
            for k in stale {
                t.remove(k.as_str())
                    .map_err(|e| StoreError::Db(e.to_string()))?;
                removed.insert(k);
            }
        }
        w.commit().map_err(|e| StoreError::Db(e.to_string()))?;
        Ok(removed.len())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chain::{Coin, TxOutItem};

    fn sample_item(in_hash: &str) -> TxOutItem {
        TxOutItem {
            chain: "BTC".into(),
            to_address: "bc1pexample".into(),
            vault_pub_key: "thorpub1vault".into(),
            coin: Coin { asset: "BTC.BTC".into(), amount: "100000".into() },
            gas_rate: 12,
            in_hash: in_hash.into(),
            tx_type: "out".into(),
            ..Default::default()
        }
    }

    #[test]
    fn put_get_list_remove_roundtrip() {
        let s = SignerStore::in_memory().unwrap();
        let it = TxOutStoreItem::new(sample_item("hashA"), 100, 0, 5);
        let key = it.key();
        s.put(&it).unwrap();

        let got = s.get(&key).unwrap().unwrap();
        assert_eq!(got.item.in_hash, "hashA");
        assert_eq!(got.status, TxStatus::Available);
        assert_eq!(s.list().unwrap().len(), 1);

        s.remove(&key).unwrap();
        assert!(s.get(&key).unwrap().is_none());
        assert_eq!(s.list().unwrap().len(), 0);
    }

    #[test]
    fn key_is_deterministic_and_distinct() {
        let a = TxOutStoreItem::new(sample_item("h1"), 10, 0, 1);
        let a2 = TxOutStoreItem::new(sample_item("h1"), 10, 0, 1);
        let b = TxOutStoreItem::new(sample_item("h1"), 10, 1, 1);
        assert_eq!(a.key(), a2.key());
        assert_ne!(a.key(), b.key()); // different index
    }

    #[test]
    fn status_transitions_persist() {
        let s = SignerStore::in_memory().unwrap();
        let mut it = TxOutStoreItem::new(sample_item("hashB"), 200, 0, 7);
        s.put(&it).unwrap();
        it.status = TxStatus::Signed;
        it.signed_tx = Some(vec![1, 2, 3]);
        s.put(&it).unwrap();
        let got = s.get(&it.key()).unwrap().unwrap();
        assert_eq!(got.status, TxStatus::Signed);
        assert_eq!(got.signed_tx, Some(vec![1, 2, 3]));
    }

    #[test]
    fn spent_utxo_tracking_and_prune() {
        let s = SignerStore::in_memory().unwrap();
        s.mark_spent(&["txaaa-0".into(), "txbbb-1".into()], 100).unwrap();
        s.mark_spent(&["txccc-0".into()], 200).unwrap();
        assert!(s.is_spent("txaaa-0").unwrap());
        assert!(!s.is_spent("txzzz-9").unwrap());

        let pruned = s.prune_spent(150).unwrap();
        assert_eq!(pruned, 2); // heights <= 150
        assert!(!s.is_spent("txaaa-0").unwrap());
        assert!(s.is_spent("txccc-0").unwrap()); // height 200 survives
    }
}
