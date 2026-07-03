//! Cosmos SIGN_MODE_DIRECT transaction signing for posting observations to
//! thornado — a self-contained port of the Go `thornadoBridge.broadcast` path
//! (cosmos-sdk `clienttx.Sign`, SIGN_MODE_DIRECT, gas 4e9).
//!
//! Uses `prost` for the thornado `MsgObservedTxIn`/`ObservedTx` protobuf and the
//! cosmos `tx`/`crypto` envelope, and the secp256k1 already in the tree for the
//! signature. No cosmos-sdk / cosmrs dependency.
//!
//! Proto field numbers mirror proto/thornado/v1/common/common.proto and
//! .../types/msg_observed_txin.proto exactly.

use prost::Message;

pub const MSG_OBSERVED_TX_IN_TYPE_URL: &str = "/types.MsgObservedTxIn";
pub const MSG_OBSERVED_TX_OUT_TYPE_URL: &str = "/types.MsgObservedTxOut";
pub const MSG_KEYGEN_VAULT_TYPE_URL: &str = "/types.MsgKeygenVault";

/// KeygenType (thornado `types.KeygenType`).
pub const KEYGEN_TYPE_BASE_VAULT: i32 = 1;

/// `MsgKeygenVault` — the message each member submits after a churn DKG so the
/// chain can form the new vault at consensus. Only the fields a successful
/// no-blame keygen needs are encoded (proto tags match Go exactly); the
/// optional blame/backup/check-signature fields are omitted.
#[derive(Clone, PartialEq, Message)]
pub struct MsgKeygenVault {
    #[prost(string, tag = "1")]
    pub id: String,
    #[prost(string, tag = "2")]
    pub vault_pub_key: String,
    #[prost(int32, tag = "3")]
    pub keygen_type: i32,
    #[prost(string, repeated, tag = "4")]
    pub pub_keys: Vec<String>,
    #[prost(int64, tag = "5")]
    pub height: i64,
    #[prost(string, repeated, tag = "7")]
    pub chains: Vec<String>,
    #[prost(bytes = "vec", tag = "8")]
    pub signer: Vec<u8>,
    #[prost(int64, tag = "9")]
    pub keygen_time: i64,
}

/// The frost id for a keygen vault message (Go `getFrostID`):
/// `hex(sha256("m:<sorted member>"… + vault_pub_key + height))`, no blame.
pub fn frost_id(members: &[String], vault_pub_key: &str, height: i64) -> String {
    use sha2::{Digest, Sha256};
    let mut sorted: Vec<&String> = members.iter().collect();
    sorted.sort();
    let mut s = String::new();
    for m in sorted {
        s.push_str("m:");
        s.push_str(m);
    }
    // (no blame pubkeys)
    s.push_str(vault_pub_key);
    s.push_str(&height.to_string());
    hex::encode(Sha256::digest(s.as_bytes()))
}

/// Build and sign a `MsgKeygenVault` as a SIGN_MODE_DIRECT tx.
#[allow(clippy::too_many_arguments)]
pub fn build_and_sign_keygen_vault(
    members: &[String],
    vault_pub_key: &str,
    height: i64,
    keygen_time_ms: i64,
    chains: &[String],
    signer_account: &[u8],
    priv_key: &[u8],
    pub_key: &[u8],
    chain_id: &str,
    account_number: u64,
    sequence: u64,
) -> Result<Vec<u8>, CosmosTxError> {
    let msg = MsgKeygenVault {
        id: frost_id(members, vault_pub_key, height),
        vault_pub_key: vault_pub_key.to_string(),
        keygen_type: KEYGEN_TYPE_BASE_VAULT,
        pub_keys: members.to_vec(),
        height,
        chains: chains.to_vec(),
        signer: signer_account.to_vec(),
        keygen_time: keygen_time_ms,
    };
    build_and_sign_any(
        MSG_KEYGEN_VAULT_TYPE_URL,
        msg.encode_to_vec(),
        priv_key,
        pub_key,
        chain_id,
        account_number,
        sequence,
    )
}
pub const SECP256K1_PUBKEY_TYPE_URL: &str = "/cosmos.crypto.secp256k1.PubKey";
/// Gas limit the Go bridge hard-codes.
pub const GAS_LIMIT: u64 = 4_000_000_000;

// ---------------------------------------------------------------------------
// thornado messages (package `types` / `common`)
// ---------------------------------------------------------------------------

#[derive(Clone, PartialEq, Message)]
pub struct Asset {
    #[prost(string, tag = "1")]
    pub chain: String,
    #[prost(string, tag = "2")]
    pub symbol: String,
    #[prost(string, tag = "3")]
    pub ticker: String,
    #[prost(bool, tag = "6")]
    pub secured: bool,
}

