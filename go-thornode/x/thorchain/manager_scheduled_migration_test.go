package thorchain

import (
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/constants"
)

type ScheduledMigrationMgrSuite struct{}

var _ = Suite(&ScheduledMigrationMgrSuite{})

func (s *ScheduledMigrationMgrSuite) TestScheduledMigrationTriggersAtCorrectHeight(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// set scheduled migration height
	scheduledHeight := ctx.BlockHeight() + 1
	mgr.K.SetMimir(ctx, constants.MimirKeyScheduledMigration, scheduledHeight)

	// EndBlock should not trigger migration at current height
	err := mgr.scheduledMigrationManager.EndBlock(ctx, mgr)
	c.Assert(err, IsNil)

	// mimir value set in mocknet migration should remain unset
	mimirValue, err := mgr.K.GetMimir(ctx, "SCHEDULED-MIGRATION-MOCKNET")
	c.Assert(err, IsNil)
	c.Assert(mimirValue, Equals, int64(-1))

	// advance to scheduled height
	ctx = ctx.WithBlockHeight(scheduledHeight)

	// EndBlock should trigger migration at scheduled height
	err = mgr.scheduledMigrationManager.EndBlock(ctx, mgr)
	c.Assert(err, IsNil)

	// mimir value set in mocknet migration should now be set
	mimirValue, err = mgr.K.GetMimir(ctx, "SCHEDULED-MIGRATION-MOCKNET")
	c.Assert(err, IsNil)
	c.Assert(mimirValue, Equals, int64(123))
}
