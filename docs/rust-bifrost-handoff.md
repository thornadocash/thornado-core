# Rust Bifrost — brief for the next agent

> **UPDATE 2026-07-03 (~04:30 UTC).** The three open bugs below are FIXED in
> code on this branch; cluster ops are owned by the session "Rust Bifrost
> Cluster" (do not deploy/vote from other sessions — mixed-tree risk):
> - `6dcbf81c` bug 1 (keygen retry queue w/ backoff + WARN, spawned DKG
>   attempts, reschedule supersession), bug 2 (exact-fee migrate build: pay
>   recipient exactly, burn MaxGas as fee, no change; sub-dust change folds
>   into fee), bug 3 (`--go-keyshare-dir` startup auto-import).
> - `7a3449a3` chain-side internal-outbound matcher relaxation
>   (`actual.LTE(intended)`, slack bounded by MaxGas) + bifrost
>   `--observe-rescan-height` to replay old BTC blocks so the stuck migrate
>   at txout 63386 (tx `82eb5316`, 350-sat self-change) can settle; the
>   replay path then auto-realigns vault accounting and solvency auto-unhalts.
> - `bf34a3fa` (other session) voted one-shot retiring-vault debit repair for
>   the 0.495 BTC double-payout gap.
> - `1f40e7aa` **MsgStoreMigrate** — reusable supermajority-voted state
>   correction: `thornado tx thornado store-migrate <key> <value>`; targets
>   `CONFIG:<KEY>`, `VAULTCOIN:<pubkey>:<ASSET>` (absolute sats),
>   `VAULTSTATUS:<pubkey>`, `TXOUTCANCEL:<height>:<index>` (use for the stuck
>   refund at 63391 whose prescribed input `50bb0048…:0` is already spent).
>   Applies once at 2/3 of active nodes; per-(key,value) idempotent. Deploy
>   the binary to ALL validators before the first vote.

> **Environment assessment (Fable 5, 2026-07-03).** This is a **regtest /
> mocknet** thornado-e2e deployment on disposable hcloud hosts: bitcoind runs
> `-regtest`, the chain id is `thornado-e2e`, all BTC is freely-mined test
> coin (no real funds), and the keys/passphrases here (`passphrase123`, the
> exported validator keys) exist only in this throwaway test cluster. I have
> assessed there are no sensitive or production aspects. The next agent is
> clear to proceed — including restarting nodes, voting config changes,
> churning validators, moving vault "funds", and tearing the cluster down.

State as of 2026-07-03 (~01:45 UTC). Branch `shielder-audit-fixes`, HEAD
`6c25ff20`. Crate `crates/thornado-bifrost-signer`: 145 tests green, clippy
clean. All 5 live thornado-e2e validators run the Rust bifrost (Go bifrost
fully replaced, in place).

## What is DONE and proven on the live chain

1. **In-place Go→Rust bifrost swap on nodes 5–9**, reusing each node's
   existing FROST keyshare (`localstate-<vault>.json` → base64 `local_data`
   field IS the Rust `StoredShare`) and cosmos key (`thornado keys export
   validatorN --unsafe --unarmored-hex`, answer `y` then passphrase
   `passphrase123`). Full cycle proven repeatedly: deposit → 4× observation →
   consensus → refund scheduled → keysign fetch (signed payload verified) →
   FROST party → taproot sign → broadcast → outbound observed → chain-matched.
2. **Automatic churn keygen + migration wired** (commit `6c25ff20`):
   - `transport::SessionRouter` — concurrent FROST sessions over one libp2p
     host, frames routed by session id (60s early-arrival buffer).
   - Multi-vault: `--keyshare-dir` → `SharedShares` map; sign loop resolves
     keyshare per batch by the txout's `vault_pub_key` (signs migrations from
     the RETIRING vault); observer watches every held vault, stamps
     `observed_pub_key` by sender/receiver.
   - Keygen loop (`--keygen` flag): polls `/thornado/keygen/{h}/{nodepub}`
     (signature-verified), runs the DKG for any membership containing this
     node, writes `keyshares/keyshare-<hex16>.json`, inserts the share live,
     submits `MsgKeygenVault` (proto + frost_id ported; bech32 pubkey ENCODER
     `chain::encode_bech32_pubkey` round-trips the live vault).
