package keeperv1

import (
	"testing"

	. "gopkg.in/check.v1"

	cstore "cosmossdk.io/core/store"
	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"

	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestPackage(t *testing.T) { TestingT(t) }

func FundModule(c *C, ctx cosmos.Context, k KVStore, name string, amt uint64) {
	coin := common.NewCoin(common.BTCAsset, cosmos.NewUint(amt))
	err := k.MintToModule(ctx, ModuleName, coin)
	c.Assert(err, IsNil)
	err = k.SendFromModuleToModule(ctx, ModuleName, name, common.NewCoins(coin))
	c.Assert(err, IsNil)
}

var serviceThornado cstore.KVStoreService

func setupKeeperForTest(c *C) (cosmos.Context, KVStore) {
	SetupConfigForTest()
	keys := cosmos.NewKVStoreKeys(
		authtypes.StoreKey, banktypes.StoreKey, stakingtypes.StoreKey, upgradetypes.StoreKey, StoreKey,
	)

	serviceThornado = runtime.NewKVStoreService(keys[StoreKey])

	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(keys[authtypes.StoreKey], cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keys[banktypes.StoreKey], cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keys[upgradetypes.StoreKey], cosmos.StoreTypeIAVL, db)
	ms.MountStoreWithDB(keys[StoreKey], cosmos.StoreTypeIAVL, db)
	err := ms.LoadLatestVersion()
	c.Assert(err, IsNil)

	ctx := cosmos.NewContext(ms, tmproto.Header{ChainID: "thornado"}, false, log.NewNopLogger())
	ctx = ctx.WithBlockHeight(18)

	encodingConfig := testutil.MakeTestEncodingConfig(
		bank.AppModuleBasic{},
		auth.AppModuleBasic{},
	)

	maccPerms := map[string][]string{
		authtypes.FeeCollectorName:     nil,
		distrtypes.ModuleName:          nil,
		minttypes.ModuleName:           {authtypes.Minter},
		stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
		ModuleName:                     {authtypes.Minter, authtypes.Burner},
		ReserveName:                    {},
		BaseName:                       {},
		BondName:                       {authtypes.Staking},
	}
	ak := authkeeper.NewAccountKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix()),
		sdk.GetConfig().GetBech32AccountAddrPrefix(),
		authtypes.NewModuleAddress(ModuleName).String(),
	)

	bk := bankkeeper.NewBaseKeeper(
		encodingConfig.Codec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		ak,
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
		log.NewNopLogger(),
	)

	uk := upgradekeeper.NewKeeper(
		nil,
		runtime.NewKVStoreService(keys[upgradetypes.StoreKey]),
		encodingConfig.Codec,
		c.MkDir(),
		nil,
		authtypes.NewModuleAddress(ModuleName).String(),
	)

	k := NewKVStore(encodingConfig.Codec, serviceThornado, bk, ak, uk, GetCurrentVersion())

	FundModule(c, ctx, k, BaseName, 100_000_000*common.One)

	return ctx, k
}

type KeeperTestSuit struct{}

var _ = Suite(&KeeperTestSuit{})

func (KeeperTestSuit) TestKeeperVersion(c *C) {
	ctx, k := setupKeeperForTest(c)

	coinsToSend := common.NewCoins(common.NewCoin(common.BTCAsset, cosmos.NewUint(1*common.One)))
	c.Check(k.SendFromModuleToModule(ctx, BaseName, BondName, coinsToSend), IsNil)

	acct := GetRandomBech32Addr()
	c.Check(k.SendFromModuleToAccount(ctx, BaseName, acct, coinsToSend), IsNil)

	// check get account balance
	coins := k.GetBalance(ctx, acct)
	c.Check(coins, HasLen, 1)

	c.Check(k.SendFromAccountToModule(ctx, acct, BaseName, coinsToSend), IsNil)

	// check no account balance
	coins = k.GetBalance(ctx, GetRandomBech32Addr())
	c.Check(coins, HasLen, 0)
}
