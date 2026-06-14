use thornado_shielder::{
    client_pubkey_for_deposit, client_pubkey_for_deposit_type, client_pubkey_from_secret,
    derive_shield_receipt, derive_shield_receipt_for_deposit,
    derive_shield_receipt_for_deposit_type, merkle_root, note_recovery_candidates, nullifier_hash,
    recipient_binding_field, recover_note_receipt, shield_authorization,
    shield_authorization_for_deposit, shield_authorization_for_deposit_type,
    shielder_withdrawal_from_receipt, validate_withdrawal_public_inputs, DenominationTree,
    withdrawal_witness_from_receipt,
    NoteCommitment, NoteReceipt, ShielderProofVerifier, WithdrawalProof, WithdrawalPublicInputs,
};
use wasm_bindgen::prelude::*;

#[wasm_bindgen]
pub fn derive_shield_receipt_json(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> Result<String, JsValue> {
    json(derive_shield_receipt(deposit_id, amount_sats, client_seed))
}

#[wasm_bindgen]
pub fn derive_shield_receipt_for_deposit_json(
    deposit_id: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> Result<String, JsValue> {
    json(derive_shield_receipt_for_deposit(
        deposit_id,
        deposit_index,
        amount_sats,
        client_seed,
    ))
}

#[wasm_bindgen]
pub fn derive_shield_receipt_for_deposit_type_json(
    deposit_id: &str,
    deposit_type: &str,
    deposit_index: u64,
    amount_sats: u64,
    client_seed: &str,
) -> Result<String, JsValue> {
    json(derive_shield_receipt_for_deposit_type(
        deposit_id,
        deposit_type,
        deposit_index,
        amount_sats,
        client_seed,
    ))
}

#[wasm_bindgen]
pub fn client_pubkey_from_secret_json(client_seed: &str) -> Result<String, JsValue> {
    Ok(client_pubkey_from_secret(client_seed))
}

#[wasm_bindgen]
pub fn client_pubkey_for_deposit_json(
    client_seed: &str,
    deposit_index: u64,
) -> Result<String, JsValue> {
    Ok(client_pubkey_for_deposit(client_seed, deposit_index))
}

#[wasm_bindgen]
pub fn client_pubkey_for_deposit_type_json(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
) -> Result<String, JsValue> {
    Ok(client_pubkey_for_deposit_type(
        client_seed,
        deposit_type,
        deposit_index,
    ))
}

#[wasm_bindgen]
pub fn shield_authorization_json(
    client_seed: &str,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments_json: &str,
) -> Result<String, JsValue> {
    let note_commitments = decode_note_commitments(note_commitments_json)?;
    json(Ok(shield_authorization(
        client_seed,
        deposit_id,
        amount_sats,
        &note_commitments,
    )))
}

#[wasm_bindgen]
pub fn shield_authorization_for_deposit_json(
    client_seed: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments_json: &str,
) -> Result<String, JsValue> {
    let note_commitments = decode_note_commitments(note_commitments_json)?;
    json(Ok(shield_authorization_for_deposit(
        client_seed,
        deposit_index,
        deposit_id,
        amount_sats,
        &note_commitments,
    )))
}

#[wasm_bindgen]
pub fn shield_authorization_for_deposit_type_json(
    client_seed: &str,
    deposit_type: &str,
    deposit_index: u64,
    deposit_id: &str,
    amount_sats: u64,
    note_commitments_json: &str,
) -> Result<String, JsValue> {
    let note_commitments = decode_note_commitments(note_commitments_json)?;
    json(Ok(shield_authorization_for_deposit_type(
        client_seed,
        deposit_type,
        deposit_index,
        deposit_id,
        amount_sats,
        &note_commitments,
    )))
}

#[wasm_bindgen]
pub fn merkle_root_json(leaves_json: &str) -> Result<String, JsValue> {
    let leaves = decode_leaves(leaves_json)?;
    Ok(merkle_root(&leaves))
}

#[wasm_bindgen]
pub fn shielder_withdrawal_from_receipt_json(
    note_json: &str,
    client_seed: &str,
    leaves_json: &str,
    recipient: &str,
    fee_sats: u64,
) -> Result<String, JsValue> {
    let note: NoteReceipt = parse_json(note_json)?;
    let leaves = decode_leaves(leaves_json)?;
    let tree = DenominationTree {
        leaves,
        known_roots: Default::default(),
    };
    json(shielder_withdrawal_from_receipt(
        &note,
        client_seed,
        &tree,
        recipient.to_string(),
        fee_sats,
    ))
}

#[wasm_bindgen]
pub fn zk_withdrawal_from_receipt_json(
    note_json: &str,
    client_seed: &str,
    leaves_json: &str,
    recipient: &str,
    fee_sats: u64,
) -> Result<String, JsValue> {
    shielder_withdrawal_from_receipt_json(note_json, client_seed, leaves_json, recipient, fee_sats)
}

#[wasm_bindgen]
pub fn withdrawal_witness_from_receipt_json(
    note_json: &str,
    leaves_json: &str,
    recipient: &str,
    fee_sats: u64,
) -> Result<String, JsValue> {
    let note: NoteReceipt = parse_json(note_json)?;
    let leaves = decode_leaves(leaves_json)?;
    let tree = DenominationTree {
        leaves,
        known_roots: Default::default(),
    };
    json(withdrawal_witness_from_receipt(
        &note,
        &tree,
        recipient.to_string(),
        fee_sats,
    ))
}

#[wasm_bindgen]
pub fn verify_withdrawal_json(proof_json: &str, public_json: &str) -> Result<(), JsValue> {
    let proof: WithdrawalProof = parse_json(proof_json)?;
    let public: WithdrawalPublicInputs = parse_json(public_json)?;
    ShielderProofVerifier
        .verify_withdrawal(&proof, &public)
        .map_err(js_error)
}

#[wasm_bindgen]
pub fn validate_withdrawal_public_json(public_json: &str) -> Result<(), JsValue> {
    let public: WithdrawalPublicInputs = parse_json(public_json)?;
    validate_withdrawal_public_inputs(&public).map_err(js_error)
}

#[wasm_bindgen]
pub fn recipient_binding_field_json(
    recipient: &str,
    fee_sats: u64,
    denomination_sats: u64,
) -> Result<String, JsValue> {
    recipient_binding_field(recipient, fee_sats, denomination_sats).map_err(js_error)
}

#[wasm_bindgen]
pub fn note_recovery_candidates_json(
    client_seed: &str,
    deposit_limit: u64,
    note_limit: u64,
) -> Result<String, JsValue> {
    json(Ok(note_recovery_candidates(
        client_seed,
        deposit_limit,
        note_limit,
    )))
}

#[wasm_bindgen]
pub fn recover_note_receipt_json(
    client_seed: &str,
    deposit_index: u64,
    note_index: u64,
    deposit_id: &str,
    denomination_sats: u64,
    commitment: &str,
) -> Result<String, JsValue> {
    json(recover_note_receipt(
        client_seed,
        deposit_index,
        note_index,
        deposit_id,
        denomination_sats,
        commitment,
    ))
}

#[wasm_bindgen]
pub fn nullifier_hash_json(nullifier: &str) -> Result<String, JsValue> {
    Ok(nullifier_hash(nullifier))
}

fn decode_note_commitments(raw: &str) -> Result<Vec<NoteCommitment>, JsValue> {
    parse_json(raw)
}

fn decode_leaves(raw: &str) -> Result<Vec<String>, JsValue> {
    parse_json(raw)
}

fn parse_json<T: serde::de::DeserializeOwned>(raw: &str) -> Result<T, JsValue> {
    serde_json::from_str(raw).map_err(|error| JsValue::from_str(&error.to_string()))
}

fn json<T: serde::Serialize>(value: thornado_shielder::Result<T>) -> Result<String, JsValue> {
    let value = value.map_err(js_error)?;
    serde_json::to_string(&value).map_err(|error| JsValue::from_str(&error.to_string()))
}

fn js_error(error: impl std::fmt::Display) -> JsValue {
    JsValue::from_str(&error.to_string())
}
