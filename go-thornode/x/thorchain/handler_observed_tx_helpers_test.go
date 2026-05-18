package thorchain

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type HandlerObservedTxHelpersSuite struct{}

var _ = Suite(&HandlerObservedTxHelpersSuite{})

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoID(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test with 8-decimal asset (BTC)
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Create observed tx with specific amount
	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))

	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "56789") // last 5 digits of 123456789

	// Test with larger amount
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(987654321)))
	refID, err = generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "54321") // last 5 digits of 987654321

	// Test with amount less than 5 digits
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123)))
	refID, err = generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "00123") // padded to 5 digits
}

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoIDWithDifferentDecimals(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test with 6-decimal asset (simulate GAIA)
	gaiaAsset := common.Asset{Chain: common.GAIAChain, Symbol: "ATOM", Ticker: "ATOM", Synth: false}
	gaiaPool := NewPool()
	gaiaPool.Asset = gaiaAsset
	gaiaPool.Decimals = 6 // GAIA has 6 decimals
	gaiaPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	gaiaPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, gaiaPool), IsNil)

	// For 6 decimals, amount should be divided by 100 (10^(8-6))
	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(gaiaAsset, cosmos.NewUint(123456780000))) // 1234.56780000 in 6-decimal format

	refID, err := generateReferenceMemoID(ctx, mgr, gaiaAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "67800") // 123456780000 / 100 = 1234567800, last 5 digits = 67800

	// Test with 7-decimal asset
	customAsset := common.Asset{Chain: common.ETHChain, Symbol: "CUSTOM", Ticker: "CUSTOM", Synth: false}
	customPool := NewPool()
	customPool.Asset = customAsset
	customPool.Decimals = 7
	customPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	customPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, customPool), IsNil)

	// For 7 decimals, amount should be divided by 10 (10^(8-7))
	tx.Tx.Coins = common.NewCoins(common.NewCoin(customAsset, cosmos.NewUint(123456780000)))
	refID, err = generateReferenceMemoID(ctx, mgr, customAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "78000") // 123456780000 / 10 = 12345678000, last 5 digits = 78000
}

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoIDWithZeroPoolDecimals(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// For non-gas assets, pool decimals of zero means THORChain default precision (1e8).
	asset := common.Asset{Chain: common.ETHChain, Symbol: "TOKEN", Ticker: "TOKEN", Synth: false}
	pool := NewPool()
	pool.Asset = asset
	pool.Decimals = 0
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(asset, cosmos.NewUint(626500002)))

	refID, err := generateReferenceMemoID(ctx, mgr, asset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "00002")
}

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoIDErrorCases(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test with empty coins
	emptyTx := GetRandomObservedTx()
	emptyTx.Tx.Coins = common.NewCoins()
	_, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, emptyTx)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*no coins.*")

	// Test with zero amount
	zeroTx := GetRandomObservedTx()
	zeroTx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.ZeroUint()))
	_, err = generateReferenceMemoID(ctx, mgr, common.BTCAsset, zeroTx)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*zero amount.*")

	// Test with empty asset
	normalTx := GetRandomObservedTx()
	normalTx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))
	_, err = generateReferenceMemoID(ctx, mgr, common.EmptyAsset, normalTx)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*asset is empty.*")
}

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoIDDecimalPrecision(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Test various decimal combinations
	testCases := []struct {
		decimals int64
		amount   uint64
		expected string
		desc     string
	}{
		{8, 123456789, "56789", "8 decimals, no adjustment"},
		{6, 123456780000, "67800", "6 decimals, divide by 100"},
		{7, 123456780000, "78000", "7 decimals, divide by 10"},
		{5, 123456780000, "56780", "5 decimals, divide by 1000"},
		{4, 123456780000, "45678", "4 decimals, divide by 10000"},
	}

	for i, tc := range testCases {
		// Create custom asset for each test case
		asset := common.Asset{
			Chain:  common.ETHChain,
			Symbol: common.Symbol("TEST" + string(rune('A'+i))),
			Ticker: common.Ticker("TEST" + string(rune('A'+i))),
			Synth:  false,
		}

		pool := NewPool()
		pool.Asset = asset
		pool.Decimals = tc.decimals
		pool.BalanceAsset = cosmos.NewUint(100 * common.One)
		pool.BalanceRune = cosmos.NewUint(100 * common.One)
		c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

		tx := GetRandomObservedTx()
		tx.Tx.Coins = common.NewCoins(common.NewCoin(asset, cosmos.NewUint(tc.amount)))

		refID, err := generateReferenceMemoID(ctx, mgr, asset, tx)
		c.Assert(err, IsNil, Commentf("Test case: %s", tc.desc))
		c.Assert(refID, Equals, tc.expected, Commentf("Test case: %s", tc.desc))
	}
}

