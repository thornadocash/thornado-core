//! libp2p transport and party coordination for FROST sessions.
//!
//! The party-coordinator handshake (`/p2p/join-party-leader`) selects the
//! online signing set before a session starts; FROST rounds then flow over
//! `/p2p/frost`. Both use the shared length-prefixed framing in [`crate::wire`].
//!
//! The coordinator decision logic is a pure state machine ([`Coordinator`]) so
//! it can be unit-tested without a network; [`Transport`] is the libp2p glue.

use std::collections::{BTreeSet, HashMap};
use std::path::Path;

use libp2p::swarm::NetworkBehaviour;
use libp2p::{identity, Multiaddr, PeerId, StreamProtocol, Swarm};
use serde::{Deserialize, Serialize};

use crate::frost_session::FROST_PROTOCOL_ID;
use crate::wire::{JoinPartyLeaderComm, ResponseType};

/// One entry in the peer registry: a participant name (its FROST pubkey), its
/// libp2p PeerId, and a dialable multiaddr.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PeerEntry {
    /// FROST participant name (secp256k1 bech32 pubkey), used as the session
    /// identity.
    pub name: String,
    /// libp2p peer id, e.g. "12D3KooW...".
    pub peer_id: String,
    /// dialable multiaddr, e.g. "/ip4/10.0.0.2/tcp/5040".
    pub addr: String,
}

/// Resolved peer registry: name ↔ PeerId, plus dial addresses.
#[derive(Debug, Clone, Default)]
pub struct PeerRegistry {
    by_name: HashMap<String, PeerId>,
    by_peer: HashMap<PeerId, String>,
    addrs: HashMap<PeerId, Multiaddr>,
}

impl PeerRegistry {
    /// Parse a JSON array of [`PeerEntry`] into a registry.
    pub fn from_entries(entries: &[PeerEntry]) -> Result<Self, P2pError> {
        let mut reg = PeerRegistry::default();
        for e in entries {
            let pid: PeerId = e
                .peer_id
                .parse()
                .map_err(|err| P2pError::Config(format!("bad peer_id {}: {err}", e.peer_id)))?;
            let addr: Multiaddr = e
                .addr
                .parse()
                .map_err(|err| P2pError::Config(format!("bad addr {}: {err}", e.addr)))?;
            reg.by_name.insert(e.name.clone(), pid);
            reg.by_peer.insert(pid, e.name.clone());
            reg.addrs.insert(pid, addr);
        }
        Ok(reg)
    }

    /// Load the registry from a JSON file.
    pub fn load(path: impl AsRef<Path>) -> Result<Self, P2pError> {
        let bytes = std::fs::read(path.as_ref())
            .map_err(|e| P2pError::Config(format!("read peers file: {e}")))?;
        let entries: Vec<PeerEntry> =
            serde_json::from_slice(&bytes).map_err(|e| P2pError::Config(e.to_string()))?;
        Self::from_entries(&entries)
    }

    pub fn peer_id(&self, name: &str) -> Option<PeerId> {
        self.by_name.get(name).copied()
    }
    pub fn name(&self, peer: &PeerId) -> Option<String> {
        self.by_peer.get(peer).cloned()
    }
    pub fn addr(&self, peer: &PeerId) -> Option<&Multiaddr> {
        self.addrs.get(peer)
    }
    /// name → PeerId map for building a [`crate::transport::Libp2pMailbox`].
    pub fn name_to_peer(&self) -> HashMap<String, PeerId> {
        self.by_name.clone()
    }
    /// All (PeerId, Multiaddr) pairs to dial/register at startup.
    pub fn dial_targets(&self) -> Vec<(PeerId, Multiaddr)> {
        self.addrs.iter().map(|(p, a)| (*p, a.clone())).collect()
    }
    pub fn len(&self) -> usize {
        self.by_name.len()
    }
    pub fn is_empty(&self) -> bool {
        self.by_name.is_empty()
    }
}

/// Protocol ID for leader-coordinated party joins (matches Go).
pub const JOIN_PARTY_LEADER_PROTOCOL: &str = "/p2p/join-party-leader";

#[derive(Debug, thiserror::Error)]
pub enum P2pError {
    #[error("transport: {0}")]
    Transport(String),
    #[error("config: {0}")]
    Config(String),
    #[error("timed out waiting for party")]
    Timeout,
    #[error("not enough peers online: have {have}, need {need}")]
    Threshold { have: usize, need: usize },
}

/// Role in a party-coordination round.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Role {
    Leader,
    Member,
}

/// Pure state machine for the leader side of `/p2p/join-party-leader`.
///
/// The leader accumulates join requests (including itself) and, once the
/// threshold is reached, emits a `Success` response listing the online set.
/// If the deadline passes first, it emits a `Timeout` response with whoever
/// showed up — matching Go `joinPartyLeader`.
pub struct Coordinator {
    session_id: String,
    threshold: usize,
    expected: BTreeSet<String>,
    online: BTreeSet<String>,
}

