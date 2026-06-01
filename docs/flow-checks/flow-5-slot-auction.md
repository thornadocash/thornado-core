# Flow 5: Standby Node Slot Auction And BTC Bid Split

Goal: prove a bonded standby node can sell its slot, a new node can buy with BTC, seller receives principal as private notes, and the buyer inherits the slot bond with any bid excess converted to protocol bond commitment.

## Results

- run: `/tmp/thornado-flow6-clean-v22`; result: PASS; evidence: `ops/scripts/real-4node-e2e.sh` Flow 5 completed with real 4-node Thornado/Bifrost plus BTC regtest.

## Happy Path

- check: seller node is bonded and eligible to sell; desired_result: bond_sats > 0, not already sold, node account status is exactly `Standby`; validated: true
- check: seller creates auction; desired_result: auction id binds seller operator pubkey, seller node pubkey, slot, original bond, reserve_sats, and expiry; validated: true
- check: auction API exposes active auction; desired_result: query returns exact seller and expiry data; validated: true
- check: buyer node6 creates bid deposit session; desired_result: owner, bidder operator pubkey, bidder node pubkey, deposit address, path index are recorded; validated: true
- check: BTC bid deposit confirms; desired_result: bitcoind UTXO at derived address equals bid amount; validated: true
- check: Bifrost observes buyer deposit; desired_result: quorum inbound observation matches auction deposit; validated: true
- check: Thornado matches auction deposit; desired_result: deposit query has auction id and buyer node pubkey; validated: true
- check: seller selects winning bid; desired_result: auction status changes `open` to `selected`, selected_bid_id is set, and bid has `selected:true`; validated: true
- check: seller auction split succeeds; desired_result: seller commitments sum to seller_payout_sats, auction status becomes `settled`, and bid has `settled:true`; validated: true
- check: seller bond is marked sold; desired_result: seller bond query has `sold:true` and bond_sats zero or principal moved to seller receipt; validated: true
- check: seller principal receipt is produced; desired_result: seller receipt note amount equals original principal; validated: true
- check: buyer bond is created; desired_result: buyer bond_sats equals original bond plus any bid excess, slot equals seller slot, and node status is Standby with empty pubkey set until Flow 6 setup; validated: true
- check: buyer protocol bond commitment policy is applied; desired_result: `protocol_bond_sats == max(bid_amount - seller_payout_sats, 0)` and only that excess is inserted as protocol commitment by current code; validated: true

## State Checks

- check: auction KV exists before bid and closes after accepted bid; desired_result: no active auction remains after settlement; validated: true
- check: seller bond cannot be reused; desired_result: sold flag prevents second sale or second principal withdrawal; validated: true
- check: buyer bid state before settlement; desired_result: bid has deposit_id and amount_sats after BTC match but no shielder bond exists for buyer until seller settlement; validated: true
- check: shielder tree receives buyer protocol commitments only for bid excess; desired_result: if bid equals original principal, no buyer protocol commitment is inserted and transferred slot bond is the original bond; validated: true
- check: shielder tree receives seller principal note; desired_result: seller note commitment is present once; validated: true
- check: node account status transitions; desired_result: seller remains non-active/sold and buyer is Standby until Flow 6 registers IP, version, and node keys; validated: true

## API, KV, And External Persistence

- check: auction query by id returns correct state at each stage; desired_result: created, bid matched, settled/closed states are consistent; validated: true
- check: deposit query for buyer bid returns `settlement:"operator_sale"` and `bond_confirmed:true`; desired_result: API matches KV deposit/bond state; validated: true
- check: bond query for seller returns sold and zero principal; desired_result: no stale bond value remains; validated: true
- check: bond query for buyer returns standby and bid principal; desired_result: node6 is ready for churn; validated: true
- check: txout query shows sweep for buyer deposit; desired_result: BTC bid funds are swept to vault; validated: true
- check: external DB/indexer records auction, bid deposit, split commitments, seller note, buyer bond; desired_result: all indexed rows match KV; validated: false

## Bad Paths

- check: unbonded node attempts to sell slot; desired_result: auction creation rejected; validated: true
- check: already sold node attempts second auction; desired_result: rejected; validated: true
- check: auction bid below reserve or `NodeSale_BidAmountMinSats`; desired_result: seller cannot select it and auction remains open/expired without settlement; validated: false
- check: bid deposit after auction expiry; desired_result: rejected/refund path deterministic; validated: false
- check: seller auction split uses wrong auction id; desired_result: rejected and bid remains selected but unsettled; validated: false
- check: seller split commitments do not match seller payout; desired_result: auction split rejected and bid remains selected but unsettled; validated: true
- check: seller attempts auction split twice; desired_result: second attempt rejected because bid is settled or deposit already has commitments; validated: true
- check: buyer uses duplicate consensus key; desired_result: set-node-keys rejected; validated: false

## Attack Paths

- check: attacker front-runs auction split with same bid id; desired_result: seller auth and selected bid binding reject non-seller; validated: true
- check: attacker creates fake auction id for seller; desired_result: bid settlement rejects because auction id is not in KV or not signed by seller; validated: true
- check: seller tries to sell after requesting leave/churn; desired_result: state machine rejects invalid status; validated: false
- check: buyer tries to acquire slot without BTC finality; desired_result: split/bond confirmation blocked; validated: true
- check: malicious seller changes keys after auction creation; desired_result: auction settlement still binds original seller bond/slot; validated: false
- check: malicious buyer abandons pending deposit; desired_result: pending state expires/refunds without affecting seller; validated: false
