# Thornado Shielder — Relation & Public-Input Specification

Status: audit-prep specification for an external ZK auditor. Every claim is grounded
in source at the cited `path:line`. Paths are relative to the repository root
(`/Users/dev/dev/thornado`).

The Thornado shielder reuses the **Tornado Cash v2.1** `Withdraw(20)` Groth16 circuit
over the BN254 curve **verbatim** for the SNARK, and wraps it with a custom Go
value/denomination layer and Bitcoin settlement. The circuit is unmodified; all
Thornado-specific semantics live in the *interpretation and off-circuit enforcement*
of the public inputs. This document specifies both.

---

## 1. The statement the circuit proves

The main component is `Withdraw(20)` (`circuits/tornado/circom/withdraw.circom:70`).
It proves, in zero knowledge, knowledge of a witness `(nullifier, secret, pathElements,
pathIndices)` such that:

1. **Commitment membership.** The Pedersen commitment
   `commitment = Pedersen(nullifier_bits ‖ secret_bits)` is the leaf at position
   `pathIndices` in a depth-20 MiMC-sponge Merkle tree whose root equals the public
   input `root` (`withdraw.circom:44-55`, `MerkleTreeChecker(20)`).
   - `commitment = Pedersen(496)` over the concatenation of the 248-bit little-endian
     decomposition of `nullifier` followed by the 248-bit decomposition of `secret`
     (`withdraw.circom:15,17-25,27`). Total input width = 496 bits.
2. **Nullifier hash correctness.** `nullifierHash = Pedersen(248)` over the 248-bit
   decomposition of `nullifier` **only** (not the secret), and this equals the public
   input `nullifierHash` (`withdraw.circom:16,22,28,47`).
3. **Tamper resistance of the "opaque" inputs.** `recipient`, `relayer`, `fee`, `refund`
   are declared *"not taking part in any computations"* (`withdraw.circom:35-38`). They
   participate in no constraint except a self-square (`recipientSquare <== recipient *
   recipient`, etc., `withdraw.circom:60-67`) whose only purpose is to prevent the
   optimizer from dropping them, so that altering any of them invalidates the proof
   (`withdraw.circom:57-59`). The circuit imposes **no structure** on these four fields —
   each is treated as an arbitrary BN254 scalar field element.

Formally, for public inputs `(root, nullifierHash, recipient, relayer, fee, refund) ∈
Fr^6` the circuit is satisfiable iff there exist `(nullifier, secret)` and a valid Merkle
authentication path such that
`Pedersen(nullifier‖secret)` sits under `root` and `Pedersen(nullifier) = nullifierHash`.

The circuit says **nothing** about amounts, recipients, denominations, or Bitcoin. All of
that is bound off-circuit (Sections 3–4).

---

## 2. The six public inputs, in circuit order

The verifying key expects exactly 6 public inputs
(`crates/thornado-shielder/src/tornado/field.rs:7` `PUBLIC_INPUT_COUNT = 6`;
`groth16.rs:51` rejects a vk with `n_public != 6`). Order is fixed by a single
constructor with positional arguments, so it cannot drift between call sites
(`groth16.rs:139-148` `public_inputs_from_withdraw(root, nullifier_hash, recipient,
relayer, fee, refund)`, called from verify at `prove.rs:251` and from prove at
`prove.rs:178-185`).

| # | Circuit name  | Native Tornado meaning | **Thornado repurposed meaning** |
|---|---------------|------------------------|---------------------------------|
| 0 | `root`        | Merkle root            | Merkle root of the note's **per-denomination** tree (Section 4) |
| 1 | `nullifierHash` | Pedersen(nullifier)  | Same; the double-spend tag |
| 2 | `recipient`   | ETH payout address     | **`recipient_binding` hash** — a BN254 field element binding `(recipient string, fee_sats, denomination_sats)`. **NOT an address.** (Section 3) |
| 3 | `relayer`     | relayer address        | **Forced to 0** |
| 4 | `fee`         | relayer fee            | `fee_sats` (the Bitcoin withdrawal fee, as a field element) |
| 5 | `refund`      | ETH gas refund         | **Forced to 0** |

Because the circuit treats inputs 2–5 as opaque field elements
(`withdraw.circom:35-38`, audit answer Q7), the trusted-setup / soundness argument is
unaffected by this repurposing: no constraint ever assumed `recipient` was a small or
structured value. The repurposing is a semantic overlay enforced **entirely off-circuit**.

