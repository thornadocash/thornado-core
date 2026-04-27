# Technical Briefing: Thornado Cash

## Title

**Thornado Cash: A 67% FROST Vault with ZK Note Splitting, Permanent Bonded Slots, Client Mirrors, and Light Nodes**

**Version:** 0.2 (Conceptual Design)  
**Date:** April 27, 2026  
**Status:** Updated to match `THORNADO.pdf`

This file keeps its original name for continuity, but the earlier "distributed custodial mixer" framing is superseded by the Thornado design described here.

## 1. Executive Summary

Thornado Cash is a threshold-controlled Bitcoin vault and state machine built around a bonded node committee. Users deposit Bitcoin, convert the deposit into fixed-denomination withdrawal notes, and later redeem those notes to arbitrary withdrawal addresses. The system uses a `67%` FROST signing quorum, permanent bonded node slots, a periodic churn process for keyset rotation, client mirrors for user access, and light nodes for transaction broadcast and state queries.

The intended privacy model is not "funds are constantly remixed by the protocol." Privacy comes from denomination fungibility plus user-controlled distance in time between deposit and withdrawal. If a user withdraws immediately after deposit, they should expect little privacy benefit.

The intended custody model is also narrower than a traditional custodian. Nodes cannot unilaterally move funds. Control is threshold-based, and keyset changes happen through the churn process.

## 2. Core System Model

- A `67%` FROST vault controls pooled Bitcoin under a shared state machine.
- Node slots are permanent and bonded.
- One node may remain in standby during a churn cycle.
- Churn happens on a fixed interval, currently specified as `7 days`.
- Funds are churned for TSS keyset change and committee rotation, not as the primary privacy mechanism.
- Users follow a `Deposit -> Split -> Withdraw` lifecycle.

## 3. Nodes, Slots, and Bonds

### 3.1 Committee Structure

- The committee operates a threshold vault using a `67%` signing threshold.
- One node can remain in standby during a churn cycle.
- Nodes that are offline for an entire churn cycle do not provide observations or TSS signatures for that cycle.

### 3.2 Bond Model

- Each node posts a bond of `10 + 2.5n BTC`, where `n` is the slot index or slot-dependent parameter used by the system design.
- The bond does not leave the system during normal operation.
- Bond principal is only expected to leave the system during shutdown.

### 3.3 Slot Transfer and Rotation

- Node slots are permanent.
- Node operators can sell slots in an auction market.
- Once a highest bidder is selected, the old node churns out and the new node churns into that slot.
- Nodes can be churned out for being offline for an entire churn cycle.

### 3.4 Penalties and Treasury

- Offline nodes pay a `1%` penalty for a full missed churn cycle.
- The reference note in the PDF states this compounds to roughly `37%` annual loss.
- A `5%` developer fund is taken from node slot sales.

## 4. Client Mirrors and Light Nodes

### 4.1 Client Mirrors

- All nodes must vouch for a client mirror using SSL and a known content hash.
- The goal is to admit mirrors that reproduce a known non-scam client build.
- If active nodes can validate a mirror, it can join the mirror set.
- Mirrors are checked weekly.
- Mirrors that are down can cause penalty points to accrue to the responsible node set or operator set, depending on the final governance design.

### 4.2 Light Nodes

- The client is attached to a light node.
- The light node broadcasts transactions and queries the Thornado state machine.

### 4.3 Distribution Model

- A landing page links out to mirrors and social discovery channels.
- Client mirrors and light nodes should be deployable with minimal operational friction.
- The release artifact set should be tied to a GitHub registry and release hash.

## 5. User Experience

### 5.1 Wallet and Request Flow

- The user opens a client mirror.
- The client generates an HD mnemonic.
- The user performs a proof of work before requesting a deposit address.
- The proof-of-work target is intended to be around `30 seconds` on an iPhone or browser-class device.

### 5.2 Deposit and Split

