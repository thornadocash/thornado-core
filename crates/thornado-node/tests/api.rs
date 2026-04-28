use axum::body::Body;
use axum::http::{Request, StatusCode};
use axum::routing::post;
use axum::{Json, Router};
use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use base64::Engine as _;
use bitcoin::consensus::encode::deserialize;
use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, Network, Transaction};
use serde::de::DeserializeOwned;
use serde_json::json;
use std::sync::Arc;
use thornado_abci::decode_tx;
use thornado_bitcoin::{script_hex, tx_bytes, txid_for_tests, RegtestUtxo};
use thornado_core::{
    apply_event, client_pubkey_from_secret, derive_split_receipt, execute_command,
    mine_deposit_pow, required_node_bond_sats, start_churn_epoch_without_keygen, state_hash,
    withdrawal_from_receipt, zk_withdrawal_from_receipt, AppState, AuthorizedWithdrawal, Command,
    CustodySignature, DenominationTree, Event, FrostCustodySigner, MockCustodySigner,
    MockProofVerifier, NodeStatus, NoteReceipt, WithdrawalProof, WithdrawalPublicInputs, BTC_SATS,
};
use thornado_node::{
    router, BifrostBitcoinWork, BitcoinVaultAddressResponse, ChurnWindowResponse, EventsResponse,
    LeavesResponse, NodeConfig, NodeState, RootResponse, StateHashResponse,
};
use tokio::sync::Mutex;
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
async fn churn_window_reports_attached_node_cycle() {
    let mut app = test_app();
    let window: ChurnWindowResponse = request_json(&mut app, "GET", "/churn/window", None).await;

    assert_eq!(window.epoch, 0);
    assert!(window.cycle_ms > 0);
    assert_eq!(window.target_active_nodes, 4);
    assert_eq!(window.max_nodes_per_churn, 1);
    assert!(window.next_churn_at_ms > window.server_now_ms);
    assert_eq!(
        window.remaining_ms,
        window.next_churn_at_ms - window.server_now_ms
    );
}

#[tokio::test]
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn http_happy_path_and_root_lookup_work() {
    let mut app = test_app();
    let initial_hash: StateHashResponse = request_json(&mut app, "GET", "/state/hash", None).await;

    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("api-happy"), "user_pubkey": client_pubkey_from_secret("api-test-seed") })),
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
        response.events.get(1),
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
    let signature = response
        .events
        .iter()
        .find_map(|event| match event {
            Event::WithdrawalAuthorized { signature, .. } => Some(signature),
            _ => None,
        })
        .unwrap();
    assert_eq!(signature.scheme, "frost-secp256k1-sha256");
    assert!(signature.signer.starts_with("frost-"));
}

