package thorchain

import (
	"fmt"

	math "cosmossdk.io/math"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type MigrationHelpersTestSuite struct{}

var _ = Suite(&MigrationHelpersTestSuite{})

func (MigrationHelpersTestSuite) TestUnsafeAddRefundOutbound(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)

	// add a vault
	vault := GetRandomVault()
	vault.Coins = common.Coins{
		// common.NewCoin(common.RuneAsset(), cosmos.NewUint(10000*common.One)),
		common.NewCoin(common.ETHAsset, cosmos.NewUint(10000*common.One)),
	}
	c.Assert(w.keeper.SetVault(w.ctx, vault), IsNil)

	// add node
	acc1 := GetRandomValidatorNode(NodeActive)
	c.Assert(w.keeper.SetNodeAccount(w.ctx, acc1), IsNil)

	// Create inbound
	inTxID := GetRandomTxHash()
	ethAddr := GetRandomETHAddress()
	vaultAddr, err := vault.PubKey.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)
	coin := common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))
	height := w.ctx.BlockHeight()

	tx := common.Tx{
		ID:          inTxID,
		Chain:       common.ETHChain,
		FromAddress: ethAddr,
		ToAddress:   vaultAddr,
		Coins:       common.Coins{coin},
		Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(1))},
		Memo:        "bad memo",
	}

	voter := NewObservedTxVoter(inTxID, ObservedTxs{
		ObservedTx{
			Tx:             tx,
			Status:         common.Status_incomplete,
			BlockHeight:    1,
			Signers:        []string{w.activeNodeAccount.NodeAddress.String(), acc1.NodeAddress.String()},
			KeysignMs:      0,
			FinaliseHeight: 1,
		},
	})
	w.keeper.SetObservedTxInVoter(w.ctx, voter)

	mgr, ok := w.mgr.(*Mgrs)
	c.Assert(ok, Equals, true)

	// add outbound using migration helper
	err = unsafeAddRefundOutbound(w.ctx, mgr, inTxID.String(), ethAddr.String(), coin, height)
	c.Assert(err, IsNil)

	items, err := w.mgr.TxOutStore().GetOutboundItems(w.ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 1)
	c.Assert(items[0].Chain, Equals, common.ETHChain)
	c.Assert(items[0].InHash.Equals(inTxID), Equals, true)
	c.Assert(items[0].ToAddress.Equals(ethAddr), Equals, true)
	c.Assert(items[0].VaultPubKey.Equals(vault.PubKey), Equals, true)
	c.Assert(items[0].Coin.Equals(coin), Equals, true)
}

func (MigrationHelpersTestSuite) TestGetTCYClaimsFromData(c *C) {
	claimer, err := getTCYClaimsFromData()
	c.Assert(err, IsNil)
	c.Assert(len(claimer), Equals, 4)
	c.Assert(claimer[0].Amount.Equal(math.NewUint(1111111111111111111)), Equals, true)
	c.Assert(claimer[0].Asset.Equals(common.AVAXAsset), Equals, true)
	address, err := common.NewAddress("0x00112c24ebee9c96d177a3aa2ff55dcb93a53c80")
	c.Assert(err, IsNil)
	c.Assert(claimer[0].L1Address.Equals(address), Equals, true)

	c.Assert(claimer[1].Amount.Equal(math.NewUint(42047799234002)), Equals, true)
	c.Assert(claimer[1].Asset.Equals(common.BTCAsset), Equals, true)
	address, err = common.NewAddress("bc1qj937cw6xap470hg427p2kxl85u3ac3ps84hye3")
	c.Assert(err, IsNil)
	c.Assert(claimer[1].L1Address.Equals(address), Equals, true)

	c.Assert(claimer[2].Amount.Equal(math.NewUint(1111)), Equals, true)
	c.Assert(claimer[2].Asset.Equals(common.DOGEAsset), Equals, true)
	address, err = common.NewAddress("tthor17gw75axcnr8747pkanye45pnrwk7p9c3uhzgff")
	c.Assert(err, IsNil)
	c.Assert(claimer[2].L1Address.Equals(address), Equals, true)

	c.Assert(claimer[3].Amount.Equal(math.NewUint(1111)), Equals, true)
	c.Assert(claimer[3].Asset.Equals(common.DOGEAsset), Equals, true)
	address, err = common.NewAddress("tthor17gw75axcnr8747pkanye45pnrwk7p9c3uhzgff")
	c.Assert(err, IsNil)
	c.Assert(claimer[3].L1Address.Equals(address), Equals, true)
}

func (MigrationHelpersTestSuite) TestSetTCYClaims(c *C) {
	ctx, mgr := setupManagerForTest(c)

	address, err := common.NewAddress("0x00112c24ebee9c96d177a3aa2ff55dcb93a53c80")
	c.Assert(err, IsNil)
	_, err = mgr.Keeper().GetTCYClaimer(ctx, address, common.AVAXAsset)
	c.Assert(err.Error(), Equals, fmt.Sprintf("TCYClaimer doesn't exist: %s", address.String()))

	address2, err := common.NewAddress("bc1qj937cw6xap470hg427p2kxl85u3ac3ps84hye3")
	c.Assert(err, IsNil)
	_, err = mgr.Keeper().GetTCYClaimer(ctx, address2, common.BTCAsset)
	c.Assert(err.Error(), Equals, fmt.Sprintf("TCYClaimer doesn't exist: %s", address2.String()))

	address3, err := common.NewAddress("tthor17gw75axcnr8747pkanye45pnrwk7p9c3uhzgff")
	c.Assert(err, IsNil)
	_, err = mgr.Keeper().GetTCYClaimer(ctx, address3, common.DOGEAsset)
	c.Assert(err.Error(), Equals, fmt.Sprintf("TCYClaimer doesn't exist: %s", address3.String()))

	c.Assert(setTCYClaims(ctx, mgr), IsNil)

	claimer, err := mgr.Keeper().GetTCYClaimer(ctx, address, common.AVAXAsset)
	c.Assert(err, IsNil)
	c.Assert(claimer.Amount.Equal(math.NewUint(1111111111111111111)), Equals, true)
	c.Assert(claimer.Asset.Equals(common.AVAXAsset), Equals, true)
	c.Assert(claimer.L1Address.Equals(address), Equals, true)

	claimer, err = mgr.Keeper().GetTCYClaimer(ctx, address2, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Assert(claimer.Amount.Equal(math.NewUint(42047799234002)), Equals, true)
	c.Assert(claimer.Asset.Equals(common.BTCAsset), Equals, true)
	c.Assert(claimer.L1Address.Equals(address2), Equals, true)

	claimer, err = mgr.Keeper().GetTCYClaimer(ctx, address3, common.DOGEAsset)
	c.Assert(err, IsNil)
	c.Assert(claimer.Amount.Equal(math.NewUint(2222)), Equals, true)
	c.Assert(claimer.Asset.Equals(common.DOGEAsset), Equals, true)
	c.Assert(claimer.L1Address.Equals(address3), Equals, true)
}
