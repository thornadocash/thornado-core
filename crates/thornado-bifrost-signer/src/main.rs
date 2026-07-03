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
    /// this node's FROST keyshare (JSON StoredShare) — enables the sign loop.
    /// Loaded into the keyshare directory alongside any churn-produced shares.
    #[arg(long, env = "KEYSHARE")]
    keyshare: Option<String>,
    /// directory of FROST keyshares (one `*.json` StoredShare per vault). The
    /// keygen loop writes new-vault shares here on churn; the sign loop signs
    /// for every vault it holds a share for (retiring + active during a churn).
    #[arg(long, default_value = "keyshares")]
    keyshare_dir: String,
    /// Go bifrost home dir holding `localstate-<vault>.json` files. Scanned at
    /// startup: each file's base64 `local_data` (a StoredShare) is converted
    /// into `keyshare_dir`, so a chain-reactivated historical vault is always
    /// signable without manual recovery.
    #[arg(long)]
    go_keyshare_dir: Option<String>,
    /// Start the observe scan at this BTC height instead of the current tip.
    /// Re-observing old blocks replays observations the chain has already
    /// finalised (it dedups); used to re-match historical internal outbounds
    /// after a matcher fix. 0 = start at tip.
    #[arg(long, default_value_t = 0)]
    observe_rescan_height: i64,
    /// enable the churn keygen loop (poll keygen blocks, run DKG, submit
    /// MsgKeygenVault). Requires the cosmos key to submit results.
    #[arg(long, default_value_t = false)]
    keygen: bool,
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
    /// How many per-deposit child addresses (taproot paths 1..=N) of each held
    /// vault to watch for inbound deposits. Deposit N is issued at child path
    /// N+1; the Go bifrost pre-derives a 4096 lookahead. 0 disables deposit
    /// observation (root-only).
    #[arg(long, default_value_t = 512)]
    deposit_lookahead: u64,
    /// How long the party leader collects join requests before deciding
    /// (below-threshold ceiling).
    #[arg(long, default_value_t = 12)]
    party_wait_secs: u64,
    /// Straggler grace: once the signing threshold has joined, how long the
    /// leader keeps waiting for FULL membership before forming the party.
    #[arg(long, default_value_t = 3)]
    party_grace_secs: u64,
    /// How long a member waits for the leader's join-party response. Also the
    /// leader's parked-join TTL — do not set below ~20s, or demand-driven
    /// leading starves under load.
    #[arg(long, default_value_t = 20)]
    join_wait_secs: u64,
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
    /// Operational recovery: re-sign batches whose prescribed inputs are
    /// already spent, using fresh UTXOs (may double-pay). Off by default.
    #[arg(long, default_value_t = false)]
    allow_respend_spent: bool,
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
    router: transport::SessionRouter,
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

    let router =
        transport::SessionRouter::new(control.clone(), registry.name_to_peer(), inbound_rx);

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
        router,
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

    let network = parse_network(&args.btc_network);
    let hrp = if matches!(network, bitcoin::Network::Bitcoin) {
        "thorpub"
    } else {
        "tthorpub"
    };
    let btc_cfg = bitcoind::BitcoindConfig {
        host: args.btc_rpc_host.clone(),
        user: args.btc_rpc_user.clone(),
        password: args.btc_rpc_pass.clone(),
        wallet: args.btc_wallet.clone(),
    };

    // Load every keyshare we hold (dir + optional single-file) into a shared
    // map keyed by hex group key. The keygen loop adds new shares on churn.
    std::fs::create_dir_all(&args.keyshare_dir).ok();
    if let Some(go_dir) = &args.go_keyshare_dir {
        match import_go_keyshares(go_dir, &args.keyshare_dir) {
            Ok(n) if n > 0 => tracing::info!(count = n, "imported Go bifrost keyshares"),
            Ok(_) => {}
            Err(e) => tracing::warn!(error = %e, "Go keyshare import failed"),
        }
    }
    let shares: sign_loop::SharedShares = Default::default();
    {
        let mut w = shares.write().unwrap();
        let mut load = |bytes: &[u8]| -> anyhow::Result<()> {
            let s: frost_session::StoredShare = serde_json::from_slice(bytes)?;
            let addr = share_vault_addr(&s, network)?;
            tracing::info!(vault = %addr, group_key = %s.public_key_compressed, "loaded FROST keyshare");
            w.insert(s.public_key_compressed.clone(), s);
            Ok(())
        };
        if let Ok(rd) = std::fs::read_dir(&args.keyshare_dir) {
            for e in rd.flatten() {
                if e.path().extension().and_then(|x| x.to_str()) == Some("json") {
                    if let Ok(b) = std::fs::read(e.path()) {
                        let _ = load(&b);
                    }
                }
            }
        }
        if let Some(path) = &args.keyshare {
            if let Ok(b) = std::fs::read(path) {
                let _ = load(&b);
            }
        }
    }
    tracing::info!(vaults = shares.read().unwrap().len(), "keyshares loaded");

    // Our FROST participant name (this node's validator secp256k1 pubkey, as
    // bech32) — needed to check keygen membership and to sign.
    let our_name = args.node_pubkey.as_ref().map(|pk| {
        if pk.contains("pub1") {
            pk.clone()
        } else {
            hex::decode(pk)
                .ok()
                .and_then(|b| chain::encode_bech32_pubkey(hrp, &b).ok())
                .unwrap_or_else(|| pk.clone())
        }
    });

    let poster = build_observation_poster(&args);
    if poster.is_none() {
        tracing::warn!("--cosmos-priv-key not set; observations/keygen will not be posted");
    }

    // Observe loop: scan bitcoind, resolve each observation's vault from the
    // live share set (so a new churn vault is watched the moment it forms),
    // split inbound/outbound, post observations.
    let temporal_store = temporal::TemporalStore::open(&args.temporal_path)?;
    let source = daemon::BitcoindBlockSource::new(bitcoind::BitcoindRpc::new(btc_cfg.clone()));
    let observe_chain = "BTC".to_string();
    let observe_secs = args.observe_poll_secs;
    let dust = args.dust_sats;
    let obs_shares = shares.clone();
    let obs_poster = poster.clone();
    let extra_vaults = args.vault_addresses.clone();
    let obs_hrp = hrp.to_string();
    let rescan_height = args.observe_rescan_height;
    let obs_lookahead = args.deposit_lookahead;
    let solvency_client = chain::ThornadoClient::new(cfg.clone());
    let solvency_rpc = bitcoind::BitcoindRpc::new(btc_cfg.clone());
    tokio::spawn(async move {
        let tip = source.block_count().await.unwrap_or(0);
        let start = if rescan_height > 0 && rescan_height < tip {
            tracing::info!(from = rescan_height, tip, "observe rescan requested");
            rescan_height
        } else {
            tip
        };
        let mut observer = daemon::Observer::new(source, network, dust, start);
        let mut ticker = tokio::time::interval(std::time::Duration::from_secs(observe_secs));
        let mut addr_cache = VaultAddrCache::new();
        let mut last_solvency_height: i64 = 0;
        loop {
            ticker.tick().await;
            // Vault address→pubkey map (root + deposit child addresses), cached
            // and rebuilt only when the held share set changes.
            let addr_to_pubkey = addr_cache
                .get(&obs_shares, network, &obs_hrp, obs_lookahead)
                .await
                .clone();
            let mut vault_addrs: std::collections::HashSet<String> =
                addr_to_pubkey.keys().cloned().collect();
            vault_addrs.extend(extra_vaults.iter().cloned());
            let vault_view = daemon::VaultView {
                vault_addresses: vault_addrs.clone(),
                protocol_addresses: vault_addrs.clone(),
                observed_vault_pubkey: String::new(),
            };
            match observer.scan_to_tip(&temporal_store, &vault_view).await {
                Ok(mut obs) if !obs.is_empty() => {
                    tracing::info!(count = obs.len(), height = observer.last_scanned(), "observed txs");
                    let Some(ref p) = obs_poster else { continue };
                    // Resolve each observation's vault: sender-vault → outbound,
                    // receiver-vault → inbound (Go GetInboundOutbound split).
                    let mut inbound = Vec::new();
                    let mut outbound = Vec::new();
                    for it in obs.drain(..) {
                        let mut it = it;
                        if let Some(pk) = addr_to_pubkey.get(&it.sender) {
                            it.observed_vault_pubkey = pk.clone();
                            outbound.push(it);
                        } else if let Some(pk) = addr_to_pubkey.get(&it.to) {
                            it.observed_vault_pubkey = pk.clone();
                            inbound.push(it);
                        }
                    }
                    for (kind, items) in [
                        (broadcast::ObservationKind::In, inbound),
                        (broadcast::ObservationKind::Out, outbound),
                    ] {
                        if items.is_empty() {
                            continue;
                        }
                        let refs: Vec<&_> = items.iter().collect();
                        let batch = to_broadcast_txin(&observe_chain, &refs);
                        match p.broadcast_observation(kind, &batch).await {
                            Ok(hash) => {
                                tracing::info!(%hash, ?kind, count = batch.tx_array.len(), "posted observation")
                            }
                            Err(e) => tracing::warn!(error = %e, ?kind, "failed to post observation"),
                        }
                    }
                }
                Ok(_) => {}
                Err(e) => tracing::warn!(error = %e, "observe scan failed"),
            }

            // Solvency: report each base vault's wallet balance so the chain's
            // solvency voter can reach supermajority consensus across the
            // active nodes (Go BTC client ReportSolvency; each node posts its
            // own MsgSolvency vote). Reports are quantized to EVEN BTC heights:
            // the solvency id hashes the reported height, so nodes reporting at
            // self-relative strides (Go's height-last>1 gate) can disagree on
            // the id forever and never converge on one voter.
            if let Some(ref p) = obs_poster {
                let tip = observer.last_scanned();
                if tip > last_solvency_height && tip % 2 == 0 {
                    match report_solvency(
                        &solvency_client,
                        &solvency_rpc,
                        p,
                        &observe_chain,
                        &addr_to_pubkey,
                        tip,
                    )
                    .await
                    {
                        Ok(()) => last_solvency_height = tip,
                        Err(e) => tracing::warn!(error = %e, "solvency report failed"),
                    }
                }
            }
        }
    });

    // Keygen (churn) loop: poll keygen blocks, run DKG for any membership we
    // belong to, write the new share, and submit MsgKeygenVault.
    if args.keygen {
        match (&our_name, &poster) {
            (Some(name), Some(p)) => {
                let kg = std::sync::Arc::new(KeygenLoop {
                    client: chain::ThornadoClient::new(cfg.clone()),
                    poster: p.clone(),
                    shares: shares.clone(),
                    router: p2p_stack.router.clone(),
                    our_name: name.clone(),
                    node_pubkey: args.node_pubkey.clone().unwrap_or_default(),
                    keyshare_dir: args.keyshare_dir.clone(),
                    hrp: hrp.to_string(),
                    network,
                    done: Default::default(),
                    in_flight: Default::default(),
                    pending: Default::default(),
                    last_scanned: std::sync::atomic::AtomicI64::new(0),
                });
                let node_pk = args.node_pubkey.clone();
                tokio::spawn(async move {
                    let verifier = build_verifier(node_pk.as_deref());
                    tracing::info!("keygen loop starting");
                    kg.run(verifier).await;
                });
            }
            _ => tracing::warn!("--keygen set but node-pubkey/cosmos-key missing; keygen disabled"),
        }
    }

    // Sign loop (needs at least one keyshare).
    if shares.read().unwrap().is_empty() {
        tracing::warn!("no keyshares; sign loop disabled (observe-only)");
        futures::future::pending::<()>().await;
        return Ok(());
    }
    let local = our_name.unwrap_or_else(|| {
        shares
            .read()
            .unwrap()
            .values()
            .next()
            .map(|s| s.participant.clone())
            .unwrap_or_default()
    });
    let sl_cfg = sign_loop::SignLoopCfg {
        local,
        vault_id: args.vault_id.clone().unwrap_or_default(),
        network,
        signing_period: args.signing_period,
        allow_respend_spent: args.allow_respend_spent,
        party_wait: std::time::Duration::from_secs(args.party_wait_secs),
        party_grace: std::time::Duration::from_secs(args.party_grace_secs),
        join_wait: std::time::Duration::from_secs(args.join_wait_secs),
        ..Default::default()
    };
    let sl = sign_loop::SignLoop::new(
        sl_cfg,
        client,
        verifier,
        store,
        bitcoind::BitcoindRpc::new(btc_cfg),
        shares,
        p2p_stack.registry,
        p2p_stack.router,
        p2p_stack.control,
        p2p_stack.joins_rx,
    );
    tracing::info!("sign loop starting");
    sl.run(std::time::Duration::from_secs(args.sign_poll_secs))
        .await;
    Ok(())
}

