# Thornado Test Fixtures

Fixtures in this directory define cross-language behavior consumed by Go and Rust tests.

Required fixture families:

- `privacy/`: commitment computation, nullifier computation, valid and invalid withdrawal proofs, verifying key hashes.
- `frost/`: DKG transcript hashes, vault participant sets, signing sessions, partial signatures, aggregate signatures.
- `bitcoin/`: outbound signing payloads, PSBT or sighash packages, UTXO metadata, expected signature verification results.

Fixture files should use deterministic JSON unless binary payloads are unavoidable. Binary values should be hex encoded with field names matching the protobuf schema where practical.
