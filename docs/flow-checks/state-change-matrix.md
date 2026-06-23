# State Change Matrix

Use this while running the local chain. Capture every row as `before`, `after`, and `delta`; do not mark validated from script success alone.

Common watch set for every step:
- check: {CometBFT height/app_hash on all 4+ nodes}; desired_result: {height advances, app_hash matches across live nodes after each committed tx}; validated: false
- check: {Thornado API vs ABCI KV for touched records}; desired_result: {API fields equal decoded KV state}; validated: false
- check: {Bifrost signer logs and local DB}; desired_result: {scanner/signing actions correspond exactly to chain txout/keygen state, no duplicate replay after restart}; validated: false
- check: {Bitcoin regtest block/mempool/UTXO state}; desired_result: {BTC txids, vouts, addresses, amounts, and confirmations match Thornado accounting}; validated: false
- check: {negative-path mutation guard}; desired_result: {rejected tx changes no deposit, bond, vault, note, nullifier, txout, auction, fee, or node state}; validated: false

## Flow 1 - Genesis FROST Vault

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Genesis start | `/status`, `/thornado/node/*`, genesis config | no blocks; node1-4 in genesis | four active node accounts, zero bond, distinct secp/ed/cons keys | no node5/6 account, no vault |
| First block | `/thornado/keygen/1/{pubkey}`, KV `keygen//1`, Bifrost logs | no keygen block | one BTC base-vault keygen for node1-4 secp members | no CLI-created keygen tx, no standby member |
| FROST shares | `bifrost*/localstate-*`, logs | no local FROST share | each genesis Bifrost stores share with same vault pubkey and 4 participants | no TSS/schnorr path, no missing participant |
| Vault accepted | `/thornado/vaults/base`, KV `vault//{vault}`, `vault_base_index//` | no base vault | exactly one `ActiveVault`, BTC address valid on regtest | no duplicate active vault |
| BTC baseline | `validateaddress`, `scantxoutset`, `listunspent` | no base-vault funds | root vault UTXO set is zero | no pre-run funding counted |

## Flow 2 - Bonded Standby Node From Notes

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Node5 precheck | `/thornado/node/address/{node5}`, `/thornado/bond/{node5_cons}`, `/thornado/nodes/metrics` | node5 absent; next slot is 1 | required bond is `100000000` sats | active set remains node1-4 |
| Deposit request | `/thornado/deposit/session/{owner}`, POW record KV if exposed | no session for deposit pubkey owner | session has owner from deposit pubkey, child BTC address, vault pubkey, path index, no requested amount | no bond/operator fields |
| BTC deposit | `getrawtransaction`, `listunspent [child]`, mempool/block count | child has no UTXO | child has exactly one 1 BTC confirmed UTXO | base vault balance unchanged until sweep signs |
| Deposit match | `/thornado/deposit/{deposit_id}`, observed tx query/logs | deposit id absent | status `deposit_matched`, amount `100000000`, settlement empty/user-pending | no bond record yet |
| Sweep queued/signed | `/thornado/keysign/{h}`, `/thornado/txout/all`, BTC raw tx | no sweep for deposit id | one `sweep` txout from child path to active base vault; `out_hash` set after signing | no duplicate sweep, no outbound to user |
| BTC sweep final | `listunspent [child]`, `listunspent [base]`, `getrawtransaction(out_hash)` | child UTXO spendable | child UTXO spent; base vault receives deposit minus BTC gas | Thornado deposit amount remains full observed amount |
| Shield to note | `/thornado/deposit/{id}`, `/thornado/shielder/sync`, KV commitment/note/denomination/root | deposit matched | status `committed`, settlement `user`, one note commitment and root stored | no node bond created by shield |
| Bond proof build | proof public JSON | note unspent | public has `recipient_policy:"bond_escrow"`, `recipient:"bond_escrow"`, `node_pub_key`, `fee_sats:0` | no BTC recipient |
| `bond-from-notes` | `/thornado/bond/{node5_cons}`, `/thornado/shielder/nullifier/{n}`, redeem query/KV | nullifier unspent; no node5 bond | nullifier spent; bond `bond_sats=100000000`, slot 1, fee share active | no txout outbound |
| Node setup | `/thornado/node/address/{node5}`, node metrics | bond exists but keys/IP missing | node5 Standby with secp/ed/cons keys, IP, version | active vault membership unchanged until churn |
| Bad paths | rejected tx logs, all above records | captured snapshots | POW replay, wrong owner, wrong node, wrong policy, underbond, duplicate nullifier all reject | no nullifier consumed except successful bond |

