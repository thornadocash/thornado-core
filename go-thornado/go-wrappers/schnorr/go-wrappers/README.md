# THORChain Schnorr Go Wrappers

This package is the in-repo Go wrapper boundary for Bitcoin Taproot Schnorr/FROST.

Target upstream:

- `https://github.com/ZcashFoundation/frost`
- crate: `frost-secp256k1-tr`

The Go API is intentionally small: deterministic participant ordering,
opaque keygen/keysign session handles, keyshare serialization, and BIP340
verification. Go moves opaque protocol messages through Bifrost P2P; Rust owns
the FROST DKG/signing state.

Build the native library before running Go tests that import this package:

```bash
schnorr/build-libgoschnorr.sh
```

The Rust crate is in `schnorr/rust/libgoschnorr` and pins
`frost-secp256k1-tr = 3.0.0`.

Docker images need `libgoschnorr.so` in
`schnorr/go-wrappers/includes/linux/{amd64,arm64}` before build. The runtime
image copies the matching library into `/usr/lib`.
