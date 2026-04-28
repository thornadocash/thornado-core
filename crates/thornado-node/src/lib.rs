use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::{Html, IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;
use std::fs;
#[cfg(unix)]
use std::os::unix::fs::OpenOptionsExt;
use std::path::PathBuf;
use std::sync::{Arc, Mutex as StdMutex};
use thornado_abci::encode_tx;
use thornado_bitcoin::{
    BitcoinBackend, BitcoinConsolidationRecord, BitcoinConsolidationRequest, BitcoinRpcConfig,
    BitcoinSolvencyReport, BitcoinWithdrawalRecord, BitcoinWithdrawalRequest, BuiltConsolidation,
    BuiltWithdrawal, DevBitcoinBackend, RegtestUtxo, RpcBitcoinBackend,
    SigningCheckpointValidation,
};
use thornado_core::{
    active_signer_count, apply_event, execute_command, load_snapshot,
    required_node_bond_sats_for_state, save_snapshot, start_churn_epoch_without_keygen, state_hash,
    AppState, AuthorizedWithdrawal, Command, CustodySignature, CustodySigner, Error as CoreError,
    Event, FrostCustodySigner, FrostCustodySignerSnapshot, FrostKeyset, FrostSignatureShare,
    FrostSigningCommitmentPublic, MockCustodySigner, MockProofVerifier, NodeStatus, NoteCommitment,
    PendingWithdrawal, StarkProofVerifier, WithdrawalProof, WithdrawalPublicInputs,
    WithdrawalRequest,
};
use thornado_store::{get_json, put_json, RedbKvStore};
use tokio::sync::Mutex;

const BITCOIN_BACKEND_KEY: &str = "bitcoin_backend";

#[derive(Clone)]
pub struct NodeState {
    inner: Arc<Mutex<AppState>>,
    custody_signer: Arc<Mutex<Option<FrostCustodySigner>>>,
    bitcoin: Arc<Mutex<NodeBitcoinBackend>>,
    snapshot_path: Option<PathBuf>,
    frost_signer_path: Option<PathBuf>,
    bitcoin_state_path: Option<PathBuf>,
    peers: Arc<Mutex<Vec<String>>>,
    frost_nonces: Arc<Mutex<BTreeMap<String, String>>>,
    consensus: Option<CometBftClient>,
    client: reqwest::Client,
    node_id: Option<String>,
    dkg_round1: Arc<Mutex<BTreeMap<String, DkgRound1State>>>,
    dkg_round2: Arc<Mutex<BTreeMap<String, DkgRound2State>>>,
}

#[derive(Debug, Clone)]
pub struct NodeConfig {
    pub snapshot_path: Option<PathBuf>,
    pub frost_signer_path: Option<PathBuf>,
    pub bitcoin_state_path: Option<PathBuf>,
    pub bitcoin_rpc: Option<BitcoinRpcConfig>,
    pub node_id: Option<String>,
}

#[derive(Debug, Clone)]
struct DkgRound1State {
    signer_id: String,
    secret_package: String,
}

#[derive(Debug, Clone)]
struct DkgRound2State {
    signer_id: String,
    secret_package: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(tag = "mode", rename_all = "snake_case")]
pub enum NodeBitcoinBackend {
    Dev(DevBitcoinBackend),
    Rpc(RpcBitcoinBackend),
}

impl NodeBitcoinBackend {
    fn dev() -> Self {
        Self::Dev(DevBitcoinBackend::new())
    }
}

impl BitcoinBackend for NodeBitcoinBackend {
    fn import_dev_utxo(&mut self, utxo: RegtestUtxo) -> thornado_bitcoin::Result<()> {
        match self {
            Self::Dev(backend) => backend.import_dev_utxo(utxo),
            Self::Rpc(backend) => backend.import_dev_utxo(utxo),
        }
    }

    fn list_utxos(&self) -> Vec<RegtestUtxo> {
        match self {
            Self::Dev(backend) => backend.list_utxos(),
            Self::Rpc(backend) => backend.list_utxos(),
        }
    }

    fn build_withdrawal(
        &mut self,
        request: BitcoinWithdrawalRequest,
    ) -> thornado_bitcoin::Result<BuiltWithdrawal> {
        match self {
            Self::Dev(backend) => backend.build_withdrawal(request),
            Self::Rpc(backend) => backend.build_withdrawal(request),
        }
    }

    fn get_withdrawal(
        &self,
        withdrawal_id: &str,
    ) -> thornado_bitcoin::Result<BitcoinWithdrawalRecord> {
        match self {
            Self::Dev(backend) => backend.get_withdrawal(withdrawal_id),
            Self::Rpc(backend) => backend.get_withdrawal(withdrawal_id),
        }
    }

    fn mark_broadcast(
        &mut self,
        withdrawal_id: &str,
        txid: String,
    ) -> thornado_bitcoin::Result<BitcoinWithdrawalRecord> {
        match self {
            Self::Dev(backend) => backend.mark_broadcast(withdrawal_id, txid),
            Self::Rpc(backend) => backend.mark_broadcast(withdrawal_id, txid),
        }
    }

    fn mark_confirmed(
        &mut self,
        withdrawal_id: &str,
        height: u64,
    ) -> thornado_bitcoin::Result<BitcoinWithdrawalRecord> {
        match self {
            Self::Dev(backend) => backend.mark_confirmed(withdrawal_id, height),
            Self::Rpc(backend) => backend.mark_confirmed(withdrawal_id, height),
        }
    }

    fn broadcast_withdrawal(
        &mut self,
        withdrawal_id: &str,
        signed_tx_hex: String,
    ) -> thornado_bitcoin::Result<BitcoinWithdrawalRecord> {
        match self {
            Self::Dev(backend) => backend.broadcast_withdrawal(withdrawal_id, signed_tx_hex),
            Self::Rpc(backend) => backend.broadcast_withdrawal(withdrawal_id, signed_tx_hex),
        }
    }

    fn validate_signing_checkpoint(
        &self,
        withdrawal_id: &str,
        unsigned_tx_hex: String,
    ) -> thornado_bitcoin::Result<SigningCheckpointValidation> {
        match self {
            Self::Dev(backend) => {
                backend.validate_signing_checkpoint(withdrawal_id, unsigned_tx_hex)
            }
            Self::Rpc(backend) => {
                backend.validate_signing_checkpoint(withdrawal_id, unsigned_tx_hex)
            }
        }
    }

    fn report_solvency(
        &self,
        expected_sats: u64,
    ) -> thornado_bitcoin::Result<BitcoinSolvencyReport> {
        match self {
            Self::Dev(backend) => backend.report_solvency(expected_sats),
            Self::Rpc(backend) => backend.report_solvency(expected_sats),
        }
    }

    fn build_consolidation(
        &mut self,
        request: BitcoinConsolidationRequest,
    ) -> thornado_bitcoin::Result<BuiltConsolidation> {
        match self {
            Self::Dev(backend) => backend.build_consolidation(request),
            Self::Rpc(backend) => backend.build_consolidation(request),
        }
    }

    fn get_consolidation(
        &self,
        consolidation_id: &str,
    ) -> thornado_bitcoin::Result<BitcoinConsolidationRecord> {
        match self {
            Self::Dev(backend) => backend.get_consolidation(consolidation_id),
            Self::Rpc(backend) => backend.get_consolidation(consolidation_id),
        }
    }

    fn broadcast_consolidation(
        &mut self,
        consolidation_id: &str,
        signed_tx_hex: String,
    ) -> thornado_bitcoin::Result<BitcoinConsolidationRecord> {
        match self {
            Self::Dev(backend) => backend.broadcast_consolidation(consolidation_id, signed_tx_hex),
            Self::Rpc(backend) => backend.broadcast_consolidation(consolidation_id, signed_tx_hex),
        }
    }
}

impl NodeState {
    pub fn new(state: AppState, snapshot_path: Option<PathBuf>) -> Self {
        Self::with_peers(state, snapshot_path, Vec::new())
    }

    pub fn with_peers(state: AppState, snapshot_path: Option<PathBuf>, peers: Vec<String>) -> Self {
        Self::with_config(
            state,
            NodeConfig {
                snapshot_path,
                frost_signer_path: None,
                bitcoin_state_path: None,
                bitcoin_rpc: None,
                node_id: None,
            },
            peers,
        )
        .expect("dev node config is valid")
    }

    pub fn with_config(
        state: AppState,
        config: NodeConfig,
        peers: Vec<String>,
    ) -> thornado_core::Result<Self> {
        let custody_signer = match config.frost_signer_path.as_ref() {
            Some(path) if path.exists() => Some(load_frost_signer(path)?),
            _ => None,
        };
        let bitcoin = match config.bitcoin_state_path.as_ref() {
            Some(path) if path.exists() => load_bitcoin_backend(path)?,
            _ => match config.bitcoin_rpc {
                Some(rpc) => NodeBitcoinBackend::Rpc(
                    RpcBitcoinBackend::new(rpc)
                        .map_err(|e| CoreError::Frost(format!("bitcoin RPC init failed: {e}")))?,
                ),
                None => NodeBitcoinBackend::dev(),
            },
        };
        Ok(Self {
            inner: Arc::new(Mutex::new(state)),
            custody_signer: Arc::new(Mutex::new(custody_signer)),
            bitcoin: Arc::new(Mutex::new(bitcoin)),
            snapshot_path: config.snapshot_path,
            frost_signer_path: config.frost_signer_path,
            bitcoin_state_path: config.bitcoin_state_path,
            peers: Arc::new(Mutex::new(peers)),
            frost_nonces: Arc::new(Mutex::new(BTreeMap::new())),
            consensus: None,
            client: reqwest::Client::new(),
            node_id: config.node_id,
            dkg_round1: Arc::new(Mutex::new(BTreeMap::new())),
            dkg_round2: Arc::new(Mutex::new(BTreeMap::new())),
        })
    }

    pub fn with_cometbft_rpc(mut self, rpc_url: String) -> Self {
        self.consensus = Some(CometBftClient::new(rpc_url));
        self
    }

    pub fn from_snapshot_or_default(snapshot_path: Option<PathBuf>) -> thornado_core::Result<Self> {
        Self::from_snapshot_or_default_with_peers(snapshot_path, Vec::new())
    }

    pub fn from_snapshot_or_default_with_peers(
        snapshot_path: Option<PathBuf>,
        peers: Vec<String>,
    ) -> thornado_core::Result<Self> {
        let state = match snapshot_path.as_ref() {
            Some(path) if path.exists() => load_snapshot(path)?,
            _ => AppState::default(),
        };
        Ok(Self::with_peers(state, snapshot_path, peers))
    }

    pub fn from_config_or_default(
        config: NodeConfig,
        peers: Vec<String>,
    ) -> thornado_core::Result<Self> {
        let state = match config.snapshot_path.as_ref() {
            Some(path) if path.exists() => load_snapshot(path)?,
            _ => AppState::default(),
        };
        Self::with_config(state, config, peers)
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct EventsResponse {
    pub events: Vec<Event>,
    pub state_hash: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tx_hash: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ApplyEventsBody {
    pub events: Vec<Event>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PeerBody {
    pub url: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PeersResponse {
    pub peers: Vec<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct StateHashResponse {
    pub state_hash: String,
}

#[derive(Debug, Clone)]
pub struct CometBftClient {
    rpc_url: String,
    client: reqwest::Client,
}

impl CometBftClient {
    pub fn new(rpc_url: String) -> Self {
        Self {
            rpc_url: rpc_url.trim_end_matches('/').to_string(),
            client: reqwest::Client::new(),
        }
    }

    async fn broadcast_tx_commit(&self, command: Command) -> Result<String, ApiError> {
        let tx = encode_tx(command).map_err(ApiError::consensus_encode)?;
        let request = CometBftRpcRequest {
            jsonrpc: "2.0",
            id: "thornado-node",
            method: "broadcast_tx_commit",
            params: CometBftBroadcastParams {
                tx: format!("0x{}", hex::encode(tx)),
            },
        };
        let response = self
            .client
            .post(&self.rpc_url)
            .json(&request)
            .send()
            .await
            .map_err(ApiError::consensus_rpc)?;
        if !response.status().is_success() {
            return Err(ApiError::consensus_status(
                self.rpc_url.clone(),
                response.status(),
                response.text().await.unwrap_or_default(),
            ));
        }
        let response = response
            .json::<CometBftRpcResponse>()
            .await
            .map_err(ApiError::consensus_rpc)?;
        if let Some(error) = response.error {
            return Err(ApiError::consensus_message(error.message));
        }
        let result = response
            .result
            .ok_or_else(|| ApiError::consensus_message("missing CometBFT result".to_string()))?;
        let check_code = result.check_tx.as_ref().and_then(|tx| tx.code).unwrap_or(0);
        let deliver_code = result
            .tx_result
            .as_ref()
            .or(result.deliver_tx.as_ref())
            .and_then(|tx| tx.code)
            .unwrap_or(0);
        if check_code != 0 || deliver_code != 0 {
            let log = result
                .tx_result
                .as_ref()
                .or(result.deliver_tx.as_ref())
                .and_then(|tx| tx.log.clone())
                .or_else(|| result.check_tx.as_ref().and_then(|tx| tx.log.clone()))
                .unwrap_or_else(|| "CometBFT rejected transaction".to_string());
            return Err(ApiError::consensus_message(log));
        }
        Ok(result.hash.unwrap_or_default())
    }
}

#[derive(Debug, Serialize)]
struct CometBftRpcRequest<'a> {
    jsonrpc: &'a str,
    id: &'a str,
    method: &'a str,
    params: CometBftBroadcastParams,
}

#[derive(Debug, Serialize)]
struct CometBftBroadcastParams {
    tx: String,
}

#[derive(Debug, Deserialize)]
struct CometBftRpcResponse {
    result: Option<CometBftBroadcastResult>,
    error: Option<CometBftRpcError>,
}

#[derive(Debug, Deserialize)]
struct CometBftRpcError {
    message: String,
}

#[derive(Debug, Deserialize)]
struct CometBftBroadcastResult {
    #[serde(default)]
    hash: Option<String>,
    #[serde(default)]
    check_tx: Option<CometBftTxResult>,
    #[serde(default)]
    deliver_tx: Option<CometBftTxResult>,
    #[serde(default)]
    tx_result: Option<CometBftTxResult>,
}

#[derive(Debug, Deserialize)]
struct CometBftTxResult {
    #[serde(default)]
    code: Option<u32>,
    #[serde(default)]
    log: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RootResponse {
    pub denomination_sats: u64,
    pub root: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct DepositRequestBody {
    pub pow_token: String,
    #[serde(default)]
    pub user_pubkey: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct DepositObserveBody {
    pub intent_id: String,
    pub txid: String,
    pub amount_sats: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct DepositConfirmBody {
    pub intent_id: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SplitBody {
    pub deposit_id: String,
    pub note_commitments: Vec<NoteCommitment>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct WithdrawBody {
    pub proof: WithdrawalProof,
    pub public: WithdrawalPublicInputs,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AuthorizeWithdrawalBody {
    pub withdrawal_id: String,
    pub signature: CustodySignature,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SubmitBitcoinBroadcastBody {
    pub withdrawal_id: String,
    pub txid: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RegisterNodeBody {
    pub node_id: String,
    pub bond_address: String,
    pub consensus_pubkey: String,
    pub signer_pubkey: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BondNodeBody {
    pub node_id: String,
    pub amount_sats: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AssignNodeSlotBody {
    pub node_id: String,
    pub slot_id: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BondParametersBody {
    pub min_bond_sats: u64,
    pub min_bond_increase_sats: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct OfflineBody {
    pub node_id: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ChurnNodeBody {
    pub node_id: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct CommitKeysetBody {
    pub epoch: u64,
    pub keyset: FrostKeyset,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeysignCommitmentBody {
    pub session_id: String,
    pub signer_id: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeysignShareBody {
    pub session_id: String,
    pub signer_id: String,
    pub request: WithdrawalRequest,
    pub commitments: Vec<FrostSigningCommitmentPublic>,
    pub key_tweak: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeysignBody {
    pub request: WithdrawalRequest,
    pub key_tweak: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeygenBody {
    pub epoch: Option<u64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BifrostKeygenWork {
    pub pending: bool,
    pub epoch: u64,
    pub participants: Vec<String>,
    pub threshold: u16,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BifrostWithdrawalsWork {
    pub pending: Vec<PendingWithdrawal>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BifrostBitcoinWork {
    pub authorized: Vec<AuthorizedWithdrawal>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeygenRound1Body {
    pub session_id: String,
    pub signer_id: String,
    pub participant_index: u16,
    pub signer_count: u16,
    pub threshold: u16,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeygenRound2Body {
    pub session_id: String,
    pub round1_packages: Vec<thornado_core::FrostDkgRound1Public>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FrostKeygenFinalizeBody {
    pub session_id: String,
    pub epoch: u64,
    pub round1_packages: Vec<thornado_core::FrostDkgRound1Public>,
    pub round2_packages: Vec<thornado_core::FrostDkgRound2Public>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ImportUtxoResponse {
    pub imported: RegtestUtxo,
    pub utxo_count: usize,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BuildBitcoinWithdrawalBody {
    pub withdrawal_id: String,
    pub fee_rate_sats_per_vb: Option<u64>,
    pub change_script_pubkey_hex: Option<String>,
    pub max_fee_rate_sats_per_vb: Option<u64>,
    pub min_relay_fee_sats: Option<u64>,
    pub max_inputs: Option<usize>,
    pub min_confirmations: Option<u64>,
    pub max_mempool_chain_length: Option<u64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UtxosResponse {
    pub utxos: Vec<RegtestUtxo>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MarkBroadcastBody {
    pub txid: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BroadcastBitcoinWithdrawalBody {
    pub signed_tx_hex: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ValidateBitcoinCheckpointBody {
    pub unsigned_tx_hex: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BitcoinSolvencyBody {
    pub expected_sats: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct BuildBitcoinConsolidationBody {
    pub consolidation_id: String,
    pub fee_rate_sats_per_vb: Option<u64>,
    pub change_script_pubkey_hex: String,
    pub min_utxos: Option<usize>,
    pub max_inputs: Option<usize>,
    pub min_confirmations: Option<u64>,
    pub max_mempool_chain_length: Option<u64>,
    pub max_fee_rate_sats_per_vb: Option<u64>,
    pub min_relay_fee_sats: Option<u64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MarkConfirmedBody {
    pub height: u64,
}

#[derive(Debug, Serialize, Deserialize)]
struct ErrorResponse {
    error: String,
}

pub fn router(state: NodeState) -> Router {
    Router::new()
        .route("/", get(ui))
        .route("/deposit/request", post(deposit_request))
        .route("/deposit/observe", post(deposit_observe))
        .route("/deposit/confirm", post(deposit_confirm))
        .route("/split", post(split))
        .route("/withdraw", post(withdraw))
        .route("/withdraw/authorize", post(withdraw_authorize))
        .route("/nodes/register", post(node_register))
        .route("/nodes/bond", post(node_bond))
        .route("/nodes/slot/assign", post(node_slot_assign))
        .route("/nodes/bond-parameters", post(node_bond_parameters))
        .route("/custody/keyset/commit", post(custody_keyset_commit))
        .route("/frost/keygen", post(frost_keygen))
        .route("/frost/keygen/round1", post(frost_keygen_round1))
        .route("/frost/keygen/round2", post(frost_keygen_round2))
        .route("/frost/keygen/finalize", post(frost_keygen_finalize))
        .route("/frost/keysign", post(frost_keysign))
        .route("/frost/keysign/commitment", post(frost_keysign_commitment))
        .route("/frost/keysign/share", post(frost_keysign_share))
        .route("/churn/standby", post(churn_standby))
        .route("/churn/start", post(churn_start))
        .route("/churn/offline", post(churn_offline))
        .route("/churn/penalties", post(churn_penalties))
        .route("/events/apply", post(apply_events))
        .route("/peers", get(peers))
        .route("/peers/add", post(peer_add))
        .route("/bitcoin/utxo/import", post(bitcoin_utxo_import))
        .route("/bitcoin/utxos", get(bitcoin_utxos))
        .route("/bitcoin/withdrawal/build", post(bitcoin_withdrawal_build))
        .route("/bitcoin/withdrawal/:id", get(bitcoin_withdrawal_get))
        .route(
            "/bitcoin/withdrawal/:id/broadcast",
            post(bitcoin_withdrawal_broadcast),
        )
        .route(
            "/bitcoin/withdrawal/:id/checkpoint/validate",
            post(bitcoin_withdrawal_checkpoint_validate),
        )
        .route(
            "/bitcoin/withdrawal/:id/mark-broadcast",
            post(bitcoin_withdrawal_mark_broadcast),
        )
        .route(
            "/bitcoin/withdrawal/:id/mark-confirmed",
            post(bitcoin_withdrawal_mark_confirmed),
        )
        .route("/bitcoin/solvency", post(bitcoin_solvency))
        .route("/bitcoin/withdrawal/broadcast/submit", post(bitcoin_broadcast_submit))
        .route(
            "/bitcoin/consolidation/build",
            post(bitcoin_consolidation_build),
        )
        .route("/bitcoin/consolidation/:id", get(bitcoin_consolidation_get))
        .route(
            "/bitcoin/consolidation/:id/broadcast",
            post(bitcoin_consolidation_broadcast),
        )
        .route("/state/hash", get(get_state_hash))
        .route("/bifrost/work/keygen", get(bifrost_work_keygen))
        .route("/bifrost/work/withdrawals", get(bifrost_work_withdrawals))
        .route("/bifrost/work/bitcoin", get(bifrost_work_bitcoin))
        .route("/bifrost/tick", post(bifrost_tick))
        .route("/notes/root/:denomination", get(get_note_root))
        .with_state(state)
}

async fn ui() -> Html<&'static str> {
    Html(include_str!("../static/index.html"))
}

async fn deposit_request(
    State(node): State<NodeState>,
    Json(body): Json<DepositRequestBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::RequestDepositAddress {
            pow_token: body.pow_token,
            user_pubkey: body.user_pubkey,
        },
    )
    .await
}

async fn deposit_observe(
    State(node): State<NodeState>,
    Json(body): Json<DepositObserveBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::ObserveDeposit {
            intent_id: body.intent_id,
            txid: body.txid,
            amount_sats: body.amount_sats,
        },
    )
    .await
}

async fn deposit_confirm(
    State(node): State<NodeState>,
    Json(body): Json<DepositConfirmBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::ConfirmDeposit {
            intent_id: body.intent_id,
        },
    )
    .await
}

async fn split(
    State(node): State<NodeState>,
    Json(body): Json<SplitBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::SplitDepositIntoNotes {
            deposit_id: body.deposit_id,
            note_commitments: body.note_commitments,
        },
    )
    .await
}

async fn withdraw(
    State(node): State<NodeState>,
    Json(body): Json<WithdrawBody>,
) -> ApiResult<EventsResponse> {
    if node.consensus.is_some() {
        return execute(
            &node,
            Command::RequestWithdrawal {
                proof: body.proof,
                public: body.public,
            },
        )
        .await;
    }
    execute(
        &node,
        Command::WithdrawNote {
            proof: body.proof,
            public: body.public,
        },
    )
    .await
}

async fn withdraw_authorize(
    State(node): State<NodeState>,
    Json(body): Json<AuthorizeWithdrawalBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::AuthorizeWithdrawal {
            withdrawal_id: body.withdrawal_id,
            signature: body.signature,
        },
    )
    .await
}

async fn node_register(
    State(node): State<NodeState>,
    Json(body): Json<RegisterNodeBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::RegisterNode {
            node_id: body.node_id,
            bond_address: body.bond_address,
            consensus_pubkey: body.consensus_pubkey,
            signer_pubkey: body.signer_pubkey,
        },
    )
    .await
}

async fn node_bond(
    State(node): State<NodeState>,
    Json(body): Json<BondNodeBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::BondNode {
            node_id: body.node_id,
            amount_sats: body.amount_sats,
        },
    )
    .await
}

async fn node_slot_assign(
    State(node): State<NodeState>,
    Json(body): Json<AssignNodeSlotBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::AssignNodeSlot {
            node_id: body.node_id,
            slot_id: body.slot_id,
        },
    )
    .await
}

async fn node_bond_parameters(
    State(node): State<NodeState>,
    Json(body): Json<BondParametersBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::SetBondParameters {
            min_bond_sats: body.min_bond_sats,
            min_bond_increase_sats: body.min_bond_increase_sats,
        },
    )
    .await
}

async fn churn_start(State(node): State<NodeState>) -> ApiResult<EventsResponse> {
    if node.consensus.is_some() {
        return execute(&node, Command::StartChurnEpoch).await;
    }
    if node.node_id.is_some() && !node.peers.lock().await.is_empty() {
        return execute_churn_with_http_frost(&node).await;
    }
    execute(&node, Command::StartChurnEpoch).await
}

async fn custody_keyset_commit(
    State(node): State<NodeState>,
    Json(body): Json<CommitKeysetBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::CommitCustodyKeyset {
            epoch: body.epoch,
            keyset: body.keyset,
        },
    )
    .await
}

async fn frost_keygen(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeygenBody>,
) -> ApiResult<EventsResponse> {
    let epoch = match body.epoch {
        Some(epoch) => epoch,
        None => node.inner.lock().await.churn.epoch,
    };
    coordinate_http_frost_keygen(&node, epoch).await
}

async fn frost_keygen_round1(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeygenRound1Body>,
) -> ApiResult<thornado_core::FrostDkgRound1Public> {
    let output =
        FrostCustodySigner::dkg_round1(body.participant_index, body.signer_count, body.threshold)
            .map_err(ApiError::bad_request)?;
    if output.signer_id != body.signer_id {
        return Err(ApiError::bad_request(CoreError::Frost(
            "FROST DKG signer id does not match participant index".to_string(),
        )));
    }
    node.dkg_round1.lock().await.insert(
        body.session_id,
        DkgRound1State {
            signer_id: output.signer_id.clone(),
            secret_package: output.secret_package,
        },
    );
    Ok(Json(thornado_core::FrostDkgRound1Public {
        signer_id: output.signer_id,
        package: output.package,
    }))
}

async fn frost_keygen_round2(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeygenRound2Body>,
) -> ApiResult<thornado_core::FrostDkgRound2Public> {
    let state = node
        .dkg_round1
        .lock()
        .await
        .remove(&body.session_id)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing FROST DKG round1 state".to_string(),
            ))
        })?;
    let output = FrostCustodySigner::dkg_round2(
        &state.signer_id,
        &state.secret_package,
        &body.round1_packages,
    )
    .map_err(ApiError::bad_request)?;
    node.dkg_round2.lock().await.insert(
        body.session_id,
        DkgRound2State {
            signer_id: output.signer_id.clone(),
            secret_package: output.secret_package,
        },
    );
    Ok(Json(thornado_core::FrostDkgRound2Public {
        signer_id: output.signer_id,
        packages: output.packages,
    }))
}

async fn frost_keygen_finalize(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeygenFinalizeBody>,
) -> ApiResult<FrostKeyset> {
    let state = node
        .dkg_round2
        .lock()
        .await
        .remove(&body.session_id)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing FROST DKG round2 state".to_string(),
            ))
        })?;
    let signer = FrostCustodySigner::dkg_finalize_single(
        &state.signer_id,
        &state.secret_package,
        &body.round1_packages,
        &body.round2_packages,
    )
    .map_err(ApiError::bad_request)?;
    let keyset = signer
        .to_keyset(body.epoch)
        .map_err(ApiError::bad_request)?;
    if let Some(path) = node.frost_signer_path.as_ref() {
        save_frost_signer(path, &signer).map_err(ApiError::internal)?;
    }
    *node.custody_signer.lock().await = Some(signer);
    Ok(Json(keyset))
}

async fn frost_keysign_commitment(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeysignCommitmentBody>,
) -> ApiResult<FrostSigningCommitmentPublic> {
    let commitment = {
        let signer = node.custody_signer.lock().await;
        let signer = signer.as_ref().ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost("missing local FROST signer".to_string()))
        })?;
        let signer_id = match body.signer_id {
            Some(signer_id) => signer_id,
            None => signer.first_signer_id().map_err(ApiError::bad_request)?,
        };
        signer
            .signing_commitment(&signer_id)
            .map_err(ApiError::bad_request)?
    };
    node.frost_nonces.lock().await.insert(
        frost_nonce_key(&body.session_id, &commitment.signer_id),
        commitment.nonces,
    );
    Ok(Json(FrostSigningCommitmentPublic {
        signer_id: commitment.signer_id,
        commitment: commitment.commitment,
    }))
}

async fn frost_keysign_share(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeysignShareBody>,
) -> ApiResult<FrostSignatureShare> {
    let nonce_key = frost_nonce_key(&body.session_id, &body.signer_id);
    let nonces = node
        .frost_nonces
        .lock()
        .await
        .remove(&nonce_key)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing FROST nonces for keysign session".to_string(),
            ))
        })?;
    let signer = node.custody_signer.lock().await;
    let signer = signer.as_ref().ok_or_else(|| {
        ApiError::bad_request(CoreError::Frost("missing local FROST signer".to_string()))
    })?;
    let share = signer
        .signature_share(
            &body.signer_id,
            &nonces,
            &body.request,
            &body.commitments,
            body.key_tweak.as_deref(),
        )
        .map_err(ApiError::bad_request)?;
    Ok(Json(share))
}

async fn frost_keysign(
    State(node): State<NodeState>,
    Json(body): Json<FrostKeysignBody>,
) -> ApiResult<CustodySignature> {
    let signature = coordinate_http_frost_keysign(&node, body.request, body.key_tweak).await?;
    Ok(Json(signature))
}

async fn churn_standby(
    State(node): State<NodeState>,
    Json(body): Json<ChurnNodeBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::RegisterStandbyNode {
            node_id: body.node_id,
        },
    )
    .await
}

async fn churn_offline(
    State(node): State<NodeState>,
    Json(body): Json<OfflineBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::MarkNodeOffline {
            node_id: body.node_id,
        },
    )
    .await
}

async fn churn_penalties(State(node): State<NodeState>) -> ApiResult<EventsResponse> {
    execute(&node, Command::ApplyChurnPenalties).await
}

async fn apply_events(
    State(node): State<NodeState>,
    Json(body): Json<ApplyEventsBody>,
) -> ApiResult<EventsResponse> {
    let mut state = node.inner.lock().await;
    for event in body.events.iter().cloned() {
        apply_event(&mut state, event).map_err(ApiError::bad_request)?;
    }
    persist_state(&node, &state)?;
    Ok(Json(EventsResponse {
        events: body.events,
        state_hash: state_hash(&state),
        tx_hash: None,
    }))
}

async fn peers(State(node): State<NodeState>) -> ApiResult<PeersResponse> {
    Ok(Json(PeersResponse {
        peers: node.peers.lock().await.clone(),
    }))
}

async fn peer_add(
    State(node): State<NodeState>,
    Json(body): Json<PeerBody>,
) -> ApiResult<PeersResponse> {
    let mut peers = node.peers.lock().await;
    let url = body.url.trim_end_matches('/').to_string();
    if !peers.iter().any(|peer| peer == &url) {
        peers.push(url);
    }
    Ok(Json(PeersResponse {
        peers: peers.clone(),
    }))
}

async fn bitcoin_utxo_import(
    State(node): State<NodeState>,
    Json(body): Json<RegtestUtxo>,
) -> ApiResult<ImportUtxoResponse> {
    let (utxo_count, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        vault
            .import_dev_utxo(body.clone())
            .map_err(ApiError::bitcoin_bad_request)?;
        (vault.list_utxos().len(), vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(ImportUtxoResponse {
        imported: body,
        utxo_count,
    }))
}

async fn bitcoin_utxos(State(node): State<NodeState>) -> ApiResult<UtxosResponse> {
    let vault = node.bitcoin.lock().await;
    Ok(Json(UtxosResponse {
        utxos: vault.list_utxos(),
    }))
}

async fn bitcoin_withdrawal_build(
    State(node): State<NodeState>,
    Json(body): Json<BuildBitcoinWithdrawalBody>,
) -> ApiResult<BuiltWithdrawal> {
    let withdrawal = {
        let state = node.inner.lock().await;
        state
            .withdrawals
            .authorized
            .get(&body.withdrawal_id)
            .cloned()
            .ok_or_else(|| ApiError::bad_request(thornado_core::Error::UnknownWithdrawal))?
    };

    let request = BitcoinWithdrawalRequest {
        withdrawal_id: withdrawal.id,
        recipient: withdrawal.recipient,
        amount_sats: withdrawal.amount_sats,
        fee_rate_sats_per_vb: body
            .fee_rate_sats_per_vb
            .unwrap_or(thornado_bitcoin::DEFAULT_FEE_RATE_SATS_PER_VB),
        change_script_pubkey_hex: body.change_script_pubkey_hex,
        max_fee_rate_sats_per_vb: body.max_fee_rate_sats_per_vb,
        min_relay_fee_sats: body.min_relay_fee_sats,
        max_inputs: body.max_inputs,
        min_confirmations: body.min_confirmations,
        max_mempool_chain_length: body.max_mempool_chain_length,
    };

    let (built, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let built = vault
            .build_withdrawal(request)
            .map_err(ApiError::bitcoin_bad_request)?;
        (built, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(built))
}

async fn bitcoin_withdrawal_get(
    State(node): State<NodeState>,
    Path(id): Path<String>,
) -> ApiResult<BitcoinWithdrawalRecord> {
    let vault = node.bitcoin.lock().await;
    let record = vault
        .get_withdrawal(&id)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(record))
}

async fn bitcoin_withdrawal_mark_broadcast(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<MarkBroadcastBody>,
) -> ApiResult<BitcoinWithdrawalRecord> {
    let (record, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let record = vault
            .mark_broadcast(&id, body.txid)
            .map_err(ApiError::bitcoin_bad_request)?;
        (record, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(record))
}

async fn bitcoin_withdrawal_broadcast(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<BroadcastBitcoinWithdrawalBody>,
) -> ApiResult<BitcoinWithdrawalRecord> {
    let (record, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let record = vault
            .broadcast_withdrawal(&id, body.signed_tx_hex)
            .map_err(ApiError::bitcoin_bad_request)?;
        (record, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(record))
}

async fn bitcoin_withdrawal_checkpoint_validate(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<ValidateBitcoinCheckpointBody>,
) -> ApiResult<SigningCheckpointValidation> {
    let vault = node.bitcoin.lock().await;
    let validation = vault
        .validate_signing_checkpoint(&id, body.unsigned_tx_hex)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(validation))
}

async fn bitcoin_withdrawal_mark_confirmed(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<MarkConfirmedBody>,
) -> ApiResult<BitcoinWithdrawalRecord> {
    let (record, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let record = vault
            .mark_confirmed(&id, body.height)
            .map_err(ApiError::bitcoin_bad_request)?;
        (record, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(record))
}

async fn bitcoin_solvency(
    State(node): State<NodeState>,
    Json(body): Json<BitcoinSolvencyBody>,
) -> ApiResult<BitcoinSolvencyReport> {
    let vault = node.bitcoin.lock().await;
    let report = vault
        .report_solvency(body.expected_sats)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(report))
}

async fn bitcoin_broadcast_submit(
    State(node): State<NodeState>,
    Json(body): Json<SubmitBitcoinBroadcastBody>,
) -> ApiResult<EventsResponse> {
    execute(
        &node,
        Command::SubmitBitcoinBroadcast {
            withdrawal_id: body.withdrawal_id,
            txid: body.txid,
        },
    )
    .await
}

async fn bitcoin_consolidation_build(
    State(node): State<NodeState>,
    Json(body): Json<BuildBitcoinConsolidationBody>,
) -> ApiResult<BuiltConsolidation> {
    let request = BitcoinConsolidationRequest {
        consolidation_id: body.consolidation_id,
        fee_rate_sats_per_vb: body
            .fee_rate_sats_per_vb
            .unwrap_or(thornado_bitcoin::DEFAULT_FEE_RATE_SATS_PER_VB),
        change_script_pubkey_hex: body.change_script_pubkey_hex,
        min_utxos: body.min_utxos,
        max_inputs: body.max_inputs,
        min_confirmations: body.min_confirmations,
        max_mempool_chain_length: body.max_mempool_chain_length,
        max_fee_rate_sats_per_vb: body.max_fee_rate_sats_per_vb,
        min_relay_fee_sats: body.min_relay_fee_sats,
    };
    let (built, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let built = vault
            .build_consolidation(request)
            .map_err(ApiError::bitcoin_bad_request)?;
        (built, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(built))
}

async fn bitcoin_consolidation_get(
    State(node): State<NodeState>,
    Path(id): Path<String>,
) -> ApiResult<BitcoinConsolidationRecord> {
    let vault = node.bitcoin.lock().await;
    let record = vault
        .get_consolidation(&id)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(record))
}

async fn bitcoin_consolidation_broadcast(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<BroadcastBitcoinWithdrawalBody>,
) -> ApiResult<BitcoinConsolidationRecord> {
    let (record, snapshot) = {
        let mut vault = node.bitcoin.lock().await;
        let record = vault
            .broadcast_consolidation(&id, body.signed_tx_hex)
            .map_err(ApiError::bitcoin_bad_request)?;
        (record, vault.clone())
    };
    persist_bitcoin(&node, &snapshot)?;
    Ok(Json(record))
}

async fn get_state_hash(State(node): State<NodeState>) -> ApiResult<StateHashResponse> {
    let state = node.inner.lock().await;
    Ok(Json(StateHashResponse {
        state_hash: state_hash(&state),
    }))
}

async fn bifrost_work_keygen(State(node): State<NodeState>) -> ApiResult<BifrostKeygenWork> {
    let state = node.inner.lock().await;
    let epoch = state.churn.epoch;
    let participants = state.churn.active_nodes.iter().cloned().collect::<Vec<_>>();
    let pending = participants.len() >= 2 && !state.custody.keysets.contains_key(&epoch);
    let threshold = if pending {
        thornado_core::frost_threshold_for_committee(participants.len() as u16)
    } else {
        0
    };
    Ok(Json(BifrostKeygenWork {
        pending,
        epoch,
        participants,
        threshold,
    }))
}

async fn bifrost_work_withdrawals(
    State(node): State<NodeState>,
) -> ApiResult<BifrostWithdrawalsWork> {
    let state = node.inner.lock().await;
    Ok(Json(BifrostWithdrawalsWork {
        pending: state.withdrawals.pending.values().cloned().collect(),
    }))
}

async fn bifrost_work_bitcoin(State(node): State<NodeState>) -> ApiResult<BifrostBitcoinWork> {
    let state = node.inner.lock().await;
    Ok(Json(BifrostBitcoinWork {
        authorized: state
            .withdrawals
            .authorized
            .values()
            .filter(|withdrawal| {
                !state
                    .withdrawals
                    .bitcoin_broadcasts
                    .contains_key(&withdrawal.id)
            })
            .cloned()
            .collect(),
    }))
}

async fn bifrost_tick(State(node): State<NodeState>) -> ApiResult<EventsResponse> {
    let mut events = Vec::new();
    let mut tx_hash = None;
    let keygen_work = {
        let state = node.inner.lock().await;
        state.churn.active_nodes.len() >= 2
            && !state.custody.keysets.contains_key(&state.churn.epoch)
    };
    if keygen_work {
        let epoch = node.inner.lock().await.churn.epoch;
        let Json(response) = coordinate_http_frost_keygen(&node, epoch).await?;
        events.extend(response.events);
        tx_hash = response.tx_hash;
    }

    let pending = {
        let state = node.inner.lock().await;
        state.withdrawals.pending.values().cloned().collect::<Vec<_>>()
    };
    for withdrawal in pending {
        let request = WithdrawalRequest {
            withdrawal_id: withdrawal.id.clone(),
            recipient: withdrawal.recipient.clone(),
            amount_sats: withdrawal.amount_sats,
            fee_sats: withdrawal.fee_sats,
            nullifier_hash: withdrawal.nullifier_hash.clone(),
        };
        let signature = coordinate_http_frost_keysign(&node, request, None).await?;
        let Json(response) = execute(
            &node,
            Command::AuthorizeWithdrawal {
                withdrawal_id: withdrawal.id,
                signature,
            },
        )
        .await?;
        events.extend(response.events);
        tx_hash = response.tx_hash.or(tx_hash);
    }

    let state = node.inner.lock().await;
    Ok(Json(EventsResponse {
        events,
        state_hash: state_hash(&state),
        tx_hash,
    }))
}

async fn get_note_root(
    State(node): State<NodeState>,
    Path(denomination): Path<u64>,
) -> ApiResult<RootResponse> {
    let state = node.inner.lock().await;
    let tree = state
        .notes
        .trees
        .get(&denomination)
        .ok_or(ApiError::bad_request(
            thornado_core::Error::UnknownDenomination,
        ))?;

    Ok(Json(RootResponse {
        denomination_sats: denomination,
        root: tree.root(),
    }))
}

async fn execute(node: &NodeState, command: Command) -> ApiResult<EventsResponse> {
    if let Some(consensus) = node.consensus.as_ref() {
        let tx_hash = consensus.broadcast_tx_commit(command).await?;
        let state = node.inner.lock().await;
        return Ok(Json(EventsResponse {
            events: Vec::new(),
            state_hash: state_hash(&state),
            tx_hash: Some(tx_hash),
        }));
    }

    let mut state = node.inner.lock().await;
    let mut custody_signer = node.custody_signer.lock().await;
    let events = execute_command_secure(
        &mut state,
        &mut custody_signer,
        node.frost_signer_path.as_ref(),
        command,
    )?;
    persist_state(node, &state)?;
    let response = EventsResponse {
        events: events.clone(),
        state_hash: state_hash(&state),
        tx_hash: None,
    };
    drop(state);
    replicate_events(node, &events).await?;
    Ok(Json(response))
}

fn persist_state(node: &NodeState, state: &AppState) -> Result<(), ApiError> {
    if let Some(path) = node.snapshot_path.as_ref() {
        save_snapshot(state, path).map_err(ApiError::internal)?;
    }
    Ok(())
}

fn persist_bitcoin(node: &NodeState, backend: &NodeBitcoinBackend) -> Result<(), ApiError> {
    if let Some(path) = node.bitcoin_state_path.as_ref() {
        save_bitcoin_backend(path, backend).map_err(ApiError::internal)?;
    }
    Ok(())
}

fn load_bitcoin_backend(path: &PathBuf) -> thornado_core::Result<NodeBitcoinBackend> {
    if let Ok(json) = fs::read_to_string(path) {
        if json.trim_start().starts_with('{') {
            return serde_json::from_str(&json).map_err(|e| CoreError::Json(e.to_string()));
        }
    }
    let store = RedbKvStore::open(path).map_err(store_error)?;
    get_json(&store, BITCOIN_BACKEND_KEY)
        .map_err(store_error)?
        .ok_or_else(|| CoreError::Io("bitcoin backend state not found".to_string()))
}

fn save_bitcoin_backend(path: &PathBuf, backend: &NodeBitcoinBackend) -> thornado_core::Result<()> {
    let store = RedbKvStore::open(path).map_err(store_error)?;
    put_json(&store, BITCOIN_BACKEND_KEY, backend).map_err(store_error)
}

fn store_error(error: thornado_store::Error) -> CoreError {
    CoreError::Io(error.to_string())
}

fn load_frost_signer(path: &PathBuf) -> thornado_core::Result<FrostCustodySigner> {
    let json = fs::read_to_string(path).map_err(|e| CoreError::Io(e.to_string()))?;
    let snapshot: FrostCustodySignerSnapshot =
        serde_json::from_str(&json).map_err(|e| CoreError::Json(e.to_string()))?;
    FrostCustodySigner::from_snapshot(&snapshot)
}

fn save_frost_signer(path: &PathBuf, signer: &FrostCustodySigner) -> thornado_core::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| CoreError::Io(e.to_string()))?;
    }
    let snapshot = signer.to_snapshot()?;
    let json = serde_json::to_vec(&snapshot).map_err(|e| CoreError::Json(e.to_string()))?;
    let mut options = fs::OpenOptions::new();
    options.create(true).truncate(true).write(true);
    #[cfg(unix)]
    options.mode(0o600);
    use std::io::Write;
    let mut file = options
        .open(path)
        .map_err(|e| CoreError::Io(e.to_string()))?;
    file.write_all(&json)
        .map_err(|e| CoreError::Io(e.to_string()))
}

async fn replicate_events(node: &NodeState, events: &[Event]) -> Result<(), ApiError> {
    let peers = node.peers.lock().await.clone();
    if events.is_empty() || peers.is_empty() {
        return Ok(());
    }
    let body = ApplyEventsBody {
        events: events.to_vec(),
    };
    for peer in peers.iter() {
        let url = format!("{}/events/apply", peer.trim_end_matches('/'));
        let response = node
            .client
            .post(&url)
            .json(&body)
            .send()
            .await
            .map_err(ApiError::replication)?;
        if !response.status().is_success() {
            return Err(ApiError::replication_status(
                url,
                response.status(),
                response.text().await.unwrap_or_default(),
            ));
        }
    }
    Ok(())
}

async fn coordinate_http_frost_keygen(node: &NodeState, epoch: u64) -> ApiResult<EventsResponse> {
    let participants = ceremony_participants(node).await?;
    let session_id = frost_ceremony_id("keygen", epoch, "", &participants);
    let leader = leader_node(&session_id, epoch, &participants)?;
    let local = node.node_id.as_ref().ok_or_else(|| {
        ApiError::bad_request(CoreError::Frost(
            "node_id is required for FROST keygen leadership".to_string(),
        ))
    })?;
    if local != &leader {
        let response = node
            .client
            .post(format!("{}/frost/keygen", leader.trim_end_matches('/')))
            .json(&FrostKeygenBody { epoch: Some(epoch) })
            .send()
            .await
            .map_err(ApiError::replication)?;
        if !response.status().is_success() {
            return Err(ApiError::replication_status(
                leader,
                response.status(),
                response.text().await.unwrap_or_default(),
            ));
        }
        return response
            .json::<EventsResponse>()
            .await
            .map(Json)
            .map_err(ApiError::replication);
    }

    let active_count = {
        let state = node.inner.lock().await;
        if state.churn.epoch != epoch {
            return Err(ApiError::bad_request(CoreError::Frost(
                "FROST keygen epoch must match current churn epoch".to_string(),
            )));
        }
        active_signer_count(&state)
    };
    let signer_count = u16::try_from(participants.len()).map_err(|_| {
        ApiError::bad_request(CoreError::Frost(
            "too many FROST keygen participants".to_string(),
        ))
    })?;
    if signer_count != active_count {
        return Err(ApiError::bad_request(CoreError::Frost(format!(
            "FROST keygen participants must match active set: got {signer_count}, expected {active_count}"
        ))));
    }
    let threshold = thornado_core::frost_threshold_for_committee(signer_count);
    let mut round1 = Vec::new();
    for (index, participant) in participants.iter().enumerate() {
        let participant_index = u16::try_from(index + 1).map_err(|_| {
            ApiError::bad_request(CoreError::Frost(
                "invalid FROST participant index".to_string(),
            ))
        })?;
        let signer_id = thornado_core::frost_signer_id_for_index(participant_index)
            .map_err(ApiError::bad_request)?;
        let body = FrostKeygenRound1Body {
            session_id: session_id.clone(),
            signer_id,
            participant_index,
            signer_count,
            threshold,
        };
        if participant == local {
            round1.push(frost_keygen_round1_local(node, body).await?);
        } else {
            round1.push(post_peer_json(node, participant, "/frost/keygen/round1", &body).await?);
        }
    }

    let mut round2 = Vec::new();
    for participant in &participants {
        let body = FrostKeygenRound2Body {
            session_id: session_id.clone(),
            round1_packages: round1.clone(),
        };
        if participant == local {
            round2.push(frost_keygen_round2_local(node, body).await?);
        } else {
            round2.push(post_peer_json(node, participant, "/frost/keygen/round2", &body).await?);
        }
    }

    let mut keysets = Vec::new();
    for participant in &participants {
        let body = FrostKeygenFinalizeBody {
            session_id: session_id.clone(),
            epoch,
            round1_packages: round1.clone(),
            round2_packages: round2.clone(),
        };
        if participant == local {
            keysets.push(frost_keygen_finalize_local(node, body).await?);
        } else {
            keysets.push(post_peer_json(node, participant, "/frost/keygen/finalize", &body).await?);
        }
    }
    let keyset = keysets
        .first()
        .cloned()
        .ok_or_else(|| ApiError::bad_request(CoreError::Frost("empty keygen".to_string())))?;
    if keysets.iter().any(|candidate| candidate != &keyset) {
        return Err(ApiError::bad_request(CoreError::Frost(
            "FROST DKG nodes produced different keysets".to_string(),
        )));
    }

    if let Some(consensus) = node.consensus.as_ref() {
        let tx_hash = consensus
            .broadcast_tx_commit(Command::CommitCustodyKeyset { epoch, keyset })
            .await?;
        let state = node.inner.lock().await;
        return Ok(Json(EventsResponse {
            events: Vec::new(),
            state_hash: state_hash(&state),
            tx_hash: Some(tx_hash),
        }));
    }

    let mut state = node.inner.lock().await;
    let event = Event::CustodyKeysetGenerated { epoch, keyset };
    apply_event(&mut state, event.clone()).map_err(ApiError::bad_request)?;
    persist_state(node, &state)?;
    let response = EventsResponse {
        events: vec![event.clone()],
        state_hash: state_hash(&state),
        tx_hash: None,
    };
    drop(state);
    replicate_events(node, &[event]).await?;
    Ok(Json(response))
}

async fn frost_keygen_round1_local(
    node: &NodeState,
    body: FrostKeygenRound1Body,
) -> Result<thornado_core::FrostDkgRound1Public, ApiError> {
    let output =
        FrostCustodySigner::dkg_round1(body.participant_index, body.signer_count, body.threshold)
            .map_err(ApiError::bad_request)?;
    if output.signer_id != body.signer_id {
        return Err(ApiError::bad_request(CoreError::Frost(
            "FROST DKG signer id does not match participant index".to_string(),
        )));
    }
    node.dkg_round1.lock().await.insert(
        body.session_id,
        DkgRound1State {
            signer_id: output.signer_id.clone(),
            secret_package: output.secret_package,
        },
    );
    Ok(thornado_core::FrostDkgRound1Public {
        signer_id: output.signer_id,
        package: output.package,
    })
}

async fn frost_keygen_round2_local(
    node: &NodeState,
    body: FrostKeygenRound2Body,
) -> Result<thornado_core::FrostDkgRound2Public, ApiError> {
    let state = node
        .dkg_round1
        .lock()
        .await
        .remove(&body.session_id)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing FROST DKG round1 state".to_string(),
            ))
        })?;
    let output = FrostCustodySigner::dkg_round2(
        &state.signer_id,
        &state.secret_package,
        &body.round1_packages,
    )
    .map_err(ApiError::bad_request)?;
    node.dkg_round2.lock().await.insert(
        body.session_id,
        DkgRound2State {
            signer_id: output.signer_id.clone(),
            secret_package: output.secret_package,
        },
    );
    Ok(thornado_core::FrostDkgRound2Public {
        signer_id: output.signer_id,
        packages: output.packages,
    })
}

async fn frost_keygen_finalize_local(
    node: &NodeState,
    body: FrostKeygenFinalizeBody,
) -> Result<FrostKeyset, ApiError> {
    let state = node
        .dkg_round2
        .lock()
        .await
        .remove(&body.session_id)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing FROST DKG round2 state".to_string(),
            ))
        })?;
    let signer = FrostCustodySigner::dkg_finalize_single(
        &state.signer_id,
        &state.secret_package,
        &body.round1_packages,
        &body.round2_packages,
    )
    .map_err(ApiError::bad_request)?;
    let keyset = signer
        .to_keyset(body.epoch)
        .map_err(ApiError::bad_request)?;
    if let Some(path) = node.frost_signer_path.as_ref() {
        save_frost_signer(path, &signer).map_err(ApiError::internal)?;
    }
    *node.custody_signer.lock().await = Some(signer);
    Ok(keyset)
}

async fn post_peer_json<T, B>(
    node: &NodeState,
    peer: &str,
    path: &str,
    body: &B,
) -> Result<T, ApiError>
where
    T: for<'de> Deserialize<'de>,
    B: Serialize + ?Sized,
{
    let url = format!("{}{}", peer.trim_end_matches('/'), path);
    let response = node
        .client
        .post(&url)
        .json(body)
        .send()
        .await
        .map_err(ApiError::replication)?;
    if !response.status().is_success() {
        return Err(ApiError::replication_status(
            url,
            response.status(),
            response.text().await.unwrap_or_default(),
        ));
    }
    response.json::<T>().await.map_err(ApiError::replication)
}

async fn ceremony_participants(node: &NodeState) -> Result<Vec<String>, ApiError> {
    let local = node.node_id.clone().ok_or_else(|| {
        ApiError::bad_request(CoreError::Frost(
            "node_id is required for FROST ceremony".to_string(),
        ))
    })?;
    let mut participants = node.peers.lock().await.clone();
    participants.push(local);
    participants.sort();
    participants.dedup();
    Ok(participants)
}

fn frost_ceremony_id(kind: &str, epoch: u64, key: &str, participants: &[String]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(kind.as_bytes());
    hasher.update(epoch.to_be_bytes());
    hasher.update(key.as_bytes());
    for participant in participants {
        hasher.update((participant.len() as u64).to_be_bytes());
        hasher.update(participant.as_bytes());
    }
    hex::encode(hasher.finalize())
}

fn leader_node(msg_id: &str, epoch: u64, participants: &[String]) -> Result<String, ApiError> {
    if msg_id.is_empty() || participants.is_empty() {
        return Err(ApiError::bad_request(CoreError::Frost(
            "invalid FROST leader inputs".to_string(),
        )));
    }
    participants
        .iter()
        .map(|participant| {
            let mut hasher = Sha256::new();
            hasher.update(msg_id.as_bytes());
            hasher.update(epoch.to_string().as_bytes());
            hasher.update(participant.as_bytes());
            (hex::encode(hasher.finalize()), participant.clone())
        })
        .min_by(|left, right| left.0.cmp(&right.0))
        .map(|(_, participant)| participant)
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost("empty FROST participant set".to_string()))
        })
}

