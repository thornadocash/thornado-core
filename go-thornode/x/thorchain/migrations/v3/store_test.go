package v3_test

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"

	v3 "gitlab.com/thorchain/thornode/v3/x/thorchain/migrations/v3"

	. "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) { TestingT(t) }

type MigrationsV3Suite struct{}

var _ = Suite(&MigrationsV3Suite{})

func (s *MigrationsV3Suite) TestV3Migrations(c *C) {
	storeKey := storetypes.NewKVStoreKey("thorchain")
	serviceThorchain := runtime.NewKVStoreService(storeKey)
	ctx := testutil.DefaultContext(storeKey, storetypes.NewTransientStoreKey("transient_test"))

	// The v3 migration is for keeper refactoring and doesn't modify store data
	// Just ensure it runs without error
	err := v3.MigrateStore(ctx, serviceThorchain)
	c.Assert(err, IsNil)
}
