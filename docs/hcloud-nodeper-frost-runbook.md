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

- Coordinator-to-worker artifact copy through the local machine is slow. Use
  `ops/scripts/hcloud-deploy-binaries.sh`; workers pull the artifact directly
  from the coordinator and verify hashes before install.

- Bad shell quoting around `$RUN_ROOT` can trigger the sourced `real-4node-e2e.sh` cleanup trap. Use direct commands or `NO_CLEANUP_TRAP=1` when sourcing helpers.

## Artifact Distribution

Use the scripted path from the local workspace:

```bash
bash ops/scripts/hcloud-deploy-binaries.sh deploy
```

This is the only binary deployment path to use for the HCloud node-per-server cluster:

1. Build `thornado` and `bifrost` on the Linux coordinator with `-tags 'regtest mocknet'`.
2. Pack one artifact under `/root/thornado/build`.
3. Serve it from the coordinator with a short-lived Python HTTP server.
4. Make all workers pull the artifact in parallel from the coordinator.
5. Verify artifact and binary hashes on every worker.
6. Atomically replace `/root/thornado/build/thornado` and `/root/thornado/build/bifrost`.

The script does not restart processes. Restart Thornado/Bifrost separately and preserve the run root.

For a Bifrost-only rollout that restarts the running HCloud Bifrost processes:

```bash
BUILD_ID=<short-name>-$(date -u +%Y%m%d%H%M%S) SKIP_SOURCE_SYNC=0 INCLUDE_UNTRACKED=0 \
  BINS='bifrost' TAGS='regtest mocknet' RUN_TESTS=0 \
  RUN_ROOT=/tmp/thornado-nodeper-20260627104200 \
  bash ops/scripts/hcloud-deploy-binaries.sh deploy-restart
```

Latest deployed Bifrost hash:

```text
1597e13316ad945e2bb6d1ff1c57f19782057e54fb82d59a866393bbbf3cd643
```

For explicit source sync before building:

```bash
SKIP_SOURCE_SYNC=0 SOURCE_FILES="go-thornado/path/file.go go-thornado/path/file_test.go" \
  bash ops/scripts/hcloud-deploy-binaries.sh deploy
```

Do not broad-rsync the repo and do not build on each worker; the workers may have stale native FROST wrapper state.

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

## 2026-06-27 Solvency Halt Incident

Root cause:

- BTC outbound `DEB75C2A44A8A7B01D635E82F7B917E8B5F2B38FC09F91DD719F55BD890970D3` spent `9,900,000` sats plus `3,094` sats gas from the active vault.
- Thornado vault accounting had not run because the observed-out voter was stranded at `1/4` signers.
- Solvency compared the stale vault amount `130081548` against the wallet amount `120178454`, saw a `9903094` gap, and halted.
- Bifrost had removed the local observation from `ondeck` after a committed one-attestation `QuorumTx`, before observed-tx quorum.

Fix deployed:

- Thornado solvency now treats signed BTC txouts with `OutHash` as pending until the observed-out voter is done, including observed gas.
- Bifrost now removes observed txs from `ondeck` only when the committed observed tx has supermajority.
- Focused tests passed for `./x/thornado` and `./bifrost/observer` with `-tags 'regtest mocknet'`.

Deployed hashes:

- `thornado`: `f7825834d03dcf4e968a777b8ae3b0098a17288ed1dd7e9da0d2a532875747d9`
- `bifrost`: `c67417302c5259ffe17e13026111ab8d0b7235dc8e4767b9da20c2254333c0d1`

Recovery:

- Rolled all four Thornado nodes and all four Bifrost nodes against the same run root; no state was deleted.
- Re-submitted the real DEB75 observed-out through normal `observe-tx-outs` validator messages from validators 1-4.
- DEB75 reached `consensus_height=2208`, `finalised_height=2208`, status `done`, with all four signers.
- Active vault accounting now matches the BTC wallet: `120178454 BTC.BTC`.
- `HALT_SOLVENCYCHECK=0`, `HALTSIGNINGBTC=0`, `HALT_CHAINGLOBAL=0`, `NODEPAUSECHAINGLOBAL=0`; `HALT_CHURNING=2` remains intentional.

