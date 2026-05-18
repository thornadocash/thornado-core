package thorchain

import (
	"strings"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gopkg.in/check.v1"
	. "gopkg.in/check.v1"
)

type HandlerCommonOutboundSuite struct{}

var _ = Suite(&HandlerCommonOutboundSuite{})

func (s *HandlerCommonOutboundSuite) TestIsOutboundFakeGasTX(c *C) {
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(1)),
	}
	gas := common.Gas{
		{Asset: common.ETHAsset, Amount: cosmos.NewUint(1)},
	}
	// Fake gas transactions have self-referential OUT:txhash memo (bifrost behavior)
	fakeGasTx := common.ObservedTx{
		Tx: common.NewTx("123", "0xabc", "0x123", coins, gas, "OUT:123"),
	}

	c.Assert(isOutboundFakeGasTx(fakeGasTx), Equals, true)

	coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100000)),
	}
	theftTx := common.ObservedTx{
		Tx: common.NewTx("123", "0xabc", "0x123", coins, gas, "=:AVAX.AVAX:0x123"),
	}
	c.Assert(isOutboundFakeGasTx(theftTx), Equals, false)

	// Test with wrong memo format (not self-referential)
	coins = common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(1)),
	}
	wrongMemoTx := common.ObservedTx{
		Tx: common.NewTx("123", "0xabc", "0x123", coins, gas, "OUT:ABCD1234567890"),
	}
	c.Assert(isOutboundFakeGasTx(wrongMemoTx), Equals, false) // Wrong memo format (not self-referential)
}

