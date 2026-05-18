package thorchain

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	sdklog "cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	se "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	thorlog "gitlab.com/thorchain/thornode/v3/log"

	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	kv1 "gitlab.com/thorchain/thornode/v3/x/thorchain/keeper/v1"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"

	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

var errKaboom = errors.New("kaboom")

func logger() thorlog.SdkLogWrapper {
	// disable log output unless -v was provided to the test command
	level := zerolog.Disabled
	if testing.Verbose() {
		level = zerolog.DebugLevel
	}
	newLogger := log.Logger.
		Level(level).
		Output(zerolog.ConsoleWriter{Out: os.Stdout}).
		With().
		CallerWithSkipFrameCount(3).Logger()
	return thorlog.SdkLogWrapper{
		Logger: &newLogger,
	}
}

type HandlerSuite struct{}

var _ = Suite(&HandlerSuite{})

func (s *HandlerSuite) SetUpSuite(*C) {
	SetupConfigForTest()
}

func FundModule(c *C, ctx cosmos.Context, k keeper.Keeper, name string, amt uint64) {
	coin := common.NewCoin(common.RuneNative, cosmos.NewUint(amt))
	err := k.MintToModule(ctx, ModuleName, coin)
	c.Assert(err, IsNil)
	err = k.SendFromModuleToModule(ctx, ModuleName, name, common.NewCoins(coin))
	c.Assert(err, IsNil)
}

func FundAccount(c *C, ctx cosmos.Context, k keeper.Keeper, addr cosmos.AccAddress, amt uint64) {
	coin := common.NewCoin(common.RuneNative, cosmos.NewUint(amt))
	err := k.MintToModule(ctx, ModuleName, coin)
	c.Assert(err, IsNil)
	err = k.SendFromModuleToAccount(ctx, ModuleName, addr, common.NewCoins(coin))
	c.Assert(err, IsNil)
}

var (
	keyThorchain     = storetypes.NewKVStoreKey(StoreKey)
	serviceThorchain = runtime.NewKVStoreService(keyThorchain)
)