#[derive(Clone, PartialEq, Message)]
pub struct Coin {
    #[prost(message, optional, tag = "1")]
    pub asset: Option<Asset>,
    #[prost(string, tag = "2")]
    pub amount: String,
    #[prost(int64, tag = "3")]
    pub decimals: i64,
}

#[derive(Clone, PartialEq, Message)]
pub struct TxInput {
    #[prost(string, tag = "1")]
    pub tx_id: String,
    #[prost(uint32, tag = "2")]
    pub vout: u32,
    #[prost(uint64, tag = "3")]
    pub amount_sats: u64,
}

#[derive(Clone, PartialEq, Message)]
pub struct Tx {
    #[prost(string, tag = "1")]
    pub id: String,
    #[prost(string, tag = "2")]
    pub chain: String,
    #[prost(string, tag = "3")]
    pub from_address: String,
    #[prost(string, tag = "4")]
    pub to_address: String,
    #[prost(message, repeated, tag = "5")]
    pub coins: Vec<Coin>,
    #[prost(message, repeated, tag = "6")]
    pub gas: Vec<Coin>,
    #[prost(uint32, tag = "7")]
    pub source_vout: u32,
    #[prost(message, repeated, tag = "8")]
    pub source_inputs: Vec<TxInput>,
}

#[derive(Clone, PartialEq, Message)]
pub struct ObservedTx {
    #[prost(message, optional, tag = "1")]
    pub tx: Option<Tx>,
    #[prost(enumeration = "Status", tag = "2")]
    pub status: i32,
    #[prost(string, repeated, tag = "3")]
    pub out_hashes: Vec<String>,
    #[prost(int64, tag = "4")]
    pub block_height: i64,
    #[prost(string, repeated, tag = "5")]
    pub signers: Vec<String>,
    #[prost(string, tag = "6")]
    pub observed_pub_key: String,
    #[prost(int64, tag = "7")]
    pub keysign_ms: i64,
    #[prost(int64, tag = "8")]
    pub finalise_height: i64,
    #[prost(string, tag = "9")]
    pub aggregator: String,
    #[prost(string, tag = "10")]
    pub aggregator_target: String,
    #[prost(string, tag = "11")]
    pub aggregator_target_limit: String,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, prost::Enumeration)]
#[repr(i32)]
pub enum Status {
    Incomplete = 0,
    Done = 1,
    Reverted = 2,
}

#[derive(Clone, PartialEq, Message)]
pub struct MsgObservedTxIn {
    #[prost(message, repeated, tag = "1")]
    pub txs: Vec<ObservedTx>,
    /// AccAddress bytes (the 20-byte account address, not bech32).
    #[prost(bytes = "vec", tag = "2")]
    pub signer: Vec<u8>,
}

// ---------------------------------------------------------------------------
// cosmos tx envelope (cosmos.tx.v1beta1 / cosmos.crypto.secp256k1)
// ---------------------------------------------------------------------------

#[derive(Clone, PartialEq, Message)]
pub struct Any {
    #[prost(string, tag = "1")]
    pub type_url: String,
    #[prost(bytes = "vec", tag = "2")]
    pub value: Vec<u8>,
}

#[derive(Clone, PartialEq, Message)]
pub struct Secp256k1PubKey {
    #[prost(bytes = "vec", tag = "1")]
    pub key: Vec<u8>,
}

#[derive(Clone, PartialEq, Message)]
pub struct TxBody {
    #[prost(message, repeated, tag = "1")]
    pub messages: Vec<Any>,
    #[prost(string, tag = "2")]
    pub memo: String,
    #[prost(uint64, tag = "3")]
    pub timeout_height: u64,
}

#[derive(Clone, PartialEq, Message)]
pub struct ModeInfoSingle {
    /// SignMode enum; SIGN_MODE_DIRECT = 1.
    #[prost(int32, tag = "1")]
    pub mode: i32,
}

#[derive(Clone, PartialEq, Message)]
pub struct ModeInfo {
    #[prost(message, optional, tag = "1")]
    pub single: Option<ModeInfoSingle>,
}

#[derive(Clone, PartialEq, Message)]
pub struct SignerInfo {
    #[prost(message, optional, tag = "1")]
    pub public_key: Option<Any>,
    #[prost(message, optional, tag = "2")]
    pub mode_info: Option<ModeInfo>,
    #[prost(uint64, tag = "3")]
    pub sequence: u64,
}

#[derive(Clone, PartialEq, Message)]
pub struct CosmosCoin {
    #[prost(string, tag = "1")]
    pub denom: String,
    #[prost(string, tag = "2")]
    pub amount: String,
}