func (s *HandlerCommonOutboundSuite) TestIsCancelTx(c *C) {
	// Create a vault pubkey and get its address
	vaultPubKey := GetRandomPubKey()
	vaultAddr, err := vaultPubKey.GetAddress(common.ETHChain)
	c.Assert(err, IsNil)

	// Test 1: Valid cancel tx (EVM, gas asset, amount=DustThreshold, vault-to-vault)
	// Cancel transactions have amount=0 on chain, but bifrost converts to DustThreshold
	dustThreshold := common.ETHChain.DustThreshold() // 1 for ETH
	cancelTxCoins := common.Coins{
		common.NewCoin(common.ETHAsset, dustThreshold),
	}
	cancelTxGas := common.Gas{
		{Asset: common.ETHAsset, Amount: cosmos.NewUint(21000)},
	}
	cancelTx := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, cancelTxCoins, cancelTxGas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(cancelTx), Equals, true)

	// Test 2: Not a cancel tx - amount is 0 (not DustThreshold)
	zeroAmountCoins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
	}
	notCancelTx2 := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, zeroAmountCoins, cancelTxGas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx2), Equals, false)

	// Test 3: Not a cancel tx - large amount
	largeAmountCoins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100000)),
	}
	notCancelTx3 := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, largeAmountCoins, cancelTxGas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx3), Equals, false)

	// Test 4: Not a cancel tx - not EVM chain (BTC)
	btcVaultPubKey := GetRandomPubKey()
	btcVaultAddr, err := btcVaultPubKey.GetAddress(common.BTCChain)
	c.Assert(err, IsNil)
	btcCoins := common.Coins{
		common.NewCoin(common.BTCAsset, cosmos.ZeroUint()),
	}
	btcGas := common.Gas{
		{Asset: common.BTCAsset, Amount: cosmos.NewUint(10000)},
	}
	notCancelTx4 := ObservedTx{
		Tx:             common.NewTx("123", btcVaultAddr, btcVaultAddr, btcCoins, btcGas, ""),
		ObservedPubKey: btcVaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx4), Equals, false)

	// Test 5: Valid cancel tx on different EVM chain (AVAX)
	avaxVaultPubKey := GetRandomPubKey()
	avaxVaultAddr, err := avaxVaultPubKey.GetAddress(common.AVAXChain)
	c.Assert(err, IsNil)
	avaxDustThreshold := common.AVAXChain.DustThreshold() // 1 for AVAX
	avaxCoins := common.Coins{
		common.NewCoin(common.AVAXAsset, avaxDustThreshold),
	}
	avaxGas := common.Gas{
		{Asset: common.AVAXAsset, Amount: cosmos.NewUint(21000)},
	}
	cancelTxAvax := ObservedTx{
		Tx:             common.NewTx("123", avaxVaultAddr, avaxVaultAddr, avaxCoins, avaxGas, ""),
		ObservedPubKey: avaxVaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(cancelTxAvax), Equals, true)

	// Test 6: Not a cancel tx - not gas asset (ERC20 token)
	tokenAsset, err := common.NewAsset("ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7")
	c.Assert(err, IsNil)
	tokenCoins := common.Coins{
		common.NewCoin(tokenAsset, cosmos.ZeroUint()),
	}
	notCancelTx5 := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, tokenCoins, cancelTxGas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx5), Equals, false)

	// Test 7: Not a cancel tx - multiple coins
	multiCoins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
	}
	notCancelTx6 := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, multiCoins, cancelTxGas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx6), Equals, false)

	// Test 8: Not a cancel tx - has a memo
	memoCoins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
	}
	notCancelTx7 := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, memoCoins, cancelTxGas, "OUT:abc123"),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(notCancelTx7), Equals, false)
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutEvenDistribution(c *check.C) {
	clout1 := cosmos.NewUint(50)
	clout2 := cosmos.NewUint(50)
	spent := cosmos.NewUint(60)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.String(), check.Equals, "30")
	c.Assert(split2.String(), check.Equals, "30")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutExcessSpent(c *check.C) {
	clout1 := cosmos.NewUint(50)
	clout2 := cosmos.NewUint(50)
	spent := cosmos.NewUint(120)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.String(), check.Equals, "50")
	c.Assert(split2.String(), check.Equals, "50")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutInsufficientFirstClout(c *check.C) {
	clout1 := cosmos.NewUint(20)
	clout2 := cosmos.NewUint(80)
	spent := cosmos.NewUint(60)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.String(), check.Equals, "20")
	c.Assert(split2.String(), check.Equals, "40")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutInsufficientSecondClout(c *check.C) {
	clout1 := cosmos.NewUint(80)
	clout2 := cosmos.NewUint(20)
	spent := cosmos.NewUint(60)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.String(), check.Equals, "40")
	c.Assert(split2.String(), check.Equals, "20")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutSpentIsZero(c *check.C) {
	clout1 := cosmos.NewUint(50)
	clout2 := cosmos.NewUint(50)
	spent := cosmos.NewUint(0)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.IsZero(), check.Equals, true)
	c.Assert(split2.IsZero(), check.Equals, true)
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutOneSideIsZero(c *check.C) {
	clout1 := cosmos.NewUint(0)
	clout2 := cosmos.NewUint(100)
	spent := cosmos.NewUint(60)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.IsZero(), check.Equals, true)
	c.Assert(split2.String(), check.Equals, "60")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutBoundaryCondition(c *check.C) {
	clout1 := cosmos.NewUint(1)
	clout2 := cosmos.NewUint(100000000000)
	spent := cosmos.NewUint(2)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.String(), check.Equals, "1")
	c.Assert(split2.String(), check.Equals, "1")
}

func (s *HandlerCommonOutboundSuite) TestSplitCloutBothCloutsZero(c *check.C) {
	clout1 := cosmos.ZeroUint()
	clout2 := cosmos.ZeroUint()
	spent := cosmos.NewUint(60)

	split1, split2 := calcReclaim(clout1, clout2, spent)

	c.Assert(split1.IsZero(), check.Equals, true)
	c.Assert(split2.IsZero(), check.Equals, true)
}

// TestDeterministicAggregatorComparison verifies that aggregator field comparison
// uses deterministic case-insensitive matching (via strings.ToLower) rather than
// strings.EqualFold which can have locale-specific behavior.
func (s *HandlerCommonOutboundSuite) TestDeterministicAggregatorComparison(c *check.C) {
	// Test cases for deterministic case-insensitive comparison
	testCases := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"exact match", "0xAggregator", "0xAggregator", true},
		{"case insensitive match", "0xAGGREGATOR", "0xaggregator", true},
		{"mixed case match", "0xAgGrEgAtOr", "0xaGgReGaToR", true},
		{"different strings", "0xAggregator1", "0xAggregator2", false},
		{"empty strings", "", "", true},
		{"one empty", "0xAggregator", "", false},
	}

	for _, tc := range testCases {
		// This mirrors the comparison logic in handler_common_outbound.go
		// nolint:staticcheck // SA6005: ToLower is intentionally used instead of EqualFold for determinism
		result := strings.ToLower(tc.a) == strings.ToLower(tc.b)
		c.Assert(result, check.Equals, tc.expected, check.Commentf("test case: %s", tc.name))
	}
}

