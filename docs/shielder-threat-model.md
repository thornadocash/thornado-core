# Thornado Shielder — Threat Model & Privacy Claim

Status: audit-prep threat model for an external auditor. Every technical claim is grounded
in source at the cited `path:line`. Paths are relative to the repository root
(`/Users/dev/dev/thornado`). Read alongside `docs/shielder-relation-spec.md` (the ZK
relation) and `docs/shielder-audit-answers.md` (the code-reading self-assessment).

---

## 1. The precise privacy claim

> **A redeem is unlinkable to its shield only among the set of same-denomination notes
> present in that denomination's Merkle tree at proof time.**

The single unlinkable step is the redeem: the Groth16 proof demonstrates that *some*
commitment in the denomination's tree is being spent, without revealing *which*
(`circuits/tornado/circom/withdraw.circom:44-55`). Everything else about the shielder is
public (Sections 3–4).

The anonymity set is therefore bounded, per redeem, by the number of live leaves in the
**same** `denominationSats` tree at the root the proof was built against. Denominations
strictly partition the anonymity set because commitments and roots are stored per
denomination (`keeper/v1/keeper_shielder.go:217-284`) and the redeem must reveal
`denomination_sats` for the root lookup (`shielder_flow.go:1234`; `public.go:32-34`).

### Correction to the whitepaper phrasing

`THORNADO.pdf` currently states that a note *"is completely fungible with all other equal
denomination notes."* This is **overstated** and should be corrected. Two qualifications
apply:

1. **Fungibility is scoped, not absolute.** A note is only interchangeable with the *other
   notes of the same denomination that already exist in the tree at proof time* — not with
   "all" notes and not across denominations. The correct phrasing is "unlinkable among the
   same-denomination anonymity set present at proof time."
2. **Fungibility ≠ unlinkability against timing/amount.** Even within a denomination, a
   redeem that follows shortly after the only shield of an uncommon denomination is
   trivially linkable by timing and amount (Section 5, non-goals). "Completely fungible"
   implies a stronger, deployment-independent guarantee than the code provides.

---

## 2. What is public vs. hidden — the shield side is fully transparent

Only the redeem hides anything. The **shield** side is entirely public on-chain:

- Deposit records carry `Owner`, `AmountSats`, `DepositAddress`
  (`keeper/v1/keeper_shielder.go:166-180`; `shielder_flow.go:296-315`).
- Note commitments and per-denomination Merkle roots are stored and queryable
  (`shielder_flow.go:1033-1084`; `keeper_shielder.go:190-268`).

So an observer knows *who deposited how much and minted which commitments*. The proof only
breaks the link between a spent `nullifierHash` and the specific commitment it burns.

At redeem, the following are written to chain state and exposed via the query API
(`querier.go:2528-2549` `shielderRedeemResponse`, `:2551-2573`
`shielderNullifierResponse`): `nullifierHash`, `merkleRoot`, `recipient` (the **BTC payout
address**, `withdrawal.Recipient.String()`), `amountSats` (= `denomination_sats`,
`shielder_flow.go:1261`), `feeSats`, and `inHash`. The `inHash` is
`shielderRedeemID = SHA256(nullifierHash | recipient | policy)` uppercased
(`shielder_flow.go:1248-1249,2041-2044`) — i.e. it is deterministically derived from the
public redeem fields and is itself public.

The BTC payout output (address + amount = `denomination − fee`) is constructed at
`withdrawal_flow.go:25,30-39` and is visible to any chain observer and to the Bitcoin
signers.

---

## 3. Per-adversary analysis

"Redeem link" below means the association between a spent `nullifierHash` and the specific
note commitment / shield that created it — the *only* thing the proof hides.

| Adversary | HIDDEN | VISIBLE |
|-----------|--------|---------|
| **BTC chain observer** | The redeem link (which shield funded a given payout) | The BTC deposit tx (shield funding); the BTC payout address + amount (= denomination − fee) and its timing (`withdrawal_flow.go:30-39`). Can attempt timing/amount correlation. |
| **Thornado validator / node operator** | The redeem link | Everything on-chain: shield `Owner`/`AmountSats`/`DepositAddress` (`keeper_shielder.go:166-180`), all commitments + roots, and every redeem field: `nullifierHash`, `denominationSats` (= `amountSats`), `feeSats`, BTC `recipient`, `inHash` (`querier.go:2528-2549`). Sees redeem tx timing. |
| **Bifrost / FROST signer** | The redeem link | The full outbound BTC payout it co-signs — recipient address + amount + `inHash` (`withdrawal_flow.go:30-39`; observed-out matching `handler_observed_tx_helpers.go`). Amount = denomination − fee, and denomination is public, so amount+timing correlation is available exactly as to a chain observer (audit answer Q36). |
| **Relayer / broadcaster** (the node serving `go-thornado/ui/ui.go`) | The redeem link (still protected by the proof) | The **submitter's network identity (source IP)** of the HTTP POST and the **full proof + public JSON** (`ui/ui.go:47` routes `/withdraw` → `withdrawMsg` `ui.go:181-190`, which reads `{proof, public}` and builds `MsgShielderRedeem`, then `browserHandler` calls `BroadcastTx`, `ui.go:107`). It therefore learns `nullifierHash`, `denominationSats`, `feeSats`, `recipient`, and can associate them with the submitter's IP and request timing. |
| **Global passive network observer** | The redeem link; note contents | Timing and size of all traffic; can correlate a shield-side request and a redeem-side request from the same IP/session absent network anonymization (none is enforced in code). |

