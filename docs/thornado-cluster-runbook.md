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
node1 5.223.55.114
node2 5.223.55.174
node3 5.223.52.254
node4 5.223.93.218
```

Inventory:

```bash
ops/distributed-regtest-nodeper.env
```

Default run root used by the active node-per-server cluster:

```bash
/tmp/thornado-nodeper-20260628131009
```

Server shape that worked: Hetzner `cpx32` or larger for every coordinator/worker.
Each worker runs one Thornado, one Bifrost, and one local `bitcoind`.
The coordinator runs build/artifact distribution, miner/wallet `bitcoind`, and harnesses.

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
ops/scripts/thornado-cluster.sh cloud sync-ops
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud status
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud resume
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 ops/scripts/thornado-cluster.sh cloud tail
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
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud resume
```

Status must show all four worker APIs, RPC heights, and Bifrost health:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud status
```

## HCloud Deploy

Use one deployment path:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy
```

For targeted source sync:

```bash
BUILD_ID=fix-name-$(date -u +%Y%m%d%H%M%S) \
SKIP_SOURCE_SYNC=0 \
SOURCE_FILES="go-thornado/path/file.go go-thornado/path/file_test.go ops/scripts/thornado-cluster.sh" \
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
ops/scripts/thornado-cluster.sh cloud deploy
```

Restart Bifrost only:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy-restart
```

Restart Thornado then Bifrost only for major protocol/state-transition fixes:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260628131009 \
  ops/scripts/thornado-cluster.sh cloud deploy-restart-all
```

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
