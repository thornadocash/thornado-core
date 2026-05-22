package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/gorilla/mux"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	reflectionv1 "cosmossdk.io/api/cosmos/reflection/v1"
	"cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"cosmossdk.io/x/upgrade"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	runtimeservices "github.com/cosmos/cosmos-sdk/runtime/services"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	"github.com/cosmos/cosmos-sdk/x/auth/migrations/legacytx"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	txmodule "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authz "github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	consensusparamkeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cast"

	appparams "github.com/thornadocash/go-thornado/app/params"
	"github.com/thornadocash/go-thornado/openapi"
	"github.com/thornadocash/go-thornado/x/thorchain"
	"github.com/thornadocash/go-thornado/x/thorchain/ebifrost"
	thorchainkeeper "github.com/thornadocash/go-thornado/x/thorchain/keeper"
	thorchainkeeperabci "github.com/thornadocash/go-thornado/x/thorchain/keeper/abci"
	thorchainkeeperv1 "github.com/thornadocash/go-thornado/x/thorchain/keeper/v1"
	thorchaintypes "github.com/thornadocash/go-thornado/x/thorchain/types"

	evm "github.com/cosmos/evm/encoding/codec"
	"github.com/cosmos/evm/ethereum/eip712"
)

const (
	appName = "thornado"
	NodeDir = ".thornado"
)

// These constants are derived from the above variables.
// These are the ones we will want to use in the code, based on
// any overrides above
var (
	// DefaultNodeHome default home directories for appd
	DefaultNodeHome = os.ExpandEnv("$HOME/") + NodeDir
)

// module account permissions
var maccPerms = map[string][]string{
	authtypes.FeeCollectorName:     nil,
	minttypes.ModuleName:           {authtypes.Minter},
	stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
	stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
	thorchain.ModuleName:           {authtypes.Minter, authtypes.Burner},
	thorchain.AsgardName:           {},
	thorchain.BondName:             {},
	thorchain.ReserveName:          {},
	thorchain.TreasuryName:         {},
}

var (
	_ runtime.AppI            = (*ThornadoApp)(nil)
	_ servertypes.Application = (*ThornadoApp)(nil)
)

// ChainApp extended ABCI application
type ThornadoApp struct {
	*baseapp.BaseApp
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry types.InterfaceRegistry

	// keys to access the substores
	keys  map[string]*storetypes.KVStoreKey
	tkeys map[string]*storetypes.TransientStoreKey

	// keepers
	AccountKeeper authkeeper.AccountKeeper
	AuthzKeeper   authzkeeper.Keeper
	BankKeeper    bankkeeper.BaseKeeper
	StakingKeeper *stakingkeeper.Keeper
	MintKeeper    mintkeeper.Keeper
	UpgradeKeeper *upgradekeeper.Keeper
	// trunk-ignore(golangci-lint/staticcheck): deprecated TODO: SDK 0.53 cleanup
	ParamsKeeper          paramskeeper.Keeper
	ConsensusParamsKeeper consensusparamkeeper.Keeper

	ThornadoKeeper   thorchainkeeper.Keeper
	EnshrinedBifrost *ebifrost.EnshrinedBifrost

	msgServiceRouter *MsgServiceRouter // router for redirecting Msg service messages

	// the module manager
	ModuleManager      *module.Manager
	BasicModuleManager module.BasicManager

	// simulation manager
	sm *module.SimulationManager

	// module configurator
	configurator module.Configurator
	once         sync.Once
}

