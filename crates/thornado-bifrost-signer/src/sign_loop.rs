//! The signing pipeline: composes the ported subsystems into the daemon's
//! sign half — poll thornado keysign → queue in the store → batch → resolve
//! the FROST party leader → join-party handshake → one taproot FROST session
//! per input → assemble witnesses → broadcast to bitcoind → record spent.
//!
//! Outbound observations are NOT posted here: the observe loop sees the
//! broadcast tx on-chain (vault as sender) and posts `MsgObservedTxOut`,
//! matching the Go bifrost's split.
//!
//! The pure decision logic (work selection, batch mapping, recipients,
//! deferral) is unit-tested here; the FROST drive is tested in
//! [`crate::transport`]; the leader handshake state machine in [`crate::p2p`].

use std::collections::BTreeMap;
use std::time::Duration;

use bitcoin::consensus::Encodable;

use crate::bitcoind::BitcoindRpc;
use crate::chain::{KeysignVerifier, ThornadoClient, TxOutItem};
use crate::frost_session::{keysign_session_id, SignSession, StoredShare};
use crate::p2p::{self, PeerRegistry};
use crate::signer::{
    frost_min_signers, frost_party_leader, next_frost_signer_attempt_height,
};
use crate::store::{SignerStore, TxOutStoreItem, TxStatus};
use crate::transport::{run_keysign_multi, Mailbox};
use crate::tx_builder::{
    apply_taproot_witness, build_unsigned, taproot_sighash, utxo_key, BuildRequest, Recipient,
    TaprootVault, UnsignedTx,
};
use crate::utxo::{prescribed_inputs, select_utxos, to_utxos};
use crate::wire::{JoinPartyLeaderComm, ResponseType};

