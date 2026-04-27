# Thornado FROST MPC Integration Plan

## Status

Conceptual engineering roadmap aligned to `THORNADO.pdf`.

This revision supersedes the earlier generic MPC-mixer plan. The target system is now a Thornado-style `67%` FROST vault plus note state machine, with bonded permanent slots, churn-based key rotation, client mirrors, light nodes, and a specific outbound-fee and bucket-claim model.

## Executive Summary

NWabiSabi-Rust is still useful, but it is no longer enough to describe the target system as "make the WabiSabi issuer distributed."

The Thornado design adds several first-class subsystems that the earlier plan either omitted or treated as optional:

- a `67%` FROST vault with state-machine control
- permanent bonded node slots
- a `7-day` churn cycle with one standby node
- slot auctions and churn-based slot handoff
- offline penalties
- shutdown governance
- client mirrors vouched by nodes
- light nodes for broadcast and state queries
- proof-of-work gated deposit-address requests
- HD mnemonic and `BIP84` note-path UX
- outbound-fee buckets and anonymous node fee-claim proofs

The old plan was still directionally correct about node runtime, durable state, Bitcoin integration, and FROST signing. It was incomplete on product-level governance, economics, client distribution, and the exact privacy model.

## What The Target System Now Requires

The Thornado target is:

- a threshold-controlled Bitcoin vault
- a state machine for deposit, split, withdraw, churn, penalty, auction, and shutdown events
- a fungible fixed-denomination note system
- privacy based on denomination fungibility plus user-controlled delay
- key rotation by churn, not privacy churn of the pooled funds

That last point matters. The earlier briefing assumed internal churn for privacy. The Thornado PDF explicitly says funds are churned for TSS keyset change, not as the main anonymity mechanism.

## Where The Repo Stands Today

This repository is still a single-process cryptography library.

It currently has:

- local scalar and group abstractions
- Pedersen-style commitment machinery
- a Sigma/Fiat-Shamir proof framework
- a local `CredentialIssuer`
- a local `WabiSabiClient`

It does not currently have:

- a node runtime
- a state machine service
- FROST
- `bitcoind` integration
- note lifecycle state
- slot market logic
- shutdown logic
- mirror or light-node infrastructure
- outbound fee bucket claims

It also still has baseline correctness work to finish in the current single-node library before it is safe to layer a distributed system on top.

## What Is Still Reusable

The most reusable parts of NWabiSabi-Rust for Thornado are:

- scalar and group arithmetic
- commitment and proof abstractions
- transcript machinery
- request and response serialization patterns

The least reusable part may be the current single-issuer MAC flow if Thornado ends up using a note-entitlement proof system that is simpler than thresholdizing the full WabiSabi issuer model.

## Critical Design Change From The Earlier Plan

The earlier plan assumed the hardest long-term step would be threshold credential issuance for the current WabiSabi issuer.

That is no longer the only plausible path.

To match Thornado, there are now two realistic proof-layer directions:

1. Reuse the current WabiSabi-style credential machinery and eventually distribute it.
2. Use NWabiSabi-Rust mainly as proof and commitment infrastructure while designing a Thornado-specific note-entitlement and note-redemption proof model.

The PDF does not force option 1. It only requires a proof that a user is entitled to denomination notes from a deposit and can later redeem a note without live ownership linkage.

That means the engineering plan should not commit prematurely to thresholdizing the current `CredentialIssuer` if a better Thornado-native note model exists.

## Required Workstreams

### 1. Finish And Stabilize The Current Library

Before building Thornado-specific subsystems, the existing library must be made internally consistent and test-clean.

That includes:

- fixing the current proof-system failures
- completing the missing credential presentation statement path
- finishing client-side credential bookkeeping
- cleaning up invalid or placeholder tests

This remains the gating Phase 0 work.

### 2. Define The Thornado State Machine

The next missing layer is not FROST itself. It is the Thornado state machine.

That state machine must model:

- deposit-address issuance
- proof-of-work admission for address requests
- deposit confirmation
- conversion from deposit to denomination-note set
- note redemption
- outbound fee charging
- fee bucket accumulation and locking
- churn epoch transitions
- node penalties
- slot handoff after auction
- shutdown vote and maintenance mode

This subsystem does not exist in the repo today.

### 3. Add The FROST Vault Runtime

FROST is still the correct primitive for the custody layer.

Required work:

- choose and integrate a maintained secp256k1 FROST implementation
- implement DKG
- persist key shares and epoch metadata
- support a `67%` signing quorum
- model one standby node per churn cycle
- rotate the active keyset on the churn interval
- sign deposit sweeps, withdrawals, and churn transactions

The standby-node rule is a policy layer on top of FROST, not a built-in FROST feature, so it needs explicit runtime logic.

### 4. Add Bond, Slot, and Auction Subsystems

The Thornado design is not just a signer committee. It is a bonded slot economy.

Required work:

- encode permanent slot identities
- represent the `10 + 2.5n BTC` bond rule
- prevent normal bond withdrawal
- implement slot auction lifecycle
- implement churn-out and churn-in for slot transfer
- apply the `5%` developer-fund rule on slot sales
- track offline penalties per churn cycle
- remove or churn out nodes that miss an entire cycle

This is a dedicated governance and accounting subsystem.

### 5. Add The Note Model

The user flow in the PDF is note-centric.

Required work:

