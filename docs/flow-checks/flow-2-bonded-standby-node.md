# Flow 2 - Bonded Standby Node Split

## Goal

Validate that a new node can bond with BTC, have the bond split into protocol commitments, register its node keys, and enter Standby without operator-controlled commitment data or premature deposit matching.

## Results

- check: {4-node genesis prerequisite completed}; desired_result: {Flow 1 PASS and FROST base vault exists before Node5 bond}; evidence: {/tmp/thornado-flow2-clean-v5 logs}; validated: true
- check: {Flow 2 happy path completed}; desired_result: {Node5 1.0 BTC bond is matched, split, swept to base vault, and node enters Standby}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-deposit.json, flow2-bond.json, flow2-node.json, flow2-sweep-txout.json}; validated: true
- check: {Flow 2 negative path suite completed}; desired_result: {bad claims, replays, underbond, duplicate keys, dust, and premature key setup are rejected}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-negative-results.md}; validated: true
- check: {Flow 2 pre-finality suite completed}; desired_result: {unconfirmed deposit is not matchable and split is rejected until BTC block inclusion}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-prefinality-results.md}; validated: true
- check: {Flow 2 supplemental suite completed}; desired_result: {expired session, stale sweep, Bifrost restart, and external DB/indexer scope are validated}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-supplemental-results.md}; validated: true
- check: {unit coverage for Flow 2 fixes}; desired_result: {UTXO Bifrost tests and outbound path matching tests pass}; evidence: {go test ./bifrost/pkg/chainclients/utxo; go test ./x/thornado -run 'TestObservedOutboundMatchesBTCSweepWithActualFee|TestObservedOutboundRejectsBTCSweepFromWrongPath|TestObservedOutboundRejectsBTCSweepOverMaxGas|TestObservedOutboundRejectsAlreadyCompletedBTCSweep'}; validated: true

## Happy Path

- check: {Node5 starts with no node account}; desired_result: {query returns no active/standby Node5 account before bonding}; validated: true
- check: {Node5 requests a BTC bond address with POW token and node keys}; desired_result: {request succeeds and returns child deposit address, deposit_id, expiry, vault pubkey, and path index}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-session-before-deposit.json}; validated: true
- check: {bond amount requirement for first non-genesis node}; desired_result: {next_slot=1, bond_start=0, increment=100000000 sats, required bond=100000000 sats}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node-metrics-before.json}; validated: true
- check: {Node5 sends exactly 1.0 BTC to derived child address}; desired_result: {BTC UTXO exists at child address before sweep}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-child-utxo-before-sweep.json}; validated: true
- check: {chain observes confirmed Node5 bond deposit}; desired_result: {deposit becomes committed/operator_bond with amount_sats=100000000 and commitment_count=1}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-deposit.json}; validated: true
- check: {split claim from Node5 owner succeeds}; desired_result: {bond_sats becomes 100000000, pending_sats becomes 0, fee_share_active=true}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-bond.json}; validated: true
- check: {protocol commitment is deterministic}; desired_result: {stored commitment is chain-derived, not supplied by the operator}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-goodbond-commitment-kv.json}; validated: true
- check: {set-node-keys succeeds after bond split}; desired_result: {node account stores secp256k1 pubkey, ed25519 pubkey, and consensus pubkey}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {set-ip-address succeeds for bonded node owner}; desired_result: {node account has ip_address=127.0.0.1}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {Node5 status after setup}; desired_result: {node is Standby, not Active, and has total_bond=100000000}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true

## State Changes

- check: {deposit KV identity}; desired_result: {deposit_id maps to requested owner, vault pubkey, path index, amount, status, and operator_bond settlement type}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-deposit.json}; validated: true
- check: {bond before split}; desired_result: {unmatched or pre-finality deposit cannot be split into bond}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-prefinality-split-before-mining-rejected.log}; validated: true
- check: {bond after split}; desired_result: {pending_sats=0, bond_sats=100000000, node_slot=1, fee_share_active=true}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-bond.json}; validated: true
- check: {node account creation}; desired_result: {Node5 account is created in Standby with bond owner and keys}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {vault accounting after sweep}; desired_result: {child UTXO is spent and sweep txout sends bond minus gas to base vault}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-child-utxo-after-sweep.json, flow2-sweep-txout.json}; validated: true
- check: {txout state}; desired_result: {sweep txout has tx_type=sweep, vault_path_index=1, in_hash=deposit_id, and out_hash populated after observation}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-sweep-txout.json}; validated: true

## Error Handling

