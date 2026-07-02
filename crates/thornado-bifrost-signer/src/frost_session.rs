//! FROST keysign session engine and wire types.
//!
//! Byte-for-byte interop with Go peers speaking `/p2p/frost`. See
//! `docs/port-spec-frost-session.md`. This module owns the message envelope,
//! the session ID derivation, and the round state machine over
//! `frost-secp256k1-tr` v3.

use std::collections::BTreeMap;

use base64::{engine::general_purpose::STANDARD as B64, Engine as _};
use frost_secp256k1_tr as frost;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

/// libp2p protocol ID for FROST sessions. Must match the Go constant.
pub const FROST_PROTOCOL_ID: &str = "/p2p/frost";
/// Length-prefix header size (u32 little-endian).
pub const LENGTH_HEADER: usize = 4;
/// Maximum frame payload (20MB), matching Go `MaxPayload`.
pub const MAX_PAYLOAD: usize = 20_000_000;

/// WrappedMessage.message_type discriminants (match Go).
pub const MSG_TYPE_KEYGEN: u8 = 6;
pub const MSG_TYPE_KEYSIGN: u8 = 7;

#[derive(Debug, thiserror::Error)]
pub enum FrostError {
    #[error("frost: {0}")]
    Frost(String),
    #[error("codec: {0}")]
    Codec(String),
    #[error("local party not selected")]
    LocalPartyNotSelected,
    #[error("unknown message kind: {0}")]
    UnknownKind(String),
    /// A protocol abort where FROST identified the misbehaving parties, or a
    /// remote peer broadcast an abort. `culprits` holds participant names.
    #[error("identifiable abort in {phase}: culprits {culprits:?}")]
    IdentifiableAbort {
        phase: String,
        culprits: Vec<String>,
    },
}

type Result<T> = std::result::Result<T, FrostError>;

/// Records why a session aborted and who was blamed.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AbortInfo {
    pub phase: String,
    pub culprits: Vec<String>,
}

/// Map an identifier (1-based, big-endian last two bytes) to a participant
/// name, if it's within range of the participant list.
fn name_for_identifier(participants: &[String], id: &frost::Identifier) -> Option<String> {
    let bytes = id.serialize();
    let idx = u16::from_be_bytes([bytes[30], bytes[31]]);
    participants.get(idx.checked_sub(1)? as usize).cloned()
}

/// Turn a FROST error into blamed participant names via `Error::culprits()`.
fn blame_names(participants: &[String], err: &frost::Error) -> Vec<String> {
    err.culprits()
        .iter()
        .filter_map(|id| name_for_identifier(participants, id))
        .collect()
}

/// Build a broadcast abort ProtocolMessage naming the culprits.
fn abort_message(kind: &str, from: &str, to: &[String], culprits: &[String]) -> ProtocolMessage {
    let payload = serde_json::to_vec(culprits).unwrap_or_default();
    ProtocolMessage {
        kind: kind.to_string(),
        from: from.to_string(),
        to: to.iter().filter(|p| *p != from).cloned().collect(),
        payload: ProtocolMessage::encode_payload(&payload),
    }
}

/// Outer libp2p envelope. `payload` carries `ProtocolMessage` JSON bytes.
///
/// Go marshals the `Payload []byte` field as a base64 string (encoding/json
/// default for `[]byte`), so we serialize it the same way for wire interop.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WrappedMessage {
    pub message_type: u8,
    pub message_id: String,
    #[serde(with = "serde_b64_bytes")]
    pub payload: Vec<u8>,
}

/// Round message exchanged between parties.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProtocolMessage {
    pub kind: String,
    pub from: String,
    pub to: Vec<String>,
    /// base64(frost package bytes)
    pub payload: String,
}

impl ProtocolMessage {
    pub fn decode_payload(&self) -> Result<Vec<u8>> {
        B64.decode(&self.payload)
            .map_err(|e| FrostError::Codec(e.to_string()))
    }
    pub fn encode_payload(bytes: &[u8]) -> String {
        B64.encode(bytes)
    }
}