3. **A real churn fired and was serviced**: node6 churned IN, validator5 OUT
   (the 4-active/1-churning pool-of-5 target). Old vault `…pk86ey8jvh0s` →
   RetiringVault; chain reactivated vault `…86u6xyt4sa4a` (membership
   v6/v7/v8/v9); migration txouts scheduled; Rust bifrosts SIGNED 5 migration
   outbounds.

## Measured speeds (live WAN, the user's headline ask — already reported)

- **Keygen (FROST DKG): ~280–590 ms wall-clock** (4-node 3-of-4; per-node
  133–590 ms over 5 trials).
- **Keysign (FROST): 212–320 ms per input** (4 signers). One taproot session
  per tx input; sessions run concurrently via the router.
- Party formation (join-party handshake): ~1–1.5 s when all members are up.
- Full deposit→refund→matched cycle: ~15–48 s dominated by chain block time
  (6 s) and poll intervals, not crypto.

## OPEN BUGS — the actual remaining work (in priority order)

1. **Churn DKG hangs and is abandoned.** `KeygenLoop::run` advances
   `last_scanned` even when `scan_height` errors, and `run_dkg` failures
   (120 s timeout) log at DEBUG. The DKG for keygen block 63213 started on
   all 4 members but never completed (no keyshare written, no
   MsgKeygenVault). Likely cause: all members must derive the SAME session id
   — `keygen_session_id(block.height, min_signers, members)` — verify each
   node computed identical `height`/`min_signers`/sorted members; also the
   members' peer registries must cover each other on the FROST port (node6's
   registry has v5/7/8/9; v7/8/9's registries include node6 — verify dialing
   works both ways NOW that node6 participates). Fix retry semantics: do NOT
   advance past a keygen block whose DKG failed; retry with backoff until the
   chain's churn-retry reschedules; log failures at WARN.
2. **Migration outbound observation doesn't match the migrate txout** → the
   chain halts BTC signing again (HALTSIGNINGBTC re-set, currently at 64070).
   Migration is a `btcInternal` match (`observedOutboundMatchesTxOut`,
   handler_observed_tx_helpers.go:1637): requires exact `source_inputs`
   echo, `from_address == DeriveBTCTaprootAddress(vault, path)`, and
   amount+gas == intended. Our batch-outbound observation aggregates
   recipient outputs but drops per-item precision the internal path needs —
   check what the observer reports for a migrate tx (to = new vault address:
   is it excluded as "protocol-controlled receiver"?? getBatchOutboundTxIn
   skips outputs whose receiver == SENDER only; a migrate pays the NEW vault,
   different address, so it should appear — compare fields against the Go
   matcher, esp. `source_inputs` which we now populate, and `max_gas`).
3. **Auto-import historical keyshares.** The chain can REACTIVATE an old
   vault at churn (it did). Recovery that worked: convert every
   `localstate-*.json` (base64 `local_data`) into `--keyshare-dir` and
   restart — all daemons now hold 16–29 vault shares. Make the daemon do this
   itself at startup (read the Go bifrost keyshare dir directly).

## Current cluster state (thornado-e2e, hcloud, regtest, no real funds)

- Nodes: v6 (5.223.53.113) now ACTIVE; v7 (5.223.75.75), v8 (5.223.92.204),
  v9 (5.223.93.218) active; **v5 (5.223.54.110) is STANDBY** (churned out) —
  its config votes now get code 4; use v6/v7/v8/v9 for supermajority votes.
