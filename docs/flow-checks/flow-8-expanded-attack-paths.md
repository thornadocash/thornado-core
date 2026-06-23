# Flow 8 - Expanded Attack Paths

Purpose: run after the normal protocol path and prove rejected actions do not mutate protocol state. This is not a replacement for Flow 1-7; it is the adversarial pass across the state they produce.

## Expanded Run Scope

- check: {run all seven protocol flows first, then Flow 8}; desired_result: {attacks execute against realistic state containing a FROST vault, note-funded standby node, user withdrawal, operator fee note, settled auction, churned node6 vault, and consolidated deposits}; validated: false
- check: {chain additional nodes after normal Flow 6}; desired_result: {node7 and node8 each bond from notes, register keys/IP/version, start real Thornado/Bifrost processes, churn active, complete FROST keygen, migrate BTC, and preserve app hash convergence}; validated: false
- check: {exercise more node states}; desired_result: {Absent, Standby, Selected, Active, Retiring/sold, and post-sale metadata update states are queried and asserted}; validated: false
- check: {exercise more user paths}; desired_result: {normal user deposit/withdraw, post-churn deposit routing, rejected amount-bound request, malformed split, wrong owner, low fee, duplicate redeem, wrong recipient, and unknown root all have artifacts}; validated: false
- check: {exercise more bad bonding}; desired_result: {spent bond proof replay, user-withdraw proof as bond proof, underbond/pending-bond behavior, wrong node/operator binding, and duplicate nullifier attempts are either implemented or explicitly listed as harness gaps}; validated: false
- check: {exercise auction in and out}; desired_result: {unfunded bid selection, fake auction bid, non-seller selection, funded bid, seller sale shielding, duplicate sale shielding, sold-node second auction, and post-sale buyer state are asserted}; validated: false

## Global Invariants

- check: {snapshot before every rejected tx includes nodes, bonds, vaults, txout queue, fee pool, shielder sync, active auctions/bids, BTC wallet UTXOs}; desired_result: {after snapshot is byte-for-byte equivalent for all records the rejected tx could touch}; validated: false
- check: {all rejected txs record CheckTx or DeliverTx code and exact log}; desired_result: {no attack is counted as passed merely because a command failed generically}; validated: false
- check: {all rejected txs leave CometBFT app hash consistent across live nodes}; desired_result: {all live Thornado nodes converge on same height/app_hash after rejection}; validated: false
- check: {Bifrost signer does not enqueue txout for any rejected shielder/bond/auction path}; desired_result: {no new BTC txout, keysign, or outbound BTC tx appears}; validated: false
- check: {BTC regtest mempool and wallet UTXOs after rejection}; desired_result: {no unexpected spend, no unexpected import-triggered sweep, no user payout}; validated: false

## Node State And Churn Attacks

- check: {genesis zero-bond node attempts to sell a slot after already churned/sold state}; desired_result: {auction-create rejected unless node has an active eligible bonded slot}; validated: false
- check: {sold node attempts to set desired churn state by changing version/IP/keys after sale}; desired_result: {metadata updates do not restore bond, fee share, or active eligibility}; validated: false
- check: {standby node starts Bifrost before allowlists include it}; desired_result: {node-gater blocks signing/keygen participation until chain state admits node}; validated: false
- check: {offline selected node during keygen}; desired_result: {keygen either waits/retries deterministically or fails without activating a malformed vault}; validated: false
- check: {retired/sold node signs after being removed from membership}; desired_result: {Bifrost ignores txouts where local signer is not a member/designated signer}; validated: false
- check: {node5 churn and node6 churn both create distinct vault rotations}; desired_result: {each rotation has one new active vault, one old retiring/inactive vault, and membership exactly matches active set}; validated: false
- check: {future node7 and node8 chained churns}; desired_result: {slot purchase, bond activation, keygen, migration, and post-churn deposit routing repeat without stale vault or app hash errors}; validated: false
- check: {low-bond kickout pattern is absent}; desired_result: {churn is driven by selected/eligible node logic, not automatic lowest-bond eviction}; validated: false

## Bad Bonding

- check: {replay Flow 2 bond-from-notes proof}; desired_result: {rejected with spent nullifier; bond_sats, slot, node status unchanged}; validated: false
- check: {use user_btc withdrawal proof as bond-from-notes}; desired_result: {rejected because recipient_policy/recipient is not bond_escrow}; validated: false
- check: {bond proof binds note to node A but MsgBondFromNotes submits node B}; desired_result: {rejected; no bond record for node B}; validated: false
- check: {underbond note below required slot price}; desired_result: {rejected or left as non-active pending according to current code; node cannot become Standby}; validated: false
- check: {overbond note above required slot price}; desired_result: {accepted only if current spec allows excess; excess accounting is explicit and queryable}; validated: false
- check: {bond-from-notes with wrong operator pubkey}; desired_result: {rejected or creates bond only for signed operator as specified; no hijack of another operator}; validated: false
- check: {duplicate nullifier race against bond and user redeem}; desired_result: {only one spend commits; loser rejects and no partial state remains}; validated: false

