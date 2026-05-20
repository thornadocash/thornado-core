package keeperv1

import (
	"fmt"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

type InvariantsSuite struct{}

var _ = Suite(&InvariantsSuite{})

func (s *InvariantsSuite) TestAsgardInvariant(c *C) {
	ctx, k := setupKeeperForTest(c)

	// empty the starting balance of asgard
	runeBal := k.GetRuneBalanceOfModule(ctx, AsgardName)
	coins := common.NewCoins(common.NewCoin(common.RuneAsset(), runeBal))
	c.Assert(k.SendFromModuleToModule(ctx, AsgardName, ReserveName, coins), IsNil)

	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(1000)
	pool.PendingInboundRune = cosmos.NewUint(100)
	c.Assert(k.SetPool(ctx, pool), IsNil)

	// derived asset pools are not included in expectations
	pool = NewPool()
	pool.Asset = common.BTCAsset.GetDerivedAsset()
	pool.BalanceRune = cosmos.NewUint(666)
	pool.PendingInboundRune = cosmos.NewUint(777)
	c.Assert(k.SetPool(ctx, pool), IsNil)

	// savers pools are not included in expectations
	pool = NewPool()
	pool.Asset = common.BTCAsset.GetSyntheticAsset()
	pool.BalanceRune = cosmos.NewUint(666)
	pool.PendingInboundRune = cosmos.NewUint(777)
	c.Assert(k.SetPool(ctx, pool), IsNil)

	swapMsg := MsgSwap{
		Tx: GetRandomTx(),
	}
	swapMsg.Tx.Coins = common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(2000)))
	c.Assert(k.SetSwapQueueItem(ctx, swapMsg, 0), IsNil)

	// synth swaps are ignored
	swapMsg.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset.GetSyntheticAsset(), cosmos.NewUint(666)))
	c.Assert(k.SetSwapQueueItem(ctx, swapMsg, 1), IsNil)

	// layer1 swaps are ignored
	swapMsg.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(777)))
	c.Assert(k.SetSwapQueueItem(ctx, swapMsg, 2), IsNil)

	invariant := AsgardInvariant(k)

	msg, broken := invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 2)
	c.Assert(msg[0], Equals, "insolvent: 666btc/btc")
	c.Assert(msg[1], Equals, "insolvent: 3100rune")

	// send the expected amount to asgard
	expCoins := common.NewCoins(
		common.NewCoin(common.BTCAsset.GetSyntheticAsset(), cosmos.NewUint(666)),
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(3100)),
	)
	for _, coin := range expCoins {
		c.Assert(k.MintToModule(ctx, ModuleName, coin), IsNil)
	}
	c.Assert(k.SendFromModuleToModule(ctx, ModuleName, AsgardName, expCoins), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// send a little more to make asgard oversolvent
	extraCoins := common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(1)))
	c.Assert(k.SendFromModuleToModule(ctx, ReserveName, AsgardName, extraCoins), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, "oversolvent: 1rune")
}

func (s *InvariantsSuite) TestBondInvariant(c *C) {
	ctx, k := setupKeeperForTest(c)

	node := GetRandomValidatorNode(NodeActive)
	node.Bond = cosmos.NewUint(1000)
	c.Assert(k.SetNodeAccount(ctx, node), IsNil)

	node = GetRandomValidatorNode(NodeActive)
	node.Bond = cosmos.NewUint(100)
	c.Assert(k.SetNodeAccount(ctx, node), IsNil)

	network := NewNetwork()
	network.BondRewardRune = cosmos.NewUint(2000)
	c.Assert(k.SetNetwork(ctx, network), IsNil)

	invariant := BondInvariant(k)

	msg, broken := invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, "insolvent: 3100rune")

	expRune := common.NewCoin(common.RuneAsset(), cosmos.NewUint(3100))
	c.Assert(k.MintToModule(ctx, ModuleName, expRune), IsNil)
	c.Assert(k.SendFromModuleToModule(ctx, ModuleName, BondName, common.NewCoins(expRune)), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// send more to make bond oversolvent
	c.Assert(k.MintToModule(ctx, ModuleName, expRune), IsNil)
	c.Assert(k.SendFromModuleToModule(ctx, ModuleName, BondName, common.NewCoins(expRune)), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, "oversolvent: 3100rune")
}

