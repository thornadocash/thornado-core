# Thornado Protocol Working Test Sheet

This is the working checklist for validating the protocol in layers. Do not mark a flow complete because a script exits zero; mark it complete only when the expected state, artifacts, and negative cases are observed.

Source references:
- FROST local behavior: `go-thornado/go-wrappers/frost/go-frost/sessions/sessions_test.go`, `go-thornado/bifrost/pkg/chainclients/btc/vault_signer.go`
- FROST service contract: `proto/frost/v1/signer.proto`
- Shielder protocol behavior: `go-thornado/x/thornado/shielder.go`, `go-thornado/x/thornado/shielder_flow.go`
- Shielder messages and queries: `go-thornado/proto/thornado/v1/types/msg_shielder.proto`, `go-thornado/proto/thornado/v1/types/query_shielder.proto`
- Shielder FFI behavior: `go-thornado/go-wrappers/shielder/engine.go`, `go-thornado/go-wrappers/shielder/engine_test.go`

## Flow 1: Local FROST

Purpose: prove the local FROST implementation can create threshold key material, sign BTC/Taproot digests, verify signatures, and reject invalid inputs before any network or Thornado state is involved.

Command:

```sh
ops/scenarios/10-frost-local.sh
```

Detailed flow to observe:

1. Run pure FROST wrapper tests.
2. Generate shares for multiple participant sets.
3. Confirm participant order is deterministic.
4. Run dealer-style keygen and session-style distributed keygen.
5. Sign a 32-byte digest with enough participants.
6. Verify each returned signature against the group public key.
7. Try signing a non-32-byte message and confirm it is rejected.
8. Create a FROST local-state-backed BTC vault signer.
9. Sign with the main vault path and verify against the untweaked secp256k1 public key.
10. Sign with a non-main deposit path and verify against the BIP340 Taproot tweaked key.
11. Attempt signing with missing state and confirm it returns a typed `tss.KeysignError`.

Data to record:

- Participant set, for example `["node-a","node-b","node-c"]`.
- Threshold, for example `2 of 3`.
- Group public key, compressed hex.
- Local party key and participant list stored in local state.
- `SigningEngine` value: must be `frost`.
- Message digest hex: exactly 32 bytes.
- Main-path signature length: 64 bytes.
- Deposit-path signature length: 64 bytes.
- Main-path verification result: pass.
- Deposit-path Taproot verification result: pass.
- Error text for missing state.
- Error text for non-32-byte signing input.

Pass checklist:

- [ ] FROST wrapper keygen returns one non-empty share per participant.
- [ ] All shares decode and agree on one group public key.
- [ ] Participant order is stable and deterministic.
- [ ] Session keygen completes for all participants.
- [ ] Session signing returns one signature per signer.
- [ ] Every signature verifies against the expected group/tweaked public key.
- [ ] Non-32-byte signing input is rejected.
- [ ] Vault signer refuses non-`secp256k1` algorithms.
- [ ] Vault signer refuses missing local state.
- [ ] Vault signer refuses non-FROST local state.
- [ ] Vault signer refuses empty FROST local data.
- [ ] Main vault path signs and verifies.
- [ ] Deposit path signs and verifies with Taproot tweak.

Not complete until:

- [ ] Test output is clean and reproducible with `-count=1`.
- [ ] The recorded public key/signature/digest data is attached to the test run notes.

## Flow 2: Basic Docker Localnet

Purpose: prove the local Docker deployment is deterministic, starts cleanly, exposes the expected service boundaries, and has a usable BTC regtest node before protocol e2e work starts.

Command:

```sh
ops/scenarios/00-basic-localnet.sh
```

Detailed flow to observe:

1. Deploy Docker localnet through `ops/scripts/deploy-localnet.sh`.
2. If host `18443/18444` are busy, auto-select free host ports while keeping container RPC at `bitcoind-regtest:18443`.
3. Start services:
   - `bitcoind-regtest`
   - `thornado-1`
   - `bifrost-1`
4. Wait for bitcoind health.
5. Check Thornado CometBFT-style status endpoint.
6. Check Thornado REST health endpoint.
7. Check Bifrost health endpoint.
8. Bootstrap regtest wallets.
9. Mine at least 101 blocks so coinbase funds are spendable.
10. Fund the client wallet.

Data to record:

- Docker Compose profiles.
- Host BTC RPC/P2P ports selected.
- Container names and health status.
- Bitcoind chain: must be `regtest`.
- Bitcoind block height after bootstrap.
- Miner wallet name.
- Client wallet name.
- Client BTC address.
- Client balance.
- Thornado `/status` response with `catching_up=false`.
- Thornado `/health` response.
- Bifrost `/health` response.

Pass checklist:

