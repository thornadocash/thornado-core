# Flow 1: 4-Node Genesis And FROST Base Vault

Goal: prove a fresh 4-validator genesis automatically produces the first FROST BTC base vault without CLI keygen and with zero Thornado bond on genesis nodes.

## Results

- result: PASS; command: `FLOW_LIMIT=1 ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-happy-final/meta/base-vaults.json`, `/tmp/thornado-flow1-happy-final/logs/bifrost-*.log`, `/tmp/thornado-flow1-happy-final/logs/thornado-*.log`; validated: true
- result: final base vault: `tthorpub1addwnpepqf6sqde37dzw8lej3pqv6atgvkggnnhzlxwku78cdarmlehl8jrncsceg68`; one `ActiveVault`, type `BaseVault`, chain `BTC`, four genesis members, no coins; validated: true
- result: local E2E config now uses `Vault_BaseMembersMin=4`, `Node_BondStartAmountSats=0`, `Node_BondSlotIncrementSats=100000000`, and `Churn_RetryIntervalBlocks=720`; no `EnableFrostBTC`, no `AsgardSize`, no `BaseVaultMembersMinimum`; validated: true
- result: negative TSS probes passed; evidence: `/tmp/thornado-flow1-negative-final/meta/negative-tss-current/results.json`; validated: true
- result: persisted Bifrost restart passed; node1 restarted with scanner start height `0`, reloaded state, stayed healthy, and did not re-run FROST keygen; evidence: `/tmp/thornado-flow1-persist-final/meta/persist-restart/result.json`, `/tmp/thornado-flow1-persist-final/logs/bifrost-1-restart-current.log`; validated: true
- result: three-active genesis fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=three_active ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-three-active/meta/three_active-base-vaults.json`; validated: true
- result: missing secp genesis fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=missing_secp ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-missing-secp/meta/genesis-validate.log`; validated: true
- result: duplicate secp genesis fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=duplicate_secp ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-duplicate-secp/meta/genesis-validate.log`; validated: true
- result: offline Bifrost fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=offline_node4 ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-offline-node4/meta/offline-bifrost-base-vault-before.json`, `/tmp/thornado-flow1-offline-node4/meta/offline-bifrost-base-vault-after.json`; validated: true
- result: forged vault fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=forged_vault_state ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-forged-vault/meta/genesis-validate.log`; validated: true
- result: mid-keygen restart fixture passed; command: `FLOW_LIMIT=1 FLOW1_SCENARIO=mid_keygen_restart ops/scripts/real-4node-e2e.sh`; evidence: `/tmp/thornado-flow1-mid-keygen-restart/meta/base-vaults.json`, `/tmp/thornado-flow1-mid-keygen-restart/logs/thornado-1-restart.log`; validated: true

## Setup And Genesis State

- check: genesis has exactly four CometBFT validators; desired_result: validator set contains node1-node4 only, equal voting power, no node5/node6 consensus entry; validated: true
- check: Thornado genesis has exactly four node accounts; desired_result: node1-node4 are `Active`, each has secp256k1 node key, ed25519 node key, consensus pubkey, version, and zero Thornado bond; validated: true
- check: genesis node accounts have `bond:"0"` and bond address equal to node operator address; desired_result: genesis does not depend on shielder bond deposits; validated: true
- check: node key roles are distinct; desired_result: secp256k1 key is vault/signing membership identity, ed25519 key is node key-set identity, consensus key is CometBFT validator identity; validated: true
- check: genesis has no initial active vault; desired_result: app genesis `vaults` is empty and the first base vault is created only after automatic keygen; validated: true
- check: FROST is the only BTC keygen/keysign path; desired_result: no `EnableFrostBTC` config exists and no legacy feature flag is required; validated: true
- check: genesis config uses `Node_SetDesired=4` and `Vault_BaseMembersMin=4`; desired_result: initial keygen membership resolves to exactly four genesis node secp keys; validated: true
- check: genesis config has `Node_BondStartAmountSats=0` and `Node_BondSlotIncrementSats=100000000`; desired_result: active genesis set is allowed at zero Thornado bond, and first non-genesis bond requires `1.0 BTC`; validated: true
- check: genesis `last_chain_heights` starts BTC after regtest funding blocks; desired_result: Bifrost scanners do not rescan pre-run funding as deposits; validated: true

Section results: all setup/genesis checks passed in the final run.

## Happy Path

- check: first committed blocks do not contain operator CLI keygen transactions; desired_result: keygen is produced by state-machine keygen/churn logic and consumed automatically by Bifrost; validated: true
- check: keygen block appears at height `1`; desired_result: `/thornado/keygen/1/{node_pubkey}` returns one `BaseVaultKeygen`; validated: true
- check: keygen membership equals node1-node4 secp pubkeys; desired_result: no standby, future, duplicate, or missing key appears; validated: true
- check: every genesis Bifrost receives the same keygen block; desired_result: signer logs show matching height, type, membership, and `chains:["BTC"]`; validated: true
- check: every genesis Bifrost completes FROST keygen; desired_result: logs show `FROST keygen complete`, `members:4`, `chains:["BTC"]`; validated: true
- check: every genesis Bifrost submits `MsgTssPool`; desired_result: Thornado receives enough valid messages for consensus; validated: true
- check: Thornado accepts keygen quorum; desired_result: one active base vault is created and no blame path is taken; validated: true
- check: base vault pubkey converts to BTC taproot address; desired_result: `bitcoin-cli validateaddress` reports valid witness v1 regtest address; validated: true
- check: no funds exist in base vault yet; desired_result: vault `coins` is null/zero and regtest `scantxoutset` for the derived address returns zero UTXOs; validated: true

Section results: all happy-path checks passed.

## API, KV, And External Persistence

- check: four RPC nodes converge; desired_result: node1-node4 RPC status reports `catching_up=false` and same app hash at the same height; validated: true
- check: keygen query by height and pubkey is stable across all Thornado APIs; desired_result: node1-node4 APIs return identical `.keygen_block`; validated: true
- check: vault query is stable across all Thornado APIs; desired_result: all APIs return the same active base vault pubkey, membership, status, and chains; validated: true
- check: KV keygen block state exists at key `keygen//1`; desired_result: direct ABCI state inspection returns a value; validated: true
- check: KV vault state exists at key `vault//{VAULT}`; desired_result: direct ABCI state inspection returns a value matching the API vault; validated: true
- check: KV base index exists at `vault_base_index//`; desired_result: decoded index includes the active base vault pubkey; validated: true
- check: Bifrost local FROST share storage exists for each node; desired_result: each `localstate-{vault}.json` has `signing_engine:"frost"`, `participant_count:4`, local data, and `local_party_key` equal to that node's secp key; validated: true
- check: Bifrost restart uses persisted scanner/share state; desired_result: restarted node1 with scanner start height `0` does not replay height-1 FROST keygen and still reports the active vault in `/status/signing`; validated: true
- check: bitcoind has no transaction involving base vault before Flow 2; desired_result: derived address is valid and `scantxoutset` has no unspents; validated: true

