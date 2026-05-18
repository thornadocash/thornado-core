package tokenlist

import (
	. "gopkg.in/check.v1"
)

type ETHTokenListSuite struct{}

var _ = Suite(&ETHTokenListSuite{})

func (s ETHTokenListSuite) TestLoad(c *C) {
	tokens := GetETHTokenList()
	c.Check(tokens.Name, Equals, "Mocknet Token List")
	c.Check(len(tokens.Tokens) > 0, Equals, true)
}