async fn coordinate_http_frost_keysign(
    node: &NodeState,
    request: WithdrawalRequest,
    key_tweak: Option<String>,
) -> Result<CustodySignature, ApiError> {
    let (coordinator, threshold, local_signer_id) = {
        let signer = node.custody_signer.lock().await;
        let signer = signer.as_ref().ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost("missing local FROST signer".to_string()))
        })?;
        (
            signer.coordinator(),
            signer
                .to_keyset(0)
                .map_err(ApiError::bad_request)?
                .threshold as usize,
            signer.first_signer_id().map_err(ApiError::bad_request)?,
        )
    };
    let session_id = format!("{}:{}", request.withdrawal_id, request.nullifier_hash);
    let local_commitment = {
        let signer = node.custody_signer.lock().await;
        let signer = signer.as_ref().expect("checked above");
        signer
            .signing_commitment(&local_signer_id)
            .map_err(ApiError::bad_request)?
    };
    node.frost_nonces.lock().await.insert(
        frost_nonce_key(&session_id, &local_commitment.signer_id),
        local_commitment.nonces,
    );
    let mut commitments = vec![FrostSigningCommitmentPublic {
        signer_id: local_commitment.signer_id,
        commitment: local_commitment.commitment,
    }];
    let peers = node.peers.lock().await.clone();
    let mut selected_peers = Vec::new();
    for peer in peers {
        if commitments.len() >= threshold {
            break;
        }
        let url = format!("{}/frost/keysign/commitment", peer.trim_end_matches('/'));
        let response = node
            .client
            .post(&url)
            .json(&FrostKeysignCommitmentBody {
                session_id: session_id.clone(),
                signer_id: None,
            })
            .send()
            .await
            .map_err(ApiError::replication)?;
        if !response.status().is_success() {
            return Err(ApiError::replication_status(
                url,
                response.status(),
                response.text().await.unwrap_or_default(),
            ));
        }
        let commitment = response
            .json::<FrostSigningCommitmentPublic>()
            .await
            .map_err(ApiError::replication)?;
        selected_peers.push(peer);
        commitments.push(commitment);
    }
    if commitments.len() < threshold {
        return Err(ApiError::bad_request(CoreError::Frost(format!(
            "insufficient FROST peers: need {threshold}, got {}",
            commitments.len()
        ))));
    }

    let local_nonces = node
        .frost_nonces
        .lock()
        .await
        .remove(&frost_nonce_key(&session_id, &local_signer_id))
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "missing local FROST nonces for keysign session".to_string(),
            ))
        })?;
    let local_share = {
        let signer = node.custody_signer.lock().await;
        let signer = signer.as_ref().expect("checked above");
        signer
            .signature_share(
                &local_signer_id,
                &local_nonces,
                &request,
                &commitments,
                key_tweak.as_deref(),
            )
            .map_err(ApiError::bad_request)?
    };
    let mut shares = vec![local_share];
    for (peer, commitment) in selected_peers.into_iter().zip(commitments.iter().skip(1)) {
        let url = format!("{}/frost/keysign/share", peer.trim_end_matches('/'));
        let response = node
            .client
            .post(&url)
            .json(&FrostKeysignShareBody {
                session_id: session_id.clone(),
                signer_id: commitment.signer_id.clone(),
                request: request.clone(),
                commitments: commitments.clone(),
                key_tweak: key_tweak.clone(),
            })
            .send()
            .await
            .map_err(ApiError::replication)?;
        if !response.status().is_success() {
            return Err(ApiError::replication_status(
                url,
                response.status(),
                response.text().await.unwrap_or_default(),
            ));
        }
        shares.push(
            response
                .json::<FrostSignatureShare>()
                .await
                .map_err(ApiError::replication)?,
        );
    }
    coordinator
        .aggregate_signature_shares(&request, &commitments, &shares, key_tweak.as_deref())
        .map_err(ApiError::bad_request)
}

