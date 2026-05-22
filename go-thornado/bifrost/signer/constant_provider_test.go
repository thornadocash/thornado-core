package signer

import (
	"fmt"

	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/bifrost/thornadoclient"
	"github.com/thornadocash/go-thornado/constants"
)

type ConstantsProviderSuite struct{}

var _ = Suite(&ConstantsProviderSuite{})

// ----- mock bridges for constants provider tests -----

type mockBridgeConstants struct {
	thornadoclient.ThornadoBridge
	constants map[string]int64
	err       error
}

func (b *mockBridgeConstants) GetConstants() (map[string]int64, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.constants, nil
}

// ----- tests -----

func (s *ConstantsProviderSuite) TestNewConstantsProvider(c *C) {
	bridge := &mockBridgeConstants{}
	cp := NewConstantsProvider(bridge)
	c.Assert(cp, NotNil)
	c.Assert(cp.requestHeight, Equals, int64(0))
	c.Assert(cp.bridge, Equals, bridge)
	c.Assert(cp.constants, NotNil)
}

func (s *ConstantsProviderSuite) TestGetInt64Value_FirstCall(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Keysign_PeriodBlocks.String(): 300,
			constants.Churn_IntervalBlocks.String(): 43200,
		},
	}
	cp := NewConstantsProvider(bridge)

	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(300))
	c.Assert(cp.requestHeight, Equals, int64(100))
}

func (s *ConstantsProviderSuite) TestGetInt64Value_CacheHit(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Keysign_PeriodBlocks.String(): 300,
			constants.Churn_IntervalBlocks.String(): 43200,
		},
	}
	cp := NewConstantsProvider(bridge)

	// First call populates cache
	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(300))

	// Second call at height within churn interval should use cache
	val, err = cp.GetInt64Value(200, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(300))
}

func (s *ConstantsProviderSuite) TestGetInt64Value_CacheExpired(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Keysign_PeriodBlocks.String(): 300,
			constants.Churn_IntervalBlocks.String(): 100,
		},
	}
	cp := NewConstantsProvider(bridge)

	// First call populates cache at height 100
	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(300))

	// Update the bridge to return new constants
	bridge.constants = map[string]int64{
		constants.Keysign_PeriodBlocks.String(): 600,
		constants.Churn_IntervalBlocks.String(): 100,
	}

	// Call at height 300 (200 blocks later, > churnInterval of 100) should refresh
	val, err = cp.GetInt64Value(300, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(600))
}

func (s *ConstantsProviderSuite) TestGetInt64Value_ErrorOnFirstCall(c *C) {
	bridge := &mockBridgeConstants{
		err: fmt.Errorf("connection refused"),
	}
	cp := NewConstantsProvider(bridge)

	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to get constants from thornado.*")
	c.Assert(val, Equals, int64(0))
}

func (s *ConstantsProviderSuite) TestGetInt64Value_ErrorOnRefresh(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Keysign_PeriodBlocks.String(): 300,
			constants.Churn_IntervalBlocks.String(): 100,
		},
	}
	cp := NewConstantsProvider(bridge)

	// First call succeeds
	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(300))

	// Make bridge return error for next call
	bridge.err = fmt.Errorf("timeout")

	// Call at height 300 (past churn interval) should fail on refresh
	val, err = cp.GetInt64Value(300, constants.Keysign_PeriodBlocks)
	c.Assert(err, NotNil)
	c.Assert(val, Equals, int64(0))
}

func (s *ConstantsProviderSuite) TestEnsureConstants_NoCacheRefreshNeeded(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Churn_IntervalBlocks.String(): 43200,
		},
	}
	cp := NewConstantsProvider(bridge)

	// First ensure populates cache
	err := cp.EnsureConstants(100)
	c.Assert(err, IsNil)
	c.Assert(cp.requestHeight, Equals, int64(100))

	// Requesting again within churn interval should not refresh
	bridge.err = fmt.Errorf("should not be called")
	err = cp.EnsureConstants(200)
	c.Assert(err, IsNil)
	c.Assert(cp.requestHeight, Equals, int64(100)) // unchanged
}

func (s *ConstantsProviderSuite) TestGetInt64Value_MissingKey(c *C) {
	bridge := &mockBridgeConstants{
		constants: map[string]int64{
			constants.Churn_IntervalBlocks.String(): 43200,
		},
	}
	cp := NewConstantsProvider(bridge)

	// Ask for a key that doesn't exist in the map - should return zero value
	val, err := cp.GetInt64Value(100, constants.Keysign_PeriodBlocks)
	c.Assert(err, IsNil)
	c.Assert(val, Equals, int64(0))
}
