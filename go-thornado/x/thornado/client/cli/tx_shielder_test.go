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
	if _, _, err := cmd.Find([]string{"shield"}); err != nil {
		t.Fatal("missing atomic deposit shield command")
	}
	if _, _, err := cmd.Find([]string{"node-sale-shield"}); err != nil {
		t.Fatal("missing atomic node sale shield command")
	}
	if _, _, err := cmd.Find([]string{"shield-fees"}); err != nil {
		t.Fatal("missing atomic fee shield command")
	}
}
