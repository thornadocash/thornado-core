package thorchain

import (
	"errors"
	"fmt"

	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common"
	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerMaintSuite struct {
	HandlerSuite
}

var _ = Suite(&HandlerMaintSuite{})

// Simple MaintHandler implementation for testing
type TestMaintHandler struct {
	nodes              map[string]NodeAccount
	failGetNodeAccount bool
	failSetNodeAccount bool
}

func NewTestMaintHandler() *TestMaintHandler {
	return &TestMaintHandler{
		nodes: make(map[string]NodeAccount),
	}
}

func (h *TestMaintHandler) AddNodeAccount(na NodeAccount) {
	h.nodes[na.NodeAddress.String()] = na
}

func (h *TestMaintHandler) Validate(ctx cosmos.Context, msg types.MsgMaint) error {
	if h.failGetNodeAccount {
		return cosmos.ErrUnauthorized("is not authorized")
	}

	// Validate that node address exists and is valid
	nodeAccount, exists := h.nodes[msg.NodeAddress.String()]
	if !exists {
		ctx.Logger().Error("fail to get node account", "address", msg.NodeAddress.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", msg.NodeAddress))
	}

	// Check that the signer is the bond address for the node
	bondAddr, err := nodeAccount.BondAddress.AccAddress()
	if err != nil {
		return err
	}

	if nodeAccount.IsEmpty() || !bondAddr.Equals(msg.Signer) {
		ctx.Logger().Error("unauthorized account", "operator", bondAddr.String(), "signer", msg.Signer.String())
		return cosmos.ErrUnauthorized(fmt.Sprintf("%s is not authorized", msg.NodeAddress))
	}

	return nil
}

func (h *TestMaintHandler) Handle(ctx cosmos.Context, msg types.MsgMaint) error {
	if h.failGetNodeAccount {
		return cosmos.ErrUnauthorized("unable to find account")
	}

	// Get node account by the NodeAddress from the message, not the signer
	nodeAccount, exists := h.nodes[msg.NodeAddress.String()]
	if !exists {
		return cosmos.ErrUnauthorized(fmt.Sprintf("unable to find account: %s", msg.NodeAddress))
	}

	if h.failSetNodeAccount {
		return errors.New("fail to save node account")
	}

	// Toggle the maintenance flag
	nodeAccount.Maintenance = !nodeAccount.Maintenance
	h.nodes[msg.NodeAddress.String()] = nodeAccount

	return nil
}

func (s *HandlerMaintSuite) TestMaintValidate(c *C) {
	ctx, _ := setupKeeperForTest(c)
	maintHandler := NewTestMaintHandler()

	// Invalid message - non-existent node account
	emptyMsg := types.NewMsgMaint(GetRandomBech32Addr(), GetRandomBech32Addr())
	err := maintHandler.Validate(ctx, *emptyMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*is not authorized.*")

	// Set up test data
	// Create node account with a specific bond address
	nodeAddress := GetRandomBech32Addr()
	bondAddress := common.Address(GetRandomBech32Addr().String())

	// Create a node account
	nodeAccount := NodeAccount{
		NodeAddress: nodeAddress,
		BondAddress: bondAddress,
		Status:      NodeActive,
		Maintenance: false,
	}

	// Add the node account to the test handler
	maintHandler.AddNodeAccount(nodeAccount)

	// Convert bond address to AccAddress for use in tests
	bondAccAddress, err := bondAddress.AccAddress()
	c.Assert(err, IsNil)

	// Test 1: Valid authorization - signer is the bond address
	msg := types.NewMsgMaint(nodeAddress, bondAccAddress)
	err = maintHandler.Validate(ctx, *msg)
	c.Assert(err, IsNil, Commentf("Bond address holder should be able to toggle maintenance"))

	// Test 2: Invalid authorization - signer is not the bond address
	randomSigner := GetRandomBech32Addr()
	msg = types.NewMsgMaint(nodeAddress, randomSigner)
	err = maintHandler.Validate(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*is not authorized.*")

	// Test 3: Invalid node address
	invalidAddr := GetRandomBech32Addr()
	msg = types.NewMsgMaint(invalidAddr, bondAccAddress)
	err = maintHandler.Validate(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, fmt.Sprintf(".*%s is not authorized.*", invalidAddr))

	// Test 4: Error getting node account
	maintHandler.failGetNodeAccount = true
	msg = types.NewMsgMaint(nodeAddress, bondAccAddress)
	err = maintHandler.Validate(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*is not authorized.*")

	// Test 5: Empty node account
	maintHandler.failGetNodeAccount = false
	emptyNodeAddr := GetRandomBech32Addr()
	emptyNode := NodeAccount{
		NodeAddress: emptyNodeAddr,
	}
	maintHandler.AddNodeAccount(emptyNode)
	msg = types.NewMsgMaint(emptyNodeAddr, bondAccAddress)
	err = maintHandler.Validate(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*empty address string is not allowed.*")
}

func (s *HandlerMaintSuite) TestMaintHandle(c *C) {
	ctx, _ := setupKeeperForTest(c)
	maintHandler := NewTestMaintHandler()

	// Test toggling maintenance from false to true
	nodeAddress := GetRandomBech32Addr()
	bondAddress := common.Address(GetRandomBech32Addr().String())

	// Create a node account
	nodeAccount := NodeAccount{
		NodeAddress: nodeAddress,
		BondAddress: bondAddress,
		Status:      NodeActive,
		Maintenance: false,
	}
	maintHandler.AddNodeAccount(nodeAccount)

	// Convert bond address to AccAddress for use in tests
	bondAccAddress, err := bondAddress.AccAddress()
	c.Assert(err, IsNil)

	// Test 1: Toggle maintenance using bond address as signer
	msg := types.NewMsgMaint(nodeAddress, bondAccAddress)
	err = maintHandler.Handle(ctx, *msg)
	c.Assert(err, IsNil)
	c.Assert(maintHandler.nodes[nodeAddress.String()].Maintenance, Equals, true)

	// Test 2: Toggle maintenance back to false
	err = maintHandler.Handle(ctx, *msg)
	c.Assert(err, IsNil)
	c.Assert(maintHandler.nodes[nodeAddress.String()].Maintenance, Equals, false)

	// Create a second node with its own bond address
	node2Address := GetRandomBech32Addr()
	bond2Address := common.Address(GetRandomBech32Addr().String())

	// Create second node account
	node2Account := NodeAccount{
		NodeAddress: node2Address,
		BondAddress: bond2Address,
		Status:      NodeActive,
		Maintenance: false,
	}
	maintHandler.AddNodeAccount(node2Account)

	bond2AccAddress, err := bond2Address.AccAddress()
	c.Assert(err, IsNil)

	// Test 3: Second node's bond address can toggle its own node
	msg2 := types.NewMsgMaint(node2Address, bond2AccAddress)
	err = maintHandler.Handle(ctx, *msg2)
	c.Assert(err, IsNil)
	c.Assert(maintHandler.nodes[node2Address.String()].Maintenance, Equals, true)

	// Test 4: Error getting node account
	maintHandler.failGetNodeAccount = true
	err = maintHandler.Handle(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*unable to find account.*")

	// Test 5: Error setting node account
	maintHandler.failGetNodeAccount = false
	maintHandler.failSetNodeAccount = true
	err = maintHandler.Handle(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*fail to save.*")

	// Test 6: Invalid node address
	invalidAddr := GetRandomBech32Addr()
	msg = types.NewMsgMaint(invalidAddr, bondAccAddress)
	maintHandler.failGetNodeAccount = false
	maintHandler.failSetNodeAccount = false
	err = maintHandler.Handle(ctx, *msg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, fmt.Sprintf(".*unable to find account: %s.*", invalidAddr))

	// Test 7: A node's bond address cannot toggle another node's maintenance
	wrongMsg := types.NewMsgMaint(nodeAddress, bond2AccAddress)
	err = maintHandler.Validate(ctx, *wrongMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*is not authorized.*")
}

// TestValidation checks that the validation logic for MaintHandler works correctly
func (s *HandlerMaintSuite) TestValidation(c *C) {
	ctx, mgr := setupManagerForTest(c)

	// Setup test data
	nodeAccount := GetRandomValidatorNode(NodeActive)
	c.Assert(mgr.Keeper().SetNodeAccount(ctx, nodeAccount), IsNil)

	// Extract required addresses
	nodeAddr := nodeAccount.NodeAddress

	bondAddr, err := nodeAccount.BondAddress.AccAddress()
	c.Assert(err, IsNil)

	// Create random addresses for testing invalid cases
	randomAddr := GetRandomBech32Addr()

	// Test case 1: Valid - bond address is authorized to toggle maintenance
	msg := types.NewMsgMaint(nodeAddr, bondAddr)
	handler := NewMaintHandler(mgr)
	err = handler.validate(ctx, *msg)
	c.Assert(err, IsNil)

	// Test case 2: Invalid - random address cannot toggle maintenance for node
	invalidMsg := types.NewMsgMaint(nodeAddr, randomAddr)
	err = handler.validate(ctx, *invalidMsg)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Matches, ".*is not authorized.*")
}
