# Thornado Cluster Runbook

This is the canonical ops runbook for local and HCloud regtest clusters.
Older HCloud and flow runbooks are historical notes unless they are referenced here.

## Hard Rules

- Build Go binaries with `-tags 'regtest mocknet'`.
- Use `ops/scripts/thornado-cluster.sh` as the entrypoint.
- Use `ops/scripts/hcloud-deploy-binaries.sh` for HCloud deploys; do not broad-rsync and do not build on every worker.
- Do not delete or reset a running cluster unless the task is explicitly a teardown.
- Do not abandon queued transactions. A queued transaction is processed or there is a bug.
- Keygen requires 100% of the targeted keygen set: active vault members plus nodes churning in.
- Keysign requires 67% of the active node set.
- Churn is halted by config. Do not stretch churn intervals as a workaround.
- Major issues are stuck transactions, halted state, or lost-funds risk: fix and redeploy.
- Minor issues are noisy logs or optimization: patch locally and hold until the next major deploy.

## Current HCloud Topology

Coordinator:

```text
5.223.51.101
```

Workers:

```text
node1 5.223.55.114  genesis Thornado; Bifrost retired
node2 5.223.55.174  genesis, churned out
node3 5.223.52.254  genesis, churned out
node4 5.223.93.218  original node4 host, reused as node9
node5 5.223.54.110  active
node6 5.223.53.113  active
node7 5.223.75.75  active
node8 5.223.92.204  active
node9 5.223.93.218  standby
```

Inventory:

```bash
ops/distributed-regtest-nodeper.env
```

Default run root used by the active node-per-server cluster:

```bash
/tmp/thornado-nodeper-20260628131009
```

Current post-churn worker set:

```bash
WORKER_NODES="5 6 7 8 9"
```

Fresh bootstrap still starts from nodes 1-4 unless `WORKER_NODES` is explicitly set.

Server shape that worked: Hetzner `cpx32` or larger for every coordinator/worker.
Each worker runs one Thornado, one Bifrost, and one local `bitcoind`.
The coordinator runs build/artifact distribution, miner/wallet `bitcoind`, and harnesses.

Bond curve is chain config, not a script constant:

```text
NODE_BONDSTARTAMOUNTSATS=100000000
NODE_BONDSLOTINCREMENTSATS=100000000
```

Required bond is `start + slot * increment`. In the current run, the next slot is 6
and requires 700000000 sats.

## Canonical Entrypoint

Show commands:

```bash
ops/scripts/thornado-cluster.sh help
```

Local:

```bash
ops/scripts/thornado-cluster.sh local build
RUN_ROOT=/tmp/thornado-node5-churn-20260619191627 ops/scripts/thornado-cluster.sh local resume
RUN_ROOT=/tmp/thornado-node5-churn-20260619191627 ops/scripts/thornado-cluster.sh local status
```

HCloud:

```bash
WORKER_NODES="5 6 7 8 9" ops/scripts/thornado-cluster.sh cloud sync-ops
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud status
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud resume
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud tail
```

Fresh HCloud run root:

```bash
RUN_ROOT=/tmp/thornado-nodeper-$(date -u +%Y%m%d%H%M%S) \
  ops/scripts/thornado-cluster.sh cloud bootstrap
```

## Local Cluster

Use the local process cluster for fast iteration. Do not start long-lived Thornado or Bifrost
processes as bare background jobs from a short-lived shell. The wrapper uses the existing
supervised real4/node5 resume path.

Build:

```bash
ops/scripts/thornado-cluster.sh local build
```

Resume an existing run root:

```bash
RUN_ROOT=/tmp/thornado-node5-churn-20260619191627 \
  BTC_RPC_PORT=24645 API_BASE=2370 GRPC_BASE=13380 RPC_BASE=33360 \
  P2P_BASE=33380 EBIFROST_BASE=58600 FROST_P2P_BASE=9340 \
  FROST_INFO_BASE=10340 METRICS_BASE=14200 \
  ops/scripts/thornado-cluster.sh local resume
```

