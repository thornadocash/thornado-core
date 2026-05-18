use std::{
    net::SocketAddr,
    path::PathBuf,
    sync::{Arc, Mutex},
};

use axum::{
    extract::{Path, State},
    http::StatusCode,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use thornado_core::{
    FrostCustodySigner, FrostCustodySignerSnapshot, FrostDkgRound1Output, FrostDkgRound1Public,
    FrostDkgRound2Output, FrostDkgRound2Public, FrostSignatureShare, FrostSigningCommitment,
    FrostSigningCommitmentPublic,
};
use thornado_frost_core::{
    generate_signer, sign_with_existing_engine, signer_info, GenerateSignerRequest,
    SignWithdrawalRequest, SignerInfo,
};

#[derive(Clone)]
pub struct AppState {
    signer: Arc<Mutex<Option<FrostCustodySigner>>>,
    snapshot_path: Option<PathBuf>,
}

#[derive(Clone, Debug, Serialize)]
pub struct HealthResponse {
    pub status: &'static str,
    pub service: &'static str,
    pub signer_loaded: bool,
}

#[derive(Clone, Debug, Serialize)]
pub struct ErrorResponse {
    pub error: String,
}

#[derive(Clone, Debug, Deserialize)]
pub struct DkgRound1Request {
    pub participant_index: u16,
    pub max_signers: u16,
    pub threshold: u16,
}

#[derive(Clone, Debug, Deserialize)]
pub struct DkgRound2Request {
    pub signer_id: String,
    pub secret_package: String,
    pub taproot_secret_package: String,
    pub round1_packages: Vec<FrostDkgRound1Public>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct DkgFinalizeRequest {
    pub signer_id: String,
    pub secret_package: String,
    pub taproot_secret_package: String,
    pub round1_packages: Vec<FrostDkgRound1Public>,
    pub round2_packages: Vec<FrostDkgRound2Public>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct SignatureShareRequest {
    pub signer_id: String,
    pub nonces: String,
    pub withdrawal: thornado_core::WithdrawalRequest,
    pub commitments: Vec<FrostSigningCommitmentPublic>,
    #[serde(default)]
    pub key_tweak: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct AggregateSharesRequest {
    pub withdrawal: thornado_core::WithdrawalRequest,
    pub commitments: Vec<FrostSigningCommitmentPublic>,
    pub shares: Vec<FrostSignatureShare>,
    #[serde(default)]
    pub key_tweak: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct TaprootSignatureShareRequest {
    pub signer_id: String,
    pub nonces: String,
    pub message_hex: String,
    pub commitments: Vec<FrostSigningCommitmentPublic>,
    #[serde(default)]
    pub merkle_root_hex: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct TaprootAggregateSharesRequest {
    pub message_hex: String,
    pub commitments: Vec<FrostSigningCommitmentPublic>,
    pub shares: Vec<FrostSignatureShare>,
    #[serde(default)]
    pub merkle_root_hex: Option<String>,
}

#[derive(Clone, Debug, Serialize)]
pub struct TaprootSignatureResponse {
    pub signature: String,
}

impl AppState {
    pub fn new(snapshot_path: Option<PathBuf>) -> anyhow::Result<Self> {
        let signer = if let Some(path) = &snapshot_path {
            if path.exists() {
                let snapshot = std::fs::read_to_string(path)?;
                let snapshot: FrostCustodySignerSnapshot = serde_json::from_str(&snapshot)?;
                Some(FrostCustodySigner::from_snapshot(&snapshot)?)
            } else {
                None
            }
        } else {
            None
        };

        Ok(Self {
            signer: Arc::new(Mutex::new(signer)),
            snapshot_path,
        })
    }

    pub fn from_signer(signer: FrostCustodySigner) -> Self {
        Self {
            signer: Arc::new(Mutex::new(Some(signer))),
            snapshot_path: None,
        }
    }

    fn signer(&self) -> Result<FrostCustodySigner, ApiError> {
        self.signer
            .lock()
            .map_err(|_| ApiError::internal("signer state lock poisoned"))?
            .clone()
            .ok_or_else(|| ApiError::not_found("no signer snapshot loaded"))
    }

    fn replace_signer(&self, signer: FrostCustodySigner) -> Result<(), ApiError> {
        if let Some(path) = &self.snapshot_path {
            if let Some(parent) = path.parent() {
                std::fs::create_dir_all(parent).map_err(ApiError::from_any)?;
            }
            let snapshot = signer.to_snapshot().map_err(ApiError::from_any)?;
            let json = serde_json::to_string_pretty(&snapshot).map_err(ApiError::from_any)?;
            std::fs::write(path, json).map_err(ApiError::from_any)?;
        }

        *self
            .signer
            .lock()
            .map_err(|_| ApiError::internal("signer state lock poisoned"))? = Some(signer);
        Ok(())
    }
}

#[derive(Debug)]
struct ApiError {
    status: StatusCode,
    message: String,
}

impl ApiError {
    fn not_found(message: impl Into<String>) -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            message: message.into(),
        }
    }

    fn bad_request(message: impl Into<String>) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: message.into(),
        }
    }

    fn internal(message: impl Into<String>) -> Self {
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message: message.into(),
        }
    }

    fn from_any(error: impl std::fmt::Display) -> Self {
        Self::bad_request(error.to_string())
    }
}

impl axum::response::IntoResponse for ApiError {
    fn into_response(self) -> axum::response::Response {
        (
            self.status,
            Json(ErrorResponse {
                error: self.message,
            }),
        )
            .into_response()
    }
}

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/health", get(health))
        .route("/v1/signer/info", get(get_signer_info))
        .route("/v1/dev/generate", post(dev_generate))
        .route("/v1/sign", post(sign_with_loaded_signer))
        .route("/v1/dkg/round1", post(dkg_round1))
        .route("/v1/dkg/round2", post(dkg_round2))
        .route("/v1/dkg/finalize", post(dkg_finalize))
        .route(
            "/v1/signers/:signer_id/commitment",
            post(signing_commitment),
        )
        .route("/v1/signature-share", post(signature_share))
        .route("/v1/aggregate", post(aggregate_shares))
        .route(
            "/v1/signers/:signer_id/taproot-commitment",
            post(taproot_signing_commitment),
        )
        .route("/v1/taproot-signature-share", post(taproot_signature_share))
        .route("/v1/taproot-aggregate", post(taproot_aggregate_shares))
        .with_state(state)
}