fn frost_nonce_key(session_id: &str, signer_id: &str) -> String {
    format!("{session_id}:{signer_id}")
}

pub async fn execute_local(node: &NodeState, command: Command) -> ApiResult<EventsResponse> {
    if let Command::WithdrawNote { proof, public } = command {
        if !node.peers.lock().await.is_empty() {
            return execute_withdraw_with_http_frost(node, proof, public).await;
        }
        return execute_local_locked(node, Command::WithdrawNote { proof, public }).await;
    }
    execute_local_locked(node, command).await
}

async fn execute_local_locked(node: &NodeState, command: Command) -> ApiResult<EventsResponse> {
    let mut state = node.inner.lock().await;
    let mut custody_signer = node.custody_signer.lock().await;
    let events = execute_command_secure(
        &mut state,
        &mut custody_signer,
        node.frost_signer_path.as_ref(),
        command,
    )?;
    persist_state(node, &state)?;
    Ok(Json(EventsResponse {
        events,
        state_hash: state_hash(&state),
        tx_hash: None,
    }))
}

async fn execute_churn_with_http_frost(node: &NodeState) -> ApiResult<EventsResponse> {
    let (events, epoch, signer_count, state_hash_after_churn) = {
        let mut state = node.inner.lock().await;
        let events = start_churn_epoch_without_keygen(&mut state).map_err(ApiError::bad_request)?;
        let epoch = state.churn.epoch;
        let signer_count = active_signer_count(&state);
        persist_state(node, &state)?;
        (events, epoch, signer_count, state_hash(&state))
    };
    replicate_events(node, &events).await?;

    if signer_count < 2 {
        return Ok(Json(EventsResponse {
            events,
            state_hash: state_hash_after_churn,
            tx_hash: None,
        }));
    }

    let Json(keygen_response) = coordinate_http_frost_keygen(node, epoch).await?;
    let mut combined_events = events;
    combined_events.extend(keygen_response.events);
    Ok(Json(EventsResponse {
        events: combined_events,
        state_hash: keygen_response.state_hash,
        tx_hash: keygen_response.tx_hash,
    }))
}

