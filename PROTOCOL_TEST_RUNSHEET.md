# Thornado Protocol Test Runsheet

This runsheet moves from cheap checks to full protocol flow. Run from repo root unless noted.

## 0. Workspace Baseline

```sh
cd /Users/dev/dev/thornado
git -C go-thornado status --short
git -C go-thornado log --oneline -8
docker version
docker compose version
```

Expected: know exactly what is dirty before testing. Do not mix unrelated generated-file churn with protocol failures.

## 1. Fast Local Unit Coverage

```sh
cd go-thornado
go test ./go-wrappers/frost/go-frost/sessions -count=1
go test ./bifrost/pkg/chainclients/utxo -run 'Frost|Bitcoin|Taproot|Signer' -count=1
go test ./bifrost/tss ./bifrost/p2p/messages ./bifrost/p2p/storage ./bifrost/signer ./x/thornado/types -count=1
```

Validates:
- FROST keygen/sign/verify wrapper.
- BTC/FROST vault signer behavior.
- TSS encryption/local-state/keyshare support.
- Message/storage/type compile compatibility.

## 2. Broader Go Regression

```sh
cd go-thornado
go test ./cmd/thornado ./cmd/bifrost ./bifrost/... -count=1
make test TEST_DIR='./bifrost/... ./frost/... ./x/thornado/types'
```

Known current observation: untagged `./bifrost/...` panics in `bifrost/pkg/chainclients/shared/utxo` confirmation adjustment tests because the mock bridge has nil config handling. Treat that separately from FROST unless it reproduces under the mocknet target.

## 3. Local Docker Regtest

Preferred deploy path:

```sh
ops/scripts/deploy-localnet.sh
```

Useful variants:

```sh
ops/scripts/deploy-localnet.sh --reset
ops/scripts/deploy-localnet.sh --profiles mock,three-node
ops/scripts/deploy-localnet.sh --btc-rpc-port 18445 --btc-p2p-port 18446
```

The deploy script auto-selects free BTC host ports if `18443/18444` are already occupied. Container-to-container RPC stays on `bitcoind-regtest:18443`.

Manual lower-level checks:

```sh
ops/scripts/wait-for-health.sh
ops/scripts/bootstrap-regtest.sh
```

Expected:
- `bitcoind-regtest` healthy.
- `thornado-1` and `bifrost-1` ports reachable.
- Miner wallet funded and client wallet has regtest BTC.

## 4. Chain Bootstrap

```sh
ops/scripts/bootstrap-thornado.sh
```

Required contract for `THORNADO_BOOTSTRAP_CMD`:
- initialize validators,
- create node accounts,
- bond nodes,
- prepare churn/keygen trigger.

Pass criteria:
- Thornado node exposes healthy status/API.
- Node accounts are active or ready to churn.
- Bifrost can query Thornado over REST/gRPC.

## 5. FROST DKG

```sh
ops/scripts/run-frost-dkg.sh
ops/scripts/run-frost-dkg.sh --status
```

Required contracts:
- `FROST_DKG_CMD` starts DKG across signer sidecars and persists local state.
- `FROST_DKG_STATUS_CMD` prints session state and final vault pubkey.

Pass criteria:
- every signer has FROST local state for the same vault pubkey,
- keyshares are backed up in `MsgKeygenVault.keyshares_backup_frost`,
- restore path can recreate local state from backup,
- vault address derives on BTC regtest.

## 6. Deposit Observation

```sh
ops/scripts/send-deposit.sh
```

`SEND_DEPOSIT_CMD` must request a Shielder deposit address, send BTC to that
address, mine the required confirmations, and wait for Bifrost observation.

Pass criteria:
- regtest BTC tx confirms,
- Bifrost observes the deposit,
- Thornado records the inbound,
- note/commitment/nullifier data is queryable through the intended API.

## 7. Withdrawal And FROST Keysign

```sh
ops/scripts/run-withdrawal.sh
```

Required contract for `RUN_WITHDRAWAL_CMD`:
- submit withdrawal proof,
- enqueue outbound,
- run BTC FROST keysign,
- broadcast signed regtest transaction,
- verify recipient balance.

Pass criteria:
- proof accepted exactly once,
- invalid or replayed proof rejected,
- FROST signs all required BTC sighashes,
- outbound tx confirms on regtest,
- Thornado marks withdrawal complete.

## 8. Negative Protocol Cases

Run after happy path:

```sh
# examples depend on final CLI/API shape
# bad proof
# duplicate nullifier
# insufficient deposit confirmations
# wrong recipient/network
# signer offline during DKG
# signer offline during keysign
# corrupted FROST message/result
```

Pass criteria:
- failures are explicit,
- no funds move on invalid proofs,
- blame points at the faulty signer where possible,
- retry/recovery paths leave local state consistent.

## 9. Cleanup

```sh
ops/scripts/collect-logs.sh ops/logs/manual-$(date +%Y%m%d-%H%M%S)
ops/scripts/localnet-down.sh --remove-orphans
```

Use `ops/scripts/localnet-reset.sh --logs` only when you intentionally want to delete localnet volumes and logs.

## Scenario Harnesses

```sh
ops/scenarios/10-frost-local.sh
ops/scenarios/00-basic-localnet.sh
ops/scenarios/20-frost-dkg.sh
ops/scenarios/30-btc-deposit.sh
ops/scenarios/40-withdrawal.sh
```

`20+` scenarios intentionally fail with a clear missing-hook message until the corresponding env command is implemented.
