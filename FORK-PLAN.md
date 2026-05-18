# Thornado Fork Plan

This plan describes the production-oriented fork strategy for building
THORChain-like Bitcoin custody, churn, observation, and signing around a
Tornado/Orchard-style privacy system.

The project should be split into four major implementation tracks that can be
worked on side by side:

1. Fork THORNode and keep the Go consensus/state-machine pieces we need.
2. Fork Bifrost and keep only the Bitcoin chain-client path.
3. Build a Rust FROST signer sidecar.
4. Build a Rust privacy sidecar or verifier library.

The main architectural decision is that there should be one canonical consensus
state machine: the Go THORNode/Cosmos SDK fork. Rust owns cryptography,
verification, proving helpers, and local secret material, but not canonical
consensus state.

## Target Shape

```text
Bitcoin network
  |
  v
Go Bifrost fork
  - Bitcoin scanner
  - inbound observation
  - outbound scheduling
  - signing session orchestration
  - broadcast, retry, solvency, reorg handling
  |
  v
Go THORNode fork / Cosmos SDK app
  - node accounts
  - bonds, jail, slashing
  - vault membership and churn
  - observed tx voters
  - outbound queues
  - privacy module state
  - final consensus accept/reject decisions
  |
  +--> Rust FROST signer sidecar
  |      - FROST DKG
  |      - key-share storage
  |      - threshold signing
  |      - signing policy checks
  |
  +--> Rust privacy sidecar or linked verifier
         - Orchard/ZK proof verification
         - commitment/nullifier helpers
         - proving helpers
         - test vectors and verifying key adapters
```

## Non-Negotiable Boundaries

Consensus state lives in the Go THORNode fork.

The following must be canonical Go app state:

- node accounts
- bonds
- slash points
- jail status
- validator/vault membership
- churn epochs
- vault public keys
- observed transaction voters
- outbound transaction queues
- note commitments
- commitment tree roots
- spent nullifiers
- withdrawal records
- privacy module params
- accepted verifying key hashes and versions

Rust sidecars may own local operational state only:

- FROST key shares
- DKG session cache
- signing session cache
- signer-local policy cache
- proof-generation cache
- witness-generation cache
- local note/witness material, if ever needed by operators

Rust sidecars must not own canonical state that affects consensus. They can
compute, verify, sign, and attest. The Go app decides state transitions.

## Repository Strategy

The current Rust workspace should not be discarded immediately. It can supply
privacy, Bitcoin test vectors, CLI tooling, and early integration harnesses.
However, the production stack should pivot toward a split repository or
multi-directory layout like:

```text
thornado/
  go-thornode/              # THORNode fork, canonical consensus app
  go-bifrost/               # Bifrost fork, Bitcoin-only chain client
  rust-frost-signer/        # Rust sidecar for FROST DKG/keysign
  rust-privacy/             # Rust sidecar/library for Orchard/ZK
  proto/                    # shared protobuf definitions
  test-fixtures/            # cross-language fixtures and vectors
  ops/                      # docker-compose, localnet, scripts
  docs/                     # architecture and protocol docs
```

If this remains a single repository, each agent should be assigned one top-level
directory and should avoid cross-directory edits except for `proto/`,
`test-fixtures/`, and `ops/` integration tasks.

## Shared Interface Rules

All cross-component contracts should be explicit and versioned.

Use protobuf for service interfaces unless there is a strong reason not to.
Rust and Go both have stable tooling for protobuf/gRPC, and protobuf gives us
an obvious place for fixture generation and compatibility checks.

Suggested shared packages:

```text
proto/frost/v1/signer.proto
proto/privacy/v1/privacy.proto
proto/common/v1/types.proto
test-fixtures/privacy/
test-fixtures/frost/
test-fixtures/bitcoin/
```

The first integration milestone should use mock sidecars. Do not block Go
module refactors on full cryptographic implementation.

## Workstream A: Go THORNode Fork

Owner: Go consensus/state-machine agents.

