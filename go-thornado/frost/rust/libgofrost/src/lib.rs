use std::collections::BTreeMap;
use std::ffi::c_void;
use std::ptr;
use std::sync::Mutex;

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use frost_secp256k1_tr as frost;
use frost_secp256k1_tr::keys::dkg;
use frost_secp256k1_tr::keys::{IdentifierList, KeyPackage, PublicKeyPackage};
use frost_secp256k1_tr::{Identifier, Signature, SigningPackage};
use once_cell::sync::Lazy;
use rand::rngs::OsRng;
use serde::{Deserialize, Serialize};

#[repr(C)]
pub struct GoFrostBuf {
    pub ptr: *mut u8,
    pub len: usize,
}

#[derive(Debug, Serialize)]
struct FfiError {
    code: String,
    phase: String,
    party: u16,
    message: String,
    evidence: String,
}

#[derive(Debug, thiserror::Error)]
enum WrapperError {
    #[error("ffi error")]
    Ffi(FfiError),
    #[error("{0}")]
    Message(String),
}

#[derive(Debug, Deserialize)]
struct KeygenInput {
    participants: Vec<String>,
    min_signers: Option<u16>,
}

#[derive(Debug, Serialize)]
struct KeygenOutput {
    shares: BTreeMap<String, String>,
    pub_key_compressed: String,
}

#[derive(Debug, Deserialize)]
struct SessionInput {
    participants: Vec<String>,
    local: String,
    min_signers: Option<u16>,
    share: Option<String>,
    message: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Clone)]
struct StoredShare {
    version: u16,
    engine: String,
    participant: String,
    participants: Vec<String>,
    participant_index: u16,
    min_signers: u16,
    max_signers: u16,
    public_key_compressed: String,
    key_package: String,
    public_key_package: String,
    #[serde(default)]
    all_key_packages: BTreeMap<String, String>,
}

#[derive(Debug, Deserialize)]
struct SignInput {
    share: String,
    message: String,
}

#[derive(Debug, Serialize)]
struct SignOutput {
    signature: String,
}

