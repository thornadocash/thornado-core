# Tornado Cash production circuits (vendored)

Vendored **live production** withdraw SNARK from [tornadocash/tornado-core](https://github.com/tornadocash/tornado-core) release **v2.1**.

See [VENDOR.md](./VENDOR.md) for exact commit pins and provenance.

## Quick start

```bash
cd circuits/tornado
npm run download-artifacts   # production proving key + R1CS bundle (~70MB)
```

Checked in already:

- `circom/withdraw.circom` + `merkleTree.circom` (production source)
- `vendor/circomlib/` @ tornadocash pin
- `artifacts/Verifier.sol` + `artifacts/withdraw_verification_key.json` (production verifier)

## Relation vs thornado-shielder

| | Production Tornado (this dir) | thornado-shielder (`tornado-cash`) |
|--|-------------------------------|-------------------------------------|
| Proof system | Groth16 / bn254 | Groth16 / bn254 (embedded v2.1 vk) |
| Commitment | Pedersen(248+248 bits) | Pure Rust Pedersen (circomlib-compatible) |
| Merkle | MiMC sponge, depth 20 | Pure Rust MiMC sponge |
| Public inputs | 6 (`root`, `nullifierHash`, `recipient`, `relayer`, `fee`, `refund`) | same |
| Ceremony | Tornado v2.1 MPC | pinned `withdraw_verification_key.json` + attestation tests |

`thornado-shielder` verifies withdraw proofs with **`ark-groth16`** against the production v2.1 verification key. Pedersen/MiMC/Merkle are implemented in pure Rust and checked against circomlibjs-generated vectors in `testdata/tornado_vectors.json`.

## Semi-trustless ceremony (≥1 honest)

Release **v2.1** Groth16 parameters come from the Tornado Cash MPC after Perpetual Powers of Tau. Security holds if **at least one** ceremony participant destroyed their toxic waste. The Rust engine pins the production vk (`engine::attestation()`, `tornado::ceremony::semi_trustless_at_least_one_honest()`).

## Audit

- [x] Source matches tornado-core `circuits/*.circom`
- [x] circomlib pin matches tornado-core `package.json`
- [x] Production vk + Verifier.sol from release v2.1
- [x] ark-groth16 verify wired to thornado-ffi / Go ante handlers
- [x] Public input hardening: recipient binding, zero relayer/refund, no note_commitment leak
- [x] Proving key + vk SHA256 pinned in `artifacts/MANIFEST.json` (`npm run verify:production-artifacts`)
- [x] Differential audit script (`npm run audit:differential`) — vk pin, native crypto vectors, optional groth16 roundtrip when `withdraw.wasm` is present