async fn execute_withdraw_with_http_frost(
    node: &NodeState,
    proof: WithdrawalProof,
    public: WithdrawalPublicInputs,
) -> ApiResult<EventsResponse> {
    reject_secret_bearing_proof(&proof)?;
    let captured = Arc::new(StdMutex::new(None));
    {
        let state = node.inner.lock().await;
        let mut preview = state.clone();
        execute_command(
            &mut preview,
            Command::WithdrawNote {
                proof: proof.clone(),
                public: public.clone(),
            },
            &StarkProofVerifier,
            &CaptureCustodySigner {
                captured: captured.clone(),
            },
        )
        .map_err(ApiError::bad_request)?;
    }
    let request = captured
        .lock()
        .expect("capture mutex poisoned")
        .clone()
        .ok_or_else(|| {
            ApiError::bad_request(CoreError::Frost(
                "withdrawal did not request a FROST signature".to_string(),
            ))
        })?;
    let signature = coordinate_http_frost_keysign(node, request.clone(), None).await?;
    let mut state = node.inner.lock().await;
    let events = execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &StarkProofVerifier,
        &PrecomputedCustodySigner { request, signature },
    )
    .map_err(ApiError::bad_request)?;
    persist_state(node, &state)?;
    let response = EventsResponse {
        events: events.clone(),
        state_hash: state_hash(&state),
        tx_hash: None,
    };
    drop(state);
    replicate_events(node, &events).await?;
    Ok(Json(response))
}

