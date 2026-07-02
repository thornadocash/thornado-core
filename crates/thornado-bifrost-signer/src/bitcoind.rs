//! bitcoind JSON-RPC client.
//!
//! Ports the subset of `bifrost/pkg/chainclients/btc/rpc/rpc.go` the signer and
//! observer need: chain tip, block/tx fetch, UTXO listing, mempool checks, and
//! broadcast. Response shapes mirror bitcoind's JSON-RPC (and Go's btcjson),
//! so a live node's replies deserialize directly.
//!
//! The transport (HTTP basic-auth JSON-RPC) is thin; the value is in the typed
//! method surface and response structs, which are unit-tested against captured
//! bitcoind payloads without needing a live node.

use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, thiserror::Error)]
pub enum RpcError {
    #[error("http: {0}")]
    Http(String),
    #[error("rpc error {code}: {message}")]
    Rpc { code: i64, message: String },
    #[error("decode: {0}")]
    Decode(String),
    #[error("empty result")]
    EmptyResult,
}

type Result<T> = std::result::Result<T, RpcError>;

/// JSON-RPC 1.0 request envelope (bitcoind style).
#[derive(Debug, Serialize)]
struct RpcRequest<'a> {
    jsonrpc: &'a str,
    id: &'a str,
    method: &'a str,
    params: Value,
}

#[derive(Debug, Deserialize)]
struct RpcResponse<T> {
    result: Option<T>,
    error: Option<RpcErrorObj>,
}

#[derive(Debug, Deserialize)]
struct RpcErrorObj {
    code: i64,
    message: String,
}

/// A UTXO as returned by `listunspent` (subset; matches btcjson
/// `ListUnspentResult`).
#[derive(Debug, Clone, Deserialize, Serialize, PartialEq)]
pub struct ListUnspentItem {
    pub txid: String,
    pub vout: u32,
    #[serde(default)]
    pub address: String,
    #[serde(rename = "scriptPubKey", default)]
    pub script_pubkey: String,
    pub amount: f64,
    pub confirmations: i64,
    #[serde(default)]
    pub spendable: bool,
}

/// A mempool entry (subset of `getmempoolentry`).
#[derive(Debug, Clone, Deserialize, PartialEq)]
pub struct MempoolEntry {
    #[serde(rename = "ancestorcount", default)]
    pub ancestor_count: i64,
    #[serde(rename = "descendantcount", default)]
    pub descendant_count: i64,
}

/// A verbose transaction output (subset of `gettxout`).
#[derive(Debug, Clone, Deserialize, PartialEq)]
pub struct TxOutInfo {
    pub confirmations: i64,
    pub value: f64,
}

/// scriptPubKey in a verbose tx vout.
#[derive(Debug, Clone, Deserialize, Default)]
pub struct ScriptPubKey {
    #[serde(default)]
    pub hex: String,
    /// bitcoind >= 22 returns a single `address`; older returns `addresses`.
    #[serde(default)]
    pub address: Option<String>,
    #[serde(default)]
    pub addresses: Vec<String>,
    #[serde(rename = "type", default)]
    pub script_type: String,
}

/// A verbose tx input.
#[derive(Debug, Clone, Deserialize, Default)]
pub struct VerboseVin {
    /// absent for coinbase inputs
    #[serde(default)]
    pub txid: Option<String>,
    #[serde(default)]
    pub vout: Option<u32>,
}

/// A verbose tx output.
#[derive(Debug, Clone, Deserialize, Default)]
pub struct VerboseVout {
    pub value: f64,
    pub n: u32,
    #[serde(rename = "scriptPubKey", default)]
    pub script_pubkey: ScriptPubKey,
}

/// A transaction as returned by verbose `getrawtransaction` / `getblock`
/// verbosity=2.
#[derive(Debug, Clone, Deserialize, Default)]
pub struct VerboseTx {
    pub txid: String,
    #[serde(default)]
    pub vin: Vec<VerboseVin>,
    #[serde(default)]
    pub vout: Vec<VerboseVout>,
}

/// A block with full transaction detail (`getblock` verbosity=2).
#[derive(Debug, Clone, Deserialize, Default)]
pub struct VerboseBlock {
    pub hash: String,
    pub height: i64,
    #[serde(rename = "previousblockhash", default)]
    pub previous_block_hash: String,
    #[serde(default)]
    pub tx: Vec<VerboseTx>,
}