- generate an HD mnemonic in the client
- map note indices to `BIP84` derivation paths
- split deposits into fixed denominations
- define the proof that a deposit entitles the user to a set of notes
- define the proof that a note is being redeemed validly
- enforce note fungibility by denomination
- ensure withdrawal submission is gasless from the user's perspective

This is the place where the repo may either:

- reuse WabiSabi-style credentials, or
- pivot to a Thornado-native note proof system built on the repo's proof primitives

That design choice should be made early.

### 6. Add Client Mirrors And Light Nodes

The PDF adds a client-distribution model that the earlier plan did not cover.

Required work:

- define mirror identity and metadata
- define node vouching rules for mirrors
- pin client releases to a known content hash
- add weekly health checks
- assign penalty points for unavailable mirrors
- define the GitHub registry and release-hash workflow
- define light-node API surface for transaction broadcast and state queries

This is product and infrastructure work, not just cryptography work.

### 7. Add Bitcoin And `bitcoind` Integration

The node runtime needs a real Bitcoin backend.

Required work:

- deposit address derivation from the FROST group key or related script policy
- UTXO monitoring
- transaction construction
- mempool and confirmation tracking
- withdrawal broadcast
- churn transaction construction for keyset changes
- `bitcoind` connectivity and operational management

The PDF explicitly assumes `Node + BitcoinD + TSS`.

### 8. Add Fee Bucket Claims

The Thornado fee design is more specific than the old plan.

Required work:

- outbound-only fee charging
- fee bucket accumulation
- bucket locking
- anonymous node claim proofs of the form "I am a node and I have not claimed this bucket"
- batching across multiple buckets

This is a nontrivial cryptographic and state-accounting subsystem. It was not in the earlier plan and should be treated as a major standalone work item.

### 9. Add Governance And Shutdown Logic

The shutdown path is explicit in the PDF and needs implementation.

Required work:

- `67%` shutdown vote tracking
- `30-day` maintenance mode
- deposit disablement during maintenance
- principal return to nodes
- equal split of fees and abandoned deposits under shutdown rules
- governance controls for outbound fee size, churn interval, and fee bucket size

Without this, the system does not match the product specification.

### 10. Add Adversarial And Distributed Testing

The test matrix must expand to cover the new Thornado-specific behaviors:

- FROST DKG and signing under dropout
- standby-node rotation
- churn-epoch transitions
- slot auction and handoff correctness
- offline penalty accrual
- mirror-vouching and health-check logic
- fee bucket lock and anonymous claim correctness
- shutdown vote and maintenance mode
- note split and redeem end-to-end flows

## Recommended Delivery Order

### Phase 0: Stabilize NWabiSabi-Rust

Finish the current library and make `cargo test` green.

### Phase 1: Choose The Note Proof Model

Decide whether Thornado will:

- extend the current WabiSabi issuer model, or
- use Thornado-native note entitlement proofs built from the repo's lower-level proof machinery

This choice affects everything above it.

### Phase 2: Build A Single-Node Thornado State Machine

Before committee distribution, implement the deposit, split, withdraw, fee, and note state transitions locally against one signing backend.

### Phase 3: Add The FROST Vault

Introduce DKG, key shares, threshold signing, and churn-based key rotation.

### Phase 4: Add Slot Economics And Governance

Implement bonds, slot auctions, penalties, and shutdown.

### Phase 5: Add Mirrors, Light Nodes, And PoW Gating

Build the public-facing client-distribution and request-admission model from the PDF.

### Phase 6: Add Fee Buckets And Anonymous Claims

Complete the node-income design.

### Phase 7: Hardening And Audits

Add operational tooling, observability, property tests, fuzzing, and external review.

## Likely Code Changes In This Repo

The repo changes are now broader than the earlier plan assumed.

Likely new areas:

- `src/state_machine/` for deposit, note, churn, auction, penalty, and shutdown logic
- `src/mpc/` for FROST sessions, DKG, epoch rotation, and active/standby selection
- `src/bitcoin/` for `bitcoind` integration and transaction logic
- `src/note/` for denomination notes, derivation, and redemption proof types
- `src/governance/` for bonds, slots, auctions, penalties, and shutdown
- `src/mirror/` for mirror registry, vouching, and health checks
- `src/light_node/` for lightweight client-facing broadcast and query services

Existing files that still matter:

- [`src/credential_issuer.rs`](src/credential_issuer.rs)
- [`src/wabisabi_client.rs`](src/wabisabi_client.rs)
- [`src/zero_knowledge/`](src/zero_knowledge)
- [`src/crypto/`](src/crypto)

Those existing modules are still the likely substrate for the proof layer, even if the top-level Thornado product no longer maps one-to-one onto the current issuer API.

## Main Risks

The main risks are now broader than "thresholdize the issuer."

They are:

- choosing the wrong note proof model too early
- overfitting the design to the current single-issuer API
- under-specifying the state machine for churn, penalties, and shutdown
- building FROST correctly but leaving governance and slot logic underspecified
- leaking information through note redemption timing, client mirrors, or light-node telemetry
- designing fee bucket claims that are either linkable or economically gameable

## Bottom Line

To match `THORNADO.pdf`, the project is no longer just "NWabiSabi plus FROST."

It is:

1. a FROST vault,
2. a Thornado-specific state machine,
3. a bonded slot economy,
4. a note-entitlement and note-redemption proof system,
5. a client mirror and light-node distribution network,
6. a governance and shutdown framework.

NWabiSabi-Rust can still be part of the proof layer, but the product shape is now much larger than the earlier generic plan described.
