package thornado

import "testing"

func TestWeightedPowPercentileUsesDepositWeight(t *testing.T) {
	samples := []depositPowRetargetSample{
		{durationMs: 60_000, weightSats: 1_000},
		{durationMs: 10_000, weightSats: 9_000},
	}

	got := weightedPowPercentile(samples, 10_000, 90)
	if got != 10_000 {
		t.Fatalf("expected weighted p90 to follow deposited weight, got %d", got)
	}
}

func TestRetargetPowDifficultyMovesOneBoundedStep(t *testing.T) {
	if got := retargetPowDifficulty(20, 2_000, 10_000, 1); got != 21 {
		t.Fatalf("expected fast p90 to increase difficulty by one, got %d", got)
	}
	if got := retargetPowDifficulty(20, 60_000, 10_000, 1); got != 19 {
		t.Fatalf("expected slow p90 to decrease difficulty by one, got %d", got)
	}
	if got := retargetPowDifficulty(20, 11_000, 10_000, 1); got != 20 {
		t.Fatalf("expected near-target p90 to keep difficulty, got %d", got)
	}
}
