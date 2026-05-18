# Go THORNode Fork Inventory

Agent: Agent 1, Go THORNode fork workstream.

Scope: Phase 0 inventory only. No THORNode modules were removed in this pass.
`FORK-PLAN.md` was read from the main checkout and was not modified.

## Fork Baseline

- Target fork area: `go-thornode/`
- Upstream inspected: `https://gitlab.com/thorchain/thornode.git`
- Upstream branch inspected: `develop`
- Upstream commit inspected: `50592ec4a0f3ab9e0e1a9a968e59e44a7ea9f571`
- Current state of this directory: upstream source copied into `go-thornode/`
  with this inventory added on top.

The public GitHub `thorchain/thornode` repository is only a stub that points to
GitLab, so the inventory below is based on the GitLab source tree.

## Top-Level App Modules

### Keep

- `app/`, `cmd/thornode/`, `app/params/`: keep Cosmos SDK app wiring, CLI/node
  process model, address prefixes, encoding, ante handling, genesis, and
  CometBFT integration.
- Cosmos SDK `auth`, `authz`, `bank`, `staking`, `genutil`, `params`,
  `consensus`, `upgrade`: keep. These are the app spine for accounts, balances,
  validator set operations, genesis, runtime params, consensus params, and
  upgrade handlers.
- Cosmos SDK `mint`: keep for now. THORChain wires it into app basics and module
  account permissions. Later it may be simplified if Thornado token economics do
  not need SDK minting.
- `x/thorchain`: keep as the primary consensus/state machine module, then strip
  feature families inside it only after dependency graph isolation.
- `x/thorchain/keeper`: keep. It contains the canonical state accessors for node
  accounts, bonds, slash points, jail, vaults, keygen/keysign metrics, observed
  tx voters, TxOut, Mimir, and upgrades.
- `x/thorchain/ebifrost`: keep for now. It is wired into proposal preparation and
  processing in `app/app.go`; removal would affect the block proposal path.
- `x/scheduler`: keep for now. It is registered in the app and runs an end
  blocker. It may be useful for delayed operational work, but its current Wasm
  dependency needs review.
- `common/`, `common/cosmos`, `constants`, `config`, `log`, `api/thorchain`,
  `openapi`: keep as shared app types, constants, configuration, logging, query,
  and API surface.
- `app/upgrades`, `x/thorchain/migrations`: keep. Upgrade/migration logic is a
  production network primitive.

The copied upstream `bifrost/` daemon and `cmd/bifrost/` entrypoint are present
in this fork because this repository will use `cmd/bifrost`. It remains upstream
multi-chain Bifrost and still needs a Bitcoin-only pruning pass.

### Remove

These are removal candidates for the Bitcoin privacy custody fork. Do not delete
them until callers are mapped and build gates exist.

- `x/thorchain/aggregators`: DEX aggregator routing.
- Swap handlers, memos, queues, and types: `handler_swap.go`,
  `handler_modify_limit_swap.go`, `swap_current.go`, `manager_swap_queue*`,
  `manager_adv_swap_queue*`, `memo_swap.go`, `memo_modify_limit_swap.go`,
  `type_streaming_swap.go`, `type_adv_swap_queue.go`, and related query code.
- Liquidity pool and LP accounting: `handler_add_liquidity.go`,
  `handler_withdraw.go`, `keeper_liquidity_provider.go`,
  `keeper_liquidity_fees.go`, `keeper_pool.go`, `manager_pool_current.go`,
  `type_pool.go`, `type_liquidity_provider.go`.
- Savers/lending/synthetic/economic swap support where present in current or
  migration code. In the inspected tree this surface appears mainly as pool,
  secured asset, trade account, rune pool, POL reserve, TCY, switch, affiliate,
  anchor, and DEX routing code.
- Affiliate and reference-memo features: `helpers_affiliates.go`,
  `handler_reference_memo.go`, `keeper_reference_memo.go`, affiliate collector
  module account usage.
- Trade accounts: `handler_trade_account_*`, `manager_trade_account_current.go`,
  `keeper_trade_accounts.go`, `type_trade_account.go`.
- Secured assets: `handler_secured_asset_*`, `manager_secured_asset_current.go`,
  `keeper_secured_asset.go`, `type_secured_asset.go`.
- Rune pool and TCY: `handler_rune_pool_*`, `keeper_rune_pool.go`,
  `handler_tcy_*`, `keeper_tcy_*`, related module accounts.
- EVM-specific and Wasm app extension surface: `chain/evm`, `x/thorchain` Wasm
  handlers, Wasm manager, and Wasm module wiring are remove candidates unless a
  specific Thornado governance or extension requirement is accepted.