// NewChainApp returns a reference to an initialized ChainApp.
func NewChainApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *ThornadoApp {
	ebifrostConfig, err := ebifrost.ReadEBifrostConfig(appOpts)
	if err != nil {
		panic(fmt.Sprintf("error while reading ebifrost config: %s", err))
	}

	ec := appparams.MakeEncodingConfig()
	interfaceRegistry := ec.InterfaceRegistry
	// This is needed for the EIP712 txs because currently is using
	// the deprecated method legacytx.StdSignBytes
	legacytx.RegressionTestingAminoCodec = ec.Amino
	eip712.SetEncodingConfig(ec.Amino, interfaceRegistry, 1)
	evm.RegisterLegacyAminoCodec(ec.Amino)
	evm.RegisterInterfaces(interfaceRegistry)

	// Below we could construct and set an application specific mempool and
	// ABCI 1.0 PrepareProposal and ProcessProposal handlers. These defaults are
	// already set in the SDK's BaseApp, this shows an example of how to override
	// them.
	//
	// Example:
	//
	// bApp := baseapp.NewBaseApp(...)
	// nonceMempool := mempool.NewSenderNonceMempool()
	// abciPropHandler := NewDefaultProposalHandler(nonceMempool, bApp)
	//
	// bApp.SetMempool(nonceMempool)
	// bApp.SetPrepareProposal(abciPropHandler.PrepareProposalHandler())
	// bApp.SetProcessProposal(abciPropHandler.ProcessProposalHandler())
	//
	// Alternatively, you can construct BaseApp options, append those to
	// baseAppOptions and pass them to NewBaseApp.
	//
	// Example:
	//
	// prepareOpt = func(app *baseapp.BaseApp) {
	// 	abciPropHandler := baseapp.NewDefaultProposalHandler(nonceMempool, app)
	// 	app.SetPrepareProposal(abciPropHandler.PrepareProposalHandler())
	// }
	// baseAppOptions = append(baseAppOptions, prepareOpt)

	// create and set dummy vote extension handler
	// voteExtOp := func(bApp *baseapp.BaseApp) {
	//	voteExtHandler := NewVoteExtensionHandler()
	//	voteExtHandler.SetHandlers(bApp)
	// }
	// baseAppOptions = append(baseAppOptions, voteExtOp)

	bApp := baseapp.NewBaseApp(appName, logger, db, ec.TxConfig.TxDecoder(), baseAppOptions...)
	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetVersion(version.Version)

	bApp.SetTxEncoder(ec.TxConfig.TxEncoder())

	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey,
		authzkeeper.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		minttypes.StoreKey,
		paramstypes.StoreKey,
		consensusparamtypes.StoreKey,
		upgradetypes.StoreKey,
		// non sdk store keys
		thorchaintypes.StoreKey,
	)

	tkeys := storetypes.NewTransientStoreKeys(paramstypes.TStoreKey)

	// register streaming services
	if err = bApp.RegisterStreamingServices(appOpts, keys); err != nil {
		panic(err)
	}

	app := &ThornadoApp{
		BaseApp:           bApp,
		legacyAmino:       ec.Amino,
		appCodec:          ec.Codec,
		txConfig:          ec.TxConfig,
		interfaceRegistry: interfaceRegistry,
		keys:              keys,
		tkeys:             tkeys,
		msgServiceRouter:  NewMsgServiceRouter(bApp.MsgServiceRouter()),
	}
	app.SetInterfaceRegistry(interfaceRegistry)

	app.ParamsKeeper = initParamsKeeper(
		app.appCodec,
		app.legacyAmino,
		keys[paramstypes.StoreKey],
		tkeys[paramstypes.TStoreKey],
	)

	// set the BaseApp's parameter store
	app.ConsensusParamsKeeper = consensusparamkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(keys[consensusparamtypes.StoreKey]),
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
		runtime.EventService{},
	)
	bApp.SetParamStore(app.ConsensusParamsKeeper.ParamsStore)

	// add keepers

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		app.appCodec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
	)
	app.AuthzKeeper = authzkeeper.NewKeeper(
		runtime.NewKVStoreService(keys[authzkeeper.StoreKey]),
		app.appCodec,
		app.MsgServiceRouter(),
		app.AccountKeeper,
	)
	app.BankKeeper = bankkeeper.NewBaseKeeper(
		app.appCodec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		app.AccountKeeper,
		BlockedAddresses(),
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
		logger,
	)

	txSigningOptions, err := tx.NewDefaultSigningOptions()
	if err != nil {
		panic(err)
	}
	thorchaintypes.DefineCustomGetSigners(txSigningOptions)
	txConfig, err := appparams.TxConfig(app.appCodec, txmodule.NewBankKeeperCoinMetadataQueryFn(app.BankKeeper))
	if err != nil {
		panic(err)
	}
	app.txConfig = txConfig

	app.StakingKeeper = stakingkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(keys[stakingtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	)
	app.MintKeeper = mintkeeper.NewKeeper(
		app.appCodec,
		runtime.NewKVStoreService(keys[minttypes.StoreKey]),
		app.StakingKeeper,
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
	)

	// get skipUpgradeHeights from the app options
	skipUpgradeHeights := map[int64]bool{}
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}
	homePath := cast.ToString(appOpts.Get(flags.FlagHome))
	// set the governance module account as the authority for conducting upgrades
	app.UpgradeKeeper = upgradekeeper.NewKeeper(
		skipUpgradeHeights,
		runtime.NewKVStoreService(keys[upgradetypes.StoreKey]),
		app.appCodec,
		homePath,
		app.BaseApp,
		authtypes.NewModuleAddress(thorchain.ModuleName).String(),
	)

	app.ThornadoKeeper = thorchainkeeperv1.NewKeeper(
		app.appCodec, runtime.NewKVStoreService(keys[thorchaintypes.StoreKey]), app.BankKeeper, app.AccountKeeper, app.UpgradeKeeper,
	)

	// --- Module Options ---
	telemetryEnabled := cast.ToBool(appOpts.Get("telemetry.enabled"))
	testApp := cast.ToBool(appOpts.Get(TestApp))

	mgrs := thorchain.NewManagers(app.ThornadoKeeper, app.appCodec, runtime.NewKVStoreService(keys[thorchaintypes.StoreKey]), app.BankKeeper, app.AccountKeeper, app.UpgradeKeeper)
	app.msgServiceRouter.AddCustomRoute("cosmos.bank.v1beta1.Msg", thorchain.NewBankSendHandler(thorchain.NewSendHandler(mgrs)))

	thorchainModule := thorchain.NewAppModule(mgrs, telemetryEnabled, testApp)
	app.EnshrinedBifrost = ebifrost.NewEnshrinedBifrost(app.appCodec, logger, ebifrostConfig)

	defaultProposalHandler := baseapp.NewDefaultProposalHandler(bApp.Mempool(), bApp)
	eBifrostProposalHandler := thorchainkeeperabci.NewProposalHandler(
		&app.ThornadoKeeper,
		app.EnshrinedBifrost,
		interfaceRegistry,
		defaultProposalHandler.PrepareProposalHandler(),
		defaultProposalHandler.ProcessProposalHandler(),
	)

	bApp.SetPrepareProposal(eBifrostProposalHandler.PrepareProposal)
	bApp.SetProcessProposal(eBifrostProposalHandler.ProcessProposal)

	// NOTE: Any module instantiated in the module manager that is later modified
	// must be passed by reference here.
	authModule := auth.NewAppModule(app.appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts, app.GetSubspace(authtypes.ModuleName))
	authzModule := authzmodule.NewAppModule(app.appCodec, app.AuthzKeeper, app.AccountKeeper, app.BankKeeper, app.InterfaceRegistry())
	bankModule := bank.NewAppModule(app.appCodec, app.BankKeeper, app.AccountKeeper, app.GetSubspace(banktypes.ModuleName))
	consensusModule := consensus.NewAppModule(app.appCodec, app.ConsensusParamsKeeper)
	genutilModule := genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app, txConfig)
	// trunk-ignore(golangci-lint/staticcheck): deprecated TODO: SDK 0.53 cleanup
	paramsModule := params.NewAppModule(app.ParamsKeeper)
	upgradeModule := upgrade.NewAppModule(app.UpgradeKeeper, app.AccountKeeper.AddressCodec())

	app.ModuleManager = module.NewManager(
		genutilModule,
		authModule,
		authzModule,
		bankModule,
		upgradeModule,
		paramsModule,
		consensusModule,
		// non sdk modules
		thorchainModule,
	)

	// BasicModuleManager defines the module BasicManager is in charge of setting up basic,
	// non-dependant module elements, such as codec registration and genesis verification.
	// By default it is composed of all the module from the module manager.
	// Additionally, app module basics can be overwritten by passing them as argument.
	app.BasicModuleManager = module.NewBasicManager(
		genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
		authModule,
		authzModule,
		bankModule,
		upgradeModule,
		paramsModule,
		consensusModule,
		mint.NewAppModule(app.appCodec, app.MintKeeper, app.AccountKeeper, nil, app.GetSubspace(minttypes.ModuleName)),
		// non sdk modules
		thorchainModule,
	)
	app.BasicModuleManager.RegisterLegacyAminoCodec(app.legacyAmino)
	app.BasicModuleManager.RegisterInterfaces(interfaceRegistry)

	// NOTE: upgrade module is required to be prioritized
	app.ModuleManager.SetOrderPreBlockers(
		upgradetypes.ModuleName,
		authtypes.ModuleName,
	)
	// NOTE: staking module is required if HistoricalEntries param > 0
	app.ModuleManager.SetOrderBeginBlockers(
		genutiltypes.ModuleName,
		authz.ModuleName,

		// additional non simd modules
		thorchaintypes.ModuleName,
	)

	app.ModuleManager.SetOrderEndBlockers(
		genutiltypes.ModuleName,
		authz.ModuleName,

		// additional non simd modules
		thorchaintypes.ModuleName,
	)

	// NOTE: The genutils module must occur after staking so that pools are
	// properly initialized with tokens from genesis accounts.
	// NOTE: The genutils module must also occur after auth so that it can access the params from auth.
	genesisModuleOrder := []string{
		// simd modules
		authtypes.ModuleName,
		authz.ModuleName,
		banktypes.ModuleName,
		genutiltypes.ModuleName,
		paramstypes.ModuleName,
		upgradetypes.ModuleName,
		consensusparamtypes.ModuleName,
		thorchaintypes.ModuleName,
	}
	app.ModuleManager.SetOrderInitGenesis(genesisModuleOrder...)
	app.ModuleManager.SetOrderExportGenesis(genesisModuleOrder...)

	// Uncomment if you want to set a custom migration order here.
	// app.ModuleManager.SetOrderMigrations(custom order)

	app.configurator = module.NewConfigurator(app.appCodec, app.msgServiceRouter, app.GRPCQueryRouter())
	err = app.ModuleManager.RegisterServices(app.configurator)
	if err != nil {
		panic(err)
	}

	// RegisterUpgradeHandlers is used for registering any on-chain upgrades.
	// Make sure it's called after `app.ModuleManager` and `app.configurator` are set.
	app.RegisterUpgradeHandlers()

	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.ModuleManager.Modules))

	reflectionSvc, err := runtimeservices.NewReflectionService()
	if err != nil {
		panic(err)
	}
	reflectionv1.RegisterReflectionServiceServer(app.GRPCQueryRouter(), reflectionSvc)

	// add test gRPC service for testing gRPC queries in isolation
	// testdata_pulsar.RegisterQueryServer(app.GRPCQueryRouter(), testdata_pulsar.QueryImpl{})

	// create the simulation manager and define the order of the modules for deterministic simulations
	//
	// NOTE: this is not required apps that don't use the simulator for fuzz testing
	// transactions
	overrideModules := map[string]module.AppModuleSimulation{
		authtypes.ModuleName: auth.NewAppModule(app.appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts, app.GetSubspace(authtypes.ModuleName)),
	}
	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, overrideModules)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)

	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)

	anteHandler, err := NewAnteHandler(
		HandlerOptions{
			HandlerOptions: ante.HandlerOptions{
				AccountKeeper:   app.AccountKeeper,
				BankKeeper:      app.BankKeeper,
				SignModeHandler: txConfig.SignModeHandler(),
				SigGasConsumer:  SigGasConsumer,
			},
			ThornadoKeeper: app.ThornadoKeeper,
		},
	)
	if err != nil {
		panic(fmt.Errorf("failed to create AnteHandler: %s", err))
	}
	app.SetAnteHandler(anteHandler)

	// In v0.46, the SDK introduces _postHandlers_. PostHandlers are like
	// antehandlers, but are run _after_ the `runMsgs` execution. They are also
	// defined as a chain, and have the same signature as antehandlers.
	//
	// In baseapp, postHandlers are run in the same store branch as `runMsgs`,
	// meaning that both `runMsgs` and `postHandler` state will be committed if
	// both are successful, and both will be reverted if any of the two fails.
	//
	// The SDK exposes a default postHandlers chain
	//
	// Please note that changing any of the anteHandler or postHandler chain is
	// likely to be a state-machine breaking change, which needs a coordinated
	// upgrade.
	app.setPostHandler()

	// At startup, after all modules have been registered, check that all proto
	// annotations are correct.
	protoFiles, err := proto.MergedRegistry()
	if err != nil {
		panic(err)
	}
	err = msgservice.ValidateProtoAnnotations(protoFiles)
	if err != nil {
		// Once we switch to using protoreflect-based antehandlers, we might
		// want to panic here instead of logging a warning.
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}

	if loadLatest {
		if err = app.LoadLatestVersion(); err != nil {
			panic(fmt.Errorf("error loading last version: %w", err))
		}
	}

	return app
}

