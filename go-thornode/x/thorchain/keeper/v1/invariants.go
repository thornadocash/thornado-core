package keeperv1

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// InvariantRoutes return the keeper's invariant routes
func (k KVStore) InvariantRoutes() []common.InvariantRoute {
	return []common.InvariantRoute{
		common.NewInvariantRoute("asgard", AsgardInvariant(k)),
		common.NewInvariantRoute("bond", BondInvariant(k)),
		common.NewInvariantRoute("thorchain", THORChainInvariant(k)),
		common.NewInvariantRoute("affiliate_collector", AffilliateCollectorInvariant(k)),
		common.NewInvariantRoute("pools", PoolsInvariant(k)),
		common.NewInvariantRoute("streaming_swaps", StreamingSwapsInvariant(k)),
		common.NewInvariantRoute("runepool", RUNEPoolInvariant(k)),
	}
}

// AsgardInvariant the asgard module backs pool rune, savers synths, and native
// coins in queued swaps
func AsgardInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		// sum all rune liquidity on pools, including pending
		var poolCoins common.Coins
		pools, err := k.GetPools(ctx)
		if err != nil {
			return []string{fmt.Sprintf("fail to get pools: %s", err)}, true
		}
		for _, pool := range pools {
			switch {
			case pool.Asset.IsSyntheticAsset():
				coin := common.NewCoin(
					pool.Asset,
					pool.BalanceAsset,
				)
				poolCoins = poolCoins.Add(coin)
			case !pool.Asset.IsDerivedAsset():
				coin := common.NewCoin(
					common.RuneAsset(),
					pool.BalanceRune.Add(pool.PendingInboundRune),
				)
				poolCoins = poolCoins.Add(coin)

				if pool.Asset.IsTCY() {
					tcyCoin := common.NewCoin(
						common.TCY,
						pool.BalanceAsset.Add(pool.PendingInboundAsset),
					)
					poolCoins = poolCoins.Add(tcyCoin)
				}

				if pool.Asset.IsWhitelisted() {
					whitelistedCoin := common.NewCoin(
						pool.Asset,
						pool.BalanceAsset.Add(pool.PendingInboundAsset),
					)
					poolCoins = poolCoins.Add(whitelistedCoin)
				}
			}
		}

		processSwaps := func(ctx cosmos.Context, k KVStore, swapIter cosmos.Iterator) common.Coins { // Replace IteratorType with the actual iterator type
			var swapCoins common.Coins
			defer swapIter.Close()

			for ; swapIter.Valid(); swapIter.Next() {
				var swap MsgSwap
				k.Cdc().MustUnmarshal(swapIter.Value(), &swap)

				if len(swap.Tx.Coins) != 1 {
					broken = true
					msg = append(msg, fmt.Sprintf("wrong number of coins for swap: %d, %s", len(swap.Tx.Coins), swap.Tx.ID))
					continue
				}

				coin := swap.Tx.Coins[0]
				if !coin.IsNative() && !swap.TargetAsset.IsNative() {
					continue // only verifying native coins in this invariant
				}

				// adjust for streaming swaps
				ss := swap.GetStreamingSwap() // GetStreamingSwap() rather than var so In.IsZero() doesn't panic

				// A non-streaming affiliate swap and streaming main swap could have the same TxID,
				// so explicitly check IsLegacyStreaming to not double-count the main swap's In and Out amounts.
				if swap.IsLegacyStreaming() {
					var err error
					ss, err = k.GetStreamingSwap(ctx, swap.Tx.ID)
					if err != nil {
						ctx.Logger().Error("error getting streaming swap", "error", err)
						continue // should never happen
					}
				}

				// Trade Assets do not correspond to Module balance coins and panic on .Native(),
				// so do not include them in swapCoins.
				if coin.IsNative() && !coin.Asset.IsTradeAsset() && !coin.Asset.IsSecuredAsset() {
					if !ss.In.IsZero() { // legacy swap queue
						// adjust for stream swap amount, the amount In has been added
						// to the pool but not deducted from the tx or module, so deduct
						// that In amount from the tx coin
						coin.Amount = common.SafeSub(coin.Amount, ss.In)
					}
					if swap.State != nil && !swap.State.In.IsZero() { // advanced swap queue
						// adjust for stream swap amount, the amount In has been added
						// to the pool but not deducted from the tx or module, so deduct
						// that In amount from the tx coin
						coin.Amount = common.SafeSub(coin.Amount, swap.State.In)
					}
					swapCoins = swapCoins.Add(coin)
				}

				if swap.TargetAsset.IsNative() && !swap.TargetAsset.IsTradeAsset() && !swap.TargetAsset.IsSecuredAsset() {
					if !ss.Out.IsZero() {
						swapCoins = swapCoins.Add(common.NewCoin(swap.TargetAsset, ss.Out))
					}
					if swap.State != nil && !swap.State.Out.IsZero() {
						swapCoins = swapCoins.Add(common.NewCoin(swap.TargetAsset, swap.State.Out))
					}
				}
			}

			return swapCoins
		}

		// sum all rune in pending swaps
		swapCoins := processSwaps(ctx, k, k.GetSwapQueueIterator(ctx))
		advSwapCoins := processSwaps(ctx, k, k.GetAdvSwapQueueItemIterator(ctx))

		// get asgard module balance
		asgardAddr := k.GetModuleAccAddress(AsgardName)
		asgardCoins := k.GetBalance(ctx, asgardAddr)

		// asgard balance is expected to equal sum of pool and swap coins
		expNative, _ := poolCoins.Add(append(swapCoins, advSwapCoins...)...).Native()

		// note: coins must be sorted for SafeSub
		diffCoins, _ := asgardCoins.SafeSub(expNative.Sort()...)
		if !diffCoins.IsZero() {
			broken = true
			for _, coin := range diffCoins {
				if coin.IsPositive() {
					msg = append(msg, fmt.Sprintf("oversolvent: %s", coin))
				} else {
					coin.Amount = coin.Amount.Neg()
					msg = append(msg, fmt.Sprintf("insolvent: %s", coin))
				}
			}
		}

		return msg, broken
	}
}

