Use concise responses, avoid flattering me, refrain from using too many lists.

For this repo's local THORNado cluster, always build and run Go binaries with:

`-tags 'regtest mocknet'`

Do not use plain `regtest` for this workspace; it causes prefix/network mismatches in the local mocknet cluster.

Local cluster guardrails:

- Do not touch a user/UI cluster unless explicitly asked. Use isolated ports for test clusters.
- Do not tear down a live cluster to fix app hash or churn issues unless explicitly asked. Prefer inspecting logs, state, RPC/API, and admin-key txs against the running data.
- Do not launch long-lived Thornado/Bifrost processes as bare background jobs from a short-lived shell; they can die when the shell exits. Use the harness, `nohup`, `screen`, or another persistent supervisor, and write PIDs/logs under the run root.
- To resume an existing local 5-process real4/node5 cluster, use the one-line deploy:
  `RUN_ROOT=/tmp/thornado-node5-churn-20260619191627 BTC_RPC_PORT=24645 API_BASE=2370 GRPC_BASE=13380 RPC_BASE=33360 P2P_BASE=33380 EBIFROST_BASE=58600 FROST_P2P_BASE=9340 FROST_INFO_BASE=10340 METRICS_BASE=14200 ops/scripts/resume-real4-cluster.sh`
- The one-line deploy must supervise and self-heal: kill the previous runner, relaunch under `launchctl` on macOS (`nohup` is only a fallback), start Bifrost nodes sequentially, wait for all five Thornado RPC ports and all five Bifrost `/ping` endpoints, and restart only a specific Bifrost if its health port does not bind.
- The one-line deploy must also run a BTC regtest miner loop against the `miner` wallet, mining one block every 20 seconds, with PID/logs under the run root.
- When preserving a cluster, restart processes against the same node homes and bitcoind datadir only; do not delete `/tmp/thornado-*` state.
- Node/Bifrost launch env must keep the same run-specific port bases, `CHAIN_ID`, signer names/passwords, BTC wallet RPC hosts, FROST P2P/info ports, and bootstrap peers.
- For node churn tests, every node must have the correct node version set before expecting churn/keygen behavior.
- The keygen REST path is hot during FROST churn. Avoid repeated keyring export/decrypt per query; cache signer private key in process or queries can hang and block Bifrost keygen polling.
- For node5 churn acceleration, validate node5 first: funded/shielded/withdrawn bond, Standby or Selected state, desired node count, churn keygen emitted, all five Bifrost signers receive/process the same keygen, new 5-member vault is stored, node5 becomes Active, then migration happens.