#[tokio::test]
async fn public_note_leaves_are_served_for_browser_proving() {
    let mut app = test_app();
    prepare_confirmed_split(&mut app).await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("api-prepare-2"), "user_pubkey": client_pubkey_from_secret("api-test-seed-2") })),
    )
    .await;
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/observe",
        Some(json!({
            "intent_id": "dep-2",
            "txid": "tx-2",
            "amount_sats": 100_000_000
        })),
    )
    .await;
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/confirm",
        Some(json!({ "intent_id": "dep-2" })),
    )
    .await;
    let later_receipt = derive_split_receipt("dep-2", 100_000_000, "api-test-seed-2").unwrap();
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/split",
        Some(json!({
            "deposit_id": "dep-2",
            "note_commitments": later_receipt.commitments()
        })),
    )
    .await;

    let leaves: LeavesResponse =
        request_json(&mut app, "GET", "/notes/leaves/100000000", None).await;

    assert_eq!(leaves.denomination_sats, 100_000_000);
    assert_eq!(leaves.leaf_count, 2);
    assert_eq!(
        leaves.leaves,
        vec![
            receipt.notes[0].commitment.clone(),
            later_receipt.notes[0].commitment.clone()
        ]
    );
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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn malformed_zk_withdrawal_proof_is_rejected() {
    let mut app = test_app();
    prepare_confirmed_split(&mut app).await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    let root: RootResponse = request_json(&mut app, "GET", "/notes/root/100000000", None).await;
    let (mut proof, public) = proof_from_receipt(&receipt.notes[0], root.root, 100_000);
    proof.orchard.as_mut().unwrap().proof_hex = "00".repeat(128);

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
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn recipient_mutation_without_matching_public_field_is_rejected() {
    let mut app = test_app();
    prepare_confirmed_split(&mut app).await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "api-test-seed").unwrap();
    let root: RootResponse = request_json(&mut app, "GET", "/notes/root/100000000", None).await;
    let (proof, mut public) = proof_from_receipt(&receipt.notes[0], root.root, 100_000);
    public.recipient = "tb1qtamperedrecipient".to_string();

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
        Some(json!({ "pow_token": pow("split-unconfirmed"), "user_pubkey": client_pubkey_from_secret("api-test-seed") })),
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
        Some(json!({ "pow_token": "not-mined", "user_pubkey": client_pubkey_from_secret("api-test-seed") })),
    )
    .await;

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn deposit_request_persists_local_frost_signer() {
    let path = std::env::temp_dir().join(format!(
        "thornado-frost-signer-{}-{}.json",
        std::process::id(),
        "api"
    ));
    let _ = std::fs::remove_file(&path);
    let state = NodeState::with_config(
        AppState::default(),
        NodeConfig {
            snapshot_path: None,
            frost_signer_path: Some(path.clone()),
            bitcoin_state_path: None,
            bitcoin_rpc: None,
            node_id: None,
            churn_cycle_ms: None,
        },
        Vec::new(),
    )
    .unwrap();
    let mut app = router(state);

    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("persist-frost"), "user_pubkey": client_pubkey_from_secret("api-test-seed") })),
    )
    .await;

    let json = std::fs::read_to_string(&path).unwrap();
    assert!(json.contains("key_packages"));
    let _ = std::fs::remove_file(path);
}

#[tokio::test]
async fn node_lifecycle_endpoints_register_bond_assign_and_churn() {
    let mut app = test_app();
    bootstrap_custody(&mut app).await;
    let vault_script = vault_script_hex_from_app(&mut app).await;
    let mut faucet = RegtestBondFaucet::new(70);
    let utxo = faucet.fund_bond(required_node_bond_sats(0), vault_script);

    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/nodes/register",
        Some(json!({
            "node_id": "node-api-1",
            "bond_address": "bond-api-1",
            "consensus_pubkey": "consensus-api-1",
            "signer_pubkey": "signer-api-1"
        })),
    )
    .await;
    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(serde_json::to_value(&utxo).unwrap()),
    )
    .await;
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/nodes/bond",
        Some(json!({
            "node_id": "node-api-1",
            "amount_sats": required_node_bond_sats(0),
            "txid": utxo.txid,
            "vout": utxo.vout
        })),
    )
    .await;
    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/nodes/slot/assign",
        Some(json!({
            "node_id": "node-api-1",
            "slot_id": 0
        })),
    )
    .await;
    let churn: EventsResponse =
        request_json(&mut app, "POST", "/churn/start", Some(json!({}))).await;

    assert!(churn.events.iter().any(|event| {
        matches!(
            event,
            Event::StandbyNodeActivated { node_id, epoch: 1 } if node_id == "node-api-1"
        )
    }));
}

