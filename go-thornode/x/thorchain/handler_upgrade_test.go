package thorchain

import (
	"errors"
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/codec"
	se "github.com/cosmos/cosmos-sdk/types/errors"
	. "gopkg.in/check.v1"

	"gitlab.com/thorchain/thornode/v3/common/cosmos"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/keeper"
	"gitlab.com/thorchain/thornode/v3/x/thorchain/types"
)

type HandlerUpgradeSuite struct{}

type TestUpgradeKeeper struct {
	keeper.KVStoreDummy
	activeAccounts   []NodeAccount
	failNodeAccount  NodeAccount
	emptyNodeAccount NodeAccount
	vaultNodeAccount NodeAccount
	proposedUpgrades map[string]*types.UpgradeProposal
	votes            map[string]map[string]bool

	scheduledPlan *upgradetypes.Plan
}

func (k *TestUpgradeKeeper) GetNodeAccount(_ cosmos.Context, addr cosmos.AccAddress) (NodeAccount, error) {
	if k.failNodeAccount.NodeAddress.Equals(addr) {
		return NodeAccount{}, errKaboom
	}
	if k.emptyNodeAccount.NodeAddress.Equals(addr) {
		return NodeAccount{}, nil
	}
	if k.vaultNodeAccount.NodeAddress.Equals(addr) {
		return NodeAccount{Type: NodeTypeVault}, nil
	}

	for _, na := range k.activeAccounts {
		if na.NodeAddress.Equals(addr) {
			return na, nil
		}
	}

	return NodeAccount{}, errKaboom
}

func (k *TestUpgradeKeeper) ProposeUpgrade(_ cosmos.Context, name string, upgrade types.UpgradeProposal) error {
	k.proposedUpgrades[name] = &upgrade
	return nil
}

func (k *TestUpgradeKeeper) GetProposedUpgrade(_ cosmos.Context, name string) (*types.UpgradeProposal, error) {
	return k.proposedUpgrades[name], nil
}

func (k *TestUpgradeKeeper) GetUpgradeProposalIterator(_ cosmos.Context) cosmos.Iterator {
	names := make([]string, 0, len(k.proposedUpgrades))
	proposals := make([]types.UpgradeProposal, 0, len(k.proposedUpgrades))
	for name, p := range k.proposedUpgrades {
		names = append(names, name)
		proposals = append(proposals, *p)
	}
	return newMockUpgradeProposalIterator(k.Cdc(), names, proposals)
}

func (k *TestUpgradeKeeper) ApproveUpgrade(_ cosmos.Context, addr cosmos.AccAddress, name string) {
	if _, found := k.votes[name]; !found {
		k.votes[name] = make(map[string]bool)
	}
	k.votes[name][addr.String()] = true
}

func (k *TestUpgradeKeeper) RejectUpgrade(_ cosmos.Context, addr cosmos.AccAddress, name string) {
	if _, found := k.votes[name]; !found {
		k.votes[name] = make(map[string]bool)
	}
	k.votes[name][addr.String()] = false
}

func (k *TestUpgradeKeeper) GetNodeAccountIterator(_ cosmos.Context) cosmos.Iterator {
	nas := make([]NodeAccount, 0, len(k.activeAccounts)+2)
	nas = append(nas, k.activeAccounts...)
	nas = append(nas, k.vaultNodeAccount, k.emptyNodeAccount)
	return newMockNodeAccountIterator(k.Cdc(), nas)
}

func (k *TestUpgradeKeeper) ListActiveValidators(_ cosmos.Context) (NodeAccounts, error) {
	return k.activeAccounts, nil
}

func (k *TestUpgradeKeeper) GetUpgradeVote(_ cosmos.Context, addr cosmos.AccAddress, name string) (bool, error) {
	propVotes, found := k.votes[name]
	if !found {
		return false, se.ErrUnknownRequest
	}
	vote, found := propVotes[addr.String()]
	if !found {
		return false, se.ErrUnknownRequest
	}
	return vote, nil
}

func (k *TestUpgradeKeeper) GetUpgradeVoteIterator(_ cosmos.Context, name string) cosmos.Iterator {
	propVotes, found := k.votes[name]
	if !found {
		panic("upgrade not found")
	}

	votes := make([]mockVote, 0, len(k.votes))
	for addr, approve := range propVotes {
		acc, err := cosmos.AccAddressFromBech32(addr)
		if err != nil {
			panic(err)
		}
		votes = append(votes, mockVote{acc: acc, approve: approve})
	}
	return newMockUpgradeVoteIterator(votes)
}

