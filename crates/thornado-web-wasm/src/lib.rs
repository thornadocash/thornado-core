#![allow(dead_code)]

use bitcoin::secp256k1::ecdsa::Signature as SecpSignature;
use bitcoin::secp256k1::{
    Message as SecpMessage, PublicKey as SecpPublicKey, Secp256k1, SecretKey,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use wasm_bindgen::prelude::*;

#[path = "../../thornado-shielder/src/orchard.rs"]
mod orchard;

type Result<T> = std::result::Result<T, Error>;

const DOMAIN: &str = "thornado-shielder-v1";
const HARDENED_CHILD_OFFSET: u64 = 1 << 31;
const DEFAULT_DENOMINATIONS_SATS: [u64; 5] =
    [1_000_000_000, 100_000_000, 10_000_000, 1_000_000, 100_000];

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Error {
    DepositTooSmall,
    InvalidProof,
    UnknownCommitment,
    UnknownMerkleRoot,
    Shielder(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::DepositTooSmall => write!(f, "deposit too small"),
            Error::InvalidProof => write!(f, "invalid proof"),
            Error::UnknownCommitment => write!(f, "unknown commitment"),
            Error::UnknownMerkleRoot => write!(f, "unknown merkle root"),
            Error::Shielder(message) => write!(f, "{message}"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteCommitment {
    pub denomination_sats: u64,
    #[serde(default)]
    pub owner_pubkey: String,
    #[serde(default)]
    pub signature: String,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SplitAuthorization {
    pub deposit_pubkey: String,
    pub signature: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteReceipt {
    pub deposit_id: String,
    #[serde(default = "default_deposit_index")]
    pub deposit_index: u64,
    pub denomination_sats: u64,
    pub index: u64,
    #[serde(default)]
    pub owner_pubkey: String,
    #[serde(default)]
    pub signature: String,
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardNoteReceipt>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteRecoveryCandidate {
    pub deposit_index: u64,
    pub index: u64,
    pub owner_pubkey: String,
}

fn default_deposit_index() -> u64 {
    1
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
    let receipt = derive_split_receipt_for_deposit(deposit_id, 1, amount_sats, client_seed)
        .map_err(js_error)?;
    serde_json::to_string(&receipt).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn derive_split_receipt_for_deposit_json(
    deposit_id: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> std::result::Result<String, JsValue> {
    let receipt =
        derive_split_receipt_for_deposit(deposit_id, deposit_index, amount_sats, client_seed)
            .map_err(js_error)?;
    serde_json::to_string(&receipt).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn client_pubkey_from_secret_json(client_seed: &str) -> std::result::Result<String, JsValue> {
    Ok(client_pubkey_for_deposit(client_seed, 1))
}

#[wasm_bindgen]
pub fn client_pubkey_for_deposit_json(
    client_seed: &str,
    deposit_index: u64,
) -> std::result::Result<String, JsValue> {
    Ok(client_pubkey_for_deposit(client_seed, deposit_index))
}

#[wasm_bindgen]
pub fn split_authorization_json(
    client_seed: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments_json: &str,
) -> std::result::Result<String, JsValue> {
    let note_commitments: Vec<NoteCommitment> = serde_json::from_str(note_commitments_json)
        .map_err(|error| JsValue::from_str(&error.to_string()))?;
    let authorization =
        split_authorization_for_deposit(client_seed, 1, deposit_id, amount_sats, &note_commitments);
    serde_json::to_string(&authorization).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn split_authorization_for_deposit_json(
    client_seed: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments_json: &str,
) -> std::result::Result<String, JsValue> {
    let note_commitments: Vec<NoteCommitment> = serde_json::from_str(note_commitments_json)
        .map_err(|error| JsValue::from_str(&error.to_string()))?;
    let authorization = split_authorization_for_deposit(
        client_seed,
        deposit_index,
        deposit_id,
        amount_sats,
        &note_commitments,
    );
    serde_json::to_string(&authorization).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn shielder_withdrawal_from_receipt_json(
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
        shielder_withdrawal_from_receipt(&note, client_seed, &leaves, recipient, fee_sats)
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

#[wasm_bindgen]
pub fn note_recovery_candidates_json(
    client_seed: &str,
    deposit_limit: u64,
    note_limit: u64,
) -> std::result::Result<String, JsValue> {
    let mut candidates = Vec::new();
    for deposit_index in 0..deposit_limit {
        for index in 1..=note_limit {
            let child_secret = note_child_secret_for_deposit(client_seed, deposit_index, "", index);
            candidates.push(NoteRecoveryCandidate {
                deposit_index,
                index: index - 1,
                owner_pubkey: deposit_owner_pubkey(&child_secret),
            });
        }
    }
    serde_json::to_string(&candidates).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn recover_note_receipt_json(
    client_seed: &str,
    deposit_index: u64,
    note_index: u64,
    deposit_id: &str,
    denomination_sats: u64,
    commitment: &str,
) -> std::result::Result<String, JsValue> {
    let child_secret =
        note_child_secret_for_deposit(client_seed, deposit_index, deposit_id, note_index + 1);
    let owner_pubkey = deposit_owner_pubkey(&child_secret);
    let nullifier = hash_parts(&[
        DOMAIN,
        "receipt-nullifier",
        &child_secret,
        &denomination_sats.to_string(),
    ]);
    let secret = hash_parts(&[
        DOMAIN,
        "receipt-secret",
        &child_secret,
        &denomination_sats.to_string(),
    ]);
    let signature =
        note_authorization_for_secret(&child_secret, &owner_pubkey, denomination_sats, commitment);
    let (expected_commitment, orchard_note) =
        orchard::create_orchard_note(&child_secret, deposit_id, note_index, denomination_sats)
            .map_err(js_error)?;
    if expected_commitment != commitment {
        return Err(JsValue::from_str("recovered note commitment mismatch"));
    }
    let note = NoteReceipt {
        deposit_id: deposit_id.to_string(),
        deposit_index,
        denomination_sats,
        index: note_index,
        owner_pubkey,
        signature,
        nullifier,
        secret,
        commitment: commitment.to_string(),
        orchard: Some(orchard_note),
    };
    serde_json::to_string(&note).map_err(|error| JsValue::from_str(&error.to_string()))
}

#[wasm_bindgen]
pub fn nullifier_hash_json(nullifier: &str) -> std::result::Result<String, JsValue> {
    Ok(hash_parts(&[DOMAIN, "nullifier-hash", nullifier]))
}

fn derive_split_receipt_for_deposit(
    deposit_id: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> Result<SplitReceipt> {
    let (denominations, remaining) = greedy_denominations(amount_sats);
    let mut notes = Vec::new();
    for (index, denomination) in denominations.iter().copied().enumerate() {
        let index = index as u64;
        let child_secret =
            note_child_secret_for_deposit(client_seed, deposit_index, deposit_id, index + 1);
        let owner_pubkey = deposit_owner_pubkey(&child_secret);
        let nullifier = hash_parts(&[
            DOMAIN,
            "receipt-nullifier",
            &child_secret,
            &denomination.to_string(),
        ]);
        let secret = hash_parts(&[
            DOMAIN,
            "receipt-secret",
            &child_secret,
            &denomination.to_string(),
        ]);
        let (commitment, orchard_note) =
            orchard::create_orchard_note(&child_secret, deposit_id, index, denomination)?;
        let signature =
            note_authorization_for_secret(&child_secret, &owner_pubkey, denomination, &commitment);
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
            deposit_index,
            denomination_sats: denomination,
            index,
            owner_pubkey,
            signature,
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

fn shielder_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    client_seed: &str,
    leaves: &[String],
    recipient: &str,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    let anchor = orchard::merkle_root_hex(leaves)?;
    let mut public = WithdrawalPublicInputs {
        nullifier_hash: String::new(),
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
    let child_secret = note_child_secret_for_deposit(
        client_seed,
        receipt.deposit_index,
        &receipt.deposit_id,
        receipt.index + 1,
    );
    let (orchard_proof, nullifier_hash, merkle_root) = orchard::prove_orchard_withdrawal(
        &child_secret,
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

fn note_child_secret_for_deposit(
    client_seed: &str,
    deposit_index: u64,
    _deposit_id: &str,
    index: u64,
) -> String {
    let hardened_index = hardened_child_index(index);
    hash_parts(&[
        DOMAIN,
        "note-child-secret",
        "m/tc84'/btc'/deposit'/note'",
        client_seed,
        &hardened_child_index(deposit_index).to_string(),
        &hardened_index.to_string(),
    ])
}

fn client_pubkey_for_deposit(client_seed: &str, deposit_index: u64) -> String {
    deposit_owner_pubkey(&deposit_owner_secret_for_deposit(
        client_seed,
        deposit_index,
    ))
}

fn deposit_owner_secret_for_deposit(client_seed: &str, deposit_index: u64) -> String {
    hex::encode(deposit_secret_key(client_seed, deposit_index).secret_bytes())
}

fn deposit_owner_pubkey(owner_secret: &str) -> String {
    let secret_bytes = hex::decode(owner_secret).unwrap_or_default();
    let Ok(secret_key) = SecretKey::from_slice(&secret_bytes) else {
        return String::new();
    };
    let secp = Secp256k1::new();
    SecpPublicKey::from_secret_key(&secp, &secret_key).to_string()
}

fn secret_key_from_hex_material(secret_hex: &str) -> SecretKey {
    let secret_bytes = hex::decode(secret_hex).unwrap_or_default();
    if let Ok(secret_key) = SecretKey::from_slice(&secret_bytes) {
        return secret_key;
    }
    for counter in 0_u32..u32::MAX {
        let digest = hash_parts_bytes(&[
            DOMAIN,
            "secp-secret-retry",
            secret_hex,
            &counter.to_string(),
        ]);
        if let Ok(secret_key) = SecretKey::from_slice(&digest) {
            return secret_key;
        }
    }
    unreachable!("sha256 should produce a valid secp256k1 secret key")
}

fn split_authorization_for_deposit(
    client_seed: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> SplitAuthorization {
    let deposit_pubkey = client_pubkey_for_deposit(client_seed, deposit_index);
    let secp = Secp256k1::new();
    let secret_key = deposit_secret_key(client_seed, deposit_index);
    let message =
        split_authorization_message(&deposit_pubkey, deposit_id, amount_sats, note_commitments);
    let signature: SecpSignature = secp.sign_ecdsa(&message, &secret_key);
    SplitAuthorization {
        signature: hex::encode(signature.serialize_der()),
        deposit_pubkey,
    }
}

fn split_authorization_message(
    deposit_pubkey: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> SecpMessage {
    let commitments_json =
        serde_json::to_string(note_commitments).expect("note commitments should serialize");
    let digest = hash_parts_bytes(&[
        DOMAIN,
        "split-authorization",
        deposit_pubkey,
        deposit_id,
        &amount_sats.to_string(),
        &commitments_json,
    ]);
    SecpMessage::from_digest_slice(&digest).expect("sha256 digest has secp256k1 message length")
}

fn note_authorization_for_secret(
    child_secret: &str,
    owner_pubkey: &str,
    denomination_sats: u64,
    commitment: &str,
) -> String {
    let secp = Secp256k1::new();
    let secret_key = secret_key_from_hex_material(child_secret);
    let digest = hash_parts_bytes(&[
        DOMAIN,
        "note-authorization",
        owner_pubkey,
        &denomination_sats.to_string(),
        commitment,
    ]);
    let message = SecpMessage::from_digest_slice(&digest)
        .expect("sha256 digest has secp256k1 message length");
    hex::encode(secp.sign_ecdsa(&message, &secret_key).serialize_der())
}

fn deposit_secret_key(client_seed: &str, deposit_index: u64) -> SecretKey {
    for counter in 0_u32..u32::MAX {
        let digest = hash_parts_bytes(&[
            DOMAIN,
            "deposit-owner-secret",
            "m/tc84'/btc'/deposit'/0'",
            client_seed,
            &hardened_child_index(deposit_index).to_string(),
            &counter.to_string(),
        ]);
        if let Ok(secret_key) = SecretKey::from_slice(&digest) {
            return secret_key;
        }
    }
    unreachable!("sha256 should produce a valid secp256k1 secret key")
}

fn hardened_child_index(index: u64) -> u64 {
    HARDENED_CHILD_OFFSET
        .checked_add(index)
        .expect("hardened child index overflow")
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
