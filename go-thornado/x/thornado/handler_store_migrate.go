package thornado

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

// StoreMigrateHandler processes MsgStoreMigrate: a supermajority-voted, typed
// state correction. Each active node submits an identical (key, value); once
// two-thirds of active nodes agree on the same value the targeted mutation is
// applied exactly once. This is the reusable admin escape hatch for correcting
// state that normal chain logic cannot (phantom vault balances, stuck txouts,
// halts) without shipping a code upgrade.
type StoreMigrateHandler struct {
	mgr Manager
}

// NewStoreMigrateHandler creates a new StoreMigrateHandler.
func NewStoreMigrateHandler(mgr Manager) StoreMigrateHandler {
	return StoreMigrateHandler{mgr: mgr}
}

// Run is the main entry point to execute store-migrate logic.
func (h StoreMigrateHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgStoreMigrate)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg store migrate failed validation", "error", err)
		return nil, err
	}
	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg store migrate", "error", err)
		return nil, err
	}
	return &cosmos.Result{}, nil
}

func (h StoreMigrateHandler) validate(ctx cosmos.Context, msg MsgStoreMigrate) error {
	if err := msg.ValidateBasic(); err != nil {
		return err
	}
	if _, err := parseStoreMigrateTarget(msg.Key); err != nil {
		return cosmos.ErrUnknownRequest(err.Error())
	}
	if _, err := validateStoreMigrateAuth(ctx, h.mgr.Keeper(), msg); err != nil {
		return err
	}
	return nil
}

func (h StoreMigrateHandler) handle(ctx cosmos.Context, msg MsgStoreMigrate) error {
	k := h.mgr.Keeper()
	nodeAccount, err := resolveActiveNodeAccountBySigner(ctx, k, msg.Signer)
	if err != nil {
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", msg.Signer))
	}

	// Record (or overwrite) this node's vote.
	k.SetStoreMigrateVote(ctx, msg.Key, msg.Value, nodeAccount.NodeAddress)
	ctx.Logger().Info("store migrate vote recorded", "node", nodeAccount.NodeAddress.String(), "key", msg.Key, "value", msg.Value)

	activeNodes, err := k.ListActiveNodes(ctx)
	if err != nil {
		return err
	}
	active := activeNodes.GetNodeAddresses()

	// Count votes for the submitted value among active nodes only.
	votes := k.GetStoreMigrateVotes(ctx, msg.Key)
	count := 0
	for signer, val := range votes.Votes {
		if val != msg.Value {
			continue
		}
		acc, err := cosmos.AccAddressFromBech32(signer)
		if err != nil {
			continue
		}
		for _, a := range active {
			if a.Equals(acc) {
				count++
				break
			}
		}
	}

	if !types.HasSuperMajority(count, len(active)) {
		return nil
	}

	// Idempotency: skip if already applied at this exact value.
	if applied, ok := k.GetStoreMigrateApplied(ctx, msg.Key); ok && applied == msg.Value {
		return nil
	}

	if err := applyStoreMigration(ctx, k, msg.Key, msg.Value); err != nil {
		// Mark applied anyway so a permanently-failing migration does not spam
		// every block; operators can re-vote a corrected (key,value).
		ctx.Logger().Error("store migration apply failed", "key", msg.Key, "value", msg.Value, "error", err)
		k.SetStoreMigrateApplied(ctx, msg.Key, msg.Value)
		k.DeleteStoreMigrateVotes(ctx, msg.Key)
		return nil
	}

	k.SetStoreMigrateApplied(ctx, msg.Key, msg.Value)
	k.DeleteStoreMigrateVotes(ctx, msg.Key)
	ctx.Logger().Info("store migration applied", "key", msg.Key, "value", msg.Value, "votes", count, "active", len(active))
	ctx.EventManager().EmitEvent(cosmos.NewEvent("store_migrate",
		cosmos.NewAttribute("key", msg.Key),
		cosmos.NewAttribute("value", msg.Value),
	))
	return nil
}

// storeMigrateTarget is a parsed migration selector.
type storeMigrateTarget struct {
	kind string
	args []string
}

