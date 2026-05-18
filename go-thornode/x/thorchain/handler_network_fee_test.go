package thorchain

import (
	"errors"
	"fmt"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type HandlerObserveNetworkFeeSuite struct{}

var _ = Suite(&HandlerObserveNetworkFeeSuite{})

type KeeperObserveNetworkFeeTest struct {
	keeper.Keeper
	errFailListActiveNodeAccount   bool
	errFailGetObservedNetworkVoter bool
	errFailSaveNetworkFee          bool
}

func (k KeeperObserveNetworkFeeTest) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.errFailListActiveNodeAccount {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k KeeperObserveNetworkFeeTest) GetObservedNetworkFeeVoter(ctx cosmos.Context, height int64, chain common.Chain, rate, size int64) (ObservedNetworkFeeVoter, error) {
	if k.errFailGetObservedNetworkVoter {
		return ObservedNetworkFeeVoter{}, errKaboom
	}
	return k.Keeper.GetObservedNetworkFeeVoter(ctx, height, chain, rate, size)
}

func (k KeeperObserveNetworkFeeTest) SaveNetworkFee(ctx cosmos.Context, chain common.Chain, networkFee NetworkFee) error {
	if k.errFailSaveNetworkFee {
		return errKaboom
	}
	return k.Keeper.SaveNetworkFee(ctx, chain, networkFee)
}

func (h *HandlerObserveNetworkFeeSuite) TestHandlerObserveNetworkFee(c *C) {
	h.testHandlerObserveNetworkFeeWithVersion(c)
}

func (*HandlerObserveNetworkFeeSuite) testHandlerObserveNetworkFeeWithVersion(c *C) {
	ctx, keeper := setupKeeperForTest(c)
	activeNodeAccount := GetRandomValidatorNode(NodeActive)
	c.Assert(keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
	handler := NewNetworkFeeHandler(NewDummyMgrWithKeeper(keeper))
	msg := NewMsgNetworkFee(1024, common.ETHChain, 256, 100, activeNodeAccount.NodeAddress)
	result, err := handler.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// already signed not cause error
	result, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// already processed
	msg1 := NewMsgNetworkFee(1024, common.ETHChain, 256, 100, activeNodeAccount.NodeAddress)
	result, err = handler.Run(ctx, msg1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// fail list active node account should fail
	handler1 := NewNetworkFeeHandler(
		NewDummyMgrWithKeeper(KeeperObserveNetworkFeeTest{
			Keeper:                       keeper,
			errFailListActiveNodeAccount: true,
		}),
	)
	result, err = handler1.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, errInternal), Equals, true)

	// fail to get observed network fee voter should return an error
	handler2 := NewNetworkFeeHandler(
		NewDummyMgrWithKeeper(KeeperObserveNetworkFeeTest{
			Keeper:                         keeper,
			errFailGetObservedNetworkVoter: true,
		}),
	)
	result, err = handler2.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)

	// fail to save network fee should result in an error
	handler3 := NewNetworkFeeHandler(
		NewDummyMgrWithKeeper(KeeperObserveNetworkFeeTest{
			Keeper:                keeper,
			errFailSaveNetworkFee: true,
		}),
	)
	msg2 := NewMsgNetworkFee(2056, common.ETHChain, 200, 102, activeNodeAccount.NodeAddress)
	result, err = handler3.Run(ctx, msg2)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)

	// invalid message should return an error
	msg3 := NewMsgReserveContributor(GetRandomTx(), ReserveContributor{}, GetRandomBech32Addr())
	result, err = handler3.Run(ctx, msg3)
	c.Check(result, IsNil)
	c.Check(err, NotNil)
	c.Check(errors.Is(err, errInvalidMessage), Equals, true)
}

func (s *HandlerObserveNetworkFeeSuite) TestObservingSlashing(c *C) {
	ctx, mgr := setupManagerForTest(c)
	height := int64(1024)
	ctx = ctx.WithBlockHeight(height)

	// Check expected slash point amounts
	observeSlashPoints := mgr.GetConstants().GetInt64Value(constants.ObserveSlashPoints)
	lackOfObservationPenalty := mgr.GetConstants().GetInt64Value(constants.LackOfObservationPenalty)
	observeFlex := mgr.GetConstants().GetInt64Value(constants.ObservationDelayFlexibility)
	c.Assert(observeSlashPoints, Equals, int64(1))
	c.Assert(lackOfObservationPenalty, Equals, int64(2))
	c.Assert(observeFlex, Equals, int64(10))

	asgardVault := GetRandomVault()
	c.Assert(mgr.Keeper().SetVault(ctx, asgardVault), IsNil)

	nas := NodeAccounts{
		// 6 Active nodes, 1 Standby node; 2/3rds consensus needs 4.
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeStandby),
	}
	for _, item := range nas {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, item), IsNil)
	}

	msg := NewMsgNetworkFee(1024, common.ETHChain, 256, 100, cosmos.AccAddress{})
	handler := NewNetworkFeeHandler(mgr)

	broadcast := func(c *C, ctx cosmos.Context, na NodeAccount, msg *MsgNetworkFee) {
		msg.Signer = na.NodeAddress
		_, err := handler.handle(ctx, *msg)
		c.Assert(err, IsNil)
	}

	checkSlashPoints := func(c *C, ctx cosmos.Context, nas NodeAccounts, expected [7]int64) {
		var slashPoints [7]int64
		for i, na := range nas {
			slashPoint, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, na.NodeAddress)
			c.Assert(err, IsNil)
			slashPoints[i] = slashPoint
		}
		c.Assert(slashPoints == expected, Equals, true, Commentf(fmt.Sprint(slashPoints)))
	}

	checkSlashPoints(c, ctx, nas, [7]int64{0, 0, 0, 0, 0, 0, 0})

	// 3/6 Active nodes observe.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[1], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[2], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 1, 0, 0, 0, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 1, 1, 0, 0, 0, 0})

	// nas[3] observes, reaching consensus (4/6, being exactly the 2/3 threshold).
	// (Active nodes which observed are decremented ObserveSlashPoints;
	//  those which haven't are incremented LackOfObservationPenalty.)
	broadcast(c, ctx, nas[3], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 2, 2, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 2, 2, 0})

	// Within the ObservationDelayFlexibility period, nas[4] observes
	// (and is decremented LackOfObservationPenalty).
	height += observeFlex
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[4], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 2, 0})

	// The ObservationDelayFlexibility period ends, after which nas[5] observes;
	// it is not incremented ObserveSlashPoints (and it is added to the list of signers)
	// and is also not decremented LackOfObservationPenalty.
	height++
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 2, 0})

	// nas[5] observes again, this time incremented ObserveSlashPoints for the extra signing.
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 3, 0})

	// Note that nas[6], the Standby node, remains unaffected by the Actives nodes' observations.
}
