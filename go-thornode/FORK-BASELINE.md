# Go THORNode Fork Baseline

This directory is the Agent 1 fork area for the Go THORNode consensus
workstream.

## Upstream

- Source: `https://gitlab.com/thorchain/thornode.git`
- Branch copied: `develop`
- Commit copied: `50592ec4a0f3ab9e0e1a9a968e59e44a7ea9f571`

The public GitHub `thorchain/thornode` repository is only a pointer to GitLab,
so this baseline uses the GitLab repository.

## Scope

Agent 1 owns the consensus/state-machine fork workstream:

- Cosmos SDK app wiring
- `x/thorchain` state-machine inventory and stripping plan
- node lifecycle, bonding, slashing, jail
- churn, vault, keygen/keysign orchestration state
- observed transaction voters
- outbound queues
- runtime params and upgrades

The copied upstream repository contains historical Bifrost paths. They are kept
unchanged in this baseline only because they ship in the upstream source tree.
The separate `go-bifrost/` workstream owns the Bitcoin-only Bifrost fork.
