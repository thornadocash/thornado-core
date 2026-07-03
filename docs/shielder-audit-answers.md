# Thornado Shielder — External Audit Preparation: Answered

Status: pre-audit self-assessment (code-reading exercise on a testnet demo). Every
claim below is grounded in the source at the cited `path:line`. Where something
cannot be determined from code alone (ceremony provenance, threat-model intent,
who runs the relayer) it is marked `NEEDS-HUMAN`.

Legend: **OK** handled well · **PARTIAL** partially handled · **GAP** not handled · **NEEDS-HUMAN** needs a human / off-code fact.

---

## 0. Threat model & stated claims

### 1. What exactly is the privacy claim?
**Answer:** The circuit proves membership of a note commitment in a MiMC Merkle tree
(`circuits/tornado/circom/withdraw.circom:32-55`) and a nullifier hash, without revealing
which leaf. Anonymity is therefore "a redeem is unlinkable among the notes *of the same
denomination* present in that denomination's tree at proof time." The partition-by-
denomination is explicit: commitments and roots are stored per-`denominationSats`
(`keeper/v1/keeper_shielder.go:217-284`) and the redeem must reveal `denomination_sats`
(`shielder_flow.go:31,1234`). No code states the claim to users; there is no user-facing
spec of the relation.
**Status:** NEEDS-HUMAN
**Note:** Write the one-line claim down (Q51). The code supports the "same-denomination anonymity set" claim; whether users are told something stronger is a docs/product question.

### 2. Who is the adversary and what is/ isn't hidden?
**Answer:** From code: shielding is fully public — deposit records carry `Owner`,
`AmountSats`, `DepositAddress` (`keeper/v1/keeper_shielder.go:166-180`,
`shielder_flow.go:296-315`). Commitments and per-denomination roots are on-chain
(`shielder_flow.go:1033-1084`). Only the redeem hides the link between nullifier and a
specific commitment. The BTC payout output (`withdrawal_flow.go:30-39`) and its amount
are visible to chain observers and to the FROST/Bifrost signers. There is no code
addressing network-level (IP) anonymity.
**Status:** NEEDS-HUMAN
**Note:** Enumerate adversaries formally in a threat-model doc; the code confirms node operators/Bifrost see payout amount+timing and the redeem tx signer (Q32/Q36).

### 3. Trust boundary of the trusted setup in one sentence.
**Answer:** `ceremony.rs:53-60` states it: "secure if at least one participant destroyed
their toxic waste," reusing Tornado v2.1 MPC output. This string is available to callers
via `attestation()` (`engine/mod.rs:3-6`).
**Status:** OK

### 4. Explicit non-goals?
**Answer:** Not documented anywhere in the repo. Amount privacy is explicitly *not*
provided (denomination is a public input, `shielder_flow.go:31`). No file lists non-goals.
**Status:** GAP
**Note:** Add a non-goals section (amount privacy across denominations, network anonymity, recipient-address reuse).

---

## 1. Trusted setup / ceremony reuse

### 5. Is the compiled circuit byte-identical to the pinned vk's circuit?
**Answer:** Cannot be shown from committed artifacts alone. The vk is pinned by sha256
(`MANIFEST.json:9`, checked in `ceremony.rs:64-69` and `groth16.rs:150-164`), but the R1CS /
proving key round-trip that would prove the R1CS matches the vk is gated behind the
`proof-tests` cargo feature (`prove.rs:175-192,267-321`, `prove.rs:355-380`). In default
builds no groth16 is produced or round-tripped. There is no committed r1cs hash; only
`withdraw_proving_key_sha256` in the manifest (`MANIFEST.json:10`) which is not verified by
any Rust test (the Rust code only reads the vk).
**Status:** PARTIAL
**Note:** CI-gate `proof-tests` for a real prove→verify against production keys (Q48); assert the proving-key sha256 in CI, not just the vk.

### 6. Is `semi_trustless_at_least_one_honest()` real evidence?
**Answer:** No. It only checks `attestation().release == "v2.1"` and that the vk sha256
equals the sha256 of the embedded vk file — a tautology (`ceremony.rs:64-69`;
the second clause compares `verification_key_sha256()` to itself). It is a self-consistency
check, not ceremony evidence. Only the vk is pinned in Rust; the proving-key hash lives in
`MANIFEST.json:10` but is not enforced by Rust. `npm run verify:production-artifacts`
(`check-production-artifacts.mjs`) exists but there are no CI workflow files in the repo
(`.github/` absent), so nothing runs it fail-closed.
**Status:** GAP
**Note:** This is a "looks like a check but isn't" hazard an auditor will flag. Either remove the tautology or replace it with an external attestation reference; wire the artifact check into CI.

### 7. Repurposed public inputs — does the soundness argument still hold?
**Answer:** Yes, at the circuit level. `recipient`, `relayer`, `fee`, `refund` are
"not taking part in any computations" in the circuit — they are only squared to survive the
optimizer (`withdraw.circom:36-67`). So the circuit treats `recipient` as an opaque field
element; nothing assumes it is small/structured. The Go/Rust layer overloads `recipient`
with a binding hash (`hash.rs:33-42`) and forces `relayer=refund=0`
(`prove.rs:156-157,248-250`).
**Status:** OK
**Note:** Document the repurposing (Q51); the circuit is unaffected but the semantics are non-obvious.