func (s *HandlerObservedTxHelpersSuite) TestGenerateReferenceMemoIDModulus(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Test that modulus operation works correctly for large numbers
	tx := GetRandomObservedTx()

	// Reference of zero is invalid.
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(9999999900000)))
	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, NotNil)
	c.Assert(refID, Equals, "")

	// Test with amount that results in exactly 99999
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(9999999999999)))
	refID, err = generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "99999") // 9999999999999 % 100000 = 99999
}

func (s *HandlerObservedTxHelpersSuite) TestReferenceMemoIntegration(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Create observed tx with empty memo
	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))

	// Generate reference memo ID
	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "56789")

	// Create ReferenceReadMemo and generate memo string
	refMemo := NewReferenceReadMemo(refID)
	memoStr := refMemo.CreateMemo()
	c.Assert(memoStr, Equals, "r:56789")

	// Create a reference memo in storage that matches our generated reference
	storedRefMemo := NewReferenceMemo(common.BTCAsset, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890", refID, 0)
	mgr.Keeper().SetReferenceMemo(ctx, storedRefMemo)

	// Test that fetchMemoFromReference can resolve our generated reference
	tx.Tx.Memo = memoStr
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, tx.Tx, 1) // tx observed at height 1, memo created at height 0
	c.Assert(resolvedMemo, Equals, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890")

	// Verify usage was tracked
	updatedRefMemo, err := mgr.Keeper().GetReferenceMemo(ctx, common.BTCAsset, refID)
	c.Assert(err, IsNil)
	c.Assert(updatedRefMemo.GetUsageCount(), Equals, int64(1))
	c.Assert(updatedRefMemo.HasBeenUsedBy(tx.Tx.ID), Equals, true)
}

func (s *HandlerObservedTxHelpersSuite) TestReferenceMemoIntegrationWithExpiredMemo(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100) // Current block height

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Set TTL for reference memos
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 50) // TTL of 50 blocks

	// Create observed tx
	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))

	// Generate reference memo ID
	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)

	// Create an expired reference memo (created 60 blocks ago)
	expiredRefMemo := NewReferenceMemo(common.BTCAsset, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890", refID, 40) // 40 + 50 < 100, so expired
	mgr.Keeper().SetReferenceMemo(ctx, expiredRefMemo)

	// Create memo string
	refMemo := NewReferenceReadMemo(refID)
	memoStr := refMemo.CreateMemo()

	// Test that fetchMemoFromReference returns empty string for expired memo
	testTx := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, memoStr)
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx, 50) // tx observed at height 50, memo created at height 40
	c.Assert(resolvedMemo, Equals, "")
}

func (s *HandlerObservedTxHelpersSuite) TestReferenceMemoIntegrationWithUsageLimit(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Set usage limit
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnMaxUse.String(), 2) // Max 2 uses

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Create observed tx
	tx := GetRandomObservedTx()
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(123456789)))

	// Generate reference memo ID
	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)

	// Create reference memo
	storedRefMemo := NewReferenceMemo(common.BTCAsset, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890", refID, 0)
	mgr.Keeper().SetReferenceMemo(ctx, storedRefMemo)

	// Create memo string
	refMemo := NewReferenceReadMemo(refID)
	memoStr := refMemo.CreateMemo()

	// First usage should succeed
	txID1, _ := common.NewTxID("1111111111111111111111111111111111111111111111111111111111111111")
	testTx := common.NewTx(txID1, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, memoStr)
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx, 1) // tx observed at height 1, memo created at height 0
	c.Assert(resolvedMemo, Equals, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890")

	// Second usage should succeed
	txID2, _ := common.NewTxID("2222222222222222222222222222222222222222222222222222222222222222")
	testTx2 := common.NewTx(txID2, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, memoStr)
	resolvedMemo = fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx2, 2) // tx observed at height 2, memo created at height 0
	c.Assert(resolvedMemo, Equals, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890")

	// Third usage should fail (exceed limit)
	txID3, _ := common.NewTxID("3333333333333333333333333333333333333333333333333333333333333333")
	testTx3 := common.NewTx(txID3, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, memoStr)
	resolvedMemo = fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx3, 3) // tx observed at height 3, memo created at height 0
	c.Assert(resolvedMemo, Equals, "")

	// Verify usage count is 3 (all attempts are tracked for audit purposes, including failures)
	updatedRefMemo, err := mgr.Keeper().GetReferenceMemo(ctx, common.BTCAsset, refID)
	c.Assert(err, IsNil)
	c.Assert(updatedRefMemo.GetUsageCount(), Equals, int64(3))
}

func (s *HandlerObservedTxHelpersSuite) TestReferenceMemoIntegrationNonExistentReference(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Generate reference memo for non-existent reference
	refMemo := NewReferenceReadMemo("99999") // This reference doesn't exist in storage
	memoStr := refMemo.CreateMemo()

	// Test that fetchMemoFromReference returns empty string for non-existent reference
	testTx := common.NewTx(common.TxID(""), common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, memoStr)
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx, 1) // tx observed at height 1, reference doesn't exist
	c.Assert(resolvedMemo, Equals, "")
}