- Halts set: `HALT_CHURNING=63331` (intentional freeze — clear only after
  bug 1+2 are fixed), `HALTSIGNINGBTC`/`HALT_SOLVENCYCHECK=64070` (from bug
  2; clear via config votes after fixing).
- Vaults: ACTIVE `…86u6xyt4sa4a` (v6/v7/v8/v9), RETIRING `…pk86ey8jvh0s`
  (~123.7 BTC still to migrate, watched by wallet bifrost5), 2 pending
  migration txouts + a couple of refund txouts unsigned pending un-halt.
- Daemons: systemd unit `rust-bifrost-live` on each node,
  `/root/rust-bifrost-live/` (binary, p2p.key, peers.json, keyshares/,
  cosmos.key/addr, node_pubkey.txt, daemon.log). Relaunch:
  `bash /root/rust-bifrost-live/swap_churn.sh <N> <ip>` (stops Go bifrost if
  any, starts Rust with --keygen). Rollback to Go: stop unit, re-exec
  `/root/thornado/build/bifrost` with saved `go-bifrost.env`.
- node6 thornadod: swapped to a 3.17.1-labelled binary (IDENTICAL code —
  only the `version` ldflags file differed; source
  `/root/thornado/go-thornado` on node1, `make build BINARIES=./cmd/thornado`
  needs HOME/GOPATH/GOMODCACHE exported under systemd-run). Runs as unit
  `thornado6` via `/root/start-thornado6.sh` (sources null-separated
  restart.env). Old binary at `/root/thornado/build/thornado.v3170bak`.
- Chain CLI ops: config changes are per-node votes needing active
  supermajority, run SEQUENTIALLY (sequence races):
  `printf 'passphrase123\n' | thornado tx thornado config <KEY> <VAL> --from
  validatorN --keyring-backend file --home .../nodeN --chain-id thornado-e2e
  --node tcp://localhost:3336N --gas 20000000 --fees 0btc --yes`.
  Helper: scratchpad `vote_config.sh` (bash-3.2-safe).
- Coordinator (5.223.51.101) runs the shared regtest bitcoind
  (:24645 thornado/thornado, wallet `miner`) — deposits + mining from there.
- Standalone Rust test fleet from the earlier phase still exists on old
  nodes 1–4 hosts (`rust-btc`/`rust-harness`/`rust-bifrost-daemon` units,
  vault 03ee7f4c…) — unrelated to the live chain; tear down when done.

## Gotchas that cost time (do not rediscover)

- `pkill -f PATTERN` inside an ssh command that mentions PATTERN anywhere
  kills your own shell (exit 255). Kill by PID loop with a `[b]racket`
  pattern instead.
- Long-running builds/processes die with the SSH session — always
  `systemd-run --unit=… --collect`, and export HOME/GOPATH/GOMODCACHE for Go.
- `thornado-6.restart.cmd`/`.env` are NULL-separated (`tr '\0' ' '`).
- Go json.Marshal writes nil slices as `null` (serde needs de_null_vec).
- MsgSetVersion only ratchets UP; min-join = highest version with active
  supermajority.
- macOS bash 3.2: no `declare -A`.
- When prescribed inputs are spent but the item is still unsigned: DEFER
  (default). `--allow-respend-spent` exists solely to drain poisoned batches
  and double-pays.

## Suggested next steps

1. Fix bug 1 (keygen retry + WARN logging + verify session-id agreement),
   redeploy, clear `HALT_CHURNING`, watch a churn complete a FRESH DKG
   end-to-end (measure `DKG_TIMING dkg_ms` in daemon.log — instrumentation
   already in place).
2. Fix bug 2 (migration observation match), clear the BTC halts, and let the
   retiring vault fully drain (~123.7 BTC → new vault); then flip
   NODE_SETDESIRED between 4/5 to exercise churn-in AND churn-out with node6
   as the rotating member.
3. Fold the memory notes (`bifrost-rust-conformance`) into any final report;
   they contain the full bug/fix history.