/// Serialized keyshare, JSON-compatible with Go `StoredShare`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StoredShare {
    pub version: u32,
    pub engine: String,
    pub participant: String,
    pub participants: Vec<String>,
    pub participant_index: u16,
    pub min_signers: u16,
    pub max_signers: u16,
    pub public_key_compressed: String,
    pub key_package: String,
    pub public_key_package: String,
}

impl StoredShare {
    pub fn key_package(&self) -> Result<frost::keys::KeyPackage> {
        let bytes = B64
            .decode(&self.key_package)
            .map_err(|e| FrostError::Codec(e.to_string()))?;
        frost::keys::KeyPackage::deserialize(&bytes).map_err(|e| FrostError::Frost(e.to_string()))
    }
    pub fn public_key_package(&self) -> Result<frost::keys::PublicKeyPackage> {
        let bytes = B64
            .decode(&self.public_key_package)
            .map_err(|e| FrostError::Codec(e.to_string()))?;
        frost::keys::PublicKeyPackage::deserialize(&bytes)
            .map_err(|e| FrostError::Frost(e.to_string()))
    }
}

/// Sorted, normalized participant list (lexicographic, deduped).
pub fn normalize_participants(raw: &[String]) -> Vec<String> {
    let mut v: Vec<String> = raw.to_vec();
    v.sort();
    v.dedup();
    v
}

/// Map a participant string to its FROST Identifier via 1-based position.
pub fn identifier_for(participants: &[String], party: &str) -> Result<frost::Identifier> {
    let idx = participants
        .iter()
        .position(|p| p == party)
        .ok_or(FrostError::LocalPartyNotSelected)?;
    let index = (idx + 1) as u16;
    frost::Identifier::try_from(index).map_err(|e| FrostError::Frost(e.to_string()))
}

/// Keysign session ID: SHA256(vaultPubKey || message), hex-encoded.
pub fn keysign_session_id(vault_pubkey: &[u8], message: &[u8]) -> String {
    let mut h = Sha256::new();
    h.update(vault_pubkey);
    h.update(message);
    hex::encode(h.finalize())
}

/// Keygen session ID: SHA256("keygen|<height>|<minSigners>|<sortedCsv>"), hex.
pub fn keygen_session_id(height: i64, min_signers: u16, participants: &[String]) -> String {
    let csv = participants.join(",");
    let s = format!("keygen|{height}|{min_signers}|{csv}");
    hex::encode(Sha256::digest(s.as_bytes()))
}

/// Distributed key generation session (FROST DKG, no dealer).
///
/// Mirrors the Go/FFI `KeygenSession`: `part1` → exchange round1 packages →
/// `part2` → exchange targeted round2 packages → `part3` → `StoredShare`.
/// Every party runs its own instance; the group secret is never reconstructed.
pub struct KeygenSession {
    local: String,
    participants: Vec<String>,
    local_index: u16,
    min_signers: u16,
    round1_secret: Option<frost::keys::dkg::round1::SecretPackage>,
    round1_packages: BTreeMap<frost::Identifier, frost::keys::dkg::round1::Package>,
    round2_secret: Option<frost::keys::dkg::round2::SecretPackage>,
    round2_packages: BTreeMap<frost::Identifier, frost::keys::dkg::round2::Package>,
    outputs: Vec<ProtocolMessage>,
    result: Option<StoredShare>,
    aborted: Option<AbortInfo>,
}