func setupManagerForTest(c *C) (cosmos.Context, *Mgrs) {
	SetupConfigForTest()
	keyAcc := cosmos.NewKVStoreKey(authtypes.StoreKey)
	keyBank := cosmos.NewKVStoreKey(banktypes.StoreKey)
	keyUpgrade := cosmos.NewKVStoreKey(upgradetypes.StoreKey)
	keyWasm := cosmos.NewKVStoreKey(wasmtypes.StoreKey)

	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, sdklog.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(keyAcc, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyThorchain, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyBank, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyWasm, cosmos.StoreTypeIAVL, db)

	err := ms.LoadLatestVersion()
	c.Assert(err, IsNil)

	ctx := cosmos.NewContext(ms, tmproto.Header{ChainID: "thorchain"}, false, logger())
	ctx = ctx.WithBlockHeight(18)
	ctx = ctx.WithBlockTime(time.Now())
	encodingConfig := testutil.MakeTestEncodingConfig(
		bank.AppModuleBasic{},
		auth.AppModuleBasic{},
	)

	ak := authkeeper.NewAccountKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyAcc),
		authtypes.ProtoBaseAccount,
		map[string][]string{
			types.ModuleName:             {authtypes.Minter, authtypes.Burner},
			types.AsgardName:             {},
			types.BondName:               {},
			types.ReserveName:            {},
			types.LendingName:            {},
			types.AffiliateCollectorName: {},
			types.TreasuryName:           {},
			types.RUNEPoolName:           {},
			types.TCYStakeName:           {},
			types.TCYClaimingName:        {},
			types.POLReserveName:         {},
		},
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress(ModuleName).String(),
	)

	bk := bankkeeper.NewBaseKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyBank),
		ak,
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
		sdklog.NewNopLogger(),
	)
	wasmDir := c.MkDir()
	wk := wasmkeeper.NewKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyWasm),
		ak, bk,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, wasmDir,
		wasmtypes.DefaultNodeConfig(), wasmtypes.VMConfig{}, wasmkeeper.BuiltInCapabilities(),
		authtypes.NewModuleAddress(ModuleName).String(),
	)

	err = wk.SetParams(ctx, wasmtypes.Params{
		CodeUploadAccess:             wasmtypes.AllowNobody,
		InstantiateDefaultPermission: wasmtypes.AccessTypeNobody,
	})
	c.Assert(err, IsNil)

	c.Assert(bk.MintCoins(ctx, ModuleName, cosmos.Coins{
		cosmos.NewCoin(common.RuneAsset().Native(), cosmos.NewInt(200_000_000_00000000)),
	}), IsNil)
	uk := upgradekeeper.NewKeeper(
		nil,
		runtime.NewKVStoreService(keyUpgrade),
		encodingConfig.Codec,
		c.MkDir(),
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
	)
	k := kv1.NewKeeper(encodingConfig.Codec, serviceThorchain, bk, ak, uk)
	FundModule(c, ctx, k, ModuleName, 10_000*common.One)
	FundModule(c, ctx, k, AsgardName, 100_000_000*common.One)
	FundModule(c, ctx, k, ReserveName, 100_000_000*common.One)
	c.Assert(k.SaveNetworkFee(ctx, common.ETHChain, NetworkFee{
		Chain:              common.ETHChain,
		TransactionSize:    1,
		TransactionFeeRate: 375_000, // 375,000 gwei
	}), IsNil)

	c.Assert(k.SaveNetworkFee(ctx, common.BTCChain, NetworkFee{
		Chain:              common.BTCChain,
		TransactionSize:    1,
		TransactionFeeRate: 6423600,
	}), IsNil)

	c.Assert(k.SaveNetworkFee(ctx, common.DOGEChain, NetworkFee{
		Chain:              common.DOGEChain,
		TransactionSize:    1,
		TransactionFeeRate: 37500,
	}), IsNil)

	os.Setenv("NET", "mocknet")
	mgr := NewManagers(k, encodingConfig.Codec, serviceThorchain, bk, ak, uk, wk)
	constants.SWVersion = GetCurrentVersion()

	_, hasVerStored := k.GetVersionWithCtx(ctx)
	c.Assert(hasVerStored, Equals, false,
		Commentf("version should not be stored until BeginBlock"))

	c.Assert(mgr.LoadManagerIfNecessary(ctx), IsNil)
	mgr.gasMgr.BeginBlock()

	verStored, hasVerStored := k.GetVersionWithCtx(ctx)
	c.Assert(hasVerStored, Equals, true,
		Commentf("version should be stored"))
	verComputed := k.GetLowestActiveVersion(ctx)
	c.Assert(verStored.String(), Equals, verComputed.String(),
		Commentf("stored version should match computed version"))

	return ctx, mgr
}

