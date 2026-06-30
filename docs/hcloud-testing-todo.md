# HCloud Testing Todo

Canonical current cluster ops runbook: [Thornado Cluster Runbook](./thornado-cluster-runbook.md).
This file remains the detailed test backlog.

Date: 2026-06-28

Run target:

- Fresh HCloud node-per-server cluster.
- Four worker servers: one Thornado, one Bifrost, one local bitcoind each.
- One coordinator server: build/artifact host, coordinator bitcoind, miner loop, wallets, harness driver.
- Build/run Go binaries with `-tags 'regtest mocknet'`.

## Cluster Relaunch

- [x] Tear down current five-server distributed-regtest cluster.
- [x] Create fresh five-server HCloud cluster sized at least `cpx32`.
- [x] Bootstrap coordinator and workers with source, binaries, bitcoind, Go, and FROST native libs.
- [x] Generate fresh run root and node inventory.
- [x] Launch coordinator bitcoind and miner loop.
- [x] Launch worker bitcoinds, Thornado validators, then Bifrost signers.
- [x] Confirm consensus height convergence and Bifrost health on all four workers.
- [x] Confirm FROST genesis keygen: same active vault on all four workers, independent local keyshares.

## Testing Order

Run refunds, fee, and dust first; then batching; then reorg/errata; then node-fault chaos.

## Refunds

- [ ] Expired deposit refund.
- [ ] Malformed deposit refund.
- [ ] Refund with high input count.
- [ ] Refund where fee nearly consumes amount.
- [ ] Refund broadcast race where only one bitcoind accepts and the rest reject as already known/spent.

## Reorgs / Errata

- [ ] Deposit observed then reorged out.
- [ ] Outbound observed then reorged out.
- [ ] Mempool-only outbound dropped.
- [ ] Node bitcoinds disagree temporarily.
- [ ] Errata only after enough BTC blocks and `67%` active-node threshold.

## Massive Fee Swings

- [ ] Fee rate changes between txout creation and signing.
- [ ] Batch fee recomputation.
- [ ] High-input consolidation.
- [ ] Dust change output avoidance.
- [ ] Fee greater than spendable amount.
- [ ] Maxgas correctness across multi-out batch.

## Dust

- [ ] Dust deposits.
- [ ] Dust refunds.
- [ ] Dust withdrawals.
- [ ] Batch containing one dust-sized output.
- [ ] Change output below dust.
- [ ] Fee consuming output.

## Exotic Txout Scripts

- [ ] Use refund path to test P2WPKH, P2WSH, and taproot-style outputs if supported.
- [ ] Invalid/unknown script.
- [ ] Oversized script.
- [ ] Nonstandard script rejected by Bitcoin Core policy.

## User Deposit Tx Shape

- [x] Multi-output BTC tx where only one output pays the vault.
- [ ] Multiple vault outputs in one tx.
- [x] Vault output plus `OP_RETURN`.
- [ ] Vault output plus dust/change outputs.
- [x] Vault output at nonzero vout.
- [ ] Duplicate-looking amounts.
- [x] Unrelated outputs before/after vault output.
- [x] Assert only the actual vault output is credited.
- [x] Assert `OP_RETURN` is ignored for value/solvency.
- [x] Assert `vout` selection is exact.
- [ ] Assert no double-counting if one tx has multiple matching vault outputs.
- [x] Assert refund path targets the correct original deposit output.

## Batching Edge Cases

- [x] 20+ withdrawals into one epoch.
- [ ] Mixed amounts.
- [ ] Many inputs.
- [x] Many outputs.
- [ ] Duplicate recipient.
- [x] Same account nonce burst.
- [ ] Batch with one invalid/dust member.
- [ ] Batch fee reconciliation to individual txs.

## FROST Coordination

- [x] Leader slow/offline mid-session.
- [ ] Participant drops after round 1.
- [x] Non-selected nodes stop retrying cleanly.
- [x] Retry reforms with different signing set.
- [x] All signing nodes broadcast.
- [x] Duplicate broadcast is harmless.