#[tokio::test]
async fn genesis_nodes_bond_from_regtest_faucet_and_churn_active() {
    let mut app = test_app();
    bootstrap_custody(&mut app).await;
    let vault_script = vault_script_hex_from_app(&mut app).await;
    let mut faucet = RegtestBondFaucet::new(80);
    let min_bond_sats = 10 * BTC_SATS;
    let min_bond_increase_sats = BTC_SATS * 5 / 2;

    request_json::<EventsResponse>(
        &mut app,
        "POST",
        "/nodes/bond-parameters",
        Some(json!({
            "min_bond_sats": min_bond_sats,
            "min_bond_increase_sats": min_bond_increase_sats
        })),
    )
    .await;

    for slot_id in 0..5_u64 {
        let node_id = format!("genesis-node-{slot_id}");
        let bond_sats = min_bond_sats + min_bond_increase_sats * slot_id;
        let utxo = faucet.fund_bond(bond_sats, vault_script.clone());
        request_json::<serde_json::Value>(
            &mut app,
            "POST",
            "/bitcoin/utxo/import",
            Some(serde_json::to_value(&utxo).unwrap()),
        )
        .await;
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/register",
            Some(json!({
                "node_id": node_id,
                "bond_address": format!("bond-genesis-{slot_id}"),
                "consensus_pubkey": format!("consensus-genesis-{slot_id}"),
                "signer_pubkey": format!("signer-genesis-{slot_id}")
            })),
        )
        .await;
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/bond",
            Some(json!({
                "node_id": node_id,
                "amount_sats": utxo.value_sats,
                "txid": utxo.txid,
                "vout": utxo.vout
            })),
        )
        .await;
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/slot/assign",
            Some(json!({
                "node_id": node_id,
                "slot_id": slot_id
            })),
        )
        .await;
    }

    let utxos: serde_json::Value = request_json(&mut app, "GET", "/bitcoin/utxos", None).await;
    assert_eq!(utxos["utxos"].as_array().unwrap().len(), 5);

    let churn: EventsResponse =
        request_json(&mut app, "POST", "/churn/start", Some(json!({}))).await;

    let activated = churn
        .events
        .iter()
        .filter(|event| matches!(event, Event::StandbyNodeActivated { .. }))
        .count();
    assert_eq!(activated, 1);
}

#[tokio::test]
async fn active_nodes_vote_to_update_churn_window() {
    let mut app = test_app();
    bootstrap_custody(&mut app).await;
    let vault_script = vault_script_hex_from_app(&mut app).await;
    let mut faucet = RegtestBondFaucet::new(90);

    for slot_id in 0..3_u64 {
        let node_id = format!("vote-node-{slot_id}");
        let utxo = faucet.fund_bond(required_node_bond_sats(slot_id), vault_script.clone());
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/register",
            Some(json!({
                "node_id": node_id,
                "bond_address": format!("bond-vote-{slot_id}"),
                "consensus_pubkey": format!("consensus-vote-{slot_id}"),
                "signer_pubkey": format!("signer-vote-{slot_id}")
            })),
        )
        .await;
        request_json::<serde_json::Value>(
            &mut app,
            "POST",
            "/bitcoin/utxo/import",
            Some(serde_json::to_value(&utxo).unwrap()),
        )
        .await;
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/bond",
            Some(json!({
                "node_id": node_id,
                "amount_sats": required_node_bond_sats(slot_id),
                "txid": utxo.txid,
                "vout": utxo.vout
            })),
        )
        .await;
        request_json::<EventsResponse>(
            &mut app,
            "POST",
            "/nodes/slot/assign",
            Some(json!({
                "node_id": node_id,
                "slot_id": slot_id
            })),
        )
        .await;
    }

    for _ in 0..3 {
        request_json::<EventsResponse>(&mut app, "POST", "/churn/start", Some(json!({}))).await;
    }
    let initial: ChurnWindowResponse = request_json(&mut app, "GET", "/churn/window", None).await;
    assert_eq!(initial.cycle_ms, 20 * 60 * 1000);
    assert_eq!(initial.target_active_nodes, 4);
    assert_eq!(initial.max_nodes_per_churn, 1);

    let first_vote: EventsResponse = request_json(
        &mut app,
        "POST",
        "/network/parameters/vote",
        Some(json!({
            "node_id": "vote-node-0",
            "churn_cycle_ms": 10 * 60 * 1000
        })),
    )
    .await;
    assert!(!first_vote
        .events
        .iter()
        .any(|event| matches!(event, Event::NetworkParametersUpdated { .. })));

    let second_vote: EventsResponse = request_json(
        &mut app,
        "POST",
        "/network/parameters/vote",
        Some(json!({
            "node_id": "vote-node-1",
            "churn_cycle_ms": 10 * 60 * 1000
        })),
    )
    .await;
    assert!(second_vote.events.iter().any(|event| {
        matches!(
            event,
            Event::NetworkParametersUpdated {
                churn_cycle_ms,
                max_nodes_per_churn,
                epoch: 3,
                ..
            } if *churn_cycle_ms == 10 * 60 * 1000 && *max_nodes_per_churn == 1
        )
    }));
    let updated: ChurnWindowResponse = request_json(&mut app, "GET", "/churn/window", None).await;
    assert_eq!(updated.cycle_ms, 10 * 60 * 1000);
}

