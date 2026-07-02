//! mock-thornado: pressure-test harness that stands in for a thornado
//! validator's chain endpoints, so a fleet of Rust bifrosts exercises its
//! FULL production path against a real bitcoind:
//!
//! - serves `/thornado/lastblock` + `/thornado/keysign/{h}/{pk}` with
//!   ECDSA-signed payloads (the daemon verifies them like production);
//! - serves `/cosmos/auth/v1beta1/accounts/{addr}` and a CometBFT-style
//!   `broadcast_tx_sync`, decoding the posted `TxRaw` protobuf and verifying
//!   the SIGN_MODE_DIRECT signature and account sequence of every observation;
//! - generates withdrawal work (prescribed UTXO inputs, like the chain),
//!   drives deposits + mining (+ optional forced reorgs) on regtest bitcoind;
//! - tracks batch completion (all prescribed inputs spent on-chain) and
//!   exposes `/stats` for the monitor.
//!
//! Test-only tooling: nothing here runs in production.

use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicI64, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use axum::extract::{Path, State};
use axum::routing::{get, post};
use axum::{Json, Router};
use clap::Parser;
use prost::Message as _;

use thornado_bifrost_signer::bitcoind::{BitcoindConfig, BitcoindRpc};
use thornado_bifrost_signer::chain::{Coin, TxOut, TxOutInput, TxOutItem};
use thornado_bifrost_signer::cosmos_tx;
use thornado_bifrost_signer::tx_builder::TaprootVault;

#[derive(Parser, Debug)]
#[command(name = "mock-thornado", about = "thornado chain stand-in for bifrost pressure tests")]
struct Args {
    /// REST listen address (thornado API)
    #[arg(long, default_value = "0.0.0.0:1317")]
    listen_rest: String,
    /// RPC listen address (CometBFT-style broadcast)
    #[arg(long, default_value = "0.0.0.0:26657")]
    listen_rpc: String,
    #[arg(long, default_value = "thornado-rust-e2e")]
    chain_id: String,
    /// FROST group pubkey (hex, compressed) — the vault under test
    #[arg(long)]
    vault_pubkey: String,
    /// keysign payload signing key (32-byte hex); generated if omitted
    #[arg(long)]
    keysign_key: Option<String>,
    #[arg(long, default_value = "127.0.0.1:24700")]
    btc_rpc_host: String,
    #[arg(long, default_value = "test")]
    btc_rpc_user: String,
    #[arg(long, default_value = "test")]
    btc_rpc_pass: String,
    /// funded wallet used for deposits, mining rewards and dest addresses
    #[arg(long, default_value = "miner")]
    btc_wallet: String,
    /// thornado block time, seconds
    #[arg(long, default_value_t = 2)]
    block_secs: u64,
    /// cadence for creating a new withdrawal batch, seconds
    #[arg(long, default_value_t = 15)]
    gen_secs: u64,
    /// outbound items per batch
    #[arg(long, default_value_t = 3)]
    batch_size: usize,
    /// max concurrently pending batches
    #[arg(long, default_value_t = 1)]
    max_pending: usize,
    /// cadence for vault deposits, seconds
    #[arg(long, default_value_t = 7)]
    deposit_secs: u64,
    /// cadence for mining a regtest block, seconds
    #[arg(long, default_value_t = 5)]
    mine_secs: u64,
    /// force a depth-2 reorg every N seconds (0 = never)
    #[arg(long, default_value_t = 0)]
    reorg_secs: u64,
    /// expected signer count (for quorum stats)
    #[arg(long, default_value_t = 4)]
    signers: usize,
    /// outbound gas rate served with work items (sats/vB)
    #[arg(long, default_value_t = 10)]
    gas_rate: i64,
}

struct Account {
    number: u64,
    sequence: u64,
}