Primary goal: fork THORNode into a smaller Thornado consensus app that keeps
node lifecycle, vaults, churn, observation voting, outbound queues, governance
params, and upgrades, while removing swap/liquidity functionality that is not
needed for privacy-focused Bitcoin custody.

### Keep

Keep the parts of THORNode that provide production network mechanics:

- Cosmos SDK app wiring
- CometBFT integration
- validator set operations
- node account lifecycle
- active/standby membership
- bonding
- slashing
- jail
- churn
- vault state
- keygen/keysign orchestration state
- observed tx voter logic
- inbound/outbound tx tracking
- Mimir or equivalent runtime params
- software upgrade handling
- genesis import/export
- pruning/state sync assumptions
- CLI/node process model
- telemetry/logging where useful

### Remove Or Stub

Remove, disable, or stub anything not needed for the Bitcoin privacy custody
path:

- swaps
- pools
- LP accounting
- savers
- lending
- synthetic assets
- affiliate logic
- DEX routing
- non-Bitcoin chain-specific modules
- chain clients embedded in consensus that are not needed
- economic features that only exist to support multi-asset swaps

Do not remove operational primitives just because their current call sites are
swap-oriented. Vaults, outbound queues, observed tx voters, signer membership,
and churn should be kept and renamed/adapted if needed.

### New Privacy Module

Add a Thornado privacy module to the Go app. Tentative name:

```text
x/privacy
```

The module should own:

- note commitments
- current commitment tree roots
- historical valid roots
- spent nullifier set
- withdrawal records
- deposit records
- denomination params
- fee params
- accepted proof system versions
- accepted verifying key hashes
- tree depth
- root retention window
- pool activation flags

Suggested keeper layout:

```text
x/privacy/
  keeper/
    keeper.go
    msg_server.go
    query_server.go
    roots.go
    commitments.go
    nullifiers.go
    params.go
  types/
    msgs.go
    params.go
    genesis.go
    keys.go
    events.go
  module.go
  genesis.go
```

### Privacy Messages

Start with a minimal transaction set:

```text
MsgDepositToNote
MsgWithdrawNote
MsgUpdatePrivacyParams
MsgRegisterVerifyingKey
```

`MsgDepositToNote` should be created from an observed Bitcoin deposit flow, not
blindly accepted from arbitrary users unless explicitly designed that way.

`MsgWithdrawNote` should include:

- proof bytes
- public inputs
- root
- nullifier
- output address or encoded withdrawal target
- denomination
- fee
- relayer fee, if relayers are supported
- proof version or verifying key ID

The Go app validates:

- root exists
- root is not expired
- nullifier is unused
- denomination is allowed
- fee is valid
- withdrawal target format is valid
- verifying key ID is active
- proof verifies through the Rust privacy verifier

Only after all checks pass should the Go app mark the nullifier spent and emit
or enqueue the outbound Bitcoin withdrawal intent.

### Consensus Determinism

The Go app must handle privacy verification deterministically.

Open design question for this workstream:

- Should the verifier be a network sidecar called during transaction execution?
- Should the verifier be linked through FFI?
- Should verification use WASM or another embedded deterministic runtime?

The lower-operational-risk consensus path is likely a linked or embedded
verifier. A network sidecar is acceptable for initial integration and testing,
but production consensus execution must carefully define timeout, retry,
versioning, and failure semantics.

Initial rule:

```text
same input + same accepted verifying key = same valid/invalid result
```

If a verifier call fails due to transport or local availability, the consensus
app must treat it as a deterministic transaction failure for that node. That can
cause liveness problems if many nodes are misconfigured, so production
deployment should pin verifier versions and run startup compatibility checks.

### Outputs

This workstream should produce:

- a buildable Go THORNode fork
- a stripped module set
- a documented list of removed THORChain features
- a privacy module with keeper, params, genesis, queries, and messages
- mock privacy verifier integration
- tests for roots, nullifiers, params, and withdrawal validation
- localnet support with the Bitcoin-only Bifrost fork

### Do Not Own

This workstream should not implement FROST cryptography.

