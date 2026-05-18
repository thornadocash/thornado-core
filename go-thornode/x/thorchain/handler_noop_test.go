package thorchain

import (
	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	. "gopkg.in/check.v1"
)

type HandlerNoOpSuite struct{}

var _ = Suite(&HandlerNoOpSuite{})

func (HandlerNoOpSuite) TestNoOpValidation(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Test 1: Empty action (should pass, does nothing)
	m := NewMsgNoOp(GetRandomObservedTx(), w.activeNodeAccount.NodeAddress, "")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// Test 2: Non-novault action (should pass, does nothing)
	m = NewMsgNoOp(GetRandomObservedTx(), w.activeNodeAccount.NodeAddress, "other")
	result, err = h.Run(w.ctx, m)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
}

func (HandlerNoOpSuite) TestNoOpVaultValidation(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Test with nonexistent vault
	obsTx := GetRandomObservedTx()
	m := NewMsgNoOp(obsTx, w.activeNodeAccount.NodeAddress, "novault")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	// The keeper returns "vault not found" when vault doesn't exist
	c.Assert(err.Error(), Matches, ".*vault not found.*")
}

func (HandlerNoOpSuite) TestNoOpInactiveVault(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Create an inactive vault
	pk := GetRandomPubKey()
	vault := NewVault(w.ctx.BlockHeight(), InactiveVault, AsgardVault, pk, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(100*common.One))})
	c.Assert(w.mgr.Keeper().SetVault(w.ctx, vault), IsNil)

	// Create observed tx pointing to the inactive vault
	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomETHAddress(),
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(10*common.One))},
		common.Gas{{Asset: common.ETHAsset, Amount: cosmos.NewUint(37500)}},
		"",
	)
	obsTx := common.NewObservedTx(tx, 33, pk, 33)

	m := NewMsgNoOp(obsTx, w.activeNodeAccount.NodeAddress, "novault")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Matches, ".*vault is not active or retiring.*")
}

func (HandlerNoOpSuite) TestNoOpInsufficientFunds(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Create an active vault with limited funds
	pk := GetRandomPubKey()
	vault := NewVault(w.ctx.BlockHeight(), ActiveVault, AsgardVault, pk, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	vault.AddFunds(common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(5*common.One))})
	c.Assert(w.mgr.Keeper().SetVault(w.ctx, vault), IsNil)

	// Create observed tx with more coins than the vault has
	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomETHAddress(),
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, cosmos.NewUint(10*common.One))},
		common.Gas{{Asset: common.ETHAsset, Amount: cosmos.NewUint(37500)}},
		"",
	)
	obsTx := common.NewObservedTx(tx, 33, pk, 33)

	m := NewMsgNoOp(obsTx, w.activeNodeAccount.NodeAddress, "novault")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(err.Error(), Matches, ".*vault has insufficient funds.*")
}

func (HandlerNoOpSuite) TestNoOpSuccessfulSubtraction(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Create an active vault with sufficient funds
	pk := GetRandomPubKey()
	vault := NewVault(w.ctx.BlockHeight(), ActiveVault, AsgardVault, pk, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	initialAmount := cosmos.NewUint(100 * common.One)
	subtractAmount := cosmos.NewUint(10 * common.One)
	vault.AddFunds(common.Coins{common.NewCoin(common.ETHAsset, initialAmount)})
	c.Assert(w.mgr.Keeper().SetVault(w.ctx, vault), IsNil)

	// Create observed tx with coins to subtract
	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomETHAddress(),
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, subtractAmount)},
		common.Gas{{Asset: common.ETHAsset, Amount: cosmos.NewUint(37500)}},
		"",
	)
	obsTx := common.NewObservedTx(tx, 33, pk, 33)

	m := NewMsgNoOp(obsTx, w.activeNodeAccount.NodeAddress, "novault")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// Verify funds were subtracted
	updatedVault, err := w.mgr.Keeper().GetVault(w.ctx, pk)
	c.Assert(err, IsNil)
	expectedAmount := initialAmount.Sub(subtractAmount)
	c.Assert(updatedVault.GetCoin(common.ETHAsset).Amount.Equal(expectedAmount), Equals, true)
}

func (HandlerNoOpSuite) TestNoOpRetiringVaultAllowed(c *C) {
	w := getHandlerTestWrapper(c, 1, true, true)
	h := NewNoOpHandler(w.mgr)

	// Create a retiring vault with sufficient funds
	pk := GetRandomPubKey()
	vault := NewVault(w.ctx.BlockHeight(), RetiringVault, AsgardVault, pk, common.Chains{common.ETHChain}.Strings(), []ChainContract{})
	initialAmount := cosmos.NewUint(100 * common.One)
	subtractAmount := cosmos.NewUint(10 * common.One)
	vault.AddFunds(common.Coins{common.NewCoin(common.ETHAsset, initialAmount)})
	c.Assert(w.mgr.Keeper().SetVault(w.ctx, vault), IsNil)

	// Create observed tx with coins to subtract
	tx := common.NewTx(
		GetRandomTxHash(),
		GetRandomETHAddress(),
		GetRandomETHAddress(),
		common.Coins{common.NewCoin(common.ETHAsset, subtractAmount)},
		common.Gas{{Asset: common.ETHAsset, Amount: cosmos.NewUint(37500)}},
		"",
	)
	obsTx := common.NewObservedTx(tx, 33, pk, 33)

	m := NewMsgNoOp(obsTx, w.activeNodeAccount.NodeAddress, "novault")
	result, err := h.Run(w.ctx, m)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// Verify funds were subtracted
	updatedVault, err := w.mgr.Keeper().GetVault(w.ctx, pk)
	c.Assert(err, IsNil)
	expectedAmount := initialAmount.Sub(subtractAmount)
	c.Assert(updatedVault.GetCoin(common.ETHAsset).Amount.Equal(expectedAmount), Equals, true)
}