/// Convert every Go bifrost `localstate-<vault>.json` in `go_dir` into a
/// StoredShare file under `keyshare_dir` (skipping shares already present).
/// Returns how many were newly imported.
fn import_go_keyshares(go_dir: &str, keyshare_dir: &str) -> anyhow::Result<usize> {
    use base64::Engine;
    #[derive(serde::Deserialize)]
    struct LocalState {
        local_data: String,
    }
    let mut imported = 0;
    for e in std::fs::read_dir(go_dir)?.flatten() {
        let name = e.file_name().to_string_lossy().to_string();
        if !name.starts_with("localstate-") || !name.ends_with(".json") {
            continue;
        }
        let convert = || -> anyhow::Result<Option<String>> {
            let ls: LocalState = serde_json::from_slice(&std::fs::read(e.path())?)?;
            let raw = base64::engine::general_purpose::STANDARD.decode(ls.local_data.trim())?;
            let share: frost_session::StoredShare = serde_json::from_slice(&raw)?;
            let key = share.public_key_compressed.clone();
            let dest = format!("{keyshare_dir}/keyshare-{}.json", &key[..16.min(key.len())]);
            if std::path::Path::new(&dest).exists() {
                return Ok(None);
            }
            std::fs::write(&dest, &raw)?;
            Ok(Some(key))
        };
        match convert() {
            Ok(Some(key)) => {
                tracing::info!(file = %name, group_key = %key, "imported Go keyshare");
                imported += 1;
            }
            Ok(None) => {}
            Err(err) => tracing::warn!(file = %name, error = %err, "skipping Go localstate"),
        }
    }
    Ok(imported)
}

