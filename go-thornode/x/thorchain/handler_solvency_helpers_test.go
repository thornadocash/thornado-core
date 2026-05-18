package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	. "gopkg.in/check.v1"
)

type HandlerSolvencyHelpersSuite struct{}

var _ = Suite(&HandlerSolvencyHelpersSuite{})

func (s *HandlerSolvencyHelpersSuite) SetUpSuite(c *C) {
	SetupConfigForTest()
}

// setupSolvencyHelpersTest creates common test fixtures for solvency helper tests.
func setupSolvencyHelpersTest(c *C) (cosmos.Context, Manager, NodeAccounts, Vault) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(1024)

	// 4 active nodes, consensus needs 2 (1/3 of 4 = 1.33, rounded up = 2)
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}
	for _, na := range nas {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)
	}

	vault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Set up ETH pool: 1000 ETH = 10000 RUNE (1 ETH = 10 RUNE)
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Set RUNE price ($5/RUNE)
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	return ctx, mgr, nas, vault
}

// bringVoterToConsensus signs a solvency voter with enough nodes to reach consensus.
func bringVoterToConsensus(voter *keeper.SolvencyVoter, nas NodeAccounts) {
	voter.Sign(nas[0].NodeAddress)
	voter.Sign(nas[1].NodeAddress)
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_DuplicateSigner(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	// First sign succeeds
	err := processSolvencyAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, sol, true)
	c.Assert(err, IsNil)

	// Duplicate sign with shouldSlashForDuplicate=true should slash but not error
	err = processSolvencyAttestation(ctx, mgr, &voter, nas[0].NodeAddress, nas, sol, true)
	c.Assert(err, IsNil)
	pts, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, nas[0].NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(pts > 1, Equals, true) // extra slash for duplicate
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_StopSolvencyCheckMimir(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Set StopSolvencyCheck to a height before current block (active stop)
	mgr.Keeper().SetMimir(ctx, "StopSolvencyCheck", ctx.BlockHeight()-1)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	// Bring to consensus
	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Even though vault is insolvent, StopSolvencyCheck prevents halting
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(-1))
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_StopSolvencyCheckChainMimir(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Set chain-specific StopSolvencyCheck
	mgr.Keeper().SetMimir(ctx, "StopSolvencyCheckETH", ctx.BlockHeight()-1)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(-1))
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_HaltChainAlreadySet(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Set SolvencyHaltETHChain to current block height (already halted)
	mgr.Keeper().SetMimir(ctx, "SolvencyHaltETHChain", ctx.BlockHeight())

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Halt value should not have changed
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, ctx.BlockHeight())
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_HaltChainManuallyHalted(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Set SolvencyHaltETHChain to 1 (indefinite manual halt)
	mgr.Keeper().SetMimir(ctx, "SolvencyHaltETHChain", 1)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Should stay at 1 (manually halted)
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(1))
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_SolvencyHeightBelowLastChainHeight(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Set last chain height higher than solvency msg height
	c.Assert(mgr.Keeper().SetLastChainHeight(ctx, common.ETHChain, 2000), IsNil)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024, // lower than lastChainHeight=2000
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Should not halt because solvency height is stale
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(-1))
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_GetVaultError(c *C) {
	ctx, mgr, nas, _ := setupSolvencyHelpersTest(c)

	// Use a pubkey that has no vault stored
	unknownPK := GetRandomPubKey()
	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, unknownPK,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get vault.*")
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_AutoUnhalt(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Chain was previously halted at block 500 (earlier than current 1024)
	mgr.Keeper().SetMimir(ctx, "SolvencyHaltETHChain", 500)

	// Vault has 1000 ETH, wallet also reports 1000 ETH → solvent
	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Chain should be auto-unhalted (mimir set to 0)
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(0))
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_EmptyCoin(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Add an empty coin to vault
	vault.Coins = append(vault.Coins, common.NewCoin(common.BTCAsset, cosmos.ZeroUint()))
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	walletCoins := common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One)))

	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.ETHChain)
	c.Assert(result, Equals, false)
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_WalletCoinEmpty(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Wallet has no ETH at all → gap = vault amount
	walletCoins := common.Coins{}

	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.ETHChain)
	c.Assert(result, Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_GapWithinPermittedRange(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Vault has 1000 ETH, wallet has 999.99 ETH → small gap
	walletCoins := common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(999_99000000)))

	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.ETHChain)
	c.Assert(result, Equals, false)
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_NoPoolLiquidity(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Replace ETH pool with empty pool
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.ZeroUint()
	ethPool.BalanceRune = cosmos.ZeroUint()
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Wallet has less ETH than vault
	walletCoins := common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(500*common.One)))

	// Pool has no liquidity → gap can't be valued in RUNE → treat as insolvent (conservative)
	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.ETHChain)
	c.Assert(result, Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_WalletMoreThanVault(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Wallet has more than vault → no gap
	walletCoins := common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(2000*common.One)))

	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.ETHChain)
	c.Assert(result, Equals, false)
}

