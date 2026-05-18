# Thornado Ops

This directory owns localnet and operations planning for the split Thornado
service topology:

- Go THORNode fork
- Go Bitcoin-only Bifrost fork
- Rust FROST signer sidecars
- Rust Shielder sidecar or embedded Shielder
- Bitcoin regtest

The files here are intentionally isolated from the current root Docker setup.
They describe the target localnet shape for the forked services and should
not assume that `go-thornode/`, `go-bifrost/`, `rust-frost-signer/`, or
`shielder/` already exist.

## Files

- `LOCALNET.md`: service topology, ports, startup order, health checks, and
  smoke-test flow.
- `docker-compose.localnet.yml`: target compose shape with placeholder build
  contexts and profiles.
- `docker-compose.mock.yml`: mock-mode override that lets ops boot service
  boundaries before the real Go/Rust directories exist.
- `env.localnet.example`: localnet environment defaults.
- `mock-service/`: tiny HTTP mock used by the mock Compose profile.
- `scripts/`: command stubs for the required localnet lifecycle.

## Quick Start

Mock mode is the default. It uses real `bitcoind` plus ops-owned mock services
for Thornode, Bifrost, FROST signer, and Shielder.

```bash
ops/scripts/localnet-up.sh
ops/scripts/wait-for-health.sh
ops/scripts/bootstrap-regtest.sh
ops/scripts/localnet-down.sh
```

Crypto/real mode expects the fork workstreams to provide buildable service
directories:

```bash
COMPOSE_PROFILES=crypto ops/scripts/localnet-up.sh
```

To customize local values:

```bash
cp ops/env.localnet.example ops/env.localnet
```

## Ownership

Ops owns service composition, startup order, health checks, scripts, logs, and
operator runbooks.

Ops does not own Go THORNode logic, Go Bifrost logic, FROST internals, Shielder
internals, or protobuf schema design.