/// The regtest/mainnet BTC address of a keyshare's vault (path 0).
/// Report each funded base vault's BTC wallet balance as a `MsgSolvency` vote
/// (Go BTC client `ReportSolvency`). The balance sums `listunspent` (mempool
/// included) over every watched address of the vault — root plus deposit child
/// paths — matching Go's `solvencyVaultPaths`. Vaults we hold no share for
/// (e.g. on a standby that never joined) are skipped, as is the whole report
/// when this node is not Active (the chain rejects non-active solvency votes).
async fn report_solvency(
    client: &chain::ThornadoClient,
    rpc: &bitcoind::BitcoindRpc,
    poster: &broadcast::ThornadoObservationClient,
    chain_name: &str,
    addr_to_pubkey: &std::collections::HashMap<String, String>,
    height: i64,
) -> anyhow::Result<()> {
    let nodes = client.get_node_accounts().await?;
    let me = poster.signer_address();
    let am_active = nodes.iter().any(|n| n.node_address == me && n.is_active());
    if !am_active {
        return Ok(());
    }
    let vaults = client.get_base_vaults().await?;
    for v in vaults.iter().filter(|v| v.has_funds_for_chain(chain_name)) {
        let addrs: Vec<String> = addr_to_pubkey
            .iter()
            .filter(|(_, pk)| **pk == v.pub_key)
            .map(|(a, _)| a.clone())
            .collect();
        if addrs.is_empty() {
            continue;
        }
        // scantxoutset, NOT wallet listunspent: the vault addresses are not
        // imported into the nodes' bitcoind wallets, so listunspent returns 0
        // on any node the Go bifrost never ran on — three zero votes reached
        // consensus and solvency-halted the chain. The UTXO-set scan needs no
        // wallet state and is identical on every node.
        let total_btc = rpc
            .scan_tx_out_set_total(&addrs)
            .await
            .map_err(|e| anyhow::anyhow!("scantxoutset: {e}"))?;
        let sats = thornado_bifrost_signer::extract::btc_to_sats(total_btc);
        match poster
            .submit_solvency(chain_name, &v.pub_key, sats, height)
            .await
        {
            Ok(hash) => {
                tracing::info!(%hash, vault = %v.pub_key, sats, height, "posted solvency")
            }
            Err(e) => tracing::warn!(error = %e, vault = %v.pub_key, "failed to post solvency"),
        }
    }
    Ok(())
}

