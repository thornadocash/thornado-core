package types

import (
	"errors"
	"strings"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	common "gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type MsgDepositSuite struct{}

var _ = Suite(&MsgDepositSuite{})

func (MsgDepositSuite) TestMsgDepositSuite(c *C) {
	acc1 := GetRandomBech32Addr()
	c.Assert(acc1.Empty(), Equals, false)

	coins := common.Coins{
		common.NewCoin(common.RuneNative, cosmos.NewUint(12*common.One)),
	}
	memo := "hello"
	msg := NewMsgDeposit(coins, memo, acc1)
	msg.Salt = []byte("salt")
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(msg.GetSigners(), NotNil)
	c.Assert(msg.GetSigners()[0].String(), Equals, acc1.String())
	c.Assert(msg.GetSalt(), DeepEquals, []byte("salt"))

	// ensure non-native assets are blocked
	coins = common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.NewUint(12*common.One)),
	}
	msg = NewMsgDeposit(coins, memo, acc1)
	c.Assert(msg.ValidateBasic(), NotNil)

	msg1 := NewMsgDeposit(coins, "memo", cosmos.AccAddress{})
	err1 := msg1.ValidateBasic()
	c.Assert(err1, NotNil)
	c.Assert(errors.Is(err1, se.ErrInvalidAddress), Equals, true)

	msg2 := NewMsgDeposit(common.Coins{
		common.NewCoin(common.EmptyAsset, cosmos.ZeroUint()),
	}, "memo", acc1)
	err2 := msg2.ValidateBasic()
	c.Assert(err2, NotNil)
	c.Assert(errors.Is(err2, se.ErrUnknownRequest), Equals, true)

	msg3 := NewMsgDeposit(common.Coins{
		common.NewCoin(common.RuneNative, cosmos.NewUint(12*common.One)),
	}, "asdfsdkljadslfasfaqcvbncvncvbncvbncvbncvbncvbncvbncvbncvbncvbnsdfasdfasfasdfkjqwerqlkwerqlerqwlkerjqlwkerjqwlkerjqwlkerjqlkwerjklqwerjqwlkerjqlwkerjwqelrasdfsdkljadslfasfaqcvbncvncvbncvbncvbncvbncvbncvbncvbncvbncvbnsdfasdfasfasdfkjqwerqlkwerqlerqwlkerjqlwkerjqwlkerjqwlkerjqlkwerjklqwerjqwlkerjqlwkerjwqelr", acc1)
	err3 := msg3.ValidateBasic()
	c.Assert(err3, NotNil)
	c.Assert(errors.Is(err3, se.ErrUnknownRequest), Equals, true)

	msg4 := NewMsgDeposit(common.Coins{
		common.NewCoin(common.RuneNative, cosmos.NewUint(12*common.One)),
	}, "memo", acc1)
	msg4.Salt = []byte(strings.Repeat("s", constants.MaxDepositSaltSize+1))
	err4 := msg4.ValidateBasic()
	c.Assert(err4, NotNil)
	c.Assert(errors.Is(err4, se.ErrUnknownRequest), Equals, true)
}
