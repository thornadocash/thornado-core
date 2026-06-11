//! Withdraw proof generation and verification (production Tornado Cash Groth16).

use ark_bn254::Fr;
use ark_ff::Zero;
use serde::{Deserialize, Serialize};

use super::field::{
    fr_from_decimal, fr_from_field_hex, fr_to_decimal, u64_to_fr, PUBLIC_INPUT_COUNT,
};
use super::groth16::{public_inputs_from_withdraw, verify_snarkjs_proof, SnarkjsProof};
use super::hash::{field_from_hex, field_to_hex, note_commitment, nullifier_hash};
use super::merkle::{incremental_root, merkle_path, verify_merkle_path};
#[cfg(feature = "proof-tests")]
use super::merkle::{MerklePath, MERKLE_TREE_DEPTH};
use crate::{Result, WithdrawalProof, WithdrawalPublicInputs};

pub use super::ceremony::ENGINE_ID as PROTOCOL_ID;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TornadoWithdrawProof {
    pub protocol: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub groth16: Option<SnarkjsProof>,
}

pub fn create_note_commitment(nullifier_hex: &str, secret_hex: &str) -> Result<String> {
    let nullifier = field_from_hex(nullifier_hex).ok_or(crate::Error::InvalidProof)?;
    let secret = field_from_hex(secret_hex).ok_or(crate::Error::InvalidProof)?;
    Ok(field_to_hex(note_commitment(nullifier, secret)?))
}

pub fn merkle_root_hex(leaves: &[String]) -> Result<String> {
    super::merkle::merkle_root_hex(leaves)
}

fn expected_recipient_binding(public: &WithdrawalPublicInputs) -> Result<Fr> {
    super::hash::recipient_binding(&public.recipient, public.fee_sats, public.denomination_sats)
}

fn enforce_public_field(explicit: Option<&String>, expected: Fr) -> Result<Fr> {
    if let Some(raw) = explicit {
        let trimmed = raw.trim();
        if !trimmed.is_empty() {
            let parsed = fr_from_decimal(trimmed).ok_or(crate::Error::InvalidProof)?;
            if parsed != expected {
                return Err(crate::Error::InvalidProof);
            }
        }
    }
    Ok(expected)
}

fn enforce_zero_public_field(explicit: Option<&String>) -> Result<()> {
    if let Some(raw) = explicit {
        let trimmed = raw.trim();
        if trimmed.is_empty() {
            return Ok(());
        }
        if fr_from_decimal(trimmed) != Some(Fr::zero()) {
            return Err(crate::Error::InvalidProof);
        }
    }
    Ok(())
}

pub fn validate_public_inputs(public: &WithdrawalPublicInputs) -> Result<()> {
    if public.recipient.trim().is_empty() {
        return Err(crate::Error::InvalidProof);
    }
    if public.fee_sats >= public.denomination_sats {
        return Err(crate::Error::InvalidProof);
    }
    enforce_public_field(
        public.recipient_field.as_ref(),
        expected_recipient_binding(public)?,
    )?;
    enforce_zero_public_field(public.relayer_field.as_ref())?;
    enforce_zero_public_field(public.refund_field.as_ref())?;
    Ok(())
}

pub fn prove_withdrawal(
    nullifier_hex: &str,
    secret_hex: &str,
    leaves: &[String],
    leaf_index: usize,
    public: &WithdrawalPublicInputs,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    let nullifier = field_from_hex(nullifier_hex).ok_or(crate::Error::InvalidProof)?;
    let secret = field_from_hex(secret_hex).ok_or(crate::Error::InvalidProof)?;
    let commitment = note_commitment(nullifier, secret)?;
    let nf_hash = nullifier_hash(nullifier)?;
    let parsed_leaves: Result<Vec<Fr>> = leaves
        .iter()
        .map(|leaf| fr_from_field_hex(leaf).ok_or(crate::Error::InvalidProof))
        .collect();
    let parsed_leaves = parsed_leaves?;
    if leaf_index >= parsed_leaves.len() || parsed_leaves[leaf_index] != commitment {
        return Err(crate::Error::InvalidProof);
    }
    let root = incremental_root(&parsed_leaves)?;
    let path = merkle_path(&parsed_leaves, leaf_index)?;
    verify_merkle_path(&field_to_hex(root), &field_to_hex(commitment), &path)?;

    validate_public_inputs(public)?;
    let recipient = expected_recipient_binding(public)?;
    let relayer = Fr::zero();
    let refund = Fr::zero();
    #[cfg(feature = "proof-tests")]
    let groth16 = {
        let fee = u64_to_fr(public.fee_sats);
        let public_inputs =
            public_inputs_from_withdraw(root, nf_hash, recipient, relayer, fee, refund);
        Some(prove_groth16(nullifier, secret, &path, &public_inputs)?)
    };
    #[cfg(not(feature = "proof-tests"))]
    let groth16 = None;

    let mut public_out = public.clone();
    public_out.nullifier_hash = fr_to_decimal(nf_hash);
    public_out.merkle_root = fr_to_decimal(root);
    public_out.recipient_field = Some(fr_to_decimal(recipient));
    public_out.relayer_field = Some(fr_to_decimal(relayer));
    public_out.refund_field = Some(fr_to_decimal(refund));

    let proof = WithdrawalProof {
        nullifier: String::new(),
        secret: String::new(),
        commitment: String::new(),
        merkle_root: public_out.merkle_root.clone(),
        tornado: Some(TornadoWithdrawProof {
            protocol: PROTOCOL_ID.to_string(),
            groth16,
        }),
    };
    Ok((proof, public_out))
}