fn share_vault_addr(
    s: &frost_session::StoredShare,
    network: bitcoin::Network,
) -> anyhow::Result<String> {
    let pk = hex::decode(&s.public_key_compressed)?;
    let vault = tx_builder::TaprootVault::derive(&pk, 0).map_err(|e| anyhow::anyhow!(e))?;
    Ok(bitcoin::Address::from_script(vault.script_pubkey().as_script(), network)?.to_string())
}

/// Map each held vault's BTC address → its bech32 pubkey, for classifying and
/// stamping observations. Includes the root (path 0) plus `deposit_lookahead`
/// per-deposit child addresses (paths 1..=N), so inbound deposits to a vault's
/// child addresses are observed (Go bifrost's deposit-address lookahead).
/// The address map over an owned slice of shares (so it can run off the async
/// executor via `spawn_blocking`).
fn vault_addr_map_from(
    shares: &[frost_session::StoredShare],
    network: bitcoin::Network,
    hrp: &str,
    deposit_lookahead: u64,
) -> std::collections::HashMap<String, String> {
    let mut m = std::collections::HashMap::new();
    for s in shares {
        let Ok(pk) = hex::decode(&s.public_key_compressed) else {
            continue;
        };
        let Ok(bech) = chain::encode_bech32_pubkey(hrp, &pk) else {
            continue;
        };
        for path in 0..=deposit_lookahead {
            let Ok(vault) = tx_builder::TaprootVault::derive(&pk, path) else {
                continue;
            };
            let Ok(addr) =
                bitcoin::Address::from_script(vault.script_pubkey().as_script(), network)
            else {
                continue;
            };
            m.insert(addr.to_string(), bech.clone());
        }
    }
    m
}

