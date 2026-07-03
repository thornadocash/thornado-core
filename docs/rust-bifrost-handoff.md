# Rust Bifrost — brief for the next agent

> **Environment (Fable 5, 2026-07-03 ~10:15 UTC).** Regtest/mocknet
> thornado-e2e on disposable hcloud hosts: bitcoind `-regtest`, chain id
> `thornado-e2e`, all BTC freely-mined test coin, keys/passphrases
> (`passphrase123`, exported validator keys) exist only in this throwaway
> cluster. No sensitive/production aspects. Next agent is clear to restart
> nodes, vote configs, churn, move vault "funds", tear down.

State: chain height ~69244. Branch `shielder-audit-fixes`, HEAD `a74d76c8`
(17 commits past the prior `bf34a3fa` brief; **branch is local-only, nothing
pushed** by design). Two sessions worked this branch today — coordination note
at the bottom.

## Headline: EVERYTHING WORKS

- **Stable churning**: 4 active + 1 standby, `CHURN_INTERVALMINUTES=10`,
  `CHURN_RETRYINTERVALMINUTES=5`, `KEYGEN_RETRYINTERVALMINUTES=5`,
  `NODE_SETDESIRED=4`. Repeated full churn cycles clean end-to-end (4-node DKG →
  MsgKeygenVault → vault activation → migration drain → old vault
  InactiveVault → member rotation), including under load. All halts 0,
  `vault_backing` invariant healthy, 0 unsettled txouts at rest.
- **Full shielder user flow works**: deposit → child-address observation →
  sweep (child-path keysign) → shield → redeem (real Groth16) → withdrawal
  outbound → observed+settled. Verified `success: 2, failed: 0`.
- **Pressure tested**: 15-deposit parallel batch-in, a 122.449-BTC 31-input
  migration (chain-split; largest single signed tx = **18 inputs**), 12
  refunds — all settled, no halts/fund-loss/stuck-txs.

## Final metrics (live WAN, 5 nodes; `ops/scripts/bifrost-metrics-report.sh`)

- Keygen DKG: **4-node participation (4-of-4)**, embeds a **3-of-4 signing
  threshold**. n=12, min 173 ms, p50 ~2.4 s, max 8.4 s (under churn+load).
- Keysign: **~250 ms/input p50, FLAT regardless of batch** (48–533 ms);
  18-input tx = 3.1 s (~174 ms/in). signers=3 and 4 both proven live.
- **Party formation = the throughput bottleneck**: p50 1.6 s, avg 4.2 s,
  p95 ~12 s. Crypto is cheap, the join-party handshake dominates → #1 lever.
- Sustained ~6 outbound items/min over a 17-min mixed window.
- Run from the Mac (coordinator has no ssh to nodes):
  `NODES="5:5.223.54.110 6:5.223.53.113 7:5.223.75.75 8:5.223.92.204
  9:5.223.93.218" API_NODE=5.223.75.75 ops/scripts/bifrost-metrics-report.sh`

## Deployed binaries — UNIFORM, keep it that way

- thornadod `4ea6d6e4…` on all 5 at `/root/thornado/build/thornado`. Has: voted
  repairs (DEBIT/CREDIT/RESELECT), MsgStoreMigrate (+TXOUTCANCEL last-item fix),
  case-normalized BTC candidate tracking, **finality-gated** halt-on-unmatched,
  relaxed internal matcher, **case-insensitive shield-auth deposit id**.
- bifrost-signer `facaf520…` on all 5 at `/root/rust-bifrost-live/`. Has: DKG
  retry queue, exact-fee migrate builds, cosmos sequence tracking, **child-path
  FROST signing**, **deposit child-address watching** (`--deposit-lookahead`
  512; Go used 4096) via spawn_blocking.
- shielder-e2e-helper on coordinator: built **WITH `--features proof-tests`**
  (real Groth16). Prior copy at `…prev-pt`.