// BondInvariant the bond module backs node bond and pending reward bond
func BondInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		// sum all rune bonded to nodes
		bondedRune := cosmos.ZeroUint()
		naIter := k.GetNodeAccountIterator(ctx)
		defer naIter.Close()
		for ; naIter.Valid(); naIter.Next() {
			var na NodeAccount
			k.Cdc().MustUnmarshal(naIter.Value(), &na)
			bondedRune = bondedRune.Add(na.Bond)
		}

		// get pending bond reward rune
		network, _ := k.GetNetwork(ctx)
		bondRewardRune := network.BondRewardRune

		// get rune balance of bond module
		bondModuleRune := k.GetBalanceOfModule(ctx, BondName, common.RuneAsset().Native())

		// bond module is expected to equal bonded rune and pending rewards
		expectedRune := bondedRune.Add(bondRewardRune)
		if expectedRune.GT(bondModuleRune) {
			broken = true
			diff := expectedRune.Sub(bondModuleRune)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			msg = append(msg, fmt.Sprintf("insolvent: %s", coin))

		} else if expectedRune.LT(bondModuleRune) {
			broken = true
			diff := bondModuleRune.Sub(expectedRune)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			msg = append(msg, fmt.Sprintf("oversolvent: %s", coin))
		}

		return msg, broken
	}
}

// THORChainInvariant the thorchain module should never hold a balance
func THORChainInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		// module balance of thorchain
		tcAddr := k.GetModuleAccAddress(ModuleName)
		tcCoins := k.GetBalance(ctx, tcAddr)

		// thorchain module should never carry a balance
		if !tcCoins.Empty() {
			broken = true
			for _, coin := range tcCoins {
				msg = append(msg, fmt.Sprintf("oversolvent: %s", coin))
			}
		}

		return msg, broken
	}
}

// AffilliateCollectorInvariant the affiliate_collector module backs accrued affiliate
// rewards
func AffilliateCollectorInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		affColModuleRune := k.GetBalanceOfModule(ctx, AffiliateCollectorName, common.RuneAsset().Native())
		affCols, err := k.GetAffiliateCollectors(ctx)
		if err != nil {
			if affColModuleRune.IsZero() {
				return nil, false
			}
			msg = append(msg, err.Error())
			return msg, true
		}

		totalAffRune := cosmos.ZeroUint()
		for _, ac := range affCols {
			totalAffRune = totalAffRune.Add(ac.RuneAmount)
		}

		if totalAffRune.GT(affColModuleRune) {
			broken = true
			diff := totalAffRune.Sub(affColModuleRune)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			msg = append(msg, fmt.Sprintf("insolvent: %s", coin))
		} else if totalAffRune.LT(affColModuleRune) {
			broken = true
			diff := affColModuleRune.Sub(totalAffRune)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			msg = append(msg, fmt.Sprintf("oversolvent: %s", coin))
		}

		return msg, broken
	}
}