async fn health(State(state): State<AppState>) -> Json<HealthResponse> {
    let signer_loaded = state
        .signer
        .lock()
        .map(|signer| signer.is_some())
        .unwrap_or(false);
    Json(HealthResponse {
        status: "ok",
        service: "thornado-frost-signer",
        signer_loaded,
    })
}

async fn get_signer_info(State(state): State<AppState>) -> Result<Json<SignerInfo>, ApiError> {
    Ok(Json(
        signer_info(&state.signer()?).map_err(ApiError::from_any)?,
    ))
}

async fn dev_generate(
    State(state): State<AppState>,
    Json(request): Json<GenerateSignerRequest>,
) -> Result<Json<SignerInfo>, ApiError> {
    let signer = generate_signer(request).map_err(ApiError::from_any)?;
    let info = signer_info(&signer).map_err(ApiError::from_any)?;
    state.replace_signer(signer)?;
    Ok(Json(info))
}

async fn sign_with_loaded_signer(
    State(state): State<AppState>,
    Json(request): Json<SignWithdrawalRequest>,
) -> Result<Json<thornado_core::CustodySignature>, ApiError> {
    Ok(Json(
        sign_with_existing_engine(&state.signer()?, &request).map_err(ApiError::from_any)?,
    ))
}

async fn dkg_round1(
    Json(request): Json<DkgRound1Request>,
) -> Result<Json<FrostDkgRound1Output>, ApiError> {
    Ok(Json(
        FrostCustodySigner::dkg_round1(
            request.participant_index,
            request.max_signers,
            request.threshold,
        )
        .map_err(ApiError::from_any)?,
    ))
}