func (k *TestUpgradeKeeper) ScheduleUpgrade(_ cosmos.Context, plan upgradetypes.Plan) error {
	k.scheduledPlan = &plan
	return nil
}

func (k *TestUpgradeKeeper) ClearUpgradePlan(_ cosmos.Context) error {
	k.scheduledPlan = nil
	return nil
}

func (k *TestUpgradeKeeper) GetUpgradePlan(_ cosmos.Context) (upgradetypes.Plan, error) {
	if k.scheduledPlan != nil {
		return *k.scheduledPlan, nil
	}
	return upgradetypes.Plan{}, upgradetypes.ErrNoUpgradePlanFound
}

var _ = Suite(&HandlerUpgradeSuite{})

func (s *HandlerUpgradeSuite) TestUpgrade(c *C) {
	ctx, _ := setupKeeperForTest(c)

	const (
		upgradeName = "1.2.3"
		upgradeInfo = "scheduled upgrade"
	)

	upgradeHeight := ctx.BlockHeight() + 100

	keeper := &TestUpgradeKeeper{
		failNodeAccount:  GetRandomValidatorNode(NodeActive),
		emptyNodeAccount: GetRandomValidatorNode(NodeStandby),
		vaultNodeAccount: GetRandomVaultNode(NodeActive),
		votes:            make(map[string]map[string]bool),
		proposedUpgrades: make(map[string]*types.UpgradeProposal),
	}

	// add some active accounts
	for i := 0; i < 10; i++ {
		keeper.activeAccounts = append(keeper.activeAccounts, GetRandomValidatorNode(NodeActive))
	}

	mgr := NewDummyMgrWithKeeper(keeper)

	handler := NewProposeUpgradeHandler(mgr)

	// invalid height
	msg := NewMsgProposeUpgrade(upgradeName, ctx.BlockHeight(), upgradeInfo, keeper.activeAccounts[0].NodeAddress)
	result, err := handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)

	// invalid msg
	msg = &MsgProposeUpgrade{}
	result, err = handler.Run(ctx, msg)
	c.Assert(result, IsNil)
	c.Assert(err, NotNil)

	// fail to get node account should fail
	msg1 := NewMsgProposeUpgrade(upgradeName, upgradeHeight, upgradeInfo, keeper.failNodeAccount.NodeAddress)
	result, err = handler.Run(ctx, msg1)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)

	// node account empty should fail
	msg2 := NewMsgProposeUpgrade(upgradeName, upgradeHeight, upgradeInfo, keeper.emptyNodeAccount.NodeAddress)
	result, err = handler.Run(ctx, msg2)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnauthorized), Equals, true)

	// vault node should fail
	msg3 := NewMsgProposeUpgrade(upgradeName, upgradeHeight, upgradeInfo, keeper.vaultNodeAccount.NodeAddress)
	result, err = handler.Run(ctx, msg3)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnauthorized), Equals, true)

	// happy path to get the upgrade proposed
	msg4 := NewMsgProposeUpgrade(upgradeName, upgradeHeight, upgradeInfo, keeper.activeAccounts[0].NodeAddress)
	result, err = handler.Run(ctx, msg4)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(keeper.scheduledPlan, IsNil)

	// proposed upgrade with same name should fail
	msg5 := NewMsgProposeUpgrade(upgradeName, upgradeHeight, upgradeInfo, keeper.activeAccounts[1].NodeAddress)
	result, err = handler.Run(ctx, msg5)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// second proposal with different name by same proposer should work
	msg6 := NewMsgProposeUpgrade("1.2.4", upgradeHeight, upgradeInfo, keeper.activeAccounts[0].NodeAddress)
	result, err = handler.Run(ctx, msg6)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(keeper.scheduledPlan, IsNil)

	// third proposal with different name by same proposer should work
	msg7 := NewMsgProposeUpgrade("1.2.5", upgradeHeight, upgradeInfo, keeper.activeAccounts[0].NodeAddress)
	result, err = handler.Run(ctx, msg7)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)
	c.Assert(keeper.scheduledPlan, IsNil)

	// fourth proposal with different name by same proposer should fail
	msg8 := NewMsgProposeUpgrade("1.2.6", upgradeHeight, upgradeInfo, keeper.activeAccounts[0].NodeAddress)
	result, err = handler.Run(ctx, msg8)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	approveHandler := NewApproveUpgradeHandler(mgr)

	// voting when already approved should fail
	msg9 := NewMsgApproveUpgrade(upgradeName, keeper.activeAccounts[0].NodeAddress)
	result, err = approveHandler.Run(ctx, msg9)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// voting on non-existent upgrade should fail
	msg10 := NewMsgApproveUpgrade("1.2.6", keeper.activeAccounts[1].NodeAddress)
	result, err = approveHandler.Run(ctx, msg10)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// vote for upgrade by 1 less than 2/3 of active accounts
	for i, na := range keeper.activeAccounts {
		if i == 0 {
			// skip the proposer because they already approved by proposing
			continue
		}
		if i == 6 {
			// break after 6 approve votes so that we are 1 less than 2/3
			break
		}
		msg := NewMsgApproveUpgrade(upgradeName, na.NodeAddress)
		result, err = approveHandler.Run(ctx, msg)
		c.Assert(err, IsNil)
		c.Assert(result, NotNil)
	}

	// upgrade should still not be scheduled
	c.Assert(keeper.scheduledPlan, IsNil)

	// vote for upgrade by one more active account
	msg11 := NewMsgApproveUpgrade(upgradeName, keeper.activeAccounts[8].NodeAddress)
	result, err = approveHandler.Run(ctx, msg11)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// upgrade should now be scheduled
	c.Assert(keeper.scheduledPlan, NotNil)

	rejectHandler := NewRejectUpgradeHandler(mgr)

	// reject upgrade by one of the active accounts to drop below 2/3
	msg12 := NewMsgRejectUpgrade(upgradeName, keeper.activeAccounts[4].NodeAddress)
	result, err = rejectHandler.Run(ctx, msg12)
	c.Assert(err, IsNil)
	c.Assert(result, NotNil)

	// rejecting when already rejected should fail
	msg13 := NewMsgRejectUpgrade(upgradeName, keeper.activeAccounts[4].NodeAddress)
	result, err = rejectHandler.Run(ctx, msg13)
	c.Assert(err, NotNil)
	c.Assert(result, IsNil)
	c.Assert(errors.Is(err, se.ErrUnknownRequest), Equals, true)

	// upgrade should now be cleared
	c.Assert(keeper.scheduledPlan, IsNil)
}

