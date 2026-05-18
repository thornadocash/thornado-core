package thorchain

import (
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"

	. "gopkg.in/check.v1"
)

type HandlerSendSuiteV87 struct{}

var _ = Suite(&HandlerSendSuiteV87{})

func (s *HandlerSendSuiteV87) TestValidate(c *C) {
	ctx, k := setupKeeperForTest(c)

	addr1 := GetRandomBech32Addr()
	addr2 := GetRandomBech32Addr()
	moduleNameAccAddr := k.GetModuleAccAddress(ModuleName)

	msg := &MsgSend{
		FromAddress: addr1,
		ToAddress:   addr2,
		Amount:      cosmos.NewCoins(cosmos.NewCoin("dummy", cosmos.NewInt(12))),
	}
	handler := NewSendHandler(NewDummyMgrWithKeeper(k))
	err := handler.validate(ctx, msg)
	c.Assert(err, IsNil)

	msg.ToAddress = moduleNameAccAddr
	err = handler.validate(ctx, msg)
	c.Assert(err, IsNil, Commentf("sending to module address: %s indicates a memo will be attached for MsgDeposit", ModuleName))

	for _, moduleName := range []string{AsgardName, BondName, ReserveName} {
		msg.ToAddress = k.GetModuleAccAddress(moduleName)
		validateErr := handler.validate(ctx, msg)
		c.Assert(validateErr, NotNil, Commentf("cannot send to module: %s", moduleName))
	}

	// invalid msg
	msg = &MsgSend{}
	err = handler.validate(ctx, msg)
	c.Assert(err, NotNil)

	// Note that cosmos.NewCoins and cosmos.ParsedCoins both sanitise (drop) zero-amount coins.
	zeroCoin := cosmos.NewCoin("dummy", cosmos.ZeroInt())
	c.Assert(zeroCoin.String(), Equals, "0dummy")
	newCoins := cosmos.NewCoins(zeroCoin)
	c.Assert(newCoins.String(), Equals, "")
	parsedCoins, err := cosmos.ParseCoins("0dummy")
	c.Assert(err, IsNil)
	c.Assert(parsedCoins.String(), Equals, "")
	// (This is a valid ParseCoins format, as shown with a non-zero amount.)
	parsedCoins, err = cosmos.ParseCoins("1dummy")
	c.Assert(err, IsNil)
	c.Assert(parsedCoins.String(), Equals, "1dummy")

	zeroCoins := cosmos.Coins{zeroCoin}
	c.Assert(zeroCoins.String(), Equals, "0dummy")

	// Test zero amount rejection when not for MsgDeposit conversion.
	msg = &MsgSend{
		FromAddress: addr1,
		ToAddress:   addr2,
		Amount:      zeroCoins,
	}
	err = handler.validate(ctx, msg)
	c.Assert(err, NotNil)
	// Test zero amount validity when for MsgDeposit conversion (such as for MsgUnbond).
	msg = &MsgSend{
		FromAddress: addr1,
		ToAddress:   moduleNameAccAddr,
		Amount:      zeroCoins,
	}
	err = handler.validate(ctx, msg)
	c.Assert(err, IsNil)
}

func (s *HandlerSendSuiteV87) TestValidateMultiple(c *C) {
	// Verify that multiple coins can be sent, except to module address
	ctx, k := setupKeeperForTest(c)

	addr1 := GetRandomBech32Addr()
	addr2 := GetRandomBech32Addr()
	handler := NewSendHandler(NewDummyMgrWithKeeper(k))
	moduleAddress := k.GetModuleAccAddress(ModuleName)

	msg := &MsgSend{
		FromAddress: addr1,
		ToAddress:   addr2,
		Amount: cosmos.NewCoins(
			cosmos.NewCoin("foo", cosmos.NewInt(10)),
			cosmos.NewCoin("bar", cosmos.NewInt(20)),
		),
	}
	err := handler.validate(ctx, msg)
	c.Assert(err, IsNil)

	bank := &banktypes.MsgSend{
		FromAddress: addr1.String(),
		ToAddress:   addr2.String(),
		Amount: cosmos.NewCoins(
			cosmos.NewCoin("foo", cosmos.NewInt(10)),
			cosmos.NewCoin("bar", cosmos.NewInt(20)),
		),
	}
	err = handler.validate(ctx, bank)
	c.Assert(err, IsNil)

	msg = &MsgSend{
		FromAddress: addr1,
		ToAddress:   moduleAddress,
		Amount: cosmos.NewCoins(
			cosmos.NewCoin("foo", cosmos.NewInt(10)),
			cosmos.NewCoin("bar", cosmos.NewInt(20)),
		),
	}
	err = handler.validate(ctx, msg)
	c.Assert(err, NotNil)

	bank = &banktypes.MsgSend{
		FromAddress: addr1.String(),
		ToAddress:   moduleAddress.String(),
		Amount: cosmos.NewCoins(
			cosmos.NewCoin("foo", cosmos.NewInt(10)),
			cosmos.NewCoin("bar", cosmos.NewInt(20)),
		),
	}
	err = handler.validate(ctx, bank)
	c.Assert(err, NotNil)
}

func (s *HandlerSendSuiteV87) TestHandle(c *C) {
	ctx, k := setupKeeperForTest(c)

	addr1 := GetRandomBech32Addr()
	addr2 := GetRandomBech32Addr()

	FundAccount(c, ctx, k, addr1, 200*common.One)

	coin, err := common.NewCoin(common.RuneNative, cosmos.NewUint(12*common.One)).Native()
	c.Assert(err, IsNil)
	msg := &MsgSend{
		FromAddress: addr1,
		ToAddress:   addr2,
		Amount:      cosmos.NewCoins(coin),
	}

	handler := NewSendHandler(NewDummyMgrWithKeeper(k))
	_, err = handler.handle(ctx, msg)
	c.Assert(err, IsNil)

	// invalid msg should result in a error
	result, err := handler.Run(ctx, NewMsgNetworkFee(ctx.BlockHeight(), common.ETHChain, 1, 10000, GetRandomBech32Addr()))
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	// insufficient funds
	coin, err = common.NewCoin(common.RuneNative, cosmos.NewUint(3000*common.One)).Native()
	c.Assert(err, IsNil)
	msg = &MsgSend{
		FromAddress: addr1,
		ToAddress:   addr2,
		Amount:      cosmos.NewCoins(coin),
	}
	_, err = handler.handle(ctx, msg)
	c.Assert(err, NotNil)
}
