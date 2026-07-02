//! Wire framing and message codecs for interop with Go bifrost peers.
//!
//! Two protocols share the same stream framing (`[u32 LE length][payload]`,
//! 20MB cap): `/p2p/frost` (FROST rounds, JSON `WrappedMessage`) and
//! `/p2p/join-party-leader` (party coordination, protobuf `JoinPartyLeaderComm`).

use crate::frost_session::{LENGTH_HEADER, MAX_PAYLOAD};

#[derive(Debug, thiserror::Error)]
pub enum WireError {
    #[error("payload length {0} exceeds max {MAX_PAYLOAD}")]
    TooLarge(u32),
    #[error("short read: need {need}, got {got}")]
    ShortRead { need: usize, got: usize },
    #[error("protobuf: {0}")]
    Protobuf(String),
}

/// Frame a payload with the 4-byte little-endian length prefix (Go
/// `WriteStreamWithBuffer`).
pub fn frame(payload: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(LENGTH_HEADER + payload.len());
    out.extend_from_slice(&(payload.len() as u32).to_le_bytes());
    out.extend_from_slice(payload);
    out
}

/// A deframed payload and the unconsumed remainder of the buffer.
pub type Deframed<'a> = (&'a [u8], &'a [u8]);

/// Read one length-prefixed frame from a buffer, returning (payload, rest).
pub fn deframe(buf: &[u8]) -> Result<Option<Deframed<'_>>, WireError> {
    if buf.len() < LENGTH_HEADER {
        return Ok(None);
    }
    let len = u32::from_le_bytes([buf[0], buf[1], buf[2], buf[3]]);
    if len as usize > MAX_PAYLOAD {
        return Err(WireError::TooLarge(len));
    }
    let end = LENGTH_HEADER + len as usize;
    if buf.len() < end {
        return Ok(None);
    }
    Ok(Some((&buf[LENGTH_HEADER..end], &buf[end..])))
}

/// Async helper: read exactly one frame from an `AsyncRead`.
pub async fn read_frame<R>(r: &mut R) -> std::io::Result<Vec<u8>>
where
    R: futures::AsyncRead + Unpin,
{
    use futures::AsyncReadExt;
    let mut len_bytes = [0u8; LENGTH_HEADER];
    r.read_exact(&mut len_bytes).await?;
    let len = u32::from_le_bytes(len_bytes) as usize;
    if len > MAX_PAYLOAD {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "payload exceeds max",
        ));
    }
    let mut payload = vec![0u8; len];
    r.read_exact(&mut payload).await?;
    Ok(payload)
}

/// Async helper: write one framed payload to an `AsyncWrite`.
pub async fn write_frame<W>(w: &mut W, payload: &[u8]) -> std::io::Result<()>
where
    W: futures::AsyncWrite + Unpin,
{
    use futures::AsyncWriteExt;
    w.write_all(&frame(payload)).await?;
    w.flush().await
}

/// Party-coordination response type (matches Go `JoinPartyLeaderComm.ResponseType`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(i32)]
pub enum ResponseType {
    Unknown = 0,
    Success = 1,
    Timeout = 2,
    LeaderNotReady = 3,
    UnknownPeer = 4,
}

impl ResponseType {
    pub fn from_i32(v: i32) -> Self {
        match v {
            1 => Self::Success,
            2 => Self::Timeout,
            3 => Self::LeaderNotReady,
            4 => Self::UnknownPeer,
            _ => Self::Unknown,
        }
    }
}

/// `JoinPartyLeaderComm` — hand-rolled proto3 codec (fields: ID=1 string,
/// MsgType=2 string, type=3 enum/varint, PeerIDs=4 repeated string).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct JoinPartyLeaderComm {
    pub id: String,
    pub msg_type: String,
    pub resp_type: ResponseType,
    pub peer_ids: Vec<String>,
}

impl JoinPartyLeaderComm {
    pub fn encode(&self) -> Vec<u8> {
        let mut out = Vec::new();
        if !self.id.is_empty() {
            proto_string(&mut out, 1, &self.id);
        }
        if !self.msg_type.is_empty() {
            proto_string(&mut out, 2, &self.msg_type);
        }
        if self.resp_type != ResponseType::Unknown {
            proto_tag(&mut out, 3, 0); // varint wire type
            proto_varint(&mut out, self.resp_type as i32 as u64);
        }
        for p in &self.peer_ids {
            proto_string(&mut out, 4, p);
        }
        out
    }

