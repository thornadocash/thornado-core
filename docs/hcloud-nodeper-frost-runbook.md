# HCloud Node-Per-Server FROST Runbook

Canonical current runbook: [Thornado Cluster Runbook](./thornado-cluster-runbook.md).
This file is retained as historical incident/setup notes.

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

- `5.223.51.101`

Workers:

- node1: `5.223.55.114`
- node2: `5.223.55.174`
- node3: `5.223.52.254`
- node4: `5.223.93.218`

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

- 2026-06-28 deployment drift: `SOURCE_FILES` source sync updated the
  coordinator before build, but did not update worker launcher scripts. Workers
  ran stale `ops/scripts/distributed-regtest-cluster.sh`, started Bifrost with
  node1-only live `/p2pid` bootstrap, and genesis keygen exhausted retries with
  incomplete party peer addresses. Fix: the deploy script now syncs source
  deltas to the coordinator and workers by default. Launcher/config script
  changes must be deployed through that same path before relaunch.

- Bad shell quoting around `$RUN_ROOT` can trigger the sourced `real-4node-e2e.sh` cleanup trap. Use direct commands or `NO_CLEANUP_TRAP=1` when sourcing helpers.

## Artifact Distribution

Use the scripted path from the local workspace:

```bash
bash ops/scripts/hcloud-deploy-binaries.sh deploy
```

This is the only binary/script deployment path to use for the HCloud node-per-server cluster:

1. If `SKIP_SOURCE_SYNC=0`, sync `SOURCE_FILES` to the coordinator and all workers.
2. Build `thornado` and `bifrost` on the Linux coordinator with `-tags 'regtest mocknet'`.
3. Pack one artifact under `/root/thornado/build`.
4. Serve it from the coordinator with a short-lived Python HTTP server.
5. Make all workers pull the artifact in parallel from the coordinator.
6. Verify artifact and binary hashes on every worker.
7. Atomically replace `/root/thornado/build/thornado` and `/root/thornado/build/bifrost`.

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
fbfa8a161767c5431751cf00eec7270c6463b4d91f6c6939c105c30726abde20
```

Latest deployed Thornado hash:

```text
742ff34ef34653136bee6ece870f379307011fdc81a4c66679dffa408d6bc2cd
```

For explicit source sync before building:

```bash
SKIP_SOURCE_SYNC=0 SOURCE_FILES="go-thornado/path/file.go go-thornado/path/file_test.go" \
  bash ops/scripts/hcloud-deploy-binaries.sh deploy
```

When a launch script changes, include it in `SOURCE_FILES`; otherwise workers
can have current binaries with stale orchestration:

```bash
SKIP_SOURCE_SYNC=0 \
  SOURCE_FILES="ops/scripts/distributed-regtest-cluster.sh ops/scripts/hcloud-deploy-binaries.sh" \
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
5. Build a complete FROST bootstrap list from `/thornado/nodes` plus `meta/node*.env`.
6. Start all four Bifrosts with the complete FROST bootstrap list.
7. Do not rely on node1-only live `/p2pid` bootstrap for genesis keygen.
8. If any Bifrost sees genesis keygen before all party peer addresses are available, relaunch cleanly; do not accept retry exhaustion.
9. Confirm `/debug/vaults/local` returns the same active vault on all four nodes.
10. Run repeated deposits and shield withdrawals from the coordinator.
11. Require 100% FROST keygen/keysign success; any miss is a bug to debug, not a transaction to abandon.

## Current Run

Run root:

```bash
/tmp/thornado-nodeper-20260628131009
```

Confirmed:

- Coordinator bitcoind is isolated under the run root.
- Miner loop mines one regtest BTC block every 20 seconds.
- Worker bitcoinds are connected to coordinator bitcoind.
- Thornado consensus is live on nodes 1-4.
- Bifrost health is live on nodes 1-4.
- FROST genesis keygen produced one active 4-member vault on all nodes, with independent local keyshares per Bifrost.
- Keygen requires 100% participation of Thornado's targeted keygen set: active vault members plus any nodes churning in.
- Keysign requires 67% of the active node set.
- Edge run `20260628134822` passed multi-output/vout/OP_RETURN/direct-refund/dust/coinbase cases.
- Node4-offline Bifrost fault run `node4-bifrost-down-final-20260628183151` passed:
  - 4 deposits, 4 shield redeems, 4 successes, 0 failures.
  - Node4 Bifrost was down during signing.
  - Sweeps and withdrawals completed with 3-of-4 FROST signing.
  - Final outbound batch hash: `2416824387A2E30AD6AD072E9F5B37488BC09D535769BA233570713C27D8A88C`.
  - Final signer queues were zero on all four workers after node4 restart.