func (s *HandlerObservedTxHelpersSuite) TestReferenceMemoIntegrationFullWorkflow(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Simulate the full workflow:
	// 1. Memoless transaction comes in
	// 2. generateReferenceMemoID creates an ID
	// 3. CreateMemo creates the memo string
	// 4. fetchMemoFromReference resolves it
	// 5. trackReferenceMemoUsage tracks usage

	// Step 1: Create memoless transaction
	tx := GetRandomObservedTx()
	tx.Tx.Memo = "" // Empty memo
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(987654321)))

	// Step 2: Generate reference ID (simulating our new functionality)
	refID, err := generateReferenceMemoID(ctx, mgr, common.BTCAsset, tx)
	c.Assert(err, IsNil)
	c.Assert(refID, Equals, "54321") // last 5 digits of 987654321

	// Step 3: Create memo string (simulating CreateMemo)
	refMemo := NewReferenceReadMemo(refID)
	generatedMemo := refMemo.CreateMemo()
	c.Assert(generatedMemo, Equals, "r:54321")

	// Simulate that a reference memo exists in storage
	actualMemo := "SWAP:BTC.BTC:bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	storedRefMemo := NewReferenceMemo(common.BTCAsset, actualMemo, refID, 0)
	mgr.Keeper().SetReferenceMemo(ctx, storedRefMemo)

	// Step 4: fetchMemoFromReference resolves the generated memo
	testTx := common.NewTx(tx.Tx.ID, common.NoAddress, common.NoAddress, common.Coins{}, common.Gas{}, generatedMemo)
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, testTx, 1) // tx observed at height 1, memo created at height 0
	c.Assert(resolvedMemo, Equals, actualMemo)

	// Step 5: Usage is automatically tracked by fetchMemoFromReference

	// Verify the full workflow worked
	updatedRefMemo, err := mgr.Keeper().GetReferenceMemo(ctx, common.BTCAsset, refID)
	c.Assert(err, IsNil)
	c.Assert(updatedRefMemo.GetUsageCount(), Equals, int64(1))
	c.Assert(updatedRefMemo.HasBeenUsedBy(tx.Tx.ID), Equals, true)
}

func (s *HandlerObservedTxHelpersSuite) TestUnfinalizedHeightPreservation(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Create a test transaction
	tx := GetRandomObservedTx()
	tx.Tx.Chain = common.BTCChain
	tx.Tx.Coins = common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1000000)))

	// Create an empty voter
	voter := NewObservedTxVoter(tx.Tx.ID, []common.ObservedTx{})
	c.Assert(voter.Height, Equals, int64(0))
	c.Assert(voter.UnfinalizedHeight, Equals, int64(0))

	// Mock node accounts for consensus
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	signer := nas[0].NodeAddress

	// Test processTxInAttestation - first consensus (non-finalized)
	ctx = ctx.WithBlockHeight(100)
	voter, ok := processTxInAttestation(ctx, mgr, voter, nas, tx, signer, false)

	// After first consensus, both Height and UnfinalizedHeight should be set to the same value
	if voter.HasConsensus(nas) && !tx.IsFinal() {
		c.Assert(voter.Height, Equals, int64(100))
		c.Assert(voter.UnfinalizedHeight, Equals, int64(100))
		c.Assert(ok, Equals, true)
	}

	// Test that UnfinalizedHeight is preserved when Height changes
	ctx = ctx.WithBlockHeight(200)

	// Add another observation to trigger finalization
	tx.FinaliseHeight = 150
	voter, ok = processTxInAttestation(ctx, mgr, voter, nas, tx, nas[1].NodeAddress, false)

	// After finalization, Height might change but UnfinalizedHeight should remain
	if voter.HasFinalised(nas) {
		c.Assert(voter.FinalisedHeight, Equals, int64(200))
		c.Assert(voter.UnfinalizedHeight, Equals, int64(100)) // Should preserve original consensus height
		c.Assert(ok, Equals, true)
	}
}

