//! `bifrost-signer` daemon: pure-Rust FROST signer bifrost.
//!
//! `run` polls the thornado chain for keysign work, coordinates a FROST
//! signing party over libp2p, builds and signs the BTC transaction, broadcasts
//! it, and posts observations (inbound and outbound) back to thornado.
//! `keygen` runs a distributed FROST DKG across the configured peers and
//! writes this node's keyshare.

use clap::{Parser, Subcommand};
use thornado_bifrost_signer::daemon::BlockSource as _;
use thornado_bifrost_signer::{
    bitcoind, broadcast, chain, daemon, frost_session, p2p, sign_loop, store, temporal, transport,
    tx_builder, wire,
};

#[derive(Parser, Debug)]
#[command(name = "bifrost-signer", about = "Pure-Rust FROST signer bifrost")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand, Debug)]
enum Command {
    /// Run the bifrost daemon (observe + sign loops).
    Run(Box<RunArgs>),
    /// Run a distributed FROST keygen (DKG) and write this node's keyshare.
    Keygen(KeygenArgs),
    /// Print this node's libp2p peer id (creating the key if missing).
    Identity(IdentityArgs),
}

#[derive(Parser, Debug)]
struct IdentityArgs {
    #[arg(long, default_value = "p2p.key")]
    p2p_key: String,
}

#[derive(Parser, Debug)]
struct RunArgs {
    /// thornado REST host, e.g. localhost:1317
    #[arg(long, env = "CHAIN_API", default_value = "localhost:1317")]
    chain_host: String,
    /// thornado CometBFT RPC, e.g. localhost:26657
    #[arg(long, env = "CHAIN_RPC", default_value = "localhost:26657")]
    chain_rpc: String,
    /// libp2p listen multiaddr
    #[arg(long, default_value = "/ip4/0.0.0.0/tcp/5040")]
    p2p_listen: String,
    /// path to this node's libp2p ed25519 key (32-byte seed, hex). Created on
    /// first run so the peer id stays stable across restarts.
    #[arg(long, default_value = "p2p.key")]
    p2p_key: String,
    /// signer store path
    #[arg(long, default_value = "signer.redb")]
    store_path: String,
    /// this node's FROST keyshare (JSON StoredShare) — enables the sign loop
    #[arg(long, env = "KEYSHARE")]
    keyshare: Option<String>,
    /// this node's compressed secp256k1 pubkey (33-byte hex or thornado
    /// bech32 `tthorpub1...`) used to verify signed keysign payloads from the
    /// local thornadod
    #[arg(long, env = "NODE_PUBKEY")]
    node_pubkey: Option<String>,
    /// the vault identifier the chain's keysign endpoint and observations use
    /// (bech32 `tthorpub1...` on a live chain). Defaults to the keyshare's
    /// group key hex, which is what the mock harness expects.
    #[arg(long, env = "VAULT_ID")]
    vault_id: Option<String>,
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
    /// extra vault BTC addresses to observe as inbound targets (repeatable);
    /// the keyshare's own vault address is always observed
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
    /// blocks between signing retries
    #[arg(long, default_value_t = 12)]
    signing_period: i64,
    /// sign loop poll interval, seconds
    #[arg(long, default_value_t = 3)]
    sign_poll_secs: u64,
    /// observe loop poll interval, seconds
    #[arg(long, default_value_t = 10)]
    observe_poll_secs: u64,
}

