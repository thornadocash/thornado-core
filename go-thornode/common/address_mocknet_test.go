//go:build mocknet
// +build mocknet

package common

import (
	. "gopkg.in/check.v1"
)

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestMocknetAddress(c *C) {
	// BCH mocknet address, invalid on mainnet
	addr, err := NewAddress("qq0y8fmkq48rt3z5dlkv87ged93ranf2ggkuz9gfl8")
	c.Assert(err, IsNil)
	c.Assert(addr.IsChain(BCHChain), Equals, true)
	c.Assert(addr.GetNetwork(BCHChain), Equals, MockNet)
	for _, chain := range AllChains {
		if chain == BCHChain {
			continue
		}

		c.Assert(addr.IsChain(chain), Equals, false)
	}

	// DOGE mocknet address, invalid on mainnet
	addr, err = NewAddress("nfWiQeddE4zsYsDuYhvpgVC7y4gjr5RyqK")
	c.Assert(err, IsNil)
	c.Assert(addr.IsChain(DOGEChain), Equals, true)
	c.Assert(addr.GetNetwork(DOGEChain), Equals, MockNet)
	for _, chain := range AllChains {
		if chain == DOGEChain {
			continue
		}

		c.Assert(addr.IsChain(chain), Equals, false)
	}

	// DOGE mainnet address, invalid on mocknet
	_, err = NewAddress("DJbKker23xfz3ufxAbqUuQwp1EBibGJJHu")
	c.Assert(err, NotNil)
}

func (s *AddressSuite) TestToTexConversion(c *C) {
	ethAddr, _ := NewAddress("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a")
	addr, err := ethAddr.ToTexAddress()
	c.Assert(err, NotNil)
	c.Assert(addr, Equals, NoAddress)

	p2pkhAddrMain, _ := NewAddress("tmEp4k7dSU8eGj32uJPi2wBSGuiNaVFBLN7")
	addr, err = p2pkhAddrMain.ToTexAddress()
	c.Assert(err, IsNil)
	c.Assert(addr.String(), Equals, "textest1xlk5w3dleprzdug6z68wtjvcw4mtqa3s9pfays")

	// tex address must be p2pkh
	p2shAddrMain, _ := NewAddress("t2BeXPVjcVq4XDf7dgW4w3mK6ZjoePxy7Gb")
	addr, err = p2shAddrMain.ToTexAddress()
	c.Assert(err, NotNil)
}
