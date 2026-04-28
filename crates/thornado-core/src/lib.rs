use frost_secp256k1_tr::keys::Tweak;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::Path;

#[cfg(feature = "orchard-zcash")]
pub mod orchard;
mod zk;
pub use zk::ZkProofVerifier;

pub type Result<T> = std::result::Result<T, Error>;

pub const DOMAIN: &str = "thornado-mvp-v1";
pub const SNAPSHOT_VERSION: u32 = 1;
pub const DEFAULT_SIGNER_COUNT: u16 = 5;
pub const DEFAULT_DEPOSIT_POW_DIFFICULTY_BITS: u8 = 16;
pub const DEFAULT_FEE_BUCKET_TARGET_SATS: u64 = 30 * 1_000_000;
pub const BTC_SATS: u64 = 100_000_000;
pub const BASE_NODE_BOND_SATS: u64 = 10 * BTC_SATS;
pub const NODE_BOND_INCREMENT_SATS: u64 = 250_000_000;
pub const DEFAULT_CHURN_CYCLE_MS: u64 = 20 * 60 * 1000;
pub const DEFAULT_TARGET_ACTIVE_NODES: u16 = 4;
pub const DEFAULT_MAX_NODES_PER_CHURN: u16 = 1;
pub const DEFAULT_BITCOIN_KEYSIGN_GRACE_EPOCHS: u64 = 1;
pub const DEFAULT_BITCOIN_ATTESTATION_GRACE_EPOCHS: u64 = 1;
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
    #[error("note owner public key does not match deposit owner")]
    InvalidNoteOwner,
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
    #[error("node account not found")]
    NodeNotFound,
    #[error("node account already exists")]
    NodeAlreadyExists,
    #[error("node slot is already assigned")]
    NodeSlotAlreadyAssigned,
    #[error("node bond is below required slot bond")]
    InsufficientNodeBond,
    #[error("node is not active")]
    NodeNotActive,
    #[error("invalid network parameter")]
    InvalidNetworkParameter,
    #[error("stale bitcoin solvency report")]
    StaleBitcoinSolvencyReport,
    #[error("invalid bitcoin solvency reporter")]
    InvalidBitcoinSolvencyReporter,
    #[error("invalid bitcoin attestation reporter")]
    InvalidBitcoinAttestationReporter,
    #[error("stale bitcoin attestation")]
    StaleBitcoinAttestation,
    #[error("conflicting bitcoin attestation")]
    ConflictingBitcoinAttestation,
    #[error("missing bitcoin attestation quorum")]
    MissingBitcoinAttestationQuorum,
    #[error("nullifier has already been spent")]
    DuplicateNullifier,
    #[error("io error: {0}")]
    Io(String),
    #[error("json error: {0}")]
    Json(String),
    #[error("frost error: {0}")]
    Frost(String),
    #[error("zk error: {0}")]
    Zk(String),
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
    ApplyDepositRetirements,
    WithdrawNote {
        proof: WithdrawalProof,
        public: WithdrawalPublicInputs,
    },
    RequestWithdrawal {
        proof: WithdrawalProof,
        public: WithdrawalPublicInputs,
    },
    AuthorizeWithdrawal {
        withdrawal_id: String,
        signature: CustodySignature,
    },
    RegisterNode {
        node_id: String,
        bond_address: String,
        consensus_pubkey: String,
        signer_pubkey: String,
    },
    BondNode {
        node_id: String,
        amount_sats: u64,
    },
    AssignNodeSlot {
        node_id: String,
        slot_id: u64,
    },
    SetBondParameters {
        min_bond_sats: u64,
        min_bond_increase_sats: u64,
    },
    VoteNetworkParameters {
        node_id: String,
        churn_cycle_ms: Option<u64>,
        target_active_nodes: Option<u16>,
        max_nodes_per_churn: Option<u16>,
        #[serde(default)]
        bitcoin_keysign_grace_epochs: Option<u64>,
        #[serde(default)]
        bitcoin_attestation_grace_epochs: Option<u64>,
    },
    RegisterStandbyNode {
        node_id: String,
    },
    StartChurnEpoch,
    CommitCustodyKeyset {
        epoch: u64,
        keyset: FrostKeyset,
    },
    SubmitBitcoinBroadcast {
        withdrawal_id: String,
        txid: String,
    },
    SubmitDepositSweep {
        deposit_id: String,
        txid: String,
    },
    SubmitVaultSweep {
        from_epoch: u64,
        to_epoch: u64,
        txid: String,
    },
    AttestBitcoinWithdrawal {
        withdrawal_id: String,
        txid: String,
        signed_tx_hash: String,
        attester: String,
        epoch: u64,
    },
    ApplyBitcoinOutboundPenalties,
    SubmitBitcoinSolvency {
        reporter: String,
        epoch: u64,
        expected_sats: u64,
        actual_sats: u64,
        spendable_sats: u64,
        solvent: bool,
    },
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
    DepositCreditCreated {
        credit_id: String,
        source_intent_id: String,
    },
    DepositObserved {
        intent_id: String,
        txid: String,
        amount_sats: u64,
    },
    DepositConfirmed {
        intent_id: String,
    },
    DepositSplitRecorded {
        deposit_id: String,
        remainder_sats: u64,
    },
    NotesMinted {
        notes: Vec<MintedNote>,
    },
    DepositIntentRetired {
        intent_id: String,
        deposit_address: String,
        custody_epoch: u64,
        reason: String,
        epoch: u64,
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
        #[serde(default)]
        custody_epoch: u64,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        deposit_key_tweak: String,
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        vault_signers: Vec<String>,
        #[serde(default, skip_serializing_if = "is_zero_u16")]
        vault_threshold: u16,
        signature: CustodySignature,
    },
    WithdrawalRequested {
        withdrawal_id: String,
        recipient: String,
        amount_sats: u64,
        fee_sats: u64,
        nullifier_hash: String,
        denomination_sats: u64,
        #[serde(default)]
        custody_epoch: u64,
        #[serde(default, skip_serializing_if = "String::is_empty")]
        deposit_key_tweak: String,
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        vault_signers: Vec<String>,
        #[serde(default, skip_serializing_if = "is_zero_u16")]
        vault_threshold: u16,
    },
    BitcoinWithdrawalBroadcastSubmitted {
        withdrawal_id: String,
        txid: String,
    },
    BitcoinDepositSweepSubmitted {
        deposit_id: String,
        txid: String,
    },
    BitcoinVaultSweepSubmitted {
        from_epoch: u64,
        to_epoch: u64,
        txid: String,
    },
    BitcoinWithdrawalAttested {
        withdrawal_id: String,
        txid: String,
        signed_tx_hash: String,
        attester: String,
        epoch: u64,
    },
    BitcoinSolvencyReported {
        reporter: String,
        expected_sats: u64,
        actual_sats: u64,
        spendable_sats: u64,
        solvent: bool,
        epoch: u64,
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
    NodeRegistered {
        node: NodeAccount,
    },
    NodeBonded {
        node_id: String,
        amount_sats: u64,
        total_bond_sats: u64,
    },
    NodeSlotAssigned {
        node_id: String,
        slot_id: u64,
        required_bond_sats: u64,
    },
    BondParametersUpdated {
        min_bond_sats: u64,
        min_bond_increase_sats: u64,
    },
    NetworkParameterVoteCast {
        node_id: String,
        churn_cycle_ms: u64,
        #[serde(default = "default_target_active_nodes")]
        target_active_nodes: u16,
        #[serde(default = "default_max_nodes_per_churn")]
        max_nodes_per_churn: u16,
        #[serde(default = "default_bitcoin_keysign_grace_epochs")]
        bitcoin_keysign_grace_epochs: u64,
        #[serde(default = "default_bitcoin_attestation_grace_epochs")]
        bitcoin_attestation_grace_epochs: u64,
        epoch: u64,
    },
    NetworkParametersUpdated {
        churn_cycle_ms: u64,
        #[serde(default = "default_target_active_nodes")]
        target_active_nodes: u16,
        #[serde(default = "default_max_nodes_per_churn")]
        max_nodes_per_churn: u16,
        #[serde(default = "default_bitcoin_keysign_grace_epochs")]
        bitcoin_keysign_grace_epochs: u64,
        #[serde(default = "default_bitcoin_attestation_grace_epochs")]
        bitcoin_attestation_grace_epochs: u64,
        epoch: u64,
    },
    NodeStatusUpdated {
        node_id: String,
        status: NodeStatus,
        epoch: u64,
    },
    StandbyNodeRegistered {
        node_id: String,
    },
    StandbyNodeActivated {
        node_id: String,
        epoch: u64,
    },
    NodeMarkedOffline {
        node_id: String,
        epoch: u64,
    },
    NodeSlashPointsAdded {
        node_id: String,
        points: u64,
        reason: String,
        epoch: u64,
    },
    BitcoinOutboundPenaltyMarked {
        withdrawal_id: String,
        node_id: String,
        penalty_kind: String,
    },
    NodeBondSlashed {
        node_id: String,
        amount_sats: u64,
        reason: String,
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
    #[serde(default)]
    pub bitcoin_solvency: BitcoinSolvencyState,
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
            bitcoin_solvency: BitcoinSolvencyState::default(),
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
    #[serde(default)]
    pub vaults: BTreeMap<u64, CustodyVault>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct CustodyVault {
    pub epoch: u64,
    #[serde(default)]
    pub signers: Vec<String>,
    pub threshold: u16,
    #[serde(default)]
    pub sweep_txid: Option<String>,
    #[serde(default)]
    pub swept_to_epoch: Option<u64>,
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
    #[serde(default)]
    pub source_intent_id: Option<String>,
    pub deposit_address: String,
    #[serde(default)]
    pub custody_epoch: u64,
    #[serde(default)]
    pub deposit_key_tweak: String,
    #[serde(default)]
    pub deposit_public_key: String,
    #[serde(default)]
    pub vault_signers: Vec<String>,
    #[serde(default)]
    pub vault_threshold: u16,
    pub pow_token: String,
    #[serde(default)]
    pub user_pubkey: String,
    pub txid: Option<String>,
    pub amount_sats: Option<u64>,
    pub confirmed: bool,
    #[serde(default)]
    pub swept_txid: Option<String>,
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

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ChurnState {
    pub epoch: u64,
    #[serde(default = "default_churn_cycle_ms")]
    pub churn_cycle_ms: u64,
    #[serde(default = "default_target_active_nodes")]
    pub target_active_nodes: u16,
    #[serde(default = "default_max_nodes_per_churn")]
    pub max_nodes_per_churn: u16,
    #[serde(default = "default_bitcoin_keysign_grace_epochs")]
    pub bitcoin_keysign_grace_epochs: u64,
    #[serde(default = "default_bitcoin_attestation_grace_epochs")]
    pub bitcoin_attestation_grace_epochs: u64,
    #[serde(default = "default_min_bond_sats")]
    pub min_bond_sats: u64,
    #[serde(default = "default_min_bond_increase_sats")]
    pub min_bond_increase_sats: u64,
    #[serde(default)]
    pub node_accounts: BTreeMap<String, NodeAccount>,
    #[serde(default)]
    pub node_slots: BTreeMap<u64, NodeSlot>,
    #[serde(default)]
    pub active_nodes: BTreeSet<String>,
    #[serde(default)]
    pub standby_nodes: BTreeSet<String>,
    pub offline_nodes: BTreeMap<u64, BTreeSet<String>>,
    pub penalized_nodes: BTreeMap<u64, BTreeSet<String>>,
    #[serde(default)]
    pub network_parameter_votes: BTreeMap<u64, BTreeMap<String, NetworkParameterVote>>,
}

impl Default for ChurnState {
    fn default() -> Self {
        Self {
            epoch: 0,
            churn_cycle_ms: DEFAULT_CHURN_CYCLE_MS,
            target_active_nodes: DEFAULT_TARGET_ACTIVE_NODES,
            max_nodes_per_churn: DEFAULT_MAX_NODES_PER_CHURN,
            bitcoin_keysign_grace_epochs: DEFAULT_BITCOIN_KEYSIGN_GRACE_EPOCHS,
            bitcoin_attestation_grace_epochs: DEFAULT_BITCOIN_ATTESTATION_GRACE_EPOCHS,
            min_bond_sats: BASE_NODE_BOND_SATS,
            min_bond_increase_sats: NODE_BOND_INCREMENT_SATS,
            node_accounts: BTreeMap::new(),
            node_slots: BTreeMap::new(),
            active_nodes: BTreeSet::new(),
            standby_nodes: BTreeSet::new(),
            offline_nodes: BTreeMap::new(),
            penalized_nodes: BTreeMap::new(),
            network_parameter_votes: BTreeMap::new(),
        }
    }
}

fn default_churn_cycle_ms() -> u64 {
    DEFAULT_CHURN_CYCLE_MS
}

fn default_target_active_nodes() -> u16 {
    DEFAULT_TARGET_ACTIVE_NODES
}

fn default_max_nodes_per_churn() -> u16 {
    DEFAULT_MAX_NODES_PER_CHURN
}

fn default_bitcoin_keysign_grace_epochs() -> u64 {
    DEFAULT_BITCOIN_KEYSIGN_GRACE_EPOCHS
}

fn default_bitcoin_attestation_grace_epochs() -> u64 {
    DEFAULT_BITCOIN_ATTESTATION_GRACE_EPOCHS
}

fn default_min_bond_sats() -> u64 {
    BASE_NODE_BOND_SATS
}

fn default_min_bond_increase_sats() -> u64 {
    NODE_BOND_INCREMENT_SATS
}

fn is_zero_u16(value: &u16) -> bool {
    *value == 0
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum NodeStatus {
    Unknown,
    Standby,
    Ready,
    Active,
    Disabled,
}

impl Default for NodeStatus {
    fn default() -> Self {
        Self::Unknown
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct NodeAccount {
    pub node_id: String,
    pub status: NodeStatus,
    pub bond_sats: u64,
    pub bond_address: String,
    pub slot_id: Option<u64>,
    pub consensus_pubkey: String,
    pub signer_pubkey: String,
    pub status_since_epoch: u64,
    pub active_since_epoch: Option<u64>,
    pub slash_points: u64,
    pub missed_observations: u64,
    pub missed_keysigns: u64,
    pub requested_leave: bool,
    pub forced_leave: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NodeSlot {
    pub slot_id: u64,
    pub owner_node_id: String,
    pub required_bond_sats: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NetworkParameterVote {
    pub node_id: String,
    pub churn_cycle_ms: u64,
    #[serde(default = "default_target_active_nodes")]
    pub target_active_nodes: u16,
    #[serde(default = "default_max_nodes_per_churn")]
    pub max_nodes_per_churn: u16,
    #[serde(default = "default_bitcoin_keysign_grace_epochs")]
    pub bitcoin_keysign_grace_epochs: u64,
    #[serde(default = "default_bitcoin_attestation_grace_epochs")]
    pub bitcoin_attestation_grace_epochs: u64,
    pub epoch: u64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalState {
    #[serde(default)]
    pub pending: BTreeMap<String, PendingWithdrawal>,
    pub authorized: BTreeMap<String, AuthorizedWithdrawal>,
    #[serde(default)]
    pub bitcoin_outbounds: BTreeMap<String, BitcoinOutbound>,
    #[serde(default)]
    pub bitcoin_broadcasts: BTreeMap<String, BitcoinBroadcastRecord>,
    #[serde(default)]
    pub bitcoin_attestations: BTreeMap<String, BitcoinWithdrawalAttestationSet>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PendingWithdrawal {
    pub id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
    pub denomination_sats: u64,
    #[serde(default)]
    pub custody_epoch: u64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub deposit_key_tweak: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub vault_signers: Vec<String>,
    #[serde(default, skip_serializing_if = "is_zero_u16")]
    pub vault_threshold: u16,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AuthorizedWithdrawal {
    pub id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
    #[serde(default)]
    pub custody_epoch: u64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub deposit_key_tweak: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub vault_signers: Vec<String>,
    #[serde(default, skip_serializing_if = "is_zero_u16")]
    pub vault_threshold: u16,
    pub signature: CustodySignature,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinOutbound {
    pub withdrawal_id: String,
    pub recipient: String,
    pub amount_sats: u64,
    pub fee_sats: u64,
    pub nullifier_hash: String,
    #[serde(default)]
    pub custody_epoch: u64,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub deposit_key_tweak: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub vault_signers: Vec<String>,
    #[serde(default, skip_serializing_if = "is_zero_u16")]
    pub vault_threshold: u16,
    pub scheduled_epoch: u64,
    #[serde(default)]
    pub leader: Option<String>,
    #[serde(default)]
    pub signers: Vec<String>,
    #[serde(default)]
    pub attesters: Vec<String>,
    #[serde(default)]
    pub signing_deadline_epoch: u64,
    #[serde(default)]
    pub attestation_deadline_epoch: u64,
    #[serde(default)]
    pub keysign_penalized: BTreeSet<String>,
    #[serde(default)]
    pub attestation_penalized: BTreeSet<String>,
    pub published_txid: Option<String>,
    pub published_epoch: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinBroadcastRecord {
    pub withdrawal_id: String,
    pub txid: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinWithdrawalAttestationSet {
    pub withdrawal_id: String,
    pub txid: String,
    pub signed_tx_hash: String,
    pub epoch: u64,
    #[serde(default)]
    pub attestations: BTreeMap<String, BitcoinWithdrawalAttestation>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinWithdrawalAttestation {
    pub attester: String,
    pub txid: String,
    pub signed_tx_hash: String,
    pub epoch: u64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinSolvencyState {
    pub latest: Option<BitcoinSolvencyRecord>,
    pub halted: bool,
    #[serde(default)]
    pub reports_by_epoch: BTreeMap<u64, BTreeMap<String, BitcoinSolvencyRecord>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BitcoinSolvencyRecord {
    pub reporter: String,
    pub expected_sats: u64,
    pub actual_sats: u64,
    pub spendable_sats: u64,
    pub solvent: bool,
    pub epoch: u64,
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
    #[serde(default)]
    pub owner_pubkey: String,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NoteReceipt {
    pub deposit_id: String,
    pub denomination_sats: u64,
    pub index: u64,
    #[serde(default)]
    pub owner_pubkey: String,
    pub nullifier: String,
    pub secret: String,
    pub commitment: String,
    #[cfg(feature = "orchard-zcash")]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardNoteReceipt>,
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
    #[cfg(feature = "orchard-zcash")]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub orchard: Option<orchard::OrchardWithdrawalProof>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WithdrawalPublicInputs {
    pub nullifier_hash: String,
    #[serde(default)]
    pub owner_pubkey: String,
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
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub key_tweak: Option<String>,
    pub signature: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostKeyset {
    pub epoch: u64,
    pub threshold: u16,
    pub max_signers: u16,
    pub group_public_key: String,
    pub public_key_package: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub taproot_group_public_key: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub taproot_public_key_package: Option<String>,
}

#[derive(Debug, Clone)]
pub struct FrostSignerNode {
    identifier: frost_secp256k1::Identifier,
    key_package: frost_secp256k1::keys::KeyPackage,
}

#[derive(Debug, Clone)]
pub struct TaprootFrostSignerNode {
    key_package: frost_secp256k1_tr::keys::KeyPackage,
}

#[derive(Debug)]
struct FrostSignerRound1 {
    identifier: frost_secp256k1::Identifier,
    nonces: frost_secp256k1::round1::SigningNonces,
    commitments: frost_secp256k1::round1::SigningCommitments,
}

#[derive(Debug)]
struct TaprootFrostSignerRound1 {
    nonces: frost_secp256k1_tr::round1::SigningNonces,
    commitments: frost_secp256k1_tr::round1::SigningCommitments,
}

#[derive(Debug, Clone)]
pub struct FrostSigningCoordinator {
    public_key_package: frost_secp256k1::keys::PublicKeyPackage,
    threshold: u16,
    max_signers: u16,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DepositChildKey {
    pub address: String,
    pub custody_epoch: u64,
    pub key_tweak: String,
    pub child_public_key: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostSigningCommitment {
    pub signer_id: String,
    pub commitment: String,
    pub nonces: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostSigningCommitmentPublic {
    pub signer_id: String,
    pub commitment: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostSignatureShare {
    pub signer_id: String,
    pub share: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostDkgRound1Output {
    pub signer_id: String,
    pub secret_package: String,
    pub package: String,
    pub taproot_secret_package: String,
    pub taproot_package: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostDkgRound1Public {
    pub signer_id: String,
    pub package: String,
    #[serde(default)]
    pub taproot_package: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostDkgRound2Output {
    pub signer_id: String,
    pub secret_package: String,
    pub packages: BTreeMap<String, String>,
    pub taproot_secret_package: String,
    pub taproot_packages: BTreeMap<String, String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostDkgRound2Public {
    pub signer_id: String,
    pub packages: BTreeMap<String, String>,
    #[serde(default)]
    pub taproot_packages: BTreeMap<String, String>,
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
        let expected_commitment = note_commitment(
            &proof.nullifier,
            &proof.secret,
            public.denomination_sats,
            &public.owner_pubkey,
        );
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
            key_tweak: None,
            signature: hash_bytes(&message),
        })
    }
}

#[derive(Debug, Clone)]
pub struct FrostCustodySigner {
    key_packages: BTreeMap<frost_secp256k1::Identifier, frost_secp256k1::keys::KeyPackage>,
    public_key_package: frost_secp256k1::keys::PublicKeyPackage,
    taproot_key_packages:
        BTreeMap<frost_secp256k1_tr::Identifier, frost_secp256k1_tr::keys::KeyPackage>,
    taproot_public_key_package: Option<frost_secp256k1_tr::keys::PublicKeyPackage>,
    threshold: u16,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostCustodySignerSnapshot {
    pub version: u32,
    pub threshold: u16,
    pub public_key_package: String,
    pub key_packages: Vec<FrostKeyPackageSnapshot>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub taproot_public_key_package: Option<String>,
    #[serde(default)]
    pub taproot_key_packages: Vec<FrostKeyPackageSnapshot>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FrostKeyPackageSnapshot {
    pub identifier: String,
    pub key_package: String,
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

    fn sign_share_with_tweak(
        &self,
        signing_package: &frost_secp256k1::SigningPackage,
        nonces: &frost_secp256k1::round1::SigningNonces,
        key_tweak: &str,
    ) -> Result<frost_secp256k1::round2::SignatureShare> {
        let randomizer = frost_randomizer_from_hex(key_tweak)?;
        #[allow(deprecated)]
        frost_rerandomized::sign(signing_package, nonces, &self.key_package, randomizer)
            .map_err(frost_error)
    }
}

impl TaprootFrostSignerNode {
    fn new(
        _identifier: frost_secp256k1_tr::Identifier,
        key_package: frost_secp256k1_tr::keys::KeyPackage,
    ) -> Self {
        Self { key_package }
    }

    fn round1_commit(&self) -> TaprootFrostSignerRound1 {
        let mut rng = rand_core::OsRng;
        let (nonces, commitments) =
            frost_secp256k1_tr::round1::commit(self.key_package.signing_share(), &mut rng);
        TaprootFrostSignerRound1 {
            nonces,
            commitments,
        }
    }

    fn sign_share(
        &self,
        signing_package: &frost_secp256k1_tr::SigningPackage,
        nonces: &frost_secp256k1_tr::round1::SigningNonces,
        merkle_root: Option<&[u8]>,
    ) -> Result<frost_secp256k1_tr::round2::SignatureShare> {
        frost_secp256k1_tr::round2::sign_with_tweak(
            signing_package,
            nonces,
            &self.key_package,
            merkle_root,
        )
        .map_err(frost_tr_error)
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
            key_tweak: None,
            signature: signature
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
        })
    }

    pub fn sign_with_child_tweak(
        &self,
        request: &WithdrawalRequest,
        signers: &[FrostSignerNode],
        key_tweak: &str,
    ) -> Result<CustodySignature> {
        if signers.len() < self.threshold as usize {
            return Err(Error::Frost(format!(
                "insufficient FROST signers: need {}, got {}",
                self.threshold,
                signers.len()
            )));
        }

        let randomizer = frost_randomizer_from_hex(key_tweak)?;
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
            #[allow(deprecated)]
            let share = frost_rerandomized::sign(
                &signing_package,
                &round.nonces,
                &signer.key_package,
                randomizer.clone(),
            )
            .map_err(frost_error)?;
            signature_shares.insert(round.identifier, share);
        }

        let randomized_params = frost_rerandomized::RandomizedParams::from_randomizer(
            self.public_key_package.verifying_key(),
            randomizer,
        );
        let signature = frost_rerandomized::aggregate(
            &signing_package,
            &signature_shares,
            &self.public_key_package,
            &randomized_params,
        )
        .map_err(frost_error)?;
        randomized_params
            .randomized_verifying_key()
            .verify(&message, &signature)
            .map_err(frost_error)?;

        Ok(CustodySignature {
            scheme: "frost-secp256k1-sha256+tweak".to_string(),
            signer: format!("frost-{}-of-{}", self.threshold, self.max_signers),
            message_digest: hash_bytes(&message),
            group_public_key: randomized_params
                .randomized_verifying_key()
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            key_tweak: Some(key_tweak.to_string()),
            signature: signature
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
        })
    }

    pub fn aggregate_signature_shares(
        &self,
        request: &WithdrawalRequest,
        commitments: &[FrostSigningCommitmentPublic],
        shares: &[FrostSignatureShare],
        key_tweak: Option<&str>,
    ) -> Result<CustodySignature> {
        if shares.len() < self.threshold as usize {
            return Err(Error::Frost(format!(
                "insufficient FROST signature shares: need {}, got {}",
                self.threshold,
                shares.len()
            )));
        }
        let message = withdrawal_signing_message(request)?;
        let signing_package = signing_package_from_commitments(request, commitments)?;
        let signature_shares = shares
            .iter()
            .map(|share| {
                Ok((
                    frost_identifier_from_hex(&share.signer_id)?,
                    frost_signature_share_from_hex(&share.share)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;

        let (scheme, group_public_key, signature) = match key_tweak {
            Some(tweak) => {
                let randomizer = frost_randomizer_from_hex(tweak)?;
                let randomized_params = frost_rerandomized::RandomizedParams::from_randomizer(
                    self.public_key_package.verifying_key(),
                    randomizer,
                );
                let signature = frost_rerandomized::aggregate(
                    &signing_package,
                    &signature_shares,
                    &self.public_key_package,
                    &randomized_params,
                )
                .map_err(frost_error)?;
                randomized_params
                    .randomized_verifying_key()
                    .verify(&message, &signature)
                    .map_err(frost_error)?;
                (
                    "frost-secp256k1-sha256+tweak".to_string(),
                    randomized_params
                        .randomized_verifying_key()
                        .serialize()
                        .map(hex::encode)
                        .map_err(frost_error)?,
                    signature,
                )
            }
            None => {
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
                (
                    "frost-secp256k1-sha256".to_string(),
                    self.group_public_key_hex()?,
                    signature,
                )
            }
        };

        Ok(CustodySignature {
            scheme,
            signer: format!("frost-{}-of-{}", self.threshold, self.max_signers),
            message_digest: hash_bytes(&message),
            group_public_key,
            key_tweak: key_tweak.map(str::to_string),
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

    pub fn child_group_public_key_hex(&self, key_tweak: &str) -> Result<String> {
        let randomizer = frost_randomizer_from_hex(key_tweak)?;
        let randomized_params = frost_rerandomized::RandomizedParams::from_randomizer(
            self.public_key_package.verifying_key(),
            randomizer,
        );
        randomized_params
            .randomized_verifying_key()
            .serialize()
            .map(hex::encode)
            .map_err(frost_error)
    }
}

impl FrostCustodySigner {
    pub fn dkg_round1(
        participant_index: u16,
        max_signers: u16,
        threshold: u16,
    ) -> Result<FrostDkgRound1Output> {
        let mut rng = rand_core::OsRng;
        let identifier = frost_identifier_from_index(participant_index)?;
        let (secret_package, package) =
            frost_secp256k1::keys::dkg::part1(identifier, max_signers, threshold, &mut rng)
                .map_err(frost_error)?;
        let taproot_identifier = frost_tr_identifier_from_index(participant_index)?;
        let (taproot_secret_package, taproot_package) = frost_secp256k1_tr::keys::dkg::part1(
            taproot_identifier,
            max_signers,
            threshold,
            &mut rng,
        )
        .map_err(frost_tr_error)?;
        Ok(FrostDkgRound1Output {
            signer_id: hex::encode(identifier.serialize()),
            secret_package: secret_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            package: package.serialize().map(hex::encode).map_err(frost_error)?,
            taproot_secret_package: taproot_secret_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_tr_error)?,
            taproot_package: taproot_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_tr_error)?,
        })
    }

    pub fn dkg_round2(
        signer_id: &str,
        secret_package: &str,
        taproot_secret_package: &str,
        round1_packages: &[FrostDkgRound1Public],
    ) -> Result<FrostDkgRound2Output> {
        let identifier = frost_identifier_from_hex(signer_id)?;
        let frost_secret_package = frost_dkg_round1_secret_from_hex(secret_package)?;
        let taproot_round1_secret_package =
            frost_tr_dkg_round1_secret_from_hex(taproot_secret_package)?;
        let frost_round1_packages = round1_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                Ok((
                    frost_identifier_from_hex(&package.signer_id)?,
                    frost_dkg_round1_package_from_hex(&package.package)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let (secret_package, packages) =
            frost_secp256k1::keys::dkg::part2(frost_secret_package, &frost_round1_packages)
                .map_err(frost_error)?;
        let taproot_round1_packages = round1_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                Ok((
                    frost_tr_identifier_from_hex(&package.signer_id)?,
                    frost_tr_dkg_round1_package_from_hex(&package.taproot_package)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let (taproot_secret_package, taproot_packages) = frost_secp256k1_tr::keys::dkg::part2(
            taproot_round1_secret_package,
            &taproot_round1_packages,
        )
        .map_err(frost_tr_error)?;
        let packages = packages
            .into_iter()
            .map(|(receiver, package)| {
                Ok((
                    hex::encode(receiver.serialize()),
                    package.serialize().map(hex::encode).map_err(frost_error)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let taproot_packages = taproot_packages
            .into_iter()
            .map(|(receiver, package)| {
                Ok((
                    hex::encode(receiver.serialize()),
                    package
                        .serialize()
                        .map(hex::encode)
                        .map_err(frost_tr_error)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        Ok(FrostDkgRound2Output {
            signer_id: hex::encode(identifier.serialize()),
            secret_package: secret_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            packages,
            taproot_secret_package: taproot_secret_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_tr_error)?,
            taproot_packages,
        })
    }

    pub fn dkg_finalize_single(
        signer_id: &str,
        secret_package: &str,
        taproot_secret_package: &str,
        round1_packages: &[FrostDkgRound1Public],
        round2_packages: &[FrostDkgRound2Public],
    ) -> Result<Self> {
        let identifier = frost_identifier_from_hex(signer_id)?;
        let frost_secret_package = frost_dkg_round2_secret_from_hex(secret_package)?;
        let taproot_secret_package = frost_tr_dkg_round2_secret_from_hex(taproot_secret_package)?;
        let frost_round1_packages = round1_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                Ok((
                    frost_identifier_from_hex(&package.signer_id)?,
                    frost_dkg_round1_package_from_hex(&package.package)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let frost_round2_packages = round2_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                let sender = frost_identifier_from_hex(&package.signer_id)?;
                let package = package
                    .packages
                    .get(signer_id)
                    .ok_or_else(|| {
                        Error::Frost(format!(
                            "missing DKG round2 package from {} to {}",
                            package.signer_id, signer_id
                        ))
                    })
                    .and_then(|package| frost_dkg_round2_package_from_hex(package))?;
                Ok((sender, package))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let (key_package, public_key_package) = frost_secp256k1::keys::dkg::part3(
            &frost_secret_package,
            &frost_round1_packages,
            &frost_round2_packages,
        )
        .map_err(frost_error)?;
        let taproot_round1_packages = round1_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                Ok((
                    frost_tr_identifier_from_hex(&package.signer_id)?,
                    frost_tr_dkg_round1_package_from_hex(&package.taproot_package)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let taproot_round2_packages = round2_packages
            .iter()
            .filter(|package| package.signer_id != signer_id)
            .map(|package| {
                let sender = frost_tr_identifier_from_hex(&package.signer_id)?;
                let package = package
                    .taproot_packages
                    .get(signer_id)
                    .ok_or_else(|| {
                        Error::Frost(format!(
                            "missing Taproot DKG round2 package from {} to {}",
                            package.signer_id, signer_id
                        ))
                    })
                    .and_then(|package| frost_tr_dkg_round2_package_from_hex(package))?;
                Ok((sender, package))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let (taproot_key_package, taproot_public_key_package) =
            frost_secp256k1_tr::keys::dkg::part3(
                &taproot_secret_package,
                &taproot_round1_packages,
                &taproot_round2_packages,
            )
            .map_err(frost_tr_error)?;
        Ok(Self {
            key_packages: BTreeMap::from([(identifier, key_package)]),
            public_key_package,
            taproot_key_packages: BTreeMap::from([(
                frost_tr_identifier_from_hex(signer_id)?,
                taproot_key_package,
            )]),
            taproot_public_key_package: Some(taproot_public_key_package),
            threshold: *frost_secret_package.min_signers(),
        })
    }

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
            taproot_key_packages: BTreeMap::new(),
            taproot_public_key_package: None,
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

    pub fn signer_ids(&self) -> Vec<String> {
        self.key_packages
            .keys()
            .map(|identifier| hex::encode(identifier.serialize()))
            .collect()
    }

    pub fn first_signer_id(&self) -> Result<String> {
        self.signer_ids()
            .into_iter()
            .next()
            .ok_or_else(|| Error::Frost("FROST signer has no key packages".to_string()))
    }

    pub fn signing_commitment(&self, signer_id: &str) -> Result<FrostSigningCommitment> {
        let signer = self.signer_node(signer_id)?;
        let round = signer.round1_commit();
        Ok(FrostSigningCommitment {
            signer_id: signer_id.to_string(),
            commitment: round
                .commitments
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            nonces: round
                .nonces
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
        })
    }

    pub fn signature_share(
        &self,
        signer_id: &str,
        nonces: &str,
        request: &WithdrawalRequest,
        commitments: &[FrostSigningCommitmentPublic],
        key_tweak: Option<&str>,
    ) -> Result<FrostSignatureShare> {
        let signer = self.signer_node(signer_id)?;
        let nonces = frost_nonces_from_hex(nonces)?;
        let signing_package = signing_package_from_commitments(request, commitments)?;
        let share = match key_tweak {
            Some(tweak) => signer.sign_share_with_tweak(&signing_package, &nonces, tweak)?,
            None => signer.sign_share(&signing_package, &nonces)?,
        };
        Ok(FrostSignatureShare {
            signer_id: signer_id.to_string(),
            share: hex::encode(share.serialize()),
        })
    }

    pub fn taproot_signing_commitment(&self, signer_id: &str) -> Result<FrostSigningCommitment> {
        let signer = self.taproot_signer_node(signer_id)?;
        let round = signer.round1_commit();
        Ok(FrostSigningCommitment {
            signer_id: signer_id.to_string(),
            commitment: round
                .commitments
                .serialize()
                .map(hex::encode)
                .map_err(frost_tr_error)?,
            nonces: round
                .nonces
                .serialize()
                .map(hex::encode)
                .map_err(frost_tr_error)?,
        })
    }

    pub fn taproot_signature_share(
        &self,
        signer_id: &str,
        nonces: &str,
        message: &[u8],
        commitments: &[FrostSigningCommitmentPublic],
        merkle_root: Option<&[u8]>,
    ) -> Result<FrostSignatureShare> {
        let signer = self.taproot_signer_node(signer_id)?;
        let nonces = frost_tr_nonces_from_hex(nonces)?;
        let signing_package = taproot_signing_package_from_commitments(message, commitments)?;
        let share = signer.sign_share(&signing_package, &nonces, merkle_root)?;
        Ok(FrostSignatureShare {
            signer_id: signer_id.to_string(),
            share: hex::encode(share.serialize()),
        })
    }

    pub fn aggregate_taproot_signature_shares(
        &self,
        message: &[u8],
        commitments: &[FrostSigningCommitmentPublic],
        shares: &[FrostSignatureShare],
        merkle_root: Option<&[u8]>,
    ) -> Result<String> {
        if shares.len() < self.threshold as usize {
            return Err(Error::Frost(format!(
                "insufficient Taproot FROST signature shares: need {}, got {}",
                self.threshold,
                shares.len()
            )));
        }
        let public_key_package = self
            .taproot_public_key_package
            .as_ref()
            .ok_or_else(|| Error::Frost("missing Taproot FROST public key package".to_string()))?;
        let signing_package = taproot_signing_package_from_commitments(message, commitments)?;
        let signature_shares = shares
            .iter()
            .map(|share| {
                Ok((
                    frost_tr_identifier_from_hex(&share.signer_id)?,
                    frost_tr_signature_share_from_hex(&share.share)?,
                ))
            })
            .collect::<Result<BTreeMap<_, _>>>()?;
        let signature = frost_secp256k1_tr::aggregate_with_tweak(
            &signing_package,
            &signature_shares,
            public_key_package,
            merkle_root,
        )
        .map_err(frost_tr_error)?;
        public_key_package
            .clone()
            .tweak(merkle_root)
            .verifying_key()
            .verify(message, &signature)
            .map_err(frost_tr_error)?;
        signature
            .serialize()
            .map(hex::encode)
            .map_err(frost_tr_error)
    }

    fn signer_node(&self, signer_id: &str) -> Result<FrostSignerNode> {
        let identifier = frost_identifier_from_hex(signer_id)?;
        let key_package = self
            .key_packages
            .get(&identifier)
            .ok_or_else(|| Error::Frost(format!("unknown FROST signer id {signer_id}")))?;
        Ok(FrostSignerNode::new(identifier, key_package.clone()))
    }

    fn taproot_signer_node(&self, signer_id: &str) -> Result<TaprootFrostSignerNode> {
        let identifier = frost_tr_identifier_from_hex(signer_id)?;
        let key_package = self
            .taproot_key_packages
            .get(&identifier)
            .ok_or_else(|| Error::Frost(format!("unknown Taproot FROST signer id {signer_id}")))?;
        Ok(TaprootFrostSignerNode::new(identifier, key_package.clone()))
    }

    pub fn coordinator(&self) -> FrostSigningCoordinator {
        FrostSigningCoordinator {
            public_key_package: self.public_key_package.clone(),
            threshold: self.threshold,
            max_signers: self.public_key_package.verifying_shares().len() as u16,
        }
    }

    pub fn to_keyset(&self, epoch: u64) -> Result<FrostKeyset> {
        Ok(FrostKeyset {
            epoch,
            threshold: self.threshold,
            max_signers: self.public_key_package.verifying_shares().len() as u16,
            group_public_key: self.group_public_key_hex()?,
            public_key_package: self
                .public_key_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            taproot_group_public_key: self.taproot_group_public_key_hex().ok(),
            taproot_public_key_package: self
                .taproot_public_key_package
                .as_ref()
                .map(|package| package.serialize().map(hex::encode).map_err(frost_tr_error))
                .transpose()?,
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

    pub fn taproot_group_public_key_hex(&self) -> Result<String> {
        let package = self
            .taproot_public_key_package
            .as_ref()
            .ok_or_else(|| Error::Frost("missing Taproot FROST public key package".to_string()))?;
        package
            .verifying_key()
            .serialize()
            .map(hex::encode)
            .map_err(frost_tr_error)
    }

    pub fn to_snapshot(&self) -> Result<FrostCustodySignerSnapshot> {
        let key_packages = self
            .key_packages
            .iter()
            .map(|(identifier, key_package)| {
                Ok(FrostKeyPackageSnapshot {
                    identifier: hex::encode(identifier.serialize()),
                    key_package: key_package
                        .serialize()
                        .map(hex::encode)
                        .map_err(frost_error)?,
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let taproot_key_packages = self
            .taproot_key_packages
            .iter()
            .map(|(identifier, key_package)| {
                Ok(FrostKeyPackageSnapshot {
                    identifier: hex::encode(identifier.serialize()),
                    key_package: key_package
                        .serialize()
                        .map(hex::encode)
                        .map_err(frost_tr_error)?,
                })
            })
            .collect::<Result<Vec<_>>>()?;
        Ok(FrostCustodySignerSnapshot {
            version: 1,
            threshold: self.threshold,
            public_key_package: self
                .public_key_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            key_packages,
            taproot_public_key_package: self
                .taproot_public_key_package
                .as_ref()
                .map(|package| package.serialize().map(hex::encode).map_err(frost_tr_error))
                .transpose()?,
            taproot_key_packages,
        })
    }

    pub fn snapshot_for_signer_id(&self, signer_id: &str) -> Result<FrostCustodySignerSnapshot> {
        let identifier = frost_identifier_from_hex(signer_id)?;
        let key_package = self
            .key_packages
            .get(&identifier)
            .ok_or_else(|| Error::Frost(format!("unknown FROST signer id {signer_id}")))?;
        Ok(FrostCustodySignerSnapshot {
            version: 1,
            threshold: self.threshold,
            public_key_package: self
                .public_key_package
                .serialize()
                .map(hex::encode)
                .map_err(frost_error)?,
            key_packages: vec![FrostKeyPackageSnapshot {
                identifier: signer_id.to_string(),
                key_package: key_package
                    .serialize()
                    .map(hex::encode)
                    .map_err(frost_error)?,
            }],
            taproot_public_key_package: self
                .taproot_public_key_package
                .as_ref()
                .map(|package| package.serialize().map(hex::encode).map_err(frost_tr_error))
                .transpose()?,
            taproot_key_packages: self
                .taproot_key_packages
                .get(&frost_tr_identifier_from_hex(signer_id)?)
                .map(|key_package| {
                    Ok(FrostKeyPackageSnapshot {
                        identifier: signer_id.to_string(),
                        key_package: key_package
                            .serialize()
                            .map(hex::encode)
                            .map_err(frost_tr_error)?,
                    })
                })
                .transpose()?
                .into_iter()
                .collect(),
        })
    }

    pub fn from_snapshot(snapshot: &FrostCustodySignerSnapshot) -> Result<Self> {
        if snapshot.version != 1 {
            return Err(Error::Frost(format!(
                "unsupported FROST signer snapshot version {}",
                snapshot.version
            )));
        }
        let public_key_package_bytes = hex::decode(&snapshot.public_key_package)
            .map_err(|e| Error::Frost(format!("invalid public key package hex: {e}")))?;
        let public_key_package =
            frost_secp256k1::keys::PublicKeyPackage::deserialize(&public_key_package_bytes)
                .map_err(frost_error)?;
        let mut key_packages = BTreeMap::new();
        for key in &snapshot.key_packages {
            let identifier_bytes = hex::decode(&key.identifier)
                .map_err(|e| Error::Frost(format!("invalid key identifier hex: {e}")))?;
            let identifier =
                frost_secp256k1::Identifier::deserialize(&identifier_bytes).map_err(frost_error)?;
            let key_package_bytes = hex::decode(&key.key_package)
                .map_err(|e| Error::Frost(format!("invalid key package hex: {e}")))?;
            let key_package = frost_secp256k1::keys::KeyPackage::deserialize(&key_package_bytes)
                .map_err(frost_error)?;
            key_packages.insert(identifier, key_package);
        }
        let taproot_public_key_package = snapshot
            .taproot_public_key_package
            .as_ref()
            .map(|package| {
                let bytes = hex::decode(package).map_err(|e| {
                    Error::Frost(format!("invalid Taproot public key package hex: {e}"))
                })?;
                frost_secp256k1_tr::keys::PublicKeyPackage::deserialize(&bytes)
                    .map_err(frost_tr_error)
            })
            .transpose()?;
        let mut taproot_key_packages = BTreeMap::new();
        for key in &snapshot.taproot_key_packages {
            let identifier = frost_tr_identifier_from_hex(&key.identifier)?;
            let key_package_bytes = hex::decode(&key.key_package)
                .map_err(|e| Error::Frost(format!("invalid Taproot key package hex: {e}")))?;
            let key_package = frost_secp256k1_tr::keys::KeyPackage::deserialize(&key_package_bytes)
                .map_err(frost_tr_error)?;
            taproot_key_packages.insert(identifier, key_package);
        }
        Ok(Self {
            key_packages,
            public_key_package,
            taproot_key_packages,
            taproot_public_key_package,
            threshold: snapshot.threshold,
        })
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
            if state
                .deposits
                .intents
                .get(&intent_id)
                .is_some_and(|intent| intent.txid.is_some())
            {
                let credit_id = format!("dep-{}", state.next_deposit_id);
                let credit_event = Event::DepositCreditCreated {
                    credit_id: credit_id.clone(),
                    source_intent_id: intent_id,
                };
                apply_event(state, credit_event.clone())?;
                let observed_event = Event::DepositObserved {
                    intent_id: credit_id,
                    txid,
                    amount_sats,
                };
                apply_event(state, observed_event.clone())?;
                return Ok(vec![credit_event, observed_event]);
            }
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
        Command::ApplyDepositRetirements => {
            let events = deposit_retirement_events(state);
            for event in events.clone() {
                apply_event(state, event)?;
            }
            Ok(events)
        }
        Command::WithdrawNote { proof, public } => {
            withdraw_note(state, proof, public, verifier, signer)
        }
        Command::RequestWithdrawal { proof, public } => {
            request_withdrawal(state, proof, public, verifier)
        }
        Command::AuthorizeWithdrawal {
            withdrawal_id,
            signature,
        } => authorize_pending_withdrawal(state, &withdrawal_id, signature),
        Command::RegisterNode {
            node_id,
            bond_address,
            consensus_pubkey,
            signer_pubkey,
        } => {
            if state.churn.node_accounts.contains_key(&node_id) {
                return Err(Error::NodeAlreadyExists);
            }
            let event = Event::NodeRegistered {
                node: NodeAccount {
                    node_id,
                    status: NodeStatus::Standby,
                    bond_sats: 0,
                    bond_address,
                    slot_id: None,
                    consensus_pubkey,
                    signer_pubkey,
                    status_since_epoch: state.churn.epoch,
                    active_since_epoch: None,
                    slash_points: 0,
                    missed_observations: 0,
                    missed_keysigns: 0,
                    requested_leave: false,
                    forced_leave: false,
                },
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::BondNode {
            node_id,
            amount_sats,
        } => {
            let node = state
                .churn
                .node_accounts
                .get(&node_id)
                .ok_or(Error::NodeNotFound)?;
            let total_bond_sats = node.bond_sats.saturating_add(amount_sats);
            let event = Event::NodeBonded {
                node_id,
                amount_sats,
                total_bond_sats,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::AssignNodeSlot { node_id, slot_id } => {
            let node = state
                .churn
                .node_accounts
                .get(&node_id)
                .ok_or(Error::NodeNotFound)?;
            if state.churn.node_slots.contains_key(&slot_id) || node.slot_id.is_some() {
                return Err(Error::NodeSlotAlreadyAssigned);
            }
            let required_bond_sats = required_node_bond_sats_for_state(state, slot_id);
            if node.bond_sats < required_bond_sats {
                return Err(Error::InsufficientNodeBond);
            }
            let event = Event::NodeSlotAssigned {
                node_id,
                slot_id,
                required_bond_sats,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::SetBondParameters {
            min_bond_sats,
            min_bond_increase_sats,
        } => {
            let event = Event::BondParametersUpdated {
                min_bond_sats,
                min_bond_increase_sats,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::VoteNetworkParameters {
            node_id,
            churn_cycle_ms,
            target_active_nodes,
            max_nodes_per_churn,
            bitcoin_keysign_grace_epochs,
            bitcoin_attestation_grace_epochs,
        } => {
            let churn_cycle_ms = churn_cycle_ms.unwrap_or(state.churn.churn_cycle_ms);
            let target_active_nodes =
                target_active_nodes.unwrap_or(state.churn.target_active_nodes);
            let max_nodes_per_churn =
                max_nodes_per_churn.unwrap_or(state.churn.max_nodes_per_churn);
            let bitcoin_keysign_grace_epochs =
                bitcoin_keysign_grace_epochs.unwrap_or(state.churn.bitcoin_keysign_grace_epochs);
            let bitcoin_attestation_grace_epochs = bitcoin_attestation_grace_epochs
                .unwrap_or(state.churn.bitcoin_attestation_grace_epochs);
            if churn_cycle_ms == 0 {
                return Err(Error::InvalidNetworkParameter);
            }
            if target_active_nodes == 0
                || max_nodes_per_churn == 0
                || bitcoin_keysign_grace_epochs == 0
                || bitcoin_attestation_grace_epochs == 0
            {
                return Err(Error::InvalidNetworkParameter);
            }
            if !state.churn.active_nodes.contains(&node_id) {
                return Err(Error::NodeNotActive);
            }
            let vote = Event::NetworkParameterVoteCast {
                node_id,
                churn_cycle_ms,
                target_active_nodes,
                max_nodes_per_churn,
                bitcoin_keysign_grace_epochs,
                bitcoin_attestation_grace_epochs,
                epoch: state.churn.epoch,
            };
            apply_event(state, vote.clone())?;
            let mut events = vec![vote];
            if network_parameter_vote_count(
                state,
                state.churn.epoch,
                churn_cycle_ms,
                target_active_nodes,
                max_nodes_per_churn,
                bitcoin_keysign_grace_epochs,
                bitcoin_attestation_grace_epochs,
            ) >= active_node_quorum(state)
                && (state.churn.churn_cycle_ms != churn_cycle_ms
                    || state.churn.target_active_nodes != target_active_nodes
                    || state.churn.max_nodes_per_churn != max_nodes_per_churn
                    || state.churn.bitcoin_keysign_grace_epochs != bitcoin_keysign_grace_epochs
                    || state.churn.bitcoin_attestation_grace_epochs
                        != bitcoin_attestation_grace_epochs)
            {
                let update = Event::NetworkParametersUpdated {
                    churn_cycle_ms,
                    target_active_nodes,
                    max_nodes_per_churn,
                    bitcoin_keysign_grace_epochs,
                    bitcoin_attestation_grace_epochs,
                    epoch: state.churn.epoch,
                };
                apply_event(state, update.clone())?;
                events.push(update);
            }
            Ok(events)
        }
        Command::RegisterStandbyNode { node_id } => {
            let event = Event::StandbyNodeRegistered { node_id };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::StartChurnEpoch => {
            let next_epoch = state.churn.epoch + 1;
            let mut events = start_churn_epoch_without_keygen(state)?;
            if let Some(signer_count) = active_keygen_count(state) {
                let keyset = FrostCustodySigner::generate_keyset_with_dkg(
                    next_epoch,
                    signer_count,
                    frost_threshold_for_committee(signer_count),
                )?;
                let keyset_event = Event::CustodyKeysetGenerated {
                    epoch: next_epoch,
                    keyset,
                };
                apply_event(state, keyset_event.clone())?;
                events.push(keyset_event);
            }
            Ok(events)
        }
        Command::CommitCustodyKeyset { epoch, keyset } => {
            validate_keyset_commit(state, epoch, &keyset)?;
            let event = Event::CustodyKeysetGenerated { epoch, keyset };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::SubmitBitcoinBroadcast {
            withdrawal_id,
            txid,
        } => {
            if !state.withdrawals.authorized.contains_key(&withdrawal_id) {
                return Err(Error::UnknownWithdrawal);
            }
            if let Some(existing) = state.withdrawals.bitcoin_broadcasts.get(&withdrawal_id) {
                if existing.txid == txid {
                    return Ok(Vec::new());
                }
                return Err(Error::ConflictingBitcoinAttestation);
            }
            if !bitcoin_withdrawal_has_attestation_quorum(state, &withdrawal_id, &txid) {
                return Err(Error::MissingBitcoinAttestationQuorum);
            }
            let event = Event::BitcoinWithdrawalBroadcastSubmitted {
                withdrawal_id,
                txid,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::SubmitDepositSweep { deposit_id, txid } => {
            let intent = state
                .deposits
                .intents
                .get(&deposit_id)
                .ok_or(Error::DepositNotFound)?;
            if let Some(existing) = intent.swept_txid.as_ref() {
                if existing == &txid {
                    return Ok(Vec::new());
                }
                return Err(Error::ConflictingBitcoinAttestation);
            }
            if !intent.confirmed {
                return Err(Error::DepositNotConfirmed);
            }
            let event = Event::BitcoinDepositSweepSubmitted { deposit_id, txid };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::SubmitVaultSweep {
            from_epoch,
            to_epoch,
            txid,
        } => {
            if from_epoch >= to_epoch || to_epoch != state.custody.active_epoch {
                return Err(Error::InvalidProof);
            }
            let vault = state
                .custody
                .vaults
                .get(&from_epoch)
                .ok_or(Error::InvalidProof)?;
            if let Some(existing) = vault.sweep_txid.as_ref() {
                if existing == &txid {
                    return Ok(Vec::new());
                }
                return Err(Error::ConflictingBitcoinAttestation);
            }
            let event = Event::BitcoinVaultSweepSubmitted {
                from_epoch,
                to_epoch,
                txid,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::AttestBitcoinWithdrawal {
            withdrawal_id,
            txid,
            signed_tx_hash,
            attester,
            epoch,
        } => {
            validate_bitcoin_attestation(state, &withdrawal_id, &attester, epoch)?;
            let event = Event::BitcoinWithdrawalAttested {
                withdrawal_id,
                txid,
                signed_tx_hash,
                attester,
                epoch,
            };
            apply_event(state, event.clone())?;
            Ok(vec![event])
        }
        Command::ApplyBitcoinOutboundPenalties => {
            let mut events = Vec::new();
            let current_epoch = state.churn.epoch;
            let outbounds = state
                .withdrawals
                .bitcoin_outbounds
                .values()
                .cloned()
                .collect::<Vec<_>>();
            for outbound in outbounds {
                let attested = state
                    .withdrawals
                    .bitcoin_attestations
                    .get(&outbound.withdrawal_id)
                    .map(|set| set.attestations.keys().cloned().collect::<BTreeSet<_>>())
                    .unwrap_or_default();
                if current_epoch > outbound.signing_deadline_epoch {
                    for node_id in outbound.signers.iter() {
                        if attested.contains(node_id)
                            || outbound.keysign_penalized.contains(node_id)
                        {
                            continue;
                        }
                        let slash = Event::NodeSlashPointsAdded {
                            node_id: node_id.clone(),
                            points: 1,
                            reason: "missed_bitcoin_keysign".to_string(),
                            epoch: current_epoch,
                        };
                        apply_event(state, slash.clone())?;
                        events.push(slash);
                        let mark = Event::BitcoinOutboundPenaltyMarked {
                            withdrawal_id: outbound.withdrawal_id.clone(),
                            node_id: node_id.clone(),
                            penalty_kind: "keysign".to_string(),
                        };
                        apply_event(state, mark.clone())?;
                        events.push(mark);
                    }
                }
                if current_epoch > outbound.attestation_deadline_epoch {
                    for node_id in outbound.attesters.iter() {
                        if attested.contains(node_id)
                            || outbound.attestation_penalized.contains(node_id)
                        {
                            continue;
                        }
                        let slash = Event::NodeSlashPointsAdded {
                            node_id: node_id.clone(),
                            points: 1,
                            reason: "missed_bitcoin_attestation".to_string(),
                            epoch: current_epoch,
                        };
                        apply_event(state, slash.clone())?;
                        events.push(slash);
                        let mark = Event::BitcoinOutboundPenaltyMarked {
                            withdrawal_id: outbound.withdrawal_id.clone(),
                            node_id: node_id.clone(),
                            penalty_kind: "attestation".to_string(),
                        };
                        apply_event(state, mark.clone())?;
                        events.push(mark);
                    }
                }
            }
            Ok(events)
        }
        Command::SubmitBitcoinSolvency {
            reporter,
            epoch,
            expected_sats,
            actual_sats,
            spendable_sats,
            solvent,
        } => {
            validate_bitcoin_solvency_reporter(state, &reporter, epoch)?;
            let event = Event::BitcoinSolvencyReported {
                reporter,
                expected_sats,
                actual_sats,
                spendable_sats,
                solvent,
                epoch,
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
                if !state.churn.active_nodes.contains(&node_id) {
                    continue;
                }
                if let Some(bond_sats) = state
                    .churn
                    .node_accounts
                    .get(&node_id)
                    .map(|node| node.bond_sats)
                {
                    let points = Event::NodeSlashPointsAdded {
                        node_id: node_id.clone(),
                        points: 1,
                        reason: "offline_churn_cycle".to_string(),
                        epoch: state.churn.epoch,
                    };
                    apply_event(state, points.clone())?;
                    events.push(points);
                    let amount_sats = offline_penalty_sats(bond_sats);
                    if amount_sats > 0 {
                        let slash = Event::NodeBondSlashed {
                            node_id: node_id.clone(),
                            amount_sats,
                            reason: "offline_churn_cycle".to_string(),
                            epoch: state.churn.epoch,
                        };
                        apply_event(state, slash.clone())?;
                        events.push(slash);
                    }
                }
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
            let mut signers = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
            signers.sort();
            state.custody.vaults.entry(epoch).or_insert(CustodyVault {
                epoch,
                signers,
                threshold: keyset.threshold,
                sweep_txid: None,
                swept_to_epoch: None,
            });
            state.custody.keysets.insert(epoch, keyset);
        }
        Event::DepositIntentCreated {
            intent_id,
            deposit_address,
            pow_token,
            user_pubkey,
        } => {
            let child_key = derive_deposit_child_key(state, &intent_id, &pow_token, &user_pubkey)?;
            if child_key.address != deposit_address {
                return Err(Error::Frost(
                    "deposit address does not match active custody child key derivation"
                        .to_string(),
                ));
            }
            let (vault_signers, vault_threshold) = active_vault_membership(state);
            state.deposits.used_pow_tokens.insert(pow_token.clone());
            state.deposits.intents.insert(
                intent_id.clone(),
                DepositIntent {
                    id: intent_id,
                    source_intent_id: None,
                    deposit_address,
                    custody_epoch: child_key.custody_epoch,
                    deposit_key_tweak: child_key.key_tweak,
                    deposit_public_key: child_key.child_public_key,
                    vault_signers,
                    vault_threshold,
                    pow_token,
                    user_pubkey,
                    txid: None,
                    amount_sats: None,
                    confirmed: false,
                    swept_txid: None,
                    split: false,
                    remainder_sats: 0,
                },
            );
            state.next_deposit_id += 1;
        }
        Event::DepositCreditCreated {
            credit_id,
            source_intent_id,
        } => {
            let source = state
                .deposits
                .intents
                .get(&source_intent_id)
                .cloned()
                .ok_or(Error::DepositNotFound)?;
            state.deposits.intents.insert(
                credit_id.clone(),
                DepositIntent {
                    id: credit_id,
                    source_intent_id: Some(source_intent_id),
                    deposit_address: source.deposit_address,
                    custody_epoch: source.custody_epoch,
                    deposit_key_tweak: source.deposit_key_tweak,
                    deposit_public_key: source.deposit_public_key,
                    vault_signers: source.vault_signers,
                    vault_threshold: source.vault_threshold,
                    pow_token: source.pow_token,
                    user_pubkey: source.user_pubkey,
                    txid: None,
                    amount_sats: None,
                    confirmed: false,
                    swept_txid: None,
                    split: false,
                    remainder_sats: 0,
                },
            );
            state.next_deposit_id += 1;
        }
        Event::DepositIntentRetired { intent_id, .. } => {
            state.deposits.intents.remove(&intent_id);
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
        Event::DepositSplitRecorded {
            deposit_id,
            remainder_sats,
        } => {
            let intent = state
                .deposits
                .intents
                .get_mut(&deposit_id)
                .ok_or(Error::DepositNotFound)?;
            intent.split = true;
            intent.remainder_sats = remainder_sats;
        }
        Event::NotesMinted { notes } => {
            for note in notes {
                let tree = state
                    .notes
                    .trees
                    .get_mut(&note.denomination_sats)
                    .ok_or(Error::UnknownDenomination)?;
                tree.insert(note.commitment);
            }
        }
        Event::NoteSpent {
            nullifier_hash,
            denomination_sats: _,
        } => {
            state.notes.spent_nullifiers.insert(nullifier_hash);
        }
        Event::WithdrawalRequested {
            withdrawal_id,
            recipient,
            amount_sats,
            fee_sats,
            nullifier_hash,
            denomination_sats,
            custody_epoch,
            deposit_key_tweak,
            vault_signers,
            vault_threshold,
        } => {
            state.withdrawals.pending.insert(
                withdrawal_id.clone(),
                PendingWithdrawal {
                    id: withdrawal_id,
                    recipient,
                    amount_sats,
                    fee_sats,
                    nullifier_hash,
                    denomination_sats,
                    custody_epoch,
                    deposit_key_tweak,
                    vault_signers,
                    vault_threshold,
                },
            );
            state.next_withdrawal_id += 1;
        }
        Event::WithdrawalAuthorized {
            withdrawal_id,
            recipient,
            amount_sats,
            fee_sats,
            nullifier_hash,
            custody_epoch,
            deposit_key_tweak,
            vault_signers,
            vault_threshold,
            signature,
        } => {
            let (leader, signers, attesters) = bitcoin_outbound_assignments_for_signers(
                state,
                &withdrawal_id,
                state.churn.epoch,
                &vault_signers,
                vault_threshold,
            );
            state.withdrawals.authorized.insert(
                withdrawal_id.clone(),
                AuthorizedWithdrawal {
                    id: withdrawal_id.clone(),
                    recipient: recipient.clone(),
                    amount_sats,
                    fee_sats,
                    nullifier_hash: nullifier_hash.clone(),
                    custody_epoch,
                    deposit_key_tweak: deposit_key_tweak.clone(),
                    vault_signers: vault_signers.clone(),
                    vault_threshold,
                    signature,
                },
            );
            state.withdrawals.bitcoin_outbounds.insert(
                withdrawal_id.clone(),
                BitcoinOutbound {
                    withdrawal_id: withdrawal_id.clone(),
                    recipient,
                    amount_sats,
                    fee_sats,
                    nullifier_hash,
                    custody_epoch,
                    deposit_key_tweak,
                    vault_signers,
                    vault_threshold,
                    scheduled_epoch: state.churn.epoch,
                    leader,
                    signers,
                    attesters,
                    signing_deadline_epoch: state
                        .churn
                        .epoch
                        .saturating_add(state.churn.bitcoin_keysign_grace_epochs),
                    attestation_deadline_epoch: state
                        .churn
                        .epoch
                        .saturating_add(state.churn.bitcoin_attestation_grace_epochs),
                    keysign_penalized: BTreeSet::new(),
                    attestation_penalized: BTreeSet::new(),
                    published_txid: None,
                    published_epoch: None,
                },
            );
            state.withdrawals.pending.remove(&withdrawal_id);
        }
        Event::BitcoinWithdrawalBroadcastSubmitted {
            withdrawal_id,
            txid,
        } => {
            state.withdrawals.bitcoin_broadcasts.insert(
                withdrawal_id.clone(),
                BitcoinBroadcastRecord {
                    withdrawal_id: withdrawal_id.clone(),
                    txid: txid.clone(),
                },
            );
            if let Some(outbound) = state.withdrawals.bitcoin_outbounds.get_mut(&withdrawal_id) {
                outbound.published_txid = Some(txid);
                outbound.published_epoch = Some(state.churn.epoch);
            }
        }
        Event::BitcoinDepositSweepSubmitted { deposit_id, txid } => {
            let intent = state
                .deposits
                .intents
                .get_mut(&deposit_id)
                .ok_or(Error::DepositNotFound)?;
            intent.swept_txid = Some(txid);
        }
        Event::BitcoinVaultSweepSubmitted {
            from_epoch,
            to_epoch,
            txid,
        } => {
            let vault = state
                .custody
                .vaults
                .get_mut(&from_epoch)
                .ok_or(Error::InvalidProof)?;
            vault.sweep_txid = Some(txid);
            vault.swept_to_epoch = Some(to_epoch);
        }
        Event::BitcoinWithdrawalAttested {
            withdrawal_id,
            txid,
            signed_tx_hash,
            attester,
            epoch,
        } => {
            let existing = state
                .withdrawals
                .bitcoin_attestations
                .get(&withdrawal_id)
                .cloned();
            if let Some(existing) = existing.as_ref() {
                if existing.txid != txid
                    || existing.signed_tx_hash != signed_tx_hash
                    || existing.epoch != epoch
                {
                    return Err(Error::ConflictingBitcoinAttestation);
                }
                if let Some(previous) = existing.attestations.get(&attester) {
                    if previous.txid != txid
                        || previous.signed_tx_hash != signed_tx_hash
                        || previous.epoch != epoch
                    {
                        return Err(Error::ConflictingBitcoinAttestation);
                    }
                }
            }
            let set = state
                .withdrawals
                .bitcoin_attestations
                .entry(withdrawal_id.clone())
                .or_insert_with(|| BitcoinWithdrawalAttestationSet {
                    withdrawal_id,
                    txid: txid.clone(),
                    signed_tx_hash: signed_tx_hash.clone(),
                    epoch,
                    attestations: BTreeMap::new(),
                });
            set.attestations.insert(
                attester.clone(),
                BitcoinWithdrawalAttestation {
                    attester,
                    txid,
                    signed_tx_hash,
                    epoch,
                },
            );
        }
        Event::BitcoinSolvencyReported {
            reporter,
            expected_sats,
            actual_sats,
            spendable_sats,
            solvent,
            epoch,
        } => {
            let record = BitcoinSolvencyRecord {
                reporter: reporter.clone(),
                expected_sats,
                actual_sats,
                spendable_sats,
                solvent,
                epoch,
            };
            state.bitcoin_solvency.latest = Some(record.clone());
            state
                .bitcoin_solvency
                .reports_by_epoch
                .entry(epoch)
                .or_default()
                .insert(reporter, record);
            let quorum = bitcoin_solvency_quorum(state);
            let reports = state
                .bitcoin_solvency
                .reports_by_epoch
                .get(&epoch)
                .map(BTreeMap::values)
                .into_iter()
                .flatten()
                .collect::<Vec<_>>();
            let insolvent_votes = reports.iter().filter(|report| !report.solvent).count();
            let solvent_votes = reports.iter().filter(|report| report.solvent).count();
            if insolvent_votes >= quorum {
                state.bitcoin_solvency.halted = true;
            } else if solvent_votes >= quorum {
                state.bitcoin_solvency.halted = false;
            }
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
        Event::NodeRegistered { node } => {
            state.churn.node_accounts.insert(node.node_id.clone(), node);
        }
        Event::NodeBonded {
            node_id,
            total_bond_sats,
            ..
        } => {
            let node = state
                .churn
                .node_accounts
                .get_mut(&node_id)
                .ok_or(Error::NodeNotFound)?;
            node.bond_sats = total_bond_sats;
        }
        Event::NodeSlotAssigned {
            node_id,
            slot_id,
            required_bond_sats,
        } => {
            let node = state
                .churn
                .node_accounts
                .get_mut(&node_id)
                .ok_or(Error::NodeNotFound)?;
            node.slot_id = Some(slot_id);
            state.churn.node_slots.insert(
                slot_id,
                NodeSlot {
                    slot_id,
                    owner_node_id: node_id,
                    required_bond_sats,
                },
            );
        }
        Event::BondParametersUpdated {
            min_bond_sats,
            min_bond_increase_sats,
        } => {
            state.churn.min_bond_sats = min_bond_sats;
            state.churn.min_bond_increase_sats = min_bond_increase_sats;
        }
        Event::NetworkParameterVoteCast {
            node_id,
            churn_cycle_ms,
            target_active_nodes,
            max_nodes_per_churn,
            bitcoin_keysign_grace_epochs,
            bitcoin_attestation_grace_epochs,
            epoch,
        } => {
            state
                .churn
                .network_parameter_votes
                .entry(epoch)
                .or_default()
                .insert(
                    node_id.clone(),
                    NetworkParameterVote {
                        node_id,
                        churn_cycle_ms,
                        target_active_nodes,
                        max_nodes_per_churn,
                        bitcoin_keysign_grace_epochs,
                        bitcoin_attestation_grace_epochs,
                        epoch,
                    },
                );
        }
        Event::NetworkParametersUpdated {
            churn_cycle_ms,
            target_active_nodes,
            max_nodes_per_churn,
            bitcoin_keysign_grace_epochs,
            bitcoin_attestation_grace_epochs,
            epoch: _,
        } => {
            state.churn.churn_cycle_ms = churn_cycle_ms;
            state.churn.target_active_nodes = target_active_nodes;
            state.churn.max_nodes_per_churn = max_nodes_per_churn;
            state.churn.bitcoin_keysign_grace_epochs = bitcoin_keysign_grace_epochs;
            state.churn.bitcoin_attestation_grace_epochs = bitcoin_attestation_grace_epochs;
        }
        Event::NodeStatusUpdated {
            node_id,
            status,
            epoch,
        } => {
            update_node_status(state, &node_id, status, epoch)?;
        }
        Event::StandbyNodeRegistered { node_id } => {
            if !state.churn.active_nodes.contains(&node_id) {
                state.churn.standby_nodes.insert(node_id);
            }
        }
        Event::StandbyNodeActivated { node_id, .. } => {
            state.churn.standby_nodes.remove(&node_id);
            state.churn.active_nodes.insert(node_id.clone());
            if let Some(node) = state.churn.node_accounts.get_mut(&node_id) {
                node.status = NodeStatus::Active;
                node.status_since_epoch = state.churn.epoch;
                node.active_since_epoch = Some(state.churn.epoch);
                node.missed_keysigns = 0;
                node.missed_observations = 0;
            }
        }
        Event::NodeMarkedOffline { node_id, epoch } => {
            state
                .churn
                .offline_nodes
                .entry(epoch)
                .or_default()
                .insert(node_id);
        }
        Event::NodeSlashPointsAdded {
            node_id,
            points,
            reason,
            ..
        } => {
            let node = state
                .churn
                .node_accounts
                .get_mut(&node_id)
                .ok_or(Error::NodeNotFound)?;
            node.slash_points = node.slash_points.saturating_add(points);
            match reason.as_str() {
                "missed_bitcoin_keysign" => {
                    node.missed_keysigns = node.missed_keysigns.saturating_add(points);
                }
                "missed_bitcoin_attestation" => {
                    node.missed_observations = node.missed_observations.saturating_add(points);
                }
                _ => {
                    node.missed_observations = node.missed_observations.saturating_add(points);
                    node.missed_keysigns = node.missed_keysigns.saturating_add(points);
                }
            }
        }
        Event::BitcoinOutboundPenaltyMarked {
            withdrawal_id,
            node_id,
            penalty_kind,
        } => {
            if let Some(outbound) = state.withdrawals.bitcoin_outbounds.get_mut(&withdrawal_id) {
                match penalty_kind.as_str() {
                    "keysign" => {
                        outbound.keysign_penalized.insert(node_id);
                    }
                    "attestation" => {
                        outbound.attestation_penalized.insert(node_id);
                    }
                    _ => {}
                }
            }
        }
        Event::NodeBondSlashed {
            node_id,
            amount_sats,
            ..
        } => {
            let node = state
                .churn
                .node_accounts
                .get_mut(&node_id)
                .ok_or(Error::NodeNotFound)?;
            node.bond_sats = node.bond_sats.saturating_sub(amount_sats);
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

pub fn note_owner_secret(client_seed: &str, deposit_id: &str, index: u64) -> String {
    let _ = (deposit_id, index);
    hash_parts(&[DOMAIN, "receipt-owner-secret", client_seed])
}

pub fn note_owner_pubkey(owner_secret: &str) -> String {
    hash_parts(&[DOMAIN, "receipt-owner-pubkey", owner_secret])
}

pub fn client_pubkey_from_secret(client_seed: &str) -> String {
    note_owner_pubkey(&note_owner_secret(client_seed, "", 0))
}

pub fn note_commitment(
    nullifier: &str,
    secret: &str,
    denomination_sats: u64,
    owner_pubkey: &str,
) -> String {
    hash_parts(&[
        DOMAIN,
        "note-commitment",
        nullifier,
        secret,
        &denomination_sats.to_string(),
        owner_pubkey,
    ])
}

pub fn nullifier_hash(nullifier: &str) -> String {
    hash_parts(&[DOMAIN, "nullifier-hash", nullifier])
}

pub fn owned_nullifier_hash(nullifier: &str, owner_secret: &str) -> String {
    hash_parts(&[DOMAIN, "owned-nullifier-hash", nullifier, owner_secret])
}

pub fn merkle_root(leaves: &[String]) -> String {
    #[cfg(feature = "orchard-zcash")]
    {
        return orchard::merkle_root_hex(leaves).unwrap_or_default();
    }
    #[cfg(not(feature = "orchard-zcash"))]
    {
        let leaves = serde_json::to_string(leaves).unwrap_or_default();
        hash_parts(&[DOMAIN, "disabled-zk-merkle-root", &leaves])
    }
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
        #[cfg(feature = "orchard-zcash")]
        orchard: None,
    };
    let public = WithdrawalPublicInputs {
        nullifier_hash: nullifier_hash(&receipt.nullifier),
        owner_pubkey: receipt.owner_pubkey.clone(),
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

pub fn zk_withdrawal_from_receipt(
    receipt: &NoteReceipt,
    client_seed: &str,
    tree: &DenominationTree,
    recipient: String,
    fee_sats: u64,
) -> Result<(WithdrawalProof, WithdrawalPublicInputs)> {
    #[cfg(feature = "orchard-zcash")]
    {
        let orchard_note = receipt.orchard.as_ref().ok_or(Error::InvalidProof)?;
        let anchor = orchard::merkle_root_hex(&tree.leaves)?;
        let context_public = WithdrawalPublicInputs {
            nullifier_hash: String::new(),
            owner_pubkey: receipt.owner_pubkey.clone(),
            denomination_sats: receipt.denomination_sats,
            recipient,
            fee_sats,
            merkle_root: anchor.clone(),
            recipient_field: None,
            relayer_field: None,
            refund_field: None,
        };
        let public_context = orchard_public_context(&context_public);
        let (orchard_proof, nullifier_hash, merkle_root) = orchard::prove_orchard_withdrawal(
            client_seed,
            orchard_note,
            &tree.leaves,
            &receipt.commitment,
            &public_context,
        )?;
        if merkle_root != anchor {
            return Err(Error::InvalidProof);
        }
        let public = WithdrawalPublicInputs {
            nullifier_hash,
            owner_pubkey: receipt.owner_pubkey.clone(),
            denomination_sats: receipt.denomination_sats,
            recipient: context_public.recipient,
            fee_sats,
            merkle_root: merkle_root.clone(),
            recipient_field: None,
            relayer_field: None,
            refund_field: None,
        };
        let proof = WithdrawalProof {
            nullifier: String::new(),
            secret: String::new(),
            commitment: String::new(),
            merkle_root,
            orchard: Some(orchard_proof),
        };
        return Ok((proof, public));
    }
    #[cfg(not(feature = "orchard-zcash"))]
    {
        let _ = (receipt, client_seed, tree, recipient, fee_sats);
        Err(Error::InvalidProof)
    }
}

#[cfg(feature = "orchard-zcash")]
pub(crate) fn orchard_public_context(public: &WithdrawalPublicInputs) -> Vec<u8> {
    hash_parts_bytes(&[
        DOMAIN,
        "orchard-withdrawal",
        &public.recipient,
        &public.fee_sats.to_string(),
        &public.denomination_sats.to_string(),
        &public.merkle_root,
    ])
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
    Ok(derive_deposit_child_key(state, intent_id, pow_token, user_pubkey)?.address)
}

pub fn derive_deposit_child_key(
    state: &AppState,
    intent_id: &str,
    pow_token: &str,
    user_pubkey: &str,
) -> Result<DepositChildKey> {
    let keyset = state
        .custody
        .keysets
        .get(&state.custody.active_epoch)
        .ok_or_else(|| Error::Frost("missing active custody keyset".to_string()))?;
    let public_key = keyset
        .taproot_group_public_key
        .as_deref()
        .or_else(|| active_custody_public_key(state))
        .ok_or_else(|| Error::Frost("missing active custody public key".to_string()))?;
    let key_tweak = derive_deposit_key_tweak(
        DOMAIN,
        "deposit-address",
        state.custody.active_epoch,
        public_key,
        user_pubkey,
        intent_id,
        pow_token,
    )?;
    let (address, child_public_key) = if keyset.taproot_group_public_key.is_some() {
        (
            taproot_address_from_internal_key_with_hidden_root(public_key, &key_tweak)?,
            public_key.to_string(),
        )
    } else {
        let coordinator = FrostSigningCoordinator::new(keyset)?;
        let child_public_key = coordinator.child_group_public_key_hex(&key_tweak)?;
        (
            taproot_address_from_group_public_key(&child_public_key)?,
            child_public_key,
        )
    };
    Ok(DepositChildKey {
        address,
        custody_epoch: state.custody.active_epoch,
        key_tweak,
        child_public_key,
    })
}

pub fn derive_vault_address(state: &AppState, epoch: u64, index: u32) -> Result<String> {
    Ok(derive_vault_child_key(state, epoch, index)?.address)
}

pub fn derive_vault_child_key(state: &AppState, epoch: u64, index: u32) -> Result<DepositChildKey> {
    let keyset = state
        .custody
        .keysets
        .get(&epoch)
        .ok_or_else(|| Error::Frost("missing custody keyset".to_string()))?;
    let public_key = keyset
        .taproot_group_public_key
        .as_deref()
        .unwrap_or(&keyset.group_public_key);
    let key_tweak = derive_deposit_key_tweak(
        DOMAIN,
        "vault-address",
        epoch,
        public_key,
        "internal-vault",
        &index.to_string(),
        "vault",
    )?;
    let (address, child_public_key) = if keyset.taproot_group_public_key.is_some() {
        (
            taproot_address_from_internal_key_with_hidden_root(public_key, &key_tweak)?,
            public_key.to_string(),
        )
    } else {
        let coordinator = FrostSigningCoordinator::new(keyset)?;
        let child_public_key = coordinator.child_group_public_key_hex(&key_tweak)?;
        (
            taproot_address_from_group_public_key(&child_public_key)?,
            child_public_key,
        )
    };
    Ok(DepositChildKey {
        address,
        custody_epoch: epoch,
        key_tweak,
        child_public_key,
    })
}

fn taproot_address_from_group_public_key(public_key: &str) -> Result<String> {
    let group_key_bytes = hex::decode(public_key)
        .map_err(|e| Error::Frost(format!("invalid custody public key hex: {e}")))?;
    let group_key = bitcoin::secp256k1::PublicKey::from_slice(&group_key_bytes)
        .map_err(|e| Error::Frost(format!("invalid custody public key: {e}")))?;
    let (internal_key, _) = group_key.x_only_public_key();
    let secp = bitcoin::secp256k1::Secp256k1::verification_only();
    Ok(bitcoin::Address::p2tr(&secp, internal_key, None, bitcoin::KnownHrp::Regtest).to_string())
}

fn taproot_address_from_internal_key_with_hidden_root(
    public_key: &str,
    merkle_root_hex: &str,
) -> Result<String> {
    let group_key_bytes = hex::decode(public_key)
        .map_err(|e| Error::Frost(format!("invalid custody public key hex: {e}")))?;
    let group_key = bitcoin::secp256k1::PublicKey::from_slice(&group_key_bytes)
        .map_err(|e| Error::Frost(format!("invalid custody public key: {e}")))?;
    let merkle_root_bytes = hex::decode(merkle_root_hex)
        .map_err(|e| Error::Frost(format!("invalid deposit key tweak hex: {e}")))?;
    let merkle_root_bytes: [u8; 32] = merkle_root_bytes
        .try_into()
        .map_err(|_| Error::Frost("deposit key tweak must be 32 bytes".to_string()))?;
    let merkle_root = bitcoin::taproot::TapNodeHash::assume_hidden(merkle_root_bytes);
    let (internal_key, _) = group_key.x_only_public_key();
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

    if signature.scheme != "frost-secp256k1-sha256"
        && signature.scheme != "frost-secp256k1-sha256+tweak"
    {
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

fn bitcoin_solvency_quorum(state: &AppState) -> usize {
    if state.churn.active_nodes.is_empty() {
        1
    } else {
        frost_threshold_for_committee(state.churn.active_nodes.len() as u16) as usize
    }
}

fn bitcoin_withdrawal_has_attestation_quorum(
    state: &AppState,
    withdrawal_id: &str,
    txid: &str,
) -> bool {
    state
        .withdrawals
        .bitcoin_attestations
        .get(withdrawal_id)
        .is_some_and(|set| {
            set.txid == txid
                && set.epoch == state.churn.epoch
                && set.attestations.len() >= bitcoin_solvency_quorum(state)
        })
}

fn validate_bitcoin_attestation(
    state: &AppState,
    withdrawal_id: &str,
    attester: &str,
    epoch: u64,
) -> Result<()> {
    if !state.withdrawals.authorized.contains_key(withdrawal_id) {
        return Err(Error::UnknownWithdrawal);
    }
    if epoch != state.churn.epoch {
        return Err(Error::StaleBitcoinAttestation);
    }
    if !state.churn.active_nodes.is_empty() && !state.churn.active_nodes.contains(attester) {
        return Err(Error::InvalidBitcoinAttestationReporter);
    }
    Ok(())
}

fn validate_bitcoin_solvency_reporter(state: &AppState, reporter: &str, epoch: u64) -> Result<()> {
    if epoch != state.churn.epoch {
        return Err(Error::StaleBitcoinSolvencyReport);
    }
    if !state.churn.active_nodes.is_empty() && !state.churn.active_nodes.contains(reporter) {
        return Err(Error::InvalidBitcoinSolvencyReporter);
    }
    Ok(())
}

pub fn frost_signer_id_for_index(index: u16) -> Result<String> {
    Ok(hex::encode(frost_identifier_from_index(index)?.serialize()))
}

pub fn start_churn_epoch_without_keygen(state: &mut AppState) -> Result<Vec<Event>> {
    let next_epoch = state.churn.epoch + 1;
    let event = Event::ChurnEpochStarted { epoch: next_epoch };
    apply_event(state, event.clone())?;
    let mut events = vec![event];
    let mut demoted_nodes = BTreeSet::new();
    let max_moves = usize::from(state.churn.max_nodes_per_churn.max(1));
    while state.churn.active_nodes.len() > state.churn.target_active_nodes as usize
        && demoted_nodes.len() < max_moves
    {
        let Some(node_id) = churn_out_candidate(state) else {
            break;
        };
        let standby = Event::NodeStatusUpdated {
            node_id: node_id.clone(),
            status: NodeStatus::Standby,
            epoch: next_epoch,
        };
        apply_event(state, standby.clone())?;
        demoted_nodes.insert(node_id);
        events.push(standby);
    }
    let standby_nodes = state
        .churn
        .standby_nodes
        .iter()
        .cloned()
        .collect::<Vec<_>>();
    let eligible_nodes = eligible_churn_in_nodes(state);
    let mut activated_count = 0usize;
    for node_id in standby_nodes.into_iter().chain(eligible_nodes) {
        if state.churn.active_nodes.len() >= state.churn.target_active_nodes as usize {
            break;
        }
        if activated_count >= max_moves {
            break;
        }
        if demoted_nodes.contains(&node_id) {
            continue;
        }
        if state.churn.active_nodes.contains(&node_id) {
            continue;
        }
        if state
            .churn
            .node_accounts
            .get(&node_id)
            .is_some_and(|node| node.status == NodeStatus::Standby)
        {
            let ready = Event::NodeStatusUpdated {
                node_id: node_id.clone(),
                status: NodeStatus::Ready,
                epoch: next_epoch,
            };
            apply_event(state, ready.clone())?;
            events.push(ready);
        }
        let event = Event::StandbyNodeActivated {
            node_id,
            epoch: next_epoch,
        };
        apply_event(state, event.clone())?;
        activated_count += 1;
        events.push(event);
    }
    let retirement_events = deposit_retirement_events(state);
    for event in retirement_events.clone() {
        apply_event(state, event.clone())?;
        events.push(event);
    }
    Ok(events)
}

pub fn active_signer_count(state: &AppState) -> u16 {
    u16::try_from(state.churn.active_nodes.len())
        .ok()
        .filter(|count| *count > 0)
        .unwrap_or(DEFAULT_SIGNER_COUNT)
}

pub fn required_node_bond_sats(slot_id: u64) -> u64 {
    required_node_bond_sats_with_params(BASE_NODE_BOND_SATS, NODE_BOND_INCREMENT_SATS, slot_id)
}

pub fn required_node_bond_sats_with_params(
    min_bond_sats: u64,
    min_bond_increase_sats: u64,
    slot_id: u64,
) -> u64 {
    min_bond_sats.saturating_add(min_bond_increase_sats.saturating_mul(slot_id))
}

pub fn required_node_bond_sats_for_state(state: &AppState, slot_id: u64) -> u64 {
    required_node_bond_sats_with_params(
        state.churn.min_bond_sats,
        state.churn.min_bond_increase_sats,
        slot_id,
    )
}

pub fn offline_penalty_sats(bond_sats: u64) -> u64 {
    bond_sats / 100
}

pub fn active_node_quorum(state: &AppState) -> usize {
    let active = state.churn.active_nodes.len();
    if active == 0 {
        1
    } else {
        (active * 2).div_ceil(3)
    }
}

fn active_vault_membership(state: &AppState) -> (Vec<String>, u16) {
    let mut signers = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
    signers.sort();
    let threshold = state
        .custody
        .keysets
        .get(&state.custody.active_epoch)
        .map(|keyset| keyset.threshold)
        .unwrap_or_else(|| frost_threshold_for_committee(signers.len() as u16));
    (signers, threshold)
}

fn deposit_retirement_events(state: &AppState) -> Vec<Event> {
    state
        .deposits
        .intents
        .values()
        .filter(|intent| !intent.split)
        .filter(|intent| should_retire_deposit_intent(state, intent))
        .map(|intent| Event::DepositIntentRetired {
            intent_id: intent.id.clone(),
            deposit_address: intent.deposit_address.clone(),
            custody_epoch: intent.custody_epoch,
            reason: "vault_signers_below_threshold".to_string(),
            epoch: state.churn.epoch,
        })
        .collect()
}

fn should_retire_deposit_intent(state: &AppState, intent: &DepositIntent) -> bool {
    if intent.vault_signers.is_empty() || intent.vault_threshold == 0 {
        return false;
    }
    let active = intent
        .vault_signers
        .iter()
        .filter(|node_id| state.churn.active_nodes.contains(*node_id))
        .count();
    active < usize::from(intent.vault_threshold)
}

pub fn bitcoin_outbound_assignments(
    state: &AppState,
    withdrawal_id: &str,
    scheduled_epoch: u64,
) -> (Option<String>, Vec<String>, Vec<String>) {
    let signers = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
    let threshold = frost_threshold_for_committee(signers.len() as u16);
    bitcoin_outbound_assignments_for_signers(
        state,
        withdrawal_id,
        scheduled_epoch,
        &signers,
        threshold,
    )
}

pub fn bitcoin_outbound_assignments_for_signers(
    state: &AppState,
    withdrawal_id: &str,
    scheduled_epoch: u64,
    signer_candidates: &[String],
    signer_threshold: u16,
) -> (Option<String>, Vec<String>, Vec<String>) {
    let mut attesters = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
    attesters.sort();
    let mut signer_candidates = signer_candidates.to_vec();
    signer_candidates.sort();
    signer_candidates.dedup();
    let threshold = usize::from(signer_threshold);
    if attesters.is_empty() || signer_candidates.is_empty() || threshold == 0 {
        return (None, Vec::new(), Vec::new());
    }

    let mut ranked = signer_candidates
        .iter()
        .map(|node_id| {
            let mut hasher = Sha256::new();
            hasher.update(withdrawal_id.as_bytes());
            hasher.update(scheduled_epoch.to_be_bytes());
            hasher.update(node_id.as_bytes());
            (hex::encode(hasher.finalize()), node_id.clone())
        })
        .collect::<Vec<_>>();
    ranked.sort_by(|left, right| left.0.cmp(&right.0).then_with(|| left.1.cmp(&right.1)));
    let leader = ranked.first().map(|(_, node_id)| node_id.clone());
    let mut signers = ranked
        .into_iter()
        .take(threshold.min(signer_candidates.len()))
        .map(|(_, node_id)| node_id)
        .collect::<Vec<_>>();
    signers.sort();
    (leader, signers, attesters)
}

pub fn network_parameter_vote_count(
    state: &AppState,
    epoch: u64,
    churn_cycle_ms: u64,
    target_active_nodes: u16,
    max_nodes_per_churn: u16,
    bitcoin_keysign_grace_epochs: u64,
    bitcoin_attestation_grace_epochs: u64,
) -> usize {
    state
        .churn
        .network_parameter_votes
        .get(&epoch)
        .map(BTreeMap::values)
        .into_iter()
        .flatten()
        .filter(|vote| {
            vote.churn_cycle_ms == churn_cycle_ms
                && vote.target_active_nodes == target_active_nodes
                && vote.max_nodes_per_churn == max_nodes_per_churn
                && vote.bitcoin_keysign_grace_epochs == bitcoin_keysign_grace_epochs
                && vote.bitcoin_attestation_grace_epochs == bitcoin_attestation_grace_epochs
        })
        .count()
}

fn churn_out_candidate(state: &AppState) -> Option<String> {
    let mut candidates = state
        .churn
        .active_nodes
        .iter()
        .filter_map(|node_id| {
            state.churn.node_accounts.get(node_id).map(|node| {
                (
                    node.node_id.clone(),
                    node.slash_points,
                    node.active_since_epoch.unwrap_or(node.status_since_epoch),
                )
            })
        })
        .collect::<Vec<_>>();
    candidates.sort_by(|a, b| {
        b.1.cmp(&a.1)
            .then_with(|| a.2.cmp(&b.2))
            .then_with(|| a.0.cmp(&b.0))
    });
    candidates.into_iter().map(|candidate| candidate.0).next()
}

fn eligible_churn_in_nodes(state: &AppState) -> Vec<String> {
    state
        .churn
        .node_accounts
        .values()
        .filter(|node| {
            node.status == NodeStatus::Standby
                && !node.forced_leave
                && node.slot_id.is_some_and(|slot_id| {
                    node.bond_sats >= required_node_bond_sats_for_state(state, slot_id)
                })
                && !state.churn.active_nodes.contains(&node.node_id)
        })
        .map(|node| node.node_id.clone())
        .collect()
}

fn update_node_status(
    state: &mut AppState,
    node_id: &str,
    status: NodeStatus,
    epoch: u64,
) -> Result<()> {
    let node = state
        .churn
        .node_accounts
        .get_mut(node_id)
        .ok_or(Error::NodeNotFound)?;
    node.status = status.clone();
    node.status_since_epoch = epoch;
    match status {
        NodeStatus::Active => {
            node.active_since_epoch = Some(epoch);
            state.churn.active_nodes.insert(node_id.to_string());
            state.churn.standby_nodes.remove(node_id);
        }
        NodeStatus::Standby => {
            node.active_since_epoch = None;
            state.churn.active_nodes.remove(node_id);
            state.churn.standby_nodes.insert(node_id.to_string());
        }
        NodeStatus::Ready => {
            state.churn.active_nodes.remove(node_id);
            state.churn.standby_nodes.remove(node_id);
        }
        NodeStatus::Disabled | NodeStatus::Unknown => {
            node.active_since_epoch = None;
            state.churn.active_nodes.remove(node_id);
            state.churn.standby_nodes.remove(node_id);
        }
    }
    Ok(())
}

fn active_keygen_count(state: &AppState) -> Option<u16> {
    let count = active_signer_count(state);
    (count >= 2).then_some(count)
}

pub fn validate_keyset_commit(state: &AppState, epoch: u64, keyset: &FrostKeyset) -> Result<()> {
    if epoch != state.churn.epoch || keyset.epoch != epoch {
        return Err(Error::Frost(
            "custody keyset epoch must match current churn epoch".to_string(),
        ));
    }

    let signer_count = active_keygen_count(state).ok_or_else(|| {
        Error::Frost("at least two active nodes are required for FROST keygen".to_string())
    })?;
    let threshold = frost_threshold_for_committee(signer_count);
    if keyset.max_signers != signer_count || keyset.threshold != threshold {
        return Err(Error::Frost(format!(
            "custody keyset must match active set: expected {threshold}-of-{signer_count}, got {}-of-{}",
            keyset.threshold, keyset.max_signers
        )));
    }
    if keyset.group_public_key.is_empty() || keyset.public_key_package.is_empty() {
        return Err(Error::Frost(
            "custody keyset public material cannot be empty".to_string(),
        ));
    }

    Ok(())
}

fn ensure_active_keyset(state: &mut AppState) -> Result<Vec<Event>> {
    if state
        .custody
        .keysets
        .contains_key(&state.custody.active_epoch)
    {
        return Ok(Vec::new());
    }

    let signer_count = active_keygen_count(state).ok_or_else(|| {
        Error::Frost("at least two active nodes are required for FROST keygen".to_string())
    })?;
    let keyset = FrostCustodySigner::generate_keyset_with_dkg(
        state.churn.epoch,
        signer_count,
        frost_threshold_for_committee(signer_count),
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
    if !intent.user_pubkey.is_empty()
        && note_commitments
            .iter()
            .any(|note| note.owner_pubkey != intent.user_pubkey)
    {
        return Err(Error::InvalidNoteOwner);
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

    let events = vec![
        Event::DepositSplitRecorded {
            deposit_id: deposit_id.to_string(),
            remainder_sats: remaining,
        },
        Event::NotesMinted { notes },
    ];
    for event in events.clone() {
        apply_event(state, event)?;
    }
    Ok(events)
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
    let custody = withdrawal_custody_metadata(state);

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
            custody_epoch: custody.custody_epoch,
            deposit_key_tweak: custody.deposit_key_tweak,
            vault_signers: custody.vault_signers,
            vault_threshold: custody.vault_threshold,
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

fn request_withdrawal<V: ProofVerifier>(
    state: &mut AppState,
    proof: WithdrawalProof,
    public: WithdrawalPublicInputs,
    verifier: &V,
) -> Result<Vec<Event>> {
    validate_withdrawal_public(state, &proof, &public, verifier)?;
    let custody = withdrawal_custody_metadata(state);
    let withdrawal_id = format!("wd-{}", state.next_withdrawal_id);
    let amount_sats = public
        .denomination_sats
        .checked_sub(public.fee_sats)
        .ok_or(Error::InvalidProof)?;
    let event = Event::WithdrawalRequested {
        withdrawal_id,
        recipient: public.recipient,
        amount_sats,
        fee_sats: public.fee_sats,
        nullifier_hash: public.nullifier_hash,
        denomination_sats: public.denomination_sats,
        custody_epoch: custody.custody_epoch,
        deposit_key_tweak: custody.deposit_key_tweak,
        vault_signers: custody.vault_signers,
        vault_threshold: custody.vault_threshold,
    };
    apply_event(state, event.clone())?;
    Ok(vec![event])
}

fn authorize_pending_withdrawal(
    state: &mut AppState,
    withdrawal_id: &str,
    signature: CustodySignature,
) -> Result<Vec<Event>> {
    let pending = state
        .withdrawals
        .pending
        .get(withdrawal_id)
        .cloned()
        .ok_or(Error::UnknownWithdrawal)?;
    if state
        .notes
        .spent_nullifiers
        .contains(&pending.nullifier_hash)
    {
        return Err(Error::DuplicateNullifier);
    }
    let request = WithdrawalRequest {
        withdrawal_id: pending.id.clone(),
        recipient: pending.recipient.clone(),
        amount_sats: pending.amount_sats,
        fee_sats: pending.fee_sats,
        nullifier_hash: pending.nullifier_hash.clone(),
    };
    verify_custody_signature(&request, &signature)?;
    let keyset = state
        .custody
        .keysets
        .get(&pending.custody_epoch)
        .ok_or_else(|| Error::Frost("missing withdrawal custody keyset".to_string()))?;
    if signature.group_public_key != keyset.group_public_key {
        return Err(Error::Frost(
            "withdrawal signature does not match active custody keyset".to_string(),
        ));
    }
    let events = vec![
        Event::NoteSpent {
            nullifier_hash: pending.nullifier_hash.clone(),
            denomination_sats: pending.denomination_sats,
        },
        Event::WithdrawalAuthorized {
            withdrawal_id: pending.id,
            recipient: pending.recipient,
            amount_sats: pending.amount_sats,
            fee_sats: pending.fee_sats,
            nullifier_hash: pending.nullifier_hash,
            custody_epoch: pending.custody_epoch,
            deposit_key_tweak: pending.deposit_key_tweak,
            vault_signers: pending.vault_signers,
            vault_threshold: pending.vault_threshold,
            signature,
        },
        Event::FeeCharged {
            amount_sats: pending.fee_sats,
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

fn validate_withdrawal_public<V: ProofVerifier>(
    state: &AppState,
    proof: &WithdrawalProof,
    public: &WithdrawalPublicInputs,
    verifier: &V,
) -> Result<()> {
    verifier.verify_withdrawal(proof, public)?;

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
    Ok(())
}

#[derive(Debug, Clone)]
struct WithdrawalCustodyMetadata {
    custody_epoch: u64,
    deposit_key_tweak: String,
    vault_signers: Vec<String>,
    vault_threshold: u16,
}

fn withdrawal_custody_metadata(state: &AppState) -> WithdrawalCustodyMetadata {
    let (vault_signers, vault_threshold) = active_vault_membership(state);
    WithdrawalCustodyMetadata {
        custody_epoch: state.custody.active_epoch,
        deposit_key_tweak: String::new(),
        vault_signers,
        vault_threshold,
    }
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

fn frost_tr_error<E: std::fmt::Debug>(error: E) -> Error {
    Error::Frost(format!("{error:?}"))
}

fn derive_deposit_key_tweak(
    domain: &str,
    label: &str,
    epoch: u64,
    public_key: &str,
    user_pubkey: &str,
    intent_id: &str,
    pow_token: &str,
) -> Result<String> {
    for counter in 0..=u32::MAX {
        let counter = counter.to_string();
        let epoch = epoch.to_string();
        let digest = hash_parts_bytes(&[
            domain,
            label,
            &epoch,
            public_key,
            user_pubkey,
            intent_id,
            pow_token,
            &counter,
        ]);
        if frost_rerandomized::Randomizer::<frost_secp256k1::Secp256K1Sha256>::deserialize(&digest)
            .is_ok()
        {
            return Ok(hex::encode(digest));
        }
    }
    Err(Error::Frost(
        "unable to derive valid nonzero FROST deposit key tweak".to_string(),
    ))
}

fn frost_randomizer_from_hex(
    tweak: &str,
) -> Result<frost_rerandomized::Randomizer<frost_secp256k1::Secp256K1Sha256>> {
    let bytes = hex::decode(tweak)
        .map_err(|e| Error::Frost(format!("invalid FROST key tweak hex: {e}")))?;
    frost_rerandomized::Randomizer::deserialize(&bytes).map_err(frost_error)
}

fn frost_identifier_from_hex(signer_id: &str) -> Result<frost_secp256k1::Identifier> {
    let bytes = hex::decode(signer_id)
        .map_err(|e| Error::Frost(format!("invalid FROST signer id hex: {e}")))?;
    frost_secp256k1::Identifier::deserialize(&bytes).map_err(frost_error)
}

fn frost_tr_identifier_from_hex(signer_id: &str) -> Result<frost_secp256k1_tr::Identifier> {
    let bytes = hex::decode(signer_id)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST signer id hex: {e}")))?;
    frost_secp256k1_tr::Identifier::deserialize(&bytes).map_err(frost_tr_error)
}

fn frost_identifier_from_index(index: u16) -> Result<frost_secp256k1::Identifier> {
    index
        .try_into()
        .map_err(|e| Error::Frost(format!("invalid participant id: {e:?}")))
}

fn frost_tr_identifier_from_index(index: u16) -> Result<frost_secp256k1_tr::Identifier> {
    index
        .try_into()
        .map_err(|e| Error::Frost(format!("invalid Taproot participant id: {e:?}")))
}

fn frost_commitment_from_hex(
    commitment: &str,
) -> Result<frost_secp256k1::round1::SigningCommitments> {
    let bytes = hex::decode(commitment)
        .map_err(|e| Error::Frost(format!("invalid FROST commitment hex: {e}")))?;
    frost_secp256k1::round1::SigningCommitments::deserialize(&bytes).map_err(frost_error)
}

fn frost_dkg_round1_package_from_hex(
    package: &str,
) -> Result<frost_secp256k1::keys::dkg::round1::Package> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid FROST DKG round1 package hex: {e}")))?;
    frost_secp256k1::keys::dkg::round1::Package::deserialize(&bytes).map_err(frost_error)
}

fn frost_dkg_round1_secret_from_hex(
    package: &str,
) -> Result<frost_secp256k1::keys::dkg::round1::SecretPackage> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid FROST DKG round1 secret hex: {e}")))?;
    frost_secp256k1::keys::dkg::round1::SecretPackage::deserialize(&bytes).map_err(frost_error)
}

fn frost_tr_dkg_round1_package_from_hex(
    package: &str,
) -> Result<frost_secp256k1_tr::keys::dkg::round1::Package> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST DKG round1 package hex: {e}")))?;
    frost_secp256k1_tr::keys::dkg::round1::Package::deserialize(&bytes).map_err(frost_tr_error)
}

fn frost_tr_dkg_round1_secret_from_hex(
    package: &str,
) -> Result<frost_secp256k1_tr::keys::dkg::round1::SecretPackage> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST DKG round1 secret hex: {e}")))?;
    frost_secp256k1_tr::keys::dkg::round1::SecretPackage::deserialize(&bytes)
        .map_err(frost_tr_error)
}

fn frost_dkg_round2_package_from_hex(
    package: &str,
) -> Result<frost_secp256k1::keys::dkg::round2::Package> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid FROST DKG round2 package hex: {e}")))?;
    frost_secp256k1::keys::dkg::round2::Package::deserialize(&bytes).map_err(frost_error)
}

fn frost_dkg_round2_secret_from_hex(
    package: &str,
) -> Result<frost_secp256k1::keys::dkg::round2::SecretPackage> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid FROST DKG round2 secret hex: {e}")))?;
    frost_secp256k1::keys::dkg::round2::SecretPackage::deserialize(&bytes).map_err(frost_error)
}

fn frost_tr_dkg_round2_package_from_hex(
    package: &str,
) -> Result<frost_secp256k1_tr::keys::dkg::round2::Package> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST DKG round2 package hex: {e}")))?;
    frost_secp256k1_tr::keys::dkg::round2::Package::deserialize(&bytes).map_err(frost_tr_error)
}

fn frost_tr_dkg_round2_secret_from_hex(
    package: &str,
) -> Result<frost_secp256k1_tr::keys::dkg::round2::SecretPackage> {
    let bytes = hex::decode(package)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST DKG round2 secret hex: {e}")))?;
    frost_secp256k1_tr::keys::dkg::round2::SecretPackage::deserialize(&bytes)
        .map_err(frost_tr_error)
}

fn frost_nonces_from_hex(nonces: &str) -> Result<frost_secp256k1::round1::SigningNonces> {
    let bytes =
        hex::decode(nonces).map_err(|e| Error::Frost(format!("invalid FROST nonces hex: {e}")))?;
    frost_secp256k1::round1::SigningNonces::deserialize(&bytes).map_err(frost_error)
}

fn frost_tr_nonces_from_hex(nonces: &str) -> Result<frost_secp256k1_tr::round1::SigningNonces> {
    let bytes = hex::decode(nonces)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST nonces hex: {e}")))?;
    frost_secp256k1_tr::round1::SigningNonces::deserialize(&bytes).map_err(frost_tr_error)
}

fn frost_signature_share_from_hex(share: &str) -> Result<frost_secp256k1::round2::SignatureShare> {
    let bytes = hex::decode(share)
        .map_err(|e| Error::Frost(format!("invalid FROST signature share hex: {e}")))?;
    frost_secp256k1::round2::SignatureShare::deserialize(&bytes).map_err(frost_error)
}

fn frost_tr_signature_share_from_hex(
    share: &str,
) -> Result<frost_secp256k1_tr::round2::SignatureShare> {
    let bytes = hex::decode(share)
        .map_err(|e| Error::Frost(format!("invalid Taproot FROST signature share hex: {e}")))?;
    frost_secp256k1_tr::round2::SignatureShare::deserialize(&bytes).map_err(frost_tr_error)
}

fn signing_package_from_commitments(
    request: &WithdrawalRequest,
    commitments: &[FrostSigningCommitmentPublic],
) -> Result<frost_secp256k1::SigningPackage> {
    let message = withdrawal_signing_message(request)?;
    let commitments = commitments
        .iter()
        .map(|commitment| {
            Ok((
                frost_identifier_from_hex(&commitment.signer_id)?,
                frost_commitment_from_hex(&commitment.commitment)?,
            ))
        })
        .collect::<Result<BTreeMap<_, _>>>()?;
    Ok(frost_secp256k1::SigningPackage::new(commitments, &message))
}

fn taproot_signing_package_from_commitments(
    message: &[u8],
    commitments: &[FrostSigningCommitmentPublic],
) -> Result<frost_secp256k1_tr::SigningPackage> {
    let commitments = commitments
        .iter()
        .map(|commitment| {
            let bytes = hex::decode(&commitment.commitment)
                .map_err(|e| Error::Frost(format!("invalid Taproot FROST commitment hex: {e}")))?;
            Ok((
                frost_tr_identifier_from_hex(&commitment.signer_id)?,
                frost_secp256k1_tr::round1::SigningCommitments::deserialize(&bytes)
                    .map_err(frost_tr_error)?,
            ))
        })
        .collect::<Result<BTreeMap<_, _>>>()?;
    Ok(frost_secp256k1_tr::SigningPackage::new(
        commitments,
        message,
    ))
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
        let owner_secret = note_owner_secret(client_seed, deposit_id, index);
        let owner_pubkey = note_owner_pubkey(&owner_secret);
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
        let (commitment, orchard_note) = {
            #[cfg(feature = "orchard-zcash")]
            {
                let (commitment, note) =
                    orchard::create_orchard_note(client_seed, deposit_id, index, denomination)?;
                (commitment, Some(note))
            }
            #[cfg(not(feature = "orchard-zcash"))]
            {
                (
                    note_commitment(&nullifier, &secret, denomination, &owner_pubkey),
                    None::<()>,
                )
            }
        };
        notes.push(NoteReceipt {
            deposit_id: deposit_id.to_string(),
            denomination_sats: denomination,
            index,
            owner_pubkey,
            nullifier,
            secret,
            commitment,
            #[cfg(feature = "orchard-zcash")]
            orchard: orchard_note,
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
                owner_pubkey: note.owner_pubkey.clone(),
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
