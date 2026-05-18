package thorchain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tmtypes "github.com/cometbft/cometbft/types"
	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerDepositSuite struct{}

var _ = Suite(&HandlerDepositSuite{})

func (s *HandlerDepositSuite) TestValidate(c *C) {
	ctx, k := setupKeeperForTest(c)

	addr := GetRandomBech32Addr()

	coins := common.Coins{
		common.NewCoin(common.RuneNative, cosmos.NewUint(200*common.One)),
	}
	msg := NewMsgDeposit(coins, fmt.Sprintf("ADD:DOGE.DOGE:%s", GetRandomRUNEAddress()), addr)

	handler := NewDepositHandler(NewDummyMgrWithKeeper(k))
	err := handler.validate(ctx, *msg)
	c.Assert(err, IsNil)

	// invalid msg
	msg = &MsgDeposit{}
	err = handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerDepositSuite) TestHandle(c *C) {
	ctx, k := setupKeeperForTest(c)
	activeNode := GetRandomValidatorNode(NodeActive)
	c.Assert(k.SetNodeAccount(ctx, activeNode), IsNil)
	dummyMgr := NewDummyMgrWithKeeper(k)
	handler := NewDepositHandler(dummyMgr)

	addr := GetRandomBech32Addr()

	coins := common.Coins{
		common.NewCoin(common.RuneNative, cosmos.NewUint(200*common.One)),
	}

	FundAccount(c, ctx, k, addr, 300*common.One)
	pool := NewPool()
	pool.Asset = common.DOGEAsset
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, pool), IsNil)
	msg := NewMsgDeposit(coins, "ADD:DOGE.DOGE", addr)

	_, err := handler.handle(ctx, *msg, 0)
	c.Assert(err, IsNil)
	// ensure observe tx had been saved
	hash := tmtypes.Tx(ctx.TxBytes()).Hash()
	txID, err := common.NewTxID(fmt.Sprintf("%X", hash))
	c.Assert(err, IsNil)
	voter, err := k.GetObservedTxInVoter(ctx, txID)
	c.Assert(err, IsNil)
	c.Assert(voter.Tx.IsEmpty(), Equals, false)
	c.Assert(voter.Tx.Status, Equals, common.Status_done)

	FundAccount(c, ctx, k, addr, 300*common.One)
	// do it again with same tx bytes - should auto-increment and succeed
	_, err = handler.handle(ctx, *msg, 0)
	c.Assert(err, IsNil)
	// verify the auto-incremented txID was used
	txIDIncremented, err := common.NewTxID(fmt.Sprintf("%X-1", hash))
	c.Assert(err, IsNil)
	voter2, err := k.GetObservedTxInVoter(ctx, txIDIncremented)
	c.Assert(err, IsNil)
	c.Assert(voter2.Tx.IsEmpty(), Equals, false)
	c.Assert(voter2.Tx.Status, Equals, common.Status_done)

	// when tx bytes are empty and salt is provided, generate tx id with msg bytes + block height
	FundAccount(c, ctx, k, addr, 300*common.One)
	ctxNoTxBytes := ctx.WithTxBytes(nil)
	saltedMsg := NewMsgDeposit(coins, "ADD:DOGE.DOGE", addr)
	saltedMsg.Salt = []byte("deposit-salt")
	_, err = handler.handle(ctxNoTxBytes, *saltedMsg, 0)
	c.Assert(err, IsNil)
	hashBytes, _ := saltedMsg.Marshal()
	h := make([]byte, 8)
	binary.BigEndian.PutUint64(h, uint64(ctx.BlockHeight()))
	hashBytes = append(hashBytes, h...)
	saltHash := tmtypes.Tx(hashBytes).Hash()
	saltTxID, err := common.NewTxID(fmt.Sprintf("%X", saltHash))
	c.Assert(err, IsNil)
	saltedVoter, err := k.GetObservedTxInVoter(ctx, saltTxID)
	c.Assert(err, IsNil)
	c.Assert(saltedVoter.Tx.IsEmpty(), Equals, false)
	c.Assert(saltedVoter.Tx.Status, Equals, common.Status_done)

	// reusing the same salt should trigger suffix counter collision handling
	FundAccount(c, ctx, k, addr, 300*common.One)
	_, err = handler.handle(ctxNoTxBytes, *saltedMsg, 0)
	c.Assert(err, IsNil)
	saltTxIDIncremented, err := common.NewTxID(fmt.Sprintf("%X-1", saltHash))
	c.Assert(err, IsNil)
	saltedVoter2, err := k.GetObservedTxInVoter(ctx, saltTxIDIncremented)
	c.Assert(err, IsNil)
	c.Assert(saltedVoter2.Tx.IsEmpty(), Equals, false)
	c.Assert(saltedVoter2.Tx.Status, Equals, common.Status_done)
}

