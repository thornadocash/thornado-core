# Rust Bifrost — brief for the next agent

> **Environment (Fable 5, updated 2026-07-03 ~15:20 UTC).** Regtest/mocknet
> thornado-e2e on disposable hcloud hosts: bitcoind `-regtest`, chain id
> `thornado-e2e`, all BTC freely-mined test coin, keys/passphrases
> (`passphrase123`, exported validator keys) exist only in this throwaway
> cluster. No sensitive/production aspects. Next agent is clear to restart
> nodes, vote configs, churn, move vault "funds", tear down.

State: chain height ~72700. Branch `shielder-audit-fixes`, HEAD `f8f83366`
(**branch is local-only, nothing pushed** by design). The four open items of
the previous brief were all closed this session (see the 15:20 fix log); two
task chips were filed for the remaining design work.

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

- thornadod `291d6ca7…` on all 5 at `/root/thornado/build/thornado`. Adds over
  the 10:15 brief: SHIELDERNOTESWEEP + SHIELDERPURGE voted targets, **BTC batch
  cap 9 recipients/tx** (btcMaxBatchRecipients).
- bifrost-signer `68d45ab4…`-lineage on all 5 at `/root/rust-bifrost-live/`
  (relaunched via swap_churn.sh with `--deposit-lookahead 1024` baked in).
  Adds: **MsgSolvency posting** (even-height, scantxoutset), **threshold+grace
  party formation**, `--max-value-outputs` recovery knob.
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

## 15:20 SESSION — the previous brief's 4 open items, all closed

1. **Solvency: properly wired into the Rust signer** (fd59b521, 33f9cd5d,
   846135ba). The single Go oracle could NEVER work — SolvencyVoter needs a
   2/3 supermajority of actives; one attester is structurally insufficient.
   Each active now posts MsgSolvency per funded base vault at EVEN BTC heights
   (the id hashes the height; self-relative strides never converge), balance
   from `scantxoutset` over root+child addr() descriptors (wallet listunspent
   returns 0 on nodes without imports — three zero votes consensus-halted BTC
   once; live lesson). Id is byte-exact vs Go common.Solvency.Hash (UPPER hex,
   golden-vector tested). 4-of-4 vote convergence verified; auto-unhalt path
   verified live repeatedly. The node6 Go oracle is RETIRED (its bootstrap
   peers are stale Go peer-ids anyway; killed by swap_churn.sh's pgrep).
2. **Party formation p95 12s → 3s** (0f388a75, 802b5ede). Leader forms the
   party at THRESHOLD after a straggler grace (3s default) instead of waiting
   the full 12s for the 4th member. Fleet under load: n=93, p50 83ms, p95
   3.0s, max 5.9s. Knobs: --party-wait-secs/--party-grace-secs/
   --join-wait-secs. DO NOT set join-wait <20s: it doubles as the leader's
   parked-join TTL and starves demand-driven leading under parallel load.
3. **Orphan notes purged** via new voted store-migrate targets:
   SHIELDERNOTESWEEP:<denom> (1b3ea832, 261e204e; 2,646 records inc. the
   legacy bool entries that broke GetShielderDenominationCommitments) — BUT
   the note records double as the invariant's mint ledger, so the sweep left
   spent(8.56B) > minted → the ENFORCED vault_backing invariant halted BTC
   every block (tug-of-war with solvency auto-unhalt; churn frozen ~2.5h).
   Resolved with SHIELDERPURGE (f8f83366) — voted PurgeShielderPoolState:
   wipes mint AND spend ledgers + historical roots together (no stale root
   can re-prove an erased nullifier). Shielder pool is now EMPTY; next flow
   re-mints from scratch. Invariant healthy, churn resumed, clean churn
   verified end-to-end after.
4. **Pressure + past-512 flood done.** COUNT=25 full flow ran; its 25-output
   withdrawal exposed THE bug of the day: the chain batcher had NO output
   cap while BOTH observers ignore >10-output txs — the tx CONFIRMED on BTC
   but could never be observed/matched (funds moved, items unsigned forever,
   record undebited). Chain now caps batches at 9 recipients (2f832a6d,
   big-bang deployed); recovery used the new --max-value-outputs (5c7b3039)
   raised fleet-wide + --observe-rescan-height to observe tx c0041d38.
   Deposit flood: 530 request-deposits in 3.5 min (zero errors), deposit at
   child path 580 observed/matched/swept under --deposit-lookahead 1024 (now
   baked into swap_churn.sh on all nodes).

## OPEN ITEMS (priority order)

1. **Churn-window solvency flap** (task chip filed): each migration/deposit
   settlement window flips insolvent→halt→auto-unhalt for ~1 min because the
   wallet moves at BTC confirmation but the vault record at observation
   consensus; excludePendingOutboundFromVault misses migrations.
2. **Sweep/invariant ledger design** (task chip filed): make the mint ledger
   sweep-proof (cumulative counters) so future sweeps can't orphan spends.
3. **Party alignment decay — now the #1 reliability gap**: per-node
   sequential sign loops drift out of alignment over hours (stale parked
   streams, "failed to answer join request", keysign timeouts when a
   "selected" member never got the leader's answer). Proven consequence: the
   cluster churned cleanly unattended for ~6.5h (~40 complete migrations, one
   every ~100 blocks), then ONE migration keysign wedged for ~55 min — the
   retiring vault held all funds, the new vault sat empty, and "fail to
   migrate funds: no source inputs" spammed every block (the pending txout's
   input reservation blocks re-scheduling; that error means "migration
   already queued", not fund loss). Remedy (verified twice): fleet signer
   restart with signer.redb purge — the batch signed within 2 min. Real fix:
   persistent party sessions or batched child-path sweeps. NOTE for queue
   checks: /thornado/txout/all returns {"txout": current, "txouts": [...]}
   — parse the "txouts" key.
4. Residual regtest dust (unchanged, documented, no protocol loss).
5. 26 unpushed commits — push/PR when the user asks.

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