func (s *HandlerObservedTxHelpersSuite) TestUnfinalizedHeightUsedInFetchMemoFromReference(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set up BTC pool
	btcPool := NewPool()
	btcPool.Asset = common.BTCAsset
	btcPool.Decimals = 8
	btcPool.BalanceAsset = cosmos.NewUint(100 * common.One)
	btcPool.BalanceRune = cosmos.NewUint(100 * common.One)
	c.Assert(mgr.Keeper().SetPool(ctx, btcPool), IsNil)

	// Set TTL for memoless transactions
	mgr.Keeper().SetMimir(ctx, constants.MemolessTxnTTL.String(), 100)

	// Create a reference memo at height 50
	refMemo := NewReferenceMemo(common.BTCAsset, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890", "12345", 50)
	mgr.Keeper().SetReferenceMemo(ctx, refMemo)

	// Create voter with UnfinalizedHeight set to 60 but Height set to 120
	voter := NewObservedTxVoter(common.TxID("testid"), []common.ObservedTx{})
	voter.Height = 120           // Later consensus height
	voter.UnfinalizedHeight = 60 // Original consensus height when first observed
	voter.Tx.Tx.Memo = "r:12345" // Reference memo

	// Test that fetchMemoFromReference uses UnfinalizedHeight (60), not Height (120)
	// Since reference was created at height 50 and UnfinalizedHeight is 60, this should succeed
	resolvedMemo := fetchMemoFromReference(ctx, mgr, common.BTCAsset, voter.Tx.Tx, voter.UnfinalizedHeight)
	c.Assert(resolvedMemo, Equals, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890")

	// If we had used Height (120) instead, let's verify it would fail the height check
	// (transaction observed after memo creation: 120 > 50, but this should be caught by height validation)
	resolvedMemoWithHeight := fetchMemoFromReference(ctx, mgr, common.BTCAsset, voter.Tx.Tx, voter.Height)
	c.Assert(resolvedMemoWithHeight, Equals, "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890") // This will still work since 120 > 50

	// Test edge case where using Height would fail but UnfinalizedHeight succeeds
	// Create reference memo at height 70
	refMemo2 := NewReferenceMemo(common.BTCAsset, "SWAP:BTC.BTC:bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", "54321", 70)
	mgr.Keeper().SetReferenceMemo(ctx, refMemo2)

	// Set voter with UnfinalizedHeight = 75 (valid) but Height = 65 (invalid - before memo creation)
	voter.Height = 65            // This would fail height validation (65 <= 70)
	voter.UnfinalizedHeight = 75 // This should pass height validation (75 > 70)
	voter.Tx.Tx.Memo = "r:54321"

	// Using UnfinalizedHeight should succeed
	resolvedMemo = fetchMemoFromReference(ctx, mgr, common.BTCAsset, voter.Tx.Tx, voter.UnfinalizedHeight)
	c.Assert(resolvedMemo, Equals, "SWAP:BTC.BTC:bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4")

	// Using Height should fail
	resolvedMemoWithHeight = fetchMemoFromReference(ctx, mgr, common.BTCAsset, voter.Tx.Tx, voter.Height)
	c.Assert(resolvedMemoWithHeight, Equals, "") // Should fail due to height validation
}

func (s *HandlerObservedTxHelpersSuite) TestFindOriginalMemoForOutboundDeterministicOrdering(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set signing transaction period
	mgr.Keeper().SetMimir(ctx, constants.SigningTransactionPeriod.String(), 300)

	// Create vault pubkey
	vaultPubKey := GetRandomPubKey()

	// Create two TxOutItems with identical on-chain fields but different memos/InHashes
	// These would look identical on-chain in a memoless transaction
	inHash1, _ := common.NewTxID("1111111111111111111111111111111111111111111111111111111111111111")
	inHash2, _ := common.NewTxID("2222222222222222222222222222222222222222222222222222222222222222")

	item1 := TxOutItem{
		Chain:       common.ETHChain,
		ToAddress:   GetRandomETHAddress(),
		VaultPubKey: vaultPubKey,
		Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
		Memo:        "OUT:" + inHash1.String(),
		InHash:      inHash1,
		GasRate:     100,
		MaxGas:      common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))},
	}

	item2 := TxOutItem{
		Chain:       common.ETHChain,
		ToAddress:   item1.ToAddress, // Same ToAddress
		VaultPubKey: vaultPubKey,     // Same VaultPubKey
		Coin:        item1.Coin,      // Same Coin
		Memo:        "OUT:" + inHash2.String(),
		InHash:      inHash2,
		GasRate:     100,
		MaxGas:      common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))},
	}

	// Compute hashes to determine which one Bifrost would sign first
	hash1 := item1.Hash()
	hash2 := item2.Hash()

	// Determine which item has the lower hash (Bifrost signs this one first)
	var firstItem, secondItem TxOutItem
	if hash1 < hash2 {
		firstItem = item1
		secondItem = item2
	} else {
		firstItem = item2
		secondItem = item1
	}

	// Set block height for context
	ctx = ctx.WithBlockHeight(100)

	// Add both TxOutItems at the same height
	txOut := NewTxOut(100)
	// Add in reverse order to ensure ordering is not just array order
	txOut.TxArray = append(txOut.TxArray, secondItem)
	txOut.TxArray = append(txOut.TxArray, firstItem)
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	// Create an observed tx that matches both items (memoless on-chain)
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			Chain:     common.ETHChain,
			ToAddress: item1.ToAddress,
			Coins:     common.Coins{item1.Coin},
			Memo:      "", // Empty memo (memoless)
		},
		ObservedPubKey: vaultPubKey,
	}

	// findOriginalMemoForOutbound should return the memo from the item with the lowest hash
	// (the one Bifrost would sign first)
	result := findOriginalMemoForOutbound(ctx, mgr, observedTx)
	c.Assert(result, Equals, firstItem.Memo, Commentf("Expected memo from item with lower hash"))
}