Only use Docker through the wrapper when explicitly testing Docker localnet:

```bash
ops/scripts/thornado-cluster.sh local docker-up
ops/scripts/thornado-cluster.sh local docker-down
ops/scripts/thornado-cluster.sh local docker-reset
```

## HCloud Setup

The proven launch order is:

1. Coordinator initializes genesis state and exports worker bundles.
2. Worker bundles are copied to the matching worker run roots.
3. Coordinator `bitcoind` starts, wallet state is loaded, and the 20s miner loop starts.
4. Worker `bitcoind` processes start and connect to coordinator Bitcoin P2P.
5. Worker Thornado validators start and reach RPC health.
6. Worker Bifrost signers start after all Thornado RPC endpoints are reachable.
7. FROST genesis keygen converges.
8. Validate vaults, signer queues, Thornado heights, and app hashes before transactions.

Run the sequence:

```bash
ops/scripts/thornado-cluster.sh cloud sync-ops

RUN_ROOT=/tmp/thornado-nodeper-$(date -u +%Y%m%d%H%M%S) \
  ops/scripts/thornado-cluster.sh cloud bootstrap
```

Resume without deleting state:

```bash
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud resume
```

Status must show the current worker APIs, RPC heights, and Bifrost health:

```bash
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud status
```

## Churn Notes

As of `2026-06-30T06:43:00Z`, the churn target is reached and churn is still enabled:

```text
active: node5, node6, node7, node8
standby: node9
target active: 4
standby slots: 1
churn interval: 10 minutes
churn retry: 5 minutes
```

Final validation for that run:

```text
ActiveVault status_since=20299 membership=4
txout queue empty
nodes/metrics next_slot=6 next_slot_bond_required_sats=700000000 active_slots=4 standby_slots=1
nodes5-9 app hashes matched at height 20340
FROST keygen at height 20296 completed in 0.5-0.6s on the four participants
migration keysign at height 20300 completed in 0.5-1.1s on the signing set
```

The failed churn at height 19766 was caused by stale
`BIFROST_FROST_BOOTSTRAP_PEERS`: active/churning nodes did not have node7/node8/node9
peer IDs and still had retired peers. The fix is to start Bifrost through
`distributed-regtest-cluster.sh start-worker-bifrost`, which prefers live
`/thornado/nodes` peer IDs over cached `meta/bifrost-bootstrap-all`.

Do not start Bifrost for a new worker before the node is registered and bonded.
Before registration, `observer is not whitelisted` logs are expected noise and the
node should be restarted through the normal launcher after registration.

State-sync snapshots were not served by the running nodes during node9 bootstrap.
The working recovery path was a controlled data copy from a synced non-active full
node, excluding `cs.wal` and `priv_validator_state.json`, then restarting Thornado
with the node's own validator state preserved.

Known follow-ups:

```text
penalty points remain high on some nodes and should be explained through protocol state
inactive Bifrosts log repeated "skipping attest ..." messages; treat as minor log cleanup
manual recovery attempts produced old "not observable inbound" warnings; keep investigating if they recur
```

### BTC Migration Source-Input Incident

On `2026-06-30`, churn/keygen succeeded but the vault at height `20402` could not
migrate assets. The chain logged:

```text
fail to add bitcoin migration txout: no source inputs for retiring vault ...
```

Root cause: BTC source-input selection treated `TxOutItem.VaultPubKey` as the UTXO
owner. That is wrong for migrations and sweeps: `VaultPubKey` is the spending/source
vault for the txout item, while the new UTXO belongs to whichever vault address is in
`ToAddress`.

Fix: source-input selection now treats a completed BTC txout as this vault's UTXO
when `ToAddress` equals the vault BTC address, regardless of the txout source
`VaultPubKey`. This was changed in both migration source selection and shared BTC
outbound source selection.

Live verification:

