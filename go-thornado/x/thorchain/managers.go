package thorchain

import (
	"errors"
	"fmt"

	"cosmossdk.io/core/store"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	"github.com/blang/semver"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/constants"
	"github.com/thornadocash/go-thornado/x/thorchain/keeper"
	kv1 "github.com/thornadocash/go-thornado/x/thorchain/keeper/v1"
	"github.com/thornadocash/go-thornado/x/thorchain/types"
)

const (
	genesisBlockHeight = 1
)

// ErrNotEnoughToPayFee will happen when the emitted asset is not enough to pay for fee
var ErrNotEnoughToPayFee = errors.New("not enough asset to pay for fees")

// Manager is an interface to define all the required methods
type Manager interface {
	GetConstants() constants.ConstantValues
	GetVersion() semver.Version
	Keeper() keeper.Keeper
	GasMgr() GasManager
	EventMgr() EventManager
	TxOutStore() TxOutStore
	NetworkMgr() NetworkManager
	ValidatorMgr() ValidatorManager
	ObMgr() ObserverManager
	Slasher() Slasher
	ScheduledMigrationManager() ScheduledMigrationManager
}

// GasManager define all the methods required to manage gas
type GasManager interface {
	BeginBlock()
	EndBlock(ctx cosmos.Context, keeper keeper.Keeper, eventManager EventManager)
	AddGasAsset(outAsset common.Asset, gas common.Gas, increaseTxCount bool)
	ProcessGas(ctx cosmos.Context, keeper keeper.Keeper)
	GetGas() common.Gas
	GetAssetOutboundFee(ctx cosmos.Context, asset common.Asset, inRune bool) (cosmos.Uint, error)
	GetGasDetails(ctx cosmos.Context, chain common.Chain) (common.Coin, int64, error)
	GetMaxGas(ctx cosmos.Context, chain common.Chain) (common.Coin, error)
	GetGasRate(ctx cosmos.Context, chain common.Chain) cosmos.Uint
	GetNetworkFee(ctx cosmos.Context, chain common.Chain) (types.NetworkFee, error)
	CalcOutboundFeeMultiplier(ctx cosmos.Context, targetSurplusRune, gasSpentRune, gasWithheldRune, maxMultiplier, minMultiplier cosmos.Uint) cosmos.Uint
}

// EventManager define methods need to be support to manage events
type EventManager interface {
	EmitEvent(ctx cosmos.Context, evt EmitEventItem) error
	EmitGasEvent(ctx cosmos.Context, gasEvent *EventGas) error
	EmitSwapEvent(ctx cosmos.Context, swap *EventSwap) error
	EmitFeeEvent(ctx cosmos.Context, feeEvent *EventFee) error
}

// TxOutStore define the method required for TxOutStore
type TxOutStore interface {
	EndBlock(ctx cosmos.Context, mgr Manager) error
	GetBlockOut(ctx cosmos.Context) (*TxOut, error)
	ClearOutboundItems(ctx cosmos.Context)
	GetOutboundItems(ctx cosmos.Context) ([]TxOutItem, error)
	TryAddTxOutItem(ctx cosmos.Context, mgr Manager, toi TxOutItem, minOut cosmos.Uint) (bool, error)
	UnSafeAddTxOutItem(ctx cosmos.Context, mgr Manager, toi TxOutItem, height int64) error
	GetOutboundItemByToAddress(cosmos.Context, common.Address) []TxOutItem
	CalcTxOutHeight(cosmos.Context, semver.Version, TxOutItem) (int64, cosmos.Uint, error)
	DiscoverOutbounds(ctx cosmos.Context, transactionFeeAsset cosmos.Uint, maxGasAsset common.Coin, toi TxOutItem, vaults Vaults) ([]TxOutItem, cosmos.Uint)
}

// ObserverManager define the method to manage observes
type ObserverManager interface {
	BeginBlock()
	EndBlock(ctx cosmos.Context, keeper keeper.Keeper)
	AppendObserver(chain common.Chain, addrs []cosmos.AccAddress)
	List() []cosmos.AccAddress
}

// ValidatorManager define the method to manage validators
type ValidatorManager interface {
	BeginBlock(ctx cosmos.Context, mgr Manager, existingValidators []string) error
	EndBlock(ctx cosmos.Context, mgr Manager) []abci.ValidatorUpdate
	processRagnarok(ctx cosmos.Context, mgr Manager) error
	NodeAccountPreflightCheck(ctx cosmos.Context, na NodeAccount, constAccessor constants.ConstantValues) (NodeStatus, error)
}