/// Config for reaching bitcoind's JSON-RPC.
#[derive(Debug, Clone)]
pub struct BitcoindConfig {
    pub host: String, // "127.0.0.1:18443"
    pub user: String,
    pub password: String,
    /// Optional wallet name; when set, requests target /wallet/<name>.
    pub wallet: Option<String>,
}

impl BitcoindConfig {
    fn url(&self) -> String {
        let base = if self.host.starts_with("http") {
            self.host.clone()
        } else {
            format!("http://{}", self.host)
        };
        match &self.wallet {
            Some(w) => format!("{base}/wallet/{w}"),
            None => base,
        }
    }
}

/// bitcoind JSON-RPC client.
pub struct BitcoindRpc {
    cfg: BitcoindConfig,
    http: reqwest::Client,
}

impl BitcoindRpc {
    pub fn new(cfg: BitcoindConfig) -> Self {
        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(30))
            .build()
            .expect("reqwest client");
        Self { cfg, http }
    }

    /// Encode a JSON-RPC request body (exposed for testing the wire shape).
    pub fn encode_request(method: &str, params: Value) -> Vec<u8> {
        serde_json::to_vec(&RpcRequest {
            jsonrpc: "1.0",
            id: "thornado-bifrost",
            method,
            params,
        })
        .expect("serialize rpc request")
    }

    /// Decode a JSON-RPC response body into the typed result. A missing/null
    /// result is an error (use [`decode_optional`] for methods where null is a
    /// valid answer, e.g. `gettxout`).
    pub fn decode_response<T: for<'de> Deserialize<'de>>(body: &[u8]) -> Result<T> {
        Self::decode_optional(body)?.ok_or(RpcError::EmptyResult)
    }

    /// Decode a response whose `result` may legitimately be null → `Ok(None)`.
    pub fn decode_optional<T: for<'de> Deserialize<'de>>(body: &[u8]) -> Result<Option<T>> {
        let resp: RpcResponse<T> =
            serde_json::from_slice(body).map_err(|e| RpcError::Decode(e.to_string()))?;
        if let Some(e) = resp.error {
            return Err(RpcError::Rpc {
                code: e.code,
                message: e.message,
            });
        }
        Ok(resp.result)
    }

    async fn call<T: for<'de> Deserialize<'de>>(&self, method: &str, params: Value) -> Result<T> {
        let body = Self::encode_request(method, params);
        let resp = self
            .http
            .post(self.cfg.url())
            .basic_auth(&self.cfg.user, Some(&self.cfg.password))
            .header("content-type", "application/json")
            .body(body)
            .send()
            .await
            .map_err(|e| RpcError::Http(e.to_string()))?;
        let bytes = resp.bytes().await.map_err(|e| RpcError::Http(e.to_string()))?;
        Self::decode_response(&bytes)
    }

    pub async fn get_block_count(&self) -> Result<i64> {
        self.call("getblockcount", Value::Array(vec![])).await
    }

    pub async fn get_block_hash(&self, height: i64) -> Result<String> {
        self.call("getblockhash", serde_json::json!([height])).await
    }

    pub async fn list_unspent(&self, addresses: &[String]) -> Result<Vec<ListUnspentItem>> {
        // listunspent minconf=0 maxconf=9999999 [addrs]
        self.call(
            "listunspent",
            serde_json::json!([0, 9_999_999, addresses]),
        )
        .await
    }

    pub async fn get_mempool_entry(&self, txid: &str) -> Result<MempoolEntry> {
        self.call("getmempoolentry", serde_json::json!([txid])).await
    }

    pub async fn get_tx_out(&self, txid: &str, vout: u32) -> Result<Option<TxOutInfo>> {
        // gettxout returns null when the output is spent/unknown.
        let body = Self::encode_request("gettxout", serde_json::json!([txid, vout]));
        let resp = self
            .http
            .post(self.cfg.url())
            .basic_auth(&self.cfg.user, Some(&self.cfg.password))
            .header("content-type", "application/json")
            .body(body)
            .send()
            .await
            .map_err(|e| RpcError::Http(e.to_string()))?;
        let bytes = resp.bytes().await.map_err(|e| RpcError::Http(e.to_string()))?;
        Self::decode_optional(&bytes)
    }

    pub async fn get_raw_mempool(&self) -> Result<Vec<String>> {
        self.call("getrawmempool", Value::Array(vec![])).await
    }

    /// Broadcast a raw transaction (hex); returns the txid.
    pub async fn send_raw_transaction(&self, tx_hex: &str) -> Result<String> {
        self.call("sendrawtransaction", serde_json::json!([tx_hex]))
            .await
    }

    /// Fetch a block with full transaction detail (`getblock <hash> 2`).
    pub async fn get_block_verbose_txs(&self, hash: &str) -> Result<VerboseBlock> {
        self.call("getblock", serde_json::json!([hash, 2])).await
    }

    /// Fetch a verbose transaction (`getrawtransaction <txid> true`).
    pub async fn get_raw_transaction(&self, txid: &str) -> Result<VerboseTx> {
        self.call("getrawtransaction", serde_json::json!([txid, true]))
            .await
    }
}