func (s *HandlerObservedTxHelpersSuite) TestFindOriginalMemoForOutboundSingleMatch(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set signing transaction period
	mgr.Keeper().SetMimir(ctx, constants.SigningTransactionPeriod.String(), 300)

	// Create vault pubkey
	vaultPubKey := GetRandomPubKey()
	inHash, _ := common.NewTxID("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	item := TxOutItem{
		Chain:        common.ETHChain,
		ToAddress:    GetRandomETHAddress(),
		VaultPubKey:  vaultPubKey,
		Coin:         common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
		Memo:         "OUT:" + inHash.String(),
		OriginalMemo: "SWAP:ETH.ETH:0x1234567890123456789012345678901234567890",
		InHash:       inHash,
		GasRate:      100,
		MaxGas:       common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))},
	}

	// Set block height for context
	ctx = ctx.WithBlockHeight(100)

	// Add TxOutItem
	txOut := NewTxOut(100)
	txOut.TxArray = append(txOut.TxArray, item)
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)

	// Create an observed tx that matches the item
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			Chain:     common.ETHChain,
			ToAddress: item.ToAddress,
			Coins:     common.Coins{item.Coin},
			Memo:      "", // Empty memo (memoless)
		},
		ObservedPubKey: vaultPubKey,
	}

	// Should return OriginalMemo when set
	result := findOriginalMemoForOutbound(ctx, mgr, observedTx)
	c.Assert(result, Equals, item.OriginalMemo)
}

func (s *HandlerObservedTxHelpersSuite) TestFindOriginalMemoForOutboundNoMatch(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set signing transaction period
	mgr.Keeper().SetMimir(ctx, constants.SigningTransactionPeriod.String(), 300)

	// Create vault pubkey
	vaultPubKey := GetRandomPubKey()

	// Set block height for context
	ctx = ctx.WithBlockHeight(100)

	// Don't add any TxOutItems

	// Create an observed tx
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			Chain:     common.ETHChain,
			ToAddress: GetRandomETHAddress(),
			Coins:     common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000))},
			Memo:      "", // Empty memo (memoless)
		},
		ObservedPubKey: vaultPubKey,
	}

	// Should return empty string when no match
	result := findOriginalMemoForOutbound(ctx, mgr, observedTx)
	c.Assert(result, Equals, "")
}

func (s *HandlerObservedTxHelpersSuite) TestFindOriginalMemoForOutboundEmptyCoins(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Create an observed tx with empty coins
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			Chain:     common.ETHChain,
			ToAddress: GetRandomETHAddress(),
			Coins:     common.Coins{}, // Empty coins
			Memo:      "",
		},
		ObservedPubKey: GetRandomPubKey(),
	}

	// Should return empty string for empty coins
	result := findOriginalMemoForOutbound(ctx, mgr, observedTx)
	c.Assert(result, Equals, "")
}

func (s *HandlerObservedTxHelpersSuite) TestFindOriginalMemoForOutboundAcrossMultipleHeights(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Set signing transaction period
	mgr.Keeper().SetMimir(ctx, constants.SigningTransactionPeriod.String(), 300)

	// Create vault pubkey
	vaultPubKey := GetRandomPubKey()

	// Create two TxOutItems at different heights with identical on-chain fields
	inHash1, _ := common.NewTxID("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	inHash2, _ := common.NewTxID("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")

	toAddr := GetRandomETHAddress()
	coin := common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000))

	item1 := TxOutItem{
		Chain:       common.ETHChain,
		ToAddress:   toAddr,
		VaultPubKey: vaultPubKey,
		Coin:        coin,
		Memo:        "OUT:" + inHash1.String(),
		InHash:      inHash1,
		GasRate:     100,
		MaxGas:      common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))},
	}

	item2 := TxOutItem{
		Chain:       common.ETHChain,
		ToAddress:   toAddr,
		VaultPubKey: vaultPubKey,
		Coin:        coin,
		Memo:        "OUT:" + inHash2.String(),
		InHash:      inHash2,
		GasRate:     100,
		MaxGas:      common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(10000))},
	}

	// Set block height for context
	ctx = ctx.WithBlockHeight(100)

	// Add item1 at height 90, item2 at height 95
	// Bifrost should sign item1 first (lower height)
	txOut90 := NewTxOut(90)
	txOut90.TxArray = append(txOut90.TxArray, item1)
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut90), IsNil)

	txOut95 := NewTxOut(95)
	txOut95.TxArray = append(txOut95.TxArray, item2)
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut95), IsNil)

	// Create an observed tx that matches both items
	observedTx := common.ObservedTx{
		Tx: common.Tx{
			Chain:     common.ETHChain,
			ToAddress: toAddr,
			Coins:     common.Coins{coin},
			Memo:      "", // Empty memo (memoless)
		},
		ObservedPubKey: vaultPubKey,
	}

	// Should return memo from item at lower height (item1 at height 90)
	result := findOriginalMemoForOutbound(ctx, mgr, observedTx)
	c.Assert(result, Equals, item1.Memo)
}