type HandlerDepositTestHelper struct {
	keeper.Keeper
}

func NewHandlerDepositTestHelper(k keeper.Keeper) *HandlerDepositTestHelper {
	return &HandlerDepositTestHelper{
		Keeper: k,
	}
}

func (s *HandlerDepositSuite) TestDifferentValidation(c *C) {
	acctAddr := GetRandomBech32Addr()
	badAsset := common.Asset{
		Chain:  common.THORChain,
		Symbol: "ETH~ETH",
	}
	testCases := []struct {
		name            string
		messageProvider func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg
		validator       func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string)
	}{
		{
			name: "invalid message should result an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg {
				return NewMsgNetworkFee(ctx.BlockHeight(), common.DOGEChain, 1, 10000, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
				c.Check(errors.Is(err, errInvalidMessage), Equals, true, Commentf(name))
			},
		},
		{
			name: "coin is not on THORChain should result in an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg {
				return NewMsgDeposit(common.Coins{
					common.NewCoin(common.DOGEAsset, cosmos.NewUint(100)),
				}, "hello", GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "invalid coin should result in error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg {
				return NewMsgDeposit(common.Coins{
					common.NewCoin(badAsset, cosmos.NewUint(100)),
				}, "hello", GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(strings.Contains(err.Error(), "invalid coin"), Equals, true, Commentf(name))
			},
		},
		{
			name: "Insufficient funds should result in an error",
			messageProvider: func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg {
				return NewMsgDeposit(common.Coins{
					common.NewCoin(common.RuneNative, cosmos.NewUint(100)),
				}, "hello", GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, Equals, se.ErrInsufficientFunds, Commentf(name))
			},
		},
		{
			name: "invalid memo should err",
			messageProvider: func(c *C, ctx cosmos.Context, helper *HandlerDepositTestHelper) cosmos.Msg {
				FundAccount(c, ctx, helper.Keeper, acctAddr, 100*common.One)
				vault := NewVault(ctx.BlockHeight(), ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.DOGEChain, common.THORChain}.Strings(), []ChainContract{})
				c.Check(helper.Keeper.SetVault(ctx, vault), IsNil)
				return NewMsgDeposit(common.Coins{
					common.NewCoin(common.RuneNative, cosmos.NewUint(2*common.One)),
				}, "hello", acctAddr)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *HandlerDepositTestHelper, name string) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
				c.Check(strings.Contains(err.Error(), "invalid tx type: hello"), Equals, true)
			},
		},
	}
	for _, tc := range testCases {
		ctx, mgr := setupManagerForTest(c)
		helper := NewHandlerDepositTestHelper(mgr.Keeper())
		mgr.K = helper
		handler := NewDepositHandler(mgr)
		msg := tc.messageProvider(c, ctx, helper)
		result, err := handler.Run(ctx, msg)
		tc.validator(c, ctx, result, err, helper, tc.name)
	}
}

