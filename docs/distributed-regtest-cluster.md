# Distributed Regtest Cluster

This runbook starts a six-server regtest deployment:

- controller: bitcoind, miner loop, wallet management, genesis nodes 1-4
- workers: nodes 5-9, each with Thornado, Bifrost, and a local bitcoind peer

All Go binaries must be built with `-tags 'regtest mocknet'`.

## Inventory

```bash
cp ops/distributed-regtest.env.example ops/distributed-regtest.env
$EDITOR ops/distributed-regtest.env
```

Use reachable private IPs. The controller IP is also used as the authoritative
Bitcoin RPC endpoint for all Bifrost wallets.

## Controller

```bash
INVENTORY=ops/distributed-regtest.env ops/scripts/distributed-regtest-cluster.sh init-controller
INVENTORY=ops/distributed-regtest.env ops/scripts/distributed-regtest-cluster.sh start-controller-genesis
INVENTORY=ops/distributed-regtest.env ops/scripts/distributed-regtest-cluster.sh validate-genesis-config
INVENTORY=ops/distributed-regtest.env ops/scripts/distributed-regtest-cluster.sh export-worker-bundles
```

Genesis includes:

- `Halt_Churning=1`
- `Node_BondStartAmountSats=100000000`
- `Node_SetDesired=4`
- `BTC_ConfirmationsMin=1`
- `BTC_ConfMultiplierBasisPoints=10000`
- `Churn_IntervalMinutes=10`
- `Churn_RetryIntervalMinutes=5`

Copy each `worker-nodeN.tgz` from `$RUN_ROOT/meta/` to the matching worker and
extract it under the same `$RUN_ROOT`.

## Workers

Run on each worker after extracting its bundle and syncing the repo/binaries:

```bash
INVENTORY=ops/distributed-regtest.env RUN_ROOT=/tmp/thornado-distributed-... NODE=5 ops/scripts/distributed-regtest-cluster.sh start-worker
```

Repeat for nodes 6-9 with the matching `NODE`.

## Bond Workers

Run from the controller after workers are healthy:

```bash
INVENTORY=ops/distributed-regtest.env RUN_ROOT=/tmp/thornado-distributed-... ops/scripts/distributed-regtest-cluster.sh bond-workers
```

The bonding driver handles request-deposit, controller miner funding, deposit
match/sweep, shield, proof generation, `bond-from-notes`, node key setup, and
version setup for nodes 5-9. Churn remains halted after bonding.

## Checks

```bash
INVENTORY=ops/distributed-regtest.env RUN_ROOT=/tmp/thornado-distributed-... ops/scripts/distributed-regtest-cluster.sh status
```

Critical checks:

- one active base vault after genesis, with node1-node4 membership
- genesis config values above are live
- nodes 5-9 show full bond, `pending_sats=0`, `fee_share_active=true`
- nodes 5-9 are Standby, Whitelisted, or Selected before churn is manually enabled