func (s *HandlerObservedTxHelpersSuite) TestTxOutItemHashConsistency(c *C) {
	// Test that txOutItemHash produces consistent results matching Bifrost's formula
	inHash, _ := common.NewTxID("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

	item := TxOutItem{
		Chain:       common.ETHChain,
		ToAddress:   GetRandomETHAddress(),
		VaultPubKey: GetRandomPubKey(),
		Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(1000000)),
		Memo:        "OUT:" + inHash.String(),
		InHash:      inHash,
	}

	// Hash should be deterministic
	hash1 := item.Hash()
	hash2 := item.Hash()
	c.Assert(hash1, Equals, hash2)

	// Hash should be 64 characters (SHA256 hex)
	c.Assert(len(hash1), Equals, 64)

	// Different memo should produce different hash
	item2 := item
	inHash2, _ := common.NewTxID("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB")
	item2.Memo = "OUT:" + inHash2.String()
	item2.InHash = inHash2
	hash3 := item2.Hash()
	c.Assert(hash1, Not(Equals), hash3)
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionOnReorg(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Create 4 active node accounts (need supermajority: 3 out of 4)
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	// Set up a vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Create the outbound tx with original gas (G1 = 22131)
	txID := GetRandomTxHash()
	originalGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	baseTx := common.Tx{
		ID:          txID,
		Chain:       common.ETHChain,
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
		Gas:         originalGas,
		Memo:        "OUT:xyz",
	}

	// Create observed tx with original gas, finalized
	obsTx := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)

	// Create empty voter
	voter := NewObservedTxVoter(txID, []common.ObservedTx{})

	// All 4 nodes observe with the original gas (G1) - reaching consensus and finalization.
	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, obsTx, na.NodeAddress, false)
	}

	// Voter should be finalized with G1
	c.Assert(voter.FinalisedHeight, Equals, int64(100))
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true)

	// Record vault balance before gas correction
	vault, err := mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceBefore := vault.GetCoin(common.ETHAsset).Amount

	// Now simulate reorg: nodes re-observe with different gas (G2 = 23061)
	reorgGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}
	reorgTx := baseTx
	reorgTx.Gas = reorgGas
	reorgObsTx := common.NewObservedTx(reorgTx, 100, vaultPubKey, 100)

	ctx = ctx.WithBlockHeight(101) // Slightly later block

	// First 2 nodes re-observe with G2 - not enough for supermajority yet
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, reorgObsTx, nas[0].NodeAddress, false)
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, reorgObsTx, nas[1].NodeAddress, false)

	// Gas should NOT be corrected yet (only 2 out of 4)
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true)

	// Third node re-observes with G2 - now we have 3/4 supermajority
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, reorgObsTx, nas[2].NodeAddress, false)

	// Gas should now be corrected
	c.Assert(voter.Tx.Tx.Gas.Equals(reorgGas), Equals, true)

	// Vault should have been debited the additional gas delta (23061 - 22131 = 930)
	vault, err = mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceAfter := vault.GetCoin(common.ETHAsset).Amount
	expectedDelta := cosmos.NewUint(23061 - 22131)
	c.Assert(vaultBalanceBefore.Sub(vaultBalanceAfter).Equal(expectedDelta), Equals, true,
		Commentf("vault balance delta should equal gas delta: got %s, expected %s",
			vaultBalanceBefore.Sub(vaultBalanceAfter), expectedDelta))

	// Fourth node re-observes with G2 - gas correction should NOT happen again
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, reorgObsTx, nas[3].NodeAddress, false)
	vault, err = mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	// Balance should remain the same (no double correction)
	c.Assert(vault.GetCoin(common.ETHAsset).Amount.Equal(vaultBalanceAfter), Equals, true)
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionFinalizedIgnoresNonFinalObservations(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Create 4 active node accounts (need supermajority: 3 out of 4)
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	// Set up a vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Finalized outbound tx with original gas (G1 = 22131)
	txID := GetRandomTxHash()
	originalGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	baseTx := common.Tx{
		ID:          txID,
		Chain:       common.ETHChain,
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
		Gas:         originalGas,
		Memo:        "OUT:xyz",
	}
	finalObsTx := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)

	voter := NewObservedTxVoter(txID, []common.ObservedTx{})
	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, finalObsTx, na.NodeAddress, false)
	}

	// Voter should be finalized with G1
	c.Assert(voter.FinalisedHeight, Equals, int64(100))
	c.Assert(voter.Tx.IsFinal(), Equals, true)
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true)

	vault, err := mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceBefore := vault.GetCoin(common.ETHAsset).Amount

	// Re-observations are NON-final (FinaliseHeight > BlockHeight) with different gas (G2).
	// Even if a supermajority agrees on these non-final observations, a finalized voter
	// should not accept them for gas correction.
	reorgGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}
	reorgTx := baseTx
	reorgTx.Gas = reorgGas
	nonFinalReorgObsTx := common.NewObservedTx(reorgTx, 100, vaultPubKey, 101)

	ctx = ctx.WithBlockHeight(101)

	// 3 out of 4 nodes submit non-final re-observations (supermajority)
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, nonFinalReorgObsTx, nas[0].NodeAddress, false)
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, nonFinalReorgObsTx, nas[1].NodeAddress, false)
	voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, nonFinalReorgObsTx, nas[2].NodeAddress, false)

	// Gas should NOT be corrected from non-final observations once voter is finalized.
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true)
	c.Assert(voter.Tx.IsFinal(), Equals, true)

	vault, err = mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	c.Assert(vault.GetCoin(common.ETHAsset).Amount.Equal(vaultBalanceBefore), Equals, true)
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionDecrease(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Create 3 active node accounts
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	// Set up a vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Create the outbound tx with original gas (G1 = 23061, higher)
	txID := GetRandomTxHash()
	originalGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}
	baseTx := common.Tx{
		ID:          txID,
		Chain:       common.ETHChain,
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
		Gas:         originalGas,
		Memo:        "OUT:xyz",
	}

	obsTx := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)
	voter := NewObservedTxVoter(txID, []common.ObservedTx{})

	// All nodes observe with G1
	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, obsTx, na.NodeAddress, false)
	}

	c.Assert(voter.FinalisedHeight, Equals, int64(100))

	vault, err := mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceBefore := vault.GetCoin(common.ETHAsset).Amount

	// Gas DECREASED after reorg (G2 = 22131)
	reorgGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	reorgTx := baseTx
	reorgTx.Gas = reorgGas
	reorgObsTx := common.NewObservedTx(reorgTx, 100, vaultPubKey, 100)

	ctx = ctx.WithBlockHeight(101)

	// All 3 nodes re-observe with G2 (2 gives supermajority for 3 nodes)
	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, reorgObsTx, na.NodeAddress, false)
	}

	// Gas should be corrected
	c.Assert(voter.Tx.Tx.Gas.Equals(reorgGas), Equals, true)

	// Vault should have GAINED back the gas delta (23061 - 22131 = 930)
	vault, err = mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceAfter := vault.GetCoin(common.ETHAsset).Amount
	expectedDelta := cosmos.NewUint(23061 - 22131)
	c.Assert(vaultBalanceAfter.Sub(vaultBalanceBefore).Equal(expectedDelta), Equals, true,
		Commentf("vault should gain back over-deducted gas: got %s, expected %s",
			vaultBalanceAfter.Sub(vaultBalanceBefore), expectedDelta))
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionSkipsInactiveVault(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Set up an INACTIVE vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.Status = InactiveVault
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Original consensus gas and corrected (higher) gas
	oldGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	newGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}

	consensusTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
			Gas:   oldGas,
		},
	}
	correctedTx := ObservedTx{
		Tx: common.Tx{
			ID:    consensusTx.Tx.ID,
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
			Gas:   newGas,
		},
		ObservedPubKey: vaultPubKey,
	}

	// Call correctOutboundGas
	correctOutboundGas(ctx, mgr, consensusTx, correctedTx)

	// Vault balance should be debited (vault accounting still happens)
	vault, err := mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	_ = vault

	// But gas manager should NOT have the additional gas (no reimbursement for inactive vault)
	gasEvents := mgr.GasMgr().GetGas()
	c.Assert(gasEvents, HasLen, 0, Commentf("inactive vault should not trigger reserve reimbursement"))
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionSkipsDuringRagnarokGasAsset(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Set ragnarok in progress
	mgr.Keeper().SetRagnarokBlockHeight(ctx, 50)

	// Set up an active vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.Status = ActiveVault
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Original consensus gas and corrected (higher) gas
	oldGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	newGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}

	// Outbound coins contain the gas asset (ETH) - should skip reimbursement
	consensusTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
			Gas:   oldGas,
		},
	}
	correctedTx := ObservedTx{
		Tx: common.Tx{
			ID:    consensusTx.Tx.ID,
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
			Gas:   newGas,
		},
		ObservedPubKey: vaultPubKey,
	}

	correctOutboundGas(ctx, mgr, consensusTx, correctedTx)

	// Gas manager should NOT have the additional gas (ragnarok + gas asset in coins)
	gasEvents := mgr.GasMgr().GetGas()
	c.Assert(gasEvents, HasLen, 0, Commentf("ragnarok with gas asset coins should not trigger reserve reimbursement"))
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionAllowedDuringRagnarokNonGasAsset(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Set ragnarok in progress
	mgr.Keeper().SetRagnarokBlockHeight(ctx, 50)

	// Set up an active vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.Status = ActiveVault
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Original consensus gas and corrected (higher) gas
	oldGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	newGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}

	// Outbound coins contain a NON-gas asset (USDC token on ETH) - should still reimburse
	usdcAsset := common.Asset{Chain: common.ETHChain, Symbol: "USDC-0XA0B86991C6218B36C1D19D4A2E9EB0CE3606EB48", Ticker: "USDC", Synth: false}
	consensusTx := common.ObservedTx{
		Tx: common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(usdcAsset, cosmos.NewUint(1000000))),
			Gas:   oldGas,
		},
	}
	correctedTx := ObservedTx{
		Tx: common.Tx{
			ID:    consensusTx.Tx.ID,
			Chain: common.ETHChain,
			Coins: common.NewCoins(common.NewCoin(usdcAsset, cosmos.NewUint(1000000))),
			Gas:   newGas,
		},
		ObservedPubKey: vaultPubKey,
	}

	correctOutboundGas(ctx, mgr, consensusTx, correctedTx)

	// Gas manager SHOULD have the additional gas (ragnarok but non-gas asset in coins)
	gasEvents := mgr.GasMgr().GetGas()
	c.Assert(gasEvents, HasLen, 1, Commentf("ragnarok with non-gas-asset coins should still trigger reserve reimbursement"))
}