- [ ] `bitcoind-regtest` is healthy.
- [ ] `thornado-1` is reachable on `26657`, `1317`, and `9090`.
- [ ] `bifrost-1` is reachable on `6040`.
- [ ] `bitcoin-cli getblockchaininfo` reports `chain=regtest`.
- [ ] Regtest has at least 101 blocks after bootstrap.
- [ ] Miner wallet exists and is loaded.
- [ ] Client wallet exists and is loaded.
- [ ] Client wallet balance equals configured `CLIENT_FUNDS_BTC` unless a prior test spent funds.
- [ ] Logs are collected for the run.

Negative checks:

- [ ] Deploy still succeeds when another container owns host port `18443`.
- [ ] Deploy with `--reset` removes old localnet volumes.
- [ ] Deploy with invalid profile fails clearly.

Not complete until:

- [ ] The localnet can be torn down and redeployed without manual cleanup.
- [ ] BTC regtest RPC works from inside compose and from the host test script.

## Flow 3: FROST DKG

Purpose: prove signer participants can form one BTC vault key through DKG, persist keyshares, expose vault public key/address, and make the resulting vault visible to Thornado/Bifrost.

Script:

```sh
ops/scenarios/20-frost-dkg.sh
```

Required hook contracts:

- `FROST_DKG_CMD`: starts DKG across signer sidecars, waits for completion, persists local state, and prints machine-readable output.
- `FROST_DKG_STATUS_CMD`: queries DKG status and prints vault pubkey/session state.

Intended service contract from `proto/frost/v1/signer.proto`:

- `GetNodeSignerInfo`
- `StartDkg`
- `GetDkgStatus`
- `GetVaultPubKey`
- `ForgetVaultShare`

Detailed flow to observe:

1. Query every signer with `GetNodeSignerInfo`.
2. Verify every signer has a unique node ID and participant index.
3. Build a `VaultParticipantSet` with participants, threshold, Thornado height, and membership epoch.
4. Call `StartDkg` on every signer with the same:
   - `session_id`
   - `vault_id`
   - `chain_id=BTC`
   - `participant_set`
   - `expires_at_height`
5. Poll `GetDkgStatus` until every signer reaches `DKG_STATUS_COMPLETE`.
6. Assert every signer reports the same:
   - `vault_id`
   - `group_public_key`
   - `transcript_hash`
7. Query `GetVaultPubKey`.
8. Verify returned `bitcoin_address` is valid for BTC regtest.
9. Confirm signer local state exists and has `SigningEngine=frost`.
10. Confirm Bifrost can read the FROST local state for the vault pubkey.
11. Confirm Thornado has/accepts the vault pubkey as an active or pending BTC vault.
12. Confirm keyshare backup path stores raw FROST keyshares under `keyshares_backup_frost`.
13. Restore one signer from backup and confirm its local state can sign again.

Data to record:

- DKG `session_id`.
- `vault_id`.
- Chain ID.
- Thornado height at DKG start.
- Membership epoch.
- Participant list and participant indexes.
- Threshold.
- Expiry height.
- Per-signer DKG status timeline.
- Group public key hex.
- Transcript hash hex.
- BTC regtest vault address.
- Local state path or storage key per signer.
- `keyshares_backup_frost` size and presence.
- Restored local state checksum or keyshare hash.

Pass checklist:

- [ ] Every signer is healthy.
- [ ] Participant set is identical across signers.
- [ ] Threshold is correct for the participant count.
- [ ] `StartDkg` returns no protocol error.
- [ ] All signers reach `DKG_STATUS_COMPLETE`.
- [ ] All signers report identical group public key.
- [ ] All signers report identical transcript hash.
- [ ] Vault pubkey converts to Thornado `common.PubKey`.
- [ ] Vault BTC address is a valid regtest Taproot address.
- [ ] Local state exists for every signer.
- [ ] Local state engine is `frost`.
- [ ] FROST keyshare backup exists and decrypts.
- [ ] Restored keyshare can participate in signing.

Negative checks:

- [ ] Duplicate participant index is rejected.
- [ ] Wrong threshold is rejected.
- [ ] Mismatched participant set is rejected or fails deterministically.
- [ ] Expired DKG session becomes `DKG_STATUS_EXPIRED`.
- [ ] `ForgetVaultShare` removes local share and prevents signing.

Not complete until:

- [ ] DKG output is machine-readable and can feed Flow 4 and Flow 5.

## Flow 4: BTC Deposit And Shielder Commitment

Purpose: prove a user can obtain a vault-derived BTC deposit address, send regtest BTC, have Bifrost observe it, have Thornado match it to the shielder session, and post private note commitments that become queryable Merkle roots.

Script:

```sh
ops/scenarios/30-btc-deposit.sh
```

Required hook contracts:

