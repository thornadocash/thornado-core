package thorchain

import (
	"fmt"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

var _ = Suite(&HandlerErrataTxSuite{})

type HandlerErrataTxSuite struct{}

type TestErrataTxKeeper struct {
	keeper.KVStoreDummy
	observedTx ObservedTxVoter
	pool       Pool
	na         NodeAccount
	lps        LiquidityProviders
	err        error
}

func (k *TestErrataTxKeeper) ListActiveValidators(_ cosmos.Context) (NodeAccounts, error) {
	return NodeAccounts{k.na}, k.err
}

func (k *TestErrataTxKeeper) GetNodeAccount(_ cosmos.Context, _ cosmos.AccAddress) (NodeAccount, error) {
	return k.na, k.err
}

func (k *TestErrataTxKeeper) GetObservedTxInVoter(_ cosmos.Context, txID common.TxID) (ObservedTxVoter, error) {
	return k.observedTx, k.err
}

func (k *TestErrataTxKeeper) GetPool(_ cosmos.Context, _ common.Asset) (Pool, error) {
	return k.pool, k.err
}

func (k *TestErrataTxKeeper) SetPool(_ cosmos.Context, pool Pool) error {
	k.pool = pool
	return k.err
}

func (k *TestErrataTxKeeper) GetLiquidityProvider(_ cosmos.Context, asset common.Asset, addr common.Address) (LiquidityProvider, error) {
	for _, lp := range k.lps {
		if lp.RuneAddress.Equals(addr) {
			return lp, k.err
		}
	}
	return LiquidityProvider{}, k.err
}

func (k *TestErrataTxKeeper) SetLiquidityProvider(_ cosmos.Context, lp LiquidityProvider) {
	for i, skr := range k.lps {
		if skr.RuneAddress.Equals(lp.RuneAddress) {
			k.lps[i] = lp
		}
	}
}

func (k *TestErrataTxKeeper) GetErrataTxVoter(_ cosmos.Context, txID common.TxID, chain common.Chain) (ErrataTxVoter, error) {
	return NewErrataTxVoter(txID, chain), k.err
}

func (s *HandlerErrataTxSuite) TestValidate(c *C) {
	ctx, _ := setupKeeperForTest(c)

	keeper := &TestErrataTxKeeper{
		na: GetRandomValidatorNode(NodeActive),
	}

	handler := NewErrataTxHandler(NewDummyMgrWithKeeper(keeper))
	// happy path
	msg := NewMsgErrataTx(GetRandomTxHash(), common.ETHChain, keeper.na.NodeAddress)
	err := handler.validate(ctx, *msg)
	c.Assert(err, IsNil)

	// invalid msg
	msg = &MsgErrataTx{}
	err = handler.validate(ctx, *msg)
	c.Assert(err, NotNil)
}

func (s *HandlerErrataTxSuite) TestErrataHandlerHappyPath(c *C) {
	ctx, mgr := setupManagerForTest(c)

	txID := GetRandomTxHash()
	na := GetRandomValidatorNode(NodeActive)
	addr := GetRandomETHAddress()
	totalUnits := cosmos.NewUint(1600)
	observedTx := ObservedTx{
		Tx: common.Tx{
			ID:          txID,
			Chain:       common.ETHChain,
			FromAddress: addr,
			Coins: common.Coins{
				common.NewCoin(common.RuneAsset(), cosmos.NewUint(30*common.One)),
			},
			Memo: fmt.Sprintf("ADD:ETH.ETH:%s", GetRandomRUNEAddress()),
		},
	}
	keeper := &TestErrataTxKeeper{
		na: na,
		observedTx: ObservedTxVoter{
			Tx:     observedTx,
			Txs:    ObservedTxs{observedTx},
			Height: 1024,
		},
		pool: Pool{
			Asset:        common.ETHAsset,
			LPUnits:      totalUnits,
			BalanceRune:  cosmos.NewUint(100 * common.One),
			BalanceAsset: cosmos.NewUint(100 * common.One),
		},
		lps: LiquidityProviders{
			{
				RuneAddress:   addr,
				LastAddHeight: 5,
				Units:         totalUnits.QuoUint64(2),
				PendingRune:   cosmos.ZeroUint(),
			},
			{
				RuneAddress:   GetRandomETHAddress(),
				LastAddHeight: 10,
				Units:         totalUnits.QuoUint64(2),
				PendingRune:   cosmos.ZeroUint(),
			},
		},
	}
	mgr.K = keeper

	handler := NewErrataTxHandler(mgr)
	msg := NewMsgErrataTx(txID, common.ETHChain, na.NodeAddress)
	_, err := handler.handle(ctx, *msg)
	c.Assert(err, IsNil)
	c.Check(keeper.pool.BalanceRune.Equal(cosmos.NewUint(70*common.One)), Equals, true)
	c.Check(keeper.pool.BalanceAsset.Equal(cosmos.NewUint(100*common.One)), Equals, true)
	c.Check(keeper.lps[0].Units.IsZero(), Equals, true, Commentf("%d", keeper.lps[0].Units.Uint64()))
	c.Check(keeper.lps[0].LastAddHeight, Equals, int64(18))
}

type ErrataTxHandlerTestHelper struct {
	keeper.Keeper
	failListActiveNodeAccount bool
	failGetErrataTxVoter      bool
	failGetObserveTxVoter     bool
	failGetPool               bool
	failGetLiquidityProvider  bool
	failSetPool               bool
}

func NewErrataTxHandlerTestHelper(k keeper.Keeper) *ErrataTxHandlerTestHelper {
	return &ErrataTxHandlerTestHelper{
		Keeper: k,
	}
}

func (k *ErrataTxHandlerTestHelper) ListActiveValidators(ctx cosmos.Context) (NodeAccounts, error) {
	if k.failListActiveNodeAccount {
		return NodeAccounts{}, errKaboom
	}
	return k.Keeper.ListActiveValidators(ctx)
}

func (k *ErrataTxHandlerTestHelper) GetErrataTxVoter(ctx cosmos.Context, txID common.TxID, chain common.Chain) (ErrataTxVoter, error) {
	if k.failGetErrataTxVoter {
		return ErrataTxVoter{}, errKaboom
	}
	return k.Keeper.GetErrataTxVoter(ctx, txID, chain)
}

func (k *ErrataTxHandlerTestHelper) GetObservedTxInVoter(ctx cosmos.Context, txID common.TxID) (ObservedTxVoter, error) {
	if k.failGetObserveTxVoter {
		return ObservedTxVoter{}, errKaboom
	}
	return k.Keeper.GetObservedTxInVoter(ctx, txID)
}

func (k *ErrataTxHandlerTestHelper) GetPool(ctx cosmos.Context, asset common.Asset) (Pool, error) {
	if k.failGetPool {
		return NewPool(), errKaboom
	}
	return k.Keeper.GetPool(ctx, asset)
}

func (k *ErrataTxHandlerTestHelper) GetLiquidityProvider(ctx cosmos.Context, asset common.Asset, addr common.Address) (LiquidityProvider, error) {
	if k.failGetLiquidityProvider {
		return LiquidityProvider{}, errKaboom
	}
	return k.Keeper.GetLiquidityProvider(ctx, asset, addr)
}

func (k *ErrataTxHandlerTestHelper) SetPool(ctx cosmos.Context, pool Pool) error {
	if k.failSetPool {
		return errKaboom
	}
	return k.Keeper.SetPool(ctx, pool)
}

func (s *HandlerErrataTxSuite) TestErrataHandlerDifferentError(c *C) {
	testCases := []struct {
		name            string
		messageProvider func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg
		validator       func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string)
	}{
		{
			name: "invalid message should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				return NewMsgNetworkFee(1024, common.ETHChain, 1, 10000, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "message fail validation should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				return NewMsgErrataTx(GetRandomTxHash(), common.BTCChain, GetRandomBech32Addr())
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to list active account should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				helper.failListActiveNodeAccount = true
				return NewMsgErrataTx(GetRandomTxHash(), common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to get errata tx voter should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				helper.failGetErrataTxVoter = true
				return NewMsgErrataTx(GetRandomTxHash(), common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "if voter already sign the errata tx voter it should not do anything",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				txID := GetRandomTxHash()
				voter, _ := helper.Keeper.GetErrataTxVoter(ctx, txID, common.BTCChain)
				voter.Sign(nodeAccount.NodeAddress)
				helper.Keeper.SetErrataTxVoter(ctx, voter)
				return NewMsgErrataTx(txID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, NotNil, Commentf(name))
				c.Check(err, IsNil, Commentf(name))
			},
		},
		{
			name: "if voter doesn't have consensus it should not do anything",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				txID := GetRandomTxHash()
				nodeAcct1 := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAcct1), IsNil)
				return NewMsgErrataTx(txID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, NotNil, Commentf(name))
				c.Check(err, IsNil, Commentf(name))
			},
		},
		{
			name: "if voter had been processed it should not do anything",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				txID := GetRandomTxHash()
				voter, _ := helper.Keeper.GetErrataTxVoter(ctx, txID, common.BTCChain)
				voter.BlockHeight = ctx.BlockHeight()
				helper.Keeper.SetErrataTxVoter(ctx, voter)
				return NewMsgErrataTx(txID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, NotNil, Commentf(name))
				c.Check(err, IsNil, Commentf(name))
			},
		},
		{
			name: "if fail to get observed tx in it should return err",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				helper.failGetObserveTxVoter = true
				return NewMsgErrataTx(GetRandomTxHash(), common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "if observed tx is empty it should return err",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				return NewMsgErrataTx(GetRandomTxHash(), common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "if chain doesn't match it should not do anything",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				observedTx := GetRandomObservedTx()
				voter := ObservedTxVoter{
					TxID:   observedTx.Tx.ID,
					Tx:     observedTx,
					Txs:    ObservedTxs{observedTx},
					Height: observedTx.BlockHeight,
				}
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return NewMsgErrataTx(voter.TxID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, NotNil, Commentf(name))
				c.Check(err, IsNil, Commentf(name))
			},
		},
		{
			name: "if the tx is not swap nor provide liquidity, it should not do anything",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				observedTx := GetRandomObservedTx()
				observedTx.Tx.Chain = common.BTCChain
				observedTx.Tx.Memo = "withdraw"
				voter := ObservedTxVoter{
					TxID:   observedTx.Tx.ID,
					Tx:     observedTx,
					Txs:    ObservedTxs{observedTx},
					Height: observedTx.BlockHeight,
				}
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return NewMsgErrataTx(voter.TxID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, NotNil, Commentf(name))
				c.Check(err, IsNil, Commentf(name))
			},
		},
		{
			name: "if it fail to get pool it should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				observedTx := GetRandomObservedTx()
				observedTx.Tx.Chain = common.BTCChain
				observedTx.Tx.Memo = "swap:ETH"
				helper.failGetPool = true
				voter := ObservedTxVoter{
					TxID:   observedTx.Tx.ID,
					Tx:     observedTx,
					Txs:    ObservedTxs{observedTx},
					Height: observedTx.BlockHeight,
				}
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return NewMsgErrataTx(voter.TxID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "if fail to get liquidity provider it should return an error",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				observedTx := GetRandomObservedTx()
				observedTx.Tx.Chain = common.BTCChain
				observedTx.Tx.Memo = "add:BTC:" + observedTx.Tx.FromAddress.String()
				lp := LiquidityProvider{
					Asset:         common.BTCAsset,
					AssetAddress:  GetRandomETHAddress(),
					LastAddHeight: 1024,
					RuneAddress:   observedTx.Tx.FromAddress,
				}
				helper.SetLiquidityProvider(ctx, lp)
				helper.failGetLiquidityProvider = true
				pool := NewPool()
				pool.Asset = common.BTCAsset
				pool.BalanceRune = cosmos.NewUint(common.One * 100)
				pool.BalanceAsset = cosmos.NewUint(common.One * 100)
				pool.Status = PoolAvailable
				c.Assert(helper.Keeper.SetPool(ctx, pool), IsNil)
				voter := ObservedTxVoter{
					TxID:   observedTx.Tx.ID,
					Tx:     observedTx,
					Txs:    ObservedTxs{observedTx},
					Height: observedTx.BlockHeight,
				}
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return NewMsgErrataTx(voter.TxID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
		{
			name: "fail to save pool should error out",
			messageProvider: func(ctx cosmos.Context, helper *ErrataTxHandlerTestHelper) cosmos.Msg {
				// add an active node account
				nodeAccount := GetRandomValidatorNode(NodeActive)
				c.Assert(helper.SetNodeAccount(ctx, nodeAccount), IsNil)
				observedTx := GetRandomObservedTx()
				observedTx.Tx.Chain = common.BTCChain
				observedTx.Tx.Memo = "swap:BTC"
				helper.failSetPool = true
				pool := NewPool()
				pool.Asset = common.BTCAsset
				pool.BalanceRune = cosmos.NewUint(common.One * 100)
				pool.BalanceAsset = cosmos.NewUint(common.One * 100)
				pool.Status = PoolAvailable
				c.Assert(helper.Keeper.SetPool(ctx, pool), IsNil)
				voter := ObservedTxVoter{
					TxID:   observedTx.Tx.ID,
					Tx:     observedTx,
					Txs:    ObservedTxs{observedTx},
					Height: observedTx.BlockHeight,
				}
				helper.Keeper.SetObservedTxInVoter(ctx, voter)
				return NewMsgErrataTx(voter.TxID, common.BTCChain, nodeAccount.NodeAddress)
			},
			validator: func(c *C, ctx cosmos.Context, result *cosmos.Result, err error, helper *ErrataTxHandlerTestHelper, name string) {
				c.Check(result, IsNil, Commentf(name))
				c.Check(err, NotNil, Commentf(name))
			},
		},
	}

	for _, tc := range testCases {
		ctx, mgr := setupManagerForTest(c)
		helper := NewErrataTxHandlerTestHelper(mgr.Keeper())
		mgr.K = helper
		msg := tc.messageProvider(ctx, helper)
		handler := NewErrataTxHandler(mgr)
		result, err := handler.Run(ctx, msg)
		tc.validator(c, ctx, result, err, helper, tc.name)
	}
}

func (*HandlerErrataTxSuite) TestProcessErrataOutboundTx(c *C) {
	ctx, mgr := setupManagerForTest(c)
	helper := NewErrataTxHandlerTestHelper(mgr.Keeper())
	node1 := GetRandomValidatorNode(NodeActive)
	node2 := GetRandomValidatorNode(NodeActive)
	node3 := GetRandomValidatorNode(NodeActive)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node1), IsNil)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node2), IsNil)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node3), IsNil)

	// fail to get observed tx out voter
	txID := GetRandomTxHash()
	er := &common.ErrataTx{
		Id:    txID,
		Chain: common.LTCChain,
	}
	k := mgr.Keeper()
	eventMgr := mgr.EventMgr()
	err := processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, NotNil)

	observedPubKey := GetRandomPubKey()
	tx := common.NewTx(txID, GetRandomLTCAddress(), GetRandomLTCAddress(),
		common.Coins{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(102400000)),
		},
		common.Gas{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(1000)),
		}, "swap:LTC.LTC")
	observedTx := []common.ObservedTx{
		common.NewObservedTx(
			tx,
			1024, observedPubKey, 1024),
	}
	txOutVoter := NewObservedTxVoter(txID, observedTx)
	helper.Keeper.SetObservedTxOutVoter(ctx, txOutVoter)
	// Tx is empty , it should fail
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, NotNil)
	txOutVoter.Add(observedTx[0], node2.NodeAddress)
	txOutVoter.Add(observedTx[0], node3.NodeAddress)

	finalisedTx := txOutVoter.GetTx(NodeAccounts{node1, node2, node3})
	c.Assert(finalisedTx.IsEmpty(), Equals, false)
	// voter.Tx must be explicitly updated in the handler,
	// for instance on consensus.
	txOutVoter.Tx = *finalisedTx

	helper.Keeper.SetObservedTxOutVoter(ctx, txOutVoter)

	// not outbound tx , it should fail
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, NotNil)

	// fail to get vault
	txInID := GetRandomTxHash()
	txOutVoter.Tx.Tx.Memo = "OUT:" + txInID.String()
	helper.Keeper.SetObservedTxOutVoter(ctx, txOutVoter)
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, NotNil)

	// Active Asgard vault, TxInVoter not exist
	asgardVault := NewVault(1, types.VaultStatus_ActiveVault, AsgardVault, observedPubKey, []string{
		common.LTCChain.String(),
		common.BTCChain.String(),
		common.ETHChain.String(),
	}, []ChainContract{})
	c.Assert(helper.Keeper.SetVault(ctx, asgardVault), IsNil)
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, IsNil)

	// inactive vault - funds are credited but vault stays InactiveVault
	// (no resurrection to RetiringVault to avoid side effects)
	asgardVault.UpdateStatus(types.VaultStatus_InactiveVault, 1024)
	c.Assert(helper.Keeper.SetVault(ctx, asgardVault), IsNil)
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, IsNil)
	// vault should remain inactive (recovery via migration)
	v, err := helper.Keeper.GetVault(ctx, asgardVault.PubKey)
	c.Assert(err, IsNil)
	c.Assert(v.Status == types.VaultStatus_InactiveVault, Equals, true)

	// With POOL , but no txin voter
	pool := NewPool()
	pool.Asset = common.LTCAsset
	pool.BalanceAsset = cosmos.NewUint(1024 * common.One)
	pool.BalanceRune = cosmos.NewUint(1024 * common.One)
	pool.Status = PoolAvailable
	c.Assert(helper.Keeper.SetPool(ctx, pool), IsNil)
	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, IsNil)

	txInbound := common.NewTx(txInID, GetRandomLTCAddress(), GetRandomLTCAddress(),
		common.Coins{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(102400000)),
		},
		common.Gas{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(1000)),
		}, "swap:LTC.LTC")
	observedTxInbound := []common.ObservedTx{
		NewObservedTx(
			txInbound,
			1024, observedPubKey, 1024),
	}
	txInVoter := NewObservedTxVoter(txInID, observedTxInbound)
	helper.Keeper.SetObservedTxInVoter(ctx, txInVoter)
	newActiveAsgardVault := NewVault(1024, ActiveVault, AsgardVault, GetRandomPubKey(), []string{
		common.BTCChain.String(),
		common.LTCChain.String(),
		common.ETHChain.String(),
	}, []ChainContract{})
	newActiveAsgardVault.AddFunds(common.Coins{
		common.NewCoin(common.LTCAsset, cosmos.NewUint(1024*common.One)),
	})
	c.Assert(helper.Keeper.SetVault(ctx, newActiveAsgardVault), IsNil)
	c.Assert(helper.Keeper.SaveNetworkFee(ctx, common.LTCChain, NetworkFee{
		Chain:              common.LTCChain,
		TransactionSize:    250,
		TransactionFeeRate: 10,
	}), IsNil)

	// clear events
	ctx = ctx.WithEventManager(cosmos.NewEventManager())

	err = processErrataOutboundTx(ctx, k, eventMgr, er)
	c.Assert(err, IsNil)
	txOut, err := helper.Keeper.GetTxOut(ctx, ctx.BlockHeight())
	c.Assert(err, IsNil)

	// we no longer re-attempt the outbound, and instead just emit a security event
	c.Assert(txOut.TxArray, HasLen, 0)
	found := 0
	for _, event := range ctx.EventManager().Events() {
		if event.Type == "security" {
			found++
		}
	}
	c.Assert(found, Equals, 1)
}

