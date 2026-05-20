//go:build chainnet
// +build chainnet

package thorchain

import (
	"github.com/thornadocash/go-thornado/common/cosmos"
)

// processMigration handles the actual migration logic.
func (m *ScheduledMigrationMgr) processMigration(ctx cosmos.Context, mgr Manager) error {
	return nil
}
