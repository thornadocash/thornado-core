//! Observer attestation quorum core.
//!
//! Ports the pure, testable heart of the Go observer attestation subsystem
//! (`bifrost/observer/generic_attestation.go`, `attestation_gossip.go`): how
//! multiple observers' signed attestations of the same observed event are
//! collected, deduplicated by signer, and checked against a super-majority
//! quorum of the active validator set.
//!
//! Wire types keep byte-compatibility with Go where they cross the network:
//! serde field names match the Go json tags exactly (`PubKey`, `Signature`,
//! `attestations`, `obsTx`, `inbound`, `allowFutureObservation`). Signatures
//! and public keys are treated as opaque bytes — no crypto is performed here;
//! signature verification lives at the network boundary in Go and is out of
//! scope for this pure accumulator.

use serde::{Deserialize, Serialize};

/// Super-majority denominator: quorum needs strictly more than 2/3, i.e.
/// `ceil(2*total/3)`. Mirrors Go `thornadotypes.SuperMajorityFactor`.
pub const SUPER_MAJORITY_FACTOR: usize = 3;

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum AttestationError {
    /// A different signature already exists from this signer for the same item.
    /// Go returns `signature already present for <pubkey>` — a signer must not
    /// produce two distinct signatures for one observed event.
    #[error("signature already present for signer {0}")]
    SignerConflict(String),
}

/// A single observer's signed attestation of an observed event.
///
/// Mirrors Go `common.Attestation`. Both fields are `[]byte` in Go and encode
/// as base64 strings under `encoding/json`, so we serialize them the same way
/// for wire interop. Field names match the Go json tags (`PubKey`,
/// `Signature`) exactly.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Attestation {
    #[serde(rename = "PubKey", with = "serde_b64_bytes")]
    pub pub_key: Vec<u8>,
    #[serde(rename = "Signature", with = "serde_b64_bytes")]
    pub signature: Vec<u8>,
}

impl Attestation {
    pub fn new(pub_key: Vec<u8>, signature: Vec<u8>) -> Self {
        Self { pub_key, signature }
    }
}

/// A batched envelope carrying the observed item together with every
/// attestation collected for it. Mirrors the shape of Go `common.QuorumTx`
/// (and the per-item entries inside `QuorumState`): the `attestations` field
/// name matches the Go json tag, and `item` stands in for the observed payload
/// (`obsTx` etc.) that is opaque to the quorum core.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct QuorumEnvelope {
    /// Opaque grouping key / observed payload identity. In Go this is the
    /// marshaled observed item (`obsTx`, network fee, solvency, errata); here
    /// it is the attestation key that groups attestations for one event.
    #[serde(rename = "obsTx")]
    pub item: String,
    #[serde(rename = "attestations", default)]
    pub attestations: Vec<Attestation>,
    #[serde(rename = "inbound", default)]
    pub inbound: bool,
    #[serde(rename = "allowFutureObservation", default)]
    pub allow_future_observation: bool,
}

/// Super-majority test, byte-for-byte with Go `thornadotypes.HasSuperMajority`.
///
/// Returns false when there are no signers or more signers than the total set.
/// Otherwise requires `signers >= ceil(2*total/3)`. Examples: total=4 needs 3,
/// total=9 needs 6, total=1 needs 1, total=3 needs 2.
pub fn has_super_majority(signers: usize, total: usize) -> bool {
    if signers > total || signers == 0 {
        return false;
    }
    let mut min = total * 2 / SUPER_MAJORITY_FACTOR;
    if (total * 2) % SUPER_MAJORITY_FACTOR > 0 {
        min += 1;
    }
    signers >= min
}

/// Pure accumulator for the attestations of a single observed event.
///
/// Deduplicates by signer public key: a signer's repeated attestation with the
/// same signature is a no-op, and a *different* signature from a signer that
/// already attested is rejected (Go treats this as an error, not a
/// double-count). `has_quorum` applies the same super-majority threshold Go
/// uses against the active validator count.
#[derive(Debug, Clone, Default)]
pub struct AttestationState {
    attestations: Vec<Attestation>,
}

impl AttestationState {
    pub fn new() -> Self {
        Self {
            attestations: Vec::new(),
        }
    }

