# Flow 9 - Node Tooling

## Goal

Validate live CLI/API behavior for node operator tooling after the churn and migration flows have produced a bonded active node set.

## Covered In Harness

- check: non-operator `node maint`; desired_result: rejected with `not authorized`; validated: true
- check: operator `node maint`; desired_result: maintenance toggles on, then off, and node query reflects both states; validated: true
- check: non-active account `config`; desired_result: rejected with `not authorized` and live config value is unchanged; validated: true
- check: non-operator `node fees set`; desired_result: rejected with `node operator signer mismatch`; validated: true
- check: operator `node fees set`; desired_result: bond query updates `operator_fee_basis_points`; validated: true
- check: excessive operator fee; desired_result: rejected and bond fee state is unchanged; validated: true
- check: `node rotate-operator`; desired_result: bond operator pubkey changes, node operator address changes, node address/slot/bond amount stay stable, and the operator bonder principal moves to the new operator; validated: true
- check: old operator after rotation; desired_result: maintenance and fee-set are rejected; validated: true
- check: new operator after rotation; desired_result: maintenance and fee-set succeed; validated: true

## Known Gaps

- `leave`: no `tx thornado node leave` command exists in the CLI. Node account fields exist (`requested_to_leave`, `forced_to_leave`, `leave_score`), but there is no live node-tool transaction for the harness to exercise.
- bond provider removal/unbond: bonders can be added/top-upped through `bond-from-notes`, but no CLI/protocol transaction currently removes a bonder or withdraws bond principal.
- fee-share distribution has unit coverage around operator commission and bonder accounting; the HCloud Flow9 live pass covers fee configuration auth/state, not a full multi-bonder fee claim distribution.
