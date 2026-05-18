package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	cosmos "gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type MsgApplySuite struct{}

var _ = Suite(&MsgApplySuite{})

func (mas *MsgApplySuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (MsgApplySuite) TestMsgApply(c *C) {
	nodeAddr := GetRandomBech32Addr()
	txId := GetRandomTxHash()
	c.Check(txId.IsEmpty(), Equals, false)
	signerAddr := GetRandomBech32Addr()
	bondAddr := GetRandomETHAddress()
	txin := GetRandomTx()
	txin.Coins[0] = common.NewCoin(common.RuneAsset(), cosmos.NewUint(10*common.One))
	txinNoID := txin
	txinNoID.ID = ""
	msgApply := NewMsgBond(txin, nodeAddr, cosmos.NewUint(common.One), bondAddr, nil, signerAddr, 5000)
	c.Assert(msgApply.ValidateBasic(), IsNil)
	c.Assert(len(msgApply.GetSigners()), Equals, 1)
	c.Assert(msgApply.GetSigners()[0].Equals(signerAddr), Equals, true)
	c.Assert(msgApply.OperatorFee, Equals, int64(5000))
	c.Assert(NewMsgBond(txin, cosmos.AccAddress{}, cosmos.NewUint(common.One), bondAddr, nil, signerAddr, -1).ValidateBasic(), NotNil)
	c.Assert(NewMsgBond(txin, nodeAddr, cosmos.ZeroUint(), bondAddr, nil, signerAddr, -1).ValidateBasic(), NotNil)
	c.Assert(NewMsgBond(txinNoID, nodeAddr, cosmos.NewUint(common.One), bondAddr, nil, signerAddr, -1).ValidateBasic(), NotNil)
	c.Assert(NewMsgBond(txin, nodeAddr, cosmos.NewUint(common.One), "", nil, signerAddr, -1).ValidateBasic(), NotNil)
	c.Assert(NewMsgBond(txin, nodeAddr, cosmos.NewUint(common.One), bondAddr, nil, cosmos.AccAddress{}, -2).ValidateBasic(), NotNil)
	c.Assert(NewMsgBond(txin, nodeAddr, cosmos.NewUint(common.One), bondAddr, nil, cosmos.AccAddress{}, 10001).ValidateBasic(), NotNil)
}
