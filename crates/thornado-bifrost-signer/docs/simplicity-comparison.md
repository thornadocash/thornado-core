# FROST-Signer-Bifrost: Rust vs Go simplicity comparison

## What was ported

A single Rust crate, `thornado-bifrost-signer`, replacing the Go FROST signing
path plus the Rust↔Go FFI boundary it sat behind:

| Rust module | Non-test LOC | Replaces (Go, non-test) | LOC |
|---|--:|---|--:|
| `frost_session.rs` | 495 | `frost/p2p_session.go` + `frost-ffi/src/lib.rs` (DKG+keysign engine) | 895 + 849 |
| `tx_builder.rs` | 245 | `btc/utxo_signer.go` + `btc/signer_internal.go` (build/sighash/witness) | 974 + 679 |
| `chain.rs` | 223 | signer subset of `thornadoclient/thornado.go` | ~600 of 1082 |
| `wire.rs` | 224 | stream framing + `join_party.pb.go` (generated) | ~200 + 672 |
| `p2p.rs` | 171 | `p2p/party_coordinator.go` (coordination logic) | 684 |
| `store.rs` | 210 | `signer/storage.go` | 398 |
| `signer.rs` | 86 | decision logic of `signer/sign.go` | ~600 of 1801 |
| `main.rs` + `lib.rs` | 102 | `cmd/bifrost` wiring | ~300 |

**Rust total: ~2,655 non-test LOC** (plus ~900 LOC of in-crate tests; 47 tests).
**Go + FFI surface it targets: ~6,400 LOC** of hand-written code across two
languages, plus ~1,300 LOC of generated protobuf.

The Rust figure grew from the initial core estimate as the chain client gained
byte-exact keysign signature verification + caching/retry, and the live async
transport (`transport.rs`) landed with an in-process DKG→keysign integration
test over real async streams. Net ratio is now ~2.4× fewer non-test lines, one
language instead of two, and no FFI.

## The honest scope line

The crate implements and **tests** the correctness-critical core end to end:

- Distributed DKG (`part1/2/3`) and threshold keysign, proven by a no-dealer
  multi-party roundtrip test.
- BIP341 taproot key-path sighash (0x81) and witness assembly.
- The party-coordinator threshold/leader state machine.
- Wire framing + protobuf codec (byte-compatible with Go peers).
- Store lifecycle + spent-UTXO anti-double-spend.
- Batch grouping, deterministic leader selection, retry-height math.

What is **not yet** fleshed out (and would add lines): the async event loop that
streams FROST rounds over live libp2p connections, broadcast/RPC posting, and
the full retry-persistence wiring. So the LOC ratio below reflects the core, not
a shipped daemon — the Go number will shrink less than 3.6× once the glue lands,
realistically landing around **2.5–3× fewer lines**.

## Where the real simplicity comes from

1. **The FFI boundary disappears.** Today: Go calls a C ABI (`gofrost_*`) into
   849 lines of Rust that hand-manage session handles in a global `Vec`, plus Go
   marshalling on the other side. In the port, the FROST engine is just a Rust
   module call — no handle table, no `GoFrostBuf`, no `unsafe`, no cross-language
   serialization of every round message. This is the single biggest win and it
   removes an entire class of audit findings (memory ownership across the fence).

2. **One serialization story.** `frost-secp256k1-tr` types serialize directly;
   Go had to base64-wrap every package to cross the ABI and again for the wire.

3. **`rust-bitcoin` owns the tx primitives.** `SighashCache` computes the BIP341
   digest; the Go path hand-rolls the taproot sighash byte-by-byte
   (`taprootKeySpendSigHash`). 245 lines replace ~1,650.

4. **The type system encodes the round state machine.** Illegal round
   transitions are unrepresentable; Go leans on runtime string-keyed checks.

5. **Tests need no dealer and no network.** The DKG→keysign proof and the
   coordinator logic run as fast in-process unit tests.

## Caveats worth keeping visible

- **Interop is by wire contract, not shared code.** Rust libp2p (0.54) and Go
  libp2p (0.48) never share a line; they must agree on protocol IDs, framing,
  and JSON/protobuf field names. These are now checked against **authoritative
  bytes emitted by the real Go types** (`go-thornado/cmd/interop-fixtures` →
  `test-fixtures/interop/`), decoded and re-encoded byte-identically in
  `wire.rs` tests. This caught two real bugs: the `WrappedMessage.payload` must
  be a base64 string (Go `[]byte` JSON encoding), not a byte array; and the
  keysign message type is `7` (`FROSTFrostKeySignMsg`), not the legacy `1`. A
  full live-peer handshake test still adds value but the encoding is now pinned.
- **DKG blame/abort round is ported.** `KeygenSession`/`SignSession` emit
  `keygen_abort`/`sign_abort` naming culprits (via `frost::Error::culprits()`
  for `part2`/`part3`/`aggregate`, and by sender for undecodable packages), and
  process inbound aborts into a typed `IdentifiableAbort`. Tested.
- **Peer discovery/dialing is wired.** `p2p::PeerRegistry` loads a JSON peer
  file (name → PeerId + multiaddr, see `docs/peers.example.json`); the daemon
  registers and dials peers at startup and routes inbound streams by
  participant name into a `Libp2pMailbox`.

## Live multi-node test (done)

`tests/live_multinode.rs` stands up **three real signer nodes over TCP**
(noise + yamux), discovers OS-assigned listen addresses, dials a full mesh (one
initiator per edge, no simultaneous-dial handshake corruption), runs a
distributed keygen, then a 2-of-3 threshold keysign — and verifies the aggregate
against the DKG group key. This exercises the entire libp2p path the in-process
transport test could not: `build_swarm` → listen → dial → `/p2p/frost` stream
negotiation → `Libp2pMailbox` → `run_keygen`/`run_keysign`.

Two production-relevant fixes fell out of getting it green: connections need a
non-zero idle timeout **and** an active ping keepalive (the stream behaviour
holds no idle substreams, so connections otherwise close between rounds), and
`Libp2pMailbox::send` now bounds and retries `open_stream` so a peer mid-
reconnect yields a retryable error instead of hanging the session.

It's marked `#[ignore]` so the default `cargo test` stays deterministic (real
sockets don't play well under the parallel test-binary run); run it explicitly:

    cargo test -p thornado-bifrost-signer --test live_multinode -- --ignored

## Remaining before production cutover

- The FFI crate (`thornado-frost-ffi`) can be deleted only once Go no longer
  calls it — i.e. after the whole signer path is cut over.
- Signing pipeline glue in `main.rs` (store → batch → party join → run_keysign
  → broadcast) is scaffolded; the decision logic and transport it composes are
  all implemented and tested.
- Interop against a live *Go* peer (this test is Rust↔Rust over the wire
  format; the codec is pinned against Go-emitted fixtures, but a Go+Rust mixed
  party is the final check before migration).

_Crate status: 56 unit/integration tests + 1 live multi-node test, all passing;
clippy-clean; ~3,000 non-test LOC._
