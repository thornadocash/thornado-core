package cli

import "testing"

func TestNodeOperatorQueryCommandSurface(t *testing.T) {
	cmd := GetCmdNodeQuery()
	paths := [][]string{
		{"status"},
		{"list"},
		{"metrics"},
		{"slot"},
		{"bond"},
		{"fees"},
		{"fee-pool"},
		{"auctions"},
		{"auction"},
		{"auction-bids"},
		{"bid"},
		{"upgrades"},
		{"upgrade"},
		{"upgrade-votes"},
	}
	for _, path := range paths {
		if _, _, err := cmd.Find(path); err != nil {
			t.Fatalf("missing node query command %v: %v", path, err)
		}
	}
}

func TestNodeOperatorQueryCommandOnRoot(t *testing.T) {
	cmd := GetQueryCmd()
	if _, _, err := cmd.Find([]string{"node", "metrics"}); err != nil {
		t.Fatalf("missing root node query command: %v", err)
	}
}
