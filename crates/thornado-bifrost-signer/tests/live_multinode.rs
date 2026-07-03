//! Live multi-node integration test: three real signer nodes form a FROST
//! party over TCP (noise+yamux), run a distributed keygen, then a threshold
//! keysign — all over actual libp2p streams, no in-process shortcuts.
//!
//! Exercises the full wiring the in-process transport test cannot: `build_swarm`
//! → listen → dial → `/p2p/frost` stream negotiation → `Libp2pMailbox` →
//! `run_keygen`/`run_keysign`.
//!
//! Nodes bind to OS-assigned ports (`:0`) and their real listen addresses are
//! discovered before the registry is built, so the test never fights fixed-port
//! `TIME_WAIT`/"address already in use" flakiness. Each edge is dialed by the
//! lower peer-id only (no simultaneous mutual dials), with explicit addresses
//! and a `DisconnectedAndNotDialing` condition so there is never a dial storm.

use std::collections::{HashMap, HashSet};
use std::time::Duration;

use futures::StreamExt;
use libp2p::identity::Keypair;
use libp2p::swarm::dial_opts::{DialOpts, PeerCondition};
use libp2p::swarm::SwarmEvent;
use libp2p::{Multiaddr, PeerId, Swarm};
use tokio::sync::mpsc;

use thornado_bifrost_signer::frost_session::{normalize_participants, SignSession, StoredShare};
use thornado_bifrost_signer::p2p::{build_swarm, frost_protocol, Behaviour, PeerEntry, PeerRegistry};
use thornado_bifrost_signer::transport::{run_keygen, run_keysign, Libp2pMailbox};
use thornado_bifrost_signer::wire;

/// A node that has bound its listener and knows its real address.
struct BoundNode {
    name: String,
    peer_id: PeerId,
    addr: Multiaddr,
    swarm: Swarm<Behaviour>,
    send_control: libp2p_stream::Control,
}

/// Build a swarm, listen on an OS-assigned TCP port, and drive it until the
/// real listen address is known.
async fn bind_node(name: String) -> BoundNode {
    let keypair = Keypair::generate_ed25519();
    let peer_id = PeerId::from(keypair.public());
    let (mut swarm, send_control) = build_swarm(keypair).unwrap();
    swarm
        .listen_on("/ip4/127.0.0.1/tcp/0".parse().unwrap())
        .unwrap();
    let addr = loop {
        if let SwarmEvent::NewListenAddr { address, .. } = swarm.select_next_some().await {
            break address;
        }
    };
    BoundNode {
        name,
        peer_id,
        addr,
        swarm,
        send_control,
    }
}

/// Per-node outcome plus phase timings (milliseconds).
struct NodeResult {
    name: String,
    share: StoredShare,
    signature: Option<[u8; 64]>,
    keygen_ms: f64,
    keysign_ms: Option<f64>,
}