```text
fixed binary hash: ba6f749793b65a785339cbec5da9a548d452b21c2c896ef2b889d87dfd278741
migration txout height: 21122
status: complete
source input: EA56DD5A24F68419B39C3B290F3CBFF4B7FBEA87F3924CE144F308E3FEB51C41:0:10668060876
out hash: 13F5F8A07E93997EF57C62BE37E7BA88397DC2A29625A0FCA236D060636185B5
active vault BTC after migration: 10668046521 sats
FROST keysign height 21124: finished in about 0.6-1.0s
next churn keygen height 21152: finished in about 0.5-1.2s
next churn migration txout height 21155: complete
next churn active vault BTC after migration: 10668025236 sats
```

### Mixed-Tree Deploy Consensus Incident

On `2026-07-02`, deploying the full working tree while a shielder Merkle FFI
migration was still in flight split consensus at height `53275`: three distinct app
hashes for the same block (old binary, new-binary majority, new-binary re-execution).
The in-flight FFI produced node-local results, and the new `app.go` mounted extra
stores so the old binary could no longer load the DB.

Rules learned:

- Never deploy while `git status` contains another session's in-flight changes.
  Compare the source delta against the previous BUILD_ID's file list first.
- `thornado rollback --hard` must run under the binary that wrote the newest
  stores; plain `rollback` is a no-op when called twice.
- Version skew blocks churn silently: after an on-chain `UpdateActiveNodeVersions`
  upgrade (network at 3.17.1), standbys stamped from `constants.Version` (3.17.0)
  never pass `NodeAccountPreflightCheck` min-version and churn no-ops with no log.
  Bump `constants.Version` together with registering the upgrade.

Recovery that worked:

```text
1. identify the one node whose DB was only touched by the old binary (node9)
2. rollback --hard everyone to the last common app hash (53274 / 1841E186...)
   using the new binary where new stores existed
3. tar node9 data excluding cs.wal and priv_validator_state.json, restore to
   nodes 5-8 keeping each node's own priv_validator_state.json
4. reinstall build per-vault-batch-20260702023644 and restart thornado + bifrost
```

### Per-Vault Batch Serialization Change

Deployed `2026-07-02` (build `per-vault-batch-20260702023644`, thornado
`133edf2931197df21f9d2ca7b73822ae8a6ac156323cc98435b00fe909ba87d4`).

BTC outbound batching is now strictly per vault with serialized signing:

- A batch block holds batchable items (`out`, `refund`) for exactly one vault.
  `Epoch` is a per-vault monotonic sequence (continues from legacy global values).
- A new batch opens one `Withdrawal_BatchWindowMinutes` window ahead of the first
  item and grows until its close height; items after close open the next epoch.
- `pending_batch` promotes to `pending_sign` only when no earlier incomplete batch
  exists for the same vault AND every item has source inputs. A batch queued behind
  an unfinished predecessor may hold empty source inputs until the predecessor's
  change output is observed; the end-block refresh fills them in.
- Mixed blocks (batchable+internal, or multi-vault) are split every end-block and by
  the `3.17.1` upgrade repair (`RepairMixedBTCPendingBatches`).
- `keeper.AppendTxOut` (sweep/migrate/consolidate) probes for an empty block, so an
  internal can never join a pending batch at its close height.

Live verification on this cluster:

```text
flow3 10/10 success run_dir=meta/parallel-flow3/20260702024358
batch txout height 49814 epoch 3552: 10 out items, 1 vault, signed as 1 BTC tx
out hash C1ED73E2248FC9C5..., single shared source input
app hashes matched on nodes 5-9 at heights 49660/49825
```

Nodes 1-3 remain halted at height 24268 from earlier partial deploys and cannot
replay history; test harnesses must remap logical node 1 to a live node, e.g.
`NODE_SPECS='1=5.223.54.110:2375:33365:5'` when running `hcloud-parallel-flow3.sh`
on the coordinator.

## HCloud Deploy

Use one deployment path:

```bash
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy
```

For targeted source sync:

```bash
BUILD_ID=fix-name-$(date -u +%Y%m%d%H%M%S) \
SKIP_SOURCE_SYNC=0 \
SOURCE_FILES="go-thornado/path/file.go go-thornado/path/file_test.go ops/scripts/thornado-cluster.sh" \
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
WORKER_NODES="5 6 7 8 9" \
ops/scripts/thornado-cluster.sh cloud deploy
```