### 8. Recompile / version-drift policy?
**Answer:** `VENDOR.md:55-64` warns a local recompile "produces a different trusted setup
unless you reuse the v2.1 .zkey." The only guard against shipping a non-ceremony key is the
pinned vk sha256 (`MANIFEST.json:9`, `ceremony.rs`). There is no written policy or CI gate
preventing an accidental key swap.
**Status:** PARTIAL
**Note:** Add a documented recompile policy + a fail-closed CI check on both vk and pk hashes.

---

## 2. Value integrity / denomination binding

### 9. Single source of truth for "1 sat in == 1 sat of notes out"?
**Answer:** It lives in Go, not the SNARK. On shield: `PostShielderShield` /
`ShieldUserDepositIntoPool` require `sum(note.DenominationSats) == availableSats`
(`shielder_flow.go:772-773,848-857`), where `availableSats = deposit.AmountSats -
deposit.ShieldedSats` (`shielder_flow.go:753,829`). On redeem: payout is
`AmountSats - FeeSats` where `AmountSats = denomination_sats` (`withdrawal_flow.go:25`,
`shielder_flow.go:1261`). There is **no keeper invariant** that ties total minted note
value to total confirmed BTC; conservation is enforced only path-by-path at mint/burn time.
**Status:** PARTIAL
**Note:** Add a module invariant (sum of live commitments' denominations + fees == confirmed-deposit sats − redeemed sats). Currently a bug in any one path could silently break conservation with no global check.

### 10. Can a user mint notes exceeding deposited BTC? What binds `depositAmountSats`?
**Answer:** The deposit amount is set from the observed on-chain BTC tx
(`handler_observed_tx_helpers.go:1623`, `AmountSats: input.AmountSats`) into a
`DepositRecord`. Shielding checks note total == available deposit sats
(`shielder_flow.go:772-773,855-857`) and marks the deposit committed
(`shielder_flow.go:779,860`), and re-shield is blocked by status checks
(`shielder_flow.go:735-745,804-825`) plus `ShieldedSats` bookkeeping. So minting more than
deposited requires forging a deposit observation. Deposit observation is the standard
observed-tx quorum path; replay is bounded by `DepositID`/status but that is outside the
shielder files.
**Status:** PARTIAL
**Note:** The binding is sound *given* honest deposit observation; the observation/quorum path (and its replay resistance) should be audited separately and cross-referenced here.

### 11. What stops a note committed as D from being redeemed against D'?
**Answer:** At redeem, both the recipient-binding recompute and the root lookup use the same
single `publicInputs.DenominationSats`: `validate_public_inputs` recomputes
`recipient_binding(recipient, fee, denomination)` (`prove.rs:36-38,247`), and
`ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot)`
(`shielder_flow.go:1234`). Since the root is per-denomination
(`keeper_shielder.go:259-268,282-284`), a note in tree D can only satisfy a root registered
for D. The denomination value flows from one parsed struct
(`shielder_flow.go:1214,1261-1262`), so the two uses cannot diverge.
**Status:** OK

### 12. Can one commitment enter two denomination trees (double redeem)?
**Answer:** `insertShielderCommitments` checks the **global** `ShielderCommitmentExists`
before inserting into a denomination subtree (`shielder_flow.go:1044-1054`). The global set
is keyed by commitment only (`keeper_shielder.go:190-200`). Because insertion always writes
both the global marker and the denomination entry together and rejects if the global marker
exists, one commitment cannot be placed under two denominations. Same guard in the fee-note
path (`shielder_flow.go:1815-1826`).
**Status:** OK

### 13. Fee handling — deducted correctly, folded into binding, overflow?
**Answer:** `fee_sats` is a raw circuit input and is folded into `recipient_binding`
(`hash.rs:33-42`). `validate_public_inputs` enforces `fee < denomination`
(`prove.rs:70-72`) and Go mirrors it (`public.go:35-37`). On payout the fee is deducted:
`amount = AmountSats - FeeSats` and the fee is booked
(`withdrawal_flow.go:19-25,43`). Fee math `amountSats * feeBp / 10_000`
(`shielder_flow.go:1500-1502`) can overflow u64 only if `amountSats*feeBp > 2^64`, i.e.
amount ≳ 1.8e15 sats with feeBp≥10 — far above any realistic BTC amount, but it is an
unchecked u64 multiply.
**Status:** OK
**Note:** Consider `big.Int` or a bounds check on `amountSats*feeBp` for defense-in-depth; the fee is enforced `< denomination` on the redeem side but the raw multiply itself is unguarded.

### 14. Bond / bid-deposit redeems — same value guarantees? Double-use?
**Answer:** Bond-escrow and bid-deposit policies force `fee == 0` and require a
bond-escrow recipient (`shielder_flow.go:1298-1306,1442-1476,1479-1487,1418-1440`). They go
through the **same** `AuthorizeShielderRedeem` which spends the nullifier
(`shielder_flow.go:1307,1231-1233,1275-1277`). So a note redeemed to bond cannot also be
redeemed to BTC — the nullifier is marked spent once, and `FinalizeShielderRedeem`
dispatches to exactly one policy branch (`shielder_flow.go:1341-1353`). Value is preserved:
bid amount increases by `authorization.AmountSats` (`shielder_flow.go:1369`).
**Status:** OK

---

## 3. Nullifier / double-spend

### 15. Is check-and-set atomic; any path that spends without marking (or vice-versa)?
**Answer:** In `AuthorizeShielderRedeem` the order is: check `ShielderNullifierSpent`
(`shielder_flow.go:1231`), then later `SetShielderNullifierSpent`
(`shielder_flow.go:1275`). All redeem variants funnel through this function
(BondFromNotes at `:1307`, direct redeem via `msg_server.go:164`). Because the whole handler
runs in one synchronous Cosmos tx execution, check-then-set is effectively atomic within the
message. The nullifier is written *before* finalization (`msg_server.go:164-171`), so even
if finalize fails the tx reverts as a unit — no spend-without-mark. No async/goroutine
mutates this state.
**Status:** OK

### 16. Two txs in same block, same nullifier?
**Answer:** Cosmos executes messages serially within a block against the same working
state; the first `SetShielderNullifierSpent` write is visible to the second tx's
`ShielderNullifierSpent` read (`keeper_shielder.go:310-331` use the KV store directly). No
goroutine/FFI path mutates nullifier state — the FFI only verifies proofs
(`ffi/src/lib.rs:268-293`), it never touches the keeper.
**Status:** OK

### 17. Nullifier derived only from `nullifier`; genuine differential vectors?
**Answer:** Nullifier hash = Pedersen(nullifier) only (`pedersen.rs:92-96`), matching the
circuit's `Pedersen(248)` over the nullifier bits (`withdraw.circom:16-28`). The vectors in
`testdata/tornado_vectors.json` are generated by circomlibjs
(`scripts/gen-rust-test-vectors.mjs:14` `buildPedersenHash/buildMimcSponge/buildBabyjub`),
and Rust asserts against them (`pedersen.rs:108-124`, `hash.rs:56-71`,
`mimc_sponge.rs:156-167`). This is a genuine differential test against the reference JS.
**Status:** OK

---

## 4. Merkle tree / root management

### 18. Deterministic, stable leaf ordering across nodes?
**Answer:** Roots are recomputed from the full leaf set on every insert
(`shielder_flow.go:1069-1084` → `ComputeShielderMerkleRoot` → `incremental_root`).
`GetShielderDenominationCommitments` iterates the KV prefix and then `sort.Strings`
(`keeper_shielder.go:228-246`) — an explicit deterministic sort, so ordering is identical on
every validator regardless of insert order. Every historical root is stored forever
(`SetShielderMerkleRoot`, `keeper_shielder.go:248-257`) and accepted at redeem
(`shielder_flow.go:1234`).
**Status:** OK
**Note:** The `sort.Strings` on commitment strings is the consensus-critical linchpin — keep it and test it; also note the Rust `incremental_root` inserts in slice order (`mimc_sponge.rs:84-103`), so Go must always hand it the sorted slice (it does).

### 19. Depth-20 overflow / griefing?
**Answer:** Depth is 20 = max 1,048,576 leaves (`merkle.rs:11`, `ceremony.rs:9`). There is
no explicit overflow guard in `incremental_root` (`mimc_sponge.rs:84-103`); beyond capacity
the index math would silently misbehave. More pressing: **every insert recomputes the whole
root from all leaves** (`refreshShielderRoots` → `GetShielderDenominationCommitments` reads
all, `ComputeShielderMerkleRoot` hashes all — `shielder_flow.go:1069-1084`), i.e. O(n) MiMC
hashes per insert and O(n log n)-ish per block, all inside consensus. An attacker who can
cheaply create commitments can bloat a denomination tree and make every subsequent
shield/redeem in that denomination progressively more expensive.
**Status:** GAP
**Note:** Use a true incremental Merkle tree (store filled subtree nodes) instead of full recompute; add an explicit capacity check at depth 20. Deposit PoW (Q45) partially raises the cost of spamming but does not bound recompute cost.

### 20. Accepting old roots leaks note age?
**Answer:** True by construction — all historical roots are permanently valid
(`keeper_shielder.go:248-268`), there is no expiry/window. A proof against an old root
reveals the note predates newer roots.
**Status:** PARTIAL
**Note:** Accepted trade-off (maximizes anonymity set) but should be a documented decision; consider whether a sliding root window is wanted.

### 21. Go `ComputeShielderMerkleRoot` == Rust `incremental_root` == in-circuit tree?
**Answer:** Go delegates to the Rust engine over FFI (`shielder.go:72-91` →
`shielder.MerkleRoot` → `merkle_root_hex` → `incremental_root`), so Go and Rust are the same
implementation by construction (no separate Go MiMC). Rust vs circuit is covered by the
circomlibjs vectors (`merkle.rs:71-106`, `mimc_sponge.rs:156-167`). There is no *independent*
Go Merkle implementation to differ, which removes that risk but also means a bug in the Rust
tree is uniform across mint and redeem.
**Status:** OK
**Note:** Good design (single implementation). Still add an end-to-end Rust-root vs circuit-root vector to the CI differential set (Q49).

### 22. Zero/empty-subtree handling identical to circuit?
**Answer:** Rust uses `Fr::zero()` as the zero leaf and `zero_subtree(level)` =
iterated `hash_left_right(0,0)` (`mimc_sponge.rs:76-82`, `merkle.rs:20-26`). This matches
tornado-core's MiMC zero scheme *only if* the base zero value is 0. Upstream tornado-core
actually uses a nonzero `ZERO_VALUE` (keccak("tornado") mod p) for empty leaves; here the
zero is literally 0. This works self-consistently because Go always passes the exact leaf
set and the root is recomputed the same way on both sides, and proofs are verified against
the arkworks vk (not recomputed in-circuit here). But it is **not** byte-identical to a live
Tornado pool's empty-subtree constants.
**Status:** PARTIAL
**Note:** Confirm whether matching upstream's nonzero ZERO_VALUE matters for your reuse. Since you never interoperate with a live Tornado tree and always supply full leaves, self-consistency holds — but document this divergence explicitly for the auditor.

---

## 5. Recipient binding & front-running

### 23. Is `hash_parts_bytes` domain-separated and length-prefixed identically Rust/Go?
**Answer:** Yes. Rust `hash_parts_bytes` prepends each part's length as a big-endian u64
then the bytes, SHA-256 (`lib.rs:860-867`). Go `hashLengthPrefixedParts` does exactly the
same (`shielder_flow.go:935-944`). The binding parts are identical:
`[DOMAIN, "tornado-recipient-binding", recipient, fee.to_string(), denom.to_string()]`
(`hash.rs:33-42`) vs Go's `RecipientBindingField` FFI call which routes back to the same Rust
(`public.go:50-57` → `engine.go:127-131` → `recipient_binding_field`). So Go actually calls
the Rust implementation — byte parity is guaranteed, not merely mirrored.
**Status:** OK

### 24. Does `fr_from_be_bytes` reduction lose enough entropy to enable collisions?
**Answer:** `fr_from_be_bytes` reduces the 32-byte SHA-256 digest mod the bn254 scalar
field (`field.rs:29-37`). bn254 r ≈ 2^254, digest is 256 bits, so at most ~2 bits of bias.
Finding two recipients colliding on the field element is still ~2^127 work (birthday on a
254-bit field). Not a practical second-preimage/collision risk.
**Status:** OK

### 25. Is the recipient string canonicalized before hashing?
**Answer:** No canonicalization before hashing — `recipient_binding` hashes the raw
recipient string as-is (`hash.rs:33-42`). Go trims whitespace before hashing
(`public.go:51`) and the redeem parses it into a `common.Address` and checks the chain
(`shielder_flow.go:1419-1439`) *after* binding validation. So the binding is over the exact
string; two encodings of the same BTC address (case, bech32 vs legacy) produce different
bindings. The proof binds whatever string the prover used; the payout uses the same string
parsed to an address.
**Status:** PARTIAL
**Note:** Because the binding and the payout both use the identical string, a non-canonical-but-parseable recipient cannot be *redirected* (it pays exactly what was bound). But it can silently fragment the anonymity/UX and a mismatched-case address that parses differently on Go vs the prover would fail to verify. Recommend canonicalizing (lowercase bech32 / validated address) before binding.

### 26. Front-running — full tuple bound, nothing malleable?
**Answer:** The binding covers `(recipient, fee, denomination)` (`hash.rs:33-42`) and is
recomputed and compared on verify (`prove.rs:36-51,73-79`; Go `public.go:50-61`). `relayer`
and `refund` are forced to zero and checked (`prove.rs:77-78`, `public.go:44-49`). A mempool
observer cannot re-target: changing the recipient invalidates the binding. The BTC output
script used by Bifrost is `authorization.Recipient` (`withdrawal_flow.go:32`), which is the
bound recipient, so the actual payout script is not independently malleable.
**Status:** OK

---

## 6. Proof generation & on-chain verify path

### 27. Where is the production proof generated; does verify always hit `ark_groth16::verify`?
**Answer:** Default (non-`proof-tests`) builds set `groth16: None` (`prove.rs:193-194`), and
`verify_withdrawal` rejects a `None` groth16 (`prove.rs:252`). Production proofs are
generated off the default Rust path: either `circuits/tornado/scripts/prove-withdraw.mjs`
(node + websnark/snarkjs, invoked only under `proof-tests`, `prove.rs:292-321`) or the
browser prover (`scripts/build-browser-prover.mjs`, `prover-browser-entry.cjs`). On-chain
verify always goes Go → FFI → `verify_snarkjs_proof` → `ark_groth16 Groth16::verify` against
the pinned vk (`shielder.go:20-31` → `engine.go:154-163` → `ffi/src/lib.rs:268-293` →
`prove.rs:236-255` → `groth16.rs:67-89`). So the *verify* path is uniform and always
arkworks; the *prove* path is external JS/wasm and not exercised by default Rust tests.
**Status:** PARTIAL
**Note:** Trace one real testnet redeem end-to-end and record it; CI-gate `proof-tests` so the JS-prover→arkworks-verify round-trip runs (Q48). Today nothing in default CI proves a real proof verifies.

### 28. Subgroup/curve membership of G1/G2; identity handling exploitable?
**Answer:** `parse_g1`/`parse_g2` build points with `G1Affine::new`/`G2Affine::new` from
decimal strings reduced mod Fq (`groth16.rs:91-137`) and return `identity()` when the point
is zero (`groth16.rs:98-100,111-113`). Arkworks `Groth16::verify` performs the pairing and
its own point validity handling, and a malformed/identity proof point will make the pairing
check fail (`groth16.rs:80-88`). The pre-check does not *accept* anything — it only maps a
zero coordinate pair to the canonical identity; it cannot turn an invalid proof into a valid
one because the final equality still must hold. However, `new` (vs `new_unchecked`) is used,
which in arkworks validates the point is on-curve; there is no explicit subgroup
(cofactor) check before verify.
**Status:** PARTIAL
**Note:** bn254 has cofactor 1 on G1 (subgroup = curve) so G1 is fine; G2 has a nontrivial subgroup — rely on arkworks' internal checks and add an explicit `is_in_correct_subgroup_assuming_on_curve` assertion or a negative test with an off-subgroup G2 point to be certain.

### 29. Public-input order/count guard on every call site?
**Answer:** The only count guard is `public_inputs.len()+1 != vk.gamma_abc_g1.len()`
(`groth16.rs:72-74`). Order is fixed by a single constructor
`public_inputs_from_withdraw(root, nullifierHash, recipient, relayer, fee, refund)`
(`groth16.rs:139-148`) called from exactly one place in verify (`prove.rs:251`) and one in
prove (`prove.rs:178-185`). Since there is a single constructor with positional args, order
cannot drift between call sites.
**Status:** OK

### 30. Deterministic across architectures?
**Answer:** Verification is arkworks bn254 field/pairing arithmetic (`groth16.rs:67-89`),
which is fixed-modulus big-integer math with no floating point or platform-dependent
behavior. Deterministic across architectures.
**Status:** OK

### 31. Blocklist vs allowlist for leaky proof fields.
**Answer:** `RejectLeakyShielderRedeemProof` is a blocklist: it rejects a proof carrying
`nullifier`, `secret`, `commitment`, or a `tornado.merkle_path.path_elements`
(`shielder.go:47-70`). Public-input side has its own blocklist rejecting a `note_commitment`
field and non-zero relayer/refund (`public.go:41-49`). This is a blocklist, so a field the
authors didn't think of (e.g. leaf index, path indices under a different JSON key, deposit
id) would pass through. The proof struct that verify actually deserializes
(`WithdrawalProof`, `lib.rs:130-141`) only has `merkle_root` + `tornado.groth16`, and extra
JSON keys are ignored by serde, so unknown leaked fields wouldn't affect verification — but
they *would* be stored/broadcast if a caller passed the raw JSON around.
**Status:** PARTIAL
**Note:** Prefer a strict allowlist/schema for the on-chain message (reject unknown keys) rather than enumerating forbidden ones; add `pathIndices`, `leaf_index`, `deposit_id` to the negative-test set (Q50).