async fn dkg_round2(
    Json(request): Json<DkgRound2Request>,
) -> Result<Json<FrostDkgRound2Output>, ApiError> {
    Ok(Json(
        FrostCustodySigner::dkg_round2(
            &request.signer_id,
            &request.secret_package,
            &request.taproot_secret_package,
            &request.round1_packages,
        )
        .map_err(ApiError::from_any)?,
    ))
}

async fn dkg_finalize(
    State(state): State<AppState>,
    Json(request): Json<DkgFinalizeRequest>,
) -> Result<Json<SignerInfo>, ApiError> {
    let signer = FrostCustodySigner::dkg_finalize_single(
        &request.signer_id,
        &request.secret_package,
        &request.taproot_secret_package,
        &request.round1_packages,
        &request.round2_packages,
    )
    .map_err(ApiError::from_any)?;
    let info = signer_info(&signer).map_err(ApiError::from_any)?;
    state.replace_signer(signer)?;
    Ok(Json(info))
}

async fn signing_commitment(
    State(state): State<AppState>,
    Path(signer_id): Path<String>,
) -> Result<Json<FrostSigningCommitment>, ApiError> {
    Ok(Json(
        state
            .signer()?
            .signing_commitment(&signer_id)
            .map_err(ApiError::from_any)?,
    ))
}

async fn signature_share(
    State(state): State<AppState>,
    Json(request): Json<SignatureShareRequest>,
) -> Result<Json<FrostSignatureShare>, ApiError> {
    Ok(Json(
        state
            .signer()?
            .signature_share(
                &request.signer_id,
                &request.nonces,
                &request.withdrawal,
                &request.commitments,
                request.key_tweak.as_deref(),
            )
            .map_err(ApiError::from_any)?,
    ))
}

async fn aggregate_shares(
    State(state): State<AppState>,
    Json(request): Json<AggregateSharesRequest>,
) -> Result<Json<thornado_core::CustodySignature>, ApiError> {
    Ok(Json(
        state
            .signer()?
            .coordinator()
            .aggregate_signature_shares(
                &request.withdrawal,
                &request.commitments,
                &request.shares,
                request.key_tweak.as_deref(),
            )
            .map_err(ApiError::from_any)?,
    ))
}

async fn taproot_signing_commitment(
    State(state): State<AppState>,
    Path(signer_id): Path<String>,
) -> Result<Json<FrostSigningCommitment>, ApiError> {
    Ok(Json(
        state
            .signer()?
            .taproot_signing_commitment(&signer_id)
            .map_err(ApiError::from_any)?,
    ))
}

async fn taproot_signature_share(
    State(state): State<AppState>,
    Json(request): Json<TaprootSignatureShareRequest>,
) -> Result<Json<FrostSignatureShare>, ApiError> {
    let message = decode_hex(&request.message_hex)?;
    let merkle_root = request
        .merkle_root_hex
        .as_deref()
        .map(decode_hex)
        .transpose()?;
    Ok(Json(
        state
            .signer()?
            .taproot_signature_share(
                &request.signer_id,
                &request.nonces,
                &message,
                &request.commitments,
                merkle_root.as_deref(),
            )
            .map_err(ApiError::from_any)?,
    ))
}

async fn taproot_aggregate_shares(
    State(state): State<AppState>,
    Json(request): Json<TaprootAggregateSharesRequest>,
) -> Result<Json<TaprootSignatureResponse>, ApiError> {
    let message = decode_hex(&request.message_hex)?;
    let merkle_root = request
        .merkle_root_hex
        .as_deref()
        .map(decode_hex)
        .transpose()?;
    Ok(Json(TaprootSignatureResponse {
        signature: state
            .signer()?
            .aggregate_taproot_signature_shares(
                &message,
                &request.commitments,
                &request.shares,
                merkle_root.as_deref(),
            )
            .map_err(ApiError::from_any)?,
    }))
}

