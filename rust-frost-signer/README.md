# Rust FROST Signer

`rust-frost-signer` is the local signer sidecar for Thornado vault keys. It owns local FROST participant state, encrypted vault shares, DKG session state, signing session state, and local policy checks for Bitcoin outbound signing requests.

The signer is not a consensus component. Thornode remains the canonical source for vault membership, churn, outbound queue selection, slashing, jail, and whether a withdrawal is allowed. Bifrost remains responsible for Bitcoin scanning, transaction construction, broadcast, retry, and solvency reporting.

## Crate Layout

```text
rust-frost-signer/
  crates/
    frost-core/       # request models, policy checks, mock signing, future FROST logic
    frost-storage/    # storage traits and in-memory dev backend
    frost-service/    # service boundary and HTTP health endpoint skeleton
    frost-cli/        # operator CLI for health/status/dev inspection
  docs/
    API-ASSUMPTIONS.md
```

`frost-core` intentionally contains no networking. It wraps the existing `thornado-core` FROST engine instead of duplicating cryptography. That engine already owns DKG, nonce generation, partial signatures, aggregation, Taproot signing support, snapshot serialization, and custody signature verification.

`frost-storage` owns persistence contracts. The first implementation is in-memory dev storage; production storage must encrypt local shares at rest before it is usable outside localnet.

`frost-service` wraps core and storage with process-level concerns. It exposes HTTP endpoints for the expected sidecar operations while the shared protobuf contract is still being finalized.

`frost-cli` is an operator entry point for health checks and, later, local signer state inspection.

## Responsibilities

The signer is responsible for:

- FROST participant identity and local key shares.
- DKG session participation and transcript tracking.
- Signing session participation, nonce state, partial signatures, and optional aggregation.
- Vault public key reporting for locally held shares.
- Local request policy checks before producing a signature.
- Signer audit logs and share backup/export policy, if export is ever allowed.

The signer is not responsible for:

- Canonical vault membership or churn decisions.
- Slash, jail, bond, or validator lifecycle state.
- Outbound queue selection or withdrawal authorization.
- Bitcoin observation, transaction broadcast, or retry.
- Privacy note, nullifier, commitment, or proof-verification state.

## API Assumptions

`proto/frost/v1/signer.proto` does not exist in this worktree yet. Local API assumptions are documented in [docs/API-ASSUMPTIONS.md](docs/API-ASSUMPTIONS.md) and should be reconciled with the shared proto when Agent 5 adds it.

The expected service surface is:

- `Health`
- `GetNodeSignerInfo`
- `StartDkg`
- `GetDkgStatus`
- `GetVaultPubKey`
- `StartSigning`
- `GetSigningStatus`
- `ForgetVaultShare`

Signing requests must include structured Bitcoin policy context, not just opaque bytes.

Current HTTP surface:

```text
GET  /health
GET  /v1/signer/info
POST /v1/dev/generate
POST /v1/sign
POST /v1/dkg/round1
POST /v1/dkg/round2
POST /v1/dkg/finalize
POST /v1/signers/{signer_id}/commitment
POST /v1/signature-share
POST /v1/aggregate
POST /v1/signers/{signer_id}/taproot-commitment
POST /v1/taproot-signature-share
POST /v1/taproot-aggregate
```

Run the sidecar with:

```sh
cargo run -p thornado-frost-cli -- serve --listen 127.0.0.1:8081 --snapshot ./data/signer.json
```

`/v1/dev/generate` creates a local DKG signer snapshot for integration tests. Production DKG should use the round 1, round 2, and finalize endpoints with authenticated peer transport.

## Storage Model

Vault share records must include:

- `vault_id`
- FROST group public key
- local participant index
- threshold
- participant set
- encrypted secret share bytes
- DKG transcript hash
- creation height or epoch
- share status

Signing session records must include:

- `session_id`
- `vault_id`
- Bitcoin sighash or PSBT digest package
- structured outbound policy fields
- nonce commitment state
- local nonce state
- partial signature, once produced
- participant set
- expiry height or expiry timestamp
- session status

Nonce storage must be conservative. A session ID cannot be reused for a conflicting message, and local nonce material must never be reused across signing sessions.

## Dev Mode

Dev mode is explicitly insecure and is only for localnet integration while the Go workstreams are still being separated.

The mock signer:

- Accepts requests for locally known vault IDs only.
- Requires `chain_id` to be `bitcoin`.
- Rejects expired requests and conflicting session reuse.
- Returns deterministic mock signature bytes derived from the session ID, vault ID, and signing payload.
- Stores signing session state in memory.

Mock signatures are not valid Bitcoin signatures and must not be used for broadcast tests that expect real network validation.

The current implementation keeps the older `InMemoryStore::start_dev_signing` mock for early Bifrost tests, but the service path now uses the existing real FROST engine from `thornado-core`.