It should not implement Orchard circuits.

It should not own Bitcoin scanning or broadcasting.

It should not store FROST key shares.

## Workstream B: Go Bifrost Bitcoin-Only Fork

Owner: Go chain-client agents.

Primary goal: fork Bifrost and remove every chain client except Bitcoin, while
preserving the production observation, signing, broadcast, retry, and solvency
machinery that THORChain already uses.

### Keep

Keep the Bitcoin operational path:

- Bitcoin block scanner
- mempool awareness where already supported
- transaction parser
- inbound observation creation
- confirmation tracking
- reorg handling
- outbound tx construction
- UTXO selection
- fee estimation
- broadcast
- retry queues
- solvency checks
- stuck transaction handling
- signer orchestration hooks
- communication with Thornode
- process lifecycle, metrics, and logging

### Remove

Remove or disable all non-Bitcoin chain clients:

- Ethereum
- Cosmos chains other than the Thornado app chain
- BNB
- BCH
- LTC
- DOGE
- AVAX
- any EVM-specific client logic
- all chain-specific signing paths not relevant to Bitcoin

Remove config branches, test fixtures, Docker services, and chain registry
entries that exist only for non-Bitcoin Bifrost clients.

### Signing Refactor

Replace the existing Bitcoin signing path with an interface that can call the
Rust FROST signer.

Suggested Go interface:

```go
type BitcoinSigner interface {
    SignOutbound(ctx context.Context, req SignOutboundRequest) (SignOutboundResponse, error)
    GetVaultPubKey(ctx context.Context, vaultID string) (PubKeyResponse, error)
    Health(ctx context.Context) error
}
```

The first implementation can be a mock signer that returns deterministic test
signatures for local integration. The production implementation should be a
gRPC client to `rust-frost-signer`.

### Outbound Flow

The desired flow:

```text
Thornode emits outbound Bitcoin intent
  -> Bifrost picks up outbound
  -> Bifrost constructs unsigned tx / PSBT / sighash package
  -> Bifrost asks rust-frost-signer to sign
  -> signer validates policy context
  -> signer returns final signature or signing session status
  -> Bifrost assembles final transaction
  -> Bifrost broadcasts
  -> Bifrost reports outbound result back to Thornode
```

The signing request should include enough policy context for the signer to make
local safety checks:

- vault ID
- chain ID
- outbound tx ID
- destination address
- amount
- fee rate
- UTXOs being spent
- Thornode block height
- Thornode-observed vault pubkey
- session ID
- expected signer set

The signer should never be the canonical source of whether an outbound is
allowed. Thornode decides that. The signer may still reject obviously unsafe,
malformed, stale, or unauthorized requests.

### Deposit Flow

The desired inbound privacy deposit flow:

```text
Bitcoin deposit observed
  -> Bifrost parses tx
  -> Bifrost sends observation to Thornode
  -> Thornode voters reach consensus on observation
  -> Thornode converts accepted observation into privacy deposit event
  -> Privacy module records note commitment/root update
```

The Bitcoin memo/address encoding scheme must be decided explicitly. Options:

- memo includes note commitment payload
- deposit address maps to a pending note commitment
- off-chain API pre-registers deposit intent
- hybrid registration plus Bitcoin memo

Do not bake a final scheme into Bifrost until the privacy module has a stable
deposit message format.

### Outputs

This workstream should produce:

- a buildable Bitcoin-only Bifrost fork
- removed non-Bitcoin chain clients
- simplified config
- Bitcoin-only localnet/regtest setup
- signer interface abstraction
- mock signer implementation
- gRPC signer client implementation
- tests for observation, outbound construction, retry, and signer errors

### Do Not Own

This workstream should not implement FROST internals.

It should not own note/nullifier/commitment state.

It should not modify Go consensus state except through existing Thornode APIs.

It should not implement Orchard proof verification.

## Workstream C: Rust FROST Signer Sidecar

Owner: Rust cryptography/signing agents.

Primary goal: build a Rust service that performs FROST DKG and threshold
Bitcoin signing for vault keys, while storing local key shares securely and
exposing a narrow API to Bifrost and/or Thornode.

