package thorchain

import (
	"github.com/blang/semver"
	. "gopkg.in/check.v1"
)

type ManagersTestSuite struct{}

var _ = Suite(&ManagersTestSuite{})

func (ManagersTestSuite) TestManagers(c *C) {
	_, mgr := setupManagerForTest(c)
	ver := semver.MustParse("0.0.1")

	// Versioning has been removed from all managers.
	gasMgr, err := GetGasManager(ver, mgr.Keeper())
	c.Assert(gasMgr, NotNil)
	c.Assert(err, IsNil)

	eventMgr, err := GetEventManager(ver)
	c.Assert(eventMgr, NotNil)
	c.Assert(err, IsNil)

	txOutStore, err := GetTxOutStore(ver, mgr.Keeper(), mgr.EventMgr(), gasMgr)
	c.Assert(txOutStore, NotNil)
	c.Assert(err, IsNil)

	vaultMgr, err := GetNetworkManager(ver, mgr.Keeper(), mgr.TxOutStore(), mgr.EventMgr())
	c.Assert(vaultMgr, NotNil)
	c.Assert(err, IsNil)

	validatorManager, err := GetValidatorManager(ver, mgr.Keeper(), mgr.NetworkMgr(), mgr.TxOutStore(), mgr.EventMgr())
	c.Assert(validatorManager, NotNil)
	c.Assert(err, IsNil)

	observerMgr, err := GetObserverManager(ver)
	c.Assert(observerMgr, NotNil)
	c.Assert(err, IsNil)

	swapQueue, err := GetSwapQueue(ver, mgr.Keeper())
	c.Assert(swapQueue, NotNil)
	c.Assert(err, IsNil)

	swapper, err := GetSwapper(ver)
	c.Assert(swapper, NotNil)
	c.Assert(err, IsNil)

	slasher, err := GetSlasher(ver, mgr.Keeper(), mgr.EventMgr())
	c.Assert(slasher, NotNil)
	c.Assert(err, IsNil)
}
