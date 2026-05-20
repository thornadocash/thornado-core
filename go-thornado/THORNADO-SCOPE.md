# Thornado THORNode Scope

This fork is scoped to the Thornado Bitcoin custody state machine described in
`THORNADO.pdf`.

## Kept Runtime Surface

- Cosmos/CometBFT node process and app wiring.
- Node accounts, permanent bonded slots, bond accounting, jail, slash points,
  active/standby membership, and churn.
- Vault state, keygen/keysign orchestration state, observed transaction voters,
  outbound queues, network fees, solvency reporting, Mimir/runtime params, and
  software upgrades.
- Shutdown and maintenance primitives remain modeled through existing node
  governance and halt/maintenance paths until a dedicated shutdown module is
  added.

## Removed Or Disabled Runtime Surface

- The Bifrost daemon entrypoint remains available at `cmd/bifrost` and the
  upstream Bifrost implementation remains under `bifrost/`. It still needs a
  Bitcoin-only pruning pass before production use.
- Internal THORChain routing no longer accepts swap, liquidity, DEX, affiliate,
  THORName, trade-account, secured-asset, RunePool, TCY, switch, reserve, donate,
  ragnarok, or Wasm execution messages.
- Observed Bitcoin deposits are restricted to operational custody memos for now.
  Privacy-note conversion belongs in the future `x/privacy` module.
- Upstream documentation, simulation harnesses, Docker assets, and operator tools
  tied to multi-chain swap operations have been removed from this fork area.

## Bifrost Follow-Up

The restored Bifrost tree is still the upstream multi-chain implementation. The
next pass should prune non-Bitcoin chain clients while preserving scanner,
parser, UTXO, fee, broadcast, retry, solvency, and observation paths.
