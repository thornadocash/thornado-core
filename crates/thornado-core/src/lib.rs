use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::Path;

mod stark;
pub use stark::{
    prove_stark_withdrawal, stark_field_from_bytes, stark_field_from_string, stark_field_from_u64,
    stark_merkle_path, verify_stark_withdrawal, StarkMerklePath, StarkProofVerifier,
    TornadoStarkProof, STARK_MERKLE_DEPTH,
};

pub type Result<T> = std::result::Result<T, Error>;

pub const DOMAIN: &str = "thornado-mvp-v1";
pub const SNAPSHOT_VERSION: u32 = 1;
pub const DEFAULT_SIGNER_COUNT: u16 = 5;
pub const DEFAULT_DEPOSIT_POW_DIFFICULTY_BITS: u8 = 16;
pub const DEFAULT_FEE_BUCKET_TARGET_SATS: u64 = 30 * 1_000_000;
pub const DEFAULT_DENOMINATIONS_SATS: [u64; 4] =
    [1_000_000_000, 100_000_000, 10_000_000, 1_000_000];

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum Error {
    #[error("invalid proof")]
    InvalidProof,
    #[error("invalid field element")]
    InvalidFieldElement,
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
    #[error("invalid deposit proof of work")]
    InvalidDepositPow,
    #[error("deposit proof of work token was already used")]
    DuplicateDepositPow,
    #[error("unsupported snapshot version {0}")]
    UnsupportedSnapshotVersion(u32),
    #[error("unknown denomination")]
    UnknownDenomination,
    #[error("unknown merkle root")]
    UnknownMerkleRoot,
    #[error("unknown note commitment")]
    UnknownCommitment,
    #[error("unknown withdrawal")]
    UnknownWithdrawal,
    #[error("nullifier has already been spent")]
    DuplicateNullifier,
    #[error("io error: {0}")]
    Io(String),
    #[error("json error: {0}")]
    Json(String),
    #[error("frost error: {0}")]
    Frost(String),
    #[error("stark error: {0}")]
    Stark(String),
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum Command {
    RequestDepositAddress {
        pow_token: String,
        user_pubkey: String,
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
    CustodyKeysetGenerated {
        epoch: u64,
        keyset: FrostKeyset,
    },
    DepositIntentCreated {
        intent_id: String,
        deposit_address: String,
        pow_token: String,
        user_pubkey: String,
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
        signature: CustodySignature,
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
    pub custody: CustodyState,
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
            custody: CustodyState::default(),
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
pub struct CustodyState {
    pub active_epoch: u64,
    pub active_group_public_key: String,
    pub keysets: BTreeMap<u64, FrostKeyset>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct DepositState {
    pub intents: BTreeMap<String, DepositIntent>,
    #[serde(default)]
    pub used_pow_tokens: BTreeSet<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DepositIntent {
    pub id: String,
    pub deposit_address: String,
    pub pow_token: String,
    #[serde(default)]
    pub user_pubkey: String,
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

    pub fn path_for_commitment(&self, commitment: &str) -> Result<StarkMerklePath> {
        let index = self
            .leaves
            .iter()
            .position(|leaf| leaf == commitment)
            .ok_or(Error::UnknownCommitment)?;
        stark_merkle_path(&self.leaves, index)
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
    pub signature: CustodySignature,
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
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub stark: Option<TornadoStarkProof>,
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalRequest {
    pub withdrawal_id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CustodySignature {
    pub scheme: String,
    pub signer: String,
    pub message_digest: String,
    pub group_public_key: String,
    pub signature: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostKeyset {
    pub epoch: u64,
    pub threshold: u16,
    pub max_signers: u16,
    pub group_public_key: String,
    pub public_key_package: String,
}

#[derive(Debug, Clone)]
pub struct FrostSignerNode {
    identifier: frost_secp256k1::Identifier,
    key_package: frost_secp256k1::keys::KeyPackage,
}

#[derive(Debug)]
struct FrostSignerRound1 {
    identifier: frost_secp256k1::Identifier,
    nonces: frost_secp256k1::round1::SigningNonces,
    commitments: frost_secp256k1::round1::SigningCommitments,
}

#[derive(Debug, Clone)]
pub struct FrostSigningCoordinator {
    public_key_package: frost_secp256k1::keys::PublicKeyPackage,
    threshold: u16,
    max_signers: u16,
}

pub trait ProofVerifier {
    fn verify_withdrawal(
        &self,
        proof: &WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> Result<()>;

    fn reveals_commitment(&self) -> bool {
        true
    }
}

pub trait CustodySigner {
    fn authorize_withdrawal(&self, request: &WithdrawalRequest) -> Result<CustodySignature>;
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
    fn authorize_withdrawal(&self, request: &WithdrawalRequest) -> Result<CustodySignature> {
        let message = withdrawal_signing_message(request)?;
        Ok(CustodySignature {
            scheme: "mock-sha256".to_string(),
            signer: "mock-frost-quorum".to_string(),
            message_digest: hash_bytes(&message),
            group_public_key: "mock".to_string(),
            signature: hash_bytes(&message),
        })
    }
}

#[derive(Debug, Clone)]
pub struct FrostCustodySigner {
    key_packages: BTreeMap<frost_secp256k1::Identifier, frost_secp256k1::keys::KeyPackage>,
    public_key_package: frost_secp256k1::keys::PublicKeyPackage,
    threshold: u16,
}

impl FrostSignerNode {
    fn new(
        identifier: frost_secp256k1::Identifier,
        key_package: frost_secp256k1::keys::KeyPackage,
    ) -> Self {
        Self {
            identifier,
            key_package,
        }
    }

    pub fn identifier_hex(&self) -> String {
        hex::encode(self.identifier.serialize())
    }

    fn round1_commit(&self) -> FrostSignerRound1 {
        let mut rng = rand_core::OsRng;
        let (nonces, commitments) =
            frost_secp256k1::round1::commit(self.key_package.signing_share(), &mut rng);
        FrostSignerRound1 {
            identifier: self.identifier,
            nonces,
            commitments,
        }
    }

    fn sign_share(
        &self,
        signing_package: &frost_secp256k1::SigningPackage,
        nonces: &frost_secp256k1::round1::SigningNonces,
    ) -> Result<frost_secp256k1::round2::SignatureShare> {
        frost_secp256k1::round2::sign(signing_package, nonces, &self.key_package)
            .map_err(frost_error)
    }
}

impl FrostSigningCoordinator {
    pub fn new(keyset: &FrostKeyset) -> Result<Self> {
        let public_key_package_bytes = hex::decode(&keyset.public_key_package)
            .map_err(|e| Error::Frost(format!("invalid public key package hex: {e}")))?;
        let public_key_package =
            frost_secp256k1::keys::PublicKeyPackage::deserialize(&public_key_package_bytes)
                .map_err(frost_error)?;
        Ok(Self {
            public_key_package,
            threshold: keyset.threshold,
            max_signers: keyset.max_signers,
        })
    }

    pub fn sign_with_nodes(
        &self,
        request: &WithdrawalRequest,
        signers: &[FrostSignerNode],
    ) -> Result<CustodySignature> {
        if signers.len() < self.threshold as usize {
            return Err(Error::Frost(format!(
                "insufficient FROST signers: need {}, got {}",
                self.threshold,
                signers.len()
            )));
        }

        let selected = &signers[..self.threshold as usize];
        let message = withdrawal_signing_message(request)?;
        let rounds = selected
            .iter()
            .map(FrostSignerNode::round1_commit)
            .collect::<Vec<_>>();
        let commitments = rounds
            .iter()
            .map(|round| (round.identifier, round.commitments.clone()))
            .collect::<BTreeMap<_, _>>();
        let signing_package = frost_secp256k1::SigningPackage::new(commitments, &message);
        let mut signature_shares = BTreeMap::new();

        for (signer, round) in selected.iter().zip(rounds.iter()) {
            let share = signer.sign_share(&signing_package, &round.nonces)?;
            signature_shares.insert(round.identifier, share);
        }

        let signature = frost_secp256k1::aggregate(
            &signing_package,
            &signature_shares,
            &self.public_key_package,
        )
        .map_err(frost_error)?;
        self.public_key_package
            .verifying_key()
            .verify(&message, &signature)
            .map_err(frost_error)?;

        Ok(CustodySignature {
            scheme: "frost-secp256k1-sha256".to_string(),
            signer: format!("frost-{}-of-{}", self.threshold, self.max_signers),
            message_digest: hash_bytes(&message),
            group_public_key: self.group_public_key_hex()?,
            signature: signature
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
        })
    }

    pub fn group_public_key_hex(&self) -> Result<String> {
        self.public_key_package
            .verifying_key()
            .serialize()
            .map(hex::encode)
            .map_err(frost_error)
    }
}

impl FrostCustodySigner {
    pub fn generate_with_dkg(max_signers: u16, threshold: u16) -> Result<Self> {
        let mut rng = rand_core::OsRng;
        let mut round1_secret_packages = BTreeMap::new();
        let mut received_round1_packages: BTreeMap<
            frost_secp256k1::Identifier,
            BTreeMap<frost_secp256k1::Identifier, frost_secp256k1::keys::dkg::round1::Package>,
        > = BTreeMap::new();

        for participant_index in 1..=max_signers {
            let participant_identifier: frost_secp256k1::Identifier = participant_index
                .try_into()
                .map_err(|e| Error::Frost(format!("invalid participant id: {e:?}")))?;
            let (round1_secret_package, round1_package) = frost_secp256k1::keys::dkg::part1(
                participant_identifier,
                max_signers,
                threshold,
                &mut rng,
            )
            .map_err(frost_error)?;
            round1_secret_packages.insert(participant_identifier, round1_secret_package);

            for receiver_index in 1..=max_signers {
                if receiver_index == participant_index {
                    continue;
                }
                let receiver_identifier: frost_secp256k1::Identifier = receiver_index
                    .try_into()
                    .map_err(|e| Error::Frost(format!("invalid participant id: {e:?}")))?;
                received_round1_packages
                    .entry(receiver_identifier)
                    .or_default()
                    .insert(participant_identifier, round1_package.clone());
            }
        }

        let mut round2_secret_packages = BTreeMap::new();
        let mut received_round2_packages: BTreeMap<
            frost_secp256k1::Identifier,
            BTreeMap<frost_secp256k1::Identifier, frost_secp256k1::keys::dkg::round2::Package>,
        > = BTreeMap::new();

        for participant_index in 1..=max_signers {
            let participant_identifier: frost_secp256k1::Identifier = participant_index
                .try_into()
                .map_err(|e| Error::Frost(format!("invalid participant id: {e:?}")))?;
            let round1_secret_package = round1_secret_packages
                .remove(&participant_identifier)
                .ok_or_else(|| Error::Frost("missing DKG round 1 secret package".to_string()))?;
            let round1_packages = received_round1_packages
                .get(&participant_identifier)
                .ok_or_else(|| Error::Frost("missing DKG round 1 packages".to_string()))?;
            let (round2_secret_package, round2_packages) =
                frost_secp256k1::keys::dkg::part2(round1_secret_package, round1_packages)
                    .map_err(frost_error)?;
            round2_secret_packages.insert(participant_identifier, round2_secret_package);

            for (receiver_identifier, round2_package) in round2_packages {
                received_round2_packages
                    .entry(receiver_identifier)
                    .or_default()
                    .insert(participant_identifier, round2_package);
            }
        }

        let mut key_packages = BTreeMap::new();
        let mut public_key_package = None;
        for participant_index in 1..=max_signers {
            let participant_identifier: frost_secp256k1::Identifier = participant_index
                .try_into()
                .map_err(|e| Error::Frost(format!("invalid participant id: {e:?}")))?;
            let round2_secret_package = round2_secret_packages
                .get(&participant_identifier)
                .ok_or_else(|| Error::Frost("missing DKG round 2 secret package".to_string()))?;
            let round1_packages = received_round1_packages
                .get(&participant_identifier)
                .ok_or_else(|| Error::Frost("missing DKG round 1 packages".to_string()))?;
            let round2_packages = received_round2_packages
                .get(&participant_identifier)
                .ok_or_else(|| Error::Frost("missing DKG round 2 packages".to_string()))?;
            let (key_package, participant_public_key_package) = frost_secp256k1::keys::dkg::part3(
                round2_secret_package,
                round1_packages,
                round2_packages,
            )
            .map_err(frost_error)?;
            if let Some(public_key_package) = &public_key_package {
                if public_key_package != &participant_public_key_package {
                    return Err(Error::Frost(
                        "DKG participants produced inconsistent public key packages".to_string(),
                    ));
                }
            } else {
                public_key_package = Some(participant_public_key_package);
            }
            key_packages.insert(participant_identifier, key_package);
        }

        Ok(Self {
            key_packages,
            public_key_package: public_key_package
                .ok_or_else(|| Error::Frost("DKG produced no public key package".to_string()))?,
            threshold,
        })
    }

    pub fn generate_keyset_with_dkg(
        epoch: u64,
        max_signers: u16,
        threshold: u16,
    ) -> Result<FrostKeyset> {
        Self::generate_with_dkg(max_signers, threshold)?.to_keyset(epoch)
    }

    pub fn signer_nodes(&self) -> Vec<FrostSignerNode> {
        self.key_packages
            .iter()
            .map(|(identifier, key_package)| FrostSignerNode::new(*identifier, key_package.clone()))
            .collect()
    }

    pub fn coordinator(&self) -> FrostSigningCoordinator {
        FrostSigningCoordinator {
            public_key_package: self.public_key_package.clone(),
            threshold: self.threshold,
            max_signers: self.key_packages.len() as u16,
        }
    }

    pub fn to_keyset(&self, epoch: u64) -> Result<FrostKeyset> {
        Ok(FrostKeyset {
            epoch,
            threshold: self.threshold,
            max_signers: self.key_packages.len() as u16,
            group_public_key: self.group_public_key_hex()?,
            public_key_package: self
                .public_key_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
        })
    }

    pub fn demo_67_percent() -> Result<Self> {
        let max_signers = 5;
        Self::generate_with_dkg(max_signers, frost_threshold_for_committee(max_signers))
    }

    pub fn group_public_key_hex(&self) -> Result<String> {
        self.public_key_package
            .verifying_key()
            .serialize()
            .map(hex::encode)
            .map_err(frost_error)
    }
}

impl CustodySigner for FrostCustodySigner {
    fn authorize_withdrawal(&self, request: &WithdrawalRequest) -> Result<CustodySignature> {
        self.coordinator()
            .sign_with_nodes(request, &self.signer_nodes())
    }
}

pub fn execute_command<V: ProofVerifier, S: CustodySigner>(
    state: &mut AppState,
    command: Command,
    verifier: &V,
    signer: &S,
) -> Result<Vec<Event>> {
    match command {
        Command::RequestDepositAddress {
            pow_token,
            user_pubkey,
        } => {
            verify_deposit_pow(&pow_token)?;
            if state.deposits.used_pow_tokens.contains(&pow_token) {
                return Err(Error::DuplicateDepositPow);
            }
            let mut events = ensure_active_keyset(state)?;
            let id = format!("dep-{}", state.next_deposit_id);
            let address = derive_deposit_address(state, &id, &pow_token, &user_pubkey)?;
            let event = Event::DepositIntentCreated {
                intent_id: id,
                deposit_address: address,
                pow_token,
                user_pubkey,
            };
            apply_event(state, event.clone())?;
            events.push(event);
            Ok(events)
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
            let next_epoch = state.churn.epoch + 1;
            let event = Event::ChurnEpochStarted { epoch: next_epoch };
            let keyset = FrostCustodySigner::generate_keyset_with_dkg(
                next_epoch,
                DEFAULT_SIGNER_COUNT,
                frost_threshold_for_committee(DEFAULT_SIGNER_COUNT),
            )?;
            let keyset_event = Event::CustodyKeysetGenerated {
                epoch: next_epoch,
                keyset,
            };
            apply_event(state, event.clone())?;
            apply_event(state, keyset_event.clone())?;
            Ok(vec![event, keyset_event])
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
        Event::CustodyKeysetGenerated { epoch, keyset } => {
            state.custody.active_epoch = epoch;
            state.custody.active_group_public_key = keyset.group_public_key.clone();
            state.custody.keysets.insert(epoch, keyset);
        }
        Event::DepositIntentCreated {
            intent_id,
            deposit_address,
            pow_token,
            user_pubkey,
        } => {
            state.deposits.used_pow_tokens.insert(pow_token.clone());
            state.deposits.intents.insert(
                intent_id.clone(),
                DepositIntent {
                    id: intent_id,
                    deposit_address,
                    pow_token,
                    user_pubkey,
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
    let nullifier = stark_field_from_string(nullifier);
    let secret = stark_field_from_string(secret);
    let denomination = stark_field_from_u64(denomination_sats);
    stark::algebraic_hash3(nullifier, secret, denomination)
}

pub fn nullifier_hash(nullifier: &str) -> String {
    stark::algebraic_hash1(stark_field_from_string(nullifier))
}

pub fn merkle_root(leaves: &[String]) -> String {
    stark::fixed_depth_merkle_root(leaves)
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
        stark: None,
    };
    let public = WithdrawalPublicInputs {
        nullifier_hash: nullifier_hash(&receipt.nullifier),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root,
        recipient_field: None,
        relayer_field: None,
        refund_field: None,
    };
    (proof, public)
}

pub fn stark_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    let path = tree.path_for_commitment(&receipt.commitment)?;
    let recipient_field = stark_field_from_bytes(recipient.as_bytes());
    let relayer_field = stark_field_from_u64(0);
    let refund_field = stark_field_from_u64(0);
    let public = WithdrawalPublicInputs {
        nullifier_hash: nullifier_hash(&receipt.nullifier),
        denomination_sats: receipt.denomination_sats,
        recipient,
        fee_sats,
        merkle_root: tree.root(),
        recipient_field: Some(recipient_field),
        relayer_field: Some(relayer_field),
        refund_field: Some(refund_field),
    };
    let stark = prove_stark_withdrawal(
        &receipt.nullifier,
        &receipt.secret,
        receipt.denomination_sats,
        &path,
        &public,
    )?;
    let proof = WithdrawalProof {
        nullifier: String::new(),
        secret: String::new(),
        commitment: String::new(),
        merkle_root: public.merkle_root.clone(),
        stark: Some(stark),
    };
    Ok((proof, public))
}

pub fn active_custody_public_key(state: &AppState) -> Option<&str> {
    (!state.custody.active_group_public_key.is_empty())
        .then_some(state.custody.active_group_public_key.as_str())
}

pub fn derive_deposit_address(
    state: &AppState,
    intent_id: &str,
    pow_token: &str,
    user_pubkey: &str,
) -> Result<String> {
    let public_key = active_custody_public_key(state)
        .ok_or_else(|| Error::Frost("missing active custody keyset".to_string()))?;
    let hidden_tap_node = hash_parts_bytes(&[
        DOMAIN,
        "deposit-address",
        &state.custody.active_epoch.to_string(),
        public_key,
        user_pubkey,
        intent_id,
        pow_token,
    ]);
    let group_key_bytes = hex::decode(public_key)
        .map_err(|e| Error::Frost(format!("invalid custody public key hex: {e}")))?;
    let group_key = bitcoin::secp256k1::PublicKey::from_slice(&group_key_bytes)
        .map_err(|e| Error::Frost(format!("invalid custody public key: {e}")))?;
    let (internal_key, _) = group_key.x_only_public_key();
    let merkle_root = bitcoin::taproot::TapNodeHash::assume_hidden(
        hidden_tap_node
            .try_into()
            .expect("sha256 output is exactly 32 bytes"),
    );
    let secp = bitcoin::secp256k1::Secp256k1::verification_only();
    Ok(bitcoin::Address::p2tr(
        &secp,
        internal_key,
        Some(merkle_root),
        bitcoin::KnownHrp::Regtest,
    )
    .to_string())
}

pub fn deposit_pow_digest(pow_token: &str) -> String {
    hash_parts(&[DOMAIN, "deposit-pow", pow_token])
}

pub fn verify_deposit_pow(pow_token: &str) -> Result<()> {
    let digest = hash_parts_bytes(&[DOMAIN, "deposit-pow", pow_token]);
    has_leading_zero_bits(&digest, DEFAULT_DEPOSIT_POW_DIFFICULTY_BITS)
        .then_some(())
        .ok_or(Error::InvalidDepositPow)
}

pub fn mine_deposit_pow(prefix: &str) -> String {
    for nonce in 0u64.. {
        let pow_token = format!("{prefix}:{nonce}");
        if verify_deposit_pow(&pow_token).is_ok() {
            return pow_token;
        }
    }
    unreachable!("u64 nonce space exhausted")
}

pub fn verify_custody_signature(
    request: &WithdrawalRequest,
    signature: &CustodySignature,
) -> Result<()> {
    if signature.scheme == "mock-sha256" {
        let message = withdrawal_signing_message(request)?;
        let digest = hash_bytes(&message);
        return (signature.message_digest == digest && signature.signature == digest)
            .then_some(())
            .ok_or_else(|| Error::Frost("mock signature mismatch".to_string()));
    }

    if signature.scheme != "frost-secp256k1-sha256" {
        return Err(Error::Frost(format!(
            "unsupported custody signature scheme {}",
            signature.scheme
        )));
    }

    let message = withdrawal_signing_message(request)?;
    if signature.message_digest != hash_bytes(&message) {
        return Err(Error::Frost("message digest mismatch".to_string()));
    }

    let public_key_bytes = hex::decode(&signature.group_public_key)
        .map_err(|e| Error::Frost(format!("invalid group public key hex: {e}")))?;
    let signature_bytes = hex::decode(&signature.signature)
        .map_err(|e| Error::Frost(format!("invalid signature hex: {e}")))?;
    let public_key =
        frost_secp256k1::VerifyingKey::deserialize(&public_key_bytes).map_err(frost_error)?;
    let signature =
        frost_secp256k1::Signature::deserialize(&signature_bytes).map_err(frost_error)?;
    public_key.verify(&message, &signature).map_err(frost_error)
}

pub fn frost_threshold_for_committee(max_signers: u16) -> u16 {
    ((max_signers as u32 * 67).div_ceil(100)) as u16
}

fn ensure_active_keyset(state: &mut AppState) -> Result<Vec<Event>> {
    if state
        .custody
        .keysets
        .contains_key(&state.custody.active_epoch)
    {
        return Ok(Vec::new());
    }

    let keyset = FrostCustodySigner::generate_keyset_with_dkg(
        state.churn.epoch,
        DEFAULT_SIGNER_COUNT,
        frost_threshold_for_committee(DEFAULT_SIGNER_COUNT),
    )?;
    let event = Event::CustodyKeysetGenerated {
        epoch: state.churn.epoch,
        keyset,
    };
    apply_event(state, event.clone())?;
    Ok(vec![event])
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
    if verifier.reveals_commitment() && !tree.leaves.contains(&proof.commitment) {
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

fn withdrawal_signing_message(request: &WithdrawalRequest) -> Result<Vec<u8>> {
    serde_json::to_vec(request).map_err(|e| Error::Json(e.to_string()))
}

fn frost_error<E: std::fmt::Debug>(error: E) -> Error {
    Error::Frost(format!("{error:?}"))
}

fn hash_json<T: Serialize>(value: &T) -> String {
    let json = serde_json::to_vec(value).expect("serializing deterministic state should not fail");
    hash_bytes(&json)
}

fn hash_parts(parts: &[&str]) -> String {
    hex::encode(hash_parts_bytes(parts))
}

fn hash_parts_bytes(parts: &[&str]) -> Vec<u8> {
    let mut hasher = Sha256::new();
    for part in parts {
        hasher.update((part.len() as u64).to_be_bytes());
        hasher.update(part.as_bytes());
    }
    hasher.finalize().to_vec()
}

fn hash_bytes(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    hex::encode(hasher.finalize())
}

fn has_leading_zero_bits(bytes: &[u8], bits: u8) -> bool {
    let full_zero_bytes = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    if bytes.iter().take(full_zero_bytes).any(|byte| *byte != 0) {
        return false;
    }

    if remaining_bits == 0 {
        return true;
    }

    let Some(byte) = bytes.get(full_zero_bytes) else {
        return false;
    };
    let mask = 0xff << (8 - remaining_bits);
    byte & mask == 0
}

pub fn happy_path_state() -> Result<(AppState, SplitReceipt)> {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: mine_deposit_pow("happy-path"),
            user_pubkey: "test-client-pubkey".to_string(),
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
        let nullifier = stark_field_from_bytes(nullifier.as_bytes());
        let secret = hash_parts(&[
            DOMAIN,
            "receipt-secret",
            client_seed,
            deposit_id,
            &index.to_string(),
        ]);
        let secret = stark_field_from_bytes(secret.as_bytes());
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