### Off-circuit enforcement of the repurposed semantics

The bindings on inputs 2, 3, 5 (and the `fee < denomination` bound) are checked in Rust
and mirrored in Go **before** the Groth16 pairing runs:

- **Rust** `validate_public_inputs` (`crates/thornado-shielder/src/tornado/prove.rs:66-80`):
  - `recipient` string must be non-empty (`:67-69`);
  - `fee_sats < denomination_sats` (`:70-72`);
  - the supplied `recipient_field`, if present, must equal the recomputed
    `recipient_binding` (`:73-76`, via `enforce_public_field` `:40-51`);
  - `relayer_field` and `refund_field`, if present, must be zero
    (`:77-78`, via `enforce_zero_public_field` `:53-64`).
  - `verify_withdrawal` recomputes `recipient = expected_recipient_binding(public)`,
    `relayer = 0`, `fee = u64_to_fr(fee_sats)`, `refund = 0` and feeds exactly those to
    the pairing (`prove.rs:236-254`) — the client-supplied fields are never trusted for
    the actual verification; they are only cross-checked.
- **Go** `ValidateRedeemPublicJSON` (`go-thornado/go-wrappers/shielder/public.go:21-63`):
  - denomination non-zero (`:32-34`), `fee_sats < denomination_sats` (`:35-37`),
    recipient non-empty (`:38-40`), no `note_commitment` field present (`:41-43`);
  - `relayer_field`/`refund_field` must be empty or `"0"`
    (`:44-49,65-71` `validateZeroBindingField`);
  - recomputes the expected `recipient_field` via `RecipientBindingField(...)` and
    compares (`:50-61`).

Go delegates the recipient binding to the **same Rust implementation** over FFI
(`public.go:50-54` → `go-thornado/go-wrappers/shielder/engine.go:127-131`
`RecipientBindingField` → `C.thornado_recipient_binding_field_json` →
`recipient_binding` in `hash.rs`). There is therefore **one** binding implementation;
Go does not re-implement it. This guarantees byte parity by construction, not by mirrored
code.

---

## 3. Recipient-binding construction (byte-for-byte, both sides)

The value placed in public input #2 is:

```
recipient_field = fr_from_be_bytes(
    SHA-256( LP("thornado-shielder-v1")
           ‖ LP("tornado-recipient-binding")
           ‖ LP(recipient)
           ‖ LP(dec(fee_sats))
           ‖ LP(dec(denomination_sats)) ) )
```

where `LP(x)` = `big-endian-u64(len(x)) ‖ x` (an 8-byte big-endian length prefix
followed by the raw UTF-8 bytes of `x`), `dec(n)` is the base-10 ASCII rendering of the
`u64`, and `fr_from_be_bytes` reduces the 32-byte digest modulo the BN254 scalar field.

### Rust (authoritative)

- `recipient_binding` (`crates/thornado-shielder/src/tornado/hash.rs:33-42`):
  ```rust
  let digest = crate::hash_parts_bytes(&[
      crate::DOMAIN,                 // "thornado-shielder-v1"
      "tornado-recipient-binding",
      recipient,
      &fee_sats.to_string(),
      &denomination_sats.to_string(),
  ]);
  super::field::fr_from_be_bytes(&digest)...
  ```
- `DOMAIN = "thornado-shielder-v1"` (`crates/thornado-shielder/src/lib.rs:18`).
- `hash_parts_bytes` (`lib.rs:860-867`): for each part, writes
  `(part.len() as u64).to_be_bytes()` then the part bytes into a single SHA-256, and
  returns the 32-byte digest. This is the length-prefixed, domain-separated hash.
- `fr_from_be_bytes` (`crates/thornado-shielder/src/tornado/field.rs:29-37`): interprets
  the digest big-endian as a `BigUint`, reduces `mod Fr::MODULUS` (BN254 r ≈ 2^254), and
  builds the field element. The digest is 256 bits over a ~254-bit field, so at most ~2
  bits of modular bias; collision resistance is ~2^127 (audit answer Q24).

### Go (delegates to Rust)

