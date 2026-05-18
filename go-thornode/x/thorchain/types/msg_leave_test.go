package types

import (
	"errors"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MsgLeaveSuite struct{}

var _ = Suite(&MsgLeaveSuite{})

func (*MsgLeaveSuite) SetupSuite(c *C) {
	SetupConfigForTest()
}

func (MsgLeaveSuite) TestMsgLeave(c *C) {
	nodeAddr := GetRandomBech32Addr()
	txID := GetRandomTxHash()
	senderETHAddr := GetRandomETHAddress()
	tx := GetRandomTx()
	tx.ID = txID
	tx.FromAddress = senderETHAddr
	msgLeave := NewMsgLeave(tx, nodeAddr, nodeAddr)
	EnsureMsgBasicCorrect(msgLeave, c)
	c.Assert(msgLeave.ValidateBasic(), IsNil)

	msgLeave1 := NewMsgLeave(tx, nodeAddr, nodeAddr)
	c.Assert(msgLeave1.ValidateBasic(), IsNil)
	msgLeave2 := NewMsgLeave(common.Tx{ID: "", FromAddress: senderETHAddr}, nodeAddr, nodeAddr)
	c.Assert(msgLeave2.ValidateBasic(), NotNil)
	msgLeave3 := NewMsgLeave(tx, nodeAddr, cosmos.AccAddress{})
	c.Assert(msgLeave3.ValidateBasic(), NotNil)
	msgLeave4 := NewMsgLeave(common.Tx{ID: txID, FromAddress: ""}, nodeAddr, nodeAddr)
	c.Assert(msgLeave4.ValidateBasic(), NotNil)

	msgLeave5 := NewMsgLeave(tx, cosmos.AccAddress{}, nodeAddr)
	err5 := msgLeave5.ValidateBasic()
	c.Assert(err5, NotNil)
	c.Assert(errors.Is(err5, se.ErrInvalidAddress), Equals, true)
}