#[test]
fn churn_out_uses_slash_points_age_then_node_id() {
    let mut state = AppState::default();
    state.churn.target_active_nodes = 5;
    state.churn.max_nodes_per_churn = 5;

    for slot_id in 0..5_u64 {
        let node_id = format!("det-node-{slot_id}");
        execute_command(
            &mut state,
            Command::RegisterNode {
                node_id: node_id.clone(),
                bond_address: format!("bond-det-{slot_id}"),
                consensus_pubkey: format!("consensus-det-{slot_id}"),
                signer_pubkey: format!("signer-det-{slot_id}"),
            },
            &MockProofVerifier,
            &MockCustodySigner,
        )
        .unwrap();
        execute_command(
            &mut state,
            Command::BondNode {
                node_id: node_id.clone(),
                amount_sats: required_node_bond_sats(slot_id),
            },
            &MockProofVerifier,
            &MockCustodySigner,
        )
        .unwrap();
        execute_command(
            &mut state,
            Command::AssignNodeSlot { node_id, slot_id },
            &MockProofVerifier,
            &MockCustodySigner,
        )
        .unwrap();
    }

    start_churn_epoch_without_keygen(&mut state).unwrap();
    assert_eq!(state.churn.active_nodes.len(), 5);

    let slash_epoch = state.churn.epoch;
    apply_event(
        &mut state,
        Event::NodeSlashPointsAdded {
            node_id: "det-node-4".to_string(),
            points: 2,
            reason: "test".to_string(),
            epoch: slash_epoch,
        },
    )
    .unwrap();
    state
        .churn
        .node_accounts
        .get_mut("det-node-3")
        .unwrap()
        .active_since_epoch = Some(0);

    state.churn.target_active_nodes = 4;
    state.churn.max_nodes_per_churn = 1;
    let slash_events = start_churn_epoch_without_keygen(&mut state).unwrap();
    assert!(slash_events.iter().any(|event| {
        matches!(
            event,
            Event::NodeStatusUpdated {
                node_id,
                status: NodeStatus::Standby,
                epoch: 2
            } if node_id == "det-node-4"
        )
    }));
    assert!(!state.churn.active_nodes.contains("det-node-4"));

    state.churn.target_active_nodes = 3;
    let age_events = start_churn_epoch_without_keygen(&mut state).unwrap();
    assert!(age_events.iter().any(|event| {
        matches!(
            event,
            Event::NodeStatusUpdated {
                node_id,
                status: NodeStatus::Standby,
                epoch: 3
            } if node_id == "det-node-3"
        )
    }));
    assert!(!state.churn.active_nodes.contains("det-node-3"));

    for node_id in ["det-node-0", "det-node-1", "det-node-2"] {
        state
            .churn
            .node_accounts
            .get_mut(node_id)
            .unwrap()
            .active_since_epoch = Some(3);
    }
    state.churn.target_active_nodes = 2;
    let tie_events = start_churn_epoch_without_keygen(&mut state).unwrap();
    assert!(tie_events.iter().any(|event| {
        matches!(
            event,
            Event::NodeStatusUpdated {
                node_id,
                status: NodeStatus::Standby,
                epoch: 4
            } if node_id == "det-node-0"
        )
    }));
    assert!(!state.churn.active_nodes.contains("det-node-0"));
}

