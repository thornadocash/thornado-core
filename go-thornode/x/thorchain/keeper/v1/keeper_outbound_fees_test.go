package keeperv1

import (
	"math"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

type KeeperOutboundFeesSuite struct{}

var _ = Suite(&KeeperOutboundFeesSuite{})

func (s *KeeperOutboundFeesSuite) TestGetSurplusForTargetMultiplier(c *C) {
	ctx, k := setupKeeperForTest(c)

	surplus := k.GetSurplusForTargetMultiplier(ctx, cosmos.NewUint(10_000))
	c.Check(surplus.String(), Equals, "689655172414")
}

func (s *KeeperOutboundFeesSuite) TestOutboundRuneRecords(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Nothing set returns 0.
	feeWithheldRune, err := k.GetOutboundFeeWithheldRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err := k.GetOutboundFeeSpentRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	// The initial withheld amount is the surplus for the target multiplier
	initialWithheldBTC := k.GetSurplusForTargetMultiplier(ctx, cosmos.NewUint(10_000))
	c.Check(feeWithheldRune.String(), Equals, initialWithheldBTC.String())
	c.Check(feeSpentRune.String(), Equals, "0")

	// Adding sets.
	err = k.AddToOutboundFeeWithheldRune(ctx, common.BTCAsset, cosmos.NewUint(uint64(200)))
	c.Assert(err, IsNil)
	initialWithheldBTC = initialWithheldBTC.Add(cosmos.NewUint(uint64(200)))
	err = k.AddToOutboundFeeSpentRune(ctx, common.BTCAsset, cosmos.NewUint(uint64(100)))
	c.Assert(err, IsNil)

	feeWithheldRune, err = k.GetOutboundFeeWithheldRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err = k.GetOutboundFeeSpentRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(feeWithheldRune.String(), Equals, initialWithheldBTC.String())
	c.Check(feeSpentRune.String(), Equals, "100")

	// Adding again adds.
	err = k.AddToOutboundFeeWithheldRune(ctx, common.BTCAsset, cosmos.NewUint(uint64(400)))
	c.Assert(err, IsNil)
	initialWithheldBTC = initialWithheldBTC.Add(cosmos.NewUint(uint64(400)))
	err = k.AddToOutboundFeeSpentRune(ctx, common.BTCAsset, cosmos.NewUint(uint64(300)))
	c.Assert(err, IsNil)

	feeWithheldRune, err = k.GetOutboundFeeWithheldRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err = k.GetOutboundFeeSpentRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(feeWithheldRune.String(), Equals, initialWithheldBTC.String())
	c.Check(feeSpentRune.String(), Equals, cosmos.NewUint(uint64(400)).String())

	// Set values are distinct by Asset.
	initialWithheldETH := k.GetSurplusForTargetMultiplier(ctx, cosmos.NewUint(10_000))
	feeWithheldRune, err = k.GetOutboundFeeWithheldRune(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err = k.GetOutboundFeeSpentRune(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	c.Check(feeWithheldRune.String(), Equals, initialWithheldETH.String())
	c.Check(feeSpentRune.String(), Equals, "0")

	err = k.AddToOutboundFeeWithheldRune(ctx, common.ETHAsset, cosmos.NewUint(uint64(50)))
	c.Assert(err, IsNil)
	initialWithheldETH = initialWithheldETH.Add(cosmos.NewUint(uint64(50)))
	err = k.AddToOutboundFeeSpentRune(ctx, common.BTCAsset, cosmos.NewUint(uint64(30)))
	c.Assert(err, IsNil)

	feeWithheldRune, err = k.GetOutboundFeeWithheldRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err = k.GetOutboundFeeSpentRune(ctx, common.BTCAsset)
	c.Assert(err, IsNil)
	c.Check(feeWithheldRune.String(), Equals, initialWithheldBTC.String())
	c.Check(feeSpentRune.String(), Equals, "430")

	feeWithheldRune, err = k.GetOutboundFeeWithheldRune(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	feeSpentRune, err = k.GetOutboundFeeSpentRune(ctx, common.ETHAsset)
	c.Assert(err, IsNil)
	c.Check(feeWithheldRune.String(), Equals, initialWithheldETH.String())
	c.Check(feeSpentRune.String(), Equals, "0")
}

func (s *KeeperOutboundFeesSuite) TestOutboundFeeOverflowSafety(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Set spent to max uint64.
	maxUint := cosmos.NewUint(math.MaxUint64)
	err := k.AddToOutboundFeeSpentRune(ctx, common.BTCAsset, maxUint)
	c.Assert(err, IsNil)

	// Adding more should not panic; it should return an error.
	c.Assert(func() {
		err = k.AddToOutboundFeeSpentRune(ctx, common.BTCAsset, cosmos.NewUint(1))
	}, Not(Panics), nil)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*overflow.*")

	// Same test for withheld rune: adding MaxUint64 overflows because
	// GetOutboundFeeWithheldRune initializes with a non-zero surplus value.
	c.Assert(func() {
		err = k.AddToOutboundFeeWithheldRune(ctx, common.DOGEAsset, maxUint)
	}, Not(Panics), nil)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*overflow.*")
}

func (s *KeeperOutboundFeesSuite) TestSafeUint64(c *C) {
	// Normal value
	val, err := safeUint64(cosmos.NewUint(42))
	c.Assert(err, IsNil)
	c.Check(val, Equals, uint64(42))

	// Max uint64 value
	val, err = safeUint64(cosmos.NewUint(math.MaxUint64))
	c.Assert(err, IsNil)
	c.Check(val, Equals, uint64(math.MaxUint64))

	// Overflow: MaxUint64 + 1 should return an error
	overflow := cosmos.NewUint(math.MaxUint64).Add(cosmos.NewUint(1))
	_, err = safeUint64(overflow)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*overflows uint64.*")
}
