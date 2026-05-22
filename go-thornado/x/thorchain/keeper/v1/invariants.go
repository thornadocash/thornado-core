package keeperv1

import (
	"fmt"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// InvariantRoutes returns the keeper's remaining invariant routes.
func (k KVStore) InvariantRoutes() []common.InvariantRoute {
	return []common.InvariantRoute{
		common.NewInvariantRoute("bond", BondInvariant(k)),
		common.NewInvariantRoute("thorchain", THORChainInvariant(k)),
		common.NewInvariantRoute("streaming_swaps", StreamingSwapsInvariant(k)),
	}
}

func BondInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		bondModule := k.GetBalanceOfModule(ctx, BondName, common.RuneAsset().Native())
		var totalBond cosmos.Uint

		iterator := k.GetNodeAccountIterator(ctx)
		defer iterator.Close()
		for ; iterator.Valid(); iterator.Next() {
			var na NodeAccount
			k.Cdc().MustUnmarshal(iterator.Value(), &na)
			totalBond = totalBond.Add(na.Bond)
		}

		if totalBond.GT(bondModule) {
			diff := totalBond.Sub(bondModule)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			return []string{fmt.Sprintf("insolvent: %s", coin)}, true
		}
		if totalBond.LT(bondModule) {
			diff := bondModule.Sub(totalBond)
			coin, _ := common.NewCoin(common.RuneAsset(), diff).Native()
			return []string{fmt.Sprintf("oversolvent: %s", coin)}, true
		}

		return nil, false
	}
}

func THORChainInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		tcAddr := k.GetModuleAccAddress(ModuleName)
		tcCoins := k.GetBalance(ctx, tcAddr)
		if tcCoins.Empty() {
			return nil, false
		}
		for _, coin := range tcCoins {
			msg = append(msg, fmt.Sprintf("oversolvent: %s", coin))
		}
		return msg, true
	}
}

func StreamingSwapsInvariant(k KVStore) common.Invariant {
	return func(ctx cosmos.Context) (msg []string, broken bool) {
		iterator := k.GetStreamingSwapIterator(ctx)
		defer iterator.Close()
		for ; iterator.Valid(); iterator.Next() {
			var stream StreamingSwap
			k.Cdc().MustUnmarshal(iterator.Value(), &stream)
			if stream.Valid() != nil {
				msg = append(msg, fmt.Sprintf("invalid streaming swap: %s", stream.TxID))
				broken = true
			}
			if stream.Count > stream.Quantity {
				msg = append(msg, fmt.Sprintf("%s: stream.count %d > stream.quantity %d", stream.TxID, stream.Count, stream.Quantity))
				broken = true
			}
			if stream.In.GT(stream.Deposit) {
				msg = append(msg, fmt.Sprintf("%s: stream.in %s > stream.deposit %s", stream.TxID, stream.In, stream.Deposit))
				broken = true
			}
		}
		return msg, broken
	}
}