- `THORNADO_BOOTSTRAP_CMD`: initializes validators, node accounts, vault state, and churn/keygen prerequisites.
- `SEND_DEPOSIT_CMD`: derives or fetches a deposit address, sends BTC regtest funds, mines confirmations, waits for Bifrost observation, and verifies Thornado state.

Important intended behavior from code:

- `MsgShielderRegisterPow` issues a deposit address for an owner and PoW token.
- Deposit session stores `owner`, `pow_token`, `deposit_address`, `vault_pub_key`, `deposit_path_index`, `created_height`, and status `address_issued`.
- `MatchShielderDeposit` accepts only BTC deposits to a registered address.
- Deposit amount must be at least `Deposit_AmountMinSats` default `546`.
- Matched deposits update status to `deposit_matched`.
- Posting commitments moves deposit through `settled` to `committed`.
- Commitments are tracked per denomination and update the denomination Merkle root.

Detailed flow to observe:

1. Bootstrap Thornado validator and vault state.
2. Confirm there is a current BTC vault.
3. Register shielder PoW:
   - owner account
   - pow token
   - optional operator/node pubkeys only for node bond cases
4. Capture `deposit_address`, `vault_pub_key`, and `deposit_path_index`.
5. Query session by owner.
6. Query deposit address mapping by address.
7. Send regtest BTC from client wallet to `deposit_address`.
8. Mine enough confirmations.
9. Verify Bifrost observes the BTC tx.
10. Verify Thornado records a `ShielderDeposit`.
11. Verify session status becomes `deposit_matched`.
12. Derive split receipt through Shielder FFI for the deposit ID and amount.
13. Post note commitments with denominations.
14. Query deposit again and verify status `committed`.
15. Query shielder roots and verify the denomination root includes the new commitment.

Data to record:

- Owner address.
- PoW token.
- Thornado height when address is issued.
- Deposit address.
- Vault pubkey.
- Deposit path index.
- BTC txid.
- BTC amount sats.
- BTC confirmations.
- Thornado deposit ID.
- Session status before and after observation.
- Deposit status before and after commitments.
- Commitments JSON.
- Denomination sats per commitment.
- Merkle root for each denomination.
- `commitment_count`.

Pass checklist:

- [ ] Current BTC vault exists before address registration.
- [ ] PoW registration returns a BTC regtest deposit address.
- [ ] Address mapping query returns the same vault pubkey/path/owner.
- [ ] Session query returns status `address_issued`.
- [ ] BTC tx pays exactly the issued deposit address.
- [ ] BTC tx confirms on regtest.
- [ ] Bifrost observes the tx once.
- [ ] Thornado matches the tx to the correct owner/session.
- [ ] Deposit status becomes `deposit_matched`.
- [ ] Deposit amount is greater than or equal to `546` sats.
- [ ] Shielder receipt contains non-empty commitments.
- [ ] Posted commitment denominations sum to deposit amount, after any dust floor handling.
- [ ] Duplicate commitments are rejected.
- [ ] Deposit status becomes `committed`.
- [ ] Merkle root exists for each denomination.
- [ ] Deposit query shows expected `commitment_count`.

Negative checks:

- [ ] Empty PoW token rejected.
- [ ] Reused unexpired PoW token rejected.
- [ ] Unknown deposit address is not matched.
- [ ] Non-BTC deposit is rejected.
- [ ] Deposit below `Deposit_AmountMinSats` rejected.
- [ ] Expired session rejected if expiry config is enabled.
- [ ] Commitments with empty values rejected.
- [ ] Commitments with duplicate values rejected.
- [ ] Commitments whose denominations do not sum to amount rejected.
- [ ] Second commitment post for same deposit rejected.

Not complete until:

- [ ] The final commitment root is usable as withdrawal public input in Flow 5.

## Flow 5: Withdrawal And BTC FROST Keysign

Purpose: prove a committed shielder note can be withdrawn exactly once, producing a valid outbound BTC transaction signed by the FROST vault and confirmed on regtest.

Script:

```sh
ops/scenarios/40-withdrawal.sh
```

Required hook contracts:

- `RUN_WITHDRAWAL_CMD`: creates/submits withdrawal proof, waits for outbound queue, runs FROST signing, broadcasts the BTC tx, and verifies recipient balance.

Important intended behavior from code:

- `MsgShielderRequestWithdrawal` carries JSON `proof` and JSON `public`.
- Public JSON must include:
  - `nullifier_hash`
  - `merkle_root`
  - `denomination_sats`
  - `recipient`
  - `fee_sats`
