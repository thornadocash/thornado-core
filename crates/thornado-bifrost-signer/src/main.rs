//! `bifrost-signer` daemon: pure-Rust FROST signer bifrost.
//!
//! Polls the thornado chain for keysign work, coordinates a FROST signing
//! party over libp2p, builds and signs the BTC transaction, and broadcasts it.

use clap::Parser;
use thornado_bifrost_signer::daemon::BlockSource as _;
use thornado_bifrost_signer::{bitcoind, chain, daemon, p2p, store, temporal, transport, wire};

#[derive(Parser, Debug)]
#[command(name = "bifrost-signer", about = "Pure-Rust FROST signer bifrost")]
struct Args {
    /// thornado REST host, e.g. localhost:1317
    #[arg(long, env = "CHAIN_API", default_value = "localhost:1317")]
    chain_host: String,
    /// thornado CometBFT RPC, e.g. localhost:26657
    #[arg(long, env = "CHAIN_RPC", default_value = "localhost:26657")]
    chain_rpc: String,
    /// libp2p listen multiaddr
    #[arg(long, default_value = "/ip4/0.0.0.0/tcp/5040")]
    p2p_listen: String,
    /// signer store path
    #[arg(long, default_value = "signer.redb")]
    store_path: String,
    /// this node's vault pubkey(s) to poll keysign for (repeatable)
    #[arg(long = "vault")]
    vaults: Vec<String>,
    /// this node's compressed secp256k1 pubkey (33 bytes, hex) used to verify
    /// signed keysign payloads from the local thornadod
    #[arg(long, env = "NODE_PUBKEY")]
    node_pubkey: Option<String>,
    /// JSON file mapping FROST participant names → libp2p peer id + multiaddr
    /// (see p2p::PeerEntry). Peers are dialed at startup.
    #[arg(long)]
    peers: Option<String>,
    /// bitcoind JSON-RPC host, e.g. 127.0.0.1:18443
    #[arg(long, env = "BTC_RPC_HOST", default_value = "127.0.0.1:18443")]
    btc_rpc_host: String,
    #[arg(long, env = "BTC_RPC_USER", default_value = "thornado")]
    btc_rpc_user: String,
    #[arg(long, env = "BTC_RPC_PASS", default_value = "password")]
    btc_rpc_pass: String,
    /// bitcoind wallet name (for wallet-scoped RPC), if any
    #[arg(long, env = "BTC_WALLET")]
    btc_wallet: Option<String>,
    /// temporal store path (block-meta / spent-UTXO tracking)
    #[arg(long, default_value = "temporal.redb")]
    temporal_path: String,
    /// BTC network: bitcoin | testnet | signet | regtest
    #[arg(long, env = "BTC_NETWORK", default_value = "regtest")]
    btc_network: String,
    /// dust threshold in sats (outputs below this are ignored)
    #[arg(long, default_value_t = 10_000)]
    dust_sats: u64,
    /// vault BTC addresses to observe as inbound targets (repeatable)
    #[arg(long = "vault-address")]
    vault_addresses: Vec<String>,
    /// node's cosmos secp256k1 secret key (32-byte hex) for posting
    /// observations to thornado. If unset, observations are logged only.
    #[arg(long, env = "COSMOS_PRIV_KEY")]
    cosmos_priv_key: Option<String>,
    /// node's cosmos account address (20-byte hex) — the tx signer identity
    #[arg(long, env = "COSMOS_ACCOUNT")]
    cosmos_account: Option<String>,
    /// bech32 signer address (thor1...) for the auth-account lookup
    #[arg(long, env = "COSMOS_SIGNER_ADDR", default_value = "")]
    cosmos_signer_addr: String,
    /// cosmos chain id, e.g. thornado-1
    #[arg(long, env = "CHAIN_ID", default_value = "thornado")]
    chain_id: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()),
        )
        .init();

    let args = Args::parse();
    tracing::info!(host = %args.chain_host, "starting bifrost-signer");

    let cfg = chain::ChainConfig {
        chain_host: args.chain_host.clone(),
        chain_rpc: args.chain_rpc.clone(),
    };
    let client = chain::ThornadoClient::new(cfg);
    let store = store::SignerStore::open(&args.store_path)?;

    let verifier: Box<dyn chain::KeysignVerifier> = match &args.node_pubkey {
        Some(pk) => Box::new(chain::Secp256k1Verifier::new(&hex::decode(pk)?)?),
        None => {
            tracing::warn!("--node-pubkey not set; keysign payload signatures are NOT verified");
            Box::new(chain::InsecureAcceptAll)
        }
    };

    // Peer registry: name → PeerId + multiaddr, for session routing.
    let registry = match &args.peers {
        Some(path) => p2p::PeerRegistry::load(path).map_err(|e| anyhow::anyhow!(e))?,
        None => {
            tracing::warn!("--peers not set; no FROST peers configured (single-node mode)");
            p2p::PeerRegistry::default()
        }
    };
    tracing::info!(peers = registry.len(), "loaded peer registry");

    // libp2p host for FROST sessions and party coordination.
    let keypair = libp2p::identity::Keypair::generate_ed25519();
    let (mut swarm, mut control) = p2p::build_swarm(keypair).map_err(|e| anyhow::anyhow!(e))?;
    swarm
        .listen_on(args.p2p_listen.parse()?)
        .map_err(|e| anyhow::anyhow!(e.to_string()))?;
    tracing::info!(peer_id = %swarm.local_peer_id(), "libp2p host ready");

    // Register peer addresses and dial them.
    for (peer, err) in p2p::register_and_dial(&mut swarm, &registry) {
        tracing::warn!(%peer, error = %err, "initial dial failed (will retry on demand)");
    }

    // Accept inbound FROST streams; translate PeerId → participant name via the
    // registry and forward decoded frames to the mailbox channel. Each stream
    // carries one length-prefixed WrappedMessage.
    let mut incoming = control
        .accept(p2p::frost_protocol())
        .map_err(|e| anyhow::anyhow!(e.to_string()))?;
    let (inbound_tx, inbound_rx) = tokio::sync::mpsc::channel::<(String, Vec<u8>)>(1024);
    let accept_registry = registry.clone();
    tokio::spawn(async move {
        use futures::StreamExt;
        while let Some((peer, mut stream)) = incoming.next().await {
            let tx = inbound_tx.clone();
            let from = accept_registry
                .name(&peer)
                .unwrap_or_else(|| peer.to_string());
            tokio::spawn(async move {
                match wire::read_frame(&mut stream).await {
                    Ok(payload) => {
                        let framed = wire::frame(&payload);
                        let _ = tx.send((from, framed)).await;
                    }
                    Err(e) => tracing::debug!(error = %e, "frost stream read failed"),
                }
            });
        }
    });

    // Mailbox the signing pipeline uses to drive FROST sessions over libp2p.
    let _mailbox = transport::Libp2pMailbox::new(control, registry.name_to_peer(), inbound_rx);
    tracing::info!("FROST transport ready");

    // Drive the swarm so listeners, dials, and streams make progress.
    tokio::spawn(async move {
        use futures::StreamExt;
        while let Some(event) = swarm.next().await {
            tracing::trace!(?event, "swarm event");
        }
    });

    // Observe loop: scan bitcoind for inbound observations to our vaults.
    let network = match args.btc_network.as_str() {
        "bitcoin" | "mainnet" => bitcoin::Network::Bitcoin,
        "testnet" => bitcoin::Network::Testnet,
        "signet" => bitcoin::Network::Signet,
        _ => bitcoin::Network::Regtest,
    };
    let btc_rpc = bitcoind::BitcoindRpc::new(bitcoind::BitcoindConfig {
        host: args.btc_rpc_host.clone(),
        user: args.btc_rpc_user.clone(),
        password: args.btc_rpc_pass.clone(),
        wallet: args.btc_wallet.clone(),
    });
    let temporal_store = temporal::TemporalStore::open(&args.temporal_path)?;
    let vault_view = daemon::VaultView {
        vault_addresses: args.vault_addresses.iter().cloned().collect(),
        protocol_addresses: args.vault_addresses.iter().cloned().collect(),
        observed_vault_pubkey: args.vaults.first().cloned().unwrap_or_default(),
    };
    let source = daemon::BitcoindBlockSource::new(btc_rpc);

    // Optional observation-posting client (needs the node's cosmos key).
    let poster = build_observation_poster(&args);
    if poster.is_none() {
        tracing::warn!("--cosmos-priv-key not set; observations will be logged, not posted");
    }
    let observe_chain = args.vaults.first().cloned().unwrap_or_default();
    tokio::spawn(async move {
        let start = source.block_count().await.unwrap_or(0);
        let mut observer = daemon::Observer::new(source, network, args.dust_sats, start);
        let mut ticker = tokio::time::interval(std::time::Duration::from_secs(10));
        loop {
            ticker.tick().await;
            match observer.scan_to_tip(&temporal_store, &vault_view).await {
                Ok(obs) if !obs.is_empty() => {
                    tracing::info!(count = obs.len(), height = observer.last_scanned(), "observed inbound txs");
                    if let Some(ref p) = poster {
                        let batch = to_broadcast_txin(&observe_chain, &obs);
                        match p
                            .broadcast_observation(broadcast::ObservationKind::In, &batch)
                            .await
                        {
                            Ok(hash) => tracing::info!(%hash, "posted observation to thornado"),
                            Err(e) => tracing::warn!(error = %e, "failed to post observation"),
                        }
                    }
                }
                Ok(_) => {}
                Err(e) => tracing::warn!(error = %e, "observe scan failed"),
            }
        }
    });

    // Main poll loop: fetch height, then keysign work for each vault.
    let mut ticker = tokio::time::interval(std::time::Duration::from_secs(6));
    loop {
        ticker.tick().await;
        let height = match client.get_block_height().await {
            Ok(h) => h,
            Err(e) => {
                tracing::warn!(error = %e, "failed to fetch block height");
                continue;
            }
        };
        for vault in &args.vaults {
            match client.get_keysign(height, vault, verifier.as_ref()).await {
                Ok(txout) => {
                    for (i, item) in txout.tx_array.iter().enumerate() {
                        let stored = store::TxOutStoreItem::new(
                            item.clone(),
                            txout.height,
                            i as i64,
                            txout.epoch,
                        );
                        if store.get(&stored.key())?.is_none() {
                            store.put(&stored)?;
                            tracing::info!(in_hash = %item.in_hash, "queued txout for signing");
                        }
                    }
                }
                Err(chain::ChainError::UnavailableBlock) => {}
                Err(e) => tracing::warn!(vault = %vault, error = %e, "keysign fetch failed"),
            }
        }
        // NOTE: the signing pipeline (batch -> party join -> FROST -> broadcast)
        // is driven from the store here; see signer.rs for the decision logic.
    }
}