Restart Bifrost only:

```bash
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy-restart
```

Restart Thornado then Bifrost only for major protocol/state-transition fixes:

```bash
WORKER_NODES="5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy-restart-all
```

For Thornado consensus/state-transition fixes on the current cluster, deploy/restart
all live Thornado processes:

```bash
WORKER_NODES="1 2 3 5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy
WORKER_NODES="1 2 3 5 6 7 8 9" RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud restart-thornado
```

Bifrost restarts must not reuse old captured env files after churn. The deploy helper
now passes `WORKER_NODES` and relaunches Bifrost via the distributed cluster launcher
so bootstrap peers are regenerated from live node metadata.

If only scripts changed and no binary restart is needed, sync the script through
the wrapper. Do not broad-rsync.

```bash
ops/scripts/thornado-cluster.sh cloud sync-ops
```

Override `SOURCE_FILES` when you need a narrower or wider targeted sync.

## Test Harnesses

One parallel Flow 3 batch:

```bash
COUNT=20 RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud test-flow3
```

Remaining edge/fault suite:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  TX_INCLUSION_TIMEOUT=1200 THORNADO_TX_TIMEOUT=60 \
  ops/scripts/thornado-cluster.sh cloud test-remaining
```

Individual lower-level harnesses still exist for focused debugging:

```bash
ops/scripts/hcloud-edge-cases.sh
ops/scripts/hcloud-refund-script-test.sh
ops/scripts/hcloud-fee-swing-test.sh
ops/scripts/hcloud-parallel-flow3.sh
ops/scripts/hcloud-remaining-tests.sh
```

Current known gap: the existing fee-swing harness proves a fee estimate increase before
a fresh batch, not rescheduling of an already-queued txout. Add a dedicated reschedule
case before marking that bucket complete.

## Debug Workflow

Start with read-only state:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud status
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 LINES=120 ops/scripts/thornado-cluster.sh cloud tail
curl -fsS http://NODE_HOST:FROST_INFO/debug/health/full
curl -fsS http://NODE_HOST:FROST_INFO/debug/signer/txouts
curl -fsS http://NODE_HOST:FROST_INFO/debug/signer/performance
curl -fsS http://NODE_HOST:FROST_INFO/debug/frost/sessions
curl -fsS http://NODE_HOST:API/thornado/txout/all
curl -fsS http://NODE_HOST:API/thornado/vaults/solvency
curl -fsS http://NODE_HOST:API/thornado/config
```

Use Bifrost debug APIs to explain where work is stuck: observer, signer queue,
FROST session, Bitcoin scanner, Thornado txout state, and vault accounting.
If observability is missing, add a read-only debug API before changing protocol behavior.

Manual re-observe or re-attest recovery must use local node machinery and operator intent.
It must not be an automatic background process and must not clear or abandon queued work.

## Incident Rules

For stuck transaction, halt, or solvency risk:

- Preserve run root and logs.
- Snapshot Thornado `/status`, `/config`, `/txout/all`, `/vaults/base`, `/vaults/solvency`.
- Snapshot all Bifrost `/debug/health/full`, `/debug/signer/txouts`, `/debug/signer/performance`, `/debug/frost/sessions`.
- Check every worker `bitcoind` for the same tx/block/mempool view.
- Identify the exact failed state transition before retrying recovery.
- Fix code, deploy through the canonical deploy path, and restart only what is necessary.

Do not:

- Clear old refunds.
- Delete state to unstick app hash unless explicitly tearing down.
- Add retry layers that hide a stuck protocol bug.
- Treat noisy logs as acceptable if they appear on the happy path.

## Historical Notes

These files contain useful incident history but are not the current operating procedure:

```text
docs/hcloud-nodeper-frost-runbook.md
docs/hcloud-bonded-rotation4-runbook.md
docs/distributed-regtest-cluster.md
docs/flow-checks/manual-runbook.md
docs/hcloud-testing-todo.md
```
