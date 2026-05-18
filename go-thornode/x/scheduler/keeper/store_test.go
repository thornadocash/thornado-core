package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/x/scheduler/types"
)

func (s *KeeperTestSuite) TestStoreMsgs(c *C) {
	sdkCtx := sdk.UnwrapSDKContext(s.ctx)
	res, err := s.keeper.GetSchedule(sdkCtx, 1000)

	c.Assert(err, IsNil)
	c.Assert(res.Height, Equals, uint64(0x3e8))
	c.Assert(res.Msgs, HasLen, 0)

	err = s.keeper.AddMsg(sdkCtx, types.MsgScheduleExecuteContract{
		After:  1000,
		Sender: accAddrs[0].String(),
		Msg:    []byte(`{"do":"something"}`),
	})

	c.Assert(err, IsNil)

	res, err = s.keeper.GetSchedule(sdkCtx, 1001)

	c.Assert(err, IsNil)
	c.Assert(res.Height, Equals, uint64(0x3e9))
	c.Assert(res.Msgs, HasLen, 1)
	c.Assert(res.Msgs[0].Sender, Equals, accAddrs[0].String())
	c.Assert(res.Msgs[0].Msg, DeepEquals, []byte(`{"do":"something"}`))

	err = s.keeper.AddMsg(sdkCtx, types.MsgScheduleExecuteContract{
		After:  1000,
		Sender: accAddrs[1].String(),
		Msg:    []byte(`{"do":"something else"}`),
	})
	c.Assert(err, IsNil)

	res, err = s.keeper.GetSchedule(sdkCtx, 1001)

	c.Assert(err, IsNil)
	c.Assert(res.Height, Equals, uint64(0x3e9))
	c.Assert(res.Msgs, HasLen, 2)
	c.Assert(res.Msgs[0].Sender, Equals, accAddrs[0].String())
	c.Assert(res.Msgs[0].Msg, DeepEquals, []byte(`{"do":"something"}`))
	c.Assert(res.Msgs[1].Sender, Equals, accAddrs[1].String())
	c.Assert(res.Msgs[1].Msg, DeepEquals, []byte(`{"do":"something else"}`))
}