Section results: all API, KV, local persistence, and external BTC checks passed.

## Bad Paths

- check: malformed pool pubkey submitted through `tss-pool`; desired_result: CLI rejects before broadcast; validated: true
- check: non-member node5 submits `MsgTssPool` for the genesis keygen members; desired_result: signer is rejected and no vault mutation occurs; validated: true
- check: active signer submits a member set that swaps in node5; desired_result: handler rejects as unauthorized and no extra vault is created; validated: true
- check: active signer submits for future/nonexistent keygen height `999`; desired_result: handler rejects as unauthorized and no extra vault is created; validated: true
- check: exact FROST replay for existing voter; desired_result: duplicate/no-op does not create a second vault; validated: true
- check: one signer submits alternate pool pubkey for the same keygen membership; desired_result: single vote is not enough to create another vault; validated: true
- check: start with only three active genesis node accounts; desired_result: keygen is not accepted because minimum membership is not met, and `/thornado/vaults/base` remains empty; validated: true
- check: one genesis node has missing or duplicate secp pubkey; desired_result: genesis/keygen validation rejects before vault creation; validated: true
- check: one Bifrost is offline during initial keygen; desired_result: no vault is created before all signer shares exist; after the late signer scans from height 1, exactly one 4-member active vault is created and every Bifrost persists matching FROST state; validated: true

Section results: live malformed/unauthorized/replay/alternate-pubkey bad paths passed. Destructive genesis-fixture variants passed.

## Attack Paths

- check: malicious signer replays old exact `MsgTssPool`; desired_result: duplicate message does not create a second vault; validated: true
- check: malicious signer submits alternate FROST pubkey with only one vote; desired_result: message cannot reach consensus and active vault count remains one; validated: true
- check: malicious signer uses valid member list but wrong signer account; desired_result: auth rejects because signer does not map to a keygen member node account; validated: true
- check: malicious signer uses valid active signer but wrong member set; desired_result: auth rejects because keygen block membership does not match; validated: true
- check: forged BTC vault pubkey address is injected into vault state; desired_result: genesis validation rejects malformed vault pubkey before any signer starts; validated: true
- check: app restart during keygen; desired_result: keygen block, voter, and Bifrost retry state recover without app hash divergence; validated: true

Section results: live message-level attack paths passed. State-corruption and mid-keygen crash fixtures passed.