impl ScriptPubKey {
    /// The single receiver address, if the output has exactly one.
    pub fn single_address(&self) -> Option<String> {
        if let Some(a) = &self.address {
            return Some(a.clone());
        }
        if self.addresses.len() == 1 {
            return Some(self.addresses[0].clone());
        }
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn url_composition() {
        let mut cfg = BitcoindConfig {
            host: "127.0.0.1:18443".into(),
            user: "u".into(),
            password: "p".into(),
            wallet: None,
        };
        assert_eq!(cfg.url(), "http://127.0.0.1:18443");
        cfg.wallet = Some("vault".into());
        assert_eq!(cfg.url(), "http://127.0.0.1:18443/wallet/vault");
    }

    #[test]
    fn request_encoding_shape() {
        let body = BitcoindRpc::encode_request("getblockcount", Value::Array(vec![]));
        let v: Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(v["jsonrpc"], "1.0");
        assert_eq!(v["method"], "getblockcount");
        assert!(v["params"].is_array());
    }

    #[test]
    fn decode_scalar_result() {
        let n: i64 = BitcoindRpc::decode_response(br#"{"result":842000,"error":null,"id":"x"}"#)
            .unwrap();
        assert_eq!(n, 842_000);
    }

    #[test]
    fn decode_rpc_error() {
        let e = BitcoindRpc::decode_response::<i64>(
            br#"{"result":null,"error":{"code":-8,"message":"bad"},"id":"x"}"#,
        )
        .unwrap_err();
        assert!(matches!(e, RpcError::Rpc { code: -8, .. }));
    }

    #[test]
    fn decode_listunspent() {
        // Shape as bitcoind returns it (amounts in BTC, scriptPubKey camelCase).
        let body = br#"{"result":[
            {"txid":"aa","vout":0,"address":"bcrt1p","scriptPubKey":"5120aa",
             "amount":0.5,"confirmations":6,"spendable":true},
            {"txid":"bb","vout":1,"scriptPubKey":"5120bb","amount":0.1,"confirmations":0}
        ],"error":null,"id":"x"}"#;
        let utxos: Vec<ListUnspentItem> = BitcoindRpc::decode_response(body).unwrap();
        assert_eq!(utxos.len(), 2);
        assert_eq!(utxos[0].txid, "aa");
        assert_eq!(utxos[0].script_pubkey, "5120aa");
        assert_eq!(utxos[0].confirmations, 6);
        assert!(utxos[0].spendable);
        assert_eq!(utxos[1].confirmations, 0);
        assert!(!utxos[1].spendable); // defaulted
    }

    #[test]
    fn decode_gettxout_null_is_none() {
        let none: Option<TxOutInfo> =
            BitcoindRpc::decode_optional(br#"{"result":null,"error":null,"id":"x"}"#).unwrap();
        assert!(none.is_none());
        let some: Option<TxOutInfo> = BitcoindRpc::decode_optional(
            br#"{"result":{"confirmations":3,"value":0.25},"error":null,"id":"x"}"#,
        )
        .unwrap();
        assert_eq!(some.unwrap().confirmations, 3);
    }

    #[test]
    fn decode_mempool_entry() {
        let e: MempoolEntry = BitcoindRpc::decode_response(
            br#"{"result":{"ancestorcount":2,"descendantcount":1,"vsize":150},"error":null,"id":"x"}"#,
        )
        .unwrap();
        assert_eq!(e.ancestor_count, 2);
        assert_eq!(e.descendant_count, 1);
    }
}
