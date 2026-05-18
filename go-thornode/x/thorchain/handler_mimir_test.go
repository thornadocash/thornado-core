package thorchain

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type HandlerMimirSuite struct{}

var _ = Suite(&HandlerMimirSuite{})

func (s *HandlerMimirSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (s *HandlerMimirSuite) TestValidate(c *C) {
	ctx, keeper := setupKeeperForTest(c)

	handler := NewMimirHandler(NewDummyMgrWithKeeper(keeper))

	// invalid msg
	msg := &MsgMimir{}
	err := handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerMimirSuite) TestMimirHandle(c *C) {
	ctx, keeper := setupKeeperForTest(c)
	handler := NewMimirHandler(NewDummyMgrWithKeeper(keeper))

	invalidMsg := NewMsgNetworkFee(ctx.BlockHeight(), common.ETHChain, 1, 10000, GetRandomBech32Addr())
	result, err := handler.Run(ctx, invalidMsg)
	c.Check(err, NotNil)
	c.Check(result, IsNil)

	// Non-validator address for MsgMimir (so fails validation).
	addr := GetRandomBech32Addr()
	msg := NewMsgMimir("foo", 55, addr)
	result, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)

	// node set mimir
	FundModule(c, ctx, keeper, BondName, 100*common.One)
	ver := "1.92.0"
	na1 := GetRandomValidatorNode(NodeActive)
	na1.Version = ver
	na2 := GetRandomValidatorNode(NodeActive)
	na2.Version = ver
	na3 := GetRandomValidatorNode(NodeActive)
	na3.Version = ver
	c.Assert(keeper.SetNodeAccount(ctx, na1), IsNil)
	c.Assert(keeper.SetNodeAccount(ctx, na2), IsNil)
	c.Assert(keeper.SetNodeAccount(ctx, na3), IsNil)

	result, err = handler.Run(ctx, NewMsgMimir("AffiliateFeeBasisPointsMax", 1, na1.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	mimirs, err := keeper.GetNodeMimirs(ctx, "AffiliateFeeBasisPointsMax")
	c.Assert(err, IsNil)
	c.Assert(mimirs.Mimirs, HasLen, 1)

	// first node set mimir , no consensus
	result, err = handler.Run(ctx, NewMsgMimir("node-mimir", 1, na1.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	mvalue, err := keeper.GetMimir(ctx, "node-mimir")
	c.Assert(err, IsNil)
	c.Assert(mvalue, Equals, int64(-1))

	// second node set mimir, reach consensus
	result, err = handler.Run(ctx, NewMsgMimir("node-mimir", 1, na2.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	mvalue, err = keeper.GetMimir(ctx, "node-mimir")
	c.Assert(err, IsNil)
	c.Assert(mvalue, Equals, int64(1))

	// third node set mimir, reach consensus
	result, err = handler.Run(ctx, NewMsgMimir("node-mimir", 1, na3.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	mvalue, err = keeper.GetMimir(ctx, "node-mimir")
	c.Assert(err, IsNil)
	c.Assert(mvalue, Equals, int64(1))

	// third node vote mimir to a different value, it should not change the admin mimir value
	result, err = handler.Run(ctx, NewMsgMimir("node-mimir", 0, na3.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	mvalue, err = keeper.GetMimir(ctx, "node-mimir")
	c.Assert(err, IsNil)
	c.Assert(mvalue, Equals, int64(1))

	// second node vote mimir to a different value , it should update admin mimir
	result, err = handler.Run(ctx, NewMsgMimir("node-mimir", 0, na2.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	mvalue, err = keeper.GetMimir(ctx, "node-mimir")
	c.Assert(err, IsNil)
	c.Assert(mvalue, Equals, int64(0))

	result, err = handler.Run(ctx, NewMsgMimir("node-mimir-1", 0, na2.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// With OperationalVotesMin 3, Operational Mimir should not be set until at least three nodes agree.
	keeper.SetMimir(ctx, constants.OperationalVotesMin.String(), 3)

	result, err = handler.Run(ctx, NewMsgMimir("HaltSigning", 1, na1.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	result, err = handler.Run(ctx, NewMsgMimir("HaltSigning", 1, na2.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	val, err := keeper.GetMimir(ctx, "HaltSigning")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(-1))
	// Now the third.
	result, err = handler.Run(ctx, NewMsgMimir("HaltSigning", 1, na3.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	val, err = keeper.GetMimir(ctx, "HaltSigning")
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(1))
}
