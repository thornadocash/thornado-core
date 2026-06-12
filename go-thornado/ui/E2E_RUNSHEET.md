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
