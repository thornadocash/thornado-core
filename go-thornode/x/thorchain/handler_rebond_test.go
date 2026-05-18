package thorchain

import (
	"fmt"

	"gitlab.com/thorchain/thornode/v3/constants"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	. "gopkg.in/check.v1"
)

type HandlerReBondSuite struct{}

type TestReBondKeeper struct {
	keeper.KVStoreDummy
	activeNodeAccount   NodeAccount
	failGetNodeAccount  NodeAccount
	notEmptyNodeAccount NodeAccount
	jailNodeAccount     NodeAccount
}

func (k *TestReBondKeeper) GetNodeAccount(_ cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	if k.activeNodeAccount.NodeAddress.Equals(addr) {
		return k.activeNodeAccount, nil
	}
	if k.failGetNodeAccount.NodeAddress.Equals(addr) {
		return NodeAccount{}, fmt.Errorf("you asked for this error")
	}
	if k.notEmptyNodeAccount.NodeAddress.Equals(addr) {
		return k.notEmptyNodeAccount, nil
	}
	if k.jailNodeAccount.NodeAddress.Equals(addr) {
		return k.jailNodeAccount, nil
	}
	return NodeAccount{}, nil
}

var _ = Suite(&HandlerReBondSuite{})

func (HandlerReBondSuite) TestReBondHandlerValidate(c *C) {
	ctx, k := setupKeeperForTest(c)

	nodeAccount := GetRandomValidatorNode(NodeStandby)
	c.Assert(k.SetNodeAccount(ctx, nodeAccount), IsNil)

	vault := GetRandomVault()
	c.Assert(k.SetVault(ctx, vault), IsNil)

	FundModule(c, ctx, k, BondName, nodeAccount.Bond.Uint64())

	handler := NewReBondHandler(NewDummyMgrWithKeeper(k))

	txIn := common.NewTx(
		GetRandomTxHash(),
		GetRandomETHAddress(),
		GetRandomETHAddress(),
		common.Coins{
			common.NewCoin(common.RuneAsset(), cosmos.ZeroUint()),
		},
		common.Gas{
			common.NewCoin(common.ETHAsset, cosmos.NewUint(10000)),
		},
		"",
	)

	address1, _ := GetRandomTHORAddress().AccAddress()
	address2, _ := GetRandomTHORAddress().AccAddress()
	address3, _ := GetRandomTHORAddress().AccAddress()

	bondProviders, err := handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)

	// add node bond
	bondProviders.Providers = append(bondProviders.Providers, NewBondProvider(nodeAccount.NodeAddress))
	bondProviders.Bond(nodeAccount.Bond, nodeAccount.NodeAddress)

	// whitelist and bond 100 rune from address2
	amount := cosmos.NewUint(100 * common.One)
	bondProviders.Providers = append(bondProviders.Providers, NewBondProvider(address2))
	bondProviders.Bond(amount, address2)
	nodeAccount.Bond = nodeAccount.Bond.Add(amount)
	c.Assert(k.SetBondProviders(ctx, bondProviders), IsNil)
	c.Assert(k.SetNodeAccount(ctx, nodeAccount), IsNil)

	// rebond 1 rune from address1 to address2 -> fail because address1 is not even whitelisted yet
	msg := NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	// whitelist address1
	bondProviders.Providers = append(bondProviders.Providers, NewBondProvider(address1))
	c.Assert(k.SetBondProviders(ctx, bondProviders), IsNil)

	// rebond 1 rune from address1 to address2 -> fail because address1 bond is zero
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	// bond 50 rune from address1
	amount = cosmos.NewUint(50 * common.One)
	bondProviders.Bond(amount, address1)
	nodeAccount.Bond = nodeAccount.Bond.Add(amount)
	c.Assert(k.SetBondProviders(ctx, bondProviders), IsNil)
	c.Assert(k.SetNodeAccount(ctx, nodeAccount), IsNil)

	// rebond 15 rune from address1 to address2
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(15*common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Get(address1).Bond.Uint64(), Equals, uint64(35*common.One))
	c.Assert(bondProviders.Get(address2).Bond.Uint64(), Equals, uint64(115*common.One))

	haltRebond := k.GetConfigInt64(ctx, constants.HaltRebond)
	c.Assert(haltRebond, Equals, int64(0))

	k.SetMimir(ctx, "HaltRebond", 1)

	// haltRebond, err = k.GetMimir(ctx, constants.HaltRebond)
	haltRebond = k.GetConfigInt64(ctx, constants.HaltRebond)
	c.Assert(err, IsNil)
	c.Assert(haltRebond, Equals, int64(1))

	// rebond 5 rune from address1 to address2 -> fail because mimir disabled
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(5*common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	k.SetMimir(ctx, "HaltRebond", 0)

	// rebond 5 rune from address1 to address2 -> ok, enabled again
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(5*common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Get(address1).Bond.Uint64(), Equals, uint64(30*common.One))
	c.Assert(bondProviders.Get(address2).Bond.Uint64(), Equals, uint64(120*common.One))

	// rebond 50 rune from node address to address1 -> fail, node address is not allowed to rebond
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address1, cosmos.NewUint(50*common.One), nodeAccount.NodeAddress)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	// rebond remaining 30 rune from address1 to address2 by providing amount = 0
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.ZeroUint(), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Has(address1), Equals, false)
	c.Assert(bondProviders.Get(address2).Bond.Uint64(), Equals, uint64(150*common.One))

	// whitelist address1 again
	bondProviders.Providers = append(bondProviders.Providers, NewBondProvider(address1))
	c.Assert(k.SetBondProviders(ctx, bondProviders), IsNil)

	// rebond should be allowed on any node status
	for _, status := range []NodeStatus{
		NodeStandby,
		NodeReady,
		NodeActive,
		NodeDisabled,
	} {
		nodeAccount, err = k.GetNodeAccount(ctx, nodeAccount.NodeAddress)
		c.Assert(err, IsNil)

		nodeAccount.Status = status
		c.Assert(k.SetNodeAccount(ctx, nodeAccount), IsNil)

		// rebond 10 rune from address2 to address1 for
		msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address1, cosmos.NewUint(20*common.One), address2)
		_, err = handler.Run(ctx, msg)
		c.Assert(err, IsNil)
	}

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Get(address1).Bond.Uint64(), Equals, uint64(80*common.One))
	c.Assert(bondProviders.Get(address2).Bond.Uint64(), Equals, uint64(70*common.One))

	// rebond all 100 rune from address2 to address1 by providing amount > than available
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address1, cosmos.NewUint(999*common.One), address2)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Get(address1).Bond.Uint64(), Equals, uint64(150*common.One))
	c.Assert(bondProviders.Has(address2), Equals, false)

	// rebond 1 rune from address1 to address2 -> fail: address2 is not whitelisted anymore
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address2, cosmos.NewUint(common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	// rebond 1 rune from address1 to address3, signed by address 2 -> fail
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address3, cosmos.NewUint(common.One), address2)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, NotNil)

	// whitelist address3
	bondProviders.Providers = append(bondProviders.Providers, NewBondProvider(address3))
	c.Assert(k.SetBondProviders(ctx, bondProviders), IsNil)

	// rebond 1 rune from address1 to address3
	msg = NewMsgReBond(txIn, nodeAccount.NodeAddress, address3, cosmos.NewUint(common.One), address1)
	_, err = handler.Run(ctx, msg)
	c.Assert(err, IsNil)

	bondProviders, err = handler.mgr.Keeper().GetBondProviders(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)
	c.Assert(bondProviders.Get(address1).Bond.Uint64(), Equals, uint64(149*common.One))
	c.Assert(bondProviders.Get(address3).Bond.Uint64(), Equals, uint64(common.One))

	nodeAccount, err = handler.mgr.Keeper().GetNodeAccount(ctx, nodeAccount.NodeAddress)
	c.Assert(err, IsNil)

	// node bond didn't change
	c.Assert(nodeAccount.Bond.Uint64(), Equals, uint64(250*common.One))
}

