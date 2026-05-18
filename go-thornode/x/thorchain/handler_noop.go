package thorchain

import (
	"fmt"
	"strings"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// NoOpHandler is to handle no-op messages (e.g. vault balance adjustments)
type NoOpHandler struct {
	mgr Manager
}

// NewNoOpHandler create a new instance of NoOpHandler
func NewNoOpHandler(mgr Manager) NoOpHandler {
	return NoOpHandler{
		mgr: mgr,
	}
}

// Run is the main entry point to execute no-op logic
func (h NoOpHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgNoOp)
	if !ok {
		return nil, errInvalidMessage
	}
	ctx.Logger().Info("receive msg no op", "tx_id", msg.ObservedTx.Tx.ID)
	if err := h.validate(ctx, *msg); err != nil {
		ctx.Logger().Error("msg no op failed validation", "error", err)
		return nil, err
	}

	if err := h.handle(ctx, *msg); err != nil {
		ctx.Logger().Error("fail to process msg noop", "error", err)
		return nil, err
	}
	return &cosmos.Result{}, nil
}

func (h NoOpHandler) validate(ctx cosmos.Context, msg MsgNoOp) error {
	return msg.ValidateBasic()
}

// handle process MsgNoOp
// For "novault" action, it subtracts coins from the vault balance (e.g. to correct for already-observed funds)
func (h NoOpHandler) handle(ctx cosmos.Context, msg MsgNoOp) error {
	action := msg.GetAction()
	if len(action) == 0 {
		return nil
	}
	if !strings.EqualFold(action, "novault") {
		return nil
	}

	// Get the vault and verify it exists
	vault, err := h.mgr.Keeper().GetVault(ctx, msg.ObservedTx.ObservedPubKey)
	if err != nil {
		return fmt.Errorf("fail to get vault: %w", err)
	}

	// Verify vault is not empty (exists)
	if vault.IsEmpty() {
		return fmt.Errorf("vault does not exist: %s", msg.ObservedTx.ObservedPubKey)
	}

	// Verify vault status is active or retiring - only these vaults should have funds modified
	if !vault.IsActive() && !vault.IsRetiring() {
		return fmt.Errorf("vault is not active or retiring: %s (status: %s)", msg.ObservedTx.ObservedPubKey, vault.Status)
	}

	// Verify the vault has sufficient funds for each coin being subtracted
	for _, coin := range msg.ObservedTx.Tx.Coins {
		vaultCoin := vault.GetCoin(coin.Asset)
		if vaultCoin.Amount.LT(coin.Amount) {
			return fmt.Errorf("vault has insufficient funds for %s: has %s, attempting to subtract %s",
				coin.Asset, vaultCoin.Amount, coin.Amount)
		}
	}

	// Subtract the coins from vault, as it has been added to
	vault.SubFunds(msg.ObservedTx.Tx.Coins)
	if err := h.mgr.Keeper().SetVault(ctx, vault); err != nil {
		return fmt.Errorf("fail to save vault: %w", err)
	}
	return nil
}