// parseStoreMigrateTarget parses "KIND:arg1:arg2..." and validates the target
// is one we know how to apply. The KIND token is case-insensitive; the args
// (pubkeys, assets) keep their original case.
func parseStoreMigrateTarget(key string) (storeMigrateTarget, error) {
	parts := strings.Split(key, ":")
	if len(parts) == 0 || parts[0] == "" {
		return storeMigrateTarget{}, fmt.Errorf("empty store migrate key")
	}
	t := storeMigrateTarget{kind: strings.ToUpper(parts[0]), args: parts[1:]}
	switch t.kind {
	case "CONFIG":
		if len(t.args) != 1 {
			return t, fmt.Errorf("CONFIG target needs CONFIG:<KEY>")
		}
	case "VAULTCOIN":
		if len(t.args) != 2 {
			return t, fmt.Errorf("VAULTCOIN target needs VAULTCOIN:<pubkey>:<ASSET>")
		}
	case "VAULTSTATUS":
		if len(t.args) != 1 {
			return t, fmt.Errorf("VAULTSTATUS target needs VAULTSTATUS:<pubkey>")
		}
	case "TXOUTCANCEL":
		if len(t.args) != 2 {
			return t, fmt.Errorf("TXOUTCANCEL target needs TXOUTCANCEL:<height>:<index>")
		}
	case "KVSET", "KVDEL":
		if len(t.args) != 1 {
			return t, fmt.Errorf("%s target needs %s:<hex-store-key>", t.kind, t.kind)
		}
		raw, err := hex.DecodeString(t.args[0])
		if err != nil || len(raw) == 0 {
			return t, fmt.Errorf("%s store key must be non-empty hex: %v", t.kind, err)
		}
	case "SHIELDERNOTESWEEP":
		if len(t.args) != 1 {
			return t, fmt.Errorf("SHIELDERNOTESWEEP target needs SHIELDERNOTESWEEP:<denominationSats>")
		}
		if denom, err := strconv.ParseUint(t.args[0], 10, 64); err != nil || denom == 0 {
			return t, fmt.Errorf("SHIELDERNOTESWEEP denomination must be a positive integer: %v", err)
		}
	default:
		return t, fmt.Errorf("unknown store migrate target %q", t.kind)
	}
	return t, nil
}

// storeMigrateKeeper is the minimal keeper surface the dispatch targets touch.
type storeMigrateKeeper interface {
	SetConfig(cosmos.Context, string, int64)
	GetVault(cosmos.Context, common.PubKey) (Vault, error)
	SetVault(cosmos.Context, Vault) error
	GetTxOut(cosmos.Context, int64) (*TxOut, error)
	SetTxOut(cosmos.Context, *TxOut) error
	ClearTxOut(cosmos.Context, int64) error
	SetRawStoreValue(ctx cosmos.Context, key, value []byte) error
	DeleteRawStoreValue(ctx cosmos.Context, key []byte)
	ValidateRawStoreKey(key []byte) error
	SweepOrphanShielderNoteRecords(ctx cosmos.Context, denominationSats uint64) (int, error)
}

