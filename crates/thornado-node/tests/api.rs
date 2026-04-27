use axum::body::Body;
use axum::http::{Request, StatusCode};
use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, Network};
use serde::de::DeserializeOwned;
use serde_json::json;
use thornado_bitcoin::{script_hex, tx_bytes, txid_for_tests};
use thornado_core::{
    derive_split_receipt, execute_command, mine_deposit_pow, withdrawal_from_receipt, AppState,
    Command, Event, MockCustodySigner, MockProofVerifier, NoteReceipt, TornadoStarkProof,
    WithdrawalProof, WithdrawalPublicInputs,
};
use thornado_node::{router, EventsResponse, NodeState, RootResponse, StateHashResponse};
use tower::ServiceExt;

#[tokio::test]
async fn serves_browser_ui() {
    let mut app = test_app();
    let response = request_raw(&mut app, "GET", "/", None).await;

    assert_eq!(response.status(), StatusCode::OK);
    let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
        .await
        .unwrap();
    let body = String::from_utf8(bytes.to_vec()).unwrap();
    assert!(body.contains("Thornado MVP"));
    assert!(body.contains("/deposit/request"));
}

#[tokio::test]
async fn http_happy_path_and_root_lookup_work() {
    let mut app = test_app();
    let initial_hash: StateHashResponse = request_json(&mut app, "GET", "/state/hash", None).await;

    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("api-happy"), "user_pubkey": "test-client-pubkey" })),
    )
    .await;
    assert_ne!(initial_hash.state_hash, response.state_hash);

    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/observe",
        Some(json!({
            "intent_id": "dep-1",
            "txid": "tx-1",
            "amount_sats": 100_000_000
        })),
    )
    .await;
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/confirm",
        Some(json!({ "intent_id": "dep-1" })),
    )
    .await;

    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/split",
        Some(json!({
            "deposit_id": "dep-1",
            "note_commitments": receipt.commitments()
        })),
    )
    .await;
    assert!(matches!(
        response.events.first(),
        Some(Event::NotesMinted { .. })
    ));

    let root: RootResponse = request_json(&mut app, "GET", "/notes/root/100000000", None).await;
    assert_eq!(root.denomination_sats, 100_000_000);

    let (proof, public) = proof_from_receipt(&receipt.notes[0], root.root, 100_000);
    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/withdraw",
        Some(json!({ "proof": proof, "public": public })),
    )
    .await;
    assert!(response
        .events
        .iter()
        .any(|event| matches!(event, Event::WithdrawalAuthorized { .. })));
}

