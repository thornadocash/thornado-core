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
	consts := NewConfigValue()
	c.Check(consts.GetInt64Value(Chain_BlockTimeSeconds), Equals, int64(6))
}

func (s *ConstantsSuite) TestCamelToSnakeUpper(c *C) {
	c.Check(CamelToSnakeUpper("Chain_BlockTimeSeconds"), Equals, "BLOCK_TIME_SECONDS")
	c.Check(CamelToSnakeUpper("Node_BadPenaltyPointsMin"), Equals, "NODE_BAD_PENALTY_POINTS_MIN")
}
