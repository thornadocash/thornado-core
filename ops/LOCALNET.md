# Thornado Localnet

Current scope is deliberately small: prove the local ops shell around Bitcoin
regtest plus Thornado service boundaries. The real state machine and Bifrost
code live in `go-thornado`; Shielder is linked through
`go-thornado/go-wrappers/shielder`, not run as a sidecar.

## Current Services

```text
bitcoind-regtest
thornode-1
bifrost-1
client
```

The `three-node` profile adds `thornode-2`, `thornode-3`, `bifrost-2`, and
`bifrost-3` as mock service boundaries.

## Ports

| Service | Internal | Host | Purpose |
| --- | ---: | ---: | --- |
| bitcoind-regtest | 18443 | 18443 | Bitcoin RPC |
| bitcoind-regtest | 18444 | 18444 | Bitcoin P2P |
| thornode-1 | 26656 | 26656 | CometBFT P2P |
| thornode-1 | 26657 | 26657 | CometBFT RPC |
| thornode-1 | 1317 | 1317 | REST/API |
| thornode-1 | 9090 | 9090 | gRPC |
| bifrost-1 | 6040 | 6040 | Health/debug |

## Smoke Gates

1. Compose stack starts.
2. `bitcoind-regtest` is reachable.
3. mock Thornode boundary is reachable.
4. mock Bifrost boundary is reachable.
5. regtest bootstrap can create a wallet and mine blocks.
6. logs are collected on failure.

## Real Integration Gates

These are the next gates once Docker packaging exists for `go-thornado`:

1. Thornado boots from `cmd/thornado`.
2. Bifrost boots from `cmd/bifrost`.
3. Bifrost observes a regtest Bitcoin deposit.
4. Thornado accepts the observation and queues outbound intent.
5. Shielder withdrawal verification runs through the Go wrapper.
6. FROST signing is added later through the Bifrost signer seam.

## Scripts

```text
ops/scripts/localnet-up.sh
ops/scripts/localnet-down.sh
ops/scripts/localnet-reset.sh
ops/scripts/wait-for-health.sh
ops/scripts/bootstrap-regtest.sh
ops/scripts/bootstrap-thornode.sh
ops/scripts/send-deposit.sh
ops/scripts/run-withdrawal.sh
ops/scripts/smoke-private-flow.sh
ops/scripts/collect-logs.sh
```

Scripts read `ops/env.localnet` when present and otherwise use
`ops/env.localnet.example`.
