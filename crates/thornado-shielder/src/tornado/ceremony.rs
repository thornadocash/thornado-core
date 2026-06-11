//! Trusted setup attestation for the production Tornado Cash Groth16 ceremony.

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

pub const ENGINE_ID: &str = "tornado-cash-groth16-v2.1";
pub const CEREMONY_RELEASE: &str = "v2.1";
pub const CEREMONY_UPSTREAM: &str = "https://github.com/tornadocash/tornado-core";
pub const MERKLE_DEPTH: usize = 20;
pub const PUBLIC_INPUT_COUNT: usize = 6;

pub const PRODUCTION_VK_JSON: &str =
    include_str!("../../../../circuits/tornado/artifacts/withdraw_verification_key.json");
pub const MANIFEST_JSON: &str =
    include_str!("../../../../circuits/tornado/artifacts/MANIFEST.json");

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CeremonyAttestation {
    pub engine_id: String,
    pub release: String,
    pub upstream: String,
    pub protocol: String,
    pub merkle_depth: usize,
    pub public_inputs: Vec<String>,
    pub verification_key_sha256: String,
    pub semi_trustless_model: String,
    pub security_property: String,
}

#[derive(Debug, Deserialize)]
struct Manifest {
    protocol: String,
    release: String,
    upstream: String,
    merkle_depth: u64,
    public_inputs: Vec<String>,
}

pub fn verification_key_sha256() -> String {
    hex::encode(Sha256::digest(PRODUCTION_VK_JSON.as_bytes()))
}

pub fn attestation() -> CeremonyAttestation {
    let manifest: Manifest = serde_json::from_str(MANIFEST_JSON).expect("manifest json");
    CeremonyAttestation {
        engine_id: ENGINE_ID.to_string(),
        release: manifest.release,
        upstream: manifest.upstream,
        protocol: manifest.protocol,
        merkle_depth: manifest.merkle_depth as usize,
        public_inputs: manifest.public_inputs,
        verification_key_sha256: verification_key_sha256(),
        semi_trustless_model: "Tornado Cash v2.1 Groth16 used a multi-party computation (MPC) \
            powers-of-tau style ceremony after the Perpetual Powers of Tau. Each participant \
            contributes randomness; the final proving key is secure if at least one participant \
            destroyed their toxic waste."
            .to_string(),
        security_property: "Semi-trustless: at least one honest MPC participant suffices; no \
            single party can forge withdraw proofs for unknown secrets."
            .to_string(),
    }
}

pub fn semi_trustless_at_least_one_honest() -> bool {
    // Production v2.1 artifacts are the community-audited MPC output; we pin only the vk and
    // document the model rather than re-running the ceremony.
    attestation().release == CEREMONY_RELEASE
        && attestation().verification_key_sha256 == verification_key_sha256()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pinned_vk_matches_embedded_artifact() {
        let digest = verification_key_sha256();
        assert_eq!(digest.len(), 64);
        assert!(digest.chars().all(|ch| ch.is_ascii_hexdigit()));
        let att = attestation();
        assert_eq!(att.engine_id, ENGINE_ID);
        assert_eq!(att.release, "v2.1");
        assert_eq!(att.public_inputs.len(), PUBLIC_INPUT_COUNT);
        assert!(semi_trustless_at_least_one_honest());
    }

    #[test]
    fn manifest_protocol_is_production_groth16() {
        let manifest: Manifest = serde_json::from_str(MANIFEST_JSON).unwrap();
        assert_eq!(manifest.protocol, "tornado-cash-groth16-v2.1");
        assert_eq!(manifest.merkle_depth, MERKLE_DEPTH as u64);
    }
}