/// Cache of the vault→child address map, rebuilt only when the held share set
/// changes (deriving thousands of taproot addresses every tick is wasteful).
struct VaultAddrCache {
    keys_fingerprint: Vec<String>,
    map: std::collections::HashMap<String, String>,
}

impl VaultAddrCache {
    fn new() -> Self {
        Self {
            keys_fingerprint: Vec::new(),
            map: std::collections::HashMap::new(),
        }
    }

    /// Return the address map, rebuilding on a blocking thread if the share
    /// set changed. Deriving thousands of taproot child addresses is CPU-bound
    /// synchronous EC math; running it inline on the async executor stalled the
    /// FROST join-party handshakes (peers saw "not enough peers online"), so it
    /// runs via `spawn_blocking`.
    async fn get(
        &mut self,
        shares: &sign_loop::SharedShares,
        network: bitcoin::Network,
        hrp: &str,
        deposit_lookahead: u64,
    ) -> &std::collections::HashMap<String, String> {
        let mut fp: Vec<String> = shares.read().unwrap().keys().cloned().collect();
        fp.sort();
        if fp != self.keys_fingerprint {
            // Snapshot the shares so the blocking task owns its own data.
            let snapshot: Vec<frost_session::StoredShare> =
                shares.read().unwrap().values().cloned().collect();
            let hrp = hrp.to_string();
            let map = tokio::task::spawn_blocking(move || {
                vault_addr_map_from(&snapshot, network, &hrp, deposit_lookahead)
            })
            .await
            .unwrap_or_default();
            self.map = map;
            self.keys_fingerprint = fp;
            tracing::info!(vaults = self.keys_fingerprint.len(), addrs = self.map.len(), "rebuilt vault address watch set");
        }
        &self.map
    }
}

