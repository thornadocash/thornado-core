# Flow Checks

Manual protocol validation workbook for Thornado/Bifrost/FROST e2e flows.

Use these files as the source of truth while stepping a fresh local chain. Every line starts with `validated: false`; only flip to `true` after manual inspection of chain state, Bifrost logs, BTC regtest state, APIs, and any external persistence.

Run context to capture before starting:
- check: run_id is recorded with timestamp, git commit, branch, local config, binary hashes, docker image ids, and BTC regtest datadir; desired_result: all artifacts are reproducible and tied to one run; validated: false
- check: all previous node, Bifrost, bitcoind, app, and external database state is torn down before genesis; desired_result: no stale app hash, mempool, wallet, sqlite/leveldb, or txout state contaminates the run; validated: false
- check: all API, RPC, gRPC, p2p, Bifrost health, and bitcoind ports are recorded; desired_result: every later query can be mapped to the node that served it; validated: false
- check: logs are captured per process with monotonic timestamps; desired_result: every state transition can be traced from request to state write to query/API visibility; validated: false
- check: CometBFT status for all Thornado nodes before each flow; desired_result: height, app hash, validator hash, catching_up, and latest block time are aligned; validated: false
- check: Bifrost health for all signer nodes before each flow; desired_result: all expected Bifrosts are live and peer count matches active signer set; validated: false
- check: BTC regtest wallet and chain state before each flow; desired_result: block count, wallet UTXOs, mempool, and known vault addresses are recorded; validated: false
- check: external persistence state before each flow; desired_result: Bifrost local state stores, scanner positions, attestation stores, and any configured DBs are snapshotted; validated: false

Documents:
- [Flow 1 Genesis FROST Vault](./flow-1-genesis-frost-vault.md)
- [Flow 2 Bonded Standby Node](./flow-2-bonded-standby-node.md)
- [Flow 3 User Deposit Split Withdraw](./flow-3-user-deposit-split-withdraw.md)
- [Flow 4 Operator Fee Claim](./flow-4-operator-fee-claim.md)
- [Flow 5 Slot Auction](./flow-5-slot-auction.md)
- [Flow 6 Churn Migration](./flow-6-churn-migration.md)
- [Flow 7 Deposit Consolidation](./flow-7-deposit-consolidation.md)

Manual validation rules:
- check: a flow is not complete until happy path, state checks, bad paths, attack paths, API/querier checks, and persistence checks are all evaluated; desired_result: no section is skipped because the script passed; validated: false
- check: every tx hash, deposit id, withdrawal id, auction id, vault pubkey, node pubkey, note commitment, nullifier, and BTC txid is written into the run notes; desired_result: all identities are traceable across API, KV, logs, and BTC; validated: false
- check: every expected rejection has an exact error code/log/message recorded; desired_result: bad paths are validated as deterministic failures, not just non-success; validated: false
- check: every external BTC movement is validated with `getrawtransaction`, `gettxout`, wallet `listunspent`, and Thornado txout/query state; desired_result: Thornado accounting matches Bitcoin reality; validated: false