// PoolsInvariant pool units and pending rune/asset should match the sum
// of units and pending rune/asset for all lps
func PoolsInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		pools, err := k.GetPools(ctx)
		if err != nil {
			return []string{fmt.Sprintf("fail to get pools: %s", err)}, true
		}
		for _, pool := range pools {
			if pool.Asset.IsNative() {
				continue // only looking at layer-one pools
			}

			lpUnits := cosmos.ZeroUint()
			lpPendingRune := cosmos.ZeroUint()
			lpPendingAsset := cosmos.ZeroUint()

			lpIter := k.GetLiquidityProviderIterator(ctx, pool.Asset)
			for ; lpIter.Valid(); lpIter.Next() {
				var lp LiquidityProvider
				k.Cdc().MustUnmarshal(lpIter.Value(), &lp)
				lpUnits = lpUnits.Add(lp.Units)
				lpPendingRune = lpPendingRune.Add(lp.PendingRune)
				lpPendingAsset = lpPendingAsset.Add(lp.PendingAsset)
			}
			_ = lpIter.Close()

			check := func(poolValue, lpValue cosmos.Uint, valueType string) {
				if poolValue.GT(lpValue) {
					diff := poolValue.Sub(lpValue)
					msg = append(msg, fmt.Sprintf("%s oversolvent: %s %s", pool.Asset, diff.String(), valueType))
					broken = true
				} else if poolValue.LT(lpValue) {
					diff := lpValue.Sub(poolValue)
					msg = append(msg, fmt.Sprintf("%s insolvent: %s %s", pool.Asset, diff.String(), valueType))
					broken = true
				}
			}

			check(pool.LPUnits, lpUnits, "units")
			check(pool.PendingInboundRune, lpPendingRune, "pending rune")
			check(pool.PendingInboundAsset, lpPendingAsset, "pending asset")
		}

		return msg, broken
	}
}