struct PendingBatch {
    height: i64,
    payload_json: String,
    signature_b64: String,
    inputs: Vec<(String, u32)>,
    created_unix: u64,
    /// Times the keysign endpoint served this batch at its exact height.
    served: u64,
}

#[derive(Default)]
struct Stats {
    batches_created: u64,
    batches_completed: u64,
    batches_stalled: u64,
    completion_secs_total: u64,
    completion_secs_max: u64,
    obs_in_txs: u64,
    obs_out_txs: u64,
    posts_accepted: u64,
    posts_bad_sig: u64,
    posts_bad_seq: u64,
    posts_undecodable: u64,
    deposits_made: u64,
    blocks_mined: u64,
    reorgs_forced: u64,
    per_signer_posts: HashMap<String, u64>,
    /// txid -> distinct signer pubkeys that observed it (bounded)
    seen_in: HashMap<String, HashSet<String>>,
    seen_out: HashMap<String, HashSet<String>>,
}

struct App {
    chain_id: String,
    vault_pubkey: String,
    vault_addr: String,
    keysign_sk: bitcoin::secp256k1::SecretKey,
    height: AtomicI64,
    next_account: AtomicU64,
    accounts: Mutex<HashMap<String, Account>>,
    pending: Mutex<Vec<PendingBatch>>,
    reserved: Mutex<HashSet<String>>,
    stats: Mutex<Stats>,
    btc: BitcoindRpc,
    /// Watch-only wallet tracking the vault address (for listunspent).
    btc_vault: BitcoindRpc,
    gas_rate: i64,
    batch_size: usize,
    max_pending: usize,
    signers: usize,
}

