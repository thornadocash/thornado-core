package thorchain

import (
	"errors"
	"strings"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerLeaveSuite struct{}

var _ = Suite(&HandlerLeaveSuite{})

func (HandlerLeaveSuite) TestLeaveHandler_NotActiveNodeLeave(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	vault := GetRandomVault()
	c.Assert(w.keeper.SetVault(w.ctx, vault), IsNil)
	leaveHandler := NewLeaveHandler(NewDummyMgrWithKeeper(w.keeper))
	acc2 := GetRandomValidatorNode(NodeStandby)
	acc2.Bond = cosmos.NewUint(100 * common.One)
	c.Assert(w.keeper.SetNodeAccount(w.ctx, acc2), IsNil)

	FundModule(c, w.ctx, w.keeper, BondName, 100*common.One)

	// The following tx is only to be used by the leave handler,
	// not the deposit handler which would have sent its Coins to BondName,
	// so it should have Amount zero to not try to send more funds than BondName has.
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		acc2.BondAddress,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.RuneAsset(), cosmos.ZeroUint())},
		common.Gas{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
		},
		"LEAVE",
	)
	msgLeave := NewMsgLeave(tx, acc2.NodeAddress, w.activeNodeAccount.NodeAddress)
	_, err := leaveHandler.Run(w.ctx, msgLeave)
	c.Assert(err, IsNil)
	accAfterLeave, err := w.keeper.GetNodeAccount(w.ctx, acc2.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(accAfterLeave.Status, Equals, NodeDisabled)
}

func (HandlerLeaveSuite) TestLeaveHandler_ActiveNodeLeave(c *C) {
	var err error
	w := getHandlerTestWrapper(c, 1, true, false)
	leaveHandler := NewLeaveHandler(NewDummyMgrWithKeeper(w.keeper))
	acc2 := GetRandomValidatorNode(NodeActive)
	acc2.Bond = cosmos.NewUint(100 * common.One)
	c.Assert(w.keeper.SetNodeAccount(w.ctx, acc2), IsNil)
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		acc2.BondAddress,
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.RuneAsset(), cosmos.ZeroUint())},
		common.Gas{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
		},
		"",
	)
	msgLeave := NewMsgLeave(tx, acc2.NodeAddress, w.activeNodeAccount.NodeAddress)
	_, err = leaveHandler.Run(w.ctx, msgLeave)
	c.Assert(err, IsNil)

	acc2, err = w.keeper.GetNodeAccount(w.ctx, acc2.NodeAddress)
	c.Assert(err, IsNil)
	c.Check(acc2.Bond.Equal(cosmos.NewUint(10000000000)), Equals, true, Commentf("Bond:%d\n", acc2.Bond.Uint64()))
}

func (HandlerLeaveSuite) TestLeaveBondProvider(c *C) {
	var err error
	w := getHandlerTestWrapper(c, 1, true, false)
	leaveHandler := NewLeaveHandler(NewDummyMgrWithKeeper(w.keeper))
	acc := GetRandomValidatorNode(NodeActive)
	acc.Bond = cosmos.NewUint(100 * common.One)
	minBond := w.keeper.GetConfigInt64(w.ctx, constants.MinimumBondInRune)

	bpBelowMin := GetRandomTHORAddress()
	bpBelowMinAccAddress, err := bpBelowMin.AccAddress()
	c.Assert(err, IsNil)
	bp1 := types.NewBondProvider(bpBelowMinAccAddress)
	// bpBelowMin bonds 0.000001 RUNE (below min bond)
	bp1.Bond = cosmos.NewUint(100)

	bpAboveMin := GetRandomTHORAddress()
	bpAboveMinAccAddress, err := bpAboveMin.AccAddress()
	c.Assert(err, IsNil)
	bp2 := types.NewBondProvider(bpAboveMinAccAddress)
	// bpAboveMin bonds exactly min bond
	bp2.Bond = cosmos.NewUint(uint64(minBond))

	bps := types.BondProviders{
		NodeAddress:     acc.NodeAddress,
		NodeOperatorFee: cosmos.NewUint(5),
		Providers:       []BondProvider{bp1, bp2},
	}

	c.Assert(w.keeper.SetBondProviders(w.ctx, bps), IsNil)
	c.Assert(w.keeper.SetNodeAccount(w.ctx, acc), IsNil)

	// try to leave with bond proivder under min bond
	txID := GetRandomTxHash()
	tx := common.NewTx(
		txID,
		bpBelowMin,
		GetRandomTHORAddress(),
		common.Coins{common.NewCoin(common.RuneAsset(), cosmos.ZeroUint())},
		common.Gas{},
		"",
	)
	msgLeave := NewMsgLeave(tx, acc.NodeAddress, bpBelowMinAccAddress)
	_, err = leaveHandler.Run(w.ctx, msgLeave)
	c.Assert(strings.Contains(err.Error(), "not authorized to manage"), Equals, true)

	// try to leave with bond proivder above min bond
	txID = GetRandomTxHash()
	tx = common.NewTx(
		txID,
		bpAboveMin,
		GetRandomTHORAddress(),
		common.Coins{common.NewCoin(common.RuneAsset(), cosmos.ZeroUint())},
		common.Gas{},
		"",
	)
	msgLeave = NewMsgLeave(tx, acc.NodeAddress, bpAboveMinAccAddress)
	_, err = leaveHandler.Run(w.ctx, msgLeave)
	c.Assert(err, IsNil)

	// acc, err = w.keeper.GetNodeAccount(w.ctx, acc.NodeAddress)
	// c.Assert(err, IsNil)
	// c.Check(acc.Bond.Equal(cosmos.NewUint(10000000001)), Equals, true, Commentf("Bond:%d\n", acc.Bond.Uint64()))
}

