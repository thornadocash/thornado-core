# Flow 2 - Bonded Standby Node From Shielded Notes

## Goal

Validate that a new node can fund a private Shielder note, spend that note to `bond_escrow` with `MsgBondFromNotes`, register node keys, and enter Standby. Direct activation from a BTC deposit split is no longer valid.

The registered node operator is the address derived from the supplied operator pubkey. The first bond must be spent by that operator address. Later note owners can join as bonders, but they must use the already registered operator pubkey. Operator rotation changes node control while preserving the permanent node address, slot, and bonder principal ledger.

## Results

- check: {current code model}; desired_result: {normal `shielder shield` rejects node-bond deposits and `MsgBondFromNotes` is the only bond activation path}; evidence: {`go-thornado/x/thornado/shielder_flow.go`, `TestNodeBondDepositPathRejected`, `TestBondFromNotesConfirmsStandbyNode`}; validated: true
- check: {fresh current e2e run}; desired_result: {Node5 funds notes, redeems note to `bond_escrow`, bond query shows Standby}; evidence: {pending}; validated: false
- historical: `/tmp/thornado-flow2-clean-v5` validated the old direct bond split path and must not be treated as current protocol evidence.

## Happy Path

- check: {Node5 starts with no node account}; desired_result: {query returns no active/standby Node5 account before bonding}; validated: true
- check: {Node5 requests a normal private deposit address with POW token and deposit pubkey}; desired_result: {request succeeds and returns child deposit address, vault pubkey, path index, expiry, and no node/bond fields}; validated: false
- check: {bond amount requirement for first non-genesis node}; desired_result: {required bond is computed from chain metrics as `bond_start_amount_sats + next_slot * bond_slot_increment_sats`; current config starts at 100000000 sats and increments by 100000000 sats}; evidence: {`/thornado/nodes/metrics`}; validated: true
- check: {Node5 sends the required BTC amount to derived child address}; desired_result: {BTC UTXO exists at child address before sweep}; validated: false
- check: {Bifrost sweeps child-path BTC to base vault}; desired_result: {sweep txout spends child UTXO and base vault receives funds}; validated: false
- check: {Node5 shields the matched deposit into private notes}; desired_result: {deposit becomes committed/user and all receipt notes are accepted into Merkle roots}; validated: false
- check: {Node5 builds a zero-fee withdrawal proof to `bond_escrow`}; desired_result: {public inputs have `recipient:"bond_escrow"`, `recipient_policy:"bond_escrow"`, `node_pub_key` equal Node5 consensus pubkey, `fee_sats:0`}; validated: false
- check: {Node5 submits `shielder bond-from-notes` for all receipt notes}; desired_result: {nullifiers are consumed, no BTC outbound txout is queued, and `bond_sats` reaches the required amount}; validated: false
- check: {underbonded node note}; desired_result: {below-required bond is stored as pending_sats and does not activate fee share or node setup}; validated: false
- check: {non-operator first bonder}; desired_result: {first bond attempt is rejected unless signer is the address derived from the operator pubkey}; validated: false
- check: {different note owner tops up an existing node bond}; desired_result: {bond_sats increases, registered operator pubkey and node address are unchanged}; validated: false
- check: {set-node-keys succeeds after note-bond}; desired_result: {node account stores secp256k1 pubkey and consensus pubkey}; validated: false
- check: {set-ip-address succeeds for bonded node owner}; desired_result: {node account has ip_address=127.0.0.1}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {registered operator rotates to a new operator pubkey}; desired_result: {bond operator_pub_key and node_operator_address update, node_address and slot remain unchanged}; validated: false
- check: {old operator after rotation}; desired_result: {old operator cannot send node-address transactions}; validated: false
- check: {new operator after rotation}; desired_result: {new operator can send node-address transactions such as maintenance}; validated: false
- check: {Node5 status after setup}; desired_result: {node is Standby, not Active, and has total_bond equal to the required chain-configured amount}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true

## State Changes

- check: {deposit KV identity before note bond}; desired_result: {deposit_id maps to requested owner, vault pubkey, path index, amount, status `committed`, settlement `user`}; validated: false
- check: {normal split cannot activate bond}; desired_result: {node-bond fields on a deposit are either absent for normal user deposits or rejected with `node bonds activate via MsgBondFromNotes`}; validated: true
- check: {bond after sufficient `MsgBondFromNotes` total}; desired_result: {pending_sats=0, bond_sats reaches required amount, node_slot is assigned, fee_share_active=true}; validated: false
- check: {node account creation}; desired_result: {Node5 account is created in Standby with registered operator address and keys}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {multiple bonders}; desired_result: {additional bond notes can increase bond_sats without replacing operator_pub_key, node_operator_address, node_address, or slot}; validated: false
- check: {bond pool ledger}; desired_result: {sum of bonder principal/pending equals node bond_sats/pending_sats}; validated: false
- check: {operator rotation}; desired_result: {registered operator changes, node address is stable, existing bond accounting is unchanged}; validated: false
- check: {vault accounting after sweep}; desired_result: {child UTXO is spent and sweep txout sends bond minus gas to base vault}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-child-utxo-after-sweep.json, flow2-sweep-txout.json}; validated: true
- check: {txout state}; desired_result: {sweep txout has tx_type=sweep, vault_path_index=1, in_hash=deposit_id, and out_hash populated after observation}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-sweep-txout.json}; validated: true

## Error Handling

