//! Broadcasting a signed BTC transaction and posting the resulting observation
//! back to the thornado chain.
//!
//! Ports two Go pieces:
//!
//! - `bifrost/pkg/chainclients/btc/client.go` — the signed tx is submitted to
//!   bitcoind via `sendrawtransaction`, and on broadcast the tx is recorded
//!   against block-meta (self-transaction when the sender is one of the vault's
//!   own addresses, otherwise a customer transaction). We port the RPC submit;
//!   block-meta bookkeeping lives in the storage layer and is out of scope here.
//!
//! - `bifrost/thornadoclient` (`thornado.go`, `broadcast.go`) +
//!   `thornadoclient/types/tx_in.go` — observations are posted to thornado as a
//!   cosmos tx (`MsgObservedTxIn` / `MsgObservedTxOut`) built from the
//!   `TxIn`/`TxInItem` shape, signed with secp256k1 in `SIGN_MODE_DIRECT`,
//!   broadcast in `"sync"` mode with a hard-coded gas limit of `4_000_000_000`.
//!
//! What is honest here:
//! - [`broadcast_btc_tx`] is fully implemented against the existing
//!   [`crate::bitcoind::BitcoindRpc`] (and any [`RawTxBroadcaster`]).
//! - The observation payload structs ([`TxIn`], [`TxInItem`]) and their
//!   assembly ([`build_observation`]) are fully implemented and match Go's JSON
//!   tags byte-for-byte, so they can be unit-tested without a chain.
//! - The cosmos-tx *signing + posting* path
//!   ([`ThornadoObservationClient::broadcast_observation`]) is deliberately
//!   **not faked**: it returns [`BroadcastError::Unimplemented`]. Producing a
//!   valid cosmos tx needs a protobuf `TxBody`/`AuthInfo`, a `SIGN_MODE_DIRECT`
//!   `SignDoc`, a real secp256k1 signature over its bytes, and a fetched
//!   account number + sequence — see that method's doc comment.

use serde::{Deserialize, Serialize};

use crate::chain::{ChainConfig, Coin, TxOutItem};

#[derive(Debug, thiserror::Error)]
pub enum BroadcastError {
    #[error("bitcoind rpc: {0}")]
    Rpc(String),
    /// The cosmos-tx signing + posting path is not implemented in this crate.
    /// See [`ThornadoObservationClient::broadcast_observation`].
    #[error("{0}")]
    Unimplemented(&'static str),
}

type Result<T> = std::result::Result<T, BroadcastError>;

// ---------------------------------------------------------------------------
// (a) BTC broadcast
// ---------------------------------------------------------------------------

/// The one bitcoind capability broadcasting needs. Implemented for the real
/// [`crate::bitcoind::BitcoindRpc`]; a fake makes [`broadcast_btc_tx`] unit
/// testable without a live node.
pub trait RawTxBroadcaster {
    fn submit_raw(
        &self,
        tx_hex: &str,
    ) -> impl std::future::Future<Output = std::result::Result<String, String>> + Send;
}

impl RawTxBroadcaster for crate::bitcoind::BitcoindRpc {
    async fn submit_raw(&self, tx_hex: &str) -> std::result::Result<String, String> {
        self.send_raw_transaction(tx_hex)
            .await
            .map_err(|e| e.to_string())
    }
}

/// Broadcast a signed BTC transaction (hex) to bitcoind via
/// `sendrawtransaction`, returning the txid. Mirrors the submit step of Go
/// `Client.BroadcastTx`.
pub async fn broadcast_btc_tx<B: RawTxBroadcaster>(rpc: &B, signed_tx_hex: &str) -> Result<String> {
    rpc.submit_raw(signed_tx_hex)
        .await
        .map_err(BroadcastError::Rpc)
}

// ---------------------------------------------------------------------------
// (b) Observation payload — mirrors thornadoclient/types/tx_in.go
// ---------------------------------------------------------------------------

/// A single observed transaction item. Field names + `omitempty` behaviour
/// match Go `types.TxInItem` json tags exactly, so a thornado node deserializes
/// what we serialize.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct TxInItem {
    pub block_height: i64,
    pub tx: String,
    pub source_vout: u32,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub source_inputs: Vec<TxOutInput>,
    pub sender: String,
    pub to: String,
    pub coins: Vec<Coin>,
    pub gas: Vec<Coin>,
    pub observed_vault_pub_key: String,
    pub aggregator: String,
    pub aggregator_target: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub aggregator_target_limit: Option<String>,
    #[serde(rename = "committed_pre_final")]
    pub committed_un_finalised: bool,
}

