package types

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type EventSuite struct{}

var _ = Suite(&EventSuite{})

func (s EventSuite) TestSwapEvent(c *C) {
	evt := NewEventSwap(
		common.ETHAsset,
		cosmos.NewUint(5),
		cosmos.NewUint(5),
		cosmos.NewUint(5),
		cosmos.ZeroUint(),
		GetRandomTx(),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
		cosmos.NewUint(5),
	)
	c.Check(evt.Type(), Equals, "swap")
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestAddLiqudityEvent(c *C) {
	evt := NewEventAddLiquidity(
		common.ETHAsset,
		cosmos.NewUint(5),
		GetRandomRUNEAddress(),
		cosmos.NewUint(5),
		cosmos.NewUint(5),
		GetRandomTxHash(),
		GetRandomTxHash(),
		GetRandomETHAddress(),
	)
	c.Check(evt.Type(), Equals, "add_liquidity")
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestWithdrawEvent(c *C) {
	evt := NewEventWithdraw(
		common.ETHAsset,
		cosmos.NewUint(6),
		5000,
		cosmos.NewDec(0),
		GetRandomTx(),
		cosmos.NewUint(100),
		cosmos.NewUint(100),
	)
	c.Check(evt.Type(), Equals, "withdraw")
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestPool(c *C) {
	evt := NewEventPool(common.ETHAsset, PoolStatus_Available)
	c.Check(evt.Type(), Equals, "pool")
	c.Check(evt.Pool.String(), Equals, common.ETHAsset.String())
	c.Check(evt.Status.String(), Equals, PoolStatus_Available.String())
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestReward(c *C) {
	evt := NewEventRewards(
		cosmos.NewUint(300),
		[]PoolAmt{{common.ETHAsset, 30}, {common.BTCAsset, 40}},
		cosmos.NewUint(50),
		cosmos.NewUint(60),
		cosmos.NewUint(70),
		cosmos.NewUint(80),
		cosmos.NewUint(90),
	)
	c.Check(evt.Type(), Equals, "rewards")
	c.Check(evt.BondReward.String(), Equals, "300")
	c.Assert(evt.PoolRewards, HasLen, 2)
	c.Check(evt.PoolRewards[0].Asset.Equals(common.ETHAsset), Equals, true)
	c.Check(evt.PoolRewards[0].Amount, Equals, int64(30))
	c.Check(evt.PoolRewards[1].Asset.Equals(common.BTCAsset), Equals, true)
	c.Check(evt.PoolRewards[1].Amount, Equals, int64(40))
	c.Check(evt.DevFundReward.String(), Equals, "50")
	c.Check(evt.IncomeBurn.String(), Equals, "60")
	c.Check(evt.TcyStakeReward.String(), Equals, "70")
	c.Check(evt.MarketingFundReward.String(), Equals, "80")
	c.Check(evt.PolReserveReward.String(), Equals, "90")
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestSlash(c *C) {
	evt := NewEventSlash(common.ETHAsset, []PoolAmt{
		{common.ETHAsset, -20},
		{common.RuneAsset(), 30},
	})
	c.Check(evt.Type(), Equals, "slash")
	c.Check(evt.Pool, Equals, common.ETHAsset)
	c.Assert(evt.SlashAmount, HasLen, 2)
	c.Check(evt.SlashAmount[0].Asset, Equals, common.ETHAsset)
	c.Check(evt.SlashAmount[0].Amount, Equals, int64(-20))
	c.Check(evt.SlashAmount[1].Asset, Equals, common.RuneAsset())
	c.Check(evt.SlashAmount[1].Amount, Equals, int64(30))
	events, err := evt.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestEventGas(c *C) {
	eg := NewEventGas()
	c.Assert(eg, NotNil)
	eg.UpsertGasPool(GasPool{
		Asset:    common.ETHAsset,
		AssetAmt: cosmos.NewUint(1000),
		RuneAmt:  cosmos.ZeroUint(),
	})
	c.Assert(eg.Pools, HasLen, 1)
	c.Assert(eg.Pools[0].Asset, Equals, common.ETHAsset)
	c.Assert(eg.Pools[0].RuneAmt.Equal(cosmos.ZeroUint()), Equals, true)
	c.Assert(eg.Pools[0].AssetAmt.Equal(cosmos.NewUint(1000)), Equals, true)

	eg.UpsertGasPool(GasPool{
		Asset:    common.ETHAsset,
		AssetAmt: cosmos.NewUint(1234),
		RuneAmt:  cosmos.NewUint(1024),
	})
	c.Assert(eg.Pools, HasLen, 1)
	c.Assert(eg.Pools[0].Asset, Equals, common.ETHAsset)
	c.Assert(eg.Pools[0].RuneAmt.Equal(cosmos.NewUint(1024)), Equals, true)
	c.Assert(eg.Pools[0].AssetAmt.Equal(cosmos.NewUint(2234)), Equals, true)

	eg.UpsertGasPool(GasPool{
		Asset:    common.BTCAsset,
		AssetAmt: cosmos.NewUint(1024),
		RuneAmt:  cosmos.ZeroUint(),
	})
	c.Assert(eg.Pools, HasLen, 2)
	c.Assert(eg.Pools[1].Asset, Equals, common.BTCAsset)
	c.Assert(eg.Pools[1].AssetAmt.Equal(cosmos.NewUint(1024)), Equals, true)
	c.Assert(eg.Pools[1].RuneAmt.Equal(cosmos.ZeroUint()), Equals, true)

	eg.UpsertGasPool(GasPool{
		Asset:    common.BTCAsset,
		AssetAmt: cosmos.ZeroUint(),
		RuneAmt:  cosmos.ZeroUint(),
	})

	c.Assert(eg.Pools, HasLen, 2)
	c.Assert(eg.Pools[1].Asset, Equals, common.BTCAsset)
	c.Assert(eg.Pools[1].AssetAmt.Equal(cosmos.NewUint(1024)), Equals, true)
	c.Assert(eg.Pools[1].RuneAmt.Equal(cosmos.ZeroUint()), Equals, true)

	eg.UpsertGasPool(GasPool{
		Asset:    common.BTCAsset,
		AssetAmt: cosmos.ZeroUint(),
		RuneAmt:  cosmos.NewUint(3333),
	})

	c.Assert(eg.Pools, HasLen, 2)
	c.Assert(eg.Pools[1].Asset, Equals, common.BTCAsset)
	c.Assert(eg.Pools[1].AssetAmt.Equal(cosmos.NewUint(1024)), Equals, true)
	c.Assert(eg.Pools[1].RuneAmt.Equal(cosmos.NewUint(3333)), Equals, true)
	events, err := eg.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (s EventSuite) TestEventFee(c *C) {
	event := NewEventFee(GetRandomTxHash(), common.Fee{
		Coins: common.Coins{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(1024)),
		},
		PoolDeduct: cosmos.NewUint(1023),
	}, cosmos.NewUint(5))
	c.Assert(event.Type(), Equals, FeeEventType)
	evts, err := event.Events()
	c.Assert(err, IsNil)
	c.Assert(evts, HasLen, 1)
}

func (s EventSuite) TestEventDonate(c *C) {
	e := NewEventDonate(common.ETHAsset, GetRandomTx())
	c.Check(e.Type(), Equals, "donate")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventRefund(c *C) {
	e := NewEventRefund(1, "refund", GetRandomTx(), common.NewFee(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100)),
	}, cosmos.ZeroUint()))
	c.Check(e.Type(), Equals, "refund")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventBond(c *C) {
	e := NewEventBond(cosmos.NewUint(100), BondType_bond_paid, GetRandomTx(), &NodeAccount{}, nil)
	c.Check(e.Type(), Equals, "bond")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventReBond(c *C) {
	tx := GetRandomTx()
	amount := cosmos.NewUint(68468161)
	nodeAddress := GetRandomValidatorNode(NodeStatus_Active)
	oldAddress := GetRandomBech32Addr()
	newAddress := GetRandomBech32Addr()

	e := NewEventReBond(amount, tx, &nodeAddress, oldAddress, newAddress)
	c.Check(e.Type(), Equals, "rebond")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventReserve(c *C) {
	e := NewEventReserve(ReserveContributor{
		Address: GetRandomETHAddress(),
		Amount:  cosmos.NewUint(100),
	}, GetRandomTx())
	c.Check(e.Type(), Equals, "reserve")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventErrata(c *C) {
	e := NewEventErrata(GetRandomTxHash(), PoolMods{
		NewPoolMod(common.ETHAsset, cosmos.NewUint(100), true, cosmos.NewUint(200), true),
	})
	c.Check(e.Type(), Equals, "errata")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventOutbound(c *C) {
	e := NewEventOutbound(GetRandomTxHash(), GetRandomTx())
	c.Check(e.Type(), Equals, "outbound")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventSlashPoint(c *C) {
	e := NewEventSlashPoint(GetRandomBech32Addr(), 100, "what ever")
	c.Check(e.Type(), Equals, "slash_points")
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventPoolStageCost(c *C) {
	e := NewEventPoolBalanceChanged(NewPoolMod(common.BTCAsset, cosmos.NewUint(100), false, cosmos.ZeroUint(), false), "test")
	c.Check(e.Type(), Equals, PoolBalanceChangeEventType)
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
}

func (EventSuite) TestEventLimitSwapClose(c *C) {
	// Test basic EventLimitSwapClose creation and properties
	txID := GetRandomTxHash()
	reason := "limit swap expired"
	blockHeight := int64(12345)

	e := NewEventLimitSwapClose(txID, reason, blockHeight)

	// Test event type
	c.Check(e.Type(), Equals, LimitSwapCloseEventType)
	c.Check(e.Type(), Equals, "limit_swap_close")

	// Test event properties
	c.Check(e.TxID.Equals(txID), Equals, true)
	c.Check(e.Reason, Equals, reason)
	c.Check(e.BlockHeight, Equals, blockHeight)

	// Test Events() method
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Check(events, NotNil)
	c.Assert(len(events), Equals, 1)

	event := events[0]
	c.Check(event.Type, Equals, "limit_swap_close")

	// Verify attributes
	attributeMap := make(map[string]string)
	for _, attr := range event.Attributes {
		attributeMap[attr.Key] = attr.Value
	}

	c.Check(attributeMap["txid"], Equals, txID.String())
	c.Check(attributeMap["reason"], Equals, reason)
	c.Check(attributeMap["block_height"], Equals, "12345")
}

func (EventSuite) TestEventLimitSwapCloseWithDifferentReasons(c *C) {
	// Test EventLimitSwapClose with different closure reasons
	testCases := []struct {
		reason string
		txID   common.TxID
		height int64
	}{
		{"limit swap completed", GetRandomTxHash(), 100},
		{"limit swap expired", GetRandomTxHash(), 200},
		{"limit swap cancelled", GetRandomTxHash(), 300},
		{"limit swap failed", GetRandomTxHash(), 400},
		{"streaming limit swap completed", GetRandomTxHash(), 500},
		{"", GetRandomTxHash(), 600}, // Empty reason
	}

	for i, testCase := range testCases {
		e := NewEventLimitSwapClose(testCase.txID, testCase.reason, testCase.height)

		// Test event type is consistent
		c.Check(e.Type(), Equals, LimitSwapCloseEventType, Commentf("Test case %d", i))

		// Test properties are correctly set
		c.Check(e.TxID.Equals(testCase.txID), Equals, true, Commentf("Test case %d: txID mismatch", i))
		c.Check(e.Reason, Equals, testCase.reason, Commentf("Test case %d: reason mismatch", i))
		c.Check(e.BlockHeight, Equals, testCase.height, Commentf("Test case %d: height mismatch", i))

		// Test Events() works for all cases
		events, err := e.Events()
		c.Check(err, IsNil, Commentf("Test case %d: Events() failed", i))
		c.Check(len(events), Equals, 1, Commentf("Test case %d: expected 1 event", i))

		event := events[0]
		c.Check(event.Type, Equals, "limit_swap_close", Commentf("Test case %d: wrong event type", i))
	}
}

func (EventSuite) TestEventLimitSwapCloseEdgeCases(c *C) {
	// Test edge cases for EventLimitSwapClose

	// Test with zero block height
	e1 := NewEventLimitSwapClose(GetRandomTxHash(), "test reason", 0)
	c.Check(e1.BlockHeight, Equals, int64(0))
	events1, err1 := e1.Events()
	c.Check(err1, IsNil)
	c.Check(len(events1), Equals, 1)

	// Test with negative block height
	e2 := NewEventLimitSwapClose(GetRandomTxHash(), "test reason", -100)
	c.Check(e2.BlockHeight, Equals, int64(-100))
	events2, err2 := e2.Events()
	c.Check(err2, IsNil)
	c.Check(len(events2), Equals, 1)

	// Test with very large block height
	largeHeight := int64(999999999999)
	e3 := NewEventLimitSwapClose(GetRandomTxHash(), "test reason", largeHeight)
	c.Check(e3.BlockHeight, Equals, largeHeight)
	events3, err3 := e3.Events()
	c.Check(err3, IsNil)
	c.Check(len(events3), Equals, 1)

	// Test with very long reason
	longReason := "This is a very long reason that contains many characters and should still work correctly when creating the event and converting to cosmos events"
	e4 := NewEventLimitSwapClose(GetRandomTxHash(), longReason, 12345)
	c.Check(e4.Reason, Equals, longReason)
	events4, err4 := e4.Events()
	c.Check(err4, IsNil)
	c.Check(len(events4), Equals, 1)
}

func (EventSuite) TestEventLimitSwapCloseComparison(c *C) {
	// Test that two EventLimitSwapClose with same data are equivalent
	txID := GetRandomTxHash()
	reason := "test reason"
	blockHeight := int64(12345)

	e1 := NewEventLimitSwapClose(txID, reason, blockHeight)
	e2 := NewEventLimitSwapClose(txID, reason, blockHeight)

	// Test that properties match
	c.Check(e1.TxID.Equals(e2.TxID), Equals, true)
	c.Check(e1.Reason, Equals, e2.Reason)
	c.Check(e1.BlockHeight, Equals, e2.BlockHeight)
	c.Check(e1.Type(), Equals, e2.Type())

	// Test that different txIDs create different events
	e3 := NewEventLimitSwapClose(GetRandomTxHash(), reason, blockHeight)
	c.Check(e1.TxID.Equals(e3.TxID), Equals, false)
	c.Check(e1.Reason, Equals, e3.Reason)           // Same reason
	c.Check(e1.BlockHeight, Equals, e3.BlockHeight) // Same height
}

func (EventSuite) TestEventLimitSwapCloseEventType(c *C) {
	// Test the event type constant
	c.Check(LimitSwapCloseEventType, Equals, "limit_swap_close")

	// Test that the event uses the correct constant
	e := NewEventLimitSwapClose(GetRandomTxHash(), "test", 100)
	c.Check(e.Type(), Equals, LimitSwapCloseEventType)

	// Test that the cosmos event also uses the correct type
	events, err := e.Events()
	c.Check(err, IsNil)
	c.Assert(len(events), Equals, 1)
	c.Check(events[0].Type, Equals, LimitSwapCloseEventType)
}
