# User Flows

## Deposit

1. Wallet requests a deposit address from `go-thornado`.
2. Bitcoin Bifrost observes BTC payment.
3. `go-thornado` confirms the deposit and records spendable value.

## Split

1. Wallet uses Shielder to derive fixed-denomination note commitments.
2. Wallet submits split authorization and commitments to `go-thornado`.
3. `go-thornado` inserts commitments into denomination pools.

## Withdraw

1. Wallet fetches the denomination tree from `go-thornado`.
2. Wallet builds a Shielder withdrawal proof locally.
3. `go-thornado` verifies proof public inputs and nullifier freshness.
4. `go-thornado` queues a Bitcoin outbound.
5. Bifrost signs through the FROST signer path and broadcasts.
