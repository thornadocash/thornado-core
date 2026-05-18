//go:build mainnet
// +build mainnet

package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
)

// processMigration handles the actual migration logic.
func (m *ScheduledMigrationMgr) processMigration(ctx cosmos.Context, mgr Manager) error {
	return nil
}
