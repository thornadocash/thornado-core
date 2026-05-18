package common

import (
	"github.com/btcsuite/btcd/chaincfg"
	dogchaincfg "github.com/eager7/dogd/chaincfg"
	ltcchaincfg "github.com/ltcsuite/ltcd/chaincfg"
	. "gopkg.in/check.v1"
)

type ChainSuite struct{}

var _ = Suite(&ChainSuite{})

func (s ChainSuite) TestChain(c *C) {
	ethChain, err := NewChain("eth")
	c.Assert(err, IsNil)
	c.Check(ethChain.Equals(ETHChain), Equals, true)
	c.Check(ethChain.IsEmpty(), Equals, false)
	c.Check(ethChain.String(), Equals, "ETH")

	_, err = NewChain("B") // too short
	c.Assert(err, NotNil)

	chains := Chains{"DOGE", "DOGE", "BTC"}
	c.Check(chains.Has("BTC"), Equals, true)
	c.Check(chains.Has("ETH"), Equals, false)
	uniq := chains.Distinct()
	c.Assert(uniq, HasLen, 2)

	algo := ETHChain.GetSigningAlgo()
	c.Assert(algo, Equals, SigningAlgoSecp256k1)

	c.Assert(BTCChain.GetGasAsset(), Equals, BTCAsset)
	c.Assert(ETHChain.GetGasAsset(), Equals, ETHAsset)
	c.Assert(LTCChain.GetGasAsset(), Equals, LTCAsset)
	c.Assert(BCHChain.GetGasAsset(), Equals, BCHAsset)
	c.Assert(DOGEChain.GetGasAsset(), Equals, DOGEAsset)
	c.Assert(GAIAChain.GetGasAsset(), Equals, ATOMAsset)
	c.Assert(NOBLEChain.GetGasAsset(), Equals, USDCAsset)
	c.Assert(TRONChain.GetGasAsset(), Equals, TRXAsset)
	c.Assert(EmptyChain.GetGasAsset(), Equals, EmptyAsset)

	c.Assert(BTCChain.AddressPrefix(MockNet), Equals, chaincfg.RegressionNetParams.Bech32HRPSegwit)
	c.Assert(BTCChain.AddressPrefix(MainNet), Equals, chaincfg.MainNetParams.Bech32HRPSegwit)
	c.Assert(BTCAsset.Chain.AddressPrefix(StageNet), Equals, chaincfg.MainNetParams.Bech32HRPSegwit)

	c.Assert(LTCChain.AddressPrefix(MockNet), Equals, ltcchaincfg.RegressionNetParams.Bech32HRPSegwit)
	c.Assert(LTCChain.AddressPrefix(MainNet), Equals, ltcchaincfg.MainNetParams.Bech32HRPSegwit)
	c.Assert(LTCChain.AddressPrefix(StageNet), Equals, ltcchaincfg.MainNetParams.Bech32HRPSegwit)

	c.Assert(DOGEChain.AddressPrefix(MockNet), Equals, dogchaincfg.RegressionNetParams.Bech32HRPSegwit)
	c.Assert(DOGEChain.AddressPrefix(MainNet), Equals, dogchaincfg.MainNetParams.Bech32HRPSegwit)
	c.Assert(DOGEChain.AddressPrefix(StageNet), Equals, dogchaincfg.MainNetParams.Bech32HRPSegwit)

	// Noble chain tests
	c.Assert(NOBLEChain.AddressPrefix(MockNet), Equals, "noble")
	c.Assert(NOBLEChain.AddressPrefix(MainNet), Equals, "noble")
	c.Assert(NOBLEChain.AddressPrefix(StageNet), Equals, "noble")

	// Test GetGasUnits
	nobleGasUnits, _ := NOBLEChain.GetGasUnits()
	gaiaGasUnits, _ := GAIAChain.GetGasUnits()
	c.Assert(nobleGasUnits, Equals, "uusdc")
	c.Assert(gaiaGasUnits, Equals, "uatom")

	// Test GetGasAssetDecimal
	c.Assert(NOBLEChain.GetGasAssetDecimal(), Equals, int64(6))
	c.Assert(GAIAChain.GetGasAssetDecimal(), Equals, int64(6))

	// Test ApproximateBlockMilliseconds
	c.Assert(NOBLEChain.ApproximateBlockMilliseconds(), Equals, int64(1500))
	c.Assert(GAIAChain.ApproximateBlockMilliseconds(), Equals, int64(6000))

	// Test InboundNotes
	c.Assert(NOBLEChain.InboundNotes(), Equals, "Transfer the inbound_address the asset with the memo. Do not use multi-in, multi-out transactions.")
	c.Assert(GAIAChain.InboundNotes(), Equals, "Transfer the inbound_address the asset with the memo. Do not use multi-in, multi-out transactions.")

	// Tron chain tests
	tronGasUnits, _ := TRONChain.GetGasUnits()
	c.Assert(tronGasUnits, Equals, "sun")
	c.Assert(TRONChain.GetGasAssetDecimal(), Equals, int64(6))
	c.Assert(TRONChain.ApproximateBlockMilliseconds(), Equals, int64(3000))
}
