package thornado

import "testing"

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
