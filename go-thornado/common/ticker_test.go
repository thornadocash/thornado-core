package common

import (
	. "gopkg.in/check.v1"
)

type TickerSuite struct{}

var _ = Suite(&TickerSuite{})

func (s TickerSuite) TestTicker(c *C) {
	const TestTicker = Ticker("TEST")

	testTicker, err := NewTicker("test")
	c.Assert(err, IsNil)
	bnbTicker, err := NewTicker("bnb")
	c.Assert(err, IsNil)
	c.Check(testTicker.IsEmpty(), Equals, false)
	c.Check(testTicker.Equals(TestTicker), Equals, true)
	c.Check(bnbTicker.Equals(TestTicker), Equals, false)
	c.Check(testTicker.String(), Equals, "TEST")

	tomobTicker, err := NewTicker("TOMOB-1E1")
	c.Assert(err, IsNil)
	c.Assert(tomobTicker.String(), Equals, "TOMOB-1E1")
	_, err = NewTicker("t") // too short
	c.Assert(err, IsNil)

	maxCharacterTicker, err := NewTicker("TICKER789-XXX")
	c.Assert(err, IsNil)
	c.Assert(maxCharacterTicker.IsEmpty(), Equals, false)
	_, err = NewTicker("too long of a ticker") // too long
	c.Assert(err, NotNil)
}
