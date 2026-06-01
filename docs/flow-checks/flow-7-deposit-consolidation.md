# Flow 7: Many Deposits, Sweep Counting, And BTC Consolidation

Goal: prove repeated post-churn deposits are matched, swept from child paths, counted by protocol, and consolidated into the active base vault.

## Happy Path

- check: consolidation threshold config is set; desired_result: `UTXO_MaxSpendCount=3` is visible through state/config query before the first Flow 7 deposit; validated: false
- check: post-churn deposit session 1 uses new active vault; desired_result: deposit vault pubkey equals Flow 6 new ActiveVault; validated: false
- check: post-churn deposit session 2 uses next path index on new active vault; desired_result: path index increments, address differs, vault pubkey same; validated: false
- check: post-churn deposit session 3 uses next path index on new active vault; desired_result: path index increments, address differs, vault pubkey same; validated: false
- check: each BTC deposit confirms; desired_result: bitcoind has UTXO at each derived address before sweep; validated: false
- check: Bifrost observes each deposit; desired_result: each deposit has inbound quorum and correct amount/address; validated: false
- check: Thornado matches each deposit; desired_result: logs/query show `deposit_matched` for all three deposit ids; validated: false
- check: Thornado queues one sweep per deposit; desired_result: txout has three `sweep` items with path indexes 1,2,3 and active vault destination; validated: false
- check: Bifrost signs each sweep; desired_result: BTC child-path UTXOs are spent and base vault receives outputs; validated: false
- check: protocol counts finalized inbound observations; desired_result: active vault `inbound_tx_count` reaches threshold from matched deposits, regardless of whether sweep outbounds have already signed; validated: false
- check: consolidation txout is generated; desired_result: tx_type `consolidate`, in_hash blank, vault_path_index 0, source vault equals active vault, destination is active root address, amount equals active vault BTC accounting minus two max-gas estimates; validated: false
- check: Bifrost signs consolidation; desired_result: BTC transaction is a self-send from the active root address back to the same active root address, reducing UTXO count while preserving control by the active FROST signer set; validated: false
- check: consolidation destination retains control by active FROST vault; desired_result: address derives from active vault pubkey path 0 and is spendable by current signer set; validated: false

## State Checks

- check: deposit records remain distinct; desired_result: three unique deposit ids, path indexes, addresses, and BTC txids; validated: false
- check: sweep txouts do not duplicate after signing; desired_result: each deposit id has one effective signed/broadcast sweep; validated: false
- check: stale sweep handling does not hide a live missing sweep; desired_result: skipped stale sweeps correspond only to already-spent source UTXOs; validated: false
- check: consolidation does not alter deposit split state; desired_result: unsplit Flow 7 deposits remain `deposit_matched`, and any later split must still use the original deposit id and observed amount; validated: false
- check: vault accounting after consolidation; desired_result: active vault coin amount equals BTC UTXO set minus gas; validated: false
- check: no child path retains spendable funds after sweeps; desired_result: `listunspent` at child addresses returns zero; validated: false
- check: no extra consolidation is queued before threshold; desired_result: one or two finalized inbound deposits do not produce consolidate txout when `UTXO_MaxSpendCount=3`; validated: false
- check: no repeated consolidation occurs without new threshold event; desired_result: after consolidation signed, no duplicate consolidate for same UTXO set; validated: false

## API, KV, And External Persistence

- check: txout query exposes all sweeps and consolidation; desired_result: API rows match KV txout records; validated: false
- check: deposit query exposes each post-churn deposit; desired_result: API fields match KV deposit records; validated: false
- check: vault query after consolidation shows expected active vault funds; desired_result: API vault accounting matches BTC listunspent; validated: false
- check: config query returns threshold config; desired_result: API config value matches KV active config; validated: false
- check: Bifrost scanner DB records all observed BTC blocks; desired_result: scanner restart does not requeue deposits; validated: false
- check: Bifrost signer store records signed sweeps/consolidation; desired_result: restart does not rebroadcast already signed txs unnecessarily; validated: false
- check: external DB/indexer records three deposits, three sweeps, one consolidation; desired_result: indexed rows match KV and Bitcoin txids; validated: false

## Bad Paths

- check: only two deposits are made; desired_result: sweeps occur but no consolidation txout; validated: false
- check: fourth deposit after consolidation threshold; desired_result: new cycle starts cleanly or queues next sweep without corrupting prior consolidation; validated: false
- check: one deposit is below dust/minimum; desired_result: deposit not counted toward consolidation or handled by explicit rule; validated: false
- check: one sweep fails due to temporary signer outage; desired_result: threshold does not consolidate nonexistent funds; validated: false
- check: consolidation gas exceeds available funds; desired_result: txout not signed and explicit insufficient funds error recorded; validated: false
- check: consolidation source UTXO already spent; desired_result: stale/insufficient handling skips safely without halting signer; validated: false
- check: deposit to retired vault after churn; desired_result: deposit not counted for active-vault consolidation; validated: false

## Attack Paths

- check: attacker dusts active vault address to manipulate spend count; desired_result: dust either does not increment `inbound_tx_count` or is recorded as a failing attack-path finding because consolidation currently counts finalized base-vault inbound observations; validated: false
- check: attacker dusts child path addresses; desired_result: sweep amount/accounting ignores or handles unexpected extra sats deterministically; validated: false
- check: attacker creates many tiny deposits to force fee-draining consolidation; desired_result: min deposit/fee policy prevents value-negative consolidation; validated: false
- check: attacker replays old deposit txid in split/commit path; desired_result: duplicate deposit id rejected; validated: false
- check: malicious observer reports fake sweep outbound; desired_result: outbound matching rejects wrong chain/address/amount/vault pubkey; validated: false
- check: malicious Bifrost broadcasts consolidation to wrong address; desired_result: observed outbound does not match txout and vault accounting is not credited incorrectly; validated: false
