# Thornado UI End-to-End Runsheet

## Target User Flow

### First-Time User

1. User opens the app and sees only the Get Address pane.
2. User reveals or enters a BIP39 secret. The pane focuses only on secret handling until done.
3. User clicks Get Address.
4. Browser performs PoW, requests a user deposit address, and shows:
   - QR code and BTC address.
   - Address expiry in human time.
   - Copy controls.
5. Deposit pane appears only after an address exists, an old address is discovered, or a deposit is seen.
6. User sends BTC externally.
7. Deposit pane tracks the selected deposit:
   - Deposit label.
   - Amount if known.
   - Address.
   - Tx ID only once known.
   - Confirmations once seen.
8. Once confirmed, Shield pane opens with a clear Shield Deposit action.
9. Shield creates notes from the confirmed deposit, with fee-bucket remainder messaging.
10. Withdraw pane shows mature notes grouped by deposit batch. Each note row shows amount and Withdraw.
11. Withdraw opens a modal for recipient address. On submit, it generates proof, submits gasless withdrawal, and shows final tx hash/status.

### Second Deposit User

1. Existing secret is reused.
2. UI scans only after Get Address or explicit restore action.
3. If the latest issued deposit address is still unexpired, show it instead of incrementing the deposit path.
4. If expired, requesting an address increments deposit path.
5. Deposit pane uses one dropdown:
   - Latest deposit first.
   - Current/new address auto-selected and menu opened after issue.
   - Each option shows `User Deposit N (amount)` and status/age, e.g. `Issued (4 sec)` or `Expired (2 days)`.
6. Selecting a deposit shows just enough detail for that deposit.

### Returning User

1. User enters existing secret.
2. UI syncs discovered deposit addresses, notes, and nullifiers without blocking the main thread.
3. Deposit pane reveals if previous deposit addresses exist.
4. Withdraw pane reveals if mature unspent notes exist.
5. User can move forward or backward by expanding/collapsing panes without losing context.

## Required Cluster Readiness

1. UI server listens on `127.0.0.1:1316`.
2. Node API listens on `127.0.0.1:1317`.
3. Four Thornado nodes are active.
4. Four Bifrost signers are healthy.
5. `/thornado/vaults/base` has one `ActiveVault`.
6. `Get Address` must not be tested before the active vault exists.

## Manual E2E Loop

1. Start cluster with `FLOW_LIMIT=1 KEEP_RUNNING=1 ops/scripts/real-4node-e2e.sh`.
2. Wait for `RESULTS Flow 1: PASS`.
3. Start UI server:
   `./build/thornado-ui --listen 127.0.0.1:1316 --node http://127.0.0.1:1317 --static-dir go-thornado/ui/static`
4. Open `http://127.0.0.1:1316/thornado/`.
5. Generate or paste secret.
6. Get Address.
7. Send BTC with regtest wallet and mine at least one block.
8. Confirm UI moves Deposit to `1 / 1`.
9. Shield Deposit.
10. Confirm notes appear.
11. Withdraw one mature note to a fresh regtest address.
12. Confirm outbound tx hash and wallet receipt.
13. Refresh page, re-enter same secret, confirm deposits/notes/nullifier state are recovered.
14. Request second deposit and confirm dropdown ordering and selected detail.

## Acceptance Criteria

The run is not accepted until the browser-visible flow completes with no manual page reloads, no stale `Failed to fetch`, no `no active shielder bitcoin vault`, and no hidden terminal-only success.

## Last Verified Local Run

Date: 2026-06-13 local time.

Environment:
- Cluster: `KEEP_RUNNING=1 FLOW_LIMIT=1 RUN_ID=ui-e2e-json-prover-1781288391 ops/scripts/real-4node-e2e.sh`
- API: `http://127.0.0.1:1317`
- UI: `http://127.0.0.1:1316/thornado/`
- Secret: `general feature warm torch drift pet promote shuffle boil author primary emotion`

Passing evidence:
- First-time flow generated a new deposit address and showed QR/address in both Get Address and selected Deposit detail.
- Address expiry displayed as `Address expires in 10 min`.
- Deposit sent: `0.12 BTC` to `bcrt1pl4e7wk2snh6arf3clyad6ch0am3qz6wwgues4y7f9e6j028yg6fqqn3j62`.
- BTC deposit tx: `39bac2ebad0f9735cbf003da0569244a78545d4c2a19d585d224bfc3b887b50c`.
- Deposit pane updated from seen `0 / 1` to confirmed `1 / 1`.
- Shield minted three notes: `0.1 BTC`, `0.01 BTC`, `0.01 BTC`.
- Refresh with the same secret immediately recovered the deposit, notes, and nullifier state in the UI.
- Withdraw pane grouped notes under `User Deposit 1 - 0.12 BTC`.
- Browser proof generated and validated in the worker without triggering `Browser proof worker failed`.
- Withdrawal tx accepted: `57CEAF1AC14AC7340CAA278496551D52CE9D48B83EE11B810A962F94A0966F49`.
- Nullifier appeared on chain with withdrawal id `B478E3A7A1B6913DF7997D95877D783BC24B0F677DB42C2EF6B278A5AAB735E4`.
- BTC wallet received `0.099 BTC` at `bcrt1q25xhfexngav9nal87pnnnhvwkhvy93jyc57pwd`, tx `9829eed50ae18d8e15840386918fa35b105c9ca6120d0f11e842e39b6ba5d7f2`.
- BTC payout was mined to 1 confirmation.
- Final repeat-user reload showed:
  - Deposit row: `User Deposit 1 (0.12 BTC) - 1 / 1`.
  - Shield notes: `0.1 BTC`, `0.01 BTC`, `0.01 BTC`, all mature.
  - Withdraw rows: `0.1 BTC Spent`, `0.01 BTC Withdraw`, `0.01 BTC Withdraw`.
- Second deposit pass with same secret:
  - `Get New Address` was visible after Deposit 1 was used.
  - Deposit 2 address issued: `bcrt1pnjh8pfs7ev8s9jcyjsspsaeh2xvvhsxcj5f4fsvrzekzw8af5ddqqsscyt`.
  - Address expiry displayed as `Address expires in 10 min`.
  - Funding tx: `aff6919d2d87ecee2de89e73fe0f1ab8aa00630924ed079764a0f1d2f45e356d`.
  - Deposit pane showed latest first: `User Deposit 2 (0.12 BTC) - 1 / 1`, then `User Deposit 1 (0.12 BTC) - 1 / 1`.

UI improvements executed in this run:
- Startup with a restored secret now scans deposit addresses only, so returning users see prior deposits without kicking off note/nullifier recovery.
- Deposit pane owns selected-batch tracking and shows a compact dropdown, latest first.
- Selected deposit detail now shows QR/address for an unexpired issued address, hides Tx ID when none exists, and keeps confirmations only where useful.
- Deposit expiry is calculated per selected batch instead of leaking stale global expiry across addresses.
- Debug derivation path labels now use `tc84'/btc'/${depositType}'/${depositIndex}'/${noteIndex}'`.
- Browser prover now uses the production `withdraw.json` with a queued witness executor, avoiding the recursive `snarkjs-old` stack overflow while staying compatible with the production proving key.
- Local E2E genesis sets `Deposit_SessionExpiryMinutes=10`; deposit address expiry no longer races the 20-block churn interval.

Current blocker:
- None for the browser E2E paths tested above.