fn execute_command_secure(
    state: &mut AppState,
    custody_signer: &mut Option<FrostCustodySigner>,
    frost_signer_path: Option<&PathBuf>,
    command: Command,
) -> Result<Vec<Event>, ApiError> {
    match command {
        Command::WithdrawNote { proof, public } => {
            reject_secret_bearing_proof(&proof)?;
            let signer = custody_signer.as_ref().ok_or_else(|| {
                ApiError::bad_request(CoreError::Frost(
                    "missing local FROST signer for active custody keyset".to_string(),
                ))
            })?;
            if signer
                .group_public_key_hex()
                .map_err(ApiError::bad_request)?
                != state.custody.active_group_public_key
            {
                return Err(ApiError::bad_request(CoreError::Frost(
                    "local FROST signer does not match active custody keyset".to_string(),
                )));
            }
            execute_command(
                state,
                Command::WithdrawNote { proof, public },
                &StarkProofVerifier,
                signer,
            )
            .map_err(ApiError::bad_request)
        }
        Command::StartChurnEpoch => {
            start_churn_with_local_frost(state, custody_signer, frost_signer_path)
        }
        command @ (Command::RegisterStandbyNode { .. }
        | Command::MarkNodeOffline { .. }
        | Command::ApplyChurnPenalties) => {
            execute_command(state, command, &MockProofVerifier, &NoopCustodySigner)
                .map_err(ApiError::bad_request)
        }
        command @ Command::RequestDepositAddress { .. } => {
            let mut events = ensure_local_frost_keyset(state, custody_signer, frost_signer_path)?;
            let signer = custody_signer.as_ref().expect("keyset ensured");
            events.extend(
                execute_command(state, command, &MockProofVerifier, signer)
                    .map_err(ApiError::bad_request)?,
            );
            Ok(events)
        }
        command => execute_command(state, command, &MockProofVerifier, &NoopCustodySigner)
            .map_err(ApiError::bad_request),
    }
}