use thornado_bifrost_signer::broadcast;

/// Build the observation-posting client if the node's cosmos key is configured.
fn build_observation_poster(args: &Args) -> Option<broadcast::ThornadoObservationClient> {
    let priv_hex = args.cosmos_priv_key.as_ref()?;
    let priv_key = hex::decode(priv_hex).ok()?;
    let account_bytes = args
        .cosmos_account
        .as_ref()
        .and_then(|h| hex::decode(h).ok())
        .unwrap_or_default();
    // derive the compressed pubkey from the secret
    let secp = bitcoin::secp256k1::Secp256k1::signing_only();
    let sk = bitcoin::secp256k1::SecretKey::from_slice(&priv_key).ok()?;
    let pub_key = sk.public_key(&secp).serialize().to_vec();

    let cfg = chain::ChainConfig {
        chain_host: args.chain_host.clone(),
        chain_rpc: args.chain_rpc.clone(),
    };
    let signer_addr = if args.cosmos_signer_addr.is_empty() {
        args.vaults.first().cloned().unwrap_or_default()
    } else {
        args.cosmos_signer_addr.clone()
    };
    Some(
        broadcast::ThornadoObservationClient::new(cfg, signer_addr).with_key(broadcast::SignerKey {
            priv_key,
            pub_key,
            account_bytes,
            chain_id: args.chain_id.clone(),
        }),
    )
}