func (s *HandlerObservedTxHelpersSuite) TestCountMatchingSigners(c *C) {
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	txID := GetRandomTxHash()
	gas1 := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	gas2 := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}

	baseTx := common.Tx{
		ID:          txID,
		Chain:       common.ETHChain,
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
		Gas:         gas1,
		Memo:        "OUT:xyz",
	}

	vaultPubKey := GetRandomPubKey()
	obsTx1 := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)
	obsTx1.Signers = []string{nas[0].NodeAddress.String(), nas[1].NodeAddress.String()}

	reorgTx := baseTx
	reorgTx.Gas = gas2
	obsTx2 := common.NewObservedTx(reorgTx, 100, vaultPubKey, 100)
	obsTx2.Signers = []string{nas[2].NodeAddress.String()}

	obsTx3 := common.NewObservedTx(reorgTx, 101, vaultPubKey, 101)
	obsTx3.Signers = []string{nas[2].NodeAddress.String(), nas[0].NodeAddress.String()}

	voter := NewObservedTxVoter(txID, []common.ObservedTx{obsTx1, obsTx2, obsTx3})

	// Count signers matching gas2
	count := countMatchingSigners(voter, obsTx2, nas)
	c.Assert(count, Equals, 2, Commentf("duplicate signer across matching observations should only count once"))

	// Count signers matching gas1
	testTx := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)
	count = countMatchingSigners(voter, testTx, nas)
	c.Assert(count, Equals, 2)
}