#[tokio::test]
async fn secret_bearing_withdrawal_proof_is_rejected() {
    let mut app = test_app();
    prepare_confirmed_split(&mut app).await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    let root: RootResponse = request_json(&mut app, "GET", "/notes/root/100000000", None).await;
    let (proof, public) = withdrawal_from_receipt(
        &receipt.notes[0],
        root.root,
        regtest_address().to_string(),
        100_000,
    );

    let response = request_raw(
        &mut app,
        "POST",
        "/withdraw",
        Some(json!({ "proof": proof, "public": public })),
    )
    .await;

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn split_without_confirmation_is_rejected() {
    let mut app = test_app();
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("split-unconfirmed"), "user_pubkey": "test-client-pubkey" })),
    )
    .await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    let response = request_raw(
        &mut app,
        "POST",
        "/split",
        Some(json!({
            "deposit_id": "dep-1",
            "note_commitments": receipt.commitments()
        })),
    )
    .await;

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn unknown_root_endpoint_is_rejected() {
    let mut app = test_app();
    let response = request_raw(&mut app, "GET", "/notes/root/42", None).await;
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn deposit_request_rejects_invalid_pow() {
    let mut app = test_app();
    let response = request_raw(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": "not-mined", "user_pubkey": "test-client-pubkey" })),
    )
    .await;

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn builds_bitcoin_withdrawal_for_authorized_withdrawal() {
    let mut app = test_app_with_authorized_withdrawal();

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(9),
            "vout": 0,
            "value_sats": 150_000_000,
            "script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    let utxos: serde_json::Value = request_json(&mut app, "GET", "/bitcoin/utxos", None).await;
    assert_eq!(utxos["utxos"].as_array().unwrap().len(), 1);

    let built: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/build",
        Some(json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    assert_eq!(built["withdrawal_id"], "wd-1");
    assert_eq!(built["output_value_sats"], 99_900_000);
    assert!(!tx_bytes(built["unsigned_tx_hex"].as_str().unwrap())
        .unwrap()
        .is_empty());

    let record: serde_json::Value =
        request_json(&mut app, "GET", "/bitcoin/withdrawal/wd-1", None).await;
    assert_eq!(record["built"]["withdrawal_id"], "wd-1");

    let broadcast_txid = txid_for_tests(10);
    let record: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/wd-1/mark-broadcast",
        Some(json!({ "txid": broadcast_txid })),
    )
    .await;
    assert_eq!(record["broadcast_txid"], txid_for_tests(10));

    let record: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/wd-1/mark-confirmed",
        Some(json!({ "height": 123 })),
    )
    .await;
    assert_eq!(record["confirmed_height"], 123);
}

#[tokio::test]
async fn dev_backend_does_not_reuse_reserved_utxos() {
    let mut app = test_app_with_authorized_withdrawal();

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(11),
            "vout": 0,
            "value_sats": 150_000_000,
            "script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/build",
        Some(json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    let response = request_raw(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/build",
        Some(json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

fn test_app() -> axum::Router {
    router(NodeState::new(AppState::default(), None))
}

fn test_app_with_authorized_withdrawal() -> axum::Router {
    router(NodeState::new(authorized_withdrawal_state(), None))
}

fn authorized_withdrawal_state() -> AppState {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("api-authorized"),
            user_pubkey: "test-client-pubkey".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-1".to_string(),
            amount_sats: 100_000_000,
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::ConfirmDeposit {
            intent_id: "dep-1".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    execute_command(
        &mut state,
        Command::SplitDepositIntoNotes {
            deposit_id: "dep-1".to_string(),
            note_commitments: receipt.commitments(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    let root = state.notes.trees.get(&100_000_000).unwrap().root();
    let (proof, public) = withdrawal_from_receipt(
        &receipt.notes[0],
        root,
        regtest_address().to_string(),
        100_000,
    );
    execute_command(
        &mut state,
        Command::WithdrawNote { proof, public },
        &verifier,
        &signer,
    )
    .unwrap();
    state
}

async fn prepare_confirmed_split(app: &mut axum::Router) {
    request_json::<EventsResponse>(
        app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("api-prepare"), "user_pubkey": "test-client-pubkey" })),
    )
    .await;
    request_json::<EventsResponse>(
        app,
        "POST",
        "/deposit/observe",
        Some(json!({
            "intent_id": "dep-1",
            "txid": "tx-1",
            "amount_sats": 100_000_000
        })),
    )
    .await;
    request_json::<EventsResponse>(
        app,
        "POST",
        "/deposit/confirm",
        Some(json!({ "intent_id": "dep-1" })),
    )
    .await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    request_json::<EventsResponse>(
        app,
        "POST",
        "/split",
        Some(json!({
            "deposit_id": "dep-1",
            "note_commitments": receipt.commitments()
        })),
    )
    .await;
}

fn proof_from_receipt(
    note: &NoteReceipt,
    root: String,
    fee_sats: u64,
) -> (WithdrawalProof, WithdrawalPublicInputs) {
    let (_, public) =
        withdrawal_from_receipt(note, root.clone(), regtest_address().to_string(), fee_sats);
    let proof = WithdrawalProof {
        nullifier: String::new(),
        secret: String::new(),
        commitment: String::new(),
        merkle_root: root,
        stark: Some(TornadoStarkProof {
            proof_hex: "public-proof-envelope".to_string(),
        }),
    };
    (proof, public)
}

fn regtest_script_hex() -> String {
    script_hex(&regtest_address().script_pubkey())
}

fn regtest_address() -> Address {
    let secp = Secp256k1::new();
    let secret = SecretKey::from_slice(&[2_u8; 32]).unwrap();
    let public_key = bitcoin::CompressedPublicKey(SecpPublicKey::from_secret_key(&secp, &secret));
    Address::p2wpkh(&public_key, Network::Regtest)
}

fn pow(label: &str) -> String {
    mine_deposit_pow(label)
}

async fn request_json<T: DeserializeOwned>(
    app: &mut axum::Router,
    method: &str,
    uri: &str,
    body: Option<serde_json::Value>,
) -> T {
    let response = request_raw(app, method, uri, body).await;
    let status = response.status();
    let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
        .await
        .unwrap();
    assert_eq!(
        status,
        StatusCode::OK,
        "{} {} failed: {}",
        method,
        uri,
        String::from_utf8_lossy(&bytes)
    );
    serde_json::from_slice(&bytes).unwrap()
}

async fn request_raw(
    app: &mut axum::Router,
    method: &str,
    uri: &str,
    body: Option<serde_json::Value>,
) -> axum::response::Response {
    let body = body
        .map(|value| Body::from(serde_json::to_vec(&value).unwrap()))
        .unwrap_or_else(Body::empty);
    let request = Request::builder()
        .method(method)
        .uri(uri)
        .header("content-type", "application/json")
        .body(body)
        .unwrap();

    app.oneshot(request).await.unwrap()
}
