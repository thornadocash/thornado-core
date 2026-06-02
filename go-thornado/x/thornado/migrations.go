package thornado

import sdk "github.com/cosmos/cosmos-sdk/types"

// Migrator handles in-place module store migrations.
type Migrator struct {
	mgr *Mgrs
}

// NewMigrator returns a new Migrator.
func NewMigrator(mgr *Mgrs) Migrator {
	return Migrator{mgr: mgr}
}

// Migrate13to14 is retained for the registered module migration path.
func (m Migrator) Migrate13to14(ctx sdk.Context) error {
	return nil
}
