# Tornado Cash circuit vendoring

Production withdraw SNARK from [tornadocash/tornado-core](https://github.com/tornadocash/tornado-core), pinned for byte-compatible Groth16 proofs.

## Pinned sources

| Component | Upstream | Pin |
|-----------|----------|-----|
| `circom/withdraw.circom` | `tornado-core/circuits/withdraw.circom` | `54c38380a180471f43e40c92f825585c00784c52` |
| `circom/merkleTree.circom` | `tornado-core/circuits/merkleTree.circom` | `3a669758c1963c590888d2792e7d393197737ce0` |
| `vendor/circomlib/` | [tornadocash/circomlib](https://github.com/tornadocash/circomlib) | `c372f14d324d57339c88451834bf2824e73bbdbc` |
| Groth16 ceremony | tornado-core release | **`v2.1`** |

## Production relation

- **Hashing:** Pedersen commitment over 248-bit nullifier + 248-bit secret; Pedersen nullifier hash
- **Merkle tree:** depth **20**, MiMC sponge pair hash (same as live ETH/BTC pools)
- **Public inputs (6):** `root`, `nullifierHash`, `recipient`, `relayer`, `fee`, `refund`
- **Private inputs:** `nullifier`, `secret`, `pathElements[20]`, `pathIndices[20]`
- **Main:** `component main = Withdraw(20);`

On-chain `Verifier.sol` from release **v2.1** verifies these proofs on Ethereum mainnet deployments.

## Layout

```
circuits/tornado/
  circom/                 # vendored .circom (include paths rewired to vendor/circomlib)
  vendor/circomlib/     # tornadocash/circomlib @ c372f14…
  artifacts/              # production keys (small vk + Verifier committed; large keys via script)
  scripts/
    download-artifacts.mjs
  package.json
```

## Download production artifacts

Large proving keys are **not** committed. Fetch the official release bundle:

```bash
cd circuits/tornado
npm run download-artifacts
```

This pulls the same files as upstream `scripts/downloadKeys.js`:

- `withdraw.json` — compiled circuit
- `withdraw_proving_key.bin` / `.json` — MPC ceremony output (production prover)
- `withdraw_verification_key.json` — production verifier key
- `Verifier.sol` — deployed Solidity verifier
- `tornado.params` / `tornado_no_zeros.params` — websnark params

Small verifier files (`Verifier.sol`, `withdraw_verification_key.json`) are checked in under `artifacts/` for reference and CI.

## Recompile (optional)

Recompilation requires Circom 0.5.x + snarkjs. A local compile produces a **different** trusted setup unless you reuse the v2.1 `.zkey`. For mainnet-compatible proofs, use the downloaded production proving key.

```bash
cd circuits/tornado
npm install
npm run download-artifacts
# circom circom/withdraw.circom --r1cs --wasm -o build/ -l vendor/circomlib/circuits
```

## License

Tornado Cash circuits are MIT-licensed. See upstream repositories for full license text.
