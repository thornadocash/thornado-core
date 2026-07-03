//! Block-scan reorg handling.
//!
//! Ports the reorg-detection and common-ancestor walk from
//! `bifrost/pkg/chainclients/btc/client_internal.go` (`processReorg` /
//! `reConfirmTx`). The chain-tip hash lookup is a closure so this is unit-
//! tested against a seeded temporal store without a live bitcoind.

use crate::temporal::TemporalStore;

#[derive(Debug, thiserror::Error)]
pub enum ScanError {
    #[error("temporal: {0}")]
    Temporal(#[from] crate::temporal::TemporalError),
}

type Result<T> = std::result::Result<T, ScanError>;

/// Whether a block at `height` (with header `prev_hash`) extends the chain we
/// recorded, or forks it. Mirrors Go `processReorg`'s decision:
/// no reorg if we have no meta for height-1, or that meta's block hash equals
/// the new block's previous hash.
pub fn is_reorg(store: &TemporalStore, height: i64, prev_hash: &str) -> Result<bool> {
    let prev = store.get_block_meta(height - 1)?;
    match prev {
        None => Ok(false),
        Some(meta) => Ok(!meta.block_hash.eq_ignore_ascii_case(prev_hash)),
    }
}

/// Walk backwards from `height-1` to `max(1, height - max_reorg_rescan)`,
/// collecting heights whose recorded block hash no longer matches the on-chain
/// hash, stopping at the first common ancestor (recorded hash == on-chain hash).
/// Mirrors Go `reConfirmTx`. `onchain_hash_at` returns the current canonical
/// block hash at a height (e.g. bitcoind `getblockhash`), or None if
/// unavailable — matching Go's "continue" on RPC failure / empty hash.
///
/// Returns the heights that diverged and must be rescanned, in descending
/// order (as Go accumulates them).
pub fn reconfirm_heights<F>(
    store: &TemporalStore,
    height: i64,
    max_reorg_rescan: i64,
    onchain_hash_at: F,
) -> Result<Vec<i64>>
where
    F: Fn(i64) -> Option<String>,
{
    let earliest = (height - max_reorg_rescan).max(1);
    let mut rescan = Vec::new();
    let mut i = height - 1;
    while i >= earliest {
        let meta = store.get_block_meta(i)?;
        let Some(meta) = meta else {
            i -= 1;
            continue; // missing meta — skip, keep walking
        };
        let Some(hash) = onchain_hash_at(meta.height) else {
            i -= 1;
            continue; // RPC failure / empty hash — skip
        };
        if hash.is_empty() {
            i -= 1;
            continue;
        }
        if meta.block_hash.eq_ignore_ascii_case(&hash) {
            break; // common ancestor: everything prior is fine
        }
        rescan.push(meta.height); // diverged — must rescan
        i -= 1;
    }
    Ok(rescan)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::temporal::{BlockMeta, TemporalStore};
    use std::collections::HashMap;

    fn seed(store: &TemporalStore, heights: &[(i64, &str)]) {
        for (h, hash) in heights {
            store
                .save_block_meta(&BlockMeta {
                    height: *h,
                    block_hash: (*hash).to_string(),
                    ..Default::default()
                })
                .unwrap();
        }
    }

    #[test]
    fn no_reorg_when_prev_meta_missing() {
        let s = TemporalStore::in_memory().unwrap();
        // no meta at all → new block 10 can't be a reorg
        assert!(!is_reorg(&s, 10, "anyhash").unwrap());
    }

    #[test]
    fn no_reorg_when_prev_hash_matches() {
        let s = TemporalStore::in_memory().unwrap();
        seed(&s, &[(9, "hash9")]);
        assert!(!is_reorg(&s, 10, "hash9").unwrap());
        assert!(!is_reorg(&s, 10, "HASH9").unwrap()); // case-insensitive
    }

    #[test]
    fn reorg_when_prev_hash_diverges() {
        let s = TemporalStore::in_memory().unwrap();
        seed(&s, &[(9, "hash9")]);
        assert!(is_reorg(&s, 10, "different").unwrap());
    }

    #[test]
    fn reconfirm_walks_back_to_common_ancestor() {
        let s = TemporalStore::in_memory().unwrap();
        // recorded chain: 5..9 with hashes hashN
        seed(&s, &[(5, "h5"), (6, "h6"), (7, "h7"), (8, "h8"), (9, "h9")]);

        // on-chain: heights 8,9 were reorged (new hashes); 7 and below match.
        let onchain: HashMap<i64, String> = [
            (5, "h5"),
            (6, "h6"),
            (7, "h7"),
            (8, "h8_new"),
            (9, "h9_new"),
        ]
        .iter()
        .map(|(h, s)| (*h, s.to_string()))
        .collect();

        // new block at height 10, generous rescan window
        let rescan = reconfirm_heights(&s, 10, 100, |h| onchain.get(&h).cloned()).unwrap();
        // walked 9 (diverged), 8 (diverged), 7 (matches → stop)
        assert_eq!(rescan, vec![9, 8]);
    }

    #[test]
    fn reconfirm_respects_max_reorg_window() {
        let s = TemporalStore::in_memory().unwrap();
        seed(&s, &[(5, "h5"), (6, "h6"), (7, "h7"), (8, "h8"), (9, "h9")]);
        // everything diverges on-chain
        let rescan = reconfirm_heights(&s, 10, 2, |h| Some(format!("h{h}_new"))).unwrap();
        // earliest = max(1, 10-2) = 8; walk 9, 8 only
        assert_eq!(rescan, vec![9, 8]);
    }

    #[test]
    fn reconfirm_skips_missing_meta() {
        let s = TemporalStore::in_memory().unwrap();
        // gap: no meta at height 8
        seed(&s, &[(7, "h7"), (9, "h9")]);
        let onchain: HashMap<i64, String> =
            [(7, "h7"), (9, "h9_new")].iter().map(|(h, s)| (*h, s.to_string())).collect();
        // 9 diverges, 8 missing (skip), 7 matches → stop
        let rescan = reconfirm_heights(&s, 10, 100, |h| onchain.get(&h).cloned()).unwrap();
        assert_eq!(rescan, vec![9]);
    }
}