impl KeygenSession {
    /// Begin DKG: run `part1` and broadcast our round1 package to all peers.
    pub fn new(local: String, participants: Vec<String>, min_signers: u16) -> Result<Self> {
        let participants = normalize_participants(&participants);
        let max_signers = participants.len() as u16;
        let local_index = participants
            .iter()
            .position(|p| *p == local)
            .ok_or(FrostError::LocalPartyNotSelected)? as u16
            + 1;
        let id = frost::Identifier::try_from(local_index)
            .map_err(|e| FrostError::Frost(e.to_string()))?;
        let (round1_secret, round1_package) =
            frost::keys::dkg::part1(id, max_signers, min_signers, rand::rngs::OsRng)
                .map_err(|e| FrostError::Frost(e.to_string()))?;

        let round1 = ProtocolMessage {
            kind: "keygen_round1".into(),
            from: local.clone(),
            to: participants.iter().filter(|p| **p != local).cloned().collect(),
            payload: ProtocolMessage::encode_payload(
                &round1_package
                    .serialize()
                    .map_err(|e| FrostError::Frost(e.to_string()))?,
            ),
        };

        Ok(Self {
            local,
            participants,
            local_index,
            min_signers,
            round1_secret: Some(round1_secret),
            round1_packages: BTreeMap::new(),
            round2_secret: None,
            round2_packages: BTreeMap::new(),
            outputs: vec![round1],
            result: None,
            aborted: None,
        })
    }

    /// Abort info if this session was aborted with blame.
    pub fn aborted(&self) -> Option<&AbortInfo> {
        self.aborted.as_ref()
    }

    /// Record blame, queue a broadcast abort, and return an identifiable-abort
    /// error naming the culprits.
    fn abort(&mut self, phase: &str, culprits: Vec<String>) -> FrostError {
        let info = AbortInfo {
            phase: phase.to_string(),
            culprits: culprits.clone(),
        };
        self.aborted = Some(info);
        self.outputs.push(abort_message(
            "keygen_abort",
            &self.local,
            &self.participants,
            &culprits,
        ));
        FrostError::IdentifiableAbort {
            phase: phase.to_string(),
            culprits,
        }
    }

    pub fn drain_outputs(&mut self) -> Vec<ProtocolMessage> {
        std::mem::take(&mut self.outputs)
    }

    pub fn finished(&self) -> bool {
        self.result.is_some()
    }

    pub fn stored_share(&self) -> Option<&StoredShare> {
        self.result.as_ref()
    }

    /// Feed an inbound keygen message. Returns true when DKG completes.
    pub fn handle(&mut self, msg: &ProtocolMessage) -> Result<bool> {
        if self.finished() {
            return Ok(true);
        }
        let from_index = self
            .participants
            .iter()
            .position(|p| *p == msg.from)
            .ok_or(FrostError::LocalPartyNotSelected)? as u16
            + 1;
        let from_id = frost::Identifier::try_from(from_index)
            .map_err(|e| FrostError::Frost(e.to_string()))?;
        let payload = msg.decode_payload()?;
        match msg.kind.as_str() {
            "keygen_round1" => {
                if from_index != self.local_index {
                    // A package we can't even decode is attributable to its
                    // sender: blame them rather than crashing the session.
                    let pkg = match frost::keys::dkg::round1::Package::deserialize(&payload) {
                        Ok(p) => p,
                        Err(_) => return Err(self.abort("keygen_round1", vec![msg.from.clone()])),
                    };
                    self.round1_packages.entry(from_id).or_insert(pkg);
                }
                self.maybe_part2()?;
            }
            "keygen_round2" => {
                if from_index != self.local_index {
                    let pkg = match frost::keys::dkg::round2::Package::deserialize(&payload) {
                        Ok(p) => p,
                        Err(_) => return Err(self.abort("keygen_round2", vec![msg.from.clone()])),
                    };
                    self.round2_packages.entry(from_id).or_insert(pkg);
                }
                self.maybe_part3()?;
            }
            "keygen_abort" => {
                // A peer blamed some parties; record and stop.
                let culprits: Vec<String> =
                    serde_json::from_slice(&payload).unwrap_or_default();
                self.aborted = Some(AbortInfo {
                    phase: "keygen_abort".into(),
                    culprits: culprits.clone(),
                });
                return Err(FrostError::IdentifiableAbort {
                    phase: "keygen_abort".into(),
                    culprits,
                });
            }
            other => return Err(FrostError::UnknownKind(other.to_string())),
        }
        Ok(self.finished())
    }

