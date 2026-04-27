use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::{Html, IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::sync::Arc;
use thornado_bitcoin::{
    BitcoinBackend, BitcoinWithdrawalRecord, BitcoinWithdrawalRequest, BuiltWithdrawal,
    DevBitcoinBackend, RegtestUtxo,
};
use thornado_core::{
    apply_event, execute_command, load_snapshot, save_snapshot, state_hash, AppState, Command,
    Error as CoreError, Event, MockCustodySigner, MockProofVerifier, NoteCommitment, ProofVerifier,
    WithdrawalProof, WithdrawalPublicInputs,
};
use tokio::sync::Mutex;

#[derive(Clone)]
pub struct NodeState {
    inner: Arc<Mutex<AppState>>,
    bitcoin: Arc<Mutex<DevBitcoinBackend>>,
    snapshot_path: Option<PathBuf>,
    peers: Arc<Vec<String>>,
    client: reqwest::Client,
}

impl NodeState {
    pub fn new(state: AppState, snapshot_path: Option<PathBuf>) -> Self {
        Self::with_peers(state, snapshot_path, Vec::new())
    }

    pub fn with_peers(state: AppState, snapshot_path: Option<PathBuf>, peers: Vec<String>) -> Self {
        Self {
            inner: Arc::new(Mutex::new(state)),
            bitcoin: Arc::new(Mutex::new(DevBitcoinBackend::new())),
            snapshot_path,
            peers: Arc::new(peers),
            client: reqwest::Client::new(),
        }
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
}

#[derive(Debug, Serialize, Deserialize)]
pub struct EventsResponse {
    pub events: Vec<Event>,
    pub state_hash: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ApplyEventsBody {
    pub events: Vec<Event>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct StateHashResponse {
    pub state_hash: String,
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
pub struct OfflineBody {
    pub node_id: String,
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
        .route("/churn/start", post(churn_start))
        .route("/churn/offline", post(churn_offline))
        .route("/churn/penalties", post(churn_penalties))
        .route("/events/apply", post(apply_events))
        .route("/bitcoin/utxo/import", post(bitcoin_utxo_import))
        .route("/bitcoin/utxos", get(bitcoin_utxos))
        .route("/bitcoin/withdrawal/build", post(bitcoin_withdrawal_build))
        .route("/bitcoin/withdrawal/:id", get(bitcoin_withdrawal_get))
        .route(
            "/bitcoin/withdrawal/:id/mark-broadcast",
            post(bitcoin_withdrawal_mark_broadcast),
        )
        .route(
            "/bitcoin/withdrawal/:id/mark-confirmed",
            post(bitcoin_withdrawal_mark_confirmed),
        )
        .route("/state/hash", get(get_state_hash))
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
    execute(
        &node,
        Command::WithdrawNote {
            proof: body.proof,
            public: body.public,
        },
    )
    .await
}

async fn churn_start(State(node): State<NodeState>) -> ApiResult<EventsResponse> {
    execute(&node, Command::StartChurnEpoch).await
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
    }))
}

async fn bitcoin_utxo_import(
    State(node): State<NodeState>,
    Json(body): Json<RegtestUtxo>,
) -> ApiResult<ImportUtxoResponse> {
    let mut vault = node.bitcoin.lock().await;
    vault
        .import_dev_utxo(body.clone())
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(ImportUtxoResponse {
        imported: body,
        utxo_count: vault.list_utxos().len(),
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
    };

    let mut vault = node.bitcoin.lock().await;
    let built = vault
        .build_withdrawal(request)
        .map_err(ApiError::bitcoin_bad_request)?;
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
    let mut vault = node.bitcoin.lock().await;
    let record = vault
        .mark_broadcast(&id, body.txid)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(record))
}

async fn bitcoin_withdrawal_mark_confirmed(
    State(node): State<NodeState>,
    Path(id): Path<String>,
    Json(body): Json<MarkConfirmedBody>,
) -> ApiResult<BitcoinWithdrawalRecord> {
    let mut vault = node.bitcoin.lock().await;
    let record = vault
        .mark_confirmed(&id, body.height)
        .map_err(ApiError::bitcoin_bad_request)?;
    Ok(Json(record))
}

async fn get_state_hash(State(node): State<NodeState>) -> ApiResult<StateHashResponse> {
    let state = node.inner.lock().await;
    Ok(Json(StateHashResponse {
        state_hash: state_hash(&state),
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
    let signer = MockCustodySigner;
    let mut state = node.inner.lock().await;
    let events = execute_command_secure(&mut state, command, &signer)?;
    persist_state(node, &state)?;
    let response = EventsResponse {
        events: events.clone(),
        state_hash: state_hash(&state),
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

async fn replicate_events(node: &NodeState, events: &[Event]) -> Result<(), ApiError> {
    if events.is_empty() || node.peers.is_empty() {
        return Ok(());
    }
    let body = ApplyEventsBody {
        events: events.to_vec(),
    };
    for peer in node.peers.iter() {
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

pub async fn execute_local(node: &NodeState, command: Command) -> ApiResult<EventsResponse> {
    let signer = MockCustodySigner;
    let mut state = node.inner.lock().await;
    let events = execute_command_secure(&mut state, command, &signer)?;
    persist_state(node, &state)?;
    Ok(Json(EventsResponse {
        events,
        state_hash: state_hash(&state),
    }))
}

fn execute_command_secure(
    state: &mut AppState,
    command: Command,
    signer: &MockCustodySigner,
) -> Result<Vec<Event>, ApiError> {
    match &command {
        Command::WithdrawNote { proof, .. } => {
            reject_secret_bearing_proof(proof)?;
            execute_command(state, command, &PublicProofEnvelopeVerifier, signer)
                .map_err(ApiError::bad_request)
        }
        _ => execute_command(state, command, &MockProofVerifier, signer)
            .map_err(ApiError::bad_request),
    }
}

fn reject_secret_bearing_proof(proof: &WithdrawalProof) -> Result<(), ApiError> {
    if proof.nullifier.is_empty() && proof.secret.is_empty() && proof.commitment.is_empty() {
        Ok(())
    } else {
        Err(ApiError::bad_request(CoreError::InvalidProof))
    }
}

#[derive(Debug, Default, Clone)]
struct PublicProofEnvelopeVerifier;

impl ProofVerifier for PublicProofEnvelopeVerifier {
    fn verify_withdrawal(
        &self,
        proof: &WithdrawalProof,
        public: &WithdrawalPublicInputs,
    ) -> thornado_core::Result<()> {
        if proof.merkle_root != public.merkle_root
            || public.nullifier_hash.is_empty()
            || public.fee_sats >= public.denomination_sats
        {
            return Err(CoreError::InvalidProof);
        }
        Ok(())
    }

    fn reveals_commitment(&self) -> bool {
        false
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