    pub fn decode(mut buf: &[u8]) -> Result<Self, WireError> {
        let mut m = JoinPartyLeaderComm {
            id: String::new(),
            msg_type: String::new(),
            resp_type: ResponseType::Unknown,
            peer_ids: Vec::new(),
        };
        while !buf.is_empty() {
            let (tag, rest) = read_varint(buf)?;
            buf = rest;
            let field = tag >> 3;
            let wire = tag & 0x7;
            match (field, wire) {
                (1, 2) => {
                    let (s, rest) = read_len_delim(buf)?;
                    m.id = String::from_utf8_lossy(s).into_owned();
                    buf = rest;
                }
                (2, 2) => {
                    let (s, rest) = read_len_delim(buf)?;
                    m.msg_type = String::from_utf8_lossy(s).into_owned();
                    buf = rest;
                }
                (3, 0) => {
                    let (v, rest) = read_varint(buf)?;
                    m.resp_type = ResponseType::from_i32(v as i32);
                    buf = rest;
                }
                (4, 2) => {
                    let (s, rest) = read_len_delim(buf)?;
                    m.peer_ids.push(String::from_utf8_lossy(s).into_owned());
                    buf = rest;
                }
                (_, 0) => {
                    let (_, rest) = read_varint(buf)?;
                    buf = rest;
                }
                (_, 2) => {
                    let (_, rest) = read_len_delim(buf)?;
                    buf = rest;
                }
                _ => return Err(WireError::Protobuf(format!("bad wire type {wire}"))),
            }
        }
        Ok(m)
    }
}