- The user deposits any amount of Bitcoin.
- After the deposit is received and confirmed, the client converts the deposit into a set of fixed-denomination notes.
- Example denominations include `10`, `1`, `0.1`, `0.01`, and similar descending powers.
- Each note index is mapped onto an HD wallet path using `BIP84`.

### 5.3 Withdrawal

- The user later chooses a withdrawal address and submits a proof for a note.
- If the proof is valid, the withdrawal submission is intended to be gasless from the user's point of view, meaning no relayer fee model is required in the submission flow as currently described.

## 6. ZK and Note Model

- The deposit address and deposit amount are known to the system.
- The user proves that a deposit entitles them to a set of denomination notes.
- Once created, a note should be fungible with every other note of the same denomination.
- The withdrawal proof must establish entitlement to the note without tying the note back to the original deposit owner in the live withdrawal flow.

This is a different emphasis from the earlier blinded-note description. The Thornado PDF is centered on note entitlement proofs and fungible denomination notes, not on active privacy churn of the pool.

## 7. Privacy and Protection Model

- Privacy is attained by distance between deposit and withdrawal, controlled by the user.
- Immediate withdrawal provides little privacy.
- Nodes cannot unilaterally move funds.
- Nodes are regularly churned, enabling ongoing entry and exit.
- Entry and exit are intended to be permissionless, subject to slot and bond rules.
- Funds are not churned primarily for privacy; funds are churned for TSS keyset change.

## 8. Fees and Node Income

### 8.1 User Fees

- Fees are charged only on outbound flows.
- Fees are not charged as a function of transaction size in the current product sketch.
- The PDF uses `100,000 sats` as the reference outbound TSS handling fee.

### 8.2 Fee Buckets

- Fees accumulate into buckets.
- The reference design uses a `30 * 0.01 BTC` bucket model.
- A node operator signs for a bucket, which locks it.

### 8.3 Anonymous Node Claims

- Each node operator signs for a `0.01 BTC` note per bucket.
- The intended proof statement is effectively: "I am a node and I have not yet claimed in this bucket."
- The claim mechanism should avoid revealing which specific operator claimed or whether a specific operator already claimed.
- Node operators can batch sign or batch claim across multiple buckets.

This part of the design is still a conceptual cryptographic sketch and will need formalization.

## 9. Governance and Shutdown

### 9.1 Parameter Governance

The current design identifies at least these tunable parameters:

- outbound fee size
- churn interval
- fee bucket size

### 9.2 Shutdown

- `67%` of nodes can vote to shut down the system.
- Shutdown enters a `30-day` maintenance mode.
- No further deposits are accepted during maintenance mode.
- Principal is returned to nodes.
- Fees and abandoned deposits are split equally under the shutdown rule described in the PDF.

## 10. Network Stack

The intended runtime stack is:

- landing page and mirror-discovery surface
- client mirrors plus light nodes
- node runtime plus `bitcoind` plus TSS/FROST services

## 11. Mapping to NWabiSabi-Rust

NWabiSabi-Rust remains relevant as a source of proof machinery, commitment logic, and protocol structure, but Thornado is now broader than the earlier framing in this repository's added docs.

The key difference is that Thornado is defined first as:

- a FROST vault
- a state machine for deposits, notes, withdrawals, churn, and shutdown
- a mirror and light-node distribution model
- a bonded slot economy

The exact way NWabiSabi credential machinery fits into the note-entitlement proof layer remains an implementation decision.

## 12. Open Implementation Questions

- How should the `67%` threshold map onto exact committee sizes and standby policy?
- How should the `10 + 2.5n BTC` bond schedule be formalized?
- What exact ZK statement should prove deposit-to-note entitlement?
- How should bucket claims be implemented without leaking operator identity or claim status?
- How should weekly mirror validation and penalty assignment be encoded on-chain or in committee state?
- What exact state-machine transitions govern churn, auction settlement, penalties, and shutdown?

These are the main areas where the PDF defines the product shape but not the full protocol details.