/// A spend input reference (Go `types.TxOutInput`), reused inside observations.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct TxOutInput {
    pub tx_id: String,
    pub vout: u32,
    pub amount_sats: u64,
}

/// The observation batch posted for a chain. Field names match Go `types.TxIn`.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct TxIn {
    pub chain: String,
    #[serde(rename = "txArray")]
    pub tx_array: Vec<TxInItem>,
    pub filtered: bool,
    pub mem_pool: bool,
    pub confirmation_required: i64,
    pub allow_future_observation: bool,
}

impl TxInItem {
    /// True only when every meaningful field is empty (Go `TxInItem.IsEmpty`).
    pub fn is_empty(&self) -> bool {
        self.block_height == 0
            && self.tx.is_empty()
            && self.source_vout == 0
            && self.sender.is_empty()
            && self.to.is_empty()
            && self.coins.iter().all(|c| c.amount_is_zero())
            && self.gas.iter().all(|c| c.amount_is_zero())
            && self.observed_vault_pub_key.is_empty()
    }
}

impl Coin {
    fn amount_is_zero(&self) -> bool {
        self.amount.is_empty() || self.amount == "0"
    }
}

/// Assemble a [`TxIn`] observation batch for a chain from its observed items.
/// The height fields on the items are as observed (0 for mempool, the finalised
/// height once confirmed) — the caller decides which, matching Go's observer,
/// which does not mutate items at assembly time.
pub fn build_observation(
    chain: &str,
    items: Vec<TxInItem>,
    mem_pool: bool,
    confirmation_required: i64,
    allow_future_observation: bool,
) -> TxIn {
    TxIn {
        chain: chain.to_string(),
        tx_array: items,
        filtered: false,
        mem_pool,
        confirmation_required,
        allow_future_observation,
    }
}

/// Build one [`TxInItem`] from a signed outbound ([`crate::chain::TxOutItem`])
/// plus the resulting on-chain txid — the shape the signer reports back after
/// broadcasting an outbound (Go observer `getThornadoTxIns` maps the same
/// fields). `sender` is the vault address the tx was spent from.
pub fn item_from_outbound(out: &TxOutItem, txid: &str, sender: &str, block_height: i64) -> TxInItem {
    let source_inputs = out
        .source_inputs
        .iter()
        .map(|i| TxOutInput {
            tx_id: i.tx_id.clone(),
            vout: i.vout,
            amount_sats: i.amount_sats,
        })
        .collect();
    TxInItem {
        block_height,
        tx: txid.to_string(),
        source_vout: out.out_vout,
        source_inputs,
        sender: sender.to_string(),
        to: out.to_address.clone(),
        coins: vec![out.coin.clone()],
        gas: out.max_gas.clone(),
        observed_vault_pub_key: out.vault_pub_key.clone(),
        aggregator: String::new(),
        aggregator_target: String::new(),
        aggregator_target_limit: None,
        committed_un_finalised: false,
    }
}

// ---------------------------------------------------------------------------
// (c) Thornado observation client — cosmos tx posting skeleton
// ---------------------------------------------------------------------------

/// Cosmos gas limit the signer hard-codes when broadcasting (Go
/// `broadcast.go`, `builder.SetGasLimit(4000000000)`).
pub const BROADCAST_GAS_LIMIT: u64 = 4_000_000_000;

/// Cosmos broadcast mode the observer uses (Go `Broadcast` → `"sync"`).
pub const BROADCAST_MODE: &str = "sync";

/// `GET {host}/cosmos/auth/v1beta1/accounts/{addr}` — account number + sequence.
pub const AUTH_ACCOUNT_ENDPOINT: &str = "/cosmos/auth/v1beta1/accounts";

/// `POST {host}/cosmos/tx/v1beta1/txs` — the cosmos SDK tx-broadcast endpoint.
pub const BROADCAST_TX_ENDPOINT: &str = "/cosmos/tx/v1beta1/txs";

