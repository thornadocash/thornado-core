package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
)

type ScheduledMigrationMgr struct {
	mgr Manager
}

// newScheduledMigrationMgr creates a new instance of ScheduledMigrationMgr.
func newScheduledMigrationMgr(mgr Manager) *ScheduledMigrationMgr {
	return &ScheduledMigrationMgr{
		mgr: mgr,
	}
}

// EndBlock processes the migration if we are at the scheduled migration height.
func (m *ScheduledMigrationMgr) EndBlock(ctx cosmos.Context, mgr Manager) error {
	// check if current height matches the scheduled migration height
	scheduledHeight, err := m.mgr.Keeper().GetMimir(ctx, constants.MimirKeyScheduledMigration)
	if err != nil {
		return nil
	}

	if ctx.BlockHeight() != scheduledHeight {
		return nil
	}

	ctx.Logger().Info("processing scheduled migration", "height", ctx.BlockHeight())
	return m.processMigration(ctx, mgr)
}