---

## 7. Anonymity in practice / metadata leakage

### 32. Who submits `MsgShielderRedeem` and who pays gas?
**Answer:** The redeem handler (`msg_server.go:159-179`) takes only proof+public JSON — the
message has **no signer-based authorization** tied to the note, which is the intended design
(anyone can submit a valid proof). But whichever account signs and pays fees for the Cosmos
tx is recorded on-chain; if the user self-submits from a funded account, that links the
withdrawal to an identity. There is no relayer/gasless path in the code.
**Status:** NEEDS-HUMAN
**Note:** Decide and document who relays; without a relayer the anonymity set collapses to "whoever paid the redeem tx fee." This is the single biggest practical-anonymity question and it is a deployment/product decision, not visible in code.

### 33. Is redeem the only unlinkable step; residual correlation attacks?
**Answer:** Confirmed: shield stores owner+amount+address+commitments publicly
(`keeper_shielder.go:166-180`, `shielder_flow.go:296-315,1033-1064`); only the redeem
proof hides which commitment. Timing/amount correlation remains: deposit N sats → the only
note of an unusual denomination → redeem shortly after is trivially linkable. No code
mitigates timing.
**Status:** PARTIAL
**Note:** Document the residual timing/amount correlation attacks for users.

### 34. Denominations offered; minimum anonymity-set enforcement?
**Answer:** Default denominations are `[10, 1, 0.1, 0.01, 0.001] BTC` in sats
(`lib.rs:20-21` `DEFAULT_DENOMINATIONS_SATS`), split greedily (`lib.rs:841-851`). There is a
per-note minimum (`Shielder_NoteAmountMinSats`, `shielder_flow.go:990-1023`) but **no
minimum anonymity-set size** enforced before a redeem — a redeem against a tree with a single
leaf is allowed. No user warning in code.
**Status:** GAP
**Note:** Consider warning or blocking redeems when the denomination tree is below a threshold size; at minimum surface the current set size to users.