## User Deposit And Privacy Attacks

- check: {request-deposit with amount argument}; desired_result: {CLI/API rejects; amount is only BTC-observed}; validated: true
- check: {POW token replay for same owner/key}; desired_result: {second request rejected or produces no additional active deposit session}; validated: false
- check: {send BTC to expired/purged deposit address}; desired_result: {deposit not matched or deterministic refund path exists; no shieldable note created silently}; validated: false
- check: {send BTC to retired vault child path after churn}; desired_result: {old vault is not issued by current sessions and unexpected funds do not corrupt active accounting}; validated: false
- check: {dust active base vault directly}; desired_result: {does not mint user deposit, does not inflate bond, does not trigger value-negative consolidation}; validated: false
- check: {dust child deposit address before legitimate user deposit}; desired_result: {amount handling is deterministic and cannot steal or grief user commitment accounting}; validated: false
- check: {split with wrong owner signature}; desired_result: {rejected; deposit remains matched/uncommitted and can still be split by owner}; validated: true
- check: {split with malformed commitment JSON}; desired_result: {rejected; no commitments, notes, roots, or settlement written}; validated: true
- check: {partial split leaving unsupported remainder}; desired_result: {rejected; deposit remains not committed}; validated: true
- check: {redeem with wrong recipient in public input}; desired_result: {proof binding rejects; nullifier remains unspent}; validated: true
- check: {redeem with low fee}; desired_result: {rejected; nullifier remains unspent; fee pool unchanged}; validated: true
- check: {redeem with unknown root}; desired_result: {rejected; no BTC txout}; validated: true
- check: {duplicate redeem}; desired_result: {rejected with spent nullifier; no second BTC payout}; validated: true

## Fee Claim Attacks

- check: {non-operator signs fee claim}; desired_result: {invalid operator signature rejection; fee debt unchanged}; validated: true
- check: {oversized fee claim}; desired_result: {rejected; no fee note commitment or debt increment}; validated: true
- check: {fee note pubkey reuse}; desired_result: {rejected; no duplicate fee note}; validated: false
- check: {fee claim after bond sold}; desired_result: {sold node has no active fee share; claim rejected or claimable zero}; validated: false
- check: {operator claims while node is Selected/transitioning}; desired_result: {claim uses stable entitlement/debt and cannot double count}; validated: false

## Auction Attacks

- check: {unbonded node creates auction}; desired_result: {rejected; no auction KV}; validated: true
- check: {fake auction id bid}; desired_result: {rejected; no bid KV}; validated: true
- check: {select bid before bid is funded}; desired_result: {rejected; auction remains open and bid unselected}; validated: true
- check: {non-seller selects bid}; desired_result: {rejected; auction/bid unchanged}; validated: true
- check: {bid below reserve}; desired_result: {seller cannot select; bid does not become bond}; validated: false
- check: {bid redeem after auction expiry}; desired_result: {rejected or quarantined as refundable per spec; no buyer bond}; validated: false
- check: {seller selects expired auction}; desired_result: {rejected; no bond transfer}; validated: false
- check: {non-seller calls node-sale-shield}; desired_result: {rejected; seller bond remains unsold until real seller shields}; validated: true
- check: {wrong seller payout denomination}; desired_result: {rejected; no seller note, no buyer bond mutation}; validated: true
- check: {duplicate node-sale-shield}; desired_result: {rejected; no second seller entitlement}; validated: true
- check: {sold node creates another auction}; desired_result: {rejected; no second auction}; validated: true
- check: {buyer changes node/operator keys after bid creation before settlement}; desired_result: {settlement binds bid-time keys or rejects mismatch; no bond hijack}; validated: false
- check: {seller changes node keys after auction creation}; desired_result: {settlement binds original seller slot/bond or rejects mismatch}; validated: false

## BTC And Bifrost Attacks

- check: {stale completed migrate txout is seen by Bifrost after Flow 6}; desired_result: {signer does not repeatedly rebroadcast or mark errata; completed txout remains complete}; validated: false
- check: {spent sweep source input reappears in pending txout scan}; desired_result: {signer waits for observation or skips safely; no SourceTxMissing errata for already-spent source}; validated: false
- check: {consolidation includes migration output}; desired_result: {rejected by expected source-input set; consolidation uses completed sweep outputs only}; validated: true
- check: {consolidation below fee floor/value negative}; desired_result: {no txout scheduled}; validated: false
- check: {Bifrost restart mid-signing}; desired_result: {signed-batch cache prevents duplicate conflicting BTC tx and resumes observation}; validated: false
- check: {bitcoind wallet rescan contention}; desired_result: {Bifrost backs off and recovers without losing observations or producing duplicate txouts}; validated: false

## Flow 8 Harness Pass Criteria

- check: {all implemented attack probes pass}; desired_result: {run-summary status PASS and artifacts under `meta/flow8-*` include before/after snapshots}; validated: false
- check: {all unimplemented attack probes are listed in this doc with owner and harness gap}; desired_result: {no implicit coverage claim}; validated: false