func (s *HandlerUpgradeSuite) TestApprovedUpgradeScheduledAfterBlockingPlanCleared(c *C) {
	ctx, _ := setupKeeperForTest(c)

	const (
		upgradeA = "1.2.3"
		upgradeB = "1.2.4"
	)

	upgradeHeight := ctx.BlockHeight() + 100

	keeper := &TestUpgradeKeeper{
		failNodeAccount:  GetRandomValidatorNode(NodeActive),
		emptyNodeAccount: GetRandomValidatorNode(NodeStandby),
		vaultNodeAccount: GetRandomVaultNode(NodeActive),
		votes:            make(map[string]map[string]bool),
		proposedUpgrades: make(map[string]*types.UpgradeProposal),
	}

	for i := 0; i < 10; i++ {
		keeper.activeAccounts = append(keeper.activeAccounts, GetRandomValidatorNode(NodeActive))
	}

	mgr := NewDummyMgrWithKeeper(keeper)
	proposeHandler := NewProposeUpgradeHandler(mgr)
	approveHandler := NewApproveUpgradeHandler(mgr)
	rejectHandler := NewRejectUpgradeHandler(mgr)

	// Propose upgrade A and get it approved + scheduled
	msg := NewMsgProposeUpgrade(upgradeA, upgradeHeight, "upgrade A", keeper.activeAccounts[0].NodeAddress)
	_, err := proposeHandler.Run(ctx, msg)
	c.Assert(err, IsNil)

	for i := 1; i <= 6; i++ {
		_, err = approveHandler.Run(ctx, NewMsgApproveUpgrade(upgradeA, keeper.activeAccounts[i].NodeAddress))
		c.Assert(err, IsNil)
	}
	c.Assert(keeper.scheduledPlan, NotNil)
	c.Assert(keeper.scheduledPlan.Name, Equals, upgradeA)

	// Propose upgrade B and get it approved (but it can't schedule because A is blocking)
	msg = NewMsgProposeUpgrade(upgradeB, upgradeHeight+10, "upgrade B", keeper.activeAccounts[0].NodeAddress)
	_, err = proposeHandler.Run(ctx, msg)
	c.Assert(err, IsNil)

	for i := 1; i <= 6; i++ {
		_, err = approveHandler.Run(ctx, NewMsgApproveUpgrade(upgradeB, keeper.activeAccounts[i].NodeAddress))
		c.Assert(err, IsNil)
	}

	// A should still be scheduled (B is approved but blocked)
	c.Assert(keeper.scheduledPlan, NotNil)
	c.Assert(keeper.scheduledPlan.Name, Equals, upgradeA)

	// Now reject upgrade A enough to drop it below majority and clear its plan
	for i := 0; i <= 4; i++ {
		_, err = rejectHandler.Run(ctx, NewMsgRejectUpgrade(upgradeA, keeper.activeAccounts[i].NodeAddress))
		c.Assert(err, IsNil)
	}

	// Upgrade B should now be auto-scheduled after A was cleared
	c.Assert(keeper.scheduledPlan, NotNil)
	c.Assert(keeper.scheduledPlan.Name, Equals, upgradeB)
}