fn now_unix() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()),
        )
        .init();
    let args = Args::parse();

    let secp = bitcoin::secp256k1::Secp256k1::new();
    let keysign_sk = match &args.keysign_key {
        Some(h) => bitcoin::secp256k1::SecretKey::from_slice(&hex::decode(h)?)?,
        None => {
            let mut seed = [0u8; 32];
            use rand::RngCore;
            rand::rngs::OsRng.fill_bytes(&mut seed);
            bitcoin::secp256k1::SecretKey::from_slice(&seed)?
        }
    };
    let keysign_pub = keysign_sk.public_key(&secp);
    println!("keysign_pubkey={}", hex::encode(keysign_pub.serialize()));

    let vault_pk = hex::decode(&args.vault_pubkey)?;
    let vault = TaprootVault::derive(&vault_pk, 0).map_err(|e| anyhow::anyhow!(e))?;
    let vault_addr = bitcoin::Address::from_script(
        vault.script_pubkey().as_script(),
        bitcoin::Network::Regtest,
    )?
    .to_string();
    println!("vault_address={vault_addr}");

    let btc = BitcoindRpc::new(BitcoindConfig {
        host: args.btc_rpc_host.clone(),
        user: args.btc_rpc_user.clone(),
        password: args.btc_rpc_pass.clone(),
        wallet: Some(args.btc_wallet.clone()),
    });
    let btc_vault = BitcoindRpc::new(BitcoindConfig {
        host: args.btc_rpc_host.clone(),
        user: args.btc_rpc_user.clone(),
        password: args.btc_rpc_pass.clone(),
        wallet: Some("vaultwatch".into()),
    });

    let app = Arc::new(App {
        chain_id: args.chain_id.clone(),
        vault_pubkey: args.vault_pubkey.clone(),
        vault_addr: vault_addr.clone(),
        keysign_sk,
        // Wall-clock-derived so a harness restart never rewinds below the
        // daemons' fetch cursor.
        height: AtomicI64::new((now_unix() / args.block_secs.max(1)) as i64),
        next_account: AtomicU64::new(1),
        accounts: Mutex::new(HashMap::new()),
        pending: Mutex::new(Vec::new()),
        reserved: Mutex::new(HashSet::new()),
        stats: Mutex::new(Stats::default()),
        btc,
        btc_vault,
        gas_rate: args.gas_rate,
        batch_size: args.batch_size.max(2),
        max_pending: args.max_pending.max(1),
        signers: args.signers,
    });

    bootstrap_wallet(&app).await;

    // Height ticker.
    {
        let app = app.clone();
        let secs = args.block_secs.max(1);
        tokio::spawn(async move {
            let mut t = tokio::time::interval(std::time::Duration::from_secs(secs));
            loop {
                t.tick().await;
                app.height.fetch_add(1, Ordering::SeqCst);
            }
        });
    }
    // Miner.
    {
        let app = app.clone();
        let secs = args.mine_secs.max(1);
        tokio::spawn(async move {
            let miner_addr = loop {
                match app.btc.get_new_address().await {
                    Ok(a) => break a,
                    Err(e) => {
                        tracing::warn!(error = %e, "getnewaddress failed; retrying");
                        tokio::time::sleep(std::time::Duration::from_secs(3)).await;
                    }
                }
            };
            let mut t = tokio::time::interval(std::time::Duration::from_secs(secs));
            loop {
                t.tick().await;
                match app.btc.generate_to_address(1, &miner_addr).await {
                    Ok(_) => {
                        app.stats.lock().unwrap().blocks_mined += 1;
                    }
                    Err(e) => tracing::warn!(error = %e, "mining failed"),
                }
            }
        });
    }
    // Depositor.
    {
        let app = app.clone();
        let secs = args.deposit_secs.max(1);
        tokio::spawn(async move {
            let mut t = tokio::time::interval(std::time::Duration::from_secs(secs));
            let mut n = 0u64;
            loop {
                t.tick().await;
                n += 1;
                let amount_btc = 0.01 + (n % 5) as f64 * 0.01;
                match app.btc.send_to_address(&app.vault_addr, amount_btc).await {
                    Ok(txid) => {
                        app.stats.lock().unwrap().deposits_made += 1;
                        tracing::info!(%txid, amount_btc, "deposited to vault");
                    }
                    Err(e) => tracing::warn!(error = %e, "deposit failed"),
                }
            }
        });
    }
    // Reorg stressor.
    if args.reorg_secs > 0 {
        let app = app.clone();
        let secs = args.reorg_secs;
        tokio::spawn(async move {
            let mut t = tokio::time::interval(std::time::Duration::from_secs(secs));
            t.tick().await; // skip immediate first tick
            loop {
                t.tick().await;
                if let Err(e) = force_reorg(&app).await {
                    tracing::warn!(error = %e, "forced reorg failed");
                }
            }
        });
    }
    // Work generator + completion checker.
    {
        let app = app.clone();
        let secs = args.gen_secs.max(2);
        tokio::spawn(async move {
            let mut t = tokio::time::interval(std::time::Duration::from_secs(secs));
            loop {
                t.tick().await;
                check_completions(&app).await;
                if let Err(e) = maybe_generate_batch(&app).await {
                    tracing::warn!(error = %e, "work generation failed");
                }
            }
        });
    }

    let rest = Router::new()
        .route("/thornado/lastblock", get(lastblock))
        .route("/thornado/keysign/:height/:pubkey", get(keysign))
        .route("/cosmos/auth/v1beta1/accounts/:addr", get(auth_account))
        .route("/stats", get(stats))
        .with_state(app.clone());
    let rpc = Router::new().route("/", post(rpc_broadcast)).with_state(app);

    let rest_listener = tokio::net::TcpListener::bind(&args.listen_rest).await?;
    let rpc_listener = tokio::net::TcpListener::bind(&args.listen_rpc).await?;
    tracing::info!(rest = %args.listen_rest, rpc = %args.listen_rpc, "mock-thornado serving");
    let rest_srv = tokio::spawn(async move { axum::serve(rest_listener, rest).await });
    let rpc_srv = tokio::spawn(async move { axum::serve(rpc_listener, rpc).await });
    let (a, b) = tokio::try_join!(rest_srv, rpc_srv)?;
    a?;
    b?;
    Ok(())
}

