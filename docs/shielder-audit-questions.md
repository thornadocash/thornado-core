# Thornado Shielder — External Audit Preparation Questions

Status: pre-audit self-assessment. The shielder is a Tornado Cash v2.1 Groth16 pool
reused verbatim for the SNARK, wrapped with a custom Go value/denomination layer and
Bitcoin settlement. This document lists the questions an external auditor should be able
to answer (and that we should answer for ourselves first), grouped by area and ordered
roughly by severity.

Key files:
- Rust engine: `crates/thornado-shielder/src/tornado/{prove,groth16,hash,merkle,pedersen,mimc_sponge,ceremony}.rs`
- FFI: `crates/thornado-ffi`, `crates/thornado-web-wasm`, `go-thornado/go-wrappers/shielder/engine.go`
- On-chain: `go-thornado/x/thornado/shielder_flow.go`, `shielder.go`, `msg_server.go`,
  `keeper/v1/keeper_shielder.go`
- Circuits: `circuits/tornado/` (vendored), `VENDOR.md`, `README.md`

---

## 0. Threat model & stated claims (answer these first)

1. What exactly is the privacy claim? Precisely: "a redeem cannot be linked to a shield,
   *among the set of notes of the same denomination present in the tree at proof time*."
   Is that the claim we make publicly, or do users believe something stronger?
2. Who is the adversary? (chain observer, validator/node operator, the BTC watchtower/Bifrost
   signers, a malicious relayer/submitter, a global passive network observer). For each, what
   is and is not hidden?
3. What is the trust boundary of the trusted setup, stated in one sentence a user can read?
   ("Secure if ≥1 of the Tornado v2.1 MPC participants destroyed their toxic waste.")
4. What are the explicit *non-goals*? (e.g. amount privacy across denominations, network-level
   anonymity, protection against a user reusing the same recipient address.)

---

## 1. Trusted setup / ceremony reuse  (HIGH)

5. Is our compiled circuit **byte-identical** to the circuit the pinned v2.1 verification key
   was generated for? We reuse Tornado's vk but define our own witness semantics — soundness
   only holds if the R1CS matches the vk. What is the proof of this? (r1cs hash vs. ceremony
   artifact, or a full proving/verifying round-trip against production keys — currently gated
   behind the `proof-tests` cargo feature.)
6. `ceremony.rs::semi_trustless_at_least_one_honest()` only checks `release == "v2.1"` and that
   the vk sha256 matches the embedded file. That is a self-consistency check, not evidence about
   the ceremony. What independent evidence do we have that the v2.1 MPC (a) actually completed as
   claimed and (b) has ≥1 honest contributor? Do we pin the *proving key* hash too, or only the vk?
   (`MANIFEST.json` claims to pin both — verify `npm run verify:production-artifacts` actually runs
   in CI and fails closed.)
7. We *repurposed* the public inputs (recipient = binding hash, relayer/refund = 0). Does the
   toxic-waste / soundness argument still apply unchanged? (It should, since the circuit treats
   these as opaque field elements — but the auditor should confirm no constraint assumed
   `recipient` was small/structured.)
8. What is our policy if we ever need to recompile the circuit (circom/snarkjs version drift)?
   `VENDOR.md` notes a local recompile produces a *different* setup — how do we prevent an
   accidental non-ceremony key from shipping?

---

## 2. Value integrity / denomination binding  (HIGH — this is the custom part)

9. The SNARK proves nothing about *amount*. Value conservation between (BTC deposited → notes
   minted) and (notes burned → BTC paid out) lives entirely in Go. Where is the single source
   of truth for "1 satoshi in == 1 satoshi of notes out", and is it covered by a keeper invariant?
10. Shield path: `parseShielderNoteCommitments` + `applyShielderNoteFloor` + `shielderDustRemainder`
    decide which denominations a deposit mints. Can a user mint notes whose denomination total
    exceeds the BTC they actually deposited/confirmed? What binds `depositAmountSats` to the
    confirmed on-chain BTC observation, and can that observation be replayed/spoofed?
11. A commitment is inserted under a specific `denominationSats` tree, but `denominationSats` is
    only bound into the proof via `recipient_binding` at *redeem* time. What prevents a note
    committed as denomination D from being redeemed against a tree of denomination D'? (Today:
    the recipient-binding recompute uses the redeem's claimed `denominationSats`, and the root
    lookup uses the same value — confirm both always come from one authenticated source and can't
    diverge.)
12. `ShielderCommitmentExists` is a **global** uniqueness check but commitments are also stored
    per-denomination. Confirm a single commitment can never be inserted into two denomination
    trees (which would let one note be redeemed twice at two amounts).
13. Fee handling: `fee_sats` is both a raw circuit public input *and* folded into
    `recipient_binding`. `validate_public_inputs` enforces `fee < denomination`. Is the fee
    actually deducted from the paid-out BTC, and does the accounting match what the proof
    committed to? Any integer overflow in `amountSats * feeBp / 10_000`?
