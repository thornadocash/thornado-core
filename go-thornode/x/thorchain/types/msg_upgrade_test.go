package types

import (
	"errors"
	"strings"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MsgProposeUpgradeSuite struct{}

var _ = Suite(&MsgProposeUpgradeSuite{})

func (MsgProposeUpgradeSuite) TestMsgProposeUpgradeSuite(c *C) {
	acc := GetRandomBech32Addr()
	c.Assert(acc.Empty(), Equals, false)
	msg := NewMsgProposeUpgrade("1.2.3", 100000, "proposed upgrade", acc)
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(msg.GetSigners(), NotNil)
	c.Assert(msg.GetSigners()[0].String(), Equals, acc.String())

	msg = NewMsgProposeUpgrade("1.2.3", 10000, "proposed upgrade", cosmos.AccAddress{})
	err := msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrInvalidAddress), Equals, true)

	longName := strings.Repeat("X", 101)
	msg = NewMsgProposeUpgrade(longName, 10000, "proposed upgrade", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "name cannot be longer than 100 characters"), Equals, true)

	msg = NewMsgProposeUpgrade("", 10000, "proposed upgrade", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "name cannot be empty"), Equals, true)

	msg = NewMsgProposeUpgrade("invalid semver", 10000, "proposed upgrade", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "name is not a valid semver"), Equals, true)

	longStr := strings.Repeat("X", 2501)
	msg = NewMsgProposeUpgrade("1.2.3", 10000, longStr, acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "info cannot be longer than 2500 characters"), Equals, true)

	msg = NewMsgProposeUpgrade("1.2.3", 0, "proposed upgrade", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "height cannot be zero"), Equals, true)
}

type MsgApproveUpgradeSuite struct{}

var _ = Suite(&MsgApproveUpgradeSuite{})

func (MsgApproveUpgradeSuite) TestMsgApproveUpgradeSuite(c *C) {
	acc := GetRandomBech32Addr()
	c.Assert(acc.Empty(), Equals, false)
	msg := NewMsgApproveUpgrade("1.2.3", acc)
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(msg.GetSigners(), NotNil)
	c.Assert(msg.GetSigners()[0].String(), Equals, acc.String())

	msg = NewMsgApproveUpgrade("1.2.3", cosmos.AccAddress{})
	err := msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrInvalidAddress), Equals, true)

	msg = NewMsgApproveUpgrade("", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "name cannot be empty"), Equals, true)
}

type MsgRejectUpgradeSuite struct{}

var _ = Suite(&MsgRejectUpgradeSuite{})

func (MsgRejectUpgradeSuite) TestMsgRejectUpgradeSuite(c *C) {
	acc := GetRandomBech32Addr()
	c.Assert(acc.Empty(), Equals, false)
	msg := NewMsgRejectUpgrade("1.2.3", acc)
	c.Assert(msg.ValidateBasic(), IsNil)
	c.Assert(msg.GetSigners(), NotNil)
	c.Assert(msg.GetSigners()[0].String(), Equals, acc.String())

	msg = NewMsgRejectUpgrade("1.2.3", cosmos.AccAddress{})
	err := msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrInvalidAddress), Equals, true)

	msg = NewMsgRejectUpgrade("", acc)
	err = msg.ValidateBasic()
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(strings.Contains(err.Error(), "name cannot be empty"), Equals, true)
}
