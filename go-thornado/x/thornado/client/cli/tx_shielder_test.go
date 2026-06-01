package cli

import "testing"

func TestShielderTxCommandSurfaceHasAtomicSettlementOnly(t *testing.T) {
	cmd := GetCmdShielder()
	for _, subcmd := range cmd.Commands() {
		switch subcmd.Name() {
		case "settle-deposit", "settle-fees":
			t.Fatalf("standalone settlement command still exposed: %s", subcmd.Name())
		}
	}
	if _, _, err := cmd.Find([]string{"split"}); err != nil {
		t.Fatal("missing atomic deposit split command")
	}
	if _, _, err := cmd.Find([]string{"auction-split"}); err != nil {
		t.Fatal("missing atomic sale split command")
	}
	if _, _, err := cmd.Find([]string{"split-fees"}); err != nil {
		t.Fatal("missing atomic fee split command")
	}
}
