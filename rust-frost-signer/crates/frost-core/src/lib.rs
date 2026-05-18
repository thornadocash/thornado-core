use serde::{Deserialize, Serialize};
use thiserror::Error;
pub use thornado_core::{
    verify_custody_signature, CustodySignature, FrostCustodySigner, FrostCustodySignerSnapshot,
    FrostDkgRound1Output, FrostDkgRound1Public, FrostDkgRound2Output, FrostDkgRound2Public,
    FrostKeyset, FrostSignatureShare, FrostSigningCommitment, FrostSigningCommitmentPublic,
    WithdrawalRequest,
};

#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct VaultId(pub String);

#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct SessionId(pub String);

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct Participant {
    pub node_id: String,
    pub index: u16,
    pub public_key: Vec<u8>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub enum ShareStatus {
    PendingDkg,
    Active,
    Retiring,
    Forgotten,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct VaultShare {
    pub vault_id: VaultId,
    pub group_public_key: Vec<u8>,
    pub participant_index: u16,
    pub threshold: u16,
    pub participants: Vec<Participant>,
    pub encrypted_secret_share: Vec<u8>,
    pub dkg_transcript_hash: Vec<u8>,
    pub creation_height: u64,
    pub status: ShareStatus,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct SigningPolicy {
    pub chain_id: String,
    pub outbound_id: String,
    pub destination_address: String,
    pub amount_sats: u64,
    pub fee_rate_sats_per_vbyte: u64,
    pub thornode_height: u64,
    pub expected_vault_public_key: Vec<u8>,
    pub expected_signers: Vec<Participant>,
    pub expires_at_height: u64,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct SigningRequest {
    pub session_id: SessionId,
    pub vault_id: VaultId,
    pub signing_payload: Vec<u8>,
    pub policy: SigningPolicy,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub enum SigningStatus {
    Pending,
    Complete,
    Failed,
    Expired,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct SigningSession {
    pub session_id: SessionId,
    pub vault_id: VaultId,
    pub signing_payload: Vec<u8>,
    pub policy: SigningPolicy,
    pub nonce_commitments: Vec<Vec<u8>>,
    pub local_nonce_state: Option<Vec<u8>>,
    pub partial_signature: Option<Vec<u8>>,
    pub status: SigningStatus,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct SigningResponse {
    pub session_id: SessionId,
    pub status: SigningStatus,
    pub signature: Option<Vec<u8>>,
}

#[derive(Debug, Error)]
pub enum SignerError {
    #[error("vault share not found")]
    VaultNotFound,
    #[error("vault share is not active")]
    VaultNotActive,
    #[error("only bitcoin signing requests are supported")]
    UnsupportedChain,
    #[error("signing request is expired")]
    Expired,
    #[error("session id already exists for a conflicting payload")]
    ConflictingSession,
    #[error("missing required signing policy field: {0}")]
    MissingPolicyField(&'static str),
    #[error("signing policy mismatch: {0}")]
    PolicyMismatch(&'static str),
}

pub fn validate_signing_request(
    request: &SigningRequest,
    vault_share: &VaultShare,
) -> Result<(), SignerError> {
    if vault_share.status != ShareStatus::Active {
        return Err(SignerError::VaultNotActive);
    }
    if request.policy.chain_id != "bitcoin" {
        return Err(SignerError::UnsupportedChain);
    }
    if request.policy.thornode_height > request.policy.expires_at_height {
        return Err(SignerError::Expired);
    }
    if request.policy.outbound_id.is_empty() {
        return Err(SignerError::MissingPolicyField("outbound_id"));
    }
    if request.policy.destination_address.is_empty() {
        return Err(SignerError::MissingPolicyField("destination_address"));
    }
    if request.policy.amount_sats == 0 {
        return Err(SignerError::MissingPolicyField("amount_sats"));
    }
    if request.policy.fee_rate_sats_per_vbyte == 0 {
        return Err(SignerError::MissingPolicyField("fee_rate_sats_per_vbyte"));
    }
    if request.policy.expected_signers.is_empty() {
        return Err(SignerError::MissingPolicyField("expected_signers"));
    }
    if request.signing_payload.is_empty() {
        return Err(SignerError::MissingPolicyField("signing_payload"));
    }
    if request.policy.expected_vault_public_key != vault_share.group_public_key {
        return Err(SignerError::PolicyMismatch("expected_vault_public_key"));
    }
    if request.policy.expected_signers != vault_share.participants {
        return Err(SignerError::PolicyMismatch("expected_signers"));
    }

    Ok(())
}

pub fn mock_signature(request: &SigningRequest) -> Vec<u8> {
    let mut out = b"thornado-dev-frost-signature:".to_vec();
    out.extend_from_slice(request.session_id.0.as_bytes());
    out.push(b':');
    out.extend_from_slice(request.vault_id.0.as_bytes());
    out.push(b':');
    out.extend_from_slice(&request.signing_payload);
    out
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct GenerateSignerRequest {
    pub max_signers: u16,
    pub threshold: Option<u16>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SignWithdrawalRequest {
    pub withdrawal: WithdrawalRequest,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub key_tweak: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct SignerInfo {
    pub signer_ids: Vec<String>,
    pub threshold: u16,
    pub max_signers: u16,
    pub group_public_key: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub taproot_group_public_key: Option<String>,
}

pub fn signer_info(signer: &FrostCustodySigner) -> thornado_core::Result<SignerInfo> {
    let keyset = signer.to_keyset(0)?;
    Ok(SignerInfo {
        signer_ids: signer.signer_ids(),
        threshold: keyset.threshold,
        max_signers: keyset.max_signers,
        group_public_key: keyset.group_public_key,
        taproot_group_public_key: keyset.taproot_group_public_key,
    })
}

pub fn generate_signer(
    request: GenerateSignerRequest,
) -> thornado_core::Result<FrostCustodySigner> {
    let threshold = request
        .threshold
        .unwrap_or_else(|| thornado_core::frost_threshold_for_committee(request.max_signers));
    FrostCustodySigner::generate_with_dkg(request.max_signers, threshold)
}

pub fn sign_with_existing_engine(
    signer: &FrostCustodySigner,
    request: &SignWithdrawalRequest,
) -> thornado_core::Result<CustodySignature> {
    let coordinator = signer.coordinator();
    let nodes = signer.signer_nodes();
    let signature = match request.key_tweak.as_deref() {
        Some(tweak) => coordinator.sign_with_child_tweak(&request.withdrawal, &nodes, tweak)?,
        None => coordinator.sign_with_nodes(&request.withdrawal, &nodes)?,
    };
    verify_custody_signature(&request.withdrawal, &signature)?;
    Ok(signature)
}

#[cfg(test)]
mod engine_tests {
    use super::*;

    fn withdrawal() -> WithdrawalRequest {
        WithdrawalRequest {
            withdrawal_id: "wd-sidecar".to_string(),
            recipient: "tb1qrecipient".to_string(),
            amount_sats: 99_900_000,
            fee_sats: 100_000,
            nullifier_hash: "nullifier-hash".to_string(),
        }
    }

    #[test]
    fn existing_frost_engine_signs_and_verifies() {
        let signer = generate_signer(GenerateSignerRequest {
            max_signers: 5,
            threshold: None,
        })
        .unwrap();
        let request = SignWithdrawalRequest {
            withdrawal: withdrawal(),
            key_tweak: None,
        };

        let signature = sign_with_existing_engine(&signer, &request).unwrap();

        assert_eq!(signature.scheme, "frost-secp256k1-sha256");
        assert_eq!(signature.signer, "frost-4-of-5");
        verify_custody_signature(&request.withdrawal, &signature).unwrap();
    }

    #[test]
    fn existing_frost_engine_snapshot_roundtrips() {
        let signer = generate_signer(GenerateSignerRequest {
            max_signers: 5,
            threshold: Some(4),
        })
        .unwrap();
        let snapshot = signer.to_snapshot().unwrap();
        let restored = FrostCustodySigner::from_snapshot(&snapshot).unwrap();

        assert_eq!(
            signer_info(&signer).unwrap().group_public_key,
            signer_info(&restored).unwrap().group_public_key
        );
    }
}