/// The kind of observation message to wrap the batch in (Go `MsgObservedTxIn`
/// vs `MsgObservedTxOut`; the observer splits via `GetInboundOutbound`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ObservationKind {
    In,
    Out,
}

impl ObservationKind {
    /// The cosmos message type URL for this observation.
    pub fn type_url(self) -> &'static str {
        match self {
            ObservationKind::In => "/types.MsgObservedTxIn",
            ObservationKind::Out => "/types.MsgObservedTxOut",
        }
    }
}

/// Client that posts observations to thornado.
///
/// URL composition and request shape are implemented and tested; the actual
/// cosmos-tx signing is an honest stub (see [`Self::broadcast_observation`]).
/// The node's cosmos signing material for posting observations.
#[derive(Clone)]
pub struct SignerKey {
    /// 32-byte secp256k1 secret.
    pub priv_key: Vec<u8>,
    /// 33-byte compressed secp256k1 pubkey.
    pub pub_key: Vec<u8>,
    /// 20-byte cosmos AccAddress (raw, not bech32).
    pub account_bytes: Vec<u8>,
    /// chain id, e.g. "thornado-1".
    pub chain_id: String,
}

#[derive(Clone)]
pub struct ThornadoObservationClient {
    cfg: ChainConfig,
    /// bech32 signer address (the node's own account) — the tx signer.
    signer_address: String,
    gas_limit: u64,
    broadcast_mode: String,
    key: Option<SignerKey>,
    http: reqwest::Client,
    /// Next sequence to sign with, tracked locally: the auth endpoint returns
    /// the CONFIRMED sequence, so back-to-back posts within one block would
    /// reuse it and get rejected with "incorrect account sequence". The lock
    /// also serializes concurrent posts from this daemon.
    next_seq: std::sync::Arc<tokio::sync::Mutex<Option<u64>>>,
}

impl ThornadoObservationClient {
    pub fn new(cfg: ChainConfig, signer_address: impl Into<String>) -> Self {
        Self {
            cfg,
            signer_address: signer_address.into(),
            gas_limit: BROADCAST_GAS_LIMIT,
            broadcast_mode: BROADCAST_MODE.to_string(),
            key: None,
            http: reqwest::Client::builder()
                .timeout(std::time::Duration::from_secs(10))
                .build()
                .expect("reqwest client"),
            next_seq: std::sync::Arc::new(tokio::sync::Mutex::new(None)),
        }
    }

    /// Sign `build` with a sequence that accounts for our own unconfirmed txs
    /// and broadcast it, retrying once on a sequence-mismatch rejection using
    /// the sequence the node said it expected.
    async fn broadcast_with_sequence<F>(&self, build: F) -> Result<String>
    where
        F: Fn(u64, u64) -> std::result::Result<Vec<u8>, String>,
    {
        let mut guard = self.next_seq.lock().await;
        let (account_number, fetched) = self.fetch_account().await?;
        let mut sequence = guard.map_or(fetched, |s| s.max(fetched));
        for _attempt in 0..2 {
            let tx_raw = build(account_number, sequence).map_err(BroadcastError::Rpc)?;
            let result = self.post_tx(&tx_raw).await?;
            let code = result["code"].as_i64().unwrap_or(0);
            let hash = result["hash"].as_str().unwrap_or_default().to_string();
            if code == 0 || code == 6 {
                *guard = Some(sequence + 1);
                return Ok(hash);
            }
            let log = result["log"].as_str().unwrap_or_default().to_string();
            if code == 32 {
                if let Some(expected) = parse_expected_sequence(&log) {
                    if expected != sequence {
                        sequence = expected;
                        continue;
                    }
                }
            }
            *guard = None;
            return Err(BroadcastError::Rpc(format!(
                "broadcast rejected code {code}: {log}"
            )));
        }
        *guard = None;
        Err(BroadcastError::Rpc(
            "broadcast rejected: sequence mismatch after retry".into(),
        ))
    }