// NetworkManager interface define the contract of network Manager
type NetworkManager interface {
	TriggerKeygen(ctx cosmos.Context, nas NodeAccounts) error
	RotateVault(ctx cosmos.Context, vault Vault) error
	BeginBlock(ctx cosmos.Context, mgr Manager) error
	EndBlock(ctx cosmos.Context, mgr Manager) error
	UpdateNetwork(ctx cosmos.Context, constAccessor constants.ConstantValues, gasManager GasManager, eventMgr EventManager) error
	CalcAnchor(_ cosmos.Context, _ Manager, _ common.Asset) (cosmos.Uint, cosmos.Uint, cosmos.Uint)
}

// Slasher define all the method to perform slash
type Slasher interface {
	BeginBlock(ctx cosmos.Context, constAccessor constants.ConstantValues)
	LackSigning(ctx cosmos.Context, mgr Manager) error
	SlashVault(ctx cosmos.Context, vaultPK common.PubKey, coins common.Coins, mgr Manager) error
	IncSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress)
	DecSlashPoints(ctx cosmos.Context, point int64, addresses ...cosmos.AccAddress)
}

type ScheduledMigrationManager interface {
	EndBlock(ctx cosmos.Context, mgr Manager) error
}

// Mgrs is an implementation of Manager interface
type Mgrs struct {
	currentVersion            semver.Version
	constAccessor             constants.ConstantValues
	gasMgr                    GasManager
	eventMgr                  EventManager
	txOutStore                TxOutStore
	networkMgr                NetworkManager
	validatorMgr              ValidatorManager
	obMgr                     ObserverManager
	slasher                   Slasher
	scheduledMigrationManager ScheduledMigrationManager

	K             keeper.Keeper
	cdc           codec.Codec
	coinKeeper    bankkeeper.Keeper
	accountKeeper authkeeper.AccountKeeper
	upgradeKeeper *upgradekeeper.Keeper
	storeService  store.KVStoreService
}

// NewManagers  create a new Manager
func NewManagers(
	keeper keeper.Keeper,
	cdc codec.Codec,
	storeService store.KVStoreService,
	coinKeeper bankkeeper.Keeper,
	accountKeeper authkeeper.AccountKeeper,
	upgradeKeeper *upgradekeeper.Keeper,
) *Mgrs {
	return &Mgrs{
		K:             keeper,
		cdc:           cdc,
		coinKeeper:    coinKeeper,
		accountKeeper: accountKeeper,
		upgradeKeeper: upgradeKeeper,
		storeService:  storeService,
	}
}

func (mgr *Mgrs) GetVersion() semver.Version {
	return mgr.currentVersion
}

func (mgr *Mgrs) GetConstants() constants.ConstantValues {
	return mgr.constAccessor
}

// LoadManagerIfNecessary detect whether there are new version available, if it is available then create a new version of Mgr
func (mgr *Mgrs) LoadManagerIfNecessary(ctx cosmos.Context) error {
	v := mgr.K.GetLowestActiveVersion(ctx)

	if !v.Equals(mgr.GetVersion()) {
		// version is different , thus all the manager need to re-create
		if err := mgr.recreateManagers(ctx, v); err != nil {
			return err
		}
	}

	storedVer, hasStoredVer := mgr.Keeper().GetVersionWithCtx(ctx)
	if !hasStoredVer || v.GT(storedVer) {
		// store the version for contextual lookups if it has been upgraded
		mgr.Keeper().SetVersionWithCtx(ctx, v)
	}

	// Emit a version event as long as there was indeed a previously-recorded version changed from.
	// (As indicated by the keeper rather than by the manager.)
	if hasStoredVer && v.GT(storedVer) {
		evt := NewEventVersion(v)
		if err := mgr.EventMgr().EmitEvent(ctx, evt); err != nil {
			ctx.Logger().Error("fail to emit version event", "error", err)
		}
	}

	return nil
}

func (mgr *Mgrs) recreateManagers(ctx cosmos.Context, v semver.Version) error {
	mgr.currentVersion = v
	mgr.constAccessor = constants.GetConstantValues(v)
	var err error

	mgr.K, err = GetKeeper(v, mgr.cdc, mgr.storeService, mgr.coinKeeper, mgr.accountKeeper, mgr.upgradeKeeper)
	if err != nil {
		return fmt.Errorf("fail to create keeper: %w", err)
	}

	mgr.gasMgr, err = GetGasManager(v, mgr.K)
	if err != nil {
		return fmt.Errorf("fail to create gas manager: %w", err)
	}

	mgr.eventMgr, err = GetEventManager(v)
	if err != nil {
		return fmt.Errorf("fail to get event manager: %w", err)
	}

	mgr.txOutStore, err = GetTxOutStore(v, mgr.K, mgr.eventMgr, mgr.gasMgr)
	if err != nil {
		return fmt.Errorf("fail to get tx out store: %w", err)
	}

	mgr.networkMgr, err = GetNetworkManager(v, mgr.K, mgr.txOutStore, mgr.eventMgr)
	if err != nil {
		return fmt.Errorf("fail to get vault manager: %w", err)
	}

	mgr.validatorMgr, err = GetValidatorManager(v, mgr.K, mgr.networkMgr, mgr.txOutStore, mgr.eventMgr)
	if err != nil {
		return fmt.Errorf("fail to get validator manager: %w", err)
	}

	mgr.obMgr, err = GetObserverManager(v)
	if err != nil {
		return fmt.Errorf("fail to get observer manager: %w", err)
	}

	mgr.slasher, err = GetSlasher(v, mgr.K, mgr.eventMgr)
	if err != nil {
		return fmt.Errorf("fail to create slasher: %w", err)
	}

	mgr.scheduledMigrationManager, err = GetScheduledMigrationManager(v, mgr)
	if err != nil {
		return fmt.Errorf("fail to create scheduled migration manager: %w", err)
	}

	return nil
}