- Non-Bitcoin chain-specific constants, token lists, chain contracts, and
  aggregator configuration.

### Unknown

- `x/denom`: unknown. It is wired into the app and can mint/burn restricted
  denoms. Keep until the Thornado asset model is decided.
- `x/scheduler`: unknown/keep for now. It depends on Wasm permission keeping,
  but the delayed-task pattern may be useful for operational or migration flows.
- `x/thorchain/ebifrost`: unknown/keep for now. It changes proposal handling and
  is too central to remove without a dedicated proposal-path review.
- `WasmKeeper` and custom Wasm module: unknown. Likely removable for a minimal
  custody chain, but scheduler and Wasm handlers currently depend on it.
- `mint`: unknown/keep for now. Needs tokenomics decision before removal.
- `manager_volume_current.go`, oracle and price feed quorum handlers: unknown.
  Likely removable if they only support swap economics, but fee and solvency
  code may still expect price inputs.
- Reserve, POL reserve, ragnarok, migration, consolidate, and switch flows:
  unknown. Some are swap/economic cleanup paths, but pieces overlap with vault
  migration, fund movement, and network safety.

## THORChain Subsystems

### Keep

- Node lifecycle: `handler_set_node_keys.go`, `handler_bond.go`,
  `handler_unbond.go`, `handler_rebond.go`, `handler_leave.go`,
  `handler_ip_address.go`, `handler_version.go`, `handler_maint.go`,
  `handler_ban.go`, `handler_operator_rotate.go`, `manager_validator_current.go`,
  `keeper_node_account.go`, `type_node_account.go`, `type_jail.go`.
- Bonding: node account `Bond`, bond providers, `BondName` module account, bond
  handlers, and bond invariants.
- Slashing and jail: `manager_slasher_current.go`, node slash point keeper
  methods, jail keeper methods, signing-failure and vault-slashing paths.
- Churn and vaults: `manager_validator_current.go`, `manager_network_current.go`,
  `keeper_vault.go`, `type_vault.go`, `type_vault_v2.go`.
- Keygen/keysign orchestration state: `handler_tss.go`,
  `handler_tss_keysign.go`, `keeper_keygen.go`, `keeper_tss.go`,
  `keeper_tss_keysign_fail.go`, `type_keygen.go`, `type_tss*.go`,
  `type_blame.go`.
- Observed tx voters: `handler_observed_txin.go`, `handler_observed_txout.go`,
  `handler_observed_tx_quorum.go`, `handler_observed_tx_helpers.go`,
  `keeper_txin.go`, `type_observed_tx.go`.
- Outbound queues and completion tracking: `manager_txout_current.go`,
  `handler_outbound_tx.go`, `handler_common_outbound.go`, `keeper_txout.go`,
  `type_tx_out.go`, `msg_tx_outbound.go`.
- Runtime params and halts: `handler_mimir.go`, `keeper_mimir_*`,
  `keeper_config.go`, `keeper_halt.go`, `type_mimir.go`.
- Network fees and solvency: `handler_network_fee*`, `handler_solvency*`,
  `keeper_network_fee.go`, `keeper_observed_network_fee_voter.go`,
  `keeper_solvency_voter.go`. Keep for Bitcoin operation, then strip
  non-Bitcoin cases.
- Upgrades and migrations: `handler_upgrade.go`, `keeper_upgrade.go`,
  `router_upgrade_*`, `migrations*`, `app/upgrades`.
- Genesis import/export: `genesis.go`, `genesis_*.go`, `types/genesis.go`,
  generated genesis proto files, app module order.

### Remove

- Swap, pool, LP, DEX, affiliate, savers/lending/synthetic, trade-account,
  secured-asset, rune-pool, TCY, and non-Bitcoin chain feature families.
- User-facing memos that only create the removed features.
- Queries that only quote swaps, liquidity, trade accounts, or removed economics.
- Internal routing has been narrowed so removed feature messages are no longer
  accepted by the consensus handler map.

### Unknown

- `handler_deposit.go` and `processOneTxIn`: keep the observed-deposit entry
  point, but the memo-to-message switch must be rewritten so Bitcoin deposits
  become privacy deposit inputs instead of swap/LP messages.
- `handler_refund.go`: likely keep a simplified refund/error path for invalid
  deposits or failed withdrawals, but current behavior is tied to swap flows.
- `handler_migrate.go`, `manager_scheduled_migration*`, `ragnarok`, and
  `consolidate`: may be needed for vault migration and retirement, but current
  semantics include old THORChain economic cleanup.