fn build_verifier(node_pubkey: Option<&str>) -> Box<dyn chain::KeysignVerifier> {
    match node_pubkey {
        Some(pk) => {
            let key = if pk.contains("pub1") {
                chain::decode_bech32_pubkey(pk).ok()
            } else {
                hex::decode(pk).ok()
            };
            match key.and_then(|k| chain::Secp256k1Verifier::new(&k).ok()) {
                Some(v) => Box::new(v),
                None => Box::new(chain::InsecureAcceptAll),
            }
        }
        None => Box::new(chain::InsecureAcceptAll),
    }
}

/// The churn keygen loop: scans thornado keygen blocks, runs the DKG for any
/// vault membership this node belongs to, writes the new keyshare, and submits
/// `MsgKeygenVault` so the chain forms the vault at consensus.
struct PendingKeygen {
    height: i64,
    kg: chain::Keygen,
    attempts: u32,
    next_at: std::time::Instant,
    first_seen: std::time::Instant,
}

/// How long a failed keygen keeps retrying before we abandon it and rely on
/// the chain's keygen-retry to reschedule at a fresh height (fresh session id).
const KEYGEN_RETRY_MAX_AGE: std::time::Duration = std::time::Duration::from_secs(15 * 60);

struct KeygenLoop {
    client: chain::ThornadoClient,
    poster: broadcast::ThornadoObservationClient,
    shares: sign_loop::SharedShares,
    router: transport::SessionRouter,
    our_name: String,
    node_pubkey: String,
    keyshare_dir: String,
    hrp: String,
    network: bitcoin::Network,
    done: std::sync::Mutex<std::collections::HashSet<String>>,
    in_flight: std::sync::Mutex<std::collections::HashSet<String>>,
    pending: std::sync::Mutex<Vec<PendingKeygen>>,
    last_scanned: std::sync::atomic::AtomicI64,
}

impl KeygenLoop {
    async fn run(self: std::sync::Arc<Self>, verifier: Box<dyn chain::KeysignVerifier>) {
        let mut ticker = tokio::time::interval(std::time::Duration::from_secs(3));
        loop {
            ticker.tick().await;
            let height = match self.client.get_block_height().await {
                Ok(h) => h,
                Err(e) => {
                    tracing::warn!(error = %e, "keygen loop: chain height unavailable");
                    continue;
                }
            };
            let last = self.last_scanned.load(std::sync::atomic::Ordering::SeqCst);
            let from = if last == 0 { (height - 1).max(1) } else { last + 1 };
            let to = height.min(from + 40);
            for h in from..=to {
                if let Err(e) = self.scan_height(h, verifier.as_ref()).await {
                    tracing::warn!(height = h, error = %e, "keygen scan failed; will re-scan");
                    break;
                }
                self.last_scanned
                    .store(h, std::sync::atomic::Ordering::SeqCst);
            }
            self.launch_due_keygens();
        }
    }

