# Flow 6: Node6 Churn-In And Base Vault Migration

Goal: prove a bonded replacement node joins the signer set, a new FROST base vault is generated, old vault retires, and funds migrate.

## Results

- run: `/tmp/thornado-flow6-clean-v22`; result: PASS; evidence: Flow 6 completed with node6 churn-in, automatic FROST keygen, BTC migration tx confirmation, old vault direct query, and new active vault query.

## Happy Path

- check: node6 starts as a real Thornado full node; desired_result: RPC, API, gRPC, p2p, and block sync are healthy; validated: true
- check: node6 starts as a real Bifrost signer; desired_result: health endpoint is ready, BTC scanner starts, P2P peers connect; validated: true
- check: node-gater allowlist includes node6 after bond state; desired_result: existing Bifrost logs show refreshed allowlist count includes node6; validated: true
- check: node6 registers IP, version, and node keys before churn; desired_result: node query shows correct secp256k1 key, consensus key, IP, version, standby status, and sufficient bond; validated: true
- check: node6 status before churn is Standby and preflight-eligible; desired_result: node query/preflight has no missing IP, missing keys, outdated version, jail, or insufficient bond error; validated: true
- check: churn selects node6; desired_result: node query transitions Standby to Selected and then Active according to protocol timing; validated: true
- check: new keygen block is emitted for updated signer set; desired_result: keygen height > genesis, membership includes node6 and active incumbents, and excludes sold seller node key; validated: true
- check: all active/selected Bifrosts receive new keygen block; desired_result: logs show same keygen id and membership on all expected nodes; validated: true
- check: FROST keygen completes for new set; desired_result: logs show `FROST keygen complete`, member count expected, BTC chain; validated: true
- check: Thornado accepts new keygen; desired_result: handler logs keygen success and creates new active vault; validated: true
- check: old vault status changes to RetiringVault; desired_result: vault query shows old pubkey retiring with existing funds; validated: true
- check: new vault status is ActiveVault; desired_result: vault query shows new pubkey, status_since churn height, membership includes node6 key; validated: true
- check: migration txouts are queued from retiring vault to new active vault; desired_result: tx_type `migrate`, blank in_hash, retiring vault source, new vault destination; validated: true
- check: FROST signs migration with retiring vault members; desired_result: BTC transaction spends old vault UTXO and pays new active vault address; validated: true
- check: Bifrost observes migration outbound; desired_result: observed outbound marks txout or BTC UTXO state validates migration; validated: true

## State Checks

- check: node6 active_block_height/status_since are set correctly; desired_result: node account reflects churn height; validated: true
- check: removed/sold node is no longer in active signer membership; desired_result: old seller key absent from new ActiveVault membership; validated: true
- check: active vault count is exactly one after churn; desired_result: one ActiveVault, one RetiringVault until migration drains; validated: true
- check: vault coin accounting transfers from old to new; desired_result: total BTC across vaults is conserved minus gas; validated: true
- check: node6 signer membership array updates on its node account; desired_result: node6 account includes the new vault pubkey in signer_membership; validated: true
- check: no deposit address is issued against retiring vault after churn; desired_result: new deposit sessions use new active vault pubkey; validated: true

## API, KV, And External Persistence

- check: node/vault APIs show node6 selected/active and sold seller excluded from new vault membership; desired_result: node6 status matches validator manager state and seller pubkey is absent from new ActiveVault membership; validated: true
- check: `/thornado/vaults/base` and direct vault query expose rotation state; desired_result: aggregate base-vault query shows retiring plus active before drain, aggregate query shows new active after drain, and `/thornado/vault/{old}` shows old retiring or inactive; validated: true
- check: `/thornado/keygen/{height}/{node6_pubkey}` returns new keygen block; desired_result: query works for node6 and incumbent pubkeys; validated: true
- check: txout query shows migration; desired_result: migration txout fields match BTC transaction; validated: true
- check: Bifrost local share storage has new vault share for node6; desired_result: localstate file exists with signing_engine `frost`, node6 local party key, 5 participants, and sold seller excluded; validated: true
- check: Bifrost scanner state for node6 catches up from genesis/start height; desired_result: no missed deposits or duplicate observations; validated: false
- check: external DB/indexer records node status, vault rotation, migration txout, BTC migration txid; desired_result: external state matches KV and Bitcoin; validated: false

## Bad Paths

- check: node6 Bifrost missing during churn; desired_result: keygen fails/retries and no invalid vault becomes active; validated: false
- check: node6 Thornado full node lags; desired_result: node does not become active until preflight/sync passes; validated: false
- check: new keygen lacks node6 member; desired_result: churn validation fails or vault membership does not activate; validated: false
- check: migration cannot pay gas; desired_result: txout is not scheduled or explicit insufficient fee error occurs; validated: false
- check: old vault already drained before migration; desired_result: no duplicate migration or negative accounting; validated: false
- check: app restart during churn; desired_result: keygen/vault/node state resumes with same app hash; validated: false

## Attack Paths

- check: malicious node6 submits mismatched FROST keygen result; desired_result: quorum rejects or blame prevents vault activation; validated: false
- check: retired/sold node signs new vault keygen; desired_result: unauthorized because not in keygen membership; validated: false
- check: malicious node tries to keep issuing deposit addresses for retiring vault; desired_result: querier/state always returns active vault only; validated: true
- check: migration transaction is replayed; desired_result: spent UTXO prevents double spend and state does not double decrement; validated: false
- check: malicious Bifrost observes fake migration; desired_result: outbound observation matching rejects wrong txid/address/amount; validated: false
