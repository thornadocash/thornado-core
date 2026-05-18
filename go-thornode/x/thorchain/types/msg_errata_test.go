package types

import (
	"errors"

	se "github.com/cosmos/cosmos-sdk/types/errors"

	common "gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"

	. "gopkg.in/check.v1"
)

type MsgErrataTxSuite struct{}

var _ = Suite(&MsgErrataTxSuite{})

func (MsgErrataTxSuite) TestMsgErrataTxSuite(c *C) {
	txID := GetRandomTxHash()
	acc1 := GetRandomBech32Addr()
	c.Assert(acc1.Empty(), Equals, false)
	msg := NewMsgErrataTx(txID, common.ETHChain, acc1)
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(msg.GetSigners(), NotNil)
	c.Assert(msg.GetSigners()[0].String(), Equals, acc1.String())

	msg1 := NewMsgErrataTx(txID, common.ETHChain, cosmos.AccAddress{})
	err1 := msg1.ValidateBasic()
	c.Assert(err1, NotNil)
	c.Assert(errors.Is(err1, se.ErrInvalidAddress), Equals, true)

	msg2 := NewMsgErrataTx(common.TxID(""), common.ETHChain, acc1)
	err2 := msg2.ValidateBasic()
	c.Assert(err2, NotNil)
	c.Assert(errors.Is(err2, se.ErrUnknownRequest), Equals, true)

	msg3 := NewMsgErrataTx(txID, common.EmptyChain, acc1)
	err3 := msg3.ValidateBasic()
	c.Assert(err3, NotNil)
	c.Assert(errors.Is(err3, se.ErrUnknownRequest), Equals, true)
}