    fn maybe_part2(&mut self) -> Result<()> {
        if self.round2_secret.is_some() {
            return Ok(());
        }
        if self.round1_packages.len() != self.participants.len().saturating_sub(1) {
            return Ok(());
        }
        let secret = self
            .round1_secret
            .take()
            .ok_or_else(|| FrostError::Frost("missing round1 secret".into()))?;
        let (round2_secret, round2_packages) =
            match frost::keys::dkg::part2(secret, &self.round1_packages) {
                Ok(v) => v,
                Err(e) => {
                    let culprits = blame_names(&self.participants, &e);
                    if culprits.is_empty() {
                        return Err(FrostError::Frost(e.to_string()));
                    }
                    return Err(self.abort("keygen_part2", culprits));
                }
            };
        self.round2_secret = Some(round2_secret);
        for (id, pkg) in round2_packages {
            let to = self.participant_name(id)?;
            self.outputs.push(ProtocolMessage {
                kind: "keygen_round2".into(),
                from: self.local.clone(),
                to: vec![to],
                payload: ProtocolMessage::encode_payload(
                    &pkg.serialize().map_err(|e| FrostError::Frost(e.to_string()))?,
                ),
            });
        }
        Ok(())
    }

    fn maybe_part3(&mut self) -> Result<()> {
        if self.finished() {
            return Ok(());
        }
        if self.round2_packages.len() != self.participants.len().saturating_sub(1) {
            return Ok(());
        }
        let round2_secret = self
            .round2_secret
            .as_ref()
            .ok_or_else(|| FrostError::Frost("missing round2 secret".into()))?;
        let (key_package, public_key_package) = match frost::keys::dkg::part3(
            round2_secret,
            &self.round1_packages,
            &self.round2_packages,
        ) {
            Ok(v) => v,
            Err(e) => {
                let culprits = blame_names(&self.participants, &e);
                if culprits.is_empty() {
                    return Err(FrostError::Frost(e.to_string()));
                }
                // part3 borrows round2_secret immutably; abort separately.
                let info = AbortInfo {
                    phase: "keygen_part3".into(),
                    culprits: culprits.clone(),
                };
                self.aborted = Some(info);
                self.outputs.push(abort_message(
                    "keygen_abort",
                    &self.local,
                    &self.participants,
                    &culprits,
                ));
                return Err(FrostError::IdentifiableAbort {
                    phase: "keygen_part3".into(),
                    culprits,
                });
            }
        };

        let public_key_compressed = hex::encode(
            public_key_package
                .verifying_key()
                .serialize()
                .map_err(|e| FrostError::Frost(e.to_string()))?,
        );
        self.result = Some(StoredShare {
            version: 1,
            engine: "frost".into(),
            participant: self.local.clone(),
            participants: self.participants.clone(),
            participant_index: self.local_index,
            min_signers: self.min_signers,
            max_signers: self.participants.len() as u16,
            public_key_compressed,
            key_package: B64.encode(
                key_package
                    .serialize()
                    .map_err(|e| FrostError::Frost(e.to_string()))?,
            ),
            public_key_package: B64.encode(
                public_key_package
                    .serialize()
                    .map_err(|e| FrostError::Frost(e.to_string()))?,
            ),
        });
        Ok(())
    }

    fn participant_name(&self, id: frost::Identifier) -> Result<String> {
        let bytes = id.serialize();
        let idx = u16::from_be_bytes([bytes[30], bytes[31]]);
        self.participants
            .get((idx - 1) as usize)
            .cloned()
            .ok_or_else(|| FrostError::Frost("identifier out of range".into()))
    }
}