#[tokio::test]
async fn cometbft_mode_submits_commands_to_rpc_without_local_mutation() {
    let submitted = Arc::new(Mutex::new(Vec::new()));
    let rpc_url = spawn_mock_cometbft(submitted.clone()).await;
    let mut app = router(NodeState::new(AppState::default(), None).with_cometbft_rpc(rpc_url));

    let before: StateHashResponse = request_json(&mut app, "GET", "/state/hash", None).await;
    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/churn/standby",
        Some(json!({ "node_id": "node-a" })),
    )
    .await;
    let after: StateHashResponse = request_json(&mut app, "GET", "/state/hash", None).await;

    assert_eq!(before.state_hash, after.state_hash);
    assert_eq!(response.events.len(), 0);
    assert_eq!(response.tx_hash.as_deref(), Some("ABC123"));
    assert_eq!(
        submitted.lock().await.as_slice(),
        [Command::RegisterStandbyNode {
            node_id: "node-a".to_string()
        }]
    );
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
async fn bitcoin_deposit_scan_observes_and_confirms_deposit_address_utxo() {
    let mut app = test_app();
    let deposit: EventsResponse = request_json(
        &mut app,
        "POST",
        "/deposit/request",
        Some(json!({
            "pow_token": pow("scan-deposit"),
            "user_pubkey": client_pubkey_from_secret("scan-deposit-seed")
        })),
    )
    .await;
    let deposit_address = deposit
        .events
        .iter()
        .find_map(|event| match event {
            Event::DepositIntentCreated {
                deposit_address, ..
            } => Some(deposit_address.clone()),
            _ => None,
        })
        .unwrap();
    let txid = txid_for_tests(12);

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid,
            "vout": 0,
            "value_sats": 100_000_000,
            "script_pubkey_hex": script_hex_for_address(&deposit_address),
            "confirmations": 1
        })),
    )
    .await;

    let scanned: EventsResponse =
        request_json(&mut app, "POST", "/bitcoin/deposits/scan", Some(json!({}))).await;
    assert!(scanned.events.iter().any(|event| {
        matches!(
            event,
            Event::DepositObserved {
                intent_id,
                txid: observed_txid,
                amount_sats: 100_000_000
            } if intent_id == "dep-1" && observed_txid == &txid
        )
    }));
    assert!(scanned.events.iter().any(|event| {
        matches!(
            event,
            Event::DepositConfirmed { intent_id } if intent_id == "dep-1"
        )
    }));
}

#[tokio::test]
async fn bitcoin_withdrawal_default_change_returns_to_vault_index_zero() {
    let mut app = test_app_with_authorized_withdrawal();
    let vault_script = vault_script_hex_from_app(&mut app).await;

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(13),
            "vout": 0,
            "value_sats": 150_000_000,
            "script_pubkey_hex": vault_script,
            "confirmations": 1
        })),
    )
    .await;

    let built: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/build",
        Some(json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2
        })),
    )
    .await;
    let tx: Transaction =
        deserialize(&hex::decode(built["unsigned_tx_hex"].as_str().unwrap()).unwrap()).unwrap();
    let change_script = tx.output.get(1).unwrap().script_pubkey.clone();

    assert_eq!(
        script_hex(&change_script),
        vault_script_hex_from_app(&mut app).await
    );
}

