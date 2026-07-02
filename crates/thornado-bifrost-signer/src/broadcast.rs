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
pub struct ThornadoObservationClient {
    cfg: ChainConfig,
    /// bech32 signer address (the node's own account) — the tx signer.
    signer_address: String,
    gas_limit: u64,
    broadcast_mode: String,
}

impl ThornadoObservationClient {
    pub fn new(cfg: ChainConfig, signer_address: impl Into<String>) -> Self {
        Self {
            cfg,
            signer_address: signer_address.into(),
            gas_limit: BROADCAST_GAS_LIMIT,
            broadcast_mode: BROADCAST_MODE.to_string(),
        }
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

    /// Sign and broadcast an observation to thornado.
    ///
    /// NOT IMPLEMENTED — returns [`BroadcastError::Unimplemented`] rather than
    /// faking a signature. A correct implementation needs the `cosmrs` crate
    /// (not currently a dependency) and would:
    ///
    /// 1. `GET` [`Self::auth_account_url`] and parse the account number +
    ///    sequence (Go `getAccountNumberAndSequenceNumber`).
    /// 2. Build a protobuf `TxBody` wrapping the `Any`-encoded observation
    ///    message ([`ObservationKind::type_url`]) whose payload is this
    ///    [`TxIn`] re-encoded as the chain's proto (`MsgObservedTxIn`/`Out`),
    ///    and an `AuthInfo` with the signer's pubkey, `SIGN_MODE_DIRECT`, the
    ///    fetched sequence, and gas limit [`BROADCAST_GAS_LIMIT`].
    /// 3. Assemble a `SignDoc` (body + auth_info + chain-id + account_number),
    ///    hash it, and produce a real secp256k1 signature with the node key.
    /// 4. `POST` the encoded `TxRaw` to [`Self::broadcast_url`] in
    ///    [`BROADCAST_MODE`] (`"sync"`) and read back the `TxHash`, retrying
    ///    on a sequence-mismatch (cosmos code 32) as Go does.
    ///
    /// Steps 2–3 require the chain proto definitions and cosmrs; without them
    /// any signature we produced would be invalid and rejected by thornado, so
    /// we refuse rather than pretend.
    pub async fn broadcast_observation(
        &self,
        _kind: ObservationKind,
        _observation: &TxIn,
    ) -> Result<String> {
        Err(BroadcastError::Unimplemented(
            "cosmos tx signing needs cosmrs",
        ))
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
        let err = client
            .broadcast_observation(ObservationKind::In, &obs)
            .await
            .unwrap_err();
        assert!(matches!(
            err,
            BroadcastError::Unimplemented(m) if m.contains("cosmrs")
        ));
    }
}
