# Go Thornado Fork Baseline

This directory is the Thornado Go fork. It owns the state machine and the
Bitcoin-only Bifrost daemon.

## Upstream

- Source: `https://gitlab.com/thorchain/thornode.git`
- Branch copied: `develop`
- Commit copied: `50592ec4a0f3ab9e0e1a9a968e59e44a7ea9f571`

The public GitHub `thorchain/thornode` repository is only a pointer to GitLab,
so this baseline uses the GitLab repository.

## Scope

`go-thornado` owns:

- Cosmos SDK app wiring
- `x/thorchain` state-machine inventory and stripping
- node lifecycle, bonding, slashing, jail
- churn, vault, keygen/keysign orchestration state
- observed transaction voters
- outbound queues
- runtime params and upgrades

The copied upstream repository contains historical Bifrost paths. Thornado keeps
only the Bitcoin-relevant Bifrost surface inside this same module.