func setupKeeperForTest(c *C) (cosmos.Context, keeper.Keeper) {
	SetupConfigForTest()
	keyAcc := cosmos.NewKVStoreKey(authtypes.StoreKey)
	keyBank := cosmos.NewKVStoreKey(banktypes.StoreKey)
	keyUpgrade := cosmos.NewKVStoreKey(upgradetypes.StoreKey)

	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, sdklog.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(keyAcc, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyThorchain, cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keyBank, cosmos.StoreTypeIAVL, db)
	err := ms.LoadLatestVersion()
	c.Assert(err, IsNil)

	ctx := cosmos.NewContext(ms, tmproto.Header{ChainID: "thorchain"}, false, logger())
	ctx = ctx.WithBlockHeight(18)
	ctx = ctx.WithTxBytes([]byte("tx"))

	encodingConfig := testutil.MakeTestEncodingConfig(
		bank.AppModuleBasic{},
		auth.AppModuleBasic{},
	)

	ak := authkeeper.NewAccountKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyAcc),
		authtypes.ProtoBaseAccount,
		map[string][]string{
			types.ModuleName:             {authtypes.Minter, authtypes.Burner},
			types.AsgardName:             {},
			types.BondName:               {},
			types.ReserveName:            {},
			types.TreasuryName:           {},
			types.LendingName:            {},
			types.AffiliateCollectorName: {},
			types.TCYStakeName:           {},
			types.TCYClaimingName:        {},
			types.POLReserveName:         {},
		},
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress(ModuleName).String(),
	)

	bk := bankkeeper.NewBaseKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keyBank),
		ak,
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
		sdklog.NewNopLogger(),
	)

	c.Assert(bk.MintCoins(ctx, ModuleName, cosmos.Coins{
		cosmos.NewCoin(common.RuneAsset().Native(), cosmos.NewInt(200_000_000_00000000)),
	}), IsNil)
	c.Assert(bk.BurnCoins(ctx, ModuleName, cosmos.Coins{
		cosmos.NewCoin(common.RuneAsset().Native(), cosmos.NewInt(200_000_000_00000000)),
	}), IsNil)
	uk := upgradekeeper.NewKeeper(
		nil,
		runtime.NewKVStoreService(keyUpgrade),
		encodingConfig.Codec,
		c.MkDir(),
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
	)
	k := kv1.NewKVStore(encodingConfig.Codec, serviceThorchain, bk, ak, uk, GetCurrentVersion())
	FundModule(c, ctx, k, ModuleName, 1000000*common.One)
	FundModule(c, ctx, k, AsgardName, common.One)
	FundModule(c, ctx, k, ReserveName, 10000*common.One)
	err = k.SaveNetworkFee(ctx, common.ETHChain, NetworkFee{
		Chain:              common.ETHChain,
		TransactionSize:    1,
		TransactionFeeRate: 375_000, // 375,000 gwei
	})
	c.Assert(err, IsNil)
	err = k.SaveNetworkFee(ctx, common.BTCChain, NetworkFee{
		Chain:              common.BTCChain,
		TransactionSize:    1,
		TransactionFeeRate: 6423600,
	})
	c.Assert(err, IsNil)
	os.Setenv("NET", "mocknet")
	return ctx, &k
}

type handlerTestWrapper struct {
	ctx                  cosmos.Context
	keeper               keeper.Keeper
	mgr                  Manager
	activeNodeAccount    NodeAccount
	notActiveNodeAccount NodeAccount
}

func getHandlerTestWrapper(c *C, height int64, withActiveNode, withActieDOGEPool bool) handlerTestWrapper {
	ctx, mgr := setupManagerForTest(c)
	ctx = ctx.WithBlockHeight(height)
	acc1 := GetRandomValidatorNode(NodeActive)
	acc1.Version = mgr.GetVersion().String()
	if withActiveNode {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, acc1), IsNil)
	}
	if withActieDOGEPool {
		p, err := mgr.Keeper().GetPool(ctx, common.DOGEAsset)
		c.Assert(err, IsNil)
		p.Asset = common.DOGEAsset
		p.Status = PoolAvailable
		p.BalanceRune = cosmos.NewUint(100 * common.One)
		p.BalanceAsset = cosmos.NewUint(100 * common.One)
		p.LPUnits = cosmos.NewUint(100 * common.One)
		c.Assert(mgr.Keeper().SetPool(ctx, p), IsNil)
	}

	FundModule(c, ctx, mgr.Keeper(), AsgardName, common.One)

	c.Assert(mgr.ValidatorMgr().BeginBlock(ctx, mgr, nil), IsNil)

	return handlerTestWrapper{
		ctx:                  ctx,
		keeper:               mgr.Keeper(),
		mgr:                  mgr,
		activeNodeAccount:    acc1,
		notActiveNodeAccount: GetRandomValidatorNode(NodeDisabled),
	}
}

