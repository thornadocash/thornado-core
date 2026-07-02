# Rust Bifrost — handoff to next agent

Goal (not yet met): pressure-test the **full** Rust-ported Bifrost on the hcloud
regtest cluster — including the **signing path**, not just observe. The observe
half passed a real pressure test (600+ txs, 30+ blocks, a reorg, ~14MB flat RSS,
0 panics). The signing half is **not wired into the daemon**, so it could not be
tested. That is the remaining work.

## What is DONE and verified
- Rust crate `crates/thornado-bifrost-signer` (~20 modules, ~130 tests incl.
  conformance vs rust-bitcoin, 2 live libp2p tests). Builds clean. Branch
  `shielder-audit-fixes`, committed through the cosmos-tx posting work.
- All bifrost subsystems exist as tested modules: frost_session, transport,
  tx_builder, chain, bitcoind, temporal, scanner, observer, extract,
  attestation, broadcast, cosmos_tx (SIGN_MODE_DIRECT), daemon (observe loop).
- Cluster observe-path pressure test PASSED (see memory `bifrost-rust-conformance`).

## What is NOT done (the goal's blockers)
1. **Sign-loop composition in `src/main.rs`.** The keysign loop currently only
   fetches keysign work into the store. It does NOT: batch (signer::batch_items)
   → resolve leader (signer::frost_party_leader) → join party (p2p::Coordinator)
   → run FROST keysign over transport::Libp2pMailbox → build+sign the BTC tx
   (tx_builder) → broadcast (broadcast::broadcast_btc_tx) → post the outbound
   observation. Every piece is built and unit-tested; they are not strung
   together in the daemon.
2. **UTXO sourcing at runtime** — bitcoind::list_unspent + signer::sort_utxos +
   filter (getUtxoToSpend logic) to feed tx_builder::BuildRequest. Ordering is
   ported/conformance-tested; the runtime fetch+filter loop isn't wired.
3. **Multi-node FROST on the cluster** — needs ≥2 Rust bifrosts (or Rust↔Go
   mixed) with a shared peer registry to actually run a keysign party.
4. **Observation posting exercised live** — cosmos_tx path is built + unit-tested
   but was run observe-only (no key) on the cluster; never posted to the chain.

## Cluster facts (hcloud, regtest, NO real funds)
- Auth: `HCLOUD_TOKEN` in `.env`. `hcloud server list` works.
- SSH `root@<ip>`; if host-key changed (IP reuse), `ssh-keygen -R <ip>` then
  `-o StrictHostKeyChecking=accept-new`.
- **Nodes 1–4 were churned OUT** (dead thornado/bifrost expected). **Active
  validators: nodes 5,6,e2e(node7),blockscan,+**. node1 (5.223.55.114) is the
  spare build/test host used here.
- Active validator example — node5 (5.223.54.110): thornado API `:2375`,
  CometBFT RPC `:33365` (live, height ~54k), bifrost FROST p2p `9345`,
  CHAIN_ID `thornado-e2e`, bitcoind `127.0.0.1:24645` (user/pass thornado/thornado,
  wallet `bifrost5`). Live bifrost env is the source of truth:
  `tr '\0' '\n' < /proc/$(pgrep -f build/bifrost)/environ`.
- FROST bootstrap-peer multiaddr list is in that env
  (`BIFROST_FROST_BOOTSTRAP_PEERS`) — reuse for multi-node.
- Real bitcoind/cli: `/opt/bitcoin-27.2/bin/`.

## What's left running on node1 (5.223.55.114) — clean up or reuse
- systemd unit `rust-bifrost` (observe-only), log `/root/rust-bifrost/observe.log`,
  binary `/root/rust-bifrost/target/release/bifrost-signer`, source under
  `/root/rust-bifrost/`. Stop: `systemctl stop rust-bifrost`.
- Isolated regtest bitcoind for load: datadir `/tmp/rust-test-btc`, RPC
  `127.0.0.1:24700` user/pass test/test. Vault addr in
  `/root/rust-bifrost/vault_addr`, miner in `/root/rust-bifrost/miner_addr`.
  Stop: `pkill -f datadir=/tmp/rust-test-btc`.

## Build notes (cost real time to discover)
- Build ON a node (x86_64); cross-compile from darwin/arm64 is painful.
- Use **rustls**, not openssl: crate Cargo.toml already sets
  `reqwest = { default-features = false, features = ["json","rustls-tls"] }`.
  Needs `build-essential` (cc) for ring/secp256k1.
- Launch via **systemd-run / systemd unit** — bare nohup/setsid die on SSH
  teardown.

## Suggested path to the goal
1. Wire the sign loop in main.rs (item 1) + UTXO sourcing (item 2).
2. Bring up 2–3 Rust bifrosts on spare nodes sharing a peer registry (item 3),
   OR interop one Rust bifrost into the live Go FROST mesh via the bootstrap
   peers (higher value, higher risk).
3. Configure a cosmos signing key for a test validator account and let it post
   (item 4).
4. Pressure test: drive inbound deposits AND trigger outbound withdrawals so the
   sign→broadcast→post path runs under load; watch for crashes, memory, keep-up,
   and correct signatures accepted by thornado.