#[derive(Debug, Default, Clone)]
struct NoopCustodySigner;

impl CustodySigner for NoopCustodySigner {
    fn authorize_withdrawal(
        &self,
        _request: &WithdrawalRequest,
    ) -> thornado_core::Result<CustodySignature> {
        Err(CoreError::Frost(
            "withdrawals must use the local FROST signer".to_string(),
        ))
    }
}

#[derive(Clone)]
struct CaptureCustodySigner {
    captured: Arc<StdMutex<Option<WithdrawalRequest>>>,
}

impl CustodySigner for CaptureCustodySigner {
    fn authorize_withdrawal(
        &self,
        request: &WithdrawalRequest,
    ) -> thornado_core::Result<CustodySignature> {
        *self.captured.lock().expect("capture mutex poisoned") = Some(request.clone());
        NoopPreviewCustodySigner.authorize_withdrawal(request)
    }
}

struct NoopPreviewCustodySigner;

impl CustodySigner for NoopPreviewCustodySigner {
    fn authorize_withdrawal(
        &self,
        request: &WithdrawalRequest,
    ) -> thornado_core::Result<CustodySignature> {
        MockCustodySigner.authorize_withdrawal(request)
    }
}

struct PrecomputedCustodySigner {
    request: WithdrawalRequest,
    signature: CustodySignature,
}

