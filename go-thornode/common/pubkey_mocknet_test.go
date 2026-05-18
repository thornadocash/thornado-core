//go:build mocknet
// +build mocknet

package common

import (
	"encoding/hex"

	. "gopkg.in/check.v1"

	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/cosmos/cosmos-sdk/crypto/codec"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

func (s *PubKeyTestSuite) TestPubKeyGetAddress(c *C) {
	for _, d := range s.keyData {
		privB, _ := hex.DecodeString(d.priv)
		pubB, _ := hex.DecodeString(d.pub)
		priv := secp256k1.PrivKey(privB)
		pubKey := priv.PubKey()
		pubT, _ := pubKey.(secp256k1.PubKey)
		pub := pubT[:]

		c.Assert(hex.EncodeToString(pub), Equals, hex.EncodeToString(pubB))

		tmp, err := codec.FromTmPubKeyInterface(pubKey)
		c.Assert(err, IsNil)
		pubBech32, err := cosmos.Bech32ifyPubKey(cosmos.Bech32PubKeyTypeAccPub, tmp)
		c.Assert(err, IsNil)

		pk, err := NewPubKey(pubBech32)
		c.Assert(err, IsNil)

		addrETH, err := pk.GetAddress(ETHChain)
		c.Assert(err, IsNil)
		c.Assert(addrETH.String(), Equals, d.addrETH.mocknet)

		addrBTC, err := pk.GetAddress(BTCChain)
		c.Assert(err, IsNil)
		c.Assert(addrBTC.String(), Equals, d.addrBTC.mocknet)

		addrLTC, err := pk.GetAddress(LTCChain)
		c.Assert(err, IsNil)
		c.Assert(addrLTC.String(), Equals, d.addrLTC.mocknet)

		addrBCH, err := pk.GetAddress(BCHChain)
		c.Assert(err, IsNil)
		c.Assert(addrBCH.String(), Equals, d.addrBCH.mocknet)

		addrDOGE, err := pk.GetAddress(DOGEChain)
		c.Assert(err, IsNil)
		c.Assert(addrDOGE.String(), Equals, d.addrDOGE.mocknet)

		addrTRON, err := pk.GetAddress(TRONChain)
		c.Assert(err, IsNil)
		c.Assert(addrTRON.IsChain(TRONChain), Equals, true)

		// Test GAIA address generation
		addrGAIA, err := pk.GetAddress(GAIAChain)
		c.Assert(err, IsNil)
		// Verify it has cosmos prefix
		c.Assert(addrGAIA.String()[:6], Equals, "cosmos")
		c.Assert(addrGAIA.IsChain(GAIAChain), Equals, true)

		// Test NOBLE address generation
		addrNOBLE, err := pk.GetAddress(NOBLEChain)
		c.Assert(err, IsNil)
		// Verify it has noble prefix
		c.Assert(addrNOBLE.String()[:5], Equals, "noble")
		c.Assert(addrNOBLE.IsChain(NOBLEChain), Equals, true)

		// Verify GAIA and NOBLE addresses are different (different prefixes)
		c.Assert(addrGAIA.String() != addrNOBLE.String(), Equals, true)
	}
}
