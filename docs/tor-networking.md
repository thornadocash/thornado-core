# Tor Networking

Thornado exposes user-facing surfaces through Tor and keeps validator-critical
traffic on direct low-latency links.

## Public Surfaces

Run these listeners on loopback and publish them with onion services:

- UI: `127.0.0.1:1316`
- Cosmos REST API: `127.0.0.1:1317`
- CometBFT RPC: `127.0.0.1:<network rpc port>`
- Optional Cosmos gRPC, if enabled: `127.0.0.1:9090`

Do not bind these public endpoints to `0.0.0.0` in production. Tor should be the
public ingress layer for users, browsers, wallets, and API clients.

Use the checked-in Tor config at `ops/tor/torrc`, equivalent to:

```torrc
HiddenServiceDir /var/lib/tor/thornado-ui/
HiddenServiceVersion 3
HiddenServicePort 80 127.0.0.1:1316

HiddenServiceDir /var/lib/tor/thornado-api/
HiddenServiceVersion 3
HiddenServicePort 80 127.0.0.1:1317

HiddenServiceDir /var/lib/tor/thornado-rpc/
HiddenServiceVersion 3
HiddenServicePort 80 127.0.0.1:26657
```

Clients should use the onion hostnames. The UI should call the onion API/RPC,
not a clearnet node URL.

The UI is self-contained for Tor serving: it proxies API requests through the
same origin and must not load third-party scripts.

## Direct Validator Traffic

Keep these direct:

- CometBFT P2P / consensus gossip
- Bifrost FROST keygen/keysign P2P
- validator-to-validator operational health links

These paths are latency-sensitive. Tor adds jitter and circuit churn that can
create missed consensus rounds, failed signing sessions, and false blame.

## Deployment Rules

- Firewall public REST/RPC/UI ports from clearnet.
- Publish only onion service hostnames for users and clients.
- Keep FROST and consensus endpoints off Tor unless explicitly running a slow
  experimental network with longer timeouts.
- Keep eBifrost bound to loopback; it is internal node plumbing.
