//! Live session transport: drives FROST rounds between peers.
//!
//! The [`Mailbox`] trait abstracts message delivery so the session driver can
//! be exercised in-process over real async channels (see tests) and run over
//! libp2p streams in production ([`Libp2pMailbox`]). Envelopes are the Go
//! `WrappedMessage` (`message_id` = hex session id, `payload` = ProtocolMessage
//! JSON) framed with the shared length prefix.

use std::collections::HashMap;
use std::future::Future;

use crate::frost_session::{
    KeygenSession, ProtocolMessage, SignSession, StoredShare, WrappedMessage, MSG_TYPE_KEYGEN,
    MSG_TYPE_KEYSIGN,
};

#[derive(Debug, thiserror::Error)]
pub enum TransportError {
    #[error("frost: {0}")]
    Frost(#[from] crate::frost_session::FrostError),
    #[error("codec: {0}")]
    Codec(String),
    #[error("mailbox closed before session completed")]
    MailboxClosed,
    #[error("transport: {0}")]
    Transport(String),
}

type Result<T> = std::result::Result<T, TransportError>;

/// Encode a ProtocolMessage into a framed WrappedMessage ready for the wire.
pub fn wrap(session_id: &str, msg_type: u8, msg: &ProtocolMessage) -> Result<Vec<u8>> {
    let payload = serde_json::to_vec(msg).map_err(|e| TransportError::Codec(e.to_string()))?;
    let wrapped = WrappedMessage {
        message_type: msg_type,
        message_id: session_id.to_string(),
        payload,
    };
    let bytes = serde_json::to_vec(&wrapped).map_err(|e| TransportError::Codec(e.to_string()))?;
    Ok(crate::wire::frame(&bytes))
}

/// Decode a framed WrappedMessage back into (session_id, ProtocolMessage).
pub fn unwrap(framed: &[u8]) -> Result<(String, ProtocolMessage)> {
    let (payload, _rest) = crate::wire::deframe(framed)
        .map_err(|e| TransportError::Codec(e.to_string()))?
        .ok_or_else(|| TransportError::Codec("incomplete frame".into()))?;
    let wrapped: WrappedMessage =
        serde_json::from_slice(payload).map_err(|e| TransportError::Codec(e.to_string()))?;
    let pm: ProtocolMessage =
        serde_json::from_slice(&wrapped.payload).map_err(|e| TransportError::Codec(e.to_string()))?;
    Ok((wrapped.message_id, pm))
}

/// Message delivery to and from other parties, keyed by participant name.
pub trait Mailbox {
    /// Send a framed WrappedMessage to a named participant.
    fn send(&mut self, to: &str, framed: Vec<u8>) -> impl Future<Output = Result<()>> + Send;
    /// Await the next inbound (from, framed) message, or `None` if closed.
    fn recv(&mut self) -> impl Future<Output = Option<(String, Vec<u8>)>> + Send;
}

/// Fan out a session's queued outputs to their target participants.
async fn flush<M: Mailbox>(
    mbox: &mut M,
    session_id: &str,
    msg_type: u8,
    outputs: Vec<ProtocolMessage>,
) -> Result<()> {
    for msg in outputs {
        let framed = wrap(session_id, msg_type, &msg)?;
        for to in &msg.to {
            mbox.send(to, framed.clone()).await?;
        }
    }
    Ok(())
}

/// Drive a keysign session to completion over the mailbox, returning the
/// 64-byte aggregate signature.
pub async fn run_keysign<M: Mailbox>(
    mbox: &mut M,
    mut session: SignSession,
    session_id: &str,
) -> Result<[u8; 64]> {
    let outs = session.drain_outputs();
    flush(mbox, session_id, MSG_TYPE_KEYSIGN, outs).await?;

    while !session.finished() {
        let (_from, framed) = mbox.recv().await.ok_or(TransportError::MailboxClosed)?;
        let (mid, pm) = unwrap(&framed)?;
        if mid != session_id {
            continue; // message for a different session
        }
        session.handle(&pm)?;
        let outs = session.drain_outputs();
        flush(mbox, session_id, MSG_TYPE_KEYSIGN, outs).await?;
    }
    session
        .signature()
        .ok_or_else(|| TransportError::Transport("finished without signature".into()))
}

/// Drive several keysign sessions (keyed by session id) to completion over one
/// mailbox, routing each inbound frame to its session. A batched BTC tx signs
/// one sighash per input; peers may progress sessions at different speeds, so
/// a single-session driver would drop frames that belong to a sibling session.
/// Frames for unknown session ids (stale attempts) are dropped.
pub async fn run_keysign_multi<M: Mailbox>(
    mbox: &mut M,
    sessions: &mut std::collections::BTreeMap<String, SignSession>,
) -> Result<std::collections::BTreeMap<String, [u8; 64]>> {
    for (sid, session) in sessions.iter_mut() {
        let outs = session.drain_outputs();
        flush(mbox, sid, MSG_TYPE_KEYSIGN, outs).await?;
    }

    while sessions.values().any(|s| !s.finished()) {
        let (_from, framed) = mbox.recv().await.ok_or(TransportError::MailboxClosed)?;
        let (mid, pm) = unwrap(&framed)?;
        let Some(session) = sessions.get_mut(&mid) else {
            continue; // stale or foreign session
        };
        session.handle(&pm)?;
        let outs = session.drain_outputs();
        flush(mbox, &mid, MSG_TYPE_KEYSIGN, outs).await?;
    }

    let mut sigs = std::collections::BTreeMap::new();
    for (sid, session) in sessions.iter() {
        let sig = session
            .signature()
            .ok_or_else(|| TransportError::Transport("finished without signature".into()))?;
        sigs.insert(sid.clone(), sig);
    }
    Ok(sigs)
}

/// Drive a distributed keygen (DKG) session to completion, returning this
/// party's `StoredShare`.
pub async fn run_keygen<M: Mailbox>(
    mbox: &mut M,
    mut session: KeygenSession,
    session_id: &str,
) -> Result<StoredShare> {
    let outs = session.drain_outputs();
    flush(mbox, session_id, MSG_TYPE_KEYGEN, outs).await?;

    while !session.finished() {
        let (_from, framed) = mbox.recv().await.ok_or(TransportError::MailboxClosed)?;
        let (mid, pm) = unwrap(&framed)?;
        if mid != session_id {
            continue;
        }
        session.handle(&pm)?;
        let outs = session.drain_outputs();
        flush(mbox, session_id, MSG_TYPE_KEYGEN, outs).await?;
    }
    session
        .stored_share()
        .cloned()
        .ok_or_else(|| TransportError::Transport("finished without share".into()))
}

/// libp2p-backed mailbox: sends open a fresh `/p2p/frost` stream per message
/// (matching Go), receives are fed from a background accept loop via a channel.
pub struct Libp2pMailbox {
    control: libp2p_stream::Control,
    peers: HashMap<String, libp2p::PeerId>,
    inbound: tokio::sync::mpsc::Receiver<(String, Vec<u8>)>,
}

impl Libp2pMailbox {
    pub fn new(
        control: libp2p_stream::Control,
        peers: HashMap<String, libp2p::PeerId>,
        inbound: tokio::sync::mpsc::Receiver<(String, Vec<u8>)>,
    ) -> Self {
        Self {
            control,
            peers,
            inbound,
        }
    }
}

impl Mailbox for Libp2pMailbox {
    async fn send(&mut self, to: &str, framed: Vec<u8>) -> Result<()> {
        let peer = *self
            .peers
            .get(to)
            .ok_or_else(|| TransportError::Transport(format!("unknown peer {to}")))?;

        // Opening a stream can transiently fail if the connection isn't ready
        // yet (or briefly dropped and re-dialed). Retry long enough to span
        // the daemon's ~10s redial cycle — the round message is idempotent at
        // the session layer.
        let mut last_err = None;
        for attempt in 0..12u32 {
            // Bound each attempt: a stuck open_stream (peer mid-reconnect)
            // must become a retryable error rather than hang the session.
            let opened = tokio::time::timeout(
                std::time::Duration::from_secs(5),
                self.control.open_stream(peer, crate::p2p::frost_protocol()),
            )
            .await;
            let opened = match opened {
                Ok(r) => r,
                Err(_) => {
                    last_err = Some("open_stream timed out".to_string());
                    continue;
                }
            };
            match opened {
                Ok(mut stream) => {
                    use futures::AsyncWriteExt;
                    // `framed` already carries the length prefix; write it raw.
                    // A write can still fail (e.g. the connection was closing
                    // under us — duplicate-connection pruning after mutual
                    // dials); that is as retryable as a failed open.
                    let write = async {
                        stream.write_all(&framed).await?;
                        stream.flush().await
                    };
                    match write.await {
                        Ok(()) => return Ok(()),
                        Err(e) => {
                            last_err = Some(e.to_string());
                            let backoff =
                                std::time::Duration::from_millis(150 * (attempt as u64 + 1));
                            tokio::time::sleep(backoff).await;
                        }
                    }
                }
                Err(e) => {
                    last_err = Some(e.to_string());
                    let backoff = std::time::Duration::from_millis(150 * (attempt as u64 + 1));
                    tokio::time::sleep(backoff).await;
                }
            }
        }
        Err(TransportError::Transport(format!(
            "open_stream to {to} failed after retries: {}",
            last_err.unwrap_or_default()
        )))
    }

