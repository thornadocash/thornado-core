package keeperv1

import (
	. "gopkg.in/check.v1"

	"github.com/thornadocash/go-thornado/constants"
)

type KeeperConfigSuite struct{}

var _ = Suite(&KeeperConfigSuite{})

func (s *KeeperConfigSuite) TestGetConfigInt64(c *C) {
	ctx, k := setupKeeperForTest(c)
	defaultBlockTime := constants.NewConfigValue().GetInt64Value(constants.Chain_BlockTimeSeconds)

	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds), Equals, defaultBlockTime)

	k.SetConfig(ctx, constants.Chain_BlockTimeSeconds.String(), 10)
	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds), Equals, int64(10))

	k.SetConfig(ctx, constants.Chain_BlockTimeSeconds.String(), -1)
	c.Assert(k.GetConfigInt64(ctx, constants.Chain_BlockTimeSeconds), Equals, defaultBlockTime)
}