// applyStoreMigration dispatches a validated, supermajority-approved migration.
func applyStoreMigration(ctx cosmos.Context, k storeMigrateKeeper, key, value string) error {
	t, err := parseStoreMigrateTarget(key)
	if err != nil {
		return err
	}
	switch t.kind {
	case "CONFIG":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("CONFIG value must be int64: %w", err)
		}
		k.SetConfig(ctx, t.args[0], v)
		return nil

	case "VAULTCOIN":
		pk, err := common.NewPubKey(t.args[0])
		if err != nil {
			return fmt.Errorf("bad vault pubkey: %w", err)
		}
		asset, err := common.NewAsset(t.args[1])
		if err != nil {
			return fmt.Errorf("bad asset: %w", err)
		}
		amt, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("VAULTCOIN value must be sats uint: %w", err)
		}
		vault, err := k.GetVault(ctx, pk)
		if err != nil {
			return fmt.Errorf("get vault: %w", err)
		}
		target := cosmos.NewUint(amt)
		current := vault.GetCoin(asset).Amount
		// Move current -> target with the existing add/sub helpers.
		if target.GTE(current) {
			vault.AddFunds(common.NewCoins(common.NewCoin(asset, target.Sub(current))))
		} else {
			vault.SubFunds(common.NewCoins(common.NewCoin(asset, current.Sub(target))))
		}
		if !vault.HasFunds() && vault.Status == RetiringVault {
			vault.UpdateStatus(InactiveVault, ctx.BlockHeight())
		}
		return k.SetVault(ctx, vault)

	case "VAULTSTATUS":
		pk, err := common.NewPubKey(t.args[0])
		if err != nil {
			return fmt.Errorf("bad vault pubkey: %w", err)
		}
		status, ok := parseVaultStatus(value)
		if !ok {
			return fmt.Errorf("unknown vault status %q", value)
		}
		vault, err := k.GetVault(ctx, pk)
		if err != nil {
			return fmt.Errorf("get vault: %w", err)
		}
		vault.UpdateStatus(status, ctx.BlockHeight())
		return k.SetVault(ctx, vault)

	case "TXOUTCANCEL":
		height, err := strconv.ParseInt(t.args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("TXOUTCANCEL height must be int64: %w", err)
		}
		idx, err := strconv.Atoi(t.args[1])
		if err != nil {
			return fmt.Errorf("TXOUTCANCEL index must be int: %w", err)
		}
		txOut, err := k.GetTxOut(ctx, height)
		if err != nil {
			return fmt.Errorf("get txout: %w", err)
		}
		if idx < 0 || idx >= len(txOut.TxArray) {
			return fmt.Errorf("txout index %d out of range (len %d)", idx, len(txOut.TxArray))
		}
		// Drop the stuck item so it is no longer signable, and clear the vault's
		// pending-tx-height reference so it stops weighing on solvency.
		item := txOut.TxArray[idx]
		txOut.TxArray = append(txOut.TxArray[:idx], txOut.TxArray[idx+1:]...)
		if len(txOut.TxArray) == 0 {
			// SetTxOut silently no-ops on an empty block, which would leave the
			// old record (item included) in the store — delete it instead.
			if err := k.ClearTxOut(ctx, height); err != nil {
				return fmt.Errorf("clear txout: %w", err)
			}
		} else if err := k.SetTxOut(ctx, txOut); err != nil {
			return fmt.Errorf("set txout: %w", err)
		}
		if !item.VaultPubKey.IsEmpty() {
			if vault, verr := k.GetVault(ctx, item.VaultPubKey); verr == nil {
				vault.RemovePendingTxBlockHeights(height)
				_ = k.SetVault(ctx, vault)
			}
		}
		return nil

	case "KVSET":
		rawKey, err := hex.DecodeString(t.args[0])
		if err != nil {
			return fmt.Errorf("KVSET key must be hex: %w", err)
		}
		rawVal, err := hex.DecodeString(value)
		if err != nil {
			return fmt.Errorf("KVSET value must be hex: %w", err)
		}
		// SetRawStoreValue re-validates that the bytes decode as the type read
		// under this prefix, so a raw write can never leave undecodable bytes
		// that would panic a reader.
		return k.SetRawStoreValue(ctx, rawKey, rawVal)

	case "KVDEL":
		rawKey, err := hex.DecodeString(t.args[0])
		if err != nil {
			return fmt.Errorf("KVDEL key must be hex: %w", err)
		}
		if err := k.ValidateRawStoreKey(rawKey); err != nil {
			return err
		}
		k.DeleteRawStoreValue(ctx, rawKey)
		return nil

	case "SHIELDERNOTESWEEP":
		// Value is an operator-chosen tag ("SWEEP1"): idempotency is per
		// (key, value), so re-running a sweep later just needs a new tag.
		denom, err := strconv.ParseUint(t.args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("SHIELDERNOTESWEEP denomination must be uint: %w", err)
		}
		n, err := k.SweepOrphanShielderNoteRecords(ctx, denom)
		if err != nil {
			return fmt.Errorf("sweep orphan shielder note records: %w", err)
		}
		ctx.Logger().Info("swept orphan shielder note records", "denomination", denom, "deleted", n)
		return nil
	}
	return fmt.Errorf("unhandled store migrate target %q", t.kind)
}

func parseVaultStatus(value string) (VaultStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "activevault":
		return types.VaultStatus_ActiveVault, true
	case "retiring", "retiringvault":
		return types.VaultStatus_RetiringVault, true
	case "inactive", "inactivevault":
		return types.VaultStatus_InactiveVault, true
	case "init", "initvault":
		return types.VaultStatus_InitVault, true
	}
	return types.VaultStatus_InactiveVault, false
}

func validateStoreMigrateAuth(ctx cosmos.Context, k keeper.Keeper, msg MsgStoreMigrate) (cosmos.Context, error) {
	return activeOperationalNodeSignerPriority(ctx, k, msg.GetSigners())
}

// StoreMigrateAnteHandler gates mempool entry and deliver-time auth.
func StoreMigrateAnteHandler(ctx cosmos.Context, v semver.Version, k keeper.Keeper, msg MsgStoreMigrate) (cosmos.Context, error) {
	return validateStoreMigrateAuth(ctx, k, msg)
}
