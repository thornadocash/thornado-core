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

## User Secrets

The client generates an HD mnemonic locally.

Each note is tied to:

- a denomination
- a note index
- a `BIP84` derivation path
- whatever local secret material is needed for the later proof system

The user's receipt bundle is therefore local wallet material plus note metadata, not a committee-held account.

## Lifecycle

### 1. Deposit-Address Request

- User opens a client mirror.
- Client generates mnemonic locally.
- User performs proof of work.
- Committee returns a deposit address and deposit intent.

### 2. Deposit

- User sends BTC to the returned address.
- Committee observes and confirms the deposit.

### 3. Split

- Client converts the confirmed amount into fixed-denomination notes.
- Client produces the data required to establish entitlement to those notes.
- Committee validates and records the split in state.

### 4. Withdraw

- User chooses one note and one withdrawal address.
- User submits a proof of entitlement for that note.
- Committee validates the request and threshold-signs the outbound transaction.

Each note can be spent independently and on its own schedule.

## Privacy Model

- The system may know the deposit address and deposit amount.
- The system should not learn the live linkage from a withdrawn note back to the user identity behind the deposit.
- Notes of the same denomination should be fungible.
- Immediate withdraw after deposit provides weak privacy.
- Waiting across multiple churn cycles and larger note sets improves the anonymity picture.

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
- exact client backup format
- exact way the committee validates split and withdraw proofs

## Relation To NWabiSabi-Rust

NWabiSabi-Rust may still contribute proof machinery, but this document does not assume the current WabiSabi issuer API is the final note architecture.

The point of this file is to pin the product behavior first:

- notes
- denominations
- user-controlled delay
- independent withdrawals

The proof construction can be chosen after that.
