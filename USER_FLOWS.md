# User Flows

This document describes the operational flows for Thornado users and node operators. The technical component design lives in `ARCHITECTURE.md`.

## User Flow

### 1. Open Client

The user opens a vouched client mirror. The client verifies or displays the expected release hash, connects to a light node, and loads public Thornado state.

### 2. Create Or Restore Wallet

For a new wallet, the client generates a mnemonic locally. For recovery, the user enters the mnemonic locally. The mnemonic never leaves the client.

The client derives deposit branches, note secrets, blinding material, and nullifiers from the mnemonic using domain-separated private derivation.

## Mnemonic System

The mnemonic is the user's root recovery secret. It is not an account identifier, login credential, server-side profile, or public wallet key. The client uses it as local entropy for every deposit and note object, while Thornode only sees one-use protocol material and public commitments.

The design target is:

```text
mnemonic
  -> wallet seed
  -> deposit branch secrets
  -> note secrets
  -> note blinding material
  -> nullifier secrets
  -> local scan tags
```

Each derivation uses domain separation. A deposit branch secret must not be reusable as a note secret, a nullifier secret, or a scan tag. Domain labels should include network and pool identifiers so testnet, mainnet, and future pools do not collide.

### Deposit Branches

For each deposit, the wallet increments or selects a local deposit index and derives a fresh branch secret. That branch is one-use. It creates the request material needed to ask Thornode for a deposit address, but the branch index and any reusable public root stay private.

Thornode may record a deposit intent, proof-of-work token, deposit address, amount, confirmation status, and local scan tag commitment. It must not learn enough to connect that deposit branch to the user's other branches.

### Note Children

After a deposit confirms, the wallet decomposes the amount into denomination notes. For each note, the wallet derives child material under the deposit branch:

```text
note_secret = derive(seed, "note-secret", network, pool, deposit_index, note_index)
note_blind = derive(seed, "note-blind", network, pool, deposit_index, note_index)
nullifier_secret = derive(seed, "nullifier", network, pool, deposit_index, note_index)
```

The public note commitment is computed from the note secret, denomination, and blinding material. The nullifier is derived separately and is revealed only when that note is spent. Public state sees commitments and later nullifiers; it should not reveal the deposit index, note index, sibling set, or mnemonic-derived key path.

### Local Metadata

The client may cache derived branches, note labels, proof artifacts, and scan progress for speed. That cache is convenience data only. Losing it must not lose funds.

Any cached metadata should be encrypted at rest when possible. A compromised cache should not reveal the mnemonic, raw note secrets, or unused nullifiers.

### Recovery Scan

A restored wallet starts from the mnemonic and scans forward through candidate deposit branches. For each candidate branch it reconstructs the expected deposit request material or scan tag, finds matching confirmed deposits in public state, recomputes the denomination split, derives note children, and checks denomination trees for matching commitments.

The wallet then derives each note's nullifier and checks the public spent-nullifier set. Notes with no matching spent nullifier are spendable. Notes with matching spent nullifiers are shown as spent.

Recovery stops only after a conservative gap limit. The gap limit is a client scanning rule, not a protocol secret; it prevents endless scanning while still allowing users to recover after unused deposit branches.

### Non-Goals

The mnemonic system must not require:

- a server account;
- an exported xpub;
- a reusable public wallet address;
- a saved note file;
- a withdrawal receipt;
- a local database backup;
- a trusted mirror remembering user state.

### 3. Request Deposit Address

The wallet selects a fresh one-use deposit branch and performs the required proof of work. The client submits a deposit-address request containing only one-use deposit material.

Thornode records the deposit intent and returns a Bitcoin deposit address. The request must not expose a reusable account key, mnemonic-derived public root, raw branch secret, or branch index.

### 4. Deposit BTC

The user sends BTC to the returned address. Bifrost observes the deposit, waits for the required confirmations, and submits the observation to Thornode. Thornode marks the deposit confirmed once consensus accepts the observation.

### 5. Split Into Notes

The wallet decomposes the confirmed amount into fixed denominations, such as `10`, `1`, `0.1`, and `0.01` BTC units where supported by policy.

The client proves that the confirmed deposit value authorizes the requested denomination notes. Thornode accepts only valid split transitions. Final public state should contain denomination-pool note commitments, not a public mapping from the deposit to its notes.

The wallet derives each note child locally before commitment insertion. Commitments from the same split should be inserted in a way that avoids exposing sibling order or batch membership through protocol metadata.

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
