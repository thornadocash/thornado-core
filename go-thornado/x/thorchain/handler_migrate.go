package thorchain

import (
	"context"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/hashicorp/go-metrics"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
)

// MigrateHandler is a handler to process MsgMigrate
type MigrateHandler struct {
	mgr Manager
}

// NewMigrateHandler create a new instance of MigrateHandler
func NewMigrateHandler(mgr Manager) MigrateHandler {
	return MigrateHandler{
		mgr: mgr,
	}
}

// Run is the main entry point of Migrate handler
func (h MigrateHandler) Run(ctx cosmos.Context, m cosmos.Msg) (*cosmos.Result, error) {
	msg, ok := m.(*MsgMigrate)
	if !ok {
		return nil, errInvalidMessage
	}
	if err := h.validate(ctx, *msg); err != nil {
		return nil, err
	}
	return h.handle(ctx, *msg)
}

func (h MigrateHandler) validate(ctx cosmos.Context, msg MsgMigrate) error {
	if err := msg.ValidateBasic(); nil != err {
		return err
	}
	return nil
}

func (h MigrateHandler) handle(ctx cosmos.Context, msg MsgMigrate) (*cosmos.Result, error) {
	ctx.Logger().Info("receive MsgMigrate", "request tx hash", msg.Tx.Tx.ID)
	// update txOut record with our TxID that sent funds out of the pool
	txOut, err := h.mgr.Keeper().GetTxOut(ctx, msg.BlockHeight)
	if err != nil {
		ctx.Logger().Error("unable to get txOut record", "error", err)
		return nil, cosmos.ErrUnknownRequest(err.Error())
	}

	migTx := msg.Tx.Tx

	shouldSlash := true
	for i, tx := range txOut.TxArray {
		if !migTx.Chain.Equals(tx.Chain) {
			continue
		}
		// migrate is the memo used by thorchain to identify fund migration between asgard vault.
		// it use migrate:{block height} to mark a tx out caused by vault rotation
		// this type of tx out is special , because it doesn't have relevant tx in to trigger it, it is trigger by thorchain itself.
		var fromAddress common.Address
		var addrErr error
		switch tx.Chain.GetSigningAlgo() {
		case common.SigningAlgoSecp256k1:
			fromAddress, addrErr = tx.VaultPubKey.GetAddress(tx.Chain)
		case common.SigningAlgoEd25519:
			fromAddress, addrErr = tx.VaultPubKeyEddsa.GetAddress(tx.Chain)
		default:
			ctx.Logger().Error("unknown signing algo", "signing_algo", tx.Chain.GetSigningAlgo())
			continue
		}
		if addrErr != nil {
			ctx.Logger().Error("fail to get address from pubkey", "chain", tx.Chain, "error", addrErr)
			continue
		}

		if tx.InHash.Equals(common.BlankTxID) &&
			tx.OutHash.IsEmpty() &&
			tx.ToAddress.Equals(migTx.ToAddress) &&
			fromAddress.Equals(migTx.FromAddress) {

			matchCoin := migTx.Coins.Contains(tx.Coin)
			// when outbound is gas asset
			if !matchCoin && tx.Coin.Asset.Equals(tx.Chain.GetGasAsset()) {
				asset := tx.Chain.GetGasAsset()
				intendToSpend := tx.Coin.Amount.Add(tx.MaxGas.ToCoins().GetCoin(asset).Amount)
				actualSpend := migTx.Coins.GetCoin(asset).Amount.Add(migTx.Gas.ToCoins().GetCoin(asset).Amount)
				if intendToSpend.Equal(actualSpend) {
					maxGasAmt := tx.MaxGas.ToCoins().GetCoin(asset).Amount
					realGasAmt := migTx.Gas.ToCoins().GetCoin(asset).Amount
					if maxGasAmt.GTE(realGasAmt) {
						ctx.Logger().Info("override match coin", "intend to spend", intendToSpend, "actual spend", actualSpend)
						matchCoin = true
					}
					// although here might detect there some some discrepancy between MaxGas , and actual gas
					// but migrate is internal tx , asset didn't leave the network , thus doesn't need to update pool
				}
			}
			if !matchCoin {
				continue
			}
			txOut.TxArray[i].OutHash = migTx.ID
			shouldSlash = false

			if err = h.mgr.Keeper().SetTxOut(ctx, txOut); nil != err {
				return nil, ErrInternal(err, "fail to save tx out")
			}
			break
		}
	}

	if shouldSlash {
		ctx.Logger().Info("slash node account,migration has no matched txout", "outbound tx", msg.Tx.Tx)
		if err = h.slash(ctx, msg.Tx); err != nil {
			return nil, ErrInternal(err, "fail to slash account")
		}
	}

	if err = h.mgr.Keeper().SetLastSignedHeight(ctx, msg.BlockHeight); err != nil {
		ctx.Logger().Info("fail to update last signed height", "error", err)
	}

	return &cosmos.Result{}, nil
}

func (h MigrateHandler) slash(ctx cosmos.Context, tx ObservedTx) error {
	toSlash := make(common.Coins, len(tx.Tx.Coins))
	copy(toSlash, tx.Tx.Coins)
	toSlash = toSlash.Add(tx.Tx.Gas.ToCoins()...)

	ctx = ctx.WithContext(context.WithValue(ctx.Context(), constants.CtxMetricLabels, []metrics.Label{
		telemetry.NewLabel("reason", "failed_migration"),
		telemetry.NewLabel("chain", string(tx.Tx.Chain)),
	}))

	return h.mgr.Slasher().SlashVault(ctx, tx.ObservedPubKey, toSlash, h.mgr)
}
