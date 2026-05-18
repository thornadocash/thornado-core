package thorchain

import (
	"errors"
	"fmt"

	se "github.com/cosmos/cosmos-sdk/types/errors"
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/constants"
	. "gopkg.in/check.v1"
)

type HandlerSolvencyTestSuite struct{}

var _ = Suite(&HandlerSolvencyTestSuite{})

func (s *HandlerSolvencyTestSuite) TestValidate(c *C) {
	ctx, mgr := setupManagerForTest(c)

	handler := NewSolvencyHandler(mgr)
	// msgSolvency signed by  none active node should be rejected
	msgSolvency, err := NewMsgSolvency(common.ETHChain,
		GetRandomPubKey(),
		common.NewCoins(
			common.NewCoin(common.ETHAsset, cosmos.NewUint(1024*common.One)),
		),
		1024,
		GetRandomBech32Addr())
	c.Assert(err, IsNil)
	c.Assert(handler.validate(ctx, *msgSolvency), NotNil)
	// active node
	var activeNodes [4]NodeAccount
	for i := 0; i < 4; i++ {
		node := GetRandomValidatorNode(NodeActive)
		activeNodes[i] = node
		c.Assert(mgr.Keeper().SetNodeAccount(ctx, node), IsNil)
	}
	// msgSolvency signed by active node should be accepted
	msgSolvency.Signer = activeNodes[0].NodeAddress

	c.Assert(err, IsNil)
	c.Assert(handler.validate(ctx, *msgSolvency), IsNil)

	result, err := handler.Run(ctx, msgSolvency)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	// solvency voter should have been created
	voter, err := mgr.Keeper().GetSolvencyVoter(ctx, msgSolvency.Id, msgSolvency.Chain)
	c.Assert(err, IsNil)
	c.Assert(voter.Empty(), Equals, false)

	asgard := NewVault(1024, ActiveVault, AsgardVault, msgSolvency.PubKey, []string{
		common.ETHChain.String(),
		common.BTCChain.String(),
		common.LTCChain.String(),
		common.BCHChain.String(),
	}, nil)
	asgard.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1024*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, asgard), IsNil)

	// Set up ETH pool with liquidity so gap can be valued in RUNE
	// Pool has 1000 ETH and 10000 RUNE, so 1 ETH = 10 RUNE
	ethPool := NewPool()
	ethPool.Asset = common.ETHAsset
	ethPool.BalanceAsset = cosmos.NewUint(1000 * common.One)
	ethPool.BalanceRune = cosmos.NewUint(10000 * common.One)
	ethPool.Status = PoolAvailable
	c.Assert(mgr.Keeper().SetPool(ctx, ethPool), IsNil)

	// Set RUNE price to $5 so DollarConfigInRune can convert the $500 threshold
	// At $5/RUNE, $500 = 100 RUNE
	mgr.Keeper().SetMimir(ctx, "DollarsPerRune", 5_00000000)

	// second active node report solvency , it should be accepted
	// reach consensus , vault is solvent , everything continues
	msgSolvency.Signer = activeNodes[1].NodeAddress
	result, err = handler.Run(ctx, msgSolvency)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// third active node report solvency , it should be accepted
	msgSolvency.Signer = activeNodes[2].NodeAddress
	result, err = handler.Run(ctx, msgSolvency)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// vault suppose to have 1024 ETH, however only 100 left , vault is insolvent , chain should stop
	msgSolvency1, err := NewMsgSolvency(common.ETHChain,
		asgard.PubKey,
		common.NewCoins(
			common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One)),
		),
		1024,
		GetRandomBech32Addr())
	c.Assert(err, IsNil)
	msgSolvency1.Signer = activeNodes[0].NodeAddress
	result, err = handler.Run(ctx, msgSolvency1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// minority , so the second voter should reach consensus
	msgSolvency1.Signer = activeNodes[1].NodeAddress
	result, err = handler.Run(ctx, msgSolvency1)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	halt, err := mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, ctx.BlockHeight())
	c.Assert(mgr.Keeper().DeleteMimir(ctx, "SolvencyHaltETHChain"), IsNil)

	// vault suppose to have 1024 ETH, however only 1000 left , but there are 20 ETH in the pending outbound queue
	// chain should not stopped
	txOut := NewTxOut(ctx.BlockHeight())
	txOut.TxArray = []TxOutItem{
		{
			Chain:       common.ETHChain,
			ToAddress:   "0x3a196410a0f5facd08fd7880a4b8551cd085c031",
			VaultPubKey: asgard.PubKey,
			Coin:        common.NewCoin(common.ETHAsset, cosmos.NewUint(20*common.One)),
			Memo:        "OUT:693c3337193b1185fb0a36d8b7ec3f612ad57e599fd25e7ad6ec887aae43b291",
			InHash:      "693c3337193b1185fb0a36d8b7ec3f612ad57e599fd25e7ad6ec887aae43b291",
		},
	}
	c.Assert(mgr.Keeper().SetTxOut(ctx, txOut), IsNil)
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + 100)
	msgSolvency2, err := NewMsgSolvency(common.ETHChain,
		asgard.PubKey,
		common.NewCoins(
			common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One)),
		),
		1024,
		GetRandomBech32Addr())

	c.Assert(err, IsNil)
	msgSolvency2.Signer = activeNodes[0].NodeAddress
	result, err = handler.Run(ctx, msgSolvency2)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	msgSolvency2.Signer = activeNodes[1].NodeAddress
	result, err = handler.Run(ctx, msgSolvency2)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	halt, err = mgr.Keeper().GetMimir(ctx, "SolvencyHaltETHChain")
	c.Assert(err, IsNil)
	c.Assert(halt, Equals, int64(-1))

	// tampered MsgSolvency should be rejected
	msgSolvency2.Coins = common.NewCoins(
		common.NewCoin(common.ETHAsset, cosmos.NewUint(1024*common.One)),
	)
	result, err = handler.Run(ctx, msgSolvency2)
	c.Assert(err, NotNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)
	c.Assert(result, IsNil)
}