/// Make sure the miner wallet has spendable funds (mine past coinbase
/// maturity) and that a watch-only wallet tracks the vault address, so the
/// work generator can see the vault's UTXOs via listunspent.
async fn bootstrap_wallet(app: &Arc<App>) {
    // Watch-only vault wallet. createwallet/loadwallet fail when it already
    // exists/is loaded — both are fine, the import below is idempotent.
    if let Err(e) = app.btc.create_wallet("vaultwatch", true).await {
        tracing::debug!(error = %e, "createwallet vaultwatch (may already exist)");
        if let Err(e) = app.btc.load_wallet("vaultwatch").await {
            tracing::debug!(error = %e, "loadwallet vaultwatch (may already be loaded)");
        }
    }
    let raw_desc = format!("addr({})", app.vault_addr);
    match app.btc.get_descriptor_info(&raw_desc).await {
        Ok(info) => {
            let desc = info["descriptor"].as_str().unwrap_or(&raw_desc).to_string();
            match app
                .btc_vault
                .import_descriptors(serde_json::json!([{"desc": desc, "timestamp": 0}]))
                .await
            {
                Ok(_) => tracing::info!(vault = %app.vault_addr, "vault descriptor imported"),
                Err(e) => tracing::warn!(error = %e, "importdescriptors failed"),
            }
        }
        Err(e) => tracing::warn!(error = %e, "getdescriptorinfo failed"),
    }

    match app.btc.get_balance().await {
        Ok(b) if b >= 1.0 => {
            tracing::info!(balance = b, "miner wallet funded");
        }
        Ok(_) | Err(_) => {
            let addr = match app.btc.get_new_address().await {
                Ok(a) => a,
                Err(e) => {
                    tracing::warn!(error = %e, "wallet bootstrap: getnewaddress failed");
                    return;
                }
            };
            match app.btc.generate_to_address(101, &addr).await {
                Ok(_) => tracing::info!("mined 101 bootstrap blocks"),
                Err(e) => tracing::warn!(error = %e, "bootstrap mining failed"),
            }
        }
    }
}

/// Depth-2 reorg: invalidate tip-1, then mine 3 replacement blocks.
async fn force_reorg(app: &Arc<App>) -> Result<(), String> {
    let count = app.btc.get_block_count().await.map_err(|e| e.to_string())?;
    if count < 3 {
        return Ok(());
    }
    let hash = app
        .btc
        .get_block_hash(count - 1)
        .await
        .map_err(|e| e.to_string())?;
    app.btc.invalidate_block(&hash).await.map_err(|e| e.to_string())?;
    let addr = app.btc.get_new_address().await.map_err(|e| e.to_string())?;
    app.btc
        .generate_to_address(3, &addr)
        .await
        .map_err(|e| e.to_string())?;
    app.stats.lock().unwrap().reorgs_forced += 1;
    tracing::info!(depth = 2, "forced reorg");
    Ok(())
}