#[tokio::test]
async fn bifrost_bitcoin_work_sweeps_retiring_vault_to_active_vault() {
    let mut state = AppState::default();
    state.churn.active_nodes.extend([
        "node-1".to_string(),
        "node-2".to_string(),
        "node-3".to_string(),
    ]);
    let signer = FrostCustodySigner::demo_67_percent().unwrap();
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 0,
            keyset: signer.to_keyset(0).unwrap(),
        },
    )
    .unwrap();
    let old_vault_script =
        script_hex_for_address(&thornado_core::derive_vault_address(&state, 0, 0).unwrap());

    state.churn.epoch = 1;
    state
        .churn
        .active_nodes
        .extend(["node-4".to_string(), "node-5".to_string()]);
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated {
            epoch: 1,
            keyset: signer.to_keyset(1).unwrap(),
        },
    )
    .unwrap();

    let mut app = router(NodeState::new(state, None));
    let old_utxo = RegtestUtxo {
        txid: txid_for_tests(14),
        vout: 0,
        value_sats: 150_000_000,
        script_pubkey_hex: old_vault_script,
        confirmations: 1,
        is_self_transfer: false,
        mempool_ancestor_count: 0,
        mempool_descendant_count: 0,
        deposit_key_tweak: None,
    };
    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(serde_json::to_value(&old_utxo).unwrap()),
    )
    .await;

    let work: BifrostBitcoinWork =
        request_json(&mut app, "GET", "/bifrost/work/bitcoin", None).await;
    assert_eq!(work.vault_sweeps.len(), 1);
    assert_eq!(work.vault_sweeps[0].from_epoch, 0);
    assert_eq!(work.vault_sweeps[0].to_epoch, 1);
    assert_eq!(work.vault_sweeps[0].include_txids, vec![old_utxo.txid]);
}

#[tokio::test]
async fn bitcoin_backend_state_persists_across_restart() {
    let path = std::env::temp_dir().join(format!(
        "thornado-bitcoin-state-{}-{}.redb",
        std::process::id(),
        "api"
    ));
    let _ = std::fs::remove_file(&path);
    let state = NodeState::with_config(
        authorized_withdrawal_state(),
        NodeConfig {
            snapshot_path: None,
            frost_signer_path: None,
            bitcoin_state_path: Some(path.clone()),
            bitcoin_rpc: None,
            node_id: None,
            churn_cycle_ms: None,
        },
        Vec::new(),
    )
    .unwrap();
    let mut app = router(state);

    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(18),
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

    assert!(std::fs::metadata(&path).unwrap().len() > 0);

    let state = NodeState::with_config(
        authorized_withdrawal_state(),
        NodeConfig {
            snapshot_path: None,
            frost_signer_path: None,
            bitcoin_state_path: Some(path.clone()),
            bitcoin_rpc: None,
            node_id: None,
            churn_cycle_ms: None,
        },
        Vec::new(),
    )
    .unwrap();
    let mut app = router(state);
    let record: serde_json::Value =
        request_json(&mut app, "GET", "/bitcoin/withdrawal/wd-1", None).await;
    assert_eq!(record["built"]["withdrawal_id"], "wd-1");

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
    let _ = std::fs::remove_file(path);
}

#[tokio::test]
async fn bitcoin_checkpoint_solvency_and_consolidation_endpoints_work() {
    let mut app = test_app_with_authorized_withdrawal();

    for byte in [21_u8, 22, 23] {
        request_json::<serde_json::Value>(
            &mut app,
            "POST",
            "/bitcoin/utxo/import",
            Some(json!({
                "txid": txid_for_tests(byte),
                "vout": 0,
                "value_sats": 150_000_000,
                "script_pubkey_hex": regtest_script_hex()
            })),
        )
        .await;
    }

    let solvency: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/solvency",
        Some(json!({ "expected_sats": 300_000_000 })),
    )
    .await;
    assert_eq!(solvency["solvent"], true);
    assert_eq!(solvency["spendable_utxo_count"], 3);

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
    let checkpoint: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/wd-1/checkpoint/validate",
        Some(json!({ "unsigned_tx_hex": built["unsigned_tx_hex"] })),
    )
    .await;
    assert_eq!(checkpoint["valid"], true);
    assert_eq!(checkpoint["withdrawal_id"], "wd-1");

    let consolidation: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/consolidation/build",
        Some(json!({
            "consolidation_id": "consolidate-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex(),
            "min_utxos": 2
        })),
    )
    .await;
    assert_eq!(consolidation["consolidation_id"], "consolidate-1");
    assert_eq!(consolidation["selected_utxos"].as_array().unwrap().len(), 2);

    let record: serde_json::Value = request_json(
        &mut app,
        "GET",
        "/bitcoin/consolidation/consolidate-1",
        None,
    )
    .await;
    assert_eq!(record["built"]["consolidation_id"], "consolidate-1");

    let record: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/consolidation/consolidate-1/broadcast",
        Some(json!({ "signed_tx_hex": consolidation["unsigned_tx_hex"] })),
    )
    .await;
    assert!(record["broadcast_txid"].as_str().is_some());
}