/// Convert extracted observations into the broadcast `TxIn` batch shape.
fn to_broadcast_txin(
    chain: &str,
    items: &[thornado_bifrost_signer::extract::TxInItem],
) -> broadcast::TxIn {
    let tx_array = items
        .iter()
        .map(|it| broadcast::TxInItem {
            block_height: it.block_height,
            tx: it.tx.clone(),
            source_vout: it.source_vout,
            source_inputs: vec![],
            sender: it.sender.clone(),
            to: it.to.clone(),
            coins: it
                .coins
                .iter()
                .map(|c| chain::Coin {
                    asset: c.asset.clone(),
                    amount: c.amount_sats.to_string(),
                })
                .collect(),
            gas: it
                .gas
                .iter()
                .map(|c| chain::Coin {
                    asset: c.asset.clone(),
                    amount: c.amount_sats.to_string(),
                })
                .collect(),
            observed_vault_pub_key: it.observed_vault_pubkey.clone(),
            aggregator: String::new(),
            aggregator_target: String::new(),
            aggregator_target_limit: None,
            committed_un_finalised: false,
        })
        .collect();
    broadcast::TxIn {
        chain: chain.to_string(),
        tx_array,
        filtered: true,
        mem_pool: false,
        confirmation_required: 0,
        allow_future_observation: false,
    }
}
