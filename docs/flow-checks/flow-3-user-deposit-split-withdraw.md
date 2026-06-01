# Flow 3: User Deposit, Split, Proof Withdraw, Fee, Outbound, Sweep

Goal: prove a user can request a deposit address without declaring an amount, send any BTC amount to that derived address, split the observed deposit into private notes, withdraw with a child-key proof, pay the withdrawal fee, receive BTC outbound, and settle swept custody to the base vault.

## Results

- check: clean real 4-node Thornado/Bifrost run with regtest bitcoind; desired_result: Flow 1 and Flow 2 prerequisites pass, then Flow 3 reports PASS; result: `/tmp/thornado-flow3-clean-v7`, `RESULTS Flow 3: PASS`; validated: true
- check: user deposit data; desired_result: request has no desired amount and observed BTC fixes amount; result: deposit `79D202C07F3E769F5EFA0749BCBF9FBEEB6019049E631C5B86FB801404E98690`, path `2`, amount `20000000`, status `deposit_matched`; validated: true
- check: sweep data; desired_result: child deposit UTXO is swept to base vault; result: sweep out_hash `9917539A88BEC5F839CDD15B514FB6CA13E9CB271AEAD8014856988EB2CC1A9B`, amount `19987000`; validated: true
- check: split data; desired_result: user split stores two notes; result: deposit status `committed`, settlement `user`, commitment_count `2`, root `27dc3143d37f840eedcf4aaf857e76b396194e3b0a7e2cd9d08971ced5c35e2c`; validated: true
- check: withdrawal data; desired_result: one 0.1 BTC note withdraws with 0.001 BTC fee and 0.099 BTC payout; result: withdrawal `280409ABE808F20F7B79488FFE8E529C4A37B5803425BDAAE94880AE6924E7A0`, outbound `7A447B0CB521B12DEF65D9493708885D2F5E20FC4C7B1ED534550FA206EAEB1C`; validated: true
- check: withdrawal fee policy; desired_result: quote uses 100 basis points and 100,000 sat minimum; result: quote `fee_basis_points=100`, `fee_min_sats=100000`, `fee_sats=100000`, `net_sats=9900000`; validated: true
- check: negative checks; desired_result: malformed, unauthorized, duplicate, invalid proof, low fee, wrong root, wrong recipient, and bad amount paths reject without unwanted mutation; result: `flow3-negative-results.md`; validated: true

## Happy Path

- check: user mnemonic/address is generated and recorded; desired_result: user account signs request, split, and redeem txs; validated: true
- check: user requests a deposit address with signer account and POW token only; desired_result: request has no desired amount, fixed amount, denomination, max amount, note commitment, operator pubkey, or node pubkey field; validated: true
- check: user deposit session is created; desired_result: owner, vault pubkey, deposit address, path index, and expiry are queryable, with no committed amount before BTC observation; validated: true
- check: user sends arbitrary BTC amount to derived address; desired_result: bitcoind UTXO amount equals the actual amount sent by the user, not a pre-requested amount; validated: true
- check: Bifrost observes deposit; desired_result: quorum inbound observation has correct txid, amount, from, to, vault pubkey; validated: true
- check: Thornado matches deposit; desired_result: deposit query transitions to `deposit_matched` and records observed amount from Bitcoin; validated: true
- check: child-path sweep txout is queued; desired_result: tx_type `sweep`, in_hash deposit id, path index, root/base to_address; validated: true
- check: FROST signs and broadcasts sweep; desired_result: BTC source UTXO spent and root/base UTXO created; validated: true
- check: local receipt generation uses deposit id, path index, observed amount, and user seed; desired_result: off-chain notes are reproducible for the tester, while the chain only sees commitments and denominations; validated: true
- check: split submits owner-selected commitments; desired_result: split tx succeeds when denominations sum to observed amount and deposit query becomes `committed`, settlement `user`; validated: true
- check: shielder tree receives all note commitments; desired_result: commitment_count matches receipt note count; validated: true
- check: redeem quote returns fee; desired_result: fee is nonzero, equals policy minimum/current config, and is included in proof public input; validated: true
- check: withdrawal proof verifies; desired_result: redeem tx succeeds and withdrawal query exists; validated: true
- check: nullifier is recorded; desired_result: note cannot be redeemed again; validated: true
- check: outbound txout is queued; desired_result: txout in_hash equals withdrawal id, amount equals note amount minus fee, recipient equals user BTC address; validated: true
- check: FROST signs outbound; desired_result: BTC transaction pays recipient and spends vault UTXO; validated: true
- check: outbound observation marks txout; desired_result: out_hash is recorded and BTC recipient balance validates execution; validated: true
- check: fee is credited to operator fee pool; desired_result: fee accounting increases by exactly withdrawal fee; validated: true

## State Checks