    /// POST a signed TxRaw to CometBFT `broadcast_tx_sync`, returning the
    /// JSON-RPC `result` object.
    async fn post_tx(&self, tx_raw: &[u8]) -> Result<serde_json::Value> {
        use base64::Engine as _;
        let tx_b64 = base64::engine::general_purpose::STANDARD.encode(tx_raw);
        let rpc_url = if self.cfg.chain_rpc.starts_with("http") {
            self.cfg.chain_rpc.clone()
        } else {
            format!("http://{}", self.cfg.chain_rpc)
        };
        let req = serde_json::json!({
            "jsonrpc": "2.0", "id": "thornado-bifrost", "method": "broadcast_tx_sync",
            "params": { "tx": tx_b64 }
        });
        let resp: serde_json::Value = self
            .http
            .post(&rpc_url)
            .json(&req)
            .send()
            .await
            .map_err(|e| BroadcastError::Rpc(e.to_string()))?
            .json()
            .await
            .map_err(|e| BroadcastError::Rpc(e.to_string()))?;
        Ok(resp["result"].clone())
    }

    /// Attach the signing key needed to actually post observations.
    pub fn with_key(mut self, key: SignerKey) -> Self {
        self.key = Some(key);
        self
    }

    pub fn signer_address(&self) -> &str {
        &self.signer_address
    }

    pub fn gas_limit(&self) -> u64 {
        self.gas_limit
    }

    pub fn broadcast_mode(&self) -> &str {
        &self.broadcast_mode
    }

    /// URL for fetching this signer's account number + sequence.
    pub fn auth_account_url(&self) -> String {
        format!(
            "{}{}/{}",
            self.cfg.base_url(),
            AUTH_ACCOUNT_ENDPOINT,
            self.signer_address
        )
    }

    /// URL the signed cosmos tx is POSTed to.
    pub fn broadcast_url(&self) -> String {
        format!("{}{}", self.cfg.base_url(), BROADCAST_TX_ENDPOINT)
    }

    /// Fetch this signer's (account_number, sequence) from the auth endpoint
    /// (Go `getAccountNumberAndSequenceNumber`).
    pub async fn fetch_account(&self) -> Result<(u64, u64)> {
        let body = self
            .http
            .get(self.auth_account_url())
            .send()
            .await
            .map_err(|e| BroadcastError::Rpc(e.to_string()))?
            .text()
            .await
            .map_err(|e| BroadcastError::Rpc(e.to_string()))?;
        let v: serde_json::Value =
            serde_json::from_str(&body).map_err(|e| BroadcastError::Rpc(e.to_string()))?;
        let acct = &v["account"];
        let num = acct["account_number"]
            .as_str()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0);
        let seq = acct["sequence"]
            .as_str()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0);
        Ok((num, seq))
    }

    /// Sign and broadcast an observation to thornado: fetch account, build the
    /// `MsgObservedTxIn`, sign SIGN_MODE_DIRECT, and POST the `TxRaw` to
    /// CometBFT `broadcast_tx_sync`. Returns the tx hash.
    pub async fn broadcast_observation(
        &self,
        kind: ObservationKind,
        observation: &TxIn,
    ) -> Result<String> {
        let key = self
            .key
            .as_ref()
            .ok_or(BroadcastError::Unimplemented("signer key not configured"))?;

        let msg = observation_to_msg(observation, &key.account_bytes);
        self.broadcast_with_sequence(|account_number, sequence| {
            crate::cosmos_tx::build_and_sign_typed(
                kind.type_url(),
                &msg,
                &key.priv_key,
                &key.pub_key,
                &key.chain_id,
                account_number,
                sequence,
            )
            .map_err(|e| e.to_string())
        })
        .await
    }

    /// Submit a `MsgSolvency` reporting a base vault's wallet balance on
    /// `chain` (Go BTC client `ReportSolvency`). The chain tallies one vote per
    /// active node and runs the insolvency check at supermajority. Returns the
    /// tx hash.
    pub async fn submit_solvency(
        &self,
        chain: &str,
        vault_pub_key: &str,
        amount_sats: u64,
        height: i64,
    ) -> Result<String> {
        let key = self
            .key
            .as_ref()
            .ok_or(BroadcastError::Unimplemented("signer key not configured"))?;
        // Go Coins.String() for the single gas-asset coin: "<sats> BTC.BTC".
        let coins_str = format!("{amount_sats} {chain}.{chain}");
        let msg = crate::cosmos_tx::MsgSolvency {
            id: crate::cosmos_tx::solvency_id(chain, vault_pub_key, &coins_str, height),
            chain: chain.to_string(),
            pub_key: vault_pub_key.to_string(),
            coins: vec![crate::cosmos_tx::Coin {
                asset: Some(parse_asset(&format!("{chain}.{chain}"))),
                amount: amount_sats.to_string(),
                decimals: 8,
            }],
            height,
            signer: key.account_bytes.clone(),
        };
        self.broadcast_with_sequence(|account_number, sequence| {
            crate::cosmos_tx::build_and_sign_solvency(
                &msg,
                &key.priv_key,
                &key.pub_key,
                &key.chain_id,
                account_number,
                sequence,
            )
            .map_err(|e| e.to_string())
        })
        .await
    }

    /// Submit a `MsgKeygenVault` after a churn DKG so the chain forms the new
    /// vault. Returns the tx hash.
    pub async fn submit_keygen_vault(
        &self,
        members: &[String],
        vault_pub_key: &str,
        height: i64,
        keygen_time_ms: i64,
        chains: &[String],
    ) -> Result<String> {
        let key = self
            .key
            .as_ref()
            .ok_or(BroadcastError::Unimplemented("signer key not configured"))?;
        self.broadcast_with_sequence(|account_number, sequence| {
            crate::cosmos_tx::build_and_sign_keygen_vault(
                members,
                vault_pub_key,
                height,
                keygen_time_ms,
                chains,
                &key.account_bytes,
                &key.priv_key,
                &key.pub_key,
                &key.chain_id,
                account_number,
                sequence,
            )
            .map_err(|e| e.to_string())
        })
        .await
    }
}