14. Node-bond / bid-deposit redeem policies (`ShielderRedeemPolicyBondEscrow`, `BidDeposit`) force
    `fee == 0` and redirect the recipient to a bond escrow. Does a shielded note therefore carry
    the same value guarantees when converted to bond as when withdrawn to BTC? Can a user
    double-use one note across a BTC redeem and a bond-from-notes redeem?

---

## 3. Nullifier / double-spend  (HIGH)

15. Nullifier hash = Pedersen(nullifier), matching upstream. Spent set is `prefixShielderNullifier`
    in the keeper, checked in `AuthorizeShielderRedeem` *before* `SetShielderNullifierSpent`. Is
    the check-and-set atomic within the message handler, and is there any code path
    (BondFromNotes, FinalizeShielderRedeem, retries, errata) that spends a note without marking
    the nullifier, or marks it without spending?
16. Can two txs in the same block both pass the `ShielderNullifierSpent` check for the same
    nullifier before either writes? (Cosmos serial execution should prevent it — confirm, and
    confirm no async/goroutine path in the FFI mutates state.)
17. Nullifier is derived only from `nullifier` (not `secret`). Is that consistent with the circuit,
    and does it match upstream exactly? (Vectors in `testdata/tornado_vectors.json` — confirm
    these are generated by circomlibjs, not by our own Rust, i.e. a genuine differential test.)

---

## 4. Merkle tree / root management  (MEDIUM–HIGH)

18. Roots are recomputed from the **full** leaf set on every insert (`incremental_root` /
    `ComputeShielderMerkleRoot(leaves)`), and every historical root is stored forever and accepted
    on redeem. Is the leaf ordering deterministic and stable across nodes (consensus-critical)?
    `GetShielderDenominationCommitments` ordering must be identical on every validator or roots
    diverge → chain halt / fork.
19. Tree depth is 20 (max 1,048,576 leaves per denomination). What happens at overflow? Is there a
    griefing/DoS vector where an attacker spams cheap commitments to fill or bloat a denomination
    tree (O(n) root recompute per insert)?
20. Accepting all historical roots maximizes the anonymity set but means a redeem proof against a
    very old root reveals the note is "old." Is that an acceptable leak? Is there any root
    expiry/window, and should there be?
21. Is the on-chain Merkle implementation (Go `ComputeShielderMerkleRoot`) proven equivalent to the
    Rust `incremental_root` and to the in-circuit MiMC tree? A mismatch = unprovable/unspendable
    notes or accepted invalid roots. Where is the cross-implementation differential test?
22. Zero/empty-subtree handling: do the Go and Rust implementations use the identical MiMC "zero
    values" as the Tornado circuit for unfilled positions?

---

## 5. Recipient binding & front-running  (MEDIUM)

23. `recipient_binding = fr_from_be_bytes(H(DOMAIN, "tornado-recipient-binding", recipient,
    fee_sats, denomination_sats))`. Which hash is `hash_parts_bytes`, and is it domain-separated and
    length-prefixed on **both** the Rust and Go sides identically? (`hashLengthPrefixedParts` in Go —
    confirm byte-for-byte parity, otherwise proofs won't verify or bindings can collide.)
24. `fr_from_be_bytes` reduces the digest mod the bn254 scalar field. Does that reduction lose
    enough entropy to matter for second-preimage/collision on (recipient, fee, denomination)? Two
    distinct recipients mapping to the same field element would let an attacker redirect a
    withdrawal.
25. Is the recipient string canonicalized before hashing (address encoding, case, checksum)? A
    non-canonical recipient that still parses could break the binding or enable redirection.
26. Front-running: the binding pins the recipient into the proof, so a mempool observer can't
    re-target the payout. Confirm the whole `(recipient, fee, denomination)` tuple is bound and
    that nothing in the redeem message is malleable/unbound (e.g. the BTC output script actually
    used by Bifrost).

---

## 6. Proof generation & the on-chain verify path  (MEDIUM)

27. In non-`proof-tests` builds `withdrawal_proof()` sets `groth16: None`; `verify_withdrawal`
    rejects a `None` groth16. So where is the *production* proof generated (WASM
    `thornado-web-wasm`? node `prove-withdraw.mjs`?) and does the production on-chain verify path
    always go through `ark_groth16::verify` against the pinned vk? Trace one real testnet redeem
    end-to-end.
28. `verify_snarkjs_proof` parses proof points from decimal strings and reduces mod field. Are
    G1/G2 points checked for subgroup membership / curve membership? `parse_g1` returns
    `identity()` when the point is zero — can a crafted proof exploit the identity/degenerate-point
    handling to pass verification? (arkworks `verify` should reject, but confirm no pre-check
    silently normalizes a malicious point.)
29. `public_inputs.len() + 1 != vk.gamma_abc_g1.len()` is the only public-input count guard.
    Confirm the 6 public inputs are always passed in the exact order the vk expects
    (root, nullifierHash, recipient, relayer, fee, refund) on every call site.
30. Is proof verification deterministic and identical across all validator architectures
    (arkworks over bn254)? Any nondeterminism = consensus fork.
31. `RejectLeakyShielderRedeemProof` is a *blocklist* (rejects proofs carrying nullifier/secret/
    commitment/merkle path). Is a blocklist the right design vs. a strict schema/allowlist? What
    other fields could a client accidentally leak that aren't checked (e.g. leaf index, path
    indices, deposit id)?