func (HandlerSuite) TestHandleTxInWithdrawLiquidityMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)

	vault := GetRandomVault()
	vault.Coins = common.Coins{
		common.NewCoin(common.DOGEAsset, cosmos.NewUint(100*common.One)),
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100*common.One)),
	}
	c.Assert(w.keeper.SetVault(w.ctx, vault), IsNil)
	vaultAddr, err := vault.PubKey.GetAddress(common.ETHChain)

	pool := NewPool()
	pool.Asset = common.DOGEAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.LPUnits = cosmos.NewUint(100)
	c.Assert(w.keeper.SetPool(w.ctx, pool), IsNil)

	runeAddr := GetRandomRUNEAddress()
	lp := LiquidityProvider{
		Asset:        common.DOGEAsset,
		RuneAddress:  runeAddr,
		AssetAddress: GetRandomDOGEAddress(),
		PendingRune:  cosmos.ZeroUint(),
		Units:        cosmos.NewUint(100),
	}
	w.keeper.SetLiquidityProvider(w.ctx, lp)

	tx := common.Tx{
		ID:    GetRandomTxHash(),
		Chain: common.ETHChain,
		Coins: common.Coins{
			common.NewCoin(common.RuneAsset(), cosmos.NewUint(1*common.One)),
		},
		Memo:        "withdraw:DOGE.DOGE",
		FromAddress: lp.RuneAddress,
		ToAddress:   vaultAddr,
		Gas: common.Gas{
			common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
		},
	}

	msg := NewMsgWithdrawLiquidity(tx, lp.RuneAddress, cosmos.NewUint(uint64(MaxWithdrawBasisPoints)), common.DOGEAsset, common.EmptyAsset, w.activeNodeAccount.NodeAddress)
	c.Assert(err, IsNil)

	handler := NewInternalHandler(w.mgr)

	FundModule(c, w.ctx, w.keeper, AsgardName, 500*common.One)
	c.Assert(w.keeper.SaveNetworkFee(w.ctx, common.DOGEChain, NetworkFee{
		Chain:              common.DOGEChain,
		TransactionSize:    1,
		TransactionFeeRate: 10000,
	}), IsNil)

	_, err = handler(w.ctx, msg)
	c.Assert(err, IsNil)
	pool, err = w.keeper.GetPool(w.ctx, common.DOGEAsset)
	c.Assert(err, IsNil)
	c.Assert(pool.IsEmpty(), Equals, false)
	c.Check(pool.Status, Equals, PoolStaged)
	c.Check(pool.LPUnits.Uint64(), Equals, uint64(0), Commentf("%d", pool.LPUnits.Uint64()))
	c.Check(pool.BalanceRune.Uint64(), Equals, uint64(0), Commentf("%d", pool.BalanceRune.Uint64()))
	remainGas := uint64(15000)
	c.Check(pool.BalanceAsset.Uint64(), Equals, remainGas, Commentf("%d", pool.BalanceAsset.Uint64())) // leave a little behind for gas
}

func (HandlerSuite) TestRefund(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)

	pool := Pool{
		Asset:        common.DOGEAsset,
		BalanceRune:  cosmos.NewUint(100 * common.One),
		BalanceAsset: cosmos.NewUint(100 * common.One),
	}
	c.Assert(w.keeper.SetPool(w.ctx, pool), IsNil)

	vault := GetRandomVault()
	c.Assert(w.keeper.SetVault(w.ctx, vault), IsNil)

	txin := NewObservedTx(
		common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.DOGEChain,
			Coins: common.Coins{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(100*common.One)),
			},
			Memo:        "withdraw:DOGE.DOGE",
			FromAddress: GetRandomDOGEAddress(),
			ToAddress:   GetRandomDOGEAddress(),
			Gas: common.Gas{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
			},
		},
		1024,
		vault.PubKey, 1024,
	)
	txOutStore := w.mgr.TxOutStore()
	c.Assert(refundTx(w.ctx, txin, w.mgr, 0, "refund", ""), IsNil)
	items, err := txOutStore.GetOutboundItems(w.ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 1)

	// check THORNode DONT create a refund transaction when THORNode don't have a pool for
	// the asset sent.
	lokiAsset, _ := common.NewAsset("DOGE.LOKI")
	txin.Tx.Coins = common.Coins{
		common.NewCoin(lokiAsset, cosmos.NewUint(100*common.One)),
	}

	c.Assert(refundTx(w.ctx, txin, w.mgr, 0, "refund", ""), IsNil)
	items, err = txOutStore.GetOutboundItems(w.ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 1)

	pool, err = w.keeper.GetPool(w.ctx, lokiAsset)
	c.Assert(err, IsNil)
	// pool should be zero since we drop coins we don't recognize on the floor
	c.Assert(pool.BalanceAsset.Equal(cosmos.ZeroUint()), Equals, true, Commentf("%d", pool.BalanceAsset.Uint64()))

	// doing it a second time should keep it at zero
	c.Assert(refundTx(w.ctx, txin, w.mgr, 0, "refund", ""), IsNil)
	items, err = txOutStore.GetOutboundItems(w.ctx)
	c.Assert(err, IsNil)
	c.Assert(items, HasLen, 1)
	pool, err = w.keeper.GetPool(w.ctx, lokiAsset)
	c.Assert(err, IsNil)
	c.Assert(pool.BalanceAsset.Equal(cosmos.ZeroUint()), Equals, true)
}