func (s *HandlerSolvencyTestSuite) TestObservingSlashing(c *C) {
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
	asgardVault.Chains = []string{common.ETHChain.String()}
	asgardVault.AddFunds(common.NewCoins(common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One))))
	c.Assert(mgr.Keeper().SetVault(ctx, asgardVault), IsNil)

	nas := NodeAccounts{
		// 6 Active nodes, 1 Standby node; 1/3rd SolvencyVoter consensus needs 2.
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

	msg, err := NewMsgSolvency(common.ETHChain,
		asgardVault.PubKey,
		common.NewCoins(
			common.NewCoin(common.ETHAsset, cosmos.NewUint(1000*common.One)),
		),
		1024,
		cosmos.AccAddress{})
	c.Assert(err, IsNil)

	handler := NewSolvencyHandler(mgr)

	broadcast := func(c *C, ctx cosmos.Context, na NodeAccount, msg *MsgSolvency) {
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

	// 1 of 6 Active nodes observe.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 0, 0, 0, 0, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 0, 0, 0, 0})

	// nas[1] observes, reaching consensus (2/6, being exactly the 1/3 SolvencyVoter threshold).
	// (Active nodes which observed are decremented ObserveSlashPoints;
	//  those which haven't are incremented LackOfObservationPenalty.)
	broadcast(c, ctx, nas[1], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{1, 0, 2, 2, 2, 2, 0})

	// nas[0] observes again.
	broadcast(c, ctx, nas[0], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 2, 2, 2, 2, 0})

	// Within the ObservationDelayFlexibility period, nas[2] observes
	// (and is decremented LackOfObservationPenalty).
	height += observeFlex
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[2], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 2, 2, 2, 0})

	// The ObservationDelayFlexibility period ends, after which nas[3] observes;
	// it is not incremented ObserveSlashPoints (and it is added to the list of signers)
	// and is also not decremented LackOfObservationPenalty.
	height++
	ctx = ctx.WithBlockHeight(height)
	broadcast(c, ctx, nas[3], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 2, 2, 2, 0})

	// nas[3] observes again, this time incremented ObserveSlashPoints for the extra signing.
	broadcast(c, ctx, nas[3], msg)
	checkSlashPoints(c, ctx, nas, [7]int64{2, 0, 0, 3, 2, 2, 0})

	// Note that nas[6], the Standby node, remains unaffected by the Actives nodes' observations.
}
