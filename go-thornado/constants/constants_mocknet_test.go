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
	c.Check(consts.GetInt64Value(BlocksPerYear), Equals, int64(5256000))
}

func (s *ConstantsSuite) TestCamelToSnakeUpper(c *C) {
	c.Check(CamelToSnakeUpper("BlocksPerYear"), Equals, "BLOCKS_PER_YEAR")
	c.Check(CamelToSnakeUpper("MinPenaltyPointsForBadNode"), Equals, "MIN_PENALTY_POINTS_FOR_BAD_NODE")
}
