//go:build chainnet
// +build chainnet

package common

import (
	. "gopkg.in/check.v1"
)

type AddressChainnetSuite struct{}

var _ = Suite(&AddressChainnetSuite{})

func (s *AddressChainnetSuite) TestCthorAddress(c *C) {
	addr, err := NewAddress("cthor1wvrexfj4sj9vrwmmhqtsmg5phr7m776dl8uxdw")
	c.Assert(err, IsNil)
	c.Check(addr.IsChain(THORChain), Equals, true)
	c.Check(addr.GetNetwork(THORChain), Equals, ChainNet)

	// cthor should not match other chains
	c.Check(addr.IsChain(ETHChain), Equals, false)
	c.Check(addr.IsChain(BTCChain), Equals, false)
	c.Check(addr.IsChain(GAIAChain), Equals, false)
}