/// A keysign session round state machine.
pub struct SignSession {
    local: String,
    participants: Vec<String>,
    key_package: frost::keys::KeyPackage,
    public_key_package: frost::keys::PublicKeyPackage,
    message: Vec<u8>,
    nonces: Option<frost::round1::SigningNonces>,
    commitments: BTreeMap<frost::Identifier, frost::round1::SigningCommitments>,
    signature_shares: BTreeMap<frost::Identifier, frost::round2::SignatureShare>,
    pending_shares: BTreeMap<frost::Identifier, frost::round2::SignatureShare>,
    outputs: Vec<ProtocolMessage>,
    result: Option<[u8; 64]>,
    aborted: Option<AbortInfo>,
    /// When true, sign/aggregate with the BIP341 key-path tweak.
    taproot: bool,
}

impl SignSession {
    /// Begin a keysign over the raw group key (no taproot tweak).
    pub fn new(
        share: &StoredShare,
        local: String,
        participants: Vec<String>,
        message: Vec<u8>,
    ) -> Result<Self> {
        Self::new_inner(share, local, participants, message, false)
    }

    /// Begin a BIP341 taproot key-path keysign: the aggregate signature is
    /// tweaked so it verifies under `tweak(group_key)` — i.e. the vault's
    /// taproot output key (`TaprootVault::derive`). Use this to sign BTC
    /// key-path sighashes.
    pub fn new_taproot(
        share: &StoredShare,
        local: String,
        participants: Vec<String>,
        message: Vec<u8>,
    ) -> Result<Self> {
        Self::new_inner(share, local, participants, message, true)
    }

    fn new_inner(
        share: &StoredShare,
        local: String,
        participants: Vec<String>,
        message: Vec<u8>,
        taproot: bool,
    ) -> Result<Self> {
        let participants = normalize_participants(&participants);
        if !participants.contains(&local) {
            return Err(FrostError::LocalPartyNotSelected);
        }
        let key_package = share.key_package()?;
        let public_key_package = share.public_key_package()?;

        let mut rng = rand::rngs::OsRng;
        let (nonces, commitments) = frost::round1::commit(key_package.signing_share(), &mut rng);

        let commit_bytes = commitments
            .serialize()
            .map_err(|e| FrostError::Frost(e.to_string()))?;
        let round1 = ProtocolMessage {
            kind: "sign_round1".into(),
            from: local.clone(),
            to: participants.iter().filter(|p| **p != local).cloned().collect(),
            payload: ProtocolMessage::encode_payload(&commit_bytes),
        };

        let mut me = Self {
            local,
            participants,
            key_package,
            public_key_package,
            message,
            nonces: Some(nonces),
            commitments: BTreeMap::new(),
            signature_shares: BTreeMap::new(),
            pending_shares: BTreeMap::new(),
            outputs: vec![round1],
            result: None,
            aborted: None,
            taproot,
        };
        let id = identifier_for(&me.participants, &me.local)?;
        me.commitments.insert(id, commitments);
        Ok(me)
    }

    /// Abort info if this keysign was aborted with blame.
    pub fn aborted(&self) -> Option<&AbortInfo> {
        self.aborted.as_ref()
    }

    pub fn drain_outputs(&mut self) -> Vec<ProtocolMessage> {
        std::mem::take(&mut self.outputs)
    }

    pub fn finished(&self) -> bool {
        self.result.is_some()
    }

    pub fn signature(&self) -> Option<[u8; 64]> {
        self.result
    }