func (app *ThornadoApp) FinalizeBlock(req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	// when skipping sdk 47 for sdk 50, the upgrade handler is called too late in BaseApp
	// this is a hack to ensure that the migration is executed when needed and not panics
	app.once.Do(func() {
		ctx := app.NewUncachedContext(false, tmproto.Header{})
		if _, err := app.ConsensusParamsKeeper.Params(ctx, &consensusparamtypes.QueryParamsRequest{}); err != nil {
			// prevents panic: consensus key is nil: collections: not found: key 'no_key' of type github.com/cosmos/gogoproto/tendermint.types.ConsensusParams
			// sdk 47:
			// Migrate Tendermint consensus parameters from x/params module to a dedicated x/consensus module.
			// see https://github.com/cosmos/cosmos-sdk/blob/v0.47.0/simapp/upgrades.go#L66
			baseAppLegacySS := app.GetSubspace(baseapp.Paramspace).WithKeyTable(paramstypes.ConsensusParamsKeyTable())
			err = baseapp.MigrateParams(sdk.UnwrapSDKContext(ctx), baseAppLegacySS, app.ConsensusParamsKeeper.ParamsStore)
			if err != nil {
				panic(err)
			}
		}
	})

	return app.BaseApp.FinalizeBlock(req)
}

func (app *ThornadoApp) setPostHandler() {
	app.SetPostHandler(sdk.ChainPostDecorators(ebifrost.NewEnshrineBifrostPostDecorator(app.EnshrinedBifrost)))
}