func (s *InvariantsSuite) TestTHORChainInvariant(c *C) {
	ctx, k := setupKeeperForTest(c)

	invariant := THORChainInvariant(k)

	// should pass since it has no coins
	msg, broken := invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// send some coins to make it oversolvent
	coins := common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(1)))
	c.Assert(k.SendFromModuleToModule(ctx, AsgardName, ModuleName, coins), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, "oversolvent: 1rune")
}

func (s *InvariantsSuite) TestStreamingSwapsInvariant(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Happy path: V1 streaming swap exists and matches stream record
	tx := GetRandomTx()
	tx.Coins = common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(1000)))
	swap := MsgSwap{
		Tx:             tx,
		TargetAsset:    common.BTCAsset,
		StreamInterval: 1,                    // makes it a legacy streaming swap
		Version:        types.SwapVersion_v1, // V1 required for IsLegacyStreaming()
	}
	c.Assert(k.SetSwapQueueItem(ctx, swap, 0), IsNil)

	stream := StreamingSwap{
		TxID:     tx.ID,
		Deposit:  cosmos.NewUint(1000),
		Count:    0,
		Quantity: 10,
		Interval: 10, // Required: must be >= 1 for Valid() to pass
		In:       cosmos.ZeroUint(),
	}
	k.SetStreamingSwap(ctx, stream)

	invariant := StreamingSwapsInvariant(k)
	msg, broken := invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// Broken path: Stream exists but swap missing
	k.RemoveSwapQueueItem(ctx, tx.ID, 0)
	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, fmt.Sprintf("swap not found for stream: %s", tx.ID))

	// Broken path: Swap mismatch deposit
	c.Assert(k.SetSwapQueueItem(ctx, swap, 0), IsNil)
	stream.Deposit = cosmos.NewUint(999)
	k.SetStreamingSwap(ctx, stream)
	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, fmt.Sprintf("%s: swap.coin 1000 != stream.deposit 999", tx.ID))

	// Fix the state to be valid again.
	c.Assert(k.SetSwapQueueItem(ctx, swap, 0), IsNil) // Ensure swap exists
	stream.Deposit = cosmos.NewUint(1000)             // Fix deposit
	k.SetStreamingSwap(ctx, stream)                   // Save fixed stream
	// Verify it's fixed
	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// V2 Market Streaming Swap (Happy Path)
	v2Tx := GetRandomTx()
	v2Tx.Coins = common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(1000)))
	v2Swap := MsgSwap{
		Tx:          v2Tx,
		TargetAsset: common.BTCAsset,
		TradeTarget: cosmos.ZeroUint(),
		SwapType:    types.SwapType_market,
		State: &types.SwapState{
			Quantity: 5,
			Count:    2,
			Deposit:  cosmos.NewUint(1000),
			In:       cosmos.ZeroUint(),
		},
	}
	c.Assert(k.SetAdvSwapQueueItem(ctx, v2Swap), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// Limit Swap test
	limitTx := GetRandomTx()
	limitTx.Coins = common.NewCoins(common.NewCoin(common.RuneAsset(), cosmos.NewUint(1000)))
	limitSwap := MsgSwap{
		Tx:          limitTx,
		TargetAsset: common.BTCAsset,
		TradeTarget: cosmos.ZeroUint(), // required for getRatio
		SwapType:    types.SwapType_limit,
		State: &types.SwapState{
			Quantity:    2,
			Count:       4, // 4 attempts total
			Deposit:     cosmos.NewUint(1000),
			In:          cosmos.ZeroUint(),
			FailedSwaps: []uint64{1, 2}, // 2 failed attempts
		},
	}
	// SuccessCount = 4 - 2 = 2. Quantity = 2. 2 <= 2. Valid.

	// Add to advanced swap queue (V2)
	c.Assert(k.SetAdvSwapQueueItem(ctx, limitSwap), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, false)
	c.Assert(msg, IsNil)

	// Broken path: Limit swap success count exceeds quantity
	limitSwap.State.Count = 5                    // 5 attempts total
	limitSwap.State.FailedSwaps = []uint64{1, 2} // 2 failed -> 3 successes > 2 Quantity
	c.Assert(k.SetAdvSwapQueueItem(ctx, limitSwap), IsNil)

	msg, broken = invariant(ctx)
	c.Assert(broken, Equals, true)
	c.Assert(len(msg), Equals, 1)
	c.Assert(msg[0], Equals, fmt.Sprintf("%s: state.success_count 3 > state.quantity 2", limitTx.ID))
}
