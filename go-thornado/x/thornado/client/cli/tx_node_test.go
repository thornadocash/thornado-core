package cli

import "testing"

func TestNodeOperatorTxCommandSurface(t *testing.T) {
	cmd := GetCmdNodeOperator()
	paths := [][]string{
		{"register"},
		{"set-keys"},
		{"set-ip"},
		{"set-version"},
		{"maint"},
		{"leave"},
		{"rotate-operator"},
		{"bond"},
		{"bond-provider", "bond"},
		{"bond-provider", "fees"},
		{"fees", "shield"},
		{"bid", "create"},
		{"sale", "create"},
		{"sale", "select-bid"},
		{"sale", "shield"},
		{"upgrade", "propose"},
		{"upgrade", "approve"},
		{"upgrade", "reject"},
	}
	for _, path := range paths {
		if _, _, err := cmd.Find(path); err != nil {
			t.Fatalf("missing node tx command %v: %v", path, err)
		}
	}
}

func TestNodeOperatorTxCommandOnRoot(t *testing.T) {
	cmd := GetTxCmd()
	if _, _, err := cmd.Find([]string{"node", "maint"}); err != nil {
		t.Fatalf("missing root node operator command: %v", err)
	}
}
