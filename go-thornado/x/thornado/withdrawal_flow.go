package thornado

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/keeper"
	"github.com/thornadocash/go-thornado/x/thornado/types"
)

func QueueAuthorizedWithdrawalTxOut(ctx cosmos.Context, k keeper.Keeper, authorization types.ShielderRedeem) (types.ShielderRedeem, error) {
	if err := authorization.Valid(); err != nil {
		return types.ShielderRedeem{}, err
	}
	if authorization.Status != types.ShielderRedeemStatusAuthorized {
		return types.ShielderRedeem{}, fmt.Errorf("shielder redeem is not authorized")
	}
	feeSats := withdrawalFeeSats(ctx, k, authorization.AmountSats)
	if feeSats >= authorization.AmountSats {
		return types.ShielderRedeem{}, fmt.Errorf("withdrawal fee exceeds amount")
	}
	authorization.FeeSats = feeSats

	amount := authorization.AmountSats - authorization.FeeSats
	gasRate, err := btcGasRateFromKeeper(ctx, k)
	if err != nil {
		return types.ShielderRedeem{}, err
	}
	item := TxOutItem{
		Chain:       common.BTCChain,
		ToAddress:   authorization.Recipient,
		VaultPubKey: authorization.VaultPubKey,
		Coin:        common.NewCoin(common.BTCAsset, cosmos.NewUint(amount)),
		GasRate:     gasRate,
		InHash:      authorization.InHash,
		ModuleName:  BaseName,
		TxType:      types.TxOutTypeOut,
	}
	if err := appendBTCExactTxOut(ctx, k, ctx.BlockHeight(), item); err != nil {
		return types.ShielderRedeem{}, err
	}
	if err := addWithdrawalFee(ctx, k, authorization.FeeSats); err != nil {
		return types.ShielderRedeem{}, err
	}
	authorization.Status = types.DepositStatusKeysignQueued
	if err := k.SetShielderRedeem(ctx, authorization); err != nil {
		return types.ShielderRedeem{}, err
	}
	return authorization, nil
}