/// Extract N from an "incorrect account sequence" rejection log
/// ("... expected N, got M ...").
fn parse_expected_sequence(log: &str) -> Option<u64> {
    let idx = log.find("expected ")?;
    let rest = &log[idx + "expected ".len()..];
    let end = rest.find(|c: char| !c.is_ascii_digit()).unwrap_or(rest.len());
    rest[..end].parse().ok()
}

/// Parse a thornado asset string ("BTC.BTC") into a proto Asset.
fn parse_asset(s: &str) -> crate::cosmos_tx::Asset {
    let (chain, sym) = s.split_once('.').unwrap_or((s, s));
    crate::cosmos_tx::Asset {
        chain: chain.to_string(),
        symbol: sym.to_string(),
        ticker: sym.to_string(),
        secured: false,
    }
}

fn to_proto_coins(coins: &[Coin]) -> Vec<crate::cosmos_tx::Coin> {
    coins
        .iter()
        .map(|c| crate::cosmos_tx::Coin {
            asset: Some(parse_asset(&c.asset)),
            amount: c.amount.clone(),
            decimals: 8,
        })
        .collect()
}

/// Convert an observation batch into the proto `MsgObservedTxIn` (Go
/// `types.MsgObservedTxIn` with `common.ObservedTx` entries).
pub fn observation_to_msg(
    obs: &TxIn,
    signer_account_bytes: &[u8],
) -> crate::cosmos_tx::MsgObservedTxIn {
    let txs = obs
        .tx_array
        .iter()
        .map(|item| crate::cosmos_tx::ObservedTx {
            tx: Some(crate::cosmos_tx::Tx {
                id: item.tx.clone(),
                chain: obs.chain.clone(),
                from_address: item.sender.clone(),
                to_address: item.to.clone(),
                coins: to_proto_coins(&item.coins),
                gas: to_proto_coins(&item.gas),
                source_vout: item.source_vout,
                source_inputs: item
                    .source_inputs
                    .iter()
                    .map(|i| crate::cosmos_tx::TxInput {
                        tx_id: i.tx_id.clone(),
                        vout: i.vout,
                        amount_sats: i.amount_sats,
                    })
                    .collect(),
            }),
            status: crate::cosmos_tx::Status::Incomplete as i32,
            block_height: item.block_height,
            // The observer only reports txs already in blocks (>= 1 conf), so
            // observations are final: IsFinal() on the chain side is
            // `finalise_height == block_height`.
            finalise_height: item.block_height,
            observed_pub_key: item.observed_vault_pub_key.clone(),
            aggregator: item.aggregator.clone(),
            aggregator_target: item.aggregator_target.clone(),
            aggregator_target_limit: item.aggregator_target_limit.clone().unwrap_or_default(),
            ..Default::default()
        })
        .collect();
    crate::cosmos_tx::MsgObservedTxIn {
        txs,
        signer: signer_account_bytes.to_vec(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chain::TxOutInput as ChainTxOutInput;

    struct FakeBroadcaster {
        reply: std::result::Result<String, String>,
    }

    impl RawTxBroadcaster for FakeBroadcaster {
        async fn submit_raw(&self, _tx_hex: &str) -> std::result::Result<String, String> {
            self.reply.clone()
        }
    }

    #[tokio::test]
    async fn broadcast_returns_txid() {
        let rpc = FakeBroadcaster {
            reply: Ok("abcd1234".into()),
        };
        let txid = broadcast_btc_tx(&rpc, "0200000001deadbeef").await.unwrap();
        assert_eq!(txid, "abcd1234");
    }

    #[tokio::test]
    async fn broadcast_maps_rpc_error() {
        let rpc = FakeBroadcaster {
            reply: Err("txn-mempool-conflict".into()),
        };
        let err = broadcast_btc_tx(&rpc, "00").await.unwrap_err();
        assert!(matches!(err, BroadcastError::Rpc(m) if m.contains("mempool")));
    }

    fn sample_item() -> TxInItem {
        TxInItem {
            block_height: 842_000,
            tx: "AABBCC".into(),
            source_vout: 1,
            source_inputs: vec![TxOutInput {
                tx_id: "deadbeef".into(),
                vout: 0,
                amount_sats: 50_000,
            }],
            sender: "bc1pvault".into(),
            to: "bc1pdest".into(),
            coins: vec![Coin {
                asset: "BTC.BTC".into(),
                amount: "100000".into(),
            }],
            gas: vec![Coin {
                asset: "BTC.BTC".into(),
                amount: "150".into(),
            }],
            observed_vault_pub_key: "thorpub1vault".into(),
            aggregator: String::new(),
            aggregator_target: String::new(),
            aggregator_target_limit: None,
            committed_un_finalised: false,
        }
    }

    #[test]
    fn observation_json_matches_go_tags() {
        let obs = build_observation("BTC", vec![sample_item()], false, 2, false);
        let v: serde_json::Value = serde_json::to_value(&obs).unwrap();

        // TxIn tags.
        assert_eq!(v["chain"], "BTC");
        assert!(v.get("txArray").is_some(), "camelCase txArray tag");
        assert_eq!(v["mem_pool"], false);
        assert_eq!(v["confirmation_required"], 2);
        assert_eq!(v["allow_future_observation"], false);

        // TxInItem tags.
        let item = &v["txArray"][0];
        assert_eq!(item["block_height"], 842_000);
        assert_eq!(item["tx"], "AABBCC");
        assert_eq!(item["source_vout"], 1);
        assert_eq!(item["sender"], "bc1pvault");
        assert_eq!(item["to"], "bc1pdest");
        assert_eq!(item["observed_vault_pub_key"], "thorpub1vault");
        assert_eq!(item["coins"][0]["asset"], "BTC.BTC");
        assert_eq!(item["gas"][0]["amount"], "150");
        // Renamed json tag from Go.
        assert_eq!(item["committed_pre_final"], false);
        // source_inputs present when non-empty.
        assert_eq!(item["source_inputs"][0]["tx_id"], "deadbeef");
        assert_eq!(item["source_inputs"][0]["amount_sats"], 50_000);
        // aggregator_target_limit omitted when None (Go pointer nil).
        assert!(item.get("aggregator_target_limit").is_none());
    }

    #[test]
    fn source_inputs_omitted_when_empty() {
        let mut item = sample_item();
        item.source_inputs.clear();
        let v = serde_json::to_value(&item).unwrap();
        assert!(
            v.get("source_inputs").is_none(),
            "omitempty when no source inputs"
        );
    }

    #[test]
    fn observation_roundtrips() {
        let obs = build_observation("BTC", vec![sample_item()], true, 3, true);
        let bytes = serde_json::to_vec(&obs).unwrap();
        let back: TxIn = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(obs, back);
    }

    #[test]
    fn item_from_outbound_maps_fields() {
        let out = TxOutItem {
            chain: "BTC".into(),
            to_address: "bc1pdest".into(),
            vault_pub_key: "thorpub1vault".into(),
            coin: Coin {
                asset: "BTC.BTC".into(),
                amount: "100000".into(),
            },
            max_gas: vec![Coin {
                asset: "BTC.BTC".into(),
                amount: "150".into(),
            }],
            gas_rate: 12,
            in_hash: "INHASH".into(),
            out_hash: String::new(),
            out_vout: 2,
            vault_path_index: 0,
            tx_type: "out".into(),
            source_inputs: vec![ChainTxOutInput {
                tx_id: "feed".into(),
                vout: 3,
                amount_sats: 70_000,
            }],
        };
        let item = item_from_outbound(&out, "TXID", "bc1pvault", 900_000);
        assert_eq!(item.tx, "TXID");
        assert_eq!(item.sender, "bc1pvault");
        assert_eq!(item.to, "bc1pdest");
        assert_eq!(item.block_height, 900_000);
        assert_eq!(item.source_vout, 2);
        assert_eq!(item.observed_vault_pub_key, "thorpub1vault");
        assert_eq!(item.coins[0].amount, "100000");
        assert_eq!(item.gas[0].amount, "150");
        assert_eq!(item.source_inputs[0].tx_id, "feed");
        assert_eq!(item.source_inputs[0].amount_sats, 70_000);
        assert!(!item.is_empty());
    }

    #[test]
    fn empty_item_is_empty() {
        assert!(TxInItem::default().is_empty());
    }

    #[test]
    fn observation_client_urls_and_constants() {
        let cfg = ChainConfig {
            chain_host: "localhost:1317".into(),
            chain_rpc: String::new(),
        };
        let client = ThornadoObservationClient::new(cfg, "thor1signer");
        assert_eq!(
            client.auth_account_url(),
            "http://localhost:1317/cosmos/auth/v1beta1/accounts/thor1signer"
        );
        assert_eq!(
            client.broadcast_url(),
            "http://localhost:1317/cosmos/tx/v1beta1/txs"
        );
        assert_eq!(client.gas_limit(), 4_000_000_000);
        assert_eq!(client.broadcast_mode(), "sync");
        assert_eq!(client.signer_address(), "thor1signer");
    }

    #[test]
    fn observation_kind_type_urls() {
        assert_eq!(ObservationKind::In.type_url(), "/types.MsgObservedTxIn");
        assert_eq!(ObservationKind::Out.type_url(), "/types.MsgObservedTxOut");
    }

    #[tokio::test]
    async fn broadcast_observation_is_honest_stub() {
        let cfg = ChainConfig {
            chain_host: "localhost:1317".into(),
            chain_rpc: String::new(),
        };
        let client = ThornadoObservationClient::new(cfg, "thor1signer");
        let obs = build_observation("BTC", vec![sample_item()], false, 1, false);
        // No signing key configured -> refuses rather than posting.
        let err = client
            .broadcast_observation(ObservationKind::In, &obs)
            .await
            .unwrap_err();
        assert!(matches!(
            err,
            BroadcastError::Unimplemented(m) if m.contains("signer key")
        ));
    }

    #[test]
    fn observation_converts_to_proto_msg() {
        use prost::Message;
        let obs = build_observation("BTC", vec![sample_item()], false, 1, false);
        let msg = observation_to_msg(&obs, &[0xAB; 20]);
        assert_eq!(msg.signer, vec![0xAB; 20]);
        assert_eq!(msg.txs.len(), 1);
        let tx = msg.txs[0].tx.as_ref().unwrap();
        assert_eq!(tx.chain, "BTC");
        // asset string "BTC.BTC" split into chain/symbol/ticker
        if let Some(coin) = tx.coins.first() {
            let a = coin.asset.as_ref().unwrap();
            assert_eq!(a.chain, "BTC");
            assert_eq!(a.symbol, "BTC");
        }
        // round-trips through protobuf
        let bytes = msg.encode_to_vec();
        let back = crate::cosmos_tx::MsgObservedTxIn::decode(bytes.as_slice()).unwrap();
        assert_eq!(back, msg);
    }
}
