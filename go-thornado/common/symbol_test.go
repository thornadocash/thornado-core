package common

import (
	. "gopkg.in/check.v1"
)

type SymbolSuite struct{}

var _ = Suite(&SymbolSuite{})

func (s SymbolSuite) TestSymbol(c *C) {
	sym, err := NewSymbol("RUNE-67C")
	c.Assert(err, IsNil)
	c.Check(sym.Valid(), IsNil)
	c.Check(sym.IsEmpty(), Equals, false)
	c.Check(sym.String(), Equals, "RUNE-67C")
	c.Check(sym.Ticker().Equals(Ticker("RUNE")), Equals, true)
	c.Check(sym.IsMiniToken(), Equals, false)

	sym, err = NewSymbol("MINIA-7A2M")
	c.Assert(err, IsNil)
	c.Check(sym.Valid(), IsNil)
	c.Assert(sym.IsMiniToken(), Equals, true)

	sym, err = NewSymbol("MINIA-7AM")
	c.Assert(err, IsNil)
	c.Check(sym.Valid(), IsNil)
	c.Assert(sym.IsMiniToken(), Equals, false)

	sym, err = NewSymbol("eth.steth")
	c.Assert(err, IsNil)
	c.Check(sym.Valid(), IsNil)

	sym = "ETH~ETH"
	err = sym.Valid()
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "symbol must be alphanumeric")
}