    /// Collect this height's keygens into the pending queue (never runs the
    /// DKG inline — scanning must not stall past rescheduled keygen blocks).
    async fn scan_height(
        &self,
        height: i64,
        verifier: &dyn chain::KeysignVerifier,
    ) -> anyhow::Result<()> {
        let block = match self
            .client
            .get_keygen_block(height, &self.node_pubkey, verifier)
            .await
        {
            Ok(b) => b,
            Err(chain::ChainError::UnavailableBlock) => return Ok(()),
            Err(e) => return Err(e.into()),
        };
        for kg in &block.keygens {
            if kg.keygen_type != "BaseVaultKeygen" || !kg.members.contains(&self.our_name) {
                continue;
            }
            self.enqueue(block.height, kg);
        }
        Ok(())
    }

    fn enqueue(&self, height: i64, kg: &chain::Keygen) {
        if self.done.lock().unwrap().contains(&kg.id)
            || self.in_flight.lock().unwrap().contains(&kg.id)
        {
            return;
        }
        let members_key = frost_session::normalize_participants(&kg.members).join(",");
        let mut pend = self.pending.lock().unwrap();
        if pend.iter().any(|p| p.kg.id == kg.id) {
            return;
        }
        // A rescheduled keygen (chain keygen-retry) supersedes older pending
        // attempts for the same member set: same DKG, fresh session id.
        pend.retain(|p| {
            let same = frost_session::normalize_participants(&p.kg.members).join(",") == members_key
                && p.height < height;
            if same {
                tracing::warn!(old_height = p.height, new_height = height, "keygen superseded by rescheduled block");
            }
            !same
        });
        tracing::info!(height, id = %kg.id, n = kg.members.len(), "keygen queued");
        pend.push(PendingKeygen {
            height,
            kg: kg.clone(),
            attempts: 0,
            next_at: std::time::Instant::now(),
            first_seen: std::time::Instant::now(),
        });
    }

    fn launch_due_keygens(self: &std::sync::Arc<Self>) {
        let now = std::time::Instant::now();
        let due: Vec<PendingKeygen> = {
            let mut pend = self.pending.lock().unwrap();
            pend.retain(|p| {
                if p.first_seen.elapsed() > KEYGEN_RETRY_MAX_AGE {
                    tracing::warn!(height = p.height, id = %p.kg.id, attempts = p.attempts,
                        "abandoning keygen after max retry age; waiting for chain reschedule");
                    return false;
                }
                true
            });
            let mut launched = Vec::new();
            pend.retain(|p| {
                if p.next_at <= now && !self.in_flight.lock().unwrap().contains(&p.kg.id) {
                    launched.push(PendingKeygen {
                        height: p.height,
                        kg: p.kg.clone(),
                        attempts: p.attempts,
                        next_at: p.next_at,
                        first_seen: p.first_seen,
                    });
                    return false;
                }
                true
            });
            launched
        };
        for entry in due {
            self.in_flight.lock().unwrap().insert(entry.kg.id.clone());
            let me = self.clone();
            tokio::spawn(async move {
                let attempt = entry.attempts + 1;
                let result = me.run_dkg(entry.height, &entry.kg).await;
                me.in_flight.lock().unwrap().remove(&entry.kg.id);
                match result {
                    Ok(()) => {
                        me.done.lock().unwrap().insert(entry.kg.id.clone());
                    }
                    Err(e) => {
                        tracing::warn!(height = entry.height, id = %entry.kg.id, attempt, error = %e,
                            "CHURN DKG attempt failed; retrying with backoff");
                        let backoff =
                            std::time::Duration::from_secs(15 * u64::from(attempt.min(4)));
                        me.pending.lock().unwrap().push(PendingKeygen {
                            height: entry.height,
                            kg: entry.kg,
                            attempts: attempt,
                            next_at: std::time::Instant::now() + backoff,
                            first_seen: entry.first_seen,
                        });
                    }
                }
            });
        }
    }

