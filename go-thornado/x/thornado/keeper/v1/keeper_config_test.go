package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/constants"
)

type KeeperConfigSuite struct{}

var _ = Suite(&KeeperConfigSuite{})

func (s *KeeperConfigSuite) TestGetConfigInt64(c *C) {
	ctx, k := setupKeeperForTest(c)

	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlocksPerYear), Equals, int64(5256000))

	k.SetConfig(ctx, constants.Chain_BlocksPerYear.String(), 10)
	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlocksPerYear), Equals, int64(10))

	k.SetConfig(ctx, constants.Chain_BlocksPerYear.String(), -1)
	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlocksPerYear), Equals, int64(5256000))
}