#[tokio::test]
async fn bitcoin_solvency_submit_updates_protocol_state() {
    let mut app = test_app();
    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(31),
            "vout": 0,
            "value_sats": 150_000_000,
            "script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    let response: EventsResponse = request_json(
        &mut app,
        "POST",
        "/bitcoin/solvency/submit",
        Some(json!({ "expected_sats": 200_000_000 })),
    )
    .await;

    assert!(matches!(
        response.events.first(),
        Some(Event::BitcoinSolvencyReported { solvent: false, .. })
    ));
}

#[tokio::test]
async fn bitcoin_withdrawal_broadcasts_before_single_attested_publish() {
    let mut state = AppState::default();
    state.withdrawals.authorized.insert(
        "wd-attest".to_string(),
        AuthorizedWithdrawal {
            id: "wd-attest".to_string(),
            recipient: regtest_address().to_string(),
            amount_sats: 100_000,
            fee_sats: 1_000,
            nullifier_hash: "nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: String::new(),
            vault_signers: Vec::new(),
            vault_threshold: 0,
            signature: mock_signature(),
        },
    );
    let mut app = router(NodeState::new(state, None));
    request_json::<serde_json::Value>(
        &mut app,
        "POST",
        "/bitcoin/utxo/import",
        Some(json!({
            "txid": txid_for_tests(32),
            "vout": 0,
            "value_sats": 150_000_000,
            "script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;
    let built: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/build",
        Some(json!({
            "withdrawal_id": "wd-attest",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        })),
    )
    .await;

    let broadcast: serde_json::Value = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/wd-attest/broadcast",
        Some(json!({ "signed_tx_hex": built["unsigned_tx_hex"] })),
    )
    .await;
    let txid = broadcast["broadcast_txid"].as_str().unwrap().to_string();

    let response = request_raw(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/broadcast/submit",
        Some(json!({ "withdrawal_id": "wd-attest", "txid": txid })),
    )
    .await;
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);

    let attest: EventsResponse = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/wd-attest/attest",
        Some(json!({ "signed_tx_hex": built["unsigned_tx_hex"] })),
    )
    .await;
    assert!(matches!(
        attest.events.first(),
        Some(Event::BitcoinWithdrawalAttested { .. })
    ));

    let publish: EventsResponse = request_json(
        &mut app,
        "POST",
        "/bitcoin/withdrawal/broadcast/submit",
        Some(json!({ "withdrawal_id": "wd-attest", "txid": txid })),
    )
    .await;
    assert!(matches!(
        publish.events.first(),
        Some(Event::BitcoinWithdrawalBroadcastSubmitted { .. })
    ));
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

async fn bootstrap_custody(app: &mut axum::Router) {
    request_json::<EventsResponse>(
        app,
        "POST",
        "/deposit/request",
        Some(json!({
            "pow_token": pow("bond-bootstrap"),
            "user_pubkey": client_pubkey_from_secret("bond-bootstrap-seed")
        })),
    )
    .await;
}

async fn vault_script_hex_from_app(app: &mut axum::Router) -> String {
    let response: BitcoinVaultAddressResponse =
        request_json(app, "GET", "/bitcoin/vault/address", None).await;
    response.script_pubkey_hex
}

fn mock_signature() -> CustodySignature {
    CustodySignature {
        scheme: "mock-sha256".to_string(),
        signer: "mock-frost-quorum".to_string(),
        message_digest: "digest".to_string(),
        group_public_key: "mock".to_string(),
        key_tweak: None,
        signature: "signature".to_string(),
    }
}

fn authorized_withdrawal_state() -> AppState {
    let mut state = AppState::default();
    let signer = FrostCustodySigner::demo_67_percent().unwrap();
    let keyset = signer.to_keyset(0).unwrap();
    apply_event(
        &mut state,
        Event::CustodyKeysetGenerated { epoch: 0, keyset },
    )
    .unwrap();
    apply_event(
        &mut state,
        Event::WithdrawalAuthorized {
            withdrawal_id: "wd-1".to_string(),
            recipient: regtest_address().to_string(),
            amount_sats: 99_900_000,
            fee_sats: 100_000,
            nullifier_hash: "api-authorized-nullifier".to_string(),
            custody_epoch: 0,
            deposit_key_tweak: String::new(),
            vault_signers: Vec::new(),
            vault_threshold: 0,
            signature: mock_signature(),
        },
    )
    .unwrap();
    state
}

async fn prepare_confirmed_split(app: &mut axum::Router) {
    request_json::<EventsResponse>(
        app,
        "POST",
        "/deposit/request",
        Some(json!({ "pow_token": pow("api-prepare"), "user_pubkey": client_pubkey_from_secret("api-test-seed") })),
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
    let mut tree = DenominationTree::default();
    tree.insert(note.commitment.clone());
    assert_eq!(tree.root(), root);
    zk_withdrawal_from_receipt(
        note,
        "api-test-seed",
        &tree,
        regtest_address().to_string(),
        fee_sats,
    )
    .unwrap()
}

fn regtest_script_hex() -> String {
    script_hex(&regtest_address().script_pubkey())
}

fn script_hex_for_address(address: &str) -> String {
    let address = address
        .parse::<Address<_>>()
        .unwrap()
        .require_network(Network::Regtest)
        .unwrap();
    script_hex(&address.script_pubkey())
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

struct RegtestBondFaucet {
    next_tx_byte: u8,
}

impl RegtestBondFaucet {
    fn new(next_tx_byte: u8) -> Self {
        Self { next_tx_byte }
    }

    fn fund_bond(&mut self, value_sats: u64, script_pubkey_hex: String) -> RegtestUtxo {
        let tx_byte = self.next_tx_byte;
        self.next_tx_byte = self.next_tx_byte.saturating_add(1);
        RegtestUtxo {
            txid: txid_for_tests(tx_byte),
            vout: 0,
            value_sats,
            script_pubkey_hex,
            confirmations: 1,
            is_self_transfer: false,
            mempool_ancestor_count: 0,
            mempool_descendant_count: 0,
            deposit_key_tweak: None,
        }
    }
}

async fn spawn_mock_cometbft(submitted: Arc<Mutex<Vec<Command>>>) -> String {
    let app = Router::new().route(
        "/",
        post(move |Json(body): Json<serde_json::Value>| {
            let submitted = submitted.clone();
            async move {
                if body["method"] == "abci_query" {
                    let state = AppState::default();
                    let path = body["params"]["path"].as_str().unwrap();
                    let value = match path {
                        "/state/hash" => state_hash(&state).into_bytes(),
                        "/state" => serde_json::to_vec(&state).unwrap(),
                        _ => Vec::new(),
                    };
                    return Json(json!({
                        "jsonrpc": "2.0",
                        "id": body["id"].clone(),
                        "result": {
                            "response": {
                                "code": 0,
                                "log": "ok",
                                "value": BASE64_STANDARD.encode(value)
                            }
                        }
                    }));
                }
                let tx_hex = body["params"]["tx"]
                    .as_str()
                    .unwrap()
                    .strip_prefix("0x")
                    .unwrap();
                let tx = hex::decode(tx_hex).unwrap();
                submitted.lock().await.push(decode_tx(&tx).unwrap().command);
                Json(json!({
                    "jsonrpc": "2.0",
                    "id": body["id"].clone(),
                    "result": {
                        "hash": "ABC123",
                        "check_tx": { "code": 0, "log": "" },
                        "tx_result": { "code": 0, "log": "" }
                    }
                }))
            }
        }),
    );
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });
    url
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
