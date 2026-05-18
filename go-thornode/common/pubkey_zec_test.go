//go:build !stagenet && !chainnet && !mocknet
// +build !stagenet,!chainnet,!mocknet

package common

import (
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type ZECPubKeyMainnetSuite struct{}

var _ = Suite(&ZECPubKeyMainnetSuite{})

func (s *ZECPubKeyMainnetSuite) TestZECAddressGeneration(c *C) {
	// Create a test pubkey using cosmos test utilities
	_, pubKey, _ := testdata.KeyTestPubAddr()
	spk, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey)
	c.Assert(err, IsNil)

	pk, err := NewPubKey(spk)
	c.Assert(err, IsNil)

	// Test mainnet address generation
	addr, err := pk.GetAddress(ZECChain)
	c.Assert(err, IsNil)
	c.Assert(addr.IsEmpty(), Equals, false)

	// Verify the address starts with mainnet prefix
	addrStr := addr.String()
	c.Check(addrStr[:2] == "t1", Equals, true)

	// Verify address length is valid
	c.Check(addrStr, HasLen, 35)

	// Verify it's recognized as a Zcash address
	c.Check(addr.IsChain(ZECChain), Equals, true)
	c.Check(addr.GetNetwork(ZECChain), Equals, MainNet)
}

func (s *ZECPubKeyMainnetSuite) TestZECAddressConsistency(c *C) {
	// Create a test pubkey
	_, pubKey, _ := testdata.KeyTestPubAddr()
	spk, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey)
	c.Assert(err, IsNil)

	pk, err := NewPubKey(spk)
	c.Assert(err, IsNil)

	// Generate address twice and verify they're the same (caching test)
	addr1, err1 := pk.GetAddress(ZECChain)
	c.Assert(err1, IsNil)

	addr2, err2 := pk.GetAddress(ZECChain)
	c.Assert(err2, IsNil)

	c.Check(addr1.Equals(addr2), Equals, true)
}

func (s *ZECPubKeyMainnetSuite) TestZECAddressFromDifferentPubKeys(c *C) {
	// Generate two different pubkeys and verify they produce different addresses
	_, pubKey1, _ := testdata.KeyTestPubAddr()
	spk1, err1 := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey1)
	c.Assert(err1, IsNil)
	pk1, err := NewPubKey(spk1)
	c.Assert(err, IsNil)

	// Create a second pubkey (different from the first)
	_, pubKey2, _ := testdata.KeyTestPubAddr()
	spk2, err2 := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey2)
	c.Assert(err2, IsNil)
	pk2, err := NewPubKey(spk2)
	c.Assert(err, IsNil)

	// Get addresses
	addr1, err := pk1.GetAddress(ZECChain)
	c.Assert(err, IsNil)

	addr2, err := pk2.GetAddress(ZECChain)
	c.Assert(err, IsNil)

	// Addresses should be different
	c.Check(addr1.Equals(addr2), Equals, false)

	// But both should be valid ZEC addresses
	c.Check(addr1.IsChain(ZECChain), Equals, true)
	c.Check(addr2.IsChain(ZECChain), Equals, true)
}

func (s *ZECPubKeyMainnetSuite) TestZECAddressNotEmpty(c *C) {
	// Empty pubkey should return empty address
	emptyPk := PubKey("")

	addr, err := emptyPk.GetAddress(ZECChain)
	c.Assert(err, IsNil)
	c.Check(addr.IsEmpty(), Equals, true)
}

func (s *ZECPubKeyMainnetSuite) TestZECAddressIsValid(c *C) {
	// Create a valid address and verify it validates correctly
	_, pubKey, _ := testdata.KeyTestPubAddr()
	spk, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, pubKey)
	c.Assert(err, IsNil)

	pk, err := NewPubKey(spk)
	c.Assert(err, IsNil)

	addr, err := pk.GetAddress(ZECChain)
	c.Assert(err, IsNil)

	// Convert to Address type and validate
	newAddr, err := NewAddress(addr.String())
	c.Assert(err, IsNil)
	c.Check(newAddr.IsChain(ZECChain), Equals, true)
	c.Check(newAddr.GetNetwork(ZECChain), Equals, MainNet)
}