func (s *HandlerObservedTxHelpersSuite) TestGasCorrectionRequiresFinalityParity(c *C) {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(100)

	// Create 4 active node accounts (supermajority = 3 out of 4)
	nas := NodeAccounts{
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
	}

	// Set up a vault with funds
	vaultPubKey := GetRandomPubKey()
	vault := GetRandomVault()
	vault.PubKey = vaultPubKey
	vault.AddFunds(common.Coins{
		common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
	})
	c.Assert(mgr.Keeper().SetVault(ctx, vault), IsNil)

	// Create the outbound tx with original gas
	txID := GetRandomTxHash()
	originalGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(22131))}
	baseTx := common.Tx{
		ID:          txID,
		Chain:       common.ETHChain,
		FromAddress: GetRandomETHAddress(),
		ToAddress:   GetRandomETHAddress(),
		Coins:       common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(305570))),
		Gas:         originalGas,
		Memo:        "OUT:xyz",
	}

	// All 4 nodes observe with original gas as FINAL (BlockHeight == FinaliseHeight)
	obsTx := common.NewObservedTx(baseTx, 100, vaultPubKey, 100)
	voter := NewObservedTxVoter(txID, []common.ObservedTx{})

	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, obsTx, na.NodeAddress, false)
	}

	c.Assert(voter.FinalisedHeight, Equals, int64(100))
	c.Assert(voter.Tx.IsFinal(), Equals, true)
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true)

	// Record vault balance before re-observations
	vault, err := mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	vaultBalanceBefore := vault.GetCoin(common.ETHAsset).Amount

	// Now simulate re-observations with different gas but NON-FINAL (BlockHeight != FinaliseHeight)
	reorgGas := common.Gas{common.NewCoin(common.ETHAsset, cosmos.NewUint(23061))}
	reorgTx := baseTx
	reorgTx.Gas = reorgGas
	nonFinalReorgObsTx := common.NewObservedTx(reorgTx, 100, vaultPubKey, 200) // not final

	ctx = ctx.WithBlockHeight(101)

	// All 4 nodes re-observe with non-final corrected gas
	for _, na := range nas {
		voter, _ = processTxOutAttestation(ctx, mgr, voter, nas, nonFinalReorgObsTx, na.NodeAddress, false)
	}

	// Gas should NOT be corrected because non-final re-observations must not mutate finalized gas
	c.Assert(voter.Tx.Tx.Gas.Equals(originalGas), Equals, true,
		Commentf("non-final re-observations should not correct gas on finalized voter"))

	// Vault balance should be unchanged
	vault, err = mgr.Keeper().GetVault(ctx, vaultPubKey)
	c.Assert(err, IsNil)
	c.Assert(vault.GetCoin(common.ETHAsset).Amount.Equal(vaultBalanceBefore), Equals, true,
		Commentf("vault balance should be unchanged when non-final re-observations are rejected"))
}
