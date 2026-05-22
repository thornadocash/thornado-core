package thornado

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
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
	return nil
}
