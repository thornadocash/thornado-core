package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type HandlerConsolidateSuite struct{}

var _ = Suite(&HandlerConsolidateSuite{})

func (s *HandlerConsolidateSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// makeValidObservedTx creates a valid ObservedTx for consolidation (same from/to address).
func makeValidObservedTx(pk common.PubKey) common.ObservedTx {
	addr := GetRandomETHAddress()
	return common.ObservedTx{
		Tx: common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			FromAddress: addr,
			ToAddress:   addr,
			Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
			Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
			Memo:        "consolidate",
		},
		BlockHeight:    1,
		ObservedPubKey: pk,
		FinaliseHeight: 1,
	}
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_InvalidMessage(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	result, err := h.Run(ctx, &MsgMimir{})
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err, Equals, errInvalidMessage)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_ValidateBasicFails(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Empty signer should fail ValidateBasic
	msg := &MsgConsolidate{
		ObservedTx: common.ObservedTx{
			Tx: common.Tx{
				ID:          GetRandomTxHash(),
				Chain:       common.ETHChain,
				FromAddress: GetRandomETHAddress(),
				ToAddress:   GetRandomETHAddress(),
				Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
				Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
				Memo:        "consolidate",
			},
			BlockHeight:    1,
			ObservedPubKey: GetRandomPubKey(),
			FinaliseHeight: 1,
		},
		Signer: cosmos.AccAddress{},
	}
	result, err := h.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)

	// Invalid observed tx (empty pubkey) should fail
	msg2 := NewMsgConsolidate(common.ObservedTx{
		Tx: common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			FromAddress: GetRandomETHAddress(),
			ToAddress:   GetRandomETHAddress(),
			Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
			Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
			Memo:        "consolidate",
		},
		BlockHeight:    1,
		FinaliseHeight: 1,
		// ObservedPubKey is empty
	}, GetRandomBech32Addr())
	result2, err2 := h.Run(ctx, msg2)
	c.Assert(err2, NotNil)
	c.Assert(result2, IsNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_NoSlash_SameAddressAsgardVault(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Set up an Asgard vault
	pk := GetRandomPubKey()
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, pk, []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Create consolidation tx with same from/to address
	obsTx := makeValidObservedTx(pk)
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	result, err := h.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_Slash_DifferentAddresses(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Set up an Asgard vault with membership so SlashVault can work
	pk := GetRandomPubKey()
	na := GetRandomValidatorNode(NodeActive)
	vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, pk, []string{
		common.ETHChain.String(),
	}, nil)
	vault.Membership = []string{na.PubKeySet.Secp256k1.String()}
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	// Set up ETH pool so SlashVault has something to work with
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Different from/to addresses → should slash
	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			FromAddress: GetRandomETHAddress(),
			ToAddress:   GetRandomETHAddress(), // different address
			Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
			Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
			Memo:        "consolidate",
		},
		BlockHeight:    1,
		ObservedPubKey: pk,
		FinaliseHeight: 1,
	}
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	// Should succeed even though slashing happens (SlashVault returns nil with proper setup)
	result, err := h.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_Slash_NonAsgardVault(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Set up a non-Asgard vault (UnknownVault type)
	pk := GetRandomPubKey()
	na := GetRandomValidatorNode(NodeActive)
	vault := NewVault(ctx.BlockHeight(), ActiveVault, UnknownVault, pk, []string{
		common.ETHChain.String(),
	}, nil)
	vault.Membership = []string{na.PubKeySet.Secp256k1.String()}
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)

	// Set up ETH pool
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Same address but non-Asgard vault → should slash
	obsTx := makeValidObservedTx(pk)
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	result, err := h.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_VaultNotFound(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Use a pubkey that has no vault stored → GetVault will error
	pk := GetRandomPubKey()
	obsTx := makeValidObservedTx(pk)
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	// Same addresses, vault not found → shouldSlash stays false, no error
	result, err := h.Run(ctx, msg)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_SlashFails(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Don't set up a vault → SlashVault will fail on GetVault
	// Use different addresses so shouldSlash = true
	pk := GetRandomPubKey()
	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			FromAddress: GetRandomETHAddress(),
			ToAddress:   GetRandomETHAddress(), // different address
			Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
			Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
			Memo:        "consolidate",
		},
		BlockHeight:    1,
		ObservedPubKey: pk,
		FinaliseHeight: 1,
	}
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	// shouldSlash = true (different addresses), SlashVault fails (no vault) → returns error
	result, err := h.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
}

func (s *HandlerConsolidateSuite) TestConsolidateHandler_DiffAddressAndVaultNotFound(c *C) {
	ctx, mgr := setupManagerForTest(c)
	h := NewConsolidateHandler(mgr)

	// Different addresses + vault not found
	// shouldSlash = true from addresses, GetVault fails → shouldSlash stays true
	// slash called → SlashVault fails → error
	pk := GetRandomPubKey()
	obsTx := common.ObservedTx{
		Tx: common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			FromAddress: GetRandomETHAddress(),
			ToAddress:   GetRandomETHAddress(),
			Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100))),
			Gas:         common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(37500))},
			Memo:        "consolidate",
		},
		BlockHeight:    1,
		ObservedPubKey: pk,
		FinaliseHeight: 1,
	}
	msg := NewMsgConsolidate(obsTx, GetRandomBech32Addr())

	result, err := h.Run(ctx, msg)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
}
