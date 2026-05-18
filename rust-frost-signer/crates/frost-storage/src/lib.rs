use std::collections::HashMap;

use thiserror::Error;
use thornado_frost_core::{
    mock_signature, validate_signing_request, SessionId, SignerError, SigningRequest,
    SigningResponse, SigningSession, SigningStatus, VaultId, VaultShare,
};

#[derive(Debug, Error)]
pub enum StorageError {
    #[error("record not found")]
    NotFound,
    #[error("signer policy error: {0}")]
    Policy(#[from] SignerError),
}

pub trait ShareStore {
    fn get_vault_share(&self, vault_id: &VaultId) -> Result<VaultShare, StorageError>;
    fn put_vault_share(&mut self, share: VaultShare) -> Result<(), StorageError>;
}

pub trait SigningSessionStore {
    fn get_signing_session(
        &self,
        session_id: &SessionId,
    ) -> Result<Option<SigningSession>, StorageError>;

    fn put_signing_session(&mut self, session: SigningSession) -> Result<(), StorageError>;
}

#[derive(Default)]
pub struct InMemoryStore {
    vault_shares: HashMap<VaultId, VaultShare>,
    signing_sessions: HashMap<SessionId, SigningSession>,
}

impl InMemoryStore {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn start_dev_signing(
        &mut self,
        request: SigningRequest,
    ) -> Result<SigningResponse, StorageError> {
        let vault_share = self.get_vault_share(&request.vault_id)?;
        validate_signing_request(&request, &vault_share)?;
        self.ensure_no_conflicting_session(&request)?;

        if let Some(existing) = self.get_signing_session(&request.session_id)? {
            return Ok(SigningResponse {
                session_id: existing.session_id,
                status: existing.status,
                signature: existing.partial_signature,
            });
        }

        let signature = mock_signature(&request);
        let session = SigningSession {
            session_id: request.session_id.clone(),
            vault_id: request.vault_id.clone(),
            signing_payload: request.signing_payload.clone(),
            policy: request.policy,
            nonce_commitments: Vec::new(),
            local_nonce_state: None,
            partial_signature: Some(signature.clone()),
            status: SigningStatus::Complete,
        };
        self.put_signing_session(session)?;

        Ok(SigningResponse {
            session_id: request.session_id,
            status: SigningStatus::Complete,
            signature: Some(signature),
        })
    }

    pub fn ensure_no_conflicting_session(
        &self,
        request: &SigningRequest,
    ) -> Result<(), StorageError> {
        if let Some(existing) = self.signing_sessions.get(&request.session_id) {
            if existing.vault_id != request.vault_id
                || existing.signing_payload != request.signing_payload
            {
                return Err(SignerError::ConflictingSession.into());
            }
        }

        Ok(())
    }
}

impl ShareStore for InMemoryStore {
    fn get_vault_share(&self, vault_id: &VaultId) -> Result<VaultShare, StorageError> {
        self.vault_shares
            .get(vault_id)
            .cloned()
            .ok_or(StorageError::NotFound)
    }

    fn put_vault_share(&mut self, share: VaultShare) -> Result<(), StorageError> {
        self.vault_shares.insert(share.vault_id.clone(), share);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use thornado_frost_core::{Participant, ShareStatus, SigningPolicy};

    fn participant(index: u16) -> Participant {
        Participant {
            node_id: format!("node-{index}"),
            index,
            public_key: vec![index as u8; 33],
        }
    }

    fn share() -> VaultShare {
        VaultShare {
            vault_id: VaultId("vault-1".to_string()),
            group_public_key: vec![7; 33],
            participant_index: 1,
            threshold: 2,
            participants: vec![participant(1), participant(2), participant(3)],
            encrypted_secret_share: b"dev-only-share".to_vec(),
            dkg_transcript_hash: b"transcript-hash".to_vec(),
            creation_height: 42,
            status: ShareStatus::Active,
        }
    }

    fn request(session_id: &str, payload: &[u8]) -> SigningRequest {
        SigningRequest {
            session_id: SessionId(session_id.to_string()),
            vault_id: VaultId("vault-1".to_string()),
            signing_payload: payload.to_vec(),
            policy: SigningPolicy {
                chain_id: "bitcoin".to_string(),
                outbound_id: "outbound-1".to_string(),
                destination_address: "bcrt1qexample".to_string(),
                amount_sats: 50_000,
                fee_rate_sats_per_vbyte: 3,
                thornode_height: 100,
                expected_vault_public_key: vec![7; 33],
                expected_signers: vec![participant(1), participant(2), participant(3)],
                expires_at_height: 110,
            },
        }
    }

    #[test]
    fn dev_signer_persists_deterministic_completed_session() {
        let mut store = InMemoryStore::new();
        store.put_vault_share(share()).unwrap();

        let response = store
            .start_dev_signing(request("session-1", b"sighash"))
            .unwrap();
        let replay = store
            .start_dev_signing(request("session-1", b"sighash"))
            .unwrap();

        assert_eq!(response.status, SigningStatus::Complete);
        assert_eq!(response.signature, replay.signature);
        assert_eq!(
            store
                .get_signing_session(&SessionId("session-1".to_string()))
                .unwrap()
                .unwrap()
                .status,
            SigningStatus::Complete
        );
    }

    #[test]
    fn dev_signer_rejects_conflicting_session_reuse() {
        let mut store = InMemoryStore::new();
        store.put_vault_share(share()).unwrap();
        store
            .start_dev_signing(request("session-1", b"sighash-a"))
            .unwrap();

        let err = store
            .start_dev_signing(request("session-1", b"sighash-b"))
            .unwrap_err();

        assert!(matches!(
            err,
            StorageError::Policy(SignerError::ConflictingSession)
        ));
    }

    #[test]
    fn dev_signer_requires_local_vault_share() {
        let mut store = InMemoryStore::new();

        let err = store
            .start_dev_signing(request("session-1", b"sighash"))
            .unwrap_err();

        assert!(matches!(err, StorageError::NotFound));
    }
}

impl SigningSessionStore for InMemoryStore {
    fn get_signing_session(
        &self,
        session_id: &SessionId,
    ) -> Result<Option<SigningSession>, StorageError> {
        Ok(self.signing_sessions.get(session_id).cloned())
    }

    fn put_signing_session(&mut self, session: SigningSession) -> Result<(), StorageError> {
        self.signing_sessions
            .insert(session.session_id.clone(), session);
        Ok(())
    }
}