func (s *HandlerDepositSuite) TestAddSwap(c *C) {
	SetupConfigForTest()
	ctx, mgr := setupManagerForTest(c)
	affAddr := GetRandomTHORAddress()
	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomTHORAddress(),
		GetRandomTHORAddress(),
		common.Coins{common.NewCoin(common.RuneNative, cosmos.NewUint(common.One))},
		common.Gas{
			{Asset: common.DOGEAsset, Amount: cosmos.NewUint(37500)},
		},
		fmt.Sprintf("=:BTC.BTC:%s", GetRandomBTCAddress().String()),
	)
	// no affiliate fee
	msg := NewMsgSwap(tx, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), common.NoAddress, cosmos.ZeroUint(), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())

	c.Assert(addSwap(ctx, mgr, *msg), IsNil)
	swap, err := getSwapQueueItem(ctx, mgr, tx.ID, 0)
	c.Assert(err, IsNil)
	// Compare core fields instead of string representation since advanced queue adds state
	c.Assert(swap.Tx.ID, Equals, msg.Tx.ID)
	c.Assert(swap.TargetAsset, Equals, msg.TargetAsset)
	c.Assert(swap.Destination, Equals, msg.Destination)

	tx.Memo = fmt.Sprintf("=:BTC.BTC:%s::%s:20000", GetRandomBTCAddress().String(), affAddr.String())

	// affiliate fee, with more than 10K as basis points
	msg1 := NewMsgSwap(tx, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), GetRandomTHORAddress(), cosmos.NewUint(20000), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())

	// Check balance before swap
	affiliateFeeAddr, err := msg1.GetAffiliateAddress().AccAddress()
	c.Assert(err, IsNil)
	acct := mgr.Keeper().GetBalance(ctx, affiliateFeeAddr)
	c.Assert(acct.AmountOf(common.RuneNative.Native()).String(), Equals, "0")

	c.Assert(addSwap(ctx, mgr, *msg1), IsNil)
	swap, err = getSwapQueueItem(ctx, mgr, tx.ID, 0)
	c.Assert(err, IsNil)
	c.Assert(swap.Tx.Coins[0].Amount.IsZero(), Equals, false)
	// Check balance after swap, should be the same
	c.Assert(acct.AmountOf(common.RuneNative.Native()).String(), Equals, "0")

	// affiliate fee not taken on deposit
	tx.Memo = fmt.Sprintf("=:BTC.BTC:%s::%s:1000", GetRandomBTCAddress().String(), affAddr.String())
	tx.Coins[0].Amount = cosmos.NewUint(common.One)
	msg2 := NewMsgSwap(tx, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), affAddr, cosmos.NewUint(1000), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(addSwap(ctx, mgr, *msg2), IsNil)
	swap, err = getSwapQueueItem(ctx, mgr, tx.ID, 0)
	c.Assert(err, IsNil)
	c.Assert(swap.Tx.Coins[0].Amount.IsZero(), Equals, false)
	c.Assert(swap.Tx.Coins[0].Amount.String(), Equals, cosmos.NewUint(common.One).String())

	affiliateFeeAddr2, err := msg2.GetAffiliateAddress().AccAddress()
	c.Assert(err, IsNil)
	acct2 := mgr.Keeper().GetBalance(ctx, affiliateFeeAddr2)
	c.Assert(acct2.AmountOf(common.RuneNative.Native()).String(), Equals, strconv.FormatInt(0, 10))

	// NONE RUNE , synth asset should be handled correctly

	synthAsset, err := common.NewAsset("BTC/BTC")
	c.Assert(err, IsNil)
	tx1 := common.NewTx(
		GetRandomTxHash(),
		GetRandomTHORAddress(),
		GetRandomTHORAddress(),
		common.Coins{common.NewCoin(synthAsset, cosmos.NewUint(common.One))},
		common.Gas{
			{Asset: common.RuneNative, Amount: cosmos.NewUint(200000)},
		},
		tx.Memo,
	)

	c.Assert(mgr.Keeper().MintToModule(ctx, ModuleName, tx1.Coins[0]), IsNil)
	c.Assert(mgr.Keeper().SendFromModuleToModule(ctx, ModuleName, AsgardName, tx1.Coins), IsNil)
	msg3 := NewMsgSwap(tx1, common.BTCAsset, GetRandomBTCAddress(), cosmos.ZeroUint(), affAddr, cosmos.NewUint(1000), "", "", nil, types.SwapType_market, 0, 0, types.SwapVersion_v1, GetRandomBech32Addr())
	c.Assert(addSwap(ctx, mgr, *msg3), IsNil)
	swap, err = getSwapQueueItem(ctx, mgr, tx1.ID, 0)
	c.Assert(err, IsNil)
	c.Assert(swap.Tx.Coins[0].Amount.IsZero(), Equals, false)
	c.Assert(swap.Tx.Coins[0].Amount.String(), Equals, cosmos.NewUint(common.One).String())

	// affiliate fee not taken on deposit
	affiliateFeeAddr3, err := msg3.GetAffiliateAddress().AccAddress()
	c.Assert(err, IsNil)
	acct3 := mgr.Keeper().GetBalance(ctx, affiliateFeeAddr3)
	c.Assert(acct3.AmountOf(common.RuneNative.Native()).String(), Equals, strconv.FormatInt(0, 10))
}