### Responsibilities

The signer owns:

- FROST participant identity
- local key shares
- DKG session participation
- signing session participation
- nonce generation and storage
- partial signatures
- aggregation if selected by the protocol design
- vault public key reporting
- local signing policy checks
- share backup/export policy, if allowed
- signer audit logs

The signer does not own:

- canonical vault membership
- churn rules
- slash decisions
- outbound queue selection
- Bitcoin observation
- Bitcoin broadcast
- privacy note/nullifier state

### Service API

Initial protobuf service:

```text
service FrostSigner {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc GetNodeSignerInfo(GetNodeSignerInfoRequest) returns (GetNodeSignerInfoResponse);
  rpc StartDkg(StartDkgRequest) returns (StartDkgResponse);
  rpc GetDkgStatus(GetDkgStatusRequest) returns (GetDkgStatusResponse);
  rpc GetVaultPubKey(GetVaultPubKeyRequest) returns (GetVaultPubKeyResponse);
  rpc StartSigning(StartSigningRequest) returns (StartSigningResponse);
  rpc GetSigningStatus(GetSigningStatusRequest) returns (GetSigningStatusResponse);
  rpc ForgetVaultShare(ForgetVaultShareRequest) returns (ForgetVaultShareResponse);
}
```

Signing request should support either:

- PSBT input
- explicit Bitcoin sighash package
- structured unsigned transaction plus UTXO metadata

Avoid accepting opaque bytes without policy context. The signer needs structured
fields for local checks.

### Storage

Use a local encrypted store for key shares.

Minimum stored entities:

- signer node ID
- vault ID
- FROST group public key
- participant index
- threshold
- participant set
- encrypted secret share
- DKG transcript or transcript hash
- creation height/epoch
- share status

Signing session storage:

- session ID
- vault ID
- message/sighash
- nonce commitments
- local nonce state
- partial signature
- participant set
- expiry height/time
- status

Nonce handling must be conservative. Do not allow nonce reuse across signing
sessions.

### Policy Checks

Before signing, the sidecar should check:

- requested vault exists locally
- signer is part of the vault
- request chain is Bitcoin
- session ID has not already been used for a conflicting message
- sighash or PSBT matches structured outbound fields
- participant set matches expected vault membership
- request is not expired
- amount/address/fee fields are present
- Bifrost/Thornode caller is authenticated

Policy checks reduce blast radius. They do not replace consensus validation.

### Protocol Integration

There are two likely orchestration models:

1. Bifrost-driven signing sessions.
2. Thornode-driven signing sessions with Bifrost only requesting final outbound
   assembly and broadcast.

The first integration should be Bifrost-driven because it is closer to existing
Bifrost signing flows. Keep the API flexible enough to move session initiation
to Thornode later if needed.

### Outputs

This workstream should produce:

- Rust service crate
- protobuf definitions or generated bindings
- encrypted local share storage
- mock/insecure dev mode for localnet
- FROST DKG implementation
- FROST Bitcoin signing implementation
- CLI for inspecting local signer state
- integration tests with multiple signer processes
- test vectors for DKG and Bitcoin signature verification
- Docker image for localnet

### Do Not Own

This workstream should not edit the Go privacy module.

It should not edit Bifrost observation logic.

It should not decide canonical vault membership.

It should not create or broadcast Bitcoin transactions.

## Workstream D: Rust Privacy Sidecar Or Library

Owner: Rust privacy/ZK agents.

Primary goal: build the Orchard/Tornado-style privacy cryptography component
used by the Go consensus app for deterministic verification and by clients or
operators for proof/witness helpers.

### Responsibilities

The privacy component owns:

- proof verification implementation
- verifying key loading and hashing
- public input serialization
- commitment helper functions
- nullifier helper functions
- Merkle path/witness helper functions
- proof generation helpers, if supported
- cross-language test vectors
- versioned proof-system adapters

It does not own:

- canonical commitment tree state
- canonical root history
- spent nullifier set
- withdrawal records
- fee accounting
- deposit acceptance
- outbound Bitcoin queues

### Service API

Initial protobuf service:

```text
service PrivacyVerifier {
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc GetVerifierInfo(GetVerifierInfoRequest) returns (GetVerifierInfoResponse);
  rpc VerifyWithdrawal(VerifyWithdrawalRequest) returns (VerifyWithdrawalResponse);
  rpc ComputeCommitment(ComputeCommitmentRequest) returns (ComputeCommitmentResponse);
  rpc ComputeNullifier(ComputeNullifierRequest) returns (ComputeNullifierResponse);
}
```

`VerifyWithdrawalRequest` should include:

- proof bytes
- public inputs
- root
- nullifier
- denomination
- recipient commitment or withdrawal address hash
- fee
- verifying key ID
- proof system version

The response should be simple:

```text
valid: bool
error_code: enum
verifier_version: string
verifying_key_hash: bytes
```

Do not return "maybe" states. Consensus callers need a deterministic yes/no
result.

### Library Mode

Because proof verification is consensus-critical, this component should also be
designed as a Rust library that can be linked or embedded from Go if the network
sidecar approach proves too risky.

Required structure:

```text
rust-privacy/
  crates/
    privacy-core/       # no networking, deterministic logic
    privacy-service/    # gRPC wrapper
    privacy-cli/        # fixture and debugging CLI
```

The consensus-safe verification function should live in `privacy-core`, not in
the gRPC service wrapper.

### Determinism Requirements

Verification must not depend on:

- wall clock time
- network access
- local mutable cache
- random numbers
- CPU feature-dependent behavior that changes results
- unpinned verifying key files
- environment-specific config

Verifying keys must be identified by hash. The Go app should store accepted
hashes in params/governance state. The Rust verifier should report its available
keys and refuse mismatched IDs.

### Outputs

This workstream should produce:

- `privacy-core` deterministic verification crate
- `privacy-service` gRPC wrapper
- `privacy-cli` fixture generator
- verifying key hash tooling
- proof verification test vectors
- commitment/nullifier test vectors
- Go-compatible serialization documentation
- Docker image for localnet

### Do Not Own

This workstream should not store spent nullifiers.

It should not decide whether a root is valid.

It should not enqueue withdrawals.

It should not scan or broadcast Bitcoin transactions.

## Workstream E: Shared Proto And Fixtures

Owner: integration agents.

Primary goal: define shared interfaces and fixtures so the Go and Rust teams can
develop independently without relying on unstable ad hoc JSON shapes.

### Proto Packages

Create:

```text
proto/common/v1/types.proto
proto/frost/v1/signer.proto
proto/privacy/v1/privacy.proto
```

Common types should include:

- chain ID
- vault ID
- session ID
- byte wrapper conventions
- Bitcoin outpoint
- Bitcoin amount
- signer participant
- error codes

Privacy types should include:

- proof bytes
- public inputs
- root
- nullifier
- commitment
- denomination
- verifying key ID
- verifying key hash
- proof system version

FROST types should include:

- DKG request/response
- vault participant set
- signing request/response
- signing status
- PSBT or sighash package
- policy context

### Fixture Rules

Every cross-language behavior needs fixtures:

- commitment computation
- nullifier computation
- valid withdrawal proof
- invalid withdrawal proof
- verifying key hash
- Bitcoin outbound signing payload
- Bitcoin signature verification result
- FROST DKG transcript hash

Fixtures should be checked into `test-fixtures/` and consumed by both Go and
Rust tests.

### Outputs

This workstream should produce:

- protobuf definitions
- generated Go bindings
- generated Rust bindings
- fixture schema
- fixture generation commands
- CI checks that generated code is current

## Workstream F: Localnet And Operations

Owner: ops/integration agents.

Primary goal: make it possible to run the split architecture locally with a
Bitcoin regtest node, Go Thornode fork, Go Bifrost fork, Rust FROST signer
nodes, and Rust privacy verifier.

### Services