func (HandlerSuite) TestGetMsgSwapFromMemo(c *C) {
	ctx, mgr := setupManagerForTest(c)
	m, err := ParseMemo(GetCurrentVersion(), "swap:DOGE.DOGE")
	swapMemo, ok := m.(SwapMemo)
	c.Assert(ok, Equals, true)
	c.Assert(err, IsNil)

	txin := common.NewObservedTx(
		common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.ETHChain,
			Coins: common.Coins{
				common.NewCoin(
					common.RuneAsset(),
					cosmos.NewUint(100*common.One),
				),
			},
			Memo:        m.String(),
			FromAddress: GetRandomDOGEAddress(),
			ToAddress:   GetRandomDOGEAddress(),
			Gas: common.Gas{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
			},
		},
		1024,
		common.EmptyPubKey, 1024,
	)

	resultMsg1, err := getMsgSwapFromMemo(ctx, mgr.Keeper(), swapMemo, txin, GetRandomBech32Addr())
	c.Assert(resultMsg1, NotNil)
	c.Assert(err, IsNil)
}

func (HandlerSuite) TestGetMsgWithdrawFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	tx := GetRandomTx()
	tx.Memo = "withdraw:BTC.BTC:10000"
	if common.RuneAsset().Equals(common.RuneNative) {
		tx.FromAddress = GetRandomTHORAddress()
	}
	obTx := NewObservedTx(tx, w.ctx.BlockHeight(), GetRandomPubKey(), w.ctx.BlockHeight())
	msg, err := processOneTxIn(w.ctx, w.keeper, obTx, w.activeNodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(msg, NotNil)
	_, isWithdraw := msg.(*MsgWithdrawLiquidity)
	c.Assert(isWithdraw, Equals, true)
}

func (HandlerSuite) TestGetMsgMigrationFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	tx := GetRandomTx()
	tx.Memo = "migrate:10"
	obTx := NewObservedTx(tx, w.ctx.BlockHeight(), GetRandomPubKey(), w.ctx.BlockHeight())
	msg, err := processOneTxIn(w.ctx, w.keeper, obTx, w.activeNodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(msg, NotNil)
	_, isMigrate := msg.(*MsgMigrate)
	c.Assert(isMigrate, Equals, true)
}

func (HandlerSuite) TestGetMsgBondFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	tx := GetRandomTx()
	tx.Coins = common.Coins{
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100*common.One)),
	}
	tx.Memo = "bond:" + GetRandomBech32Addr().String()
	obTx := NewObservedTx(tx, w.ctx.BlockHeight(), GetRandomPubKey(), w.ctx.BlockHeight())
	msg, err := processOneTxIn(w.ctx, w.keeper, obTx, w.activeNodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(msg, NotNil)
	_, isBond := msg.(*MsgBond)
	c.Assert(isBond, Equals, true)
}

func (HandlerSuite) TestGetMsgUnBondFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	tx := GetRandomTx()
	tx.Coins = common.Coins{
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100*common.One)),
	}
	tx.Memo = "unbond:" + GetRandomTHORAddress().String() + ":1000"
	obTx := NewObservedTx(tx, w.ctx.BlockHeight(), GetRandomPubKey(), w.ctx.BlockHeight())
	msg, err := processOneTxIn(w.ctx, w.keeper, obTx, w.activeNodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(msg, NotNil)
	_, isUnBond := msg.(*MsgUnBond)
	c.Assert(isUnBond, Equals, true)
}