## Flow 3 - User Deposit, Shield, Withdraw

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| User key | mnemonic/pubkey/owner derivation | no user session | owner address derives from supplied deposit pubkey | no amount in request |
| Deposit request | `/thornado/deposit/session/{owner}` | no session | child address issued with active vault pubkey and path index | no deposit id yet |
| User BTC deposit | BTC mempool/block, `listunspent [child]` | child empty | arbitrary sent amount appears at child after confirmation | amount not read from request |
| Deposit match | `/thornado/deposit/{id}` | id absent | amount_sats equals BTC observed amount | no shield commitments yet |
| Sweep | txout/keysign, BTC child/base UTXOs | child UTXO live | child spent; base vault UTXO increases by net sweep | no user withdrawal txout |
| Shield | deposit query, shielder sync, KV commitments/note records/root | deposit matched | committed/user, commitments sum to spendable amount after floor policy | no fee entitlement from shield except floor remainder if any |
| Fee quote | `/thornado/shielder/redeem/quote/{denom}`, config | note unspent | quote uses 100 bps and min 100000 sats; `fee_sats=max(1%,100000)` | note/nullifier still unspent |
| Redeem proof | proof public JSON | note unspent | root known, denomination matches note, recipient BTC, fee exact | no chain mutation yet |
| Redeem tx | redeem query, nullifier query, fee entitlement query | nullifier unspent; fee baseline captured | status `keysign_queued`, nullifier spent, fee entitlement increases by fee | no duplicate nullifier accepted |
| Outbound signs | txout/keysign, BTC raw tx, recipient `listunspent` | no recipient UTXO | one `out` tx pays `denom-fee` to recipient | no second payout for same withdrawal id |
| Bad paths | nullifier query, deposit query, shielder sync/KV root | snapshots before rejection | bad proof, wrong recipient, larger amount, low fee, unknown root reject | low-fee rejection leaves nullifier unspent |

## Flow 4 - Operator Fee Claim

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Fee baseline | `/thornado/fee/entitlements`, `/thornado/bond/{node}` | fees accrued from Flow 3 | claimable = accrued - debt for active fee-share bond | no claim deposit yet |
| Commitment build | operator signature payload, note pubkeys | no fee-note pubkeys used | signature binds node pubkey, owner, claimable sats, commitments, note pubkeys | no reused note pubkey |
| `split-fees` | fee entitlement query, deposit query, shielder sync/KV | claimable positive | operator_fee deposit committed directly; commitments stored; fee debt increases | no BTC deposit/session/txout |
| Bond after claim | `/thornado/bond/{node}` | fee debt old value | fee debt equals claimed amount, claimable drops | bond_sats unchanged |
| Bad paths | fee query, shielder sync/KV | snapshots | bad sig, wrong amount, reused fee pubkey, wrong operator reject | no fee debt increment on rejection |

## Flow 5 - Slot Auction With Shielded Bid

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Seller precheck | `/thornado/bond/{seller}`, `/thornado/node/address/{seller}` | seller Standby, unsold, bond > 0 | eligible for sale; slot captured | active set unchanged |
| Auction create | `/thornado/node/auction/{id}`, KV `node_slot_auction//{id}` | no open auction for seller | open auction with seller node, slot, reserve, original bond | seller bond not sold yet |
| Bid create | `/thornado/node/auction/{id}/bids`, `/thornado/node/bid/{bid}` | no bid | bid binds buyer owner/operator/node, amount 0, `deposit_address:"bond_escrow"` | no BTC address issued for auction |
| Bid note funding deposit | Flow 3 deposit/sweep/shield watches | buyer note absent | buyer owns private note funded through normal deposit | bid amount still 0 until redeem |
| Bid redeem | redeem query, nullifier, bid query | nullifier unspent; bid amount 0 | redeem status `settled`, policy `bid_deposit`, bid amount increments by note amount | no BTC outbound txout |
| Select bid | auction/bid query and KV | auction open, funded bid unselected | auction status current code `settled`; selected_bid_id set; bid selected/settled | seller principal not shielded until sale shield succeeds |
| Seller sale shield | sale entitlement deposit query, shielder sync/KV | seller bond unsold; sale note absent | operator_sale deposit committed; seller note commitments stored | no BTC txout |
| Bonds after sale | seller/buyer bond queries, node queries | seller has bond; buyer no slot bond | seller sold with bond_sats 0; buyer inherits slot and bond_sats = bid principal/excess policy | active membership unchanged until churn |
| Bad paths | auction/bid/bond snapshots | snapshots | non-seller select, unfunded select, wrong payout, duplicate sale shield reject | no auction/bond mutation on rejection |