/// A batch completes when every prescribed input is spent on-chain/in-mempool
/// — bitcoind accepted the FROST-signed spend, the core success signal.
async fn check_completions(app: &Arc<App>) {
    let candidates: Vec<(i64, Vec<(String, u32)>, u64, u64)> = {
        let pending = app.pending.lock().unwrap();
        pending
            .iter()
            .map(|b| (b.height, b.inputs.clone(), b.created_unix, b.served))
            .collect()
    };
    for (height, inputs, created, served) in candidates {
        let mut all_spent = true;
        for (txid, vout) in &inputs {
            match app.btc.get_tx_out(txid, *vout).await {
                Ok(None) => {}
                Ok(Some(_)) => {
                    all_spent = false;
                    break;
                }
                Err(e) => {
                    tracing::warn!(error = %e, "gettxout failed");
                    all_spent = false;
                    break;
                }
            }
        }
        let age = now_unix().saturating_sub(created);
        if all_spent {
            let mut pending = app.pending.lock().unwrap();
            pending.retain(|b| b.height != height);
            let mut reserved = app.reserved.lock().unwrap();
            for (txid, vout) in &inputs {
                reserved.remove(&format!("{txid}-{vout}"));
            }
            let mut stats = app.stats.lock().unwrap();
            stats.batches_completed += 1;
            stats.completion_secs_total += age;
            stats.completion_secs_max = stats.completion_secs_max.max(age);
            tracing::info!(height, age_secs = age, "batch completed (inputs spent)");
        } else if served == 0 && age > 60 {
            // Never delivered (no daemon was at that height) — drop it so the
            // generator recreates the work at a current height.
            let mut pending = app.pending.lock().unwrap();
            pending.retain(|b| b.height != height);
            let mut reserved = app.reserved.lock().unwrap();
            for (txid, vout) in &inputs {
                reserved.remove(&format!("{txid}-{vout}"));
            }
            tracing::warn!(height, "batch never served; requeueing at a new height");
        } else if age > 300 {
            let mut stats = app.stats.lock().unwrap();
            stats.batches_stalled += 1;
            tracing::warn!(height, age_secs = age, served, "batch still pending (stall?)");
        }
    }
}

/// Create a new withdrawal batch if below the pending cap and the vault has
/// confirmed funds. Prescribes the exact inputs (like the chain does), so all
/// signing parties build the identical transaction.
async fn maybe_generate_batch(app: &Arc<App>) -> Result<(), String> {
    if app.pending.lock().unwrap().len() >= app.max_pending {
        return Ok(());
    }
    let unspent = app
        .btc_vault
        .list_unspent(std::slice::from_ref(&app.vault_addr))
        .await
        .map_err(|e| e.to_string())?;
    let reserved = app.reserved.lock().unwrap().clone();
    let mut candidates: Vec<_> = unspent
        .iter()
        .filter(|u| u.confirmations >= 1 && !reserved.contains(&format!("{}-{}", u.txid, u.vout)))
        .collect();
    candidates.sort_by(|a, b| {
        b.confirmations
            .cmp(&a.confirmations)
            .then_with(|| a.txid.cmp(&b.txid))
    });
    if candidates.is_empty() {
        return Ok(());
    }

    let h = app.height.load(Ordering::SeqCst);
    let serve_height = h + 2;

    // Withdrawal amounts: deterministic pseudo-variety, all above dust.
    let amounts: Vec<u64> = (0..app.batch_size)
        .map(|i| 50_000 + ((serve_height as u64 * 7919 + i as u64 * 104_729) % 150_000))
        .collect();
    let total: u64 = amounts.iter().sum();

    // Prescribe inputs covering total + generous fee buffer.
    let mut inputs = Vec::new();
    let mut covered = 0u64;
    for u in candidates {
        let sats = (u.amount * 1e8).round() as u64;
        inputs.push(TxOutInput {
            tx_id: u.txid.clone(),
            vout: u.vout,
            amount_sats: sats,
        });
        covered += sats;
        if covered >= total + 100_000 {
            break;
        }
    }
    if covered < total + 100_000 {
        tracing::info!(covered, need = total + 100_000, "vault not funded enough yet");
        return Ok(());
    }

    let mut tx_array = Vec::with_capacity(app.batch_size);
    for (i, amount) in amounts.iter().enumerate() {
        let dest = app.btc.get_new_address().await.map_err(|e| e.to_string())?;
        let in_hash = {
            use sha2::{Digest, Sha256};
            hex::encode(Sha256::digest(format!("in|{serve_height}|{i}").as_bytes()))
                .to_uppercase()
        };
        tx_array.push(TxOutItem {
            chain: "BTC".into(),
            to_address: dest,
            vault_pub_key: app.vault_pubkey.clone(),
            coin: Coin {
                asset: "BTC.BTC".into(),
                amount: amount.to_string(),
            },
            max_gas: vec![Coin {
                asset: "BTC.BTC".into(),
                amount: "10000".into(),
            }],
            gas_rate: app.gas_rate,
            in_hash,
            out_hash: String::new(),
            out_vout: 0,
            vault_path_index: 0,
            tx_type: "out".into(),
            source_inputs: inputs.clone(),
        });
    }

    let txout = TxOut {
        height: serve_height,
        tx_array,
        epoch: 1,
        status: "pending".into(),
        signing_leader: String::new(),
        signing_attempt: 0,
        retry_until_height: serve_height + 1000,
    };
    let payload_json = serde_json::to_string(&txout).map_err(|e| e.to_string())?;

    // Sign like thornadod: ECDSA compact over SHA256(compact payload JSON).
    let signature_b64 = {
        use base64::Engine as _;
        use sha2::{Digest, Sha256};
        let secp = bitcoin::secp256k1::Secp256k1::signing_only();
        let digest: [u8; 32] = Sha256::digest(payload_json.as_bytes()).into();
        let msg = bitcoin::secp256k1::Message::from_digest(digest);
        let sig = secp.sign_ecdsa(&msg, &app.keysign_sk);
        base64::engine::general_purpose::STANDARD.encode(sig.serialize_compact())
    };

    {
        let mut reserved = app.reserved.lock().unwrap();
        for i in &inputs {
            reserved.insert(format!("{}-{}", i.tx_id, i.vout));
        }
    }
    let input_refs: Vec<(String, u32)> =
        inputs.iter().map(|i| (i.tx_id.clone(), i.vout)).collect();
    app.pending.lock().unwrap().push(PendingBatch {
        height: serve_height,
        payload_json,
        signature_b64,
        inputs: input_refs,
        created_unix: now_unix(),
        served: 0,
    });
    app.stats.lock().unwrap().batches_created += 1;
    tracing::info!(height = serve_height, items = app.batch_size, inputs = inputs.len(), total_sats = total, "created withdrawal batch");
    Ok(())
}

