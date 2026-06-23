# Flow 5: Standby Node Slot Auction With Shielded Bid Funding

Goal: prove a bonded standby node can sell its slot, a new node can buy by spending shielded notes to `bond_escrow`, seller receives principal as private notes, and the buyer inherits the bonded slot.

## Results

- current code model: `auction-bid-create` opens a bid with `deposit_address:"bond_escrow"`, the bidder funds it by redeeming notes with `recipient_policy:"bid_deposit"` and `bid_id`, and `node-sale-shield` shields the seller entitlement. Validated in unit coverage by `TestBidDepositRedeemCreditsBidWithoutOutbound` and `TestNodeBidDepositAndSaleShieldThenSellerUnshield`.
- historical: `/tmp/thornado-flow6-clean-v22` used the older BTC bid deposit / `auction-split` path and must not be treated as current protocol evidence.
- fresh current e2e run: pending.

## Happy Path

- check: seller node is bonded and eligible to sell; desired_result: bond_sats > 0, not already sold, node account status is exactly `Standby`; validated: true
- check: seller creates auction; desired_result: auction id binds seller operator pubkey, seller node pubkey, slot, original bond, reserve_sats, and expiry; validated: true
- check: auction API exposes active auction; desired_result: query returns exact seller and expiry data; validated: true
- check: buyer node6 creates bid record; desired_result: bid binds auction id, bidder owner, bidder operator pubkey, bidder node pubkey, and `deposit_address:"bond_escrow"`; validated: false
- check: buyer has or creates private notes; desired_result: notes are funded by the normal user deposit/sweep/shield flow before bid funding; validated: false
- check: buyer redeems note to bid deposit; desired_result: proof public inputs have `recipient:"bond_escrow"`, `recipient_policy:"bid_deposit"`, `bid_id`, and `fee_sats:0`; validated: false
- check: bid redeem creates no BTC outbound; desired_result: nullifier is spent, redeem status becomes `settled`, and bid `amount_sats` increments; validated: false
- check: seller selects winning bid; desired_result: auction status changes `open` to `selected`, selected_bid_id is set, and bid has `selected:true`; validated: true
- check: seller `node-sale-shield` succeeds; desired_result: seller commitments sum to seller_payout_sats, auction status becomes `settled`, and bid has `settled:true`; validated: false
- check: seller bond is marked sold; desired_result: seller bond query has `sold:true` and bond_sats zero or principal moved to seller receipt; validated: true
- check: seller principal receipt is produced; desired_result: seller receipt note amount equals original principal; validated: true
- check: buyer bond is created; desired_result: buyer bond_sats equals original bond plus any bid excess, slot equals seller slot, and node status is Standby with empty pubkey set until Flow 6 setup; validated: true
- check: bid excess policy is applied; desired_result: `protocol_bond_sats == max(bid_amount - seller_payout_sats, 0)` and dust remainder goes to fee accounting; validated: false

## State Checks

- check: auction KV exists before bid and closes after accepted bid; desired_result: no active auction remains after settlement; validated: true
- check: seller bond cannot be reused; desired_result: sold flag prevents second sale or second principal withdrawal; validated: true
- check: buyer bid state before settlement; desired_result: bid has `amount_sats` from one or more settled bid-deposit redeems and no shielder bond exists for buyer until seller settlement; validated: false
- check: bidder note spend is private except public policy fields; desired_result: chain sees nullifier/root/amount plus `bid_id`, not the original BTC funding deposit identity; validated: false
- check: shielder tree receives seller principal note; desired_result: seller note commitment is present once; validated: true
- check: node account status transitions; desired_result: seller remains non-active/sold and buyer is Standby until Flow 6 registers IP, version, and node keys; validated: true

## API, KV, And External Persistence

- check: auction query by id returns correct state at each stage; desired_result: created, bid matched, settled/closed states are consistent; validated: true
- check: bid redeem query returns `recipient_policy:"bid_deposit"` and `status:"settled"`; desired_result: API/KV prove bid was funded from notes without outbound BTC; validated: false
- check: seller sale entitlement query returns `settlement:"operator_sale"` and `bond_confirmed:true`; desired_result: API matches KV deposit/bond state after `node-sale-shield`; validated: false
- check: bond query for seller returns sold and zero principal; desired_result: no stale bond value remains; validated: true
- check: bond query for buyer returns standby and bid principal; desired_result: node6 is ready for churn; validated: true
- check: txout query has no bid-deposit outbound; desired_result: bid funding is internal note settlement, not a BTC transfer; validated: false
- check: external DB/indexer records auction, bid-deposit redeem, seller note, buyer bond; desired_result: all indexed rows match KV; validated: false

## Bad Paths

- check: unbonded node attempts to sell slot; desired_result: auction creation rejected; validated: true
- check: already sold node attempts second auction; desired_result: rejected; validated: true
- check: auction bid below reserve or `NodeSale_BidAmountMinSats`; desired_result: seller cannot select it and auction remains open/expired without settlement; validated: false
- check: bid-deposit redeem after auction expiry; desired_result: rejected because bid is not open for deposit; validated: false
- check: seller sale shield uses wrong auction id; desired_result: rejected and bid remains selected but unsettled; validated: false
- check: seller sale shield commitments do not match seller payout; desired_result: `node-sale-shield` rejected and bid remains selected but unsettled; validated: false
- check: seller attempts sale shield twice; desired_result: second attempt rejected because bid is settled or entitlement already committed; validated: false
- check: buyer uses duplicate consensus key; desired_result: set-node-keys rejected; validated: false

## Attack Paths

- check: attacker front-runs node sale shield with same bid id; desired_result: seller auth and selected bid binding reject non-seller; validated: false
- check: attacker creates fake auction id for seller; desired_result: bid settlement rejects because auction id is not in KV or not signed by seller; validated: true
- check: seller tries to sell after requesting leave/churn; desired_result: state machine rejects invalid status; validated: false
- check: buyer tries to acquire slot without a settled bid-deposit note redeem; desired_result: seller cannot select an unfunded bid; validated: false
- check: malicious seller changes keys after auction creation; desired_result: auction settlement still binds original seller bond/slot; validated: false
- check: malicious buyer abandons pending deposit; desired_result: pending state expires/refunds without affecting seller; validated: false
