# Manual Protocol Validation Runbook

This runbook is the source of truth for manual validation. The harness may start the
cluster, submit transactions, and collect snapshots, but script success is not proof.
Each stage is validated from chain state, BTC state, Bifrost state, and accounting
deltas.

## Validation Levels

- critical: vault membership, BTC movement, bond accounting, note/nullifier state,
  fee accounting, app hash convergence, and rejected attack state deltas.
- progress: node status transitions, timing of churn ticks, expected selected node,
  log ordering, and exact height sequencing.
- diagnostic: Bifrost peer timing, retry logs, scanner lag, and mempool timing.

Do not fail a manual run only because a progress or diagnostic item differs. Record
it, inspect the critical state, and continue if the critical state is correct.

## Common Snapshot

Capture this before and after every stage:

```bash
curl -fsS "$API/thornado/nodes" > nodes.json
curl -fsS "$API/thornado/nodes/metrics" > nodes-metrics.json
curl -fsS "$API/thornado/vaults/base" > vaults-base.json
curl -fsS "$API/thornado/txout/all" > txout-all.json
curl -fsS "$API/thornado/config" > config.json
curl -fsS "$RPC/status" > status.json
bitcoin-cli -regtest -rpcwallet=miner listunspent > btc-utxos.json
```

Also capture Bifrost logs for the touched nodes and any local FROST state files for
new active vault pubkeys.

## Stage 1 - Genesis FROST Vault

- action: start 4 genesis Thornado nodes and 4 Bifrost signers.
- critical: exactly one ActiveVault exists.
- critical: ActiveVault membership equals the four active node secp keys.
- critical: each Bifrost signer has local FROST state for the active vault.
- critical: app hash converges across all live Thornado nodes.
- progress: keygen appears automatically without CLI MsgTssPool intervention.
- progress: genesis nodes may have zero bond.

## Stage 2 - Bond Standby Node

- action: request deposit for node bond owner, fund BTC child address, sweep to base
  vault, shield to notes, withdraw notes to bond escrow.
- critical: deposit request has no desired amount.
- critical: BTC child UTXO amount equals user-sent amount.
- critical: sweep spends child UTXO to current ActiveVault.
- critical: shield commitments and Merkle root exist in API and KV.
- critical: bond-from-notes spends nullifiers exactly once.
- critical: node bond increases by the expected required sats.
- progress: node remains Standby until churn.

## Stage 3 - User Deposit / Split / Withdraw

- action: user requests deposit with key only, sends arbitrary BTC, shields, splits,
  withdraws with child proof.
- critical: observed amount comes from BTC, not request args.
- critical: fee is `max(100 bps, 100000 sats)`.
- critical: redeem spends the note nullifier exactly once.
- critical: outbound pays recipient `denom - fee`.
- critical: operator fee entitlement increases by fee.
- progress: batching or txout height may vary.

## Stage 4 - Operator Fee Claim

- action: operator claims accumulated fees into shielded notes.
- critical: claimable fee decreases by claimed amount.
- critical: fee debt/accounting increases by claimed amount.
- critical: commitments are stored and synced.
- critical: no BTC txout is created for fee shielding.
- progress: note denominations may vary by splitter policy.

## Stage 5 - Slot Auction

- action: seller opens slot auction, buyer funds bid through shielded notes, seller
  shields sale proceeds.
- critical: auction and bid bind seller node, buyer operator, buyer node key, and
  bid amount.
- critical: bid funding spends buyer nullifier once.
- critical: seller sale shield creates seller note commitments.
- critical: seller bond becomes sold/zeroed according to current code behavior.
- critical: buyer bond becomes active/eligible according to selected bid.
- progress: exact auction status string is less important than state deltas.

## Stage 6 - Churn And Migration

- action: halt churn while preparing node, start node/Bifrost, clear halt, wait for
  churn, then validate migration.
- critical: `Halt_Churning=1` prevents churn while node setup is incomplete.
- critical: after `Halt_Churning=0`, next eligible churn or retry may happen later;
  exact height is not critical.
- critical: new ActiveVault membership equals active node secp keys.
- critical: old ActiveVault becomes RetiringVault.
- critical: migration txout spends old vault UTXOs and pays new vault address.
- critical: BTC migration confirms and value is conserved minus gas.
- critical: no stale completed migration is rebroadcast.
- progress: which node status says Selected first, and exact selected height, is
  progress only.

## Stage 7 - Deposit Counting And Consolidation

- action: send multiple deposits to active vault child addresses and watch sweep plus
  consolidation.
- critical: each deposit gets a unique child address/path.
- critical: each child UTXO is swept once.
- critical: consolidation spends active vault UTXOs back to the active vault.
- critical: consolidation value is conserved minus gas.
- progress: exact consolidation height and number of intermediate base UTXOs may vary.

## Stage 8 - Attack Paths

- action: submit rejected attack txs and compare before/after snapshots.
- critical: replayed bond proof rejects and no bond/nullifier state changes.
- critical: wrong proof policy rejects and no note state changes.
- critical: duplicate sale shield rejects and no second seller note is created.
- critical: sold node cannot restore eligibility through metadata changes.
- critical: fake auction bid rejects and no bid/auction state changes.
- critical: post-churn deposits route to the current ActiveVault only.
- progress: reject code/log text may vary; state delta is the authoritative check.

## Manual Decision Rule

The flow is passing when all critical checks pass. Progress mismatches are recorded
as observations unless they imply a critical mismatch.

The flow is failing when any critical value is wrong, missing, duplicated, or
unconserved.