- `ValidateRedeemPublicJSON` computes the expected field via
  `RecipientBindingField(strings.TrimSpace(recipient), fee_sats, denomination_sats)`
  (`go-thornado/go-wrappers/shielder/public.go:50-54`), which is an FFI call straight into
  the Rust `recipient_binding` above (`engine.go:127-131`). **There is no independent Go
  reimplementation of the binding hash**, so the two sides are guaranteed identical.
- Go **does** independently implement the same length-prefixed SHA-256 primitive
  (`hashLengthPrefixedParts`, `go-thornado/x/thornado/shielder_flow.go:935-944`:
  `binary.BigEndian.PutUint64(length, len(part))` then the bytes) but only for the
  **shield-authorization** signature (`shielder_flow.go:902-909`), not for the recipient
  binding. It is byte-identical in construction to Rust `hash_parts_bytes`
  (`lib.rs:860-867`) and serves as a cross-check reference.

**Note for the auditor:** the recipient string is hashed **as-is** (only whitespace is
trimmed on the Go side, `public.go:51`); there is no address canonicalization before
hashing. Because the same string is used both for the binding and for the actual BTC
payout script (`withdrawal_flow.go:32`, `authorization.Recipient`), a non-canonical but
parseable address cannot be *redirected* — it pays exactly what was bound — but two
encodings of the same address produce different bindings (audit answer Q25).

---

## 4. Denomination binding (NOT in the circuit)

The circuit proves nothing about the note's denomination. Denomination integrity is
enforced entirely in the Go keeper via **per-denomination Merkle trees**, plus its
inclusion in the recipient binding (Section 3):

- Commitments are stored per `denominationSats` under a denomination-scoped key prefix
  (`go-thornado/x/thornado/keeper/v1/keeper_shielder.go:217-226`
  `SetShielderDenominationCommitment`; key layout
  `shielderDenominationCommitmentKey`/`shielderDenominationPrefix` `:274-280` —
  `prefixShielderDenomCommitment + %020d(denominationSats) + "/" + commitment`).
- Roots are stored per `denominationSats` (`keeper_shielder.go:248-257`
  `SetShielderMerkleRoot`; key `shielderMerkleRootKey` `:282-284`).
- At redeem, the root must exist **for the claimed denomination**:
  `ShielderMerkleRootExists(ctx, publicInputs.DenominationSats, publicInputs.MerkleRoot)`
  (`shielder_flow.go:1234`; existence check `keeper_shielder.go:259-268`).

The claimed `denomination_sats` flows from a single parsed struct into both the
recipient-binding recompute and the root lookup (`shielder_flow.go:1214,1234,1261`), so the
two uses cannot diverge (audit answer Q11). A commitment cannot be inserted into two
denomination trees: insertion first checks the **global** `ShielderCommitmentExists`
(`keeper_shielder.go:198-200`, keyed by commitment only) before writing the
per-denomination entry, and rejects duplicates (audit answer Q12). The payout amount is
`denomination_sats − fee_sats` (`withdrawal_flow.go:25`, with
`AmountSats = publicInputs.DenominationSats` at `shielder_flow.go:1261`).

**Consequence:** a note committed as denomination D can only satisfy a root registered for
D, and its redeem necessarily reveals D (it is a required public input for the root
lookup) — amount is public at withdrawal.

---

## 5. Note & tree format

| Element | Definition | Source |
|---------|------------|--------|
| Nullifier | 248-bit field element | `withdraw.circom:17,19`; Rust packs 31 bytes = 248 bits (`pedersen.rs:77-82`) |
| Secret | 248-bit field element | `withdraw.circom:18,20` |
| Commitment | `Pedersen(nullifier‖secret)` over 496 bits | `withdraw.circom:15,27`; Rust `note_commitment` concatenates two 31-byte limbs (`pedersen.rs:84-90`) |
| Nullifier hash | `Pedersen(nullifier)` over 248 bits | `withdraw.circom:16,28`; Rust `nullifier_hash` (`pedersen.rs:92-96`) |
| Pair hash | MiMC-sponge `hash_left_right = multi_hash([l,r], key=0, 1 output)` | `mimc_sponge.rs:72-74` |
| Tree depth | 20 (max 1,048,576 leaves per denomination) | `merkle.rs:11`; `withdraw.circom:70`; `ceremony.rs:9` |
| Root computation | full-leaf `incremental_root`, leaves in sorted order | Rust `mimc_sponge.rs:84-103`; Go feeds `sort.Strings`-sorted commitments (`keeper_shielder.go:244`) |