impl CustodySigner for PrecomputedCustodySigner {
    fn authorize_withdrawal(
        &self,
        request: &WithdrawalRequest,
    ) -> thornado_core::Result<CustodySignature> {
        if request != &self.request {
            return Err(CoreError::Frost(
                "precomputed FROST signature request mismatch".to_string(),
            ));
        }
        Ok(self.signature.clone())
    }
}

fn ensure_local_frost_keyset(
    state: &mut AppState,
    custody_signer: &mut Option<FrostCustodySigner>,
    frost_signer_path: Option<&PathBuf>,
) -> Result<Vec<Event>, ApiError> {
    if let Some(signer) = custody_signer.as_ref() {
        if signer
            .group_public_key_hex()
            .map_err(ApiError::bad_request)?
            == state.custody.active_group_public_key
            && state
                .custody
                .keysets
                .contains_key(&state.custody.active_epoch)
        {
            return Ok(Vec::new());
        }
    }

    if !state.custody.active_group_public_key.is_empty()
        && state
            .custody
            .keysets
            .contains_key(&state.custody.active_epoch)
    {
        return Err(ApiError::bad_request(CoreError::Frost(
            "active custody keyset exists but local FROST shares are unavailable".to_string(),
        )));
    }

    install_local_frost_keyset(state, custody_signer, frost_signer_path, state.churn.epoch)
        .map(|event| vec![event])
}