impl Coordinator {
    pub fn new_leader(
        session_id: impl Into<String>,
        leader: impl Into<String>,
        expected: impl IntoIterator<Item = String>,
        threshold: usize,
    ) -> Self {
        let leader = leader.into();
        let mut online = BTreeSet::new();
        online.insert(leader); // leader counts itself
        Self {
            session_id: session_id.into(),
            threshold,
            expected: expected.into_iter().collect(),
            online,
        }
    }

    /// Record a join request from a peer. Ignores peers not in the expected
    /// set (Go responds `UnknownPeer`; we simply don't count them).
    pub fn on_request(&mut self, peer: &str) -> bool {
        if !self.expected.contains(peer) {
            return false;
        }
        self.online.insert(peer.to_string())
    }

    pub fn ready(&self) -> bool {
        self.online.len() >= self.threshold
    }

    /// Build the response the leader broadcasts to all online peers.
    pub fn response(&self, timed_out: bool) -> JoinPartyLeaderComm {
        let resp_type = if self.ready() {
            ResponseType::Success
        } else if timed_out {
            ResponseType::Timeout
        } else {
            ResponseType::LeaderNotReady
        };
        JoinPartyLeaderComm {
            id: self.session_id.clone(),
            msg_type: "response".into(),
            resp_type,
            peer_ids: self.online.iter().cloned().collect(),
        }
    }

    /// The selected online signing set (sorted, deduped).
    pub fn selected(&self) -> Vec<String> {
        self.online.iter().cloned().collect()
    }
}

/// Interpret a leader response on the member side: returns the selected set on
/// success, or an error describing why the party did not form.
pub fn interpret_response(msg: &JoinPartyLeaderComm) -> Result<Vec<String>, P2pError> {
    match msg.resp_type {
        ResponseType::Success => Ok(msg.peer_ids.clone()),
        ResponseType::Timeout => Err(P2pError::Timeout),
        _ => Err(P2pError::Threshold {
            have: msg.peer_ids.len(),
            need: 0,
        }),
    }
}

/// libp2p behaviour: the stream protocol multiplexer plus ping. FROST rounds
/// and party coordination run over raw streams via `Control`; ping keeps
/// otherwise-idle connections warm between signing rounds.
#[derive(NetworkBehaviour)]
pub struct Behaviour {
    pub stream: libp2p_stream::Behaviour,
    pub ping: libp2p::ping::Behaviour,
}

/// The two protocols we speak.
pub fn frost_protocol() -> StreamProtocol {
    StreamProtocol::new(FROST_PROTOCOL_ID)
}
pub fn join_party_protocol() -> StreamProtocol {
    StreamProtocol::new(JOIN_PARTY_LEADER_PROTOCOL)
}

/// Build a tokio-backed libp2p swarm speaking noise + yamux over TCP, exposing
/// the stream behaviour. Returns the swarm and a `Control` for opening streams.
pub fn build_swarm(
    keypair: identity::Keypair,
) -> Result<(Swarm<Behaviour>, libp2p_stream::Control), P2pError> {
    let swarm = libp2p::SwarmBuilder::with_existing_identity(keypair)
        .with_tokio()
        .with_tcp(
            libp2p::tcp::Config::default(),
            libp2p::noise::Config::new,
            libp2p::yamux::Config::default,
        )
        .map_err(|e| P2pError::Transport(e.to_string()))?
        .with_behaviour(|_| Behaviour {
            stream: libp2p_stream::Behaviour::new(),
            ping: libp2p::ping::Behaviour::new(
                libp2p::ping::Config::new()
                    .with_interval(std::time::Duration::from_secs(5)),
            ),
        })
        .map_err(|e| P2pError::Transport(e.to_string()))?
        // Keep connections alive between signing rounds: the stream behaviour
        // holds no substreams idle, so without this the swarm would close every
        // connection immediately (default idle timeout is 0).
        .with_swarm_config(|cfg| {
            cfg.with_idle_connection_timeout(std::time::Duration::from_secs(300))
        })
        .build();
    let control = swarm.behaviour().stream.new_control();
    Ok((swarm, control))
}

/// Register each peer's address and dial it, so streams can be opened later.
/// Failures to dial are logged by the caller via the returned error list; a
/// peer that's momentarily unreachable can still be reached once it comes up
/// because the address stays registered.
pub fn register_and_dial(
    swarm: &mut Swarm<Behaviour>,
    registry: &PeerRegistry,
) -> Vec<(PeerId, String)> {
    let mut failures = Vec::new();
    for (peer, addr) in registry.dial_targets() {
        swarm.add_peer_address(peer, addr.clone());
        if let Err(e) = swarm.dial(addr.clone()) {
            failures.push((peer, e.to_string()));
        }
    }
    failures
}

