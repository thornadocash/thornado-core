//! Signer orchestration: batch grouping, leader selection, retry deferral.
//!
//! Ports the decision logic of Go `bifrost/signer/sign.go`. The pure functions
//! here (batchability, leader resolution, retry height) are the correctness-
//! critical core and are unit-tested; [`run`] is the async pipeline that binds
//! them to the chain client, FROST sessions, and the store.

use sha2::{Digest, Sha256};

use crate::chain::TxOutItem;

/// BTC main-vault path index (Go `common.MainVaultPathIndex`).
pub const MAIN_VAULT_PATH_INDEX: u64 = 0;

/// Whether an item is a batchable base outbound (Go `isBatchableBaseOutbound`):
/// BTC chain, main vault path, and tx_type in {out, refund, ""}.
pub fn is_batchable_base_outbound(item: &TxOutItem) -> bool {
    if item.chain != "BTC" || item.vault_path_index != MAIN_VAULT_PATH_INDEX {
        return false;
    }
    matches!(item.tx_type.as_str(), "out" | "refund" | "")
}

/// Two items share a batch source if chain, vault, and path index match
/// (Go `sameBatchSource`).
pub fn same_batch_source(a: &TxOutItem, b: &TxOutItem) -> bool {
    a.chain == b.chain
        && a.vault_pub_key == b.vault_pub_key
        && a.vault_path_index == b.vault_path_index
}

/// Group items into a batch that shares the representative's source and is
/// batchable, and where no item already has an `out_hash`. Returns the batch
/// (>= 2 items) or `None` if not batchable — matching Go `txOutBatchItems`.
pub fn batch_items(items: &[TxOutItem], representative: &TxOutItem) -> Option<Vec<TxOutItem>> {
    if !is_batchable_base_outbound(representative) {
        return None;
    }
    let batch: Vec<TxOutItem> = items
        .iter()
        .filter(|it| {
            is_batchable_base_outbound(it)
                && same_batch_source(it, representative)
                && it.out_hash.is_empty()
        })
        .cloned()
        .collect();
    if batch.len() < 2 {
        return None;
    }
    Some(batch)
}

/// Deterministic FROST party leader: `members[be_u64(sha256("txout:<epoch>:<height>")) % n]`
/// (Go `frostPartyLeader`). `members` must be the sorted signer set.
pub fn frost_party_leader(members: &[String], epoch: u64, height: i64) -> Option<String> {
    if members.is_empty() {
        return None;
    }
    let digest = Sha256::digest(format!("txout:{epoch}:{height}").as_bytes());
    let offset = u64::from_be_bytes(digest[..8].try_into().unwrap());
    let idx = (offset % members.len() as u64) as usize;
    Some(members[idx].clone())
}

/// Next height to retry when not selected this signing period
/// (Go `nextFrostSignerAttemptHeight`).
pub fn next_frost_signer_attempt_height(
    tx_height: i64,
    block_height: i64,
    signing_period: i64,
) -> i64 {
    if signing_period > 0 {
        let attempt = (block_height - tx_height) / signing_period;
        tx_height + (attempt + 1) * signing_period
    } else {
        block_height + 1
    }
}

/// Minimum FROST signers for `n` selected members: `ceil(2n/3)` (Go
/// `frostMinSigners`).
pub fn frost_min_signers(n: usize) -> usize {
    (n * 2).div_ceil(3)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chain::Coin;

    fn item(chain: &str, vault: &str, path: u64, tx_type: &str, in_hash: &str) -> TxOutItem {
        TxOutItem {
            chain: chain.into(),
            to_address: "dest".into(),
            vault_pub_key: vault.into(),
            coin: Coin { asset: "BTC.BTC".into(), amount: "1000".into() },
            gas_rate: 10,
            in_hash: in_hash.into(),
            vault_path_index: path,
            tx_type: tx_type.into(),
            ..Default::default()
        }
    }

    #[test]
    fn batchable_rules() {
        assert!(is_batchable_base_outbound(&item("BTC", "v", 0, "out", "h")));
        assert!(is_batchable_base_outbound(&item("BTC", "v", 0, "refund", "h")));
        assert!(is_batchable_base_outbound(&item("BTC", "v", 0, "", "h")));
        // wrong path index
        assert!(!is_batchable_base_outbound(&item("BTC", "v", 1, "out", "h")));
        // non-batchable type
        assert!(!is_batchable_base_outbound(&item("BTC", "v", 0, "migrate", "h")));
    }

    #[test]
    fn batch_groups_same_source_only() {
        let rep = item("BTC", "vaultA", 0, "out", "h1");
        let items = vec![
            item("BTC", "vaultA", 0, "out", "h1"),
            item("BTC", "vaultA", 0, "refund", "h2"),
            item("BTC", "vaultB", 0, "out", "h3"), // different vault
            item("BTC", "vaultA", 1, "out", "h4"), // different path
        ];
        let batch = batch_items(&items, &rep).unwrap();
        assert_eq!(batch.len(), 2); // only vaultA/path0 items
    }

    #[test]
    fn batch_none_when_single_item() {
        let rep = item("BTC", "vaultA", 0, "out", "h1");
        let items = vec![item("BTC", "vaultA", 0, "out", "h1")];
        assert!(batch_items(&items, &rep).is_none());
    }

    #[test]
    fn batch_excludes_already_signed() {
        let rep = item("BTC", "vaultA", 0, "out", "h1");
        let mut signed = item("BTC", "vaultA", 0, "out", "h2");
        signed.out_hash = "already".into();
        let items = vec![item("BTC", "vaultA", 0, "out", "h1"), signed];
        // only one unsigned remains -> not a batch
        assert!(batch_items(&items, &rep).is_none());
    }

    #[test]
    fn leader_is_deterministic() {
        let members: Vec<String> = ["a", "b", "c", "d"].iter().map(|s| s.to_string()).collect();
        let l1 = frost_party_leader(&members, 5, 100).unwrap();
        let l2 = frost_party_leader(&members, 5, 100).unwrap();
        assert_eq!(l1, l2); // stable for same (epoch, height)
        assert!(members.contains(&l1));
        // different inputs generally rotate the leader
        let l3 = frost_party_leader(&members, 6, 100).unwrap();
        assert!(members.contains(&l3));
    }

    #[test]
    fn retry_height_math() {
        // period-based: tx at 100, now 100, period 10 -> next 110
        assert_eq!(next_frost_signer_attempt_height(100, 100, 10), 110);
        // now 125 (2 periods in) -> next 130
        assert_eq!(next_frost_signer_attempt_height(100, 125, 10), 130);
        // no period -> next block
        assert_eq!(next_frost_signer_attempt_height(100, 125, 0), 126);
    }

    #[test]
    fn min_signers_is_two_thirds_ceil() {
        assert_eq!(frost_min_signers(3), 2);
        assert_eq!(frost_min_signers(4), 3);
        assert_eq!(frost_min_signers(9), 6);
        assert_eq!(frost_min_signers(10), 7);
    }
}