- check: {POW token replay}; desired_result: {second request with same POW token is rejected}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-pow-replay-rejected.log}; validated: true
- check: {wrong operator pubkey on existing node bond}; desired_result: {top-up with an operator pubkey different from the registered operator is rejected with `bond operator mismatch`}; validated: false
- check: {first bonder not operator}; desired_result: {initial bond spend is rejected with `first node bonder must be the operator`}; validated: false
- check: {different bonder note top-up}; desired_result: {a different signer can spend their own note into the existing node bond when using the registered operator pubkey}; validated: false
- check: {wrong node pubkey in public inputs}; desired_result: {`bond public node pubkey mismatch` or `bond node pubkey mismatch`}; validated: false
- check: {duplicate note bond}; desired_result: {second submission with same proof/nullifier is rejected by nullifier set}; validated: false
- check: {old operator after rotation}; desired_result: {maintenance/config/node-address transaction is rejected}; validated: false
- check: {new operator after rotation}; desired_result: {maintenance/config/node-address transaction succeeds}; validated: false
- check: {set-node-keys before bond}; desired_result: {unbonded Node6 cannot set keys}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-keys-before-bond-rejected.log}; validated: true
- check: {dust deposit}; desired_result: {dust stays address_issued and does not become matched bond}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-dust-session-after.json}; validated: true
- check: {underbond for second slot}; desired_result: {Node6 1.0 BTC bond remains pending when required bond is 2.0 BTC}; validated: false
- check: {duplicate consensus key}; desired_result: {Node6 cannot reuse Node5 consensus key}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-duplicate-consensus-key-rejected.log}; validated: true
- check: {valid keys after duplicate-key rejection}; desired_result: {Node6 can set unique keys after rejection}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-negative-results.md}; validated: true
- check: {expired bond session}; desired_result: {deposit after expiry is not accepted as a valid bond and no sweep is queued}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-expired-session-after.json, flow2-expired-sweep-search.json, flow2-supplemental-results.md}; validated: true

## Attack Paths

- check: {attacker submits normal user redeem instead of bond redeem}; desired_result: {redeem either pays BTC as `user_btc` or is rejected; it never creates node bond}; validated: false
- check: {attacker front-runs bond note with same nullifier}; desired_result: {proof is bound to `bond_escrow` and node pubkey; one successful spend consumes the nullifier and all replays reject}; validated: false
- check: {attacker binds note to another node pubkey}; desired_result: {public input node key must match `MsgBondFromNotes.node_pub_key`}; validated: false
- check: {mempool-only bond}; desired_result: {deposit is not matchable until BTC block inclusion}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-prefinality-results.md}; validated: true
- check: {external inbound mempool deposit}; desired_result: {Bifrost ignores non-internal inbound mempool tx until block confirmation}; evidence: {go test ./bifrost/pkg/chainclients/utxo}; validated: true
- check: {wrong child sweep source}; desired_result: {observed sweep from wrong child path cannot satisfy txout}; evidence: {go test ./x/thornado -run TestObservedOutboundRejectsBTCSweepFromWrongPath}; validated: true
- check: {gas spike or unusually high BTC fee}; desired_result: {observed BTC sweep cannot satisfy txout when observed gas exceeds MaxGas}; evidence: {go test ./x/thornado -run TestObservedOutboundRejectsBTCSweepOverMaxGas}; validated: true
- check: {stale sweep after source UTXO spent}; desired_result: {stale/manual observation does not mutate completed txout and no duplicate accounting occurs}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-stale-raw-observation.json, flow2-stale-fake-tx-query.json, flow2-supplemental-results.md; go test ./x/thornado -run TestObservedOutboundRejectsAlreadyCompletedBTCSweep}; validated: true

## API And Querier Checks

- check: {deposit query}; desired_result: {funding deposit query returns committed user deposit with correct deposit_id and amount}; validated: false
- check: {redeem/nullifier query}; desired_result: {bond note redeem exists with status `authorized` or `settled`, policy `bond_escrow`, `fee_sats:0`, and nullifier spent}; validated: false
- check: {bond query}; desired_result: {bond query returns Node5 bond_sats equal to the required amount and pending_sats=0 after `MsgBondFromNotes`}; validated: false
- check: {bond query after top-up}; desired_result: {bond query returns increased bond_sats and unchanged operator_pub_key}; validated: false
- check: {bond query after rotation}; desired_result: {bond query returns new operator_pub_key and unchanged node_address/slot/bond_sats}; validated: false
- check: {node query}; desired_result: {node query returns Standby Node5 with keys, IP, registered operator address, and bond}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {node query after rotation}; desired_result: {node_operator_address changes to the new operator and node_address remains stable}; validated: false
- check: {node metrics query}; desired_result: {required bond math follows Node_BondStartAmountSats=0 and slot increment=100000000}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node-metrics-before.json}; validated: true
- check: {txout query}; desired_result: {sweep txout is emitted and later observed with out_hash}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-sweep-txout.json}; validated: true
- check: {state machine KV}; desired_result: {note commitment, Merkle root, spent nullifier, and shielder node bond records exist and match APIs}; validated: false
- check: {Bifrost registered child-path solvency}; desired_result: {registered child-path UTXOs are counted before sweep so solvency does not halt}; evidence: {Flow 2 PASS and go test ./bifrost/pkg/chainclients/utxo}; validated: true
- check: {Bifrost scanner DB restart persistence}; desired_result: {after restart, scanner does not lose matched bond/sweep state}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-restart-deposit-before.json, flow2-restart-deposit-after.json, flow2-supplemental-results.md}; validated: true
- check: {external indexer/database}; desired_result: {local regtest run has no external DB/indexer and uses Thornado KV plus Bifrost local LevelDB only}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-supplemental-results.md}; validated: true

## Current Status

- check: {remaining unvalidated Flow 2 scenarios}; desired_result: {fresh local regtest run covers the current note-bond flow}; validated: false