- Gas manager: keep for outbound fee accounting, but remove multi-chain/swap
  fee branches after Bitcoin-only dependencies are clear.
- Oracle/price feed: unknown until Bitcoin fee, solvency, and denomination
  policies are finalized.

## Required Dependency Map

### Node Lifecycle, Bonding, Slashing, Jail

- App dependencies: `auth`, `bank`, `staking`, `params`, `upgrade`, `x/thorchain`.
- Keeper state: node accounts, bond providers, slash points, jail records,
  network state, Mimir/halts.
- Manager dependencies: `ValidatorManager`, `NetworkManager`, `Slasher`,
  `EventManager`, `Keeper`.
- Important call paths:
  - `MsgBond`, `MsgUnBond`, `MsgReBond`, `MsgLeave`, `MsgSetNodeKeys`,
    `MsgSetIPAddress`, `MsgVersion`, `MsgMaint`, `MsgBan`, operator rotation.
  - Validator begin/end blockers update membership and call network/keygen paths.
  - Slasher increments/decrements slash points and can jail or slash vault
    members for signing/vault failures.

### Churn, Vaults, Keygen, Keysign

- Keeper state: vaults, vault statuses, pubkeys, membership, keygen blocks,
  TSS keygen/keysign metrics, TSS keysign failure voters, blame.
- Manager dependencies:
  - `ValidatorManager` selects active/standby membership and triggers churn.
  - `NetworkManager.TriggerKeygen` writes keygen work for active node sets.
  - `NetworkManager.RotateVault` transitions vault statuses after keygen.
  - `TxOutStore` and `Slasher` interact with retiring vaults and missed signing.
- Current code assumes TSS names. For Thornado, keep state semantics but plan a
  naming/adaptation layer for FROST vault sessions and FROST group pubkeys.

### Observed Tx Voters

- Keeper state: inbound and outbound `ObservedTxVoter`, observed links, network
  fee voters, solvency voters, errata voters.
- Handler dependencies:
  - `MsgObservedTxIn` reaches quorum, parses memo, creates internal messages,
    updates voters, and may slash bad observations.
  - `MsgObservedTxOut` confirms outbound completion and links to TxOut.
  - Quorum helpers depend on node account membership and active observers.
- Thornado change needed: accepted Bitcoin deposits should feed `x/privacy`
  deposit handling, not swap/LP paths.

### Outbound Queues

- Keeper state: TxOut blocks/items, outbound voters, vault state, network fees.
- Manager dependencies:
  - `TxOutStore.TryAddTxOutItem` chooses vaults, calculates outbound height and
    fees, and stores TxOut items.
  - `Slasher.LackSigning` can reschedule unobserved outbounds.
  - `GasManager` and network fee keeper feed fee calculation.
- Thornado change needed: privacy withdrawal acceptance should enqueue Bitcoin
  outbound intents; Bifrost owns Bitcoin construction/sign/broadcast.

### Mimir, Params, Halts

- Keeper state: Mimir key/value store, config getters, halt flags, network fees,
  consensus params.
- Dependencies: handlers, managers, keepers, and tests read Mimir directly for
  feature flags, chain halts, fee policy, migration safety, and limits.
- Keep a runtime params module. Later rename/curate keys to Thornado-specific
  params and remove swap/LP/DEX-only keys.

### Upgrades

- App dependencies: Cosmos SDK `upgrade`, `app/upgrades`, `x/thorchain`
  upgrade handlers and migration helpers.
- Keeper dependencies: node accounts, vaults, network, Mimir, pool/economic
  state in current upstream.
- Keep the upgrade mechanism, but every historical migration touching removed
  economics must be reviewed before old mainnet state import is supported.

## First Refactor Order

1. Copy/fork upstream source into `go-thornode/` and record the same upstream
   commit in a dedicated fork note.
2. Make the unmodified fork build.
3. Add compile-time or config-time guards around obvious removed handlers before
   deleting files.
4. Keep vault/churn/keygen/keysign, observed voters, TxOut, node lifecycle,
   Mimir, and upgrades green while removing swap/LP/economic feature families.
5. Add `x/privacy` only after the stripped app spine still builds.

## Shared Interface Notes

- Need a stable `proto/privacy/v1` withdrawal verification contract before
  production `x/privacy` validation is finalized.
- Need a stable FROST signer public key/session model before replacing current
  TSS keygen/keysign semantics.
- These needs are noted here only; this workstream did not edit `proto/`,
  `rust-frost-signer/`, `rust-privacy/`, or `go-bifrost/`.
