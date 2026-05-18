//go:build mocknet
// +build mocknet

package constants

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type ConstantsSuite struct{}

var _ = Suite(&ConstantsSuite{})

func (s *ConstantsSuite) Test010(c *C) {
	consts := NewConstantValue()
	c.Check(consts.GetInt64Value(PoolCycle), Equals, int64(43200))
}

func (s *ConstantsSuite) TestCamelToSnakeUpper(c *C) {
	c.Check(CamelToSnakeUpper("PoolCycle"), Equals, "POOL_CYCLE")
	c.Check(CamelToSnakeUpper("L1SlipMinBps"), Equals, "L1_SLIP_MIN_BPS")
	c.Check(CamelToSnakeUpper("TNSRegisterFee"), Equals, "TNS_REGISTER_FEE")
	c.Check(CamelToSnakeUpper("MaxNodeToChurnOutForLowVersion"), Equals, "MAX_NODE_TO_CHURN_OUT_FOR_LOW_VERSION")
}
