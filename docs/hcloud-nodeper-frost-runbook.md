# HCloud Node-Per-Server FROST Runbook

Date: 2026-06-27

## Goal

Run a realistic 4-validator Thornado regtest cluster on HCloud:

- 1 Thornado node per server.
- 1 local `bitcoind` per Thornado/Bifrost server.
- 1 coordinator server for build, genesis, wallets, mining, and transaction driving.
- FROST keygen/keysign over Bifrost libp2p between the four node servers.

Build Go binaries with:

```bash
-tags 'regtest mocknet'
```

## Inventory

Coordinator:

- `5.223.93.218`

Workers:

- node1: `5.223.51.101`
- node2: `5.223.55.114`
- node3: `5.223.55.174`
- node4: `5.223.92.204`

Server size used for all five:

- Hetzner `cpx32`
- 4 vCPU, 8 GB RAM, 160 GB disk

## What Worked

- Old single-server Docker cluster was fully torn down.
- Old stale Thornado/Bifrost/bitcoind processes on the worker servers were killed.
- Coordinator source was patched with read-only Bifrost debug APIs.
- Coordinator builds pass with `regtest mocknet` after syncing the matching FROST Go wrapper.
- Binary hashes were verified identical across coordinator and all four workers before launch.
- Genesis keygen converged once all four Thornado nodes were producing blocks and all four Bifrost nodes were online.
- All four Bifrost nodes stored the same active FROST vault/public key and membership, each with its own independent local keyshare:
  - `tthorpub1addwnpepqv2sxtgzkyf6897gvqwr03dx7mavcpffdgcak0wfqvj0jta9mzdl2efkjsl`
  - BTC address `bcrt1prw2hcce2gnrm22umrrls3fm2vxa73f7tjq3qp63l75nyah6p57ys2a2zp7`
- Native Linux libraries are present:
  - `/root/thornado/target/release/libthornado_ffi.so`
  - `/root/thornado/go-thornado/go-wrappers/frost/includes/linux/amd64/libgofrost.so`

## What Did Not Work

- Broad repo rsync to HCloud was too slow and was cancelled.
- Targeted source sync initially missed `go-wrappers/frost/go-frost/sessions`, causing:

```text
undefined: frostsessions.SignSessionNewWithTweak
```

Fix: sync the wrapper package and Linux `libgofrost.so`, then rebuild on the coordinator.

- Starting Bifrost before 4-node consensus was live made node1 retry the genesis keygen later:

```text
failed to get node account
ensure party peers: no bootstrap address for peer ...
scheduled keygen retry
```

Fix: start all worker `bitcoind` and Thornado processes first, wait for consensus, then start Bifrost.

- Coordinator-to-worker artifact copy through local relay is slow. Better sequence:
  1. Download `/tmp/thornado-node-artifacts.tgz` from coordinator once.
  2. Upload the local artifact to all workers in parallel.
  3. Verify hashes on all workers.

- Bad shell quoting around `$RUN_ROOT` can trigger the sourced `real-4node-e2e.sh` cleanup trap. Use direct commands or `NO_CLEANUP_TRAP=1` when sourcing helpers.

## Current Build Command

Run on coordinator:

```bash
cd /root/thornado/go-thornado
PATH=/usr/local/go/bin:$PATH go test -tags "regtest mocknet" ./bifrost/signer ./bifrost/pkg/chainclients/shared/signercache
PATH=/usr/local/go/bin:$PATH go test -tags "regtest mocknet" ./bifrost/pkg/chainclients/btc -run "TestTxAlreadySignedDoesNotBlockInternalRecovery|TestTxBatchAlreadySignedRequiresEveryItem|TestMarkTxBatchSignedMarksEveryItem|TestFilterUtxosBySourceInputs|TestMigrateOutputAmount|TestNormalOutputAmount"
mkdir -p ../build
PATH=/usr/local/go/bin:$PATH go build -tags "regtest mocknet" -o ../build/bifrost ./cmd/bifrost
PATH=/usr/local/go/bin:$PATH go build -tags "regtest mocknet" -o ../build/thornado ./cmd/thornado
PATH=/usr/local/go/bin:$PATH go build -tags "regtest mocknet" -o ../build/shielder-e2e-helper ./cmd/shielder-e2e-helper
```

