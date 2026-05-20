use bitcoin::secp256k1::ecdsa::Signature as SecpSignature;
use bitcoin::secp256k1::{
    Message as SecpMessage, PublicKey as SecpPublicKey, Secp256k1, SecretKey,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::BTreeSet;
use std::str::FromStr;

#[cfg(feature = "orchard-zcash")]
pub mod orchard;

pub type Result<T> = std::result::Result<T, Error>;

pub const DOMAIN: &str = "thornado-shielder-v1";
pub const HARDENED_CHILD_OFFSET: u64 = 1 << 31;
pub const DEFAULT_DENOMINATIONS_SATS: [u64; 4] =
    [1_000_000_000, 100_000_000, 10_000_000, 1_000_000];

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum Error {
    #[error("invalid proof")]
    InvalidProof,
    #[error("deposit amount does not produce any supported denomination notes")]
    DepositTooSmall,
    #[error("invalid split authorization")]
    InvalidSplitAuthorization,
    #[error("unknown merkle root")]
    UnknownMerkleRoot,
    #[error("unknown note commitment")]
    UnknownCommitment,
    #[error("shielder error: {0}")]
    Shielder(String),
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct DenominationTree {
    pub leaves: Vec<String>,
    pub known_roots: BTreeSet<String>,
}

impl DenominationTree {
    pub fn insert(&mut self, commitment: String) {
        self.leaves.push(commitment);
        self.known_roots.insert(self.root());
    }

    pub fn root(&self) -> String {
        merkle_root(&self.leaves)
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
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    #[cfg(feature = "orchard-zcash")]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardNoteReceipt>,
}

fn default_deposit_index() -> u64 {
    1
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SplitReceipt {
    pub notes: Vec<NoteReceipt>,
    pub remainder_sats: u64,
}

impl SplitReceipt {
    pub fn commitments(&self) -> Vec<NoteCommitment> {
        self.notes
            .iter()
            .map(|note| NoteCommitment {
                denomination_sats: note.denomination_sats,
                owner_pubkey: String::new(),
                commitment: note.commitment.clone(),
            })
            .collect()
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalProof {
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    pub merkle_root: String,
    #[cfg(feature = "orchard-zcash")]
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

#[derive(Debug, Default, Clone)]
pub struct ShielderProofVerifier;

impl ShielderProofVerifier {
    pub fn verify_withdrawal(
        &self,
        proof: &WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()> {
        verify_withdrawal(proof, public)
    }
}

pub fn derive_split_receipt(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> Result<SplitReceipt> {
    derive_split_receipt_for_deposit(deposit_id, 1, amount_sats, client_seed)
}

pub fn derive_split_receipt_for_deposit(
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
        let (commitment, orchard_note) = {
            #[cfg(feature = "orchard-zcash")]
            {
                let (commitment, note) =
                    orchard::create_orchard_note(&child_secret, deposit_id, index, denomination)?;
                (commitment, Some(note))
            }
            #[cfg(not(feature = "orchard-zcash"))]
            {
                (
                    note_commitment(&nullifier, &secret, denomination, ""),
                    None::<()>,
                )
            }
        };
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
            deposit_index,
            denomination_sats: denomination,
            index,
            owner_pubkey: String::new(),
            nullifier,
            secret,
            commitment,
            #[cfg(feature = "orchard-zcash")]
            orchard: orchard_note,
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

pub fn client_pubkey_from_secret(client_seed: &str) -> String {
    deposit_owner_pubkey(&deposit_owner_secret(client_seed))
}

pub fn client_pubkey_for_deposit(client_seed: &str, deposit_index: u64) -> String {
    deposit_owner_pubkey(&deposit_owner_secret_for_deposit(
        client_seed,
        deposit_index,
    ))
}

pub fn split_authorization(
    client_seed: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> SplitAuthorization {
    split_authorization_for_deposit(client_seed, 1, deposit_id, amount_sats, note_commitments)
}

pub fn split_authorization_for_deposit(
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
    let signature = secp.sign_ecdsa(&message, &secret_key);
    SplitAuthorization {
        signature: hex::encode(signature.serialize_der()),
        deposit_pubkey,
    }
}

pub fn verify_split_authorization(
    deposit_pubkey: &str,
    deposit_id: &str,
    authorization: &SplitAuthorization,
    note_commitments: &[NoteCommitment],
) -> Result<()> {
    if deposit_pubkey.is_empty() || authorization.deposit_pubkey != deposit_pubkey {
        return Err(Error::InvalidSplitAuthorization);
    }
    let pubkey =
        SecpPublicKey::from_str(deposit_pubkey).map_err(|_| Error::InvalidSplitAuthorization)?;
    let signature = hex::decode(&authorization.signature)
        .ok()
        .and_then(|bytes| SecpSignature::from_der(&bytes).ok())
        .ok_or(Error::InvalidSplitAuthorization)?;
    let message = split_authorization_message(
        deposit_pubkey,
        deposit_id,
        note_commitments
            .iter()
            .map(|note| note.denomination_sats)
            .sum(),
        note_commitments,
    );
    Secp256k1::verification_only()
        .verify_ecdsa(&message, &signature, &pubkey)
        .map_err(|_| Error::InvalidSplitAuthorization)
}

pub fn merkle_root(leaves: &[String]) -> String {
    #[cfg(feature = "orchard-zcash")]
    {
        orchard::merkle_root_hex(leaves).unwrap_or_default()
    }
    #[cfg(not(feature = "orchard-zcash"))]
    {
        let leaves = serde_json::to_string(leaves).unwrap_or_default();
        hash_parts(&[DOMAIN, "disabled-zk-merkle-root", &leaves])
    }
}

pub fn shielder_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    client_seed: &str,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    #[cfg(feature = "orchard-zcash")]
    {
        let orchard_note = receipt.orchard.as_ref().ok_or(Error::InvalidProof)?;
        let anchor = orchard::merkle_root_hex(&tree.leaves)?;
        let context_public = WithdrawalPublicInputs {
            nullifier_hash: String::new(),
            owner_pubkey: receipt.owner_pubkey.clone(),
            denomination_sats: receipt.denomination_sats,
            recipient,
            fee_sats,
            merkle_root: anchor.clone(),
            recipient_field: None,
            relayer_field: None,
            refund_field: None,
        };
        let public_context = orchard_public_context(&context_public);
        let child_secret = note_child_secret_for_deposit(
            client_seed,
            receipt.deposit_index,
            &receipt.deposit_id,
            receipt.index + 1,
        );
        let (orchard_proof, nullifier_hash, merkle_root) = orchard::prove_orchard_withdrawal(
            &child_secret,
            orchard_note,
            &tree.leaves,
            &receipt.commitment,
            &public_context,
        )?;
        if merkle_root != anchor {
            return Err(Error::InvalidProof);
        }
        let public = WithdrawalPublicInputs {
            nullifier_hash,
            owner_pubkey: receipt.owner_pubkey.clone(),
            denomination_sats: receipt.denomination_sats,
            recipient: context_public.recipient,
            fee_sats,
            merkle_root: merkle_root.clone(),
            recipient_field: None,
            relayer_field: None,
            refund_field: None,
        };
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
    #[cfg(not(feature = "orchard-zcash"))]
    {
        let _ = (receipt, client_seed, tree, recipient, fee_sats);
        Err(Error::InvalidProof)
    }
}

pub fn verify_withdrawal(proof: &WithdrawalProof, public: &WithdrawalPublicInputs) -> Result<()> {
    #[cfg(feature = "orchard-zcash")]
    {
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
    #[cfg(not(feature = "orchard-zcash"))]
    {
        let _ = (proof, public);
        Err(Error::InvalidProof)
    }
}

pub fn nullifier_hash(nullifier: &str) -> String {
    hash_parts(&[DOMAIN, "nullifier-hash", nullifier])
}

pub fn note_child_secret(client_seed: &str, deposit_id: &str, index: u64) -> String {
    note_child_secret_for_deposit(client_seed, 1, deposit_id, index)
}

pub fn note_child_secret_for_deposit(
    client_seed: &str,
    deposit_index: u64,
    deposit_id: &str,
    index: u64,
) -> String {
    let hardened_index = hardened_child_index(index);
    hash_parts(&[
        DOMAIN,
        "note-child-secret",
        "m/tc84'/btc'/deposit'/note'",
        client_seed,
        &hardened_child_index(deposit_index).to_string(),
        deposit_id,
        &hardened_index.to_string(),
    ])
}

pub fn hardened_child_index(index: u64) -> u64 {
    HARDENED_CHILD_OFFSET
        .checked_add(index)
        .expect("hardened child index overflow")
}

#[cfg(feature = "orchard-zcash")]
pub(crate) fn orchard_public_context(public: &WithdrawalPublicInputs) -> Vec<u8> {
    hash_parts_bytes(&[
        DOMAIN,
        "orchard-withdrawal",
        &public.recipient,
        &public.fee_sats.to_string(),
        &public.denomination_sats.to_string(),
        &public.merkle_root,
    ])
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

fn deposit_owner_secret(client_seed: &str) -> String {
    deposit_owner_secret_for_deposit(client_seed, 1)
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

#[cfg(not(feature = "orchard-zcash"))]
fn note_commitment(
    nullifier: &str,
    secret: &str,
    denomination_sats: u64,
    owner_pubkey: &str,
) -> String {
    hash_parts(&[
        DOMAIN,
        "note-commitment",
        nullifier,
        secret,
        &denomination_sats.to_string(),
        owner_pubkey,
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

fn hash_parts_bytes(parts: &[&str]) -> Vec<u8> {
    let mut hasher = Sha256::new();
    for part in parts {
        hasher.update((part.len() as u64).to_be_bytes());
        hasher.update(part.as_bytes());
    }
    hasher.finalize().to_vec()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_receipt_and_authorization_roundtrip() {
        let receipt = derive_split_receipt("dep-1", 100_000_000, "client-seed").unwrap();
        let commitments = receipt.commitments();
        let authorization = split_authorization("client-seed", "dep-1", 100_000_000, &commitments);
        verify_split_authorization(
            &authorization.deposit_pubkey,
            "dep-1",
            &authorization,
            &commitments,
        )
        .unwrap();
    }

    #[test]
    #[cfg_attr(
        not(feature = "proof-tests"),
        ignore = "expensive proof test; run with `cargo test -p thornado-shielder --features proof-tests`"
    )]
    fn shielder_withdrawal_proves_and_verifies() {
        let receipt = derive_split_receipt("dep-1", 100_000_000, "client-seed").unwrap();
        let note = receipt.notes.first().unwrap();
        let tree = DenominationTree {
            leaves: vec![note.commitment.clone()],
            known_roots: Default::default(),
        };
        let (proof, public) = shielder_withdrawal_from_receipt(
            note,
            "client-seed",
            &tree,
            "bcrt1qrecipient".to_string(),
            1_000,
        )
        .unwrap();
        verify_withdrawal(&proof, &public).unwrap();
    }
}
