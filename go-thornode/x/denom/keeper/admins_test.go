package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

func (s *KeeperTestSuite) TestAdminMsgs() {
	s.SetupTest()

	addr0bal := int64(0)
	addr1bal := int64(0)
	// Create a denom
	res, err := s.msgServer.CreateDenom(s.ctx,
		types.NewMsgCreateDenom(
			accAddrs[0].String(),
			"bitcoin",
		),
	)
	s.Require().NoError(err)
	denom := res.GetNewTokenDenom()

	// Make sure that the admin is set correctly
	queryRes, err := s.queryClient.DenomAdmin(s.ctx,
		&types.QueryDenomAdminRequest{
			Denom: res.GetNewTokenDenom(),
		})
	s.Require().NoError(err)
	s.Require().Equal(
		accAddrs[0].String(),
		queryRes.Admin,
	)

	// Test minting to admins own account
	_, err = s.msgServer.MintTokens(s.ctx,
		types.NewMsgMintTokens(
			accAddrs[0].String(),
			sdk.NewInt64Coin(denom, 10),
			accAddrs[0].String(),
		),
	)
	addr0bal += 10
	s.Require().NoError(err)
	s.Require().Equal(
		s.bankKeeper.GetBalance(s.ctx, accAddrs[0], denom).Amount.Int64(),
		addr0bal, s.bankKeeper.GetBalance(s.ctx, accAddrs[0], denom),
	)

	// Test burning from own account
	_, err = s.msgServer.BurnTokens(s.ctx,
		types.NewMsgBurnTokens(
			accAddrs[0].String(),
			sdk.NewInt64Coin(denom, 5),
		),
	)
	s.Require().NoError(err)
	s.Require().Equal(
		s.bankKeeper.GetBalance(s.ctx, accAddrs[1], denom).Amount.Int64(),
		addr1bal,
	)

	// Test Change Admin
	_, err = s.msgServer.ChangeDenomAdmin(s.ctx,
		types.NewMsgChangeDenomAdmin(accAddrs[0].String(),
			denom,
			accAddrs[1].String(),
		),
	)
	s.Require().NoError(err)
	queryRes, err = s.queryClient.DenomAdmin(s.ctx,
		&types.QueryDenomAdminRequest{
			Denom: res.GetNewTokenDenom(),
		},
	)
	s.Require().NoError(err)
	s.Require().Equal(
		accAddrs[1].String(),
		queryRes.Admin,
	)

	// Make sure old admin can no longer do actions
	_, err = s.msgServer.MintTokens(s.ctx,
		types.NewMsgMintTokens(
			accAddrs[0].String(),
			sdk.NewInt64Coin(denom, 5),
			accAddrs[0].String(),
		),
	)
	s.Require().Error(err)

	_, err = s.msgServer.BurnTokens(s.ctx,
		types.NewMsgBurnTokens(
			accAddrs[0].String(),
			sdk.NewInt64Coin(denom, 5),
		),
	)
	s.Require().Error(err)

	// Make sure the new admin works
	_, err = s.msgServer.MintTokens(s.ctx,
		types.NewMsgMintTokens(
			accAddrs[1].String(),
			sdk.NewInt64Coin(denom, 5),
			accAddrs[1].String(),
		),
	)
	addr1bal += 5
	s.Require().NoError(err)
	s.Require().Equal(
		s.bankKeeper.GetBalance(s.ctx, accAddrs[1], denom).Amount.Int64(),
		addr1bal,
	)

	// Try setting admin to empty
	_, err = s.msgServer.ChangeDenomAdmin(s.ctx,
		types.NewMsgChangeDenomAdmin(
			accAddrs[1].String(),
			denom,
			"",
		),
	)
	s.Require().NoError(err)
	queryRes, err = s.queryClient.DenomAdmin(s.ctx,
		&types.QueryDenomAdminRequest{
			Denom: res.GetNewTokenDenom(),
		},
	)
	s.Require().NoError(err)
	s.Require().Equal("", queryRes.Admin)
}
