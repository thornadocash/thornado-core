# FROST Session Port Spec (Go → Rust)

Source of truth for interop. A Rust FROST session must match Go peers byte-for-byte.

## FFI surface being replaced (crates/thornado-frost-ffi/src/lib.rs)
Session-handle state machine over `frost-secp256k1-tr` v3. Functions: keygen_session_new,
sign_session_new, session_output_message, session_message_receiver, session_input_message,
session_finish, session_abort_message, session_free, free.

- keygen: `dkg::part1` → part2 → part3, KeyPackage + PublicKeyPackage.
- keysign: `round1::commit` → `round2::sign[_with_tweak]` → `aggregate[_with_tweak]`, verify.
- Identifier→u16: last 2 bytes of 32-byte serialization, big-endian. participant_index 1-based.

## Wire protocol
- libp2p protocol ID: `/p2p/frost`
- Framing: `[4-byte length little-endian][payload]`, max 20MB, read/write timeout 20s.
- WrappedMessage JSON: `{ "message_type": 6|7, "message_id": hex(sessionID), "payload": <ProtocolMessage JSON bytes> }` (6=keygen,7=keysign).
- ProtocolMessage JSON: `{ "kind", "from", "to":[], "payload": base64(frost bytes) }`.
  kinds: keygen_round1, keygen_round2, sign_round1, sign_round2, keygen_abort, sign_abort.
- session ID keygen: SHA256("keygen|<height>|<minSigners>|<sortedParticipantsCsv>").
- session ID keysign: SHA256(vaultPubKey || message).

## Keyshare (StoredShare) JSON
version=1, engine="frost", participant, participants[], participant_index(u16),
min_signers(u16), max_signers(u16), public_key_compressed(hex 33B),
key_package(base64), public_key_package(base64).

## Lifecycle
- keygen: all members; 2 comm rounds; threshold=all-join.
- keysign: selected online subset; threshold ceil(2/3*n); explicit leader/initiator;
  buffer round2 shares in pending_shares until all round1 commitments collected;
  tweaks: taproot_key_path, merkle_root, child_tweak (additive scalar/point).
- retryable errors: "missing round2 secret", "out-of-order frost message".
- ErrLocalPartyNotSelected when local not in selected set.

## Interop invariants
JSON field names exactly as above (snake_case); message kinds lowercase_underscore;
base64 standard RFC4648; participants normalized+deduped+lexicographically sorted;
signature output 64-byte BIP340 Schnorr; constants postJoinSync=0, LengthHeader=4.