func (HandlerLeaveSuite) TestLeaveValidation(c *C) {
	w := getHandlerTestWrapper(c, 1, true, false)
	testCases := []struct {
		name          string
		msgLeave      *MsgLeave
		expectedError error
	}{
		{
			name: "empty from address should fail",
			msgLeave: NewMsgLeave(common.Tx{
				ID:          GetRandomTxHash(),
				Chain:       common.ETHChain,
				FromAddress: "",
				ToAddress:   GetRandomETHAddress(),
				Coins: common.Coins{
					common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
				},
				Gas: common.Gas{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One)),
				},
				Memo: "",
			}, w.activeNodeAccount.NodeAddress, w.activeNodeAccount.NodeAddress),
			expectedError: se.ErrInvalidAddress,
		},
		{
			name: "non-matching from address should fail",
			msgLeave: NewMsgLeave(common.Tx{
				ID:          GetRandomTxHash(),
				Chain:       common.ETHChain,
				FromAddress: GetRandomETHAddress(),
				ToAddress:   GetRandomETHAddress(),
				Coins: common.Coins{
					common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
				},
				Gas: common.Gas{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One)),
				},
				Memo: "",
			}, w.activeNodeAccount.NodeAddress, w.activeNodeAccount.NodeAddress),
			expectedError: se.ErrUnauthorized,
		},
		{
			name: "empty tx id should fail",
			msgLeave: NewMsgLeave(common.Tx{
				ID:          common.TxID(""),
				Chain:       common.ETHChain,
				FromAddress: w.activeNodeAccount.BondAddress,
				ToAddress:   GetRandomETHAddress(),
				Coins: common.Coins{
					common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
				},
				Gas: common.Gas{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One)),
				},
				Memo: "",
			}, w.activeNodeAccount.NodeAddress, w.activeNodeAccount.NodeAddress),
			expectedError: se.ErrUnknownRequest,
		},
		{
			name: "empty signer should fail",
			msgLeave: NewMsgLeave(common.Tx{
				ID:          GetRandomTxHash(),
				Chain:       common.ETHChain,
				FromAddress: w.activeNodeAccount.BondAddress,
				ToAddress:   GetRandomETHAddress(),
				Coins: common.Coins{
					common.NewCoin(common.ETHAsset, cosmos.ZeroUint()),
				},
				Gas: common.Gas{
					common.NewCoin(common.ETHAsset, cosmos.NewUint(common.One)),
				},
				Memo: "",
			}, w.activeNodeAccount.NodeAddress, cosmos.AccAddress{}),
			expectedError: se.ErrInvalidAddress,
		},
	}
	for _, item := range testCases {
		c.Log(item.name)
		leaveHandler := NewLeaveHandler(NewDummyMgrWithKeeper(w.keeper))
		_, err := leaveHandler.Run(w.ctx, item.msgLeave)
		c.Check(errors.Is(err, item.expectedError), Equals, true, Commentf("name:%s, %s", item.name, err))
	}
}

type LeaveHandlerTestHelper struct {
	keeper.Keeper
	failGetNodeAccount bool
	failGetVault       bool
	failSetNodeAccount bool
}

func NewLeaveHandlerTestHelper(k keeper.Keeper) *LeaveHandlerTestHelper {
	return &LeaveHandlerTestHelper{
		Keeper: k,
	}
}

func (h *LeaveHandlerTestHelper) GetNodeAccount(ctx cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	if h.failGetNodeAccount {
		return NodeAccount{}, errKaboom
	}
	return h.Keeper.GetNodeAccount(ctx, addr)
}

func (h *LeaveHandlerTestHelper) SetNodeAccount(ctx cosmos.Context, na NodeAccount) error {
	if h.failSetNodeAccount {
		return errKaboom
	}
	return h.Keeper.SetNodeAccount(ctx, na)
}

func (h *LeaveHandlerTestHelper) GetVault(ctx cosmos.Context, pk common.PubKey) (Vault, error) {
	if h.failGetVault {
		return Vault{}, errKaboom
	}
	return h.Keeper.GetVault(ctx, pk)
}

