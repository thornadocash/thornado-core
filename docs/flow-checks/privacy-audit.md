# Privacy Audit — Shield/Unshield Churn Model

Target: transitive privacy via fixed-denom pool churn; transparent boundaries at bond escrow, fee accrual, and auction settlement.

**Engine:** Tornado Cash Groth16 withdraw v2.1 (`tornado-cash-groth16-v2.1`), verified in Rust via `ark-groth16` + pinned production vk. Ceremony assumption: ≥1 honest MPC participant (same as mainnet Tornado v2.1).

## Flow 1 — User

| Vector | Status |
|--------|--------|
| Commitment → deposit ID in KV | Pass — no mapping for user notes |
| Redeem proof persisted on chain | Pass — public inputs only; `note_commitment` rejected |
| Auto-shield on match | Implemented |
| Groth16 withdraw on hot path | Implemented — `thornado-shielder` / `tornado-cash` default |
| Recipient string ↔ SNARK binding | Pass — `recipient_field` must match `recipient_binding(recipient, fee, denom)` |
| Relayer / refund public fields | Pass — must be zero (no relayer model on thornado) |
| Private note material in proof JSON | Pass — `RejectLeakyShielderRedeemProof` rejects nullifier/secret/commitment |
| Merkle path in proof JSON | Pass — rejected if present under `tornado.merkle_path` |
| Public note commitment on redeem | Pass — rejected in public JSON and Rust `validate_public_inputs` |
| Denomination enforced off-circuit | Pass — separate Merkle trees + `ShielderMerkleRootExists(denom, root)` |

## Flow 2 — User → Operator (Bond)

| Vector | Status |
|--------|--------|
| Bond via transparent escrow | Implemented — `MsgBondFromNotes`, recipient `bond_escrow` |
| Legacy bond BTC deposit | Removed — rejected at PoW registration |
| Protocol-tagged pool leaves for bond | Removed — no `shielderBondCommitment` on hot path |
| Bond event linkable to note spend | Expected — bond amount/slot public; nullifier unlinkable if churn holds |
| Bond public input validation | Pass — `ValidateRedeemPublicJSON` in `ValidateBasic` |

## Flow 3 — Operator Fees

| Vector | Status |
|--------|--------|
| Fee claim → pool notes → redeem | Implemented — `MsgShielderShieldFees` then `MsgShielderRedeem` |
| Direct transparent fee payout | None |
| Uneconomical small fee notes | Mitigated — claim must net positive after min withdrawal fee |

## Flow 4 — Auction Bidder

| Vector | Status |
|--------|--------|
| Bid via shield → unshield → bid address | Implemented — `recipient_policy: bid_deposit` + `bid_id` |
| Transparent-only bid shortcut | Still possible via external BTC to bid address (same match path) |
| Bid recipient binding | Pass — must match registered bid deposit address |

## Flow 5 — Auction Seller

| Vector | Status |
|--------|--------|
| Seller payout as pool notes | Implemented — `MsgNodeSlotAuctionShield` |
| Buyer excess as protocol pool commitment | Removed — buyer bond from transparent bid + `transferNodeSlotSaleBond` |
| Seller unshield | Standard `MsgShielderRedeem` |

## Cryptographic stack (production)

| Layer | Trust / review focus |
|-------|----------------------|
| Groth16 v2.1 vk | Pinned `circuits/tornado/artifacts/withdraw_verification_key.json`; ceremony attestation tests |
| Withdraw relation | Production `withdraw.circom` + circomlib pin |
| Note derivation | `thornado-shielder-v1` SHA256 → bn254; must stay aligned with Pedersen Num2Bits(248) |
| Pedersen / MiMC / Merkle | Pure Rust in `thornado-shielder` (`pedersen.rs`, `mimc_sponge.rs`, `merkle.rs`); vector tests vs `tornado_vectors.json` |
| Verifier | `ark-groth16` snarkjs JSON parser; `npm run audit:differential` (vk pin + native crypto; optional groth16 roundtrip with wasm) |
| Client proving | Production `.zkey` (~70MB); download via `npm run download-artifacts` |

## Remaining operational items

- E2E regtest refresh for flows 2 and 5 under bond/bid semantics
- Full negative-path integration tests for `MsgBondFromNotes` and `bid_deposit` redeem with live Groth16 proofs (`proof-tests` feature + `withdraw.wasm`)

## Test commands

```bash
# Rust engine + ceremony + vector tests
cargo test -p thornado-shielder

# Groth16 roundtrip (requires Node + downloaded zkey + withdraw.wasm)
cd circuits/tornado && npm run download-artifacts
cargo test -p thornado-shielder --features proof-tests

# Artifact pin + vk + native crypto differential audit
cd circuits/tornado && npm ci && npm run download-artifacts && npm run audit:differential

# FFI + Go wrapper
cargo build --release -p thornado-ffi
cd go-thornado && go test ./go-wrappers/shielder/... ./x/thornado/ -run 'Shielder|RejectLeaky|ValidateShielder|BondFromNotes|Recipient'
```