    async fn recv(&mut self) -> Option<(String, Vec<u8>)> {
        self.inbound.recv().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::frost_session::normalize_participants;
    use std::collections::HashMap;
    use tokio::sync::mpsc;

    /// In-process mailbox over tokio channels: the "network" is a shared map of
    /// per-party senders. Exercises the real framing + envelope + driver.
    struct MemMailbox {
        me: String,
        senders: HashMap<String, mpsc::UnboundedSender<(String, Vec<u8>)>>,
        inbox: mpsc::UnboundedReceiver<(String, Vec<u8>)>,
    }

    impl Mailbox for MemMailbox {
        async fn send(&mut self, to: &str, framed: Vec<u8>) -> Result<()> {
            if let Some(tx) = self.senders.get(to) {
                let _ = tx.send((self.me.clone(), framed));
            }
            Ok(())
        }
        async fn recv(&mut self) -> Option<(String, Vec<u8>)> {
            self.inbox.recv().await
        }
    }

    fn party_names(n: usize) -> Vec<String> {
        normalize_participants(&(0..n).map(|i| format!("party{i}")).collect::<Vec<_>>())
    }

    #[test]
    fn wrap_unwrap_roundtrip() {
        let pm = ProtocolMessage {
            kind: "sign_round1".into(),
            from: "party0".into(),
            to: vec!["party1".into()],
            payload: "AAAA".into(),
        };
        let framed = wrap("sid123", MSG_TYPE_KEYSIGN, &pm).unwrap();
        let (mid, got) = unwrap(&framed).unwrap();
        assert_eq!(mid, "sid123");
        assert_eq!(got.kind, "sign_round1");
        assert_eq!(got.from, "party0");
    }

    /// Full DKG then keysign driven entirely through the async transport over
    /// in-process channels — no dealer, no shared state, real framing.
    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn distributed_keygen_then_keysign_over_transport() {
        let names = party_names(3);
        let min = 2u16;

        // Build a channel per party and a shared sender map.
        let mut receivers = HashMap::new();
        let mut senders = HashMap::new();
        for name in &names {
            let (tx, rx) = mpsc::unbounded_channel();
            senders.insert(name.clone(), tx);
            receivers.insert(name.clone(), rx);
        }

        // ---- DKG phase ----
        let kg_id = "keygen-sid";
        let mut kg_handles = Vec::new();
        for name in &names {
            let mbox = MemMailbox {
                me: name.clone(),
                senders: senders.clone(),
                inbox: receivers.remove(name).unwrap(),
            };
            let session = KeygenSession::new(name.clone(), names.clone(), min).unwrap();
            let id = kg_id.to_string();
            let mut mbox = mbox;
            kg_handles.push((
                name.clone(),
                tokio::spawn(async move { run_keygen(&mut mbox, session, &id).await.map(|s| (s, mbox)) }),
            ));
        }

        let mut shares = HashMap::new();
        let mut mailboxes = HashMap::new();
        for (name, h) in kg_handles {
            let (share, mbox) = h.await.unwrap().unwrap();
            shares.insert(name.clone(), share);
            mailboxes.insert(name, mbox);
        }
        assert_eq!(shares.len(), 3);

        // All parties agree on the group key.
        let group_hex: Vec<String> = shares
            .values()
            .map(|s| s.public_key_package().unwrap())
            .map(|p| hex::encode(p.verifying_key().serialize().unwrap()))
            .collect();
        assert!(group_hex.windows(2).all(|w| w[0] == w[1]));

        // ---- keysign phase: threshold subset (2 of 3) ----
        let chosen: Vec<String> = names[..min as usize].to_vec();
        let message = b"thornado taproot sighash placeholder--32b".to_vec();
        let ks_id = "keysign-sid";

        // Drain any residual channel state by reusing the same mailboxes; the
        // chosen subset signs, non-chosen parties simply don't participate.
        let mut ks_handles = Vec::new();
        for name in &chosen {
            let mut mbox = mailboxes.remove(name).unwrap();
            let session =
                SignSession::new(&shares[name], name.clone(), chosen.clone(), message.clone())
                    .unwrap();
            let id = ks_id.to_string();
            ks_handles.push(tokio::spawn(async move {
                run_keysign(&mut mbox, session, &id).await
            }));
        }

        let mut sigs = Vec::new();
        for h in ks_handles {
            sigs.push(h.await.unwrap().unwrap());
        }
        // Both signers produced the identical aggregate signature.
        assert_eq!(sigs.len(), 2);
        assert_eq!(sigs[0], sigs[1]);

        // Verify against the DKG group key.
        let pkp = shares[&chosen[0]].public_key_package().unwrap();
        let sig = frost_secp256k1_tr::Signature::deserialize(&sigs[0]).unwrap();
        pkp.verifying_key().verify(&message, &sig).unwrap();
    }
}