#[derive(Clone, PartialEq, Message)]
pub struct Fee {
    #[prost(message, repeated, tag = "1")]
    pub amount: Vec<CosmosCoin>,
    #[prost(uint64, tag = "2")]
    pub gas_limit: u64,
}

#[derive(Clone, PartialEq, Message)]
pub struct AuthInfo {
    #[prost(message, repeated, tag = "1")]
    pub signer_infos: Vec<SignerInfo>,
    #[prost(message, optional, tag = "2")]
    pub fee: Option<Fee>,
}

#[derive(Clone, PartialEq, Message)]
pub struct SignDoc {
    #[prost(bytes = "vec", tag = "1")]
    pub body_bytes: Vec<u8>,
    #[prost(bytes = "vec", tag = "2")]
    pub auth_info_bytes: Vec<u8>,
    #[prost(string, tag = "3")]
    pub chain_id: String,
    #[prost(uint64, tag = "4")]
    pub account_number: u64,
}

#[derive(Clone, PartialEq, Message)]
pub struct TxRaw {
    #[prost(bytes = "vec", tag = "1")]
    pub body_bytes: Vec<u8>,
    #[prost(bytes = "vec", tag = "2")]
    pub auth_info_bytes: Vec<u8>,
    #[prost(bytes = "vec", repeated, tag = "3")]
    pub signatures: Vec<Vec<u8>>,
}

/// SIGN_MODE_DIRECT = 1.
pub const SIGN_MODE_DIRECT: i32 = 1;

#[derive(Debug, thiserror::Error)]
pub enum CosmosTxError {
    #[error("signing: {0}")]
    Signing(String),
}

/// Build and sign a SIGN_MODE_DIRECT tx carrying `msg`, returning the encoded
/// `TxRaw` bytes ready to broadcast (base64 → CometBFT broadcast_tx_sync).
///
/// `priv_key` is the node's 32-byte secp256k1 secret; `pub_key` its 33-byte
/// compressed pubkey. The cosmos signature is ECDSA over SHA256(sign_doc).
pub fn build_and_sign(
    msg: &MsgObservedTxIn,
    priv_key: &[u8],
    pub_key: &[u8],
    chain_id: &str,
    account_number: u64,
    sequence: u64,
) -> Result<Vec<u8>, CosmosTxError> {
    build_and_sign_typed(
        MSG_OBSERVED_TX_IN_TYPE_URL,
        msg,
        priv_key,
        pub_key,
        chain_id,
        account_number,
        sequence,
    )
}

/// Like [`build_and_sign`] but with an explicit message type URL, so the same
/// `MsgObservedTxIn` shape can be posted as `MsgObservedTxOut` (the two proto
/// messages have identical fields).
pub fn build_and_sign_typed(
    type_url: &str,
    msg: &MsgObservedTxIn,
    priv_key: &[u8],
    pub_key: &[u8],
    chain_id: &str,
    account_number: u64,
    sequence: u64,
) -> Result<Vec<u8>, CosmosTxError> {
    build_and_sign_any(
        type_url,
        msg.encode_to_vec(),
        priv_key,
        pub_key,
        chain_id,
        account_number,
        sequence,
    )
}