#[derive(Debug, thiserror::Error)]
pub enum SignLoopError {
    #[error("chain: {0}")]
    Chain(#[from] crate::chain::ChainError),
    #[error("store: {0}")]
    Store(#[from] crate::store::StoreError),
    #[error("btc rpc: {0}")]
    Btc(#[from] crate::bitcoind::RpcError),
    #[error("tx build: {0}")]
    Tx(#[from] crate::tx_builder::TxError),
    #[error("party: {0}")]
    Party(#[from] p2p::P2pError),
    #[error("frost: {0}")]
    Frost(#[from] crate::transport::TransportError),
    #[error("session setup: {0}")]
    Session(#[from] crate::frost_session::FrostError),
    #[error("keysign timed out")]
    KeysignTimeout,
    #[error("prescribed inputs already spent; outbound in flight, deferring")]
    InputsSpent,
    #[error("not selected for this party")]
    NotSelected,
    #[error("config: {0}")]
    Config(String),
}

type Result<T> = std::result::Result<T, SignLoopError>;

/// Keyshares shared between the sign loop and the keygen loop, keyed by hex
/// group key. The keygen loop inserts a new vault's share as soon as its DKG
/// completes so the sign loop can sign migrations from it immediately.
pub type SharedShares =
    std::sync::Arc<std::sync::RwLock<std::collections::HashMap<String, StoredShare>>>;

/// A member's join-party request accepted by the daemon's stream listener,
/// waiting for the leader (this node) to respond on the same stream.
pub struct JoinRequest {
    /// Participant name resolved from the stream's remote PeerId.
    pub from: String,
    pub msg: JoinPartyLeaderComm,
    pub stream: libp2p::Stream,
}

/// Tunables for the sign loop.
pub struct SignLoopCfg {
    /// Our FROST participant name (the share's `participant`).
    pub local: String,
    /// The vault identifier used by the chain's keysign endpoint (the FROST
    /// group pubkey, hex-compressed).
    pub vault_id: String,
    pub network: bitcoin::Network,
    /// Blocks between signing retries (Go signing transaction period).
    pub signing_period: i64,
    /// Minimum confirmations for runtime-selected UTXOs.
    pub min_utxo_conf: u64,
    /// How long the leader collects join requests before deciding.
    pub party_wait: Duration,
    /// Grace the leader gives stragglers once threshold is met: after this
    /// long it stops waiting for FULL membership and forms the party with
    /// whoever joined. `party_wait` remains the below-threshold ceiling.
    pub party_grace: Duration,
    /// How long a member waits for the leader's response.
    pub join_wait: Duration,
    /// Ceiling for driving all of a tx's FROST sessions to completion.
    pub keysign_timeout: Duration,
    /// Max keysign heights fetched per tick when catching up.
    pub fetch_window: i64,
    /// Max batches signed per tick.
    pub max_batches_per_tick: usize,
    /// Operational escape hatch: when a batch's prescribed inputs are already
    /// spent, re-sign it with fresh UTXOs instead of deferring. Off by default
    /// (deferring is safe — the outbound is normally already in flight). Turn
    /// on ONLY to drain a batch poisoned by a buggy prior signer, accepting a
    /// possible double-payment.
    pub allow_respend_spent: bool,
}

impl Default for SignLoopCfg {
    fn default() -> Self {
        Self {
            local: String::new(),
            vault_id: String::new(),
            network: bitcoin::Network::Regtest,
            // Party attempts must be SHORT relative to the signing period, or
            // sequential attempts convoy: every node sits in a member-wait for
            // one batch while its own leadership window for another expires.
            signing_period: 30,
            min_utxo_conf: 1,
            party_wait: Duration::from_secs(12),
            party_grace: Duration::from_secs(3),
            // join_wait doubles as the leader's parked-join TTL: under load a
            // busy leader may take most of this long to reach a demanded
            // session, so cutting it (tried 8s) starves demand-driven leading
            // and parties fail "not enough peers online".
            join_wait: Duration::from_secs(20),
            keysign_timeout: Duration::from_secs(60),
            fetch_window: 20,
            max_batches_per_tick: 5,
            allow_respend_spent: false,
        }
    }
}

// ---------------------------------------------------------------------------
// Pure decision helpers (unit-tested)
// ---------------------------------------------------------------------------

/// Store items ready to sign at `height`: Available, not deferred, sorted by
/// (height, index) so every party picks the same representative.
pub fn available_work(mut items: Vec<TxOutStoreItem>, height: i64) -> Vec<TxOutStoreItem> {
    items.retain(|it| {
        it.status == TxStatus::Available
            && it.deferred_until_height <= height
            && it.item.out_hash.is_empty()
    });
    items.sort_by(|a, b| (a.height, a.index).cmp(&(b.height, b.index)));
    items
}

/// A stable fingerprint of an item's prescribed spend inputs (sorted). Items
/// batch into ONE bitcoin tx only if they share the exact same inputs — the
/// chain's batch matcher (`markObservedOutboundTxOutBatch`) requires every
/// batched item to carry identical `source_inputs`, and matches the observed
/// tx against their union. Refunds that each spend their own distinct deposit
/// UTXO are therefore NOT one batch; each signs as its own single-item tx.
fn source_inputs_key(item: &TxOutItem) -> String {
    let mut ins: Vec<(String, u32)> = item
        .source_inputs
        .iter()
        .map(|s| (s.tx_id.clone(), s.vout))
        .collect();
    ins.sort();
    ins.iter()
        .map(|(t, v)| format!("{t}:{v}"))
        .collect::<Vec<_>>()
        .join(",")
}

/// The next batch to sign from the available work: the representative is the
/// first item; every other available item that shares its EXACT prescribed
/// input union joins it, regardless of tx type or path. The chain builds a
/// unified epoch batch (child-path sweeps, main-path refunds/withdrawals, and a
/// migrate/consolidate remainder) as one block on one shared union, and it must
/// sign as one BTC tx — so grouping is by the union, not by type/path (the old
/// Go `sameBatchSource` rule, which only co-signed same-path base outbounds,
/// would strand the sweeps and let the remainder spend the union alone). Items
/// with no prescribed inputs, or a different union, sign on their own.
pub fn next_batch(avail: &[TxOutStoreItem]) -> Option<Vec<TxOutStoreItem>> {
    let rep = avail.first()?;
    let rep_key = source_inputs_key(&rep.item);
    // A shared-input batch needs non-empty inputs on every member; an empty
    // key means "select fresh", which is inherently single-item.
    if rep_key.is_empty() {
        return Some(vec![rep.clone()]);
    }
    Some(
        avail
            .iter()
            .filter(|it| source_inputs_key(&it.item) == rep_key)
            .cloned()
            .collect(),
    )
}

/// Recipient outputs for the batch, in batch order (all parties must agree).
pub fn recipients_for(
    batch: &[TxOutStoreItem],
    network: bitcoin::Network,
) -> Result<Vec<Recipient>> {
    use std::str::FromStr;
    let mut out = Vec::with_capacity(batch.len());
    for it in batch {
        let amount = it
            .item
            .coin
            .amount_u64()
            .map_err(|e| SignLoopError::Config(format!("bad amount: {e}")))?;
        // vin-only items (unified-batch sweeps, pinned migrates) carry a zeroed
        // coin: they contribute inputs, not outputs
        if amount == 0 {
            continue;
        }
        let addr = bitcoin::Address::from_str(&it.item.to_address)
            .map_err(|e| SignLoopError::Config(format!("bad to_address {}: {e}", it.item.to_address)))?
            .require_network(network)
            .map_err(|e| SignLoopError::Config(format!("address network: {e}")))?;
        out.push(Recipient {
            script_pubkey: addr.script_pubkey(),
            amount_sats: amount,
        });
    }
    Ok(out)
}

/// Session id for the join-party handshake of one signing attempt: the batch
/// identity in plaintext (`epoch-height-`) plus a hash of the full batch
/// facts. The plaintext prefix lets a leader resolve WHICH batch a member is
/// asking it to lead (demand-driven leading), the hash pins the exact items.
pub fn party_session_id(vault_id: &str, epoch: u64, height: i64, in_hashes: &[String]) -> String {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    h.update(b"party|");
    h.update(vault_id.as_bytes());
    h.update(epoch.to_be_bytes());
    h.update(height.to_be_bytes());
    for ih in in_hashes {
        h.update(ih.as_bytes());
        h.update(b"|");
    }
    format!("{epoch}-{height}-{}", hex::encode(&h.finalize()[..16]))
}

/// Parse the `epoch-height` prefix out of a party session id.
pub fn parse_party_session_id(id: &str) -> Option<(u64, i64)> {
    let mut parts = id.splitn(3, '-');
    let epoch = parts.next()?.parse().ok()?;
    let height = parts.next()?.parse().ok()?;
    parts.next()?;
    Some((epoch, height))
}

/// True when a bitcoind broadcast error means the tx is already known —
/// another party won the race, which is success for us.
pub fn broadcast_error_is_benign(message: &str) -> bool {
    let m = message.to_ascii_lowercase();
    m.contains("already in block chain")
        || m.contains("already known")
        || m.contains("already in mempool")
        || m.contains("txn-already")
        || m.contains("inputs missing or spent") // sibling tx confirmed first
        || m.contains("bad-txns-inputs-missingorspent")
}

// ---------------------------------------------------------------------------
// Party formation over /p2p/join-party-leader
// ---------------------------------------------------------------------------

/// Leader side: collect join requests for `session_id` until every member is
/// in, then answer every joined stream. Success needs `threshold` parties
/// (leader included). Stragglers get `grace` from party start; once threshold
/// is met and `grace` has elapsed the party forms with whoever joined —
/// `wait` is only the ceiling while still BELOW threshold (a laggard must not
/// stall every batch for the full wait). `seed` carries already-received
/// (parked) requests; requests for other session ids are parked back via
/// `parked` so demand-driven leading can pick them up next.
#[allow(clippy::too_many_arguments)]
pub async fn leader_form_party(
    joins: &mut tokio::sync::mpsc::Receiver<JoinRequest>,
    parked: &mut Vec<(std::time::Instant, JoinRequest)>,
    seed: Vec<JoinRequest>,
    session_id: &str,
    local: &str,
    members: &[String],
    threshold: usize,
    wait: Duration,
    grace: Duration,
) -> Result<Vec<String>> {
    let mut coordinator =
        p2p::Coordinator::new_leader(session_id, local, members.iter().cloned(), threshold);
    let mut joined: Vec<(String, libp2p::Stream)> = Vec::new();
    for req in seed {
        if req.msg.id == session_id && coordinator.on_request(&req.from) {
            joined.push((req.from.clone(), req.stream));
        }
    }
    let start = tokio::time::Instant::now();
    let deadline = start + wait;
    let grace_deadline = start + grace.min(wait);

    while coordinator.selected().len() < members.len() {
        let next_deadline = if coordinator.ready() {
            grace_deadline
        } else {
            deadline
        };
        let req = tokio::select! {
            r = joins.recv() => r,
            _ = tokio::time::sleep_until(next_deadline) => break,
        };
        let Some(req) = req else { break };
        if req.msg.id != session_id {
            parked.push((std::time::Instant::now(), req));
            continue;
        }
        if coordinator.on_request(&req.from) {
            joined.push((req.from.clone(), req.stream));
        }
    }

    let timed_out = !coordinator.ready();
    let response = coordinator.response(timed_out);
    let framed = response.encode();
    for (name, mut stream) in joined {
        if let Err(e) = crate::wire::write_frame(&mut stream, &framed).await {
            tracing::warn!(member = %name, error = %e, "failed to answer join request");
        }
    }
    if timed_out {
        return Err(p2p::P2pError::Threshold {
            have: coordinator.selected().len(),
            need: threshold,
        }
        .into());
    }
    Ok(coordinator.selected())
}

/// Member side: open a join-party stream to the leader, announce ourselves,
/// and wait for the leader's selected set.
pub async fn member_join_party(
    control: &mut libp2p_stream::Control,
    leader_peer: libp2p::PeerId,
    local: &str,
    session_id: &str,
    wait: Duration,
) -> Result<Vec<String>> {
    let request = JoinPartyLeaderComm {
        id: session_id.to_string(),
        msg_type: "request".into(),
        resp_type: ResponseType::Unknown,
        peer_ids: vec![local.to_string()],
    };
    let attempt = async {
        let mut stream = control
            .open_stream(leader_peer, p2p::join_party_protocol())
            .await
            .map_err(|e| p2p::P2pError::Transport(e.to_string()))?;
        crate::wire::write_frame(&mut stream, &request.encode())
            .await
            .map_err(|e| p2p::P2pError::Transport(e.to_string()))?;
        let resp_bytes = crate::wire::read_frame(&mut stream)
            .await
            .map_err(|e| p2p::P2pError::Transport(e.to_string()))?;
        let resp = JoinPartyLeaderComm::decode(&resp_bytes)
            .map_err(|e| p2p::P2pError::Transport(e.to_string()))?;
        p2p::interpret_response(&resp)
    };
    match tokio::time::timeout(wait, attempt).await {
        Ok(res) => Ok(res?),
        Err(_) => Err(p2p::P2pError::Timeout.into()),
    }
}

// ---------------------------------------------------------------------------
// FROST signing of a built transaction
// ---------------------------------------------------------------------------

/// The FROST keysign session id for each input of `unsigned`
/// (`keysign_session_id(vault, sighash_i)`), in input order.
pub fn keysign_session_ids(unsigned: &UnsignedTx, vault_pub: &[u8]) -> Result<Vec<String>> {
    let mut ids = Vec::with_capacity(unsigned.tx.input.len());
    for i in 0..unsigned.tx.input.len() {
        let sighash = taproot_sighash(unsigned, i)?;
        ids.push(keysign_session_id(vault_pub, &sighash));
    }
    Ok(ids)
}

/// Run one taproot FROST session per input of `unsigned` (session id =
/// `keysign_session_id(vault, sighash)`), then assemble every witness. The
/// mailbox must deliver every input session's frames (the router registers it
/// under all of them); `run_keysign_multi` fans them to the right session.
#[allow(clippy::too_many_arguments)]
pub async fn frost_sign_tx<M: Mailbox>(
    mailbox: &mut M,
    share: &StoredShare,
    local: &str,
    selected: &[String],
    unsigned: &mut UnsignedTx,
    vault_pub: &[u8],
    input_paths: &[u64],
    timeout: Duration,
) -> Result<()> {
    if input_paths.len() != unsigned.tx.input.len() {
        return Err(SignLoopError::Config(format!(
            "input path count {} does not match inputs {}",
            input_paths.len(),
            unsigned.tx.input.len()
        )));
    }
    let mut sessions = BTreeMap::new();
    let mut sid_order = Vec::with_capacity(unsigned.tx.input.len());
    for i in 0..unsigned.tx.input.len() {
        let child_tweak = (input_paths[i] != 0)
            .then(|| crate::tx_builder::child_path_tweak(vault_pub, input_paths[i]));
        let sighash = taproot_sighash(unsigned, i)?;
        let sid = keysign_session_id(vault_pub, &sighash);
        let session = SignSession::new_taproot_with_child_tweak(
            share,
            local.to_string(),
            selected.to_vec(),
            sighash.to_vec(),
            child_tweak,
        )?;
        sid_order.push(sid.clone());
        sessions.insert(sid, session);
    }

    let sigs = match tokio::time::timeout(timeout, run_keysign_multi(mailbox, &mut sessions)).await
    {
        Ok(res) => res?,
        Err(_) => return Err(SignLoopError::KeysignTimeout),
    };

    for (i, sid) in sid_order.iter().enumerate() {
        let sig = sigs
            .get(sid)
            .ok_or_else(|| SignLoopError::Config("missing signature for input".into()))?;
        apply_taproot_witness(unsigned, i, sig)?;
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// The daemon sign loop
// ---------------------------------------------------------------------------

/// Everything the loop needs from the daemon.
pub struct SignLoop {
    pub cfg: SignLoopCfg,
    pub client: ThornadoClient,
    pub verifier: Box<dyn KeysignVerifier>,
    pub store: SignerStore,
    pub btc: BitcoindRpc,
    /// All keyshares this node holds, keyed by the vault's hex group key. A
    /// churning node holds the retiring vault's share (to sign migrations) and
    /// the new vault's share at once. Shared with the keygen loop, which adds
    /// the new vault's share the moment a DKG completes.
    pub shares: SharedShares,
    pub registry: PeerRegistry,
    pub router: crate::transport::SessionRouter,
    pub control: libp2p_stream::Control,
    pub joins: tokio::sync::mpsc::Receiver<JoinRequest>,
    /// Join requests received while attending other sessions. A member asking
    /// us to lead a batch pulls that batch to the front of our queue
    /// (demand-driven leading) — without this, deferral-timing skew lets every
    /// node's queue head diverge and no party ever forms.
    parked: Vec<(std::time::Instant, JoinRequest)>,
}

impl SignLoop {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        cfg: SignLoopCfg,
        client: ThornadoClient,
        verifier: Box<dyn KeysignVerifier>,
        store: SignerStore,
        btc: BitcoindRpc,
        shares: SharedShares,
        registry: PeerRegistry,
        router: crate::transport::SessionRouter,
        control: libp2p_stream::Control,
        joins: tokio::sync::mpsc::Receiver<JoinRequest>,
    ) -> Self {
        Self {
            cfg,
            client,
            verifier,
            store,
            btc,
            shares,
            registry,
            router,
            control,
            joins,
            parked: Vec::new(),
        }
    }

    /// The keyshare for a chain vault identifier (bech32 `tthorpub…` or hex),
    /// if this node holds it.
    fn resolve_share(&self, vault_id: &str) -> Option<StoredShare> {
        let hex = if vault_id.contains("pub1") {
            hex::encode(crate::chain::decode_bech32_pubkey(vault_id).ok()?)
        } else {
            vault_id.to_string()
        };
        self.shares.read().unwrap().get(&hex).cloned()
    }

    /// The vault's taproot script/address facts for a share + path index.
    fn vault_for(share: &StoredShare, path_index: u64) -> Result<TaprootVault> {
        let pk = hex::decode(&share.public_key_compressed)
            .map_err(|e| SignLoopError::Config(format!("share pubkey hex: {e}")))?;
        Ok(TaprootVault::derive(&pk, path_index)?)
    }

    /// One poll cycle: fetch new keysign work, then sign what is ready.
    pub async fn tick(&mut self) {
        let height = match self.client.get_block_height().await {
            Ok(h) => h,
            Err(e) => {
                tracing::warn!(error = %e, "sign: failed to fetch thornado height");
                return;
            }
        };
        if let Err(e) = self.fetch_work(height).await {
            tracing::warn!(error = %e, "sign: keysign fetch failed");
        }
        if let Err(e) = self.retire_stale(height).await {
            tracing::warn!(error = %e, "sign: stale-work retirement failed");
        }
        for _ in 0..self.cfg.max_batches_per_tick {
            match self.process_next(height).await {
                Ok(true) => continue,
                Ok(false) => break,
                Err(e) => {
                    tracing::warn!(error = %e, "sign: batch attempt failed");
                    break;
                }
            }
        }
    }

    /// Discover pending keysign work from the chain's txout queue and queue it.
    ///
    /// The chain keeps a batch at its original close height until it is signed,
    /// so a linear height walk would miss batches held for retry below the
    /// tip. We ask the queue which heights have unsigned items for our vault,
    /// then fetch each one through the SIGNED keysign endpoint so every item is
    /// still authenticated against the node key.
    async fn fetch_work(&mut self, _height: i64) -> Result<()> {
        let pending = self.client.get_pending_tx_out_keysigns().await?;
        // Distinct (height, vault) with unsigned items for a vault we can sign.
        // A churning node holds both the retiring and the new vault's share, so
        // this naturally covers migration outbounds from the retiring vault.
        let mut targets: std::collections::BTreeSet<(i64, String)> =
            std::collections::BTreeSet::new();
        for txout in &pending {
            for it in &txout.tx_array {
                if it.out_hash.is_empty() && self.resolve_share(&it.vault_pub_key).is_some() {
                    targets.insert((txout.height, it.vault_pub_key.clone()));
                }
            }
        }
        // Reconcile: evict Available store items the chain no longer lists as
        // pending (e.g. a voted TXOUTCANCEL removed the block). Phantom items
        // scramble every node's attempt schedule — peers keep leading parties
        // for work nobody else recognizes.
        let chain_pending: std::collections::HashSet<(i64, String)> = pending
            .iter()
            .flat_map(|t| {
                t.tx_array
                    .iter()
                    .filter(|it| it.out_hash.is_empty())
                    .map(move |it| (t.height, it.in_hash.clone()))
            })
            .collect();
        for it in self.store.list()? {
            if it.status != TxStatus::Available || !it.item.out_hash.is_empty() {
                continue;
            }
            if !chain_pending.contains(&(it.height, it.item.in_hash.clone())) {
                tracing::info!(in_hash = %it.item.in_hash, height = it.height, "evicting store item no longer pending on chain");
                self.store.remove(&it.key())?;
            }
        }

        for (height, vault) in targets {
            let signed = match self
                .client
                .get_keysign(height, &vault, self.verifier.as_ref())
                .await
            {
                Ok(s) => s,
                Err(crate::chain::ChainError::UnavailableBlock) => continue,
                Err(e) => return Err(e.into()),
            };
            for (i, item) in signed.tx_array.iter().enumerate() {
                if !item.out_hash.is_empty() {
                    continue; // already signed on-chain
                }
                let mut stored =
                    TxOutStoreItem::new(item.clone(), signed.height, i as i64, signed.epoch);
                stored.retry_until_height = signed.retry_until_height;
                if self.store.get(&stored.key())?.is_none() {
                    self.store.put(&stored)?;
                    tracing::info!(in_hash = %item.in_hash, height = signed.height, vault = %vault, "queued txout for signing");
                }
            }
        }
        Ok(())
    }

    /// Retire queued work that has expired past the chain's
    /// `retry_until_height` so old copies cannot starve current batches. We do
    /// NOT retire on "prescribed inputs spent" alone: if the chain still lists
    /// the item as unsigned, its outbound was never credited and it must be
    /// re-signed (with fresh inputs) — retiring it here would deadlock the
    /// chain's per-vault batch queue behind an item nobody will sign.
    async fn retire_stale(&mut self, height: i64) -> Result<()> {
        for it in self.store.list()? {
            if it.status != TxStatus::Available || !it.item.out_hash.is_empty() {
                continue;
            }
            if it.retry_until_height > 0 && height > it.retry_until_height {
                tracing::info!(in_hash = %it.item.in_hash, "retiring expired txout");
                self.store.remove(&it.key())?;
            }
        }
        Ok(())
    }

    /// Absorb pending join requests into the parked buffer and drop entries
    /// whose member has certainly given up waiting.
    fn park_joins(&mut self) {
        while let Ok(req) = self.joins.try_recv() {
            self.parked.push((std::time::Instant::now(), req));
        }
        let ttl = self.cfg.join_wait.min(Duration::from_secs(25));
        self.parked.retain(|(at, _)| at.elapsed() < ttl);
    }

    /// A batch other members are asking US to lead, if any: resolve the
    /// (epoch, height) prefix of a parked join request against our store,
    /// ignoring local deferral — member demand overrides it.
    fn demanded_batch(&self, all: &[TxOutStoreItem]) -> Option<Vec<TxOutStoreItem>> {
        for (_, req) in &self.parked {
            let Some((epoch, height)) = parse_party_session_id(&req.msg.id) else {
                continue;
            };
            let subset: Vec<TxOutStoreItem> = all
                .iter()
                .filter(|it| {
                    it.epoch == epoch
                        && it.height == height
                        && it.status == TxStatus::Available
                        && it.item.out_hash.is_empty()
                })
                .cloned()
                .collect();
            let Some(batch) = next_batch(&subset) else { continue };
            // The batch's vault decides the FROST member set and the session id.
            let Some(share) = self.resolve_share(&batch[0].item.vault_pub_key) else {
                continue;
            };
            let members = crate::frost_session::normalize_participants(&share.participants);
            let leader = frost_party_leader(&members, epoch, height);
            if leader.as_deref() == Some(self.cfg.local.as_str()) {
                let in_hashes: Vec<String> =
                    batch.iter().map(|it| it.item.in_hash.clone()).collect();
                let sid =
                    party_session_id(&batch[0].item.vault_pub_key, epoch, height, &in_hashes);
                if sid == req.msg.id {
                    return Some(batch);
                }
            }
        }
        None
    }

    /// Try to sign the next ready batch — one that members demand we lead
    /// first, else our queue head. Ok(true) = signed one (try another),
    /// Ok(false) = nothing ready. Failures defer the batch and bubble up.
    async fn process_next(&mut self, height: i64) -> Result<bool> {
        self.park_joins();
        let all = self.store.list()?;
        let batch = match self.demanded_batch(&all) {
            Some(b) => b,
            None => {
                let avail = available_work(all, height);
                match next_batch(&avail) {
                    Some(b) => b,
                    None => return Ok(false),
                }
            }
        };
        let rep = batch[0].clone();

        match self.sign_batch(&batch, height).await {
            Ok(txid) => {
                tracing::info!(%txid, items = batch.len(), "signed and broadcast outbound batch");
                Ok(true)
            }
            Err(e) => {
                let retry_at =
                    next_frost_signer_attempt_height(rep.height, height, self.cfg.signing_period);
                for it in &batch {
                    let mut deferred = it.clone();
                    deferred.deferred_until_height = retry_at;
                    self.store.put(&deferred)?;
                }
                if matches!(e, SignLoopError::InputsSpent) {
                    // Expected while an already-broadcast outbound awaits
                    // observation; not a failure.
                    tracing::debug!(retry_at, items = batch.len(), "outbound in flight; deferring");
                } else {
                    tracing::warn!(error = %e, retry_at, items = batch.len(), "signing failed; deferred");
                }
                Err(e)
            }
        }
    }

    async fn sign_batch(&mut self, batch: &[TxOutStoreItem], height: i64) -> Result<String> {
        let rep = batch[0].clone();
        // Resolve the vault the batch belongs to — the retiring vault for a
        // migration, the active vault for a normal outbound.
        let vault_id = rep.item.vault_pub_key.clone();
        let share = self
            .resolve_share(&vault_id)
            .ok_or_else(|| SignLoopError::Config(format!("no keyshare for vault {vault_id}")))?;
        let members = crate::frost_session::normalize_participants(&share.participants);
        let threshold = frost_min_signers(members.len()).max(share.min_signers as usize);
        let leader = frost_party_leader(&members, rep.epoch, rep.height)
            .ok_or_else(|| SignLoopError::Config("empty member set".into()))?;

        let in_hashes: Vec<String> = batch.iter().map(|it| it.item.in_hash.clone()).collect();
        let sid = party_session_id(&vault_id, rep.epoch, rep.height, &in_hashes);

        let party_start = std::time::Instant::now();
        let selected = if leader == self.cfg.local {
            // Hand any parked requests for this session to the coordinator.
            let mut seed = Vec::new();
            let mut rest = Vec::new();
            for (at, req) in self.parked.drain(..) {
                if req.msg.id == sid {
                    seed.push(req);
                } else {
                    rest.push((at, req));
                }
            }
            self.parked = rest;
            leader_form_party(
                &mut self.joins,
                &mut self.parked,
                seed,
                &sid,
                &self.cfg.local,
                &members,
                threshold,
                self.cfg.party_wait,
                self.cfg.party_grace,
            )
            .await?
        } else {
            let leader_peer = self
                .registry
                .peer_id(&leader)
                .ok_or_else(|| SignLoopError::Config(format!("no peer entry for leader {leader}")))?;
            member_join_party(
                &mut self.control,
                leader_peer,
                &self.cfg.local,
                &sid,
                self.cfg.join_wait,
            )
            .await?
        };
        if !selected.contains(&self.cfg.local) {
            return Err(SignLoopError::NotSelected);
        }
        let party_ms = party_start.elapsed().as_millis();
        tracing::info!(leader = %leader, selected = selected.len(), party_ms, "party formed");

        // Build the identical unsigned tx on every selected party. A unified
        // epoch batch (multiple items, or a zeroed vin-only item) spends from
        // the vault ROOT with per-input child tweaks; only a legacy lone
        // coin-bearing item at a child path keeps the child-vault shape.
        let unified = batch.len() > 1
            || batch
                .iter()
                .any(|it| it.item.coin.amount_u64().unwrap_or(0) == 0)
            || batch
                .iter()
                .any(|it| it.item.source_inputs.iter().any(|s| s.path_index != 0));
        let spend_path = if unified { 0 } else { rep.item.vault_path_index };
        let vault = Self::vault_for(&share, spend_path)?;
        let vault_addr = bitcoin::Address::from_script(
            vault.script_pubkey().as_script(),
            self.cfg.network,
        )
        .map_err(|e| SignLoopError::Config(format!("vault address: {e}")))?
        .to_string();

        let recipients = recipients_for(batch, self.cfg.network)?;
        let recipients_total: u64 = recipients.iter().map(|r| r.amount_sats).sum();
        let fee_rate = rep.item.gas_rate.max(1) as u64;

        // Prescribed inputs win. If the chain prescribed a UTXO that is already
        // spent on-chain, the outbound was almost certainly already broadcast
        // (by us or a peer) and is simply awaiting observation — DEFER and let
        // the chain match it. Re-signing with fresh inputs would pay the
        // recipient a second time; every retry would pay again. When there are
        // no prescribed inputs at all (a lone unbatched item), select fresh.
        let batch_items_ref: Vec<TxOutItem> = batch.iter().map(|it| it.item.clone()).collect();
        let prescribed = prescribed_inputs(&batch_items_ref).is_some();
        let internal_batch = batch
            .iter()
            .all(|it| matches!(it.item.tx_type.as_str(), "migrate" | "sweep" | "consolidate"));
        let inputs = match prescribed_inputs(&batch_items_ref) {
            Some(inputs) => {
                let mut all_spendable = true;
                for u in &inputs {
                    if self
                        .btc
                        .get_tx_out(&u.txid.to_string(), u.vout)
                        .await?
                        .is_none()
                    {
                        all_spendable = false;
                        break;
                    }
                }
                if all_spendable {
                    inputs
                } else if self.cfg.allow_respend_spent {
                    tracing::warn!(items = batch.len(), "prescribed inputs spent; re-sweeping with fresh UTXOs (operational override)");
                    let unspent = self
                        .btc
                        .list_unspent(std::slice::from_ref(&vault_addr))
                        .await?;
                    select_utxos(
                        to_utxos(&unspent),
                        recipients_total,
                        fee_rate,
                        recipients.len(),
                        self.cfg.min_utxo_conf,
                        |key| self.store.is_spent(key).unwrap_or(false),
                    )?
                } else {
                    return Err(SignLoopError::InputsSpent);
                }
            }
            None => {
                let unspent = self
                    .btc
                    .list_unspent(std::slice::from_ref(&vault_addr))
                    .await?;
                select_utxos(
                    to_utxos(&unspent),
                    recipients_total,
                    fee_rate,
                    recipients.len(),
                    self.cfg.min_utxo_conf,
                    |key| self.store.is_spent(key).unwrap_or(false),
                )?
            }
        };
        let input_keys: Vec<String> =
            inputs.iter().map(|u| utxo_key(&u.txid, u.vout)).collect();

        // Per-input signing paths: unified batches sign each input under its
        // own taproot path; the legacy lone-item shape signs every input under
        // the item's path (child sweeps prescribe inputs without path stamps).
        let input_paths: Vec<u64> = if unified {
            inputs.iter().map(|u| u.path_index).collect()
        } else {
            vec![rep.item.vault_path_index; inputs.len()]
        };
        let base_pubkey = unified
            .then(|| hex::decode(&share.public_key_compressed))
            .transpose()
            .map_err(|e| SignLoopError::Config(format!("share pubkey hex: {e}")))?;

        // A remainder item (unpinned internal at the root path with a chain
        // computed coin) absorbs union - vouts - gas exactly: no change output.
        let has_remainder = batch.iter().any(|it| {
            matches!(it.item.tx_type.as_str(), "migrate" | "consolidate")
                && it.item.vault_path_index == 0
                && it.item.coin.amount_u64().unwrap_or(0) > 0
        });
        let exact = prescribed
            && !recipients.is_empty()
            && (has_remainder || (internal_batch && !unified));

        let mut unsigned = build_unsigned(&BuildRequest {
            vault,
            base_pubkey,
            inputs,
            recipients,
            fee_rate,
            spend_all: false,
            exact_fee_remainder: exact,
        })?;

        // The FROST session is keyed by the raw group key from the share;
        // cfg.vault_id is the chain's identifier string (bech32 on a live
        // chain, hex in test harnesses) and is only used for URLs.
        let vault_pub = hex::decode(&share.public_key_compressed)
            .map_err(|e| SignLoopError::Config(format!("share pubkey hex: {e}")))?;
        let n_inputs = unsigned.tx.input.len();
        {
            let sighash0 = taproot_sighash(&unsigned, 0)
                .map(|h| hex::encode(&h[..8]))
                .unwrap_or_default();
            tracing::info!(
                height = rep.height,
                txid = %unsigned.tx.compute_txid(),
                sighash0 = %sighash0,
                inputs = n_inputs,
                outputs = unsigned.tx.output.len(),
                paths = ?input_paths,
                unified,
                exact,
                "SIGN_ATTEMPT unsigned tx facts"
            );
        }
        // Register one routed mailbox covering every input session, so
        // concurrent keygen/keysign sessions on this host don't cross wires.
        let session_ids = keysign_session_ids(&unsigned, &vault_pub)?;
        let mut mbox = self.router.sessions_multi(&session_ids);
        let keysign_start = std::time::Instant::now();
        frost_sign_tx(
            &mut mbox,
            &share,
            &self.cfg.local,
            &selected,
            &mut unsigned,
            &vault_pub,
            &input_paths,
            self.cfg.keysign_timeout,
        )
        .await?;
        let keysign_ms = keysign_start.elapsed().as_millis();
        // One taproot FROST session PER input; report both total and per-input.
        tracing::info!(
            keysign_ms,
            inputs = n_inputs,
            per_input_ms = keysign_ms / (n_inputs.max(1) as u128),
            signers = selected.len(),
            "KEYSIGN_TIMING frost keysign complete"
        );

        let mut raw = Vec::new();
        unsigned
            .tx
            .consensus_encode(&mut raw)
            .map_err(|e| SignLoopError::Config(format!("encode tx: {e}")))?;
        let tx_hex = hex::encode(&raw);
        let txid = unsigned.tx.compute_txid().to_string();

        match self.btc.send_raw_transaction(&tx_hex).await {
            Ok(_) => {}
            Err(crate::bitcoind::RpcError::Rpc { message, .. })
                if broadcast_error_is_benign(&message) =>
            {
                tracing::debug!(%txid, %message, "broadcast raced; already known");
            }
            Err(e) => return Err(e.into()),
        }

        self.store.mark_spent(&input_keys, height)?;
        for it in batch {
            let mut done = it.clone();
            done.status = TxStatus::Broadcast;
            done.item.out_hash = txid.clone();
            done.signed_tx = Some(raw.clone());
            self.store.put(&done)?;
        }
        Ok(txid)
    }

    /// Run forever with the given tick interval.
    pub async fn run(mut self, interval: Duration) {
        let mut ticker = tokio::time::interval(interval);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        loop {
            ticker.tick().await;
            self.tick().await;
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chain::Coin;

    fn stored(in_hash: &str, height: i64, index: i64, status: TxStatus, deferred: i64) -> TxOutStoreItem {
        stored_in(in_hash, height, index, status, deferred, &[("utxoA", 0, 200_000)])
    }

    fn stored_in(
        in_hash: &str,
        height: i64,
        index: i64,
        status: TxStatus,
        deferred: i64,
        inputs: &[(&str, u32, u64)],
    ) -> TxOutStoreItem {
        use crate::chain::TxOutInput;
        let mut it = TxOutStoreItem::new(
            TxOutItem {
                chain: "BTC".into(),
                to_address: "bcrt1qw508d6qejxtdg4y5r3zarvary0c5xw7kygt080".into(),
                vault_pub_key: "vault".into(),
                coin: Coin { asset: "BTC.BTC".into(), amount: "100000".into() },
                gas_rate: 10,
                in_hash: in_hash.into(),
                tx_type: "out".into(),
                source_inputs: inputs
                    .iter()
                    .map(|(t, v, a)| TxOutInput {
                        path_index: 0,
                        tx_id: (*t).into(),
                        vout: *v,
                        amount_sats: *a,
                    })
                    .collect(),
                ..Default::default()
            },
            height,
            index,
            7,
        );
        it.status = status;
        it.deferred_until_height = deferred;
        it
    }

    #[test]
    fn available_work_filters_and_sorts() {
        let items = vec![
            stored("c", 12, 0, TxStatus::Available, 0),
            stored("a", 10, 1, TxStatus::Available, 0),
            stored("b", 10, 0, TxStatus::Available, 0),
            stored("x", 9, 0, TxStatus::Broadcast, 0),   // wrong status
            stored("y", 9, 0, TxStatus::Available, 100), // deferred
        ];
        let avail = available_work(items, 50);
        let hashes: Vec<&str> = avail.iter().map(|it| it.item.in_hash.as_str()).collect();
        assert_eq!(hashes, vec!["b", "a", "c"]);
    }

    #[test]
    fn deferred_work_returns_after_height() {
        let items = vec![stored("y", 9, 0, TxStatus::Available, 100)];
        assert!(available_work(items.clone(), 99).is_empty());
        assert_eq!(available_work(items, 100).len(), 1);
    }

    #[test]
    fn next_batch_groups_items_sharing_inputs() {
        // Two items that spend the SAME prescribed inputs → one batch tx.
        let shared = &[("shared", 0, 300_000)];
        let avail = vec![
            stored_in("a", 10, 0, TxStatus::Available, 0, shared),
            stored_in("b", 10, 1, TxStatus::Available, 0, shared),
        ];
        let batch = next_batch(&avail).unwrap();
        assert_eq!(batch.len(), 2);
    }

    #[test]
    fn next_batch_splits_items_with_distinct_inputs() {
        // Independent refunds each spend their OWN deposit UTXO → NOT one
        // batch; the chain's batch matcher requires identical source_inputs.
        let avail = vec![
            stored_in("a", 10, 0, TxStatus::Available, 0, &[("depA", 0, 250_000)]),
            stored_in("b", 10, 1, TxStatus::Available, 0, &[("depB", 0, 350_000)]),
        ];
        let batch = next_batch(&avail).unwrap();
        assert_eq!(batch.len(), 1);
        assert_eq!(batch[0].item.in_hash, "a");
    }

    #[test]
    fn next_batch_single_when_inputs_distinct() {
        // A migrate spending its OWN distinct inputs does not share a union
        // with the other item, so it signs alone. Grouping is by union, not by
        // type: it is the distinct inputs, not the "migrate" type, that isolate
        // it here.
        let mut lone = stored_in("a", 10, 0, TxStatus::Available, 0, &[("mig", 0, 500_000)]);
        lone.item.tx_type = "migrate".into();
        let other = stored_in("b", 11, 0, TxStatus::Available, 0, &[("other", 0, 100_000)]);
        let batch = next_batch(&[lone.clone(), other]).unwrap();
        assert_eq!(batch.len(), 1);
        assert_eq!(batch[0].item.in_hash, "a");
    }

    #[test]
    fn next_batch_groups_mixed_types_sharing_union() {
        // A unified epoch batch: child-path sweeps, a main-path refund, and a
        // consolidate remainder all carry the SAME prescribed union, so they
        // must sign as ONE tx — grouping is by union, not by type/path. The old
        // sameBatchSource rule would have signed the consolidate alone and
        // stranded the sweeps.
        let union = &[("u0", 0, 20_000_000), ("u1", 0, 20_000_000), ("u2", 0, 15_000_000)];
        let mut sweep = stored_in("s", 10, 0, TxStatus::Available, 0, union);
        sweep.item.tx_type = "sweep".into();
        sweep.item.vault_path_index = 51;
        let mut refund = stored_in("r", 10, 1, TxStatus::Available, 0, union);
        refund.item.tx_type = "refund".into();
        let mut cons = stored_in("c", 10, 2, TxStatus::Available, 0, union);
        cons.item.tx_type = "consolidate".into();
        let batch = next_batch(&[sweep, refund, cons]).unwrap();
        assert_eq!(batch.len(), 3, "sweep+refund+consolidate sharing a union must sign together");
        let types: std::collections::HashSet<&str> =
            batch.iter().map(|it| it.item.tx_type.as_str()).collect();
        assert!(types.contains("sweep") && types.contains("refund") && types.contains("consolidate"));
    }

    #[test]
    fn recipients_map_addresses_and_amounts() {
        let batch = vec![stored("a", 10, 0, TxStatus::Available, 0)];
        let rec = recipients_for(&batch, bitcoin::Network::Regtest).unwrap();
        assert_eq!(rec.len(), 1);
        assert_eq!(rec[0].amount_sats, 100_000);
        assert!(!rec[0].script_pubkey.is_empty());
    }

    #[test]
    fn recipients_reject_wrong_network() {
        let batch = vec![stored("a", 10, 0, TxStatus::Available, 0)];
        assert!(recipients_for(&batch, bitcoin::Network::Bitcoin).is_err());
    }

    #[test]
    fn party_session_id_is_deterministic_and_input_sensitive() {
        let a = party_session_id("v", 1, 10, &["h1".into(), "h2".into()]);
        let b = party_session_id("v", 1, 10, &["h1".into(), "h2".into()]);
        let c = party_session_id("v", 1, 11, &["h1".into(), "h2".into()]);
        let d = party_session_id("v", 1, 10, &["h1".into(), "hX".into()]);
        assert_eq!(a, b);
        assert_ne!(a, c);
        assert_ne!(a, d); // same prefix, different batch facts
        assert_eq!(parse_party_session_id(&a), Some((1, 10)));
        assert_eq!(parse_party_session_id(&c), Some((1, 11)));
        assert_eq!(parse_party_session_id("garbage"), None);
    }

    #[test]
    fn benign_broadcast_errors_recognized() {
        assert!(broadcast_error_is_benign("Transaction already in block chain"));
        assert!(broadcast_error_is_benign("txn-already-in-mempool"));
        assert!(broadcast_error_is_benign("txn-already-known"));
        assert!(!broadcast_error_is_benign("mandatory-script-verify-flag-failed"));
    }

    /// End-to-end signing of a real 2-input tx across 3 in-process parties:
    /// DKG shares, per-input taproot FROST sessions over channel mailboxes,
    /// witness assembly, and script-level verification via rust-bitcoin.
    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn frost_sign_tx_produces_valid_taproot_spend() {
        use crate::frost_session::{normalize_participants, KeygenSession};
        use crate::transport::run_keygen;
        use crate::tx_builder::Utxo;
        use std::collections::HashMap;
        use tokio::sync::mpsc;

        struct MemMailbox {
            me: String,
            senders: HashMap<String, mpsc::UnboundedSender<(String, Vec<u8>)>>,
            inbox: mpsc::UnboundedReceiver<(String, Vec<u8>)>,
        }
        impl Mailbox for MemMailbox {
            async fn send(
                &mut self,
                to: &str,
                framed: Vec<u8>,
            ) -> std::result::Result<(), crate::transport::TransportError> {
                if let Some(tx) = self.senders.get(to) {
                    let _ = tx.send((self.me.clone(), framed));
                }
                Ok(())
            }
            async fn recv(&mut self) -> Option<(String, Vec<u8>)> {
                self.inbox.recv().await
            }
        }

        let names = normalize_participants(&["p0".into(), "p1".into(), "p2".into()]);
        let mut senders = HashMap::new();
        let mut receivers = HashMap::new();
        for n in &names {
            let (tx, rx) = mpsc::unbounded_channel();
            senders.insert(n.clone(), tx);
            receivers.insert(n.clone(), rx);
        }

        // DKG
        let mut handles = Vec::new();
        for n in &names {
            let mut mbox = MemMailbox {
                me: n.clone(),
                senders: senders.clone(),
                inbox: receivers.remove(n).unwrap(),
            };
            let session = KeygenSession::new(n.clone(), names.clone(), 2).unwrap();
            let n2 = n.clone();
            handles.push(tokio::spawn(async move {
                let share = run_keygen(&mut mbox, session, "kg").await.unwrap();
                (n2, share, mbox)
            }));
        }
        let mut shares = HashMap::new();
        let mut mailboxes = HashMap::new();
        for h in handles {
            let (n, share, mbox) = h.await.unwrap();
            shares.insert(n.clone(), share);
            mailboxes.insert(n, mbox);
        }

        // Build the vault + a 2-input unsigned tx from the group key.
        let group_hex = shares[&names[0]].public_key_compressed.clone();
        let group_pub = hex::decode(&group_hex).unwrap();
        let vault = TaprootVault::derive(&group_pub, 0).unwrap();
        use bitcoin::hashes::Hash;
        let mk_utxo = |b: u8, sats: u64| Utxo {
            path_index: 0,
            txid: bitcoin::Txid::from_byte_array([b; 32]),
            vout: 0,
            amount_sats: sats,
            confirmations: 6,
        };
        let req = BuildRequest {
            vault: vault.clone(),
            base_pubkey: None,
            inputs: vec![mk_utxo(1, 100_000), mk_utxo(2, 60_000)],
            recipients: vec![Recipient {
                script_pubkey: vault.script_pubkey(),
                amount_sats: 120_000,
            }],
            fee_rate: 2,
            spend_all: false,
                exact_fee_remainder: false,
        };

        // All parties sign concurrently.
        let mut sign_handles = Vec::new();
        for n in &names {
            let mut mbox = mailboxes.remove(n).unwrap();
            let share = shares[n].clone();
            let sel = names.clone();
            let n2 = n.clone();
            let req_inputs = req.inputs.clone();
            let req_recipients = req.recipients.clone();
            let vault2 = vault.clone();
            let group_pub2 = group_pub.clone();
            sign_handles.push(tokio::spawn(async move {
                let mut unsigned = build_unsigned(&BuildRequest {
                    vault: vault2,
                    base_pubkey: None,
                    inputs: req_inputs,
                    recipients: req_recipients,
                    fee_rate: 2,
                    spend_all: false,
                exact_fee_remainder: false,
                })
                .unwrap();
                frost_sign_tx(
                    &mut mbox,
                    &share,
                    &n2,
                    &sel,
                    &mut unsigned,
                    &group_pub2,
                    &[0, 0],
                    Duration::from_secs(30),
                )
                .await
                .unwrap();
                unsigned
            }));
        }
        let mut signed = Vec::new();
        for h in sign_handles {
            signed.push(h.await.unwrap());
        }

        // Every party assembled the identical fully-signed tx.
        let txids: Vec<String> = signed.iter().map(|u| u.tx.compute_txid().to_string()).collect();
        assert!(txids.windows(2).all(|w| w[0] == w[1]));

        // rust-bitcoin as the oracle: each input's schnorr sig verifies against
        // the vault output key over the BIP341 sighash.
        let unsigned = &signed[0];
        for i in 0..unsigned.tx.input.len() {
            let w = &unsigned.tx.input[i].witness;
            assert_eq!(w.len(), 1);
            let sig_bytes = w.iter().next().unwrap();
            assert_eq!(sig_bytes.len(), 65);
            assert_eq!(sig_bytes[64], 0x81);
            let sighash = taproot_sighash(unsigned, i).unwrap();
            let secp = bitcoin::secp256k1::Secp256k1::verification_only();
            let xonly =
                bitcoin::secp256k1::XOnlyPublicKey::from_slice(&vault.output_key).unwrap();
            let sig =
                bitcoin::secp256k1::schnorr::Signature::from_slice(&sig_bytes[..64]).unwrap();
            let msg = bitcoin::secp256k1::Message::from_digest(sighash);
            secp.verify_schnorr(&sig, &msg, &xonly).expect("schnorr sig valid for vault key");
        }
    }

    /// Child-path (deposit sweep) signing: the FROST layer shifts every share
    /// by the child scalar, so the signature verifies under the CHILD taproot
    /// output key (`TaprootVault::derive(pk, path)`), rust-bitcoin as oracle.
    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn frost_sign_tx_child_path_produces_valid_taproot_spend() {
        use crate::frost_session::{normalize_participants, KeygenSession};
        use crate::transport::run_keygen;
        use crate::tx_builder::Utxo;
        use std::collections::HashMap;
        use tokio::sync::mpsc;

        struct MemMailbox {
            me: String,
            senders: HashMap<String, mpsc::UnboundedSender<(String, Vec<u8>)>>,
            inbox: mpsc::UnboundedReceiver<(String, Vec<u8>)>,
        }
        impl Mailbox for MemMailbox {
            async fn send(
                &mut self,
                to: &str,
                framed: Vec<u8>,
            ) -> std::result::Result<(), crate::transport::TransportError> {
                if let Some(tx) = self.senders.get(to) {
                    let _ = tx.send((self.me.clone(), framed));
                }
                Ok(())
            }
            async fn recv(&mut self) -> Option<(String, Vec<u8>)> {
                self.inbox.recv().await
            }
        }

        let names = normalize_participants(&["p0".into(), "p1".into(), "p2".into()]);
        let mut senders = HashMap::new();
        let mut receivers = HashMap::new();
        for n in &names {
            let (tx, rx) = mpsc::unbounded_channel();
            senders.insert(n.clone(), tx);
            receivers.insert(n.clone(), rx);
        }

        let mut handles = Vec::new();
        for n in &names {
            let mut mbox = MemMailbox {
                me: n.clone(),
                senders: senders.clone(),
                inbox: receivers.remove(n).unwrap(),
            };
            let session = KeygenSession::new(n.clone(), names.clone(), 2).unwrap();
            let n2 = n.clone();
            handles.push(tokio::spawn(async move {
                let share = run_keygen(&mut mbox, session, "kg-child").await.unwrap();
                (n2, share, mbox)
            }));
        }
        let mut shares = HashMap::new();
        let mut mailboxes = HashMap::new();
        for h in handles {
            let (n, share, mbox) = h.await.unwrap();
            shares.insert(n.clone(), share);
            mailboxes.insert(n, mbox);
        }

        const PATH: u64 = 7;
        let group_pub = hex::decode(&shares[&names[0]].public_key_compressed).unwrap();
        let child_vault = TaprootVault::derive(&group_pub, PATH).unwrap();
        let root_vault = TaprootVault::derive(&group_pub, 0).unwrap();
        use bitcoin::hashes::Hash;
        let sweep_input = Utxo {
            path_index: 0,
            txid: bitcoin::Txid::from_byte_array([5; 32]),
            vout: 1,
            amount_sats: 50_000,
            confirmations: 6,
        };

        let mut sign_handles = Vec::new();
        for n in &names {
            let mut mbox = mailboxes.remove(n).unwrap();
            let share = shares[n].clone();
            let sel = names.clone();
            let n2 = n.clone();
            let child2 = child_vault.clone();
            let root2 = root_vault.clone();
            let input2 = sweep_input.clone();
            let group_pub2 = group_pub.clone();
            sign_handles.push(tokio::spawn(async move {
                let mut unsigned = build_unsigned(&BuildRequest {
                    vault: child2,
                    base_pubkey: None,
                    inputs: vec![input2],
                    recipients: vec![Recipient {
                        script_pubkey: root2.script_pubkey(),
                        amount_sats: 49_000,
                    }],
                    fee_rate: 2,
                    spend_all: false,
                    exact_fee_remainder: true,
                })
                .unwrap();
                frost_sign_tx(
                    &mut mbox,
                    &share,
                    &n2,
                    &sel,
                    &mut unsigned,
                    &group_pub2,
                    &[PATH],
                    Duration::from_secs(30),
                )
                .await
                .unwrap();
                unsigned
            }));
        }
        let mut signed = Vec::new();
        for h in sign_handles {
            signed.push(h.await.unwrap());
        }
        let txids: Vec<String> =
            signed.iter().map(|u| u.tx.compute_txid().to_string()).collect();
        assert!(txids.windows(2).all(|w| w[0] == w[1]));

        let unsigned = &signed[0];
        let w = &unsigned.tx.input[0].witness;
        let sig_bytes = w.iter().next().unwrap();
        let sighash = taproot_sighash(unsigned, 0).unwrap();
        let secp = bitcoin::secp256k1::Secp256k1::verification_only();
        let xonly =
            bitcoin::secp256k1::XOnlyPublicKey::from_slice(&child_vault.output_key).unwrap();
        let sig = bitcoin::secp256k1::schnorr::Signature::from_slice(&sig_bytes[..64]).unwrap();
        let msg = bitcoin::secp256k1::Message::from_digest(sighash);
        secp.verify_schnorr(&sig, &msg, &xonly)
            .expect("schnorr sig valid for CHILD vault key");
    }

    /// Unified epoch batch: one tx spending a CHILD-path deposit UTXO and a
    /// ROOT UTXO, each input FROST-signed under its own tweak; both witnesses
    /// must verify against their respective taproot output keys and the
    /// prevouts must carry per-path scripts.
    #[tokio::test(flavor = "multi_thread")]
    async fn frost_sign_tx_multi_path_batch_produces_valid_spends() {
        use crate::frost_session::KeygenSession;
        use crate::transport::run_keygen;
        use crate::tx_builder::Utxo;
        use std::collections::HashMap;
        use tokio::sync::mpsc;

        struct MemMailbox {
            me: String,
            senders: HashMap<String, mpsc::UnboundedSender<(String, Vec<u8>)>>,
            inbox: mpsc::UnboundedReceiver<(String, Vec<u8>)>,
        }
        impl Mailbox for MemMailbox {
            async fn send(
                &mut self,
                to: &str,
                framed: Vec<u8>,
            ) -> std::result::Result<(), crate::transport::TransportError> {
                if let Some(tx) = self.senders.get(to) {
                    let _ = tx.send((self.me.clone(), framed));
                }
                Ok(())
            }
            async fn recv(&mut self) -> Option<(String, Vec<u8>)> {
                self.inbox.recv().await
            }
        }

        let names: Vec<String> = vec!["p0".into(), "p1".into(), "p2".into()];
        let mut senders = HashMap::new();
        let mut receivers = HashMap::new();
        for n in &names {
            let (tx, rx) = mpsc::unbounded_channel();
            senders.insert(n.clone(), tx);
            receivers.insert(n.clone(), rx);
        }

        let mut handles = Vec::new();
        for n in &names {
            let mut mbox = MemMailbox {
                me: n.clone(),
                senders: senders.clone(),
                inbox: receivers.remove(n).unwrap(),
            };
            let session = KeygenSession::new(n.clone(), names.clone(), 2).unwrap();
            let n2 = n.clone();
            handles.push(tokio::spawn(async move {
                let share = run_keygen(&mut mbox, session, "kg-multi").await.unwrap();
                (n2, share, mbox)
            }));
        }
        let mut shares = HashMap::new();
        let mut mailboxes = HashMap::new();
        for h in handles {
            let (n, share, mbox) = h.await.unwrap();
            shares.insert(n.clone(), share);
            mailboxes.insert(n, mbox);
        }

        const PATH: u64 = 3;
        let group_pub = hex::decode(&shares[&names[0]].public_key_compressed).unwrap();
        let child_vault = TaprootVault::derive(&group_pub, PATH).unwrap();
        let root_vault = TaprootVault::derive(&group_pub, 0).unwrap();
        use bitcoin::hashes::Hash;
        let inputs = vec![
            Utxo {
                txid: bitcoin::Txid::from_byte_array([7; 32]),
                vout: 0,
                amount_sats: 20_000_000,
                confirmations: 6,
                path_index: PATH,
            },
            Utxo {
                txid: bitcoin::Txid::from_byte_array([8; 32]),
                vout: 1,
                amount_sats: 5_000_000,
                confirmations: 6,
                path_index: 0,
            },
        ];
        let migrate_to = TaprootVault::derive(&group_pub, 0).unwrap();

        let mut sign_handles = Vec::new();
        for n in &names {
            let mut mbox = mailboxes.remove(n).unwrap();
            let share = shares[n].clone();
            let sel = names.clone();
            let n2 = n.clone();
            let inputs2 = inputs.clone();
            let root2 = root_vault.clone();
            let dest2 = migrate_to.clone();
            let group_pub2 = group_pub.clone();
            sign_handles.push(tokio::spawn(async move {
                let mut unsigned = build_unsigned(&BuildRequest {
                    vault: root2,
                    base_pubkey: Some(group_pub2.clone()),
                    inputs: inputs2,
                    recipients: vec![Recipient {
                        script_pubkey: dest2.script_pubkey(),
                        amount_sats: 24_990_000,
                    }],
                    fee_rate: 2,
                    spend_all: false,
                    exact_fee_remainder: true,
                })
                .unwrap();
                frost_sign_tx(
                    &mut mbox,
                    &share,
                    &n2,
                    &sel,
                    &mut unsigned,
                    &group_pub2,
                    &[PATH, 0],
                    Duration::from_secs(30),
                )
                .await
                .unwrap();
                unsigned
            }));
        }
        let mut signed = Vec::new();
        for h in sign_handles {
            signed.push(h.await.unwrap());
        }
        let txids: Vec<String> =
            signed.iter().map(|u| u.tx.compute_txid().to_string()).collect();
        assert!(txids.windows(2).all(|w| w[0] == w[1]), "parties disagree on txid");

        let unsigned = &signed[0];
        assert_eq!(
            unsigned.prevouts[0].script_pubkey,
            child_vault.script_pubkey(),
            "child input prevout must carry the child script"
        );
        assert_eq!(
            unsigned.prevouts[1].script_pubkey,
            root_vault.script_pubkey(),
            "root input prevout must carry the root script"
        );

        let secp = bitcoin::secp256k1::Secp256k1::verification_only();
        for (i, vault_key) in [(0usize, &child_vault), (1usize, &root_vault)] {
            let w = &unsigned.tx.input[i].witness;
            let sig_bytes = w.iter().next().unwrap();
            let sighash = taproot_sighash(unsigned, i).unwrap();
            let xonly =
                bitcoin::secp256k1::XOnlyPublicKey::from_slice(&vault_key.output_key).unwrap();
            let sig =
                bitcoin::secp256k1::schnorr::Signature::from_slice(&sig_bytes[..64]).unwrap();
            let msg = bitcoin::secp256k1::Message::from_digest(sighash);
            secp.verify_schnorr(&sig, &msg, &xonly)
                .unwrap_or_else(|e| panic!("input {i} sig invalid under its path key: {e}"));
        }
    }
}