Localnet should include:

- bitcoind regtest
- one or more Go Thornode nodes
- one or more Go Bifrost instances
- N Rust FROST signer instances
- one Rust privacy verifier per Thornode node, or embedded verifier mode
- optional relayer/client process

### Required Flows

Localnet must support:

1. initialize network
2. create node accounts
3. bond nodes
4. churn nodes into active vault membership
5. run FROST DKG for a vault
6. observe a Bitcoin deposit
7. create a privacy note commitment
8. verify a withdrawal proof
9. enqueue outbound Bitcoin withdrawal
10. FROST-sign the outbound
11. broadcast withdrawal
12. observe outbound completion

### Outputs

This workstream should produce:

- Dockerfiles for all services
- docker-compose localnet
- regtest bootstrap scripts
- health checks
- smoke test script
- logs directory convention
- docs for common failure modes

## Phase 0: Fork And Inventory

Goal: establish clean fork baselines and map the removal surface before major
refactors start.

Workstream A:

- fork THORNode into `go-thornode/`
- record upstream commit hash
- make the fork build without behavior changes
- list all modules and classify keep/remove/unknown
- identify keeper dependencies between modules
- identify signer/keygen/vault/churn dependencies
- identify observed tx voter dependencies

Workstream B:

- fork Bifrost into `go-bifrost/`
- record upstream commit hash
- make the fork build without behavior changes
- list all chain clients
- classify Bitcoin dependencies versus multi-chain dependencies
- identify current signing interfaces
- identify current Bitcoin test coverage

Workstream C:

- create Rust signer service skeleton
- choose FROST crate strategy
- define storage abstraction
- define insecure dev mode
- create health endpoint

Workstream D:

- split deterministic privacy logic from networking concerns
- identify existing Thornado Rust crates that can be reused
- define initial proof/public-input serialization
- create health endpoint

Workstream E:

- create proto directory
- define initial common types
- set up Go/Rust code generation

Exit criteria:

- every workstream builds independently
- upstream fork commits are documented
- keep/remove inventory exists
- mock services can start

## Phase 1: Strip To Bitcoin Privacy Spine

Goal: remove obvious non-required surface while preserving the production
mechanics needed for custody, churn, and Bitcoin operations.

Workstream A:

- remove or disable swap modules
- remove or disable liquidity pool modules
- remove or disable saver/lending/synthetic modules
- keep node account lifecycle
- keep bonding/slashing/jail
- keep vault/churn/keygen/keysign state
- keep observed tx voters
- keep outbound queues
- keep runtime params
- keep upgrade handling
- add compile-time or config-time feature gates where removal is risky
- document each removed module and replacement assumption

Workstream B:

- remove non-Bitcoin chain clients
- remove non-Bitcoin chain configs
- remove non-Bitcoin signer paths
- keep Bitcoin scanner
- keep Bitcoin parser
- keep Bitcoin UTXO/fee/broadcast logic
- keep observation reporting
- keep outbound retry logic
- replace direct signer coupling with `BitcoinSigner` interface

Workstream C:

- implement mock signing API
- define request validation rules
- add local in-memory storage backend
- add CLI for health/status

Workstream D:

- implement mock verifier API
- implement verifying key info endpoint
- implement deterministic error code model
- add placeholder commitment/nullifier helpers if real ones are not ready

Exit criteria:

- Go Thornode fork builds with stripped module set
- Go Bifrost fork builds with only Bitcoin client enabled
- Bifrost can call mock signer
- Thornode can call mock privacy verifier
- localnet can start with mocks

## Phase 2: Privacy Consensus Module

Goal: make privacy state canonical in the Go THORNode fork while still using a
mock verifier.

Workstream A:

- add `x/privacy`
- define genesis state
- define params
- define keeper store prefixes
- implement commitment storage
- implement root storage
- implement nullifier storage
- implement withdrawal record storage
- implement deposit message or internal deposit handler
- implement withdrawal message
- add queries for roots, commitments, nullifiers, params
- emit events for deposits and withdrawals
- wire module into app
- add migration/upgrade hooks if needed