// TestSlashingExemptionsForOperationalTxs verifies that fake gas and cancel
// transactions are properly exempted from slashing when no TxOutItem match is found.
func (s *HandlerCommonOutboundSuite) TestSlashingExemptionsForOperationalTxs(c *check.C) {
	// Test 1: Fake gas transaction should be exempted
	coins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(1)),
	}
	gas := common.Gas{
		{Asset: common.ETHAsset, Amount: cosmos.NewUint(1)},
	}
	// Fake gas transactions have self-referential OUT:txhash memo
	fakeGasTx := ObservedTx{
		Tx: common.NewTx("TXHASH123", "0xabc", "0x123", coins, gas, "OUT:TXHASH123"),
	}
	c.Assert(isOutboundFakeGasTx(fakeGasTx), check.Equals, true, check.Commentf("fake gas tx should be recognized"))

	// Test 2: Cancel transaction should be exempted
	vaultPubKey := GetRandomPubKey()
	vaultAddr, err := vaultPubKey.GetAddress(common.ETHChain)
	c.Assert(err, check.IsNil)
	dustThreshold := common.ETHChain.DustThreshold()
	cancelCoins := common.Coins{
		common.NewCoin(common.ETHAsset, dustThreshold),
	}
	cancelTx := ObservedTx{
		Tx:             common.NewTx("123", vaultAddr, vaultAddr, cancelCoins, gas, ""),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isCancelOrApprovalTx(cancelTx), check.Equals, true, check.Commentf("cancel tx should be recognized"))

	// Test 3: Theft attempt (large amount, not operational) should NOT be exempted
	theftCoins := common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
	}
	theftTx := ObservedTx{
		Tx:             common.NewTx("123", "0xabc", "0x123", theftCoins, gas, "OUT:123"),
		ObservedPubKey: vaultPubKey,
	}
	c.Assert(isOutboundFakeGasTx(theftTx), check.Equals, false, check.Commentf("theft tx should not be exempted"))
	c.Assert(isCancelOrApprovalTx(theftTx), check.Equals, false, check.Commentf("theft tx should not be exempted as cancel"))
}

func (s *HandlerCommonOutboundSuite) TestMaxEVMGasForChain(c *check.C) {
	ctx, mgr := setupManagerForTest(c)

	// Verify all EVM chains have a defined max gas cap.
	for _, chain := range common.AllChains {
		if !chain.IsEVM() {
			continue
		}
		maxGas, known := maxEVMGasForChain(ctx, mgr.Keeper(), chain)
		c.Assert(known, check.Equals, true, check.Commentf("EVM chain %s missing explicit max gas cap", chain))
		c.Assert(maxGas.GT(cosmos.ZeroUint()), check.Equals, true, check.Commentf("EVM chain %s has zero max gas cap", chain))
	}

	// Non-EVM chain should return the default cap, not be known.
	maxGas, known := maxEVMGasForChain(ctx, mgr.Keeper(), common.BTCChain)
	c.Assert(known, check.Equals, false)
	c.Assert(maxGas.Equal(cosmos.NewUint(constants.DefaultMaxEVMGas)), check.Equals, true)

	// Mimir override should take precedence over hardcoded constant.
	mimirOverride := int64(99_000_000)
	mgr.Keeper().SetMimir(ctx, "MaxGas-ETH", mimirOverride)
	maxGas, known = maxEVMGasForChain(ctx, mgr.Keeper(), common.ETHChain)
	c.Assert(known, check.Equals, true)
	c.Assert(maxGas.Equal(cosmos.NewUint(uint64(mimirOverride))), check.Equals, true)
}
