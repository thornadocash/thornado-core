# User Flows

This document describes the operational flows for Thornado users and node operators. The technical component design lives in `ARCHITECTURE.md`.

## User Flow

### 1. Open Client

The user opens a vouched client mirror. The client verifies or displays the expected release hash, connects to a light node, and loads public Thornado state.

### 2. Create Or Restore Wallet

For a new wallet, the client generates a mnemonic locally. For recovery, the user enters the mnemonic locally. The mnemonic never leaves the client.

The client derives deposit branches, note secrets, blinding material, and nullifiers from the mnemonic using domain-separated private derivation.

### 3. Request Deposit Address

The wallet selects a fresh one-use deposit branch and performs the required proof of work. The client submits a deposit-address request containing only one-use deposit material.

Thornode records the deposit intent and returns a Bitcoin deposit address. The request must not expose a reusable account key, mnemonic-derived public root, or branch index.

### 4. Deposit BTC

The user sends BTC to the returned address. Bifrost observes the deposit, waits for the required confirmations, and submits the observation to Thornode. Thornode marks the deposit confirmed once consensus accepts the observation.

### 5. Split Into Notes

The wallet decomposes the confirmed amount into fixed denominations, such as `10`, `1`, `0.1`, and `0.01` BTC units where supported by policy.

The client proves that the confirmed deposit value authorizes the requested denomination notes. Thornode accepts only valid split transitions. Final public state should contain denomination-pool note commitments, not a public mapping from the deposit to its notes.

### 6. Wait

The user chooses when to withdraw each note. Waiting improves the anonymity set because each note becomes harder to distinguish from other notes of the same denomination.

Immediate withdrawal is allowed only if policy permits it, but it provides weak privacy.

### 7. Withdraw

The user chooses an unspent note and a withdrawal address. The client proves note membership and ownership, reveals the note nullifier, and submits the withdrawal request through a light node or relay path.

Thornode verifies the proof, checks the nullifier is unused, records the spend, charges the outbound fee, and places the withdrawal in the outbound queue. Bifrost constructs the Bitcoin transaction. FROST signer sidecars produce partial signatures after local policy checks. Bifrost broadcasts the signed transaction.

### 8. Recover

A restored client derives candidate deposit branches and note children from the mnemonic, scans Bitcoin and Thornado state, finds matching deposits and note commitments, checks nullifiers, and rebuilds the spendable note set.

No extra receipt, voucher, local database, or server account should be required for recovery.

## Node Operator Flow

### 1. Prepare Infrastructure

An operator provisions Thornode, Bifrost, a Bitcoin backend, the FROST signer sidecar, persistent storage, monitoring, and optional mirror or light-node services.

The signer host must protect encrypted share storage and signing-session nonce state. Bitcoin and Thornode services need stable networking and durable disks.

### 2. Join Or Acquire Slot

The operator obtains a node slot according to system rules and posts the required bond. Thornode records the slot, bond, operator keys, network addresses, and service metadata.

If the slot is acquired from another operator, handoff completes through the slot-transfer and churn process rather than by informal key sharing.

### 3. Enter Churn

During churn, active membership changes and a new signer epoch may be created. The operator's signer sidecar participates in DKG, persists its encrypted share, and reports signer metadata.

Thornode finalizes the active set and vault epoch. Bifrost and signer sidecars then use that epoch for authorized vault operations.

### 4. Run Daily Operations

The operator keeps services online:

- Thornode participates in consensus and state transitions.
- Bifrost observes Bitcoin and reports deposits, solvency, and outbound status.
- The FROST signer answers authorized DKG and signing requests.
- Light-node and mirror services, if provided, respond to client traffic and health checks.

Missed observations, missed signatures, unavailable mirrors, or full-cycle downtime can produce penalties according to Thornode state.

### 5. Sign Outbounds

When Thornode approves an outbound transaction, Bifrost builds the Bitcoin signing payload and requests FROST signatures. Each signer sidecar checks the vault epoch, request expiry, chain, transaction policy, and session uniqueness before producing a partial signature.

The aggregate signature is used only for the approved Bitcoin transaction. Nonce material must never be reused for conflicting messages.

### 6. Claim Fees

Outbound fees accumulate into fee buckets. Operators claim from eligible buckets through the configured claim flow. The target claim design should prove eligibility without unnecessarily linking the claim to an operator identity.

### 7. Transfer Or Exit Slot

An operator exits by transferring the slot through the approved slot mechanism or by a governance-defined shutdown path. Normal operation should not allow unilateral withdrawal of bonded principal outside those rules.

Slot transfer triggers churn so the departing operator's signer share is removed from the active vault epoch and the incoming operator receives a fresh role in the next signer set.

### 8. Shutdown Mode

If governance enters shutdown, operators stop accepting new deposit work, keep recovery and withdrawal paths available as policy requires, participate in final signing tasks, and follow Thornode's principal and fee distribution rules.