## Artifact Distribution

Pack on coordinator:

```bash
cd /root/thornado
tar -czf /tmp/thornado-node-artifacts.tgz \
  build/bifrost \
  build/thornado \
  build/shielder-e2e-helper \
  ops/scripts/distributed-regtest-cluster.sh \
  ops/scripts/real-4node-e2e.sh \
  ops/distributed-regtest-nodeper.env \
  go-thornado/go-wrappers/frost/includes/linux/amd64/libgofrost.so \
  target/release/libthornado_ffi.so
```

Copy/extract to each worker:

```bash
scp /tmp/thornado-node-artifacts.tgz root@$NODE_HOST:/tmp/
ssh root@$NODE_HOST 'mkdir -p /root/thornado && cd /root/thornado && tar -xzf /tmp/thornado-node-artifacts.tgz'
```

## Cluster Setup

Use:

```bash
INVENTORY=/root/thornado/ops/distributed-regtest-nodeper.env
WORKER_NODES="1 2 3 4"
RUN_ROOT=/tmp/thornado-nodeper-YYYYMMDDHHMMSS
SKIP_BUILD=1
```

Coordinator init:

```bash
cd /root/thornado
INVENTORY=$INVENTORY WORKER_NODES="$WORKER_NODES" RUN_ROOT=$RUN_ROOT SKIP_BUILD=1 \
  ops/scripts/distributed-regtest-cluster.sh init-controller
```

Export node bundles:

```bash
INVENTORY=$INVENTORY WORKER_NODES="$WORKER_NODES" RUN_ROOT=$RUN_ROOT \
  ops/scripts/distributed-regtest-cluster.sh export-worker-bundles
```

Copy `worker-nodeN.tgz` to the matching worker and extract under the same `$RUN_ROOT`.

## Convergence Plan

1. Start coordinator `bitcoind` and miner loop.
2. Start each worker `bitcoind`; each connects to coordinator `bitcoind`.
3. Start Thornado on all four workers.
4. Wait for all four Thornado RPC endpoints and block height convergence.
5. Start Bifrost node1 first with empty FROST bootstrap allowed.
6. Confirm node1 `/p2pid` is available.
7. Start Bifrost node2-node4 using node1 live `/p2pid` bootstrap.
8. If node1 saw genesis keygen before peers were online, wait for its scheduled retry height.
9. Confirm `/debug/vaults/local` returns the same active vault on all four nodes.
10. Run repeated deposits and shield withdrawals from the coordinator.
11. Require 100% FROST keygen/keysign success; any miss is a bug to debug, not a transaction to abandon.

## Current Run

Run root:

```bash
/tmp/thornado-nodeper-20260627104200
```

Confirmed:

- Coordinator bitcoind is isolated under the run root.
- Miner loop mines one regtest BTC block every 20 seconds.
- Worker bitcoinds are connected to coordinator bitcoind.
- Thornado consensus is live on nodes 1-4.
- Bifrost health is live on nodes 1-4.
- FROST genesis keygen produced one active 4-member vault on all nodes, with independent local keyshares per Bifrost.

## Debug APIs Added

Bifrost read-only endpoints:

- `/debug/health/full`
- `/debug/signer/txouts`
- `/debug/signer/txout/{in_hash}`
- `/debug/btc/txout/{in_hash}`
- `/debug/vaults/local`

Use these to explain why a queued txout is not progressing without mutating protocol state.

## Rules

- Do not change protocol behavior to hide failures.
- Do not abandon queued transactions as stale.
- Halt churning through config when needed; do not stretch churn intervals as a workaround.
- Prefer targeted sync/build artifacts over broad repo rsync.
- Do not delete running state unless explicitly tearing down that cluster.