func (s *HandlerDepositSuite) TestTargetModule(c *C) {
	acctAddr := GetRandomBech32Addr()
	testCases := []struct {
		name            string
		moduleName      string
		messageProvider func(c *C, ctx cosmos.Context) *MsgDeposit
		validator       func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, name string, balDelta cosmos.Uint)
	}{
		{
			name:       "thorname coins should go to reserve",
			moduleName: ReserveName,
			messageProvider: func(c *C, ctx cosmos.Context) *MsgDeposit {
				addr := GetRandomRUNEAddress()
				coin := common.NewCoin(common.RuneAsset(), cosmos.NewUint(20_00000000))
				return NewMsgDeposit(common.Coins{coin}, "name:test:THOR:"+addr.String(), acctAddr)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, name string, balDelta cosmos.Uint) {
				c.Check(err, IsNil, Commentf(name))
				c.Assert(cosmos.NewUint(20_00000000).String(), Equals, balDelta.String(), Commentf(name))
			},
		},
	}
	for _, tc := range testCases {
		ctx, mgr := setupManagerForTest(c)
		handler := NewDepositHandler(mgr)
		msg := tc.messageProvider(c, ctx)
		totalCoins := common.NewCoins(msg.Coins[0])
		c.Assert(mgr.Keeper().MintToModule(ctx, ModuleName, totalCoins[0]), IsNil)
		c.Assert(mgr.Keeper().SendFromModuleToAccount(ctx, ModuleName, msg.Signer, totalCoins), IsNil)
		balBefore := mgr.Keeper().GetRuneBalanceOfModule(ctx, tc.moduleName)
		result, err := handler.Run(ctx, msg)
		balAfter := mgr.Keeper().GetRuneBalanceOfModule(ctx, tc.moduleName)
		balDelta := balAfter.Sub(balBefore)
		tc.validator(c, ctx, result, err, tc.name, balDelta)
	}
}

// Helper function to get swap queue item from either regular or advanced queue
// depending on the EnableAdvSwapQueue constant
func getSwapQueueItem(ctx cosmos.Context, mgr Manager, txID common.TxID, index int) (MsgSwap, error) {
	// First try the regular queue
	swap, err := mgr.Keeper().GetSwapQueueItem(ctx, txID, index)
	if err == nil {
		return swap, nil
	}

	// If not found in regular queue, try advanced queue
	return mgr.Keeper().GetAdvSwapQueueItem(ctx, txID, index)
}