    /// Add an attestation, deduplicating by signer.
    ///
    /// - Identical signature already present → ignore (Ok, no change), matching
    ///   Go's "already have the signature, ignore".
    /// - Same signer pubkey but a different signature → `SignerConflict`,
    ///   matching Go's "signature already present for <pubkey>" error. This is
    ///   what prevents a signer's later attestation from double-counting.
    /// - Otherwise the attestation is recorded.
    pub fn add(&mut self, attestation: Attestation) -> Result<(), AttestationError> {
        for existing in &self.attestations {
            if existing.signature == attestation.signature {
                return Ok(());
            }
            if existing.pub_key == attestation.pub_key {
                return Err(AttestationError::SignerConflict(hex::encode(
                    &attestation.pub_key,
                )));
            }
        }
        self.attestations.push(attestation);
        Ok(())
    }

    /// Number of distinct signers that have attested.
    pub fn count(&self) -> usize {
        self.attestations.len()
    }

    /// Whether a given signer has already attested.
    pub fn has_signer(&self, pub_key: &[u8]) -> bool {
        self.attestations.iter().any(|a| a.pub_key == pub_key)
    }

    /// Read-only view of the collected attestations.
    pub fn attestations(&self) -> &[Attestation] {
        &self.attestations
    }

    /// True once a super-majority of the `active_validator_count` have attested,
    /// using the same threshold as Go (`HasSuperMajority`).
    pub fn has_quorum(&self, active_validator_count: usize) -> bool {
        has_super_majority(self.count(), active_validator_count)
    }
}

/// Compute the attestation key that groups attestations for the same observed
/// event. Attestations sharing a key belong to one `AttestationState`.
///
/// The Go key (`txKey` / the map keys in `AttestationGossip`) is composed of the
/// properties that uniquely identify an observed item — for an observed tx:
/// chain, id, observed vault pubkey, a unique payload hash, and the
/// finalization/direction flags. We mirror that as a stable, delimited string
/// so identical events collide and distinct ones do not.
pub fn attestation_key(
    chain: &str,
    id: &str,
    observed_pubkey: &str,
    unique_hash: &str,
    allow_future_observation: bool,
    finalized: bool,
    inbound: bool,
) -> String {
    format!(
        "{chain}|{id}|{observed_pubkey}|{unique_hash}|{allow_future_observation}|{finalized}|{inbound}"
    )
}

/// serde helper: Go's `encoding/json` encodes `[]byte` as a base64 string
/// (standard alphabet, with padding). Match that exactly for wire interop.
mod serde_b64_bytes {
    use base64::engine::general_purpose::STANDARD as B64;
    use base64::Engine as _;
    use serde::{Deserialize, Deserializer, Serializer};

