# Docker Local Testnet

This starts five Thornado HTTP nodes and one Bitcoin Core regtest node.
Each Thornado node has a stable `node_id`, a persistent app snapshot, a
persistent FROST signer file, and a persistent Bitcoin backend store under its
own Docker volume.

```bash
docker compose up --build
```

HTTP nodes:

- `node-1`: http://127.0.0.1:3030
- `node-2`: http://127.0.0.1:3031
- `node-3`: http://127.0.0.1:3032
- `node-4`: http://127.0.0.1:3033
- `node-5`: http://127.0.0.1:3034

Bitcoin RPC is exposed on `127.0.0.1:18443` with `user/password`.
The Compose bootstrap creates a `thornado` regtest wallet and mines 101 blocks
so `listunspent` has mature spendable coins.

Useful checks:

```bash
curl -fsS http://127.0.0.1:3030/state/hash
curl -fsS http://127.0.0.1:3030/peers
docker compose exec bitcoin bitcoin-cli -regtest -rpcuser=user -rpcpassword=password -rpcwallet=thornado listunspent 0
```

Vote a live 5-node network to a 20-minute churn cycle:

```bash
for n in 1 2 3 4; do
  docker compose exec node-1 curl -fsS \
    -H 'content-type: application/json' \
    -d "{\"node_id\":\"http://node-$n:3030\",\"churn_cycle_ms\":1200000,\"target_active_nodes\":4,\"max_nodes_per_churn\":1}" \
    http://node-1:3030/network/parameters/vote
done

docker compose exec node-1 curl -fsS http://node-1:3030/churn/window
```

The current repo also includes a host-side live regtest withdrawal smoke test:

```bash
scripts/live_regtest_smoke.sh
```

That script requires `bitcoind` and `bitcoin-cli` on the host. The Compose stack
is still the easiest way to run the five HTTP nodes.

Reset all Docker state:

```bash
docker compose down -v
```