fn decode_hex(value: &str) -> Result<Vec<u8>, ApiError> {
    hex::decode(value).map_err(|e| ApiError::bad_request(format!("invalid hex: {e}")))
}

pub async fn serve(addr: SocketAddr, snapshot_path: Option<PathBuf>) -> anyhow::Result<()> {
    let state = AppState::new(snapshot_path)?;
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, router(state)).await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use axum::{
        body::Body,
        http::{Request, StatusCode},
    };
    use serde_json::{json, Value};
    use thornado_core::{FrostCustodySigner, WithdrawalRequest};
    use tower::ServiceExt;

    use super::*;

    fn withdrawal() -> WithdrawalRequest {
        WithdrawalRequest {
            withdrawal_id: "wd-sidecar-e2e".to_string(),
            recipient: "tb1qrecipient".to_string(),
            amount_sats: 99_900_000,
            fee_sats: 100_000,
            nullifier_hash: "nullifier-hash".to_string(),
        }
    }

    async fn post(router: &Router, path: &str, body: Value) -> (StatusCode, Value) {
        let response = router
            .clone()
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri(path)
                    .header("content-type", "application/json")
                    .body(Body::from(body.to_string()))
                    .unwrap(),
            )
            .await
            .unwrap();
        let status = response.status();
        let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .unwrap();
        let value = serde_json::from_slice(&bytes).unwrap();
        (status, value)
    }

    #[tokio::test]
    async fn sidecar_generates_persists_and_signs_with_existing_engine() {
        let dir = std::env::temp_dir().join(format!(
            "thornado-frost-sidecar-{}.json",
            std::process::id()
        ));
        let _ = std::fs::remove_file(&dir);
        let state = AppState::new(Some(dir.clone())).unwrap();
        let app = router(state);

        let (status, info) = post(
            &app,
            "/v1/dev/generate",
            json!({ "max_signers": 5, "threshold": 4 }),
        )
        .await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(info["threshold"], 4);
        assert!(dir.exists());

        let (status, signature) = post(
            &app,
            "/v1/sign",
            json!({
                "withdrawal": withdrawal(),
            }),
        )
        .await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(signature["scheme"], "frost-secp256k1-sha256");

        let reloaded = router(AppState::new(Some(dir.clone())).unwrap());
        let (status, reloaded_signature) = post(
            &reloaded,
            "/v1/sign",
            json!({
                "withdrawal": withdrawal(),
            }),
        )
        .await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(reloaded_signature["scheme"], "frost-secp256k1-sha256");
        let _ = std::fs::remove_file(dir);
    }

    #[tokio::test]
    async fn sidecar_runs_commit_share_aggregate_flow() {
        let signer = FrostCustodySigner::generate_with_dkg(5, 4).unwrap();
        let signer_ids = signer.signer_ids();
        let app = router(AppState::from_signer(signer));
        let withdrawal = withdrawal();

        let mut commitments_private = Vec::new();
        let mut commitments_public = Vec::new();
        for signer_id in signer_ids.iter().take(4) {
            let (status, commitment) = post(
                &app,
                &format!("/v1/signers/{signer_id}/commitment"),
                json!({}),
            )
            .await;
            assert_eq!(status, StatusCode::OK);
            commitments_public.push(json!({
                "signer_id": commitment["signer_id"],
                "commitment": commitment["commitment"],
            }));
            commitments_private.push(commitment);
        }

        let mut shares = Vec::new();
        for commitment in &commitments_private {
            let (status, share) = post(
                &app,
                "/v1/signature-share",
                json!({
                    "signer_id": commitment["signer_id"],
                    "nonces": commitment["nonces"],
                    "withdrawal": withdrawal,
                    "commitments": commitments_public,
                }),
            )
            .await;
            assert_eq!(status, StatusCode::OK);
            shares.push(share);
        }

        let (status, signature) = post(
            &app,
            "/v1/aggregate",
            json!({
                "withdrawal": withdrawal,
                "commitments": commitments_public,
                "shares": shares,
            }),
        )
        .await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(signature["signer"], "frost-4-of-5");
    }
}
