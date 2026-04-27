use bitcoin::secp256k1::{PublicKey as SecpPublicKey, Secp256k1, SecretKey};
use bitcoin::{Address, Network};
use reqwest::Client;
use serde::de::DeserializeOwned;
use serde_json::json;
use thornado_bitcoin::{script_hex, txid_for_tests};
use thornado_core::{
    derive_split_receipt, execute_command, mine_deposit_pow, withdrawal_from_receipt, AppState,
    Command, MockCustodySigner, MockProofVerifier,
};
use thornado_node::{router, EventsResponse, NodeState, RootResponse, StateHashResponse};
use tokio::net::TcpListener;
use tokio::task::JoinHandle;

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn five_http_nodes_with_five_dev_bitcoin_backends_run_the_same_flow() {
    let nodes = spawn_nodes(5, seeded_state()).await;
    let client = Client::new();
    let leader = &nodes[0];

    let initial_hashes = get_all::<StateHashResponse>(&client, &nodes, "/state/hash").await;
    assert_all_equal(initial_hashes.iter().map(|hash| hash.state_hash.as_str()));

    let deposit_response = post::<EventsResponse>(
        &client,
        leader,
        "/deposit/request",
        json!({ "pow_token": pow("five-node-deposit"), "user_pubkey": "five-node-client" }),
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
            "intent_id": "dep-2",
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
        json!({ "intent_id": "dep-2" }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    let receipt = derive_split_receipt("dep-2", 100_000_000, "five-node-seed").unwrap();
    post::<EventsResponse>(
        &client,
        leader,
        "/split",
        json!({
            "deposit_id": "dep-2",
            "note_commitments": receipt.commitments()
        }),
    )
    .await;
    assert_replicated(&client, &nodes).await;

    let roots = get_all::<RootResponse>(&client, &nodes, "/notes/root/100000000").await;
    assert_all_equal(roots.iter().map(|root| root.root.as_str()));

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

fn seeded_state() -> AppState {
    let mut state = AppState::default();
    let verifier = MockProofVerifier;
    let signer = MockCustodySigner;
    execute_command(
        &mut state,
        Command::RequestDepositAddress {
            pow_token: pow("five-node-bootstrap"),
            user_pubkey: "five-node-client".to_string(),
        },
        &verifier,
        &signer,
    )
    .unwrap();
    execute_command(
        &mut state,
        Command::ObserveDeposit {
            intent_id: "dep-1".to_string(),
            txid: "tx-five-node-bootstrap".to_string(),
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
    let receipt = derive_split_receipt("dep-1", 100_000_000, "five-node-bootstrap-seed").unwrap();
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

async fn assert_replicated(client: &Client, nodes: &[TestNode]) {
    let hashes = get_all::<StateHashResponse>(client, nodes, "/state/hash").await;
    assert_all_equal(hashes.iter().map(|hash| hash.state_hash.as_str()));
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
    assert!(values.iter().all(|value| value == first));
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