Workstream B:

- map accepted Bitcoin observations into Thornode deposit inputs
- preserve confirmation/reorg behavior
- expose enough deposit metadata for privacy commitment creation
- keep deposit parsing scheme configurable until finalized

Workstream D:

- keep mock verifier stable
- provide deterministic fixture responses
- align serialization with Go message types

Exit criteria:

- deposit creates commitment and root update
- withdrawal checks root/nullifier/fee/denom
- withdrawal calls mock verifier
- valid withdrawal marks nullifier spent
- duplicate nullifier fails
- expired/unknown root fails
- tests cover keeper behavior

## Phase 3: FROST DKG And Vault Key Integration

Goal: replace mock vault key behavior with Rust FROST-controlled vault public
keys and signing sessions.

Workstream A:

- adapt keygen/churn state to refer to FROST vault sessions
- store vault public keys reported by signer flow
- define failure behavior for incomplete DKG
- define slash/jail hooks for signer non-participation, if in scope
- expose vault membership to Bifrost/signers

Workstream B:

- call signer `StartDkg` during vault creation flow, if Bifrost owns orchestration
- otherwise consume Thornode DKG status and continue existing outbound flow
- request signer vault pubkey
- use FROST vault pubkey for deposit addresses

Workstream C:

- implement real DKG
- persist key shares
- report group public key
- support multiple vaults
- support signer restart and recovery
- add multi-process integration tests

Exit criteria:

- localnet can create a FROST-backed vault
- vault pubkey is visible in Thornode state
- Bifrost derives/uses Bitcoin deposit address from vault pubkey
- signer restarts without losing shares

## Phase 4: FROST Bitcoin Signing

Goal: sign real Bitcoin outbound transactions through the Rust signer sidecar.

Workstream B:

- construct PSBT or sighash package for outbound
- send structured signing request to signer
- poll signing status or receive final response
- assemble final Bitcoin transaction
- broadcast signed transaction on regtest
- report outbound result to Thornode
- handle signer timeout and retry
- handle partial signing failure

Workstream C:

- implement signing session protocol
- implement nonce management
- implement partial signature flow
- implement aggregation or coordinator behavior
- verify final Bitcoin signature
- reject conflicting session reuse
- reject stale or malformed policy context
- add test vectors

Exit criteria:

- localnet can sign and broadcast a Bitcoin outbound from a FROST vault
- invalid signing requests are rejected
- retry behavior does not reuse nonces unsafely
- Bifrost signer error handling is tested

## Phase 5: Real Privacy Verification

Goal: replace mock privacy verification with deterministic Orchard/ZK proof
verification.

Workstream A:

- store accepted verifying key hashes in params
- add governance/admin path for verifier key registration
- require proof version/key ID in withdrawal messages
- handle verifier invalid result deterministically
- add startup compatibility checks if using sidecar mode

Workstream D:

- implement real proof verification in `privacy-core`
- implement verifying key loading by ID/hash
- implement public input serialization
- implement commitment/nullifier helpers
- produce valid and invalid proof fixtures
- expose gRPC wrapper through `privacy-service`
- add CLI fixture generation

Workstream E:

- lock fixture schemas
- add Go tests consuming Rust-generated fixtures
- add Rust tests consuming checked-in fixtures

Exit criteria:

- valid proof fixture passes in Go withdrawal flow
- invalid proof fixture fails
- wrong verifying key hash fails
- wrong public inputs fail
- duplicate nullifier still fails before or after verification

## Phase 6: End-To-End Private Bitcoin Flow

Goal: prove the production-shaped deposit and withdrawal path across all four
major components.

Full flow:

```text
start localnet
  -> bond nodes
  -> churn active vault
  -> complete FROST DKG
  -> derive Bitcoin deposit address
  -> send regtest BTC deposit
  -> Bifrost observes deposit
  -> Thornode accepts observation
  -> Privacy module records commitment/root
  -> user/client generates withdrawal proof
  -> Thornode verifies proof
  -> nullifier is marked spent
  -> outbound Bitcoin intent is queued
  -> Bifrost builds outbound tx
  -> FROST signer signs
  -> Bifrost broadcasts
  -> Thornode records outbound completion
```

