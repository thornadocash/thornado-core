use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, Network};
use reqwest::Client;
use serde::de::DeserializeOwned;
use serde_json::json;
use thornado_bitcoin::{script_hex, txid_for_tests};
use thornado_core::{
    apply_event, client_pubkey_from_secret, derive_split_receipt, mine_deposit_pow,
    zk_withdrawal_from_receipt, AppState, DenominationTree, Event, FrostCustodySigner,
    FrostCustodySignerSnapshot, WithdrawalProof, WithdrawalRequest,
};
use thornado_node::{
    router, EventsResponse, NodeConfig, NodeState, RootResponse, StateHashResponse,
};
use tokio::net::TcpListener;
use tokio::task::JoinHandle;

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn five_http_nodes_with_five_dev_bitcoin_backends_run_the_same_flow() {
    let nodes = spawn_nodes(5, AppState::default()).await;
    let client = Client::new();
    let leader = &nodes[0];

    let initial_hashes = get_all::<StateHashResponse>(&client, &nodes, "/state/hash").await;
    assert_all_equal(initial_hashes.iter().map(|hash| hash.state_hash.as_str()));

    let deposit_response = post::<EventsResponse>(
        &client,
        leader,
        "/deposit/request",
        json!({ "pow_token": pow("five-node-deposit"), "user_pubkey": client_pubkey_from_secret("five-node-seed") }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    let deposit_address = deposit_response
        .events
        .iter()
        .find_map(|event| match event {
            thornado_core::Event::DepositIntentCreated {
                deposit_address, ..
            } => Some(deposit_address.as_str()),
            _ => None,
        })
        .unwrap();
    assert!(deposit_address.starts_with("bcrt1p"));

    post::<EventsResponse>(
        &client,
        leader,
        "/deposit/observe",
        json!({
            "intent_id": "dep-1",
            "txid": "tx-five-node",
            "amount_sats": 100_000_000
        }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    post::<EventsResponse>(
        &client,
        leader,
        "/deposit/confirm",
        json!({ "intent_id": "dep-1" }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    let receipt = derive_split_receipt("dep-1", 100_000_000, "five-node-seed").unwrap();
    post::<EventsResponse>(
        &client,
        leader,
        "/split",
        json!({
            "deposit_id": "dep-1",
            "note_commitments": receipt.commitments()
        }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    let roots = get_all::<RootResponse>(&client, &nodes, "/notes/root/100000000").await;
    assert_all_equal(roots.iter().map(|root| root.root.as_str()));

    let (proof, public) = public_proof_from_receipt(
        &receipt.notes[0],
        "five-node-seed",
        roots[0].root.clone(),
        regtest_address().to_string(),
        100_000,
    );
    let withdrawal = post::<EventsResponse>(
        &client,
        leader,
        "/withdraw",
        json!({ "proof": proof, "public": public }),
    )
    .await;
    assert!(withdrawal.events.iter().any(|event| {
        matches!(
            event,
            Event::WithdrawalAuthorized { signature, .. }
                if signature.scheme == "frost-secp256k1-sha256"
        )
    }));
    assert_replicated(&client, &nodes).await;

    for (index, node) in nodes.iter().enumerate() {
        post::<serde_json::Value>(
            &client,
            node,
            "/bitcoin/utxo/import",
            json!({
                "txid": txid_for_tests(20 + index as u8),
                "vout": 0,
                "value_sats": 150_000_000,
                "script_pubkey_hex": regtest_script_hex()
            }),
        )
        .await;
    }

    let built = post_all::<serde_json::Value>(
        &client,
        &nodes,
        "/bitcoin/withdrawal/build",
        json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        }),
    )
    .await;

    for tx in &built {
        assert_eq!(tx["withdrawal_id"], "wd-1");
        assert_eq!(tx["output_value_sats"], 99_900_000);
        assert!(tx["unsigned_tx_hex"].as_str().unwrap().len() > 20);
    }

    let broadcast_txid = txid_for_tests(99);
    let records = post_all::<serde_json::Value>(
        &client,
        &nodes,
        "/bitcoin/withdrawal/wd-1/mark-broadcast",
        json!({ "txid": broadcast_txid }),
    )
    .await;
    for record in records {
        assert_eq!(record["broadcast_txid"], txid_for_tests(99));
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn one_node_deploy_churns_in_one_node_at_a_time_until_five() {
    let mut current_state = AppState::default();
    let client = Client::new();
    let mut nodes = vec![spawn_node(current_state.clone(), Vec::new()).await];
    let first_registration = post::<EventsResponse>(
        &client,
        &nodes[0],
        "/churn/standby",
        json!({ "node_id": nodes[0].base_url }),
    )
    .await;
    apply_events_to_state(&mut current_state, &first_registration.events);
    let first_churn = post::<EventsResponse>(&client, &nodes[0], "/churn/start", json!({})).await;
    apply_events_to_state(&mut current_state, &first_churn.events);
    assert_active_and_standby(&current_state, 1, 0);

    for next_node in 2..=5 {
        let new_node = spawn_node(current_state.clone(), Vec::new()).await;
        nodes.push(new_node);
        let standby_node = nodes.last().unwrap();
        let standby = post::<EventsResponse>(
            &client,
            &nodes[0],
            "/churn/standby",
            json!({ "node_id": standby_node.base_url }),
        )
        .await;
        apply_events_to_state(&mut current_state, &standby.events);
        apply_events(&client, standby_node, &standby.events).await;
        assert_active_and_standby(&current_state, next_node - 1, 1);

        for node in &nodes {
            let peers = get::<serde_json::Value>(&client, node, "/peers").await;
            let expected_peer_count = if node.base_url == standby_node.base_url {
                0
            } else {
                next_node - 2
            };
            assert_eq!(
                peers["peers"].as_array().unwrap().len(),
                expected_peer_count
            );
        }

        let churn = post::<EventsResponse>(&client, &nodes[0], "/churn/start", json!({})).await;
        apply_events_to_state(&mut current_state, &churn.events);
        apply_events(&client, standby_node, &churn.events).await;
        assert!(churn.events.iter().any(|event| {
            matches!(event, Event::ChurnEpochStarted { epoch } if *epoch == next_node as u64)
        }));
        assert!(churn.events.iter().any(|event| {
            matches!(event, Event::StandbyNodeActivated { node_id, .. } if node_id == &standby_node.base_url)
        }));
        assert!(churn
            .events
            .iter()
            .any(|event| matches!(event, Event::CustodyKeysetGenerated { .. })));

        for existing in &nodes[..nodes.len() - 1] {
            add_peer(&client, existing, standby_node).await;
            add_peer(&client, standby_node, existing).await;
        }
        assert_active_and_standby(&current_state, next_node, 0);
        assert_replicated(&client, &nodes).await;

        for node in &nodes {
            let peers = get::<serde_json::Value>(&client, node, "/peers").await;
            assert_eq!(peers["peers"].as_array().unwrap().len(), next_node - 1);
        }
    }

    assert_eq!(nodes.len(), 5);
    assert_replicated(&client, &nodes).await;

    let deposit = post::<EventsResponse>(
        &client,
        &nodes[0],
        "/deposit/request",
        json!({ "pow_token": pow("post-churn-deposit"), "user_pubkey": client_pubkey_from_secret("post-churn-seed") }),
    )
    .await;
    apply_events_to_state(&mut current_state, &deposit.events);
    assert_replicated(&client, &nodes).await;

    post::<EventsResponse>(
        &client,
        &nodes[0],
        "/deposit/observe",
        json!({
            "intent_id": "dep-1",
            "txid": "tx-post-churn",
            "amount_sats": 100_000_000
        }),
    )
    .await;
    post::<EventsResponse>(
        &client,
        &nodes[0],
        "/deposit/confirm",
        json!({ "intent_id": "dep-1" }),
    )
    .await;
    let receipt = derive_split_receipt("dep-1", 100_000_000, "post-churn-seed").unwrap();
    post::<EventsResponse>(
        &client,
        &nodes[0],
        "/split",
        json!({
            "deposit_id": "dep-1",
            "note_commitments": receipt.commitments()
        }),
    )
    .await;
    let root: RootResponse = get(&client, &nodes[0], "/notes/root/100000000").await;
    let (proof, public) = public_proof_from_receipt(
        &receipt.notes[0],
        "post-churn-seed",
        root.root,
        regtest_address().to_string(),
        100_000,
    );
    post::<EventsResponse>(
        &client,
        &nodes[0],
        "/withdraw",
        json!({ "proof": proof, "public": public }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    for (index, node) in nodes.iter().enumerate() {
        post::<serde_json::Value>(
            &client,
            node,
            "/bitcoin/utxo/import",
            json!({
                "txid": txid_for_tests(50 + index as u8),
                "vout": 0,
                "value_sats": 150_000_000,
                "script_pubkey_hex": regtest_script_hex()
            }),
        )
        .await;
    }

    let built = post_all::<serde_json::Value>(
        &client,
        &nodes,
        "/bitcoin/withdrawal/build",
        json!({
            "withdrawal_id": "wd-1",
            "fee_rate_sats_per_vb": 2,
            "change_script_pubkey_hex": regtest_script_hex()
        }),
    )
    .await;
    assert_eq!(built.len(), 5);
    for tx in built {
        assert_eq!(tx["withdrawal_id"], "wd-1");
        assert_eq!(tx["output_value_sats"], 99_900_000);
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn five_http_nodes_run_threshold_frost_keysign_over_http() {
    let signer =
        FrostCustodySigner::generate_with_dkg(5, thornado_core::frost_threshold_for_committee(5))
            .unwrap();
    let mut initial_state = AppState::default();
    apply_event(
        &mut initial_state,
        Event::CustodyKeysetGenerated {
            epoch: 0,
            keyset: signer.to_keyset(0).unwrap(),
        },
    )
    .unwrap();
    let child = thornado_core::derive_deposit_child_key(
        &initial_state,
        "dep-http-1",
        &pow("http-child-keysign"),
        "http-client",
    )
    .unwrap();
    let nodes = spawn_nodes_with_frost_shares(5, initial_state, &signer).await;
    let client = Client::new();
    let request = WithdrawalRequest {
        withdrawal_id: "wd-http-frost".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "http-nullifier".to_string(),
    };

    let signature: thornado_core::CustodySignature = post(
        &client,
        &nodes[0],
        "/frost/keysign",
        json!({
            "request": request,
            "key_tweak": child.key_tweak
        }),
    )
    .await;

    assert_eq!(signature.scheme, "frost-secp256k1-sha256+tweak");
    assert_eq!(signature.group_public_key, child.child_public_key);
    thornado_core::verify_custody_signature(
        &WithdrawalRequest {
            withdrawal_id: "wd-http-frost".to_string(),
            recipient: "tb1qrecipient".to_string(),
            amount_sats: 99_900_000,
            fee_sats: 100_000,
            nullifier_hash: "http-nullifier".to_string(),
        },
        &signature,
    )
    .unwrap();
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
#[cfg_attr(
    not(feature = "proof-tests"),
    ignore = "expensive proof test; run with `cargo test -p thornado-node --features proof-tests`"
)]
async fn five_http_nodes_churn_with_live_frost_keygen_then_deposit_and_keysign() {
    let nodes = spawn_nodes_with_empty_frost_paths(5, |base_urls| {
        let mut state = AppState::default();
        for node_id in base_urls.iter().take(4) {
            apply_event(
                &mut state,
                Event::StandbyNodeActivated {
                    node_id: node_id.clone(),
                    epoch: 0,
                },
            )
            .unwrap();
        }
        apply_event(
            &mut state,
            Event::StandbyNodeRegistered {
                node_id: base_urls[4].clone(),
            },
        )
        .unwrap();
        state
    })
    .await;
    let client = Client::new();

    let churn = post::<EventsResponse>(&client, &nodes[2], "/churn/start", json!({})).await;
    assert!(churn.events.iter().any(|event| {
        matches!(
            event,
            Event::CustodyKeysetGenerated {
                epoch: 1,
                keyset,
            } if keyset.max_signers == 5
                && keyset.threshold == thornado_core::frost_threshold_for_committee(5)
        )
    }));
    assert_replicated(&client, &nodes).await;

    let deposit = post::<EventsResponse>(
        &client,
        &nodes[0],
        "/deposit/request",
        json!({ "pow_token": pow("live-keygen-deposit"), "user_pubkey": client_pubkey_from_secret("live-keygen-seed") }),
    )
    .await;
    let mut key_state = AppState::default();
    let keyset_event = churn
        .events
        .iter()
        .find(|event| matches!(event, Event::CustodyKeysetGenerated { .. }))
        .cloned()
        .unwrap();
    apply_event(&mut key_state, keyset_event).unwrap();
    let key_tweak = thornado_core::derive_deposit_child_key(
        &key_state,
        "dep-1",
        &pow("live-keygen-deposit"),
        &client_pubkey_from_secret("live-keygen-seed"),
    )
    .unwrap()
    .key_tweak;
    let (deposit_address, key_tweak) = deposit
        .events
        .iter()
        .find_map(|event| match event {
            Event::DepositIntentCreated {
                deposit_address, ..
            } => Some((deposit_address.clone(), key_tweak.clone())),
            _ => None,
        })
        .unwrap();
    assert!(deposit_address.starts_with("bcrt1p"));
    assert_replicated(&client, &nodes).await;

    let request = WithdrawalRequest {
        withdrawal_id: "wd-live-frost".to_string(),
        recipient: "tb1qrecipient".to_string(),
        amount_sats: 99_900_000,
        fee_sats: 100_000,
        nullifier_hash: "live-nullifier".to_string(),
    };
    let signature: thornado_core::CustodySignature = post(
        &client,
        &nodes[4],
        "/frost/keysign",
        json!({
            "request": request,
            "key_tweak": key_tweak
        }),
    )
    .await;
    assert_eq!(signature.scheme, "frost-secp256k1-sha256+tweak");
    thornado_core::verify_custody_signature(
        &WithdrawalRequest {
            withdrawal_id: "wd-live-frost".to_string(),
            recipient: "tb1qrecipient".to_string(),
            amount_sats: 99_900_000,
            fee_sats: 100_000,
            nullifier_hash: "live-nullifier".to_string(),
        },
        &signature,
    )
    .unwrap();
}

struct TestNode {
    base_url: String,
    task: JoinHandle<()>,
}

impl Drop for TestNode {
    fn drop(&mut self) {
        self.task.abort();
    }
}

async fn spawn_nodes(count: usize, initial_state: AppState) -> Vec<TestNode> {
    let mut listeners = Vec::new();
    let mut base_urls = Vec::new();
    for _ in 0..count {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        base_urls.push(format!("http://{}", listener.local_addr().unwrap()));
        listeners.push(listener);
    }

    let mut nodes = Vec::new();
    for (index, listener) in listeners.into_iter().enumerate() {
        let peers = base_urls
            .iter()
            .enumerate()
            .filter_map(|(peer_index, url)| (peer_index != index).then_some(url.clone()))
            .collect();
        let app = router(NodeState::with_peers(initial_state.clone(), None, peers));
        let task = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        nodes.push(TestNode {
            base_url: base_urls[index].clone(),
            task,
        });
    }
    nodes
}

async fn spawn_nodes_with_frost_shares(
    count: usize,
    initial_state: AppState,
    signer: &FrostCustodySigner,
) -> Vec<TestNode> {
    let dir = std::env::temp_dir().join(format!("thornado-frost-http-{}", std::process::id()));
    let _ = std::fs::create_dir_all(&dir);
    let signer_ids = signer.signer_ids();
    let mut listeners = Vec::new();
    let mut base_urls = Vec::new();
    for _ in 0..count {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        base_urls.push(format!("http://{}", listener.local_addr().unwrap()));
        listeners.push(listener);
    }

    let mut nodes = Vec::new();
    for (index, listener) in listeners.into_iter().enumerate() {
        let path = dir.join(format!("share-{index}.json"));
        let snapshot: FrostCustodySignerSnapshot =
            signer.snapshot_for_signer_id(&signer_ids[index]).unwrap();
        std::fs::write(&path, serde_json::to_vec(&snapshot).unwrap()).unwrap();
        let peers = base_urls
            .iter()
            .enumerate()
            .filter_map(|(peer_index, url)| (peer_index != index).then_some(url.clone()))
            .collect();
        let app = router(
            NodeState::with_config(
                initial_state.clone(),
                NodeConfig {
                    snapshot_path: None,
                    frost_signer_path: Some(path),
                    bitcoin_state_path: None,
                    bitcoin_rpc: None,
                    node_id: Some(base_urls[index].clone()),
                    churn_cycle_ms: None,
                },
                peers,
            )
            .unwrap(),
        );
        let task = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        nodes.push(TestNode {
            base_url: base_urls[index].clone(),
            task,
        });
    }
    nodes
}

async fn spawn_nodes_with_empty_frost_paths<F>(count: usize, build_state: F) -> Vec<TestNode>
where
    F: FnOnce(&[String]) -> AppState,
{
    let dir = std::env::temp_dir().join(format!("thornado-live-frost-http-{}", std::process::id()));
    let _ = std::fs::create_dir_all(&dir);
    let mut listeners = Vec::new();
    let mut base_urls = Vec::new();
    for _ in 0..count {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        base_urls.push(format!("http://{}", listener.local_addr().unwrap()));
        listeners.push(listener);
    }
    let initial_state = build_state(&base_urls);

    let mut nodes = Vec::new();
    for (index, listener) in listeners.into_iter().enumerate() {
        let peers = base_urls
            .iter()
            .enumerate()
            .filter_map(|(peer_index, url)| (peer_index != index).then_some(url.clone()))
            .collect();
        let app = router(
            NodeState::with_config(
                initial_state.clone(),
                NodeConfig {
                    snapshot_path: None,
                    frost_signer_path: Some(dir.join(format!("share-{index}.json"))),
                    bitcoin_state_path: None,
                    bitcoin_rpc: None,
                    node_id: Some(base_urls[index].clone()),
                    churn_cycle_ms: None,
                },
                peers,
            )
            .unwrap(),
        );
        let task = tokio::spawn(async move {
            axum::serve(listener, app).await.unwrap();
        });
        nodes.push(TestNode {
            base_url: base_urls[index].clone(),
            task,
        });
    }
    nodes
}

async fn spawn_node(initial_state: AppState, peers: Vec<String>) -> TestNode {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let base_url = format!("http://{}", listener.local_addr().unwrap());
    let app = router(NodeState::with_peers(initial_state, None, peers));
    let task = tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });
    TestNode { base_url, task }
}

fn public_proof_from_receipt(
    note: &thornado_core::NoteReceipt,
    client_seed: &str,
    root: String,
    recipient: String,
    fee_sats: u64,
) -> (WithdrawalProof, thornado_core::WithdrawalPublicInputs) {
    let mut tree = DenominationTree::default();
    tree.insert(note.commitment.clone());
    assert_eq!(tree.root(), root);
    zk_withdrawal_from_receipt(note, client_seed, &tree, recipient, fee_sats).unwrap()
}

async fn assert_replicated(client: &Client, nodes: &[TestNode]) {
    let hashes = get_all::<StateHashResponse>(client, nodes, "/state/hash").await;
    assert_all_equal(hashes.iter().map(|hash| hash.state_hash.as_str()));
}

async fn add_peer(client: &Client, node: &TestNode, peer: &TestNode) {
    post::<serde_json::Value>(client, node, "/peers/add", json!({ "url": peer.base_url })).await;
}

async fn apply_events(client: &Client, node: &TestNode, events: &[Event]) {
    post::<EventsResponse>(client, node, "/events/apply", json!({ "events": events })).await;
}

fn apply_events_to_state(state: &mut AppState, events: &[Event]) {
    for event in events.iter().cloned() {
        apply_event(state, event).unwrap();
    }
}

fn assert_active_and_standby(state: &AppState, active: usize, standby: usize) {
    assert_eq!(state.churn.active_nodes.len(), active);
    assert_eq!(state.churn.standby_nodes.len(), standby);
}

async fn get_all<T: DeserializeOwned>(client: &Client, nodes: &[TestNode], path: &str) -> Vec<T> {
    let mut responses = Vec::new();
    for node in nodes {
        responses.push(get(client, node, path).await);
    }
    responses
}

async fn post_all<T: DeserializeOwned>(
    client: &Client,
    nodes: &[TestNode],
    path: &str,
    body: serde_json::Value,
) -> Vec<T> {
    let mut responses = Vec::new();
    for node in nodes {
        responses.push(post(client, node, path, body.clone()).await);
    }
    responses
}

async fn get<T: DeserializeOwned>(client: &Client, node: &TestNode, path: &str) -> T {
    client
        .get(format!("{}{}", node.base_url, path))
        .send()
        .await
        .unwrap()
        .error_for_status()
        .unwrap()
        .json::<T>()
        .await
        .unwrap()
}

async fn post<T: DeserializeOwned>(
    client: &Client,
    node: &TestNode,
    path: &str,
    body: serde_json::Value,
) -> T {
    let response = client
        .post(format!("{}{}", node.base_url, path))
        .json(&body)
        .send()
        .await
        .unwrap();
    let status = response.status();
    let bytes = response.bytes().await.unwrap();
    assert!(
        status.is_success(),
        "POST {path} failed: {}",
        String::from_utf8_lossy(&bytes)
    );
    serde_json::from_slice::<T>(&bytes).unwrap()
}

fn assert_all_equal<'a>(values: impl IntoIterator<Item = &'a str>) {
    let values = values.into_iter().collect::<Vec<_>>();
    let first = values.first().unwrap();
    assert!(
        values.iter().all(|value| value == first),
        "values are not equal: {values:?}"
    );
}

fn regtest_script_hex() -> String {
    script_hex(&regtest_address().script_pubkey())
}

fn regtest_address() -> Address {
    let secp = Secp256k1::new();
    let secret = SecretKey::from_slice(&[3_u8; 32]).unwrap();
    let public_key = bitcoin::CompressedPublicKey(SecpPublicKey::from_secret_key(&secp, &secret));
    Address::p2wpkh(&public_key, Network::Regtest)
}

fn pow(label: &str) -> String {
    mine_deposit_pow(label)
}
