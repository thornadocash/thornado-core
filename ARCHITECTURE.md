# Architecture

Thornado uses `go-thornado` as the canonical state machine. The Rust state-machine prototype has been removed.

## Components

- `go-thornado`: Go THORChain fork for consensus, node lifecycle, bonding, churning, slashing, vault accounting, Bitcoin observation, outbound queues, Mimir/halts, and eBifrost.
- `crates/thornado-shielder`: Rust Shielder privacy engine for note derivation, Orchard commitments, Merkle roots, withdrawal proof construction, and proof verification.
- `crates/thornado-ffi`: C ABI around Shielder for Go and other native callers.
- `crates/thornado-web-wasm`: browser Shielder bindings for wallet-side note/proof flows.
- `proto`: shared contracts for Shielder, FROST, and common identifiers.
- `ops`: localnet composition and runbooks.
- `test-fixtures`: deterministic Bitcoin, FROST, and Shielder fixtures.

## Boundaries

`go-thornado` owns consensus state. Shielder owns privacy proofs only. FROST signing will be plumbed into `go-thornado` as a signer sidecar/interface; it should not reintroduce a second Rust state machine.

Withdrawal flow target:

1. Client derives notes and Shielder proofs locally.
2. `go-thornado` verifies Shielder public inputs and nullifier freshness.
3. `go-thornado` queues authorized Bitcoin outbounds.
4. Bitcoin Bifrost obtains FROST signatures and broadcasts.

The only Rust code left in the main workspace is privacy support, not chain state.