---

## 7. Anonymity in practice / metadata leakage  (MEDIUM)

32. Who submits the `MsgShielderRedeem` transaction, and what account signs/pays for it? If the
    user's own funded account submits, the tx signer links the withdrawal to an identity,
    defeating the pool. Is there a relayer/gasless path, and if so who runs it and what do they see?
33. Shielding is fully public (deposit address, owner, amount, commitments all on-chain). Confirm
    the *only* unlinkable step is the redeem, and document the timing/amount correlation attacks
    that remain (e.g. deposit N sats then withdraw the only note of that denomination shortly
    after).
34. Denominations partition the anonymity set. What denominations do we offer, and how small can an
    anonymity set get before a redeem is effectively deanonymized? Do we warn users, or enforce a
    minimum set size before allowing a redeem?
35. Does the redeem reveal the `denominationSats` (it must, for the root lookup)? So amount is
    public at withdrawal — confirm this is the intended model and stated to users.
36. Bifrost / FROST signers see the BTC payout output. Can they correlate payouts back to shields
    via amount + timing? What do node operators learn?

---

## 8. Client-side note secrecy & key material  (MEDIUM)

37. Where do `nullifier` and `secret` entropy come from in the wallet (`thornado-web-wasm`)? What
    RNG, and is it CSPRNG on every target (browser, mobile)? Weak entropy → note theft or
    linkability.
38. How are notes backed up / recovered by the user? Is there a deterministic derivation from a
    seed (`ClientPubKeyForDeposit`, `DeriveShieldReceipt` suggest yes) — if so, does a leaked seed
    compromise *all* past and future notes and break their unlinkability?
39. Note format: 248-bit nullifier + 248-bit secret (Pedersen). Confirm the wallet enforces the
    248-bit range so field elements never exceed the intended domain (over-large values could map
    to a different commitment than the circuit computes).
40. Is any note material ever transmitted to the server/chain (shield authorization signatures,
    receipts)? Trace `ShieldAuthorization` / `DeriveShieldReceipt` to confirm no secret leaves the
    client.

---

## 9. FFI / memory safety boundary  (MEDIUM)

41. The Go↔Rust boundary (`thornado-ffi`, `go-wrappers/shielder/engine.go`) passes JSON across a C
    ABI. Who owns/frees the returned C strings? Any leak or double-free (`takeString`, `LastError`
    global)? Is `LastError` global state thread-safe under concurrent verify calls?
42. Is all attacker-controlled input (proof JSON, public JSON) size-bounded before crossing into
    Rust? Can a huge/malicious JSON cause OOM or panic that crosses the FFI as UB rather than an
    error?
43. Do any Rust panics unwind across the C ABI (UB)? Are all FFI entry points wrapped in
    `catch_unwind`?

---

## 10. Consensus, DoS & operational  (MEDIUM–LOW)

44. Proof verification cost: what is the gas/CPU cost of a redeem, and is it metered so a stream of
    invalid proofs can't cheaply DoS block production?
45. Deposit PoW (`validateDepositPowToken`, `RetargetDepositPowDifficulty`) — what attack is this
    defending against, and can the retarget be gamed to lower difficulty and spam the pool?
46. Determinism of `refreshShielderRoots` across nodes (see Q18) — is root computation inside
    consensus (deterministic, gas-metered) or off-chain?
47. Upgrade/migration story: if a bug is found in the pool, can commitments/roots be migrated
    without invalidating outstanding notes or breaking value conservation?

---

## 11. Testing & assurance gaps to close before the audit

48. Enable and CI-gate the `proof-tests` feature so a real production-key round-trip runs, not just
    native-crypto vectors.
49. Cross-implementation differential tests: Rust vs. Go vs. circomlibjs for Pedersen, MiMC-sponge,
    nullifier hash, and Merkle root — asserted in CI.
50. Negative tests: forged proof, reused nullifier, wrong denomination root, mismatched
    recipient_binding, malleated proof points, over-range field elements, empty/duplicate
    commitments, denomination-crossing note.
51. A written spec of the *exact* relation and public-input semantics (this repurposed-inputs
    scheme is not documented anywhere the auditor can read it) — the single most valuable artifact
    to hand an external ZK auditor.