/// Run one node: dial peers, wait for full connectivity, then run keygen and
/// (if selected) keysign. Returns the node's share, signature, and phase timings.
async fn run_node(
    node: BoundNode,
    registry: PeerRegistry,
    all_names: Vec<String>,
    min: u16,
    chosen: Vec<String>,
    message: Vec<u8>,
) -> NodeResult {
    let BoundNode {
        name,
        peer_id: local,
        addr: _,
        mut swarm,
        send_control,
    } = node;

    let expected: HashSet<PeerId> =
        registry.dial_targets().into_iter().map(|(p, _)| p).collect();
    let targets = registry.dial_targets();

    // Inbound frost streams → mailbox channel, translating PeerId → name.
    let mut accept_control = swarm.behaviour().stream.new_control();
    let mut incoming = accept_control.accept(frost_protocol()).unwrap();
    let (inbound_tx, inbound_rx) = mpsc::channel::<(String, Vec<u8>)>(1024);
    let accept_registry = registry.clone();
    tokio::spawn(async move {
        while let Some((peer, mut stream)) = incoming.next().await {
            let tx = inbound_tx.clone();
            let from = accept_registry.name(&peer).unwrap_or_else(|| peer.to_string());
            tokio::spawn(async move {
                if let Ok(payload) = wire::read_frame(&mut stream).await {
                    let _ = tx.send((from, wire::frame(&payload))).await;
                }
            });
        }
    });

    // Drive the swarm; dial each edge once (lower peer-id initiates), with
    // explicit addresses and a condition that suppresses duplicate/among-dialing
    // attempts. Report connection readiness to the waiter below.
    let (ready_tx, mut ready_rx) = mpsc::channel::<PeerId>(16);
    tokio::spawn(async move {
        let mut connected: HashSet<PeerId> = HashSet::new();
        let mut redial = tokio::time::interval(Duration::from_millis(500));
        loop {
            tokio::select! {
                _ = redial.tick() => {
                    for (peer, addr) in &targets {
                        if local < *peer && !connected.contains(peer) {
                            let opts = DialOpts::peer_id(*peer)
                                .addresses(vec![addr.clone()])
                                .condition(PeerCondition::DisconnectedAndNotDialing)
                                .build();
                            let _ = swarm.dial(opts);
                        }
                    }
                }
                event = swarm.select_next_some() => {
                    match event {
                        SwarmEvent::ConnectionEstablished { peer_id, .. } => {
                            if connected.insert(peer_id) {
                                let _ = ready_tx.send(peer_id).await;
                            }
                        }
                        SwarmEvent::ConnectionClosed { peer_id, .. } => {
                            connected.remove(&peer_id);
                        }
                        _ => {}
                    }
                }
            }
        }
    });

    // Wait for connectivity to every peer.
    let mut have: HashSet<PeerId> = HashSet::new();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(30);
    while have.len() < expected.len() {
        match tokio::time::timeout_at(deadline, ready_rx.recv()).await {
            Ok(Some(peer)) if expected.contains(&peer) => {
                have.insert(peer);
            }
            Ok(Some(_)) => {}
            _ => panic!("{name} connected to {}/{} peers before timeout", have.len(), expected.len()),
        }
    }

    let mut mbox = Libp2pMailbox::new(send_control, registry.name_to_peer(), inbound_rx);

    // Distributed keygen over the live connections.
    let session = thornado_bifrost_signer::frost_session::KeygenSession::new(
        name.clone(),
        all_names,
        min,
    )
    .unwrap();
    // Time keygen from this node's perspective (dialing/connectivity excluded).
    let kg_start = std::time::Instant::now();
    let share = tokio::time::timeout(
        Duration::from_secs(30),
        run_keygen(&mut mbox, session, "live-keygen"),
    )
    .await
    .unwrap_or_else(|_| panic!("[{name}] keygen timed out"))
    .unwrap();
    let keygen_ms = kg_start.elapsed().as_secs_f64() * 1000.0;

    // Threshold keysign if this node was chosen.
    let (signature, keysign_ms) = if chosen.contains(&name) {
        let s = SignSession::new(&share, name.clone(), chosen.clone(), message.clone()).unwrap();
        let ks_start = std::time::Instant::now();
        let sig = tokio::time::timeout(
            Duration::from_secs(30),
            run_keysign(&mut mbox, s, "live-keysign"),
        )
        .await
        .unwrap_or_else(|_| panic!("[{name}] keysign timed out"))
        .unwrap();
        (Some(sig), Some(ks_start.elapsed().as_secs_f64() * 1000.0))
    } else {
        (None, None)
    };

    NodeResult {
        name,
        share,
        signature,
        keygen_ms,
        keysign_ms,
    }
}