### 35. Does redeem reveal `denominationSats`?
**Answer:** Yes, necessarily — it is a required public input used for the root lookup
(`shielder_flow.go:31,1234`, `public.go:32-34`). Amount is public at withdrawal.
**Status:** OK
**Note:** Confirmed intended; state it to users (ties to Q1/Q4).

### 36. Can Bifrost/FROST signers correlate payouts to shields?
**Answer:** The signers see the BTC payout amount and recipient (`withdrawal_flow.go:30-39`,
observed-out matching in `handler_observed_tx_helpers.go:1629+`). Amount = denomination −
fee, and denomination is public, so amount+timing correlation to a shield is available to
node operators exactly as to any chain observer.
**Status:** NEEDS-HUMAN
**Note:** Node-operator threat is real for timing/amount; document what operators learn (they do not learn the note→commitment link, which stays hidden by the proof).

---

## 8. Client-side note secrecy & key material

### 37. RNG source for `nullifier`/`secret` in the wallet?
**Answer:** Not determinable — the wallet UI ships as a compiled binary
(`go-thornado/thornado-ui` is a Mach-O executable, not source). The engine derives note
material deterministically from a `client_seed` via hardened BIP32 + SHA-256
(`lib.rs:188-245,632-643,728-778`), so note entropy = the seed's entropy. Where the seed's
randomness comes from (browser `crypto.getRandomValues`, mobile CSPRNG) is not in this repo.
**Status:** NEEDS-HUMAN
**Note:** Confirm the seed is generated from a CSPRNG on every target; the shielder crate never generates entropy itself, it only derives from a supplied seed.