- check: {POW token replay}; desired_result: {second request with same POW token is rejected}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-pow-replay-rejected.log}; validated: true
- check: {wrong owner split}; desired_result: {non-owner cannot claim Node5 bond deposit}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-wrong-owner-split-rejected.log}; validated: true
- check: {wrong deposit id}; desired_result: {split fails with deposit not found}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-wrong-deposit-id-rejected.log}; validated: true
- check: {duplicate split}; desired_result: {second split of same deposit is rejected}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-duplicate-split-rejected.log}; validated: true
- check: {set-node-keys before bond}; desired_result: {unbonded Node6 cannot set keys}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-keys-before-bond-rejected.log}; validated: true
- check: {dust deposit}; desired_result: {dust stays address_issued and does not become matched bond}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-dust-session-after.json}; validated: true
- check: {underbond for second slot}; desired_result: {Node6 1.0 BTC bond is rejected when required bond is 2.0 BTC}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-underbond-split-rejected.log}; validated: true
- check: {duplicate consensus key}; desired_result: {Node6 cannot reuse Node5 consensus key}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-duplicate-consensus-key-rejected.log}; validated: true
- check: {valid keys after duplicate-key rejection}; desired_result: {Node6 can set unique keys after rejection}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-negative-results.md}; validated: true
- check: {expired bond session}; desired_result: {deposit after expiry is not accepted as a valid bond and no sweep is queued}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-expired-session-after.json, flow2-expired-sweep-search.json, flow2-supplemental-results.md}; validated: true

## Attack Paths

- check: {attacker supplies fake commitment text}; desired_result: {chain ignores supplied text and stores deterministic protocol commitment}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-attacker-commitment-kv.json, flow2-node6-goodbond-commitment-kv.json}; validated: true
- check: {attacker claims another node deposit}; desired_result: {split is rejected for wrong owner}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-wrong-owner-split-rejected.log}; validated: true
- check: {front-run duplicate split}; desired_result: {already committed deposit cannot be split again}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-duplicate-split-rejected.log}; validated: true
- check: {mempool-only bond}; desired_result: {deposit is not matchable until BTC block inclusion}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-prefinality-results.md}; validated: true
- check: {external inbound mempool deposit}; desired_result: {Bifrost ignores non-internal inbound mempool tx until block confirmation}; evidence: {go test ./bifrost/pkg/chainclients/utxo}; validated: true
- check: {wrong child sweep source}; desired_result: {observed sweep from wrong child path cannot satisfy txout}; evidence: {go test ./x/thornado -run TestObservedOutboundRejectsBTCSweepFromWrongPath}; validated: true
- check: {gas spike or unusually high BTC fee}; desired_result: {observed BTC sweep cannot satisfy txout when observed gas exceeds MaxGas}; evidence: {go test ./x/thornado -run TestObservedOutboundRejectsBTCSweepOverMaxGas}; validated: true
- check: {stale sweep after source UTXO spent}; desired_result: {stale/manual observation does not mutate completed txout and no duplicate accounting occurs}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-stale-raw-observation.json, flow2-stale-fake-tx-query.json, flow2-supplemental-results.md; go test ./x/thornado -run TestObservedOutboundRejectsAlreadyCompletedBTCSweep}; validated: true

## API And Querier Checks

- check: {deposit query}; desired_result: {deposit query returns committed operator_bond with correct deposit_id and amount}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-deposit.json}; validated: true
- check: {bond query}; desired_result: {bond query returns Node5 bond_sats=100000000 and pending_sats=0}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-bond.json}; validated: true
- check: {node query}; desired_result: {node query returns Standby Node5 with keys, IP, and bond}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node.json}; validated: true
- check: {node metrics query}; desired_result: {required bond math follows Node_BondStartAmountSats=0 and slot increment=100000000}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node-metrics-before.json}; validated: true
- check: {txout query}; desired_result: {sweep txout is emitted and later observed with out_hash}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-sweep-txout.json}; validated: true
- check: {state machine KV commitment}; desired_result: {protocol commitment is stored under deterministic key and attacker value is absent}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-node6-goodbond-commitment-kv.json, flow2-node6-attacker-commitment-kv.json}; validated: true
- check: {Bifrost registered child-path solvency}; desired_result: {registered child-path UTXOs are counted before sweep so solvency does not halt}; evidence: {Flow 2 PASS and go test ./bifrost/pkg/chainclients/utxo}; validated: true
- check: {Bifrost scanner DB restart persistence}; desired_result: {after restart, scanner does not lose matched bond/sweep state}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-restart-deposit-before.json, flow2-restart-deposit-after.json, flow2-supplemental-results.md}; validated: true
- check: {external indexer/database}; desired_result: {local regtest run has no external DB/indexer and uses Thornado KV plus Bifrost local LevelDB only}; evidence: {/tmp/thornado-flow2-clean-v5/meta/flow2-supplemental-results.md}; validated: true

## Current Status

- check: {remaining unvalidated Flow 2 scenarios}; desired_result: {no remaining Flow 2 gaps in local regtest scope}; validated: true