func (*HandlerErrataTxSuite) TestProcessErrataOutboundTx_EnsureFundsAreCreditedToInactiveVault(c *C) {
	ctx, mgr := setupManagerForTest(c)
	helper := NewErrataTxHandlerTestHelper(mgr.Keeper())
	mgr.K = helper
	k := mgr.Keeper()
	eventMgr := mgr.EventMgr()
	handler := NewErrataTxHandler(mgr)
	node1 := GetRandomValidatorNode(NodeActive)
	node2 := GetRandomValidatorNode(NodeActive)
	node3 := GetRandomValidatorNode(NodeActive)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node1), IsNil)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node2), IsNil)
	c.Assert(helper.Keeper.SetNodeAccount(ctx, node3), IsNil)

	retiredPubKey := GetRandomPubKey()
	activePubKey := GetRandomPubKey()
	// inactive vault, TxInVoter not exist
	inactiveVault := NewVault(1, types.VaultStatus_InactiveVault, AsgardVault, retiredPubKey, []string{
		common.LTCChain.String(),
		common.BTCChain.String(),
		common.ETHChain.String(),
	}, []ChainContract{})

	activeVault := NewVault(1, types.VaultStatus_ActiveVault, AsgardVault, activePubKey, []string{
		common.LTCChain.String(),
		common.BTCChain.String(),
		common.ETHChain.String(),
	}, []ChainContract{})
	c.Assert(helper.Keeper.SetVault(ctx, inactiveVault), IsNil)
	c.Assert(helper.Keeper.SetVault(ctx, activeVault), IsNil)
	// migration , internal tx , should cause the vault to be set back to retiring

	txID1 := GetRandomTxHash()
	internalMigrationTx := NewMsgErrataTx(txID1, common.LTCChain, node1.NodeAddress)
	internalErrata := &common.ErrataTx{
		Id:    txID1,
		Chain: common.LTCChain,
	}

	migrateTx := common.NewTx(txID1,
		GetRandomLTCAddress(),
		GetRandomLTCAddress(),
		common.Coins{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(102400000)),
		},
		common.Gas{
			common.NewCoin(common.LTCAsset, cosmos.NewUint(1000)),
		}, "migrate:111")

	// observed outbound tx in the retired vault
	observedTx := []common.ObservedTx{
		common.NewObservedTx(
			migrateTx,
			1024, retiredPubKey, 1024),
	}
	txOutVoter := NewObservedTxVoter(txID1, observedTx)
	txOutVoter.Add(observedTx[0], node2.NodeAddress)
	txOutVoter.Add(observedTx[0], node3.NodeAddress)

	finalisedTx := txOutVoter.GetTx(NodeAccounts{node1, node2, node3})
	c.Assert(finalisedTx.IsEmpty(), Equals, false)
	// voter.Tx must be explicitly updated in the handler,
	// for instance on consensus.
	txOutVoter.Tx = *finalisedTx

	helper.Keeper.SetObservedTxOutVoter(ctx, txOutVoter)

	// (identical) observed inbound tx in the same retired vault
	observedInboundTx := []common.ObservedTx{
		common.NewObservedTx(

			migrateTx,
			1024, retiredPubKey, 1024),
	}
	txInVoter := NewObservedTxVoter(txID1, observedTx)
	txInVoter.Add(observedInboundTx[0], node2.NodeAddress)
	txInVoter.Add(observedInboundTx[0], node3.NodeAddress)

	finalisedTx = txInVoter.GetTx(NodeAccounts{node1, node2, node3})
	c.Assert(finalisedTx.IsEmpty(), Equals, false)
	// voter.Tx must be explicitly updated in the handler,
	// for instance on consensus.
	txInVoter.Tx = *finalisedTx
	helper.Keeper.SetObservedTxInVoter(ctx, txInVoter)

	err := processErrataOutboundTx(ctx, k, eventMgr, internalErrata)
	c.Assert(err, IsNil)
	v, err := helper.Keeper.GetVault(ctx, retiredPubKey)
	c.Assert(err, IsNil)
	// Vault should remain InactiveVault (not resurrected to RetiringVault)
	// to avoid side effects like blocking churns or affecting node unbonding.
	// Recovery from InactiveVaults with funds should be handled via migration.
	c.Assert(v.Status == types.VaultStatus_InactiveVault, Equals, true)
	c.Assert(v.HasFunds(), Equals, true)
	ltcCoin := v.GetCoin(common.LTCAsset)
	c.Assert(ltcCoin.Equals(common.NewCoin(common.LTCAsset, cosmos.NewUint(102400000))), Equals, true)

	// reset the inactive vault
	c.Assert(helper.Keeper.SetVault(ctx, inactiveVault), IsNil)
	errataVoter := NewErrataTxVoter(txID1, common.LTCChain)
	errataVoter.Sign(node2.NodeAddress)
	errataVoter.Sign(node3.NodeAddress)
	helper.Keeper.SetErrataTxVoter(ctx, errataVoter)
	result, err := handler.handle(ctx, *internalMigrationTx)
	c.Assert(result, NotNil)
	c.Assert(err, IsNil)
	v, err = helper.Keeper.GetVault(ctx, retiredPubKey)
	c.Assert(err, IsNil)
	// Same here - vault stays InactiveVault, funds are credited
	c.Assert(v.Status == types.VaultStatus_InactiveVault, Equals, true)
	c.Assert(v.HasFunds(), Equals, true)
	ltcCoin = v.GetCoin(common.LTCAsset)
	c.Assert(ltcCoin.Equals(common.NewCoin(common.LTCAsset, cosmos.NewUint(102400000))), Equals, true)
}