- All-online FROST broadcast run `all-online-20260628234842` passed:
  - 4 deposits, 4 shield redeems, 4 successes, 0 failures.
  - The 4 withdrawals reconciled into one BTC outbound batch.
  - Each signed BTC transaction had three signer broadcasts, matching the 67% selected signing set.
  - Duplicate/late broadcast handling was harmless.
  - Final signer queues were zero on all four workers.
- Node1 restart-during-signing run `restart-flow-logwatch-20260628235806` passed:
  - Node1 Bifrost was stopped at `2026-06-29T00:01:05Z` while outbounds were waiting to sign.
  - Node1 was restarted at `2026-06-29T00:01:25Z` through `ops/scripts/hcloud-deploy-binaries.sh restart-bifrost`.
  - Node2, node3, and node4 completed the 4-output outbound batch with 3-of-4 FROST signing.
  - Final outbound batch hash: `F75AB8BC5C7C8949FF21B28E982C6BA980745017CEADE682CBE33844875EFB33`.
  - Final signer queues were zero on all four workers after node1 restart.
- Current minor local-only cleanup: attestation stream `protocol not supported` during peer restart is demoted locally from error to debug; deploy only with the next major change.

## 2026-06-28 Direct Refund Source-Input Bug

Root cause:

- Direct deposits to the active base vault queued a normal refund with the direct deposit as the intended source input.
- BTC pending-batch gas refresh reselected source inputs for all batchable txouts, including refunds that already had a prescribed direct source input.
- The refund could spend an older vault UTXO instead of the direct deposit UTXO.

Fix deployed:

- Direct base-vault refund source inputs are pinned to the inbound deposit UTXO.
- Pending-batch refresh preserves pinned source inputs where the txout input equals the txout `in_hash`.

## 2026-06-28 FROST Offline-Leader Bug

Root cause:

- Keysign preconnect originally required every vault member, so 3-of-4 signing failed if one Bifrost was offline.
- After threshold preconnect, an offline node could still be selected as party leader.
- The fast leader-unavailable path did not persist `SigningLeaderRetry` for internal txouts, so sweeps retried the same dead leader in a tight loop.

Fix deployed:

- Keygen still requires 100% of Thornado's targeted keygen set.
- Keysign preconnect accepts the 67% active-node threshold.
- Keysign checks the designated leader first and fails fast if unreachable.
- Leader-unavailable retry persists the next-leader offset for all txout types, including sweeps.
- Timeout/success responses are sent only to online/selected peers.
- `EnsurePeersConnectedWithin` clears libp2p dial backoff while waiting for delayed peers.
- Regression test: `TestDirectBaseVaultRefundPinnedSourceSurvivesPendingBatchRefresh`.

Validation:

- Direct base-vault refund in edge run `20260628134822` completed with source input equal to its own inbound tx and `vout0`.

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

## 2026-06-28 HCloud Pressure Run Fixes

Cluster:

- Coordinator: `5.223.93.218`
- Workers: `5.223.51.101`, `5.223.55.114`, `5.223.55.174`, `5.223.92.204`
- Run root: `/tmp/thornado-nodeper-20260627104200`
- Deployed Thornado hash: `581609b14a31f2a9041880acb3a0d1eb2ce08bbe6f2f00ab27064930546668ac`
- Deployed Bifrost hash: `e6a5997787e7e8bb2632fb6854611b329eec1a8003cd42c654e4e02649099f20`

What failed:

- The first 50-flow pressure run produced valid FROST signatures, then Bitcoin Core rejected broadcast with `Fee exceeds maximum configured by user`.
- Thornado also logged `Failed to send event to subscriber`; the ebifrost event stream used an unbuffered subscriber channel and could drop events unless the Bifrost receiver was waiting at that exact instant.

Fixes deployed:

- Bifrost now broadcasts signed BTC txs with `SendRawTransaction(..., 0)` so local max-fee policy does not reject valid high-fee regtest consolidation or sweep txs.
- Thornado ebifrost subscriber queues are buffered and queue depth is logged if a subscriber falls behind.
- The pressure harness retries the same offline-signed tx on transient RPC transport errors and confirms inclusion before treating a nonzero sync response as failure.

Validation:

- Run `/tmp/thornado-nodeper-20260627104200/meta/parallel-flow3/20260628091829`: `COUNT=50`, success `50`, failed `0`.
- Run `/tmp/thornado-nodeper-20260627104200/meta/parallel-flow3/20260628100556`: `COUNT=20`, success `20`, failed `0`.
- The 20-flow run produced 6 batched BTC outbounds; every outbound reached `final_count=4` and completed.
- Keysign timing on the 20-flow run: min `584ms`, max `1304ms`, avg `946.7ms`.
- Final signer queues were zero on all four workers.

Current minor local-only cleanup:

