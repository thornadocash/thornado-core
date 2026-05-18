package common

import (
	. "gopkg.in/check.v1"
)

type ChainNetworkSuite struct{}

var _ = Suite(&ChainNetworkSuite{})

func (s *ChainNetworkSuite) TestSoftEquals(c *C) {
	c.Assert(MainNet.SoftEquals(MainNet), Equals, true)
	c.Assert(MockNet.SoftEquals(MockNet), Equals, true)
	c.Assert(MainNet.SoftEquals(MockNet), Equals, false)
	c.Assert(ChainNet.SoftEquals(ChainNet), Equals, true)
	c.Assert(ChainNet.SoftEquals(MainNet), Equals, false)
	c.Assert(StageNet.SoftEquals(StageNet), Equals, true)
	c.Assert(StageNet.SoftEquals(ChainNet), Equals, true)
}
