//! Thornado chain client (signer subset).
//!
//! Ports the parts of the Go `bifrost/thornadoclient` the signer path needs
//! (`thornado.go`, `keysign.go`, `block_height.go`, `node_account.go`):
//! fetching keysign work, block heights, vaults/signers, node accounts, the
//! txout queue, network fees, sync status and config values. Broadcasting
//! (cosmos tx posting) is out of scope for this module.
//!
//! Interop notes (must match Go byte-for-byte):
//! - `GET /thornado/keysign/{height}/{pubkey}` returns
//!   `{ "keysign": <TxOut JSON>, "signature": base64 }`. The signature is a
//!   cosmos-style secp256k1 ECDSA signature (64-byte r||s over SHA256 of the
//!   message) of the *compacted* keysign JSON exactly as received —
//!   whitespace stripped, key order preserved. A height mismatch between the
//!   request and the signed payload is rejected (replay protection).
//! - `/thornado/txout/all` serializes numbers as quoted strings while the
//!   keysign payload uses JSON numbers; all numeric fields accept both.
//! - Responses are cached per URL for one block time like Go's
//!   `httpResponseCache`, with a per-URL lock so concurrent callers of the
//!   same endpoint share a single in-flight request.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use base64::Engine;
use bitcoin::secp256k1::{self, Secp256k1, VerifyOnly};
use serde::de::{self, Deserializer};
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;
use sha2::{Digest, Sha256};

/// REST endpoint templates (match Go constants).
pub const KEYSIGN_ENDPOINT: &str = "/thornado/keysign"; // /{height}/{pubkey}
pub const KEYGEN_ENDPOINT: &str = "/thornado/keygen"; // /{height}/{pubkey}
pub const LAST_BLOCK_ENDPOINT: &str = "/thornado/lastblock";
pub const VAULT_ENDPOINT: &str = "/thornado/vault"; // /{pubkey}
pub const SIGNER_MEMBERSHIP_ENDPOINT: &str = "/thornado/vaults"; // /{pubkey}/signers
pub const NETWORK_FEE_ENDPOINT: &str = "/thornado/network_fee";
pub const NODE_ACCOUNT_ENDPOINT: &str = "/thornado/node"; // /{address}
pub const NODE_ACCOUNTS_ENDPOINT: &str = "/thornado/nodes";
pub const TX_OUT_ENDPOINT: &str = "/thornado/txout"; // /all
pub const CONFIG_ENDPOINT: &str = "/thornado/config";
pub const CONFIG_DEFAULTS_ENDPOINT: &str = "/thornado/config/defaults";
pub const STATUS_ENDPOINT: &str = "/status"; // on the RPC host

/// Cosmos gas limit the signer uses when broadcasting (Go hard-coded).
pub const BROADCAST_GAS_LIMIT: u64 = 4_000_000_000;

/// Mainnet thornado block time; per-URL response cache TTL.
pub const DEFAULT_BLOCK_TIME: Duration = Duration::from_secs(6);

const MAX_ATTEMPTS: u32 = 4;
const RETRY_BASE_DELAY: Duration = Duration::from_millis(500);

#[derive(Debug, thiserror::Error)]
pub enum ChainError {
    #[error("http: {0}")]
    Http(String),
    #[error("decode: {0}")]
    Decode(String),
    #[error("unexpected status code {status} from {path}")]
    Status { status: u16, path: String },
    #[error("unavailable block")]
    UnavailableBlock,
    #[error("invalid keysign: {0}")]
    InvalidKeysign(String),
    #[error("not found: {0}")]
    NotFound(String),
}

type Result<T> = std::result::Result<T, ChainError>;

impl From<reqwest::Error> for ChainError {
    fn from(e: reqwest::Error) -> Self {
        ChainError::Http(e.to_string())
    }
}

impl From<serde_json::Error> for ChainError {
    fn from(e: serde_json::Error) -> Self {
        ChainError::Decode(e.to_string())
    }
}

// ---------------------------------------------------------------------------
// Keysign payload authentication
// ---------------------------------------------------------------------------

/// Verifies the node-signed keysign payload, mirroring Go's
/// `cryptotypes.PubKey.VerifySignature`. The signing key is this node's own
/// validator key: thornadod signs the txout so its local bifrost can detect
/// tampering between the two processes.
pub trait KeysignVerifier: Send + Sync {
    fn verify(&self, msg: &[u8], sig: &[u8]) -> bool;
}

/// Cosmos-style secp256k1 verifier: ECDSA over SHA256(msg), 64-byte compact
/// r||s signature, built from the node's 33-byte compressed pubkey.
pub struct Secp256k1Verifier {
    ctx: Secp256k1<VerifyOnly>,
    key: secp256k1::PublicKey,
}

impl Secp256k1Verifier {
    pub fn new(compressed: &[u8]) -> Result<Self> {
        let key = secp256k1::PublicKey::from_slice(compressed)
            .map_err(|e| ChainError::Decode(format!("bad pubkey: {e}")))?;
        Ok(Self {
            ctx: Secp256k1::verification_only(),
            key,
        })
    }
}

impl KeysignVerifier for Secp256k1Verifier {
    fn verify(&self, msg: &[u8], sig: &[u8]) -> bool {
        let digest: [u8; 32] = Sha256::digest(msg).into();
        let message = secp256k1::Message::from_digest(digest);
        let Ok(signature) = secp256k1::ecdsa::Signature::from_compact(sig) else {
            return false;
        };
        self.ctx.verify_ecdsa(&message, &signature, &self.key).is_ok()
    }
}

/// Decode a thornado bech32 pubkey (`tthorpub1...`/`thorpub1...`) into the
/// 33-byte compressed secp256k1 key: bech32 data is the 5-byte amino
/// `PubKeySecp256k1` prefix plus the key.
pub fn decode_bech32_pubkey(s: &str) -> Result<Vec<u8>> {
    let (_hrp, data) = bitcoin::bech32::decode(s)
        .map_err(|e| ChainError::Decode(format!("bech32 pubkey: {e}")))?;
    if data.len() < 33 {
        return Err(ChainError::Decode(format!(
            "bech32 pubkey too short: {} bytes",
            data.len()
        )));
    }
    Ok(data[data.len() - 33..].to_vec())
}