## Flow 6 - Churn And Vault Migration

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Node6 readiness | node/bond/preflight APIs, Bifrost health | node6 Standby with sold slot bond | IP/version/keys present, Bifrost peers healthy | no active membership yet |
| Churn selection | node queries, validator/churn logs | node6 Standby | node6 Selected then Active at churn height; seller excluded | no low-bond automatic kickout pattern |
| New keygen | `/thornado/keygen/{h}/{pubkey}`, Bifrost logs/localstate | old active vault only | keygen membership includes node6 and incumbents, excludes sold seller | no keygen with wrong members |
| New vault | `/thornado/vaults/base`, KV vault/index | one active vault | new ActiveVault plus old RetiringVault until drain | no deposits issued to old vault |
| Migration txout | txout/keysign, BTC raw tx | old vault has UTXO(s) | `migrate` txout spends old root UTXO to new root address | no user-facing payout |
| BTC migration final | old/new `listunspent`, vault query | old vault funded | old root spent/drained; new root receives net amount; total conserved minus gas | no duplicate migrate |
| Post-churn deposit request | session query | active vault is new | new sessions use new vault pubkey/path index | old vault not returned |
| Bad paths | keygen/vault/node snapshots | snapshots | lagging node, offline Bifrost, bad keygen, fake migration reject/retry | no invalid vault activation |

## Flow 7 - Deposit Counting And Consolidation

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Config | `/thornado/config`, KV config | default threshold | `UTXO_MaxSpendCount=3` visible | unrelated config unchanged |
| Deposit sessions 1-3 | session query | active vault from Flow 6 | three unique child addresses/path indexes on new vault | no old vault address |
| BTC deposits 1-3 | BTC raw tx/listunspent | children empty | each child has one confirmed UTXO | no base-vault credit before sweep |
| Match 1-3 | deposit query/logs | ids absent | each deposit `deposit_matched` with exact observed amount | no accidental merge of deposit ids |
| Sweep 1-3 | txout/keysign, BTC child/base UTXOs | child UTXOs live | one signed sweep per deposit, child UTXOs spent, base receives net outputs | no duplicate sweep |
| Count threshold | vault query/KV counters/logs | inbound count below threshold | count reaches 3 from finalized inbound/sweep events as implemented | rejected/dust deposits not counted unless explicitly documented |
| Consolidate txout | txout/keysign | multiple base UTXOs | one `consolidate` from active root to same active root, amount = spendable minus gas policy | no user/deposit state mutation |
| BTC consolidation final | raw tx, root `listunspent` before/after | multiple root UTXOs | fewer root UTXOs, same active vault control, value conserved minus gas | no send to retired vault/wrong address |
| Bad paths | snapshots | snapshots | two deposits do not consolidate; stale spent UTXO skipped; signer outage does not consolidate nonexistent funds | no repeated consolidate without new threshold event |

## Flow 8 - Expanded Attack Paths

| Step | Watch | Before | Expected after | Must not change |
| --- | --- | --- | --- | --- |
| Snapshot guard | nodes, bonds, vaults, txouts, shielder sync, fee pool, BTC UTXOs | captured before attack tx | rejected tx has exact code/log | touched protocol state unchanged |
| Bond replay | Flow 2 proof/nullifier, node5/node6 bonds | successful bond exists | replay rejected as spent nullifier | no bond, slot, fee-share, or node status mutation |
| Wrong bond policy | user redeem proof submitted as bond | user note/proof exists | rejected because public inputs do not bind bond escrow | no bond created and no BTC outbound |
| Auction replay | Flow 5 auction/bid/sale artifacts | auction settled and seller sold | duplicate sale shield/second auction rejected | no second seller note or buyer bond |
| Node metadata after sale | sold node and buyer node queries | seller sold, buyer standby/active | seller metadata changes cannot restore eligibility | bond_sats and fee_share unchanged |
| Retired vault attack | vaults, deposit sessions, BTC txout | old vault retiring/inactive | new deposit sessions use active vault only | no child address issued for retired vault |
| Dust and rescan | BTC wallet, vault counters, txouts | base vault funded | dust does not create deposit, bond, or user note | no unexpected consolidation or value loss |
| Bifrost stale txout | logs, signer DB, txout queue | migration/sweeps complete | stale completed txout is skipped/reconciled | no rebroadcast, no errata, no app hash divergence |
