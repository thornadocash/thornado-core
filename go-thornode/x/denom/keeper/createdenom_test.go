package keeper_test

import (
	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func (s *KeeperTestSuite) TestMsgCreateDenom() {
	s.SetupTest()

	// Creating a denom should work
	res, err := s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(
			accAddrs[0].String(),
			"bitcoin",
		))
	s.Require().NoError(err)
	s.Require().NotEmpty(res.GetNewTokenDenom())

	// Make sure that the admin is set correctly
	queryRes, err := s.queryClient.DenomAdmin(s.ctx, &types.QueryDenomAdminRequest{
		Denom: res.GetNewTokenDenom(),
	})
	s.Require().NoError(err)
	s.Require().Equal(accAddrs[0].String(), queryRes.Admin)

	// Make sure that a second version of the same denom can't be recreated
	_, err = s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(accAddrs[0].String(),
			"bitcoin",
		))
	s.Require().Error(err)

	// Creating a second denom should work
	res, err = s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(accAddrs[0].String(),
			"litecoin",
		))
	s.Require().NoError(err)
	s.Require().NotEmpty(res.GetNewTokenDenom())

	// Make sure that a second account can't create a denom with the same nonce
	_, err = s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(accAddrs[1].String(),
			"bitcoin",
		))
	s.Require().Error(err)

	// Make sure that an address with a "/" in it can't create denoms
	_, err = s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(
			"addr.eth/creator",
			"bitcoin",
		))
	s.Require().Error(err)
}
