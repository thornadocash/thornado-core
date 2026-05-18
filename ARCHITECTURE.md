# Architecture

## Status

Canonical design document aligned to `THORNADO.pdf`.

This document is the top-level summary for the Thornado design in this repository. When design notes disagree, this file is authoritative.

Related documents:

- [`TECHNICAL_BRIEFING_DISTRIBUTED_CUSTODIAL_MIXER.md`](TECHNICAL_BRIEFING_DISTRIBUTED_CUSTODIAL_MIXER.md) — product and governance briefing
- [`FROST_MPC_NETWORK_INTEGRATION_PLAN.md`](FROST_MPC_NETWORK_INTEGRATION_PLAN.md) — implementation roadmap
- [`BFT_CONSENSUS_DESIGN.md`](BFT_CONSENSUS_DESIGN.md) — replicated-state requirements
- [`THRESHOLD_MPC_STACK_ANALYSIS.md`](THRESHOLD_MPC_STACK_ANALYSIS.md) — cryptographic surfaces
- [`TORNADO_STYLE_ARCHITECTURE.md`](TORNADO_STYLE_ARCHITECTURE.md) — note and withdrawal architecture

This architecture is conceptual. None of it is implemented in the current Rust library.

## One-Paragraph Summary

Thornado is a `67%` FROST-controlled Bitcoin vault with a replicated state machine, permanent bonded slots, periodic churn for TSS key rotation, fixed-denomination withdrawal notes, client mirrors, and light nodes. Users create an HD mnemonic locally, derive fresh unlinkable deposit branches, deposit any amount of BTC, split confirmed deposits into blinded denomination authorizations, and later mint deterministic note commitments into denomination pools. The mnemonic is the user's only required recovery secret: a restored client derives candidate deposit branches and note children, scans public state for matching commitments and nullifiers, and withdraws from recovered unspent notes. Privacy comes from never exposing a stable user key, never publishing a deposit-to-note mapping, hiding note owner keys inside commitments, and user-controlled time between deposit, mint, and withdrawal. The committee's churn mechanism exists for keyset rotation and operator turnover, not as the primary privacy mechanism.

## System Layers

```
┌──────────────────────────────────────────────────────────────┐
│  CLIENT LAYER       Mirrors, HD mnemonic, state scan, PoW    │
├──────────────────────────────────────────────────────────────┤
│  NOTE LAYER         Deposit -> split -> withdraw proofs      │
├──────────────────────────────────────────────────────────────┤
│  STATE LAYER        Deposits, notes, churn, slots, fees,     │
│                    penalties, shutdown                       │
├──────────────────────────────────────────────────────────────┤
│  CUSTODY LAYER      FROST threshold signing for Bitcoin      │
├──────────────────────────────────────────────────────────────┤
│  BITCOIN LAYER      Deposit UTXOs, withdrawals, churn txs,   │
│                    bitcoind integration                      │
└──────────────────────────────────────────────────────────────┘
```

## Core Invariants

- No single node can move funds unilaterally.
- Churn exists for key rotation and slot turnover, not for primary privacy.
- Privacy is weak if the user deposits and withdraws immediately.
- The committee must not see a stable user public key across deposits.
- Public state must not reveal which notes are siblings or which deposit created a note.
- Node slots are permanent and bonded.
- Bonds remain inside the system during normal operation.
- Shutdown is an explicit governance action, not an operator escape hatch.
- Client mirrors must be vouched for and content-pinned.
- Notes are fungible by denomination.
- The user's HD mnemonic is the only required recovery secret.
- No local-only note, voucher, authorization, or receipt may be required for recovery.

## Main Objects In State

- `Slot`
- `Bond`
- `ChurnEpoch`
- `StandbyNode`
- `DepositIntent`
- `ObservedDeposit`
- `ClaimedDeposit`
- `Note`
- `WithdrawalRequest`
- `FeeBucket`
- `MirrorRegistration`
- `PenaltyRecord`
- `ShutdownVote`

## Committee Model

- The signing threshold is `67%`.
- One node may remain in standby for a churn cycle.
- Churn happens on a fixed interval, currently `7 days`.
- Offline nodes lose observation and signing rights for the missed cycle.
- Offline nodes pay a `1%` penalty for a fully missed churn cycle.
- Slots can be sold in an auction market and handed over through churn-out / churn-in.

## User Flow

### 1. Request Deposit Slot

- Client mirror generates an HD mnemonic locally.
- User performs proof of work before requesting a deposit address.
- Request includes fresh one-use deposit material derived from the mnemonic and a local deposit index.
- Committee returns a unique deposit address and deposit intent tied only to that one-use deposit key.

### 2. Deposit

- User deposits any amount of BTC.
- Committee observes and confirms the deposit in replicated state.

### 3. Split

- Client converts the confirmed amount into fixed-denomination notes.
- Example denominations include `10`, `1`, `0.1`, `0.01`, and similar descending powers.
- Client derives branch/child note keys locally, largest denomination to smallest.
- Split creates blinded denomination authorizations, not a public `deposit -> notes` record.
- Blinded authorizations are recorded or retrievable in a form recoverable from the mnemonic.
- Final note commitments hide their child owner keys and are inserted into denomination pools without exposing sibling linkage.

### 4. Withdraw

- User presents a note proof and a withdrawal address.
- Committee validates the request in state and threshold-signs the outbound transaction.
- Submission is intended to be gasless from the user's perspective.

### Recovery

- The HD mnemonic is the only required user backup.
- A restored client derives candidate one-use deposit branches and child note commitments, scans chain and Thornado state for matches, checks spent nullifiers, and rebuilds the spendable note set.
- Cached note metadata is allowed for speed, but must be reproducible from the mnemonic plus public chain and system state.

## Privacy Model

- Deposit amount and deposit address are known to the system.
- Deposit requests from the same user must not share a stable public identifier.
- Note commitments from the same split must not share a public batch, branch, child, or sibling identifier.
- The user later proves entitlement to withdraw a denomination note.
- Notes are intended to be fungible with all other notes of the same denomination.
- The privacy gain is determined mainly by denomination set size and the delay between deposit and withdrawal.

## Fee Model

- Fees are charged only on outbound activity.
- The reference outbound handling fee in the PDF is `100,000 sats`.
- Fees accumulate into buckets.
- Node operators claim bucket payouts through an anonymous proof flow.

## Governance

- `67%` of nodes can vote to shut down the system.
- Shutdown enters a `30-day` maintenance mode.
- No new deposits are accepted during maintenance mode.
- Principal is returned to nodes.
- Fees and abandoned deposits are split equally under the shutdown rule.

## Mirror and Light-Node Model

- Nodes vouch for mirrors using SSL and known content hashes.
- Mirrors are checked weekly.
- Mirror downtime contributes to penalties.
- Clients attach to light nodes for broadcast and state queries.
- Release distribution should use a registry and pinned release hash.

## Mapping to This Repository

The current repository provides useful cryptographic building blocks, but not the Thornado system itself.

What exists today:

- local proof machinery
- local cryptographic primitives
- a single-process issuer/client library

What Thornado still needs:

- FROST vault runtime
- replicated state machine
- Bitcoin integration
- note proof model
- slot and bond subsystem
- mirror and light-node infrastructure
- fee bucket claims
- shutdown and penalty logic

## Immediate Implication

The target architecture is larger than "distributed WabiSabi." Thornado is a FROST vault plus a note state machine plus a slot economy plus a client-distribution network.
