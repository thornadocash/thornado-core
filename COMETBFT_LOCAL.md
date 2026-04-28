# Thornado CometBFT Local Mode

This repo now has a real ABCI boundary for CometBFT:

- `thornado-abci` runs the deterministic application.
- `thornado-node --cometbft-rpc <url>` submits user/operator commands to CometBFT RPC instead of mutating local state.
- FROST key generation and Bitcoin side effects stay outside consensus.

## Build

```bash
cargo build
```

## One-Node ABCI App

Run the ABCI app:

```bash
cargo run -p thornado-abci -- --listen 127.0.0.1:26658 --genesis-state genesis.json
```

Configure CometBFT with:

```toml
proxy_app = "tcp://127.0.0.1:26658"
```

Then run CometBFT normally from its node home:

```bash
cometbft start --home .cometbft-node-0
```

## HTTP Node In Consensus Mode

Point the Thornado HTTP node at the CometBFT RPC endpoint:

```bash
cargo run -p thornado-node -- \
  --listen 127.0.0.1:3030 \
  --cometbft-rpc http://127.0.0.1:26657
```

Mutating HTTP endpoints encode a `ThornadoTx` and call CometBFT `broadcast_tx_commit`.
The local node state is not mutated directly in this mode.

## Churn And Keyset Flow

Consensus-safe churn is split into two steps:

1. Submit `/churn/start`.
   This advances the epoch and promotes standby nodes.
2. Run FROST DKG off-consensus for the new active set.
   Submit the public result through `/custody/keyset/commit`.

The ABCI app validates that the committed keyset epoch, signer count, and threshold match the finalized active set.

## Current Limits

- The repo tests the ABCI application and CometBFT RPC submission boundary, but does not vendor or launch a CometBFT binary.
- CometBFT validator-set updates are not emitted yet.
- FROST networking is still separate from consensus; only public keyset commits are consensus transactions.
- Bitcoin UTXOs remain node-local sidecar state. Consensus stores withdrawal authorization, not wallet state.
- Real ZK withdrawal verification remains behind the verifier trait boundary.
