package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

type MsgReferenceMemoSuite struct{}

var _ = Suite(&MsgReferenceMemoSuite{})

func (mas *MsgReferenceMemoSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

func (MsgReferenceMemoSuite) TestMsgReferenceMemo(c *C) {
	signerAddr := GetRandomBech32Addr()
	msgApply := NewMsgReferenceMemo(common.BTCAsset, "test memo", signerAddr)
	c.Assert(msgApply.ValidateBasic(), IsNil)
	c.Assert(msgApply.Route(), Equals, RouterKey)
	c.Assert(msgApply.Type(), Equals, "ref_memo")
	c.Assert(msgApply.GetSignBytes(), NotNil)
	c.Assert(len(msgApply.GetSigners()), Equals, 1)
	c.Assert(msgApply.GetSigners()[0].Equals(signerAddr), Equals, true)

	// failure cases
	c.Check(NewMsgReferenceMemo(common.EmptyAsset, "test memo", signerAddr).ValidateBasic(), NotNil)
	c.Check(NewMsgReferenceMemo(common.BTCAsset, "", signerAddr).ValidateBasic(), NotNil)
}