fn proto_tag(out: &mut Vec<u8>, field: u32, wire: u32) {
    proto_varint(out, ((field << 3) | wire) as u64);
}
fn proto_varint(out: &mut Vec<u8>, mut v: u64) {
    loop {
        let mut b = (v & 0x7f) as u8;
        v >>= 7;
        if v != 0 {
            b |= 0x80;
        }
        out.push(b);
        if v == 0 {
            break;
        }
    }
}
fn proto_string(out: &mut Vec<u8>, field: u32, s: &str) {
    proto_tag(out, field, 2);
    proto_varint(out, s.len() as u64);
    out.extend_from_slice(s.as_bytes());
}
fn read_varint(buf: &[u8]) -> Result<(u64, &[u8]), WireError> {
    let mut result = 0u64;
    let mut shift = 0;
    for (i, &b) in buf.iter().enumerate() {
        result |= ((b & 0x7f) as u64) << shift;
        if b & 0x80 == 0 {
            return Ok((result, &buf[i + 1..]));
        }
        shift += 7;
        if shift >= 64 {
            return Err(WireError::Protobuf("varint overflow".into()));
        }
    }
    Err(WireError::ShortRead {
        need: shift / 7 + 1,
        got: buf.len(),
    })
}
fn read_len_delim(buf: &[u8]) -> Result<(&[u8], &[u8]), WireError> {
    let (len, rest) = read_varint(buf)?;
    let len = len as usize;
    if rest.len() < len {
        return Err(WireError::ShortRead {
            need: len,
            got: rest.len(),
        });
    }
    Ok((&rest[..len], &rest[len..]))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn frame_roundtrip() {
        let payload = b"hello frost";
        let framed = frame(payload);
        assert_eq!(&framed[..4], &(payload.len() as u32).to_le_bytes());
        let (got, rest) = deframe(&framed).unwrap().unwrap();
        assert_eq!(got, payload);
        assert!(rest.is_empty());
    }

    #[test]
    fn deframe_partial_returns_none() {
        assert!(deframe(&[1, 0]).unwrap().is_none()); // header incomplete
        let mut buf = 100u32.to_le_bytes().to_vec();
        buf.extend_from_slice(b"short");
        assert!(deframe(&buf).unwrap().is_none()); // body incomplete
    }

    #[test]
    fn deframe_rejects_oversize() {
        let buf = (MAX_PAYLOAD as u32 + 1).to_le_bytes().to_vec();
        assert!(matches!(deframe(&buf), Err(WireError::TooLarge(_))));
    }

    #[test]
    fn join_party_proto_roundtrip() {
        let msg = JoinPartyLeaderComm {
            id: "session-abc".into(),
            msg_type: "response".into(),
            resp_type: ResponseType::Success,
            peer_ids: vec!["peerA".into(), "peerB".into(), "peerC".into()],
        };
        let encoded = msg.encode();
        let decoded = JoinPartyLeaderComm::decode(&encoded).unwrap();
        assert_eq!(msg, decoded);
    }

    #[test]
    fn join_party_request_minimal() {
        let msg = JoinPartyLeaderComm {
            id: "id1".into(),
            msg_type: "request".into(),
            resp_type: ResponseType::Unknown,
            peer_ids: vec![],
        };
        let decoded = JoinPartyLeaderComm::decode(&msg.encode()).unwrap();
        assert_eq!(decoded.id, "id1");
        assert_eq!(decoded.msg_type, "request");
        assert_eq!(decoded.resp_type, ResponseType::Unknown);
        assert!(decoded.peer_ids.is_empty());
    }

    #[test]
    fn decode_skips_unknown_fields() {
        // field 9, varint wire type, value 42 — must be ignored.
        let mut buf = Vec::new();
        proto_tag(&mut buf, 9, 0);
        proto_varint(&mut buf, 42);
        proto_string(&mut buf, 1, "keep");
        let decoded = JoinPartyLeaderComm::decode(&buf).unwrap();
        assert_eq!(decoded.id, "keep");
    }

    // ---- Cross-language interop against fixtures emitted by the real Go
    // types (go-thornado/cmd/interop-fixtures). Regenerate with:
    //   cd go-thornado && go run ./cmd/interop-fixtures ../test-fixtures/interop

    fn fixture(name: &str) -> Vec<u8> {
        let path = format!("{}/../../test-fixtures/interop/{}", env!("CARGO_MANIFEST_DIR"), name);
        std::fs::read(&path).unwrap_or_else(|e| panic!("read fixture {path}: {e}"))
    }

    #[test]
    fn go_join_party_response_decodes() {
        // Authoritative gogo-proto bytes from Go messages.JoinPartyLeaderComm.
        let bytes = fixture("join_party_response.pb");
        let decoded = JoinPartyLeaderComm::decode(&bytes).unwrap();
        assert_eq!(decoded.id, "0f1e2d3c4b5a69788796a5b4c3d2e1f0");
        assert_eq!(decoded.msg_type, "response");
        assert_eq!(decoded.resp_type, ResponseType::Success);
        assert_eq!(decoded.peer_ids, vec!["party0", "party1", "party2"]);
        // Re-encode must be byte-identical to what Go produced.
        assert_eq!(decoded.encode(), bytes, "Rust re-encode diverges from Go proto bytes");
    }

    #[test]
    fn go_join_party_request_decodes() {
        let bytes = fixture("join_party_request.pb");
        let decoded = JoinPartyLeaderComm::decode(&bytes).unwrap();
        assert_eq!(decoded.msg_type, "request");
        assert_eq!(decoded.resp_type, ResponseType::Unknown);
        assert!(decoded.peer_ids.is_empty());
        assert_eq!(decoded.encode(), bytes);
    }

    #[test]
    fn go_wrapped_keysign_envelope_decodes() {
        use crate::frost_session::{ProtocolMessage, WrappedMessage, MSG_TYPE_KEYSIGN};
        // Authoritative JSON from Go json.Marshal(messages.WrappedMessage{...}).
        let bytes = fixture("wrapped_keysign.json");
        let wrapped: WrappedMessage = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(wrapped.message_type, MSG_TYPE_KEYSIGN); // 7
        assert_eq!(wrapped.message_id, "0f1e2d3c4b5a69788796a5b4c3d2e1f0");
        // payload is base64-decoded (Go []byte JSON encoding) back to the
        // ProtocolMessage JSON bytes.
        let pm: ProtocolMessage = serde_json::from_slice(&wrapped.payload).unwrap();
        assert_eq!(pm.kind, "sign_round1");
        assert_eq!(pm.from, "party0");
        assert_eq!(pm.to, vec!["party1", "party2"]);
        // Re-serialize the envelope and confirm Go can round-trip it: the
        // payload field must be base64, not a JSON byte array.
        let reser = serde_json::to_vec(&wrapped).unwrap();
        let reparsed: serde_json::Value = serde_json::from_slice(&reser).unwrap();
        assert!(reparsed["payload"].is_string(), "payload must serialize as base64 string for Go interop");
        assert_eq!(reparsed["message_type"], 7);
    }
}
