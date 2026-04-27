# Thornado Withdrawal Circuits

This directory contains the reference Tornado Cash Classic-style Circom
withdrawal circuit. The Rust implementation uses the transparent Winterfell
STARK backend in `crates/thornado-core/src/stark.rs` instead of Groth16.

The circuit proves:

- the prover knows `nullifier` and `secret`;
- `commitment = Pedersen(nullifier || secret)`;
- `nullifierHash = Pedersen(nullifier)`;
- `commitment` is included in a MiMC Merkle tree with public `root`;
- public withdrawal fields are bound into the proof.

Public signal order is:

1. `root`
2. `nullifierHash`
3. `recipient`
4. `relayer`
5. `fee`
6. `refund`

Compile the constraint system from the repository root:

```sh
npm install circomlib snarkjs
npx circom circuits/withdraw.circom --r1cs --wasm --sym -o build/circuits
```

Do not use `groth16 setup` or a circuit-specific `.zkey` for this project. The
production path is the Rust STARK circuit:

- backend: Winterfell STARK/FRI;
- trusted setup: none;
- note hash: Thornado algebraic field hash;
- Merkle tree: fixed-depth Thornado algebraic field tree.

The Circom files remain as a Tornado Cash reference artifact, but the checked
Rust verifier does not consume Circom R1CS or snarkjs output.

The circuit source is intentionally close to Tornado Cash Core's
[`withdraw.circom`](https://github.com/tornadocash/tornado-core/blob/master/circuits/withdraw.circom)
and
[`merkleTree.circom`](https://github.com/tornadocash/tornado-core/blob/master/circuits/merkleTree.circom),
adapted only for this repo's path layout.