#[derive(Parser, Debug)]
struct KeygenArgs {
    /// our FROST participant name (must appear in --participants and peers)
    #[arg(long)]
    local_name: String,
    /// all participant names, comma separated
    #[arg(long, value_delimiter = ',')]
    participants: Vec<String>,
    /// signing threshold (min signers)
    #[arg(long)]
    min_signers: u16,
    /// JSON peer registry (other participants' peer ids + multiaddrs)
    #[arg(long)]
    peers: String,
    /// libp2p listen multiaddr
    #[arg(long, default_value = "/ip4/0.0.0.0/tcp/5040")]
    p2p_listen: String,
    /// path to this node's libp2p ed25519 key (created if missing)
    #[arg(long, default_value = "p2p.key")]
    p2p_key: String,
    /// where to write this node's keyshare JSON
    #[arg(long, default_value = "keyshare.json")]
    out: String,
    /// seconds to wait for peer connections before starting DKG
    #[arg(long, default_value_t = 5)]
    settle_secs: u64,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()),
        )
        .init();

    match Cli::parse().command {
        Command::Run(args) => run_daemon(*args).await,
        Command::Keygen(args) => run_keygen_cmd(args).await,
        Command::Identity(args) => {
            let keypair = load_p2p_key(&args.p2p_key)?;
            println!("peer_id={}", libp2p::PeerId::from(keypair.public()));
            Ok(())
        }
    }
}

/// Load (or create) the stable libp2p identity.
fn load_p2p_key(path: &str) -> anyhow::Result<libp2p::identity::Keypair> {
    if let Ok(hex_seed) = std::fs::read_to_string(path) {
        let mut seed: [u8; 32] = hex::decode(hex_seed.trim())?
            .try_into()
            .map_err(|_| anyhow::anyhow!("p2p key must be a 32-byte hex seed"))?;
        return Ok(libp2p::identity::Keypair::ed25519_from_bytes(&mut seed)?);
    }
    let keypair = libp2p::identity::Keypair::generate_ed25519();
    let seed = keypair
        .clone()
        .try_into_ed25519()
        .map_err(|_| anyhow::anyhow!("generated key is not ed25519"))?
        .secret();
    std::fs::write(path, hex::encode(seed.as_ref()))?;
    tracing::info!(%path, "generated new p2p identity");
    Ok(keypair)
}

/// Shared libp2p bring-up: swarm, dials, frost + join-party accept loops.
struct P2pStack {
    registry: p2p::PeerRegistry,
    control: libp2p_stream::Control,
    mailbox: transport::Libp2pMailbox,
    joins_rx: tokio::sync::mpsc::Receiver<sign_loop::JoinRequest>,
}

