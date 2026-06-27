# HCloud Bonded Rotation4 Runbook

## Goal

Run a clean HCloud THORNado real5 compose cluster through `FLOW_MODE=bonded_rotation4` until it passes:

- genesis starts with four active, unbonded nodes
- node5 bonds through request-deposit, BTC deposit, shield, bond-from-notes
- each churn keeps `Node_SetDesired=4`, rotating one node in and one node out
- each churned-out node is bonded before being churned back in
- final state has exactly four active nodes and all four have active shielder bonds

## Commands

Inspect the current HCloud server and cluster:

```bash
ops/scripts/hcloud-bonded-rotation4.sh inspect
```

Sync harness changes without restarting:

```bash
ops/scripts/hcloud-bonded-rotation4.sh sync
```

Destroy the existing compose volumes and run the clean flow:

```bash
ops/scripts/hcloud-bonded-rotation4.sh run
```

After a good image exists, retry harness-only changes without rebuilding:

```bash
BUILD_ARG=--no-build ops/scripts/hcloud-bonded-rotation4.sh run
```

## Current Notes

- `2026-06-22`: Existing HCloud compose cluster was a failed `churn_spike_flow8` run, not a clean genesis state.
- Failure observed before reset: `flow8-node7-churn destination vault was not credited after migration`.
- The HCloud compose setup bind-mounts `ops/scripts/real-4node-e2e.sh` from `/root/thornado`, so syncing that file updates the harness inside the running container.
- Host ports map to container ports; inside-container API/RPC checks are more reliable than host-mapped ports.
- Clean reset rebuild can spend several minutes exporting/unpacking Docker layers. Check `docker system df` before assuming the build is wedged; this host had about 108GB of build cache and 30GB free.
- `set_config_from_active_nodes` must not wait for empty block production after the last config vote. The chain may stop at the last tx block, so the harness now polls `/thornado/config` until the target value is visible.
- Clean churn sequence is: set `Halt_Churning=1`, finish bonding/registration/node/Bifrost prep, set migration config and desired count, then clear `Halt_Churning` as the final trigger. Halt again immediately after migration settles.
- `ops/scripts/docker-real5-node5-churn.sh` accepts `BUILD_ARG=--no-build` so bind-mounted harness changes can be retried without recompiling the image.
- Config writes are idempotent in the harness: query `/thornado/config` first and skip voting if the target value is already live. This avoids unnecessary txs and waits during halted prep.
- If churn is unhalted after the regular churn height was skipped, the next attempt waits for `Churn_RetryIntervalMinutes`. Do not assume unhalt means immediate selection.
- With four active nodes and `Vault_BaseMembersMin=4`, `findCountToRemove` alone returns zero even when one selected node is ready to replace the leaving node. The churn decision must consider same-churn replacements so 1-in/1-out at the BFT minimum can keygen.
- `2026-06-22`: The clean bonded-rotation flow failed immediately after initial `Halt_Churning=1` because `/thornado/nodes/metrics` returns HTTP 500 when the four genesis nodes have no bond slots. For this flow, bootstrap each replacement with a fixed 1 BTC bond instead of asking metrics before any bonded node exists.
- `2026-06-22`: API reads can briefly return HTTP 500 around startup/config application even though the endpoint is healthy seconds later. Harness-critical reads for `/thornado/config` and `/thornado/nodes` now use retrying `wait_api_json_file`.
- `2026-06-22`: `set_config_from_active_nodes` still used raw per-node `/thornado/node/{addr}` reads through `node_query`; a transient 500 inside command substitution exited the script without a useful run summary. `node_query` now retries and writes debug artifacts.
- `2026-06-22`: Config voting must skip node env files for unregistered future nodes. Retrying `node_query` against node5 before registration caused a 120s stall; `set_config_from_active_nodes` now uses a quiet status read and continues when a node is not registered yet.
- `2026-06-22`: Round 2 failed because the harness kept bonding every returning node with 1 BTC. Churned-out nodes keep their allocated shielder slot, so node3 in slot 2 required 2 BTC and the 1 BTC bond correctly stayed pending. The harness now computes the required bond from the node's existing slot, using `/thornado/nodes/metrics` for slot pricing and falling back to 1 BTC only for the initial no-bond genesis metrics gap.
- `2026-06-22`: A fresh round 1 passed keygen and signed the BTC migrate, but the harness checked vault balances immediately after local BTC confirmation and failed before outbound observation quorum updated Thornado vault accounting. The rotation harness now waits for `/thornado/tx/{hash}` to report final observation, then polls old/new vault accounting before asserting credit/drain.
- `2026-06-22`: Clean halt/unhalt churn can skip the regular churn height while halted. If retry is left at 2 minutes, each missed retry adds a long stall. The bonded-rotation harness now explicitly sets `Churn_IntervalMinutes=1` and `Churn_RetryIntervalMinutes=1` before each unhalt trigger.
- `2026-06-22`: Round 2 retry fired but skipped because BTC was halted by solvency. The observed gap was exactly one 1 BTC shielder/bond flow (`vault amount=399984820`, wallet amount=`299990100`). Do not set `Halt_SolvencyCheck=1` as a harness workaround: that is a halt height and makes Bifrost skip inbound observations, so bond deposits never match. Keep it at `0` and fix any recurring solvency/accounting gap separately.
- `2026-06-22`: The recurring solvency gap was the previous BTC migration amount. Bifrost observes a vault-to-vault BTC migrate as both inbound to the destination and outbound from the source. Normal inbound handling already credits the destination vault, so the outbound helper must only settle the source vault. Extra destination credit double-counted migrations and caused later solvency halts.
- `2026-06-22`: After removing outbound destination credit, round 2 drained the old vault but left the new active vault with `coins=null`. Thornado observed memoless BTC migrations as ordinary unmatched inbounds, so the destination was not credited. Thornode credits migrate inbounds once they reach observation quorum, before external BTC finality. Thornado must do the same by matching the observed hash/address to a stored `TxOutTypeMigrate` item; outbound handling remains source-only and finality-gated.
- `2026-06-22`: Full `FLOW_MODE=churn_spike_flow8` first failed in Flow 5 on a negative assertion, not migration. Once an auction bid is selected, non-seller `node-sale-shield` can reject with `node sale entitlement is not shieldable` before the seller mismatch guard. The harness should assert the entitlement guard for that step.
- `2026-06-22`: Flow 5 then waited forever on a stale sale entitlement deposit id. Code derives `sha256("thornado:node-sale-entitlement:v2|<auction>|<bid>|<owner>")`; the harness still used the old v1 `auction|bid` preimage. Use the v2 owner-bound id before polling `/thornado/deposit/{id}`.
- `2026-06-23`: Flow 9 is the live node-tooling pass after Flow 8. It dynamically picks an active bonded node, validates maint/config/fee-set auth, rotates the operator to node5's key, and proves old-operator rejection plus new-operator control.
- `2026-06-23`: Tooling gaps are real: there is no live `node leave` tx command, and there is no bond-provider remove/unbond transaction. Do not claim HCloud validates those until protocol/CLI support exists.
- `2026-06-23`: Flow 9 fee assertions must coerce `operator_fee_basis_points` with `tonumber`; the API returns string amounts. The over-max fee sad path is rejected by CLI argument validation with `Usage:` before CheckTx, so assert CLI rejection plus unchanged bond state.
- `2026-06-23`: Operator rotation correctly moves the operator bonder row, but `updated_height` changes. Compare bonder ledger contents with `updated_height` removed; otherwise Flow 9 fails even though principal and bonder ownership are correct.
- `2026-06-23`: One HCloud run reached Flow 8 node8 migration with destination credited but old vault still `RetiringVault` and not drained because `/thornado/tx/{hash}` showed `inbound_confirmation_counted.completed=false` with a very large `remaining_confirmation_seconds`. This blocks the full Flow8->Flow9 harness. Manual Flow9 continuation against the live rotated state passed after the rotate assertion issue was isolated.
- `2026-06-23`: Do not source `real-4node-e2e.sh` manually without `NO_CLEANUP_TRAP=1`. A failing manual validation will run `cleanup_runtime` and kill child Thornado/Bifrost processes. For live diagnostics, export `NO_CLEANUP_TRAP=1 KEEP_RUNNING=1`.
- `2026-06-23`: Restarting extra nodes can hit an ephemeral-port collision where another Thornado process holds node8's fixed ebifrost port `50158` as an outbound local port. HCloud containers denied `ip_local_reserved_ports`, so avoid unnecessary in-place restarts; if it happens, identify the owner through `/proc/net/tcp` before restarting only the affected process.
- `2026-06-24`: A BTC vault-to-vault migrate must be fully accounted from one final inbound observation: credit the destination vault with the observed output amount, settle the source vault by the stored `SourceInputs`, and clear the migrate pending height. Do not rely on a separate outbound observation to drain the retiring vault.
- `2026-06-24`: For manual `observe-tx-ins`, pass the API root (`http://127.0.0.1:1318`), not `/thornado`; the CLI appends its own Thornado paths. When restarting Bifrost from env files, use `set -a; source ...; set +a` so FROST and BTC chain env vars are actually exported.
- `2026-06-25`: Do not rolling-restart Thornado app binaries after state-transition code changes. A rolling restart caused node1 to replay a block with the new app and reject old-node app hashes. For live binary upgrades, stop all active/standby Thornado processes at the same height, swap binaries, then start the live validator set together. If an inactive node diverges during a failed rolling attempt, keep it down and recover the active quorum first.
- `2026-06-25`: The live HCloud set after manual churn was node5/node6/node7/node8 Active and node2 Standby; node1/node3/node4 were not required for consensus. During the `/thornado/gas` upgrade, node2 Bifrost ran and joined P2P but did not bind its `6042` info `/ping` endpoint after restart; active Bifrosts 5-8 were healthy. Treat standby Bifrost info-health separately from active quorum health.
- `2026-06-25`: Mocknet defaults set `Chain_BlockTimeSeconds=1`, but the HCloud cluster produces roughly 6-second blocks. Harness churn setup must explicitly vote `Chain_BlockTimeSeconds=6` before setting churn intervals, otherwise "10 minute" churn math becomes 100 blocks instead of 10 real minutes.
