# Rust Bifrost — status

GOAL MET (2026-07-02): the **full** Rust-ported Bifrost — observe AND sign —
(1) pressure-tested on a standalone 4-node regtest cluster, then (2) deployed
**in place on the live thornado-e2e validators (nodes 5–9)**, replacing the Go
bifrost, reusing each node's existing FROST keyshare and cosmos key. The live
chain now observes BTC deposits, FROST-signs refunds/outbounds across the real
Go-DKG'd vault, and settles them — all on Rust. See "In-place live upgrade"
and "Pressure test results" below.

## In-place live upgrade (nodes 5–9, thornado-e2e)

- Each Rust daemon reuses the node's EXISTING FROST keyshare
  (`.../bifrostN/localstate-<vault>.json`, whose base64 `local_data` field is
  the Rust `StoredShare` verbatim), cosmos key (`thornado keys export
  validatorN --unsafe --unarmored-hex`, answer the `y` confirm), and node
  pubkey. Active vault = 4-of participants, 3-of-4 threshold; members are
  v5/v7/v8/v9, v6 is standby (observer-only, no keyshare).
- Shared regtest bitcoind on the coordinator (5.223.51.101:24645, per-node
  wallet `bifrostN`); daemons talk to their local node's API/RPC and post
  observations with the node's own cosmos key.
- Under `/root/rust-bifrost-live/` on each node; systemd unit
  `rust-bifrost-live`. Rollback: `systemctl stop rust-bifrost-live` then
  re-exec the Go bifrost with the saved `go-bifrost.env` (kept by the swap
  script). The Go binary is untouched.
- **Verified live**: a 0.4 BTC deposit was observed by all 4 members
  (consensus, finalised), its refund FROST-signed by the live vault party and
  broadcast to bitcoind, the outbound observed with the correct recipient and
  **matched by the chain** (txout item → `complete`), leaving BTC signing
  un-halted and the queue drained.
- Five interop bugs were found and fixed by running against the real chain
  (all committed): bech32 pubkey/account decoding; tolerating Go's `null`
  slices; **batch-outbound observation** (vault-sent txs must be observed as
  the recipient output, not the change, or the chain halts BTC as an
  unmatched outbound); **pending-work discovery** via `/thornado/txout/all`
  (the chain holds unsigned batches at their original height, which a linear
  height-walk misses); and **input-based batching** (the chain's batch matcher
  requires every batched item to share identical `source_inputs`, so
  independent refunds — each spending its own deposit UTXO — must sign as
  separate single-item txs, not one union tx). Plus the safety rule: when
  prescribed inputs are already spent, DEFER — never re-sign with fresh UTXOs
  (double-pays and re-halts). A `--allow-respend-spent` flag (default off)
  exists only to drain a batch poisoned by a buggy prior signer.
- Un-halt after investigating: vote the config keys from each active
  validator — `thornado tx thornado config HaltSigningBTC 0` and
  `Halt_SolvencyCheck 0` (operational config, needs 2/3 node votes; run
  sequentially to avoid sequence races).

## What runs where (cluster, hcloud regtest — NO real funds)

4 churned-out validator hosts repurposed as the Rust fleet (the LIVE chain
validators on nodes 5/6/e2e/blockscan were never touched):

| name  | host          | units |
|-------|---------------|-------|
| rust1 | 5.223.55.114  | rust-btc (bitcoind :24700 rpc, :24701 p2p public), rust-harness (mock-thornado :13170 REST / :26670 RPC), rust-bifrost-daemon |
| rust2 | 5.223.55.174  | rust-btc (connect=node1), rust-bifrost-daemon |
| rust3 | 5.223.52.254  | rust-btc (connect=node1), rust-bifrost-daemon |
| rust4 | 5.223.93.218  | rust-btc (connect=node1), rust-bifrost-daemon |

- bitcoind creds `rusttest`/`rustpass9199`, datadir `/tmp/rust-btc`; node1 has
  wallets `miner` (funds/mining) and `vaultwatch` (watch-only, vault address).
- Everything under `/root/rust-bifrost/` (trimmed one-crate workspace; build
  ON the node: `cargo build --release --features harness`).
- FROST vault: 4-party DKG over WAN, group key
  `03ee7f4c68c5c91b1433708d62a73d102f67a19b3aad2ee4b18088af01a90ad60c`
  (3-of-4). Keyshares at `/root/rust-bifrost/keyshare.json` per node.
- Stats live at `http://5.223.55.114:13170/stats`.
- Tear-down: `systemctl stop rust-bifrost-daemon rust-harness rust-btc` per
  node (harness only on node1).

## Architecture that shipped (crate `crates/thornado-bifrost-signer`)

Daemon (`bifrost-signer run`) — both halves wired in `main.rs`:

- **Observe loop**: bitcoind scan → reorg detect/rescan → extract →
  temporal persistence → posts `MsgObservedTxIn` (vault is receiver) and
  `MsgObservedTxOut` (vault is sender) via SIGN_MODE_DIRECT
  (`cosmos_tx::build_and_sign_typed`).
- **Sign loop** (`sign_loop.rs`): per-height keysign fetch (signed payloads
  verified against `--node-pubkey`) → store queue → batch
  (`signer::batch_items`) → deterministic leader → join-party handshake over
  `/p2p/join-party-leader` (leader collects member streams, answers with the
  selected set; threshold ceil(2n/3)) → one taproot FROST session per input,
  driven concurrently by `transport::run_keysign_multi` (routes frames by
  session id — a single-session driver drops sibling-session frames) →
  witness assembly → broadcast (tolerates already-known / inputs-spent) →
  `mark_spent` + `out_hash`. Failures defer via
  `next_frost_signer_attempt_height` and retry.
- **UTXO sourcing** (`utxo.rs`): prescribed `source_inputs` win (the chain
  dictates inputs so every party builds the identical tx); live `listunspent`
  selection as fallback. Do NOT filter on `spendable` — watch-only wallets
  report false.
- **Mesh**: stable p2p identity file (`identity` subcommand prints the peer
  id), peer registry JSON, and a 10s conditional redial
  (`PeerCondition::Disconnected`) — without it `open_stream` fails with
  "no addresses for peer" after startup races or drops.
- **Keygen** (`bifrost-signer keygen`): distributed FROST DKG over libp2p,
  writes the `StoredShare` JSON; 120s timeout.

Test harness (`mock-thornado`, `--features harness`): stands in for a
thornado validator — serves `/thornado/lastblock` + ECDSA-signed
`/thornado/keysign/{h}/{pk}` (exact-height serving; wall-clock heights so
restarts never rewind below daemon cursors; never-served batches requeue),
`/cosmos/auth/...accounts/{addr}`, and a CometBFT `broadcast_tx_sync` that
**decodes the posted TxRaw and verifies the SIGN_MODE_DIRECT signature and
account sequence of every observation**. It also generates withdrawal work
with prescribed inputs, drives deposits + mining + forced depth-2 reorgs, and
tracks completion (all prescribed inputs spent on-chain) at `/stats`.
Daemons use hex(cosmos pubkey) as their auth-account key by default.

## Pressure test results (final run, 2026-07-02, 62+ min sustained)

Local mini-cluster (4 daemons + harness + regtest bitcoind on one Mac)
validated the full path end-to-end before deploying.

Cluster, 4 nodes over WAN, serialized per-vault batching (the production
shape), deposits every 3s, 5-output withdrawal batches every ~17s, blocks
every 3s, forced depth-2 reorg every 2 min:

- **215/216 batches completed** (last in flight) — 1,075 withdrawal outputs
  FROST-signed (3-of-4, all 4 participating) and accepted by bitcoind;
  completion avg **17s**, max 18s, **0 requeues**
- **3,896 observation posts, every one SIGN_MODE_DIRECT
  signature-verified: 0 bad signatures, 0 bad sequences, 0 undecodable**
- 1,190/1,190 inbound txids observed at full 4/4 quorum; 215/215 outbound
  at quorum
- 1,250 deposits, 1,250 blocks, **31 forced reorgs — all absorbed**
- RSS byte-flat at ~3.9MB per daemon for the whole run; **0 warnings,
  0 errors, 0 panics, 0 restarts on all 4 nodes**

Two liveness bugs were found and fixed by the earlier pressure iterations
(both committed):
- **Party-formation convoy**: deferral-timing skew diverged the nodes'
  queue heads so no leader ever attempted the batch its members wanted →
  fixed by demand-driven leading (session ids carry the batch identity;
  a leader prioritizes batches members ask it to lead) plus short party
  waits relative to the signing period.
- **Stale-work starvation**: requeued batches left immortal Available
  copies in every store that starved fresh work → fixed by retiring items
  past the chain's `retry_until_height` and finishing batches whose
  prescribed inputs are already spent on-chain.

## Known limits / next steps

1. Cosmos key loading is raw hex via flag/env — production wants keyring
   integration.
2. Party-formation retry is period-based (`--signing-period`); a stuck leader
   simply defers to the next period. Fine under test; consider leader
   rotation on repeated failure (Go rotates via attempt counter in the
   leader hash — epoch/height inputs already exist).
3. Mixed Go↔Rust FROST party interop is wire-compatible by construction
   (byte-for-byte envelopes, tested vs golden vectors + live libp2p tests)
   but a joint Go+Rust signing ceremony has not been run against the live
   chain's Go mesh.
4. `migrate`/`consolidate` (spend_all) outbounds are not exercised by the
   harness.