// ---------------------------------------------------------------------------
// REST handlers
// ---------------------------------------------------------------------------

async fn lastblock(State(app): State<Arc<App>>) -> Json<serde_json::Value> {
    let h = app.height.load(Ordering::SeqCst);
    Json(serde_json::json!([{
        "chain": "BTC",
        "thornado": h,
        "last_observed_in": 0,
        "last_signed_out": 0,
    }]))
}

async fn keysign(
    State(app): State<Arc<App>>,
    Path((height, pubkey)): Path<(i64, String)>,
) -> ([(&'static str, &'static str); 1], String) {
    let headers = [("content-type", "application/json")];
    if pubkey != app.vault_pubkey {
        return (headers, r#"{"keysign":{"height":0,"tx_array":[]},"signature":""}"#.into());
    }
    let mut pending = app.pending.lock().unwrap();
    if let Some(batch) = pending.iter_mut().find(|b| b.height == height) {
        batch.served += 1;
        let body = format!(
            "{{\"keysign\":{},\"signature\":\"{}\"}}",
            batch.payload_json, batch.signature_b64
        );
        return (headers, body);
    }
    (headers, r#"{"keysign":{"height":0,"tx_array":[]},"signature":""}"#.into())
}

async fn auth_account(
    State(app): State<Arc<App>>,
    Path(addr): Path<String>,
) -> Json<serde_json::Value> {
    let mut accounts = app.accounts.lock().unwrap();
    let next = &app.next_account;
    let account = accounts.entry(addr).or_insert_with(|| Account {
        number: next.fetch_add(1, Ordering::SeqCst),
        sequence: 0,
    });
    Json(serde_json::json!({
        "account": {
            "account_number": account.number.to_string(),
            "sequence": account.sequence.to_string(),
        }
    }))
}

async fn stats(State(app): State<Arc<App>>) -> Json<serde_json::Value> {
    let stats = app.stats.lock().unwrap();
    let quorum_needed = (app.signers * 2).div_ceil(3);
    let in_quorum = stats
        .seen_in
        .values()
        .filter(|s| s.len() >= quorum_needed)
        .count();
    let out_quorum = stats
        .seen_out
        .values()
        .filter(|s| s.len() >= quorum_needed)
        .count();
    let avg_completion = if stats.batches_completed > 0 {
        stats.completion_secs_total / stats.batches_completed
    } else {
        0
    };
    Json(serde_json::json!({
        "height": app.height.load(Ordering::SeqCst),
        "batches_created": stats.batches_created,
        "batches_completed": stats.batches_completed,
        "batches_pending": app.pending.lock().unwrap().len(),
        "batches_stalled_reports": stats.batches_stalled,
        "completion_secs_avg": avg_completion,
        "completion_secs_max": stats.completion_secs_max,
        "posts_accepted": stats.posts_accepted,
        "posts_bad_sig": stats.posts_bad_sig,
        "posts_bad_seq": stats.posts_bad_seq,
        "posts_undecodable": stats.posts_undecodable,
        "obs_in_txs": stats.obs_in_txs,
        "obs_out_txs": stats.obs_out_txs,
        "distinct_in_txids": stats.seen_in.len(),
        "distinct_out_txids": stats.seen_out.len(),
        "in_txids_with_quorum": in_quorum,
        "out_txids_with_quorum": out_quorum,
        "quorum_needed": quorum_needed,
        "per_signer_posts": stats.per_signer_posts.clone(),
        "deposits_made": stats.deposits_made,
        "blocks_mined": stats.blocks_mined,
        "reorgs_forced": stats.reorgs_forced,
    }))
}

// ---------------------------------------------------------------------------
// CometBFT-style RPC: broadcast_tx_sync with full signature verification
// ---------------------------------------------------------------------------

async fn rpc_broadcast(
    State(app): State<Arc<App>>,
    Json(req): Json<serde_json::Value>,
) -> Json<serde_json::Value> {
    let id = req["id"].clone();
    let method = req["method"].as_str().unwrap_or_default();
    if method != "broadcast_tx_sync" {
        return Json(serde_json::json!({
            "jsonrpc": "2.0", "id": id,
            "error": {"code": -32601, "message": "method not found"}
        }));
    }
    let tx_b64 = req["params"]["tx"].as_str().unwrap_or_default();
    let (code, log) = match handle_tx(&app, tx_b64) {
        Ok(()) => (0, String::new()),
        Err((code, log)) => (code, log),
    };
    let hash = {
        use sha2::{Digest, Sha256};
        hex::encode(Sha256::digest(tx_b64.as_bytes())).to_uppercase()
    };
    Json(serde_json::json!({
        "jsonrpc": "2.0", "id": id,
        "result": {"code": code, "data": "", "log": log, "hash": hash}
    }))
}

fn handle_tx(app: &Arc<App>, tx_b64: &str) -> Result<(), (i64, String)> {
    use base64::Engine as _;
    let undecodable = |what: &str| {
        app.stats.lock().unwrap().posts_undecodable += 1;
        (2i64, format!("undecodable {what}"))
    };
    let raw = base64::engine::general_purpose::STANDARD
        .decode(tx_b64)
        .map_err(|_| undecodable("base64"))?;
    let tx_raw = cosmos_tx::TxRaw::decode(raw.as_slice()).map_err(|_| undecodable("TxRaw"))?;
    let body =
        cosmos_tx::TxBody::decode(tx_raw.body_bytes.as_slice()).map_err(|_| undecodable("TxBody"))?;
    let auth = cosmos_tx::AuthInfo::decode(tx_raw.auth_info_bytes.as_slice())
        .map_err(|_| undecodable("AuthInfo"))?;

    let signer_info = auth
        .signer_infos
        .first()
        .ok_or_else(|| undecodable("signer_infos"))?;
    let pk_any = signer_info
        .public_key
        .as_ref()
        .ok_or_else(|| undecodable("public_key"))?;
    let pk = cosmos_tx::Secp256k1PubKey::decode(pk_any.value.as_slice())
        .map_err(|_| undecodable("pubkey"))?;
    let pk_hex = hex::encode(&pk.key);
    let sequence = signer_info.sequence;

    // The daemon uses hex(pubkey) as its cosmos_signer_addr, so the account
    // registered at the auth endpoint is keyed by the same string.
    let account_number = {
        let accounts = app.accounts.lock().unwrap();
        let account = accounts.get(&pk_hex).ok_or((
            5i64,
            format!("unknown account {pk_hex} (auth endpoint never queried)"),
        ))?;
        if account.sequence != sequence {
            app.stats.lock().unwrap().posts_bad_seq += 1;
            return Err((
                32,
                format!("bad sequence: got {sequence}, want {}", account.sequence),
            ));
        }
        account.number
    };

    // Verify the SIGN_MODE_DIRECT signature.
    let sign_doc = cosmos_tx::SignDoc {
        body_bytes: tx_raw.body_bytes.clone(),
        auth_info_bytes: tx_raw.auth_info_bytes.clone(),
        chain_id: app.chain_id.clone(),
        account_number,
    };
    let sign_bytes = sign_doc.encode_to_vec();
    let sig_bytes = tx_raw
        .signatures
        .first()
        .ok_or_else(|| undecodable("signatures"))?;
    {
        use sha2::{Digest, Sha256};
        let secp = bitcoin::secp256k1::Secp256k1::verification_only();
        let digest: [u8; 32] = Sha256::digest(&sign_bytes).into();
        let msg = bitcoin::secp256k1::Message::from_digest(digest);
        let pubkey = bitcoin::secp256k1::PublicKey::from_slice(&pk.key)
            .map_err(|_| undecodable("pubkey bytes"))?;
        let sig = bitcoin::secp256k1::ecdsa::Signature::from_compact(sig_bytes)
            .map_err(|_| undecodable("signature bytes"))?;
        if secp.verify_ecdsa(&msg, &sig, &pubkey).is_err() {
            app.stats.lock().unwrap().posts_bad_sig += 1;
            return Err((4, "signature verification failed".into()));
        }
    }

    // Accept: bump sequence, record the observation contents.
    {
        let mut accounts = app.accounts.lock().unwrap();
        if let Some(a) = accounts.get_mut(&pk_hex) {
            a.sequence += 1;
        }
    }
    let mut stats = app.stats.lock().unwrap();
    stats.posts_accepted += 1;
    *stats.per_signer_posts.entry(pk_hex.clone()).or_insert(0) += 1;
    for any in &body.messages {
        let is_in = any.type_url == cosmos_tx::MSG_OBSERVED_TX_IN_TYPE_URL;
        let is_out = any.type_url == cosmos_tx::MSG_OBSERVED_TX_OUT_TYPE_URL;
        if !is_in && !is_out {
            continue;
        }
        let Ok(msg) = cosmos_tx::MsgObservedTxIn::decode(any.value.as_slice()) else {
            stats.posts_undecodable += 1;
            continue;
        };
        for obs in &msg.txs {
            let txid = obs.tx.as_ref().map(|t| t.id.clone()).unwrap_or_default();
            let book = if is_in {
                stats.obs_in_txs += 1;
                &mut stats.seen_in
            } else {
                stats.obs_out_txs += 1;
                &mut stats.seen_out
            };
            if book.len() < 100_000 {
                book.entry(txid).or_default().insert(pk_hex.clone());
            }
        }
    }
    Ok(())
}