func (HandlerSuite) TestGetMsgReBondFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	tx := GetRandomTx()
	tx.Coins = common.Coins{
		common.NewCoin(common.RuneAsset(), cosmos.NewUint(100*common.One)),
	}
	for _, template := range []string{
		"rebond:%s:%s:123456789",
		"rebond:%s:%s:0",
		"rebond:%s:%s",
	} {
		tx.Memo = fmt.Sprintf(
			template,
			GetRandomTHORAddress().String(),
			GetRandomTHORAddress().String(),
		)
		obTx := NewObservedTx(tx, w.ctx.BlockHeight(), GetRandomPubKey(), w.ctx.BlockHeight())
		msg, err := processOneTxIn(w.ctx, w.keeper, obTx, w.activeNodeAccount.NodeAddress)
		c.Assert(err, IsNil)
		c.Assert(msg, NotNil)
		_, isReBond := msg.(*MsgReBond)
		c.Assert(isReBond, Equals, true)
	}
}

func (HandlerSuite) TestGetMsgLiquidityFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	// provide DOGE, however THORNode send T-CAN as coin , which is incorrect, should result in an error
	m, err := ParseMemo(GetCurrentVersion(), fmt.Sprintf("add:DOGE.DOGE:%s", GetRandomRUNEAddress()))
	c.Assert(err, IsNil)
	lpMemo, ok := m.(AddLiquidityMemo)
	c.Assert(ok, Equals, true)
	tcanAsset, err := common.NewAsset("DOGE.TCAN-014")
	c.Assert(err, IsNil)
	runeAsset := common.RuneAsset()
	c.Assert(err, IsNil)

	txin := common.NewObservedTx(
		common.Tx{
			ID:    GetRandomTxHash(),
			Chain: common.ETHChain,
			Coins: common.Coins{
				common.NewCoin(tcanAsset,
					cosmos.NewUint(100*common.One)),
				common.NewCoin(runeAsset,
					cosmos.NewUint(100*common.One)),
			},
			Memo:        m.String(),
			FromAddress: GetRandomDOGEAddress(),
			ToAddress:   GetRandomDOGEAddress(),
			Gas: common.Gas{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
			},
		},
		1024,
		common.EmptyPubKey, 1024,
	)

	msg, err := getMsgAddLiquidityFromMemo(w.ctx, lpMemo, txin, GetRandomBech32Addr())
	c.Assert(msg, NotNil)
	c.Assert(err, IsNil)

	// Asymentic liquidity provision should works fine, only RUNE
	txin.Tx.Coins = common.Coins{
		common.NewCoin(runeAsset,
			cosmos.NewUint(100*common.One)),
	}

	// provide only rune should be fine
	msg1, err1 := getMsgAddLiquidityFromMemo(w.ctx, lpMemo, txin, GetRandomBech32Addr())
	c.Assert(msg1, NotNil)
	c.Assert(err1, IsNil)

	dogeAsset, err := common.NewAsset("DOGE.DOGE")
	c.Assert(err, IsNil)
	txin.Tx.Coins = common.Coins{
		common.NewCoin(dogeAsset,
			cosmos.NewUint(100*common.One)),
	}

	// provide only token(DOGE) should be fine
	msg2, err2 := getMsgAddLiquidityFromMemo(w.ctx, lpMemo, txin, GetRandomBech32Addr())
	c.Assert(msg2, NotNil)
	c.Assert(err2, IsNil)

	lokiAsset, _ := common.NewAsset("DOGE.LOKI")
	// Make sure the RUNE Address and Asset Address set correctly
	txin.Tx.Coins = common.Coins{
		common.NewCoin(runeAsset,
			cosmos.NewUint(100*common.One)),
		common.NewCoin(lokiAsset,
			cosmos.NewUint(100*common.One)),
	}

	runeAddr := GetRandomRUNEAddress()
	lokiAddLiquidityMemo, err := ParseMemo(GetCurrentVersion(), fmt.Sprintf("add:DOGE.LOKI:%s", runeAddr))
	c.Assert(err, IsNil)
	msg4, err4 := getMsgAddLiquidityFromMemo(w.ctx, lokiAddLiquidityMemo.(AddLiquidityMemo), txin, GetRandomBech32Addr())
	c.Assert(err4, IsNil)
	c.Assert(msg4, NotNil)
	msgAddLiquidity, ok := msg4.(*MsgAddLiquidity)
	c.Assert(ok, Equals, true)
	c.Assert(msgAddLiquidity, NotNil)
	c.Assert(msgAddLiquidity.RuneAddress, Equals, runeAddr)
	c.Assert(msgAddLiquidity.AssetAddress, Equals, txin.Tx.FromAddress)
}