#[derive(Debug, Deserialize)]
struct VerifyInput {
    public_key_package: String,
    message: String,
    signature: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
struct ProtocolMessage {
    kind: String,
    from: String,
    to: Vec<String>,
    payload: String,
}

struct KeygenSession {
    local: String,
    participants: Vec<String>,
    local_index: u16,
    min_signers: u16,
    round1_secret: Option<dkg::round1::SecretPackage>,
    round1_packages: BTreeMap<Identifier, dkg::round1::Package>,
    round2_secret: Option<dkg::round2::SecretPackage>,
    round2_packages: BTreeMap<Identifier, dkg::round2::Package>,
    outputs: Vec<Vec<u8>>,
    result: Option<Vec<u8>>,
}

struct SignSession {
    local: String,
    participants: Vec<String>,
    key_participants: Vec<String>,
    message: Vec<u8>,
    key_package: KeyPackage,
    public_key_package: PublicKeyPackage,
    nonces: Option<frost::round1::SigningNonces>,
    commitments: BTreeMap<Identifier, frost::round1::SigningCommitments>,
    signature_shares: BTreeMap<Identifier, frost::round2::SignatureShare>,
    outputs: Vec<Vec<u8>>,
    result: Option<Vec<u8>>,
}

enum SessionKind {
    Keygen(KeygenSession),
    Sign(SignSession),
}

static SESSIONS: Lazy<Mutex<Vec<Option<SessionKind>>>> = Lazy::new(|| Mutex::new(Vec::new()));

fn read_json<'a, T: Deserialize<'a>>(ptr: *const u8, len: usize) -> Result<T, WrapperError> {
    if ptr.is_null() {
        return Err(WrapperError::Message("null input pointer".to_string()));
    }
    let bytes = unsafe { std::slice::from_raw_parts(ptr, len) };
    serde_json::from_slice(bytes).map_err(|e| WrapperError::Message(e.to_string()))
}

fn write_json<T: Serialize>(value: &T, out: *mut GoFrostBuf) -> i32 {
    if out.is_null() {
        return -1;
    }
    match serde_json::to_vec(value) {
        Ok(bytes) => write_bytes(bytes, out),
        Err(_) => -1,
    }
}

fn write_bytes(mut bytes: Vec<u8>, out: *mut GoFrostBuf) -> i32 {
    if out.is_null() {
        return -1;
    }
    let buf = GoFrostBuf {
        ptr: bytes.as_mut_ptr(),
        len: bytes.len(),
    };
    std::mem::forget(bytes);
    unsafe { ptr::write(out, buf) };
    0
}

fn write_error(err: WrapperError, out: *mut GoFrostBuf) -> i32 {
    let ffi = match err {
        WrapperError::Ffi(e) => e,
        WrapperError::Message(message) => FfiError {
            code: "ERROR".to_string(),
            phase: "wrapper".to_string(),
            party: 0,
            message,
            evidence: String::new(),
        },
    };
    let _ = write_json(&ffi, out);
    -1
}

fn normalize_participants(mut participants: Vec<String>) -> Vec<String> {
    participants.iter_mut().for_each(|p| *p = p.trim().to_string());
    participants.retain(|p| !p.is_empty());
    participants.sort();
    participants.dedup();
    participants
}

fn identifier(index: u16) -> Result<Identifier, WrapperError> {
    Identifier::try_from(index).map_err(|e| WrapperError::Message(e.to_string()))
}

fn participant_index(participants: &[String], local: &str) -> Result<u16, WrapperError> {
    participants
        .iter()
        .position(|p| p == local)
        .map(|i| (i + 1) as u16)
        .ok_or_else(|| WrapperError::Message(format!("local participant {local} missing")))
}

fn participant_by_id(participants: &[String], id: Identifier) -> Result<String, WrapperError> {
    let index = identifier_to_u16(&id)
        .ok_or_else(|| WrapperError::Message("invalid participant identifier".to_string()))?;
    participants
        .get(index.saturating_sub(1) as usize)
        .cloned()
        .ok_or_else(|| WrapperError::Message(format!("participant index {index} missing")))
}

fn encode_msg(msg: ProtocolMessage) -> Result<Vec<u8>, WrapperError> {
    serde_json::to_vec(&msg).map_err(|e| WrapperError::Message(e.to_string()))
}

fn decode_msg(bytes: &[u8]) -> Result<ProtocolMessage, WrapperError> {
    serde_json::from_slice(bytes).map_err(|e| WrapperError::Message(e.to_string()))
}

fn queue_msg(
    outputs: &mut Vec<Vec<u8>>,
    kind: &str,
    from: &str,
    to: Vec<String>,
    payload: Vec<u8>,
) -> Result<(), WrapperError> {
    outputs.push(encode_msg(ProtocolMessage {
        kind: kind.to_string(),
        from: from.to_string(),
        to,
        payload: B64.encode(payload),
    })?);
    Ok(())
}

fn keygen(input: KeygenInput) -> Result<KeygenOutput, WrapperError> {
    let participants = normalize_participants(input.participants);
    let max_signers = participants.len() as u16;
    let min_signers = input.min_signers.unwrap_or((max_signers * 2 / 3) + 1);
    let identifiers = (1..=max_signers)
        .map(identifier)
        .collect::<Result<Vec<_>, _>>()?;
    let (secret_shares, public_key_package) = frost::keys::generate_with_dealer(
        max_signers,
        min_signers,
        IdentifierList::Custom(&identifiers),
        OsRng,
    )
    .map_err(frost_error("keygen"))?;

    let public_key_package_bytes = public_key_package
        .serialize()
        .map_err(frost_error("keygen"))?;
    let public_key_package_b64 = B64.encode(public_key_package_bytes);
    let pub_key_compressed = public_key_package
        .verifying_key()
        .serialize()
        .map_err(frost_error("keygen"))
        .map(hex::encode)?;

    let mut all_key_packages = BTreeMap::new();
    let mut keyed_packages = Vec::new();
    for (i, participant) in participants.iter().enumerate() {
        let id = identifier((i + 1) as u16)?;
        let secret_share = secret_shares
            .get(&id)
            .ok_or_else(|| WrapperError::Message("missing secret share".to_string()))?;
        let key_package = KeyPackage::try_from(secret_share.clone()).map_err(frost_error("keygen"))?;
        let encoded = B64.encode(key_package.serialize().map_err(frost_error("keygen"))?);
        all_key_packages.insert(participant.clone(), encoded.clone());
        keyed_packages.push(((i + 1) as u16, participant.clone(), encoded));
    }

    let mut shares = BTreeMap::new();
    for (participant_index, participant, key_package) in keyed_packages {
        let stored = StoredShare {
            version: 1,
            engine: "frost".to_string(),
            participant: participant.clone(),
            participants: participants.clone(),
            participant_index,
            min_signers,
            max_signers,
            public_key_compressed: pub_key_compressed.clone(),
            key_package,
            public_key_package: public_key_package_b64.clone(),
            all_key_packages: all_key_packages.clone(),
        };
        let bytes = serde_json::to_vec(&stored).map_err(|e| WrapperError::Message(e.to_string()))?;
        shares.insert(participant, B64.encode(bytes));
    }

    Ok(KeygenOutput {
        shares,
        pub_key_compressed,
    })
}

fn keygen_session_new(input: SessionInput) -> Result<SessionKind, WrapperError> {
    let participants = normalize_participants(input.participants);
    let max_signers = participants.len() as u16;
    let min_signers = input.min_signers.unwrap_or((max_signers * 2 / 3) + 1);
    let local_index = participant_index(&participants, &input.local)?;
    let (round1_secret, round1_package) = dkg::part1(identifier(local_index)?, max_signers, min_signers, OsRng)
        .map_err(frost_error("keygen"))?;

    let mut outputs = Vec::new();
    queue_msg(
        &mut outputs,
        "keygen_round1",
        &input.local,
        participants.clone(),
        round1_package.serialize().map_err(frost_error("keygen"))?,
    )?;

    Ok(SessionKind::Keygen(KeygenSession {
        local: input.local,
        participants,
        local_index,
        min_signers,
        round1_secret: Some(round1_secret),
        round1_packages: BTreeMap::new(),
        round2_secret: None,
        round2_packages: BTreeMap::new(),
        outputs,
        result: None,
    }))
}

fn sign_session_new(input: SessionInput) -> Result<SessionKind, WrapperError> {
    let participants = normalize_participants(input.participants);
    let _ = participant_index(&participants, &input.local)?;
    let share = input.share.ok_or_else(|| WrapperError::Message("missing keyshare".to_string()))?;
    let share_bytes = B64.decode(share).map_err(|e| WrapperError::Message(e.to_string()))?;
    let stored: StoredShare =
        serde_json::from_slice(&share_bytes).map_err(|e| WrapperError::Message(e.to_string()))?;
    let key_package = KeyPackage::deserialize(
        &B64.decode(&stored.key_package)
            .map_err(|e| WrapperError::Message(e.to_string()))?,
    )
    .map_err(frost_error("keysign"))?;
    let public_key_package = PublicKeyPackage::deserialize(
        &B64.decode(&stored.public_key_package)
            .map_err(|e| WrapperError::Message(e.to_string()))?,
    )
    .map_err(frost_error("keysign"))?;
    let message = B64.decode(
        input
            .message
            .ok_or_else(|| WrapperError::Message("missing message".to_string()))?,
    )
    .map_err(|e| WrapperError::Message(e.to_string()))?;
    if message.len() != 32 {
        return Err(WrapperError::Message(format!(
            "FROST messages must be 32 bytes, got {}",
            message.len()
        )));
    }
    let (nonces, commitments) = frost::round1::commit(key_package.signing_share(), &mut OsRng);
    let mut outputs = Vec::new();
    queue_msg(
        &mut outputs,
        "sign_round1",
        &input.local,
        participants.clone(),
        commitments.serialize().map_err(frost_error("keysign"))?,
    )?;

    Ok(SessionKind::Sign(SignSession {
        local: input.local,
        participants,
        key_participants: stored.participants.clone(),
        message,
        key_package,
        public_key_package,
        nonces: Some(nonces),
        commitments: BTreeMap::new(),
        signature_shares: BTreeMap::new(),
        outputs,
        result: None,
    }))
}

impl KeygenSession {
    fn input(&mut self, bytes: &[u8]) -> Result<bool, WrapperError> {
        if self.result.is_some() {
            return Ok(true);
        }
        let msg = decode_msg(bytes)?;
        let from_index = participant_index(&self.participants, &msg.from)?;
        let from_id = identifier(from_index)?;
        let payload = B64.decode(&msg.payload).map_err(|e| WrapperError::Message(e.to_string()))?;
        match msg.kind.as_str() {
            "keygen_round1" => {
                if from_index != self.local_index {
                    let pkg = dkg::round1::Package::deserialize(&payload).map_err(frost_error("keygen"))?;
                    self.round1_packages.entry(from_id).or_insert(pkg);
                }
                if self.round1_packages.len() == self.participants.len().saturating_sub(1)
                    && self.round2_secret.is_none()
                {
                    let secret = self
                        .round1_secret
                        .take()
                        .ok_or_else(|| WrapperError::Message("missing round1 secret".to_string()))?;
                    let (round2_secret, round2_packages) =
                        dkg::part2(secret, &self.round1_packages).map_err(frost_error("keygen"))?;
                    self.round2_secret = Some(round2_secret);
                    for (id, pkg) in round2_packages {
                        let to = participant_by_id(&self.participants, id)?;
                        queue_msg(
                            &mut self.outputs,
                            "keygen_round2",
                            &self.local,
                            vec![to],
                            pkg.serialize().map_err(frost_error("keygen"))?,
                        )?;
                    }
                }
            }
            "keygen_round2" => {
                if from_index != self.local_index {
                    let pkg = dkg::round2::Package::deserialize(&payload).map_err(frost_error("keygen"))?;
                    self.round2_packages.entry(from_id).or_insert(pkg);
                }
                if self.round2_packages.len() == self.participants.len().saturating_sub(1)
                    && self.result.is_none()
                {
                    let round2_secret = self
                        .round2_secret
                        .as_ref()
                        .ok_or_else(|| WrapperError::Message("missing round2 secret".to_string()))?;
                    let (key_package, public_key_package) =
                        dkg::part3(round2_secret, &self.round1_packages, &self.round2_packages)
                            .map_err(frost_error("keygen"))?;
                    let public_key_package_b64 = B64.encode(
                        public_key_package
                            .serialize()
                            .map_err(frost_error("keygen"))?,
                    );
                    let public_key_compressed = hex::encode(
                        public_key_package
                            .verifying_key()
                            .serialize()
                            .map_err(frost_error("keygen"))?,
                    );
                    let stored = StoredShare {
                        version: 1,
                        engine: "frost".to_string(),
                        participant: self.local.clone(),
                        participants: self.participants.clone(),
                        participant_index: self.local_index,
                        min_signers: self.min_signers,
                        max_signers: self.participants.len() as u16,
                        public_key_compressed,
                        key_package: B64.encode(key_package.serialize().map_err(frost_error("keygen"))?),
                        public_key_package: public_key_package_b64,
                        all_key_packages: BTreeMap::new(),
                    };
                    self.result =
                        Some(serde_json::to_vec(&stored).map_err(|e| WrapperError::Message(e.to_string()))?);
                }
            }
            _ => return Err(WrapperError::Message(format!("unexpected keygen message {}", msg.kind))),
        }
        Ok(self.result.is_some())
    }
}

impl SignSession {
    fn input(&mut self, bytes: &[u8]) -> Result<bool, WrapperError> {
        if self.result.is_some() {
            return Ok(true);
        }
        let msg = decode_msg(bytes)?;
        let from_index = participant_index(&self.key_participants, &msg.from)?;
        let from_id = identifier(from_index)?;
        let payload = B64.decode(&msg.payload).map_err(|e| WrapperError::Message(e.to_string()))?;
        match msg.kind.as_str() {
            "sign_round1" => {
                let commitments =
                    frost::round1::SigningCommitments::deserialize(&payload).map_err(frost_error("keysign"))?;
                self.commitments.entry(from_id).or_insert(commitments);
                if self.commitments.len() == self.participants.len() && self.signature_shares.is_empty() {
                    let signing_package = SigningPackage::new(self.commitments.clone(), &self.message);
                    let nonces = self
                        .nonces
                        .take()
                        .ok_or_else(|| WrapperError::Message("missing signing nonces".to_string()))?;
                    let share = frost::round2::sign(&signing_package, &nonces, &self.key_package)
                        .map_err(frost_error("keysign"))?;
                    queue_msg(
                        &mut self.outputs,
                        "sign_round2",
                        &self.local,
                        self.participants.clone(),
                        share.serialize(),
                    )?;
                }
            }
            "sign_round2" => {
                let share = frost::round2::SignatureShare::deserialize(&payload)
                    .map_err(frost_error("keysign"))?;
                self.signature_shares.entry(from_id).or_insert(share);
                if self.signature_shares.len() == self.participants.len() && self.result.is_none() {
                    let signing_package = SigningPackage::new(self.commitments.clone(), &self.message);
                    let signature =
                        frost::aggregate(&signing_package, &self.signature_shares, &self.public_key_package)
                            .map_err(frost_error("keysign"))?;
                    self.public_key_package
                        .verifying_key()
                        .verify(&self.message, &signature)
                        .map_err(frost_error("verify"))?;
                    self.result = Some(signature.serialize().map_err(frost_error("keysign"))?);
                }
            }
            _ => return Err(WrapperError::Message(format!("unexpected sign message {}", msg.kind))),
        }
        Ok(self.result.is_some())
    }
}

fn sign(input: SignInput) -> Result<SignOutput, WrapperError> {
    if B64.decode(&input.message).map_err(|e| WrapperError::Message(e.to_string()))?.len() != 32 {
        return Err(WrapperError::Message("FROST messages must be 32 bytes".to_string()));
    }
    let message = B64.decode(&input.message).map_err(|e| WrapperError::Message(e.to_string()))?;
    let share_bytes = B64.decode(&input.share).map_err(|e| WrapperError::Message(e.to_string()))?;
    let share: StoredShare =
        serde_json::from_slice(&share_bytes).map_err(|e| WrapperError::Message(e.to_string()))?;
    if share.all_key_packages.is_empty() {
        return Err(WrapperError::Message(
            "one-shot signing is unavailable for distributed FROST keyshares".to_string(),
        ));
    }
    let public_key_package = PublicKeyPackage::deserialize(
        &B64.decode(&share.public_key_package)
            .map_err(|e| WrapperError::Message(e.to_string()))?,
    )
    .map_err(frost_error("keysign"))?;

    let mut commitments_map = BTreeMap::new();
    let mut nonces_map = BTreeMap::new();
    let mut key_packages = BTreeMap::new();

    for (i, participant) in share
        .participants
        .iter()
        .take(share.min_signers as usize)
        .enumerate()
    {
        let id = identifier((i + 1) as u16)?;
        let key_package_b64 = share
            .all_key_packages
            .get(participant)
            .ok_or_else(|| WrapperError::Message("missing signing key package".to_string()))?;
        let key_package = KeyPackage::deserialize(
            &B64.decode(key_package_b64)
                .map_err(|e| WrapperError::Message(e.to_string()))?,
        )
        .map_err(frost_error("keysign"))?;
        let (nonces, commitments) = frost::round1::commit(key_package.signing_share(), &mut OsRng);
        commitments_map.insert(id, commitments);
        nonces_map.insert(id, nonces);
        key_packages.insert(id, key_package);
    }

    let signing_package = SigningPackage::new(commitments_map, &message);
    let mut signature_shares = BTreeMap::new();
    for (id, key_package) in key_packages.iter() {
        let nonces = nonces_map
            .get(id)
            .ok_or_else(|| WrapperError::Message("missing nonces".to_string()))?;
        let sig_share = frost::round2::sign(&signing_package, nonces, key_package)
            .map_err(frost_error("keysign"))?;
        signature_shares.insert(*id, sig_share);
    }

    let signature = frost::aggregate(&signing_package, &signature_shares, &public_key_package)
        .map_err(frost_error("keysign"))?;
    public_key_package
        .verifying_key()
        .verify(&message, &signature)
        .map_err(frost_error("verify"))?;

    Ok(SignOutput {
        signature: B64.encode(signature.serialize().map_err(frost_error("keysign"))?),
    })
}

fn verify(input: VerifyInput) -> Result<(), WrapperError> {
    let public_key_package = PublicKeyPackage::deserialize(
        &B64.decode(&input.public_key_package)
            .map_err(|e| WrapperError::Message(e.to_string()))?,
    )
    .map_err(frost_error("verify"))?;
    let message = B64.decode(&input.message).map_err(|e| WrapperError::Message(e.to_string()))?;
    let signature = Signature::deserialize(
        &B64.decode(&input.signature)
            .map_err(|e| WrapperError::Message(e.to_string()))?,
    )
    .map_err(frost_error("verify"))?;
    public_key_package
        .verifying_key()
        .verify(&message, &signature)
        .map_err(frost_error("verify"))
}

fn frost_error(phase: &'static str) -> impl Fn(frost::Error) -> WrapperError {
    move |err| {
        let party = err
            .culprits()
            .first()
            .and_then(identifier_to_u16)
            .unwrap_or(0);
        WrapperError::Ffi(FfiError {
            code: if party == 0 {
                "ERROR".to_string()
            } else {
                "IDENTIFIABLE_ABORT".to_string()
            },
            phase: phase.to_string(),
            party,
            message: err.to_string(),
            evidence: String::new(),
        })
    }
}

fn identifier_to_u16(id: &Identifier) -> Option<u16> {
    let bytes = id.serialize();
    if bytes.len() < 32 {
        return None;
    }
    Some(u16::from_be_bytes([bytes[30], bytes[31]]))
}

fn insert_session(session: SessionKind) -> i32 {
    let mut sessions = SESSIONS.lock().expect("session lock poisoned");
    for (idx, slot) in sessions.iter_mut().enumerate() {
        if slot.is_none() {
            *slot = Some(session);
            return idx as i32;
        }
    }
    sessions.push(Some(session));
    (sessions.len() - 1) as i32
}

fn with_session<T>(
    handle: i32,
    f: impl FnOnce(&mut SessionKind) -> Result<T, WrapperError>,
) -> Result<T, WrapperError> {
    let mut sessions = SESSIONS.lock().expect("session lock poisoned");
    let session = sessions
        .get_mut(handle as usize)
        .and_then(Option::as_mut)
        .ok_or_else(|| WrapperError::Message(format!("invalid frost session handle {handle}")))?;
    f(session)
}

#[no_mangle]
pub extern "C" fn gofrost_keygen(ptr: *const u8, len: usize, out: *mut GoFrostBuf) -> i32 {
    match read_json::<KeygenInput>(ptr, len).and_then(keygen) {
        Ok(value) => write_json(&value, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_sign(ptr: *const u8, len: usize, out: *mut GoFrostBuf) -> i32 {
    match read_json::<SignInput>(ptr, len).and_then(sign) {
        Ok(value) => write_json(&value, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_verify(ptr: *const u8, len: usize, out: *mut GoFrostBuf) -> i32 {
    match read_json::<VerifyInput>(ptr, len).and_then(|input| verify(input).map(|_| ())) {
        Ok(()) => write_json(&serde_json::json!({"ok": true}), out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_keygen_session_new(
    ptr: *const u8,
    len: usize,
    handle_out: *mut i32,
    err_out: *mut GoFrostBuf,
) -> i32 {
    if handle_out.is_null() {
        return write_error(WrapperError::Message("null handle output".to_string()), err_out);
    }
    match read_json::<SessionInput>(ptr, len).and_then(keygen_session_new) {
        Ok(session) => {
            unsafe { ptr::write(handle_out, insert_session(session)) };
            0
        }
        Err(err) => write_error(err, err_out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_sign_session_new(
    ptr: *const u8,
    len: usize,
    handle_out: *mut i32,
    err_out: *mut GoFrostBuf,
) -> i32 {
    if handle_out.is_null() {
        return write_error(WrapperError::Message("null handle output".to_string()), err_out);
    }
    match read_json::<SessionInput>(ptr, len).and_then(sign_session_new) {
        Ok(session) => {
            unsafe { ptr::write(handle_out, insert_session(session)) };
            0
        }
        Err(err) => write_error(err, err_out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_output_message(
    handle: i32,
    out: *mut GoFrostBuf,
) -> i32 {
    match with_session(handle, |session| {
        let output = match session {
            SessionKind::Keygen(s) => {
                if s.outputs.is_empty() { None } else { Some(s.outputs.remove(0)) }
            }
            SessionKind::Sign(s) => {
                if s.outputs.is_empty() { None } else { Some(s.outputs.remove(0)) }
            }
        };
        Ok(output.unwrap_or_default())
    }) {
        Ok(bytes) => write_bytes(bytes, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_message_receiver(
    _handle: i32,
    ptr: *const u8,
    len: usize,
    index: u32,
    out: *mut GoFrostBuf,
) -> i32 {
    let result = (|| {
        if ptr.is_null() {
            return Err(WrapperError::Message("null message pointer".to_string()));
        }
        let bytes = unsafe { std::slice::from_raw_parts(ptr, len) };
        let msg = decode_msg(bytes)?;
        Ok(msg.to.get(index as usize).cloned().unwrap_or_default().into_bytes())
    })();
    match result {
        Ok(bytes) => write_bytes(bytes, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_input_message(
    handle: i32,
    ptr: *const u8,
    len: usize,
    finished_out: *mut u32,
    err_out: *mut GoFrostBuf,
) -> i32 {
    if finished_out.is_null() {
        return write_error(WrapperError::Message("null finished output".to_string()), err_out);
    }
    let result = with_session(handle, |session| {
        if ptr.is_null() {
            return Err(WrapperError::Message("null message pointer".to_string()));
        }
        let bytes = unsafe { std::slice::from_raw_parts(ptr, len) };
        match session {
            SessionKind::Keygen(s) => s.input(bytes),
            SessionKind::Sign(s) => s.input(bytes),
        }
    });
    match result {
        Ok(finished) => {
            unsafe { ptr::write(finished_out, if finished { 1 } else { 0 }) };
            0
        }
        Err(err) => write_error(err, err_out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_finish(handle: i32, out: *mut GoFrostBuf) -> i32 {
    match with_session(handle, |session| match session {
        SessionKind::Keygen(s) => s
            .result
            .clone()
            .ok_or_else(|| WrapperError::Message("FROST keygen session is not finished".to_string())),
        SessionKind::Sign(s) => s
            .result
            .clone()
            .ok_or_else(|| WrapperError::Message("FROST sign session is not finished".to_string())),
    }) {
        Ok(bytes) => write_bytes(bytes, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_abort_message(handle: i32, out: *mut GoFrostBuf) -> i32 {
    let result = with_session(handle, |session| {
        let (phase, local, participants) = match session {
            SessionKind::Keygen(s) => ("keygen_abort", s.local.clone(), s.participants.clone()),
            SessionKind::Sign(s) => ("sign_abort", s.local.clone(), s.participants.clone()),
        };
        let local_index = participant_index(&participants, &local)?;
        encode_msg(ProtocolMessage {
            kind: phase.to_string(),
            from: local,
            to: participants,
            payload: B64.encode(format!("protocol abort {local_index}").as_bytes()),
        })
    });
    match result {
        Ok(bytes) => write_bytes(bytes, out),
        Err(err) => write_error(err, out),
    }
}

#[no_mangle]
pub extern "C" fn gofrost_session_free(handle: i32) -> i32 {
    let mut sessions = SESSIONS.lock().expect("session lock poisoned");
    if let Some(slot) = sessions.get_mut(handle as usize) {
        *slot = None;
    }
    0
}

#[no_mangle]
pub extern "C" fn gofrost_free(ptr: *mut c_void, len: usize) {
    if ptr.is_null() || len == 0 {
        return;
    }
    unsafe {
        drop(Vec::from_raw_parts(ptr as *mut u8, len, len));
    }
}
