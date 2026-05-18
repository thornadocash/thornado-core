package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/suite"
	"gitlab.com/thorchain/thornode/v3/x/denom/keeper"
	"gitlab.com/thorchain/thornode/v3/x/denom/types"
)

var (
	authAcc  = sdk.AccAddress([]byte("auth1_______________"))
	accAddrs = []sdk.AccAddress{
		sdk.AccAddress([]byte("addr1_______________")),
		sdk.AccAddress([]byte("addr2_______________")),
	}
)

type KeeperTestSuite struct {
	suite.Suite

	ctx        context.Context
	bankKeeper bankkeeper.Keeper
	keeper     keeper.Keeper

	queryClient types.QueryClient
	msgServer   types.MsgServer

	encCfg moduletestutil.TestEncodingConfig
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (suite *KeeperTestSuite) SetupTest() {
	keys := storetypes.NewKVStoreKeys(authtypes.StoreKey, banktypes.StoreKey, types.StoreKey)
	testCtx := testutil.DefaultContextWithKeys(
		keys,
		storetypes.NewTransientStoreKeys(),
		storetypes.NewMemoryStoreKeys(),
	)

	encCfg := moduletestutil.MakeTestEncodingConfig()
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	authKeeper := authkeeper.NewAccountKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		map[string][]string{
			types.ModuleName: {authtypes.Minter, authtypes.Burner},
		},
		authcodec.NewBech32Codec(prefix),
		prefix,
		authAcc.String(),
	)
	bankKeeper := bankkeeper.NewBaseKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		authKeeper,
		map[string]bool{},
		authAcc.String(),
		log.NewNopLogger(),
	)

	suite.bankKeeper = bankKeeper

	suite.ctx = testCtx
	suite.keeper = keeper.NewKeeper(
		encCfg.Codec,
		runtime.NewKVStoreService(keys[types.StoreKey]),
		authKeeper,
		bankKeeper,
		authAcc.String(),
	)

	authtypes.RegisterInterfaces(encCfg.InterfaceRegistry)
	types.RegisterInterfaces(encCfg.InterfaceRegistry)
	queryHelper := baseapp.NewQueryServerTestHelper(testCtx, encCfg.InterfaceRegistry)
	types.RegisterQueryServer(queryHelper, suite.keeper)

	suite.queryClient = types.NewQueryClient(queryHelper)
	suite.msgServer = keeper.NewMsgServerImpl(suite.keeper)
	suite.encCfg = encCfg
}