func (HandlerSuite) TestMsgLeaveFromMemo(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	addr := types.GetRandomBech32Addr()
	txin := common.NewObservedTx(
		common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			Coins:       common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(1))},
			Memo:        fmt.Sprintf("LEAVE:%s", addr.String()),
			FromAddress: GetRandomDOGEAddress(),
			ToAddress:   GetRandomDOGEAddress(),
			Gas: common.Gas{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
			},
		},
		1024,
		common.EmptyPubKey, 1024,
	)

	msg, err := processOneTxIn(w.ctx, w.keeper, txin, addr)
	c.Assert(err, IsNil)
	msgV, ok := msg.(sdk.HasValidateBasic)
	c.Assert(ok, Equals, true)
	c.Check(msgV.ValidateBasic(), IsNil)
}

func (s *HandlerSuite) TestReserveContributor(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	addr := types.GetRandomBech32Addr()
	txin := common.NewObservedTx(
		common.Tx{
			ID:          GetRandomTxHash(),
			Chain:       common.ETHChain,
			Coins:       common.Coins{common.NewCoin(common.RuneAsset(), cosmos.NewUint(1))},
			Memo:        "reserve",
			FromAddress: GetRandomDOGEAddress(),
			ToAddress:   GetRandomDOGEAddress(),
			Gas: common.Gas{
				common.NewCoin(common.DOGEAsset, cosmos.NewUint(10000)),
			},
		},
		1024,
		GetRandomPubKey(), 1024,
	)

	msg, err := processOneTxIn(w.ctx, w.keeper, txin, addr)
	c.Assert(err, IsNil)
	msgV, ok := msg.(sdk.HasValidateBasic)
	c.Assert(ok, Equals, true)
	c.Check(msgV.ValidateBasic(), IsNil)
	_, isReserve := msg.(*MsgReserveContributor)
	c.Assert(isReserve, Equals, true)
}

