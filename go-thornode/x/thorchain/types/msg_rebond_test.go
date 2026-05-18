package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type MsgRebondSuite struct{}

var _ = Suite(&MsgRebondSuite{})

func (s *MsgRebondSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (s *MsgRebondSuite) TestMsgRebondFromMemo(c *C) {
	nodeAddr := GetRandomBech32Addr()
	newAddr := GetRandomBech32Addr()
	signerAddr := GetRandomBech32Addr()

	tx := GetRandomTx()
	tx.Coins[0] = common.NewCoin(common.RuneAsset(), cosmos.NewUint(200000))

	txNoId := tx
	txNoId.ID = ""

	txNoFrom := tx
	txNoFrom.FromAddress = ""

	msg := NewMsgReBond(tx, nodeAddr, newAddr, cosmos.NewUint(12345), signerAddr)
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(len(msg.GetSigners()), Equals, 1)
	c.Assert(msg.GetSigners()[0].Equals(signerAddr), Equals, true)

	zero := cosmos.ZeroUint()

	for _, msg = range []*MsgReBond{
		NewMsgReBond(tx, nil, newAddr, zero, signerAddr),
		NewMsgReBond(tx, nodeAddr, nil, zero, signerAddr),
		NewMsgReBond(tx, nodeAddr, newAddr, zero, nil),
		NewMsgReBond(txNoId, nodeAddr, newAddr, zero, signerAddr),
		NewMsgReBond(txNoFrom, nodeAddr, newAddr, zero, signerAddr),
	} {
		c.Assert(msg.ValidateBasic(), NotNil)
	}
}
