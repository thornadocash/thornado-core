package common

import "testing"

func TestNewAddressAcceptsInternalSentinels(t *testing.T) {
	bondEscrow, err := NewAddress("bond_escrow")
	if err != nil {
		t.Fatalf("bond escrow address rejected: %v", err)
	}
	if !bondEscrow.IsBondEscrow() {
		t.Fatalf("expected bond escrow address, got %s", bondEscrow)
	}

	noop, err := NewAddress("noop")
	if err != nil {
		t.Fatalf("noop address rejected: %v", err)
	}
	if !noop.IsNoop() {
		t.Fatalf("expected noop address, got %s", noop)
	}
}