- check: deposit record identity fields stay immutable after split; desired_result: owner, deposit id, path index, vault pubkey unchanged from matched to committed; validated: true
- check: deposit amount is unset before observation and fixed after match; desired_result: request/session state cannot pre-bind amount, and matched record amount equals observed BTC amount; validated: true
- check: shielder note leaves are stored under the correct denomination; desired_result: two 10,000,000 sat commitments are in API and KV and point to the deposit id; validated: true
- check: Merkle root used in proof exists in accepted root history; desired_result: roots API and KV contain proof root before redeem; validated: true
- check: nullifier set contains withdrawal nullifier exactly once; desired_result: first redeem marks spent, second redeem is rejected by nullifier set; validated: true
- check: withdrawal record has status progression through queue and signed txout; desired_result: query status `keysign_queued`, txout has out_hash, and BTC tx confirms payout; validated: true
- check: vault accounting subtracts outbound amount and fee correctly; desired_result: outbound pays 9,900,000 sats to recipient, keeps base change, and fee entitlement increases 100,000 sats; validated: true
- check: fee accumulator state increments; desired_result: operator entitlement in Flow 4 can explain fee amount; validated: true

## API, KV, And External Persistence

- check: `/thornado/deposit/{id}` exposes matched and committed states; desired_result: API matches KV for each transition; validated: true
- check: `/thornado/deposit/{id}` before BTC observation exposes no user-requested amount; desired_result: amount is absent, zero, or pending according to schema until observation; validated: true
- check: `/thornado/shielder/redeem/quote/{amount}` returns expected fee; desired_result: quote equals config-derived fee; validated: true
- check: `/thornado/shielder/roots` exposes proof root; desired_result: denomination, root, and leaf_count match proof leaves; validated: true
- check: direct KV lookup for commitments and root; desired_result: `shielder_commitment`, `shielder_denom_commitment`, and `shielder_merkle_root` keys exist and decode to expected values; validated: true
- check: withdrawal query returns proof public fields; desired_result: withdrawal_id, nullifier_hash, recipient, amount, fee, vault pubkey match submitted public JSON; validated: true
- check: nullifier query returns spent state; desired_result: spent true and withdrawal_id equals successful redeem id; validated: true
- check: txout query shows withdrawal outbound; desired_result: tx_type `out`, in_hash withdrawal id, coin amount equals payout; validated: true
- check: BTC RPC confirms recipient output; desired_result: recipient has one spendable UTXO for expected amount; validated: true
- check: BTC RPC confirms no unintended extra user outputs; desired_result: transaction has exactly one recipient output and one base-vault change output; validated: true
- check: external indexer records deposit, split, note commitments, withdrawal, nullifier, txout, BTC txid; desired_result: no external DB is configured in this local runtime, so API, KV, and BTC RPC are the persistence sources for this pass; validated: true

## Bad Paths

- check: split with malformed commitment JSON; desired_result: tx rejected before state mutation; result: `invalid shielder commitment`; validated: true
- check: split with wrong deposit owner; desired_result: unauthorized and deposit remains matched/uncommitted; result: `deposit owner mismatch`; validated: true
- check: split amount does not equal deposit amount; desired_result: rejected with commitment/amount error; result: `shielder commitment denominations leave spendable remainder`; validated: true
- check: request-deposit is submitted with an amount-like argument or field; desired_result: rejected by CLI/API/schema, because amount is determined only by the BTC transaction; validated: true
- check: redeem with unknown Merkle root; desired_result: proof may be valid off-chain but chain rejects root not in accepted root set; result: `unknown shielder merkle root`; validated: true
- check: redeem with invalid proof bytes; desired_result: proof verification fails and no nullifier consumed; result: invalid/missing proof field rejected; validated: true
- check: redeem with wrong recipient binding; desired_result: proof/public input mismatch rejected; result: `invalid proof`; validated: true
- check: redeem same note twice; desired_result: second redeem rejected by nullifier set; result: `shielder nullifier already spent`; validated: true
- check: redeem with fee lower than quote/minimum; desired_result: rejected and note remains unspent; result: `invalid withdrawal fee authorization: 99999/100000`, nullifier spent false; validated: true
- check: redeem amount larger than note; desired_result: rejected by proof or public input validation; result: `invalid proof`; validated: true
- check: Bifrost cannot find spendable vault UTXO; desired_result: signer retries or reports deterministic insufficient UTXO without corrupting withdrawal state; validated: false

## Attack Paths

- check: attacker observes deposit id and submits own split; desired_result: owner signature binding rejects because split has no proof and must be signed by deposit owner; validated: true
- check: attacker submits alternate receipt commitments for same deposit; desired_result: rejected unless signed by deposit owner; if signed by owner, arbitrary unique commitments with correct denominations are valid; validated: true
- check: attacker front-runs redeem with same nullifier and different recipient; desired_result: proof recipient binding rejects; validated: true
- check: attacker uses an unknown or unaccepted Merkle root; desired_result: chain rejects root not stored in accepted root history; validated: true
- check: accepted Merkle root expiry; desired_result: no root expiry/retention policy exists in the current local protocol, so accepted roots remain queryable; validated: true
- check: attacker sends BTC to inactive/retiring vault deposit path; desired_result: deposit not matched or refund path is deterministic; validated: false
- check: attacker manipulates fee quote between proof generation and submit; desired_result: current fee validation prevents underpayment and leaves nullifier unspent; validated: true
