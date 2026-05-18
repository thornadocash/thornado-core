# Thornado Localnet Plan

This document defines the target localnet for the forked Thornado services.
It is a coordination contract for the Go THORNode, Go Bifrost, Rust FROST
signer, Shielder, proto, and ops workstreams.

## Goals

The localnet must eventually prove one full private Bitcoin custody flow:

```text
start localnet
  -> initialize Thornode validators
  -> create node accounts
  -> bond nodes
  -> churn active vault membership
  -> run FROST DKG
  -> derive Bitcoin vault deposit address
  -> send regtest BTC deposit
  -> Bifrost observes deposit
  -> Thornode accepts observation
  -> Shielder records commitment/root
  -> client generates withdrawal proof
  -> Thornode verifies proof
  -> nullifier is marked spent
  -> outbound Bitcoin intent is queued
  -> Bifrost builds outbound tx
  -> FROST signer signs tx
  -> Bifrost broadcasts tx
  -> Thornode records outbound completion
```

The first useful version does not need real FROST or real proofs. It should
start the Go processes and mock Rust services, then exercise service boundaries.

## Service Topology

Target services:

```text
bitcoind-regtest

thornode-1
thornode-2
thornode-3

bifrost-1
bifrost-2
bifrost-3

frost-signer-1
frost-signer-2
frost-signer-3

shielder-1
shielder-2
shielder-3

client
```

The one-verifier-per-Thornode shape is the default sidecar mode. If the
production decision moves to embedded verification, the verifier containers can
stay useful for fixture generation and non-consensus proof tooling.

## Ports

Default port map:

| Service | Internal | Host | Purpose |
| --- | ---: | ---: | --- |
| bitcoind-regtest | 18443 | 18443 | Bitcoin RPC |
| bitcoind-regtest | 18444 | 18444 | Bitcoin P2P |
| thornode-1 | 26656 | 26656 | CometBFT P2P |
| thornode-1 | 26657 | 26657 | CometBFT RPC |
| thornode-1 | 1317 | 1317 | REST/API |
| thornode-1 | 9090 | 9090 | gRPC |
| thornode-2 | 26656 | 26756 | CometBFT P2P |
| thornode-2 | 26657 | 26757 | CometBFT RPC |
| thornode-2 | 1317 | 1318 | REST/API |
| thornode-2 | 9090 | 9091 | gRPC |
| thornode-3 | 26656 | 26856 | CometBFT P2P |
| thornode-3 | 26657 | 26857 | CometBFT RPC |
| thornode-3 | 1317 | 1319 | REST/API |
| thornode-3 | 9090 | 9092 | gRPC |
| bifrost-1 | 6040 | 6040 | Health/debug |
| bifrost-2 | 6040 | 6041 | Health/debug |
| bifrost-3 | 6040 | 6042 | Health/debug |
| frost-signer-1 | 7001 | 7001 | FROST signer gRPC |
| frost-signer-2 | 7001 | 7002 | FROST signer gRPC |
| frost-signer-3 | 7001 | 7003 | FROST signer gRPC |
| shielder-1 | 7101 | 7101 | Shielder gRPC |
| shielder-2 | 7101 | 7102 | Shielder gRPC |
| shielder-3 | 7101 | 7103 | Shielder gRPC |

Host ports are for local debugging only. Containers should communicate through
Docker DNS names and internal ports.

## Data And Logs

Use named Docker volumes for durable service state:

```text
thornado-bitcoin
thornado-thornode-1
thornado-thornode-2
thornado-thornode-3
thornado-bifrost-1
thornado-bifrost-2
thornado-bifrost-3
thornado-frost-signer-1
thornado-frost-signer-2
thornado-frost-signer-3
```

Use a host-mounted logs directory:

```text
ops/logs/
  bitcoind/
  thornode-1/
  thornode-2/
  thornode-3/
  bifrost-1/
  bifrost-2/
  bifrost-3/
  frost-signer-1/
  frost-signer-2/
  frost-signer-3/
  shielder-1/
  shielder-2/
  shielder-3/
  smoke/
```

`ops/logs/` should stay untracked.

## Startup Order

Start services in this order:

1. `bitcoind-regtest`
2. `shielder-*`
3. `frost-signer-*`
4. `thornode-*`
5. `bifrost-*`
6. `client`

Reasoning:

- Thornode startup can validate shielder availability in sidecar mode.
- Bifrost should start after Thornode RPC/API endpoints are available.
- FROST signers should be online before keygen/churn tests.
- The client should only run after all service health checks pass.

## Health Checks

Minimum health checks:

```text
bitcoind-regtest:
  bitcoin-cli -regtest getblockchaininfo

thornode:
  GET /status on CometBFT RPC
  GET /health or equivalent app endpoint
  grpc health check when available

bifrost:
  GET /health or equivalent debug endpoint
  confirms connection to bitcoind
  confirms connection to thornode
  confirms connection to frost signer

frost-signer:
  grpc Health
  reports node signer ID
  reports storage status
  reports dev/mock/real mode

shielder:
  grpc Health
  reports verifier version
  reports available verifying key hashes
  reports mock/real mode
```

Health checks should be strict for smoke tests but lenient enough during early
development to allow mock services.

## Localnet Modes

### Mock Mode

Purpose: prove service boundaries.

Expected behavior:

- FROST signer returns deterministic mock vault pubkeys/signatures.
- Shielder returns deterministic fixture-based valid/invalid results.
- Thornode and Bifrost exercise real request paths but not real cryptography.

This is the first integration target.