func (s *HandlerErrataTxSuite) TestObservingSlashing(c *C) {
	ctx, mgr := setupManagerForTest(c)
	height := int64(1024)
	ctx = ctx.WithBlockHeight(height)

	// Check expected slash point amounts
	observeSlashPoints := mgr.GetConstants().GetInt64Value(constants.ObserveSlashPoints)
	lackOfObservationPenalty := mgr.GetConstants().GetInt64Value(constants.LackOfObservationPenalty)
	observeFlex := mgr.GetConstants().GetInt64Value(constants.ObservationDelayFlexibility)
	c.Assert(observeSlashPoints, Equals, int64(1))
	c.Assert(lackOfObservationPenalty, Equals, int64(2))
	c.Assert(observeFlex, Equals, int64(10))

	asgardVault := GetRandomVault()
	c.Assert(mgr.Keeper().SetVault(ctx, asgardVault), IsNil)

	nas := NodeAccounts{
		// 6 Active nodes, 1 Standby node; 2/3rds consensus needs 4.
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeActive),
		GetRandomValidatorNode(NodeStandby),
	}
	for _, item := range nas {
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, item), IsNil)
	}

	txID := GetRandomTxHash()
	addr := GetRandomETHAddress()
	totalUnits := cosmos.NewUint(1600)

	observedTx := ObservedTx{
		Tx: common.Tx{
			ID:          txID,
			Chain:       common.ETHChain,
			FromAddress: addr,
			Coins: common.Coins{
				common.NewCoin(common.RuneAsset(), cosmos.NewUint(30*common.One)),
			},
			Memo: fmt.Sprintf("ADD:ETH.ETH:%s", GetRandomRUNEAddress()),
		},
		BlockHeight:    1024,
		FinaliseHeight: 1024,
	}
	voter := ObservedTxVoter{
		TxID:            observedTx.Tx.ID,
		Tx:              observedTx,
		Txs:             common.ObservedTxs{observedTx},
		Height:          observedTx.BlockHeight,
		FinalisedHeight: observedTx.BlockHeight,
	}
	mgr.Keeper().SetObservedTxInVoter(ctx, voter)

	pool := Pool{
		Asset:        common.ETHAsset,
		LPUnits:      totalUnits,
		BalanceRune:  cosmos.NewUint(100 * common.One),
		BalanceAsset: cosmos.NewUint(100 * common.One),
		Status:       PoolAvailable,
	}
	c.Assert(mgr.Keeper().SetPool(ctx, pool), IsNil)

	lp := LiquidityProvider{
		RuneAddress:   addr,
		LastAddHeight: 5,
		Units:         totalUnits.QuoUint64(2),
		PendingRune:   cosmos.ZeroUint(),
	}
	mgr.Keeper().SetLiquidityProvider(ctx, lp)
	lp = LiquidityProvider{
		RuneAddress:   GetRandomETHAddress(),
		LastAddHeight: 10,
		Units:         totalUnits.QuoUint64(2),
		PendingRune:   cosmos.ZeroUint(),
	}
	mgr.Keeper().SetLiquidityProvider(ctx, lp)

	msg := NewMsgErrataTx(voter.TxID, common.BTCChain, cosmos.AccAddress{})
	handler := NewErrataTxHandler(mgr)

	broadcast := func(c *C, ctx cosmos.Context, na NodeAccount, msg *MsgErrataTx) {
		msg.Signer = na.NodeAddress
		_, err := handler.handle(ctx, *msg)
		c.Assert(err, IsNil)
	}

	checkSlashPoints := func(c *C, ctx cosmos.Context, nas NodeAccounts, expected [7]int64) {
		var slashPoints [7]int64
		for i, na := range nas {
			slashPoint, err := mgr.Keeper().GetNodeAccountSlashPoints(ctx, na.NodeAddress)
			c.Assert(err, IsNil)
			slashPoints[i] = slashPoint
		}
		c.Assert(slashPoints == expected, Equals, true, Commentf(fmt.Sprint(slashPoints)))
	}

	checkSlashPoints(c, ctx, nas, [7]int64{0, 0, 0, 0, 0, 0, 0})

	// 3/6 Active nodes observe.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[1], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 0, 0, 0, 0, 0})
	broadcast(c, ctx, nas[2], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 1, 1, 0, 0, 0, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 1, 1, 0, 0, 0, 0})

	// nas[3] observes, reaching consensus (4/6, being exactly the 2/3 threshold).
	// (Active nodes which observed are decremented ObserveSlashPoints;
	//  those which haven't are incremented LackOfObservationPenalty.)
	broadcast(c, ctx, nas[3], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 2, 2, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 2, 2, 0})

	// Within the ObservationDelayFlexibility period, nas[4] observes
	// (and is decremented LackOfObservationPenalty).
	height += observeFlex
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[4], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 2, 0})

	// The ObservationDelayFlexibility period ends, after which nas[5] observes;
	// it is not incremented ObserveSlashPoints (and it is added to the list of signers)
	// and is also not decremented LackOfObservationPenalty.
	height++
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 2, 0})

	// nas[5] observes again, this time incremented ObserveSlashPoints for the extra signing.
	broadcast(c, ctx, nas[5], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 3, 0})

	// Note that nas[6], the Standby node, remains unaffected by the Actives nodes' observations.
}