fn start_p2p(
    keypair: libp2p::identity::Keypair,
    listen: &str,
    peers_path: Option<&str>,
) -> anyhow::Result<P2pStack> {
    let registry = match peers_path {
        Some(path) => p2p::PeerRegistry::load(path).map_err(|e| anyhow::anyhow!(e))?,
        None => {
            tracing::warn!("--peers not set; no FROST peers configured (single-node mode)");
            p2p::PeerRegistry::default()
        }
    };
    tracing::info!(peers = registry.len(), "loaded peer registry");

    let (mut swarm, mut control) = p2p::build_swarm(keypair).map_err(|e| anyhow::anyhow!(e))?;
    swarm
        .listen_on(listen.parse()?)
        .map_err(|e| anyhow::anyhow!(e.to_string()))?;
    tracing::info!(peer_id = %swarm.local_peer_id(), "libp2p host ready");

    for (peer, err) in p2p::register_and_dial(&mut swarm, &registry) {
        tracing::warn!(%peer, error = %err, "initial dial failed (will retry on demand)");
    }

    // Accept inbound FROST streams; translate PeerId → participant name via
    // the registry and forward decoded frames to the mailbox channel. Each
    // stream carries one length-prefixed WrappedMessage.
    let mut incoming = control
        .accept(p2p::frost_protocol())
        .map_err(|e| anyhow::anyhow!(e.to_string()))?;
    let (inbound_tx, inbound_rx) = tokio::sync::mpsc::channel::<(String, Vec<u8>)>(4096);
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

    // Accept join-party streams: read the member's request, hand the open
    // stream to the sign loop so the leader can answer on it.
    let mut joins_incoming = control
        .accept(p2p::join_party_protocol())
        .map_err(|e| anyhow::anyhow!(e.to_string()))?;
    let (joins_tx, joins_rx) = tokio::sync::mpsc::channel::<sign_loop::JoinRequest>(256);
    let joins_registry = registry.clone();
    tokio::spawn(async move {
        use futures::StreamExt;
        while let Some((peer, mut stream)) = joins_incoming.next().await {
            let tx = joins_tx.clone();
            let from = joins_registry
                .name(&peer)
                .unwrap_or_else(|| peer.to_string());
            tokio::spawn(async move {
                match wire::read_frame(&mut stream).await {
                    Ok(payload) => match wire::JoinPartyLeaderComm::decode(&payload) {
                        Ok(msg) => {
                            let _ = tx
                                .send(sign_loop::JoinRequest { from, msg, stream })
                                .await;
                        }
                        Err(e) => tracing::debug!(error = %e, "bad join-party request"),
                    },
                    Err(e) => tracing::debug!(error = %e, "join-party stream read failed"),
                }
            });
        }
    });

    let mailbox_control = control.clone();
    let mailbox =
        transport::Libp2pMailbox::new(mailbox_control, registry.name_to_peer(), inbound_rx);

    // Drive the swarm so listeners, dials, and streams make progress, and
    // keep the mesh connected: every few seconds re-dial any registry peer
    // we lost (startup races, restarts, dropped connections).
    let dial_targets = registry.dial_targets();
    tokio::spawn(async move {
        use futures::StreamExt;
        use libp2p::swarm::dial_opts::{DialOpts, PeerCondition};
        let mut redial = tokio::time::interval(std::time::Duration::from_secs(10));
        loop {
            tokio::select! {
                event = swarm.next() => {
                    match event {
                        Some(event) => tracing::trace!(?event, "swarm event"),
                        None => break,
                    }
                }
                _ = redial.tick() => {
                    for (peer, addr) in &dial_targets {
                        if swarm.is_connected(peer) {
                            continue;
                        }
                        let opts = DialOpts::peer_id(*peer)
                            .addresses(vec![addr.clone()])
                            .condition(PeerCondition::Disconnected)
                            .build();
                        if let Err(e) = swarm.dial(opts) {
                            tracing::debug!(%peer, error = %e, "redial failed");
                        }
                    }
                }
            }
        }
    });

    Ok(P2pStack {
        registry,
        control,
        mailbox,
        joins_rx,
    })
}

fn parse_network(name: &str) -> bitcoin::Network {
    match name {
        "bitcoin" | "mainnet" => bitcoin::Network::Bitcoin,
        "testnet" => bitcoin::Network::Testnet,
        "signet" => bitcoin::Network::Signet,
        _ => bitcoin::Network::Regtest,
    }
}