Go delegates the Merkle root computation to the same Rust engine over FFI
(`go-thornado/go-wrappers/shielder/shielder.go` → `merkle_root_hex` →
`incremental_root`), so there is no separate Go MiMC implementation (audit answer Q21).

### Zero-value deviation from upstream (flagged here, documented deliberately)

Upstream tornado-core uses a **nonzero** empty-leaf constant
`ZERO_VALUE = keccak256("tornado") mod p`. **Thornado deliberately uses a literal `0` /
`Fr::zero()` as the zero leaf and zero-subtree base**:

- `zero_leaf() = Fr::zero()` (`merkle.rs:20-22`);
- `zero_subtree(level)` starts from `Fr::zero()` and iterates
  `hash_left_right(value, value)` (`mimc_sponge.rs:76-82`).

This is self-consistent because Go always supplies the exact, full leaf set and the root is
recomputed identically on both the mint and redeem paths, and proofs are verified against
the pinned arkworks vk (never recomputed in-circuit here). It is **not byte-identical** to a
live Tornado pool's empty-subtree constants, and Thornado never interoperates with a live
Tornado tree. **This deviation is documented here explicitly for the auditor** (audit
answer Q22); it does not affect soundness in the Thornado context but must be understood
when comparing against upstream reference vectors.

---

## 6. Trusted-setup boundary

The verification key is the **Tornado Cash v2.1** Groth16 vk, reused as-is:

- Engine identity `tornado-cash-groth16-v2.1`, release `v2.1`, upstream
  `github.com/tornadocash/tornado-core` (`ceremony.rs:6-8`).
- Security model, verbatim from the code (`ceremony.rs:53-60`): the v2.1 MPC
  powers-of-tau-style ceremony is *"secure if at least one participant destroyed their
  toxic waste"* — semi-trustless, at least one honest participant suffices, and no single
  party can forge withdraw proofs for unknown secrets.
- The vk is **pinned by SHA-256**: the embedded vk file
  (`circuits/tornado/artifacts/withdraw_verification_key.json`, included at
  `ceremony.rs:12-13`) is hashed and compared to itself/attestation
  (`ceremony.rs:39-41,64-69`; `groth16.rs:150-164` `vk_digest_hex`).
  Expected digest `3eedcf6ec6b5c24219ed19c7a33966fbfa6a03ae7c84124270589091e87cf8d3`
  (`circuits/tornado/artifacts/MANIFEST.json`). The manifest also pins
  `withdraw_proving_key_sha256`
  (`9b1b2e7aed08ab0cc0c511710331ba2941e03162800e59e39dc65cb5f6f79daf`), though only the
  vk hash is enforced by Rust (audit answer Q6).

**Auditor caveat (from the self-assessment):** `semi_trustless_at_least_one_honest()`
(`ceremony.rs:64-69`) is a *self-consistency* check (release string + vk-hash-equals-itself),
not independent ceremony evidence, and there is currently no committed proof that the
compiled R1CS matches the pinned vk (the prove→verify round-trip is gated behind the
`proof-tests` cargo feature). See `docs/shielder-audit-answers.md` Q5/Q6 for the open items.

---

## 7. On-chain verification path (summary)

Redeem verification always runs arkworks `Groth16::verify` against the pinned vk:

Go `VerifyShielderRedeemJSON` (`go-thornado/go-wrappers/shielder/shielder.go`) → FFI
`thornado_verify_withdrawal_json` → Rust `verify_withdrawal` (`prove.rs:236-255`) →
`verify_snarkjs_proof` (`groth16.rs:67-89`) → `ark_groth16::Groth16::<Bn254>::verify`.
The public inputs fed to the pairing are recomputed on the verifier side
(`prove.rs:245-251`): `root` and `nullifierHash` from the public JSON, `recipient` from the
recomputed binding, `relayer = refund = 0`, `fee = fee_sats`. A `None` groth16 proof is
rejected (`prove.rs:252`); in default (non-`proof-tests`) builds the Rust prover produces
`groth16: None` (`prove.rs:193-194`), so production proofs are generated off the default Rust
path (browser/node prover) and only the verify path is exercised by the crate.
