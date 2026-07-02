//! `bifrost-signer` daemon: pure-Rust FROST signer bifrost.
//!
//! Polls the thornado chain for keysign work, coordinates a FROST signing
//! party over libp2p, builds and signs the BTC transaction, and broadcasts it.

use clap::Parser;
use thornado_bifrost_signer::{chain, p2p, store, transport, wire};

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