func (s *HandlerSolvencyHelpersSuite) TestInsolvencyCheck_DifferentChainCoinSkipped(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Check BTC chain insolvency - vault has no BTC coins
	walletCoins := common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(100*common.One)))

	result := insolvencyCheck(ctx, mgr, vault, walletCoins, common.BTCChain)
	c.Assert(result, Equals, false)
}

func (s *HandlerSolvencyHelpersSuite) TestDeductVaultBlockPendingOutbound_NoMatchingVault(c *C) {
	vault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))

	block := &TxOut{
		Height: 1024,
		TxArray: []TxOutItem{
			{
				Chain:       common.ETHChain,
				VaultPubKey: GetRandomPubKey(), // different vault
				Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
			},
		},
	}

	result := deductVaultBlockPendingOutbound(vault, block)
	// Balance should be unchanged since VaultPubKey doesn't match
	ethCoin := result.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.NewUint(1000*common.One)), Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestDeductVaultBlockPendingOutbound_AlreadySent(c *C) {
	vault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))

	block := &TxOut{
		Height: 1024,
		TxArray: []TxOutItem{
			{
				Chain:       common.ETHChain,
				VaultPubKey: vault.PubKey,
				Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
				OutHash:     GetRandomTxHash(), // already sent
			},
		},
	}

	result := deductVaultBlockPendingOutbound(vault, block)
	// Balance should be unchanged since tx already has OutHash
	ethCoin := result.Coins.GetCoin(common.ETHAsset)
	c.Assert(ethCoin.Amount.Equal(cosmos.NewUint(1000*common.One)), Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestDeductVaultBlockPendingOutbound_WithGas(c *C) {
	vault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), []string{
		common.ETHChain.String(),
	}, nil)
	vault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))

	block := &TxOut{
		Height: 1024,
		TxArray: []TxOutItem{
			{
				Chain:       common.ETHChain,
				VaultPubKey: vault.PubKey,
				Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
				MaxGas: common.Gas{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(1*common.One)),
				},
			},
		},
	}

	result := deductVaultBlockPendingOutbound(vault, block)
	// Should deduct both the coin amount and gas
	ethCoin := result.Coins.GetCoin(common.ETHAsset)
	expected := cosmos.NewUint(899 * common.One) // 1000 - 100 - 1
	c.Assert(ethCoin.Amount.Equal(expected), Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestExcludePendingOutboundFromVault_Success(c *C) {
	ctx, mgr, _, vault := setupSolvencyHelpersTest(c)

	// Set a pending outbound at a height within the signing period (before current block)
	prevHeight := ctx.BlockHeight() - 1
	txOut := NewTxOut(prevHeight)
	txOut.TxArray = []TxOutItem{
		{
			Chain:       common.ETHChain,
			VaultPubKey: vault.PubKey,
			Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(50*common.One)),
		},
	}
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	adjusted, err := excludePendingOutboundFromVault(ctx, mgr, vault)
	c.Assert(err, IsNil)
	ethCoin := adjusted.Coins.GetCoin(common.ETHAsset)
	expected := cosmos.NewUint(950 * common.One) // 1000 - 50
	c.Assert(ethCoin.Amount.Equal(expected), Equals, true)

	// Original vault coins should not be mutated
	originalETH := vault.Coins.GetCoin(common.ETHAsset)
	c.Assert(originalETH.Amount.Equal(cosmos.NewUint(1000*common.One)), Equals, true)
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_InsolventButAlreadyHalted(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// Chain was previously halted at block 500 (haltChain > 0, but < blockHeight)
	mgr.Keeper().SetMimir(ctx, "SolvencyHaltETHChain", 500)

	// Vault has 1000 ETH, wallet only has 100 ETH → insolvent
	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Halt value should be refreshed to the current block height to prevent
	// a different solvent vault from unhalting the chain prematurely.
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, ctx.BlockHeight())
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_InsolventAndUnhalted(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	// No halt set (default -1)

	// Vault has 1000 ETH, wallet only has 100 ETH → insolvent
	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 0

	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Should halt the chain
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, ctx.BlockHeight())
}

func (s *HandlerSolvencyHelpersSuite) TestProcessSolvencyAttestation_ConsensusAlreadyReached(c *C) {
	ctx, mgr, nas, vault := setupSolvencyHelpersTest(c)

	voter := NewSolvencyVoter(
		GetRandomTxHash(), common.ETHChain, vault.PubKey,
		common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))),
		1024,
	)
	sol := &common.Solvency{
		Id: voter.Id, Chain: voter.Chain, PubKey: voter.PubKey, Coins: voter.Coins, Height: voter.Height,
	}

	// Mark consensus already reached
	bringVoterToConsensus(&voter, nas)
	voter.ConsensusBlockHeight = 100

	// Late observer within flex period should get slash points decremented
	err := processSolvencyAttestation(ctx, mgr, &voter, nas[2].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)

	// Late observer beyond flex period
	ctx = ctx.WithBlockHeight(voter.ConsensusBlockHeight + 100)
	err = processSolvencyAttestation(ctx, mgr, &voter, nas[3].NodeAddress, nas, sol, false)
	c.Assert(err, IsNil)
}