pub fn verify_withdrawal(proof: &WithdrawalProof, public: &WithdrawalPublicInputs) -> Result<()> {
    validate_public_inputs(public)?;
    let tornado = proof.tornado.as_ref().ok_or(crate::Error::InvalidProof)?;
    if tornado.protocol != PROTOCOL_ID {
        return Err(crate::Error::InvalidProof);
    }
    if proof.merkle_root != public.merkle_root {
        return Err(crate::Error::InvalidProof);
    }
    let root = fr_from_decimal(&public.merkle_root).ok_or(crate::Error::InvalidProof)?;
    let nf_hash = fr_from_decimal(&public.nullifier_hash).ok_or(crate::Error::InvalidProof)?;
    let recipient = expected_recipient_binding(public)?;
    let relayer = Fr::zero();
    let fee = u64_to_fr(public.fee_sats);
    let refund = Fr::zero();
    let public_inputs = public_inputs_from_withdraw(root, nf_hash, recipient, relayer, fee, refund);
    let groth16 = tornado.groth16.as_ref().ok_or(crate::Error::InvalidProof)?;
    verify_snarkjs_proof(groth16, &public_inputs)?;
    Ok(())
}

pub fn redact_private_fields(proof: &mut WithdrawalProof) {
    proof.nullifier.clear();
    proof.secret.clear();
    proof.commitment.clear();
}

pub fn public_input_count() -> usize {
    PUBLIC_INPUT_COUNT
}

#[cfg(feature = "proof-tests")]
fn prove_groth16(
    nullifier: Fr,
    secret: Fr,
    path: &MerklePath,
    public_inputs: &[Fr; 6],
) -> Result<SnarkjsProof> {
    use std::io::Write;
    use std::process::{Command, Stdio};

    if path.path_elements.len() != MERKLE_TREE_DEPTH {
        return Err(crate::Error::InvalidProof);
    }
    let witness = serde_json::json!({
        "nullifier": fr_to_decimal(nullifier),
        "secret": fr_to_decimal(secret),
        "pathElements": path.path_elements,
        "pathIndices": path.path_indices,
        "root": fr_to_decimal(public_inputs[0]),
        "nullifierHash": fr_to_decimal(public_inputs[1]),
        "recipient": fr_to_decimal(public_inputs[2]),
        "relayer": fr_to_decimal(public_inputs[3]),
        "fee": fr_to_decimal(public_inputs[4]),
        "refund": fr_to_decimal(public_inputs[5]),
    });
    let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
    let script = manifest_dir.join("../../circuits/tornado/scripts/prove-withdraw.mjs");
    if !script.exists() {
        return Err(crate::Error::Shielder(
            "missing circuits/tornado/scripts/prove-withdraw.mjs".into(),
        ));
    }
    let mut child = Command::new("node")
        .arg(script)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|err| crate::Error::Shielder(err.to_string()))?;
    child
        .stdin
        .take()
        .ok_or_else(|| crate::Error::Shielder("prover stdin unavailable".into()))?
        .write_all(witness.to_string().as_bytes())
        .map_err(|err| crate::Error::Shielder(err.to_string()))?;
    let output = child
        .wait_with_output()
        .map_err(|err| crate::Error::Shielder(err.to_string()))?;
    if !output.status.success() {
        return Err(crate::Error::Shielder(
            String::from_utf8_lossy(&output.stderr).into_owned(),
        ));
    }
    serde_json::from_slice(&output.stdout).map_err(|_| crate::Error::InvalidProof)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn merkle_and_commitment_native_checks() -> Result<()> {
        // Covered by `hash::matches_circomlib_vectors`; keep a lightweight smoke hook here.
        let _ = create_note_commitment(
            "0101010101010101010101010101010101010101010101010101010101010101",
            "0202020202020202020202020202020202020202020202020202020202020202",
        )?;
        Ok(())
    }

    #[test]
    fn rejects_mismatched_recipient_field() {
        let public = WithdrawalPublicInputs {
            nullifier_hash: "1".into(),
            denomination_sats: 100_000,
            recipient: "bcrt1qrecipient".into(),
            fee_sats: 1_000,
            merkle_root: "2".into(),
            recipient_field: Some("999".into()),
            relayer_field: None,
            refund_field: None,
        };
        assert_eq!(
            validate_public_inputs(&public),
            Err(crate::Error::InvalidProof)
        );
    }

    #[test]
    #[cfg_attr(
        not(feature = "proof-tests"),
        ignore = "requires production proving key; run with `cargo test -p thornado-shielder --features proof-tests` after `npm run download-artifacts`"
    )]
    fn groth16_withdraw_roundtrip() {
        let nullifier_hex = format!("00{}", hex::encode([3_u8; 31]));
        let secret_hex = format!("00{}", hex::encode([4_u8; 31]));
        let commitment = create_note_commitment(&nullifier_hex, &secret_hex).unwrap();
        let leaves = vec![commitment.clone()];
        let public = WithdrawalPublicInputs {
            nullifier_hash: String::new(),
            denomination_sats: 100_000,
            recipient: "bcrt1qrecipient".to_string(),
            fee_sats: 1_000,
            merkle_root: merkle_root_hex(&leaves).unwrap(),
            recipient_field: None,
            relayer_field: None,
            refund_field: None,
        };
        let (mut proof, public_out) =
            prove_withdrawal(&nullifier_hex, &secret_hex, &leaves, 0, &public).unwrap();
        verify_withdrawal(&proof, &public_out).unwrap();
        redact_private_fields(&mut proof);
        verify_withdrawal(&proof, &public_out).unwrap();
    }
}