async fn run_daemon(args: RunArgs) -> anyhow::Result<()> {
    tracing::info!(host = %args.chain_host, "starting bifrost-signer");

    let cfg = chain::ChainConfig {
        chain_host: args.chain_host.clone(),
        chain_rpc: args.chain_rpc.clone(),
    };
    let client = chain::ThornadoClient::new(cfg.clone());
    let store = store::SignerStore::open(&args.store_path)?;

    let verifier: Box<dyn chain::KeysignVerifier> = match &args.node_pubkey {
        Some(pk) => {
            let key = if pk.contains("pub1") {
                chain::decode_bech32_pubkey(pk)?
            } else {
                hex::decode(pk)?
            };
            Box::new(chain::Secp256k1Verifier::new(&key)?)
        }
        None => {
            tracing::warn!("--node-pubkey not set; keysign payload signatures are NOT verified");
            Box::new(chain::InsecureAcceptAll)
        }
    };

    let keypair = load_p2p_key(&args.p2p_key)?;
    let p2p_stack = start_p2p(keypair, &args.p2p_listen, args.peers.as_deref())?;

    // Optional keyshare: with it the sign loop runs; without it observe-only.
    let share: Option<frost_session::StoredShare> = match &args.keyshare {
        Some(path) => {
            let bytes = std::fs::read(path)?;
            Some(serde_json::from_slice(&bytes)?)
        }
        None => {
            tracing::warn!("--keyshare not set; sign loop disabled (observe-only)");
            None
        }
    };

    let network = parse_network(&args.btc_network);
    let btc_cfg = bitcoind::BitcoindConfig {
        host: args.btc_rpc_host.clone(),
        user: args.btc_rpc_user.clone(),
        password: args.btc_rpc_pass.clone(),
        wallet: args.btc_wallet.clone(),
    };

    // Vault facts: address derived from the share's group key (path 0). The
    // chain-facing identifier may differ (bech32 on a live chain).
    let mut vault_addresses: Vec<String> = args.vault_addresses.clone();
    let mut vault_id = String::new();
    if let Some(share) = &share {
        vault_id = args
            .vault_id
            .clone()
            .unwrap_or_else(|| share.public_key_compressed.clone());
        let pk = hex::decode(&share.public_key_compressed)?;
        let vault = tx_builder::TaprootVault::derive(&pk, 0).map_err(|e| anyhow::anyhow!(e))?;
        let addr = bitcoin::Address::from_script(vault.script_pubkey().as_script(), network)?;
        tracing::info!(vault = %addr, vault_id = %vault_id, "loaded FROST keyshare");
        vault_addresses.push(addr.to_string());
    }

    // Observe loop: scan bitcoind, split inbound/outbound, post observations.
    let temporal_store = temporal::TemporalStore::open(&args.temporal_path)?;
    let vault_view = daemon::VaultView {
        vault_addresses: vault_addresses.iter().cloned().collect(),
        protocol_addresses: vault_addresses.iter().cloned().collect(),
        observed_vault_pubkey: vault_id.clone(),
    };
    let source = daemon::BitcoindBlockSource::new(bitcoind::BitcoindRpc::new(btc_cfg.clone()));
    let poster = build_observation_poster(&args);
    if poster.is_none() {
        tracing::warn!("--cosmos-priv-key not set; observations will be logged, not posted");
    }
    let observe_chain = "BTC".to_string();
    let vault_set: std::collections::HashSet<String> = vault_addresses.iter().cloned().collect();
    let observe_secs = args.observe_poll_secs;
    let dust = args.dust_sats;
    tokio::spawn(async move {
        let start = source.block_count().await.unwrap_or(0);
        let mut observer = daemon::Observer::new(source, network, dust, start);
        let mut ticker = tokio::time::interval(std::time::Duration::from_secs(observe_secs));
        loop {
            ticker.tick().await;
            match observer.scan_to_tip(&temporal_store, &vault_view).await {
                Ok(obs) if !obs.is_empty() => {
                    tracing::info!(count = obs.len(), height = observer.last_scanned(), "observed txs");
                    let Some(ref p) = poster else { continue };
                    // Outbound = the vault is the sender; inbound = the vault
                    // is the receiver (Go GetInboundOutbound split).
                    let (outbound, inbound): (Vec<_>, Vec<_>) =
                        obs.iter().partition(|it| vault_set.contains(&it.sender));
                    for (kind, items) in [
                        (broadcast::ObservationKind::In, inbound),
                        (broadcast::ObservationKind::Out, outbound),
                    ] {
                        if items.is_empty() {
                            continue;
                        }
                        let batch = to_broadcast_txin(&observe_chain, &items);
                        match p.broadcast_observation(kind, &batch).await {
                            Ok(hash) => {
                                tracing::info!(%hash, ?kind, count = batch.tx_array.len(), "posted observation")
                            }
                            Err(e) => {
                                tracing::warn!(error = %e, ?kind, "failed to post observation")
                            }
                        }
                    }
                }
                Ok(_) => {}
                Err(e) => tracing::warn!(error = %e, "observe scan failed"),
            }
        }
    });

    // Sign loop (needs a keyshare).
    match share {
        Some(share) => {
            let sl_cfg = sign_loop::SignLoopCfg {
                local: share.participant.clone(),
                vault_id,
                network,
                signing_period: args.signing_period,
                ..Default::default()
            };
            let sl = sign_loop::SignLoop::new(
                sl_cfg,
                client,
                verifier,
                store,
                bitcoind::BitcoindRpc::new(btc_cfg),
                share,
                p2p_stack.registry,
                p2p_stack.mailbox,
                p2p_stack.control,
                p2p_stack.joins_rx,
            );
            tracing::info!("sign loop starting");
            sl.run(std::time::Duration::from_secs(args.sign_poll_secs))
                .await;
        }
        None => {
            // Observe-only: park forever.
            futures::future::pending::<()>().await;
        }
    }
    Ok(())
}

