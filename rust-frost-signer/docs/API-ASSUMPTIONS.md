# FROST Signer API Assumptions

These notes are local to the Rust FROST signer workstream because `proto/frost/v1/signer.proto` is not present yet. When the shared proto exists, this document should become a checklist for reconciling Rust service models with the versioned protobuf contract.

## Service

Expected service name: `FrostSigner`.

Expected RPCs:

- `Health`
- `GetNodeSignerInfo`
- `StartDkg`
- `GetDkgStatus`
- `GetVaultPubKey`
- `StartSigning`
- `GetSigningStatus`
- `ForgetVaultShare`

## Signing Request

`StartSigningRequest` should support one of:

- PSBT input plus UTXO metadata.
- Explicit Bitcoin sighash package.
- Structured unsigned transaction plus UTXO metadata.

The request should always include policy context:

- `session_id`
- `vault_id`
- `chain_id`
- outbound transaction ID from Thornode or Bifrost
- destination address
- amount in sats
- fee rate
- UTXOs being spent
- Thornode block height
- Thornode-observed vault public key
- expected signer set
- expiry height or expiry timestamp

The signer should reject opaque signing bytes unless they are paired with enough structured fields to verify what is being signed.

## Status Model

DKG and signing status responses should use explicit states:

- `unknown`
- `pending`
- `ready`
- `complete`
- `failed`
- `expired`

Errors should be deterministic for the same request and local signer state. Transport failures are service-level errors; policy failures should be returned as typed signer errors.

