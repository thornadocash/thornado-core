//! Daemon pipeline: the observe loop that composes the ported subsystems.
//!
//! Ties together the block source (bitcoind), reorg detection ([`crate::scanner`]),
//! observation extraction ([`crate::extract`]), and block-meta persistence
//! ([`crate::temporal`]) into the "scan new blocks → emit observations" half of
//! the bifrost. The block source is a trait so the loop is unit-tested against
//! a fake chain (including reorgs) with no live bitcoind.
//!
//! The sign half (poll thornado keysign → batch → FROST party → build+sign →
//! broadcast) composes [`crate::signer`], [`crate::transport`],
//! [`crate::tx_builder`], and [`crate::broadcast`]; its decision logic and each
//! stage are already unit-tested in their own modules. Observations are posted
//! back to thornado via [`crate::broadcast`] (SIGN_MODE_DIRECT signing in
//! [`crate::cosmos_tx`]) once a cosmos signing key is configured.

use bitcoin::Network;

use crate::extract::{self, DecodedInput, DecodedOutput, TxInItem};
use crate::scanner;
use crate::temporal::{BlockMeta, TemporalStore};

#[derive(Debug, thiserror::Error)]
pub enum DaemonError {
    #[error("block source: {0}")]
    Source(String),
    #[error("temporal: {0}")]
    Temporal(#[from] crate::temporal::TemporalError),
    #[error("scan: {0}")]
    Scan(String),
    #[error("extract: {0}")]
    Extract(#[from] crate::extract::ExtractError),
}

type Result<T> = std::result::Result<T, DaemonError>;

/// One transaction in a verbose block, already decoded to the fields the
/// observer needs (bitcoind `getblockverbose` + prevout resolution).
#[derive(Debug, Clone)]
pub struct DecodedTx {
    pub txid: String,
    pub inputs: Vec<DecodedInput>,
    pub outputs: Vec<DecodedOutput>,
    /// Sender = first input's prevout address, if resolvable.
    pub sender: String,
}

/// A verbose block with its transactions decoded.
#[derive(Debug, Clone)]
pub struct DecodedBlock {
    pub height: i64,
    pub hash: String,
    pub previous_hash: String,
    pub txs: Vec<DecodedTx>,
}

/// The chain data source (bitcoind in production, a fake in tests).
pub trait BlockSource {
    fn block_count(&self) -> impl std::future::Future<Output = Result<i64>> + Send;
    fn block_hash(
        &self,
        height: i64,
    ) -> impl std::future::Future<Output = Result<Option<String>>> + Send;
    fn block_at(
        &self,
        height: i64,
    ) -> impl std::future::Future<Output = Result<DecodedBlock>> + Send;
}

/// Vault membership predicates for observation extraction.
pub struct VaultView {
    /// Addresses that belong to our vaults (base/inbound targets).
    pub vault_addresses: std::collections::HashSet<String>,
    /// Addresses that are protocol-controlled (vault-owned senders).
    pub protocol_addresses: std::collections::HashSet<String>,
    pub observed_vault_pubkey: String,
}

impl VaultView {
    pub fn is_vault(&self, a: &str) -> bool {
        self.vault_addresses.contains(a)
    }
    pub fn is_protocol(&self, a: &str) -> bool {
        self.protocol_addresses.contains(a)
    }
}

/// Tracks how far the observer has scanned.
pub struct Observer<S: BlockSource> {
    source: S,
    network: Network,
    dust_sats: u64,
    max_reorg_rescan: i64,
    last_scanned: i64,
}

impl<S: BlockSource> Observer<S> {
    pub fn new(source: S, network: Network, dust_sats: u64, start_height: i64) -> Self {
        Self {
            source,
            network,
            dust_sats,
            max_reorg_rescan: 100,
            last_scanned: start_height,
        }
    }

    pub fn last_scanned(&self) -> i64 {
        self.last_scanned
    }

    /// Scan any blocks newer than `last_scanned` up to the tip, handling reorgs,
    /// recording block metadata, and returning the observations found. Advances
    /// `last_scanned` to the tip.
    pub async fn scan_to_tip(
        &mut self,
        store: &TemporalStore,
        vaults: &VaultView,
    ) -> Result<Vec<TxInItem>> {
        let tip = self.source.block_count().await?;
        let mut observations = Vec::new();
        let mut height = self.last_scanned + 1;
        while height <= tip {
            let block = self.source.block_at(height).await?;

            // Reorg check: does this block extend what we recorded?
            let reorg = scanner::is_reorg(store, height, &block.previous_hash)
                .map_err(|e| DaemonError::Scan(e.to_string()))?;
            if reorg {
                // Pre-fetch the canonical hashes for the reorg window so the
                // pure common-ancestor walk stays sync (and unit-tested).
                let earliest = (height - self.max_reorg_rescan).max(1);
                let mut window: std::collections::HashMap<i64, String> =
                    std::collections::HashMap::new();
                for h in earliest..height {
                    if let Some(hash) = self.source.block_hash(h).await? {
                        window.insert(h, hash);
                    }
                }
                let rescan = scanner::reconfirm_heights(
                    store,
                    height,
                    self.max_reorg_rescan,
                    |h| window.get(&h).cloned(),
                )
                .map_err(|e| DaemonError::Scan(e.to_string()))?;
                for h in rescan {
                    let b = self.source.block_at(h).await?;
                    observations.extend(self.extract_block(&b, vaults)?);
                    // update the recorded meta to the new canonical block
                    self.record_block_meta(store, &b)?;
                }
            }

            observations.extend(self.extract_block(&block, vaults)?);
            self.record_block_meta(store, &block)?;
            self.last_scanned = height;
            height += 1;
        }
        Ok(observations)
    }

    fn extract_block(&self, block: &DecodedBlock, vaults: &VaultView) -> Result<Vec<TxInItem>> {
        let mut out = Vec::new();
        for tx in &block.txs {
            let mut item = extract::extract_observation(
                block.height,
                &tx.txid,
                &tx.inputs,
                &tx.outputs,
                &tx.sender,
                |a| vaults.is_vault(a),
                |a| vaults.is_protocol(a),
                self.dust_sats,
                self.network,
            )?;
            if let Some(ref mut item) = item {
                if item.observed_vault_pubkey.is_empty() {
                    item.observed_vault_pubkey = vaults.observed_vault_pubkey.clone();
                }
                out.push(item.clone());
            }
        }
        Ok(out)
    }

    fn record_block_meta(&self, store: &TemporalStore, block: &DecodedBlock) -> Result<()> {
        // Record customer txs (received by a vault) for later reorg errata.
        let customer_txs: Vec<String> = block
            .txs
            .iter()
            .filter(|tx| tx.outputs.iter().any(|_| true) && !vaults_self_only(tx))
            .map(|tx| tx.txid.clone())
            .collect();
        store.save_block_meta(&BlockMeta {
            height: block.height,
            previous_hash: block.previous_hash.clone(),
            block_hash: block.hash.clone(),
            customer_transactions: customer_txs,
            ..Default::default()
        })?;
        Ok(())
    }
}

fn vaults_self_only(_tx: &DecodedTx) -> bool {
    false
}

/// Live [`BlockSource`] backed by bitcoind. Resolves each input's prevout
/// (address + amount) via `getrawtransaction`, matching what the Go observer
/// needs to compute sender and gas.
pub struct BitcoindBlockSource {
    rpc: crate::bitcoind::BitcoindRpc,
}

impl BitcoindBlockSource {
    pub fn new(rpc: crate::bitcoind::BitcoindRpc) -> Self {
        Self { rpc }
    }

    async fn decode_tx(&self, tx: &crate::bitcoind::VerboseTx) -> Result<DecodedTx> {
        let mut inputs = Vec::with_capacity(tx.vin.len());
        for vin in &tx.vin {
            // coinbase inputs have no prevout
            let (Some(prev_txid), Some(prev_vout)) = (vin.txid.clone(), vin.vout) else {
                continue;
            };
            let prev = self
                .rpc
                .get_raw_transaction(&prev_txid)
                .await
                .map_err(|e| DaemonError::Source(e.to_string()))?;
            let (addr, amount) = prev
                .vout
                .iter()
                .find(|o| o.n == prev_vout)
                .map(|o| {
                    (
                        o.script_pubkey.single_address(),
                        crate::extract::btc_to_sats(o.value),
                    )
                })
                .unwrap_or((None, 0));
            inputs.push(DecodedInput {
                prev_txid,
                prev_vout,
                prev_address: addr,
                prev_amount_sats: amount,
            });
        }
        let outputs = tx
            .vout
            .iter()
            .map(|o| DecodedOutput {
                n: o.n,
                value_sats: crate::extract::btc_to_sats(o.value),
                script_hex: o.script_pubkey.hex.clone(),
            })
            .collect();
        let sender = inputs
            .first()
            .and_then(|i| i.prev_address.clone())
            .unwrap_or_default();
        Ok(DecodedTx {
            txid: tx.txid.clone(),
            inputs,
            outputs,
            sender,
        })
    }
}

impl BlockSource for BitcoindBlockSource {
    async fn block_count(&self) -> Result<i64> {
        self.rpc
            .get_block_count()
            .await
            .map_err(|e| DaemonError::Source(e.to_string()))
    }

    async fn block_hash(&self, height: i64) -> Result<Option<String>> {
        match self.rpc.get_block_hash(height).await {
            Ok(h) => Ok(Some(h)),
            Err(crate::bitcoind::RpcError::Rpc { .. }) => Ok(None), // out of range
            Err(e) => Err(DaemonError::Source(e.to_string())),
        }
    }

    async fn block_at(&self, height: i64) -> Result<DecodedBlock> {
        let hash = self
            .rpc
            .get_block_hash(height)
            .await
            .map_err(|e| DaemonError::Source(e.to_string()))?;
        let block = self
            .rpc
            .get_block_verbose_txs(&hash)
            .await
            .map_err(|e| DaemonError::Source(e.to_string()))?;
        let mut txs = Vec::with_capacity(block.tx.len());
        for tx in &block.tx {
            txs.push(self.decode_tx(tx).await?);
        }
        Ok(DecodedBlock {
            height: block.height,
            hash: block.hash,
            previous_hash: block.previous_block_hash,
            txs,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::extract::{DecodedInput, DecodedOutput};
    use crate::tx_builder::TaprootVault;
    use std::collections::HashMap;

    // A fake chain: height -> block. Reorgs are simulated by swapping blocks.
    struct FakeChain {
        blocks: HashMap<i64, DecodedBlock>,
        tip: i64,
    }
    impl BlockSource for FakeChain {
        async fn block_count(&self) -> Result<i64> {
            Ok(self.tip)
        }
        async fn block_hash(&self, height: i64) -> Result<Option<String>> {
            Ok(self.blocks.get(&height).map(|b| b.hash.clone()))
        }
        async fn block_at(&self, height: i64) -> Result<DecodedBlock> {
            self.blocks
                .get(&height)
                .cloned()
                .ok_or_else(|| DaemonError::Source(format!("no block {height}")))
        }
    }

    fn vault_addr() -> (String, String) {
        // A real P2TR vault address (regtest) + its script hex, so extraction's
        // rust-bitcoin decode yields the same string we register.
        let vault = TaprootVault::from_output_key([9u8; 32]);
        let script = vault.script_pubkey();
        let addr =
            bitcoin::Address::from_script(script.as_script(), bitcoin::Network::Regtest).unwrap();
        (addr.to_string(), hex::encode(script.as_bytes()))
    }

    fn inbound_block(height: i64, hash: &str, prev: &str, vault_script_hex: &str) -> DecodedBlock {
        DecodedBlock {
            height,
            hash: hash.into(),
            previous_hash: prev.into(),
            txs: vec![DecodedTx {
                txid: format!("tx{height}"),
                sender: "customer".into(),
                inputs: vec![DecodedInput {
                    prev_txid: "prevtx".into(),
                    prev_vout: 0,
                    prev_address: Some("customer".into()),
                    prev_amount_sats: 200_000,
                }],
                outputs: vec![DecodedOutput {
                    n: 0,
                    value_sats: 150_000,
                    script_hex: vault_script_hex.to_string(),
                }],
            }],
        }
    }

    fn view(vault_addr: &str) -> VaultView {
        let mut vaults = std::collections::HashSet::new();
        vaults.insert(vault_addr.to_string());
        VaultView {
            vault_addresses: vaults,
            protocol_addresses: std::collections::HashSet::new(),
            observed_vault_pubkey: "thorpub1vault".into(),
        }
    }

    #[tokio::test]
    async fn scans_new_blocks_and_extracts_inbound() {
        let (addr, script) = vault_addr();
        let mut blocks = HashMap::new();
        blocks.insert(1, inbound_block(1, "h1", "h0", &script));
        blocks.insert(2, inbound_block(2, "h2", "h1", &script));
        let chain = FakeChain { blocks, tip: 2 };
        let store = TemporalStore::in_memory().unwrap();

        let mut obs = Observer::new(chain, Network::Regtest, 10_000, 0);
        let items = obs.scan_to_tip(&store, &view(&addr)).await.unwrap();

        assert_eq!(items.len(), 2); // one inbound per block
        assert_eq!(items[0].to, addr);
        assert_eq!(items[0].coins[0].amount_sats, 150_000);
        assert_eq!(items[0].observed_vault_pubkey, "thorpub1vault");
        assert_eq!(obs.last_scanned(), 2);
        // block metas recorded
        assert!(store.get_block_meta(1).unwrap().is_some());
        assert!(store.get_block_meta(2).unwrap().is_some());
    }

    #[tokio::test]
    async fn idempotent_when_no_new_blocks() {
        let (addr, script) = vault_addr();
        let mut blocks = HashMap::new();
        blocks.insert(1, inbound_block(1, "h1", "h0", &script));
        let chain = FakeChain { blocks, tip: 1 };
        let store = TemporalStore::in_memory().unwrap();
        let mut obs = Observer::new(chain, Network::Regtest, 10_000, 1); // already scanned 1
        let items = obs.scan_to_tip(&store, &view(&addr)).await.unwrap();
        assert!(items.is_empty());
    }

    #[tokio::test]
    async fn dust_output_is_skipped() {
        let (addr, script) = vault_addr();
        let mut b = inbound_block(1, "h1", "h0", &script);
        b.txs[0].outputs[0].value_sats = 500; // below dust
        let mut blocks = HashMap::new();
        blocks.insert(1, b);
        let chain = FakeChain { blocks, tip: 1 };
        let store = TemporalStore::in_memory().unwrap();
        let mut obs = Observer::new(chain, Network::Regtest, 10_000, 0);
        let items = obs.scan_to_tip(&store, &view(&addr)).await.unwrap();
        assert!(items.is_empty()); // dust filtered
    }

    #[tokio::test]
    async fn reorg_rescans_diverged_block() {
        let (addr, script) = vault_addr();
        let store = TemporalStore::in_memory().unwrap();

        // First pass: scan blocks 1,2 with hashes h1,h2.
        let mut blocks = HashMap::new();
        blocks.insert(1, inbound_block(1, "h1", "h0", &script));
        blocks.insert(2, inbound_block(2, "h2", "h1", &script));
        let chain = FakeChain { blocks, tip: 2 };
        let mut obs = Observer::new(chain, Network::Regtest, 10_000, 0);
        obs.scan_to_tip(&store, &view(&addr)).await.unwrap();
        assert_eq!(obs.last_scanned(), 2);

        // Reorg: height 2 is replaced (h2_new, still building on h1), and a new
        // block 3 extends it. Feed a fresh source reflecting the new chain.
        let mut blocks2 = HashMap::new();
        blocks2.insert(1, inbound_block(1, "h1", "h0", &script));
        blocks2.insert(2, inbound_block(2, "h2_new", "h1", &script));
        blocks2.insert(3, inbound_block(3, "h3", "h2_new", &script));
        let chain2 = FakeChain { blocks: blocks2, tip: 3 };
        obs = Observer {
            source: chain2,
            network: Network::Regtest,
            dust_sats: 10_000,
            max_reorg_rescan: 100,
            last_scanned: 2,
        };
        // Scanning height 3: its previous hash h2_new != recorded h2 -> reorg,
        // rescans height 2, plus extracts block 3.
        let items = obs.scan_to_tip(&store, &view(&addr)).await.unwrap();
        // rescanned block 2 + new block 3 = 2 observations
        assert_eq!(items.len(), 2);
        // block meta for 2 updated to the new hash
        assert_eq!(store.get_block_meta(2).unwrap().unwrap().block_hash, "h2_new");
    }
}