func (s *HandlerDepositSuite) TestMemolessNativeDeposit(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(100)

	mgr := NewDummyMgrWithKeeper(k)
	handler := NewDepositHandler(mgr)

	// Set up pool for BTC
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, pool), IsNil)

	addr := GetRandomBech32Addr()
	FundAccount(c, ctx, k, addr, 1000*common.One)

	// Register a reference memo for RUNE swaps
	btcAddr := GetRandomBTCAddress()
	refMemo := NewReferenceMemo(common.RuneNative, fmt.Sprintf("=:BTC.BTC:%s", btcAddr), "00001", 90)
	refMemo.RegisteredBy = addr
	refMemo.RegistrationHash, _ = common.NewTxID("ABC123")
	k.SetReferenceMemo(ctx, refMemo)

	// Test 1: Memoless deposit with amount encoding reference 00001
	// Amount ends in 00001 (e.g., 100000001) to encode reference
	amount := cosmos.NewUint(100000001) // Last 5 digits: 00001
	coins := common.Coins{common.NewCoin(common.RuneNative, amount)}

	msg := NewMsgDeposit(coins, "", addr) // Empty memo
	c.Assert(msg, NotNil)

	result, err := handler.handle(ctx, *msg, 0)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// The handler should have generated "r:00001" and resolved it to the swap memo
	// Verify via the swap queue (swap should be queued)
	// Note: This is a basic test; full integration would verify the swap executes correctly
}

func (s *HandlerDepositSuite) TestExtractReferenceFromNativeAmount(c *C) {
	ctx, k := setupKeeperForTest(c)
	mgr := NewDummyMgrWithKeeper(k)
	handler := NewDepositHandler(mgr)

	// Test 1: Extract reference from amount
	ref, err := handler.extractReferenceFromNativeAmount(ctx, 100000042)
	c.Assert(err, IsNil)
	c.Assert(ref, Equals, "00042")

	// Test 2: Extract reference with different value
	ref, err = handler.extractReferenceFromNativeAmount(ctx, 123456789)
	c.Assert(err, IsNil)
	c.Assert(ref, Equals, "56789")

	// Test 3: Zero amount should error
	_, err = handler.extractReferenceFromNativeAmount(ctx, 0)
	c.Assert(err, NotNil)

	// Test 4: Amount divisible by modulus should error (zero reference)
	// With modulus 100000, an amount like 500000000 would give reference 00000
	_, err = handler.extractReferenceFromNativeAmount(ctx, 500000000)
	c.Assert(err, NotNil)
	c.Assert(strings.Contains(err.Error(), "zero reference"), Equals, true)
}

func (s *HandlerDepositSuite) TestMemolessNativeDepositReferenceResolution(c *C) {
	ctx, k := setupKeeperForTest(c)
	ctx = ctx.WithBlockHeight(100)

	mgr := NewDummyMgrWithKeeper(k)
	handler := NewDepositHandler(mgr)

	// Set up pool for BTC
	pool := NewPool()
	pool.Asset = common.BTCAsset
	pool.BalanceRune = cosmos.NewUint(100 * common.One)
	pool.BalanceAsset = cosmos.NewUint(100 * common.One)
	pool.Status = PoolAvailable
	c.Assert(k.SetPool(ctx, pool), IsNil)

	addr := GetRandomBech32Addr()
	FundAccount(c, ctx, k, addr, 1000*common.One)

	// Register a reference memo with explicit reference "r:00123"
	btcAddr := GetRandomBTCAddress()
	refMemo := NewReferenceMemo(common.RuneNative, fmt.Sprintf("=:BTC.BTC:%s", btcAddr), "00123", 90)
	refMemo.RegisteredBy = addr
	refMemo.RegistrationHash, _ = common.NewTxID("DEF456")
	k.SetReferenceMemo(ctx, refMemo)

	// Test: Deposit with explicit reference memo "r:00123"
	amount := cosmos.NewUint(50 * common.One)
	coins := common.Coins{common.NewCoin(common.RuneNative, amount)}

	msg := NewMsgDeposit(coins, "r:00123", addr) // Explicit reference memo
	c.Assert(msg, NotNil)

	result, err := handler.handle(ctx, *msg, 0)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// The handler should have resolved "r:00123" to "=:BTC.BTC:bc1qyyy"
	// and queued a swap
}