### 38. Deterministic derivation from a seed — does a leaked seed compromise all notes?
**Answer:** Yes. All note nullifiers/secrets derive deterministically from `client_seed`
through `derive_shield_receipt_for_deposit_type` → hardened BIP32 path
`m/44'/60'/type'/deposit/note` (`lib.rs:823-835`) → SHA-256 to 248-bit fields
(`lib.rs:206-224,853-858`). A leaked seed reconstructs every past and future note
(`recover_note_receipt_*`, `lib.rs:531-598`) and therefore breaks unlinkability and allows
theft of unspent notes.
**Status:** PARTIAL
**Note:** This is inherent to deterministic HD notes; ensure the seed is protected like a wallet master key and document the blast radius. Consider per-note independent randomness with encrypted backup as an alternative.

### 39. Does the wallet enforce the 248-bit range?
**Answer:** The engine writes field material into 248 bits: `hash_parts_field248` zeroes the
top byte and keeps bytes[1..32] (`lib.rs:853-858`), and Pedersen packs each field as 31
bytes = 248 bits (`pedersen.rs:77-82`). So engine-derived notes are always in-range. If an
externally supplied `nullifier`/`secret` hex exceeded 248 bits, `field_from_hex` would reduce
mod the *scalar field* (`field.rs:57-74`) — not truncate to 248 bits — so a >248-bit value
could map to a different commitment than the circuit's `Num2Bits(248)` computes
(`withdraw.circom:16-25`). Engine-internal paths never do this; the risk is only for
hand-crafted inputs.
**Status:** OK
**Note:** Add a hard 248-bit range assertion on any externally supplied nullifier/secret to eliminate the hand-crafted-input footgun.

