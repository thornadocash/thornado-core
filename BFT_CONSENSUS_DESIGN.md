# BFT Consensus Design Notes

## Status

Conceptual design notes for the Thornado replicated-state layer, aligned to `THORNADO.pdf`.

This document does not lock the project to a specific consensus engine. It describes what the consensus layer must do for Thornado.

## Scope

The committee must agree on:

- deposit-address intents
- observed deposits and confirmation status
- deposit-to-note split finalization
- note redemptions and spent-note markers
- outbound fee charging
- fee bucket locks and claims
- churn-epoch transitions
- standby-node assignment
- offline penalties
- slot auction settlement
- churn-out / churn-in membership changes
- mirror registrations and health status
- shutdown votes and maintenance-mode transitions

Throughput requirements are modest. Consistency, auditability, replay resistance, and operator accountability matter more than raw TPS.

## Recommended Consensus Properties

- Byzantine fault tolerance
- immediate or fast finality
- deterministic ordered log
- explicit validator-set changes at epoch boundaries
- durable replicated storage
- cryptographic evidence of equivocation
- clean embedding into a Rust node runtime

## Suggested Architecture

A leader-based BFT replicated state machine in the Tendermint or HotStuff family is the right shape for Thornado.

The replicated log should contain state transitions, not raw private material.

On-log:

- intent creation
- deposit observation
- claim finalization
- note spend authorization
- fee bucket state transitions
- penalty records
- slot transfer authorization
- epoch transitions
- shutdown events

Off-log:

- FROST nonce exchange
- partial signatures
- wallet secrets
- note secrets
- per-request network metadata

The split is deliberate. Consensus decides what is authorized. FROST executes the Bitcoin signing after authorization.

## Quorum Semantics

The PDF uses `67%` for signing and shutdown governance. The consensus layer should align with that operator mental model even if the underlying engine uses standard `3f + 1` validator math internally.

At the protocol level:

- custody actions require the configured signing quorum
- governance actions require the configured governance quorum
- validator-set changes happen only at churn boundaries

## Validator Set and Churn

- Slots are permanent.
- One node may be standby during a churn cycle.
- Churn interval is currently `7 days`.
- Membership changes should happen only at epoch boundaries.
- Nodes offline for an entire cycle should be eligible for penalty and churn-out.

Consensus must therefore carry:

- current epoch number
- active validator set
- standby assignment
- pending slot transfer records
- penalty accumulation

## Storage Requirements

The state layer needs durable storage for:

- deposit intents
- observed deposits
- note states
- spent-note markers or equivalent nullifier records
- fee buckets
- slot ownership
- bond balances and penalty balances
- mirror registry entries
- churn epochs
- shutdown status

## Engine Selection Criteria

The engine should be chosen based on:

- embeddability into a single Rust node binary
- audit surface
- operational simplicity
- validator-set change support
- durability guarantees
- ecosystem stability

A pre-existing Rust BFT library may still be the fastest path, but the product specification does not require a specific one by name.

## Failure and Slashing Signals

Consensus should emit durable evidence for:

- equivocation
- missing an entire churn cycle
- invalid mirror attestations
- invalid slot-transfer attempts
- invalid shutdown actions

This evidence is what lets the rest of the system apply penalties and slot changes cleanly.

## Open Design Questions

- Should governance and custody use the same quorum rule or separate thresholds?
- Should mirror-health penalties be on-chain state, committee state, or off-chain observability input?
- Should slot auctions settle directly inside consensus state or through an external settlement record?
- Should fee-bucket claim proofs be validated entirely inside consensus execution or pre-validated off the hot path?

Those are implementation questions, not reasons to defer the need for a BFT state layer.