Common to **all** adversaries: `recipient` (BTC address), `denomination_sats`, `fee_sats`,
`nullifierHash`, and `inHash` (= `SHA256(nullifierHash|recipient|policy)`) are public. The
shield side (owner, deposit amount, commitments) is fully public. The **only** protected
quantity anywhere in the system is the mapping from a redeemed `nullifierHash` back to a
specific commitment/shield.

### Redeem is signer-less, fee-less, gasless

`MsgShielderRedeem.GetSigners()` returns `nil`
(`go-thornado/x/thornado/types/msg_shielder.go:171`), and the ante decorator treats it as an
**intrinsic-auth native tx** that bypasses signature/fee verification
(`ante.go:54-56,61-75` `isIntrinsicAuthNativeTx`, which returns without calling `next` for
txs consisting only of `MsgDepositRequestPow`, `MsgShielderShield`, `MsgShielderRedeem`).
This is deliberate: **anyone** can submit a valid proof, there is no account tied to the
note, and the submitter pays no gas — which removes the "the fee-paying signer deanonymizes
the withdrawal" problem that a normal signed tx would create. The residual deanonymization
vector is therefore **network-level** (the relayer/broadcaster IP), not a Cosmos signer
(audit answer Q32).

---

## 4. Trusted-setup trust boundary

The shielder's soundness rests on the reused **Tornado Cash v2.1** Groth16 ceremony:
**the system is secure if at least one participant of the v2.1 MPC destroyed their toxic
waste** (`crates/thornado-shielder/src/tornado/ceremony.rs:53-60`). The verification key is
pinned by SHA-256 (`ceremony.rs:39-41,64-69`;
`MANIFEST.json` digest `3eedcf6…87cf8d3`). If that assumption fails (all participants
colluded and retained the toxic waste), an adversary could forge withdraw proofs and mint
BTC payouts without a genuine note — a total value-integrity break, independent of the
privacy properties above. See `docs/shielder-relation-spec.md` §6 and
`docs/shielder-audit-answers.md` Q5/Q6 for the open verification items (no committed
R1CS↔vk round-trip; `semi_trustless_at_least_one_honest()` is a self-consistency check, not
ceremony evidence).

---

## 5. Explicit non-goals

The following are **not** provided by the shielder as implemented. They are limitations to
state to users and to the auditor, each grounded in code:

1. **Amount privacy across denominations.** The denomination is a required public input at
   redeem, revealed for the root lookup (`shielder_flow.go:1234`; `public.go:32-34`); the
   payout amount is `denomination − fee`. Amount is public at withdrawal. Splitting a
   deposit into standard denominations (`DEFAULT_DENOMINATIONS_SATS`, `lib.rs:20-21`)
   partitions — it does not hide — the amount.

2. **Network-level anonymity for the user.** There is **no in-code Tor / mixnet / IP
   anonymization** on the browser API endpoint. The `/withdraw` handler
   (`ui/ui.go:181-190`) receives the proof over a plain HTTP POST and broadcasts it
   (`ui.go:107`); the serving node sees the submitter's IP. Any network-level unlinkability
   is a deployment concern the operator and user must handle out of band.

3. **Protection against recipient-address reuse.** The recipient string is bound into the
   proof and paid out verbatim (`hash.rs:33-42`; `withdrawal_flow.go:32`) with no
   canonicalization (`public.go:51`) and no reuse detection. A user who reuses the same BTC
   address across redeems links those withdrawals to each other on the BTC chain, defeating
   the pool for that user.

4. **No minimum anonymity-set enforcement.** There is a per-note minimum *value*
   (`Shielder_NoteAmountMinSats`, `shielder_flow.go:990-1023`) but **no minimum
   anonymity-set size** guarding a redeem. A **set-of-1 redeem** — the first/only leaf of a
   denomination, redeemed immediately — is currently allowed with no on-chain guard and no
   in-code user warning (audit answer Q34). Such a redeem is fully deanonymized: the single
   leaf is the only possible source.

5. **Seed compromise = total loss, no forward secrecy.** All note nullifiers/secrets are
   derived deterministically from a single `client_seed` via a hardened BIP32 path
   (`m/44'/60'/type'/deposit/note`) reduced with SHA-256 to 248-bit fields
   (`lib.rs:206-224,823-835,853-858`). A leaked seed reconstructs **every** past and future
   note (`recover_note_receipt_*`, `lib.rs:531-598`), breaking unlinkability for all of the
   user's notes and enabling theft of unspent ones. There is no forward secrecy: the seed
   must be protected like a wallet master key (audit answer Q38).

---

## 6. Summary for the auditor

- **Only the redeem is unlinkable**, and only within the same-denomination anonymity set
  present at proof time. The whitepaper's "completely fungible" claim overstates this and
  should be corrected (Section 1).
- **Everything else is public**: the entire shield side, plus each redeem's
  `nullifierHash`, `denominationSats`, `feeSats`, BTC `recipient`, and `inHash`.
- **The redeem is signer-less/fee-less/gasless** by design, so the deanonymization risk is
  **network-level** (relayer/broadcaster IP), not the Cosmos tx signer.
- **Value integrity** ultimately rests on the reused v2.1 trusted setup (≥1 honest MPC
  participant) and on the off-circuit Go value/denomination layer, not on the SNARK alone.
- **The stated non-goals** (amount privacy, network anonymity, address-reuse protection,
  minimum anonymity set, seed forward-secrecy) are real gaps to communicate to users and to
  assess as design decisions.