The current mock mode uses `ops/docker-compose.mock.yml` and
`ops/mock-service/` to provide simple HTTP/TCP-compatible containers for all
unfinished Go/Rust services. It is intentionally shallow: it verifies container
topology, port exposure, startup order, health waiting, log collection, and
Bitcoin regtest bootstrap.

### Crypto Mode

Purpose: prove real FROST and real Shielder verification.

Expected behavior:

- FROST DKG creates real vault pubkeys.
- FROST signer produces valid Bitcoin signatures.
- Shielder validates real proof fixtures.
- Bitcoin runs on regtest.

This mode should use the same service names and ports as mock mode.

## Required Scripts

The script surface should remain stable even while implementations evolve:

```text
ops/scripts/localnet-up.sh
ops/scripts/localnet-down.sh
ops/scripts/localnet-reset.sh
ops/scripts/wait-for-health.sh
ops/scripts/bootstrap-regtest.sh
ops/scripts/bootstrap-thornode.sh
ops/scripts/run-frost-dkg.sh
ops/scripts/send-deposit.sh
ops/scripts/run-withdrawal.sh
ops/scripts/smoke-private-flow.sh
ops/scripts/collect-logs.sh
```

Scripts should read `ops/env.localnet` when present and otherwise use defaults
from `ops/env.localnet.example`.

Until the owning workstreams publish real CLIs or HTTP/gRPC commands, these
scripts use environment hooks:

```text
THORNODE_BOOTSTRAP_CMD
FROST_DKG_CMD
FROST_DKG_STATUS_CMD
SEND_DEPOSIT_CMD
RUN_WITHDRAWAL_CMD
```

Each hook is executed with `bash -lc` from the repository root. The hook names
are the handoff contract between ops and the implementation agents.

## Smoke Test Gates

### Gate 1: Mock Boundary Smoke

Required checks:

- compose stack starts
- bitcoind is reachable
- mock shielder is reachable
- mock FROST signer is reachable
- Thornode can call shielder
- Bifrost can call FROST signer
- logs are collected on failure

### Gate 2: Bitcoin-Only Spine

Required checks:

- Bifrost observes regtest Bitcoin tx
- Thornode receives observation
- Thornode queues a mock outbound
- Bifrost constructs a Bitcoin outbound request
- mock signer returns a deterministic response

### Gate 3: FROST Vault

Required checks:

- FROST DKG completes
- vault pubkey is reported
- Thornode stores vault pubkey
- Bifrost derives deposit address
- signer restart preserves share state

### Gate 4: Signed Bitcoin Outbound

Required checks:

- Bifrost constructs real outbound transaction
- FROST signer signs
- transaction broadcasts on regtest
- Bitcoin node confirms transaction
- Thornode records outbound completion

### Gate 5: Shielder Proof

Required checks:

- shielder reports expected verifying key hash
- valid withdrawal proof passes
- invalid proof fails
- duplicate nullifier fails
- unknown root fails

### Gate 6: Full Private Flow

Required checks:

- one script runs deposit to withdrawal end to end
- final Bitcoin recipient balance increases
- nullifier is spent
- replayed withdrawal fails
- logs identify every cross-service request ID

## Common Failure Modes

### bitcoind is not ready

Symptoms:

- Bifrost cannot connect to Bitcoin RPC.
- regtest bootstrap cannot mine blocks.

Debug:

```bash
docker compose -f ops/docker-compose.localnet.yml logs bitcoind-regtest
bitcoin-cli -regtest -rpcconnect=127.0.0.1 -rpcport=18443 getblockchaininfo
```

### Thornode starts before verifier

Symptoms:

- Thornode fails startup compatibility checks.
- Withdrawal tx validation fails due to verifier transport errors.

Debug:

```bash
docker compose -f ops/docker-compose.localnet.yml ps shielder-1
docker compose -f ops/docker-compose.localnet.yml logs shielder-1 thornode-1
```

### Bifrost cannot reach signer

Symptoms:

- Outbound signing stalls.
- Bifrost retry queue grows.

Debug:

```bash
docker compose -f ops/docker-compose.localnet.yml ps frost-signer-1
docker compose -f ops/docker-compose.localnet.yml logs bifrost-1 frost-signer-1
```

### FROST DKG does not complete

Symptoms:

- No vault pubkey is reported.
- Churn/keygen flow stalls.

Debug:

```bash
ops/scripts/run-frost-dkg.sh --status
docker compose -f ops/docker-compose.localnet.yml logs frost-signer-1 frost-signer-2 frost-signer-3
```

### Withdrawal proof fails

Symptoms:

- Thornode rejects `MsgWithdrawNote`.
- Verifier returns invalid proof or key mismatch.

Debug:

```bash
docker compose -f ops/docker-compose.localnet.yml logs thornode-1 shielder-1
```

Check:

- root exists
- root is not expired
- nullifier is unused
- verifying key hash matches params
- public inputs match proof fixture

## Reset Policy

`localnet-reset.sh` should:

- stop all localnet services
- remove localnet containers
- remove localnet volumes
- remove generated localnet state
- preserve `ops/logs/` unless `--logs` is provided

Signer share state is intentionally disposable in localnet. Production backup
and recovery rules belong to the signer workstream and hardening phase.

## Open Ops Decisions

- exact Thornode health endpoint names
- exact Bifrost health endpoint names
- gRPC health protocol package for Rust services
- whether shielder is sidecar or embedded in production
- whether Bifrost or Thornode coordinates FROST DKG
- final Bitcoin deposit memo/address scheme
- final client/relayer process responsibilities