// StreamingSwapsInvariant every streaming swap should have a corresponding
// queued swap, stream deposit should equal the queued swap's source coin,
// and the stream should be internally consistent
func StreamingSwapsInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		// fetch all streaming/limit swaps from the advanced swap queue (V2)
		var v2StreamingSwaps []MsgSwap
		advSwapIter := k.GetAdvSwapQueueItemIterator(ctx)
		defer advSwapIter.Close()
		for ; advSwapIter.Valid(); advSwapIter.Next() {
			var swap MsgSwap
			k.Cdc().MustUnmarshal(advSwapIter.Value(), &swap)
			if swap.State != nil && swap.IsStreaming() {
				v2StreamingSwaps = append(v2StreamingSwaps, swap)
			}
		}

		// Validate V2 (advanced queue) streaming swaps using their embedded State
		for _, swap := range v2StreamingSwaps {
			if swap.State == nil {
				broken = true
				msg = append(msg, fmt.Sprintf("%s: advanced swap missing state", swap.Tx.ID.String()))
				continue
			}

			// Check that coin amount matches deposit in state
			if len(swap.Tx.Coins) == 0 {
				broken = true
				msg = append(msg, fmt.Sprintf("%s: advanced swap missing coins", swap.Tx.ID.String()))
				continue
			} else if !swap.Tx.Coins[0].Amount.Equal(swap.State.Deposit) {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: swap.coin %s != state.deposit %s",
					swap.Tx.ID.String(),
					swap.Tx.Coins[0].Amount,
					swap.State.Deposit.String()))
			}

			// Check count doesn't exceed quantity
			// For limit swaps, check successful count; for market streaming swaps, check total count
			var countToCheck uint64
			var countLabel string
			if swap.IsLimitSwap() {
				countToCheck = swap.SuccessCount()
				countLabel = "state.success_count"
			} else {
				countToCheck = swap.State.Count
				countLabel = "state.count"
			}

			if countToCheck > swap.State.Quantity {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: %s %d > state.quantity %d",
					swap.Tx.ID.String(),
					countLabel,
					countToCheck,
					swap.State.Quantity))
			}

			// Check In doesn't exceed Deposit
			if swap.State.In.GT(swap.State.Deposit) {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: state.in %s > state.deposit %s",
					swap.Tx.ID.String(),
					swap.State.In.String(),
					swap.State.Deposit.String()))
			}
		}

		// fetch all streaming swaps from the regular swap queue (V1)
		var v1StreamingSwaps []MsgSwap
		swapIter := k.GetSwapQueueIterator(ctx)
		defer swapIter.Close()
		for ; swapIter.Valid(); swapIter.Next() {
			var swap MsgSwap
			k.Cdc().MustUnmarshal(swapIter.Value(), &swap)
			// Skip limit swaps - they have interval=1 but are not streaming swaps
			if swap.IsLimitSwap() {
				continue
			}
			if swap.IsLegacyStreaming() {
				v1StreamingSwaps = append(v1StreamingSwaps, swap)
			}
		}

		// fetch all stream swap records (only used by V1)
		var streams []StreamingSwap
		ssIter := k.GetStreamingSwapIterator(ctx)
		defer ssIter.Close()
		for ; ssIter.Valid(); ssIter.Next() {
			var stream StreamingSwap
			k.Cdc().MustUnmarshal(ssIter.Value(), &stream)
			streams = append(streams, stream)
		}

		// Validate V1 streaming swaps against StreamingSwap records
		v1SwapMap := make(map[string]MsgSwap, len(v1StreamingSwaps))
		for _, swap := range v1StreamingSwaps {
			v1SwapMap[swap.Tx.ID.String()] = swap
		}

		for _, stream := range streams {
			swap, found := v1SwapMap[stream.TxID.String()]
			if !found {
				broken = true
				msg = append(msg, fmt.Sprintf("swap not found for stream: %s", stream.TxID.String()))
				continue
			}

			if len(swap.Tx.Coins) == 0 {
				broken = true
				msg = append(msg, fmt.Sprintf("%s: swap missing coins", stream.TxID.String()))
				continue
			} else if !swap.Tx.Coins[0].Amount.Equal(stream.Deposit) {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: swap.coin %s != stream.deposit %s",
					stream.TxID.String(),
					swap.Tx.Coins[0].Amount,
					stream.Deposit.String()))
			}
			if stream.Count > stream.Quantity {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: stream.count %d > stream.quantity %d",
					stream.TxID.String(),
					stream.Count,
					stream.Quantity))
			}
			if stream.In.GT(stream.Deposit) {
				broken = true
				msg = append(msg, fmt.Sprintf(
					"%s: stream.in %s > stream.deposit %s",
					stream.TxID.String(),
					stream.In.String(),
					stream.Deposit.String()))
			}
		}

		return msg, broken
	}
}

// RUNEPoolInvariant asserts that the RUNEPool units and provider units are consistent.
func RUNEPoolInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		runePool, err := k.GetRUNEPool(ctx)
		if err != nil {
			ctx.Logger().Error("error getting rune pool", "error", err)
			return []string{err.Error()}, true
		}

		providerUnits := cosmos.ZeroUint()
		providerDeposited := cosmos.ZeroUint()
		providerWithdrawn := cosmos.ZeroUint()
		iterator := k.GetRUNEProviderIterator(ctx)
		defer iterator.Close()
		for ; iterator.Valid(); iterator.Next() {
			var rp RUNEProvider
			k.Cdc().MustUnmarshal(iterator.Value(), &rp)
			if rp.RuneAddress.Empty() {
				continue
			}
			providerUnits = providerUnits.Add(rp.Units)
			providerDeposited = providerDeposited.Add(rp.DepositAmount)
			providerWithdrawn = providerWithdrawn.Add(rp.WithdrawAmount)
		}

		if !providerUnits.Equal(runePool.PoolUnits) {
			m := fmt.Sprintf(
				"pool units %s != provider units %s",
				runePool.PoolUnits, providerUnits,
			)
			msg = append(msg, m)
			broken = true
		}

		if !providerDeposited.Equal(runePool.RuneDeposited) {
			m := fmt.Sprintf(
				"rune deposited %s != provider rune deposited %s",
				runePool.RuneDeposited, providerDeposited,
			)
			msg = append(msg, m)
			broken = true
		}

		if !providerWithdrawn.Equal(runePool.RuneWithdrawn) {
			m := fmt.Sprintf(
				"rune withdrawn %s != provider rune withdrawn %s",
				runePool.RuneWithdrawn, providerWithdrawn,
			)
			msg = append(msg, m)
			broken = true
		}

		return msg, broken
	}
}
