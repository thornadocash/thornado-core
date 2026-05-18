package thorchain

import (
	"github.com/blang/semver"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
)

type DummyMgr struct {
	K                         keeper.Keeper
	constAccessor             constants.ConstantValues
	gasMgr                    GasManager
	eventMgr                  EventManager
	txOutStore                TxOutStore
	networkMgr                NetworkManager
	validatorMgr              ValidatorManager
	obMgr                     ObserverManager
	poolMgr                   PoolManager
	swapQ                     SwapQueue
	advSwapQueue              AdvSwapQueue
	slasher                   Slasher
	tradeMgr                  TradeAccountManager
	securedMgr                SecuredAssetManager
	wasmMgr                   WasmManager
	switchMgr                 SwitchManager
	scheduledMigrationManager ScheduledMigrationManager
}

func NewDummyMgrWithKeeper(k keeper.Keeper) *DummyMgr {
	return &DummyMgr{
		K:                         k,
		constAccessor:             constants.GetConstantValues(GetCurrentVersion()),
		gasMgr:                    NewDummyGasManager(),
		eventMgr:                  NewDummyEventMgr(),
		txOutStore:                NewTxStoreDummy(),
		networkMgr:                NewNetworkMgrDummy(),
		validatorMgr:              NewValidatorDummyMgr(),
		obMgr:                     NewDummyObserverManager(),
		poolMgr:                   NewDummyPoolManager(),
		slasher:                   NewDummySlasher(),
		tradeMgr:                  NewDummyTradeAccountManager(),
		wasmMgr:                   NewDummyWasmManager(),
		switchMgr:                 NewDummySwitchManager(),
		advSwapQueue:              NewDummyAdvSwapQueue(),
		scheduledMigrationManager: NewDummyScheduledMigrationManager(),
	}
}

func NewDummyMgr() *DummyMgr {
	return &DummyMgr{
		K:                         keeper.KVStoreDummy{},
		constAccessor:             constants.GetConstantValues(GetCurrentVersion()),
		gasMgr:                    NewDummyGasManager(),
		eventMgr:                  NewDummyEventMgr(),
		txOutStore:                NewTxStoreDummy(),
		networkMgr:                NewNetworkMgrDummy(),
		validatorMgr:              NewValidatorDummyMgr(),
		obMgr:                     NewDummyObserverManager(),
		poolMgr:                   NewDummyPoolManager(),
		slasher:                   NewDummySlasher(),
		tradeMgr:                  NewDummyTradeAccountManager(),
		wasmMgr:                   NewDummyWasmManager(),
		switchMgr:                 NewDummySwitchManager(),
		advSwapQueue:              NewDummyAdvSwapQueue(),
		scheduledMigrationManager: NewDummyScheduledMigrationManager(),
	}
}

func (m DummyMgr) GetVersion() semver.Version               { return GetCurrentVersion() }
func (m DummyMgr) GetConstants() constants.ConstantValues   { return m.constAccessor }
func (m DummyMgr) Keeper() keeper.Keeper                    { return m.K }
func (m DummyMgr) GasMgr() GasManager                       { return m.gasMgr }
func (m DummyMgr) EventMgr() EventManager                   { return m.eventMgr }
func (m DummyMgr) TxOutStore() TxOutStore                   { return m.txOutStore }
func (m DummyMgr) NetworkMgr() NetworkManager               { return m.networkMgr }
func (m DummyMgr) ValidatorMgr() ValidatorManager           { return m.validatorMgr }
func (m DummyMgr) ObMgr() ObserverManager                   { return m.obMgr }
func (m DummyMgr) PoolMgr() PoolManager                     { return m.poolMgr }
func (m DummyMgr) SwapQ() SwapQueue                         { return m.swapQ }
func (m DummyMgr) Slasher() Slasher                         { return m.slasher }
func (m DummyMgr) AdvSwapQueueMgr() AdvSwapQueue            { return m.advSwapQueue }
func (m DummyMgr) TradeAccountManager() TradeAccountManager { return m.tradeMgr }
func (m DummyMgr) SecuredAssetManager() SecuredAssetManager { return m.securedMgr }
func (m DummyMgr) WasmManager() WasmManager                 { return m.wasmMgr }
func (m DummyMgr) SwitchManager() SwitchManager             { return m.switchMgr }
func (m DummyMgr) ScheduledMigrationManager() ScheduledMigrationManager {
	return m.scheduledMigrationManager
}

// DummyAdvSwapQueue is for test purpose
type DummyAdvSwapQueue struct{}

// NewDummyAdvSwapQueue create a new instance of DummyAdvSwapQueue for test purpose
func NewDummyAdvSwapQueue() *DummyAdvSwapQueue {
	return &DummyAdvSwapQueue{}
}

func (d *DummyAdvSwapQueue) AddSwapQueueItem(ctx cosmos.Context, mgr Manager, msg *MsgSwap) error {
	return nil
}

func (d *DummyAdvSwapQueue) EndBlock(ctx cosmos.Context, mgr Manager, telemetryEnabled bool) error {
	return nil
}

// DummyScheduledMigrationManager is for test purpose
type DummyScheduledMigrationManager struct{}

// NewDummyScheduledMigrationManager create a new instance of DummyScheduledMigrationManager for test purpose
func NewDummyScheduledMigrationManager() *DummyScheduledMigrationManager {
	return &DummyScheduledMigrationManager{}
}

func (d *DummyScheduledMigrationManager) EndBlock(ctx cosmos.Context, mgr Manager) error {
	return nil
}