async fn run_keygen_cmd(args: KeygenArgs) -> anyhow::Result<()> {
    let keypair = load_p2p_key(&args.p2p_key)?;
    let mut p2p_stack = start_p2p(keypair, &args.p2p_listen, Some(&args.peers))?;

    tracing::info!(secs = args.settle_secs, "waiting for peer connections to settle");
    tokio::time::sleep(std::time::Duration::from_secs(args.settle_secs)).await;

    let participants = frost_session::normalize_participants(&args.participants);
    let session_id = frost_session::keygen_session_id(0, args.min_signers, &participants);
    let session = frost_session::KeygenSession::new(
        args.local_name.clone(),
        participants.clone(),
        args.min_signers,
    )
    .map_err(|e| anyhow::anyhow!(e))?;

    tracing::info!(local = %args.local_name, n = participants.len(), min = args.min_signers, "starting DKG");
    let share = tokio::time::timeout(
        std::time::Duration::from_secs(120),
        transport::run_keygen(&mut p2p_stack.mailbox, session, &session_id),
    )
    .await
    .map_err(|_| anyhow::anyhow!("DKG timed out after 120s"))?
    .map_err(|e| anyhow::anyhow!(e))?;

    std::fs::write(&args.out, serde_json::to_vec_pretty(&share)?)?;
    let pk = hex::decode(&share.public_key_compressed)?;
    let vault = tx_builder::TaprootVault::derive(&pk, 0).map_err(|e| anyhow::anyhow!(e))?;
    let regtest_addr =
        bitcoin::Address::from_script(vault.script_pubkey().as_script(), bitcoin::Network::Regtest)?;
    tracing::info!(
        out = %args.out,
        group_key = %share.public_key_compressed,
        vault_regtest = %regtest_addr,
        "DKG complete; keyshare written"
    );
    println!("group_key={}", share.public_key_compressed);
    println!("vault_regtest={regtest_addr}");
    Ok(())
}

/// Build the observation-posting client if the node's cosmos key is configured.
fn build_observation_poster(args: &RunArgs) -> Option<broadcast::ThornadoObservationClient> {
    let priv_hex = args.cosmos_priv_key.as_ref()?;
    let priv_key = hex::decode(priv_hex).ok()?;
    let account_bytes = args
        .cosmos_account
        .as_ref()
        .and_then(|a| {
            if a.contains('1') && !a.chars().all(|c| c.is_ascii_hexdigit()) {
                chain::decode_bech32_account(a).ok()
            } else {
                hex::decode(a).ok()
            }
        })
        .unwrap_or_default();
    // derive the compressed pubkey from the secret
    let secp = bitcoin::secp256k1::Secp256k1::signing_only();
    let sk = bitcoin::secp256k1::SecretKey::from_slice(&priv_key).ok()?;
    let pub_key = sk.public_key(&secp).serialize().to_vec();

    let cfg = chain::ChainConfig {
        chain_host: args.chain_host.clone(),
        chain_rpc: args.chain_rpc.clone(),
    };
    // Default the auth-lookup identity to hex(pubkey): mock-thornado keys
    // accounts by it, and a real chain deployment always sets the bech32.
    let signer_addr = if args.cosmos_signer_addr.is_empty() {
        hex::encode(&pub_key)
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
    items: &[&thornado_bifrost_signer::extract::TxInItem],
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