Workstream F:

- automate the flow in a smoke test
- collect logs from all services
- expose common debug commands
- document reset/retry steps

Exit criteria:

- one command runs the full regtest private deposit/withdrawal path
- logs are sufficient to debug each boundary
- repeated withdrawal with same nullifier fails
- failed signer/verifier services produce clear errors

## Phase 7: Hardening

Goal: move from working localnet to production-readiness work.

Areas:

- verifier version pinning
- signer authentication
- mTLS or equivalent service authentication
- key-share encryption
- key-share backup and recovery policy
- DKG failure handling
- signer equivocation detection
- slashing evidence for non-participation, if feasible
- Bitcoin reorg safety around deposit finality
- outbound fee bumping
- stuck transaction recovery
- root retention tuning
- nullifier set pruning strategy, if any
- state export/import
- chain upgrades
- load testing proof verification
- load testing signer sessions
- chaos testing signer/verifier restarts

Exit criteria:

- documented production threat model
- documented operational runbooks
- deterministic upgrade procedure
- reproducible localnet and testnet deployment
- CI covers unit, integration, and smoke tests

## Agent Coordination Rules

Agents should work in isolated directories whenever possible.

Suggested ownership:

- Agent A: `go-thornode/`
- Agent B: `go-bifrost/`
- Agent C: `rust-frost-signer/`
- Agent D: `rust-privacy/`
- Agent E: `proto/` and `test-fixtures/`
- Agent F: `ops/`

Cross-directory edits should be announced in the task description before work
starts. Generated protobuf bindings are allowed cross-directory edits, but the
proto owner should coordinate schema changes.

Avoid large mixed commits. Prefer commits like:

- `go-thornode: remove swap module wiring`
- `go-bifrost: isolate bitcoin chain client`
- `rust-frost-signer: add dkg session store`
- `rust-privacy: add verifier key hash fixtures`
- `proto: define frost signer api`
- `ops: add regtest localnet services`

## Integration Gates

Use explicit gates to avoid every workstream depending on unfinished real
cryptography.

Gate 1:

- Go Thornode calls mock privacy verifier.
- Go Bifrost calls mock signer.

Gate 2:

- Go Thornode has real privacy state.
- Go Bifrost has Bitcoin-only observation/outbound.
- Rust services still may be mocks.

Gate 3:

- FROST DKG produces vault pubkey.
- Bifrost can use that pubkey.

Gate 4:

- FROST signs real Bitcoin outbound.
- Privacy verifier may still be mock.

Gate 5:

- Real privacy verifier validates fixtures.
- FROST signer signs real outbound.

Gate 6:

- Full private deposit/withdrawal flow works on regtest.

## Key Open Decisions

These should be resolved early but should not block fork inventory work:

- Is privacy verification called over gRPC in production consensus, or linked
  through FFI/embedded runtime?
- What exact note commitment format is used for Bitcoin deposits?
- How does a Bitcoin deposit bind to a note commitment?
- Are withdrawals relayer-supported from day one?
- Is Bifrost or Thornode the signing-session coordinator?
- What FROST crate/protocol implementation is acceptable for production?
- How are signer identities authenticated?
- What is the key-share backup/recovery policy?
- What is the root retention window?
- Which proof system version is the first production target?

## Immediate Next Tasks

1. Create fork baselines for THORNode and Bifrost.
2. Create `proto/` with initial mockable service definitions.
3. Create Rust sidecar skeletons with health endpoints.
4. Inventory THORNode modules as keep/remove/unknown.
5. Inventory Bifrost chain clients and Bitcoin dependencies.
6. Bring up a mock localnet with Go processes calling Rust mock services.

The project should avoid waiting for real FROST or real Orchard verification
before proving the process boundaries. The first useful milestone is a stripped
Go spine with mock sidecars and stable interfaces.
