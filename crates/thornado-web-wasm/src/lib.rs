#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use wasm_bindgen::prelude::*;

#[path = "../../thornado-core/src/orchard.rs"]
mod orchard;

type Result<T> = std::result::Result<T, Error>;

const DOMAIN: &str = "thornado-mvp-v1";
const DEFAULT_DENOMINATIONS_SATS: [u64; 4] = [1_000_000_000, 100_000_000, 10_000_000, 1_000_000];

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    DepositTooSmall,
    InvalidProof,
    UnknownCommitment,
    UnknownMerkleRoot,
    Zk(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::DepositTooSmall => write!(f, "deposit too small"),
            Error::InvalidProof => write!(f, "invalid proof"),
            Error::UnknownCommitment => write!(f, "unknown commitment"),
            Error::UnknownMerkleRoot => write!(f, "unknown merkle root"),
            Error::Zk(message) => write!(f, "{message}"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteCommitment {
    pub denomination_sats: u64,
    #[serde(default)]
    pub owner_pubkey: String,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteReceipt {
    pub deposit_id: String,
    pub denomination_sats: u64,
    pub index: u64,
    #[serde(default)]
    pub owner_pubkey: String,
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardNoteReceipt>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SplitReceipt {
    pub notes: Vec<NoteReceipt>,
    pub remainder_sats: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalProof {
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    pub merkle_root: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardWithdrawalProof>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalPublicInputs {
    pub nullifier_hash: String,
    #[serde(default)]
    pub owner_pubkey: String,
    pub denomination_sats: u64,
    pub recipient: String,
    pub fee_sats: u64,
    pub merkle_root: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub recipient_field: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub relayer_field: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub refund_field: Option<String>,
}

#[wasm_bindgen]
pub fn derive_split_receipt_json(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> std::result::Result<String, JsValue> {
    let receipt = derive_split_receipt(deposit_id, amount_sats, client_seed).map_err(js_error)?;
    serde_json::to_string(&receipt).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn client_pubkey_from_secret_json(client_seed: &str) -> std::result::Result<String, JsValue> {
    Ok(client_pubkey_from_secret(client_seed))
}

#[wasm_bindgen]
pub fn zk_withdrawal_from_receipt_json(
    note_json: &str,
    client_seed: &str,
    leaves_json: &str,
    recipient: &str,
    fee_sats: u64,
) -> std::result::Result<String, JsValue> {
    let note: NoteReceipt =
        serde_json::from_str(note_json).map_err(|error| JsValue::from_str(&error.to_string()))?;
    let leaves: Vec<String> =
        serde_json::from_str(leaves_json).map_err(|error| JsValue::from_str(&error.to_string()))?;
    let (proof, public) =
        zk_withdrawal_from_receipt(&note, client_seed, &leaves, recipient, fee_sats)
            .map_err(js_error)?;
    serde_json::to_string(&(proof, public)).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn verify_withdrawal_json(
    proof_json: &str,
    public_json: &str,
) -> std::result::Result<(), JsValue> {
    let proof: WithdrawalProof =
        serde_json::from_str(proof_json).map_err(|error| JsValue::from_str(&error.to_string()))?;
    let public: WithdrawalPublicInputs =
        serde_json::from_str(public_json).map_err(|error| JsValue::from_str(&error.to_string()))?;
    verify_withdrawal(&proof, &public).map_err(js_error)
}

fn derive_split_receipt(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> Result<SplitReceipt> {
    let (denominations, remaining) = greedy_denominations(amount_sats);
    let mut notes = Vec::new();
    for (index, denomination) in denominations.iter().copied().enumerate() {
        let index = index as u64;
        let nullifier = hash_parts(&[
            DOMAIN,
            "receipt-nullifier",
            client_seed,
            deposit_id,
            &index.to_string(),
        ]);
        let secret = hash_parts(&[
            DOMAIN,
            "receipt-secret",
            client_seed,
            deposit_id,
            &index.to_string(),
        ]);
        let (commitment, orchard_note) =
            orchard::create_orchard_note(client_seed, deposit_id, index, denomination)?;
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
            denomination_sats: denomination,
            index,
            owner_pubkey: client_pubkey_from_secret(client_seed),
            nullifier,
            secret,
            commitment,
            orchard: Some(orchard_note),
        });
    }
    if notes.is_empty() {
        return Err(Error::DepositTooSmall);
    }
    Ok(SplitReceipt {
        notes,
        remainder_sats: remaining,
    })
}

fn zk_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    client_seed: &str,
    leaves: &[String],
    recipient: &str,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    let anchor = orchard::merkle_root_hex(leaves)?;
    let mut public = WithdrawalPublicInputs {
        nullifier_hash: String::new(),
        owner_pubkey: receipt.owner_pubkey.clone(),
        denomination_sats: receipt.denomination_sats,
        recipient: recipient.to_string(),
        fee_sats,
        merkle_root: anchor.clone(),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    let public_context = orchard_public_context(&public);
    let orchard_note = receipt.orchard.as_ref().ok_or(Error::InvalidProof)?;
    let (orchard_proof, nullifier_hash, merkle_root) = orchard::prove_orchard_withdrawal(
        client_seed,
        orchard_note,
        leaves,
        &receipt.commitment,
        &public_context,
    )?;
    if merkle_root != anchor {
        return Err(Error::InvalidProof);
    }
    public.nullifier_hash = nullifier_hash;
    Ok((
        WithdrawalProof {
            nullifier: String::new(),
            secret: String::new(),
            commitment: String::new(),
            merkle_root,
            orchard: Some(orchard_proof),
        },
        public,
    ))
}

fn verify_withdrawal(proof: &WithdrawalProof, public: &WithdrawalPublicInputs) -> Result<()> {
    let orchard = proof.orchard.as_ref().ok_or(Error::InvalidProof)?;
    if orchard.anchor_hex != public.merkle_root {
        return Err(Error::InvalidProof);
    }
    let matching_nullifiers = orchard
        .actions
        .iter()
        .filter(|action| action.nullifier_hex == public.nullifier_hash)
        .count();
    if matching_nullifiers != 1 {
        return Err(Error::InvalidProof);
    }
    if orchard.value_balance.unsigned_abs() != public.denomination_sats {
        return Err(Error::InvalidProof);
    }
    orchard::verify_orchard_withdrawal(orchard, &orchard_public_context(public))
}

fn orchard_public_context(public: &WithdrawalPublicInputs) -> Vec<u8> {
    hash_parts_bytes(&[
        DOMAIN,
        "orchard-withdrawal",
        &public.recipient,
        &public.fee_sats.to_string(),
        &public.denomination_sats.to_string(),
        &public.merkle_root,
    ])
}

fn greedy_denominations(amount_sats: u64) -> (Vec<u64>, u64) {
    let mut remaining = amount_sats;
    let mut denominations = Vec::new();
    for denomination in DEFAULT_DENOMINATIONS_SATS {
        while remaining >= denomination {
            denominations.push(denomination);
            remaining -= denomination;
        }
    }
    (denominations, remaining)
}

fn hash_parts(parts: &[&str]) -> String {
    hex::encode(hash_parts_bytes(parts))
}

fn note_owner_secret(client_seed: &str) -> String {
    hash_parts(&[DOMAIN, "receipt-owner-secret", client_seed])
}

fn client_pubkey_from_secret(client_seed: &str) -> String {
    hash_parts(&[
        DOMAIN,
        "receipt-owner-pubkey",
        &note_owner_secret(client_seed),
    ])
}

fn hash_parts_bytes(parts: &[&str]) -> Vec<u8> {
    let mut hasher = Sha256::new();
    for part in parts {
        hasher.update((part.len() as u64).to_be_bytes());
        hasher.update(part.as_bytes());
    }
    hasher.finalize().to_vec()
}

fn js_error(error: Error) -> JsValue {
    JsValue::from_str(&error.to_string())
}