    /// Feed an inbound round message. Returns true when the session completes.
    pub fn handle(&mut self, msg: &ProtocolMessage) -> Result<bool> {
        if self.finished() {
            return Ok(true);
        }
        let id = identifier_for(&self.participants, &msg.from)?;
        let bytes = msg.decode_payload()?;
        match msg.kind.as_str() {
            "sign_round1" => {
                let c = frost::round1::SigningCommitments::deserialize(&bytes)
                    .map_err(|e| FrostError::Frost(e.to_string()))?;
                self.commitments.insert(id, c);
                self.maybe_sign()?;
            }
            "sign_round2" => {
                let s = frost::round2::SignatureShare::deserialize(&bytes)
                    .map_err(|e| FrostError::Frost(e.to_string()))?;
                // Buffer round2 shares that arrive before all round1 commitments.
                if self.commitments.len() == self.participants.len() {
                    self.signature_shares.insert(id, s);
                } else {
                    self.pending_shares.insert(id, s);
                }
                self.maybe_aggregate()?;
            }
            "sign_abort" => {
                let culprits: Vec<String> = serde_json::from_slice(&bytes).unwrap_or_default();
                self.aborted = Some(AbortInfo {
                    phase: "sign_abort".into(),
                    culprits: culprits.clone(),
                });
                return Err(FrostError::IdentifiableAbort {
                    phase: "sign_abort".into(),
                    culprits,
                });
            }
            other => return Err(FrostError::UnknownKind(other.to_string())),
        }
        Ok(self.finished())
    }

    fn maybe_sign(&mut self) -> Result<()> {
        if self.nonces.is_none() {
            return Ok(());
        }
        if self.commitments.len() != self.participants.len() {
            return Ok(());
        }
        let signing_package = frost::SigningPackage::new(self.commitments.clone(), &self.message);
        let nonces = self.nonces.take().expect("checked above");
        let share = if self.taproot {
            frost::round2::sign_with_tweak(&signing_package, &nonces, &self.key_package, None)
        } else {
            frost::round2::sign(&signing_package, &nonces, &self.key_package)
        }
        .map_err(|e| FrostError::Frost(e.to_string()))?;

        let id = identifier_for(&self.participants, &self.local)?;
        self.signature_shares.insert(id, share);
        // absorb any round2 shares that arrived early
        let pending = std::mem::take(&mut self.pending_shares);
        self.signature_shares.extend(pending);

        let share_bytes = share.serialize();
        self.outputs.push(ProtocolMessage {
            kind: "sign_round2".into(),
            from: self.local.clone(),
            to: self
                .participants
                .iter()
                .filter(|p| **p != self.local)
                .cloned()
                .collect(),
            payload: ProtocolMessage::encode_payload(&share_bytes),
        });
        self.maybe_aggregate()?;
        Ok(())
    }