/// Open a stream to `peer` for `protocol` and send one framed payload.
pub async fn send_framed(
    control: &mut libp2p_stream::Control,
    peer: PeerId,
    protocol: StreamProtocol,
    payload: &[u8],
) -> Result<(), P2pError> {
    let mut stream = control
        .open_stream(peer, protocol)
        .await
        .map_err(|e| P2pError::Transport(e.to_string()))?;
    crate::wire::write_frame(&mut stream, payload)
        .await
        .map_err(|e| P2pError::Transport(e.to_string()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn names(n: usize) -> Vec<String> {
        (0..n).map(|i| format!("party{i}")).collect()
    }

    #[test]
    fn leader_forms_party_at_threshold() {
        let all = names(4);
        let mut c = Coordinator::new_leader("sid", "party0", all.clone(), 3);
        assert!(!c.ready()); // only leader so far
        assert!(c.on_request("party1"));
        assert!(!c.ready());
        assert!(c.on_request("party2"));
        assert!(c.ready()); // 3 of 4

        let resp = c.response(false);
        assert_eq!(resp.resp_type, ResponseType::Success);
        assert_eq!(resp.peer_ids, vec!["party0", "party1", "party2"]);
    }

    #[test]
    fn leader_rejects_unexpected_peer() {
        let mut c = Coordinator::new_leader("sid", "party0", names(3), 2);
        assert!(!c.on_request("intruder"));
        assert!(c.on_request("party1"));
        assert!(c.ready());
    }

    #[test]
    fn leader_timeout_when_below_threshold() {
        let mut c = Coordinator::new_leader("sid", "party0", names(4), 3);
        c.on_request("party1");
        let resp = c.response(true);
        assert_eq!(resp.resp_type, ResponseType::Timeout);
        assert_eq!(resp.peer_ids.len(), 2); // leader + party1
    }

    #[test]
    fn member_interprets_success_and_failure() {
        let ok = JoinPartyLeaderComm {
            id: "s".into(),
            msg_type: "response".into(),
            resp_type: ResponseType::Success,
            peer_ids: names(3),
        };
        assert_eq!(interpret_response(&ok).unwrap(), names(3));

        let to = JoinPartyLeaderComm {
            id: "s".into(),
            msg_type: "response".into(),
            resp_type: ResponseType::Timeout,
            peer_ids: names(1),
        };
        assert!(matches!(interpret_response(&to), Err(P2pError::Timeout)));
    }

    #[test]
    fn build_swarm_succeeds() {
        let kp = identity::Keypair::generate_ed25519();
        let (swarm, _control) = build_swarm(kp).unwrap();
        // Smoke check: the local peer id is derived and the behaviour exists.
        assert!(!swarm.local_peer_id().to_string().is_empty());
    }

    #[test]
    fn peer_registry_parses_and_resolves() {
        // Two real ed25519-derived peer ids.
        let a = PeerId::from(identity::Keypair::generate_ed25519().public());
        let b = PeerId::from(identity::Keypair::generate_ed25519().public());
        let entries = vec![
            PeerEntry {
                name: "thorpub1a".into(),
                peer_id: a.to_string(),
                addr: "/ip4/10.0.0.1/tcp/5040".into(),
            },
            PeerEntry {
                name: "thorpub1b".into(),
                peer_id: b.to_string(),
                addr: "/ip4/10.0.0.2/tcp/5040".into(),
            },
        ];
        let reg = PeerRegistry::from_entries(&entries).unwrap();
        assert_eq!(reg.len(), 2);
        assert_eq!(reg.peer_id("thorpub1a"), Some(a));
        assert_eq!(reg.name(&b).as_deref(), Some("thorpub1b"));
        assert!(reg.addr(&a).is_some());
        assert_eq!(reg.name_to_peer().len(), 2);
        assert_eq!(reg.dial_targets().len(), 2);
    }

    #[test]
    fn peer_registry_rejects_bad_entries() {
        let bad = vec![PeerEntry {
            name: "x".into(),
            peer_id: "not-a-peer-id".into(),
            addr: "/ip4/1.2.3.4/tcp/1".into(),
        }];
        assert!(matches!(
            PeerRegistry::from_entries(&bad),
            Err(P2pError::Config(_))
        ));
    }

    #[tokio::test]
    async fn register_and_dial_registers_addresses() {
        // dial() spawns onto the tokio executor, so this needs a runtime.
        let a = PeerId::from(identity::Keypair::generate_ed25519().public());
        let reg = PeerRegistry::from_entries(&[PeerEntry {
            name: "peerA".into(),
            peer_id: a.to_string(),
            addr: "/ip4/127.0.0.1/tcp/15555".into(),
        }])
        .unwrap();
        let (mut swarm, _c) = build_swarm(identity::Keypair::generate_ed25519()).unwrap();
        // Dialing a loopback addr that no one is listening on registers the
        // address without a synchronous error (failures surface async later).
        let failures = register_and_dial(&mut swarm, &reg);
        assert!(failures.is_empty(), "unexpected dial failures: {failures:?}");
    }
}