/// Sign an arbitrary already-encoded message under `type_url` as a
/// SIGN_MODE_DIRECT tx and return the encoded `TxRaw`.
pub fn build_and_sign_any(
    type_url: &str,
    msg_value: Vec<u8>,
    priv_key: &[u8],
    pub_key: &[u8],
    chain_id: &str,
    account_number: u64,
    sequence: u64,
) -> Result<Vec<u8>, CosmosTxError> {
    use bitcoin::secp256k1::{ecdsa, Message as SecpMessage, Secp256k1, SecretKey};
    use sha2::{Digest, Sha256};

    let body = TxBody {
        messages: vec![Any {
            type_url: type_url.to_string(),
            value: msg_value,
        }],
        memo: String::new(),
        timeout_height: 0,
    };
    let body_bytes = body.encode_to_vec();

    let pk_any = Any {
        type_url: SECP256K1_PUBKEY_TYPE_URL.to_string(),
        value: Secp256k1PubKey {
            key: pub_key.to_vec(),
        }
        .encode_to_vec(),
    };
    let auth_info = AuthInfo {
        signer_infos: vec![SignerInfo {
            public_key: Some(pk_any),
            mode_info: Some(ModeInfo {
                single: Some(ModeInfoSingle {
                    mode: SIGN_MODE_DIRECT,
                }),
            }),
            sequence,
        }],
        fee: Some(Fee {
            amount: vec![],
            gas_limit: GAS_LIMIT,
        }),
    };
    let auth_info_bytes = auth_info.encode_to_vec();

    let sign_doc = SignDoc {
        body_bytes: body_bytes.clone(),
        auth_info_bytes: auth_info_bytes.clone(),
        chain_id: chain_id.to_string(),
        account_number,
    };
    let sign_bytes = sign_doc.encode_to_vec();

    // Cosmos secp256k1: ECDSA over SHA256(sign_bytes), 64-byte compact sig.
    let digest: [u8; 32] = Sha256::digest(&sign_bytes).into();
    let secp = Secp256k1::signing_only();
    let sk = SecretKey::from_slice(priv_key).map_err(|e| CosmosTxError::Signing(e.to_string()))?;
    let message = SecpMessage::from_digest(digest);
    let sig: ecdsa::Signature = secp.sign_ecdsa(&message, &sk);
    let compact = sig.serialize_compact().to_vec();

    let tx_raw = TxRaw {
        body_bytes,
        auth_info_bytes,
        signatures: vec![compact],
    };
    Ok(tx_raw.encode_to_vec())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sample_msg() -> MsgObservedTxIn {
        MsgObservedTxIn {
            txs: vec![ObservedTx {
                tx: Some(Tx {
                    id: "ABC123".into(),
                    chain: "BTC".into(),
                    from_address: "bc1pcustomer".into(),
                    to_address: "bc1pvault".into(),
                    coins: vec![Coin {
                        asset: Some(Asset {
                            chain: "BTC".into(),
                            symbol: "BTC".into(),
                            ticker: "BTC".into(),
                            secured: false,
                        }),
                        amount: "150000".into(),
                        decimals: 8,
                    }],
                    gas: vec![],
                    source_vout: 0,
                    source_inputs: vec![],
                }),
                status: Status::Incomplete as i32,
                block_height: 100,
                observed_pub_key: "thorpub1vault".into(),
                ..Default::default()
            }],
            signer: vec![1u8; 20],
        }
    }

    #[test]
    fn msg_encodes_and_roundtrips() {
        let msg = sample_msg();
        let bytes = msg.encode_to_vec();
        assert!(!bytes.is_empty());
        let back = MsgObservedTxIn::decode(bytes.as_slice()).unwrap();
        assert_eq!(back, msg);
        assert_eq!(back.txs[0].tx.as_ref().unwrap().coins[0].amount, "150000");
    }

    #[test]
    fn sign_produces_deterministic_txraw() {
        // fixed key → RFC6979 deterministic ECDSA → stable output
        let sk = [7u8; 32];
        let secp = bitcoin::secp256k1::Secp256k1::signing_only();
        let secret = bitcoin::secp256k1::SecretKey::from_slice(&sk).unwrap();
        let pk = secret.public_key(&secp).serialize().to_vec();

        let a = build_and_sign(&sample_msg(), &sk, &pk, "thornado-1", 5, 9).unwrap();
        let b = build_and_sign(&sample_msg(), &sk, &pk, "thornado-1", 5, 9).unwrap();
        assert_eq!(a, b, "SIGN_MODE_DIRECT output must be deterministic");

        // decodes as a TxRaw with exactly one 64-byte signature
        let tx = TxRaw::decode(a.as_slice()).unwrap();
        assert_eq!(tx.signatures.len(), 1);
        assert_eq!(tx.signatures[0].len(), 64);
        assert!(!tx.body_bytes.is_empty());
        assert!(!tx.auth_info_bytes.is_empty());

        // different sequence → different signature (sequence is in auth_info)
        let c = build_and_sign(&sample_msg(), &sk, &pk, "thornado-1", 5, 10).unwrap();
        assert_ne!(a, c);
    }

    #[test]
    fn auth_info_carries_gas_and_direct_mode() {
        let sk = [7u8; 32];
        let secp = bitcoin::secp256k1::Secp256k1::signing_only();
        let pk = bitcoin::secp256k1::SecretKey::from_slice(&sk)
            .unwrap()
            .public_key(&secp)
            .serialize()
            .to_vec();
        let raw = build_and_sign(&sample_msg(), &sk, &pk, "thornado-1", 0, 0).unwrap();
        let tx = TxRaw::decode(raw.as_slice()).unwrap();
        let auth = AuthInfo::decode(tx.auth_info_bytes.as_slice()).unwrap();
        assert_eq!(auth.fee.unwrap().gas_limit, GAS_LIMIT);
        let mode = auth.signer_infos[0].mode_info.as_ref().unwrap().single.as_ref().unwrap().mode;
        assert_eq!(mode, SIGN_MODE_DIRECT);
    }
}