// Name returns the name of the App
func (app *ThornadoApp) Name() string { return app.BaseApp.Name() }

// PreBlocker application updates every pre block
func (app *ThornadoApp) PreBlocker(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	return app.ModuleManager.PreBlock(ctx)
}

func (a *ThornadoApp) Configurator() module.Configurator {
	return a.configurator
}

// InitChainer application update at chain initialization
func (app *ThornadoApp) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	err := app.UpgradeKeeper.SetModuleVersionMap(ctx, app.ModuleManager.GetVersionMap())
	if err != nil {
		panic(err)
	}
	response, err := app.ModuleManager.InitGenesis(ctx, app.appCodec, genesisState)
	return response, err
}

// LoadHeight loads a particular height
func (app *ThornadoApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// LegacyAmino returns legacy amino codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *ThornadoApp) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// AppCodec returns app codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *ThornadoApp) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns ChainApp's InterfaceRegistry
func (app *ThornadoApp) InterfaceRegistry() types.InterfaceRegistry {
	return app.interfaceRegistry
}

// TxConfig returns ChainApp's TxConfig
func (app *ThornadoApp) TxConfig() client.TxConfig {
	return app.txConfig
}

// AutoCliOpts returns the autocli options for the app.
func (app *ThornadoApp) AutoCliOpts() autocli.AppOptions {
	modules := make(map[string]appmodule.AppModule, 0)
	for _, m := range app.ModuleManager.Modules {
		if moduleWithName, ok := m.(module.HasName); ok {
			moduleName := moduleWithName.Name()
			if appModule, ok2 := moduleWithName.(appmodule.AppModule); ok2 {
				modules[moduleName] = appModule
			}
		}
	}

	return autocli.AppOptions{
		Modules:               modules,
		ModuleOptions:         runtimeservices.ExtractAutoCLIOptions(app.ModuleManager.Modules),
		AddressCodec:          authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		ValidatorAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		ConsensusAddressCodec: authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	}
}

// DefaultGenesis returns a default genesis from the registered AppModuleBasic's.
func (a *ThornadoApp) DefaultGenesis() map[string]json.RawMessage {
	return a.BasicModuleManager.DefaultGenesis(a.appCodec)
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *ThornadoApp) GetKey(storeKey string) *storetypes.KVStoreKey {
	return app.keys[storeKey]
}

// GetStoreKeys returns all the stored store keys.
func (app *ThornadoApp) GetStoreKeys() []storetypes.StoreKey {
	keys := make([]storetypes.StoreKey, 0, len(app.keys))
	for _, key := range app.keys {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Name() < keys[j].Name()
	})
	return keys
}

