package aggregators

import (
	"testing"

	"gitlab.com/thorchain/thornode/v3/common"
	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type DexAggregatorSuite struct{}

var _ = Suite(&DexAggregatorSuite{})

func (s *DexAggregatorSuite) TestFetchDexAggregator_Found(c *C) {
	// The mocknet aggregator list includes an ETH aggregator ending in "3848"
	addr, err := FetchDexAggregator(common.ETHChain, "3848")
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "0x69800327b38A4CeF30367Dec3f64c2f2386f3848")
}

func (s *DexAggregatorSuite) TestFetchDexAggregator_FoundAVAX(c *C) {
	addr, err := FetchDexAggregator(common.AVAXChain, "cFA0F20f")
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "0x1429859428C0aBc9C2C47C8Ee9FBaf82cFA0F20f")
}

func (s *DexAggregatorSuite) TestFetchDexAggregator_WrongChain(c *C) {
	// Suffix exists on ETH but we ask for BTC chain
	_, err := FetchDexAggregator(common.BTCChain, "3848")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "3848 aggregator not found")
}

func (s *DexAggregatorSuite) TestFetchDexAggregator_NotFound(c *C) {
	_, err := FetchDexAggregator(common.ETHChain, "nonexistent")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "nonexistent aggregator not found")
}

func (s *DexAggregatorSuite) TestFetchDexAggregator_SuffixMatch(c *C) {
	// Test with a longer suffix that still matches
	addr, err := FetchDexAggregator(common.ETHChain, "0Cec49fd2")
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "0xd31f7e39afECEc4855fecc51b693F9A0Cec49fd2")
}

func (s *DexAggregatorSuite) TestFetchDexAggregatorGasLimit_Found(c *C) {
	gasLimit, err := FetchDexAggregatorGasLimit(common.ETHChain, "3848")
	c.Assert(err, IsNil)
	c.Assert(gasLimit, Equals, uint64(400_000))
}

func (s *DexAggregatorSuite) TestFetchDexAggregatorGasLimit_FoundAVAX(c *C) {
	gasLimit, err := FetchDexAggregatorGasLimit(common.AVAXChain, "cFA0F20f")
	c.Assert(err, IsNil)
	c.Assert(gasLimit, Equals, uint64(400_000))
}

func (s *DexAggregatorSuite) TestFetchDexAggregatorGasLimit_WrongChain(c *C) {
	_, err := FetchDexAggregatorGasLimit(common.BTCChain, "3848")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "3848 aggregator not found")
}

func (s *DexAggregatorSuite) TestFetchDexAggregatorGasLimit_NotFound(c *C) {
	_, err := FetchDexAggregatorGasLimit(common.ETHChain, "nonexistent")
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "nonexistent aggregator not found")
}

func (s *DexAggregatorSuite) TestFetchDexAggregatorGasLimit_SuffixMatch(c *C) {
	gasLimit, err := FetchDexAggregatorGasLimit(common.ETHChain, "0Cec49fd2")
	c.Assert(err, IsNil)
	c.Assert(gasLimit, Equals, uint64(400_000))
}

func (s *DexAggregatorSuite) TestDexAggregatorsNotEmpty(c *C) {
	aggs := DexAggregators()
	c.Assert(len(aggs) > 0, Equals, true)
}
