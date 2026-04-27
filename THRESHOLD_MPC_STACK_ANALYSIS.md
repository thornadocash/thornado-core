# Threshold MPC Across the Thornado Stack

## Status

Conceptual cryptographic surface analysis aligned to `THORNADO.pdf`.

Companion to:

- [`ARCHITECTURE.md`](ARCHITECTURE.md)
- [`TECHNICAL_BRIEFING_DISTRIBUTED_CUSTODIAL_MIXER.md`](TECHNICAL_BRIEFING_DISTRIBUTED_CUSTODIAL_MIXER.md)
- [`FROST_MPC_NETWORK_INTEGRATION_PLAN.md`](FROST_MPC_NETWORK_INTEGRATION_PLAN.md)

## TL;DR

The Thornado design does not require "thresholdize every secret in the current WabiSabi issuer."

Its main cryptographic surfaces are:

1. Bitcoin transaction signing
2. DKG and epoch resharing
3. Note-entitlement and note-redemption proofs
4. Anonymous node fee-bucket claim proofs
5. Admission and integrity controls around mirrors and request gating

Only the first two are clearly FROST-native. The rest are proof-system and state-machine design problems.

## Surface 1: Bitcoin Custody

**What it is:** threshold control of pooled Bitcoin.

**Primitive:** FROST on `secp256k1`.

**Needed for:**

- deposit sweeps
- withdrawals
- churn transactions
- any bond or shutdown fund movement

**Maturity:** high.

This is the cleanest and most obvious MPC surface in Thornado.

## Surface 2: DKG and Resharing

**What it is:** committee key setup and churn-based key rotation.

**Primitive:** standard DKG plus proactive resharing for the chosen FROST stack.

**Needed for:**

- initial committee bring-up
- `7-day` churn epochs
- slot transfer handoff
- standby-node and active-set changes

**Maturity:** high, but operationally sensitive.

## Surface 3: Note Proofs

**What it is:** the proof that a deposit entitles the user to a set of fixed-denomination notes, and the later proof that a note can be redeemed.

**Primitive:** not fixed by the PDF.

The Thornado PDF requires:

- fixed-denomination notes
- note fungibility by denomination
- a ZK proof of withdrawal entitlement
- gasless user submission semantics

It does not require a specific proving system, blind-signature system, or credential scheme.

That means the project still needs to choose between at least two paths:

- reuse part of the current NWabiSabi/WabiSabi credential machinery
- design a Thornado-native note proof system with different primitives

This is the biggest unresolved cryptographic design choice in the stack.

## Surface 4: Anonymous Fee-Bucket Claims

**What it is:** the node-operator claim flow for outbound fee buckets.

The PDF requires a proof with semantics close to:

> I am a node and I have not yet claimed in this bucket.

While also not revealing:

- which operator claimed
- whether a specific operator already claimed

This is not the same problem as note withdrawal and not the same problem as FROST signing.

It likely needs:

- anonymous membership proof
- one-claim-per-bucket enforcement
- batch-claim support

This is a major cryptographic subsystem and should be treated that way.

## Surface 5: Client Integrity and Admission

This is not threshold cryptography in the narrow sense, but it is part of the trust surface.

The PDF requires:

- proof-of-work gating before deposit-address issuance
- mirrors vouched by nodes
- SSL and known content hash
- weekly mirror validation

These features exist to control abuse and reduce phishing or scam-surface risk. They should be treated as first-class protocol surfaces, even if they are not MPC primitives.

## Optional Surface: Thresholdize The Current WabiSabi Issuer

This remains optional under the Thornado PDF.

If the final note model chooses to reuse the current WabiSabi issuer flow, then the project reopens the earlier hard problems:

- threshold MAC issuance
- threshold issuance proofs
- possibly distributed blind or voucher issuance

If the final note model does not use the current issuer API, those surfaces disappear from the critical path.

That is why the updated implementation plan no longer assumes threshold credential issuance is mandatory.

## What This Means For The Repo

The current repository is strongest as:

- a proof substrate
- a cryptographic utility layer
- a local protocol sandbox

It is not yet a Thornado runtime, and the current `CredentialIssuer` should not be treated as a fixed center of the final architecture.

## Main Risks

- choosing a note proof design too early
- assuming FROST solves the privacy layer
- underestimating the fee-bucket claim cryptography
- treating mirror attestation and PoW admission as mere UI features instead of protocol surfaces

## Bottom Line

The Thornado stack is narrower than "thresholdize all of WabiSabi" in one sense and broader in another.

It is narrower because FROST only has to own custody for sure.

It is broader because the product also needs:

- a note proof system
- fee-claim proofs
- mirror attestation
- churn and slot governance

Those are the actual cryptographic and protocol surfaces now implied by `THORNADO.pdf`.
