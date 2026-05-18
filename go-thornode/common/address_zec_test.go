package common

import (
	. "gopkg.in/check.v1"
)

type ZECAddressSuite struct{}

var _ = Suite(&ZECAddressSuite{})

func (s *ZECAddressSuite) TestZECAddressValidation(c *C) {
	// Sapling address (z), not supported
	_, err := NewAddress("zs1z7rejlpsa98s2rrrfkwmaxu53e4ue0ulcrw0h4x5g8jl04tak0d3mm47vdtahatqrlkngh9sly")
	c.Assert(err, NotNil)

	// Unified address (u), not supported
	_, err = NewAddress("u1yvwppp7ann6n3pgkysdu0spvr50w4jf4jwgme3c8x8fp4av59rupgvdd3fddc3f2cwrk3ghs5lxt87ggj8cvjuzcrf4jkejwlu9pc83gk2vtx03ucqcc3ed0furcuypqs6d6swu3nws")
	c.Assert(err, NotNil)

	// Wrong checksum
	_, err = NewAddress("t2DcaUhoxrJDCj1xJmzWRdkHofRUM1erfih")
	c.Assert(err, NotNil)
}

func (s *ZECAddressSuite) TestZECNotChain(c *C) {
	// Test that non-ZEC addresses don't match ZECChain
	ethAddr, _ := NewAddress("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a")
	c.Assert(ethAddr.IsChain(ZECChain), Equals, false)

	btcAddr, _ := NewAddress("1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX")
	c.Assert(btcAddr.IsChain(ZECChain), Equals, false)

	thorAddr, _ := NewAddress("thor1kljxxccrheghavaw97u78le6yy3sdj7h696nl4")
	c.Assert(thorAddr.IsChain(ZECChain), Equals, false)
}