    pub fn serialize<S: Serializer>(v: &[u8], s: S) -> Result<S::Ok, S::Error> {
        s.serialize_str(&B64.encode(v))
    }
    pub fn deserialize<'de, D: Deserializer<'de>>(d: D) -> Result<Vec<u8>, D::Error> {
        let s = String::deserialize(d)?;
        B64.decode(&s).map_err(serde::de::Error::custom)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Fabricate an attestation from fake signer `n`: pubkey and signature are
    /// deterministic, distinct byte strings.
    fn att(signer: u8) -> Attestation {
        Attestation::new(vec![signer, 0xAA], vec![signer, 0xBB, 0xCC])
    }

    /// A signer re-signing the same item with a *different* signature.
    fn att_resign(signer: u8) -> Attestation {
        Attestation::new(vec![signer, 0xAA], vec![signer, 0xDD, 0xEE])
    }

    #[test]
    fn quorum_boundary_matches_go() {
        // ceil(2*total/3): the exact thresholds Go's HasSuperMajority yields.
        assert_eq!(has_super_majority(0, 0), false);
        assert!(has_super_majority(1, 1));
        assert_eq!(has_super_majority(1, 2), false);
        assert!(has_super_majority(2, 2));
        assert_eq!(has_super_majority(1, 3), false);
        assert!(has_super_majority(2, 3)); // total=3 -> need 2
        // total=4 -> need 3
        assert_eq!(has_super_majority(2, 4), false);
        assert!(has_super_majority(3, 4));
        // total=9 -> need 6
        assert_eq!(has_super_majority(5, 9), false);
        assert!(has_super_majority(6, 9));
        // more signers than total is never a quorum
        assert_eq!(has_super_majority(5, 4), false);
    }

    #[test]
    fn dedup_same_signature_is_noop() {
        let mut s = AttestationState::new();
        s.add(att(1)).unwrap();
        // exact same attestation again — ignored, count unchanged
        s.add(att(1)).unwrap();
        assert_eq!(s.count(), 1);
    }

    #[test]
    fn signer_resigning_conflicts_and_does_not_double_count() {
        let mut s = AttestationState::new();
        s.add(att(1)).unwrap();
        let err = s.add(att_resign(1)).unwrap_err();
        assert!(matches!(err, AttestationError::SignerConflict(_)));
        // the conflicting later attestation did NOT add a second count
        assert_eq!(s.count(), 1);
    }

    #[test]
    fn distinct_signers_accumulate() {
        let mut s = AttestationState::new();
        for i in 0..4u8 {
            s.add(att(i)).unwrap();
        }
        assert_eq!(s.count(), 4);
        assert!(s.has_signer(&[2, 0xAA]));
        assert!(!s.has_signer(&[9, 0xAA]));
    }

    #[test]
    fn quorum_at_exact_boundary_n4() {
        // N=4 active validators -> quorum at 3.
        let mut s = AttestationState::new();
        s.add(att(0)).unwrap();
        s.add(att(1)).unwrap();
        assert!(!s.has_quorum(4)); // 2/4, below
        s.add(att(2)).unwrap();
        assert!(s.has_quorum(4)); // 3/4, quorum
    }

    #[test]
    fn quorum_at_exact_boundary_n9() {
        // N=9 active validators -> quorum at 6.
        let mut s = AttestationState::new();
        for i in 0..5u8 {
            s.add(att(i)).unwrap();
        }
        assert!(!s.has_quorum(9)); // 5/9, below
        s.add(att(5)).unwrap();
        assert!(s.has_quorum(9)); // 6/9, quorum
    }

    #[test]
    fn duplicate_signers_do_not_reach_quorum() {
        // Only two distinct signers, resigned repeatedly, never reaches 3-of-4.
        let mut s = AttestationState::new();
        s.add(att(0)).unwrap();
        s.add(att(0)).unwrap(); // dup sig -> noop
        s.add(att(1)).unwrap();
        s.add(att(1)).unwrap(); // dup sig -> noop
        assert_eq!(s.count(), 2);
        assert!(!s.has_quorum(4));
    }

    #[test]
    fn attestation_key_groups_and_separates() {
        let a = attestation_key("BTC", "TXID1", "vault", "h1", false, true, true);
        let b = attestation_key("BTC", "TXID1", "vault", "h1", false, true, true);
        let c = attestation_key("BTC", "TXID2", "vault", "h1", false, true, true);
        assert_eq!(a, b); // same event -> same key
        assert_ne!(a, c); // different tx id -> different key
    }

    #[test]
    fn attestation_json_uses_go_field_names_and_base64() {
        let a = Attestation::new(vec![0x01, 0x02, 0x03], vec![0xFF, 0xEE]);
        let j = serde_json::to_string(&a).unwrap();
        // Go json tags: PubKey / Signature; []byte -> standard base64.
        assert!(j.contains("\"PubKey\":\"AQID\""));
        assert!(j.contains("\"Signature\":\"/+4=\""));
        let back: Attestation = serde_json::from_str(&j).unwrap();
        assert_eq!(back, a);
    }

    #[test]
    fn envelope_json_field_names_match_go() {
        let env = QuorumEnvelope {
            item: "key".into(),
            attestations: vec![att(0)],
            inbound: true,
            allow_future_observation: false,
        };
        let j = serde_json::to_string(&env).unwrap();
        assert!(j.contains("\"obsTx\":"));
        assert!(j.contains("\"attestations\":"));
        assert!(j.contains("\"inbound\":true"));
        assert!(j.contains("\"allowFutureObservation\":false"));
        let back: QuorumEnvelope = serde_json::from_str(&j).unwrap();
        assert_eq!(back, env);
    }
}
