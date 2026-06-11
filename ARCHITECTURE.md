# Architecture

Thornado uses `go-thornado` as the canonical state machine. The Rust state-machine prototype has been removed.

## Components

- `go-thornado`: Go Thornado chain for consensus, node lifecycle, bonding, churning, vault accounting, Bitcoin observation, outbound queues, BTC vault maintenance halts, and eBifrost.
- `crates/thornado-shielder`: Rust Shielder privacy engine — Tornado Cash Groth16 withdraw verification (v2.1 vk), Pedersen/MiMC note format, note derivation, Merkle roots, and proof construction.
- `crates/thornado-ffi`: C ABI around Shielder for Go and other native callers.
- `crates/thornado-web-wasm`: browser Shielder bindings for wallet-side note/proof flows.
- `crates/thornado-frost-ffi`: C ABI around the BTC FROST keygen/keysign engine for Go Bifrost callers.
- `proto`: shared contracts for Shielder, FROST, and common identifiers.
- `ops`: localnet composition and runbooks.
- `test-fixtures`: deterministic Bitcoin, FROST, and Shielder fixtures.

## Boundaries

`go-thornado` owns consensus state. Shielder owns privacy proofs only. Bitcoin Bifrost uses the Go-wrapped FROST engine for keygen and signing; it does not reintroduce a second Rust state machine.

Withdrawal flow target:

1. Client derives notes and Shielder proofs locally.
2. `go-thornado` verifies Shielder public inputs and nullifier freshness.
3. `go-thornado` queues authorized Bitcoin outbounds.
4. Bitcoin Bifrost obtains FROST signatures and broadcasts.

The Rust workspace contains protocol engines and native/browser bindings, not chain state.