/// Bring up `n` nodes over TCP, run distributed keygen then a `min`-of-`n`
/// keysign, assert correctness, and print phase timings. Wall-clock keygen /
/// keysign are measured as the max across participating nodes (the party is
/// only done when the slowest member is).
async fn run_party(n: usize, min: u16) {
    let overall = std::time::Instant::now();

    // Phase 1: bind all nodes and discover their real listen addresses.
    let mut bound = Vec::new();
    for i in 0..n {
        bound.push(bind_node(format!("thorpub1party{i}")).await);
    }
    let all_names =
        normalize_participants(&bound.iter().map(|b| b.name.clone()).collect::<Vec<_>>());
    let directory: Vec<PeerEntry> = bound
        .iter()
        .map(|b| PeerEntry {
            name: b.name.clone(),
            peer_id: b.peer_id.to_string(),
            addr: b.addr.to_string(),
        })
        .collect();

    let chosen: Vec<String> = all_names[..min as usize].to_vec();
    let message = b"thornado taproot sighash placeholder--32b".to_vec();

    // Phase 2: run every node; each dials the others and drives its sessions.
    let mut tasks = Vec::new();
    for node in bound {
        let registry = {
            let entries: Vec<PeerEntry> =
                directory.iter().filter(|e| e.name != node.name).cloned().collect();
            PeerRegistry::from_entries(&entries).unwrap()
        };
        let names = all_names.clone();
        let chosen_c = chosen.clone();
        let msg = message.clone();
        tasks.push(tokio::spawn(run_node(node, registry, names, min, chosen_c, msg)));
    }

    let mut shares: HashMap<String, StoredShare> = HashMap::new();
    let mut sigs: Vec<[u8; 64]> = Vec::new();
    let mut keygen_max = 0.0f64;
    let mut keysign_max = 0.0f64;
    for t in tasks {
        let r = t.await.unwrap();
        keygen_max = keygen_max.max(r.keygen_ms);
        if let Some(ks) = r.keysign_ms {
            keysign_max = keysign_max.max(ks);
        }
        shares.insert(r.name, r.share);
        if let Some(s) = r.signature {
            sigs.push(s);
        }
    }

    // Every node derived the same group key.
    let group_keys: HashSet<String> = shares
        .values()
        .map(|s| {
            hex::encode(
                s.public_key_package()
                    .unwrap()
                    .verifying_key()
                    .serialize()
                    .unwrap(),
            )
        })
        .collect();
    assert_eq!(group_keys.len(), 1, "nodes disagree on DKG group key");

    // The chosen signers produced the same valid aggregate signature.
    assert_eq!(sigs.len(), min as usize);
    assert!(sigs.windows(2).all(|w| w[0] == w[1]), "signers disagree");
    let pkp = shares[&chosen[0]].public_key_package().unwrap();
    let sig = frost_secp256k1_tr::Signature::deserialize(&sigs[0]).unwrap();
    pkp.verifying_key()
        .verify(&message, &sig)
        .expect("aggregate verifies against live-DKG group key");

    eprintln!(
        "TIMING n={n} min-of-n={min}: keygen(all {n}) {keygen_ms:.1}ms | \
         keysign({min}-of-{n}) {keysign_ms:.1}ms | total incl. setup {total:.1}ms",
        keygen_ms = keygen_max,
        keysign_ms = keysign_max,
        total = overall.elapsed().as_secs_f64() * 1000.0,
    );
}

// Real-socket integration tests: isolated from the parallel unit-test run to
// keep `cargo test` deterministic. Run explicitly (show timing with --nocapture):
//   cargo test -p thornado-bifrost-signer --test live_multinode -- --ignored --nocapture
#[ignore = "live TCP integration; run with --test live_multinode -- --ignored"]
#[tokio::test(flavor = "multi_thread", worker_threads = 6)]
async fn three_nodes_keygen_then_keysign_over_tcp() {
    run_party(3, 2).await;
}

#[ignore = "live TCP integration; run with --test live_multinode -- --ignored"]
#[tokio::test(flavor = "multi_thread", worker_threads = 8)]
async fn four_nodes_keygen_then_keysign_over_tcp() {
    run_party(4, 3).await;
}
