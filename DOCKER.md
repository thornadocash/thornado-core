# Docker Local Testnet

This starts five Thornado HTTP nodes and one Bitcoin Core regtest node.

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

Reset all Docker state:

```bash
docker compose down -v
```
