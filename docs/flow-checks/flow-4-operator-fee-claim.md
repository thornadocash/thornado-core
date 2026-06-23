# Flow 4: Operator Fee Claim After SPLIT

Goal: prove an operator with active fee share can claim accrued withdrawal fees into private Shielder note commitments.

Latest run: `/tmp/thornado-flow4-clean-v5`, `FLOW_LIMIT=4`, real 4-node Thornado/Bifrost with regtest bitcoind.

## Happy Path

- check: operator bond has `fee_share_active:true`; desired_result: fee share became active after the node bonded by spending shielded notes to `bond_escrow`; validated: true
- check: fee entitlement query returns claimable amount; desired_result: `claimable_sats == accrued_sats - fee_debt_sats`; validated: true
- check: operator signs fee claim commitment; desired_result: signature payload binds node pubkey, owner, accrued_sats, fee_per_slot_share, note commitments, and fee note pubkeys, and verifies against stored operator pubkey; validated: true
- check: fee claim tx succeeds; desired_result: committed `operator_fee` deposit record is created directly, with `amount_sats == claimable_sats` and no pending BTC deposit session; validated: true
- check: fee commitments are inserted into shielder tree; desired_result: each note commitment exists exactly once with its claimed denomination and fee note pubkey is marked used; validated: true
- check: fee debt updates; desired_result: `fee_debt_sats` becomes current `accrued_sats` after claim; validated: true
- check: operator receipt is generated; desired_result: receipt note denomination equals claimable amount; validated: true
- check: standalone BTC redeem quote for 100k fee note; desired_result: rejected because `Withdrawal_FeeMinSats == 100000`, so the note is collected but cannot be redeemed alone with positive net output; validated: true

Results:
- before claim: `claimable_sats=100000`, `accrued_sats=100000`, `fee_debt_sats=0`, `fee_per_slot_share=100000`
- claim tx: `5A68DBB56CBB354A44FD78F68A131B2E6D09D69B9624BA5CA5A73A725FC801C6`, deliver code `0`
- fee deposit: `F9B37171F88D04D8CA30BD22E543343A5C19998B490B15074A07CFAF4B7BE3FB`, `settlement=operator_fee`, `status=committed`, `amount_sats=100000`, `commitment_count=1`
- standalone redeem quote: HTTP `500`, `withdrawal fee exceeds amount`

## State Checks

- check: fee pool total before claim; desired_result: total accrued explains claimable amount; validated: true
- check: fee pool total after claim; desired_result: claimed amount increases while historical accrual is preserved; validated: true
- check: bond record retains original bond sats; desired_result: fee claim does not reduce principal bond; validated: true
- check: shielder tree leaf for fee note is distinguishable from user split notes only by commitment/denomination, not by public note owner; desired_result: privacy model maintained for the note commitment; validated: true
- check: fee note is not redeemed during fee collection; desired_result: split-fees creates spendable note commitments and does not create a redeem/nullifier record; validated: true

Results:
- fee pool before: `total_collected_sats=100000`, `total_claimed_sats=0`, `fee_per_slot_share=100000`
- fee pool after: `total_collected_sats=100000`, `total_claimed_sats=100000`, `fee_per_slot_share=100000`
- bond before/after: `bond_sats=100000000`; `fee_debt_sats` changed `0 -> 100000`
- fee note root exists for denomination `100000` with one leaf

## API, KV, And External Persistence

- check: fee entitlement API returns node pubkey, claimable_sats, accrued_sats, fee_debt_sats, fee_per_slot_share; desired_result: all fields match KV bond and fee pool state; validated: true
- check: bond query after claim shows updated debt; desired_result: no stale entitlement remains; validated: true
- check: shielder commitment query/tree contains fee note; desired_result: external proof generator can build a valid path once note has net-positive redeem economics; validated: true
- check: fee claim deposit query returns `settlement:"operator_fee"` and `status:"committed"`; desired_result: amount, owner, commitment count, and matched height match fee split tx; validated: true
- check: fee note pubkey KV is indexed; desired_result: fee note pubkey maps to the operator fee deposit id and cannot be reused; validated: true
- check: fee-note redeem query returns expected recipient, amount, fee, and nullifier when the note is redeemed; desired_result: not exercised for 100k note because standalone redeem has zero/negative net after min fee; validated: false
- check: external database/indexer records claim, commitments, fee note pubkeys, redeem/nullifier, and BTC txid when redeemed; desired_result: KV/API checked; no separate external DB was present in this harness; validated: false

Results:
- ABCI commitment KV maps `386868959bcc868047dae627f8dde19790d8cffe51aa8b4c9f6ca64933365e07` to `F9B37171F88D04D8CA30BD22E543343A5C19998B490B15074A07CFAF4B7BE3FB`
- ABCI denomination commitment KV maps the `100000` leaf to the same deposit id
- ABCI fee-note pubkey KV maps `02b0a63370f67e5a67541f8cb69df23d3fb4288e5b00c9148538a8b83d966b0cc3` to the same deposit id
- ABCI merkle root KV returns `true` for the `100000` root

## Bad Paths

- check: unbonded node claims fees; desired_result: rejected with no fee state mutation; validated: false
- check: bonded node with inactive fee share claims fees; desired_result: rejected as no confirmed fee share; validated: false
- check: claim when `claimable_sats` is zero; desired_result: rejected with explicit result; validated: true
- check: fee commitment denominations greater than entitlement; desired_result: rejected and fee debt unchanged; validated: true
- check: claim with wrong operator signature; desired_result: rejected; validated: true
- check: claim with mismatched node pubkey/operator pubkey; desired_result: rejected; validated: false
- check: duplicate claim using same entitlement snapshot; desired_result: second claim rejected or produces zero claim; validated: true
- check: withdraw fee note twice; desired_result: second withdrawal rejected by nullifier; validated: false

Results:
- wrong signature rejected: `invalid shielder fee operator signature: signature verification failed`
- oversized claim rejected: `shielder commitment denominations exceed amount`
- duplicate claim rejected: `no shielder fees claimable`

## Attack Paths

- check: attacker claims another operator's fee using public node pubkey; desired_result: signature/auth binding rejects; validated: false
- check: attacker replays old signed fee claim after debt changed; desired_result: stale payload rejected; validated: true
- check: attacker claims fee with commitment they do not control; desired_result: protocol may allow but funds are locked to commitment; no operator principal loss; validated: false
- check: malicious operator claims during slot sale; desired_result: fee entitlement is bound to correct seller/new owner state; validated: false
- check: malicious operator claims just before churn; desired_result: fee debt and active share remain consistent after churn; validated: false

Results:
- replay of the same signed claim after debt update was rejected as `no shielder fees claimable`
- slot-sale and churn interactions remain for Flow 5/6 validation