- Pre-errata BTC source-input miss logging was demoted from warn to debug locally. Keep it until the next major deploy.

## 2026-06-28 Edge Case Harness

Harness:

```bash
RUN_ROOT=/tmp/thornado-nodeper-20260627104200 \
  TX_INCLUSION_TIMEOUT=1200 THORNADO_TX_TIMEOUT=60 \
  ./ops/scripts/hcloud-edge-cases.sh
```

Cases currently wired:

- Non-vault BTC tx with `OP_RETURN`: must not be tracked by Thornado.
- Multi-output BTC deposit with unrelated outputs and `OP_RETURN`: only the registered vault output may match; sweep must spend the exact matched source vout.
- Dust payment to a registered deposit address: must remain `address_issued` and must not queue a sweep.

Bug found:

- Bifrost `ignoreTx` rejected any BTC tx containing a Bitcoin Core `nulldata` output.
- That skipped valid user deposits where one output paid the vault and another output was `OP_RETURN`.
- Live stuck tx: `49a3d8cda60adbe9dc2c587513585d34befa04de3eff1950b7bd330d5232cd13`, vault output `vout=3`, amount `20000000` sats.

Fix deployed:

- Bifrost hash: `82049dfc59e285d039a3068915f4293f68ac78213d4cc7188a82acb4ffc31dd2`
- `ignoreTx` now skips `nulldata` outputs while still inspecting the rest of the transaction.
- Added a small normal Go regression test that runs without the gocheck fixture: `TestIgnoreTxSkipsNulldataOutputs`.

Recovery that worked:

```bash
xargs -0 -a /tmp/thornado-nodeper-20260627104200/meta/bifrost-N.restart.env \
  sh -c 'exec env "$@" /root/thornado/build/bifrost observe-tx --log-level debug --chain BTC <txid>' sh
```

Run it once per worker node. This submits normal observed inbound attestations; it does not clear or mutate state directly.

Validation:

- Recovered stuck tx matched and swept; sweep observation reached `final_count=4`.
- Clean run: `/tmp/thornado-nodeper-20260627104200/meta/edge-cases/20260628105348`
- Result: all three wired edge cases passed.
- Multi-output clean-run sweep source input matched the actual vault output and reached `final_count=4`.
- Final signer queues were zero on all four workers.

## 2026-06-28 Node-Per-Server Fast Batch Run

Cluster:

- Coordinator: `5.223.51.101`
- Workers: `5.223.55.114`, `5.223.55.174`, `5.223.52.254`, `5.223.93.218`
- Run root: `/tmp/thornado-nodeper-20260628131009`
- Deployed Thornado hash: `7149e3f9c44b7c61b59249535e92bc7d3fab8daf4f7e86ff7c2f6f49d9dda82e`
- Deployed Bifrost hash: `ced35c890b8b844f74425b071f5091f335f980fb7e289d5f4df9e19e8e3bc7d0`

Major bug fixed:

- Duplicate or losing FROST signing attempts could block forever waiting for a per-session `sync.Mutex`.
- Context cancellation did not unblock `Mutex.Lock()`, so a completed txout could leave stale signer goroutines behind.
- Fix: replace the FROST session mutex with a context-aware per-session lock and record `session_lock_wait_start` / `session_lock_acquired` in debug sessions.

Validation:

- Run: `/tmp/thornado-nodeper-20260628131009/meta/parallel-flow3/20260628151555`
- Result: `requested=24`, `success=24`, `failed=0`.
- Outbound batching: one BTC outbound hash for all 24 withdrawals.
- Batch shape: `tx_array=24`, `source_inputs=1`, `max_gas=1`.
- Signer queues ended empty on all four Bifrosts.
- Debug signer performance ended with `unfinished=0`, `errors=0` on all four Bifrosts.
- Max recorded signer duration after the lock fix: node1 `6737ms`, node2 `7273ms`, node3 `6912ms`, node4 `6998ms`.

Observation note:

- The delayed deposit matching seen during the run was not BTC confirmation delay and not deposit size.
- Deposits were `0.20000000 BTC`; lag was observation attestation/finalization into Thornado.

Harness fix:

- `hcloud-parallel-flow3.sh` and `hcloud-continue-parallel-flow3.sh` now prepare withdrawal proofs first, then submit redeem txs concurrently with locally incremented account sequence.
- This produced the expected single outbound batch instead of splitting across several Thornado blocks.

Operational warning:

- Do not source `ops/scripts/real-4node-e2e.sh` in diagnostic SSH shells. It installs cleanup traps. Use direct RPC/API commands or purpose-built harness scripts.

Minor local-only cleanup waiting for next major deploy:

- Suppress missing `address_book.seed` debug spam.
- Move duplicate cached-source spent notices to trace.
- Log BTC mempool batches at debug unless errors are present.