// GetTKey returns the TransientStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *ThornadoApp) GetTKey(storeKey string) *storetypes.TransientStoreKey {
	return app.tkeys[storeKey]
}

// GetSubspace returns a param subspace for a given module name.
//
// NOTE: This is solely to be used for testing purposes.
func (app *ThornadoApp) GetSubspace(moduleName string) paramstypes.Subspace {
	subspace, _ := app.ParamsKeeper.GetSubspace(moduleName)
	return subspace
}

// SimulationManager implements the SimulationApp interface
func (app *ThornadoApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *ThornadoApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx

	// Must call this before registering any GRPC gateway routes
	thorchain.CustomGRPCGatewayRouter(apiSvr)

	// Register new tx routes from grpc-gateway.
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register new CometBFT queries routes from grpc-gateway.
	cmtservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register node gRPC service for grpc-gateway.
	nodeservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register grpc-gateway routes for all modules.
	app.BasicModuleManager.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// register thornado-specific swagger API from root so that other applications can override easily
	if err := RegisterSwaggerAPI(apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}

	// register built in cosmos swagger API
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

// RegisterSwaggerAPI provides a common function which registers swagger route with API Server
func RegisterSwaggerAPI(rtr *mux.Router, swaggerEnabled bool) error {
	// Health Check Endpoint
	rtr.HandleFunc(
		fmt.Sprintf("/%s/ping", thorchain.ModuleName),
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"ping":"pong"}`)
		},
	).Methods(http.MethodGet, http.MethodOptions)

	if !swaggerEnabled {
		return nil
	}

	// api doc handlers
	rtr.HandleFunc(fmt.Sprintf("/%s/doc/openapi.yaml", thorchain.ModuleName), openapi.HandleSpecYAML)
	rtr.HandleFunc(fmt.Sprintf("/%s/doc/openapi.json", thorchain.ModuleName), openapi.HandleSpecJSON)
	rtr.HandleFunc(fmt.Sprintf("/%s/doc", thorchain.ModuleName), openapi.HandleSwaggerUI)
	rtr.HandleFunc(fmt.Sprintf("/%s/doc/", thorchain.ModuleName), openapi.HandleSwaggerUI)

	return nil
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *ThornadoApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *ThornadoApp) RegisterTendermintService(clientCtx client.Context) {
	cmtApp := server.NewCometABCIWrapper(app)
	cmtservice.RegisterTendermintService(
		clientCtx,
		app.BaseApp.GRPCQueryRouter(),
		app.interfaceRegistry,
		cmtApp.Query,
	)
}

func (app *ThornadoApp) RegisterNodeService(clientCtx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg)

	if err := app.EnshrinedBifrost.Start(); err != nil && !errors.Is(err, ebifrost.ErrAlreadyStarted) {
		panic(fmt.Errorf("failed to start bifrost service: %w", err))
	}
}

func (app *ThornadoApp) Close() error {
	app.EnshrinedBifrost.Stop()

	return app.BaseApp.Close()
}

// GetMaccPerms returns a copy of the module account permissions
//
// NOTE: This is solely to be used for testing purposes.
func GetMaccPerms() map[string][]string {
	dupMaccPerms := make(map[string][]string)
	for k, v := range maccPerms {
		dupMaccPerms[k] = v
	}

	return dupMaccPerms
}

// BlockedAddresses returns all the app's blocked account addresses.
func BlockedAddresses() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range GetMaccPerms() {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}

	// Allow lending and treasury to receive funds
	delete(modAccAddrs, authtypes.NewModuleAddress(thorchaintypes.LendingName).String())
	delete(modAccAddrs, authtypes.NewModuleAddress(thorchaintypes.TreasuryName).String())

	return modAccAddrs
}

// MsgServiceRouter returns the MsgServiceRouter.
func (app *ThornadoApp) MsgServiceRouter() *MsgServiceRouter {
	return app.msgServiceRouter
}

// SetInterfaceRegistry sets the InterfaceRegistry.
func (app *ThornadoApp) SetInterfaceRegistry(registry types.InterfaceRegistry) {
	app.interfaceRegistry = registry
	app.msgServiceRouter.SetInterfaceRegistry(registry)
	app.BaseApp.SetInterfaceRegistry(registry)
}

// trunk-ignore(golangci-lint/staticcheck): deprecated TODO: SDK 0.53 cleanup
func initParamsKeeper(appCodec codec.BinaryCodec, legacyAmino *codec.LegacyAmino, key, tkey storetypes.StoreKey) paramskeeper.Keeper {
	// trunk-ignore(golangci-lint/staticcheck): deprecated TODO: SDK 0.53 cleanup
	paramsKeeper := paramskeeper.NewKeeper(appCodec, legacyAmino, key, tkey)

	// required for testing finalized block migration
	paramsKeeper.Subspace(baseapp.Paramspace)

	paramsKeeper.Subspace(authtypes.ModuleName)
	paramsKeeper.Subspace(banktypes.ModuleName)
	paramsKeeper.Subspace(stakingtypes.ModuleName)
	paramsKeeper.Subspace(minttypes.ModuleName)

	return paramsKeeper
}