type mockNodeAccountIterator struct {
	cdc          codec.BinaryCodec
	nodeAccounts []NodeAccount
	i            int
}

func newMockNodeAccountIterator(cdc codec.BinaryCodec, nodeAccounts []NodeAccount) *mockNodeAccountIterator {
	return &mockNodeAccountIterator{cdc: cdc, nodeAccounts: nodeAccounts}
}

func (it *mockNodeAccountIterator) Domain() (start, end []byte) { return nil, nil }
func (it *mockNodeAccountIterator) Valid() bool                 { return it.i < len(it.nodeAccounts) }
func (it *mockNodeAccountIterator) Next()                       { it.i++ }
func (it *mockNodeAccountIterator) Key() (key []byte)           { return nil }
func (it *mockNodeAccountIterator) Value() (value []byte) {
	bz, err := it.cdc.Marshal(&it.nodeAccounts[it.i])
	if err != nil {
		panic(fmt.Errorf("failed to marshal: %w", err))
	}
	return bz
}
func (it *mockNodeAccountIterator) Error() error { return nil }
func (it *mockNodeAccountIterator) Close() error { return nil }

type mockUpgradeVoteIterator struct {
	upgradeVotes []mockVote
	i            int
}

type mockVote struct {
	acc     cosmos.AccAddress
	approve bool
}

func newMockUpgradeVoteIterator(upgradeVotes []mockVote) *mockUpgradeVoteIterator {
	return &mockUpgradeVoteIterator{upgradeVotes: upgradeVotes}
}

func (it *mockUpgradeVoteIterator) Domain() (start, end []byte) { return nil, nil }
func (it *mockUpgradeVoteIterator) Valid() bool                 { return it.i < len(it.upgradeVotes) }
func (it *mockUpgradeVoteIterator) Next()                       { it.i++ }
func (it *mockUpgradeVoteIterator) Key() (key []byte) {
	return it.upgradeVotes[it.i].acc.Bytes()
}

func (it *mockUpgradeVoteIterator) Value() (value []byte) {
	if it.upgradeVotes[it.i].approve {
		return []byte{0x01}
	}
	return []byte{0x00}
}

func (it *mockUpgradeVoteIterator) Error() error { return nil }
func (it *mockUpgradeVoteIterator) Close() error { return nil }

type mockUpgradeProposalIterator struct {
	cdc       codec.BinaryCodec
	names     []string
	proposals []types.UpgradeProposal
	i         int
}

func newMockUpgradeProposalIterator(cdc codec.BinaryCodec, names []string, proposals []types.UpgradeProposal) *mockUpgradeProposalIterator {
	return &mockUpgradeProposalIterator{cdc: cdc, names: names, proposals: proposals}
}

func (it *mockUpgradeProposalIterator) Domain() (start, end []byte) { return nil, nil }
func (it *mockUpgradeProposalIterator) Valid() bool                 { return it.i < len(it.proposals) }
func (it *mockUpgradeProposalIterator) Next()                       { it.i++ }
func (it *mockUpgradeProposalIterator) Key() (key []byte) {
	return []byte("upgr_props/" + it.names[it.i])
}

func (it *mockUpgradeProposalIterator) Value() (value []byte) {
	bz, err := it.cdc.Marshal(&it.proposals[it.i])
	if err != nil {
		panic(fmt.Errorf("failed to marshal: %w", err))
	}
	return bz
}
func (it *mockUpgradeProposalIterator) Error() error { return nil }
func (it *mockUpgradeProposalIterator) Close() error { return nil }
