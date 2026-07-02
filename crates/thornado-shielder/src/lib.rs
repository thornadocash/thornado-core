use hmac::{Hmac, Mac};
use k256::ecdsa::signature::hazmat::{PrehashSigner, PrehashVerifier};
use k256::ecdsa::{Signature as SecpSignature, SigningKey, VerifyingKey};
use k256::elliptic_curve::PrimeField;
use k256::{FieldBytes, Scalar, SecretKey};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256, Sha512};
use std::collections::BTreeSet;

pub mod engine;

pub mod tornado;

pub use engine::{attestation, semi_trustless_at_least_one_honest, CeremonyAttestation, ENGINE_ID};

pub type Result<T> = std::result::Result<T, Error>;

pub const DOMAIN: &str = "thornado-shielder-v1";
pub const HARDENED_CHILD_OFFSET: u64 = 1 << 31;
pub const DEFAULT_DENOMINATIONS_SATS: [u64; 5] =
    [1_000_000_000, 100_000_000, 10_000_000, 1_000_000, 100_000];

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum Error {
    #[error("invalid proof")]
    InvalidProof,
    #[error("deposit amount does not produce any supported denomination notes")]
    DepositTooSmall,
    #[error("invalid shield authorization")]
    InvalidShieldAuthorization,
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
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ShieldAuthorization {
    pub deposit_pubkey: String,
    pub signature: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FeeClaimAuthorization {
    pub receipt: ShieldReceipt,
    pub commitments: Vec<NoteCommitment>,
    pub fee_note_pubkeys: Vec<String>,
    pub operator_signature: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteReceipt {
    pub deposit_id: String,
    #[serde(default = "default_deposit_type")]
    pub deposit_type: String,
    #[serde(default = "default_deposit_index")]
    pub deposit_index: u64,
    pub denomination_sats: u64,
    pub index: u64,
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
}

fn default_deposit_index() -> u64 {
    0
}

fn default_deposit_type() -> String {
    "user".to_string()
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ShieldReceipt {
    pub notes: Vec<NoteReceipt>,
    pub remainder_sats: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteRecoveryCandidate {
    #[serde(default = "default_deposit_type")]
    pub deposit_type: String,
    pub deposit_index: u64,
    pub index: u64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum UserPathChild {
    Hardened(u32),
    Normal(u32),
}

impl ShieldReceipt {
    pub fn commitments(&self) -> Vec<NoteCommitment> {
        self.notes
            .iter()
            .map(|note| NoteCommitment {
                denomination_sats: note.denomination_sats,
                commitment: note.commitment.clone(),
            })
            .collect()
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct WithdrawalProof {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub nullifier: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub secret: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub commitment: String,
    pub merkle_root: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tornado: Option<tornado::prove::TornadoWithdrawProof>,
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

pub fn derive_shield_receipt(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> Result<ShieldReceipt> {
    derive_shield_receipt_for_deposit(deposit_id, 0, amount_sats, client_seed)
}

pub fn derive_shield_receipt_for_deposit(
    deposit_id: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> Result<ShieldReceipt> {
    derive_shield_receipt_for_deposit_type(deposit_id, "", deposit_index, amount_sats, client_seed)
}

pub fn derive_shield_receipt_for_deposit_type(
    deposit_id: &str,
    deposit_type: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> Result<ShieldReceipt> {
    let (denominations, remaining) = greedy_denominations(amount_sats);
    let mut notes = Vec::new();
    for (index, denomination) in denominations.iter().copied().enumerate() {
        let index = index as u64;
        let child_secret = note_child_secret_for_deposit_type(
            client_seed,
            deposit_type,
            deposit_index,
            deposit_id,
            index + 1,
        );
        let nullifier = hash_parts_field248(&[
            DOMAIN,
            "receipt-nullifier",
            &child_secret,
            deposit_id,
            &denomination.to_string(),
        ]);
        let secret = hash_parts_field248(&[
            DOMAIN,
            "receipt-secret",
            &child_secret,
            deposit_id,
            &denomination.to_string(),
        ]);
        let nullifier_fp = tornado::field_from_hex(&nullifier).ok_or(Error::InvalidProof)?;
        let secret_fp = tornado::field_from_hex(&secret).ok_or(Error::InvalidProof)?;
        let commitment = tornado::field_to_hex(
            tornado::note_commitment(nullifier_fp, secret_fp).map_err(|_| Error::InvalidProof)?,
        );
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
            deposit_type: normalized_deposit_type(deposit_type).to_string(),
            deposit_index,
            denomination_sats: denomination,
            index,
            nullifier,
            secret,
            commitment,
        });
    }

    if notes.is_empty() {
        return Err(Error::DepositTooSmall);
    }

    Ok(ShieldReceipt {
        notes,
        remainder_sats: remaining,
    })
}

pub fn client_pubkey_from_secret(client_seed: &str) -> String {
    client_pubkey_for_deposit(client_seed, 0)
}

pub fn client_pubkey_for_deposit(client_seed: &str, deposit_index: u64) -> String {
    client_pubkey_for_deposit_type(client_seed, "", deposit_index)
}

pub fn client_pubkey_for_deposit_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
) -> String {
    deposit_pubkey_from_secret(&deposit_root_secret_for_deposit_type(
        client_seed,
        deposit_type,
        deposit_index,
    ))
}

pub fn shield_authorization(
    client_seed: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> ShieldAuthorization {
    shield_authorization_for_deposit(client_seed, 0, deposit_id, amount_sats, note_commitments)
}

pub fn shield_authorization_for_deposit(
    client_seed: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> ShieldAuthorization {
    shield_authorization_for_deposit_type(
        client_seed,
        "",
        deposit_index,
        deposit_id,
        amount_sats,
        note_commitments,
    )
}

pub fn shield_authorization_for_deposit_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> ShieldAuthorization {
    let deposit_pubkey = client_pubkey_for_deposit_type(client_seed, deposit_type, deposit_index);
    let secret_key = deposit_secret_key_for_type(client_seed, deposit_type, deposit_index);
    let message =
        shield_authorization_message(&deposit_pubkey, deposit_id, amount_sats, note_commitments);
    let signing_key = SigningKey::from(secret_key);
    let signature: SecpSignature = signing_key
        .sign_prehash(&message)
        .expect("32-byte shield authorization digest should sign");
    ShieldAuthorization {
        signature: hex::encode(signature.to_der().as_bytes()),
        deposit_pubkey,
    }
}

pub fn fee_claim_authorization_for_deposit_type(
    client_seed: &str,
    signer_deposit_type: &str,
    signer_deposit_index: u64,
    note_deposit_type: &str,
    note_deposit_index: u64,
    claim_ref: &str,
    node_pubkey: &str,
    owner: &str,
    accrued_sats: u64,
    fee_per_slot_share_sats: u64,
    amount_sats: u64,
) -> Result<FeeClaimAuthorization> {
    let receipt = derive_shield_receipt_for_deposit_type(
        claim_ref,
        note_deposit_type,
        note_deposit_index,
        amount_sats,
        client_seed,
    )?;
    let commitments = receipt.commitments();
    let fee_note_pubkeys = receipt
        .notes
        .iter()
        .map(|note| {
            deposit_pubkey_from_secret(&note_child_secret_for_deposit_type(
                client_seed,
                note_deposit_type,
                note_deposit_index,
                claim_ref,
                note.index + 1,
            ))
        })
        .collect::<Vec<_>>();
    let payload = fee_claim_payload(
        node_pubkey,
        owner,
        accrued_sats,
        fee_per_slot_share_sats,
        &commitments,
        &fee_note_pubkeys,
    );
    let secret_key =
        deposit_secret_key_for_type(client_seed, signer_deposit_type, signer_deposit_index);
    let signing_key = SigningKey::from(secret_key);
    let signature: SecpSignature = signing_key
        .sign_prehash(&payload)
        .expect("32-byte fee claim digest should sign");
    let signature = signature.normalize_s().unwrap_or(signature);
    Ok(FeeClaimAuthorization {
        receipt,
        commitments,
        fee_note_pubkeys,
        operator_signature: hex::encode(signature.to_bytes()),
    })
}

pub fn verify_shield_authorization(
    deposit_pubkey: &str,
    deposit_id: &str,
    authorization: &ShieldAuthorization,
    note_commitments: &[NoteCommitment],
) -> Result<()> {
    if deposit_pubkey.is_empty() || authorization.deposit_pubkey != deposit_pubkey {
        return Err(Error::InvalidShieldAuthorization);
    }
    let pubkey_bytes =
        hex::decode(deposit_pubkey).map_err(|_| Error::InvalidShieldAuthorization)?;
    let pubkey = VerifyingKey::from_sec1_bytes(&pubkey_bytes)
        .map_err(|_| Error::InvalidShieldAuthorization)?;
    let signature = hex::decode(&authorization.signature)
        .ok()
        .and_then(|bytes| SecpSignature::from_der(&bytes).ok())
        .ok_or(Error::InvalidShieldAuthorization)?;
    let message = shield_authorization_message(
        deposit_pubkey,
        deposit_id,
        note_commitments
            .iter()
            .map(|note| note.denomination_sats)
            .sum(),
        note_commitments,
    );
    pubkey
        .verify_prehash(&message, &signature)
        .map_err(|_| Error::InvalidShieldAuthorization)
}

pub fn merkle_root(leaves: &[String]) -> String {
    tornado::merkle_root_hex(leaves).unwrap_or_default()
}

/// Request to append a single leaf to an incremental Merkle tree. `filled_subtrees`
/// are decimal field elements (empty is treated as all-zero, i.e. an empty tree);
/// `leaf` is a field-hex commitment (same encoding the keeper stores).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MerkleAppendRequest {
    #[serde(default)]
    pub filled_subtrees: Vec<String>,
    pub next_index: u64,
    pub leaf: String,
}

/// Result of an incremental append: the new root and updated filled subtrees, both
/// as decimal field elements. The root matches `fr_to_decimal` of the tree root, so
/// it is byte-identical to what the keeper stores from the full-recompute path.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MerkleAppendResponse {
    pub root: String,
    pub filled_subtrees: Vec<String>,
}

pub fn merkle_append(request: &MerkleAppendRequest) -> Result<MerkleAppendResponse> {
    use tornado::field::{fr_from_decimal, fr_from_field_hex, fr_to_decimal};
    let depth = tornado::merkle::MERKLE_TREE_DEPTH;
    let filled: Vec<ark_bn254::Fr> = if request.filled_subtrees.is_empty() {
        vec![ark_bn254::Fr::from(0u64); depth]
    } else {
        if request.filled_subtrees.len() != depth {
            return Err(Error::InvalidProof);
        }
        request
            .filled_subtrees
            .iter()
            .map(|value| fr_from_decimal(value).ok_or(Error::InvalidProof))
            .collect::<Result<Vec<_>>>()?
    };
    let leaf = fr_from_field_hex(&request.leaf).ok_or(Error::InvalidProof)?;
    let (root, new_filled) = tornado::merkle::append_leaf(&filled, request.next_index, leaf)?;
    Ok(MerkleAppendResponse {
        root: fr_to_decimal(root),
        filled_subtrees: new_filled.into_iter().map(fr_to_decimal).collect(),
    })
}

pub fn shielder_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    client_seed: &str,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    let _ = client_seed;
    let leaf_index = tree
        .leaves
        .iter()
        .position(|leaf| leaf == &receipt.commitment)
        .ok_or(Error::UnknownCommitment)?;
    let public = WithdrawalPublicInputs {
        nullifier_hash: String::new(),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root: tree.root(),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    tornado::prove_withdrawal(
        &receipt.nullifier,
        &receipt.secret,
        &tree.leaves,
        leaf_index,
        &public,
    )
}

pub fn withdrawal_witness_from_receipt(
    receipt: &NoteReceipt,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<serde_json::Value> {
    let leaf_index = tree
        .leaves
        .iter()
        .position(|leaf| leaf == &receipt.commitment)
        .ok_or(Error::UnknownCommitment)?;
    let public = WithdrawalPublicInputs {
        nullifier_hash: String::new(),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root: tree.root(),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    tornado::withdrawal_witness_json(
        &receipt.nullifier,
        &receipt.secret,
        &tree.leaves,
        leaf_index,
        &public,
    )
}

pub fn withdrawal_proof_and_witness_from_receipt(
    receipt: &NoteReceipt,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs, serde_json::Value)> {
    let leaf_index = tree
        .leaves
        .iter()
        .position(|leaf| leaf == &receipt.commitment)
        .ok_or(Error::UnknownCommitment)?;
    let public = WithdrawalPublicInputs {
        nullifier_hash: String::new(),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root: tree.root(),
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    tornado::prove_withdrawal_and_witness(
        &receipt.nullifier,
        &receipt.secret,
        &tree.leaves,
        leaf_index,
        &public,
    )
}

pub fn validate_withdrawal_public_inputs(public: &WithdrawalPublicInputs) -> Result<()> {
    tornado::validate_public_inputs(public)
}

pub fn recipient_binding_field(
    recipient: &str,
    fee_sats: u64,
    denomination_sats: u64,
) -> Result<String> {
    tornado::recipient_binding_decimal(recipient, fee_sats, denomination_sats)
}

pub fn note_recovery_candidates(
    _client_seed: &str,
    deposit_limit: u64,
    note_limit: u64,
) -> Vec<NoteRecoveryCandidate> {
    let mut candidates = Vec::new();
    for deposit_type in ["user", "node"] {
        for deposit_index in 0..deposit_limit {
            for index in 1..=note_limit {
                candidates.push(NoteRecoveryCandidate {
                    deposit_type: deposit_type.to_string(),
                    deposit_index,
                    index: index - 1,
                });
            }
        }
    }
    candidates
}

pub fn recover_note_receipt(
    client_seed: &str,
    deposit_index: u64,
    note_index: u64,
    deposit_id: &str,
    denomination_sats: u64,
    commitment: &str,
) -> Result<NoteReceipt> {
    recover_note_receipt_for_deposit_type(
        client_seed,
        "",
        deposit_index,
        note_index,
        deposit_id,
        denomination_sats,
        commitment,
    )
}

pub fn recover_note_receipt_for_deposit_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
    note_index: u64,
    deposit_id: &str,
    denomination_sats: u64,
    commitment: &str,
) -> Result<NoteReceipt> {
    let child_secret = note_child_secret_for_deposit_type(
        client_seed,
        deposit_type,
        deposit_index,
        deposit_id,
        note_index + 1,
    );
    let nullifier = hash_parts_field248(&[
        DOMAIN,
        "receipt-nullifier",
        &child_secret,
        deposit_id,
        &denomination_sats.to_string(),
    ]);
    let secret = hash_parts_field248(&[
        DOMAIN,
        "receipt-secret",
        &child_secret,
        deposit_id,
        &denomination_sats.to_string(),
    ]);
    let nullifier_fp = tornado::field_from_hex(&nullifier).ok_or(Error::InvalidProof)?;
    let secret_fp = tornado::field_from_hex(&secret).ok_or(Error::InvalidProof)?;
    let expected_commitment = tornado::field_to_hex(
        tornado::note_commitment(nullifier_fp, secret_fp).map_err(|_| Error::InvalidProof)?,
    );
    if expected_commitment != commitment {
        return Err(Error::UnknownCommitment);
    }
    Ok(NoteReceipt {
        deposit_id: deposit_id.to_string(),
        deposit_type: normalized_deposit_type(deposit_type).to_string(),
        deposit_index,
        denomination_sats,
        index: note_index,
        nullifier,
        secret,
        commitment: commitment.to_string(),
    })
}

pub fn verify_withdrawal(proof: &WithdrawalProof, public: &WithdrawalPublicInputs) -> Result<()> {
    tornado::verify_withdrawal(proof, public)
}

/// Strip note-specific fields from a withdrawal proof after verification.
pub fn redact_spent_commitment(proof: &mut WithdrawalProof) {
    tornado::redact_private_fields(proof);
}

pub fn nullifier_hash(nullifier: &str) -> String {
    let Some(nullifier) = tornado::field_from_hex(nullifier) else {
        return String::new();
    };
    let Ok(hash) = tornado::nullifier_hash(nullifier) else {
        return String::new();
    };
    tornado::field::fr_to_decimal(hash)
}

pub fn note_child_secret(client_seed: &str, deposit_id: &str, index: u64) -> String {
    note_child_secret_for_deposit(client_seed, 0, deposit_id, index)
}

pub fn note_child_secret_for_deposit(
    client_seed: &str,
    deposit_index: u64,
    _deposit_id: &str,
    index: u64,
) -> String {
    note_child_secret_for_deposit_type(client_seed, "", deposit_index, _deposit_id, index)
}

pub fn note_child_secret_for_deposit_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
    _deposit_id: &str,
    index: u64,
) -> String {
    hex::encode(
        derive_bip32_secret_key_for_type(client_seed, deposit_type, deposit_index, index)
            .to_bytes(),
    )
}

pub fn hardened_child_index(index: u64) -> u64 {
    HARDENED_CHILD_OFFSET
        .checked_add(index)
        .expect("hardened child index overflow")
}

fn shield_authorization_message(
    deposit_pubkey: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments: &[NoteCommitment],
) -> Vec<u8> {
    let commitments_json =
        serde_json::to_string(note_commitments).expect("note commitments should serialize");
    let digest = hash_parts_bytes(&[
        DOMAIN,
        "shield-authorization",
        deposit_pubkey,
        deposit_id,
        &amount_sats.to_string(),
        &commitments_json,
    ]);
    digest
}

fn fee_claim_payload(
    node_pubkey: &str,
    owner: &str,
    accrued: u64,
    fee_per_slot_share: u64,
    notes: &[NoteCommitment],
    note_pubkeys: &[String],
) -> Vec<u8> {
    let mut parts = vec![
        "thornado:fee-claim:v1".to_string(),
        node_pubkey.trim().to_string(),
        owner.trim().to_string(),
        accrued.to_string(),
        fee_per_slot_share.to_string(),
    ];
    for (note, pubkey) in notes.iter().zip(note_pubkeys.iter()) {
        parts.push(format!(
            "{}:{}:{}",
            note.denomination_sats,
            note.commitment.trim(),
            pubkey.trim()
        ));
    }
    Sha256::digest(parts.join("|").as_bytes()).to_vec()
}

fn deposit_root_secret_for_deposit_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
) -> String {
    hex::encode(
        derive_bip32_secret_key_for_type(client_seed, deposit_type, deposit_index, 0).to_bytes(),
    )
}

fn deposit_pubkey_from_secret(deposit_secret: &str) -> String {
    let secret_bytes = hex::decode(deposit_secret).unwrap_or_default();
    let Ok(secret_key) = SecretKey::from_slice(&secret_bytes) else {
        return String::new();
    };
    let signing_key = SigningKey::from(secret_key);
    VerifyingKey::from(&signing_key)
        .to_encoded_point(true)
        .as_bytes()
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn deposit_secret_key_for_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
) -> SecretKey {
    derive_bip32_secret_key_for_type(client_seed, deposit_type, deposit_index, 0)
}

fn derive_bip32_secret_key_for_type(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
    note_index: u64,
) -> SecretKey {
    let path = thornado_bip32_path_for_type(deposit_type, deposit_index, note_index)
        .expect("thornado path indexes should fit BIP32 hardened indexes");
    derive_hardened_bip32_private_key(&decode_client_seed(client_seed), &path)
        .expect("thornado hardened BIP32 derivation should succeed")
}

fn derive_hardened_bip32_private_key(seed: &[u8], path: &[UserPathChild]) -> Result<SecretKey> {
    let master = hmac_sha512(b"Bitcoin seed", seed);
    let mut key = secret_key_from_bytes(&master[..32])?;
    let mut chain_code = [0_u8; 32];
    chain_code.copy_from_slice(&master[32..]);

    for child in path {
        let (child_index, mut data) = match child {
            UserPathChild::Hardened(index) => {
                let mut data = Vec::with_capacity(37);
                data.push(0);
                data.extend_from_slice(&key.to_bytes());
                (HARDENED_CHILD_OFFSET as u32 + index, data)
            }
            UserPathChild::Normal(index) => {
                let signing_key = SigningKey::from(key.clone());
                let mut data = Vec::with_capacity(37);
                data.extend_from_slice(
                    VerifyingKey::from(&signing_key)
                        .to_encoded_point(true)
                        .as_bytes(),
                );
                (*index, data)
            }
        };
        data.extend_from_slice(&child_index.to_be_bytes());
        let derived = hmac_sha512(&chain_code, &data);
        let child_tweak = scalar_from_bytes(&derived[..32])?;
        let parent = scalar_from_bytes(&key.to_bytes())?;
        let child_key = child_tweak.add(&parent);
        if bool::from(child_key.is_zero()) {
            return Err(Error::Shielder("derived zero child key".to_string()));
        }
        key = SecretKey::from_slice(&child_key.to_bytes())
            .map_err(|err| Error::Shielder(err.to_string()))?;
        chain_code.copy_from_slice(&derived[32..]);
    }
    Ok(key)
}

fn hmac_sha512(key: &[u8], data: &[u8]) -> [u8; 64] {
    let mut mac = Hmac::<Sha512>::new_from_slice(key).expect("HMAC accepts any key length");
    mac.update(data);
    mac.finalize().into_bytes().into()
}

fn secret_key_from_bytes(bytes: &[u8]) -> Result<SecretKey> {
    SecretKey::from_slice(bytes).map_err(|err| Error::Shielder(err.to_string()))
}

fn scalar_from_bytes(bytes: &[u8]) -> Result<Scalar> {
    let field_bytes = FieldBytes::from_slice(bytes);
    Option::<Scalar>::from(Scalar::from_repr(*field_bytes))
        .filter(|scalar| !bool::from(scalar.is_zero()))
        .ok_or_else(|| Error::Shielder("invalid secp256k1 scalar".to_string()))
}

fn decode_client_seed(client_seed: &str) -> Vec<u8> {
    let trimmed = client_seed.trim();
    if let Ok(seed) = hex::decode(trimmed) {
        if !seed.is_empty() {
            return seed;
        }
    }
    hash_parts_bytes(&[DOMAIN, "bip39-seed-fallback", trimmed]).to_vec()
}

fn deposit_type_purpose_index(deposit_type: &str) -> u32 {
    if normalized_deposit_type(deposit_type) == "node" {
        1
    } else {
        0
    }
}

fn normalized_deposit_type(deposit_type: &str) -> &'static str {
    if deposit_type.eq_ignore_ascii_case("node") {
        "node"
    } else {
        "user"
    }
}

fn thornado_bip32_path_for_type(
    deposit_type: &str,
    deposit_index: u64,
    note_index: u64,
) -> Result<Vec<UserPathChild>> {
    Ok(vec![
        UserPathChild::Hardened(44),
        UserPathChild::Hardened(60),
        UserPathChild::Hardened(deposit_type_purpose_index(deposit_type)),
        UserPathChild::Normal(bip32_index(deposit_index)?),
        UserPathChild::Normal(bip32_index(note_index)?),
    ])
}

fn bip32_index(index: u64) -> Result<u32> {
    u32::try_from(index).map_err(|_| Error::Shielder("BIP32 index overflow".to_string()))
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

fn hash_parts_field248(parts: &[&str]) -> String {
    let digest = hash_parts_bytes(parts);
    let mut field = [0_u8; 32];
    field[1..].copy_from_slice(&digest[1..32]);
    hex::encode(field)
}

pub(crate) fn hash_parts_bytes(parts: &[&str]) -> Vec<u8> {
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
    fn redact_spent_commitment_clears_note_specific_fields() {
        let mut proof = WithdrawalProof {
            nullifier: "nf".to_string(),
            secret: "secret".to_string(),
            commitment: "cmx".to_string(),
            merkle_root: "root".to_string(),
            tornado: Some(tornado::TornadoWithdrawProof {
                protocol: tornado::PROTOCOL_ID.to_string(),
                groth16: Some(tornado::groth16::SnarkjsProof {
                    pi_a: vec!["0".into(), "0".into(), "1".into()],
                    pi_b: vec![
                        vec!["0".into(), "0".into()],
                        vec!["0".into(), "0".into()],
                        vec!["1".into(), "0".into()],
                    ],
                    pi_c: vec!["0".into(), "0".into(), "1".into()],
                    protocol: Some("groth".into()),
                }),
            }),
        };
        redact_spent_commitment(&mut proof);
        assert!(proof.nullifier.is_empty());
        assert!(proof.secret.is_empty());
        assert!(proof.commitment.is_empty());
        assert!(proof.tornado.as_ref().unwrap().groth16.is_some());
    }

    #[test]
    fn withdrawal_proof_serialization_omits_empty_note_fields() {
        let proof = WithdrawalProof {
            nullifier: String::new(),
            secret: String::new(),
            commitment: String::new(),
            merkle_root: "root".to_string(),
            tornado: None,
        };
        let json = serde_json::to_string(&proof).unwrap();
        assert!(!json.contains("nullifier"));
        assert!(!json.contains("secret"));
        assert!(!json.contains("commitment"));
        assert!(json.contains("merkle_root"));
    }

    #[test]
    fn shield_receipt_and_authorization_roundtrip() {
        let receipt = derive_shield_receipt("dep-1", 100_000_000, "client-seed").unwrap();
        let commitments = receipt.commitments();
        let authorization = shield_authorization("client-seed", "dep-1", 100_000_000, &commitments);
        verify_shield_authorization(
            &authorization.deposit_pubkey,
            "dep-1",
            &authorization,
            &commitments,
        )
        .unwrap();
    }

    #[test]
    fn note_derivation_uses_evm_rooted_hardened_bip32_path() {
        let path = thornado_bip32_path_for_type("user", 7, 3).unwrap();
        assert_eq!(
            path,
            vec![
                UserPathChild::Hardened(44),
                UserPathChild::Hardened(60),
                UserPathChild::Hardened(0),
                UserPathChild::Normal(7),
                UserPathChild::Normal(3),
            ]
        );

        let seed = hex::encode([42_u8; 64]);
        let user = note_child_secret_for_deposit_type(&seed, "user", 0, "", 1);
        let node = note_child_secret_for_deposit_type(&seed, "node", 0, "", 1);
        assert_ne!(user, node);

        let user_deposit_pubkey = client_pubkey_for_deposit_type(&seed, "user", 0);
        let node_typed_deposit_pubkey = client_pubkey_for_deposit_type(&seed, "node", 0);
        assert_ne!(user_deposit_pubkey, node_typed_deposit_pubkey);

        let user_receipt =
            derive_shield_receipt_for_deposit_type("dep-1", "user", 0, 100_000_000, &seed).unwrap();
        let node_receipt =
            derive_shield_receipt_for_deposit_type("dep-1", "node", 0, 100_000_000, &seed).unwrap();
        assert_eq!(user_receipt.notes[0].deposit_type, "user");
        assert_eq!(node_receipt.notes[0].deposit_type, "node");
        assert_ne!(
            user_receipt.notes[0].commitment,
            node_receipt.notes[0].commitment
        );
    }

    #[test]
    #[cfg_attr(
        not(feature = "proof-tests"),
        ignore = "expensive proof test; run with `cargo test -p thornado-shielder --features proof-tests`"
    )]
    fn shielder_withdrawal_proves_and_verifies() {
        let receipt = derive_shield_receipt("dep-1", 100_000_000, "client-seed").unwrap();
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