// Keeper return Keeper
func (mgr *Mgrs) Keeper() keeper.Keeper { return mgr.K }

// GasMgr return GasManager
func (mgr *Mgrs) GasMgr() GasManager { return mgr.gasMgr }

// EventMgr return EventMgr
func (mgr *Mgrs) EventMgr() EventManager { return mgr.eventMgr }

// TxOutStore return an TxOutStore
func (mgr *Mgrs) TxOutStore() TxOutStore { return mgr.txOutStore }

// VaultMgr return a valid NetworkManager
func (mgr *Mgrs) NetworkMgr() NetworkManager { return mgr.networkMgr }

// ValidatorMgr return an implementation of ValidatorManager
func (mgr *Mgrs) ValidatorMgr() ValidatorManager { return mgr.validatorMgr }

// ObMgr return an implementation of ObserverManager
func (mgr *Mgrs) ObMgr() ObserverManager { return mgr.obMgr }

// Slasher return an implementation of Slasher
func (mgr *Mgrs) Slasher() Slasher { return mgr.slasher }

func (mgr *Mgrs) ScheduledMigrationManager() ScheduledMigrationManager {
	return mgr.scheduledMigrationManager
}

// GetKeeper return Keeper
func GetKeeper(
	version semver.Version,
	cdc codec.BinaryCodec,
	storeService store.KVStoreService,
	coinKeeper bankkeeper.Keeper,
	accountKeeper authkeeper.AccountKeeper,
	upgradeKeeper *upgradekeeper.Keeper,
) (keeper.Keeper, error) {
	kvs := kv1.NewKVStore(cdc, storeService, coinKeeper, accountKeeper, upgradeKeeper, version)
	return &kvs, nil
}

// GetGasManager return GasManager
func GetGasManager(version semver.Version, keeper keeper.Keeper) (GasManager, error) {
	constAccessor := constants.GetConstantValues(version)
	return newGasMgr(constAccessor, keeper), nil
}

// GetEventManager will return an implementation of EventManager
func GetEventManager(version semver.Version) (EventManager, error) {
	return newEventMgr(), nil
}

// GetTxOutStore will return an implementation of the txout store that
func GetTxOutStore(version semver.Version, keeper keeper.Keeper, eventMgr EventManager, gasManager GasManager) (TxOutStore, error) {
	constAccessor := constants.GetConstantValues(version)
	return newTxOutStorage(keeper, constAccessor, eventMgr, gasManager), nil
}

// GetNetworkManager  retrieve a NetworkManager that is compatible with the given version
func GetNetworkManager(version semver.Version, keeper keeper.Keeper, txOutStore TxOutStore, eventMgr EventManager) (NetworkManager, error) {
	return newNetworkMgr(keeper, txOutStore, eventMgr), nil
}

// GetValidatorManager create a new instance of Validator Manager
func GetValidatorManager(_ semver.Version, keeper keeper.Keeper, networkMgr NetworkManager, txOutStore TxOutStore, eventMgr EventManager) (ValidatorManager, error) {
	return newValidatorMgr(keeper, networkMgr, txOutStore, eventMgr), nil
}

// GetObserverManager return an instance that implements ObserverManager interface
// when there is no version can match the given semver , it will return nil
func GetObserverManager(version semver.Version) (ObserverManager, error) {
	return newObserverMgr(), nil
}

// GetSlasher return an implementation of Slasher
func GetSlasher(version semver.Version, keeper keeper.Keeper, eventMgr EventManager) (Slasher, error) {
	return newSlasher(keeper, eventMgr), nil
}

func GetScheduledMigrationManager(_ semver.Version, mgr Manager) (ScheduledMigrationManager, error) {
	return newScheduledMigrationMgr(mgr), nil
}