## Debug APIs Added

Bifrost read-only endpoints:

- `/debug/health/full`
- `/debug/signer/txouts`
- `/debug/signer/txout/{in_hash}`
- `/debug/btc/txout/{in_hash}`
- `/debug/vaults/local`

Use these to explain why a queued txout is not progressing without mutating protocol state.

Recovery endpoint:

- `POST /debug/btc/txout/{in_hash}/observe-recovered`

Use only when the chain transaction already exists and Thornado has not reconciled the observed outbound. This submits the recovered observation; it does not clear or abandon the queued txout.

## 2026-06-28 Refund Observation Stall

Incident:

- Refund input hash: `B1ACAE4A75D5D8EFAC66ABD5487565F7C406D164E04E69A4BE1AE15C74793EFF`
- BTC refund tx: `0d9bf84956537b07ca0ed771d4338f1c90eecb47682b5a3a3e051f772df75e82`
- Thornado had the refund stuck at `pending_sign`, while the BTC tx was already confirmed.
- All four Bifrost signer stores had recovered observations, but the deferred FROST retry path only checked completion and did not submit the stored observation.

Recovery that worked:

```bash
for spec in 5.223.51.101:10341 5.223.55.114:10342 5.223.55.174:10343 5.223.92.204:10344; do
  h=${spec%:*}; p=${spec#*:}
  curl -sS -X POST --max-time 10 \
    "http://$h:$p/debug/btc/txout/B1ACAE4A75D5D8EFAC66ABD5487565F7C406D164E04E69A4BE1AE15C74793EFF/observe-recovered"
done
```

Result:

- Thornado marked the refund txout `complete` on all four nodes.
- `/thornado/tx/0D9BF84956537B07CA0ED771D4338F1C90EECB47682B5A3A3E051F772DF75E82` returned status `done`.
- All four signers attested the observed outbound.
- Signer queues returned to zero; BTC scanners were healthy and height-aligned.

Fix deployed:

- Bifrost now submits a deferred, recovered pre-sign observation once before waiting for another signing retry period.
- It only does this when the stored item has an observation and no signed tx/checkpoint payload, so non-leader FROST participants do not attest an unbroadcast transaction.
- After submission, the cached local observation is cleared to avoid per-block retry noise; the queued txout remains until Thornado marks it complete.

## Rules

- Do not change protocol behavior to hide failures.
- Do not abandon queued transactions as stale.
- Halt churning through config when needed; do not stretch churn intervals as a workaround.
- Prefer targeted sync/build artifacts over broad repo rsync.
- Do not delete running state unless explicitly tearing down that cluster.
- Major means stuck transactions, halted state, or lost-funds risk; fix and redeploy.
- Minor means noisy logs or optimization; patch locally and hold until the next major deploy.

## 2026-06-28 Repeated Flow 3 Validation

Current cluster:

- Coordinator: `5.223.93.218`
- Workers: `5.223.51.101`, `5.223.55.114`, `5.223.55.174`, `5.223.92.204`
- Run root: `/tmp/thornado-nodeper-20260627104200`
- Deployed Bifrost hash: `f3e13f5f76a2b44947e0ee5692d45a4bdaa82aef8eb9af13edf83849e2194dd5`
- Active repeat log: `/tmp/thornado-nodeper-20260627104200/logs/repeat-flow3-20260628044841.log`

Repeat validation command:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260627104200 ITERATIONS=20 \
  nohup bash ops/scripts/hcloud-repeated-flow3.sh \
  >/tmp/thornado-nodeper-20260627104200/logs/repeat-flow3-$(date -u +%Y%m%d%H%M%S).log 2>&1 &
```

Harness fast-mode:

- `WAIT_OBSERVED_OUT_FINAL_EACH=0` starts the next iteration immediately after Flow 3 validates signing, BTC broadcast, recipient payment, and fee accounting.
- `WAIT_OBSERVED_OUT_FINAL_END=1` still verifies every recorded outbound observation at the end.
- Use per-iteration finality only when debugging reconciliation timing: `WAIT_OBSERVED_OUT_FINAL_EACH=1`.