fn start_churn_with_local_frost(
    state: &mut AppState,
    custody_signer: &mut Option<FrostCustodySigner>,
    frost_signer_path: Option<&PathBuf>,
) -> Result<Vec<Event>, ApiError> {
    let next_epoch = state.churn.epoch + 1;
    let churn = Event::ChurnEpochStarted { epoch: next_epoch };
    apply_event(state, churn.clone()).map_err(ApiError::bad_request)?;
    let mut events = vec![churn];
    let standby_nodes = state
        .churn
        .standby_nodes
        .iter()
        .cloned()
        .collect::<Vec<_>>();
    let eligible_nodes = state
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
        .collect::<Vec<_>>();
    for node_id in standby_nodes.into_iter().chain(eligible_nodes) {
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
            apply_event(state, ready.clone()).map_err(ApiError::bad_request)?;
            events.push(ready);
        }
        let event = Event::StandbyNodeActivated {
            node_id,
            epoch: next_epoch,
        };
        apply_event(state, event.clone()).map_err(ApiError::bad_request)?;
        events.push(event);
    }
    if active_signer_count(state) >= 2 {
        let keyset =
            install_local_frost_keyset(state, custody_signer, frost_signer_path, next_epoch)?;
        events.push(keyset);
    }
    Ok(events)
}

fn install_local_frost_keyset(
    state: &mut AppState,
    custody_signer: &mut Option<FrostCustodySigner>,
    frost_signer_path: Option<&PathBuf>,
    epoch: u64,
) -> Result<Event, ApiError> {
    let signer_count = active_signer_count(state);
    let signer = FrostCustodySigner::generate_with_dkg(
        signer_count,
        thornado_core::frost_threshold_for_committee(signer_count),
    )
    .map_err(ApiError::bad_request)?;
    let keyset = signer.to_keyset(epoch).map_err(ApiError::bad_request)?;
    let event = Event::CustodyKeysetGenerated { epoch, keyset };
    apply_event(state, event.clone()).map_err(ApiError::bad_request)?;
    if let Some(path) = frost_signer_path {
        save_frost_signer(path, &signer).map_err(ApiError::internal)?;
    }
    *custody_signer = Some(signer);
    Ok(event)
}

fn reject_secret_bearing_proof(proof: &WithdrawalProof) -> Result<(), ApiError> {
    if proof.nullifier.is_empty() && proof.secret.is_empty() && proof.commitment.is_empty() {
        Ok(())
    } else {
        Err(ApiError::bad_request(CoreError::InvalidProof))
    }
}

type ApiResult<T> = Result<Json<T>, ApiError>;

#[derive(Debug)]
pub struct ApiError {
    status: StatusCode,
    message: String,
}

impl ApiError {
    fn bad_request(error: thornado_core::Error) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: error.to_string(),
        }
    }

    fn internal(error: thornado_core::Error) -> Self {
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message: error.to_string(),
        }
    }

    fn bitcoin_bad_request(error: thornado_bitcoin::Error) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: error.to_string(),
        }
    }

    fn replication(error: reqwest::Error) -> Self {
        Self {
            status: StatusCode::BAD_GATEWAY,
            message: format!("peer replication failed: {error}"),
        }
    }

    fn replication_status(url: String, status: StatusCode, body: String) -> Self {
        Self {
            status: StatusCode::BAD_GATEWAY,
            message: format!("peer replication failed for {url}: {status}: {body}"),
        }
    }

    fn consensus_encode(error: thornado_abci::Error) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: format!("consensus transaction encode failed: {error}"),
        }
    }

    fn consensus_rpc(error: reqwest::Error) -> Self {
        Self {
            status: StatusCode::BAD_GATEWAY,
            message: format!("CometBFT RPC failed: {error}"),
        }
    }

    fn consensus_status(url: String, status: StatusCode, body: String) -> Self {
        Self {
            status: StatusCode::BAD_GATEWAY,
            message: format!("CometBFT RPC failed for {url}: {status}: {body}"),
        }
    }

    fn consensus_message(message: String) -> Self {
        Self {
            status: StatusCode::BAD_GATEWAY,
            message: format!("CometBFT rejected transaction: {message}"),
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (
            self.status,
            Json(ErrorResponse {
                error: self.message,
            }),
        )
            .into_response()
    }
}
