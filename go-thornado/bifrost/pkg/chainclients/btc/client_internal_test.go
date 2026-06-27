package btc

import "testing"

func TestEstimatedAverageTxSizeFallback(t *testing.T) {
	var client Client

	if got := client.estimatedAverageTxSize(); got != 1000 {
		t.Fatalf("expected default transaction size 1000, got %d", got)
	}

	client.cfg.UTXO.EstimatedAverageTxSize = 1200
	if got := client.estimatedAverageTxSize(); got != 1200 {
		t.Fatalf("expected configured transaction size 1200, got %d", got)
	}
}