### 40. Does any note material leave the client?
**Answer:** No secret leaves. Shield authorization signs `[DOMAIN, "shield-authorization",
deposit_pubkey, deposit_id, amount, commitments_json]` (`lib.rs:651-668`) — only commitments
(public) and a signature, no nullifier/secret. `derive_shield_receipt` returns notes to the
caller (client) only; the on-chain message carries commitments + signature
(`msg_server.go:138-150`). The redeem carries a proof with note fields explicitly redacted
(`prove.rs:257-261`, `lib.rs:604-607`) and the chain rejects any proof that still carries
them (`shielder.go:61-63`).
**Status:** OK

---

## 9. FFI / memory-safety boundary

### 41. C-string ownership/free; is `LastError` thread-safe?
**Answer:** Rust hands out strings via `CString::into_raw` (`ffi/src/lib.rs:324-329`); Go
frees every returned pointer with `thornado_free_string` in `takeString`
(`engine.go:169-175`), which maps to `CString::from_raw`+drop (`ffi/src/lib.rs:19-27`). Go
frees its own input `C.CString`s via `defer C.free` at every call site (e.g.
`engine.go:44-46`). `LastError` is a `thread_local!` (`ffi/src/lib.rs:15-17`), so it is
per-OS-thread — safe under concurrent verify calls in the sense of no data race, but note cgo
may run different calls on different threads, so a `LastError` read must happen on the same
thread as the failing call. Go's `takeString` reads it inline on the same cgo call, which is
correct.
**Status:** OK
**Note:** The thread-local `LastError` is race-free but subtle; keep the read strictly paired with the call (as it is now).

