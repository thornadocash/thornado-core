# Architecture

This is the canonical technical architecture for Thornado. Other docs should describe local operation, generated APIs, or package-specific usage; they should not define a competing system design.

## Summary

Thornado is a Bitcoin privacy and custody system built from a replicated node state machine, Bitcoin observation and broadcast services, FROST-controlled vault signing, fixed-denomination private notes, and a client distribution layer. Users deposit BTC, split confirmed value into denomination notes, and later withdraw notes independently. Node operators run the consensus, Bitcoin, signer, and mirror infrastructure that keeps the system available.

Privacy comes from denomination pools, unlinkable note commitments, nullifier-based spends, and user-controlled delay between deposit, split, and withdrawal. Churn rotates vault keysets and operator membership; it is not the primary anonymity mechanism.

## Component Map

```text
Client / Wallet
  -> Mirror / Light Node
  -> Thornode State Machine
  -> Bifrost Bitcoin Observer
  -> FROST Signer Sidecars
  -> Bitcoin Network

Shared:
  -> Protobuf contracts
  -> Shielder proof engine
  -> Ops localnet
  -> Deterministic fixtures
```

## Repository Layout

- `crates/thornado-core`: Rust cryptography, note primitives, proof helpers, FROST engine bindings, and shared domain logic.
- `crates/thornado-node`: Rust node executable for local or experimental node workflows.
- `crates/thornado-abci`: ABCI boundary for replicated state integration.
- `crates/thornado-bitcoin`: Bitcoin transaction, script, address, and backend integration helpers.
- `crates/thornado-store`: persistence abstractions and local storage.
- `crates/thornado-cli`: operator and developer CLI.
- `crates/thornado-web-wasm`: browser-facing WASM package for wallet/proof workflows.
- `go-thornode`: Go consensus application and Bifrost fork used for replicated state, node lifecycle, queues, and Bitcoin observation.
- `rust-frost-signer`: local signer sidecar for FROST DKG, vault shares, signing sessions, and signer policy checks.
- `proto`: shared cross-component protobuf contracts.
- `circuits`: reference zero-knowledge circuit material.
- `ops`: localnet composition, scripts, mock services, and runbooks.
- `test-fixtures`: deterministic Bitcoin, FROST, and Shielder fixtures.

## Thornode State Machine

Thornode is the canonical state owner. It records deposits, note commitments, spent nullifiers, outbound queues, vault membership, signer epochs, slot ownership, bond accounting, penalties, fee buckets, mirror registrations, and shutdown state.

The state machine validates:

- deposit-address issuance and proof-of-work admission;
- Bitcoin deposit observations and finality;
- split authorization from confirmed deposit value into denomination notes;
- note commitment insertion into denomination pools;
- withdrawal proof validity and nullifier freshness;
- outbound queue selection and fee accounting;
- node churn, standby status, penalties, and slot transfers;
- vault epoch changes and signer set commitments;
- governance and shutdown transitions.

Consensus state must be deterministic. Transport details, local signer storage, Bitcoin RPC failures, and mirror availability checks may influence submitted messages, but the accepted state transitions must be reproducible by every node.

## Bitcoin And Bifrost

Bifrost owns Bitcoin chain integration. It watches deposit addresses and vault UTXOs, reports observations to Thornode, builds outbound transactions from approved queues, broadcasts signed transactions, tracks confirmations, and reports solvency.

Bitcoin responsibilities:

- derive or request deposit addresses from the active vault policy;
- monitor mempool and confirmed blocks;
- normalize UTXO observations into Thornode messages;
- construct withdrawal, sweep, and churn transactions;
- coordinate signing with FROST signer sidecars;
- rebroadcast and retry transactions as needed;
- report vault balances and stuck outbound state.

## FROST Custody Layer

Vault funds are controlled by a FROST threshold key. No single node can move funds. Thornode decides whether a transaction is authorized; signer sidecars decide whether their local policy permits producing a partial signature for that authorized request.

The signer sidecar owns:

- local participant identity;
- encrypted FROST shares;
- DKG session state and transcripts;
- nonce commitments and signing sessions;
- partial signatures and optional aggregation;
- local audit logs and share backup policy.

The signer does not own membership, slash/jail state, outbound queue selection, withdrawal authorization, Bitcoin observation, or note state.

The target threshold is `67%` of the active signer set. Churn creates a new signer epoch and vault keyset, then moves custody under the new epoch through explicit state-machine transitions and Bitcoin transactions.

## Note And Privacy Layer

Deposits are split into fixed-denomination notes. A note is represented publicly by a hiding commitment in a denomination pool. Spending a note reveals a nullifier, not the deposit or sibling notes that created it.

Required privacy invariants:

- no stable user public key is exposed;
- deposit branches are one-use;
- note commitments from the same split are unlinkable;
- public state does not contain a deposit-to-note mapping;
- nullifiers prevent double spend without revealing the original deposit;
- notes are fungible within the same denomination pool.

The wallet derives deposit and note secrets from a local mnemonic using domain-separated private derivation. Recovery must require only the mnemonic plus public Bitcoin and Thornado state.

## Client, Mirrors, And Light Nodes

The browser client handles mnemonic generation, deposit requests, note derivation, split proofs, withdrawal proofs, state scanning, and recovery. It should never send mnemonic material, reusable account keys, raw note secrets, or branch indexes to a server.

Mirrors distribute the client. Nodes vouch for mirrors by release hash and availability checks. Light nodes provide client-facing state queries and transaction submission without becoming custody authorities.

The client distribution layer must reduce phishing and stale-client risk by pinning releases, checking mirror availability, and making the expected client hash visible to operators and nodes.

## Shared Protobuf Contracts

`proto` is the compatibility boundary between Go services, Rust services, fixtures, and tests. Packages are versioned by path. Existing field numbers must not be reused. Consensus-facing callers must depend on structured error codes, not transport-specific strings.

Primary contracts:

- `proto/common/v1`: shared identifiers, byte encodings, amounts, and error codes;
- `proto/frost/v1`: signer sidecar API;
- `proto/shielder/v1`: Shielder API.

## Fee And Operator Economics

Fees are charged on outbound activity. Fees accumulate into buckets that operators can claim through a proof flow that should avoid linking a claim to a specific operator where possible.

Node slots are bonded. Slot ownership, handoff, penalties, and shutdown accounting live in Thornode state. Bonds do not leave normal operation except through explicit system rules such as approved shutdown or defined slot-transfer settlement.

## Governance And Shutdown

Governance controls operational parameters such as outbound fee size, churn interval, fee bucket size, signer thresholds, mirror policy, and shutdown state.

Shutdown is an explicit state-machine mode. During shutdown, new deposits stop, pending withdrawals and recovery paths are handled by policy, node principal is returned according to system rules, and remaining fees or abandoned funds are distributed by the shutdown rule.

## Localnet And Testing

`ops` defines the local deployment shape. Mock mode lets service boundaries boot before all real implementations are wired. Crypto mode uses buildable Go and Rust services.

The test stack should cover:

- Bitcoin deposit observation and reorg handling;
- split and withdrawal proof verification;
- nullifier uniqueness;
- FROST DKG and signing under dropout;
- churn and signer epoch transitions;
- outbound queue construction and broadcast retry;
- bond, slot, penalty, and shutdown accounting;
- mirror registration and health checks;
- mnemonic-only wallet recovery.
