# Thornado Ops

This directory owns localnet and operations scripts for the current repo shape:

- `go-thornado`: Thornado state machine and Bitcoin Bifrost daemon.
- `crates/thornado-shielder`: Rust Shielder privacy engine.
- `go-thornado/go-wrappers/shielder`: Go cgo wrapper around `thornado-ffi`.
- Bitcoin regtest.

FROST signer services are future work. Shielder is not a standalone service in
this pass.

## Files

- `LOCALNET.md`: current localnet scope and smoke-test gates.
- `docker-compose.localnet.yml`: bitcoind plus mock Thornode/Bifrost service boundaries.
- `env.localnet.example`: localnet environment defaults.
- `mock-service/`: tiny HTTP mock used by the mock Compose profile.
- `scripts/`: localnet lifecycle and smoke-test hooks.

## Quick Start

```bash
ops/scripts/localnet-up.sh
ops/scripts/wait-for-health.sh
ops/scripts/bootstrap-regtest.sh
ops/scripts/localnet-down.sh
```

Build the real Shielder wrapper from repo root:

```bash
cd go-thornado
make shielder-ffi
CGO_ENABLED=1 go test ./go-wrappers/shielder
```
