# Thornado-Style Note Architecture

## Status

Conceptual note and withdrawal architecture aligned to `THORNADO.pdf`.

This document focuses on the privacy and note lifecycle only. It does not choose a final proof system implementation.

## Core Idea

A user deposit is converted into a set of fixed-denomination notes. Each note is later redeemable independently. Privacy comes from the fact that notes are fungible by denomination and from the user choosing meaningful distance between deposit and withdrawal.

The committee does not need to provide privacy by constantly remixing funds. The committee needs to:

- confirm deposits
- authorize note creation
- track note spends
- threshold-sign withdrawals

## Denomination Model

The note ladder is fixed.

Example denominations:

- `10 BTC`
- `1 BTC`
- `0.1 BTC`
- `0.01 BTC`

Any deposit is greedily decomposed into notes across that ladder.

Example:

`12.34 BTC = 1×10 + 2×1 + 3×0.1 + 4×0.01`

Each note is an independent withdrawal object.

## Privacy Invariants

The user's HD mnemonic is the only required backup, but it is only a local root secret. The protocol must never expose the mnemonic, a master public key, an xpub, a reusable account key, a public branch index, or any other stable user identifier.

Every visible object must look independent:

- deposits from the same mnemonic are unlinkable
- denomination notes from the same deposit are unlinkable
- notes at adjacent branch, child, or index positions are unlinkable
- a withdrawal nullifier is unlinkable to its note commitment until the note is spent
- public state must not contain a `deposit -> notes` or `note -> sibling notes` mapping

This is the core rule:

```text
deterministic to the wallet
pseudorandom to everyone else
```

## Deterministic Note Wallet

The wallet derives all secrets from the mnemonic with domain-separated private derivation. Public branch and child indexes are forbidden.

```text
root = BIP39Seed(mnemonic)

deposit_secret[i] = PRF(root, "thornado/deposit", network, pool, i)
deposit_scan_tag[i] = H("thornado/deposit-scan", deposit_secret[i])

note_secret[i][j] = PRF(root, "thornado/note-key", network, pool, i, j)
note_blind[i][j] = PRF(root, "thornado/note-blind", network, pool, i, j)
nullifier_secret[i][j] = PRF(root, "thornado/nullifier", network, pool, i, j)

note_owner_pubkey[i][j] = Pub(note_secret[i][j])
note_commitment[i][j] = Commit(denomination, note_owner_pubkey[i][j], note_blind[i][j])
nullifier[i][j] = PRF(nullifier_secret[i][j], "spend")
```

The note is committed to the key at its branch, child, and index, but the public state sees only the hiding commitment. It must not see the raw note owner public key, branch index, child index, or sibling set.

## Private Split And Mint

After a deposit confirms, the client splits the amount locally from largest denomination to smallest. The note count is bounded by the denomination ladder and the maximum supported deposit size.

The split must not publish final note commitments as one grouped batch. A grouped split leaks sibling linkage even if the commitments themselves are hiding.

The target flow is:

1. User proves control of the confirmed deposit.
2. User proves, in zero knowledge, that requested denomination authorizations sum to the deposit value minus fees or change.
3. Committee issues blinded denomination authorizations.
4. User derives final note commitments offline from mnemonic branch and child indexes.
5. User redeems each blinded authorization to insert one final note commitment into the matching denomination pool.

Blinded authorizations must not become a second backup secret. They must be recorded or retrievable in public system state as blinded records, with any client-side blinding and unblinding material derived from the mnemonic. A user who restores before redeeming authorizations must be able to recover and redeem them from the mnemonic alone.

Redemption should use relays, batching, delays, or equivalent transport privacy so note insertions are not linked by request metadata. At the protocol layer, the final public state is only denomination-pool commitments and spent nullifiers.

## User Flow

### 1. Create Wallet

- User generates one HD mnemonic locally.
- No account is created on-chain.
- No stable public key leaves the client.

### 2. Request Deposit Address

- Wallet picks local deposit branch `i`.
- Wallet derives one-use deposit material for branch `i`.
- User performs proof of work.
- Committee returns a unique deposit address and deposit intent for that one deposit only.

### 3. Deposit

- User sends any amount of BTC to the returned address.
- Committee observes and confirms the deposit.

### 4. Split

- User clicks `split`.
- Wallet decomposes the amount into fixed denominations, largest to smallest.
- Wallet derives note child secrets for each denomination note.
- User proves deposit ownership and value conservation without revealing the final note sibling set.
- Committee returns blinded denomination authorizations.
- Authorization recovery material is mnemonic-derived; no separate receipt is required.

### 5. Commit Notes

- Wallet creates one hiding commitment per note using the branch and child key material.
- Each note commitment is inserted into its denomination pool.
- Public state cannot tell which notes came from the same mnemonic, deposit, split, branch, or child range.

### 6. Restore

- User enters only the HD mnemonic.
- Wallet derives candidate deposit branches and scan tags.
- Wallet finds matching deposits, recomputes the bounded denomination split, recovers any unredeemed blinded authorizations, derives child note commitments, and scans denomination trees.
- Wallet derives each note's nullifier and checks the public spent set.

### 7. Withdraw

- User chooses any recovered unspent note.
- Wallet proves membership in the denomination pool and knowledge of the hidden note key.
- Wallet reveals the note nullifier.
- Committee verifies the proof and threshold-signs the outbound Bitcoin transaction.

Each note can be spent independently and on its own schedule.

## Privacy Model

The protocol may expose a deposit address, deposit amount, denomination pool, note commitment, and spent nullifier. It must not expose any value that links those objects to a mnemonic or to each other as siblings.

The system must not learn:

- which deposits belong to the same user
- which note commitments came from the same deposit
- which notes are adjacent branch or child indexes
- which withdrawal came from which deposit
- any stable user public key or account identifier

Timing, IP addresses, browser fingerprinting, funding-wallet history, unusual amounts, and immediate withdrawal behavior are outside the cryptographic note format. The client and relay layer must handle those separately.

## What The PDF Fixes And What It Leaves Open

Fixed by the PDF:

- note-based architecture
- denomination ladder
- user-held wallet material
- proof-of-work before deposit-address issuance
- gasless withdrawal UX target

Still open:

- exact proof system
- exact note secret format
- exact spent-note marker model
- exact deterministic derivation format
- exact way the committee validates split and withdraw proofs

## Relation To NWabiSabi-Rust

NWabiSabi-Rust may still contribute proof machinery, but this document does not assume the current WabiSabi issuer API is the final note architecture.

The point of this file is to pin the product behavior first:

- notes
- denominations
- user-controlled delay
- independent withdrawals

The proof construction can be chosen after that.
