# Thornado Protobuf Contracts

This directory owns shared cross-component schemas for the Thornado split architecture.

## Ownership

- `proto/common/v1/` contains types shared by Go THORNode, Go Bifrost, Rust FROST signer, Rust privacy, and fixtures.
- `proto/frost/v1/` contains the signer sidecar API used by Bifrost or Thornode orchestration.
- `proto/privacy/v1/` contains the deterministic privacy verifier API and helper request shapes.

Workstream owners may propose interface changes in their own directories first. Contract changes should land here only after the affected Go and Rust consumers agree on compatibility.

## Compatibility Rules

- Packages are versioned in the path and protobuf package name.
- Do not renumber, rename, or reuse existing field numbers after consumers exist.
- Additive fields are preferred for `v1`.
- Breaking changes require a new package version, such as `v2`.
- Bytes fields must document their canonical encoding in fixtures before production use.
- RPC responses must return deterministic `ErrorCode` values. Consensus-facing callers must not depend on transport-specific error text.

## Generation Expectations

Generated Go and Rust bindings are intentionally not checked in yet. Initial consumers should generate bindings from these source `.proto` files with pinned tool versions and then add CI checks that generated code is current.

Expected future commands:

```sh
buf lint proto
buf generate proto
```

Until `buf.yaml` and generation plugins are committed, do not wire generated code into Go or Rust packages.
