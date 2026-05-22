package keeperv1

import (
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/thornadocash/go-thornado/common"
	"github.com/thornadocash/go-thornado/common/cosmos"
	"github.com/thornadocash/go-thornado/x/thornado/types"
	. "gopkg.in/check.v1"
)

type KeeperUpgradeSuite struct{}

var _ = Suite(&KeeperUpgradeSuite{})

func (s *KeeperUpgradeSuite) TestUpgrade(c *C) {
	ctx, k := setupKeeperForTest(c)

	// Add node accounts
	na1 := GetRandomNode(NodeActive)
	na1.Bond = cosmos.NewUint(100 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na1), IsNil)
	na2 := GetRandomNode(NodeActive)
	na2.Bond = cosmos.NewUint(200 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na2), IsNil)
	na3 := GetRandomNode(NodeActive)
	na3.Bond = cosmos.NewUint(300 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na3), IsNil)
	na4 := GetRandomNode(NodeActive)
	na4.Bond = cosmos.NewUint(400 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na4), IsNil)
	na5 := GetRandomNode(NodeActive)
	na5.Bond = cosmos.NewUint(500 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na5), IsNil)
	na6 := GetRandomNode(NodeActive)
	na6.Bond = cosmos.NewUint(600 * common.One)
	c.Assert(k.SetNodeAccount(ctx, na6), IsNil)

	const (
		upgradeName = "1.2.3"
		upgradeInfo = "scheduled upgrade"
	)

	upgradeHeight := ctx.BlockHeight() + 100

	// propose upgrade
	c.Assert(k.ProposeUpgrade(ctx, upgradeName, types.UpgradeProposal{
		Height: upgradeHeight,
		Info:   upgradeInfo,
	}), IsNil)

	k.ApproveUpgrade(ctx, na1.NodeAddress, upgradeName)
	k.ApproveUpgrade(ctx, na2.NodeAddress, upgradeName)
	k.ApproveUpgrade(ctx, na3.NodeAddress, upgradeName)
	k.ApproveUpgrade(ctx, na4.NodeAddress, upgradeName)
	k.ApproveUpgrade(ctx, na5.NodeAddress, upgradeName)

	var proposals int
	var proposal types.Upgrade
	var proposalKey []byte
	pIter := k.GetUpgradeProposalIterator(ctx)
	for ; pIter.Valid(); pIter.Next() {
		proposals++
		k.cdc.MustUnmarshal(pIter.Value(), &proposal)
		proposalKey = pIter.Key()
	}

	c.Assert(proposals, Equals, 1)
	c.Assert(proposalKey, DeepEquals, []byte("upgr_props/1.2.3"))
	c.Assert(proposal.Height, Equals, upgradeHeight)
	c.Assert(proposal.Info, Equals, upgradeInfo)

	p, err := k.GetProposedUpgrade(ctx, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(p.Height, Equals, upgradeHeight)
	c.Assert(p.Info, Equals, upgradeInfo)

	var approves int
	vIter := k.GetUpgradeVoteIterator(ctx, upgradeName)
	for ; vIter.Valid(); vIter.Next() {
		c.Assert(vIter.Value(), DeepEquals, []byte{0x1})
		approves++
	}

	c.Assert(approves, Equals, 5)

	c.Assert(k.ScheduleUpgrade(ctx, upgradetypes.Plan{
		Name:   upgradeName,
		Height: upgradeHeight,
		Info:   upgradeInfo,
	}), IsNil)

	plan, err := k.GetUpgradePlan(ctx)
	c.Assert(err, Equals, nil)
	c.Assert(plan.Name, Equals, upgradeName)
	c.Assert(plan.Height, Equals, upgradeHeight)
	c.Assert(plan.Info, Equals, upgradeInfo)

	uq, err := UpgradeApprovedByMajority(ctx, k, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(uq.Approved, Equals, true)
	c.Assert(uq.ApprovingVals, Equals, 5)
	c.Assert(uq.TotalActive, Equals, 6)
	c.Assert(uq.NeededForQuorum, Equals, 0)

	k.RejectUpgrade(ctx, na2.NodeAddress, upgradeName)

	uq, err = UpgradeApprovedByMajority(ctx, k, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(uq.Approved, Equals, true)
	c.Assert(uq.ApprovingVals, Equals, 4)
	c.Assert(uq.TotalActive, Equals, 6)
	c.Assert(uq.NeededForQuorum, Equals, 0)

	k.RejectUpgrade(ctx, na1.NodeAddress, upgradeName)

	err = k.ClearUpgradePlan(ctx)
	c.Assert(err, IsNil)
	_, err = k.GetUpgradePlan(ctx)
	c.Assert(err, Equals, upgradetypes.ErrNoUpgradePlanFound)

	uq, err = UpgradeApprovedByMajority(ctx, k, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(uq.Approved, Equals, false)
	c.Assert(uq.ApprovingVals, Equals, 3)
	c.Assert(uq.TotalActive, Equals, 6)
	c.Assert(uq.NeededForQuorum, Equals, 1)

	k.RejectUpgrade(ctx, na3.NodeAddress, upgradeName)
	k.RejectUpgrade(ctx, na4.NodeAddress, upgradeName)
	k.RejectUpgrade(ctx, na5.NodeAddress, upgradeName)
	k.RejectUpgrade(ctx, na6.NodeAddress, upgradeName)

	uq, err = UpgradeApprovedByMajority(ctx, k, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(uq.Approved, Equals, false)
	c.Assert(uq.ApprovingVals, Equals, 0)
	c.Assert(uq.TotalActive, Equals, 6)
	c.Assert(uq.NeededForQuorum, Equals, 4)

	var rejects int
	vIter = k.GetUpgradeVoteIterator(ctx, upgradeName)
	for ; vIter.Valid(); vIter.Next() {
		c.Assert(vIter.Value(), DeepEquals, []byte{0xFF})
		rejects++
	}

	c.Assert(rejects, Equals, 6)

	ctx = ctx.WithBlockHeight(upgradeHeight + 1)

	c.Assert(k.RemoveExpiredUpgradeProposals(ctx), IsNil)

	u, err := k.GetProposedUpgrade(ctx, upgradeName)
	c.Assert(err, IsNil)
	c.Assert(u, IsNil)

	proposals = 0
	pIter = k.GetUpgradeProposalIterator(ctx)
	for ; pIter.Valid(); pIter.Next() {
		proposals++
	}

	c.Assert(proposals, Equals, 0)

	var votes int
	vIter = k.GetUpgradeVoteIterator(ctx, upgradeName)
	for ; vIter.Valid(); vIter.Next() {
		votes++
	}

	c.Assert(votes, Equals, 0)
}