func (HandlerLeaveSuite) TestLeaveDifferentValidations(c *C) {
	testCases := []struct {
		name            string
		messageProvider func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg
		validator       func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg)
	}{
		{
			name: "invalid message type should return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				return NewMsgNetworkFee(1024, common.BTCChain, 1, 10000, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "fail to get node account should return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				helper.failGetNodeAccount = true
				return NewMsgLeave(GetRandomTx(), GetRandomBech32Addr(), GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "empty node account should return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				return NewMsgLeave(GetRandomTx(), GetRandomBech32Addr(), GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "fail to refund bond with no vaults should not return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeStandby)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.Coins[0].Amount = cosmos.ZeroUint()
				tx.FromAddress = nodeAccount.BondAddress
				// when there is no asgard vault to refund, refund shouldn't fail
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "non-zero message coins should return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeStandby)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.FromAddress = nodeAccount.BondAddress
				vault := GetRandomVault()
				c.Assert(helper.SetVault(ctx, vault), IsNil)
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "vault not exist should refund bond",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeStandby)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.Coins[0].Amount = cosmos.ZeroUint()
				tx.FromAddress = nodeAccount.BondAddress
				vault := GetRandomVault()
				c.Assert(helper.SetVault(ctx, vault), IsNil)
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to get unrelated vault should not return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeStandby)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.Coins[0].Amount = cosmos.ZeroUint()
				tx.FromAddress = nodeAccount.BondAddress
				helper.failGetVault = true
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to save node account should return an error",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeStandby)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.FromAddress = nodeAccount.BondAddress
				asgardVault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain, common.BTCChain}.Strings(), []ChainContract{})
				c.Assert(helper.Keeper.SetVault(ctx, asgardVault), IsNil)
				helper.failSetNodeAccount = true
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, NotNil, Commentf(name))
				c.Check(result, IsNil, Commentf(name))
			},
		},
		{
			name: "when node account is active, allow leave but don't return bond",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Check(activeNodeAccount.Bond.Equal(cosmos.NewUint(1000*common.One)), Equals, true, Commentf(activeNodeAccount.Bond.String()))
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				tx := GetRandomTx()
				tx.Coins[0].Amount = cosmos.ZeroUint()
				tx.FromAddress = activeNodeAccount.BondAddress
				return NewMsgLeave(tx, activeNodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) {
				c.Check(err, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
				unwrappedMsgLeave, ok := msg.(*MsgLeave)
				c.Assert(ok, Equals, true)
				activeNodeAccount, err := helper.Keeper.GetNodeAccount(ctx, unwrappedMsgLeave.NodeAddress)
				c.Assert(err, IsNil)
				c.Check(activeNodeAccount.Bond.Equal(cosmos.NewUint(1000*common.One)), Equals, true, Commentf(activeNodeAccount.Bond.String()))
			},
		},
		{
			name: "when node account is still belongs to a retiring vault , don't return bond",
			messageProvider: func(ctx cosmos.Context, helper *LeaveHandlerTestHelper) cosmos.Msg {
				nodeAccount := GetRandomValidatorNode(NodeDisabled)
				nodeAccount.Bond = cosmos.NewUint(100)
				activeNodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, activeNodeAccount), IsNil)
				c.Assert(helper.Keeper.SetNodeAccount(ctx, nodeAccount), IsNil)
				tx := GetRandomTx()
				tx.Coins[0].Amount = cosmos.ZeroUint()
				tx.FromAddress = nodeAccount.BondAddress
				asgardVault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain, common.BTCChain}.Strings(), []ChainContract{})
				c.Assert(helper.Keeper.SetVault(ctx, asgardVault), IsNil)

				retiringVault := NewVault(1000, RetiringVault, AsgardVault, GetRandomPubKey(), common.Chains{common.ETHChain, common.BTCChain}.Strings(), []ChainContract{})
				retiringVault.Membership = common.PubKeys{
					nodeAccount.PubKeySet.Secp256k1,
					GetRandomPubKey(),
				}.Strings()
				c.Assert(helper.Keeper.SetVault(ctx, retiringVault), IsNil)
				return NewMsgLeave(tx, nodeAccount.NodeAddress, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err2 error, helper *LeaveHandlerTestHelper, name string, msg cosmos.Msg) { // nolint
				leaveMsg, ok := msg.(*MsgLeave)
				c.Assert(ok, Equals, true)
				na, err := helper.GetNodeAccount(ctx, leaveMsg.NodeAddress)
				c.Assert(err, IsNil)
				c.Assert(na.Bond.Equal(cosmos.NewUint(100)), Equals, true)
				c.Check(err2, IsNil, Commentf(name))
				c.Check(result, NotNil, Commentf(name))
			},
		},
	}

	for _, tc := range testCases {
		ctx, mgr := setupManagerForTest(c)
		FundModule(c, ctx, mgr.Keeper(), BondName, 1000*common.One)
		helper := NewLeaveHandlerTestHelper(mgr.Keeper())
		mgr.K = helper
		handler := NewLeaveHandler(mgr)
		msg := tc.messageProvider(ctx, helper)
		result, err := handler.Run(ctx, msg)
		tc.validator(c, ctx, result, err, helper, tc.name, msg)
	}
}