/// The amino type-prefix for a `PubKeySecp256k1` (4-byte prefix + 0x21 length),
/// which precedes the 33-byte compressed key in a thornado bech32 pubkey.
pub const AMINO_SECP256K1_PREFIX: [u8; 5] = [0xeb, 0x5a, 0xe9, 0x87, 0x21];

/// Encode a 33-byte compressed secp256k1 key as a thornado bech32 pubkey with
/// the given hrp (`tthorpub` on testnet, `thorpub` on mainnet) — the inverse of
/// [`decode_bech32_pubkey`].
pub fn encode_bech32_pubkey(hrp: &str, compressed: &[u8]) -> Result<String> {
    use bitcoin::bech32::{Bech32, Hrp};
    if compressed.len() != 33 {
        return Err(ChainError::Decode(format!(
            "compressed pubkey is {} bytes, want 33",
            compressed.len()
        )));
    }
    let mut data = AMINO_SECP256K1_PREFIX.to_vec();
    data.extend_from_slice(compressed);
    let hrp = Hrp::parse(hrp).map_err(|e| ChainError::Decode(format!("bad hrp: {e}")))?;
    bitcoin::bech32::encode::<Bech32>(hrp, &data)
        .map_err(|e| ChainError::Decode(format!("bech32 encode: {e}")))
}

/// Decode a thornado bech32 account address (`tthor1...`) to its 20 raw bytes.
pub fn decode_bech32_account(s: &str) -> Result<Vec<u8>> {
    let (_hrp, data) = bitcoin::bech32::decode(s)
        .map_err(|e| ChainError::Decode(format!("bech32 account: {e}")))?;
    if data.len() != 20 {
        return Err(ChainError::Decode(format!(
            "account address is {} bytes, want 20",
            data.len()
        )));
    }
    Ok(data)
}

/// Accepts any payload WITHOUT verifying the keysign signature. Only for
/// tests and local development where the node key is not available; never use
/// on a network where funds are at stake.
pub struct InsecureAcceptAll;

impl KeysignVerifier for InsecureAcceptAll {
    fn verify(&self, _msg: &[u8], _sig: &[u8]) -> bool {
        true
    }
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

/// A coin amount as returned by the API (cosmos Uint amounts arrive as quoted
/// strings, occasionally as JSON numbers).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct Coin {
    #[serde(default)]
    pub asset: String,
    #[serde(default, deserialize_with = "flexnum::de_string")]
    pub amount: String,
}

impl Coin {
    pub fn amount_u64(&self) -> Result<u64> {
        self.amount
            .parse()
            .map_err(|_| ChainError::Decode(format!("bad amount {}", self.amount)))
    }
}

/// A prescribed spend input (UTXO reference) for batched/sweep outbounds.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct TxOutInput {
    #[serde(default)]
    pub tx_id: String,
    #[serde(default, deserialize_with = "flexnum::de_u32")]
    pub vout: u32,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub amount_sats: u64,
}

/// One outbound to sign (Go `TxArrayItem`).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct TxOutItem {
    #[serde(default)]
    pub chain: String,
    #[serde(default)]
    pub to_address: String,
    #[serde(default)]
    pub vault_pub_key: String,
    #[serde(default)]
    pub coin: Coin,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub max_gas: Vec<Coin>,
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub gas_rate: i64,
    #[serde(default)]
    pub in_hash: String,
    #[serde(default)]
    pub out_hash: String,
    #[serde(default, deserialize_with = "flexnum::de_u32")]
    pub out_vout: u32,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub vault_path_index: u64,
    #[serde(default)]
    pub tx_type: String,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub source_inputs: Vec<TxOutInput>,
}

/// A keysign batch (Go `TxOut`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TxOut {
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub height: i64,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub tx_array: Vec<TxOutItem>,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub epoch: u64,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub signing_leader: String,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub signing_attempt: u64,
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub retry_until_height: i64,
}

impl TxOut {
    pub fn has_unsigned_item(&self) -> bool {
        self.tx_array.iter().any(|item| item.out_hash.is_empty())
    }
}