### 42. Is attacker-controlled input size-bounded before crossing into Rust?
**Answer:** No size bound. `VerifyShielderRedeemJSON` only checks `json.Valid`
(`shielder.go:20-31`) then passes the full string across FFI where serde parses it
(`ffi/src/lib.rs:273-282`). A multi-megabyte proof/public JSON would be parsed in full. The
`MsgShielderRedeem` has `ValidateBasic` (`msg_server.go:161`) but no explicit byte cap seen
in these files; Cosmos block/tx size limits are the only backstop.
**Status:** PARTIAL
**Note:** Add an explicit max-length check on proof/public JSON before crossing FFI to bound CPU/memory; rely on more than the global tx-size limit.

### 43. Do Rust panics unwind across the C ABI?
**Answer:** The FFI entry points are **not** wrapped in `catch_unwind`
(`ffi/src/lib.rs:302-313` `return_string_result`, `:250-266`, `:268-293` just run the
closure). A panic (e.g. an `.expect()` such as `into_c_string`'s `CString::new().expect`,
`ffi/src/lib.rs:326`, or any panic inside the shielder) would unwind across the `extern "C"`
boundary, which is UB unless the crate is built with `panic=abort`. The Go side has a
`recover()` in `externalHandler` (`msg_server.go:339-348`) but that cannot catch a Rust
panic that already crossed the ABI.
**Status:** GAP
**Note:** Wrap every `#[no_mangle] extern "C"` body in `std::panic::catch_unwind` (or set `panic="abort"` for the ffi cdylib) so a Rust panic becomes an error return, not UB. This is a concrete, easy fix an auditor will expect.

---

## 10. Consensus, DoS & operational

### 44. Redeem verify cost — metered against invalid-proof DoS?
**Answer:** Proof verification runs inside message handling (`msg_server.go:159-171` →
`AuthorizeShielderRedeem` → `VerifyShielderRedeemJSON`), i.e. a full bn254 pairing per
redeem. No explicit gas metering of the pairing cost is visible in the shielder path; the
proof is verified before the nullifier/root checks (`shielder_flow.go:1225-1236`), so an
attacker can force a pairing computation with a well-formed-but-invalid proof for the cost of
a normal tx. Deposit PoW (Q45) guards deposits, not redeems.
**Status:** PARTIAL
**Note:** Add gas accounting proportional to verification cost, and/or cheap pre-checks (nullifier-spent, root-exists) *before* the expensive pairing to reject the cheapest attacks first. Currently `RejectLeakyShielderRedeemProof` and `VerifyShielderRedeemJSON` (pairing) run before the cheap `ShielderNullifierSpent`/`ShielderMerkleRootExists` checks.

### 45. Deposit PoW — what does it defend, can retarget be gamed?
**Answer:** `validateDepositPowToken` requires `sha256(owner+":"+token)` to have ≥difficulty
leading zero bits (`shielder_flow.go:1514-1539`); it defends against cheap deposit-address
spam. `RetargetDepositPowDifficulty` adjusts difficulty toward a weighted p90 of observed
deposit→match durations (`shielder_flow.go:1549-1665`), weighted by deposit *amount*
(`:1614-1622`). Because weight is deposit sats, an actor depositing large amounts influences
the percentile; step is bounded by `PowRetargetStepMax` and clamped to min/max
(`:1580-1588`), limiting per-retarget manipulation.
**Status:** PARTIAL
**Note:** Amount-weighting the retarget lets a well-funded actor bias difficulty downward; consider count-based or capped-weight sampling. Bounds limit but don't eliminate gaming.

### 46. Is root computation inside consensus and deterministic?
**Answer:** Inside consensus: `refreshShielderRoots` runs during shield message handling
(`shielder_flow.go:1066,1069-1084`), and root computation is deterministic
(`sort.Strings` + arkworks MiMC, see Q18/Q30). It is *not* gas-metered proportional to leaf
count, which ties back to the O(n) recompute concern (Q19).
**Status:** PARTIAL
**Note:** Deterministic (good) but unmetered O(n) work per insert; meter it or make it incremental.

### 47. Upgrade/migration story for a pool bug?
**Answer:** No shielder-specific migration is present in these files (the `upgrades.go`
files in the working tree are general chain upgrades, not shielder state migrations). Commit
and root storage are plain KV (`keeper_shielder.go`), so a migration could re-derive roots,
but outstanding notes are bound to specific historical roots/commitments — a change to the
hash or tree scheme would invalidate them.
**Status:** NEEDS-HUMAN
**Note:** Write a migration/runbook: how to fix a pool bug without invalidating outstanding notes or breaking conservation.