## Node / Network Faults

- [x] Restart one Bifrost during signing.
- [ ] Restart one Thornado.
- [ ] Restart one bitcoind.
- [ ] Delayed peer.
- [ ] Partition one node.
- [ ] Bitcoind RPC timeout.
- [ ] Stale BTC scanner height.

## Vault / Solvency

- [ ] Sweep/migrate in mempool counted correctly.
- [ ] Reorg reverses solvency and unhalts via errata.
- [ ] Vault UTXO spent-but-unobserved.
- [ ] Base vault churn boundary.
- [ ] Deposit address expiry only due to churn.

## Idempotency

- [ ] Re-observe same deposit/outbound.
- [ ] Repeat attestations.
- [ ] Duplicate signer retry.
- [ ] Duplicate broadcast.
- [ ] Restart from persisted signer queue.

## Results

Current run root: `/tmp/thornado-nodeper-20260628131009`.

Genesis FROST keygen passed on all four Bifrosts at `2026-06-28T13:16:35Z` with `members=4`.

Keygen threshold rule: 100% of the targeted keygen set: active vault members plus any nodes churning in.

Keysign threshold rule: 67% of the active node set.

Edge run `/tmp/thornado-nodeper-20260628131009/meta/edge-cases/20260628134822` passed:

- Multi-output one-vault deposit.
- Direct base-vault deposit queued and completed refund using the direct deposit UTXO.
- Vault output at vout 5 with `OP_RETURN` before it.
- Vault output with `OP_RETURN` after it.
- Dust deposit ignored.
- Coinbase-to-deposit-address ignored.

Fast batch run `/tmp/thornado-nodeper-20260628131009/meta/parallel-flow3/20260628151555` passed:

- 24 deposits, shields, redeems, and observed outbounds.
- 24 withdrawals were signed as one BTC outbound batch.
- Batch shape: `tx_array=24`, `source_inputs=1`, `max_gas=1`.
- Final signer queues were zero on all four workers.
- FROST signer performance reported `unfinished=0` and `errors=0` on all four workers.

Node4-offline Bifrost fault run `/tmp/thornado-nodeper-20260628131009/meta/node-fault/node4-bifrost-down-final-20260628183151` passed:

- Node4 Bifrost was stopped during signing.
- First dead-leader attempt failed fast and rotated leader.
- 4 deposits, 4 shield redeems, 4 successes, 0 failures.
- 4 withdrawals reconciled into one outbound batch: `2416824387A2E30AD6AD072E9F5B37488BC09D535769BA233570713C27D8A88C`.
- Final signer queues were zero on all four workers after node4 restart.

All-online FROST broadcast run `/tmp/thornado-nodeper-20260628131009/meta/frost-broadcast/all-online-20260628234842` passed:

- 4 deposits, shields, redeems, and observed outbounds; 4 successes, 0 failures.
- The 4 withdrawals reconciled into one BTC outbound batch.
- Each signed BTC tx had three signer broadcasts, matching the 67% selected signing set.
- Duplicate/late broadcasts were harmless; final signer queues were zero on all four workers.

Node1 restart-during-signing run `/tmp/thornado-nodeper-20260628131009/meta/node-fault/restart-flow-logwatch-20260628235806` passed:

- Node1 Bifrost was stopped at `2026-06-29T00:01:05Z` while the run was waiting for outbound signing, then restarted at `2026-06-29T00:01:25Z`.
- Node2, node3, and node4 completed the 4-output outbound batch with 3-of-4 FROST signing while node1 was down.
- 4 deposits, 4 shield redeems, 4 successes, 0 failures.
- Final outbound batch hash: `F75AB8BC5C7C8949FF21B28E982C6BA980745017CEADE682CBE33844875EFB33`.
- Final signer queues were zero on all four workers after node1 restart.

Current minor local-only cleanup:

- Attestation stream `protocol not supported` during peer restart is demoted locally from error to debug. Keep it local until the next major deploy.