- Proof and public JSON are verified by the Shielder wrapper.
- Nullifier must not already be spent.
- Merkle root must be known for the denomination.
- Recipient must be BTC.
- Fee must equal configured withdrawal fee unless public `fee_sats` is zero.
- Current fee policy is `Withdrawal_FeeBasisPoints=100`; local e2e sets `Withdrawal_FeeMinSats=100000` unless a specific test overrides it.
- Accepted withdrawal stores status `keysign_queued`.
- Accepted withdrawal queues BTC outbound with:
  - `InHash = withdrawal_id`
  - `VaultPubKey = current BTC vault`
  - amount `denomination_sats - fee_sats`
  - max gas based on `fee_sats`

Detailed flow to observe:

1. Start from a committed deposit from Flow 4.
2. Query roots and select the root matching the note denomination.
3. Build withdrawal proof and public input through Shielder FFI.
4. Verify proof locally before submitting.
5. Capture recipient BTC regtest address and starting balance.
6. Submit `MsgShielderRequestWithdrawal`.
7. Query withdrawal by `withdrawal_id`.
8. Query nullifier by `nullifier_hash`.
9. Confirm withdrawal status `keysign_queued`.
10. Confirm outbound item exists with correct in-hash, vault pubkey, recipient, amount, and gas.
11. Start FROST signing via signer service or Bifrost vault signer.
12. Poll signing status until `SIGNING_STATUS_COMPLETE`.
13. Verify returned signature or signed transaction.
14. Broadcast signed BTC transaction to regtest.
15. Mine confirmation.
16. Verify recipient balance increased by net amount.
17. Verify nullifier replay fails.

Data to record:

- Deposit ID consumed.
- Commitment consumed.
- Denomination sats.
- Merkle root.
- Nullifier hash.
- Withdrawal ID.
- Owner address.
- Recipient BTC address.
- Recipient starting balance.
- Fee sats.
- Net sats.
- Vault pubkey.
- Outbound in-hash.
- Signing session ID.
- Signing status timeline.
- Signature hex or signed tx hex.
- Broadcast BTC txid.
- Confirmation block height.
- Recipient ending balance.
- Nullifier query result.

Pass checklist:

- [ ] Local proof verification succeeds before submission.
- [ ] Public JSON is valid and contains all required fields.
- [ ] Merkle root is known for the denomination.
- [ ] Nullifier is initially unspent.
- [ ] Recipient is BTC regtest.
- [ ] Fee sats equals configured fee calculation.
- [ ] Withdrawal request is accepted.
- [ ] Withdrawal ID is deterministic from nullifier and recipient.
- [ ] Withdrawal query returns status `keysign_queued`.
- [ ] Nullifier query returns `spent=true` and the withdrawal ID.
- [ ] Outbound amount equals `denomination_sats - fee_sats`.
- [ ] Outbound vault pubkey equals the current BTC vault.
- [ ] FROST signing starts with expected vault ID, chain ID, signer set, and public key.
- [ ] FROST signing reaches `SIGNING_STATUS_COMPLETE`.
- [ ] Signature is 64 bytes for BIP340 path, or signed tx is valid.
- [ ] BTC tx broadcasts successfully.
- [ ] BTC tx confirms on regtest.
- [ ] Recipient balance increases by net sats minus any expected wallet display effects.
- [ ] Reusing the same proof/nullifier is rejected.

Negative checks:

- [ ] Invalid proof rejected.
- [ ] Invalid public JSON rejected.
- [ ] Unknown Merkle root rejected.
- [ ] Already spent nullifier rejected.
- [ ] Non-BTC recipient rejected.
- [ ] Incorrect fee rejected.
- [ ] Fee greater than or equal to denomination rejected.
- [ ] Signing with wrong vault pubkey rejected.
- [ ] Signing with wrong signer set rejected.
- [ ] Corrupted signature rejected.
- [ ] Expired signing session fails clearly.

Not complete until:

- [ ] Chain state, signer state, Bifrost outbound state, and BTC regtest state all agree on one completed withdrawal.

## Run Notes Template

Copy this block per run:

```text
Date:
Git commit:
Dirty files:
Flow:
Command:
Result:
Artifacts/log path:
Observed data:
Failures:
Next action:
```

## Full Harness

Use this when validating all sections in one pass:

```sh
ops/scenarios/run-all.sh
```

The full harness must not stop after the first failed section. It writes one log per section and one summary file under:

```text
ops/logs/protocol-tests/<timestamp>/RESULTS.md
```

Expected behavior while the e2e hooks are still missing:

- Flow 1 should pass.
- Flow 2 should pass.
- In `mock` profile, Flow 3/4/5 use `ops/scripts/mock-e2e-hooks.sh` unless real hook commands are set.
- Mock hooks validate scenario wiring and real BTC regtest wallet movement, but not real signer sidecars or real Thornado state transitions.
- In non-mock profile, Flow 3/4/5 require real `FROST_DKG_CMD`, `THORNADO_BOOTSTRAP_CMD`, `SEND_DEPOSIT_CMD`, and `RUN_WITHDRAWAL_CMD`.
