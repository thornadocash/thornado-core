package thornado

import (
	"testing"

	"github.com/thornadocash/go-thornado/common/cosmos"
)

func TestLeaveHandlerMarksNodeRequestedToLeave(t *testing.T) {
	k := newShielderFlowTestKeeper()
	ctx := flowTestContext().WithBlockHeight(123).WithEventManager(cosmos.NewEventManager())
	node := GetRandomNode(NodeActive)
	node.RequestedToLeave = false
	node.LeaveScore = 0
	if err := k.SetNodeAccount(ctx, node); err != nil {
		t.Fatal(err)
	}

	handler := NewLeaveHandler(newShielderFlowTestManager(k))
	if _, err := handler.Run(ctx, NewMsgLeave(node.NodeAddress, node.NodeAddress)); err != nil {
		t.Fatal(err)
	}

	got, err := k.GetNodeAccount(ctx, node.NodeAddress)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequestedToLeave {
		t.Fatal("node was not marked requested-to-leave")
	}
	if got.LeaveScore != 123 {
		t.Fatalf("unexpected leave score: got %d want %d", got.LeaveScore, 123)
	}
}
