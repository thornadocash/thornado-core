use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::Path;

pub type Result<T> = std::result::Result<T, Error>;

pub const DOMAIN: &str = "thornado-mvp-v1";
pub const SNAPSHOT_VERSION: u32 = 1;
pub const DEFAULT_FEE_BUCKET_TARGET_SATS: u64 = 30 * 1_000_000;
pub const DEFAULT_DENOMINATIONS_SATS: [u64; 4] =
    [1_000_000_000, 100_000_000, 10_000_000, 1_000_000];

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum Error {
    #[error("invalid proof")]
    InvalidProof,
    #[error("deposit intent not found")]
    DepositNotFound,
    #[error("deposit is not confirmed")]
    DepositNotConfirmed,
    #[error("deposit was already split")]
    DepositAlreadySplit,
    #[error("deposit amount does not produce any supported denomination notes")]
    DepositTooSmall,
    #[error("note commitments do not match the greedy denomination split")]
    InvalidNoteCommitments,
    #[error("unsupported snapshot version {0}")]
    UnsupportedSnapshotVersion(u32),
    #[error("unknown denomination")]
    UnknownDenomination,
    #[error("unknown merkle root")]
    UnknownMerkleRoot,
    #[error("unknown note commitment")]
    UnknownCommitment,
    #[error("nullifier has already been spent")]
    DuplicateNullifier,
    #[error("io error: {0}")]
    Io(String),
    #[error("json error: {0}")]
    Json(String),
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum Command {
    RequestDepositAddress {
        pow_token: String,
    },
    ObserveDeposit {
        intent_id: String,
        txid: String,
        amount_sats: u64,
    },
    ConfirmDeposit {
        intent_id: String,
    },
    SplitDepositIntoNotes {
        deposit_id: String,
        note_commitments: Vec<NoteCommitment>,
    },
    WithdrawNote {
        proof: WithdrawalProof,
        public: WithdrawalPublicInputs,
    },
    StartChurnEpoch,
    MarkNodeOffline {
        node_id: String,
    },
    ApplyChurnPenalties,
    SaveSnapshot {
        path: String,
    },
    LoadSnapshot {
        path: String,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum Event {
    DepositIntentCreated {
        intent_id: String,
        deposit_address: String,
        pow_token: String,
    },
    DepositObserved {
        intent_id: String,
        txid: String,
        amount_sats: u64,
    },
    DepositConfirmed {
        intent_id: String,
    },
    NotesMinted {
        deposit_id: String,
        notes: Vec<MintedNote>,
        remainder_sats: u64,
    },
    NoteSpent {
        nullifier_hash: String,
        denomination_sats: u64,
    },
    WithdrawalAuthorized {
        withdrawal_id: String,
        recipient: String,
        amount_sats: u64,
        fee_sats: u64,
        nullifier_hash: String,
        signature: MockSignature,
    },
    FeeCharged {
        amount_sats: u64,
    },
    FeeBucketUpdated {
        current_bucket_sats: u64,
        sealed_buckets: u64,
    },
    ChurnEpochStarted {
        epoch: u64,
    },
    NodeMarkedOffline {
        node_id: String,
        epoch: u64,
    },
    MockPenaltyApplied {
        node_id: String,
        epoch: u64,
    },
    SnapshotWritten {
        path: String,
        state_hash: String,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AppState {
    pub deposits: DepositState,
    pub notes: NoteState,
    pub fees: FeeState,
    pub churn: ChurnState,
    pub withdrawals: WithdrawalState,
    pub next_deposit_id: u64,
    pub next_withdrawal_id: u64,
}

impl Default for AppState {
    fn default() -> Self {
        Self {
            deposits: DepositState::default(),
            notes: NoteState::new(DEFAULT_DENOMINATIONS_SATS),
            fees: FeeState::default(),
            churn: ChurnState::default(),
            withdrawals: WithdrawalState::default(),
            next_deposit_id: 1,
            next_withdrawal_id: 1,
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct DepositState {
    pub intents: BTreeMap<String, DepositIntent>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DepositIntent {
    pub id: String,
    pub deposit_address: String,
    pub pow_token: String,
    pub txid: Option<String>,
    pub amount_sats: Option<u64>,
    pub confirmed: bool,
    pub split: bool,
    pub remainder_sats: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteState {
    pub trees: BTreeMap<u64, DenominationTree>,
    pub spent_nullifiers: BTreeSet<String>,
}

impl NoteState {
    pub fn new(denominations: impl IntoIterator<Item = u64>) -> Self {
        let trees = denominations
            .into_iter()
            .map(|denom| (denom, DenominationTree::default()))
            .collect();
        Self {
            trees,
            spent_nullifiers: BTreeSet::new(),
        }
    }
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

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct FeeState {
    pub current_bucket_sats: u64,
    pub sealed_buckets: u64,
    pub total_collected_sats: u64,
    pub bucket_target_sats: u64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct ChurnState {
    pub epoch: u64,
    pub offline_nodes: BTreeMap<u64, BTreeSet<String>>,
    pub penalized_nodes: BTreeMap<u64, BTreeSet<String>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalState {
    pub authorized: BTreeMap<String, AuthorizedWithdrawal>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AuthorizedWithdrawal {
    pub id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
    pub signature: MockSignature,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct MintedNote {
    pub denomination_sats: u64,
    pub commitment: String,
    pub index: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteCommitment {
    pub denomination_sats: u64,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteReceipt {
    pub deposit_id: String,
    pub denomination_sats: u64,
    pub index: u64,
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Snapshot {
    pub version: u32,
    pub state: AppState,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SplitReceipt {
    pub notes: Vec<NoteReceipt>,
    pub remainder_sats: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalProof {
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    pub merkle_root: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalPublicInputs {
    pub nullifier_hash: String,
    pub denomination_sats: u64,
    pub recipient: String,
    pub fee_sats: u64,
    pub merkle_root: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalRequest {
    pub withdrawal_id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct MockSignature {
    pub signer: String,
    pub digest: String,
}

pub trait ProofVerifier {
    fn verify_withdrawal(
        &self,
        proof: &WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()>;
}

pub trait CustodySigner {
    fn authorize_withdrawal(&self, request: &WithdrawalRequest) -> Result<MockSignature>;
}

#[derive(Debug, Default, Clone)]
pub struct MockProofVerifier;

impl ProofVerifier for MockProofVerifier {
    fn verify_withdrawal(
        &self,
        proof: &WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()> {
        let expected_commitment =
            note_commitment(&proof.nullifier, &proof.secret, public.denomination_sats);
        let expected_nullifier_hash = nullifier_hash(&proof.nullifier);

        if expected_commitment != proof.commitment
            || expected_nullifier_hash != public.nullifier_hash
            || proof.merkle_root != public.merkle_root
        {
            return Err(Error::InvalidProof);
        }

        Ok(())
    }
}

#[derive(Debug, Default, Clone)]
pub struct MockCustodySigner;

impl CustodySigner for MockCustodySigner {
    fn authorize_withdrawal(&self, request: &WithdrawalRequest) -> Result<MockSignature> {
        Ok(MockSignature {
            signer: "mock-frost-quorum".to_string(),
            digest: hash_json(request),
        })
    }
}

pub fn execute_command<V: ProofVerifier, S: CustodySigner>(
    state: &mut AppState,
    command: Command,
    verifier: &V,
    signer: &S,
) -> Result<Vec<Event>> {
    match command {
        Command::RequestDepositAddress { pow_token } => {
            let id = format!("dep-{}", state.next_deposit_id);
            let address = format!("tb1qmock{}", state.next_deposit_id);
            let event = Event::DepositIntentCreated {
                intent_id: id,
                deposit_address: address,
                pow_token,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::ObserveDeposit {
            intent_id,
            txid,
            amount_sats,
        } => {
            ensure_deposit_exists(state, &intent_id)?;
            let event = Event::DepositObserved {
                intent_id,
                txid,
                amount_sats,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::ConfirmDeposit { intent_id } => {
            ensure_deposit_exists(state, &intent_id)?;
            let event = Event::DepositConfirmed { intent_id };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::SplitDepositIntoNotes {
            deposit_id,
            note_commitments,
        } => split_deposit(state, &deposit_id, note_commitments),
        Command::WithdrawNote { proof, public } => {
            withdraw_note(state, proof, public, verifier, signer)
        }
        Command::StartChurnEpoch => {
            let event = Event::ChurnEpochStarted {
                epoch: state.churn.epoch + 1,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::MarkNodeOffline { node_id } => {
            let event = Event::NodeMarkedOffline {
                node_id,
                epoch: state.churn.epoch,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::ApplyChurnPenalties => {
            let nodes = state
                .churn
                .offline_nodes
                .get(&state.churn.epoch)
                .cloned()
                .unwrap_or_default();
            let mut events = Vec::new();
            for node_id in nodes {
                let event = Event::MockPenaltyApplied {
                    node_id,
                    epoch: state.churn.epoch,
                };
                apply_event(state, event.clone())?;
                events.push(event);
            }
            Ok(events)
        }
        Command::SaveSnapshot { path } => {
            save_snapshot(state, &path)?;
            let event = Event::SnapshotWritten {
                path,
                state_hash: state_hash(state),
            };
            Ok(vec![event])
        }
        Command::LoadSnapshot { path } => {
            *state = load_snapshot(&path)?;
            Ok(Vec::new())
        }
    }
}

pub fn apply_event(state: &mut AppState, event: Event) -> Result<()> {
    match event {
        Event::DepositIntentCreated {
            intent_id,
            deposit_address,
            pow_token,
        } => {
            state.deposits.intents.insert(
                intent_id.clone(),
                DepositIntent {
                    id: intent_id,
                    deposit_address,
                    pow_token,
                    txid: None,
                    amount_sats: None,
                    confirmed: false,
                    split: false,
                    remainder_sats: 0,
                },
            );
            state.next_deposit_id += 1;
        }
        Event::DepositObserved {
            intent_id,
            txid,
            amount_sats,
        } => {
            let intent = state
                .deposits
                .intents
                .get_mut(&intent_id)
                .ok_or(Error::DepositNotFound)?;
            intent.txid = Some(txid);
            intent.amount_sats = Some(amount_sats);
        }
        Event::DepositConfirmed { intent_id } => {
            let intent = state
                .deposits
                .intents
                .get_mut(&intent_id)
                .ok_or(Error::DepositNotFound)?;
            intent.confirmed = true;
        }
        Event::NotesMinted {
            deposit_id,
            notes,
            remainder_sats,
        } => {
            for note in notes {
                let tree = state
                    .notes
                    .trees
                    .get_mut(&note.denomination_sats)
                    .ok_or(Error::UnknownDenomination)?;
                tree.insert(note.commitment);
            }
            let intent = state
                .deposits
                .intents
                .get_mut(&deposit_id)
                .ok_or(Error::DepositNotFound)?;
            intent.split = true;
            intent.remainder_sats = remainder_sats;
        }
        Event::NoteSpent {
            nullifier_hash,
            denomination_sats: _,
        } => {
            state.notes.spent_nullifiers.insert(nullifier_hash);
        }
        Event::WithdrawalAuthorized {
            withdrawal_id,
            recipient,
            amount_sats,
            fee_sats,
            nullifier_hash,
            signature,
        } => {
            state.withdrawals.authorized.insert(
                withdrawal_id.clone(),
                AuthorizedWithdrawal {
                    id: withdrawal_id,
                    recipient,
                    amount_sats,
                    fee_sats,
                    nullifier_hash,
                    signature,
                },
            );
            state.next_withdrawal_id += 1;
        }
        Event::FeeCharged { amount_sats } => {
            state.fees.total_collected_sats += amount_sats;
            state.fees.current_bucket_sats += amount_sats;
            let target = fee_bucket_target(state);
            while state.fees.current_bucket_sats >= target {
                state.fees.current_bucket_sats -= target;
                state.fees.sealed_buckets += 1;
            }
        }
        Event::FeeBucketUpdated { .. } => {}
        Event::ChurnEpochStarted { epoch } => {
            state.churn.epoch = epoch;
        }
        Event::NodeMarkedOffline { node_id, epoch } => {
            state
                .churn
                .offline_nodes
                .entry(epoch)
                .or_default()
                .insert(node_id);
        }
        Event::MockPenaltyApplied { node_id, epoch } => {
            state
                .churn
                .penalized_nodes
                .entry(epoch)
                .or_default()
                .insert(node_id);
        }
        Event::SnapshotWritten { .. } => {}
    }
    Ok(())
}

pub fn save_snapshot(state: &AppState, path: impl AsRef<Path>) -> Result<()> {
    let snapshot = Snapshot {
        version: SNAPSHOT_VERSION,
        state: state.clone(),
    };
    let json = serde_json::to_string_pretty(&snapshot).map_err(|e| Error::Json(e.to_string()))?;
    fs::write(path, json).map_err(|e| Error::Io(e.to_string()))
}

pub fn load_snapshot(path: impl AsRef<Path>) -> Result<AppState> {
    let json = fs::read_to_string(path).map_err(|e| Error::Io(e.to_string()))?;
    load_snapshot_str(&json)
}

pub fn load_snapshot_str(json: &str) -> Result<AppState> {
    let value: serde_json::Value =
        serde_json::from_str(json).map_err(|e| Error::Json(e.to_string()))?;

    if value.get("version").is_some() || value.get("state").is_some() {
        let version = value
            .get("version")
            .and_then(|version| version.as_u64())
            .ok_or_else(|| Error::Json("snapshot version must be an integer".to_string()))?
            as u32;
        if version != SNAPSHOT_VERSION {
            return Err(Error::UnsupportedSnapshotVersion(version));
        }
        let state = value
            .get("state")
            .cloned()
            .ok_or_else(|| Error::Json("snapshot state is missing".to_string()))?;
        serde_json::from_value(state).map_err(|e| Error::Json(e.to_string()))
    } else {
        serde_json::from_value(value).map_err(|e| Error::Json(e.to_string()))
    }
}

pub fn replay_events(events: impl IntoIterator<Item = Event>) -> Result<AppState> {
    let mut state = AppState::default();
    for event in events {
        apply_event(&mut state, event)?;
    }
    Ok(state)
}

pub fn state_hash(state: &AppState) -> String {
    hash_json(state)
}

pub fn note_commitment(nullifier: &str, secret: &str, denomination_sats: u64) -> String {
    hash_parts(&[
        DOMAIN,
        "commitment",
        nullifier,
        secret,
        &denomination_sats.to_string(),
    ])
}

pub fn nullifier_hash(nullifier: &str) -> String {
    hash_parts(&[DOMAIN, "nullifier", nullifier])
}

pub fn merkle_root(leaves: &[String]) -> String {
    if leaves.is_empty() {
        return hash_parts(&[DOMAIN, "empty-tree"]);
    }

    let mut level = leaves.to_vec();
    while level.len() > 1 {
        let mut next = Vec::new();
        for pair in level.chunks(2) {
            let right = pair.get(1).unwrap_or(&pair[0]);
            next.push(hash_parts(&[DOMAIN, "node", &pair[0], right]));
        }
        level = next;
    }
    level[0].clone()
}

pub fn withdrawal_from_receipt(
    receipt: &NoteReceipt,
    merkle_root: String,
    recipient: String,
    fee_sats: u64,
) -> (WithdrawalProof, WithdrawalPublicInputs) {
    let proof = WithdrawalProof {
        nullifier: receipt.nullifier.clone(),
        secret: receipt.secret.clone(),
        commitment: receipt.commitment.clone(),
        merkle_root: merkle_root.clone(),
    };
    let public = WithdrawalPublicInputs {
        nullifier_hash: nullifier_hash(&receipt.nullifier),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root,
    };
    (proof, public)
}

fn split_deposit(
    state: &mut AppState,
    deposit_id: &str,
    note_commitments: Vec<NoteCommitment>,
) -> Result<Vec<Event>> {
    let intent = state
        .deposits
        .intents
        .get(deposit_id)
        .ok_or(Error::DepositNotFound)?;

    if !intent.confirmed {
        return Err(Error::DepositNotConfirmed);
    }
    if intent.split {
        return Err(Error::DepositAlreadySplit);
    }

    let amount_sats = intent.amount_sats.unwrap_or_default();
    let (expected_denominations, remaining) = greedy_denominations(amount_sats);

    if expected_denominations.is_empty() {
        return Err(Error::DepositTooSmall);
    }
    if note_commitments.len() != expected_denominations.len()
        || note_commitments
            .iter()
            .map(|note| note.denomination_sats)
            .ne(expected_denominations.iter().copied())
    {
        return Err(Error::InvalidNoteCommitments);
    }

    let notes = note_commitments
        .into_iter()
        .enumerate()
        .map(|(index, note)| MintedNote {
            denomination_sats: note.denomination_sats,
            commitment: note.commitment,
            index: index as u64,
        })
        .collect();

    let event = Event::NotesMinted {
        deposit_id: deposit_id.to_string(),
        notes,
        remainder_sats: remaining,
    };
    apply_event(state, event.clone())?;
    Ok(vec![event])
}

fn withdraw_note<V: ProofVerifier, S: CustodySigner>(
    state: &mut AppState,
    proof: WithdrawalProof,
    public: WithdrawalPublicInputs,
    verifier: &V,
    signer: &S,
) -> Result<Vec<Event>> {
    verifier.verify_withdrawal(&proof, &public)?;

    if state
        .notes
        .spent_nullifiers
        .contains(&public.nullifier_hash)
    {
        return Err(Error::DuplicateNullifier);
    }

    let tree = state
        .notes
        .trees
        .get(&public.denomination_sats)
        .ok_or(Error::UnknownDenomination)?;
    if !tree.known_roots.contains(&public.merkle_root) {
        return Err(Error::UnknownMerkleRoot);
    }
    if !tree.leaves.contains(&proof.commitment) {
        return Err(Error::UnknownCommitment);
    }

    let withdrawal_id = format!("wd-{}", state.next_withdrawal_id);
    let amount_sats = public
        .denomination_sats
        .checked_sub(public.fee_sats)
        .ok_or(Error::InvalidProof)?;
    let request = WithdrawalRequest {
        withdrawal_id: withdrawal_id.clone(),
        recipient: public.recipient.clone(),
        amount_sats,
        fee_sats: public.fee_sats,
        nullifier_hash: public.nullifier_hash.clone(),
    };
    let signature = signer.authorize_withdrawal(&request)?;

    let nullifier_hash = public.nullifier_hash.clone();
    let events = vec![
        Event::NoteSpent {
            nullifier_hash: nullifier_hash.clone(),
            denomination_sats: public.denomination_sats,
        },
        Event::WithdrawalAuthorized {
            withdrawal_id,
            recipient: public.recipient,
            amount_sats,
            fee_sats: public.fee_sats,
            nullifier_hash,
            signature,
        },
        Event::FeeCharged {
            amount_sats: public.fee_sats,
        },
    ];

    for event in events.clone() {
        apply_event(state, event)?;
    }
    let bucket = Event::FeeBucketUpdated {
        current_bucket_sats: state.fees.current_bucket_sats,
        sealed_buckets: state.fees.sealed_buckets,
    };
    Ok([events, vec![bucket]].concat())
}

fn ensure_deposit_exists(state: &AppState, id: &str) -> Result<()> {
    state
        .deposits
        .intents
        .contains_key(id)
        .then_some(())
        .ok_or(Error::DepositNotFound)
}

fn fee_bucket_target(state: &AppState) -> u64 {
    if state.fees.bucket_target_sats == 0 {
        DEFAULT_FEE_BUCKET_TARGET_SATS
    } else {
        state.fees.bucket_target_sats
    }
}

fn hash_json<T: Serialize>(value: &T) -> String {
    let json = serde_json::to_vec(value).expect("serializing deterministic state should not fail");
    hash_bytes(&json)
}

fn hash_parts(parts: &[&str]) -> String {
    let mut hasher = Sha256::new();
    for part in parts {
        hasher.update((part.len() as u64).to_be_bytes());
        hasher.update(part.as_bytes());
    }
    hex::encode(hasher.finalize())
}

fn hash_bytes(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    hex::encode(hasher.finalize())
}

pub fn happy_path_state() -> Result<(AppState, SplitReceipt)> {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: "mock".to_string(),
        },
        &verifier,
        &signer,
    )?;
    execute_command(
        &mut state,
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-demo".to_string(),
            amount_sats: 111_000_000,
        },
        &verifier,
        &signer,
    )?;
    execute_command(
        &mut state,
        Command::ConfirmDeposit {
            intent_id: "dep-1".to_string(),
        },
        &verifier,
        &signer,
    )?;
    let receipt = derive_split_receipt("dep-1", 111_000_000, "demo-client-seed")?;
    execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-1".to_string(),
            note_commitments: receipt.commitments(),
        },
        &verifier,
        &signer,
    )?;
    Ok((state, receipt))
}

pub fn derive_split_receipt(
    deposit_id: &str,
    amount_sats: u64,
    client_seed: &str,
) -> Result<SplitReceipt> {
    let (denominations, remaining) = greedy_denominations(amount_sats);
    let mut notes = Vec::new();
    for (index, denomination) in denominations.iter().copied().enumerate() {
        let index = index as u64;
        let nullifier = hash_parts(&[
            DOMAIN,
            "receipt-nullifier",
            client_seed,
            deposit_id,
            &index.to_string(),
        ]);
        let secret = hash_parts(&[
            DOMAIN,
            "receipt-secret",
            client_seed,
            deposit_id,
            &index.to_string(),
        ]);
        let commitment = note_commitment(&nullifier, &secret, denomination);
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
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

    Ok(SplitReceipt {
        notes,
        remainder_sats: remaining,
    })
}

impl SplitReceipt {
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