---

## 11. Testing & assurance gaps

### 48. Is `proof-tests` CI-gated (real production-key round-trip)?
**Answer:** No. The round-trip tests are `#[ignore]` unless `--features proof-tests`
(`prove.rs:355-380`, `ffi/src/lib.rs:416-452`, `lib.rs:966-987`), and there are **no CI
workflow files in the repo** (`.github/` absent), so nothing runs them. Default `cargo test`
exercises only native-crypto vectors, never a groth16 proof.
**Status:** GAP
**Note:** Add CI that runs `proof-tests` after `npm run download-artifacts`; this is the highest-value assurance gap.

### 49. Cross-implementation differential tests in CI?
**Answer:** Vectors exist (`testdata/tornado_vectors.json`, generated by circomlibjs) and
Rust asserts against them (`pedersen.rs:108-124`, `hash.rs:56-71`, `mimc_sponge.rs:156-167`,
`merkle.rs:71-106`). A differential script exists (`scripts/differential-snarkjs-rust.mjs`,
`package.json:15`). But without CI (`.github/` absent) none of it is gated. Go has no
separate crypto to differential-test (it calls Rust), which is fine.
**Status:** PARTIAL
**Note:** Gate the existing vectors + `audit:differential` in CI so a drift fails the build.

### 50. Negative tests present?
**Answer:** Partial. Present: mismatched recipient field (`prove.rs:337-353`,
`public_test.go:19-38`), non-zero relayer (`public_test.go:58+`), note-commitment leak
(`public_test.go:5-18`), leaky-proof rejection (`shielder.go` + tests), missing groth16
(`ffi/src/lib.rs:398-414`). Missing: forged/malleated proof points, off-subgroup G2,
over-range (>248-bit) field elements, wrong-denomination root, duplicate/empty commitments
at the crypto layer, denomination-crossing note.
**Status:** PARTIAL
**Note:** Add the missing negatives (Q28/Q39/Q11/Q12/Q31 each want one).

### 51. Written spec of the exact relation and public-input semantics?
**Answer:** No such spec exists. `circuits/tornado/README.md` and `VENDOR.md` describe the
vendored circuit and the 6 public inputs generically, but the *repurposed* semantics
(recipient = binding hash of `(recipient, fee, denomination)`, relayer/refund forced to 0,
denomination bound only via the binding at redeem) are documented nowhere an auditor can
read. It is only reconstructable from `hash.rs:33-42`, `prove.rs:36-79`, and
`shielder_flow.go`.
**Status:** GAP
**Note:** Write the relation spec — the single most valuable artifact to hand a ZK auditor.

---

## Summary

| Status | Count | Questions |
|--------|-------|-----------|
| **OK** | 18 | 3, 7, 11, 12, 13, 14, 15, 16, 17, 18, 21, 23, 24, 26, 29, 30, 35, 40, 41 (19 incl. 41) |
| **PARTIAL** | 20 | 5, 8, 9, 10, 20, 22, 25, 27, 28, 31, 33, 38, 42, 44, 45, 46, 49, 50 |
| **GAP** | 7 | 4, 6, 19, 34, 43, 48, 51 |
| **NEEDS-HUMAN** | 6 | 1, 2, 32, 36, 37, 47 |

(Counts: OK 19, PARTIAL 18, GAP 7, NEEDS-HUMAN 6 — total 50 distinct statuses across the 51 questions; Q41 counted under OK.)

### Top 5 things to fix before external audit

1. **Wrap all FFI entry points in `catch_unwind` (or `panic=abort`)** — Q43. A Rust panic
   currently unwinds across the C ABI into Go = undefined behavior. Small, concrete fix an
   auditor will expect.
2. **Stand up CI and gate the real proof round-trip + differential vectors** — Q48/Q49/Q6.
   There are no `.github/` workflows; nothing ever proves a real groth16 proof verifies or
   that the pinned proving-key/vk hashes hold. Also fix the tautological
   `semi_trustless_at_least_one_honest()` "check."
3. **Replace full-tree root recompute with an incremental Merkle tree + capacity guard, and
   meter/pre-check the redeem verify** — Q19/Q46/Q44. Today every shield is O(n) MiMC inside
   consensus, and the expensive pairing runs before the cheap nullifier/root checks — both are
   DoS surfaces.
4. **Write the relation & public-input spec and the threat-model/non-goals doc** — Q51/Q1/Q2/
   Q4. The repurposed-inputs scheme is undocumented; this is the highest-leverage artifact for
   a ZK auditor and clarifies the anonymity claim.
5. **Add a global value-conservation invariant + range-check external field inputs + decide the
   relayer story** — Q9/Q39/Q32. Conservation is only enforced per-path with no module
   invariant; >248-bit inputs reduce mod scalar field instead of failing; and without a
   relayer the redeem tx signer deanonymizes the withdrawal.