/// Per-chain last block heights (Go `/thornado/lastblock`).
#[derive(Debug, Clone, Deserialize)]
pub struct LastBlock {
    #[serde(default)]
    pub chain: String,
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub thornado: i64,
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub last_observed_in: i64,
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub last_signed_out: i64,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct PubKeySet {
    #[serde(default)]
    pub secp256k1: String,
    #[serde(default)]
    pub ed25519: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct NodeAccount {
    #[serde(default)]
    pub node_address: String,
    #[serde(default, deserialize_with = "flexnum::de_string")]
    pub status: String,
    #[serde(default)]
    pub pub_key_set: PubKeySet,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub signer_membership: Vec<String>,
    #[serde(default)]
    pub peer_id: String,
    #[serde(default)]
    pub version: String,
}

impl NodeAccount {
    pub fn is_active(&self) -> bool {
        self.status.eq_ignore_ascii_case("active")
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct Vault {
    #[serde(default)]
    pub pub_key: String,
    #[serde(default)]
    pub pub_key_eddsa: String,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub membership: Vec<String>,
}

/// One scheduled keygen in a keygen block (Go `types.Keygen`). `members` are
/// the FROST participant pubkeys (bech32) that must run the DKG together.
#[derive(Debug, Clone, Deserialize)]
pub struct Keygen {
    #[serde(default)]
    pub id: String,
    #[serde(rename = "type", default)]
    pub keygen_type: String, // "BaseVaultKeygen"
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub members: Vec<String>,
}

/// A keygen block: the churn instruction the chain issues at a rotation
/// (Go `types.KeygenBlock`).
#[derive(Debug, Clone, Deserialize)]
pub struct KeygenBlock {
    #[serde(default, deserialize_with = "flexnum::de_i64")]
    pub height: i64,
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    pub keygens: Vec<Keygen>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct NetworkFee {
    #[serde(default)]
    pub chain: String,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub transaction_size: u64,
    #[serde(default, deserialize_with = "flexnum::de_u64")]
    pub transaction_fee_rate: u64,
}

#[derive(Deserialize)]
struct KeysignResponse {
    keysign: Option<Box<RawValue>>,
    #[serde(default)]
    signature: String,
}

#[derive(Deserialize)]
struct KeygenResponse {
    keygen_block: Option<Box<RawValue>>,
    #[serde(default)]
    signature: String,
}

#[derive(Deserialize)]
struct TxOutQueue {
    #[serde(default, deserialize_with = "flexnum::de_null_vec")]
    txouts: Vec<TxOut>,
}

#[derive(Deserialize)]
struct RpcStatus {
    result: RpcResult,
}

#[derive(Deserialize)]
struct RpcResult {
    sync_info: RpcSyncInfo,
}

#[derive(Deserialize)]
struct RpcSyncInfo {
    #[serde(default)]
    catching_up: bool,
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

/// Configuration for locating the thornado node.
#[derive(Debug, Clone)]
pub struct ChainConfig {
    pub chain_host: String,
    pub chain_rpc: String,
}

impl ChainConfig {
    /// Build a base URL, prefixing http:// if the host lacks a scheme (Go logic).
    pub fn base_url(&self) -> String {
        normalize_base(&self.chain_host)
    }

    pub fn rpc_url(&self) -> String {
        normalize_base(&self.chain_rpc)
    }

    pub fn keysign_url(&self, height: i64, vault_pubkey: &str) -> String {
        format!("{}{}/{}/{}", self.base_url(), KEYSIGN_ENDPOINT, height, vault_pubkey)
    }

    pub fn last_block_url(&self) -> String {
        format!("{}{}", self.base_url(), LAST_BLOCK_ENDPOINT)
    }

    pub fn signers_url(&self, vault_pubkey: &str) -> String {
        format!("{}{}/{}/signers", self.base_url(), SIGNER_MEMBERSHIP_ENDPOINT, vault_pubkey)
    }
}

fn normalize_base(host: &str) -> String {
    let url = if host.starts_with("http") {
        host.to_string()
    } else {
        format!("http://{host}")
    };
    url.trim_end_matches('/').to_string()
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

#[derive(Default)]
struct CacheSlot {
    body: Option<Vec<u8>>,
    checked: Option<Instant>,
}

/// HTTP client for the thornado REST API (signer read path).
pub struct ThornadoClient {
    cfg: ChainConfig,
    http: reqwest::Client,
    block_time: Duration,
    cache: Mutex<HashMap<String, Arc<tokio::sync::Mutex<CacheSlot>>>>,
}

impl ThornadoClient {
    pub fn new(cfg: ChainConfig) -> Self {
        let http = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .expect("reqwest client");
        Self {
            cfg,
            http,
            block_time: DEFAULT_BLOCK_TIME,
            cache: Mutex::new(HashMap::new()),
        }
    }

    /// Override the per-URL response cache TTL (mocknet uses 1s blocks).
    pub fn with_block_time(mut self, block_time: Duration) -> Self {
        self.block_time = block_time;
        self
    }

    fn url(&self, path: &str) -> String {
        format!("{}/{}", self.cfg.base_url(), path.trim_start_matches('/'))
    }

    /// Cached GET: at most one upstream request per URL per block time, with
    /// retry/backoff on transport errors, 429 and 5xx. Only 200 responses are
    /// cached (Go `httpResponseCache` semantics).
    async fn get(&self, url: &str) -> Result<(Vec<u8>, u16)> {
        let slot = {
            let mut cache = self.cache.lock().expect("chain response cache poisoned");
            cache.entry(url.to_string()).or_default().clone()
        };
        let mut slot = slot.lock().await;

        if let (Some(body), Some(checked)) = (&slot.body, slot.checked) {
            if checked.elapsed() < self.block_time {
                return Ok((body.clone(), 200));
            }
        }

        let (body, status) = self.get_with_retry(url).await?;
        if status == 200 {
            slot.body = Some(body.clone());
            slot.checked = Some(Instant::now());
        }
        Ok((body, status))
    }

    async fn get_with_retry(&self, url: &str) -> Result<(Vec<u8>, u16)> {
        let mut last_err = None;
        for attempt in 0..MAX_ATTEMPTS {
            if attempt > 0 {
                tokio::time::sleep(RETRY_BASE_DELAY * 2u32.pow(attempt - 1)).await;
            }
            match self.http.get(url).send().await {
                Ok(resp) => {
                    let status = resp.status().as_u16();
                    if status == 429 || (500..600).contains(&status) {
                        last_err = Some(ChainError::Status {
                            status,
                            path: url.to_string(),
                        });
                        continue;
                    }
                    let body = resp.bytes().await?;
                    return Ok((body.to_vec(), status));
                }
                Err(err) => last_err = Some(err.into()),
            }
        }
        Err(last_err.unwrap_or(ChainError::Http("no attempts made".to_string())))
    }

    async fn get_json_ok<T: serde::de::DeserializeOwned>(&self, path: &str) -> Result<T> {
        let url = self.url(path);
        let (body, status) = self.get(&url).await?;
        if status != 200 {
            return Err(ChainError::Status {
                status,
                path: path.to_string(),
            });
        }
        Ok(serde_json::from_slice(&body)?)
    }

    /// GET the keysign batch for a vault at a thornado height. 404 →
    /// `UnavailableBlock`. The payload signature is verified against the node
    /// key and the signed height must match the requested one.
    pub async fn get_keysign(
        &self,
        height: i64,
        vault_pubkey: &str,
        verifier: &dyn KeysignVerifier,
    ) -> Result<TxOut> {
        let path = format!("{KEYSIGN_ENDPOINT}/{height}/{vault_pubkey}");
        let url = self.url(&path);
        let (body, status) = self.get(&url).await?;
        if status == 404 {
            return Err(ChainError::UnavailableBlock);
        }
        if status != 200 {
            return Err(ChainError::Status { status, path });
        }
        let wrapper: KeysignResponse = serde_json::from_slice(&body)?;
        verify_and_decode_keysign(&wrapper, verifier, height)
    }

    /// GET the keygen block for this node at a thornado height. 404 →
    /// `UnavailableBlock`. The payload is signed by the node key exactly like
    /// keysign; the signature is verified before the block is trusted.
    pub async fn get_keygen_block(
        &self,
        height: i64,
        node_pubkey: &str,
        verifier: &dyn KeysignVerifier,
    ) -> Result<KeygenBlock> {
        let path = format!("{KEYGEN_ENDPOINT}/{height}/{node_pubkey}");
        let url = self.url(&path);
        let (body, status) = self.get(&url).await?;
        if status == 404 {
            return Err(ChainError::UnavailableBlock);
        }
        if status != 200 {
            return Err(ChainError::Status { status, path });
        }
        let wrapper: KeygenResponse = serde_json::from_slice(&body)?;
        let raw = wrapper
            .keygen_block
            .as_deref()
            .map(RawValue::get)
            .ok_or_else(|| ChainError::InvalidKeysign("empty keygen block".to_string()))?;
        let block: KeygenBlock = serde_json::from_str(raw)?;
        // Empty blocks (no churn scheduled) carry no meaningful signature.
        if block.keygens.is_empty() {
            return Ok(block);
        }
        if wrapper.signature.is_empty() {
            return Err(ChainError::InvalidKeysign("keygen signature: empty".to_string()));
        }
        let signed = compact_json(raw.as_bytes());
        let sig = base64::engine::general_purpose::STANDARD
            .decode(&wrapper.signature)
            .map_err(|_| ChainError::InvalidKeysign("keygen signature: cannot decode".to_string()))?;
        if !verifier.verify(&signed, &sig) {
            return Err(ChainError::InvalidKeysign(
                "keygen signature: bad signature".to_string(),
            ));
        }
        Ok(block)
    }

    async fn get_last_blocks(&self) -> Result<Vec<LastBlock>> {
        self.get_json_ok(LAST_BLOCK_ENDPOINT).await
    }

    /// GET the current thornado block height.
    pub async fn get_block_height(&self) -> Result<i64> {
        let blocks = self.get_last_blocks().await?;
        blocks
            .first()
            .map(|b| b.thornado)
            .ok_or_else(|| ChainError::NotFound("lastblock is empty".to_string()))
    }

    pub async fn get_last_signed_out_height(&self, chain: &str) -> Result<i64> {
        let blocks = self.get_last_blocks().await?;
        blocks
            .iter()
            .find(|b| b.chain == chain)
            .map(|b| b.last_signed_out)
            .ok_or_else(|| ChainError::NotFound(format!("lastblock for chain {chain}")))
    }

    pub async fn get_last_observed_in_height(&self, chain: &str) -> Result<i64> {
        let blocks = self.get_last_blocks().await?;
        blocks
            .iter()
            .find(|b| b.chain == chain)
            .map(|b| b.last_observed_in)
            .ok_or_else(|| ChainError::NotFound(format!("lastblock for chain {chain}")))
    }

    /// GET the raw signer set for a vault (may be empty).
    pub async fn get_signers(&self, vault_pubkey: &str) -> Result<Vec<String>> {
        let path = format!("{SIGNER_MEMBERSHIP_ENDPOINT}/{vault_pubkey}/signers");
        let signers: Option<Vec<String>> = self.get_json_ok(&path).await?;
        Ok(signers.unwrap_or_default())
    }

    /// The pubkeys that should join a keysign for this vault. Falls back to
    /// deriving the party from the vault membership and active node accounts
    /// when `/signers` is empty or unavailable (mirrors Go `GetKeysignParty`).
    pub async fn get_keysign_party(&self, vault_pubkey: &str) -> Result<Vec<String>> {
        if let Ok(keys) = self.get_signers(vault_pubkey).await {
            if !keys.is_empty() {
                return Ok(keys);
            }
        }
        let vault = self.get_vault(vault_pubkey).await?;
        let nodes = self.get_node_accounts().await?;
        derive_keysign_party(vault_pubkey, &vault, &nodes)
    }

    /// GET /thornado/vault/{pubkey}
    pub async fn get_vault(&self, pubkey: &str) -> Result<Vault> {
        self.get_json_ok(&format!("{VAULT_ENDPOINT}/{pubkey}")).await
    }

    /// GET /thornado/node/{address}
    pub async fn get_node_account(&self, address: &str) -> Result<NodeAccount> {
        self.get_json_ok(&format!("{NODE_ACCOUNT_ENDPOINT}/{address}"))
            .await
    }

    /// GET /thornado/nodes
    pub async fn get_node_accounts(&self) -> Result<Vec<NodeAccount>> {
        self.get_json_ok(NODE_ACCOUNTS_ENDPOINT).await
    }

    /// Secp256k1 pubkeys of all Active node accounts.
    pub async fn fetch_active_nodes(&self) -> Result<Vec<String>> {
        let nodes = self.get_node_accounts().await?;
        Ok(nodes
            .iter()
            .filter(|n| n.is_active())
            .map(|n| n.pub_key_set.secp256k1.clone())
            .filter(|pk| !pk.is_empty())
            .collect())
    }

    /// GET /thornado/network_fee → (transaction_size, transaction_fee_rate).
    pub async fn get_network_fee(&self) -> Result<(u64, u64)> {
        let fee: NetworkFee = self.get_json_ok(NETWORK_FEE_ENDPOINT).await?;
        Ok((fee.transaction_size, fee.transaction_fee_rate))
    }

    pub async fn has_network_fee(&self) -> Result<bool> {
        let (size, _) = self.get_network_fee().await?;
        Ok(size > 0)
    }

    async fn get_tx_out_queue(&self) -> Result<Vec<TxOut>> {
        let queue: TxOutQueue = self
            .get_json_ok(&format!("{TX_OUT_ENDPOINT}/all"))
            .await?;
        Ok(queue.txouts.into_iter().filter(|t| t.height > 0).collect())
    }

    /// TxOuts that still contain at least one unsigned item.
    pub async fn get_pending_tx_out_keysigns(&self) -> Result<Vec<TxOut>> {
        let all = self.get_tx_out_queue().await?;
        Ok(all
            .into_iter()
            .filter(|t| !t.tx_array.is_empty() && t.has_unsigned_item())
            .collect())
    }

    pub async fn get_all_tx_out_keysigns(&self) -> Result<Vec<TxOut>> {
        self.get_tx_out_queue().await
    }

    /// GET {chain_rpc}/status → sync_info.catching_up.
    pub async fn is_catching_up(&self) -> Result<bool> {
        let url = format!("{}{STATUS_ENDPOINT}", self.cfg.rpc_url());
        let (body, status) = self.get(&url).await?;
        if status != 200 {
            return Err(ChainError::Status {
                status,
                path: STATUS_ENDPOINT.to_string(),
            });
        }
        let resp: RpcStatus = serde_json::from_slice(&body)?;
        Ok(resp.result.sync_info.catching_up)
    }

    pub async fn wait_to_catch_up(&self) -> Result<()> {
        loop {
            if !self.is_catching_up().await? {
                return Ok(());
            }
            tracing::info!("thornado is not caught up... waiting...");
            tokio::time::sleep(self.block_time).await;
        }
    }

    /// A config value: overrides from /thornado/config win over genesis
    /// defaults from /thornado/config/defaults; key matching is normalized
    /// (case/underscore/dash-insensitive, suffix match), mirroring Go.
    pub async fn get_config_value(&self, key: &str) -> Result<i64> {
        let overrides = self.get_config_values(CONFIG_ENDPOINT).await?;
        if let Some(v) = lookup_config_value(&overrides, key) {
            return Ok(v);
        }
        let defaults = self.get_config_values(CONFIG_DEFAULTS_ENDPOINT).await?;
        lookup_config_value(&defaults, key)
            .ok_or_else(|| ChainError::NotFound(format!("config key: {key}")))
    }

    async fn get_config_values(&self, endpoint: &str) -> Result<HashMap<String, i64>> {
        let url = self.url(endpoint);
        let (body, status) = self.get(&url).await?;
        if status != 200 {
            return Err(ChainError::Status {
                status,
                path: endpoint.to_string(),
            });
        }
        decode_config_int64_values(&body)
    }
}

// ---------------------------------------------------------------------------
// Keysign decoding + verification (Go verifyAndDecodeKeysign)
// ---------------------------------------------------------------------------

/// Decode and authenticate a keysign response: an empty tx_array needs no
/// signature; otherwise the base64 ECDSA signature must verify over the
/// compacted raw payload, and the signed height must equal the requested one
/// (replay protection).
fn verify_and_decode_keysign(
    wrapper: &KeysignResponse,
    verifier: &dyn KeysignVerifier,
    block_height: i64,
) -> Result<TxOut> {
    let raw = wrapper
        .keysign
        .as_deref()
        .map(RawValue::get)
        .filter(|s| !s.trim().is_empty() && s.trim() != "null")
        .ok_or_else(|| ChainError::InvalidKeysign("empty payload".to_string()))?;

    let tx_out: TxOut = serde_json::from_str(raw)?;
    if tx_out.tx_array.is_empty() {
        return Ok(tx_out);
    }
    if wrapper.signature.is_empty() {
        return Err(ChainError::InvalidKeysign("signature: empty".to_string()));
    }
    let signed = compact_json(raw.as_bytes());
    let sig = base64::engine::general_purpose::STANDARD
        .decode(&wrapper.signature)
        .map_err(|_| ChainError::InvalidKeysign("signature: cannot decode".to_string()))?;
    if !verifier.verify(&signed, &sig) {
        return Err(ChainError::InvalidKeysign("signature: bad signature".to_string()));
    }
    if tx_out.height != block_height {
        return Err(ChainError::InvalidKeysign(format!(
            "block height mismatch ({} vs {})",
            tx_out.height, block_height
        )));
    }
    Ok(tx_out)
}

/// Strip insignificant whitespace from JSON without reordering keys — the
/// byte-exact equivalent of Go's `json.Compact`, which is what the chain
/// signature commits to.
pub fn compact_json(raw: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(raw.len());
    let mut in_string = false;
    let mut escaped = false;
    for &b in raw {
        if in_string {
            out.push(b);
            if escaped {
                escaped = false;
            } else if b == b'\\' {
                escaped = true;
            } else if b == b'"' {
                in_string = false;
            }
            continue;
        }
        match b {
            b' ' | b'\t' | b'\n' | b'\r' => {}
            b'"' => {
                in_string = true;
                out.push(b);
            }
            _ => out.push(b),
        }
    }
    out
}

// ---------------------------------------------------------------------------
// Keysign party derivation (fallback when /signers is empty)
// ---------------------------------------------------------------------------

fn derive_keysign_party(
    vault_pubkey: &str,
    vault: &Vault,
    nodes: &[NodeAccount],
) -> Result<Vec<String>> {
    let members: Vec<&str> = vault.membership.iter().map(String::as_str).collect();
    let mut keys: Vec<String> = nodes
        .iter()
        .filter(|n| n.is_active())
        .filter(|n| !n.pub_key_set.secp256k1.is_empty())
        .filter(|n| members.is_empty() || members.contains(&n.pub_key_set.secp256k1.as_str()))
        .filter(|n| {
            n.signer_membership
                .iter()
                .any(|m| m.eq_ignore_ascii_case(vault_pubkey))
        })
        .map(|n| n.pub_key_set.secp256k1.clone())
        .collect();
    if keys.is_empty() {
        return Err(ChainError::NotFound(format!(
            "no active key sign party members for vault {vault_pubkey}"
        )));
    }
    keys.sort();
    keys.dedup();
    Ok(keys)
}

// ---------------------------------------------------------------------------
// Config value decoding (Go decodeConfigInt64Values / lookupConfigValue)
// ---------------------------------------------------------------------------

fn decode_config_int64_values(buf: &[u8]) -> Result<HashMap<String, i64>> {
    let top: HashMap<String, serde_json::Value> = serde_json::from_slice(buf)?;
    let mut values = HashMap::new();
    for (key, raw) in top {
        if let Some(v) = parse_config_i64(&raw) {
            values.insert(key, v);
            continue;
        }
        if let serde_json::Value::Object(entries) = raw {
            for (name, entry) in entries {
                if let Some(v) = parse_config_i64(&entry) {
                    values.insert(format!("{key}_{name}"), v);
                }
            }
        }
    }
    Ok(values)
}

fn parse_config_i64(raw: &serde_json::Value) -> Option<i64> {
    let raw = match raw {
        serde_json::Value::Object(map) => map.get("value").unwrap_or(raw),
        _ => raw,
    };
    match raw {
        serde_json::Value::Number(n) => n.as_i64(),
        serde_json::Value::String(s) => s.parse().ok(),
        _ => None,
    }
}

fn lookup_config_value(values: &HashMap<String, i64>, key: &str) -> Option<i64> {
    if let Some(v) = values.get(key) {
        return Some(*v);
    }
    let normalized = normalize_config_key(key);
    values
        .iter()
        .find(|(candidate, _)| {
            let c = normalize_config_key(candidate);
            c == normalized || c.ends_with(&normalized)
        })
        .map(|(_, v)| *v)
}

fn normalize_config_key(key: &str) -> String {
    key.chars()
        .filter(|c| *c != '_' && *c != '-')
        .flat_map(char::to_uppercase)
        .collect()
}

// ---------------------------------------------------------------------------
// Lenient numeric decoding: the chain emits numbers as JSON numbers on some
// endpoints and as quoted strings on others (cosmos Uint, txout queue).
// Unparseable values decode to zero, matching Go's parseRaw* helpers.
// ---------------------------------------------------------------------------

mod flexnum {
    use super::*;

    /// Go's json.Marshal encodes nil slices as `null`; treat that as empty.
    pub fn de_null_vec<'de, D, T>(d: D) -> std::result::Result<Vec<T>, D::Error>
    where
        D: Deserializer<'de>,
        T: Deserialize<'de>,
    {
        Ok(Option::<Vec<T>>::deserialize(d)?.unwrap_or_default())
    }

    struct Flex;

    impl<'de> de::Visitor<'de> for Flex {
        type Value = i128;

        fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
            f.write_str("a number or numeric string")
        }

        fn visit_i64<E: de::Error>(self, v: i64) -> std::result::Result<i128, E> {
            Ok(v as i128)
        }

        fn visit_u64<E: de::Error>(self, v: u64) -> std::result::Result<i128, E> {
            Ok(v as i128)
        }

        fn visit_f64<E: de::Error>(self, v: f64) -> std::result::Result<i128, E> {
            Ok(v as i128)
        }

        fn visit_str<E: de::Error>(self, v: &str) -> std::result::Result<i128, E> {
            Ok(v.trim().parse().unwrap_or(0))
        }

        fn visit_unit<E: de::Error>(self) -> std::result::Result<i128, E> {
            Ok(0)
        }

        fn visit_none<E: de::Error>(self) -> std::result::Result<i128, E> {
            Ok(0)
        }

        fn visit_some<D: Deserializer<'de>>(self, d: D) -> std::result::Result<i128, D::Error> {
            d.deserialize_any(Flex)
        }
    }

    pub fn de_i64<'de, D: Deserializer<'de>>(d: D) -> std::result::Result<i64, D::Error> {
        Ok(d.deserialize_any(Flex)?
            .clamp(i64::MIN as i128, i64::MAX as i128) as i64)
    }

    pub fn de_u64<'de, D: Deserializer<'de>>(d: D) -> std::result::Result<u64, D::Error> {
        Ok(d.deserialize_any(Flex)?.clamp(0, u64::MAX as i128) as u64)
    }

    pub fn de_u32<'de, D: Deserializer<'de>>(d: D) -> std::result::Result<u32, D::Error> {
        Ok(d.deserialize_any(Flex)?.clamp(0, u32::MAX as i128) as u32)
    }

    struct FlexString;

    impl<'de> de::Visitor<'de> for FlexString {
        type Value = String;

        fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
            f.write_str("a string or number")
        }

        fn visit_str<E: de::Error>(self, v: &str) -> std::result::Result<String, E> {
            Ok(v.to_string())
        }

        fn visit_i64<E: de::Error>(self, v: i64) -> std::result::Result<String, E> {
            Ok(v.to_string())
        }

        fn visit_u64<E: de::Error>(self, v: u64) -> std::result::Result<String, E> {
            Ok(v.to_string())
        }

        fn visit_unit<E: de::Error>(self) -> std::result::Result<String, E> {
            Ok(String::new())
        }
    }

    pub fn de_string<'de, D: Deserializer<'de>>(d: D) -> std::result::Result<String, D::Error> {
        d.deserialize_any(FlexString)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bitcoin::secp256k1::SecretKey;

    fn signed_keysign(raw: &str, key: &SecretKey) -> KeysignResponse {
        let ctx = Secp256k1::new();
        let digest: [u8; 32] = Sha256::digest(compact_json(raw.as_bytes())).into();
        let sig = ctx.sign_ecdsa(&secp256k1::Message::from_digest(digest), key);
        KeysignResponse {
            keysign: Some(RawValue::from_string(raw.to_string()).unwrap()),
            signature: base64::engine::general_purpose::STANDARD.encode(sig.serialize_compact()),
        }
    }

    fn verifier_for(key: &SecretKey) -> Secp256k1Verifier {
        let ctx = Secp256k1::new();
        let pubkey = secp256k1::PublicKey::from_secret_key(&ctx, key);
        Secp256k1Verifier::new(&pubkey.serialize()).unwrap()
    }

    const RAW_TXOUT: &str = r#"{
        "height": 42,
        "tx_array": [
            {
                "chain": "BTC",
                "to_address": "bc1pdest",
                "vault_pub_key": "thorpub1vault",
                "coin": { "asset": "BTC.BTC", "amount": "100000" },
                "max_gas": [{ "asset": "BTC.BTC", "amount": "100" }],
                "gas_rate": "12",
                "in_hash": "ABC123",
                "vault_path_index": 0,
                "tx_type": "out",
                "source_inputs": [
                    { "tx_id": "deadbeef", "vout": 1, "amount_sats": 50000 }
                ]
            }
        ]
    }"#;

    #[test]
    fn base_url_prefixes_scheme() {
        let c = ChainConfig { chain_host: "localhost:1317".into(), chain_rpc: "".into() };
        assert_eq!(c.base_url(), "http://localhost:1317");
        let c2 = ChainConfig { chain_host: "https://node.example".into(), chain_rpc: "".into() };
        assert_eq!(c2.base_url(), "https://node.example");
        let c3 = ChainConfig { chain_host: "http://node.example/".into(), chain_rpc: "".into() };
        assert_eq!(c3.base_url(), "http://node.example");
    }

    #[test]
    fn keysign_url_shape() {
        let c = ChainConfig { chain_host: "h:1317".into(), chain_rpc: "".into() };
        assert_eq!(
            c.keysign_url(42, "thorpub1abc"),
            "http://h:1317/thornado/keysign/42/thorpub1abc"
        );
        assert_eq!(
            c.signers_url("thorpub1abc"),
            "http://h:1317/thornado/vaults/thorpub1abc/signers"
        );
    }

    #[test]
    fn compact_json_strips_whitespace_outside_strings_only() {
        let raw = br#"{ "a" : "x y\" z" , "b" : [ 1 , 2 ] }"#;
        assert_eq!(compact_json(raw), br#"{"a":"x y\" z","b":[1,2]}"#.to_vec());
    }

    #[test]
    fn keysign_roundtrip_verifies_and_decodes() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let wrapper = signed_keysign(RAW_TXOUT, &key);
        let tx_out = verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).unwrap();
        assert_eq!(tx_out.height, 42);
        assert_eq!(tx_out.tx_array.len(), 1);
        let item = &tx_out.tx_array[0];
        assert_eq!(item.coin.amount_u64().unwrap(), 100000);
        assert_eq!(item.gas_rate, 12);
        assert_eq!(item.max_gas[0].amount_u64().unwrap(), 100);
        assert_eq!(item.source_inputs[0].amount_sats, 50000);
        assert!(tx_out.has_unsigned_item());
    }

    #[test]
    fn keysign_rejects_height_mismatch() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let wrapper = signed_keysign(RAW_TXOUT, &key);
        let err = verify_and_decode_keysign(&wrapper, &verifier_for(&key), 43).unwrap_err();
        assert!(matches!(err, ChainError::InvalidKeysign(m) if m.contains("height mismatch")));
    }

    #[test]
    fn keysign_rejects_wrong_key_and_missing_signature() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let other = SecretKey::from_slice(&[9u8; 32]).unwrap();
        let mut wrapper = signed_keysign(RAW_TXOUT, &key);
        assert!(verify_and_decode_keysign(&wrapper, &verifier_for(&other), 42).is_err());
        wrapper.signature = String::new();
        let err = verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).unwrap_err();
        assert!(matches!(err, ChainError::InvalidKeysign(m) if m.contains("empty")));
    }

