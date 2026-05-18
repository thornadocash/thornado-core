package types

import (
	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

type VaultSuiteV2 struct{}

var _ = Suite(&VaultSuiteV2{})

func (s *VaultSuiteV2) TestVaultV2(c *C) {
	vault := Vault{}
	c.Check(vault.IsEmpty(), Equals, true)
	c.Check(vault.Valid(), NotNil)

	poolPk, err := common.NewPubKey("tthorpub1addwnpepqvwfjs23krpfteqgqecmaw2hek54y6uy99jrs2utgfhzu2a48wfhkw4xen7")
	c.Check(err, IsNil)

	poolPkEddsa, err := common.NewPubKey("tthorpub1zcjduepqnd6pdwxvzu35dh60d256sewc5xczlxa35jrzq4uyd95x5476afrs9lzd5u")

	c.Check(err, IsNil)
	vault = NewVaultV2(12, VaultStatus_ActiveVault, VaultType_AsgardVault, poolPk, common.Chains{common.ETHChain, common.SOLChain}.Strings(), []ChainContract{}, poolPkEddsa)

	// Test getting ECDSA Address
	ethAddr, err := vault.GetAddress(common.ETHChain)
	c.Check(err, IsNil)
	c.Check(ethAddr.String(), Equals, "0x39da14bfdcfb127766bdbe0c335ef1bb8bce77a2")

	// Test Getting EdDSA Address
	solAddr, err := vault.GetAddress(common.SOLChain)
	c.Check(err, IsNil)
	c.Check(solAddr.String(), Equals, "BTps2v7MvutCD9CEQhETk2ArNgyaaoRpn4SpVvb4dZGJ")
}