func (s *HandlerSuite) TestMsgServer(c *C) {
	ctx, mgr := setupManagerForTest(c)
	newMsgServer := NewMsgServerImpl(mgr)
	ctx = ctx.WithBlockHeight(1024)
	msg := NewMsgNetworkFee(1024, common.ETHChain, 1, 10000, GetRandomBech32Addr())
	result, err := newMsgServer.NetworkFee(ctx, msg)
	c.Check(err, NotNil)
	c.Check(errors.Is(err, se.ErrUnauthorized), Equals, true)
	c.Check(result, IsNil)
	na := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, na), IsNil)
	FundModule(c, ctx, mgr.Keeper(), BondName, 10*common.One)
	result, err = newMsgServer.SetVersion(ctx, NewMsgSetVersion("0.1.0", na.NodeAddress))
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (s *HandlerSuite) TestFuzzyMatching(c *C) {
	ctx, mgr := setupManagerForTest(c)
	k := mgr.Keeper()
	p1 := NewPool()
	p1.Asset = common.DOGEAsset
	p1.BalanceRune = cosmos.NewUint(10 * common.One)
	c.Assert(k.SetPool(ctx, p1), IsNil)

	// real USDT
	p2 := NewPool()
	p2.Asset, _ = common.NewAsset("ETH.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7")
	p2.BalanceRune = cosmos.NewUint(80 * common.One)
	c.Assert(k.SetPool(ctx, p2), IsNil)

	// fake USDT, attempt to clone end of contract address
	p3 := NewPool()
	p3.Asset, _ = common.NewAsset("ETH.USDT-0XD084B83C305DAFD76AE3E1B4E1F1FE213D831EC7")
	p3.BalanceRune = cosmos.NewUint(20 * common.One)
	c.Assert(k.SetPool(ctx, p3), IsNil)

	// fake USDT, bad contract address
	p4 := NewPool()
	p4.Asset, _ = common.NewAsset("ETH.USDT-0XD084B83C305DAFD76AE3E1B4E1F1FE2ECCCB3988")
	p4.BalanceRune = cosmos.NewUint(20 * common.One)
	c.Assert(k.SetPool(ctx, p4), IsNil)

	// fake USDT, on different chain
	p5 := NewPool()
	p5.Asset, _ = common.NewAsset("BSC.USDT-0XDAC17F958D2EE523A2206206994597C13D831EC7")
	p5.BalanceRune = cosmos.NewUint(30 * common.One)
	c.Assert(k.SetPool(ctx, p5), IsNil)

	// fake USDT, right contract address, wrong ticker
	p6 := NewPool()
	p6.Asset, _ = common.NewAsset("ETH.UST-0XDAC17F958D2EE523A2206206994597C13D831EC7")
	p6.BalanceRune = cosmos.NewUint(90 * common.One)
	c.Assert(k.SetPool(ctx, p6), IsNil)

	result := fuzzyAssetMatch(ctx, k, p1.Asset)
	c.Check(result.Equals(p1.Asset), Equals, true)
	result = fuzzyAssetMatch(ctx, k, p6.Asset)
	c.Check(result.Equals(p6.Asset), Equals, true)

	check, _ := common.NewAsset("ETH.USDT")
	result = fuzzyAssetMatch(ctx, k, check)
	c.Check(result.Equals(p2.Asset), Equals, true)
	check, _ = common.NewAsset("ETH.USDT-")
	result = fuzzyAssetMatch(ctx, k, check)
	c.Check(result.Equals(p2.Asset), Equals, true)
	check, _ = common.NewAsset("ETH.USDT-1EC7")
	result = fuzzyAssetMatch(ctx, k, check)
	c.Check(result.Equals(p2.Asset), Equals, true)

	check, _ = common.NewAsset("ETH/USDT-1EC7")
	result = fuzzyAssetMatch(ctx, k, check)
	c.Check(result.Synth, Equals, true)
	c.Check(result.Equals(p2.Asset.GetSyntheticAsset()), Equals, true)

	check, _ = common.NewAsset("ETH~USDT")
	result = fuzzyAssetMatch(ctx, k, check)
	c.Check(result.Synth, Equals, false)
	c.Check(result.Trade, Equals, true)
	c.Check(result.Equals(p2.Asset.GetTradeAsset()), Equals, true)
}

func (s *HandlerSuite) TestMemoFetchAddress(c *C) {
	ctx, k := setupKeeperForTest(c)

	thorAddr := GetRandomTHORAddress()
	name := NewTHORName("hello", 50, []THORNameAlias{{Chain: common.THORChain, Address: thorAddr}})
	k.SetTHORName(ctx, name)

	dogeAddr := GetRandomDOGEAddress()
	addr, err := FetchAddress(ctx, k, dogeAddr.String(), common.ETHChain)
	c.Assert(err, IsNil)
	c.Check(addr.Equals(dogeAddr), Equals, true)

	addr, err = FetchAddress(ctx, k, "hello", common.THORChain)
	c.Assert(err, IsNil)
	c.Check(addr.Equals(thorAddr), Equals, true)

	addr, err = FetchAddress(ctx, k, "hello.thor", common.THORChain)
	c.Assert(err, IsNil)
	c.Check(addr.Equals(thorAddr), Equals, true)
}

func (s *HandlerSuite) TestExternalAssetMatch(c *C) {
	c.Check(externalAssetMatch(common.ETHChain, "7a0"), Equals, "0xd601c6A3a36721320573885A8d8420746dA3d7A0")
	c.Check(externalAssetMatch(common.ETHChain, "foobar"), Equals, "foobar")
	c.Check(externalAssetMatch(common.ETHChain, "3"), Equals, "3")
	c.Check(externalAssetMatch(common.ETHChain, ""), Equals, "")
	c.Check(externalAssetMatch(common.BTCChain, "foo"), Equals, "foo")
}