    fn maybe_aggregate(&mut self) -> Result<()> {
        if self.signature_shares.len() != self.participants.len() {
            return Ok(());
        }
        if self.commitments.len() != self.participants.len() {
            return Ok(());
        }
        let signing_package = frost::SigningPackage::new(self.commitments.clone(), &self.message);
        // aggregate validates each share and, on failure, names the parties
        // whose shares were invalid (Error::InvalidSignatureShare).
        let agg = if self.taproot {
            frost::aggregate_with_tweak(
                &signing_package,
                &self.signature_shares,
                &self.public_key_package,
                None,
            )
        } else {
            frost::aggregate(
                &signing_package,
                &self.signature_shares,
                &self.public_key_package,
            )
        };
        let signature = match agg {
            Ok(s) => s,
            Err(e) => {
                let culprits = blame_names(&self.participants, &e);
                if culprits.is_empty() {
                    return Err(FrostError::Frost(e.to_string()));
                }
                let info = AbortInfo {
                    phase: "keysign_aggregate".into(),
                    culprits: culprits.clone(),
                };
                self.aborted = Some(info);
                self.outputs.push(abort_message(
                    "sign_abort",
                    &self.local,
                    &self.participants,
                    &culprits,
                ));
                return Err(FrostError::IdentifiableAbort {
                    phase: "keysign_aggregate".into(),
                    culprits,
                });
            }
        };

        // For the raw-key case, verify against the group key. For taproot,
        // aggregate_with_tweak already validated the shares internally and the
        // tweaked verifying key isn't reachable (frost's Tweak trait is
        // private); the signature's validity under the taproot output key is
        // confirmed downstream by the BTC node accepting the witness.
        if !self.taproot {
            self.public_key_package
                .verifying_key()
                .verify(&self.message, &signature)
                .map_err(|e| FrostError::Frost(e.to_string()))?;
        }

        let sig_bytes = signature
            .serialize()
            .map_err(|e| FrostError::Frost(e.to_string()))?;
        if sig_bytes.len() != 64 {
            return Err(FrostError::Frost(format!(
                "unexpected signature length {}",
                sig_bytes.len()
            )));
        }
        let mut out = [0u8; 64];
        out.copy_from_slice(&sig_bytes);
        self.result = Some(out);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use frost_secp256k1_tr as frost;
    use std::collections::BTreeMap;

    fn party_names(n: usize) -> Vec<String> {
        (0..n).map(|i| format!("party{i}")).collect()
    }

    /// Pump ProtocolMessages between a set of session state machines until no
    /// more messages are in flight. Returns nothing; callers inspect sessions.
    fn pump<F: FnMut(&str, &ProtocolMessage) -> Vec<ProtocolMessage>>(
        mut inflight: Vec<ProtocolMessage>,
        mut deliver: F,
    ) {
        for _ in 0..200 {
            if inflight.is_empty() {
                break;
            }
            let mut next = Vec::new();
            for msg in inflight.drain(..) {
                for target in msg.to.clone() {
                    next.extend(deliver(&target, &msg));
                }
            }
            inflight = next;
        }
    }

    /// Run DISTRIBUTED DKG across `n` parties (no dealer) and return each
    /// party's StoredShare plus the resulting group verifying key.
    fn distributed_keygen(
        names: &[String],
        min: u16,
    ) -> (BTreeMap<String, StoredShare>, frost::VerifyingKey) {
        let names = normalize_participants(names);
        let mut sessions: BTreeMap<String, KeygenSession> = BTreeMap::new();
        let mut inflight = Vec::new();
        for name in &names {
            let mut s = KeygenSession::new(name.clone(), names.clone(), min).unwrap();
            inflight.extend(s.drain_outputs());
            sessions.insert(name.clone(), s);
        }
        pump(inflight, |target, msg| {
            let mut out = Vec::new();
            if let Some(s) = sessions.get_mut(target) {
                s.handle(msg).unwrap();
                out.extend(s.drain_outputs());
            }
            out
        });

        let mut shares = BTreeMap::new();
        let mut group_key: Option<frost::VerifyingKey> = None;
        for (name, s) in &sessions {
            let stored = s.stored_share().expect("DKG completed for every party");
            let pkp = stored.public_key_package().unwrap();
            group_key = Some(*pkp.verifying_key());
            shares.insert(name.clone(), stored.clone());
        }
        (shares, group_key.unwrap())
    }

    /// Every party derives the SAME group key from independent DKG runs.
    #[test]
    fn distributed_keygen_agrees_on_group_key() {
        let names = party_names(4);
        let (shares, group_key) = distributed_keygen(&names, 3);
        assert_eq!(shares.len(), 4);
        let expected = hex::encode(group_key.serialize().unwrap());
        for s in shares.values() {
            let pkp = s.public_key_package().unwrap();
            assert_eq!(hex::encode(pkp.verifying_key().serialize().unwrap()), expected);
        }
    }

    /// Full path with NO dealer anywhere: distributed DKG produces the shares,
    /// then a threshold subset runs distributed keysign, and the aggregate
    /// verifies against the DKG group key.
    #[test]
    fn distributed_keygen_then_keysign() {
        let names = party_names(4);
        let min = 3u16;
        let (shares, group_key) = distributed_keygen(&names, min);

        // Choose a threshold-sized signing subset.
        let chosen: Vec<String> = normalize_participants(&names)[..min as usize].to_vec();
        let message = b"thornado taproot sighash placeholder--32b".to_vec();

        let mut sessions: BTreeMap<String, SignSession> = BTreeMap::new();
        let mut inflight = Vec::new();
        for name in &chosen {
            let mut s =
                SignSession::new(&shares[name], name.clone(), chosen.clone(), message.clone())
                    .unwrap();
            inflight.extend(s.drain_outputs());
            sessions.insert(name.clone(), s);
        }

        let mut signature: Option<[u8; 64]> = None;
        pump(inflight, |target, msg| {
            let mut out = Vec::new();
            if let Some(s) = sessions.get_mut(target) {
                if s.handle(msg).unwrap() {
                    signature = s.signature();
                }
                out.extend(s.drain_outputs());
            }
            out
        });

        let sig = signature.expect("aggregated signature produced");
        let sig = frost::Signature::deserialize(&sig).unwrap();
        group_key
            .verify(&message, &sig)
            .expect("aggregate verifies against distributed-DKG group key");
    }

    #[test]
    fn session_ids_match_spec_shape() {
        let id = keysign_session_id(b"vault", b"msg");
        assert_eq!(id.len(), 64); // hex sha256
        let kg = keygen_session_id(100, 2, &party_names(3));
        assert_eq!(kg.len(), 64);
    }

    #[test]
    fn keygen_blames_sender_of_undecodable_round1() {
        let names = party_names(3);
        let mut victim = KeygenSession::new(names[0].clone(), names.clone(), 2).unwrap();
        // A round1 message from party1 with a garbage (undecodable) payload.
        let bad = ProtocolMessage {
            kind: "keygen_round1".into(),
            from: names[1].clone(),
            to: vec![names[0].clone()],
            payload: ProtocolMessage::encode_payload(b"not a valid dkg package"),
        };
        let err = victim.handle(&bad).unwrap_err();
        match err {
            FrostError::IdentifiableAbort { phase, culprits } => {
                assert_eq!(phase, "keygen_round1");
                assert_eq!(culprits, vec![names[1].clone()]);
            }
            other => panic!("expected IdentifiableAbort, got {other:?}"),
        }
        // The victim recorded the abort and queued a broadcast keygen_abort.
        assert_eq!(victim.aborted().unwrap().culprits, vec![names[1].clone()]);
        let outs = victim.drain_outputs();
        assert!(outs.iter().any(|m| m.kind == "keygen_abort"));
    }

    #[test]
    fn keygen_receiving_abort_stops_with_blame() {
        let names = party_names(3);
        let mut s = KeygenSession::new(names[0].clone(), names.clone(), 2).unwrap();
        let culprits = vec![names[2].clone()];
        let abort = ProtocolMessage {
            kind: "keygen_abort".into(),
            from: names[1].clone(),
            to: vec![names[0].clone()],
            payload: ProtocolMessage::encode_payload(&serde_json::to_vec(&culprits).unwrap()),
        };
        let err = s.handle(&abort).unwrap_err();
        assert!(matches!(err, FrostError::IdentifiableAbort { .. }));
        assert_eq!(s.aborted().unwrap().culprits, culprits);
    }

    #[test]
    fn keysign_receiving_abort_stops_with_blame() {
        // Build a real share via DKG, then feed a sign_abort into a keysign.
        let names = party_names(3);
        let (shares, _gk) = distributed_keygen(&names, 2);
        let chosen: Vec<String> = names[..2].to_vec();
        let mut s = SignSession::new(
            &shares[&chosen[0]],
            chosen[0].clone(),
            chosen.clone(),
            b"thornado taproot sighash placeholder--32b".to_vec(),
        )
        .unwrap();
        let culprits = vec![chosen[1].clone()];
        let abort = ProtocolMessage {
            kind: "sign_abort".into(),
            from: chosen[1].clone(),
            to: vec![chosen[0].clone()],
            payload: ProtocolMessage::encode_payload(&serde_json::to_vec(&culprits).unwrap()),
        };
        let err = s.handle(&abort).unwrap_err();
        assert!(matches!(err, FrostError::IdentifiableAbort { .. }));
        assert_eq!(s.aborted().unwrap().culprits, culprits);
    }
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
