Use concise responses, avoid flattering me, refrain from using too many lists.

For this repo's local THORNado cluster, always build and run Go binaries with:

`-tags 'regtest mocknet'`

Do not use plain `regtest` for this workspace; it causes prefix/network mismatches in the local mocknet cluster.