    #[test]
    fn keysign_rejects_tampered_payload() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let mut wrapper = signed_keysign(RAW_TXOUT, &key);
        let tampered = RAW_TXOUT.replace("100000", "900000");
        wrapper.keysign = Some(RawValue::from_string(tampered).unwrap());
        assert!(verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).is_err());
    }

    #[test]
    fn keysign_empty_tx_array_skips_signature() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let wrapper = KeysignResponse {
            keysign: Some(RawValue::from_string(r#"{"height":42}"#.to_string()).unwrap()),
            signature: String::new(),
        };
        let tx_out = verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).unwrap();
        assert!(tx_out.tx_array.is_empty());
    }

    /// Go's json.Marshal writes nil slices as `null` — the live chain returns
    /// `"tx_array": null` for empty keysign blocks.
    #[test]
    fn keysign_null_tx_array_is_empty() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let wrapper = KeysignResponse {
            keysign: Some(
                RawValue::from_string(r#"{"height":42,"tx_array":null}"#.to_string()).unwrap(),
            ),
            signature: String::new(),
        };
        let tx_out = verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).unwrap();
        assert!(tx_out.tx_array.is_empty());
    }

    #[test]
    fn keysign_empty_payload_rejected() {
        let key = SecretKey::from_slice(&[7u8; 32]).unwrap();
        let wrapper = KeysignResponse {
            keysign: None,
            signature: String::new(),
        };
        assert!(verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).is_err());
        let wrapper = KeysignResponse {
            keysign: Some(RawValue::from_string("null".to_string()).unwrap()),
            signature: String::new(),
        };
        assert!(verify_and_decode_keysign(&wrapper, &verifier_for(&key), 42).is_err());
    }

    #[test]
    fn parse_last_block_array_with_quoted_numbers() {
        let json = r#"[{"chain":"THOR","thornado":"999","last_observed_in":"10","last_signed_out":"5"},
                       {"chain":"BTC","thornado":999,"last_observed_in":10,"last_signed_out":5}]"#;
        let blocks: Vec<LastBlock> = serde_json::from_str(json).unwrap();
        assert_eq!(blocks[0].thornado, 999);
        assert_eq!(blocks[0].last_observed_in, 10);
        assert_eq!(blocks[1].thornado, 999);
        assert_eq!(blocks[1].last_signed_out, 5);
    }

    #[test]
    fn txout_queue_parses_string_numbers_and_filters_invalid() {
        let body = r#"{
            "txouts": [
                {
                    "height": "12",
                    "epoch": "3",
                    "status": "pending",
                    "signing_leader": "thorpub1leader",
                    "signing_attempt": "2",
                    "retry_until_height": "99",
                    "tx_array": [
                        {
                            "chain": "BTC",
                            "coin": { "asset": "BTC.BTC", "amount": "777" },
                            "out_vout": "1",
                            "vault_path_index": "5"
                        }
                    ]
                },
                { "height": "bogus", "tx_array": [] }
            ]
        }"#;
        let queue: TxOutQueue = serde_json::from_str(body).unwrap();
        let valid: Vec<_> = queue.txouts.into_iter().filter(|t| t.height > 0).collect();
        assert_eq!(valid.len(), 1);
        let t = &valid[0];
        assert_eq!(
            (t.height, t.epoch, t.signing_attempt, t.retry_until_height),
            (12, 3, 2, 99)
        );
        assert_eq!(t.tx_array[0].coin.amount_u64().unwrap(), 777);
        assert_eq!(t.tx_array[0].out_vout, 1);
        assert_eq!(t.tx_array[0].vault_path_index, 5);
        assert!(t.has_unsigned_item());
    }

    #[test]
    fn derive_party_filters_and_sorts() {
        let vault = Vault {
            pub_key: "vaultpk".to_string(),
            pub_key_eddsa: String::new(),
            membership: vec!["pk_b".to_string(), "pk_a".to_string(), "pk_c".to_string()],
        };
        let node = |pk: &str, status: &str, member: bool| NodeAccount {
            node_address: format!("addr_{pk}"),
            status: status.to_string(),
            pub_key_set: PubKeySet {
                secp256k1: pk.to_string(),
                ed25519: String::new(),
            },
            signer_membership: if member {
                vec!["VAULTPK".to_string()]
            } else {
                vec![]
            },
            peer_id: String::new(),
            version: String::new(),
        };
        let nodes = vec![
            node("pk_b", "Active", true),
            node("pk_a", "Active", true),
            node("pk_c", "Standby", true),
            node("pk_d", "Active", true),
            node("pk_e", "Active", false),
        ];
        let party = derive_keysign_party("vaultpk", &vault, &nodes).unwrap();
        assert_eq!(party, vec!["pk_a".to_string(), "pk_b".to_string()]);
    }

    #[test]
    fn config_values_flatten_and_lookup() {
        let body = br#"{
            "MaxOutboundAttempts": 5,
            "SigningPeriod": { "value": "30" },
            "chains": { "BTC": 7, "notanumber": "x" }
        }"#;
        let values = decode_config_int64_values(body).unwrap();
        assert_eq!(values.get("MaxOutboundAttempts"), Some(&5));
        assert_eq!(values.get("SigningPeriod"), Some(&30));
        assert_eq!(values.get("chains_BTC"), Some(&7));
        assert_eq!(lookup_config_value(&values, "signing-period"), Some(30));
        assert_eq!(lookup_config_value(&values, "BTC"), Some(7));
        assert_eq!(lookup_config_value(&values, "missing"), None);
    }

    #[test]
    fn node_status_accepts_string_or_number() {
        let n: NodeAccount =
            serde_json::from_str(r#"{"status":"Active","pub_key_set":{"secp256k1":"pk"}}"#)
                .unwrap();
        assert!(n.is_active());
        assert_eq!(n.pub_key_set.secp256k1, "pk");
        let n: NodeAccount = serde_json::from_str(r#"{"status":4}"#).unwrap();
        assert!(!n.is_active());
        assert_eq!(n.status, "4");
    }

    #[test]
    fn network_fee_accepts_string_or_number() {
        let f: NetworkFee = serde_json::from_str(
            r#"{"chain":"BTC","transaction_size":"250","transaction_fee_rate":12}"#,
        )
        .unwrap();
        assert_eq!((f.transaction_size, f.transaction_fee_rate), (250, 12));
    }

    /// Real vector from the thornado-e2e chain: this bech32 vault pubkey and
    /// its keyshare's `public_key_compressed` hex refer to the same key.
    #[test]
    fn bech32_pubkey_decodes_to_compressed_secp() {
        let bech = "tthorpub1addwnpepq0q8zefgywacpyulgdd3a0lyleqjt9yxfj9kmgn0us2q5cspk86ey8jvh0s";
        let key = decode_bech32_pubkey(bech).unwrap();
        assert_eq!(
            hex::encode(&key),
            "03c071652823bb80939f435b1ebfe4fe412594864c8b6da26fe4140a6201b1f592"
        );
    }

    #[test]
    fn bech32_pubkey_round_trips() {
        let bech = "tthorpub1addwnpepq0q8zefgywacpyulgdd3a0lyleqjt9yxfj9kmgn0us2q5cspk86ey8jvh0s";
        let key = decode_bech32_pubkey(bech).unwrap();
        assert_eq!(encode_bech32_pubkey("tthorpub", &key).unwrap(), bech);
    }

    #[test]
    fn bech32_account_decodes_to_20_bytes() {
        // validator5's account address on the e2e chain.
        let addr = decode_bech32_account("tthor1va6tuv96gxerfupz4lk0e0xxm6amy82dsdpzur").unwrap();
        assert_eq!(addr.len(), 20);
        assert!(decode_bech32_account("tthorpub1addwnpepq0q8zefgywacpyulgdd3a0lyleqjt9yxfj9kmgn0us2q5cspk86ey8jvh0s").is_err());
    }
}