func (HandlerReBondSuite) TestReBondHandlerFaultyMsg(c *C) {
	ctx, k := setupKeeperForTest(c)

	nodeAccount := GetRandomValidatorNode(NodeStandby)
	c.Assert(k.SetNodeAccount(ctx, nodeAccount), IsNil)

	vault := GetRandomVault()
	c.Assert(k.SetVault(ctx, vault), IsNil)

	FundModule(c, ctx, k, BondName, nodeAccount.Bond.Uint64())

	handler := NewReBondHandler(NewDummyMgrWithKeeper(k))

	testMsgs := []MsgReBond{
		{
			TxIn:                   GetRandomTx(),
			NodeAddress:            nil,
			NewBondProviderAddress: GetRandomBech32Addr(),
			Amount:                 cosmos.NewUint(1),
			Signer:                 nodeAccount.NodeAddress,
		},
		{
			TxIn:                   GetRandomTx(),
			NodeAddress:            GetRandomBech32Addr(),
			NewBondProviderAddress: nil,
			Amount:                 cosmos.NewUint(1),
			Signer:                 nodeAccount.NodeAddress,
		},
		{
			TxIn:                   GetRandomTx(),
			NodeAddress:            GetRandomBech32Addr(),
			NewBondProviderAddress: GetRandomBech32Addr(),
			Amount:                 cosmos.ZeroUint(),
			Signer:                 nodeAccount.NodeAddress,
		},
		{
			TxIn:                   GetRandomTx(),
			NodeAddress:            GetRandomBech32Addr(),
			NewBondProviderAddress: GetRandomBech32Addr(),
			Amount:                 cosmos.NewUint(1),
			Signer:                 nil,
		},
	}

	for _, msg := range testMsgs {
		c.Assert(handler.handle(ctx, msg), NotNil)
	}
}
