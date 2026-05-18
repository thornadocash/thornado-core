//go:build !stagenet && !chainnet && !mocknet
// +build !stagenet,!chainnet,!mocknet

package common

import (
	. "gopkg.in/check.v1"
)

var _ = Suite(&AddressSuite{})

func (s *AddressSuite) TestMainnetAddress(c *C) {
	// DOGE mainnet address, invalid on mocknet
	addr, err := NewAddress("DJbKker23xfz3ufxAbqUuQwp1EBibGJJHu")
	c.Assert(err, IsNil)
	c.Assert(addr.IsChain(DOGEChain), Equals, true)
	c.Assert(addr.GetNetwork(DOGEChain), Equals, MainNet)

	// BCH mocknet address, invalid on mainnet
	_, err = NewAddress("qq0y8fmkq48rt3z5dlkv87ged93ranf2ggkuz9gfl8")
	c.Assert(err, NotNil)

	// DOGE mocknet address, invalid on mainnet
	_, err = NewAddress("nfWiQeddE4zsYsDuYhvpgVC7y4gjr5RyqK")
	c.Assert(err, NotNil)
}

func (s *AddressSuite) TestToTexConversion(c *C) {
	ethAddr, _ := NewAddress("0x90f2b1ae50e6018230e90a33f98c7844a0ab635a")
	addr, err := ethAddr.ToTexAddress()
	c.Assert(err, NotNil)
	c.Assert(addr, Equals, NoAddress)

	p2pkhAddrMain, _ := NewAddress("t1bgJzZzNhbXEgFcRo9XmJHdLadhEAbzFbh")
	addr, err = p2pkhAddrMain.ToTexAddress()
	c.Assert(err, IsNil)
	c.Assert(addr.String(), Equals, "tex1cd8heyc2v8fwe7q78m88x7q9zs2fu82cth96vn")

	// tex address must be p2pkh
	p2shAddrMain, _ := NewAddress("t3cNKv7UpFVqcmRJrvFCMiuzGj9ywpKtDP5")
	_, err = p2shAddrMain.ToTexAddress()
	c.Assert(err, NotNil)
}

func (s *AddressSuite) TestConvertToNewBCHAddressFormat(c *C) {
	addr1 := "1EFJFJm7Y9mTVsCBXA9PKuRuzjgrdBe4rR"
	addr1Result, err := ConvertToNewBCHAddressFormat(Address(addr1))
	c.Assert(err, IsNil)
	c.Assert(addr1Result.IsEmpty(), Equals, false)
	c.Assert(addr1Result.String(), Equals, "qzg5mkh7rkw3y8kw47l3rrnvhmenvctmd56xg38a70")

	addr3 := "qzg5mkh7rkw3y8kw47l3rrnvhmenvctmd56xg38a70"
	addr3Result, err := ConvertToNewBCHAddressFormat(Address(addr3))
	c.Assert(err, IsNil)
	c.Assert(addr3Result.IsEmpty(), Equals, false)
	c.Assert(addr3Result.String(), Equals, "qzg5mkh7rkw3y8kw47l3rrnvhmenvctmd56xg38a70")

	addr4 := "18P1smBRB8zgfHT2qU9mnrbkHuinL1VRQe"
	addr4Result, err := ConvertToNewBCHAddressFormat(Address(addr4))
	c.Assert(err, IsNil)
	c.Assert(addr4Result.IsEmpty(), Equals, false)
	c.Assert(addr4Result.String(), Equals, "qpg09septgjye6rw6lp3wep6s7j73je2tg5sea68x9")

	addr5 := "qrwz8uegrdd08x57uxzapthc6lm4fxmnwv0apsganr"
	addr5Result, err := ConvertToNewBCHAddressFormat(Address(addr5))
	c.Assert(err, NotNil)
	c.Assert(addr5Result.IsEmpty(), Equals, true)

	addr6 := "whatever"
	addr6Result, err := ConvertToNewBCHAddressFormat(Address(addr6))
	c.Assert(err, NotNil)
	c.Assert(addr6Result.IsEmpty(), Equals, true)

	addr7 := "3PLcoeUdBbYjQ3FZ98bSBdszNfXyEK3n91"
	addr7Result, err := ConvertToNewBCHAddressFormat(Address(addr7))
	c.Assert(err, IsNil)
	c.Assert(addr7Result.IsEmpty(), Equals, false)
	c.Assert(addr7Result.String(), Equals, "prkhwf3etusv88eu7fekcxgce7pj0vuf4sys9u2mns")
}