    async fn run_dkg(&self, height: i64, kg: &chain::Keygen) -> anyhow::Result<()> {
        let members = frost_session::normalize_participants(&kg.members);
        let min_signers =
            sign_loop_min_signers(members.len());
        let session_id = frost_session::keygen_session_id(height, min_signers, &members);
        let session = frost_session::KeygenSession::new(
            self.our_name.clone(),
            members.clone(),
            min_signers,
        )
        .map_err(|e| anyhow::anyhow!(e))?;
        tracing::info!(height, n = members.len(), min = min_signers, "CHURN starting DKG");
        let start = std::time::Instant::now();
        let mut mbox = self.router.session(&session_id);
        let share = tokio::time::timeout(
            std::time::Duration::from_secs(120),
            transport::run_keygen(&mut mbox, session, &session_id),
        )
        .await
        .map_err(|_| anyhow::anyhow!("DKG timed out"))?
        .map_err(|e| anyhow::anyhow!(e))?;
        let dkg_ms = start.elapsed().as_millis();

        let group_hex = share.public_key_compressed.clone();
        let pk = hex::decode(&group_hex)?;
        let vault_bech = chain::encode_bech32_pubkey(&self.hrp, &pk)?;
        let vault_addr = share_vault_addr(&share, self.network)?;
        tracing::info!(dkg_ms, vault = %vault_addr, vault_id = %vault_bech, "DKG_TIMING CHURN keygen complete");

        // Persist + expose the share immediately so the sign loop can migrate.
        let path = format!("{}/keyshare-{}.json", self.keyshare_dir, &group_hex[..16.min(group_hex.len())]);
        std::fs::write(&path, serde_json::to_vec_pretty(&share)?)?;
        self.shares.write().unwrap().insert(group_hex.clone(), share);

        // Submit MsgKeygenVault so the chain forms the vault at consensus.
        match self
            .poster
            .submit_keygen_vault(&members, &vault_bech, height, dkg_ms as i64, &["BTC".to_string()])
            .await
        {
            Ok(hash) => tracing::info!(%hash, vault = %vault_bech, "submitted MsgKeygenVault"),
            Err(e) => tracing::warn!(error = %e, "failed to submit MsgKeygenVault"),
        }
        Ok(())
    }
}

/// FROST threshold for `n` members: ceil(2n/3) (Go `frostMinSigners`).
fn sign_loop_min_signers(n: usize) -> u16 {
    ((n * 2).div_ceil(3)) as u16
}

async fn run_keygen_cmd(args: KeygenArgs) -> anyhow::Result<()> {
    let keypair = load_p2p_key(&args.p2p_key)?;
    let p2p_stack = start_p2p(keypair, &args.p2p_listen, Some(&args.peers))?;

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
    let dkg_start = std::time::Instant::now();
    let mut mbox = p2p_stack.router.session(&session_id);
    let share = tokio::time::timeout(
        std::time::Duration::from_secs(120),
        transport::run_keygen(&mut mbox, session, &session_id),
    )
    .await
    .map_err(|_| anyhow::anyhow!("DKG timed out after 120s"))?
    .map_err(|e| anyhow::anyhow!(e))?;
    let dkg_ms = dkg_start.elapsed().as_millis();
    tracing::info!(dkg_ms, n = participants.len(), min = args.min_signers, "DKG_TIMING keygen complete");
    println!("dkg_ms={dkg_ms}");

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
            source_inputs: it
                .source_inputs
                .iter()
                .map(|s| broadcast::TxOutInput {
                    tx_id: s.tx_id.clone(),
                    vout: s.vout,
                    amount_sats: s.amount_sats,
                })
                .collect(),
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
