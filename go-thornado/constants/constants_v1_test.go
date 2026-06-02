package constants

import "testing"

func TestDepositPowDifficultyDefaultIsEnabled(t *testing.T) {
	if got := NewConfigValue().GetInt64Value(Deposit_PowDifficultyMin); got <= 0 {
		t.Fatalf("expected deposit PoW difficulty to be enabled by default, got %d", got)
	}
}
