package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestFindCountToRemoveWithReplacementsAllowsOneInOneOutAtBFTMinimum(t *testing.T) {
	active := NodeAccounts{
		{LeaveScore: 10},
		{},
		{},
		{},
	}

	if got := findCountToRemove(active); got != 0 {
		t.Fatalf("findCountToRemove() = %d, want 0 without replacement context", got)
	}
	if got := findCountToRemoveWithReplacements(active, 1, 4, 4); got != 1 {
		t.Fatalf("findCountToRemoveWithReplacements() = %d, want 1", got)
	}
}

func TestFindCountToRemoveWithReplacementsRequiresReplacement(t *testing.T) {
	active := NodeAccounts{
		{LeaveScore: 10},
		{},
		{},
		{},
	}

	if got := findCountToRemoveWithReplacements(active, 0, 4, 4); got != 0 {
		t.Fatalf("findCountToRemoveWithReplacements() = %d, want 0", got)
	}
}

func TestMarkLowBondActorKeepsBondedActiveNode(t *testing.T) {
	k := newShielderFlowTestKeeper()
	ctx := flowTestContext().WithBlockHeight(100)
	mgr := newNodeMgr(k, nil, nil, nil)

	bonded := mustSetTestNode(t, ctx, k, NodeActive, 10, 100_000_000)
	lowNodes := []NodeAccount{
		mustSetTestNode(t, ctx, k, NodeActive, 20, 0),
		mustSetTestNode(t, ctx, k, NodeActive, 30, 0),
		mustSetTestNode(t, ctx, k, NodeActive, 40, 0),
	}
	mustSetTestNode(t, ctx, k, NodeSelected, 50, 0)

	marked, err := mgr.markLowBondActor(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !marked {
		t.Fatal("expected low-bond actor to be marked")
	}

	gotBonded, err := k.GetNodeAccount(ctx, bonded.NodeAddress)
	if err != nil {
		t.Fatal(err)
	}
	if gotBonded.LeaveScore != 0 {
		t.Fatalf("bonded node was marked to leave: %d", gotBonded.LeaveScore)
	}

	lowMarked := false
	for _, node := range lowNodes {
		got, err := k.GetNodeAccount(ctx, node.NodeAddress)
		if err != nil {
			t.Fatal(err)
		}
		lowMarked = lowMarked || got.LeaveScore > 0
	}
	if !lowMarked {
		t.Fatal("no zero-bond active node was marked to leave")
	}
}

func TestMarkLowBondActorRejectsLowerBondReplacement(t *testing.T) {
	k := newShielderFlowTestKeeper()
	ctx := flowTestContext().WithBlockHeight(100)
	mgr := newNodeMgr(k, nil, nil, nil)

	active := mustSetTestNode(t, ctx, k, NodeActive, 10, 10)
	mustSetTestNode(t, ctx, k, NodeActive, 20, 20)
	mustSetTestNode(t, ctx, k, NodeActive, 30, 30)
	mustSetTestNode(t, ctx, k, NodeActive, 40, 40)
	mustSetTestNode(t, ctx, k, NodeSelected, 50, 1)

	marked, err := mgr.markLowBondActor(ctx, 4)
	if err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("lower-bond selected node should not displace a higher-bond active node")
	}

	got, err := k.GetNodeAccount(ctx, active.NodeAddress)
	if err != nil {
		t.Fatal(err)
	}
	if got.LeaveScore != 0 {
		t.Fatalf("active node was marked despite lower-bond replacement: %d", got.LeaveScore)
	}
}

func mustSetTestNode(t *testing.T, ctx cosmos.Context, k *shielderFlowTestKeeper, status NodeStatus, statusSince int64, bond uint64) NodeAccount {
	t.Helper()
	node := GetRandomNode(status)
	node.StatusSince = statusSince
	node.Bond = cosmos.NewUint(bond)
	node.LeaveScore = 0
	if err := k.SetNodeAccount(ctx, node); err != nil {
		t.Fatal(err)
	}
	return node
}