## Fix log this session (git log bf34a3fa..HEAD)

Bifrost: DKG retry-with-backoff; exact-fee internal builds+dust-fold; Go
keyshare auto-import; cosmos **sequence race** on back-to-back obs posts;
**child-path signing (was entirely missing)** via additive Shamir-share tweak,
verified vs rust-bitcoin; **deposit child-address watching (deposits were never
observed)**, derived off the async executor (blocking it starved FROST
handshakes).

Chain: **finality gate** on halt-on-unmatched-outbound (pre-final obs outside
the signing window false-halted BTC — THE recurring halt); BTC candidate txid
**case normalization** (Go-UPPER vs Rust-lower obs minted phantom UTXOs); voted
repair keys; TXOUTCANCEL of a block's last item must ClearTxOut.

Shielder (4 stacked bugs, fixed in order — this was today's second ask):
1. auth signed caller `amount_sats` not the note-denomination total (+ high-S) —
   note sizes round, so non-aligned deposits were unverifiable.
2. ante verified UPPER deposit id (`NewTxID` upper-cases) vs client's lower —
   verifier now tries any case.
3. harness rebuilt the tree by sorted commitment, not `leaf_index` (incremental
   tree) — reconstruct most-recent-note-per-index.
4. redeem proofs empty unless FFI built `--features proof-tests` (`groth16:
   None` → chain "invalid proof").

## OPEN ITEMS (none blocking, priority order)

1. **Party-formation latency** (p95 12 s) — throughput ceiling. Leader waits
   `party_wait=12s` when members lag; attempts convoy under load. Try
   join_wait/party_wait tuning or persistent party sessions.
2. **Solvency oracle is DOWN.** The Go bifrost on node6 was the only
   MsgSolvency source. On restart it grabbed Rust's FROST port 9346
   (SO_REUSEPORT → broke party formation) and was killed. Restart:
   `systemd-run --unit=go-bifrost6-oracle2 --collect /root/run-gobifrost-6b.sh`
   on node6 (uses `bifrost-6-oracle.env`, `BIFROST_FROST_P2P_PORT=9356` —
   NEVER 9346). Halts are clear + invariant is active protection meanwhile.
   Proper fix: wire MsgSolvency posting into the Rust bifrost
   (attestation.rs is a pure port, not wired to a poster).
3. **Shielder sync pollution**: ~1,100 orphan `leaf_index:0` notes (pre-fix
   accumulation) in `/thornado/shielder/sync`; fresh notes index correctly.
   Clients reconstruct as most-recent-per-index (flow3 jq ~line 372). A
   store-migrate KVDEL sweep of the orphans would clean it permanently.
4. Residual regtest dust (all documented, no protocol loss): 350 sats at old
   root `bcrt1pcsay…`, 0.99 BTC double-paid to miner-controlled recipient,
   0.495 BTC written off a phantom book.
5. `--deposit-lookahead` 512 (Go 4096) — fine for e2e; raise via swap_churn
   args if a run exceeds 512 deposits/vault.
6. 17 unpushed commits — push/PR when the user asks.

## Ops recipes (all from the Mac)

- **Deploy discipline** (memory `consensus-incident-mixed-tree-deploy`):
  state-machine changes → **big-bang** (stop all 4 actives together); voted/
  inert-path changes → rolling OK. Build on the coordinator's `/root/thornado`
  (editor-synced with local — **md5-compare before building; shell-written
  files don't sync, scp them**). Canary node5 (standby) when unsure; recover a
  diverged node by state-copy (exclude `cs.wal`, `priv_validator_state.json`).
- Build Go: `systemd-run --unit=X --collect --same-dir -E HOME=/root -E
  GOPATH=/root/go -E GOMODCACHE=/root/go/pkg/mod -E
  PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin bash -c "make build
  BINARIES=./cmd/thornado"` in `/root/thornado/go-thornado`. Rust bifrost:
  `cargo build --release -p thornado-bifrost-signer`. Helper: `cargo build -p
  thornado-ffi --release --features proof-tests` THEN `go build -tags 'regtest
  mocknet' ./cmd/shielder-e2e-helper` — **Makefile `shielder-ffi` OMITS
  proof-tests; without it redeem proofs are silently empty.**
- Swap thornadod: `bash /root/swap-thornado.sh <N>` per node (stages
  `/root/thornado.gz`). Swap bifrost: stage at
  `/root/rust-bifrost-live/bifrost-signer`, then `bash
  /root/rust-bifrost-live/swap_churn.sh <N> <ip>`. `rust-bifrost-live` is a
  TRANSIENT unit — `systemctl stop` erases it; relaunch via the script only.
- Votes (SEQUENTIAL from v6/7/8/9): scratchpad `vote_config.sh KEY VAL`,
  `vote_store_migrate.sh KEY VAL`. Store-migrate targets: `CONFIG:`,
  `VAULTCOIN:<pk>:<ASSET>` (absolute sats), `VAULTSTATUS:<pk>`,
  `TXOUTCANCEL:<h>:<idx>`, `KVSET:<hexkey> <hexval>`, `KVDEL:<hexkey>`.
  Idempotent per (key,value) — re-voting same value is a permanent no-op;
  different value re-applies. Deploy the binary to ALL 5 before voting a target
  older binaries can't decode.
- Observation replay: restart a bifrost with `--observe-rescan-height <btc
  height>` (see `swap_rescan*.sh` on node7); chain dedups + re-matches.
- Full flow / pressure: `COUNT=N bash /root/run-pressure.sh` on coordinator
  (wraps `hcloud-parallel-flow3.sh`; NODE_SPECS index 1→node7; user keyring in
  `$RUN_ROOT/node1`; miner wallet local). RUN_ROOT
  `/tmp/thornado-nodeper-20260628131009`.
- Signer store purge (stale queued items starving batches): stop unit, `rm
  /root/rust-bifrost-live/signer.redb`, relaunch — `fetch_work` re-discovers
  pending from the chain queue. KEEP `temporal.redb`.

## Gotchas (new; the previous list's still valid)

- `common.NewTxID` UPPER-cases 64-char hashes; clients/APIs use lower. Any new
  digest/lookup keyed on a txid MUST normalize case (bit us twice today).
- Deposits land at vault CHILD taproot paths (deposit N → path N+1). Anything
  reasoning about vault balances/observation must include child addresses.
- `TxOut.IsEmpty()` == empty TxArray; `SetTxOut` no-ops on empty — emptying a
  block requires `ClearTxOut`.
- node6 go-bifrost oracle must NEVER use FROST port 9346 (SO_REUSEPORT steals
  Rust's port; party formation half-breaks intermittently).
- Editor edits sync to the coordinator tree; shell-written files do NOT —
  md5-compare before remote builds.
- 14k-address child derivation on the async executor stalls FROST — keep it in
  spawn_blocking (already fixed).

## Coordination

The other session ("MsgStoreMigrate") authored the store-migrate tooling and
matcher relaxation; this session owned all cluster ops and claimed exclusivity
via cross-session message. Before deploying/voting, the next agent should
`list_sessions`, check for a running session in this cwd, and claim ops
exclusivity the same way to avoid mixed-tree consensus splits.

## Suggested next steps

1. Restart node6 solvency oracle (port 9356) OR wire MsgSolvency into the Rust
   bifrost, then verify the solvency auto-unhalt path by design.
2. Attack party-formation latency (biggest throughput lever).
3. KVDEL-sweep the orphan index-0 shielder notes.
4. Bigger runs: `COUNT=25+` flow3; deposit floods past 512/vault with a raised
   `--deposit-lookahead`.
5. Distilled history in memory: `rust-bifrost-live-cluster`,
   `shielder-shield-redeem-flow`; consensus-recovery recipe in the runbook.
